package service

import (
	"context"
	"time"

	chatpb "chatview/api/gen/chatview/chat"
	"chatview/server/internal/db"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type existingMessageRow struct {
	ID           string    `db:"id"`
	Text         string    `db:"text"`
	Timestamp    time.Time `db:"timestamp"`
	ServerSeq    int64     `db:"server_seq"`
	ParticipantA string    `db:"participant_a"`
	ParticipantB string    `db:"participant_b"`
}

func (s *ChatService) lookupMessageByClientID(ctx context.Context, sender, clientMessageID string) (existingMessageRow, bool, error) {
	var row existingMessageRow
	err := s.Store.DB.GetContext(ctx, &row, `
		SELECT m.id::text AS id, m.text, m.timestamp, m.server_seq, c.participant_a, c.participant_b
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id
		WHERE m.sender_pub_key = $1 AND m.client_message_id = $2
	`, sender, clientMessageID)
	if err != nil {
		if db.IsNotFound(err) {
			return existingMessageRow{}, false, nil
		}
		return existingMessageRow{}, false, err
	}
	return row, true, nil
}

func (r existingMessageRow) toSendMessageResp(sender, receiver, text string) (*chatpb.SendMessageResp, error) {
	existingReceiver := r.ParticipantA
	if existingReceiver == sender {
		existingReceiver = r.ParticipantB
	}
	if existingReceiver != receiver || r.Text != text {
		return nil, status.Error(codes.AlreadyExists, "client_message_id already used with different message")
	}
	return &chatpb.SendMessageResp{
		MessageId:    r.ID,
		Timestamp:    r.Timestamp.UTC().Format(time.RFC3339Nano),
		ServerSeq:    r.ServerSeq,
		Deduplicated: true,
	}, nil
}
