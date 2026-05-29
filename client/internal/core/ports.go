package core

import (
	"context"

	"chatview/client/internal/domain"
)

type rpcPort interface {
	authPort
	chatPort
	eventPort
	adminPort
}

type authPort interface {
	Login(ctx context.Context, publicKeyHex string, sign func([]byte) []byte) (domain.LoginResult, error)
	ClearSession()
}

type chatPort interface {
	ListFriends(ctx context.Context) ([]domain.Friend, error)
	AddFriend(ctx context.Context, publicKey string) error
	GetHistory(ctx context.Context, peerPublicKey string, cursor string, limit int32, direction string) (domain.HistoryPage, error)
	SendMessageWithID(ctx context.Context, receiverPublicKey string, text string, clientMessageID string) (domain.SendResult, error)
	MarkConversationRead(ctx context.Context, peerPublicKey string, seq int64) error
}

type eventPort interface {
	Subscribe(ctx context.Context) (<-chan domain.Event, <-chan error)
}

type adminPort interface {
	PollAdminEvents(ctx context.Context) (domain.AdminUpdate, error)
	SetUserStatus(ctx context.Context, publicKey string, banned bool) error
	Broadcast(ctx context.Context, text string) error
}
