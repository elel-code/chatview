package eventhub

import (
	"context"
	"sync"
	"time"

	"chatview/api/gen/chatview/events"
	"chatview/server/internal/db"
)

type Hub struct {
	mu      sync.RWMutex
	streams map[string]map[string]chan *eventspb.ServerEvent
}

func New() *Hub {
	return &Hub{streams: make(map[string]map[string]chan *eventspb.ServerEvent)}
}

func (h *Hub) Register(pubKey, clientID string, ch chan *eventspb.ServerEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.streams[pubKey] == nil {
		h.streams[pubKey] = make(map[string]chan *eventspb.ServerEvent)
	}
	h.streams[pubKey][clientID] = ch
}

func (h *Hub) Unregister(pubKey, clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.streams[pubKey], clientID)
	if len(h.streams[pubKey]) == 0 {
		delete(h.streams, pubKey)
	}
}

func (h *Hub) Push(pubKey string, event *eventspb.ServerEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.streams[pubKey] {
		select {
		case ch <- event:
		default:
		}
	}
}

func (h *Hub) Broadcast(event *eventspb.ServerEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, clients := range h.streams {
		for _, ch := range clients {
			select {
			case ch <- event:
			default:
			}
		}
	}
}

func (h *Hub) PushAdmins(ctx context.Context, store *db.Store, event *eventspb.ServerEvent) {
	var pubKeys []string
	if err := store.DB.SelectContext(ctx, &pubKeys, `
        SELECT pub_key
        FROM users
        WHERE role = 1 AND status = 1
    `); err != nil {
		return
	}
	for _, pubKey := range pubKeys {
		h.Push(pubKey, event)
	}
}

func (h *Hub) KickUser(pubKey string) {
	h.mu.Lock()
	clients := h.streams[pubKey]
	delete(h.streams, pubKey)
	h.mu.Unlock()

	for _, ch := range clients {
		close(ch)
	}
}

func (h *Hub) OnlinePubKeys() map[string]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]bool, len(h.streams))
	for pubKey := range h.streams {
		out[pubKey] = true
	}
	return out
}

func RunPresenceHealer(ctx context.Context, store *db.Store, hub *Hub, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = store.HealPresence(context.Background(), hub.OnlinePubKeys())
		case <-ctx.Done():
			return
		}
	}
}
