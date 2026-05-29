package eventhub

import (
	"testing"

	eventspb "chatview/api/gen/chatview/events"
)

func TestHubPushBroadcastUnregisterAndKick(t *testing.T) {
	hub := New()
	userA := make(chan *eventspb.ServerEvent, 4)
	userB := make(chan *eventspb.ServerEvent, 4)
	hub.Register("user-a", "client-a", userA)
	hub.Register("user-b", "client-b", userB)

	targeted := &eventspb.ServerEvent{Event: &eventspb.ServerEvent_SystemBroadcast{
		SystemBroadcast: &eventspb.SystemBroadcastEvent{Text: "targeted"},
	}}
	hub.Push("user-a", targeted)
	if got := <-userA; got != targeted {
		t.Fatalf("targeted event mismatch")
	}
	assertNoEvent(t, userB)

	broadcast := &eventspb.ServerEvent{Event: &eventspb.ServerEvent_SystemBroadcast{
		SystemBroadcast: &eventspb.SystemBroadcastEvent{Text: "all"},
	}}
	hub.Broadcast(broadcast)
	if got := <-userA; got != broadcast {
		t.Fatalf("broadcast user-a mismatch")
	}
	if got := <-userB; got != broadcast {
		t.Fatalf("broadcast user-b mismatch")
	}

	online := hub.OnlinePubKeys()
	if !online["user-a"] || !online["user-b"] || len(online) != 2 {
		t.Fatalf("unexpected online users: %#v", online)
	}

	hub.Unregister("user-a", "client-a")
	hub.Push("user-a", targeted)
	assertNoEvent(t, userA)
	if online := hub.OnlinePubKeys(); online["user-a"] || !online["user-b"] {
		t.Fatalf("unexpected online users after unregister: %#v", online)
	}

	hub.KickUser("user-b")
	if _, ok := <-userB; ok {
		t.Fatal("user-b channel was not closed")
	}
	if online := hub.OnlinePubKeys(); len(online) != 0 {
		t.Fatalf("unexpected online users after kick: %#v", online)
	}
}

func TestHubDropsWhenChannelIsFull(t *testing.T) {
	hub := New()
	ch := make(chan *eventspb.ServerEvent)
	hub.Register("user", "client", ch)

	done := make(chan struct{})
	go func() {
		hub.Push("user", &eventspb.ServerEvent{})
		close(done)
	}()
	<-done
}

func assertNoEvent(t *testing.T, ch <-chan *eventspb.ServerEvent) {
	t.Helper()
	select {
	case event := <-ch:
		t.Fatalf("unexpected event: %#v", event)
	default:
	}
}
