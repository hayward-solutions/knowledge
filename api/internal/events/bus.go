package events

import (
	"log"
	"sync"
)

type Bus struct {
	mu       sync.RWMutex
	handlers []func(any)
	wg       sync.WaitGroup
}

func NewBus() *Bus { return &Bus{} }

func (b *Bus) Subscribe(h func(any)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

func (b *Bus) Publish(event any) {
	b.mu.RLock()
	hs := make([]func(any), len(b.handlers))
	copy(hs, b.handlers)
	b.mu.RUnlock()
	for _, h := range hs {
		b.wg.Add(1)
		go func(h func(any)) {
			defer b.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("event handler panic for %T: %v", event, r)
				}
			}()
			h(event)
		}(h)
	}
}

func (b *Bus) Wait() { b.wg.Wait() }
