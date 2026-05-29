package service

import (
	"slices"
	"strings"
	"testing"
)

func TestMessageHistoryLimitPreservesSmallPositiveValues(t *testing.T) {
	tests := map[int32]int32{
		-1:  30,
		0:   30,
		1:   1,
		29:  29,
		30:  30,
		99:  99,
		100: 100,
		101: 100,
	}
	for raw, want := range tests {
		if got := messageHistoryLimit(raw); got != want {
			t.Fatalf("messageHistoryLimit(%d) = %d, want %d", raw, got, want)
		}
	}
}

func TestParseCursor(t *testing.T) {
	seq, ok, err := parseCursor("42")
	if err != nil || !ok || seq != 42 {
		t.Fatalf("parseCursor valid = seq %d ok %v err %v", seq, ok, err)
	}
	seq, ok, err = parseCursor(" ")
	if err != nil || ok || seq != 0 {
		t.Fatalf("parseCursor empty = seq %d ok %v err %v", seq, ok, err)
	}
	if _, _, err := parseCursor("bad"); err == nil {
		t.Fatal("parseCursor invalid returned nil error")
	}
}

func TestNormalizeDirection(t *testing.T) {
	if got := normalizeDirection(" newer "); got != "newer" {
		t.Fatalf("normalizeDirection newer = %q", got)
	}
	if got := normalizeDirection("anything"); got != "older" {
		t.Fatalf("normalizeDirection fallback = %q", got)
	}
}

func TestHistoryQueryBuildsCursorPredicates(t *testing.T) {
	query, args := historyQuery("conv-1", 42, true, "newer", 11)
	if !strings.Contains(query, "server_seq > $2") || !strings.Contains(query, "ORDER BY server_seq ASC") {
		t.Fatalf("unexpected newer query:\n%s", query)
	}
	if !slices.Equal(args, []any{"conv-1", int64(42), int32(11)}) {
		t.Fatalf("newer args = %#v", args)
	}

	query, args = historyQuery("conv-1", 42, true, "older", 11)
	if !strings.Contains(query, "server_seq < $2") || !strings.Contains(query, "ORDER BY server_seq DESC") {
		t.Fatalf("unexpected older query:\n%s", query)
	}
	if !slices.Equal(args, []any{"conv-1", int64(42), int32(11)}) {
		t.Fatalf("older args = %#v", args)
	}

	query, args = historyQuery("conv-1", 0, false, "newer", 11)
	if strings.Contains(query, "server_seq >") || !slices.Equal(args, []any{"conv-1", int32(11)}) {
		t.Fatalf("unexpected newer no-cursor query/args:\n%s\n%#v", query, args)
	}
}
