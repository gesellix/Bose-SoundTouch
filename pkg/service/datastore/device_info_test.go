package datastore

import (
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestSaveDeviceInfo_MergesName(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)
	account := "1234567"
	device := "001122334455"

	// 1. Initial save with name
	info1 := &models.ServiceDeviceInfo{
		DeviceID:    device,
		AccountID:   account,
		Name:        "Living Room",
		ProductCode: "SoundTouch 20",
	}
	if err := ds.SaveDeviceInfo(account, device, info1); err != nil {
		t.Fatalf("First SaveDeviceInfo failed: %v", err)
	}

	// 2. Verify name is saved
	saved1, err := ds.GetDeviceInfo(account, device)
	if err != nil {
		t.Fatalf("First GetDeviceInfo failed: %v", err)
	}
	if saved1.Name != "Living Room" {
		t.Errorf("Expected name 'Living Room', got '%s'", saved1.Name)
	}

	// 3. Save with empty name (simulating power_on)
	info2 := &models.ServiceDeviceInfo{
		DeviceID:    device,
		AccountID:   account,
		Name:        "",
		ProductCode: "SoundTouch 20",
		IPAddress:   "192.168.1.100",
	}
	if err := ds.SaveDeviceInfo(account, device, info2); err != nil {
		t.Fatalf("Second SaveDeviceInfo failed: %v", err)
	}

	// 4. Verify name is preserved
	saved2, err := ds.GetDeviceInfo(account, device)
	if err != nil {
		t.Fatalf("Second GetDeviceInfo failed: %v", err)
	}
	if saved2.Name != "Living Room" {
		t.Errorf("Expected name 'Living Room' to be preserved, but got '%s'", saved2.Name)
	}
	if saved2.IPAddress != "192.168.1.100" {
		t.Errorf("Expected IPAddress '192.168.1.100', got '%s'", saved2.IPAddress)
	}
}
