package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/go-chi/chi/v5"
)

func TestHandleMgmtAccountDetails_Recents(t *testing.T) {
	tempBaseDir := "mgmt_test_data"
	err := os.MkdirAll(tempBaseDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBaseDir)

	ds := datastore.NewDataStore(tempBaseDir)
	err = ds.Initialize()
	if err != nil {
		t.Fatal(err)
	}

	accountID := "1234567"
	deviceID := "001122334455"

	// Setup a device with a recent item that has utcTime and name in ContentItem
	deviceDir := ds.AccountDeviceDir(accountID, deviceID)
	err = os.MkdirAll(deviceDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	recentsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<recents>
    <recent id="2538285498" utcTime="1690000000">
        <contentItem source="INTERNET_RADIO" type="stationurl">
            <itemName>For Your Darkest Days</itemName>
        </contentItem>
    </recent>
</recents>`
	err = os.WriteFile(deviceDir+"/Recents.xml", []byte(recentsXML), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Also need a device info file to be listed
	deviceInfo := models.ServiceDeviceInfo{
		AccountID: accountID,
		DeviceID:  deviceID,
		Name:      "Test Device",
	}
	err = ds.SaveDeviceInfo(accountID, deviceID, &deviceInfo)
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{ds: ds}

	r := chi.NewRouter()
	r.Get("/mgmt/accounts/{accountId}", server.HandleMgmtAccountDetails)

	req := httptest.NewRequest("GET", "/mgmt/accounts/1234567", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response struct {
		Devices []struct {
			Recents []models.FullResponseRecent `json:"recents"`
		} `json:"devices"`
	}

	err = json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.Devices) == 0 {
		t.Fatal("Expected at least one device")
	}

	recents := response.Devices[0].Recents
	if len(recents) == 0 {
		t.Fatal("Expected one recent item")
	}

	r0 := recents[0]
	if r0.Name != "For Your Darkest Days" {
		t.Errorf("Expected recent name 'For Your Darkest Days', got '%s'", r0.Name)
	}

	if r0.CreatedOn != "1690000000" {
		t.Errorf("Expected recent created_on '1690000000' (from utcTime), got '%s'", r0.CreatedOn)
	}

	// Test Preset mapping as well
	presetsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<presets>
    <preset id="1" createdOn="1690000001">
        <ContentItem source="SPOTIFY" type="tracklisturl" sourceAccount="test-user">
            <itemName>test-playlist</itemName>
        </ContentItem>
    </preset>
</presets>`
	err = os.WriteFile(deviceDir+"/Presets.xml", []byte(presetsXML), 0644)
	if err != nil {
		t.Fatal(err)
	}

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)

	var response2 struct {
		Devices []struct {
			Presets []models.FullResponsePreset `json:"presets"`
		} `json:"devices"`
	}
	err = json.Unmarshal(w2.Body.Bytes(), &response2)
	if err != nil {
		t.Fatal(err)
	}

	if len(response2.Devices[0].Presets) == 0 {
		t.Fatal("Expected one preset")
	}
	p0 := response2.Devices[0].Presets[0]
	if p0.Name != "test-playlist" {
		t.Errorf("Expected preset name 'test-playlist', got '%s'", p0.Name)
	}
	if p0.CreatedOn != "1690000001" {
		t.Errorf("Expected preset created_on '1690000001', got '%s'", p0.CreatedOn)
	}

	// Verify ButtonNumber/ID handling
	if p0.ButtonNumber != "1" {
		t.Errorf("Expected button_number '1', got '%s'", p0.ButtonNumber)
	}
}
