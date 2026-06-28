package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestApplyPersistedSettings(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "main-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ds := datastore.NewDataStore(tmpDir)

	t.Run("overrides true with false", func(t *testing.T) {
		config := &serviceConfig{
			redact:  true,
			logBody: true,
			record:  true,
		}

		// Simulate the bug by using the old bitwise OR logic in the test,
		// which should fail if we expect false.
		// config.redact = config.redact || false -> stays true

		settings := datastore.Settings{
			RedactLogs:         false,
			LogBodies:          false,
			RecordInteractions: false,
		}
		err := ds.SaveSettings(settings)
		if err != nil {
			t.Fatalf("Failed to save settings: %v", err)
		}

		applyPersistedSettings(ds, config)

		if config.redact != false {
			t.Errorf("Expected redact to be false, got true")
		}
		if config.logBody != false {
			t.Errorf("Expected logBody to be false, got true")
		}
		if config.record != false {
			t.Errorf("Expected record to be false, got true")
		}
	})

	t.Run("retains false when settings are false", func(t *testing.T) {
		settings := datastore.Settings{
			RedactLogs: false,
		}
		err := ds.SaveSettings(settings)
		if err != nil {
			t.Fatalf("Failed to save settings: %v", err)
		}

		config := &serviceConfig{
			redact: false,
		}

		applyPersistedSettings(ds, config)

		if config.redact != false {
			t.Errorf("Expected redact to be false, got true")
		}
	})

	t.Run("overrides false with true", func(t *testing.T) {
		settings := datastore.Settings{
			RedactLogs: true,
		}
		err := ds.SaveSettings(settings)
		if err != nil {
			t.Fatalf("Failed to save settings: %v", err)
		}

		config := &serviceConfig{
			redact: false,
		}

		applyPersistedSettings(ds, config)

		if config.redact != true {
			t.Errorf("Expected redact to be true, got false")
		}
	})
}

func TestMergeTLSExtraHosts(t *testing.T) {
	cases := []struct {
		name      string
		cli       []string
		persisted []string
		want      []string
	}{
		{
			name:      "CLI only",
			cli:       []string{"a.example"},
			persisted: nil,
			want:      []string{"a.example"},
		},
		{
			name:      "Persisted only",
			cli:       nil,
			persisted: []string{"b.example"},
			want:      []string{"b.example"},
		},
		{
			name:      "CLI wins ordering, persisted appended",
			cli:       []string{"a.example"},
			persisted: []string{"b.example"},
			want:      []string{"a.example", "b.example"},
		},
		{
			name:      "Dedupes overlap",
			cli:       []string{"a.example", "b.example"},
			persisted: []string{"b.example", "c.example"},
			want:      []string{"a.example", "b.example", "c.example"},
		},
		{
			name:      "Drops empty + whitespace",
			cli:       []string{"  ", "a.example", ""},
			persisted: []string{"", "  b.example  "},
			want:      []string{"a.example", "b.example"},
		},
		{
			name:      "Both empty",
			cli:       nil,
			persisted: nil,
			want:      []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeTLSExtraHosts(tc.cli, tc.persisted)
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch: got %v, want %v", got, tc.want)
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %q, want %q (full: %v vs %v)", i, got[i], tc.want[i], got, tc.want)
				}
			}
		})
	}
}

func TestGetDomains_IncludesOAuthDerivation(t *testing.T) {
	// Hostname-based serverURL: the derived OAuth variant must end up
	// in the served TLS cert SAN list, otherwise the speaker rejects
	// the TLS handshake on Spotify / Amazon Music token refresh.
	got := getDomains("http://mac.fritz.box:8000", "https://mac.fritz.box:8443", "mac.fritz.box", nil)

	want := "macoauth.fritz.box"
	if !contains(got, want) {
		t.Errorf("expected SAN list to include %q (derived from serverURL), got: %v", want, got)
	}
}

func TestGetDomains_IPServerURLProducesNoOAuthDerivation(t *testing.T) {
	// IP-based serverURL deliberately yields no derivation (the speaker's
	// `<first-label>oauth.<rest>` construction would be malformed for an
	// IP and no DNS resolver can answer for it). The cert SAN list must
	// not pretend to cover something that can never be queried.
	got := getDomains("http://192.168.0.30:8000", "https://192.168.0.30:8443", "192.168.0.30", nil)

	for _, h := range got {
		if h == "192oauth.168.0.30" {
			t.Errorf("SAN list must not include malformed IP-derived OAuth name, got: %v", got)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}

	return false
}

func TestSettingsFileExists(t *testing.T) {
	dir := t.TempDir()

	if settingsFileExists(dir) {
		t.Fatal("expected false for a dir without settings.json")
	}

	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	if !settingsFileExists(dir) {
		t.Fatal("expected true once settings.json is present")
	}

	if settingsFileExists("") {
		t.Fatal("expected false for an empty data dir")
	}
}

// applyFirstRunSeed mirrors the startup gate in the CLI Action: a default
// settings.json is written only when none exists yet, so a hand-authored file
// is never clobbered.
func applyFirstRunSeed(ds *datastore.DataStore, config *serviceConfig) {
	existed := settingsFileExists(config.dataDir)

	applyPersistedSettings(ds, config)

	if !existed {
		createDefaultSettings(ds, *config)
	}
}

func TestFirstRunSeed_PreservesHandAuthoredSettings(t *testing.T) {
	dir := t.TempDir()

	// Operator pre-seeds proxy trust but leaves server_url to the --server-url
	// flag. Before the fix this was treated as "first run" and overwritten.
	if err := os.WriteFile(filepath.Join(dir, "settings.json"),
		[]byte(`{"trust_forwarded_headers":true,"trusted_proxy_cidrs":["10.0.0.0/8"]}`), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	ds := datastore.NewDataStore(dir)
	config := &serviceConfig{dataDir: dir, serverURL: "http://192.0.2.1:8000"}

	applyFirstRunSeed(ds, config)

	got, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}

	if !got.TrustForwardedHeaders {
		t.Error("trust_forwarded_headers was clobbered on startup")
	}

	if len(got.TrustedProxyCIDRs) != 1 || got.TrustedProxyCIDRs[0] != "10.0.0.0/8" {
		t.Errorf("trusted_proxy_cidrs was clobbered, got %v", got.TrustedProxyCIDRs)
	}
}

func TestFirstRunSeed_WritesDefaultsWhenAbsent(t *testing.T) {
	dir := t.TempDir()

	ds := datastore.NewDataStore(dir)
	config := &serviceConfig{dataDir: dir, serverURL: "http://192.0.2.1:8000"}

	applyFirstRunSeed(ds, config)

	got, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}

	if got.ServerURL != "http://192.0.2.1:8000" {
		t.Errorf("expected defaults to be written with server_url, got %q", got.ServerURL)
	}
}
