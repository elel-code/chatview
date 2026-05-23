#pragma once

#include "client-types.hpp"

#include <chrono>
#include <cstdint>
#include <expected>
#include <filesystem>
#include <functional>
#include <future>
#include <memory>
#include <mutex>
#include <span>
#include <string>
#include <string_view>
#include <thread>
#include <vector>

#include <agrpc/alarm.hpp>
#include <agrpc/grpc_context.hpp>
#include <asio/awaitable.hpp>
#include <asio/executor_work_guard.hpp>
#include <coco/task/task.hpp>
#include <grpcpp/grpcpp.h>

#include <chatview/admin.grpc.pb.h>
#include <chatview/auth.grpc.pb.h>
#include <chatview/chat.grpc.pb.h>
#include <chatview/common/types.pb.h>
#include <chatview/events.grpc.pb.h>

namespace chatview::client
{

class RpcClient
{
  public:
    static auto create(std::string target, bool use_tls, std::filesystem::path ca_cert_path, std::string ssl_target_name_override)
        -> std::expected<std::unique_ptr<RpcClient>, std::string>;

    RpcClient(const RpcClient &) = delete;
    auto operator=(const RpcClient &) -> RpcClient & = delete;
    RpcClient(RpcClient &&) = delete;
    auto operator=(RpcClient &&) -> RpcClient & = delete;

    ~RpcClient();

    auto login(this RpcClient &self, const std::string &public_key_hex, std::span<const unsigned char> secret_key)
        -> std::expected<LoginResult, std::string>;

    auto login_async(this RpcClient &self, std::string public_key_hex, std::vector<unsigned char> secret_key)
        -> coco::task<std::expected<LoginResult, std::string>>;

    auto clear_session(this RpcClient &self) -> void;
    auto public_key(this const RpcClient &self) -> std::string;
    auto list_friends(this RpcClient &self) -> std::expected<std::vector<chatview::common::FriendInfo>, std::string>;
    auto list_friends_async(this RpcClient &self) -> coco::task<std::expected<std::vector<chatview::common::FriendInfo>, std::string>>;

    auto send_message(this RpcClient &self, const std::string &receiver_pub_key, const std::string &text,
                      const std::string &client_message_id) -> std::expected<chatview::chat::SendMessageResp, std::string>;

    auto send_message_async(this RpcClient &self, std::string receiver_pub_key, std::string text, std::string client_message_id)
        -> coco::task<std::expected<chatview::chat::SendMessageResp, std::string>>;

    auto get_history(this RpcClient &self, const std::string &peer_pub_key, const MessageHistoryQuery &query)
        -> std::expected<chatview::common::MessageHistoryPage, std::string>;

    auto get_history_async(this RpcClient &self, std::string peer_pub_key, MessageHistoryQuery query)
        -> coco::task<std::expected<chatview::common::MessageHistoryPage, std::string>>;

    auto mark_conversation_read(this RpcClient &self, const std::string &peer_pub_key, std::int64_t last_read_server_seq) -> ExpectedVoid;
    auto mark_conversation_read_async(this RpcClient &self, std::string peer_pub_key, std::int64_t last_read_server_seq)
        -> coco::task<ExpectedVoid>;
    auto add_friend(this RpcClient &self, const std::string &target_pub_key) -> ExpectedVoid;
    auto add_friend_async(this RpcClient &self, std::string target_pub_key) -> coco::task<ExpectedVoid>;
    auto set_user_status(this RpcClient &self, const std::string &target_pub_key, std::string_view user_status) -> ExpectedVoid;
    auto set_user_status_async(this RpcClient &self, std::string target_pub_key, std::string user_status) -> coco::task<ExpectedVoid>;
    auto broadcast(this RpcClient &self, const std::string &text) -> ExpectedVoid;
    auto broadcast_async(this RpcClient &self, std::string text) -> coco::task<ExpectedVoid>;
    auto poll_admin_events(this RpcClient &self) -> std::expected<AdminUpdate, std::string>;
    auto poll_admin_events_async(this RpcClient &self) -> coco::task<std::expected<AdminUpdate, std::string>>;

    auto start_event_stream(this RpcClient &self, std::function<void(const chatview::events::ServerEvent &)> on_event) -> void;
    auto stop_event_stream(this RpcClient &self) -> void;
    auto cancel_event_stream(this RpcClient &self) -> void;

  private:
    auto event_stream_loop(this RpcClient &self, std::uint64_t generation,
                           std::function<void(const chatview::events::ServerEvent &)> on_event) -> asio::awaitable<void>;

    template <typename SubscribeRPC>
    auto run_event_stream_once(this RpcClient &self, std::uint64_t generation,
                               const std::function<void(const chatview::events::ServerEvent &)> &on_event, std::chrono::seconds &backoff)
        -> asio::awaitable<grpc::Status>;

    RpcClient(std::string target, bool use_tls, std::filesystem::path ca_cert_path, std::string ssl_target_name_override,
              std::shared_ptr<grpc::Channel> channel, std::unique_ptr<chatview::auth::AuthService::Stub> auth_stub,
              std::unique_ptr<chatview::chat::ChatService::Stub> chat_stub,
              std::unique_ptr<chatview::events::EventService::Stub> event_stub,
              std::unique_ptr<chatview::admin::AdminService::Stub> admin_stub);

    auto configure_context(this RpcClient &self, grpc::ClientContext &context, std::chrono::steady_clock::duration timeout) -> void;
    auto make_context(this RpcClient &self, std::chrono::steady_clock::duration timeout) -> std::unique_ptr<grpc::ClientContext>;
    auto stream_active(this RpcClient &self, std::uint64_t generation) -> bool;
    auto set_stream_context(this RpcClient &self, std::uint64_t generation, grpc::ClientContext *context) -> bool;
    auto clear_stream_context(this RpcClient &self, grpc::ClientContext *context) -> void;
    auto set_stream_alarm(this RpcClient &self, std::uint64_t generation, agrpc::Alarm *alarm) -> bool;
    auto clear_stream_alarm(this RpcClient &self, std::uint64_t generation, agrpc::Alarm *alarm) -> void;

    template <auto PrepareAsync, typename Stub, typename Request, typename Response>
    auto unary_once(this RpcClient &self, Stub &stub, const Request &request, Response &response,
                    std::chrono::steady_clock::duration timeout) -> grpc::Status;

    template <auto PrepareAsync, typename Stub, typename Request, typename Response>
    auto unary_once_async(this RpcClient &self, Stub &stub, const Request &request, Response &response,
                          std::chrono::steady_clock::duration timeout);

    template <auto PrepareAsync, typename Stub, typename Request, typename Response>
    auto call_with_retry(this RpcClient &self, Stub &stub, const Request &request, Response &response) -> grpc::Status;

    template <auto PrepareAsync, typename Stub, typename Request, typename Response>
    auto call_with_retry_async(this RpcClient &self, Stub &stub, const Request &request, Response &response) -> coco::task<grpc::Status>;

    std::string target_;
    bool use_tls_ = true;
    std::filesystem::path ca_cert_path_;
    std::string ssl_target_name_override_;
    std::shared_ptr<grpc::Channel> channel_;
    std::unique_ptr<chatview::auth::AuthService::Stub> auth_stub_;
    std::unique_ptr<chatview::chat::ChatService::Stub> chat_stub_;
    std::unique_ptr<chatview::events::EventService::Stub> event_stub_;
    std::unique_ptr<chatview::admin::AdminService::Stub> admin_stub_;
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
    grpc::ClientContext *stream_context_ = nullptr;
    agrpc::Alarm *stream_alarm_ = nullptr;
};

} // namespace chatview::client
