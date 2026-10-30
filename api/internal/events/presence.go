package events

import (
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

const (
	DefaultPresenceTTL   = 30 * time.Second
	defaultSweepInterval = 10 * time.Second
)

type presenceEntry struct {
	user     domain.UserRef
	lastSeen time.Time
}

type Presence struct {
	mu    sync.Mutex
	ttl   time.Duration
	pages map[uuid.UUID]map[uuid.UUID]presenceEntry
	now   func() time.Time
	stop  chan struct{}
	once  sync.Once
}

func NewPresence() *Presence {
	return NewPresenceWithTTL(DefaultPresenceTTL, defaultSweepInterval)
}

func NewPresenceWithTTL(ttl, sweepEvery time.Duration) *Presence {
	p := &Presence{
		ttl:   ttl,
		pages: map[uuid.UUID]map[uuid.UUID]presenceEntry{},
		now:   time.Now,
		stop:  make(chan struct{}),
	}
	go p.sweepLoop(sweepEvery)
	return p
}

func (p *Presence) Touch(pageID uuid.UUID, user domain.UserRef) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := p.pages[pageID]
	if m == nil {
		m = map[uuid.UUID]presenceEntry{}
		p.pages[pageID] = m
	}
	m[user.ID] = presenceEntry{user: user, lastSeen: p.now()}
}

func (p *Presence) Editors(pageID uuid.UUID) []domain.UserRef {
	p.mu.Lock()
	defer p.mu.Unlock()
	cutoff := p.now().Add(-p.ttl)
	out := []domain.UserRef{}
	for _, e := range p.pages[pageID] {
		if e.lastSeen.After(cutoff) {
			out = append(out, e.user)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DisplayName != out[j].DisplayName {
			return out[i].DisplayName < out[j].DisplayName
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out
}

func (p *Presence) Remove(pageID, userID uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if m := p.pages[pageID]; m != nil {
		delete(m, userID)
		if len(m) == 0 {
			delete(p.pages, pageID)
		}
	}
}

func (p *Presence) Close() {
	p.once.Do(func() { close(p.stop) })
}

func (p *Presence) sweepLoop(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.sweep()
		}
	}
}

func (p *Presence) sweep() {
	p.mu.Lock()
	defer p.mu.Unlock()
	cutoff := p.now().Add(-p.ttl)
	for pageID, m := range p.pages {
		for uid, e := range m {
			if !e.lastSeen.After(cutoff) {
				delete(m, uid)
			}
		}
		if len(m) == 0 {
			delete(p.pages, pageID)
		}
	}
}
