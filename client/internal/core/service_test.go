package core

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"chatview/client/internal/identity"
	"chatview/client/internal/rpcclient"
	"chatview/client/internal/storage"
)

func TestServiceLocksAfterBadPINAttempts(t *testing.T) {
	store := identity.NewStore(filepath.Join(t.TempDir(), "identity-go.bin"))
	service := NewService(store, nil, nil)

	if _, err := service.CreateIdentity("123456"); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := service.Login(context.Background(), "000000"); err == nil || err.Error() != "wrong pin" {
			t.Fatalf("attempt %d: expected wrong pin, got %v", attempt, err)
		}
	}

	state := service.AuthLockState()
	if state.RemainingAttempts != 0 || state.LockedUntil == "" {
		t.Fatalf("expected lockout, got %#v", state)
	}
	if _, err := service.Login(context.Background(), "123456"); err == nil || err.Error() != "too many attempts" {
		t.Fatalf("expected lockout error, got %v", err)
	}
}

func TestServiceValidatesInputsAndOfflineWrites(t *testing.T) {
	service := NewService(nil, nil, nil)

	if err := service.AddFriend(context.Background(), ""); err == nil || err.Error() != "public key is required" {
		t.Fatalf("expected public key validation, got %v", err)
	}
	if _, err := service.GetHistory(context.Background(), "", "", 50, "older"); err == nil || err.Error() != "peer public key is required" {
		t.Fatalf("expected peer validation, got %v", err)
	}
	if _, err := service.SendMessage(context.Background(), "", "hello"); err == nil || err.Error() != "receiver public key is required" {
		t.Fatalf("expected receiver validation, got %v", err)
	}
	if _, err := service.SendMessage(context.Background(), "peer", "  "); err == nil || err.Error() != "message is required" {
		t.Fatalf("expected message validation, got %v", err)
	}

	service.setSession("me", 0, true)
	if err := service.AddFriend(context.Background(), "peer"); err == nil || err.Error() != "offline" {
		t.Fatalf("expected offline add friend error, got %v", err)
	}
	if err := service.Broadcast(context.Background(), "hello"); err == nil || err.Error() != "offline" {
		t.Fatalf("expected offline broadcast error, got %v", err)
	}
	if err := service.SetUserStatus(context.Background(), "peer", true); err == nil || err.Error() != "offline" {
		t.Fatalf("expected offline set user status error, got %v", err)
	}
	if _, err := service.PollAdminEvents(context.Background()); err == nil || err.Error() != "offline" {
		t.Fatalf("expected offline admin poll error, got %v", err)
	}
}

func TestServiceSyncConversationFillsServerSeqGaps(t *testing.T) {
	cache, err := storage.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	if err := cache.SetOwner("me"); err != nil {
		t.Fatal(err)
	}
	if err := cache.SaveMessages("alice", []storage.Message{
		{ID: "m1", Sender: "alice", Text: "one", Timestamp: "2026-01-01T00:00:01Z", Delivery: "incoming", ServerSeq: 1},
		{ID: "m2", Sender: "alice", Text: "two", Timestamp: "2026-01-01T00:00:02Z", Delivery: "incoming", ServerSeq: 2},
		{ID: "m5", Sender: "alice", Text: "five", Timestamp: "2026-01-01T00:00:05Z", Delivery: "incoming", ServerSeq: 5},
	}); err != nil {
		t.Fatal(err)
	}

	rpc := &syncFakeRPC{history: []rpcclient.Message{
		{ID: "m1", Sender: "alice", Text: "one", Timestamp: "2026-01-01T00:00:01Z", Delivery: "incoming", ServerSeq: 1},
		{ID: "m2", Sender: "alice", Text: "two", Timestamp: "2026-01-01T00:00:02Z", Delivery: "incoming", ServerSeq: 2},
		{ID: "m3", Sender: "alice", Text: "three", Timestamp: "2026-01-01T00:00:03Z", Delivery: "incoming", ServerSeq: 3},
		{ID: "m4", Sender: "alice", Text: "four", Timestamp: "2026-01-01T00:00:04Z", Delivery: "incoming", ServerSeq: 4},
		{ID: "m5", Sender: "alice", Text: "five", Timestamp: "2026-01-01T00:00:05Z", Delivery: "incoming", ServerSeq: 5},
		{ID: "m6", Sender: "alice", Text: "six", Timestamp: "2026-01-01T00:00:06Z", Delivery: "incoming", ServerSeq: 6},
	}}
	service := NewService(nil, rpc, cache)
	service.setSession("me", 0, false)

	page, err := service.SyncConversation(context.Background(), "alice", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !containsServerSeq(page.Messages, 3) || !containsServerSeq(page.Messages, 4) || !containsServerSeq(page.Messages, 6) {
		t.Fatalf("sync did not return expected messages: %#v", page.Messages)
	}

	messages, err := cache.Messages("alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 6 {
		t.Fatalf("expected six cached messages, got %#v", messages)
	}
	for i, message := range messages {
		if want := int64(i + 1); message.ServerSeq != want {
			t.Fatalf("message %d has seq %d, want %d: %#v", i, message.ServerSeq, want, messages)
		}
	}
	if previous, missing, ok, err := cache.FirstServerSeqGap("alice"); err != nil || ok {
		t.Fatalf("unexpected remaining gap: previous=%d missing=%d ok=%v err=%v", previous, missing, ok, err)
	}
	if len(rpc.calls) < 1 || rpc.calls[0].direction != "newer" || rpc.calls[0].cursor != "2" {
		t.Fatalf("expected first sync call to fill from seq 2, got %#v", rpc.calls)
	}
}

type historyCall struct {
	cursor    string
	direction string
}

type syncFakeRPC struct {
	history []rpcclient.Message
	calls   []historyCall
}

func (r *syncFakeRPC) Login(context.Context, string, func([]byte) []byte) (rpcclient.LoginResult, error) {
	return rpcclient.LoginResult{}, nil
}

func (r *syncFakeRPC) ClearSession() {}

func (r *syncFakeRPC) ListFriends(context.Context) ([]rpcclient.Friend, error) {
	return nil, nil
}

func (r *syncFakeRPC) AddFriend(context.Context, string) error {
	return nil
}

func (r *syncFakeRPC) GetHistory(_ context.Context, _ string, cursor string, limit int32, direction string) (rpcclient.HistoryPage, error) {
	r.calls = append(r.calls, historyCall{cursor: cursor, direction: direction})
	if limit <= 0 {
		limit = 30
	}
	cursorSeq, _ := strconv.ParseInt(cursor, 10, 64)
	var messages []rpcclient.Message
	if direction == "newer" {
		for _, message := range r.history {
			if cursor == "" || message.ServerSeq > cursorSeq {
				messages = append(messages, message)
			}
		}
	} else {
		for i := len(r.history) - 1; i >= 0; i-- {
			message := r.history[i]
			if cursor == "" || message.ServerSeq < cursorSeq {
				messages = append(messages, message)
			}
		}
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}
	hasMore := int32(len(messages)) > limit
	if hasMore {
		messages = messages[:limit]
	}
	nextCursor := ""
	if hasMore && len(messages) > 0 {
		nextCursor = strconv.FormatInt(messages[len(messages)-1].ServerSeq, 10)
	}
	return rpcclient.HistoryPage{Messages: messages, NextCursor: nextCursor, HasMore: hasMore}, nil
}

func (r *syncFakeRPC) SendMessageWithID(context.Context, string, string, string) (rpcclient.SendResult, error) {
	return rpcclient.SendResult{}, nil
}

func (r *syncFakeRPC) MarkConversationRead(context.Context, string, int64) error {
	return nil
}

func (r *syncFakeRPC) Subscribe(context.Context) (<-chan rpcclient.Event, <-chan error) {
	events := make(chan rpcclient.Event)
	errs := make(chan error)
	close(events)
	close(errs)
	return events, errs
}

func (r *syncFakeRPC) PollAdminEvents(context.Context) (rpcclient.AdminUpdate, error) {
	return rpcclient.AdminUpdate{}, nil
}

func (r *syncFakeRPC) SetUserStatus(context.Context, string, bool) error {
	return nil
}

func (r *syncFakeRPC) Broadcast(context.Context, string) error {
	return nil
}
