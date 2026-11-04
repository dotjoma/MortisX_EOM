package multi

import (
	"sync"
	"time"
)

// DebouncedPersister wraps a save function and debounces write operations.
type DebouncedPersister struct {
	mu       sync.Mutex
	saveFn   func(b Bot) error
	interval time.Duration
	pending  map[string]Bot
	timer    *time.Timer
	clock    Clock
}

// NewDebouncedPersister creates a DebouncedPersister with the given debounce window.
func NewDebouncedPersister(interval time.Duration, saveFn func(b Bot) error, clock Clock) *DebouncedPersister {
	if clock == nil {
		clock = RealClock{}
	}
	return &DebouncedPersister{
		saveFn:   saveFn,
		interval: interval,
		pending:  make(map[string]Bot),
		clock:    clock,
	}
}

// Save marks a bot as dirty and schedules a coalesced debounced save.
func (dp *DebouncedPersister) Save(b Bot) error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	id := b.ID()
	dp.pending[id] = b

	if dp.timer == nil {
		dp.timer = time.AfterFunc(dp.interval, dp.flush)
	}
	return nil
}

func (dp *DebouncedPersister) flush() {
	dp.mu.Lock()
	if len(dp.pending) == 0 {
		dp.timer = nil
		dp.mu.Unlock()
		return
	}
	var sample Bot
	for _, b := range dp.pending {
		sample = b
		break
	}
	dp.pending = make(map[string]Bot)
	dp.timer = nil
	dp.mu.Unlock()

	if dp.saveFn != nil && sample != nil {
		_ = dp.saveFn(sample)
	}
}

// Flush immediately writes all pending saves synchronously.
func (dp *DebouncedPersister) Flush() error {
	dp.mu.Lock()
	if dp.timer != nil {
		dp.timer.Stop()
		dp.timer = nil
	}
	if len(dp.pending) == 0 {
		dp.mu.Unlock()
		return nil
	}
	var sample Bot
	for _, b := range dp.pending {
		sample = b
		break
	}
	dp.pending = make(map[string]Bot)
	dp.mu.Unlock()

	if dp.saveFn != nil && sample != nil {
		return dp.saveFn(sample)
	}
	return nil
}
