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
	// Should contain INDEX as we updated swupdate.xml
	if !strings.Contains(string(body), "INDEX") {
		t.Errorf("Unexpected response: %s", string(body))
	}
	if !strings.Contains(string(body), "0x0933") {
		t.Errorf("Response missing VideoWave (0x0933) info: %s", string(body))
	}
}

func TestMargeAccountFull(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
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
	tempDir, err := os.MkdirTemp("", "st-test-*")
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
	tempDir, err := os.MkdirTemp("", "st-test-*")
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

func TestMargeAddRecentRoute(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
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

	if res.StatusCode != http.StatusCreated {
		t.Errorf("Expected status Created, got %v", res.Status)
	}

	// Verify file was saved
	recentData, _ := os.ReadFile(filepath.Join(deviceDir, "Recents.xml"))
	if !strings.Contains(string(recentData), "Recent Station") {
		t.Error("Recent was not saved to datastore")
	}
}

func TestMargeNativeStreamingRoutes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-native-*")
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

	// Mock Sources.xml for recent tests
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

	if err := os.WriteFile(filepath.Join(deviceDir, "Presets.xml"), []byte(`<presets></presets>`), 0644); err != nil {
		t.Fatalf("Failed to write Presets.xml: %v", err)
	}

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	t.Run("POST /streaming/account/{account}/device/{device}/recent", func(t *testing.T) {
		payload := `
			<recent>
				<name>New Route Recent</name>
				<sourceid>SRC1</sourceid>
				<location>/station/s999</location>
				<contentItemType>station</contentItemType>
			</recent>`

		res, err := http.Post(ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/recent", "application/xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status Created, got %v: %s", res.Status, string(body))
		}

		if ct := res.Header.Get("Content-Type"); ct != "application/vnd.bose.streaming-v1.2+xml" {
			t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.2+xml, got %v", ct)
		}

		// Verify file was saved
		recentData, _ := os.ReadFile(filepath.Join(deviceDir, "Recents.xml"))
		if !strings.Contains(string(recentData), "New Route Recent") {
			t.Error("Recent from native route was not saved to datastore")
		}
	})

	t.Run("GET /streaming/account/{account}/full", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/streaming/account/" + account + "/full")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
		}

		if ct := res.Header.Get("Content-Type"); ct != "application/vnd.bose.streaming-v1.2+xml" {
			t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.2+xml, got %v", ct)
		}

		fullData, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(fullData), account) {
			t.Error("Account full response does not contain account ID")
		}
	})

	t.Run("GET /streaming/software/update/account/{account}", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/streaming/software/update/account/" + account)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
		}

		if ct := res.Header.Get("Content-Type"); ct != "application/vnd.bose.streaming-v1.2+xml" {
			t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.2+xml, got %v", ct)
		}

		swData, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(swData), "software_update") {
			t.Errorf("Response missing software_update tag: %s", string(swData))
		}
	})

	t.Run("GET /streaming/account/{account}/device/{device}/recent", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/streaming/account/" + account + "/device/" + deviceID + "/recent")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
		}

		if ct := res.Header.Get("Content-Type"); ct != "application/vnd.bose.streaming-v1.2+xml" {
			t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.2+xml, got %v", ct)
		}

		etag := res.Header.Get("ETag")
		if etag == "" {
			t.Error("Expected ETag header")
		}

		recentData, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(recentData), "recents") {
			t.Errorf("Response missing recents tag: %s", string(recentData))
		}

		// Test 304
		req, _ := http.NewRequest("GET", ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/recent", nil)
		req.Header.Set("If-None-Match", etag)
		res2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res2.Body.Close()
		if res2.StatusCode != http.StatusNotModified {
			t.Errorf("Expected 304 Not Modified, got %v", res2.Status)
		}
	})

	t.Run("GET /streaming/account/{account}/device/{device}/presets", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/streaming/account/" + account + "/device/" + deviceID + "/presets")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
		}

		if ct := res.Header.Get("Content-Type"); ct != "application/vnd.bose.streaming-v1.2+xml" {
			t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.2+xml, got %v", ct)
		}

		etag := res.Header.Get("ETag")
		if etag == "" {
			t.Error("Expected ETag header")
		}

		presetData, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(presetData), "presets") {
			t.Errorf("Response missing presets tag: %s", string(presetData))
		}

		// Test 304
		req, _ := http.NewRequest("GET", ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/presets", nil)
		req.Header.Set("If-None-Match", etag)
		res2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res2.Body.Close()
		if res2.StatusCode != http.StatusNotModified {
			t.Errorf("Expected 304 Not Modified, got %v", res2.Status)
		}
	})

	t.Run("POST /streaming/account/{account}/device/{device}/presets/{presetNumber}", func(t *testing.T) {
		payload := `
			<preset>
				<name>New Native Preset</name>
				<sourceid>SRC1</sourceid>
				<location>/station/s777</location>
				<contentItemType>station</contentItemType>
			</preset>`

		res, err := http.Post(ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/presets/1", "application/xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
		}

		if ct := res.Header.Get("Content-Type"); ct != "application/vnd.bose.streaming-v1.2+xml" {
			t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.2+xml, got %v", ct)
		}

		// Verify file was saved
		presetData, _ := os.ReadFile(filepath.Join(deviceDir, "Presets.xml"))
		if !strings.Contains(string(presetData), "New Native Preset") {
			t.Error("Preset from native route was not saved to datastore")
		}
	})

	t.Run("GET /streaming/account/{account}/device/{device}/group/", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/streaming/account/" + account + "/device/" + deviceID + "/group/")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
		}

		groupData, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(groupData), "<group") {
			t.Errorf("Response missing group tag: %s", string(groupData))
		}
	})

	t.Run("GET /streaming/account/{account}/device/{device}/group/server", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/streaming/account/" + account + "/device/" + deviceID + "/group/server")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %v", res.Status)
		}
	})

	t.Run("GET /streaming/account/{account}/device/{device}/group/member", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/streaming/account/" + account + "/device/" + deviceID + "/group/member")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404 Not Found, got %v", res.Status)
		}
	})

	t.Run("GET /marge/accounts/{account}/devices/{device}/group", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/accounts/" + account + "/devices/" + deviceID + "/group")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
		}

		if ct := res.Header.Get("Content-Type"); ct != "application/vnd.bose.streaming-v1.2+xml" {
			t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.2+xml, got %v", ct)
		}

		groupData, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(groupData), "<group") {
			t.Errorf("Response missing group tag: %s", string(groupData))
		}
	})
}

func TestMargeAddRemoveDevice(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
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

	t.Run("EmptyBody", func(t *testing.T) {
		res, err := http.Post(ts.URL+"/marge/streaming/support/power_on", "application/xml", bytes.NewReader([]byte("<powerOn/>")))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}
	})

	t.Run("FullBody", func(t *testing.T) {
		payload := `<?xml version="1.0" encoding="UTF-8" ?><device-data><device id="A81B6A536A98"><serialnumber>I6332527703739342000020</serialnumber><firmware-version>27.0.6.46330</firmware-version><product product_code="SoundTouch 10 sm2" type="5"><serialnumber>069231P63364828AE</serialnumber></product></device><diagnostic-data><device-landscape><rssi>Excellent</rssi><gateway-ip-address>192.168.1.1</gateway-ip-address><macaddresses><macaddress>A81B6A536A98</macaddress></macaddresses><ip-address>192.168.1.100</ip-address><network-connection-type>Wireless</network-connection-type></device-landscape></diagnostic-data></device-data>`
		res, err := http.Post(ts.URL+"/marge/streaming/support/power_on", "application/vnd.bose.streaming-v1.2+xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}
	})
}

func TestMargeAdvancedFeatures(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
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
		if !strings.Contains(string(body), "<keyName>ELIGIBLE_FOR_TRIAL</keyName>") {
			t.Errorf("Response body missing ELIGIBLE_FOR_TRIAL: %s", body)
		}
		if !strings.Contains(string(body), "<keyName>STREAMING_QUALITY</keyName>") {
			t.Errorf("Response body missing STREAMING_QUALITY: %s", body)
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

		contentType := res.Header.Get("Content-Type")
		if contentType != "application/vnd.bose.streaming-v1.2+xml" {
			t.Errorf("Invalid content type: %s", contentType)
		}

		token := res.Header.Get("Authorization")
		if !strings.HasPrefix(token, "Bearer st-local-token-") {
			t.Errorf("Invalid token header: %s", token)
		}

		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), "<bearertoken") {
			t.Errorf("Response body missing <bearertoken: %s", body)
		}
		if !strings.Contains(string(body), token) {
			t.Errorf("Response body missing token value: %s", body)
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

		if ct := res.Header.Get("Content-Type"); ct != "" {
			t.Errorf("Expected no Content-Type for customer support upload (empty body), got %v", ct)
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

	t.Run("AddRecent_Reproduction", func(t *testing.T) {
		account := "3230304"
		device := "A81B6A536A98"

		// Setup sources for this device
		deviceDir := ds.AccountDeviceDir(account, device)
		_ = os.MkdirAll(deviceDir, 0755)
		_ = os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte("<recents/>"), 0644)
		// No Sources.xml

		path := "/marge/streaming/account/" + account + "/device/" + device + "/recent"
		payload := `<?xml version="1.0" encoding="UTF-8" ?><recent><lastplayedat>2026-02-25T23:03:14+00:00</lastplayedat><sourceid>10863533</sourceid><name>My top tracks playlist</name><location>/playback/container/c3BvdGlmeTpwbGF5bGlzdDo3YklIMERKRUdoVjFSZ2duandOYWxn</location><contentItemType>tracklisturl</contentItemType></recent>`

		res, err := http.Post(ts.URL+path, "application/xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status Created (201), got %v: %s", res.Status, body)
		}

		// Verify it was saved
		recents, err := ds.GetRecents(account, device)
		if err != nil {
			t.Fatalf("Failed to get recents: %v", err)
		}
		if len(recents) == 0 {
			t.Error("Recents list is empty")
		} else if recents[0].Name != "My top tracks playlist" {
			t.Errorf("Expected name 'My top tracks playlist', got '%s'", recents[0].Name)
		}
	})
}
