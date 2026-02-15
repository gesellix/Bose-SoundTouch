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
			redact:               true,
			logBody:              true,
			record:               true,
			enableSoundcorkProxy: true,
		}

		// Simulate the bug by using the old bitwise OR logic in the test,
		// which should fail if we expect false.
		// config.redact = config.redact || false -> stays true

		settings := datastore.Settings{
			RedactLogs:           false,
			LogBodies:            false,
			RecordInteractions:   false,
			EnableSoundcorkProxy: false,
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
		if config.enableSoundcorkProxy != false {
			t.Errorf("Expected enableSoundcorkProxy to be false, got true")
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
