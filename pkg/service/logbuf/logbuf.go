// Package logbuf provides a bounded, in-memory ring buffer for log
// lines. It implements io.Writer so it can be installed as a
// second sink under stdlib log via log.SetOutput(io.MultiWriter(
// os.Stderr, buf)). Each newline-terminated chunk written to the
// buffer becomes one Entry with a monotonic sequence number and a
// capture timestamp; the admin UI polls these entries and renders
// them as a live trace of the running service.
//
// The buffer holds at most Capacity entries; older entries are
// evicted FIFO when new ones arrive. Callers that fall behind can
// detect this via the dropped count returned by Since.
package logbuf

import (
	"bytes"
	"sync"
	"time"
)

// Entry is a single buffered log line. Message excludes the
// trailing newline. Seq is monotonic across the process lifetime
// of the Buffer (it keeps counting up even past evictions, so
// clients can use it as a high-water mark).
type Entry struct {
	Seq     uint64    `json:"seq"`
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}

// Buffer is a bounded, concurrent-safe in-memory log ring.
type Buffer struct {
	mu       sync.Mutex
	capacity int
	entries  []Entry // ring; oldest first
	nextSeq  uint64  // seq assigned to the next entry written
	partial  bytes.Buffer
	now      func() time.Time
}

// New returns a Buffer that holds up to capacity entries. A
// non-positive capacity is clamped to a small default (16) — a
// zero-capacity buffer would silently drop every line and make
// the feature look broken, which is worse than picking a number.
func New(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = 16
	}

	return &Buffer{
		capacity: capacity,
		entries:  make([]Entry, 0, capacity),
		nextSeq:  1,
		now:      time.Now,
	}
}

// Write implements io.Writer. Bytes are split on '\n'; each
// complete line is appended as an Entry. A trailing partial line
// (no terminating newline) is held until the next Write supplies
// the rest. Returns len(p) on success (never reports a short
// write — the buffer always accepts the bytes, even when older
// entries get evicted).
func (b *Buffer) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	remaining := p
	for {
		idx := bytes.IndexByte(remaining, '\n')
		if idx < 0 {
			// No newline in the rest of p — stash it.
			b.partial.Write(remaining)
			break
		}

		// We have a complete line: anything held in `partial` +
		// remaining[:idx]. The newline itself is dropped.
		var line string

		if b.partial.Len() > 0 {
			b.partial.Write(remaining[:idx])
			line = b.partial.String()

			b.partial.Reset()
		} else {
			line = string(remaining[:idx])
		}

		b.append(line)

		remaining = remaining[idx+1:]
		if len(remaining) == 0 {
			break
		}
	}

	return len(p), nil
}

// append adds one Entry under the lock, evicting the oldest when
// the buffer is at capacity.
func (b *Buffer) append(message string) {
	entry := Entry{
		Seq:     b.nextSeq,
		Time:    b.now().UTC(),
		Message: message,
	}
	b.nextSeq++

	if len(b.entries) < b.capacity {
		b.entries = append(b.entries, entry)
		return
	}

	// At capacity: drop the oldest, slide left, append. A
	// circular index would avoid the copy, but at 2k entries
	// the copy is negligible and the slice API is simpler.
	copy(b.entries, b.entries[1:])
	b.entries[len(b.entries)-1] = entry
}

// Snapshot returns a copy of every currently buffered entry, in
// order of increasing Seq.
func (b *Buffer) Snapshot() []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]Entry, len(b.entries))
	copy(out, b.entries)

	return out
}

// Since returns all entries with Seq strictly greater than since,
// up to limit. nextSince is the highest Seq among the returned
// entries (or the input `since` when none match), suitable for
// the caller's next poll. dropped is the count of entries the
// caller missed — i.e. entries with Seq > since that have already
// been evicted from the buffer. A limit <= 0 means "no limit".
func (b *Buffer) Since(since uint64, limit int) (entries []Entry, nextSince, dropped uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	nextSince = since

	if len(b.entries) == 0 {
		return nil, nextSince, 0
	}

	oldest := b.entries[0].Seq
	if since+1 < oldest {
		dropped = oldest - (since + 1)
	}

	// Binary-walking the ring isn't worth it at 2k entries.
	out := make([]Entry, 0, len(b.entries))

	for i := range b.entries {
		if b.entries[i].Seq <= since {
			continue
		}

		out = append(out, b.entries[i])
		nextSince = b.entries[i].Seq

		if limit > 0 && len(out) >= limit {
			break
		}
	}

	return out, nextSince, dropped
}

// Capacity returns the configured maximum number of entries.
func (b *Buffer) Capacity() int {
	return b.capacity
}

// Len returns the current count of buffered entries.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.entries)
}
