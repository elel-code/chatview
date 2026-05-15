module;

#include "chatview/chat.grpc.pb.h"
#include "chatview/chat.pb.h"

export module chatview.proto.chat;

export import chatview.proto.common;

export namespace chatview::proto::chat
{
using ::chatview::chat::AddFriendReq;
using ::chatview::chat::AddFriendResp;
using ::chatview::chat::ChatService;
using ::chatview::chat::GetMessageHistoryReq;
using ::chatview::chat::GetMessageHistoryResp;
using ::chatview::chat::ListFriendsReq;
using ::chatview::chat::ListFriendsResp;
using ::chatview::chat::MarkConversationReadReq;
using ::chatview::chat::MarkConversationReadResp;
using ::chatview::chat::SendMessageReq;
using ::chatview::chat::SendMessageResp;
}
