package service

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	chatpb "chatview/api/gen/chatview/chat"
	commonpb "chatview/api/gen/chatview/common"
	"chatview/server/internal/contextx"
	"chatview/server/internal/db"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type messageRow struct {
	ID              string    `db:"id"`
	SenderPublicKey string    `db:"sender_pub_key"`
	Text            string    `db:"text"`
	Timestamp       time.Time `db:"timestamp"`
	ServerSeq       int64     `db:"server_seq"`
}

func (s *ChatService) GetMessageHistory(ctx context.Context, req *chatpb.GetMessageHistoryReq) (*chatpb.GetMessageHistoryResp, error) {
	pubKey := contextx.PublicKey(ctx)
	limit := messageHistoryLimit(req.GetLimit())
	convID, err := db.LookupConversation(ctx, s.Store.DB, pubKey, req.GetPeerPublicKey())
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

	direction := normalizeDirection(req.GetDirection())
	query, args := historyQuery(convID, cursor, hasCursor, direction, limit+1)
	var rows []messageRow
	if err := s.Store.DB.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	hasMore := int32(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}
	if direction == "older" {
		slices.Reverse(rows)
	}

	page := &commonpb.MessageHistoryPage{Messages: make([]*commonpb.ChatMessage, 0, len(rows)), HasMore: hasMore}
	for _, row := range rows {
		delivery := commonpb.MessageDelivery_MESSAGE_DELIVERY_SENT
		if row.SenderPublicKey != pubKey {
			delivery = commonpb.MessageDelivery_MESSAGE_DELIVERY_INCOMING
		}
		page.Messages = append(page.Messages, &commonpb.ChatMessage{
			Id:              row.ID,
			SenderPublicKey: row.SenderPublicKey,
			Text:            row.Text,
			Timestamp:       row.Timestamp.UTC().Format(time.RFC3339Nano),
			Delivery:        delivery,
			ServerSeq:       row.ServerSeq,
		})
	}
	if hasMore && len(rows) > 0 {
		if direction == "newer" {
			page.NextCursor = strconv.FormatInt(rows[len(rows)-1].ServerSeq, 10)
		} else {
			page.NextCursor = strconv.FormatInt(rows[0].ServerSeq, 10)
		}
	}
	return &chatpb.GetMessageHistoryResp{Page: page}, nil
}

func messageHistoryLimit(raw int32) int32 {
	if raw <= 0 {
		return 30
	}
	return min(raw, 100)
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
	if direction == "newer" {
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
