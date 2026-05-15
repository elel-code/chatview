module;

#include "chatview/auth.grpc.pb.h"
#include "chatview/auth.pb.h"

export module chatview.proto.auth;

export namespace chatview::proto::auth
{
using ::chatview::auth::AuthService;
using ::chatview::auth::LoginReq;
using ::chatview::auth::LoginResp;
using ::chatview::auth::RequestChallengeReq;
using ::chatview::auth::RequestChallengeResp;
}
