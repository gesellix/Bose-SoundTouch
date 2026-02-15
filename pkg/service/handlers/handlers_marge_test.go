package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestMargeSourceProviders(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/marge/streaming/sourceproviders")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "<sourceProviders>") {
		t.Error("Response missing <sourceProviders> tag")
	}
}

func TestMargeSoftwareUpdate(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/marge/updates/soundtouch")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	body, _ := io.ReadAll(res.Body)
	// Should contain software_update or INDEX (if swupdate.xml exists)
	if !strings.Contains(string(body), "software_update") && !strings.Contains(string(body), "INDEX") {
		t.Errorf("Unexpected response: %s", string(body))
	}
}

func TestMargeAccountFull(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "soundcork-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	account := "12345"
	deviceID := "ABCDE"
	accountDir := filepath.Join(tempDir, "accounts", account)

	deviceDir := filepath.Join(accountDir, "devices", deviceID)
	err = os.MkdirAll(deviceDir, 0755)

	if err != nil {
		t.Fatalf("Failed to create device dir: %v", err)
	}

	// Mock DeviceInfo.xml
	if err := os.WriteFile(filepath.Join(deviceDir, "DeviceInfo.xml"), []byte(`
		<info deviceID="ABCDE">
			<name>Test Speaker</name>
			<type>SoundTouch 20</type>
			<moduleType>Series II</moduleType>
			<components>
				<component>
					<componentCategory>SCM</componentCategory>
					<softwareVersion>19.0.5</softwareVersion>
					<serialNumber>SN123</serialNumber>
				</component>
			</components>
			<networkInfo type="SCM">
				<ipAddress>192.168.1.100</ipAddress>
			</networkInfo>
		</info>
	`), 0644); err != nil {
		t.Fatalf("Failed to write DeviceInfo.xml: %v", err)
	}

	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/marge/accounts/" + account + "/full")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "ABCDE") || !strings.Contains(string(body), "Test Speaker") {
		t.Errorf("Response missing expected device data: %s", string(body))
	}
}

func TestMargePresets(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "soundcork-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	account := "12345"
	deviceID := "any"

	accountDir := filepath.Join(tempDir, "accounts", account)
	deviceDir := filepath.Join(accountDir, "devices", deviceID)
	err = os.MkdirAll(deviceDir, 0755)

	if err != nil {
		t.Fatalf("Failed to create device dir: %v", err)
	}

	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Mock Sources.xml and Presets.xml
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(`
		<sources>
			<source id="123" displayName="TUNEIN" secret="" secretType="Audio">
				<sourceKey type="TUNEIN" account=""/>
			</source>
		</sources>
	`), 0644); err != nil {
		t.Fatalf("Failed to write Sources.xml: %v", err)
	}

	if err := os.WriteFile(filepath.Join(deviceDir, "Presets.xml"), []byte(`
		<presets>
			<preset id="1">
				<ContentItem source="TUNEIN" type="station" location="/station/s123" sourceAccount="" isPresetable="true">
					<itemName>Test Station</itemName>
					<containerArt>http://example.com/art.jpg</containerArt>
				</ContentItem>
			</preset>
		</presets>
	`), 0644); err != nil {
		t.Fatalf("Failed to write Presets.xml: %v", err)
	}

	res, err := http.Get(ts.URL + "/marge/accounts/" + account + "/devices/any/presets")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if !strings.Contains(string(body), "Test Station") {
		t.Errorf("Response missing preset data: %s", string(body))
	}
}

func TestMargeUpdatePreset(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "soundcork-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	account := "12345"
	deviceID := "DEV1"

	accountDir := filepath.Join(tempDir, "accounts", account)
	deviceDir := filepath.Join(accountDir, "devices", deviceID)
	err = os.MkdirAll(deviceDir, 0755)

	if err != nil {
		t.Fatalf("Failed to create device dir: %v", err)
	}

	// Mock Sources.xml
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(`
		<sources>
			<source id="SRC1" displayName="TUNEIN" secret="" secretType="Audio">
				<sourceKey type="TUNEIN" account=""/>
			</source>
		</sources>
	`), 0644); err != nil {
		t.Fatalf("Failed to write Sources.xml: %v", err)
	}

	if err := os.WriteFile(filepath.Join(deviceDir, "Presets.xml"), []byte(`<presets></presets>`), 0644); err != nil {
		t.Fatalf("Failed to write Presets.xml: %v", err)
	}

	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	payload := `
		<preset>
			<name>New Preset</name>
			<sourceid>SRC1</sourceid>
			<location>/station/s999</location>
			<contentItemType>station</contentItemType>
			<containerArt>http://example.com/new.jpg</containerArt>
		</preset>`

	res, err := http.Post(ts.URL+"/marge/accounts/"+account+"/devices/"+deviceID+"/presets/1", "application/xml", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
	}

	// Verify file was saved
	presetData, _ := os.ReadFile(filepath.Join(deviceDir, "Presets.xml"))
	if !strings.Contains(string(presetData), "New Preset") {
		t.Error("Preset was not saved to datastore")
	}
}

func TestMargeDeviceInfo(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "soundcork-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	account := "12345"
	deviceID := "DEV1"

	accountDir := filepath.Join(tempDir, "accounts", account)
	deviceDir := filepath.Join(accountDir, "devices", deviceID)
	err = os.MkdirAll(deviceDir, 0755)

	if err != nil {
		t.Fatalf("Failed to create device dir: %v", err)
	}

	// Mock Sources.xml
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(`
		<sources>
			<source id="SRC1" displayName="TUNEIN" secret="" secretType="Audio">
				<sourceKey type="TUNEIN" account=""/>
			</source>
		</sources>
	`), 0644); err != nil {
		t.Fatalf("Failed to write Sources.xml: %v", err)
	}

	if err := os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte(`<recents></recents>`), 0644); err != nil {
		t.Fatalf("Failed to write Recents.xml: %v", err)
	}

	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	payload := `
		<recent>
			<name>Recent Station</name>
			<sourceid>SRC1</sourceid>
			<location>/station/s888</location>
			<contentItemType>station</contentItemType>
		</recent>`

	res, err := http.Post(ts.URL+"/marge/accounts/"+account+"/devices/"+deviceID+"/recents", "application/xml", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	// Verify file was saved
	recentData, _ := os.ReadFile(filepath.Join(deviceDir, "Recents.xml"))
	if !strings.Contains(string(recentData), "Recent Station") {
		t.Error("Recent was not saved to datastore")
	}
}

func TestMargeAddRemoveDevice(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "soundcork-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	account := "12345"

	accountDir := filepath.Join(tempDir, "accounts", account)
	err = os.MkdirAll(accountDir, 0755)

	if err != nil {
		t.Fatalf("Failed to create account dir: %v", err)
	}

	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// 1. Add Device
	payload := `
		<device deviceid="NEWDEV">
			<name>New Speaker</name>
			<type>SoundTouch 10</type>
			<moduleType>Series I</moduleType>
			<components>
				<component>
					<componentCategory>SCM</componentCategory>
					<softwareVersion>1.0.0</softwareVersion>
					<serialNumber>SN_NEW</serialNumber>
				</component>
			</components>
			<networkInfo type="SCM">
				<ipAddress>192.168.1.101</ipAddress>
			</networkInfo>
		</device>`

	res, err := http.Post(ts.URL+"/marge/accounts/"+account+"/devices", "application/xml", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	_ = res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("AddDevice: Expected status OK, got %v", res.Status)
	}

	deviceFile := filepath.Join(accountDir, "devices", "NEWDEV", "DeviceInfo.xml")
	if _, err := os.Stat(deviceFile); os.IsNotExist(err) {
		t.Error("DeviceInfo.xml was not created")
	}

	// 2. Remove Device
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/marge/accounts/"+account+"/devices/NEWDEV", nil)

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	_ = res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("RemoveDevice: Expected status OK, got %v", res.Status)
	}

	if _, err := os.Stat(deviceFile); !os.IsNotExist(err) {
		t.Error("DeviceInfo.xml was not deleted")
	}
}

func TestMargePowerOn(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Post(ts.URL+"/marge/streaming/support/power_on", "application/xml", bytes.NewReader([]byte("<powerOn/>")))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}
}

func TestMargeAdvancedFeatures(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "soundcork-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	t.Run("ProviderSettings", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/streaming/account/123/provider_settings")
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), "<boseId>123</boseId>") {
			t.Errorf("Response body missing account ID: %s", body)
		}
	})

	t.Run("StreamingToken", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/streaming/device/DEV1/streaming_token")
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		token := res.Header.Get("Authorization")
		if !strings.HasPrefix(token, "Bearer soundcork-local-token-") {
			t.Errorf("Invalid token header: %s", token)
		}
	})

	t.Run("CustomerSupport", func(t *testing.T) {
		payload := `<?xml version="1.0" encoding="UTF-8" ?>
		<device-data>
			<device id="587A628A4042">
				<serialnumber>P123</serialnumber>
				<firmware-version>27.0.6</firmware-version>
				<product product_code="SoundTouch 10" type="5">
					<serialnumber>SN123</serialnumber>
				</product>
			</device>
			<diagnostic-data>
				<device-landscape>
					<rssi>Good</rssi>
					<ip-address>192.168.1.100</ip-address>
				</device-landscape>
			</diagnostic-data>
		</device-data>`

		res, err := http.Post(ts.URL+"/marge/streaming/support/customersupport", "application/vnd.bose.streaming-v1.2+xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}

		_ = res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		// Verify event was recorded
		events := ds.GetDeviceEvents("587A628A4042")
		found := false

		for _, e := range events {
			if e.Type == "customer-support-upload" {
				found = true

				if e.Data["firmware"] != "27.0.6" {
					t.Errorf("Expected firmware 27.0.6, got %v", e.Data["firmware"])
				}

				break
			}
		}

		if !found {
			t.Error("Customer support event not found in event log")
		}
	})
}
