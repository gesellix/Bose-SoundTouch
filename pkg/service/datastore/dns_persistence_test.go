package datastore

import (
	"os"
	"testing"
	"time"
)

func TestDNSDiscoveryPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-dns-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)

	now := time.Now().Round(time.Second)
	discoveries := []DNSDiscoveryEntry{
		{
			Hostname:      "api.bose.com",
			FirstSeen:     now.Add(-1 * time.Hour),
			LastSeen:      now,
			QueryCount:    10,
			IsBoseService: true,
			IsIntercepted: true,
			RemoteAddr:    "192.168.1.100",
		},
		{
			Hostname:      "google.com",
			FirstSeen:     now.Add(-2 * time.Hour),
			LastSeen:      now.Add(-1 * time.Hour),
			QueryCount:    5,
			IsBoseService: false,
			IsIntercepted: false,
			RemoteAddr:    "192.168.1.101",
		},
	}

	// Test Save
	err = ds.SaveDNSDiscoveries(discoveries)
	if err != nil {
		t.Fatalf("SaveDNSDiscoveries failed: %v", err)
	}

	// Test Load
	loaded, err := ds.LoadDNSDiscoveries()
	if err != nil {
		t.Fatalf("LoadDNSDiscoveries failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("Expected 2 discoveries, got %d", len(loaded))
	}

	// Check if sorted by LastSeen (SaveDNSDiscoveries sorts them)
	if loaded[0].Hostname != "api.bose.com" {
		t.Errorf("Expected api.bose.com to be first, got %s", loaded[0].Hostname)
	}

	// Test Clear
	err = ds.ClearDNSDiscoveries()
	if err != nil {
		t.Fatalf("ClearDNSDiscoveries failed: %v", err)
	}

	loadedAfterClear, err := ds.LoadDNSDiscoveries()
	if err != nil {
		t.Fatalf("LoadDNSDiscoveries after clear failed: %v", err)
	}

	if len(loadedAfterClear) != 0 {
		t.Errorf("Expected 0 discoveries after clear, got %d", len(loadedAfterClear))
	}
}
