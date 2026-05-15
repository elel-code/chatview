module;

#include "chatview/common/types.pb.h"

export module chatview.proto.common;

export namespace chatview::proto::common
{
using ::chatview::common::AdminStats;
using ::chatview::common::AdminUpdate;
using ::chatview::common::ChatMessage;
using ::chatview::common::Empty;
using ::chatview::common::FriendInfo;
using ::chatview::common::MessageDelivery;
using ::chatview::common::MESSAGE_DELIVERY_FAILED;
using ::chatview::common::MESSAGE_DELIVERY_INCOMING;
using ::chatview::common::MESSAGE_DELIVERY_PENDING;
using ::chatview::common::MESSAGE_DELIVERY_SENT;
using ::chatview::common::MESSAGE_DELIVERY_UNSPECIFIED;
using ::chatview::common::MessageHistoryPage;
using ::chatview::common::OnlineStatus;
using ::chatview::common::ONLINE_STATUS_OFFLINE;
using ::chatview::common::ONLINE_STATUS_ONLINE;
using ::chatview::common::ONLINE_STATUS_UNSPECIFIED;
using ::chatview::common::OperationResult;
using ::chatview::common::UserInfo;
using ::chatview::common::UserStatus;
using ::chatview::common::USER_STATUS_ACTIVE;
using ::chatview::common::USER_STATUS_BANNED;
using ::chatview::common::USER_STATUS_UNSPECIFIED;
}
