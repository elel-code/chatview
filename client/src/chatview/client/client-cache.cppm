module;

#include <algorithm>
#include <concepts>
#include <cstdint>
#include <expected>
#include <functional>
#include <iterator>
#include <optional>
#include <ranges>
#include <set>
#include <string>
#include <thread>
#include <utility>
#include <vector>

#include <coco/task/task.hpp>
#include <sqlite_orm/sqlite_orm.h>

export module chatview.client:cache;

import :types;
import :detail;
import :rpc;
import :bridge;
import chatview.proto.common;
import chatview.storage.cache;

namespace chatview::client
{

export using NativeTask = std::function<void(std::stop_token)>;
export using NativeTaskSpawner = std::function<void(NativeTask)>;

namespace cache_detail
{
auto parse_cursor(const std::optional<std::string>& cursor) -> std::expected<std::optional<std::int64_t>, std::string>
{
    if (!cursor || cursor->empty()) {
        return std::optional<std::int64_t>{};
    }
    try {
        std::size_t parsed = 0;
        const auto value = std::stoll(*cursor, &parsed);
        if (parsed != cursor->size() || value < 0) {
            return std::unexpected{"invalid cursor"};
        }
        return std::optional<std::int64_t>{value};
    } catch (...) {
        return std::unexpected{"invalid cursor"};
    }
}

auto max_server_seq(const std::vector<storage::MessageRow>& rows) -> std::optional<std::int64_t>
{
    std::optional<std::int64_t> result;
    for (const auto& row : rows) {
        if (row.server_seq && (!result || *row.server_seq > *result)) {
            result = row.server_seq;
        }
    }
    return result;
}

auto rows_to_page(std::vector<storage::MessageRow> rows, int limit, std::string_view direction) -> MessageHistoryPage
{
    const auto has_more = static_cast<int>(rows.size()) > limit;
    if (has_more) {
        rows.resize(static_cast<std::size_t>(limit));
    }

    std::ranges::sort(rows, {}, [](const storage::MessageRow& row) {
        return row.server_seq.value_or(0);
    });

    auto next_cursor = std::optional<std::string>{};
    if (has_more && !rows.empty()) {
        if (direction == "newer") {
            if (auto max_seq = max_server_seq(rows)) {
                next_cursor = std::to_string(*max_seq);
            }
        } else if (rows.front().server_seq) {
            next_cursor = std::to_string(*rows.front().server_seq);
        }
    }

    std::vector<ChatMessage> messages;
    messages.reserve(rows.size());
    std::ranges::transform(rows, std::back_inserter(messages), detail::message_from_row);
    return MessageHistoryPage{.messages = std::move(messages), .nextCursor = std::move(next_cursor), .hasMore = has_more};
}

auto proto_to_page(const chatview::proto::common::MessageHistoryPage& page) -> MessageHistoryPage
{
    std::vector<ChatMessage> messages;
    messages.reserve(static_cast<std::size_t>(page.messages_size()));
    for (const auto& msg : page.messages()) {
        messages.push_back(detail::message_from_row(detail::message_from_proto(msg, "")));
    }

    auto next_cursor = std::optional<std::string>{};
    if (!page.next_cursor().empty()) {
        next_cursor = page.next_cursor();
    }
    return MessageHistoryPage{.messages = std::move(messages), .nextCursor = std::move(next_cursor), .hasMore = page.has_more()};
}

auto is_contiguous_older(const std::vector<storage::MessageRow>& rows, std::int64_t cursor, int limit) -> bool
{
    if (rows.empty()) {
        return false;
    }
    auto expected = cursor - 1;
    const auto count = std::min<std::size_t>(rows.size(), static_cast<std::size_t>(limit));
    for (std::size_t i = 0; i < count; ++i) {
        if (!rows[i].server_seq || *rows[i].server_seq != expected) {
            return false;
        }
        --expected;
    }
    return static_cast<int>(rows.size()) > limit || rows[count - 1].server_seq.value_or(0) == 1;
}

auto is_contiguous_newer(const std::vector<storage::MessageRow>& rows, std::int64_t cursor) -> bool
{
    if (rows.empty()) {
        return false;
    }
    auto expected = cursor + 1;
    for (const auto& row : rows) {
        if (!row.server_seq || *row.server_seq != expected) {
            return false;
        }
        ++expected;
    }
    return true;
}
}

export class CacheController
{
public:
    CacheController(storage::CacheDatabase& cache, RpcClient& rpc, NativeBridge& bridge) :
        cache_(cache),
        rpc_(rpc),
        bridge_(bridge)
    {
    }

    auto list_friends(this CacheController& self) -> std::expected<std::vector<Friend>, std::string>
    {
        auto remote = self.fetch_and_cache_friends();
        if (remote) {
            return remote;
        }

        auto cached = self.cache_.with_storage([](auto& storage) {
            auto rows = storage.template get_all<storage::FriendRow>();
            std::vector<Friend> friends;
            friends.reserve(rows.size());
            std::ranges::transform(rows, std::back_inserter(friends), detail::friend_from_row);
            std::ranges::sort(friends, {}, &Friend::alias);
            return friends;
        });

        if (!cached.empty()) {
            return cached;
        }
        return remote;
    }

    auto list_friends_async(this CacheController& self) -> coco::task<std::expected<std::vector<Friend>, std::string>>
    {
        auto remote = co_await self.fetch_and_cache_friends_async();
        if (remote) {
            co_return remote;
        }

        auto cached = self.cache_.with_storage([](auto& storage) {
            auto rows = storage.template get_all<storage::FriendRow>();
            std::vector<Friend> friends;
            friends.reserve(rows.size());
            std::ranges::transform(rows, std::back_inserter(friends), detail::friend_from_row);
            std::ranges::sort(friends, {}, &Friend::alias);
            return friends;
        });

        if (!cached.empty()) {
            co_return cached;
        }
        co_return remote;
    }

    auto get_conversations(this CacheController& self) -> std::expected<std::vector<Friend>, std::string>
    {
        return self.cache_.with_storage([](auto& storage) {
            auto rows = storage.template get_all<storage::FriendRow>();
            std::vector<Friend> friends;
            friends.reserve(rows.size());
            std::ranges::transform(rows, std::back_inserter(friends), detail::friend_from_row);
            std::ranges::sort(friends, {}, &Friend::alias);
            return friends;
        });
    }

    auto get_messages(
        this CacheController& self,
        const std::string& peer_pub_key,
        const MessageHistoryQuery& query) -> std::expected<MessageHistoryPage, std::string>
    {
        const auto limit = detail::limit_or_default(query.limit);
        const auto direction = detail::direction_or_default(query);
        auto parsed_cursor = cache_detail::parse_cursor(query.cursor);
        if (!parsed_cursor) {
            return std::unexpected{parsed_cursor.error()};
        }

        auto effective_cursor = *parsed_cursor;
        if (direction == "newer" && !effective_cursor) {
            effective_cursor = self.last_seen_seq(peer_pub_key);
            if (!effective_cursor) {
                effective_cursor = self.max_cached_seq(peer_pub_key);
            }
        }

        auto cached = self.cached_messages(peer_pub_key, direction, effective_cursor, limit + 1);
        if (direction == "older" && effective_cursor &&
            cache_detail::is_contiguous_older(cached, *effective_cursor, limit)) {
            return cache_detail::rows_to_page(std::move(cached), limit, direction);
        }
        if (direction == "newer" && effective_cursor && cache_detail::is_contiguous_newer(cached, *effective_cursor)) {
            auto page = cache_detail::rows_to_page(std::move(cached), limit, direction);
            self.update_last_seen_seq(peer_pub_key, page);
            return page;
        }

        auto remote_query = query;
        if (direction == "newer" && effective_cursor && !remote_query.cursor) {
            remote_query.cursor = std::to_string(*effective_cursor);
        } else if (direction == "newer" && !effective_cursor) {
            remote_query.direction = "older";
            remote_query.cursor.reset();
        }
        auto remote = self.rpc_.get_history(peer_pub_key, remote_query);
        if (!remote) {
            if (!cached.empty()) {
                return cache_detail::rows_to_page(std::move(cached), limit, direction);
            }
            return std::unexpected{remote.error()};
        }

        self.cache_remote_page(peer_pub_key, *remote);
        auto page = cache_detail::proto_to_page(*remote);
        self.update_last_seen_seq(peer_pub_key, page);
        return page;
    }

    auto get_messages_async(
        this CacheController& self,
        std::string peer_pub_key,
        MessageHistoryQuery query) -> coco::task<std::expected<MessageHistoryPage, std::string>>
    {
        const auto limit = detail::limit_or_default(query.limit);
        const auto direction = detail::direction_or_default(query);
        auto parsed_cursor = cache_detail::parse_cursor(query.cursor);
        if (!parsed_cursor) {
            co_return std::unexpected{parsed_cursor.error()};
        }

        auto effective_cursor = *parsed_cursor;
        if (direction == "newer" && !effective_cursor) {
            effective_cursor = self.last_seen_seq(peer_pub_key);
            if (!effective_cursor) {
                effective_cursor = self.max_cached_seq(peer_pub_key);
            }
        }

        auto cached = self.cached_messages(peer_pub_key, direction, effective_cursor, limit + 1);
        if (direction == "older" && effective_cursor &&
            cache_detail::is_contiguous_older(cached, *effective_cursor, limit)) {
            co_return cache_detail::rows_to_page(std::move(cached), limit, direction);
        }
        if (direction == "newer" && effective_cursor && cache_detail::is_contiguous_newer(cached, *effective_cursor)) {
            auto page = cache_detail::rows_to_page(std::move(cached), limit, direction);
            self.update_last_seen_seq(peer_pub_key, page);
            co_return page;
        }

        auto remote_query = query;
        if (direction == "newer" && effective_cursor && !remote_query.cursor) {
            remote_query.cursor = std::to_string(*effective_cursor);
        } else if (direction == "newer" && !effective_cursor) {
            remote_query.direction = "older";
            remote_query.cursor.reset();
        }
        auto remote = co_await self.rpc_.get_history_async(peer_pub_key, remote_query);
        if (!remote) {
            if (!cached.empty()) {
                co_return cache_detail::rows_to_page(std::move(cached), limit, direction);
            }
            co_return std::unexpected{remote.error()};
        }

        self.cache_remote_page(peer_pub_key, *remote);
        auto page = cache_detail::proto_to_page(*remote);
        self.update_last_seen_seq(peer_pub_key, page);
        co_return page;
    }

    auto mark_conversation_read(
        this CacheController& self,
        const std::string& peer_pub_key,
        std::optional<std::int64_t> last_read_server_seq = std::nullopt) -> ExpectedVoid
    {
        const auto seq_for_server = last_read_server_seq.value_or(0);
        auto marked = self.rpc_.mark_conversation_read(peer_pub_key, seq_for_server);
        if (!marked) {
            return marked;
        }

        self.cache_.with_storage([&](auto& storage) {
            auto rows = storage.template get_all<storage::FriendRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::FriendRow::pub_key, peer_pub_key)));
            if (rows.empty()) {
                return;
            }
            auto row = rows.front();
            row.unread = 0;
            if (last_read_server_seq) {
                row.last_seen_seq = std::max(row.last_seen_seq, *last_read_server_seq);
            } else if (auto max_seq = self.max_cached_seq(peer_pub_key)) {
                row.last_seen_seq = std::max(row.last_seen_seq, *max_seq);
            }
            storage.replace(row);
        });
        return {};
    }

    auto mark_conversation_read_async(
        this CacheController& self,
        std::string peer_pub_key,
        std::optional<std::int64_t> last_read_server_seq = std::nullopt) -> coco::task<ExpectedVoid>
    {
        const auto seq_for_server = last_read_server_seq.value_or(0);
        auto marked = co_await self.rpc_.mark_conversation_read_async(peer_pub_key, seq_for_server);
        if (!marked) {
            co_return marked;
        }

        self.cache_.with_storage([&](auto& storage) {
            auto rows = storage.template get_all<storage::FriendRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::FriendRow::pub_key, peer_pub_key)));
            if (rows.empty()) {
                return;
            }
            auto row = rows.front();
            row.unread = 0;
            if (last_read_server_seq) {
                row.last_seen_seq = std::max(row.last_seen_seq, *last_read_server_seq);
            } else if (auto max_seq = self.max_cached_seq(peer_pub_key)) {
                row.last_seen_seq = std::max(row.last_seen_seq, *max_seq);
            }
            storage.replace(row);
        });
        co_return {};
    }

private:
    auto fetch_and_cache_friends(this CacheController& self) -> std::expected<std::vector<Friend>, std::string>
    {
        auto remote = self.rpc_.list_friends();
        if (!remote) {
            return std::unexpected{remote.error()};
        }
        return self.merge_remote_friends(*remote, false);
    }

    auto fetch_and_cache_friends_async(this CacheController& self) -> coco::task<std::expected<std::vector<Friend>, std::string>>
    {
        auto remote = co_await self.rpc_.list_friends_async();
        if (!remote) {
            co_return std::unexpected{remote.error()};
        }
        co_return self.merge_remote_friends(*remote, false);
    }

    auto merge_remote_friends(
        this CacheController& self,
        const std::vector<chatview::proto::common::FriendInfo>& remote,
        bool dispatch_changes) -> std::expected<std::vector<Friend>, std::string>
    {
        std::set<std::string> remote_keys;
        std::vector<Friend> result;
        result.reserve(remote.size());

        self.cache_.with_storage([&](auto& storage) {
            for (const auto& fi : remote) {
                remote_keys.insert(fi.pub_key());
                auto rows = storage.template get_all<storage::FriendRow>(
                    sqlite_orm::where(sqlite_orm::is_equal(&storage::FriendRow::pub_key, fi.pub_key())));

                auto row = rows.empty()
                    ? storage::FriendRow{
                          .pub_key = fi.pub_key(),
                          .alias = fi.alias(),
                          .is_online = fi.is_online(),
                          .unread = fi.unread(),
                          .last_seen_seq = 0,
                          .updated_at = detail::now_iso(),
                      }
                    : rows.front();

                const auto changed = rows.empty() || row.alias != fi.alias() || row.is_online != fi.is_online() || row.unread != fi.unread();
                row.alias = fi.alias();
                row.is_online = fi.is_online();
                row.unread = fi.unread();
                row.updated_at = detail::now_iso();
                storage.replace(row);

                auto friend_info = detail::friend_from_row(row);
                result.push_back(friend_info);
                if (dispatch_changes && changed) {
                    self.bridge_.dispatch_friend_status(friend_info);
                }
            }

            for (const auto& row : storage.template get_all<storage::FriendRow>()) {
                if (!remote_keys.contains(row.pub_key)) {
                    storage.template remove<storage::FriendRow>(row.pub_key);
                    if (dispatch_changes) {
                        self.bridge_.dispatch_friend_removed(row.pub_key);
                    }
                }
            }
        });

        std::ranges::sort(result, {}, &Friend::alias);
        return result;
    }

    auto cached_messages(
        this CacheController& self,
        const std::string& peer_pub_key,
        const std::string& direction,
        std::optional<std::int64_t> cursor,
        int limit) -> std::vector<storage::MessageRow>
    {
        return self.cache_.with_storage([&](auto& storage) {
            if (direction == "newer") {
                const auto after = cursor.value_or(0);
                return storage.template get_all<storage::MessageRow>(
                    sqlite_orm::where(sqlite_orm::is_equal(&storage::MessageRow::peer_pub_key, peer_pub_key) &&
                                      sqlite_orm::greater_than(&storage::MessageRow::server_seq, after)),
                    sqlite_orm::order_by(&storage::MessageRow::server_seq),
                    sqlite_orm::limit(static_cast<std::size_t>(limit)));
            }
            if (cursor) {
                return storage.template get_all<storage::MessageRow>(
                    sqlite_orm::where(sqlite_orm::is_equal(&storage::MessageRow::peer_pub_key, peer_pub_key) &&
                                      sqlite_orm::less_than(&storage::MessageRow::server_seq, *cursor)),
                    sqlite_orm::order_by(&storage::MessageRow::server_seq).desc(),
                    sqlite_orm::limit(static_cast<std::size_t>(limit)));
            }
            return storage.template get_all<storage::MessageRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::MessageRow::peer_pub_key, peer_pub_key)),
                sqlite_orm::order_by(&storage::MessageRow::server_seq).desc(),
                sqlite_orm::limit(static_cast<std::size_t>(limit)));
        });
    }

    auto cache_remote_page(this CacheController& self, const std::string& peer_pub_key, const chatview::proto::common::MessageHistoryPage& page) -> void
    {
        self.cache_.with_storage([&](auto& storage) {
            for (const auto& msg : page.messages()) {
                storage.replace(detail::message_from_proto(msg, peer_pub_key));
            }
        });
    }

    auto last_seen_seq(this CacheController& self, const std::string& peer_pub_key) -> std::optional<std::int64_t>
    {
        return self.cache_.with_storage([&](auto& storage) -> std::optional<std::int64_t> {
            auto rows = storage.template get_all<storage::FriendRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::FriendRow::pub_key, peer_pub_key)));
            if (rows.empty() || rows.front().last_seen_seq <= 0) {
                return std::nullopt;
            }
            return rows.front().last_seen_seq;
        });
    }

    auto max_cached_seq(this CacheController& self, const std::string& peer_pub_key) -> std::optional<std::int64_t>
    {
        auto rows = self.cache_.with_storage([&](auto& storage) {
            return storage.template get_all<storage::MessageRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::MessageRow::peer_pub_key, peer_pub_key)),
                sqlite_orm::order_by(&storage::MessageRow::server_seq).desc(),
                sqlite_orm::limit(1));
        });
        if (rows.empty()) {
            return std::nullopt;
        }
        return rows.front().server_seq;
    }

    auto update_last_seen_seq(this CacheController& self, const std::string& peer_pub_key, const MessageHistoryPage& page) -> void
    {
        std::optional<std::int64_t> max_seq;
        for (const auto& message : page.messages) {
            auto rows = self.cache_.with_storage([&](auto& storage) {
                return storage.template get_all<storage::MessageRow>(
                    sqlite_orm::where(sqlite_orm::is_equal(&storage::MessageRow::id, message.id)));
            });
            if (!rows.empty() && rows.front().server_seq && (!max_seq || *rows.front().server_seq > *max_seq)) {
                max_seq = rows.front().server_seq;
            }
        }
        if (!max_seq) {
            return;
        }

        self.cache_.with_storage([&](auto& storage) {
            auto rows = storage.template get_all<storage::FriendRow>(
                sqlite_orm::where(sqlite_orm::is_equal(&storage::FriendRow::pub_key, peer_pub_key)));
            if (rows.empty()) {
                return;
            }
            auto row = rows.front();
            row.last_seen_seq = std::max(row.last_seen_seq, *max_seq);
            storage.replace(row);
        });
    }

    storage::CacheDatabase& cache_;
    RpcClient& rpc_;
    NativeBridge& bridge_;
};

}
