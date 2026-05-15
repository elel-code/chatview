module;

#include "chatview/admin.grpc.pb.h"
#include "chatview/admin.pb.h"

export module chatview.proto.admin;

export import chatview.proto.common;

export namespace chatview::proto::admin
{
using ::chatview::admin::AdminService;
using ::chatview::admin::BroadcastReq;
using ::chatview::admin::BroadcastResp;
using ::chatview::admin::PollAdminEventsReq;
using ::chatview::admin::PollAdminEventsResp;
using ::chatview::admin::SetUserStatusReq;
using ::chatview::admin::SetUserStatusResp;
}
