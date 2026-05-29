package service

import (
	"context"
	"strings"

	chatpb "chatview/api/gen/chatview/chat"
	commonpb "chatview/api/gen/chatview/common"
	"chatview/server/internal/contextx"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
