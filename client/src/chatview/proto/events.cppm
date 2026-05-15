module;

#include "chatview/events.grpc.pb.h"
#include "chatview/events.pb.h"

export module chatview.proto.events;

export namespace chatview::proto::events
{
using ::chatview::events::AdminUpdateEvent;
using ::chatview::events::EventService;
using ::chatview::events::ForceOfflineEvent;
using ::chatview::events::FriendStatusEvent;
using ::chatview::events::NewMessageEvent;
using ::chatview::events::ServerEvent;
using ::chatview::events::SubscribeReq;
using ::chatview::events::SystemBroadcastEvent;
}
