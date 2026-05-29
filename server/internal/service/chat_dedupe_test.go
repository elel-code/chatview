package service

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExistingMessageRowToSendMessageResp(t *testing.T) {
	ts := time.Date(2026, 5, 29, 1, 2, 3, 4, time.FixedZone("test", 8*60*60))
	row := existingMessageRow{
		ID:           "message-1",
		Text:         "hello",
		Timestamp:    ts,
		ServerSeq:    42,
		ParticipantA: "sender",
		ParticipantB: "receiver",
	}

	resp, err := row.toSendMessageResp("sender", "receiver", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Deduplicated || resp.MessageId != "message-1" || resp.ServerSeq != 42 {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if resp.Timestamp != ts.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("timestamp = %q", resp.Timestamp)
	}
}

func TestExistingMessageRowToSendMessageRespFindsReceiverFromOtherParticipant(t *testing.T) {
	row := existingMessageRow{
		Text:         "hello",
		ParticipantA: "receiver",
		ParticipantB: "sender",
	}
	if _, err := row.toSendMessageResp("sender", "receiver", "hello"); err != nil {
		t.Fatal(err)
	}
}

func TestExistingMessageRowToSendMessageRespRejectsConflicts(t *testing.T) {
	row := existingMessageRow{
		Text:         "hello",
		ParticipantA: "sender",
		ParticipantB: "receiver",
	}
	tests := map[string]struct {
		receiver string
		text     string
	}{
		"different receiver": {receiver: "other", text: "hello"},
		"different text":     {receiver: "receiver", text: "changed"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := row.toSendMessageResp("sender", test.receiver, test.text)
			if status.Code(err) != codes.AlreadyExists {
				t.Fatalf("code = %s, want %s; err=%v", status.Code(err), codes.AlreadyExists, err)
			}
		})
	}
}
