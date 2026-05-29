package rpcclient

import (
	commonpb "chatview/api/gen/chatview/common"
	eventspb "chatview/api/gen/chatview/events"
)

func friendFromProto(friend *commonpb.FriendInfo) Friend {
	return Friend{
		PublicKey: friend.PubKey,
		Alias:     friend.Alias,
		Online:    friend.IsOnline,
		Unread:    friend.Unread,
	}
}

func messageFromProto(message *commonpb.ChatMessage) Message {
	if message == nil {
		return Message{}
	}
	return Message{
		ID:        message.Id,
		Sender:    message.SenderPubKey,
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

func eventFromProto(event *eventspb.ServerEvent) Event {
	if event == nil {
		return Event{Kind: "unknown"}
	}
	switch typed := event.Event.(type) {
	case *eventspb.ServerEvent_NewMessage:
		return Event{Kind: "new_message", PublicKey: typed.NewMessage.FromPubKey, Count: typed.NewMessage.Count}
	case *eventspb.ServerEvent_FriendStatus:
		return Event{Kind: "friend_status", PublicKey: typed.FriendStatus.PubKey}
	case *eventspb.ServerEvent_SystemBroadcast:
		return Event{Kind: "system_broadcast", Text: typed.SystemBroadcast.Text}
	case *eventspb.ServerEvent_ForceOffline:
		return Event{Kind: "force_offline", Reason: typed.ForceOffline.Reason}
	case *eventspb.ServerEvent_AdminUpdate:
		return Event{Kind: "admin_update"}
	default:
		return Event{Kind: "unknown"}
	}
}
