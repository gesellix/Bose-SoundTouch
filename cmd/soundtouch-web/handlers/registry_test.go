// Package handlers contains tests for the device registry API on
// WebApp (GetDevice, AddDevice, TouchDevice, DeviceSnapshot,
// DeviceCount).
package handlers

import (
	"fmt"
	"sync"
	"testing"

	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func newRegistryDevice(name string) *webtypes.DeviceConnection {
	return &webtypes.DeviceConnection{
		DeviceInfo: &models.DeviceInfo{Name: name},
		Status:     webtypes.DeviceStatus{IsConnected: true},
	}
}

func TestAddDevice_Inserts(t *testing.T) {
	app := NewWebApp()
	conn := newRegistryDevice("first")

	if !app.AddDevice("host-1", conn) {
		t.Fatal("AddDevice returned false on first insert")
	}

	got, ok := app.GetDevice("host-1")
	if !ok {
		t.Fatal("GetDevice did not find the device after AddDevice")
	}

	if got != conn {
		t.Errorf("GetDevice returned a different pointer than inserted")
	}
}

func TestAddDevice_RejectsDuplicateAndBumpsLastSeen(t *testing.T) {
	app := NewWebApp()
	original := newRegistryDevice("first")
	app.AddDevice("host-1", original)

	originalSeen := original.LastSeen
	replacement := newRegistryDevice("second")

	if app.AddDevice("host-1", replacement) {
		t.Fatal("AddDevice returned true on duplicate; expected false")
	}

	got, _ := app.GetDevice("host-1")
	if got != original {
		t.Error("Duplicate AddDevice replaced the existing device pointer")
	}

	if !got.LastSeen.After(originalSeen) {
		t.Error("Duplicate AddDevice did not bump LastSeen on existing device")
	}
}

func TestTouchDevice(t *testing.T) {
	app := NewWebApp()

	if app.TouchDevice("missing") {
		t.Error("TouchDevice returned true for unknown id")
	}

	conn := newRegistryDevice("first")
	app.AddDevice("host-1", conn)
	seenBefore := conn.LastSeen

	if !app.TouchDevice("host-1") {
		t.Fatal("TouchDevice returned false for known id")
	}

	if !conn.LastSeen.After(seenBefore) {
		t.Error("TouchDevice did not bump LastSeen")
	}
}

func TestDeviceSnapshotAndCount(t *testing.T) {
	app := NewWebApp()

	if got := app.DeviceCount(); got != 0 {
		t.Errorf("DeviceCount on empty app = %d; want 0", got)
	}

	if snap := app.DeviceSnapshot(); len(snap) != 0 {
		t.Errorf("DeviceSnapshot on empty app = %v; want []", snap)
	}

	for i := 0; i < 5; i++ {
		app.AddDevice(fmt.Sprintf("host-%d", i), newRegistryDevice(fmt.Sprintf("n%d", i)))
	}

	if got := app.DeviceCount(); got != 5 {
		t.Errorf("DeviceCount after 5 adds = %d; want 5", got)
	}

	snap := app.DeviceSnapshot()
	if len(snap) != 5 {
		t.Errorf("DeviceSnapshot len = %d; want 5", len(snap))
	}

	// Spot-check that the snapshot ids match what we inserted.
	seen := map[string]bool{}
	for _, entry := range snap {
		seen[entry.ID] = true
	}

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("host-%d", i)
		if !seen[id] {
			t.Errorf("DeviceSnapshot missing %s", id)
		}
	}
}

// TestRegistryConcurrent exercises the registry from many goroutines
// at once. Before the introduction of devicesMu this would either
// panic with "fatal error: concurrent map read and map write" or be
// flagged by the race detector. The test runs under `go test -race`
// in CI so a future regression that re-exposes the underlying map
// without locking would be caught here.
func TestRegistryConcurrent(t *testing.T) {
	app := NewWebApp()

	const workers = 16

	const opsPerWorker = 200

	var wg sync.WaitGroup
	wg.Add(workers * 4)

	// Writers: insert distinct ids across workers.
	for w := 0; w < workers; w++ {
		go func(worker int) {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				id := fmt.Sprintf("w%d-%d", worker, i)
				app.AddDevice(id, newRegistryDevice(id))
			}
		}(w)
	}

	// Touchers: bump LastSeen on a shared id (which may or may not
	// exist yet — both branches are exercised).
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				app.TouchDevice("shared")
			}
		}()
	}

	// Readers via snapshot.
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				_ = app.DeviceSnapshot()
			}
		}()
	}

	// Readers via direct lookup.
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				_, _ = app.GetDevice("shared")
				_ = app.DeviceCount()
			}
		}()
	}

	wg.Wait()

	// Sanity check: every writer inserted opsPerWorker devices, plus
	// the "shared" entry was never AddDevice'd so should be absent.
	if got, want := app.DeviceCount(), workers*opsPerWorker; got != want {
		t.Errorf("DeviceCount after concurrent inserts = %d; want %d", got, want)
	}

	if _, ok := app.GetDevice("shared"); ok {
		t.Error("shared device should not exist (only TouchDevice was called for it)")
	}
}
