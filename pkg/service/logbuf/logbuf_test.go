package logbuf

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuffer_SingleLineWrite(t *testing.T) {
	b := New(8)

	n, err := b.Write([]byte("hello\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if n != len("hello\n") {
		t.Errorf("expected %d bytes consumed, got %d", len("hello\n"), n)
	}

	got := b.Snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}

	if got[0].Message != "hello" {
		t.Errorf("expected message 'hello', got %q", got[0].Message)
	}

	if got[0].Seq != 1 {
		t.Errorf("expected seq=1, got %d", got[0].Seq)
	}
}

func TestBuffer_MultiLineSingleWrite(t *testing.T) {
	b := New(8)
	_, _ = b.Write([]byte("one\ntwo\nthree\n"))

	got := b.Snapshot()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}

	want := []string{"one", "two", "three"}
	for i := range got {
		if got[i].Message != want[i] {
			t.Errorf("entry %d: want %q, got %q", i, want[i], got[i].Message)
		}

		if got[i].Seq != uint64(i+1) {
			t.Errorf("entry %d: want seq=%d, got %d", i, i+1, got[i].Seq)
		}
	}
}

func TestBuffer_PartialLineBuffered(t *testing.T) {
	b := New(8)

	_, _ = b.Write([]byte("hel"))
	_, _ = b.Write([]byte("lo "))
	_, _ = b.Write([]byte("world\n"))

	got := b.Snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}

	if got[0].Message != "hello world" {
		t.Errorf("expected reassembled line, got %q", got[0].Message)
	}
}

func TestBuffer_PartialLineDoesNotLeakUntilNewline(t *testing.T) {
	b := New(8)
	_, _ = b.Write([]byte("no newline here"))

	if b.Len() != 0 {
		t.Errorf("expected zero entries for an unterminated write, got %d", b.Len())
	}
}

func TestBuffer_RingEvictionAtCapacity(t *testing.T) {
	const capacity = 3

	b := New(capacity)
	for i := 1; i <= 5; i++ {
		_, _ = fmt.Fprintf(b, "line %d\n", i)
	}

	got := b.Snapshot()
	if len(got) != capacity {
		t.Fatalf("expected %d entries after eviction, got %d", capacity, len(got))
	}

	// Oldest surviving entry should be "line 3" with Seq=3.
	if got[0].Message != "line 3" || got[0].Seq != 3 {
		t.Errorf("oldest survivor: want line 3/seq 3, got %q/seq %d", got[0].Message, got[0].Seq)
	}

	if got[capacity-1].Message != "line 5" || got[capacity-1].Seq != 5 {
		t.Errorf("newest entry: want line 5/seq 5, got %q/seq %d", got[capacity-1].Message, got[capacity-1].Seq)
	}
}

func TestBuffer_SinceFilters(t *testing.T) {
	b := New(8)
	for i := 1; i <= 5; i++ {
		_, _ = fmt.Fprintf(b, "line %d\n", i)
	}

	entries, nextSince, dropped := b.Since(2, 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries since=2, got %d", len(entries))
	}

	if entries[0].Seq != 3 {
		t.Errorf("first entry seq: want 3, got %d", entries[0].Seq)
	}

	if nextSince != 5 {
		t.Errorf("nextSince: want 5, got %d", nextSince)
	}

	if dropped != 0 {
		t.Errorf("dropped: want 0, got %d", dropped)
	}
}

func TestBuffer_SinceReportsDropped(t *testing.T) {
	b := New(3)
	for i := 1; i <= 10; i++ {
		_, _ = fmt.Fprintf(b, "line %d\n", i)
	}

	// Capacity 3 → only seq 8,9,10 remain. Polling with since=2
	// means seq 3..7 (five entries) were evicted before we saw them.
	entries, nextSince, dropped := b.Since(2, 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if dropped != 5 {
		t.Errorf("expected dropped=5, got %d", dropped)
	}

	if nextSince != 10 {
		t.Errorf("nextSince: want 10, got %d", nextSince)
	}
}

func TestBuffer_SinceRespectsLimit(t *testing.T) {
	b := New(10)
	for i := 1; i <= 5; i++ {
		_, _ = fmt.Fprintf(b, "line %d\n", i)
	}

	entries, nextSince, _ := b.Since(0, 2)
	if len(entries) != 2 {
		t.Fatalf("expected limit=2 to cap result, got %d", len(entries))
	}

	if nextSince != 2 {
		t.Errorf("nextSince after limit: want 2, got %d", nextSince)
	}
}

func TestBuffer_SinceNoNewEntries(t *testing.T) {
	b := New(4)
	_, _ = b.Write([]byte("only\n"))

	entries, nextSince, dropped := b.Since(5, 0)
	if len(entries) != 0 {
		t.Errorf("expected no entries, got %d", len(entries))
	}

	if nextSince != 5 {
		t.Errorf("nextSince should pass through since when no matches: want 5, got %d", nextSince)
	}

	if dropped != 0 {
		t.Errorf("dropped: want 0, got %d", dropped)
	}
}

func TestBuffer_TimestampMonotonic(t *testing.T) {
	b := New(4)
	// Inject a deterministic clock so the test isn't time-flaky.
	tick := time.Unix(1_700_000_000, 0)
	b.now = func() time.Time {
		t := tick
		tick = tick.Add(time.Millisecond)

		return t
	}

	_, _ = b.Write([]byte("a\nb\n"))

	got := b.Snapshot()
	if !got[1].Time.After(got[0].Time) {
		t.Errorf("expected later entry to have a later timestamp, got %v vs %v", got[1].Time, got[0].Time)
	}
}

func TestBuffer_ConcurrentWrites(t *testing.T) {
	b := New(10000)

	const writers = 8
	const perWriter = 500

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				_, _ = fmt.Fprintf(b, "w%d-i%d\n", id, i)
			}
		}(w)
	}

	wg.Wait()

	got := b.Snapshot()
	if len(got) != writers*perWriter {
		t.Fatalf("expected %d entries, got %d", writers*perWriter, len(got))
	}

	// Seq must be strictly increasing and contiguous from 1.
	for i := range got {
		if got[i].Seq != uint64(i+1) {
			t.Fatalf("seq gap at index %d: got %d", i, got[i].Seq)
		}

		if !strings.HasPrefix(got[i].Message, "w") {
			t.Errorf("unexpected message: %q", got[i].Message)
		}
	}
}

func TestBuffer_ZeroCapacityClamped(t *testing.T) {
	b := New(0)
	if b.Capacity() == 0 {
		t.Errorf("zero capacity should be clamped to a positive default")
	}
}

func TestBuffer_EmptyWriteNoOp(t *testing.T) {
	b := New(4)

	n, err := b.Write(nil)
	if err != nil {
		t.Errorf("nil write: %v", err)
	}

	if n != 0 {
		t.Errorf("nil write should consume 0 bytes, got %d", n)
	}

	if b.Len() != 0 {
		t.Errorf("buffer should be empty, got %d entries", b.Len())
	}
}
