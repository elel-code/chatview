package storage

import (
	"path/filepath"
	"testing"
)

func TestStoreCachesFriendsAndMessages(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SaveFriends([]Friend{{
		PublicKey: "alice",
		Alias:     "Alice",
		Online:    true,
		Unread:    2,
	}}); err != nil {
		t.Fatal(err)
	}

	friends, err := store.Friends()
	if err != nil {
		t.Fatal(err)
	}
	if len(friends) != 1 || friends[0].PublicKey != "alice" || !friends[0].Online || friends[0].Unread != 2 {
		t.Fatalf("unexpected friends: %#v", friends)
	}

	if err := store.SaveMessages("alice", []Message{
		{ID: "m2", Sender: "alice", Text: "second", Timestamp: "2026-01-01T00:00:02Z", Delivery: "incoming", ServerSeq: 2},
		{ID: "m1", Sender: "me", Text: "first", Timestamp: "2026-01-01T00:00:01Z", Delivery: "sent", ServerSeq: 1},
	}); err != nil {
		t.Fatal(err)
	}

	messages, err := store.Messages("alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != "m1" || messages[1].ID != "m2" {
		t.Fatalf("unexpected messages: %#v", messages)
	}

	if err := store.MarkConversationRead("alice", 2); err != nil {
		t.Fatal(err)
	}
	friends, err = store.Friends()
	if err != nil {
		t.Fatal(err)
	}
	if friends[0].Unread != 0 {
		t.Fatalf("expected unread reset, got %d", friends[0].Unread)
	}
}

func TestStoreHistoryPaginationAndGaps(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SaveMessages("alice", []Message{
		{ID: "m1", Sender: "alice", Text: "one", Timestamp: "2026-01-01T00:00:01Z", Delivery: "incoming", ServerSeq: 1},
		{ID: "m2", Sender: "alice", Text: "two", Timestamp: "2026-01-01T00:00:02Z", Delivery: "incoming", ServerSeq: 2},
		{ID: "m4", Sender: "alice", Text: "four", Timestamp: "2026-01-01T00:00:04Z", Delivery: "incoming", ServerSeq: 4},
		{ID: "m5", Sender: "alice", Text: "five", Timestamp: "2026-01-01T00:00:05Z", Delivery: "incoming", ServerSeq: 5},
	}); err != nil {
		t.Fatal(err)
	}

	messages, nextCursor, hasMore, err := store.History("alice", "", 2, "older")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ServerSeq != 4 || messages[1].ServerSeq != 5 || nextCursor != "4" || !hasMore {
		t.Fatalf("unexpected older page: messages=%#v next=%q hasMore=%v", messages, nextCursor, hasMore)
	}

	messages, nextCursor, hasMore, err = store.History("alice", "2", 2, "newer")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ServerSeq != 4 || messages[1].ServerSeq != 5 || nextCursor != "" || hasMore {
		t.Fatalf("unexpected newer page: messages=%#v next=%q hasMore=%v", messages, nextCursor, hasMore)
	}

	maxSeq, err := store.MaxServerSeq("alice")
	if err != nil {
		t.Fatal(err)
	}
	if maxSeq != 5 {
		t.Fatalf("unexpected max server seq: %d", maxSeq)
	}

	previous, missing, ok, err := store.FirstServerSeqGap("alice")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || previous != 2 || missing != 3 {
		t.Fatalf("unexpected gap: previous=%d missing=%d ok=%v", previous, missing, ok)
	}
}

func TestStoreOutboxLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.EnqueueOutbox(OutboxItem{
		ID:          "client-1",
		ReceiverKey: "alice",
		Text:        "queued",
		CreatedAt:   "2026-01-01T00:00:00Z",
	}, "me"); err != nil {
		t.Fatal(err)
	}
	pending, failed, err := store.OutboxStatus()
	if err != nil {
		t.Fatal(err)
	}
	if pending != 1 || failed != 0 {
		t.Fatalf("unexpected status: pending=%d failed=%d", pending, failed)
	}

	due, err := store.DueOutbox("2026-01-01T00:00:01Z", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != "client-1" {
		t.Fatalf("unexpected due items: %#v", due)
	}

	if err := store.MarkOutboxRetry("client-1", 1, "2026-01-01T00:00:10Z", "offline"); err != nil {
		t.Fatal(err)
	}
	due, err = store.DueOutbox("2026-01-01T00:00:02Z", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("expected retry delay, got %#v", due)
	}

	if err := store.MarkOutboxFailed("client-1", 5, "failed"); err != nil {
		t.Fatal(err)
	}
	pending, failed, err = store.OutboxStatus()
	if err != nil {
		t.Fatal(err)
	}
	if pending != 0 || failed != 1 {
		t.Fatalf("unexpected status after failure: pending=%d failed=%d", pending, failed)
	}

	if err := store.RetryFailedOutbox(); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkOutboxSent("client-1", "server-1", "2026-01-01T00:00:03Z", 3, "me"); err != nil {
		t.Fatal(err)
	}
	pending, failed, err = store.OutboxStatus()
	if err != nil {
		t.Fatal(err)
	}
	if pending != 0 || failed != 0 {
		t.Fatalf("unexpected status after sent: pending=%d failed=%d", pending, failed)
	}
	messages, err := store.Messages("alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].ID != "server-1" || messages[0].Delivery != "sent" {
		t.Fatalf("unexpected sent messages: %#v", messages)
	}

	message, err := store.MessageByClientID("client-1")
	if err != nil {
		t.Fatal(err)
	}
	if message.ID != "server-1" || message.ClientID != "client-1" {
		t.Fatalf("unexpected message by client id: %#v", message)
	}
}

func TestStoreOwnerIsolation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SetOwner("me-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFriends([]Friend{{PublicKey: "alice", Alias: "Alice", Unread: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessages("alice", []Message{
		{ID: "m1", Sender: "alice", Text: "owner one", Timestamp: "2026-01-01T00:00:01Z", Delivery: "incoming", ServerSeq: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.EnqueueOutbox(OutboxItem{
		ID:          "client-1",
		ReceiverKey: "alice",
		Text:        "queued",
		CreatedAt:   "2026-01-01T00:00:02Z",
	}, "me-1"); err != nil {
		t.Fatal(err)
	}

	if err := store.SetOwner("me-2"); err != nil {
		t.Fatal(err)
	}
	friends, err := store.Friends()
	if err != nil {
		t.Fatal(err)
	}
	if len(friends) != 0 {
		t.Fatalf("expected empty friends for second owner, got %#v", friends)
	}
	messages, err := store.Messages("alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected empty messages for second owner, got %#v", messages)
	}
	pending, failed, err := store.OutboxStatus()
	if err != nil {
		t.Fatal(err)
	}
	if pending != 0 || failed != 0 {
		t.Fatalf("expected empty outbox for second owner, got pending=%d failed=%d", pending, failed)
	}

	if err := store.SaveFriends([]Friend{{PublicKey: "bob", Alias: "Bob", Unread: 3}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessages("bob", []Message{
		{ID: "m2", Sender: "bob", Text: "owner two", Timestamp: "2026-01-01T00:00:03Z", Delivery: "incoming", ServerSeq: 1},
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.SetOwner("me-1"); err != nil {
		t.Fatal(err)
	}
	friends, err = store.Friends()
	if err != nil {
		t.Fatal(err)
	}
	if len(friends) != 1 || friends[0].PublicKey != "alice" || friends[0].Unread != 1 {
		t.Fatalf("unexpected first owner friends: %#v", friends)
	}
	messages, err = store.Messages("alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || !hasMessageID(messages, "m1") || !hasMessageID(messages, "client-1") {
		t.Fatalf("unexpected first owner messages: %#v", messages)
	}
}

func hasMessageID(messages []Message, id string) bool {
	for _, message := range messages {
		if message.ID == id {
			return true
		}
	}
	return false
}
