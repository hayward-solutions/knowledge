package events

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

func TestBusDeliversToAllHandlers(t *testing.T) {
	b := NewBus()
	var a, c atomic.Int32
	b.Subscribe(func(ev any) {
		if _, ok := ev.(domain.PagePublished); ok {
			a.Add(1)
		}
	})
	b.Subscribe(func(ev any) { c.Add(1) })
	b.Publish(domain.PagePublished{PageID: uuid.New()})
	b.Publish(domain.PageTrashed{PageID: uuid.New()})
	b.Wait()
	if a.Load() != 1 || c.Load() != 2 {
		t.Fatalf("a=%d c=%d", a.Load(), c.Load())
	}
}

func TestBusRecoversPanics(t *testing.T) {
	b := NewBus()
	var ok atomic.Bool
	b.Subscribe(func(any) { panic("boom") })
	b.Subscribe(func(any) { ok.Store(true) })
	b.Publish(struct{}{})
	b.Wait()
	if !ok.Load() {
		t.Fatal("second handler did not run after first panicked")
	}
}

func TestHubPublishSubscribe(t *testing.T) {
	h := NewHub()
	u := uuid.New()
	ch, unsub := h.Subscribe(u)
	other, unsubOther := h.Subscribe(uuid.New())
	defer unsubOther()
	h.Publish(u, Event{Kind: "notification", Payload: 1})
	select {
	case ev := <-ch:
		if ev.Kind != "notification" {
			t.Fatalf("kind %q", ev.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	select {
	case <-other:
		t.Fatal("other user received event")
	default:
	}
	unsub()
	unsub()
	if _, open := <-ch; open {
		t.Fatal("channel should be closed")
	}
	if h.Subscribers(u) != 0 {
		t.Fatal("subscriber not removed")
	}
	h.Publish(u, Event{Kind: "x"})
}

func TestHubDropsWhenFull(t *testing.T) {
	h := NewHub()
	u := uuid.New()
	ch, unsub := h.Subscribe(u)
	defer unsub()
	for i := 0; i < 100; i++ {
		h.Publish(u, Event{Kind: "k", Payload: i})
	}
	if len(ch) != 16 {
		t.Fatalf("buffered %d, want 16", len(ch))
	}
}

func TestPresenceTTL(t *testing.T) {
	p := NewPresenceWithTTL(30*time.Second, time.Hour)
	defer p.Close()
	now := time.Now()
	p.now = func() time.Time { return now }
	page := uuid.New()
	alice := domain.UserRef{ID: uuid.New(), DisplayName: "Alice"}
	bob := domain.UserRef{ID: uuid.New(), DisplayName: "Bob"}
	p.Touch(page, bob)
	p.Touch(page, alice)
	eds := p.Editors(page)
	if len(eds) != 2 || eds[0].DisplayName != "Alice" {
		t.Fatalf("editors = %+v", eds)
	}
	now = now.Add(20 * time.Second)
	p.Touch(page, bob)
	now = now.Add(15 * time.Second)
	eds = p.Editors(page)
	if len(eds) != 1 || eds[0].DisplayName != "Bob" {
		t.Fatalf("editors after ttl = %+v", eds)
	}
	p.sweep()
	if len(p.pages[page]) != 1 {
		t.Fatalf("sweep left %d entries", len(p.pages[page]))
	}
	now = now.Add(time.Minute)
	p.sweep()
	if len(p.pages) != 0 {
		t.Fatal("sweep should drop empty pages")
	}
	if eds := p.Editors(uuid.New()); eds == nil || len(eds) != 0 {
		t.Fatal("Editors should return an empty non-nil slice")
	}
}
