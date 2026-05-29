package service

import (
	"context"
	"strings"

	chatpb "chatview/api/gen/chatview/chat"
	"chatview/server/internal/contextx"
	"chatview/server/internal/db"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *ChatService) MarkConversationRead(ctx context.Context, req *chatpb.MarkConversationReadReq) (*chatpb.MarkConversationReadResp, error) {
	pubKey := contextx.PublicKey(ctx)
	peer := strings.TrimSpace(req.GetPeerPublicKey())
	if peer == "" || peer == pubKey {
		return nil, status.Error(codes.InvalidArgument, "invalid peer_public_key")
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
