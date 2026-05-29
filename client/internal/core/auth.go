package core

import (
	"context"
	"crypto/ed25519"
	"errors"
	"slices"
	"strings"
	"time"

	"chatview/client/internal/domain"
	"chatview/client/internal/identity"
)

var offlineLoginErrorMarkers = []string{
	"service unavailable",
	"request timed out",
	"connection refused",
	"deadline exceeded",
	"unavailable",
}

func (s *Service) HasLocalIdentity() bool {
	return s.identity.Exists()
}

func (s *Service) CreateIdentity(pin string) (identity.Identity, error) {
	return s.identity.Create(pin)
}

func (s *Service) ImportIdentity(privateKeyHex, pin string) error {
	return s.identity.Import(privateKeyHex, pin)
}

func (s *Service) ExportPrivateKey(pin string) (string, error) {
	return s.identity.ExportPrivateKey(pin)
}

func (s *Service) Login(ctx context.Context, pin string) (domain.LoginResult, error) {
	if s.isLocked() {
		return domain.LoginResult{}, errors.New("too many attempts")
	}
	keypair, err := s.identity.Load(pin)
	if err != nil {
		s.recordBadPIN()
		return domain.LoginResult{}, err
	}
	result, err := s.rpc.Login(ctx, keypair.PublicHex, func(challenge []byte) []byte {
		return ed25519.Sign(keypair.Private, challenge)
	})
	if err != nil {
		if !isOfflineLoginError(err) {
			return domain.LoginResult{}, err
		}
		if err := s.setCacheOwner(keypair.PublicHex); err != nil {
			return domain.LoginResult{}, err
		}
		s.resetLockout()
		s.setSession(keypair.PublicHex, 0, true)
		return domain.LoginResult{PublicKey: keypair.PublicHex, Role: 0, Offline: true}, nil
	}
	if err := s.setCacheOwner(result.PublicKey); err != nil {
		return domain.LoginResult{}, err
	}
	s.resetLockout()
	s.setSession(result.PublicKey, result.Role, false)
	return domain.LoginResult{PublicKey: result.PublicKey, Role: result.Role}, nil
}

func (s *Service) Logout() {
	s.stopOutboxWorker()
	s.rpc.ClearSession()
	if s.cache != nil {
		_ = s.cache.SetOwner("")
	}
	s.setSession("", 0, false)
}

func (s *Service) AuthLockState() domain.AuthLockState {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if !s.lockedUntil.IsZero() && now.After(s.lockedUntil) {
		s.lockedUntil = time.Time{}
		s.remainingAttempts = 5
	}
	state := domain.AuthLockState{RemainingAttempts: s.remainingAttempts}
	if !s.lockedUntil.IsZero() {
		state.LockedUntil = s.lockedUntil.UTC().Format(time.RFC3339)
	}
	return state
}

func (s *Service) isLocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if !s.lockedUntil.IsZero() && now.Before(s.lockedUntil) {
		return true
	}
	if !s.lockedUntil.IsZero() {
		s.lockedUntil = time.Time{}
		s.remainingAttempts = 5
	}
	return false
}

func (s *Service) recordBadPIN() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remainingAttempts--
	if s.remainingAttempts <= 0 {
		s.lockedUntil = time.Now().Add(30 * time.Second)
		s.remainingAttempts = 0
	}
}

func (s *Service) resetLockout() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remainingAttempts = 5
	s.lockedUntil = time.Time{}
}

func isOfflineLoginError(err error) bool {
	text := strings.ToLower(err.Error())
	return slices.ContainsFunc(offlineLoginErrorMarkers, func(marker string) bool {
		return strings.Contains(text, marker)
	})
}
