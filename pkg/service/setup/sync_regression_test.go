package setup

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestSyncPresets_PreservesID(t *testing.T) {
	// 1. Setup mock SoundTouch device (HTTP server)
	mockDevice := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/presets":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8" ?>
<presets>
    <preset id="1" createdOn="1719128436" updatedOn="1728740382">
        <ContentItem source="SPOTIFY" type="tracklisturl" location="/playback/container/c3BvdGlmeTpwbGF5bGlzdDo1Mm5QaVJrbWVmSkZPeHh1M1ZTd1hh" sourceAccount="test-user" isPresetable="true">
            <itemName>test-playlist</itemName>
            <containerArt>https://i.scdn.co/image/ab67616d00001e025ff75c5d082fc50a3a74ad7b</containerArt>
        </ContentItem>
    </preset>
    <preset id="6" createdOn="1585502139" updatedOn="1769977469">
        <ContentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s213886" isPresetable="true">
            <itemName>WDR 2 Rheinland</itemName>
            <containerArt>https://cdn-radiotime-logos.tunein.com/s213886g.png</containerArt>
        </ContentItem>
    </preset>
</presets>`)
		case "/info":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<nowPlaying deviceID="001122334455" source="STANDBY" />`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockDevice.Close()

	// 2. Setup DataStore
	tempDir, err := os.MkdirTemp("", "st-sync-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	ds := datastore.NewDataStore(tempDir)

	// 3. Setup Manager
	m := NewManager("http://localhost:8080", ds, nil)
	// Since HTTPGet is private, we'll rely on NewManager setting it to http.Get,
	// which will work fine with our httptest server.

	// 4. Run Sync
	deviceIP := mockDevice.Listener.Addr().String()
	accountID := "1234567"
	deviceID := "001122334455"

	m.syncPresets(deviceIP, accountID, deviceID)

	// 5. Verify the saved file
	presetFile := filepath.Join(tempDir, "accounts", accountID, "devices", deviceID, "Presets.xml")
	data, err := os.ReadFile(presetFile)
	if err != nil {
		t.Fatalf("Failed to read saved presets file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `id="1"`) {
		t.Errorf("Saved XML missing id=\"1\":\n%s", content)
	}
	if !strings.Contains(content, `id="6"`) {
		t.Errorf("Saved XML missing id=\"6\":\n%s", content)
	}
}

func TestSyncPresets_PreservesEmptySourceAccount(t *testing.T) {
	// 1. Setup mock SoundTouch device (HTTP server)
	mockDevice := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/presets" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8" ?>
<presets>
    <preset id="6" createdOn="1585502139" updatedOn="1769977469">
        <ContentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s213886" sourceAccount="" isPresetable="true">
            <itemName>WDR 2 Rheinland</itemName>
            <containerArt>https://cdn-radiotime-logos.tunein.com/s213886g.png</containerArt>
        </ContentItem>
    </preset>
</presets>`)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockDevice.Close()

	// 2. Setup DataStore
	tempDir, err := os.MkdirTemp("", "st-sync-test-sourceaccount-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	ds := datastore.NewDataStore(tempDir)

	// 3. Setup Manager
	m := NewManager("http://localhost:8080", ds, nil)

	// 4. Run Sync
	deviceIP := mockDevice.Listener.Addr().String()
	accountID := "1234567"
	deviceID := "001122334455"

	m.syncPresets(deviceIP, accountID, deviceID)

	// 5. Verify the saved file
	presetFile := filepath.Join(tempDir, "accounts", accountID, "devices", deviceID, "Presets.xml")
	data, err := os.ReadFile(presetFile)
	if err != nil {
		t.Fatalf("Failed to read saved presets file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `sourceAccount=""`) {
		t.Errorf("Saved XML missing sourceAccount=\"\":\n%s", content)
	}
}

func TestSyncRecents_PreservesID(t *testing.T) {
	// 1. Setup mock SoundTouch device (HTTP server)
	mockDevice := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/recents":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8" ?>
<recents>
    <recent deviceID="001122334455" utcTime="1719128436" id="101">
        <contentItem source="SPOTIFY" type="tracklisturl" location="/playback/container/c3BvdGlmeTpwbGF5bGlzdDo1Mm5QaVJrbWVmSkZPeHh1M1ZTd1hh" sourceAccount="test-user" isPresetable="true">
            <itemName>test-playlist</itemName>
            <containerArt>https://i.scdn.co/image/ab67616d00001e025ff75c5d082fc50a3a74ad7b</containerArt>
        </contentItem>
    </recent>
</recents>`)
		case "/info":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<nowPlaying deviceID="001122334455" source="STANDBY" />`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockDevice.Close()

	// 2. Setup DataStore
	tempDir, err := os.MkdirTemp("", "st-sync-recents-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	ds := datastore.NewDataStore(tempDir)

	// 3. Setup Manager
	m := NewManager("http://localhost:8080", ds, nil)

	// 4. Run Sync
	deviceIP := mockDevice.Listener.Addr().String()
	accountID := "1234567"
	deviceID := "001122334455"

	m.syncRecents(deviceIP, accountID, deviceID)

	// 5. Verify the saved file
	recentFile := filepath.Join(tempDir, "accounts", accountID, "devices", deviceID, "Recents.xml")
	data, err := os.ReadFile(recentFile)
	if err != nil {
		t.Fatalf("Failed to read saved recents file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `id="101"`) {
		t.Errorf("Saved XML missing id=\"101\":\n%s", content)
	}
}

func TestSyncRecents_PreservesEmptySourceAccount(t *testing.T) {
	// 1. Setup mock SoundTouch device (HTTP server)
	mockDevice := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recents" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8" ?>
<recents>
    <recent deviceID="001122334455" utcTime="1719128436" id="101">
        <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s213886" sourceAccount="" isPresetable="true">
            <itemName>WDR 2 Rheinland</itemName>
            <containerArt>https://cdn-radiotime-logos.tunein.com/s213886g.png</containerArt>
        </contentItem>
    </recent>
</recents>`)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockDevice.Close()

	// 2. Setup DataStore
	tempDir, err := os.MkdirTemp("", "st-sync-recents-test-sourceaccount-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	ds := datastore.NewDataStore(tempDir)

	// 3. Setup Manager
	m := NewManager("http://localhost:8080", ds, nil)

	// 4. Run Sync
	deviceIP := mockDevice.Listener.Addr().String()
	accountID := "1234567"
	deviceID := "001122334455"

	m.syncRecents(deviceIP, accountID, deviceID)

	// 5. Verify the saved file
	recentFile := filepath.Join(tempDir, "accounts", accountID, "devices", deviceID, "Recents.xml")
	data, err := os.ReadFile(recentFile)
	if err != nil {
		t.Fatalf("Failed to read saved recents file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `sourceAccount=""`) {
		t.Errorf("Saved XML missing sourceAccount=\"\":\n%s", content)
	}
}

func TestSyncSources_Format(t *testing.T) {
	// 1. Setup mock SoundTouch device (HTTP server)
	mockDevice := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8" ?>
<info deviceID="001122334455">
    <name>Test Device</name>
    <type>SoundTouch 10</type>
    <margeAccountUUID>1234567</margeAccountUUID>
</info>`)
		case "/sources":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8" ?>
<sources deviceID="001122334455">
    <sourceItem source="AUX" status="READY" isLocal="true" multiroomallowed="true">AUX IN</sourceItem>
    <sourceItem source="SPOTIFY" sourceAccount="test-user" status="READY" isLocal="false" multiroomallowed="true">test-user</sourceItem>
</sources>`)
		case "/presets", "/recents":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<root></root>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockDevice.Close()

	// 2. Setup DataStore
	tempDir, err := os.MkdirTemp("", "st-sync-sources-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	ds := datastore.NewDataStore(tempDir)

	// 3. Setup Manager
	m := NewManager("http://localhost:8080", ds, nil)

	// 4. Run Sync
	deviceIP := mockDevice.Listener.Addr().String()
	accountID := "1234567"
	deviceID := "001122334455"
	err = m.SyncDeviceData(deviceIP)
	if err != nil {
		t.Fatalf("SyncDeviceData failed: %v", err)
	}

	// 5. Verify the saved file
	sourceFile := filepath.Join(tempDir, "accounts", accountID, "devices", deviceID, "Sources.xml")
	data, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatalf("Failed to read saved sources file at %s: %v", sourceFile, err)
	}

	content := string(data)
	if !strings.Contains(content, `<source displayName="AUX IN" secret="" secretType="token">`) {
		t.Errorf("Saved XML missing or incorrect AUX source:\n%s", content)
	}
	if !strings.Contains(content, `<source displayName="test-user" secret="" secretType="token_version_3">`) {
		t.Errorf("Saved XML missing or incorrect Spotify source:\n%s", content)
	}
	if strings.Contains(content, "<sourcename>") {
		t.Errorf("Saved XML contains legacy tags:\n%s", content)
	}
}
