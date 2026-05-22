package main

import (
	"os"
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
