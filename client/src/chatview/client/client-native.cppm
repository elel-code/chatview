module;

#include <expected>
#include <ctime>
#include <cstdint>
#include <filesystem>
#include <functional>
#include <memory>
#include <optional>
#include <string>
#include <utility>
#include <vector>

#include <coco/task/task.hpp>

export module chatview.client:native;

import :types;
import :detail;
import :identity;
import :rpc;
import :bridge;
import :session;
import :cache;
import :outbox;
import chatview.storage.cache;

namespace chatview::client
{

export class NativeClient
{
public:
    using ScriptDispatcher = NativeBridge::ScriptDispatcher;

    static auto create(NativeClientOptions options, ScriptDispatcher dispatcher) -> std::expected<std::unique_ptr<NativeClient>, std::string>
    {
        try {
            if (options.dataDir.empty()) {
                options.dataDir = detail::default_data_dir();
            }
            if (options.grpcTarget.empty()) {
                options.grpcTarget = "127.0.0.1:50051";
            }
            if (!options.grpcUseTls) {
                options.grpcUseTls = detail::default_grpc_use_tls(options.grpcTarget);
            }

            std::filesystem::create_directories(options.dataDir);

            auto identity = IdentityStore::create(options.dataDir / "identity.bin");
            if (!identity) {
                return std::unexpected{identity.error()};
            }

            auto cache = storage::CacheDatabase::create(options.dataDir / "cache.db");
            if (!cache) {
                return std::unexpected{cache.error()};
            }

            auto rpc = RpcClient::create(
                options.grpcTarget,
                *options.grpcUseTls,
                options.grpcCaCertPath,
                options.grpcSslTargetNameOverride);
            if (!rpc) {
                return std::unexpected{rpc.error()};
            }

            return std::unique_ptr<NativeClient>{
                new NativeClient{std::move(*identity), std::move(*cache), std::move(*rpc), std::move(dispatcher)}};
        } catch (const std::exception& ex) {
            return std::unexpected{std::string{ex.what()}};
        }
    }

    NativeClient(const NativeClient&) = delete;
    auto operator=(const NativeClient&) -> NativeClient& = delete;
    NativeClient(NativeClient&&) = delete;
    auto operator=(NativeClient&&) -> NativeClient& = delete;

    ~NativeClient()
    {
        outbox_.stop();
        stop_event_stream();
    }

    auto hasLocalIdentity(this const NativeClient& self) -> bool
    {
        return self.session_.has_local_identity();
    }

    auto createIdentity(this NativeClient& self, const std::string& pin) -> std::expected<IdentityResult, std::string>
    {
        return self.session_.create_identity(pin);
    }

    auto importIdentity(this NativeClient& self, const std::string& private_key, const std::string& pin) -> ExpectedVoid
    {
        return self.session_.import_identity(private_key, pin);
    }

    auto login(this NativeClient& self, const std::string& pin) -> std::expected<LoginResult, std::string>
    {
        auto result = self.session_.login(pin);
        if (!result) {
            return result;
        }
        self.outbox_.recover();
        self.outbox_.start();
        self.start_event_stream();
        return result;
    }

    auto loginAsync(this NativeClient& self, const std::string& pin) -> coco::task<std::expected<LoginResult, std::string>>
    {
        auto result = co_await self.session_.login_async(pin);
        if (!result) {
            co_return result;
        }
        self.outbox_.recover();
        self.outbox_.start();
        self.start_event_stream();
        co_return result;
    }

    auto exportPrivateKey(this NativeClient& self, const std::string& pin) -> std::expected<std::string, std::string>
    {
        return self.session_.export_private_key(pin);
    }

    auto lockSession(this NativeClient& self) -> ExpectedVoid
    {
        self.outbox_.stop();
        self.stop_event_stream();
        return self.session_.lock();
    }

    auto getAuthLockState(this NativeClient& self) -> AuthLockState
    {
        return self.session_.current_lock_state();
    }

    auto listFriends(this NativeClient& self) -> std::expected<std::vector<Friend>, std::string>
    {
        return self.cache_controller_.list_friends();
    }

    auto listFriendsAsync(this NativeClient& self) -> coco::task<std::expected<std::vector<Friend>, std::string>>
    {
        co_return co_await self.cache_controller_.list_friends_async();
    }

    auto getConversations(this NativeClient& self) -> std::expected<std::vector<Friend>, std::string>
    {
        return self.cache_controller_.get_conversations();
    }

    auto getMessageHistory(
        this NativeClient& self,
        const std::string& peer_pub_key,
        const MessageHistoryQuery& query) -> std::expected<MessageHistoryPage, std::string>
    {
        return self.cache_controller_.get_messages(peer_pub_key, query);
    }

    auto getMessageHistoryAsync(
        this NativeClient& self,
        std::string peer_pub_key,
        MessageHistoryQuery query) -> coco::task<std::expected<MessageHistoryPage, std::string>>
    {
        co_return co_await self.cache_controller_.get_messages_async(std::move(peer_pub_key), std::move(query));
    }

    auto getMessages(
        this NativeClient& self,
        const std::string& peer_pub_key,
        const MessageHistoryQuery& query) -> std::expected<MessageHistoryPage, std::string>
    {
        return self.getMessageHistory(peer_pub_key, query);
    }

    auto sendMessage(this NativeClient& self, const std::string& receiver, const std::string& text) -> std::expected<SendMessageResult, std::string>
    {
        const auto client_msg_id = detail::now_iso() + ":" + receiver + ":" + std::to_string(std::time(nullptr));
        return self.outbox_.send_message(receiver, text, client_msg_id);
    }

    auto sendMessage(
        this NativeClient& self,
        const std::string& receiver,
        const std::string& text,
        const std::string& client_msg_id) -> std::expected<SendMessageResult, std::string>
    {
        return self.outbox_.send_message(receiver, text, client_msg_id.empty()
            ? detail::now_iso() + ":" + receiver + ":" + std::to_string(std::time(nullptr))
            : client_msg_id);
    }

    auto sendMessageAsync(
        this NativeClient& self,
        std::string receiver,
        std::string text,
        std::string client_msg_id) -> coco::task<std::expected<SendMessageResult, std::string>>
    {
        co_return co_await self.outbox_.send_message_async(
            receiver,
            text,
            client_msg_id.empty()
                ? detail::now_iso() + ":" + receiver + ":" + std::to_string(std::time(nullptr))
                : client_msg_id);
    }

    auto markConversationRead(this NativeClient& self, const std::string& peer_pub_key) -> ExpectedVoid
    {
        return self.cache_controller_.mark_conversation_read(peer_pub_key);
    }

    auto markConversationRead(
        this NativeClient& self,
        const std::string& peer_pub_key,
        std::optional<std::int64_t> last_read_server_seq) -> ExpectedVoid
    {
        return self.cache_controller_.mark_conversation_read(peer_pub_key, last_read_server_seq);
    }

    auto markConversationReadAsync(
        this NativeClient& self,
        std::string peer_pub_key,
        std::optional<std::int64_t> last_read_server_seq) -> coco::task<ExpectedVoid>
    {
        co_return co_await self.cache_controller_.mark_conversation_read_async(std::move(peer_pub_key), last_read_server_seq);
    }

    auto addFriend(this NativeClient& self, const std::string& target_pub_key) -> ExpectedVoid
    {
        return self.rpc_->add_friend(target_pub_key);
    }

    auto addFriendAsync(this NativeClient& self, std::string target_pub_key) -> coco::task<ExpectedVoid>
    {
        co_return co_await self.rpc_->add_friend_async(std::move(target_pub_key));
    }

    auto adminSetUserStatus(this NativeClient& self, const std::string& target_pub_key, const std::string& status) -> ExpectedVoid
    {
        return self.rpc_->set_user_status(target_pub_key, status);
    }

    auto adminSetUserStatusAsync(this NativeClient& self, std::string target_pub_key, std::string status) -> coco::task<ExpectedVoid>
    {
        co_return co_await self.rpc_->set_user_status_async(std::move(target_pub_key), std::move(status));
    }

    auto adminBroadcast(this NativeClient& self, const std::string& text) -> ExpectedVoid
    {
        return self.rpc_->broadcast(text);
    }

    auto adminBroadcastAsync(this NativeClient& self, std::string text) -> coco::task<ExpectedVoid>
    {
        co_return co_await self.rpc_->broadcast_async(std::move(text));
    }

    auto pollAdminEvents(this NativeClient& self) -> std::expected<AdminUpdate, std::string>
    {
        return self.rpc_->poll_admin_events();
    }

    auto pollAdminEventsAsync(this NativeClient& self) -> coco::task<std::expected<AdminUpdate, std::string>>
    {
        co_return co_await self.rpc_->poll_admin_events_async();
    }

    auto getOutboxStatus(this NativeClient& self) -> OutboxStatus
    {
        return self.outbox_.status();
    }

    auto retryOutboxMessage(this NativeClient& self, const std::string& message_id) -> ExpectedVoid
    {
        return self.outbox_.retry(message_id);
    }

    auto clearOutbox(this NativeClient& self) -> ExpectedVoid
    {
        return self.outbox_.clear_failed();
    }

private:
    NativeClient(IdentityStore identity, storage::CacheDatabase cache, std::unique_ptr<RpcClient> rpc, ScriptDispatcher dispatcher) :
        cache_(std::move(cache)),
        rpc_(std::move(rpc)),
        bridge_(std::move(dispatcher)),
        session_(std::move(identity), *rpc_),
        cache_controller_(cache_, *rpc_, bridge_),
        outbox_(cache_, *rpc_, bridge_)
    {
    }

    auto start_event_stream(this NativeClient& self) -> void
    {
        self.rpc_->start_event_stream([&self](const auto& event) {
            self.bridge_.dispatch_server_event(event, [&self] {
                self.rpc_->clear_session();
            });
        });
    }

    auto stop_event_stream(this NativeClient& self) -> void
    {
        self.rpc_->stop_event_stream();
    }

    storage::CacheDatabase cache_;
    std::unique_ptr<RpcClient> rpc_;
    NativeBridge bridge_;
    SessionController session_;
    CacheController cache_controller_;
    OutboxManager outbox_;
};

export auto default_options() -> NativeClientOptions
{
    return NativeClientOptions{
        .dataDir = detail::default_data_dir(),
        .grpcTarget = "127.0.0.1:50051",
    };
}

}
