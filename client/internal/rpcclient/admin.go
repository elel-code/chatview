package rpcclient

import (
	"context"

	adminpb "chatview/api/gen/chatview/admin"
	commonpb "chatview/api/gen/chatview/common"
	"chatview/client/internal/domain"
)

func (c *Client) PollAdminEvents(ctx context.Context) (domain.AdminUpdate, error) {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	resp, err := c.admin.PollAdminEvents(ctx, &adminpb.PollAdminEventsReq{})
	if err != nil {
		return domain.AdminUpdate{}, rpcError(err)
	}
	if resp.Update == nil {
		return domain.AdminUpdate{}, nil
	}
	update := domain.AdminUpdate{}
	if resp.Update.Stats != nil {
		update.Stats = domain.AdminStats{
			OnlineUsers: resp.Update.Stats.OnlineUsers,
			TotalUsers:  resp.Update.Stats.TotalUsers,
			BannedUsers: resp.Update.Stats.BannedUsers,
		}
	}
	update.Users = mapSlice(resp.Update.Users, func(user *commonpb.UserInfo) domain.UserInfo {
		return domain.UserInfo{
			PublicKey: user.PublicKey,
			Online:    user.IsOnline,
			Banned:    user.IsBanned,
		}
	})
	return update, nil
}

func (c *Client) SetUserStatus(ctx context.Context, publicKey string, banned bool) error {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	statusValue := commonpb.UserStatus_USER_STATUS_ACTIVE
	if banned {
		statusValue = commonpb.UserStatus_USER_STATUS_BANNED
	}
	_, err := c.admin.SetUserStatus(ctx, &adminpb.SetUserStatusReq{
		TargetPublicKey: publicKey,
		Status:          statusValue,
	})
	return rpcError(err)
}

func (c *Client) Broadcast(ctx context.Context, text string) error {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	_, err := c.admin.Broadcast(ctx, &adminpb.BroadcastReq{Text: text})
	return rpcError(err)
}
