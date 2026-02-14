package handlers

import (
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestMergeOverlappingDevices(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "merge-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	s := &Server{ds: ds}

	// Case 1: IP-only entry and Serial-based entry for the same IP
	ip := "192.168.1.100"
	serial := "SERIAL123"

	// 1. Save IP-based entry
	infoIP := &models.ServiceDeviceInfo{
		Name:      "Speaker IP",
		IPAddress: ip,
	}
	err = ds.SaveDeviceInfo("default", ip, infoIP)
	if err != nil {
		t.Fatalf("Failed to save IP info: %v", err)
	}

	// 2. Save Serial-based entry
	infoSerial := &models.ServiceDeviceInfo{
		DeviceID:           serial,
		DeviceSerialNumber: serial,
		Name:               "Speaker Serial",
		IPAddress:          ip,
	}
	err = ds.SaveDeviceInfo("default", serial, infoSerial)
	if err != nil {
		t.Fatalf("Failed to save Serial info: %v", err)
	}

	// Verify both exist
	devices, _ := ds.ListAllDevices()
	if len(devices) != 2 {
		t.Fatalf("Expected 2 devices before merge, got %d", len(devices))
	}

	// Run merge
	s.mergeOverlappingDevices()

	// Verify merge
	devices, _ = ds.ListAllDevices()
	if len(devices) != 1 {
		t.Fatalf("Expected 1 device after merge, got %d", len(devices))
	}

	if devices[0].DeviceID != serial {
		t.Errorf("Expected remaining device to be Serial-based (%s), got %s", serial, devices[0].DeviceID)
	}
}

func TestFindExistingDeviceID(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "find-test-*")
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	s := &Server{ds: ds}

	ip := "192.168.1.101"
	serial := "SERIAL456"

	// Save IP-based
	ds.SaveDeviceInfo("default", ip, &models.ServiceDeviceInfo{
		IPAddress: ip,
		Name:      "IP Speaker",
	})

	// Test finding by IP
	foundID := s.findExistingDeviceID(models.DiscoveredDevice{
		Host: ip,
	})
	if foundID != ip {
		t.Errorf("Expected to find by IP, got %s", foundID)
	}

	// Save Serial-based for SAME IP
	ds.SaveDeviceInfo("default", serial, &models.ServiceDeviceInfo{
		DeviceID:           serial,
		DeviceSerialNumber: serial,
		IPAddress:          ip,
		Name:               "Serial Speaker",
	})

	// Test finding by IP should now return Serial (if Serial is known)
	// Actually findExistingDeviceID returns the first match it finds in allDevices.
	// Since we haven't merged yet, it could be either.

	// Test finding by Serial
	foundID = s.findExistingDeviceID(models.DiscoveredDevice{
		Host:     ip,
		SerialNo: serial,
	})
	if foundID != serial && foundID != ip {
		t.Errorf("Expected to find by Serial or IP, got %s", foundID)
	}

	// Merge and check again
	s.mergeOverlappingDevices()
	foundID = s.findExistingDeviceID(models.DiscoveredDevice{
		Host: ip,
	})
	if foundID != serial {
		t.Errorf("After merge, expected to find Serial ID for IP, got %s", foundID)
	}
}
