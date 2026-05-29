package core

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"chatview/client/internal/domain"
)

func (s *Service) ListFriends(ctx context.Context) ([]domain.Friend, error) {
	if s.IsOffline() {
		if s.cache == nil {
			return nil, errors.New("offline")
		}
		cached, err := s.cache.Friends()
		if err != nil {
			return nil, err
		}
		return friendsFromCache(cached), nil
	}
	friends, err := s.rpc.ListFriends(ctx)
	if err != nil {
		if s.cache == nil {
			return nil, err
		}
		cached, cacheErr := s.cache.Friends()
		if cacheErr != nil || len(cached) == 0 {
			return nil, err
		}
		return friendsFromCache(cached), nil
	}
	result := friends
	if s.cache != nil {
		_ = s.cache.SaveFriends(friendsToCache(result))
	}
	return result, nil
}

func (s *Service) AddFriend(ctx context.Context, publicKey string) error {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" {
		return errors.New("public key is required")
	}
	if s.IsOffline() {
		return errors.New("offline")
	}
	return s.rpc.AddFriend(ctx, publicKey)
}

func (s *Service) GetHistory(ctx context.Context, peerPublicKey string, cursor string, limit int32, direction string) (domain.HistoryPage, error) {
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if peerPublicKey == "" {
		return domain.HistoryPage{}, errors.New("peer public key is required")
	}
	if s.IsOffline() {
		return s.cachedHistory(peerPublicKey, cursor, limit, direction)
	}
	page, err := s.rpc.GetHistory(ctx, peerPublicKey, cursor, limit, direction)
	if err != nil {
		if s.cache == nil {
			return domain.HistoryPage{}, err
		}
		cached, cacheErr := s.cachedHistory(peerPublicKey, cursor, limit, direction)
		if cacheErr != nil || len(cached.Messages) == 0 {
			return domain.HistoryPage{}, err
		}
		return cached, nil
	}
	return s.cacheHistoryPage(peerPublicKey, page), nil
}

func (s *Service) SyncConversation(ctx context.Context, peerPublicKey string, expectedCount int32) (domain.HistoryPage, error) {
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if peerPublicKey == "" {
		return domain.HistoryPage{}, errors.New("peer public key is required")
	}
	if s.IsOffline() {
		return domain.HistoryPage{}, errors.New("offline")
	}
	if s.cache == nil {
		return s.GetHistory(ctx, peerPublicKey, "", syncLimit(expectedCount), "older")
	}

	limit := syncLimit(expectedCount)
	synced, err := s.fillHistoryGaps(ctx, peerPublicKey, limit)
	if err != nil {
		return domain.HistoryPage{}, err
	}
	latest, err := s.syncNewerHistory(ctx, peerPublicKey, limit, expectedCount)
	if err != nil {
		return domain.HistoryPage{}, err
	}
	synced = append(synced, latest...)
	return domain.HistoryPage{Messages: synced}, nil
}

func (s *Service) fillHistoryGaps(ctx context.Context, peerPublicKey string, limit int32) ([]domain.Message, error) {
	var synced []domain.Message
	attemptedGaps := make(map[int64]bool)
	for attempts := 0; attempts < 100; attempts++ {
		previousSeq, missingSeq, ok, err := s.cache.FirstServerSeqGap(peerPublicKey)
		if err != nil {
			return nil, err
		}
		if !ok || attemptedGaps[previousSeq] {
			break
		}
		attemptedGaps[previousSeq] = true
		page, err := s.GetHistory(ctx, peerPublicKey, strconv.FormatInt(previousSeq, 10), limit, "newer")
		if err != nil {
			return nil, err
		}
		synced = append(synced, page.Messages...)
		if len(page.Messages) == 0 || !containsServerSeq(page.Messages, missingSeq) {
			break
		}
	}
	return synced, nil
}

func (s *Service) syncNewerHistory(ctx context.Context, peerPublicKey string, limit int32, expectedCount int32) ([]domain.Message, error) {
	cursor, err := s.cache.MaxServerSeq(peerPublicKey)
	if err != nil {
		return nil, err
	}
	if cursor == 0 {
		page, err := s.GetHistory(ctx, peerPublicKey, "", syncLimit(expectedCount), "older")
		return page.Messages, err
	}

	var synced []domain.Message
	for attempts := 0; attempts < 100; attempts++ {
		page, err := s.GetHistory(ctx, peerPublicKey, strconv.FormatInt(cursor, 10), limit, "newer")
		if err != nil {
			return nil, err
		}
		if len(page.Messages) == 0 {
			break
		}
		synced = append(synced, page.Messages...)
		nextCursor := maxServerSeq(page.Messages)
		if nextCursor <= cursor {
			break
		}
		cursor = nextCursor
		if !page.HasMore {
			break
		}
	}
	return synced, nil
}

func (s *Service) cachedHistory(peerPublicKey string, cursor string, limit int32, direction string) (domain.HistoryPage, error) {
	if s.cache == nil {
		return domain.HistoryPage{}, errors.New("offline")
	}
	messages, nextCursor, hasMore, err := s.cache.History(peerPublicKey, cursor, int(limit), direction)
	if err != nil {
		return domain.HistoryPage{}, err
	}
	return domain.HistoryPage{
		Messages:   messagesFromCache(messages),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *Service) cacheHistoryPage(peerPublicKey string, page domain.HistoryPage) domain.HistoryPage {
	messages := page.Messages
	if s.cache != nil {
		_ = s.cache.SaveMessages(peerPublicKey, messagesToCache(peerPublicKey, messages))
	}
	return domain.HistoryPage{
		Messages:   messages,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
}
