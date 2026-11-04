package multi

import "sync"

// KeyedSerializer manages per-key mutexes for SerializeBy declarations (e.g., accountKey).
// Ensures operations sharing a specific key execute sequentially across worker pool goroutines.
type KeyedSerializer struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewKeyedSerializer creates a new KeyedSerializer.
func NewKeyedSerializer() *KeyedSerializer {
	return &KeyedSerializer{
		locks: make(map[string]*sync.Mutex),
	}
}

// Lock acquires the mutex associated with the specified key.
// Returns an unlock function that must be called when the critical section is complete.
func (ks *KeyedSerializer) Lock(key string) func() {
	if key == "" {
		return func() {}
	}

	ks.mu.Lock()
	m, ok := ks.locks[key]
	if !ok {
		m = &sync.Mutex{}
		ks.locks[key] = m
	}
	ks.mu.Unlock()

	m.Lock()
	return m.Unlock
}

// RemoveKey removes the lock associated with the given key to free memory when a bot is deleted.
func (ks *KeyedSerializer) RemoveKey(key string) {
	if key == "" {
		return
	}
	ks.mu.Lock()
	delete(ks.locks, key)
	ks.mu.Unlock()
}
