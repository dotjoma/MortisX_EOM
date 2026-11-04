package multi

import (
	"sort"
	"sync"
)

// ActivityManager coordinates cross-module exclusive activity locks on bots.
// Prevents conflicting operations (e.g. Farm movement breaking a Transfer trade handshake).
type ActivityManager struct {
	mu         sync.Mutex
	activities map[string]string // botID -> activity name
}

// NewActivityManager creates a new ActivityManager.
func NewActivityManager() *ActivityManager {
	return &ActivityManager{
		activities: make(map[string]string),
	}
}

// Get returns the current activity for a bot, or empty string if idle.
func (am *ActivityManager) Get(botID string) string {
	am.mu.Lock()
	defer am.mu.Unlock()
	return am.activities[botID]
}

// Set forcibly sets the activity for a bot (useful for tests or external engine sync).
func (am *ActivityManager) Set(botID, activity string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	if activity == "" {
		delete(am.activities, botID)
	} else {
		am.activities[botID] = activity
	}
}

// Clear releases the activity lock on a bot.
func (am *ActivityManager) Clear(botID string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.activities, botID)
}

// TryLock attempts to acquire the named activity lock on a single bot.
// If already busy with another activity, returns ok=false and the current activity name.
func (am *ActivityManager) TryLock(botID string, activity string) (unlock func(), ok bool, current string) {
	if activity == "" {
		return func() {}, true, ""
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	cur, exists := am.activities[botID]
	if exists && cur != "" {
		return nil, false, cur
	}

	am.activities[botID] = activity
	return func() {
		am.Clear(botID)
	}, true, ""
}

// TryLockMultiple acquires activity locks on multiple bots in all-or-nothing fashion.
// To prevent deadlocks across concurrent multi-bot dispatches, bot IDs are sorted lexicographically.
// If any bot in the set cannot be locked, all previously acquired locks in the set are rolled back.
func (am *ActivityManager) TryLockMultiple(botIDs []string, activity string) (unlock func(), ok bool, busyBotID string, current string) {
	if activity == "" || len(botIDs) == 0 {
		return func() {}, true, "", ""
	}

	// Deduplicate and sort IDs lexicographically to avoid deadlocks
	uniqueMap := make(map[string]struct{}, len(botIDs))
	for _, id := range botIDs {
		if id != "" {
			uniqueMap[id] = struct{}{}
		}
	}
	sortedIDs := make([]string, 0, len(uniqueMap))
	for id := range uniqueMap {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	am.mu.Lock()
	defer am.mu.Unlock()

	// First pass: verify none are busy
	for _, id := range sortedIDs {
		cur, exists := am.activities[id]
		if exists && cur != "" {
			return nil, false, id, cur
		}
	}

	// Second pass: acquire all
	for _, id := range sortedIDs {
		am.activities[id] = activity
	}

	unlock = func() {
		am.mu.Lock()
		defer am.mu.Unlock()
		for _, id := range sortedIDs {
			if am.activities[id] == activity {
				delete(am.activities, id)
			}
		}
	}

	return unlock, true, "", ""
}
