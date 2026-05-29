package ui

import (
	"slices"
	"testing"

	"chatview/client/internal/core"
)

func TestMessageDetailSkipsEmptyParts(t *testing.T) {
	message := core.Message{
		Timestamp: "2026-05-29T00:00:00Z",
		Delivery:  "sent",
		Error:     "retrying",
	}
	if got := messageDetail(message); got != "2026-05-29T00:00:00Z  sent  retrying" {
		t.Fatalf("messageDetail full = %q", got)
	}
	if got := messageDetail(core.Message{Delivery: "incoming"}); got != "incoming" {
		t.Fatalf("messageDetail partial = %q", got)
	}
}

func TestSortMessagesPrefersServerSeqThenTimestampThenID(t *testing.T) {
	messages := []core.Message{
		{ID: "z", Timestamp: "2026-05-29T00:00:03Z", ServerSeq: 0},
		{ID: "c", Timestamp: "2026-05-29T00:00:02Z", ServerSeq: 2},
		{ID: "b", Timestamp: "2026-05-29T00:00:01Z", ServerSeq: 1},
		{ID: "a", Timestamp: "", ServerSeq: 0},
	}
	sortMessages(messages)

	got := []string{messages[0].ID, messages[1].ID, messages[2].ID, messages[3].ID}
	if !slices.Equal(got, []string{"a", "b", "c", "z"}) {
		t.Fatalf("sorted IDs = %#v", got)
	}
}

func TestTruncateTextAndShortKey(t *testing.T) {
	if got := truncateText("  hello世界  ", 7); got != "hello世界" {
		t.Fatalf("truncateText no truncation = %q", got)
	}
	if got := truncateText("hello世界", 5); got != "hello..." {
		t.Fatalf("truncateText truncated = %q", got)
	}
	if got := shortKey("short"); got != "short" {
		t.Fatalf("shortKey short = %q", got)
	}
	if got := shortKey("1234567890abcdef"); got != "12345678...cdef" {
		t.Fatalf("shortKey long = %q", got)
	}
}
