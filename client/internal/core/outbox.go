package core

import (
	"context"
	"time"

	"chatview/client/internal/storage"
)

func (s *Service) OutboxStatus() OutboxStatus {
	if s.cache == nil {
		return OutboxStatus{}
	}
	pending, failed, err := s.cache.OutboxStatus()
	if err != nil {
		return OutboxStatus{}
	}
	return OutboxStatus{Pending: pending, Failed: failed}
}

func (s *Service) RetryFailedOutbox() error {
	if s.cache == nil {
		return nil
	}
	return s.cache.RetryFailedOutbox()
}

func (s *Service) ClearFailedOutbox() error {
	if s.cache == nil {
		return nil
	}
	return s.cache.ClearFailedOutbox()
}

func (s *Service) StartOutboxWorker(ctx context.Context) {
	s.startOutboxWorker(ctx)
}

func (s *Service) startOutboxWorker(parent context.Context) {
	if s.cache == nil || s.IsOffline() {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	s.stopOutboxWorker()
	_ = s.cache.RecoverOutbox()
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.outboxCancel = cancel
	s.mu.Unlock()
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			s.processOutbox(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) stopOutboxWorker() {
	s.mu.Lock()
	cancel := s.outboxCancel
	s.outboxCancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Service) processOutbox(ctx context.Context) {
	if s.cache == nil {
		return
	}
	items, err := s.cache.DueOutbox(nowRFC3339(), 20)
	if err != nil {
		return
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		_ = s.sendOutboxItem(ctx, item)
	}
}

func (s *Service) sendOutboxItem(ctx context.Context, item storage.OutboxItem) error {
	if s.cache == nil {
		return nil
	}
	attempts := item.Attempts + 1
	_ = s.cache.MarkOutboxSending(item.ID)
	result, err := s.rpc.SendMessageWithID(ctx, item.ReceiverKey, item.Text, item.ID)
	if err == nil {
		return s.cache.MarkOutboxSent(item.ID, result.ID, result.Timestamp, result.ServerSeq, s.PublicKey())
	}
	if attempts >= 5 {
		_ = s.cache.MarkOutboxFailed(item.ID, attempts, err.Error())
		return err
	}
	_ = s.cache.MarkOutboxRetry(item.ID, attempts, nextRetryTime(attempts), err.Error())
	return err
}
