package core

import (
	"chatview/client/internal/rpcclient"
	"chatview/client/internal/storage"
)

func friendsFromCache(friends []storage.Friend) []Friend {
	return mapSlice(friends, func(friend storage.Friend) Friend {
		return Friend{
			PublicKey: friend.PublicKey,
			Alias:     friend.Alias,
			Online:    friend.Online,
			Unread:    friend.Unread,
		}
	})
}

func friendsFromRPC(friends []rpcclient.Friend) []Friend {
	return mapSlice(friends, func(friend rpcclient.Friend) Friend {
		return Friend{
			PublicKey: friend.PublicKey,
			Alias:     friend.Alias,
			Online:    friend.Online,
			Unread:    friend.Unread,
		}
	})
}

func friendsToCache(friends []Friend) []storage.Friend {
	return mapSlice(friends, func(friend Friend) storage.Friend {
		return storage.Friend{
			PublicKey: friend.PublicKey,
			Alias:     friend.Alias,
			Online:    friend.Online,
			Unread:    friend.Unread,
		}
	})
}

func messagesFromCache(messages []storage.Message) []Message {
	return mapSlice(messages, func(message storage.Message) Message {
		return Message{
			ID:        message.ID,
			Sender:    message.Sender,
			Text:      message.Text,
			Timestamp: message.Timestamp,
			Delivery:  message.Delivery,
			Error:     message.Error,
			ServerSeq: message.ServerSeq,
		}
	})
}

func messagesFromRPC(messages []rpcclient.Message) []Message {
	return mapSlice(messages, func(message rpcclient.Message) Message {
		return Message{
			ID:        message.ID,
			Sender:    message.Sender,
			Text:      message.Text,
			Timestamp: message.Timestamp,
			Delivery:  message.Delivery,
			Error:     message.Error,
			ServerSeq: message.ServerSeq,
		}
	})
}

func messagesToCache(peerKey string, messages []Message) []storage.Message {
	return mapSlice(messages, func(message Message) storage.Message {
		return storage.Message{
			ID:        message.ID,
			ClientID:  message.ID,
			PeerKey:   peerKey,
			Sender:    message.Sender,
			Text:      message.Text,
			Timestamp: message.Timestamp,
			Delivery:  message.Delivery,
			Error:     message.Error,
			ServerSeq: message.ServerSeq,
		}
	})
}
