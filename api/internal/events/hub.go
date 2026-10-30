package events

import (
	"sync"

	"github.com/google/uuid"
)

type Event struct {
	Kind    string `json:"kind"`
	Payload any    `json:"payload"`
}

const hubBuffer = 16

type Hub struct {
	mu   sync.RWMutex
	subs map[uuid.UUID]map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: map[uuid.UUID]map[chan Event]struct{}{}}
}

func (h *Hub) Subscribe(userID uuid.UUID) (chan Event, func()) {
	ch := make(chan Event, hubBuffer)
	h.mu.Lock()
	if h.subs[userID] == nil {
		h.subs[userID] = map[chan Event]struct{}{}
	}
	h.subs[userID][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if m, ok := h.subs[userID]; ok {
				delete(m, ch)
				if len(m) == 0 {
					delete(h.subs, userID)
				}
			}
			close(ch)
		})
	}
	return ch, unsubscribe
}

func (h *Hub) Publish(userID uuid.UUID, ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[userID] {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (h *Hub) Subscribers(userID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs[userID])
}
