package marge

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestConcurrentUpdatePresetNoLostUpdates is a regression test for #614's
// 2026-08-23 reproduction: a reporter's script stored six presets via rapid,
// overlapping PUT .../preset/N requests (visible in the speaker's own log as
// interleaved connection IDs, never waiting for one PUT to complete before
// firing the next). One preset silently vanished from Presets.xml.
//
// UpdatePreset used to do GetPresets, mutate one slot, SavePresets as three
// separate steps with no lock spanning them — a classic lost-update race:
// two concurrent calls can each read the same starting list, mutate
// different slots, and the second writer's SavePresets clobbers the first
// writer's update. Fixed by routing the write through
// datastore.MutatePresets, which holds a single write lock for the whole
// read-mutate-write cycle.
func TestConcurrentUpdatePresetNoLostUpdates(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "marge-concurrent-update-preset-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	account := "1234567"
	device := "B0D5CC25479C"

	const presetCount = 6

	var wg sync.WaitGroup

	errs := make([]error, presetCount)

	for i := 1; i <= presetCount; i++ {
		wg.Add(1)

		go func(presetNumber int) {
			defer wg.Done()

			// sourceid 10003 is the canonical LOCAL_INTERNET_RADIO built-in
			// (see CanonicalSourceByID) — same shape as Henri's own repro
			// script, which stored six LOCAL_INTERNET_RADIO presets.
			putXML := []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<preset>
    <name>Station %d</name>
    <sourceid>10003</sourceid>
    <location>/custom/v1/playback/station%d</location>
    <contentItemType>stationurl</contentItemType>
</preset>`, presetNumber, presetNumber))

			_, err := UpdatePreset(ds, account, device, presetNumber, putXML)
			errs[presetNumber-1] = err
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("UpdatePreset(preset=%d) returned error: %v", i+1, err)
		}
	}

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if len(presets) != presetCount {
		t.Fatalf("expected %d presets after %d concurrent UpdatePreset calls, got %d: %+v", presetCount, presetCount, len(presets), presets)
	}

	for i, p := range presets {
		want := fmt.Sprintf("Station %d", i+1)
		if p.Name != want {
			t.Errorf("preset slot %d: expected name %q, got %q — a concurrent update was lost", i+1, want, p.Name)
		}
	}
}
