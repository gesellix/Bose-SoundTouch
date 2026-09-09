package webtypes

import (
	"sync"
	"testing"
)

// TestBalanceRefreshCoalesces pins the burst behaviour. Dragging the Player's
// balance slider emits several balanceUpdated frames inside one second —
// four, measured on hardware — and each used to start its own read of an
// endpoint that blocks on a sleeping speaker.
func TestBalanceRefreshCoalesces(t *testing.T) {
	conn := &DeviceConnection{}

	if !conn.BeginBalanceRefresh() {
		t.Fatal("the first caller must perform the read")
	}

	// Three more events arrive while that read is in flight.
	for i := range 3 {
		if conn.BeginBalanceRefresh() {
			t.Errorf("event %d started a second concurrent read", i+2)
		}
	}

	// They collapse into exactly one follow-up, not three.
	if !conn.EndBalanceRefresh() {
		t.Fatal("a read requested during the first one was dropped")
	}

	if conn.EndBalanceRefresh() {
		t.Error("the burst produced more than one follow-up read")
	}

	// Back to idle: the next event reads again.
	if !conn.BeginBalanceRefresh() {
		t.Error("state did not return to idle")
	}
}

// TestBalanceRefreshIsRaceFree exercises the CAS loops under concurrency.
func TestBalanceRefreshIsRaceFree(t *testing.T) {
	conn := &DeviceConnection{}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		readers int
	)

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if !conn.BeginBalanceRefresh() {
				return
			}

			for {
				mu.Lock()
				readers++
				mu.Unlock()

				if !conn.EndBalanceRefresh() {
					return
				}
			}
		}()
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if readers == 0 {
		t.Error("no read was performed at all")
	}

	if readers > 50 {
		t.Errorf("performed %d reads for 50 events; coalescing did not bound them", readers)
	}
}
