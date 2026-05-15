package service

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	chatpb "chatview/gen/chatview/chat"
	commonpb "chatview/gen/chatview/common"
	eventspb "chatview/gen/chatview/events"
	"chatview/internal/contextx"
	"chatview/internal/db"
	"chatview/internal/eventhub"

	"github.com/jmoiron/sqlx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ChatService struct {
	chatpb.UnimplementedChatServiceServer
	Store *db.Store
	Hub   *eventhub.Hub
}

type friendRow struct {
	PubKey   string `db:"pub_key"`
	Alias    string `db:"alias"`
	IsOnline bool   `db:"is_online"`
	Unread   int32  `db:"unread"`
}

func (s *ChatService) ListFriends(ctx context.Context, _ *chatpb.ListFriendsReq) (*chatpb.ListFriendsResp, error) {
	pubKey := contextx.PubKey(ctx)
	var rows []friendRow
	if err := s.Store.DB.SelectContext(ctx, &rows, `
		SELECT f.friend_pub_key AS pub_key, f.alias,
		       EXISTS (
		           SELECT 1 FROM sessions ss
		           WHERE ss.pub_key = f.friend_pub_key AND ss.is_online = true AND ss.expires_at > now()
		       ) AS is_online,
		       COALESCE((
		           SELECT COUNT(*)::int
		           FROM conversations c
		           JOIN messages m ON m.conversation_id = c.id
		           LEFT JOIN conversation_reads r ON r.conversation_id = c.id AND r.user_pub_key = $1
		           WHERE (
		               (c.participant_a = $1 AND c.participant_b = f.friend_pub_key)
		               OR (c.participant_a = f.friend_pub_key AND c.participant_b = $1)
		           )
		             AND m.sender_pub_key <> $1
		             AND m.server_seq > COALESCE(r.last_read_seq, 0)
		       ), 0) AS unread
		FROM friendships f
		WHERE f.user_pub_key = $1
		ORDER BY f.created_at DESC
	`, pubKey); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	resp := &chatpb.ListFriendsResp{Friends: make([]*commonpb.FriendInfo, 0, len(rows))}
	for _, row := range rows {
		resp.Friends = append(resp.Friends, &commonpb.FriendInfo{
			PubKey:   row.PubKey,
			Alias:    row.Alias,
			IsOnline: row.IsOnline,
			Unread:   row.Unread,
		})
	}
	return resp, nil
}

func (s *ChatService) AddFriend(ctx context.Context, req *chatpb.AddFriendReq) (*chatpb.AddFriendResp, error) {
	pubKey := contextx.PubKey(ctx)
	target := strings.TrimSpace(req.GetTargetPubKey())
	if target == "" || target == pubKey {
		return nil, status.Error(codes.InvalidArgument, "invalid target_pub_key")
	}
	var exists bool
	if err := s.Store.DB.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM users WHERE pub_key = $1 AND status = 1)`, target); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if !exists {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if _, err := s.Store.DB.ExecContext(ctx, `
		INSERT INTO friendships (user_pub_key, friend_pub_key)
		VALUES ($1, $2)
		ON CONFLICT (user_pub_key, friend_pub_key) DO NOTHING
	`, pubKey, target); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	return &chatpb.AddFriendResp{}, nil
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
	sender := contextx.PubKey(ctx)
	receiver := strings.TrimSpace(req.GetReceiverPubKey())
	text := strings.TrimSpace(req.GetText())
	clientMessageID := strings.TrimSpace(req.GetClientMessageId())
	if receiver == "" || receiver == sender {
		return nil, status.Error(codes.InvalidArgument, "invalid receiver_pub_key")
	}
	if text == "" {
		return nil, status.Error(codes.InvalidArgument, "text is required")
	}
	if clientMessageID != "" {
		existing, found, err := s.lookupMessageByClientID(ctx, sender, clientMessageID)
		if err != nil {
			return nil, status.Error(codes.Internal, "database error")
		}
		if found {
			return existing.toSendMessageResp(sender, receiver, text)
		}
	}
	var receiverOK bool
	if err := s.Store.DB.GetContext(ctx, &receiverOK, `SELECT EXISTS(SELECT 1 FROM users WHERE pub_key = $1 AND status = 1)`, receiver); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if !receiverOK {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	tx, err := s.Store.DB.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	ts := time.Now().UTC()
	var msgID string
	var serverSeq int64
	err = func(tx *sqlx.Tx) error {
		convID, err := db.EnsureConversation(ctx, tx, sender, receiver)
		if err != nil {
			return err
		}
		if err := tx.GetContext(ctx, &serverSeq, `
			UPDATE conversations
			SET next_seq = next_seq + 1
			WHERE id = $1
			RETURNING next_seq - 1
		`, convID); err != nil {
			return err
		}
		err = tx.GetContext(ctx, &msgID, `
			INSERT INTO messages (conversation_id, sender_pub_key, text, timestamp, server_seq, client_message_id)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id::text
		`, convID, sender, text, ts, serverSeq, clientMessageID)
		return err
	}(tx)
	if err != nil {
		_ = tx.Rollback()
		if db.IsSerializationFailure(err) {
			return nil, err
		}
		if clientMessageID != "" && db.IsUniqueViolation(err) {
			existing, found, lookupErr := s.lookupMessageByClientID(ctx, sender, clientMessageID)
			if lookupErr != nil {
				return nil, status.Error(codes.Internal, "database error")
			}
			if found {
				return existing.toSendMessageResp(sender, receiver, text)
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

	unread, err := s.unreadCount(ctx, receiver, sender)
	if err != nil || unread <= 0 {
		unread = 1
	}
	s.Hub.Push(receiver, &eventspb.ServerEvent{Event: &eventspb.ServerEvent_NewMessage{
		NewMessage: &eventspb.NewMessageEvent{FromPubKey: sender, Count: unread},
	}})
	return &chatpb.SendMessageResp{
		MessageId: msgID,
		Timestamp: ts.Format(time.RFC3339Nano),
		ServerSeq: serverSeq,
	}, nil
}

type messageRow struct {
	ID           string    `db:"id"`
	SenderPubKey string    `db:"sender_pub_key"`
	Text         string    `db:"text"`
	Timestamp    time.Time `db:"timestamp"`
	ServerSeq    int64     `db:"server_seq"`
}

func (s *ChatService) GetMessageHistory(ctx context.Context, req *chatpb.GetMessageHistoryReq) (*chatpb.GetMessageHistoryResp, error) {
	pubKey := contextx.PubKey(ctx)
	limit := req.GetLimit()
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	convID, err := db.LookupConversation(ctx, s.Store.DB, pubKey, req.GetPeerPubKey())
	if err != nil {
		if db.IsNotFound(err) {
			return &chatpb.GetMessageHistoryResp{Page: &commonpb.MessageHistoryPage{}}, nil
		}
		return nil, status.Error(codes.Internal, "database error")
	}
	cursor, hasCursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid cursor")
	}

	query, args := historyQuery(convID, cursor, hasCursor, req.GetDirection(), limit+1)
	var rows []messageRow
	if err := s.Store.DB.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	hasMore := int32(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}
	if normalizeDirection(req.GetDirection()) == "older" {
		reverseMessages(rows)
	}

	page := &commonpb.MessageHistoryPage{Messages: make([]*commonpb.ChatMessage, 0, len(rows)), HasMore: hasMore}
	for _, row := range rows {
		delivery := commonpb.MessageDelivery_MESSAGE_DELIVERY_SENT
		if row.SenderPubKey != pubKey {
			delivery = commonpb.MessageDelivery_MESSAGE_DELIVERY_INCOMING
		}
		page.Messages = append(page.Messages, &commonpb.ChatMessage{
			Id:           row.ID,
			SenderPubKey: row.SenderPubKey,
			Text:         row.Text,
			Timestamp:    row.Timestamp.UTC().Format(time.RFC3339Nano),
			Delivery:     delivery,
			ServerSeq:    row.ServerSeq,
		})
	}
	if hasMore && len(rows) > 0 {
		if normalizeDirection(req.GetDirection()) == "newer" {
			page.NextCursor = fmt.Sprintf("%d", rows[len(rows)-1].ServerSeq)
		} else {
			page.NextCursor = fmt.Sprintf("%d", rows[0].ServerSeq)
		}
	}
	return &chatpb.GetMessageHistoryResp{Page: page}, nil
}

func (s *ChatService) MarkConversationRead(ctx context.Context, req *chatpb.MarkConversationReadReq) (*chatpb.MarkConversationReadResp, error) {
	pubKey := contextx.PubKey(ctx)
	peer := strings.TrimSpace(req.GetPeerPubKey())
	if peer == "" || peer == pubKey {
		return nil, status.Error(codes.InvalidArgument, "invalid peer_pub_key")
	}
	convID, err := db.LookupConversation(ctx, s.Store.DB, pubKey, peer)
	if err != nil {
		if db.IsNotFound(err) {
			return &chatpb.MarkConversationReadResp{}, nil
		}
		return nil, status.Error(codes.Internal, "database error")
	}
	var maxSeq int64
	if err := s.Store.DB.GetContext(ctx, &maxSeq, `
		SELECT COALESCE(MAX(server_seq), 0)
		FROM messages
		WHERE conversation_id = $1
	`, convID); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	lastRead := req.GetLastReadServerSeq()
	if lastRead <= 0 || lastRead > maxSeq {
		lastRead = maxSeq
	}
	_, err = s.Store.DB.ExecContext(ctx, `
		INSERT INTO conversation_reads (conversation_id, user_pub_key, last_read_seq)
		VALUES ($1, $2, $3)
		ON CONFLICT (conversation_id, user_pub_key) DO UPDATE
		SET last_read_seq = GREATEST(conversation_reads.last_read_seq, EXCLUDED.last_read_seq),
		    updated_at = now()
	`, convID, pubKey, lastRead)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	return &chatpb.MarkConversationReadResp{}, nil
}

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

func (s *ChatService) unreadCount(ctx context.Context, userPubKey, peerPubKey string) (int32, error) {
	convID, err := db.LookupConversation(ctx, s.Store.DB, userPubKey, peerPubKey)
	if err != nil {
		return 0, err
	}
	var count int32
	err = s.Store.DB.GetContext(ctx, &count, `
		SELECT COUNT(*)::int
		FROM messages m
		LEFT JOIN conversation_reads r ON r.conversation_id = m.conversation_id AND r.user_pub_key = $2
		WHERE m.conversation_id = $1
		  AND m.sender_pub_key <> $2
		  AND m.server_seq > COALESCE(r.last_read_seq, 0)
	`, convID, userPubKey)
	return count, err
}

func parseCursor(raw string) (int64, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, false, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	return n, true, err
}

func normalizeDirection(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "newer") {
		return "newer"
	}
	return "older"
}

func historyQuery(convID string, cursor int64, hasCursor bool, direction string, limit int32) (string, []any) {
	args := []any{convID, limit}
	if normalizeDirection(direction) == "newer" {
		if hasCursor {
			args = []any{convID, cursor, limit}
			return `
				SELECT id::text AS id, sender_pub_key, text, timestamp, server_seq
				FROM messages
				WHERE conversation_id = $1 AND server_seq > $2
				ORDER BY server_seq ASC
				LIMIT $3
			`, args
		}
		return `
			SELECT id::text AS id, sender_pub_key, text, timestamp, server_seq
			FROM messages
			WHERE conversation_id = $1
			ORDER BY server_seq ASC
			LIMIT $2
		`, args
	}
	if hasCursor {
		args = []any{convID, cursor, limit}
		return `
			SELECT id::text AS id, sender_pub_key, text, timestamp, server_seq
			FROM messages
			WHERE conversation_id = $1 AND server_seq < $2
			ORDER BY server_seq DESC
			LIMIT $3
		`, args
	}
	return `
		SELECT id::text AS id, sender_pub_key, text, timestamp, server_seq
		FROM messages
		WHERE conversation_id = $1
		ORDER BY server_seq DESC
		LIMIT $2
	`, args
}

func reverseMessages(rows []messageRow) {
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}
