package health

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestLooksLikeSoundTouch(t *testing.T) {
	cases := []struct {
		name string
		dev  *models.ServiceDeviceInfo
		want bool
	}{
		{name: "SoundTouch type", dev: &models.ServiceDeviceInfo{ProductCode: "SoundTouch", Name: "Bose_Bad"}, want: true},
		{name: "SoundTouch 10 sm2 type", dev: &models.ServiceDeviceInfo{ProductCode: "SoundTouch 10 sm2"}, want: true},
		{name: "Wave Music System III", dev: &models.ServiceDeviceInfo{ProductCode: "Wave Music System III"}, want: true},
		{name: "Onkyo HT-R695", dev: &models.ServiceDeviceInfo{ProductCode: "HT-R695", Name: "Onkyo HT-R695 E9A20F"}, want: false},
		{name: "Dreambox dm920", dev: &models.ServiceDeviceInfo{ProductCode: "dm920", Name: "dm920"}, want: false},
		{name: "LG OLED", dev: &models.ServiceDeviceInfo{ProductCode: "OLED55G2", Name: "[LG] webOS TV"}, want: false},
		{name: "Empty", dev: &models.ServiceDeviceInfo{}, want: false},
		{name: "Nil", dev: nil, want: false},
		{name: "Name only", dev: &models.ServiceDeviceInfo{ProductCode: "", Name: "My SoundTouch 30"}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeSoundTouch(tc.dev); got != tc.want {
				t.Errorf("got %v, want %v (dev=%+v)", got, tc.want, tc.dev)
			}
		})
	}
}

// TestDefaultAccountNonBoseCheck_FlagsNonBoseAndIgnoresBose drives the
// check end-to-end against a temporary datastore seeded with the same
// shape we saw in NorbertBauer's #269 diagnostic bundle: a Dreambox
// and an Onkyo under default, plus an unpaired Bose SoundTouch that
// must NOT trigger the warning.
func TestDefaultAccountNonBoseCheck_FlagsNonBoseAndIgnoresBose(t *testing.T) {
	tmp := t.TempDir()
	ds := datastore.NewDataStore(tmp)

	t.Cleanup(func() { _ = ds.Close() })

	mustWriteDeviceInfo(t, tmp, "default", "192.168.1.10",
		`<?xml version="1.0" encoding="UTF-8"?><info deviceID="192.168.1.10"><name>dm920</name><type>dm920</type><discoveryMethod>SSDP/UPnP</discoveryMethod></info>`)
	mustWriteDeviceInfo(t, tmp, "default", "192.168.1.12",
		`<?xml version="1.0" encoding="UTF-8"?><info deviceID="192.168.1.12"><name>Onkyo HT-R695 E9A20F</name><type>HT-R695</type><discoveryMethod>SSDP/UPnP</discoveryMethod></info>`)
	mustWriteDeviceInfo(t, tmp, "default", "AABBCCDDEEFF",
		`<?xml version="1.0" encoding="UTF-8"?><info deviceID="AABBCCDDEEFF"><name>Bose Living Room</name><type>SoundTouch 30 sm2</type><discoveryMethod>SSDP/UPnP</discoveryMethod></info>`)

	got := runDefaultAccountNonBoseDevicesCheck(ds)

	if len(got) != 2 {
		t.Fatalf("expected 2 findings (Dreambox + Onkyo), got %d: %+v", len(got), got)
	}

	flaggedIDs := map[string]bool{}
	for _, f := range got {
		flaggedIDs[f.Target.Device] = true

		if f.Severity != SeverityWarning {
			t.Errorf("expected SeverityWarning, got %v on %+v", f.Severity, f)
		}

		if len(f.QuickFixes) != 1 || f.QuickFixes[0].ID != FixIDEvictDefaultNonBoseDevice {
			t.Errorf("expected one Evict QuickFix, got %+v", f.QuickFixes)
		}
	}

	if !flaggedIDs["192.168.1.10"] || !flaggedIDs["192.168.1.12"] {
		t.Errorf("expected both Dreambox + Onkyo flagged, got: %v", flaggedIDs)
	}

	if flaggedIDs["AABBCCDDEEFF"] {
		t.Errorf("unpaired Bose SoundTouch must not be flagged; got: %v", flaggedIDs)
	}
}

// mustWriteDeviceInfo writes a DeviceInfo.xml under
// <baseDir>/accounts/<account>/devices/<device>/DeviceInfo.xml.
// Fails the test on any IO error.
func mustWriteDeviceInfo(t *testing.T, baseDir, account, device, body string) {
	t.Helper()

	dir := filepath.Join(baseDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "DeviceInfo.xml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
