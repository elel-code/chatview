package core

import (
	"context"
	"sync"
	"time"

	"chatview/client/internal/identity"
	"chatview/client/internal/rpcclient"
	"chatview/client/internal/storage"
)

type Service struct {
	identity *identity.Store
	rpc      chatRPC
	cache    *storage.Store

	mu                sync.Mutex
	remainingAttempts int
	lockedUntil       time.Time
	outboxCancel      context.CancelFunc
	publicKey         string
	role              int32
	offline           bool
}

type chatRPC interface {
	Login(ctx context.Context, publicKeyHex string, sign func([]byte) []byte) (rpcclient.LoginResult, error)
	ClearSession()
	ListFriends(ctx context.Context) ([]rpcclient.Friend, error)
	AddFriend(ctx context.Context, publicKey string) error
	GetHistory(ctx context.Context, peerPublicKey string, cursor string, limit int32, direction string) (rpcclient.HistoryPage, error)
	SendMessageWithID(ctx context.Context, receiverPublicKey string, text string, clientMessageID string) (rpcclient.SendResult, error)
	MarkConversationRead(ctx context.Context, peerPublicKey string, seq int64) error
	Subscribe(ctx context.Context) (<-chan rpcclient.Event, <-chan error)
	PollAdminEvents(ctx context.Context) (rpcclient.AdminUpdate, error)
	SetUserStatus(ctx context.Context, publicKey string, banned bool) error
	Broadcast(ctx context.Context, text string) error
}

func NewService(identityStore *identity.Store, rpc chatRPC, cache *storage.Store) *Service {
	return &Service{
		identity:          identityStore,
		rpc:               rpc,
		cache:             cache,
		remainingAttempts: 5,
	}
}
