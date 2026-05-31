package tts

import (
	"sync"
	"time"
)

// clipCache is a small, bounded in-memory store of synthesized audio clips,
// mirroring the in-memory approach the "ding" endpoint uses. Entries expire
// after a TTL; when the cache is full the oldest entry is evicted. This is a
// best-effort cache for recently spoken clips, not durable storage.
type clipCache struct {
	mu         sync.Mutex
	entries    map[string]*clipEntry
	ttl        time.Duration
	maxEntries int
	now        func() time.Time // injectable clock for tests
}

type clipEntry struct {
	audio       []byte
	contentType string
	storedAt    time.Time
}

// newClipCache returns a cache holding at most maxEntries clips, each valid for
// ttl. Non-positive values fall back to sane defaults.
func newClipCache(ttl time.Duration, maxEntries int) *clipCache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	if maxEntries <= 0 {
		maxEntries = 32
	}

	return &clipCache{
		entries:    make(map[string]*clipEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

// put stores audio under id, evicting expired entries and, if still over
// capacity, the oldest remaining entry.
func (c *clipCache) put(id string, audio []byte, contentType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.evictExpiredLocked(now)

	c.entries[id] = &clipEntry{
		audio:       audio,
		contentType: contentType,
		storedAt:    now,
	}

	for len(c.entries) > c.maxEntries {
		c.evictOldestLocked()
	}
}

// get returns the audio and content type for id if present and not expired.
func (c *clipCache) get(id string) (audio []byte, contentType string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, found := c.entries[id]
	if !found {
		return nil, "", false
	}

	if c.now().Sub(entry.storedAt) > c.ttl {
		delete(c.entries, id)
		return nil, "", false
	}

	return entry.audio, entry.contentType, true
}

// evictExpiredLocked removes all entries older than the TTL. Caller holds mu.
func (c *clipCache) evictExpiredLocked(now time.Time) {
	for id, entry := range c.entries {
		if now.Sub(entry.storedAt) > c.ttl {
			delete(c.entries, id)
		}
	}
}

// evictOldestLocked removes the single oldest entry. Caller holds mu.
func (c *clipCache) evictOldestLocked() {
	var (
		oldestID string
		oldestAt time.Time
		found    bool
	)

	for id, entry := range c.entries {
		if !found || entry.storedAt.Before(oldestAt) {
			oldestID, oldestAt, found = id, entry.storedAt, true
		}
	}

	if found {
		delete(c.entries, oldestID)
	}
}
