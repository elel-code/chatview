module;

#include <array>
#include <chrono>
#include <coroutine>
#include <cstdint>
#include <exception>
#include <expected>
#include <filesystem>
#include <fstream>
#include <functional>
#include <future>
#include <memory>
#include <mutex>
#include <span>
#include <string>
#include <string_view>
#include <thread>
#include <utility>
#include <vector>

#include <coco/task/task.hpp>

export module chatview.client:rpc;

import asio_grpc;
import :types;
import :detail;
import chatview.proto.auth;
import chatview.proto.chat;
import chatview.proto.events;
import chatview.proto.admin;

namespace chatview::client
{

namespace rpc_detail
{
auto exception_status(std::exception_ptr exception) -> grpc::Status
{
    try {
        if (exception) {
            std::rethrow_exception(exception);
        }
    } catch (const std::exception& ex) {
        return grpc::Status{grpc::StatusCode::UNKNOWN, ex.what()};
    } catch (...) {
        return grpc::Status{grpc::StatusCode::UNKNOWN, "unknown coroutine failure"};
    }
    return {};
}

auto read_text_file(const std::filesystem::path& path) -> std::expected<std::string, std::string>
{
    std::ifstream input{path, std::ios::binary};
    if (!input) {
        return std::unexpected{"failed to open TLS CA file: " + path.string()};
    }

    std::string contents;
    input.seekg(0, std::ios::end);
    contents.resize(static_cast<std::size_t>(input.tellg()));
    input.seekg(0, std::ios::beg);
    if (!contents.empty()) {
        input.read(contents.data(), static_cast<std::streamsize>(contents.size()));
    }
    if (!input) {
        return std::unexpected{"failed to read TLS CA file: " + path.string()};
    }
    return contents;
}

auto make_channel(
    const std::string& target,
    bool use_tls,
    const std::filesystem::path& ca_cert_path,
    const std::string& ssl_target_name_override) -> std::expected<std::shared_ptr<grpc::Channel>, std::string>
{
    if (!use_tls) {
        return grpc::CreateChannel(target, grpc::InsecureChannelCredentials());
    }

    grpc::SslCredentialsOptions ssl_options;
    if (!ca_cert_path.empty()) {
        auto pem = read_text_file(ca_cert_path);
        if (!pem) {
            return std::unexpected{pem.error()};
        }
        ssl_options.pem_root_certs = std::move(*pem);
    }

    auto credentials = grpc::SslCredentials(ssl_options);
    if (ssl_target_name_override.empty()) {
        return grpc::CreateChannel(target, credentials);
    }

    grpc::ChannelArguments args;
    args.SetSslTargetNameOverride(ssl_target_name_override);
    return grpc::CreateCustomChannel(target, credentials, args);
}

}

class RpcClient
{
public:
    static auto create(
        std::string target,
        bool use_tls,
        std::filesystem::path ca_cert_path,
        std::string ssl_target_name_override) -> std::expected<std::unique_ptr<RpcClient>, std::string>
    {
        try {
            auto channel = rpc_detail::make_channel(target, use_tls, ca_cert_path, ssl_target_name_override);
            if (!channel) {
                return std::unexpected{channel.error()};
            }
            return std::unique_ptr<RpcClient>{new RpcClient{
                std::move(target),
                use_tls,
                std::move(ca_cert_path),
                std::move(ssl_target_name_override),
                *channel,
                chatview::proto::auth::AuthService::NewStub(*channel),
                chatview::proto::chat::ChatService::NewStub(*channel),
                chatview::proto::events::EventService::NewStub(*channel),
                chatview::proto::admin::AdminService::NewStub(*channel)}};
        } catch (const std::exception& ex) {
            return std::unexpected{std::string{ex.what()}};
        }
    }

    RpcClient(const RpcClient&) = delete;
    auto operator=(const RpcClient&) -> RpcClient& = delete;
    RpcClient(RpcClient&&) = delete;
    auto operator=(RpcClient&&) -> RpcClient& = delete;

    ~RpcClient()
    {
        stop_event_stream();
        grpc_work_guard_.reset();
        grpc_context_.stop();
        if (grpc_thread_.joinable()) {
            grpc_thread_.join();
        }
    }

    auto login(
        this RpcClient& self,
        const std::string& public_key_hex,
        std::span<const unsigned char> secret_key) -> std::expected<LoginResult, std::string>
    {
        chatview::proto::auth::RequestChallengeReq challenge_req;
        challenge_req.set_pub_key(public_key_hex);

        chatview::proto::auth::RequestChallengeResp challenge_resp;
        auto status = call_with_retry<&chatview::proto::auth::AuthService::Stub::PrepareAsyncRequestChallenge>(
            self,
            *self.auth_stub_,
            challenge_req,
            challenge_resp);
        if (!status.ok()) {
            return std::unexpected{detail::grpc_error(status)};
        }

        std::array<unsigned char, bssl::ed25519_signature_size> signature{};
        if (!bssl::ed25519_sign(
                signature,
                std::span<const unsigned char>{
                    reinterpret_cast<const unsigned char*>(challenge_resp.challenge().data()),
                    challenge_resp.challenge().size()},
                secret_key)) {
            return std::unexpected{"failed to sign challenge"};
        }
        detail::SecureBufferCleanup signature_cleanup{std::span<unsigned char>{signature}};

        chatview::proto::auth::LoginReq login_req;
        login_req.set_pub_key(public_key_hex);
        login_req.set_challenge_signature(signature.data(), signature.size());

        chatview::proto::auth::LoginResp login_resp;
        status = call_with_retry<&chatview::proto::auth::AuthService::Stub::PrepareAsyncLogin>(
            self,
            *self.auth_stub_,
            login_req,
            login_resp);
        if (!status.ok()) {
            if (status.error_code() == grpc::StatusCode::PERMISSION_DENIED) {
                return std::unexpected{"account banned"};
            }
            return std::unexpected{detail::grpc_error(status)};
        }

        {
            std::scoped_lock lock{self.session_mutex_};
            self.session_token_ = login_resp.session_token();
            self.public_key_ = login_resp.pub_key();
        }
        return LoginResult{.publicKey = login_resp.pub_key(), .role = login_resp.role()};
    }

    auto login_async(
        this RpcClient& self,
        std::string public_key_hex,
        std::vector<unsigned char> secret_key) -> coco::task<std::expected<LoginResult, std::string>>
    {
        chatview::proto::auth::RequestChallengeReq challenge_req;
        challenge_req.set_pub_key(public_key_hex);

        chatview::proto::auth::RequestChallengeResp challenge_resp;
        auto status = co_await call_with_retry_async<&chatview::proto::auth::AuthService::Stub::PrepareAsyncRequestChallenge>(
            self,
            *self.auth_stub_,
            challenge_req,
            challenge_resp);
        if (!status.ok()) {
            co_return std::unexpected{detail::grpc_error(status)};
        }

        std::array<unsigned char, bssl::ed25519_signature_size> signature{};
        if (!bssl::ed25519_sign(
                signature,
                std::span<const unsigned char>{
                    reinterpret_cast<const unsigned char*>(challenge_resp.challenge().data()),
                    challenge_resp.challenge().size()},
                secret_key)) {
            co_return std::unexpected{"failed to sign challenge"};
        }
        detail::SecureBufferCleanup signature_cleanup{std::span<unsigned char>{signature}};

        chatview::proto::auth::LoginReq login_req;
        login_req.set_pub_key(public_key_hex);
        login_req.set_challenge_signature(signature.data(), signature.size());

        chatview::proto::auth::LoginResp login_resp;
        status = co_await call_with_retry_async<&chatview::proto::auth::AuthService::Stub::PrepareAsyncLogin>(
            self,
            *self.auth_stub_,
            login_req,
            login_resp);
        if (!status.ok()) {
            if (status.error_code() == grpc::StatusCode::PERMISSION_DENIED) {
                co_return std::unexpected{"account banned"};
            }
            co_return std::unexpected{detail::grpc_error(status)};
        }

        {
            std::scoped_lock lock{self.session_mutex_};
            self.session_token_ = login_resp.session_token();
            self.public_key_ = login_resp.pub_key();
        }
        co_return LoginResult{.publicKey = login_resp.pub_key(), .role = login_resp.role()};
    }

    auto clear_session(this RpcClient& self) -> void
    {
        std::scoped_lock lock{self.session_mutex_};
        self.session_token_.clear();
        self.public_key_.clear();
    }

    auto public_key(this const RpcClient& self) -> std::string
    {
        std::scoped_lock lock{self.session_mutex_};
        return self.public_key_;
    }

    auto list_friends(this RpcClient& self) -> std::expected<std::vector<chatview::proto::common::FriendInfo>, std::string>
    {
        chatview::proto::chat::ListFriendsResp response;
        chatview::proto::chat::ListFriendsReq request;
        auto status = call_with_retry<&chatview::proto::chat::ChatService::Stub::PrepareAsyncListFriends>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            return std::unexpected{detail::grpc_error(status)};
        }
        return std::vector<chatview::proto::common::FriendInfo>{response.friends().begin(), response.friends().end()};
    }

    auto list_friends_async(this RpcClient& self) -> coco::task<std::expected<std::vector<chatview::proto::common::FriendInfo>, std::string>>
    {
        chatview::proto::chat::ListFriendsResp response;
        chatview::proto::chat::ListFriendsReq request;
        auto status = co_await call_with_retry_async<&chatview::proto::chat::ChatService::Stub::PrepareAsyncListFriends>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            co_return std::unexpected{detail::grpc_error(status)};
        }
        co_return std::vector<chatview::proto::common::FriendInfo>{response.friends().begin(), response.friends().end()};
    }

    auto send_message(
        this RpcClient& self,
        const std::string& receiver_pub_key,
        const std::string& text,
        const std::string& client_message_id) -> std::expected<chatview::proto::chat::SendMessageResp, std::string>
    {
        chatview::proto::chat::SendMessageReq request;
        request.set_receiver_pub_key(receiver_pub_key);
        request.set_text(text);
        request.set_client_message_id(client_message_id);

        chatview::proto::chat::SendMessageResp response;
        auto status = call_with_retry<&chatview::proto::chat::ChatService::Stub::PrepareAsyncSendMessage>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            return std::unexpected{detail::grpc_error(status)};
        }
        return response;
    }

    auto send_message_async(
        this RpcClient& self,
        std::string receiver_pub_key,
        std::string text,
        std::string client_message_id) -> coco::task<std::expected<chatview::proto::chat::SendMessageResp, std::string>>
    {
        chatview::proto::chat::SendMessageReq request;
        request.set_receiver_pub_key(std::move(receiver_pub_key));
        request.set_text(std::move(text));
        request.set_client_message_id(std::move(client_message_id));

        chatview::proto::chat::SendMessageResp response;
        auto status = co_await call_with_retry_async<&chatview::proto::chat::ChatService::Stub::PrepareAsyncSendMessage>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            co_return std::unexpected{detail::grpc_error(status)};
        }
        co_return response;
    }

    auto get_history(
        this RpcClient& self,
        const std::string& peer_pub_key,
        const MessageHistoryQuery& query) -> std::expected<chatview::proto::common::MessageHistoryPage, std::string>
    {
        chatview::proto::chat::GetMessageHistoryReq request;
        request.set_peer_pub_key(peer_pub_key);
        request.set_cursor(query.cursor.value_or(""));
        request.set_limit(detail::limit_or_default(query.limit));
        request.set_direction(detail::direction_or_default(query));

        chatview::proto::chat::GetMessageHistoryResp response;
        auto status = call_with_retry<&chatview::proto::chat::ChatService::Stub::PrepareAsyncGetMessageHistory>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            return std::unexpected{detail::grpc_error(status)};
        }
        return response.page();
    }

    auto get_history_async(
        this RpcClient& self,
        std::string peer_pub_key,
        MessageHistoryQuery query) -> coco::task<std::expected<chatview::proto::common::MessageHistoryPage, std::string>>
    {
        chatview::proto::chat::GetMessageHistoryReq request;
        request.set_peer_pub_key(std::move(peer_pub_key));
        request.set_cursor(query.cursor.value_or(""));
        request.set_limit(detail::limit_or_default(query.limit));
        request.set_direction(detail::direction_or_default(query));

        chatview::proto::chat::GetMessageHistoryResp response;
        auto status = co_await call_with_retry_async<&chatview::proto::chat::ChatService::Stub::PrepareAsyncGetMessageHistory>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            co_return std::unexpected{detail::grpc_error(status)};
        }
        co_return response.page();
    }

    auto mark_conversation_read(this RpcClient& self, const std::string& peer_pub_key, std::int64_t last_read_server_seq) -> ExpectedVoid
    {
        chatview::proto::chat::MarkConversationReadReq request;
        request.set_peer_pub_key(peer_pub_key);
        request.set_last_read_server_seq(last_read_server_seq);

        chatview::proto::chat::MarkConversationReadResp response;
        auto status = call_with_retry<&chatview::proto::chat::ChatService::Stub::PrepareAsyncMarkConversationRead>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            return std::unexpected{detail::grpc_error(status)};
        }
        return {};
    }

    auto mark_conversation_read_async(this RpcClient& self, std::string peer_pub_key, std::int64_t last_read_server_seq) -> coco::task<ExpectedVoid>
    {
        chatview::proto::chat::MarkConversationReadReq request;
        request.set_peer_pub_key(std::move(peer_pub_key));
        request.set_last_read_server_seq(last_read_server_seq);

        chatview::proto::chat::MarkConversationReadResp response;
        auto status = co_await call_with_retry_async<&chatview::proto::chat::ChatService::Stub::PrepareAsyncMarkConversationRead>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            co_return std::unexpected{detail::grpc_error(status)};
        }
        co_return {};
    }

    auto add_friend(this RpcClient& self, const std::string& target_pub_key) -> ExpectedVoid
    {
        chatview::proto::chat::AddFriendReq request;
        request.set_target_pub_key(target_pub_key);

        chatview::proto::chat::AddFriendResp response;
        auto status = call_with_retry<&chatview::proto::chat::ChatService::Stub::PrepareAsyncAddFriend>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            return std::unexpected{detail::grpc_error(status)};
        }
        return {};
    }

    auto add_friend_async(this RpcClient& self, std::string target_pub_key) -> coco::task<ExpectedVoid>
    {
        chatview::proto::chat::AddFriendReq request;
        request.set_target_pub_key(std::move(target_pub_key));

        chatview::proto::chat::AddFriendResp response;
        auto status = co_await call_with_retry_async<&chatview::proto::chat::ChatService::Stub::PrepareAsyncAddFriend>(
            self,
            *self.chat_stub_,
            request,
            response);
        if (!status.ok()) {
            co_return std::unexpected{detail::grpc_error(status)};
        }
        co_return {};
    }

    auto set_user_status(this RpcClient& self, const std::string& target_pub_key, std::string_view user_status) -> ExpectedVoid
    {
        chatview::proto::admin::SetUserStatusReq request;
        request.set_target_pub_key(target_pub_key);
        request.set_status(user_status == "banned" ? chatview::proto::common::USER_STATUS_BANNED : chatview::proto::common::USER_STATUS_ACTIVE);

        chatview::proto::admin::SetUserStatusResp response;
        auto status = call_with_retry<&chatview::proto::admin::AdminService::Stub::PrepareAsyncSetUserStatus>(
            self,
            *self.admin_stub_,
            request,
            response);
        if (!status.ok()) {
            return std::unexpected{detail::grpc_error(status)};
        }
        return {};
    }

    auto set_user_status_async(this RpcClient& self, std::string target_pub_key, std::string user_status) -> coco::task<ExpectedVoid>
    {
        chatview::proto::admin::SetUserStatusReq request;
        request.set_target_pub_key(std::move(target_pub_key));
        request.set_status(user_status == "banned" ? chatview::proto::common::USER_STATUS_BANNED : chatview::proto::common::USER_STATUS_ACTIVE);

        chatview::proto::admin::SetUserStatusResp response;
        auto status = co_await call_with_retry_async<&chatview::proto::admin::AdminService::Stub::PrepareAsyncSetUserStatus>(
            self,
            *self.admin_stub_,
            request,
            response);
        if (!status.ok()) {
            co_return std::unexpected{detail::grpc_error(status)};
        }
        co_return {};
    }

    auto broadcast(this RpcClient& self, const std::string& text) -> ExpectedVoid
    {
        chatview::proto::admin::BroadcastReq request;
        request.set_text(text);

        chatview::proto::admin::BroadcastResp response;
        auto status = call_with_retry<&chatview::proto::admin::AdminService::Stub::PrepareAsyncBroadcast>(
            self,
            *self.admin_stub_,
            request,
            response);
        if (!status.ok()) {
            return std::unexpected{detail::grpc_error(status)};
        }
        return {};
    }

    auto broadcast_async(this RpcClient& self, std::string text) -> coco::task<ExpectedVoid>
    {
        chatview::proto::admin::BroadcastReq request;
        request.set_text(std::move(text));

        chatview::proto::admin::BroadcastResp response;
        auto status = co_await call_with_retry_async<&chatview::proto::admin::AdminService::Stub::PrepareAsyncBroadcast>(
            self,
            *self.admin_stub_,
            request,
            response);
        if (!status.ok()) {
            co_return std::unexpected{detail::grpc_error(status)};
        }
        co_return {};
    }

    auto poll_admin_events(this RpcClient& self) -> std::expected<AdminUpdate, std::string>
    {
        chatview::proto::admin::PollAdminEventsReq request;
        chatview::proto::admin::PollAdminEventsResp response;

        auto status = call_with_retry<&chatview::proto::admin::AdminService::Stub::PrepareAsyncPollAdminEvents>(
            self,
            *self.admin_stub_,
            request,
            response);
        if (!status.ok()) {
            return std::unexpected{detail::grpc_error(status)};
        }

        AdminUpdate update;
        update.users.reserve(static_cast<std::size_t>(response.update().users_size()));
        for (const auto& user : response.update().users()) {
            update.users.push_back(UserInfo{
                .pubKey = user.pub_key(),
                .isOnline = user.is_online(),
                .isBanned = user.is_banned(),
            });
        }
        update.stats = AdminStats{
            .onlineUsers = response.update().stats().online_users(),
            .totalUsers = response.update().stats().total_users(),
            .bannedUsers = response.update().stats().banned_users(),
        };
        return update;
    }

    auto poll_admin_events_async(this RpcClient& self) -> coco::task<std::expected<AdminUpdate, std::string>>
    {
        chatview::proto::admin::PollAdminEventsReq request;
        chatview::proto::admin::PollAdminEventsResp response;

        auto status = co_await call_with_retry_async<&chatview::proto::admin::AdminService::Stub::PrepareAsyncPollAdminEvents>(
            self,
            *self.admin_stub_,
            request,
            response);
        if (!status.ok()) {
            co_return std::unexpected{detail::grpc_error(status)};
        }

        AdminUpdate update;
        update.users.reserve(static_cast<std::size_t>(response.update().users_size()));
        for (const auto& user : response.update().users()) {
            update.users.push_back(UserInfo{
                .pubKey = user.pub_key(),
                .isOnline = user.is_online(),
                .isBanned = user.is_banned(),
            });
        }
        update.stats = AdminStats{
            .onlineUsers = response.update().stats().online_users(),
            .totalUsers = response.update().stats().total_users(),
            .bannedUsers = response.update().stats().banned_users(),
        };
        co_return update;
    }

    auto start_event_stream(
        this RpcClient& self,
        std::function<void(const chatview::proto::events::ServerEvent&)> on_event) -> void
    {
        self.stop_event_stream();

        auto done = std::make_shared<std::promise<void>>();
        std::uint64_t generation;
        {
            std::scoped_lock lock{self.stream_mutex_};
            self.stream_active_ = true;
            generation = ++self.stream_generation_;
            self.stream_done_ = done->get_future().share();
        }

        asio::co_spawn(
            self.grpc_context_,
            self.event_stream_loop(generation, std::move(on_event)),
            [done](std::exception_ptr) {
                done->set_value();
            });
    }

    auto stop_event_stream(this RpcClient& self) -> void
    {
        std::shared_future<void> done;
        {
            std::scoped_lock lock{self.stream_mutex_};
            if (!self.stream_active_ && !self.stream_done_.valid()) {
                return;
            }
            self.stream_active_ = false;
            ++self.stream_generation_;
            if (self.stream_context_ != nullptr) {
                self.stream_context_->TryCancel();
            }
            if (self.stream_alarm_ != nullptr) {
                self.stream_alarm_->cancel();
            }
            done = self.stream_done_;
        }

        if (done.valid() && std::this_thread::get_id() != self.grpc_thread_.get_id()) {
            done.wait();
        }

        {
            std::scoped_lock lock{self.stream_mutex_};
            if (!self.stream_active_) {
                self.stream_done_ = {};
            }
        }
    }

    auto cancel_event_stream(this RpcClient& self) -> void
    {
        self.stop_event_stream();
    }

private:
    auto event_stream_loop(
        this RpcClient& self,
        std::uint64_t generation,
        std::function<void(const chatview::proto::events::ServerEvent&)> on_event) -> asio::awaitable<void>
    {
        using SubscribeRPC = agrpc::ClientRPC<&chatview::proto::events::EventService::Stub::PrepareAsyncSubscribe>;

        auto backoff = std::chrono::seconds{1};
        while (self.stream_active(generation)) {
            auto status = co_await self.run_event_stream_once<SubscribeRPC>(generation, on_event, backoff);
            if (!self.stream_active(generation)) {
                break;
            }
            if (!status.ok()) {
                auto alarm = agrpc::Alarm{self.grpc_context_};
                if (!self.set_stream_alarm(generation, &alarm)) {
                    break;
                }
                const auto expired = co_await alarm.wait(grpc::monotonic_deadline_after(backoff), asio::use_awaitable);
                self.clear_stream_alarm(generation, &alarm);
                if (!expired || !self.stream_active(generation)) {
                    break;
                }
                backoff = std::min(backoff * 2, std::chrono::seconds{60});
            }
        }
    }

    template<typename SubscribeRPC>
    auto run_event_stream_once(
        this RpcClient& self,
        std::uint64_t generation,
        const std::function<void(const chatview::proto::events::ServerEvent&)>& on_event,
        std::chrono::seconds& backoff) -> asio::awaitable<grpc::Status>
    {
        SubscribeRPC rpc{self.grpc_context_};
        self.configure_context(rpc.context(), std::chrono::hours{24});
        if (!self.set_stream_context(generation, &rpc.context())) {
            co_return grpc::Status{grpc::StatusCode::CANCELLED, "event stream stopped"};
        }

        chatview::proto::events::SubscribeReq request;
        request.set_client_id("chatview-desktop");

        if (!co_await rpc.start(*self.event_stub_, request, asio::use_awaitable)) {
            auto status = co_await rpc.finish(asio::use_awaitable);
            self.clear_stream_context(&rpc.context());
            co_return status;
        }

        while (self.stream_active(generation)) {
            chatview::proto::events::ServerEvent event;
            if (!co_await rpc.read(event, asio::use_awaitable)) {
                break;
            }
            if (!self.stream_active(generation)) {
                break;
            }
            on_event(event);
            backoff = std::chrono::seconds{1};
        }

        if (!self.stream_active(generation)) {
            rpc.cancel();
        }
        auto status = co_await rpc.finish(asio::use_awaitable);
        self.clear_stream_context(&rpc.context());
        co_return status;
    }

    RpcClient(
        std::string target,
        bool use_tls,
        std::filesystem::path ca_cert_path,
        std::string ssl_target_name_override,
        std::shared_ptr<grpc::Channel> channel,
        std::unique_ptr<chatview::proto::auth::AuthService::Stub> auth_stub,
        std::unique_ptr<chatview::proto::chat::ChatService::Stub> chat_stub,
        std::unique_ptr<chatview::proto::events::EventService::Stub> event_stub,
        std::unique_ptr<chatview::proto::admin::AdminService::Stub> admin_stub) :
        target_(std::move(target)),
        use_tls_(use_tls),
        ca_cert_path_(std::move(ca_cert_path)),
        ssl_target_name_override_(std::move(ssl_target_name_override)),
        channel_(std::move(channel)),
        auth_stub_(std::move(auth_stub)),
        chat_stub_(std::move(chat_stub)),
        event_stub_(std::move(event_stub)),
        admin_stub_(std::move(admin_stub))
    {
        grpc_thread_ = std::jthread([this](std::stop_token) {
            grpc_context_.run();
        });
    }

    auto configure_context(this RpcClient& self, grpc::ClientContext& context, std::chrono::steady_clock::duration timeout) -> void
    {
        context.set_deadline(grpc::monotonic_deadline_after(timeout));

        std::scoped_lock lock{self.session_mutex_};
        if (!self.session_token_.empty()) {
            context.AddMetadata("authorization", "Bearer " + self.session_token_);
        }
    }

    auto make_context(this RpcClient& self, std::chrono::steady_clock::duration timeout) -> std::unique_ptr<grpc::ClientContext>
    {
        auto context = std::make_unique<grpc::ClientContext>();
        self.configure_context(*context, timeout);
        return context;
    }

    auto stream_active(this RpcClient& self, std::uint64_t generation) -> bool
    {
        std::scoped_lock lock{self.stream_mutex_};
        return self.stream_active_ && self.stream_generation_ == generation;
    }

    auto set_stream_context(this RpcClient& self, std::uint64_t generation, grpc::ClientContext* context) -> bool
    {
        std::scoped_lock lock{self.stream_mutex_};
        if (!self.stream_active_ || self.stream_generation_ != generation) {
            return false;
        }
        self.stream_context_ = context;
        return true;
    }

    auto clear_stream_context(this RpcClient& self, grpc::ClientContext* context) -> void
    {
        std::scoped_lock lock{self.stream_mutex_};
        if (context == nullptr || self.stream_context_ == context) {
            self.stream_context_ = nullptr;
        }
    }

    auto set_stream_alarm(this RpcClient& self, std::uint64_t generation, agrpc::Alarm* alarm) -> bool
    {
        std::scoped_lock lock{self.stream_mutex_};
        if (!self.stream_active_ || self.stream_generation_ != generation) {
            return false;
        }
        self.stream_alarm_ = alarm;
        return true;
    }

    auto clear_stream_alarm(this RpcClient& self, std::uint64_t generation, agrpc::Alarm* alarm) -> void
    {
        std::scoped_lock lock{self.stream_mutex_};
        if (self.stream_generation_ == generation && self.stream_alarm_ == alarm) {
            self.stream_alarm_ = nullptr;
        }
    }

    template<auto PrepareAsync, typename Stub, typename Request, typename Response>
    static auto unary_once(
        RpcClient& self,
        Stub& stub,
        const Request& request,
        Response& response,
        std::chrono::steady_clock::duration timeout) -> grpc::Status
    {
        using RPC = agrpc::ClientRPC<PrepareAsync>;

        auto status = std::make_shared<grpc::Status>();
        auto completion = std::make_shared<std::promise<grpc::Status>>();
        auto completion_future = completion->get_future();
        auto context = self.make_context(timeout);
        asio::co_spawn(
            self.grpc_context_,
            [&, context = std::move(context), status]() mutable -> asio::awaitable<void> {
                *status = co_await RPC::request(self.grpc_context_, stub, *context, request, response, asio::use_awaitable);
            },
            [status, completion](std::exception_ptr exception) {
                if (exception) {
                    *status = rpc_detail::exception_status(exception);
                }
                completion->set_value(*status);
            });

        return completion_future.get();
    }

    template<auto PrepareAsync, typename Stub, typename Request, typename Response>
    static auto unary_once_async(
        RpcClient& self,
        Stub& stub,
        const Request& request,
        Response& response,
        std::chrono::steady_clock::duration timeout)
    {
        using RPC = agrpc::ClientRPC<PrepareAsync>;

        struct Awaiter
        {
            RpcClient& self;
            Stub& stub;
            const Request& request;
            Response& response;
            std::chrono::steady_clock::duration timeout;
            std::shared_ptr<grpc::Status> status = std::make_shared<grpc::Status>();

            auto await_ready() const noexcept -> bool
            {
                return false;
            }

            auto await_suspend(std::coroutine_handle<> continuation) -> bool
            {
                try {
                    auto context = self.make_context(timeout);
                    auto completion_status = status;
                    auto& grpc_context = self.grpc_context_;
                    auto& rpc_stub = stub;
                    auto& rpc_request = request;
                    auto& rpc_response = response;
                    asio::co_spawn(
                        grpc_context,
                        [&grpc_context, &rpc_stub, &rpc_request, &rpc_response, context = std::move(context), completion_status]() mutable -> asio::awaitable<void> {
                            *completion_status = co_await RPC::request(
                                grpc_context,
                                rpc_stub,
                                *context,
                                rpc_request,
                                rpc_response,
                                asio::use_awaitable);
                        },
                        [continuation, completion_status](std::exception_ptr exception) mutable {
                            if (exception) {
                                *completion_status = rpc_detail::exception_status(exception);
                            }
                            continuation.resume();
                        });
                    return true;
                } catch (...) {
                    *status = rpc_detail::exception_status(std::current_exception());
                    return false;
                }
            }

            auto await_resume() -> grpc::Status
            {
                return *status;
            }
        };

        return Awaiter{
            .self = self,
            .stub = stub,
            .request = request,
            .response = response,
            .timeout = timeout,
        };
    }

    template<auto PrepareAsync, typename Stub, typename Request, typename Response>
    static auto call_with_retry(RpcClient& self, Stub& stub, const Request& request, Response& response) -> grpc::Status
    {
        grpc::Status status;
        for (auto attempt = 0; attempt < detail::max_rpc_attempts; ++attempt) {
            status = unary_once<PrepareAsync>(self, stub, request, response, std::chrono::seconds{10});
            if (status.ok()) {
                return status;
            }
            if (status.error_code() != grpc::StatusCode::UNAVAILABLE && status.error_code() != grpc::StatusCode::DEADLINE_EXCEEDED) {
                return status;
            }
        }
        return status;
    }

    template<auto PrepareAsync, typename Stub, typename Request, typename Response>
    static auto call_with_retry_async(RpcClient& self, Stub& stub, const Request& request, Response& response) -> coco::task<grpc::Status>
    {
        grpc::Status status;
        for (auto attempt = 0; attempt < detail::max_rpc_attempts; ++attempt) {
            status = co_await unary_once_async<PrepareAsync>(self, stub, request, response, std::chrono::seconds{10});
            if (status.ok()) {
                co_return status;
            }
            if (status.error_code() != grpc::StatusCode::UNAVAILABLE && status.error_code() != grpc::StatusCode::DEADLINE_EXCEEDED) {
                co_return status;
            }
        }
        co_return status;
    }

    std::string target_;
    bool use_tls_ = true;
    std::filesystem::path ca_cert_path_;
    std::string ssl_target_name_override_;
    std::shared_ptr<grpc::Channel> channel_;
    std::unique_ptr<chatview::proto::auth::AuthService::Stub> auth_stub_;
    std::unique_ptr<chatview::proto::chat::ChatService::Stub> chat_stub_;
    std::unique_ptr<chatview::proto::events::EventService::Stub> event_stub_;
    std::unique_ptr<chatview::proto::admin::AdminService::Stub> admin_stub_;
    agrpc::GrpcContext grpc_context_;
    asio::executor_work_guard<agrpc::GrpcContext::executor_type> grpc_work_guard_{grpc_context_.get_executor()};
    std::jthread grpc_thread_;
    mutable std::mutex session_mutex_;
    std::string session_token_;
    std::string public_key_;
    std::mutex stream_mutex_;
    bool stream_active_ = false;
    std::uint64_t stream_generation_ = 0;
    std::shared_future<void> stream_done_;
    grpc::ClientContext* stream_context_ = nullptr;
    agrpc::Alarm* stream_alarm_ = nullptr;
};

}
