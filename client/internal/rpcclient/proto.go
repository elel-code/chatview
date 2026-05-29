package rpcclient

import (
	commonpb "chatview/api/gen/chatview/common"
	eventspb "chatview/api/gen/chatview/events"
	"chatview/client/internal/domain"
)

func friendFromProto(friend *commonpb.FriendInfo) domain.Friend {
	return domain.Friend{
		PublicKey: friend.PublicKey,
		Alias:     friend.Alias,
		Online:    friend.IsOnline,
		Unread:    friend.Unread,
	}
}

func messageFromProto(message *commonpb.ChatMessage) domain.Message {
	if message == nil {
		return domain.Message{}
	}
	return domain.Message{
		ID:        message.Id,
		Sender:    message.SenderPublicKey,
		Text:      message.Text,
		Timestamp: message.Timestamp,
		Delivery:  deliveryString(message.Delivery),
		Error:     message.Error,
		ServerSeq: message.ServerSeq,
	}
}

func deliveryString(delivery commonpb.MessageDelivery) string {
	switch delivery {
	case commonpb.MessageDelivery_MESSAGE_DELIVERY_INCOMING:
		return "incoming"
	case commonpb.MessageDelivery_MESSAGE_DELIVERY_SENT:
		return "sent"
	case commonpb.MessageDelivery_MESSAGE_DELIVERY_FAILED:
		return "failed"
	default:
		return "pending"
	}
}

func eventFromProto(event *eventspb.ServerEvent) domain.Event {
	if event == nil {
		return domain.Event{Kind: "unknown"}
	}
	switch typed := event.Event.(type) {
	case *eventspb.ServerEvent_NewMessage:
		return domain.Event{Kind: "new_message", PublicKey: typed.NewMessage.FromPublicKey, Count: typed.NewMessage.Count}
	case *eventspb.ServerEvent_FriendStatus:
		return domain.Event{Kind: "friend_status", PublicKey: typed.FriendStatus.PublicKey}
	case *eventspb.ServerEvent_SystemBroadcast:
		return domain.Event{Kind: "system_broadcast", Text: typed.SystemBroadcast.Text}
	case *eventspb.ServerEvent_ForceOffline:
		return domain.Event{Kind: "force_offline", Reason: typed.ForceOffline.Reason}
	case *eventspb.ServerEvent_AdminUpdate:
		return domain.Event{Kind: "admin_update"}
	default:
		return domain.Event{Kind: "unknown"}
	}
}
