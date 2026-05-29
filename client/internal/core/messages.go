package core

import (
	"context"
	"errors"
	"strings"

	"chatview/client/internal/domain"
	"chatview/client/internal/storage"
)

func (s *Service) SendMessage(ctx context.Context, receiverPublicKey string, text string) (domain.Message, error) {
	receiverPublicKey = strings.TrimSpace(receiverPublicKey)
	if receiverPublicKey == "" {
		return domain.Message{}, errors.New("receiver public key is required")
	}
	if strings.TrimSpace(text) == "" {
		return domain.Message{}, errors.New("message is required")
	}
	clientID := newClientMessageID()
	now := nowRFC3339()
	pending := domain.Message{
		ID:        clientID,
		Sender:    s.PublicKey(),
		Text:      text,
		Timestamp: now,
		Delivery:  "pending",
	}
	if s.cache == nil {
		message, err := s.rpc.SendMessageWithID(ctx, receiverPublicKey, text, clientID)
		if err != nil {
			return domain.Message{}, err
		}
		return domain.Message{
			ID:        message.ID,
			Sender:    s.PublicKey(),
			Text:      text,
			Timestamp: message.Timestamp,
			Delivery:  "sent",
			ServerSeq: message.ServerSeq,
		}, nil
	}
	if err := s.cache.EnqueueOutbox(storage.OutboxItem{
		ID:          clientID,
		ReceiverKey: receiverPublicKey,
		Text:        text,
		Status:      0,
		CreatedAt:   now,
	}, pending.Sender); err != nil {
		return domain.Message{}, err
	}
	if s.IsOffline() {
		return pending, nil
	}
	if err := s.sendOutboxItem(ctx, storage.OutboxItem{ID: clientID, ReceiverKey: receiverPublicKey, Text: text}); err != nil {
		pending.Error = err.Error()
		return pending, nil
	}
	item, err := s.cache.MessageByClientID(clientID)
	if err == nil && item.ID != "" {
		return messagesFromCache([]storage.Message{item})[0], nil
	}
	return domain.Message{
		ID:        clientID,
		Sender:    pending.Sender,
		Text:      text,
		Timestamp: now,
		Delivery:  "sent",
	}, nil
}

func (s *Service) MarkConversationRead(ctx context.Context, peerPublicKey string, seq int64) error {
	if !s.IsOffline() {
		if err := s.rpc.MarkConversationRead(ctx, peerPublicKey, seq); err != nil {
			return err
		}
	}
	if s.cache != nil {
		_ = s.cache.MarkConversationRead(peerPublicKey, seq)
	}
	return nil
}
