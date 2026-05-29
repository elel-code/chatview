package core

import (
	"chatview/client/internal/domain"
	"chatview/client/internal/storage"
)

func friendsFromCache(friends []storage.Friend) []domain.Friend {
	return mapSlice(friends, func(friend storage.Friend) domain.Friend {
		return domain.Friend{
			PublicKey: friend.PublicKey,
			Alias:     friend.Alias,
			Online:    friend.Online,
			Unread:    friend.Unread,
		}
	})
}

func friendsToCache(friends []domain.Friend) []storage.Friend {
	return mapSlice(friends, func(friend domain.Friend) storage.Friend {
		return storage.Friend{
			PublicKey: friend.PublicKey,
			Alias:     friend.Alias,
			Online:    friend.Online,
			Unread:    friend.Unread,
		}
	})
}

func messagesFromCache(messages []storage.Message) []domain.Message {
	return mapSlice(messages, func(message storage.Message) domain.Message {
		return domain.Message{
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

func messagesToCache(peerKey string, messages []domain.Message) []storage.Message {
	return mapSlice(messages, func(message domain.Message) storage.Message {
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
