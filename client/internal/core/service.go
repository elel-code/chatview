package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

func (s *Service) Login(ctx context.Context, pin string) (LoginResult, error) {
	if s.isLocked() {
		return LoginResult{}, errors.New("too many attempts")
	}
	keypair, err := s.identity.Load(pin)
	if err != nil {
		s.recordBadPIN()
		return LoginResult{}, err
	}
	result, err := s.rpc.Login(ctx, keypair.PublicHex, func(challenge []byte) []byte {
		return ed25519.Sign(keypair.Private, challenge)
	})
	if err != nil {
		if !isOfflineLoginError(err) {
			return LoginResult{}, err
		}
		if err := s.setCacheOwner(keypair.PublicHex); err != nil {
			return LoginResult{}, err
		}
		s.resetLockout()
		s.setSession(keypair.PublicHex, 0, true)
		return LoginResult{PublicKey: keypair.PublicHex, Role: 0, Offline: true}, nil
	}
	if err := s.setCacheOwner(result.PublicKey); err != nil {
		return LoginResult{}, err
	}
	s.resetLockout()
	s.setSession(result.PublicKey, result.Role, false)
	return LoginResult{PublicKey: result.PublicKey, Role: result.Role}, nil
}

func (s *Service) Logout() {
	s.stopOutboxWorker()
	s.rpc.ClearSession()
	if s.cache != nil {
		_ = s.cache.SetOwner("")
	}
	s.setSession("", 0, false)
}

func (s *Service) AuthLockState() AuthLockState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lockedUntil.IsZero() && time.Now().After(s.lockedUntil) {
		s.lockedUntil = time.Time{}
		s.remainingAttempts = 5
	}
	state := AuthLockState{RemainingAttempts: s.remainingAttempts}
	if !s.lockedUntil.IsZero() {
		state.LockedUntil = s.lockedUntil.UTC().Format(time.RFC3339)
	}
	return state
}

func (s *Service) ListFriends(ctx context.Context) ([]Friend, error) {
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
	result := make([]Friend, 0, len(friends))
	for _, friend := range friends {
		result = append(result, Friend{
			PublicKey: friend.PublicKey,
			Alias:     friend.Alias,
			Online:    friend.Online,
			Unread:    friend.Unread,
		})
	}
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

func (s *Service) GetHistory(ctx context.Context, peerPublicKey string, cursor string, limit int32, direction string) (HistoryPage, error) {
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if peerPublicKey == "" {
		return HistoryPage{}, errors.New("peer public key is required")
	}
	if s.IsOffline() {
		return s.cachedHistory(peerPublicKey, cursor, limit, direction)
	}
	page, err := s.rpc.GetHistory(ctx, peerPublicKey, cursor, limit, direction)
	if err != nil {
		if s.cache == nil {
			return HistoryPage{}, err
		}
		cached, cacheErr := s.cachedHistory(peerPublicKey, cursor, limit, direction)
		if cacheErr != nil || len(cached.Messages) == 0 {
			return HistoryPage{}, err
		}
		return cached, nil
	}
	return s.cacheHistoryPage(peerPublicKey, page), nil
}

func (s *Service) SyncConversation(ctx context.Context, peerPublicKey string, expectedCount int32) (HistoryPage, error) {
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if peerPublicKey == "" {
		return HistoryPage{}, errors.New("peer public key is required")
	}
	if s.IsOffline() {
		return HistoryPage{}, errors.New("offline")
	}
	if s.cache == nil {
		return s.GetHistory(ctx, peerPublicKey, "", syncLimit(expectedCount), "older")
	}

	var synced []Message
	limit := syncLimit(expectedCount)
	attemptedGaps := make(map[int64]bool)
	for attempts := 0; attempts < 100; attempts++ {
		previousSeq, missingSeq, ok, err := s.cache.FirstServerSeqGap(peerPublicKey)
		if err != nil {
			return HistoryPage{}, err
		}
		if !ok || attemptedGaps[previousSeq] {
			break
		}
		attemptedGaps[previousSeq] = true
		page, err := s.GetHistory(ctx, peerPublicKey, strconv.FormatInt(previousSeq, 10), limit, "newer")
		if err != nil {
			return HistoryPage{}, err
		}
		synced = append(synced, page.Messages...)
		if len(page.Messages) == 0 || !containsServerSeq(page.Messages, missingSeq) {
			break
		}
	}

	cursor, err := s.cache.MaxServerSeq(peerPublicKey)
	if err != nil {
		return HistoryPage{}, err
	}
	if cursor == 0 {
		return s.GetHistory(ctx, peerPublicKey, "", syncLimit(expectedCount), "older")
	}
	for attempts := 0; attempts < 100; attempts++ {
		page, err := s.GetHistory(ctx, peerPublicKey, strconv.FormatInt(cursor, 10), limit, "newer")
		if err != nil {
			return HistoryPage{}, err
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

	return HistoryPage{Messages: synced}, nil
}

func (s *Service) cachedHistory(peerPublicKey string, cursor string, limit int32, direction string) (HistoryPage, error) {
	if s.cache == nil {
		return HistoryPage{}, errors.New("offline")
	}
	messages, nextCursor, hasMore, err := s.cache.History(peerPublicKey, cursor, int(limit), direction)
	if err != nil {
		return HistoryPage{}, err
	}
	return HistoryPage{
		Messages:   messagesFromCache(messages),
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *Service) cacheHistoryPage(peerPublicKey string, page rpcclient.HistoryPage) HistoryPage {
	messages := messagesFromRPC(page.Messages)
	if s.cache != nil {
		_ = s.cache.SaveMessages(peerPublicKey, messagesToCache(peerPublicKey, messages))
	}
	return HistoryPage{
		Messages:   messages,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
}

func (s *Service) SendMessage(ctx context.Context, receiverPublicKey string, text string) (Message, error) {
	receiverPublicKey = strings.TrimSpace(receiverPublicKey)
	if receiverPublicKey == "" {
		return Message{}, errors.New("receiver public key is required")
	}
	if strings.TrimSpace(text) == "" {
		return Message{}, errors.New("message is required")
	}
	clientID := newClientMessageID()
	now := time.Now().UTC().Format(time.RFC3339)
	pending := Message{
		ID:        clientID,
		Sender:    s.PublicKey(),
		Text:      text,
		Timestamp: now,
		Delivery:  "pending",
	}
	if s.cache == nil {
		message, err := s.rpc.SendMessageWithID(ctx, receiverPublicKey, text, clientID)
		if err != nil {
			return Message{}, err
		}
		return Message{
			ID:        message.ID,
			Sender:    s.PublicKey(),
			Text:      text,
			Timestamp: message.Timestamp,
			Delivery:  "sent",
			ServerSeq: message.ServerSeq,
		}, nil
	}
	if err := s.cache.EnqueueOutbox(storage.OutboxItem{
		ID:          clientID,
		ReceiverKey: receiverPublicKey,
		Text:        text,
		Status:      0,
		CreatedAt:   now,
	}, pending.Sender); err != nil {
		return Message{}, err
	}
	if s.IsOffline() {
		return pending, nil
	}
	if err := s.sendOutboxItem(ctx, storage.OutboxItem{ID: clientID, ReceiverKey: receiverPublicKey, Text: text}); err != nil {
		pending.Error = err.Error()
		return pending, nil
	}
	item, err := s.cache.MessageByClientID(clientID)
	if err == nil && item.ID != "" {
		return messagesFromCache([]storage.Message{item})[0], nil
	}
	return Message{
		ID:        clientID,
		Sender:    pending.Sender,
		Text:      text,
		Timestamp: now,
		Delivery:  "sent",
	}, nil
}

func (s *Service) MarkConversationRead(ctx context.Context, peerPublicKey string, seq int64) error {
	if !s.IsOffline() {
		if err := s.rpc.MarkConversationRead(ctx, peerPublicKey, seq); err != nil {
			return err
		}
	}
	if s.cache != nil {
		_ = s.cache.MarkConversationRead(peerPublicKey, seq)
	}
	return nil
}

func (s *Service) Subscribe(ctx context.Context) (<-chan Event, <-chan error) {
	rpcEvents, rpcErrs := s.rpc.Subscribe(ctx)
	events := make(chan Event, 16)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		for {
			select {
			case event, ok := <-rpcEvents:
				if !ok {
					return
				}
				events <- Event{
					Kind:      event.Kind,
					PublicKey: event.PublicKey,
					Text:      event.Text,
					Reason:    event.Reason,
					Count:     event.Count,
				}
			case err, ok := <-rpcErrs:
				if ok && err != nil {
					errs <- err
				}
				return
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
		}
	}()
	return events, errs
}

func (s *Service) PublicKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publicKey
}

func (s *Service) IsOffline() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offline
}

func (s *Service) PollAdminEvents(ctx context.Context) (AdminUpdate, error) {
	if s.IsOffline() {
		return AdminUpdate{}, errors.New("offline")
	}
	update, err := s.rpc.PollAdminEvents(ctx)
	if err != nil {
		return AdminUpdate{}, err
	}
	users := make([]UserInfo, 0, len(update.Users))
	for _, user := range update.Users {
		users = append(users, UserInfo{
			PublicKey: user.PublicKey,
			Online:    user.Online,
			Banned:    user.Banned,
		})
	}
	return AdminUpdate{
		Users: users,
		Stats: AdminStats{
			OnlineUsers: update.Stats.OnlineUsers,
			TotalUsers:  update.Stats.TotalUsers,
			BannedUsers: update.Stats.BannedUsers,
		},
	}, nil
}

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

func (s *Service) isLocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lockedUntil.IsZero() && time.Now().Before(s.lockedUntil) {
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
	items, err := s.cache.DueOutbox(time.Now().UTC().Format(time.RFC3339), 20)
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

func (s *Service) setSession(publicKey string, role int32, offline bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publicKey = publicKey
	s.role = role
	s.offline = offline
}

func (s *Service) setCacheOwner(publicKey string) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.SetOwner(publicKey)
}

func friendsFromCache(friends []storage.Friend) []Friend {
	result := make([]Friend, 0, len(friends))
	for _, friend := range friends {
		result = append(result, Friend{
			PublicKey: friend.PublicKey,
			Alias:     friend.Alias,
			Online:    friend.Online,
			Unread:    friend.Unread,
		})
	}
	return result
}

func friendsToCache(friends []Friend) []storage.Friend {
	result := make([]storage.Friend, 0, len(friends))
	for _, friend := range friends {
		result = append(result, storage.Friend{
			PublicKey: friend.PublicKey,
			Alias:     friend.Alias,
			Online:    friend.Online,
			Unread:    friend.Unread,
		})
	}
	return result
}

func messagesFromCache(messages []storage.Message) []Message {
	result := make([]Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, Message{
			ID:        message.ID,
			Sender:    message.Sender,
			Text:      message.Text,
			Timestamp: message.Timestamp,
			Delivery:  message.Delivery,
			Error:     message.Error,
			ServerSeq: message.ServerSeq,
		})
	}
	return result
}

func messagesFromRPC(messages []rpcclient.Message) []Message {
	result := make([]Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, Message{
			ID:        message.ID,
			Sender:    message.Sender,
			Text:      message.Text,
			Timestamp: message.Timestamp,
			Delivery:  message.Delivery,
			Error:     message.Error,
			ServerSeq: message.ServerSeq,
		})
	}
	return result
}

func messagesToCache(peerKey string, messages []Message) []storage.Message {
	result := make([]storage.Message, 0, len(messages))
	for _, message := range messages {
		result = append(result, storage.Message{
			ID:        message.ID,
			ClientID:  message.ID,
			PeerKey:   peerKey,
			Sender:    message.Sender,
			Text:      message.Text,
			Timestamp: message.Timestamp,
			Delivery:  message.Delivery,
			Error:     message.Error,
			ServerSeq: message.ServerSeq,
		})
	}
	return result
}

func containsServerSeq(messages []Message, seq int64) bool {
	for _, message := range messages {
		if message.ServerSeq == seq {
			return true
		}
	}
	return false
}

func maxServerSeq(messages []Message) int64 {
	var seq int64
	for _, message := range messages {
		if message.ServerSeq > seq {
			seq = message.ServerSeq
		}
	}
	return seq
}

func syncLimit(expectedCount int32) int32 {
	if expectedCount <= 0 {
		return 100
	}
	if expectedCount < 30 {
		return 30
	}
	if expectedCount > 100 {
		return 100
	}
	return expectedCount
}

func newClientMessageID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}

func nextRetryTime(attempts int) string {
	delay := 5 * time.Second
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
			break
		}
	}
	return time.Now().UTC().Add(delay).Format(time.RFC3339)
}

func isOfflineLoginError(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "service unavailable") ||
		strings.Contains(text, "request timed out") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "deadline exceeded") ||
		strings.Contains(text, "unavailable")
}
