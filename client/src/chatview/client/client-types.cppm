module;

#include "client-types.hpp"

export module chatview.client:types;

export namespace chatview::client
{
using ::chatview::client::AdminStats;
using ::chatview::client::AdminUpdate;
using ::chatview::client::AuthLockState;
using ::chatview::client::ChatMessage;
using ::chatview::client::ExpectedVoid;
using ::chatview::client::Friend;
using ::chatview::client::IdentityResult;
using ::chatview::client::LoginResult;
using ::chatview::client::MessageHistoryPage;
using ::chatview::client::MessageHistoryQuery;
using ::chatview::client::NativeClientOptions;
using ::chatview::client::OutboxStatus;
using ::chatview::client::SendMessageResult;
using ::chatview::client::UserInfo;
} // namespace chatview::client
