package datastore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetConfiguredSourcesRepairsNumericSourceKeyType covers the migration
// half of issue 697: Sources.xml files written before the AddSource fix hold
// a media server under type "7", its provider ID. Reading them must yield
// "STORED_MUSIC", so presets match the source again without the owner having
// to re-register anything. Types that only look similar stay untouched.
func TestGetConfiguredSourcesRepairsNumericSourceKeyType(t *testing.T) {
	const (
		account = "0000000"
		device  = "DEVICEID01"
	)

	ds := NewDataStore(t.TempDir())

	dir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	legacy := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sources>
  <source id="SRC_media" type="Audio" sourceproviderid="7">
    <sourceKey type="7" account="00000000-0000-0000-0000-0000000000aa/0"/>
  </source>
  <source id="SRC_unknown" type="Audio" sourceproviderid="999">
    <sourceKey type="999" account="someone"/>
  </source>
  <source id="SRC_mismatch" type="Audio" sourceproviderid="25">
    <sourceKey type="7" account="other"/>
  </source>
</sources>`

	if err := os.WriteFile(filepath.Join(dir, "Sources.xml"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write Sources.xml: %v", err)
	}

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("get sources: %v", err)
	}

	want := map[string]string{
		"SRC_media":    "STORED_MUSIC",
		"SRC_unknown":  "999",
		"SRC_mismatch": "7",
	}

	got := map[string]string{}
	for _, s := range sources {
		if _, ok := want[s.ID]; ok {
			got[s.ID] = s.SourceKey.Type

			if s.SourceKeyType != s.SourceKey.Type {
				t.Errorf("%s: legacy SourceKeyType %q differs from SourceKey.Type %q", s.ID, s.SourceKeyType, s.SourceKey.Type)
			}
		}
	}

	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: source key type = %q, want %q", id, got[id], w)
		}
	}
}
