package service

import (
	"context"
	"database/sql"
	"strings"
	"time"

	chatpb "chatview/api/gen/chatview/chat"
	eventspb "chatview/api/gen/chatview/events"
	"chatview/server/internal/contextx"
	"chatview/server/internal/db"

	"github.com/jmoiron/sqlx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type sendMessageInput struct {
	sender          string
	receiver        string
	text            string
	clientMessageID string
}

func (s *ChatService) SendMessage(ctx context.Context, req *chatpb.SendMessageReq) (*chatpb.SendMessageResp, error) {
	var lastErr error
	for attempt := range 3 {
		resp, err := s.sendMessageOnce(ctx, req)
		if err == nil {
			return resp, nil
		}
		if db.IsSerializationFailure(err) {
			lastErr = err
			time.Sleep(time.Duration(5*attempt*attempt) * time.Millisecond)
			continue
		}
		return nil, err
	}
	return nil, status.Errorf(codes.Aborted, "send message failed after retries: %v", lastErr)
}

func (s *ChatService) sendMessageOnce(ctx context.Context, req *chatpb.SendMessageReq) (*chatpb.SendMessageResp, error) {
	input, err := newSendMessageInput(ctx, req)
	if err != nil {
		return nil, err
	}
	if input.clientMessageID != "" {
		existing, found, err := s.lookupMessageByClientID(ctx, input.sender, input.clientMessageID)
		if err != nil {
			return nil, status.Error(codes.Internal, "database error")
		}
		if found {
			return existing.toSendMessageResp(input.sender, input.receiver, input.text)
		}
	}
	var receiverOK bool
	if err := s.Store.DB.GetContext(ctx, &receiverOK, `SELECT EXISTS(SELECT 1 FROM users WHERE pub_key = $1 AND status = 1)`, input.receiver); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if !receiverOK {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	tx, err := s.Store.DB.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	defer tx.Rollback()

	ts := time.Now().UTC()
	msgID, serverSeq, err := insertMessage(ctx, tx, input, ts)
	if err != nil {
		if db.IsSerializationFailure(err) {
			return nil, err
		}
		if input.clientMessageID != "" && db.IsUniqueViolation(err) {
			existing, found, lookupErr := s.lookupMessageByClientID(ctx, input.sender, input.clientMessageID)
			if lookupErr != nil {
				return nil, status.Error(codes.Internal, "database error")
			}
			if found {
				return existing.toSendMessageResp(input.sender, input.receiver, input.text)
			}
		}
		return nil, status.Error(codes.Internal, "database error")
	}
	if err := tx.Commit(); err != nil {
		if db.IsSerializationFailure(err) {
			return nil, err
		}
		return nil, status.Error(codes.Internal, "database error")
	}

	unread, err := s.unreadCount(ctx, input.receiver, input.sender)
	if err != nil || unread <= 0 {
		unread = 1
	}
	s.Hub.Push(input.receiver, &eventspb.ServerEvent{Event: &eventspb.ServerEvent_NewMessage{
		NewMessage: &eventspb.NewMessageEvent{FromPubKey: input.sender, Count: unread},
	}})
	return &chatpb.SendMessageResp{
		MessageId: msgID,
		Timestamp: ts.Format(time.RFC3339Nano),
		ServerSeq: serverSeq,
	}, nil
}

func newSendMessageInput(ctx context.Context, req *chatpb.SendMessageReq) (sendMessageInput, error) {
	input := sendMessageInput{
		sender:          contextx.PubKey(ctx),
		receiver:        strings.TrimSpace(req.GetReceiverPubKey()),
		text:            strings.TrimSpace(req.GetText()),
		clientMessageID: strings.TrimSpace(req.GetClientMessageId()),
	}
	if input.receiver == "" || input.receiver == input.sender {
		return sendMessageInput{}, status.Error(codes.InvalidArgument, "invalid receiver_pub_key")
	}
	if input.text == "" {
		return sendMessageInput{}, status.Error(codes.InvalidArgument, "text is required")
	}
	return input, nil
}

func insertMessage(ctx context.Context, tx *sqlx.Tx, input sendMessageInput, ts time.Time) (string, int64, error) {
	convID, err := db.EnsureConversation(ctx, tx, input.sender, input.receiver)
	if err != nil {
		return "", 0, err
	}
	var serverSeq int64
	if err := tx.GetContext(ctx, &serverSeq, `
		UPDATE conversations
		SET next_seq = next_seq + 1
		WHERE id = $1
		RETURNING next_seq - 1
	`, convID); err != nil {
		return "", 0, err
	}
	var msgID string
	err = tx.GetContext(ctx, &msgID, `
		INSERT INTO messages (conversation_id, sender_pub_key, text, timestamp, server_seq, client_message_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text
	`, convID, input.sender, input.text, ts, serverSeq, input.clientMessageID)
	return msgID, serverSeq, err
}
