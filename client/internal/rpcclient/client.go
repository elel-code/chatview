package rpcclient

import (
	"errors"
	"strings"
	"sync"

	adminpb "chatview/api/gen/chatview/admin"
	authpb "chatview/api/gen/chatview/auth"
	chatpb "chatview/api/gen/chatview/chat"
	eventspb "chatview/api/gen/chatview/events"

	"google.golang.org/grpc"
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
