package rpcclient

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	adminpb "chatview/api/gen/chatview/admin"
	authpb "chatview/api/gen/chatview/auth"
	chatpb "chatview/api/gen/chatview/chat"
	commonpb "chatview/api/gen/chatview/common"
	eventspb "chatview/api/gen/chatview/events"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Options struct {
	Target                string
	UseTLS                bool
	CACertPath            string
	SSLTargetNameOverride string
}

type LoginResult struct {
	PublicKey string
	Role      int32
}

type Friend struct {
	PublicKey string
	Alias     string
	Online    bool
	Unread    int32
}

type Message struct {
	ID        string
	Sender    string
	Text      string
	Timestamp string
	Delivery  string
	Error     string
	ServerSeq int64
}

type HistoryPage struct {
	Messages   []Message
	NextCursor string
	HasMore    bool
}

type SendResult struct {
	ID        string
	Timestamp string
	ServerSeq int64
}

type Event struct {
	Kind      string
	PublicKey string
	Text      string
	Reason    string
	Count     int32
}

type AdminStats struct {
	OnlineUsers int32
	TotalUsers  int32
	BannedUsers int32
}

type UserInfo struct {
	PublicKey string
	Online    bool
	Banned    bool
}

type AdminUpdate struct {
	Users []UserInfo
	Stats AdminStats
}

type Client struct {
	conn   *grpc.ClientConn
	auth   authpb.AuthServiceClient
	chat   chatpb.ChatServiceClient
	events eventspb.EventServiceClient
	admin  adminpb.AdminServiceClient

	mu           sync.RWMutex
	sessionToken string
	publicKey    string
}

func New(options Options) (*Client, error) {
	if strings.TrimSpace(options.Target) == "" {
		return nil, errors.New("gRPC target is required")
	}

	transport, err := transportCredentials(options)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(options.Target, grpc.WithTransportCredentials(transport))
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:   conn,
		auth:   authpb.NewAuthServiceClient(conn),
		chat:   chatpb.NewChatServiceClient(conn),
		events: eventspb.NewEventServiceClient(conn),
		admin:  adminpb.NewAdminServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Login(ctx context.Context, publicKeyHex string, sign func([]byte) []byte) (LoginResult, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()

	challenge, err := c.auth.RequestChallenge(ctx, &authpb.RequestChallengeReq{PubKey: publicKeyHex})
	if err != nil {
		return LoginResult{}, rpcError(err)
	}
	signature := sign(challenge.Challenge)
	resp, err := c.auth.Login(ctx, &authpb.LoginReq{
		PubKey:             publicKeyHex,
		ChallengeSignature: signature,
	})
	if err != nil {
		if status.Code(err) == codes.PermissionDenied {
			return LoginResult{}, errors.New("account banned")
		}
		return LoginResult{}, rpcError(err)
	}

	c.mu.Lock()
	c.sessionToken = resp.SessionToken
	c.publicKey = resp.PubKey
	c.mu.Unlock()

	return LoginResult{PublicKey: resp.PubKey, Role: resp.Role}, nil
}

func (c *Client) ClearSession() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionToken = ""
	c.publicKey = ""
}

func (c *Client) PublicKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.publicKey
}

func (c *Client) ListFriends(ctx context.Context) ([]Friend, error) {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	resp, err := c.chat.ListFriends(ctx, &chatpb.ListFriendsReq{})
	if err != nil {
		return nil, rpcError(err)
	}
	friends := make([]Friend, 0, len(resp.Friends))
	for _, friend := range resp.Friends {
		friends = append(friends, Friend{
			PublicKey: friend.PubKey,
			Alias:     friend.Alias,
			Online:    friend.IsOnline,
			Unread:    friend.Unread,
		})
	}
	return friends, nil
}

func (c *Client) AddFriend(ctx context.Context, publicKey string) error {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	_, err := c.chat.AddFriend(ctx, &chatpb.AddFriendReq{TargetPubKey: publicKey})
	return rpcError(err)
}

func (c *Client) GetHistory(ctx context.Context, peerPublicKey string, cursor string, limit int32, direction string) (HistoryPage, error) {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	resp, err := c.chat.GetMessageHistory(ctx, &chatpb.GetMessageHistoryReq{
		PeerPubKey: peerPublicKey,
		Cursor:     cursor,
		Limit:      limit,
		Direction:  direction,
	})
	if err != nil {
		return HistoryPage{}, rpcError(err)
	}
	if resp.Page == nil {
		return HistoryPage{}, nil
	}
	messages := make([]Message, 0, len(resp.Page.Messages))
	for _, message := range resp.Page.Messages {
		messages = append(messages, messageFromProto(message))
	}
	return HistoryPage{
		Messages:   messages,
		NextCursor: resp.Page.NextCursor,
		HasMore:    resp.Page.HasMore,
	}, nil
}

func (c *Client) SendMessage(ctx context.Context, receiverPublicKey string, text string) (SendResult, error) {
	return c.SendMessageWithID(ctx, receiverPublicKey, text, randomMessageID())
}

func (c *Client) SendMessageWithID(ctx context.Context, receiverPublicKey string, text string, clientMessageID string) (SendResult, error) {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	resp, err := c.chat.SendMessage(ctx, &chatpb.SendMessageReq{
		ReceiverPubKey:  receiverPublicKey,
		Text:            text,
		ClientMessageId: clientMessageID,
	})
	if err != nil {
		return SendResult{}, rpcError(err)
	}
	return SendResult{
		ID:        resp.MessageId,
		Timestamp: resp.Timestamp,
		ServerSeq: resp.ServerSeq,
	}, nil
}

func (c *Client) MarkConversationRead(ctx context.Context, peerPublicKey string, seq int64) error {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	_, err := c.chat.MarkConversationRead(ctx, &chatpb.MarkConversationReadReq{
		PeerPubKey:        peerPublicKey,
		LastReadServerSeq: seq,
	})
	return rpcError(err)
}

func (c *Client) PollAdminEvents(ctx context.Context) (AdminUpdate, error) {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	resp, err := c.admin.PollAdminEvents(ctx, &adminpb.PollAdminEventsReq{})
	if err != nil {
		return AdminUpdate{}, rpcError(err)
	}
	if resp.Update == nil {
		return AdminUpdate{}, nil
	}
	update := AdminUpdate{}
	if resp.Update.Stats != nil {
		update.Stats = AdminStats{
			OnlineUsers: resp.Update.Stats.OnlineUsers,
			TotalUsers:  resp.Update.Stats.TotalUsers,
			BannedUsers: resp.Update.Stats.BannedUsers,
		}
	}
	update.Users = make([]UserInfo, 0, len(resp.Update.Users))
	for _, user := range resp.Update.Users {
		update.Users = append(update.Users, UserInfo{
			PublicKey: user.PubKey,
			Online:    user.IsOnline,
			Banned:    user.IsBanned,
		})
	}
	return update, nil
}

func (c *Client) SetUserStatus(ctx context.Context, publicKey string, banned bool) error {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	statusValue := commonpb.UserStatus_USER_STATUS_ACTIVE
	if banned {
		statusValue = commonpb.UserStatus_USER_STATUS_BANNED
	}
	_, err := c.admin.SetUserStatus(ctx, &adminpb.SetUserStatusReq{
		TargetPubKey: publicKey,
		Status:       statusValue,
	})
	return rpcError(err)
}

func (c *Client) Broadcast(ctx context.Context, text string) error {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	_, err := c.admin.Broadcast(ctx, &adminpb.BroadcastReq{Text: text})
	return rpcError(err)
}

func (c *Client) Subscribe(ctx context.Context) (<-chan Event, <-chan error) {
	events := make(chan Event, 16)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)

		stream, err := c.events.Subscribe(c.authContext(ctx), &eventspb.SubscribeReq{ClientId: randomMessageID()})
		if err != nil {
			errs <- rpcError(err)
			return
		}
		for {
			event, err := stream.Recv()
			if err != nil {
				errs <- rpcError(err)
				return
			}
			events <- eventFromProto(event)
		}
	}()
	return events, errs
}

func (c *Client) authContext(ctx context.Context) context.Context {
	c.mu.RLock()
	token := c.sessionToken
	c.mu.RUnlock()
	if token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

func transportCredentials(options Options) (credentials.TransportCredentials, error) {
	if !options.UseTLS {
		return insecure.NewCredentials(), nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if options.SSLTargetNameOverride != "" {
		tlsConfig.ServerName = options.SSLTargetNameOverride
	}
	if options.CACertPath != "" {
		pem, err := os.ReadFile(options.CACertPath)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = pool
	}
	return credentials.NewTLS(tlsConfig), nil
}

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 10*time.Second)
}

func messageFromProto(message *commonpb.ChatMessage) Message {
	if message == nil {
		return Message{}
	}
	return Message{
		ID:        message.Id,
		Sender:    message.SenderPubKey,
		Text:      message.Text,
		Timestamp: message.Timestamp,
		Delivery:  deliveryString(message.Delivery),
		Error:     message.Error,
		ServerSeq: message.ServerSeq,
	}
}

func deliveryString(delivery commonpb.MessageDelivery) string {
	switch delivery {
	case commonpb.MessageDelivery_MESSAGE_DELIVERY_INCOMING:
		return "incoming"
	case commonpb.MessageDelivery_MESSAGE_DELIVERY_SENT:
		return "sent"
	case commonpb.MessageDelivery_MESSAGE_DELIVERY_FAILED:
		return "failed"
	default:
		return "pending"
	}
}

func eventFromProto(event *eventspb.ServerEvent) Event {
	if event == nil {
		return Event{Kind: "unknown"}
	}
	switch typed := event.Event.(type) {
	case *eventspb.ServerEvent_NewMessage:
		return Event{Kind: "new_message", PublicKey: typed.NewMessage.FromPubKey, Count: typed.NewMessage.Count}
	case *eventspb.ServerEvent_FriendStatus:
		return Event{Kind: "friend_status", PublicKey: typed.FriendStatus.PubKey}
	case *eventspb.ServerEvent_SystemBroadcast:
		return Event{Kind: "system_broadcast", Text: typed.SystemBroadcast.Text}
	case *eventspb.ServerEvent_ForceOffline:
		return Event{Kind: "force_offline", Reason: typed.ForceOffline.Reason}
	case *eventspb.ServerEvent_AdminUpdate:
		return Event{Kind: "admin_update"}
	default:
		return Event{Kind: "unknown"}
	}
}

func randomMessageID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}

func rpcError(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	message := st.Message()
	switch st.Code() {
	case codes.PermissionDenied:
		if message == "" {
			return errors.New("permission denied")
		}
		return fmt.Errorf("permission denied: %s", message)
	case codes.Unauthenticated:
		if message == "" {
			return errors.New("unauthenticated")
		}
		return fmt.Errorf("unauthenticated: %s", message)
	case codes.Unavailable:
		if message == "" {
			return errors.New("service unavailable")
		}
		return fmt.Errorf("service unavailable: %s", message)
	case codes.DeadlineExceeded:
		if message == "" {
			return errors.New("request timed out")
		}
		return fmt.Errorf("request timed out: %s", message)
	case codes.InvalidArgument:
		if message == "" {
			return errors.New("invalid argument")
		}
		return fmt.Errorf("invalid argument: %s", message)
	case codes.NotFound:
		if message == "" {
			return errors.New("not found")
		}
		return fmt.Errorf("not found: %s", message)
	case codes.AlreadyExists:
		if message == "" {
			return errors.New("already exists")
		}
		return fmt.Errorf("already exists: %s", message)
	default:
		if message == "" {
			return fmt.Errorf("grpc error %d", st.Code())
		}
		return fmt.Errorf("grpc error %d: %s", st.Code(), message)
	}
}
