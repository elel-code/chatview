module;

#include <cstddef>
#include <functional>
#include <optional>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

export module chatview.client:bridge;

import :types;
import :detail;
import chatview.proto.events;

namespace chatview::client
{

export class NativeBridge
{
public:
    using ScriptDispatcher = std::function<void(std::string)>;

    explicit NativeBridge(ScriptDispatcher dispatcher) : dispatcher_(std::move(dispatcher)) {}

    auto dispatch_script(this NativeBridge& self, std::string script) -> void
    {
        if (self.dispatcher_) {
            self.dispatcher_(std::move(script));
        }
    }

    auto dispatch_custom_event(this NativeBridge& self, std::string_view event_name, const std::string& detail_json) -> void
    {
        self.dispatch_script(
            "window.dispatchEvent(new CustomEvent(" + detail::js_string(event_name) + ",{detail:" + detail_json + "}))");
    }

    auto dispatch_server_event(
        this NativeBridge& self,
        const chatview::proto::events::ServerEvent& event,
        std::move_only_function<void()> clear_session) -> void
    {
        switch (event.event_case()) {
        case chatview::proto::events::ServerEvent::kNewMessage:
            self.dispatch_custom_event("saucer:messages-pending", detail::js_string(event.new_message().from_pub_key()));
            break;
        case chatview::proto::events::ServerEvent::kFriendStatus:
            self.dispatch_friend_status(Friend{
                .pubKey = event.friend_status().pub_key(),
                .alias = event.friend_status().alias(),
                .isOnline = event.friend_status().is_online(),
                .unread = 0,
            });
            break;
        case chatview::proto::events::ServerEvent::kSystemBroadcast:
            self.dispatch_custom_event("saucer:system-broadcast", detail::js_string(event.system_broadcast().text()));
            break;
        case chatview::proto::events::ServerEvent::kForceOffline:
            clear_session();
            self.dispatch_custom_event("saucer:force-offline", detail::js_string(event.force_offline().reason()));
            break;
        case chatview::proto::events::ServerEvent::kAdminUpdate:
            self.dispatch_custom_event("saucer:admin-update", "null");
            break;
        default:
            break;
        }
    }

    auto dispatch_friend_status(this NativeBridge& self, const Friend& friend_info) -> void
    {
        self.dispatch_custom_event(
            "saucer:friend-status",
            "{pubKey:" + detail::js_string(friend_info.pubKey) +
            ",alias:" + detail::js_string(friend_info.alias) +
            ",isOnline:" + (friend_info.isOnline ? "true" : "false") +
            "}");
    }

    auto dispatch_friend_removed(this NativeBridge& self, const std::string& pub_key) -> void
    {
        self.dispatch_custom_event("saucer:friend-removed", "{pubKey:" + detail::js_string(pub_key) + "}");
    }

    auto dispatch_cached_messages(this NativeBridge& self, const std::string& pub_key, const std::vector<ChatMessage>& messages) -> void
    {
        std::string json = "[";
        for (std::size_t i = 0; i < messages.size(); ++i) {
            const auto& message = messages[i];
            if (i != 0) {
                json += ",";
            }
            json += "{id:" + detail::js_string(message.id) + ",sender:" + detail::js_string(message.sender) +
                    ",text:" + detail::js_string(message.text) + ",timestamp:" + detail::js_string(message.timestamp) +
                    ",delivery:" + detail::js_string(message.delivery);
            if (message.error) {
                json += ",error:" + detail::js_string(*message.error);
            }
            json += "}";
        }
        json += "]";
        self.dispatch_custom_event(
            "saucer:cached-messages",
            "{pubKey:" + detail::js_string(pub_key) + ",messages:" + json + "}");
    }

    auto dispatch_message_status(
        this NativeBridge& self,
        const std::string& client_msg_id,
        const std::string& delivery,
        const std::optional<std::string>& server_msg_id,
        const std::optional<std::string>& timestamp,
        const std::optional<std::string>& error) -> void
    {
        std::string payload = "{clientMessageId:" + detail::js_string(client_msg_id) +
                              ",delivery:" + detail::js_string(delivery);
        if (server_msg_id) {
            payload += ",serverMessageId:" + detail::js_string(*server_msg_id);
        }
        if (timestamp) {
            payload += ",timestamp:" + detail::js_string(*timestamp);
        }
        if (error) {
            payload += ",error:" + detail::js_string(*error);
        }
        payload += "}";
        self.dispatch_custom_event("saucer:message-status", payload);
    }

private:
    ScriptDispatcher dispatcher_;
};

}
