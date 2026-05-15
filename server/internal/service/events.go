package service

import (
	"context"

	eventspb "chatview/gen/chatview/events"
	"chatview/internal/contextx"
	"chatview/internal/db"
	"chatview/internal/eventhub"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type EventService struct {
	eventspb.UnimplementedEventServiceServer
	Store *db.Store
	Hub   *eventhub.Hub
}

func (s *EventService) Subscribe(req *eventspb.SubscribeReq, stream eventspb.EventService_SubscribeServer) error {
	principal, ok := contextx.PrincipalFrom(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "invalid or expired token")
	}
	clientID := req.GetClientId()
	if clientID == "" {
		clientID = principal.Token
	}
	ch := make(chan *eventspb.ServerEvent, 64)
	s.Hub.Register(principal.PubKey, clientID, ch)

	bg := context.Background()
	wasOnline, _ := s.Store.IsUserOnline(bg, principal.PubKey)
	_ = s.Store.MarkSessionClient(bg, principal.Token, clientID, true)
	if !wasOnline {
		s.notifyFriendsStatus(bg, principal.PubKey, true)
		s.notifyAdmins(bg)
	}

	defer func() {
		s.Hub.Unregister(principal.PubKey, clientID)
		_ = s.Store.MarkSessionClient(bg, principal.Token, clientID, false)
		isOnline, _ := s.Store.IsUserOnline(bg, principal.PubKey)
		if !isOnline {
			s.notifyFriendsStatus(bg, principal.PubKey, false)
			s.notifyAdmins(bg)
		}
	}()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

type watcherRow struct {
	PubKey string `db:"user_pub_key"`
	Alias  string `db:"alias"`
}

func (s *EventService) notifyFriendsStatus(ctx context.Context, pubKey string, online bool) {
	var watchers []watcherRow
	if err := s.Store.DB.SelectContext(ctx, &watchers, `
		SELECT user_pub_key, alias
		FROM friendships
		WHERE friend_pub_key = $1
	`, pubKey); err != nil {
		return
	}
	for _, watcher := range watchers {
		s.Hub.Push(watcher.PubKey, &eventspb.ServerEvent{Event: &eventspb.ServerEvent_FriendStatus{
			FriendStatus: &eventspb.FriendStatusEvent{
				PubKey:   pubKey,
				Alias:    watcher.Alias,
				IsOnline: online,
			},
		}})
	}
}

func (s *EventService) notifyAdmins(ctx context.Context) {
	s.Hub.PushAdmins(ctx, s.Store, &eventspb.ServerEvent{Event: &eventspb.ServerEvent_AdminUpdate{
		AdminUpdate: &eventspb.AdminUpdateEvent{},
	}})
}
