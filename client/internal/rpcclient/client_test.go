package rpcclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"net"
	"sync"
	"testing"
	"time"

	adminpb "chatview/api/gen/chatview/admin"
	authpb "chatview/api/gen/chatview/auth"
	chatpb "chatview/api/gen/chatview/chat"
	commonpb "chatview/api/gen/chatview/common"
	eventspb "chatview/api/gen/chatview/events"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestClientAgainstFakeGRPCServer(t *testing.T) {
	server := &fakeChatViewServer{
		challenge: []byte("challenge-bytes"),
		token:     "session-token",
	}
	target, stop := startFakeGRPCServer(t, server)
	defer stop()

	client, err := New(Options{Target: target, UseTLS: false})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyHex := hex.EncodeToString(publicKey)

	login, err := client.Login(context.Background(), publicKeyHex, func(challenge []byte) []byte {
		if string(challenge) != "challenge-bytes" {
			t.Fatalf("unexpected challenge: %q", string(challenge))
		}
		return ed25519.Sign(privateKey, challenge)
	})
	if err != nil {
		t.Fatal(err)
	}
	if login.PublicKey != publicKeyHex || login.Role != 1 {
		t.Fatalf("unexpected login result: %#v", login)
	}

	friends, err := client.ListFriends(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(friends) != 1 || friends[0].PublicKey != "peer-a" || !friends[0].Online || friends[0].Unread != 3 {
		t.Fatalf("unexpected friends: %#v", friends)
	}

	sent, err := client.SendMessageWithID(context.Background(), "peer-a", "hello", "client-1")
	if err != nil {
		t.Fatal(err)
	}
	if sent.ID != "server-1" || sent.Timestamp != "2026-05-15T00:00:00Z" || sent.ServerSeq != 11 {
		t.Fatalf("unexpected send result: %#v", sent)
	}

	history, err := client.GetHistory(context.Background(), "peer-a", "", 30, "older")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Messages) != 1 || history.Messages[0].Delivery != "incoming" || history.NextCursor != "10" || !history.HasMore {
		t.Fatalf("unexpected history: %#v", history)
	}

	if err := client.MarkConversationRead(context.Background(), "peer-a", 10); err != nil {
		t.Fatal(err)
	}
	if err := client.SetUserStatus(context.Background(), "peer-a", true); err != nil {
		t.Fatal(err)
	}
	if err := client.Broadcast(context.Background(), "maintenance"); err != nil {
		t.Fatal(err)
	}
	admin, err := client.PollAdminEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if admin.Stats.OnlineUsers != 1 || admin.Stats.TotalUsers != 2 || admin.Stats.BannedUsers != 1 {
		t.Fatalf("unexpected admin stats: %#v", admin.Stats)
	}
	if len(admin.Users) != 1 || admin.Users[0].PublicKey != "peer-a" || !admin.Users[0].Banned {
		t.Fatalf("unexpected admin users: %#v", admin.Users)
	}

	ctx, cancel := context.WithCancel(context.Background())
	events, errs := client.Subscribe(ctx)
	select {
	case event := <-events:
		if event.Kind != "new_message" || event.PublicKey != "peer-a" {
			t.Fatalf("unexpected event: %#v", event)
		}
	case err := <-errs:
		t.Fatalf("unexpected subscribe error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
	cancel()

	server.mu.Lock()
	defer server.mu.Unlock()
	if server.requestChallengePublicKey != publicKeyHex {
		t.Fatalf("challenge requested with %q", server.requestChallengePublicKey)
	}
	if server.loginPublicKey != publicKeyHex || !server.loginSignatureOK {
		t.Fatalf("login signature was not verified: pub=%q ok=%v", server.loginPublicKey, server.loginSignatureOK)
	}
	if server.clientMessageID != "client-1" || server.markReadSeq != 10 || server.bannedStatus != commonpb.UserStatus_USER_STATUS_BANNED || server.broadcastText != "maintenance" {
		t.Fatalf("server did not receive expected RPC payloads: %#v", server)
	}
	if server.authenticatedCalls < 8 {
		t.Fatalf("expected authenticated RPCs, got %d", server.authenticatedCalls)
	}
}

type fakeChatViewServer struct {
	authpb.UnimplementedAuthServiceServer
	chatpb.UnimplementedChatServiceServer
	adminpb.UnimplementedAdminServiceServer
	eventspb.UnimplementedEventServiceServer

	mu sync.Mutex

	challenge []byte
	token     string

	requestChallengePublicKey string
	loginPublicKey            string
	loginSignatureOK          bool
	clientMessageID           string
	markReadSeq               int64
	bannedStatus              commonpb.UserStatus
	broadcastText             string
	authenticatedCalls        int
}

func startFakeGRPCServer(t *testing.T, fake *fakeChatViewServer) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	authpb.RegisterAuthServiceServer(server, fake)
	chatpb.RegisterChatServiceServer(server, fake)
	adminpb.RegisterAdminServiceServer(server, fake)
	eventspb.RegisterEventServiceServer(server, fake)

	go func() {
		if err := server.Serve(listener); err != nil && err != grpc.ErrServerStopped {
			t.Errorf("fake gRPC server failed: %v", err)
		}
	}()
	return listener.Addr().String(), func() {
		server.Stop()
		_ = listener.Close()
	}
}

func (s *fakeChatViewServer) RequestChallenge(_ context.Context, req *authpb.RequestChallengeReq) (*authpb.RequestChallengeResp, error) {
	s.mu.Lock()
	s.requestChallengePublicKey = req.PubKey
	s.mu.Unlock()
	return &authpb.RequestChallengeResp{Challenge: s.challenge}, nil
}

func (s *fakeChatViewServer) Login(_ context.Context, req *authpb.LoginReq) (*authpb.LoginResp, error) {
	publicKey, err := hex.DecodeString(req.PubKey)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid public key")
	}
	signatureOK := ed25519.Verify(ed25519.PublicKey(publicKey), s.challenge, req.ChallengeSignature)
	s.mu.Lock()
	s.loginPublicKey = req.PubKey
	s.loginSignatureOK = signatureOK
	s.mu.Unlock()
	if !signatureOK {
		return nil, status.Error(codes.PermissionDenied, "bad signature")
	}
	return &authpb.LoginResp{SessionToken: s.token, Role: 1, PubKey: req.PubKey}, nil
}

func (s *fakeChatViewServer) ListFriends(ctx context.Context, _ *chatpb.ListFriendsReq) (*chatpb.ListFriendsResp, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	return &chatpb.ListFriendsResp{Friends: []*commonpb.FriendInfo{{
		PubKey:   "peer-a",
		Alias:    "Peer A",
		IsOnline: true,
		Unread:   3,
	}}}, nil
}

func (s *fakeChatViewServer) AddFriend(ctx context.Context, _ *chatpb.AddFriendReq) (*chatpb.AddFriendResp, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	return &chatpb.AddFriendResp{}, nil
}

func (s *fakeChatViewServer) SendMessage(ctx context.Context, req *chatpb.SendMessageReq) (*chatpb.SendMessageResp, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.clientMessageID = req.ClientMessageId
	s.mu.Unlock()
	return &chatpb.SendMessageResp{
		MessageId: "server-1",
		Timestamp: "2026-05-15T00:00:00Z",
		ServerSeq: 11,
	}, nil
}

func (s *fakeChatViewServer) GetMessageHistory(ctx context.Context, _ *chatpb.GetMessageHistoryReq) (*chatpb.GetMessageHistoryResp, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	return &chatpb.GetMessageHistoryResp{Page: &commonpb.MessageHistoryPage{
		Messages: []*commonpb.ChatMessage{{
			Id:           "server-0",
			SenderPubKey: "peer-a",
			Text:         "hi",
			Timestamp:    "2026-05-15T00:00:00Z",
			Delivery:     commonpb.MessageDelivery_MESSAGE_DELIVERY_INCOMING,
			ServerSeq:    10,
		}},
		NextCursor: "10",
		HasMore:    true,
	}}, nil
}

func (s *fakeChatViewServer) MarkConversationRead(ctx context.Context, req *chatpb.MarkConversationReadReq) (*chatpb.MarkConversationReadResp, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.markReadSeq = req.LastReadServerSeq
	s.mu.Unlock()
	return &chatpb.MarkConversationReadResp{}, nil
}

func (s *fakeChatViewServer) SetUserStatus(ctx context.Context, req *adminpb.SetUserStatusReq) (*adminpb.SetUserStatusResp, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.bannedStatus = req.Status
	s.mu.Unlock()
	return &adminpb.SetUserStatusResp{}, nil
}

func (s *fakeChatViewServer) Broadcast(ctx context.Context, req *adminpb.BroadcastReq) (*adminpb.BroadcastResp, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.broadcastText = req.Text
	s.mu.Unlock()
	return &adminpb.BroadcastResp{}, nil
}

func (s *fakeChatViewServer) PollAdminEvents(ctx context.Context, _ *adminpb.PollAdminEventsReq) (*adminpb.PollAdminEventsResp, error) {
	if err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	return &adminpb.PollAdminEventsResp{Update: &commonpb.AdminUpdate{
		Users: []*commonpb.UserInfo{{
			PubKey:   "peer-a",
			IsOnline: false,
			IsBanned: true,
		}},
		Stats: &commonpb.AdminStats{
			OnlineUsers: 1,
			TotalUsers:  2,
			BannedUsers: 1,
		},
	}}, nil
}

func (s *fakeChatViewServer) Subscribe(req *eventspb.SubscribeReq, stream grpc.ServerStreamingServer[eventspb.ServerEvent]) error {
	if req.ClientId == "" {
		return status.Error(codes.InvalidArgument, "missing client id")
	}
	if err := s.requireAuth(stream.Context()); err != nil {
		return err
	}
	if err := stream.Send(&eventspb.ServerEvent{Event: &eventspb.ServerEvent_NewMessage{
		NewMessage: &eventspb.NewMessageEvent{FromPubKey: "peer-a", Count: 1},
	}}); err != nil {
		return err
	}
	<-stream.Context().Done()
	if stream.Context().Err() == context.Canceled {
		return nil
	}
	return stream.Context().Err()
}

func (s *fakeChatViewServer) requireAuth(ctx context.Context) error {
	values := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(values) != 1 || values[0] != "Bearer "+s.token {
		return status.Error(codes.Unauthenticated, "missing bearer token")
	}
	s.mu.Lock()
	s.authenticatedCalls++
	s.mu.Unlock()
	return nil
}
