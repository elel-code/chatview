package core

import (
	"context"
	"errors"
	"strings"

	"chatview/client/internal/domain"
)

func (s *Service) PollAdminEvents(ctx context.Context) (domain.AdminUpdate, error) {
	if s.IsOffline() {
		return domain.AdminUpdate{}, errors.New("offline")
	}
	update, err := s.rpc.PollAdminEvents(ctx)
	if err != nil {
		return domain.AdminUpdate{}, err
	}
	return update, nil
}

func (s *Service) SetUserStatus(ctx context.Context, publicKey string, banned bool) error {
	if s.IsOffline() {
		return errors.New("offline")
	}
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return errors.New("public key is required")
	}
	return s.rpc.SetUserStatus(ctx, publicKey, banned)
}

func (s *Service) Broadcast(ctx context.Context, text string) error {
	if s.IsOffline() {
		return errors.New("offline")
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("broadcast text is required")
	}
	return s.rpc.Broadcast(ctx, text)
}
