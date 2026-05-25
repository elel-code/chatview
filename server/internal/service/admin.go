package service

import (
	"context"

	adminpb "chatview/api/gen/chatview/admin"
	commonpb "chatview/api/gen/chatview/common"
	eventspb "chatview/api/gen/chatview/events"
	"chatview/server/internal/contextx"
	"chatview/server/internal/db"
	"chatview/server/internal/eventhub"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AdminService struct {
	adminpb.UnimplementedAdminServiceServer
	Store *db.Store
	Hub   *eventhub.Hub
}

func (s *AdminService) SetUserStatus(ctx context.Context, req *adminpb.SetUserStatusReq) (*adminpb.SetUserStatusResp, error) {
	if req.GetTargetPubKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_pub_key is required")
	}
	statusValue := int32(req.GetStatus())
	if statusValue != int32(commonpb.UserStatus_USER_STATUS_ACTIVE) && statusValue != int32(commonpb.UserStatus_USER_STATUS_BANNED) {
		return nil, status.Error(codes.InvalidArgument, "invalid status")
	}
	tx, err := s.Store.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	result, err := tx.ExecContext(ctx, `UPDATE users SET status = $2, updated_at = now() WHERE pub_key = $1`, req.GetTargetPubKey(), statusValue)
	if err == nil && statusValue == int32(commonpb.UserStatus_USER_STATUS_BANNED) {
		_, err = tx.ExecContext(ctx, `DELETE FROM challenges WHERE pub_key = $1`, req.GetTargetPubKey())
		if err == nil {
			_, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE pub_key = $1`, req.GetTargetPubKey())
		}
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, status.Error(codes.Internal, "database error")
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		_ = tx.Rollback()
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if err := tx.Commit(); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	if statusValue == int32(commonpb.UserStatus_USER_STATUS_BANNED) {
		s.Hub.Push(req.GetTargetPubKey(), &eventspb.ServerEvent{Event: &eventspb.ServerEvent_ForceOffline{
			ForceOffline: &eventspb.ForceOfflineEvent{Reason: "account_banned"},
		}})
		s.Hub.KickUser(req.GetTargetPubKey())
	}
	s.Hub.PushAdmins(ctx, s.Store, &eventspb.ServerEvent{Event: &eventspb.ServerEvent_AdminUpdate{
		AdminUpdate: &eventspb.AdminUpdateEvent{},
	}})
	return &adminpb.SetUserStatusResp{}, nil
}

func (s *AdminService) Broadcast(ctx context.Context, req *adminpb.BroadcastReq) (*adminpb.BroadcastResp, error) {
	if req.GetText() == "" {
		return nil, status.Error(codes.InvalidArgument, "text is required")
	}
	from := contextx.PubKey(ctx)
	s.Hub.Broadcast(&eventspb.ServerEvent{Event: &eventspb.ServerEvent_SystemBroadcast{
		SystemBroadcast: &eventspb.SystemBroadcastEvent{Text: req.GetText(), FromAdmin: from},
	}})
	return &adminpb.BroadcastResp{}, nil
}

type adminUserRow struct {
	PubKey   string `db:"pub_key"`
	IsOnline bool   `db:"is_online"`
	IsBanned bool   `db:"is_banned"`
}

func (s *AdminService) PollAdminEvents(ctx context.Context, _ *adminpb.PollAdminEventsReq) (*adminpb.PollAdminEventsResp, error) {
	var users []adminUserRow
	if err := s.Store.DB.SelectContext(ctx, &users, `
		SELECT u.pub_key,
		       EXISTS (
		           SELECT 1 FROM sessions ss
		           WHERE ss.pub_key = u.pub_key AND ss.is_online = true AND ss.expires_at > now()
		       ) AS is_online,
		       (u.status = 2) AS is_banned
		FROM users u
		ORDER BY u.created_at DESC
	`); err != nil {
		return nil, status.Error(codes.Internal, "database error")
	}
	stats := &commonpb.AdminStats{TotalUsers: int32(len(users))}
	resp := &adminpb.PollAdminEventsResp{Update: &commonpb.AdminUpdate{Stats: stats}}
	for _, row := range users {
		if row.IsOnline {
			stats.OnlineUsers++
		}
		if row.IsBanned {
			stats.BannedUsers++
		}
		resp.Update.Users = append(resp.Update.Users, &commonpb.UserInfo{
			PubKey:   row.PubKey,
			IsOnline: row.IsOnline,
			IsBanned: row.IsBanned,
		})
	}
	return resp, nil
}
