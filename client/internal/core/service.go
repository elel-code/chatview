package core

import (
	"context"
	"sync"
	"time"

	"chatview/client/internal/identity"
	"chatview/client/internal/storage"
)

type Service struct {
	identity *identity.Store
	rpc      rpcPort
	cache    *storage.Store

	mu                sync.Mutex
	remainingAttempts int
	lockedUntil       time.Time
	outboxCancel      context.CancelFunc
	publicKey         string
	role              int32
	offline           bool
}

func NewService(identityStore *identity.Store, rpc rpcPort, cache *storage.Store) *Service {
	return &Service{
		identity:          identityStore,
		rpc:               rpc,
		cache:             cache,
		remainingAttempts: 5,
	}
}
