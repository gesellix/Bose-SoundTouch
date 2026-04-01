package marge

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestRaceConditionFullSync(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "soundtouch-test-race")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	accountID := "test-account"
	deviceID := "test-device"

	// Initial data
	initialInfo := &models.ServiceDeviceInfo{
		DeviceID:    deviceID,
		AccountID:   accountID,
		Name:        "Initial Name",
		ProductCode: "SoundTouch 10",
	}
	if err := ds.SaveDeviceInfo(accountID, deviceID, initialInfo); err != nil {
		t.Fatalf("Failed to save initial info: %v", err)
	}

	// Wait for disk sync/OS to stabilize the initial file if needed
	time.Sleep(100 * time.Millisecond)

	// We'll run a loop where one goroutine reads and another writes
	// and check if we ever get an empty name.

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	var emptyNameFound bool
	var mu sync.Mutex

	// Reader goroutine
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				xmlData, err := AccountFullToXML(ds, accountID)
				if err != nil {
					// It's possible to get "file not found" or "permission denied" or "empty file" if we hit the middle of a write
					// but here we are most interested in getting an incomplete response
					continue
				}

				if contains(string(xmlData), "<name/>") || contains(string(xmlData), "<name></name>") {
					mu.Lock()
					emptyNameFound = true
					mu.Unlock()
					t.Logf("RaceConditionFullSync: Found empty <name/> or <name></name> in XML: %s\n", string(xmlData))
					return
				}
				if !contains(string(xmlData), "<name>") && !contains(string(xmlData), "<name/>") {
					mu.Lock()
					emptyNameFound = true
					mu.Unlock()
					t.Logf("RaceConditionFullSync: Name tag COMPLETELY MISSING in XML: %s\n", string(xmlData))
					return
				}
			}
		}
	}()

	// Writer goroutine
	go func() {
		defer wg.Done()
		info := &models.ServiceDeviceInfo{
			DeviceID:    deviceID,
			AccountID:   accountID,
			Name:        "Updated Name",
			ProductCode: "SoundTouch 10",
		}
		for {
			select {
			case <-stop:
				return
			default:
				_ = ds.SaveDeviceInfo(accountID, deviceID, info)
			}
		}
	}()

	// Run for a short time
	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()

	if emptyNameFound {
		t.Errorf("Race condition detected: found empty name in /full response during concurrent write")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
