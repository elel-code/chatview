package rpcclient

import (
	"context"
	"errors"
	"strings"
	"sync"

	adminpb "chatview/api/gen/chatview/admin"
	authpb "chatview/api/gen/chatview/auth"
	chatpb "chatview/api/gen/chatview/chat"
	commonpb "chatview/api/gen/chatview/common"
	eventspb "chatview/api/gen/chatview/events"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

func (c *Client) ListFriends(ctx context.Context) ([]Friend, error) {
	ctx, cancel := withTimeout(c.authContext(ctx))
	defer cancel()

	resp, err := c.chat.ListFriends(ctx, &chatpb.ListFriendsReq{})
	if err != nil {
		return nil, rpcError(err)
	}
	return mapSlice(resp.Friends, friendFromProto), nil
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
	return HistoryPage{
		Messages:   mapSlice(resp.Page.Messages, messageFromProto),
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
	update.Users = mapSlice(resp.Update.Users, func(user *commonpb.UserInfo) UserInfo {
		return UserInfo{
			PublicKey: user.PubKey,
			Online:    user.IsOnline,
			Banned:    user.IsBanned,
		}
	})
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
