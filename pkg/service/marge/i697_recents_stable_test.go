package marge

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestRecentsServeTheSameBytesTwice guards the ETag over the served recents
// (issue 697 follow-up). Recents taken over from the speaker carry no
// createdOn, and rendering filled in the current time, so two requests never
// returned the same list. createdOn now falls back to the time the item was
// last played.
func TestRecentsServeTheSameBytesTwice(t *testing.T) {
	const (
		account = "0000000"
		device  = "DEVICEID01"
	)

	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{device: nil})

	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="1" utcTime="1789000000">
        <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s0000" sourceAccount="" isPresetable="true">
            <itemName>Station</itemName>
        </contentItem>
    </recent>
</recents>`
	if err := os.WriteFile(ds.AccountDeviceDir(account, device)+"/Recents.xml", []byte(recentsXML), 0o644); err != nil {
		t.Fatalf("write Recents.xml: %v", err)
	}

	first, err := RecentsToXML(ds, account, device)
	if err != nil {
		t.Fatalf("recents XML: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	second, err := RecentsToXML(ds, account, device)
	if err != nil {
		t.Fatalf("recents XML: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Fatalf("two renderings of unchanged recents differ:\n%s\n---\n%s", first, second)
	}

	if !strings.Contains(string(first), "<createdOn>2026-09-10T00:26:40") {
		t.Errorf("createdOn does not fall back to the last-played time:\n%s", first)
	}
}
