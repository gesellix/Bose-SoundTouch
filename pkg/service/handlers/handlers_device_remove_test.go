package handlers

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestRemoveDeviceByID verifies the extracted removal helper deletes the
// device from the datastore, fires the devices-changed hook (so the
// embedded web UI re-syncs), and reports not-found without firing the
// hook when no matching device exists.
func TestRemoveDeviceByID(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	server := NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)

	const (
		account  = "1000001"
		deviceID = "DEVICEID01"
	)

	if err := ds.SaveDeviceInfo(account, deviceID, &models.ServiceDeviceInfo{
		DeviceID:  deviceID,
		Name:      "Test Speaker",
		IPAddress: "192.0.2.10",
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	var hookFired int

	server.SetDevicesChangedHook(func() { hookFired++ })

	found, err := server.RemoveDeviceByID(deviceID)
	if err != nil {
		t.Fatalf("RemoveDeviceByID: %v", err)
	}

	if !found {
		t.Fatal("RemoveDeviceByID reported the device as not found")
	}

	if hookFired != 1 {
		t.Errorf("devices-changed hook fired %d times; want 1", hookFired)
	}

	devices, err := ds.ListAllDevices()
	if err != nil {
		t.Fatalf("ListAllDevices: %v", err)
	}

	for i := range devices {
		if devices[i].DeviceID == deviceID {
			t.Error("device still present in datastore after removal")
		}
	}

	// A second removal finds nothing and must not fire the hook again.
	found, err = server.RemoveDeviceByID(deviceID)
	if err != nil {
		t.Fatalf("RemoveDeviceByID (second call): %v", err)
	}

	if found {
		t.Error("RemoveDeviceByID reported a removed device as found")
	}

	if hookFired != 1 {
		t.Errorf("devices-changed hook fired %d times after no-op removal; want 1", hookFired)
	}
}
