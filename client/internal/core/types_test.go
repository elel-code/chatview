package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDTOJSONCompatibility(t *testing.T) {
	message := Message{
		ID:        "m1",
		Sender:    "alice",
		Text:      "quote \" and newline\n",
		Timestamp: "2026-05-15T00:00:00Z",
		Delivery:  "failed",
		Error:     "network",
		ServerSeq: 42,
	}

	payload, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(payload)
	for _, want := range []string{
		`"id":"m1"`,
		`"sender":"alice"`,
		`"delivery":"failed"`,
		`"error":"network"`,
		`"serverSeq":42`,
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("missing %s in %s", want, jsonText)
		}
	}

	var parsed Message
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed != message {
		t.Fatalf("round trip changed message: %#v != %#v", parsed, message)
	}

	outbox := OutboxStatus{Pending: 2, Failed: 1}
	payload, err = json.Marshal(outbox)
	if err != nil {
		t.Fatal(err)
	}
	jsonText = string(payload)
	for _, want := range []string{`"pending":2`, `"failed":1`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("missing %s in %s", want, jsonText)
		}
	}
}
