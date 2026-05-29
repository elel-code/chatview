package service

import (
	"context"
	"testing"

	adminpb "chatview/api/gen/chatview/admin"
	eventspb "chatview/api/gen/chatview/events"
	"chatview/server/internal/contextx"
	"chatview/server/internal/eventhub"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBroadcastValidatesTextAndPublishesEvent(t *testing.T) {
	hub := eventhub.New()
	events := make(chan *eventspb.ServerEvent, 1)
	hub.Register("user", "client", events)

	service := &AdminService{Hub: hub}
	ctx := contextx.WithPrincipal(context.Background(), contextx.Principal{PubKey: "admin"})
	if _, err := service.Broadcast(ctx, &adminpb.BroadcastReq{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty broadcast code = %s, want %s", status.Code(err), codes.InvalidArgument)
	}
	if _, err := service.Broadcast(ctx, &adminpb.BroadcastReq{Text: "maintenance"}); err != nil {
		t.Fatal(err)
	}

	event := <-events
	broadcast := event.GetSystemBroadcast()
	if broadcast == nil || broadcast.Text != "maintenance" || broadcast.FromAdmin != "admin" {
		t.Fatalf("unexpected broadcast event: %#v", event)
	}
}
