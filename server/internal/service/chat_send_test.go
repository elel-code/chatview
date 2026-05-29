package service

import (
	"context"
	"testing"

	chatpb "chatview/api/gen/chatview/chat"
	"chatview/server/internal/contextx"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewSendMessageInputTrimsAndValidates(t *testing.T) {
	ctx := contextx.WithPrincipal(context.Background(), contextx.Principal{PubKey: "sender"})
	input, err := newSendMessageInput(ctx, &chatpb.SendMessageReq{
		ReceiverPubKey:  " receiver ",
		Text:            " hello ",
		ClientMessageId: " client-1 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.sender != "sender" || input.receiver != "receiver" || input.text != "hello" || input.clientMessageID != "client-1" {
		t.Fatalf("unexpected input: %#v", input)
	}
}

func TestNewSendMessageInputRejectsInvalidRequests(t *testing.T) {
	ctx := contextx.WithPrincipal(context.Background(), contextx.Principal{PubKey: "sender"})
	tests := map[string]*chatpb.SendMessageReq{
		"missing receiver": {},
		"self receiver":    {ReceiverPubKey: "sender", Text: "hello"},
		"empty text":       {ReceiverPubKey: "receiver", Text: " "},
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newSendMessageInput(ctx, req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code = %s, want %s; err=%v", status.Code(err), codes.InvalidArgument, err)
			}
		})
	}
}
