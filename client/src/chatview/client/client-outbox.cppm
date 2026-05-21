module;

#include <chrono>
#include <condition_variable>
#include <ctime>
#include <expected>
#include <mutex>
#include <optional>
#include <string>
#include <thread>
#include <utility>
#include <vector>

#include <coco/task/task.hpp>
#include <sqlite_orm/sqlite_orm.h>

export module chatview.client:outbox;

import :types;
import :detail;
import :rpc;
import :bridge;
import chatview.proto.chat;
import chatview.storage.cache;

namespace chatview::client
{

export class OutboxManager
{
public:
    OutboxManager(storage::CacheDatabase& cache, RpcClient& rpc, NativeBridge& bridge) :
        cache_(cache),
        rpc_(rpc),
        bridge_(bridge)
    {
    }

    ~OutboxManager()
    {
        stop();
    }

    auto send_message(
        this OutboxManager& self,
        const std::string& receiver,
        const std::string& text,
        const std::string& client_msg_id) -> std::expected<SendMessageResult, std::string>
    {
        {
            const auto timestamp = detail::now_iso();
            auto message = storage::MessageRow{
                .id = client_msg_id,
                .client_msg_id = client_msg_id,
                .peer_pub_key = receiver,
                .sender_pub_key = self.rpc_.public_key(),
                .text = text,
                .timestamp = timestamp,
                .server_seq = std::nullopt,
                .delivery = detail::string_to_delivery("pending"),
                .error = std::nullopt,
                .created_at = timestamp,
            };
            auto outbox = storage::OutboxRow{
                .id = client_msg_id,
                .receiver_pub_key = receiver,
                .text = text,
                .attempts = 1,
                .next_retry_at = std::nullopt,
                .error = std::nullopt,
                .status = 1,
                .created_at = timestamp,
            };
            self.cache_.with_storage([&](auto& store) {
                store.replace(message);
                store.replace(outbox);
            });
        }

        auto response = self.rpc_.send_message(receiver, text, client_msg_id);
        if (!response) {
            self.mark_failed(client_msg_id, response.error());
            return std::unexpected{response.error()};
        }

        self.mark_sent(client_msg_id, receiver, *response);
        return SendMessageResult{
            .messageId = response->message_id(),
            .timestamp = response->timestamp(),
            .deduplicated = response->deduplicated() ? std::optional<bool>{true} : std::nullopt,
        };
    }

    auto send_message_async(
        this OutboxManager& self,
        std::string receiver,
        std::string text,
        std::string client_msg_id) -> coco::task<std::expected<SendMessageResult, std::string>>
    {
        {
            const auto timestamp = detail::now_iso();
            auto message = storage::MessageRow{
                .id = client_msg_id,
                .client_msg_id = client_msg_id,
                .peer_pub_key = receiver,
                .sender_pub_key = self.rpc_.public_key(),
                .text = text,
                .timestamp = timestamp,
                .server_seq = std::nullopt,
                .delivery = detail::string_to_delivery("pending"),
                .error = std::nullopt,
                .created_at = timestamp,
            };
            auto outbox = storage::OutboxRow{
                .id = client_msg_id,
                .receiver_pub_key = receiver,
                .text = text,
                .attempts = 1,
                .next_retry_at = std::nullopt,
                .error = std::nullopt,
                .status = 1,
                .created_at = timestamp,
            };
            self.cache_.with_storage([&](auto& store) {
                store.replace(message);
                store.replace(outbox);
            });
        }

        auto response = co_await self.rpc_.send_message_async(receiver, text, client_msg_id);
        if (!response) {
            self.mark_failed(client_msg_id, response.error());
            co_return std::unexpected{response.error()};
        }

        self.mark_sent(client_msg_id, receiver, *response);
        co_return SendMessageResult{
            .messageId = response->message_id(),
            .timestamp = response->timestamp(),
            .deduplicated = response->deduplicated() ? std::optional<bool>{true} : std::nullopt,
        };
    }

    auto status(this OutboxManager& self) -> OutboxStatus
    {
        return self.cache_.with_storage([](auto& storage) -> OutboxStatus {
            return OutboxStatus{
                .pending = storage.template count<storage::OutboxRow>(
                    sqlite_orm::where(sqlite_orm::is_equal(&storage::OutboxRow::status, 0))),
                .failed = storage.template count<storage::OutboxRow>(
                    sqlite_orm::where(sqlite_orm::is_equal(&storage::OutboxRow::status, 2))),
            };
        });
    }

    auto retry(this OutboxManager& self, const std::string& message_id) -> ExpectedVoid
    {
        auto found = self.cache_.with_storage([&](auto& storage) {
            auto rows = storage.template get_all<storage::OutboxRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::OutboxRow::id, message_id)));
            if (rows.empty()) {
                return false;
            }
            auto row = rows.front();
            row.status = 0;
            row.next_retry_at = std::nullopt;
            row.error = std::nullopt;
            storage.replace(row);
            return true;
        });
        if (found) {
            self.wake_worker();
        }
        return {};
    }

    auto clear_failed(this OutboxManager& self) -> ExpectedVoid
    {
        self.cache_.with_storage([](auto& storage) {
            storage.template remove_all<storage::OutboxRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::OutboxRow::status, 2)));
        });
        return {};
    }

    auto recover(this OutboxManager& self) -> void
    {
        self.cache_.with_storage([](auto& storage) {
            auto rows = storage.template get_all<storage::OutboxRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::OutboxRow::status, 1)));
            for (auto& row : rows) {
                row.status = 0;
                row.next_retry_at = std::nullopt;
                storage.replace(row);
            }
        });
    }

    auto start(this OutboxManager& self) -> void
    {
        self.stop();
        {
            std::scoped_lock lock{self.wake_mutex_};
            self.wake_requested_ = false;
        }
        self.outbox_thread_ = std::jthread([&self](std::stop_token token) {
            while (!token.stop_requested()) {
                self.process_due();
                std::unique_lock lock{self.wake_mutex_};
                self.wake_cv_.wait_for(lock, std::chrono::seconds{5}, [&] {
                    return token.stop_requested() || self.wake_requested_;
                });
                self.wake_requested_ = false;
            }
        });
    }

    auto stop(this OutboxManager& self) -> void
    {
        if (self.outbox_thread_.joinable()) {
            self.outbox_thread_.request_stop();
            self.wake_worker();
            self.outbox_thread_.join();
        }
    }

private:
    auto wake_worker(this OutboxManager& self) -> void
    {
        {
            std::scoped_lock lock{self.wake_mutex_};
            self.wake_requested_ = true;
        }
        self.wake_cv_.notify_one();
    }

    auto process_due(this OutboxManager& self) -> void
    {
        const auto now = detail::now_iso();
        auto due = self.cache_.with_storage([&](auto& storage) {
            auto rows = storage.template get_all<storage::OutboxRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::OutboxRow::status, 0)));
            std::vector<storage::OutboxRow> result;
            for (auto& row : rows) {
                if (!row.next_retry_at || *row.next_retry_at <= now) {
                    row.status = 1;
                    row.attempts += 1;
                    row.error = std::nullopt;
                    storage.replace(row);
                    result.push_back(row);
                }
            }
            return result;
        });

        for (const auto& row : due) {
            auto response = self.rpc_.send_message(row.receiver_pub_key, row.text, row.id);
            if (response) {
                self.mark_sent(row.id, row.receiver_pub_key, *response);
                self.bridge_.dispatch_message_status(
                    row.id,
                    "sent",
                    response->message_id(),
                    response->timestamp(),
                    std::nullopt);
                continue;
            }

            const auto permanently_failed = row.attempts >= 5;
            self.cache_.with_storage([&](auto& storage) {
                auto outbox = storage.template get_all<storage::OutboxRow>(
                    sqlite_orm::where(sqlite_orm::is_equal(&storage::OutboxRow::id, row.id)));
                for (auto& current : outbox) {
                    current.status = permanently_failed ? 2 : 0;
                    current.next_retry_at = permanently_failed
                        ? std::nullopt
                        : std::optional<std::string>{self.next_retry_time(current.attempts)};
                    current.error = response.error();
                    storage.replace(current);
                    break;
                }

                auto messages = storage.template get_all<storage::MessageRow>(
                    sqlite_orm::where(sqlite_orm::is_equal(&storage::MessageRow::client_msg_id, row.id)));
                if (!messages.empty()) {
                    auto message = messages.front();
                    message.delivery = detail::string_to_delivery(permanently_failed ? "failed" : "pending");
                    message.error = response.error();
                    storage.replace(message);
                }
            });

            if (permanently_failed) {
                self.bridge_.dispatch_message_status(row.id, "failed", std::nullopt, std::nullopt, response.error());
            }
        }
    }

    auto next_retry_time(this OutboxManager&, int attempts) -> std::string
    {
        auto delay = std::chrono::seconds{5};
        for (auto i = 1; i < attempts; ++i) {
            delay *= 2;
            if (delay >= std::chrono::minutes{5}) {
                delay = std::chrono::minutes{5};
                break;
            }
        }
        return detail::to_iso(std::chrono::system_clock::now() + delay);
    }

    auto mark_sent(
        this OutboxManager& self,
        const std::string& client_msg_id,
        const std::string& receiver,
        const chatview::proto::chat::SendMessageResp& response) -> void
    {
        self.cache_.with_storage([&](auto& storage) {
            storage.template remove<storage::OutboxRow>(client_msg_id);

            auto existing = storage.template get_all<storage::MessageRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::MessageRow::client_msg_id, client_msg_id)));
            const auto text = existing.empty() ? std::string{} : existing.front().text;
            const auto created_at = existing.empty() ? detail::now_iso() : existing.front().created_at;
            if (!existing.empty()) {
                storage.template remove<storage::MessageRow>(existing.front().id);
            }

            storage.replace(storage::MessageRow{
                .id = response.message_id(),
                .client_msg_id = client_msg_id,
                .peer_pub_key = receiver,
                .sender_pub_key = self.rpc_.public_key(),
                .text = text,
                .timestamp = response.timestamp(),
                .server_seq = response.server_seq() > 0 ? std::optional<std::int64_t>{response.server_seq()} : std::nullopt,
                .delivery = detail::string_to_delivery("sent"),
                .error = std::nullopt,
                .created_at = created_at,
            });
        });
    }

    auto mark_failed(this OutboxManager& self, const std::string& client_msg_id, const std::string& error) -> void
    {
        self.cache_.with_storage([&](auto& storage) {
            auto outbox = storage.template get_all<storage::OutboxRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::OutboxRow::id, client_msg_id)));
            for (auto& row : outbox) {
                row.status = 2;
                row.next_retry_at = std::nullopt;
                row.error = error;
                storage.replace(row);
                break;
            }

            auto messages = storage.template get_all<storage::MessageRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::MessageRow::client_msg_id, client_msg_id)));
            if (!messages.empty()) {
                auto message = messages.front();
                message.delivery = detail::string_to_delivery("failed");
                message.error = error;
                storage.replace(message);
            }
        });
    }

    storage::CacheDatabase& cache_;
    RpcClient& rpc_;
    NativeBridge& bridge_;
    std::jthread outbox_thread_;
    std::mutex wake_mutex_;
    std::condition_variable wake_cv_;
    bool wake_requested_ = false;
};

}
