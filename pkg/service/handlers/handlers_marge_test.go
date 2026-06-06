package handlers

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestMargeCreateAccount(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	reqBody := `<account>
		<preferredLanguage>de</preferredLanguage>
	</account>`

	res, err := http.Post(ts.URL+"/marge/streaming/account", "application/vnd.bose.streaming-v1.2+xml", strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusCreated {
		t.Errorf("Expected status Created, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if contentType != "application/vnd.bose.streaming-v1.2+xml" {
		t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.2+xml, got %v", contentType)
	}

	body, _ := io.ReadAll(res.Body)
	var resp models.AccountFullResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.AccountStatus != "OK" {
		t.Errorf("Expected AccountStatus OK, got %v", resp.AccountStatus)
	}
	if resp.PreferredLanguage != "de" {
		t.Errorf("Expected PreferredLanguage de, got %v", resp.PreferredLanguage)
	}
	if len(resp.ID) != 7 {
		t.Errorf("Expected 7-digit ID, got %v", resp.ID)
	}

	// Verify default sources. AUX (id=10001) is intentionally excluded from
	// cloud responses — real Bose never emitted AUX in /full; the speaker
	// enumerates AUX from its own hardware via isLocal=true in :8090/sources.
	// INTERNET_RADIO (id=10002) is also excluded — it is a legacy stub that
	// AfterTouch no longer adds to new devices. See GetInitialSources() and
	// pkg/service/marge/marge.go getAccountSources.
	if len(resp.Sources) != 3 {
		t.Errorf("Expected 3 cloud default sources (AUX and INTERNET_RADIO excluded), got %d", len(resp.Sources))
	} else {
		if resp.Sources[0].ID != "10003" {
			t.Errorf("Expected first cloud source ID 10003 (LOCAL_INTERNET_RADIO), got %s", resp.Sources[0].ID)
		}
	}

	// Verify it was saved in datastore
	info, err := ds.GetAccountInfo(resp.ID)
	if err != nil {
		t.Errorf("Failed to get account from datastore: %v", err)
	}
	if info == nil {
		t.Error("Account not found in datastore")
	} else if info.PreferredLanguage != "de" {
		t.Errorf("Expected saved PreferredLanguage de, got %v", info.PreferredLanguage)
	}
}

func TestMargeLogin(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	accountID := "9876543"
	_ = ds.SaveAccountInfo(accountID, &models.ServiceAccountInfo{
		AccountID:         accountID,
		PreferredLanguage: "fr",
	})

	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	reqBody := `<login>
		<username>test@example.com</username>
		<password>secret</password>
	</login>`

	res, err := http.Post(ts.URL+"/marge/streaming/account/login", "application/vnd.bose.streaming-v1.2+xml", strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	credentials := res.Header.Get("Credentials")
	if credentials != "mock-token-"+accountID {
		t.Errorf("Expected Credentials mock-token-%s, got %v", accountID, credentials)
	}

	body, _ := io.ReadAll(res.Body)
	var resp models.AccountFullResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.ID != accountID {
		t.Errorf("Expected ID %s, got %v", accountID, resp.ID)
	}
}

func TestMargeLogin_NoAccount(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	reqBody := `<login>
		<username>none@example.com</username>
		<password>secret</password>
	</login>`

	res, err := http.Post(ts.URL+"/marge/streaming/account/login", "application/vnd.bose.streaming-v1.2+xml", strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %v", res.Status)
	}
}

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
				<ipAddress>192.0.2.100</ipAddress>
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

// TestMargeAccountFullExcludesEmptyAmazonSource is a regression test for the two-device scenario
// observed in production: device AABBCCDDEEFF (alphabetically last, used as lastDeviceID)
// has Sources.xml with 6 sources but no Amazon. The first device has Amazon with empty
// credentials (written before OAuth was implemented). Amazon must NOT appear in /full —
// an empty-credential Amazon causes the speaker's AmazonController to fail JSON parsing.
func TestMargeAccountFullExcludesEmptyAmazonSource(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-amazon-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	account := "1000001"

	// First device (alphabetically): has Amazon in Sources.xml
	firstDeviceID := "AABBCCDDEE0A"
	firstDir := filepath.Join(tempDir, "accounts", account, "devices", firstDeviceID)
	if err := os.MkdirAll(firstDir, 0755); err != nil {
		t.Fatalf("Failed to create first device dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(firstDir, "DeviceInfo.xml"), []byte(`
		<info deviceID="AABBCCDDEE0A">
			<name>Kitchen SoundTouch</name>
			<type>SoundTouch 20 scm</type>
		</info>
	`), 0644); err != nil {
		t.Fatalf("Failed to write first device DeviceInfo.xml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(firstDir, "Sources.xml"), []byte(`<sources>
		<source id="10006" type="AMAZON" createdOn="2026-01-01T00:00:00.000+00:00" updatedOn="2026-01-01T00:00:00.000+00:00" displayName="Amazon Music" secret="" secretType="token" sourceproviderid="20">
			<sourceKey type="AMAZON" account=""/>
		</source>
	</sources>`), 0644); err != nil {
		t.Fatalf("Failed to write first device Sources.xml: %v", err)
	}

	// Second device (alphabetically last = lastDeviceID): 6 sources but NO Amazon.
	// This reproduces the real full.xml returned by the live service.
	lastDeviceID := "AABBCCDDEEFF"
	lastDir := filepath.Join(tempDir, "accounts", account, "devices", lastDeviceID)
	if err := os.MkdirAll(lastDir, 0755); err != nil {
		t.Fatalf("Failed to create last device dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lastDir, "DeviceInfo.xml"), []byte(`
		<info deviceID="AABBCCDDEEFF">
			<name>Another Speaker</name>
			<type>SoundTouch 300</type>
		</info>
	`), 0644); err != nil {
		t.Fatalf("Failed to write last device DeviceInfo.xml: %v", err)
	}
	// Sources.xml mirrors the real persisted file: AUX, INTERNET_RADIO, LOCAL_INTERNET_RADIO,
	// TUNEIN, RADIO_BROWSER, Spotify — no Amazon.
	lastSourcesXML := `<sources>
		<source id="10001" type="Audio" createdOn="2015-03-11T19:12:38.000+00:00" updatedOn="2015-03-11T19:12:38.000+00:00" displayName="AUX IN" secret="" secretType="token" sourceproviderid="9">
			<sourceKey type="AUX" account="AUX"/>
		</source>
		<source id="10002" type="Audio" createdOn="2015-03-11T19:12:38.000+00:00" updatedOn="2015-03-11T19:12:38.000+00:00" displayName="" secret="" secretType="token" sourceproviderid="2">
			<sourceKey type="INTERNET_RADIO" account=""/>
		</source>
		<source id="10003" type="Audio" createdOn="2019-01-24T08:18:37.000+00:00" updatedOn="2019-02-03T18:35:45.000+00:00" displayName="" secret="eyJzZXJpYWwiOiJsb2NhbC1pbnRlcm5ldC1yYWRpbyJ9" secretType="token" sourceproviderid="11">
			<sourceKey type="LOCAL_INTERNET_RADIO" account=""/>
		</source>
		<source id="10004" type="Audio" createdOn="2017-07-20T16:43:48.000+00:00" updatedOn="2017-07-20T16:43:48.000+00:00" displayName="" secret="eyJzZXJpYWwiOiJ0dW5laW4ifQ==" secretType="token" sourceproviderid="25">
			<sourceKey type="TUNEIN" account=""/>
		</source>
		<source id="10005" type="Audio" createdOn="2026-02-16T01:01:01.000+00:00" updatedOn="2026-02-16T01:01:01.000+00:00" displayName="" secret="" secretType="token" sourceproviderid="39">
			<sourceKey type="RADIO_BROWSER" account=""/>
		</source>
		<source id="SRC_1776706409" type="Audio" createdOn="2026-04-20T17:33:29.483+00:00" updatedOn="2026-04-20T17:33:29.483+00:00" displayName="" secret="bs-6c58d056c2d35df85f57ad2334b0cdc4" secretType="token_version_3" sourceproviderid="15">
			<sourceKey type="SPOTIFY" account="gesellix"/>
		</source>
	</sources>`
	if err := os.WriteFile(filepath.Join(lastDir, "Sources.xml"), []byte(lastSourcesXML), 0644); err != nil {
		t.Fatalf("Failed to write last device Sources.xml: %v", err)
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
		t.Errorf("Expected 200 OK, got %v", res.Status)
	}

	body, _ := io.ReadAll(res.Body)
	bodyStr := string(body)

	// Amazon with empty credentials must not appear — the speaker's AmazonController
	// fails to parse an empty secret and returns MUSIC_SERVICE_ACCOUNT_LOGIN_FAILED.
	if strings.Contains(bodyStr, "<sourceproviderid>20</sourceproviderid>") {
		t.Errorf("/full response must not include an empty-credential Amazon source; body:\n%s", bodyStr)
	}

	// The cloud-visible sources from lastDeviceID's stored Sources.xml
	// must all be present. AUX (sourceproviderid=9) is intentionally
	// excluded — real Bose never emitted AUX in /full; the speaker
	// enumerates AUX from its own hardware via isLocal=true. See
	// pkg/service/marge/marge.go getAccountSources.
	for _, wantProviderID := range []string{
		"<sourceproviderid>2</sourceproviderid>",  // INTERNET_RADIO
		"<sourceproviderid>11</sourceproviderid>", // LOCAL_INTERNET_RADIO
		"<sourceproviderid>25</sourceproviderid>", // TUNEIN
		"<sourceproviderid>39</sourceproviderid>", // RADIO_BROWSER
		"<sourceproviderid>15</sourceproviderid>", // Spotify
	} {
		if !strings.Contains(bodyStr, wantProviderID) {
			t.Errorf("/full response is missing source with %s; body:\n%s", wantProviderID, bodyStr)
		}
	}

	// And explicitly assert AUX is NOT present.
	if strings.Contains(bodyStr, "<sourceproviderid>9</sourceproviderid>") {
		t.Errorf("/full response must not include AUX (sourceproviderid=9); body:\n%s", bodyStr)
	}
}

func TestMargeAccountSources(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "12345"
	deviceID := "DEV1"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", deviceID)
	_ = os.MkdirAll(deviceDir, 0755)

	// Mock Sources.xml. PANDORA is used because (a) it maps to a
	// constants.StaticProviders entry, so HasResolvableProviderID accepts it
	// and ensureSourceProviderID fills its providerid — sources with no
	// resolvable providerid are now filtered before serving, since the speaker
	// rejects an empty <sourceproviderid> as INVALID_SOURCE (issue #334); and
	// (b) it is not one of the username-blanking types
	// (TUNEIN/INTERNET_RADIO/LOCAL_INTERNET_RADIO), so the serve path still
	// emits <username>, keeping that assertion meaningful.
	sourcesXML := `
		<sources>
			<source id="SRC1" type="Audio" createdOn="2024-01-01T00:00:00Z" updatedOn="2024-01-01T00:00:00Z" displayName="Source1" secret="TOKEN1" secretType="token" sourceName="SourceName1">
				<sourceKey type="PANDORA" account="User1"/>
			</source>
		</sources>`
	_ = os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644)

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/streaming/account/" + account + "/sources")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if contentType != "application/vnd.bose.streaming-v1.1+xml" {
		t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.1+xml, got %v", contentType)
	}

	body, _ := io.ReadAll(res.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "SRC1") {
		t.Errorf("Response missing expected source ID: %s", bodyStr)
	}

	// Verify current XML structure produced by marge.go.
	expectedSnippets := []string{
		"<sources>",
		"<source id=\"SRC1\" type=\"Audio\"",
		"<createdOn>2024-01-01T00:00:00Z</createdOn>",
		"<updatedOn>2024-01-01T00:00:00Z</updatedOn>",
		"<credential type=\"token\">TOKEN1</credential>",
		"<name>Source1</name>",
		"<sourcename></sourcename>",
		"<sourceSettings/>",
		"<username>User1</username>",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(bodyStr, snippet) {
			t.Errorf("Response missing expected snippet [%s]: %s", snippet, bodyStr)
		}
	}
}

func TestMargeAccountPresets(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "12345"
	device1 := "DEVICE1"
	device2 := "DEVICE2"

	// Setup presets for device1
	presets1 := []models.ServicePreset{
		{
			ID: "1",
			ServiceContentItem: models.ServiceContentItem{
				Name: "Station 1",
			},
		},
	}
	if err := ds.SavePresets(account, device1, presets1); err != nil {
		t.Fatal(err)
	}

	// Setup presets for device2
	presets2 := []models.ServicePreset{
		{
			ID: "2",
			ServiceContentItem: models.ServiceContentItem{
				Name: "Station 2",
			},
		},
	}
	if err := ds.SavePresets(account, device2, presets2); err != nil {
		t.Fatal(err)
	}

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Test /streaming/account/{account}/presets/all
	res, err := http.Get(ts.URL + "/streaming/account/" + account + "/presets/all")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if contentType != "application/vnd.bose.streaming-v1.1+xml" {
		t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.1+xml, got %v", contentType)
	}

	body, _ := io.ReadAll(res.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "<presets>") {
		t.Error("Response body missing <presets>")
	}
	if !strings.Contains(bodyStr, "buttonNumber=\"1\"") {
		t.Error("Response body missing preset 1")
	}
	if !strings.Contains(bodyStr, "buttonNumber=\"2\"") {
		t.Error("Response body missing preset 2")
	}
}

func TestMargeAccountDevices(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "12345"
	deviceID := "DEV1"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", deviceID)
	_ = os.MkdirAll(deviceDir, 0755)

	// Mock DeviceInfo.json
	deviceInfo := models.ServiceDeviceInfo{
		DeviceID:            deviceID,
		Name:                "Test Device",
		IPAddress:           "192.0.2.100",
		DeviceSerialNumber:  "ABCDE12345",
		ProductCode:         "SoundTouch 20",
		ProductSerialNumber: "066802942560222AE",
	}
	_ = ds.SaveDeviceInfo(account, deviceID, &deviceInfo)

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/streaming/account/" + account + "/devices")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	contentType := res.Header.Get("Content-Type")
	if contentType != "application/vnd.bose.streaming-v1.1+xml" {
		t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.1+xml, got %v", contentType)
	}

	body, _ := io.ReadAll(res.Body)
	bodyStr := string(body)

	// Verify current XML structure produced by marge.go
	expectedSnippets := []string{
		"<devices>",
		"<device deviceid=\"DEV1\">",
		"<name>Test Device</name>",
		"<ipaddress>192.0.2.100</ipaddress>",
		"<providerSettings>",
		"ELIGIBLE_FOR_TRIAL",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(bodyStr, snippet) {
			t.Errorf("Response missing expected snippet [%s]: %s", snippet, bodyStr)
		}
	}
}

func TestMargeAccountSourcesNoDevices(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "12345"

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/streaming/account/" + account + "/sources")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	body, _ := io.ReadAll(res.Body)
	bodyStr := string(body)

	// Verify that we get the default cloud sources with correct IDs. AUX
	// (id=10001) is intentionally excluded — real Bose never emitted AUX
	// in cloud responses; the speaker enumerates AUX from its own
	// hardware (isLocal=true on :8090/sources). INTERNET_RADIO (id=10002)
	// is also intentionally excluded — it is a legacy stub that AfterTouch
	// no longer adds to new or no-device accounts. See GetInitialSources()
	// and pkg/service/marge/marge.go getAccountSources.
	expectedSnippets := []string{
		"<sources>",
		"<source id=\"10004\" type=\"Audio\"",
		"<source id=\"10003\" type=\"Audio\"",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(bodyStr, snippet) {
			t.Errorf("Response missing expected snippet [%s]: %s", snippet, bodyStr)
		}
	}

	if strings.Contains(bodyStr, "<source id=\"10001\"") {
		t.Errorf("Response must not include AUX (id=10001); body:\n%s", bodyStr)
	}

	if strings.Contains(bodyStr, "<source id=\"10002\"") {
		t.Errorf("Response must not include INTERNET_RADIO (id=10002) for accounts with no device; body:\n%s", bodyStr)
	}

	// Verify that no sources have empty display names
	if strings.Count(bodyStr, "displayName=\"\"") != 0 {
		t.Errorf("Expected no sources with empty displayName, got %d: %s", strings.Count(bodyStr, "displayName=\"\""), bodyStr)
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
				<contentItem source="TUNEIN" type="station" location="/station/s123" sourceAccount="" isPresetable="true">
					<itemName>Test Station</itemName>
					<containerArt>http://example.com/art.jpg</containerArt>
				</contentItem>
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

	// Verify response body has correct XML structure (upstream parity)
	body, _ := io.ReadAll(res.Body)
	bodyStr := string(body)

	// Verify no wrapping <presets> element
	if strings.HasPrefix(bodyStr, "<?xml version=\"1.0\" encoding=\"UTF-8\"?><presets>") {
		t.Errorf("Response should NOT be wrapped in <presets>: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "<preset buttonNumber=\"1\">") {
		t.Errorf("Response missing <preset buttonNumber=\"1\">: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "source=\"TUNEIN\"") {
		t.Errorf("Response should NOT have source attribute on root element: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "<sourceid>") {
		t.Errorf("Response should NOT have <sourceid> element: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "<source") || !strings.Contains(bodyStr, "id=\"SRC1\"") {
		t.Errorf("Response missing nested <source id=\"SRC1\">: %s", bodyStr)
	}
	// Verify two distinct <username> elements
	usernameCount := strings.Count(bodyStr, "<username>")
	if usernameCount != 2 {
		t.Errorf("Expected 2 <username> elements, got %d: %s", usernameCount, bodyStr)
	}
	if !strings.Contains(bodyStr, "<username>New Preset</username>") {
		t.Errorf("Response missing <username>New Preset</username>: %s", bodyStr)
	}

	// Verify empty tags are present (parity requirement)
	//if !strings.Contains(bodyStr, "<sourcename></sourcename>") && !strings.Contains(bodyStr, "<sourcename/>") {
	//	t.Errorf("Response missing empty <sourcename>: %s", bodyStr)
	//}
	//if !strings.Contains(bodyStr, "<name></name>") && !strings.Contains(bodyStr, "<name/>") {
	//	t.Errorf("Response missing empty <name>: %s", bodyStr)
	//}
	if !strings.Contains(bodyStr, "<sourceSettings></sourceSettings>") && !strings.Contains(bodyStr, "<sourceSettings/>") {
		t.Errorf("Response missing empty <sourceSettings>: %s", bodyStr)
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

	t.Run("PUT /streaming/account/{account}/device/{device}/preset/{presetNumber} - valid Sources.xml", func(t *testing.T) {
		payload := `
			<preset>
				<name>PUT Native Preset Singular</name>
				<sourceid>SRC1</sourceid>
				<location>/station/s888</location>
				<contentItemType>station</contentItemType>
			</preset>`

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/preset/6", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/xml")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
		}
	})

	t.Run("PUT /streaming/account/{account}/device/{device}/preset/{presetNumber}", func(t *testing.T) {
		payload := `
			<preset>
				<name>PUT Native Preset Singular</name>
				<sourceid>SRC1</sourceid>
				<location>/station/s888</location>
				<contentItemType>station</contentItemType>
			</preset>`

		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/preset/6", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/xml")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("Expected status OK, got %v: %s", res.Status, string(body))
		}

		// Verify file was saved
		presetData, _ := os.ReadFile(filepath.Join(deviceDir, "Presets.xml"))
		if !strings.Contains(string(presetData), "PUT Native Preset Singular") {
			t.Error("Preset from singular native PUT route was not saved to datastore")
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
				<ipAddress>192.0.2.101</ipAddress>
			</networkInfo>
		</device>`

	res, err := http.Post(ts.URL+"/marge/accounts/"+account+"/devices", "application/xml", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	_ = res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Errorf("AddDevice: Expected status Created, got %v", res.Status)
	}

	location := res.Header.Get("Location")
	if !strings.Contains(location, "/account/"+account+"/device/NEWDEV") {
		t.Errorf("AddDevice: Expected Location header containing /account/%s/device/NEWDEV, got %s", account, location)
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
		payload := `<?xml version="1.0" encoding="UTF-8" ?><device-data><device id="001122334455"><serialnumber>I6332527703739342000020</serialnumber><firmware-version>27.0.6.46330</firmware-version><product product_code="SoundTouch 10 sm2" type="5"><serialnumber>069231P63364828AE</serialnumber></product></device><diagnostic-data><device-landscape><rssi>Excellent</rssi><gateway-ip-address>192.0.2.1</gateway-ip-address><macaddresses><macaddress>001122334455</macaddress></macaddresses><ip-address>192.0.2.100</ip-address><network-connection-type>Wireless</network-connection-type></device-landscape></diagnostic-data></device-data>`
		res, err := http.Post(ts.URL+"/marge/streaming/support/power_on", "application/vnd.bose.streaming-v1.2+xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}
	})

	t.Run("Persistence", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "st-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer func() { _ = os.RemoveAll(tempDir) }()

		ds := datastore.NewDataStore(tempDir)
		r, _ := setupRouter("http://localhost:8001", ds)
		ts2 := httptest.NewServer(r)
		defer ts2.Close()

		deviceID := "001122334455"
		serialNumber := "I6332527703739342000020"
		firmware := "27.0.6.46330"
		productCode := "SoundTouch 10 sm2"
		productSerial := "069231P63364828AE"
		ipAddress := "192.0.2.100"
		macAddress := "001122334455"

		payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" ?>
		<device-data>
			<device id="%s">
				<serialnumber>%s</serialnumber>
				<firmware-version>%s</firmware-version>
				<product product_code="%s" type="5">
					<serialnumber>%s</serialnumber>
				</product>
			</device>
			<diagnostic-data>
				<device-landscape>
					<rssi>Excellent</rssi>
					<gateway-ip-address>192.0.2.1</gateway-ip-address>
					<macaddresses>
						<macaddress>%s</macaddress>
					</macaddresses>
					<ip-address>%s</ip-address>
					<network-connection-type>Wireless</network-connection-type>
				</device-landscape>
			</diagnostic-data>
		</device-data>`, deviceID, serialNumber, firmware, productCode, productSerial, macAddress, ipAddress)

		res, err := http.Post(ts2.URL+"/marge/streaming/support/power_on", "application/vnd.bose.streaming-v1.2+xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		// Verify data in datastore
		info, err := ds.GetDeviceInfo("default", deviceID)
		if err != nil {
			t.Fatalf("Failed to get device info: %v", err)
		}

		if info.DeviceID != deviceID {
			t.Errorf("Expected DeviceID %s, got %s", deviceID, info.DeviceID)
		}
		if info.DeviceSerialNumber != serialNumber {
			t.Errorf("Expected SerialNumber %s, got %s", serialNumber, info.DeviceSerialNumber)
		}
		if info.FirmwareVersion != firmware {
			t.Errorf("Expected Firmware %s, got %s", firmware, info.FirmwareVersion)
		}
		if info.ProductCode != productCode {
			t.Errorf("Expected ProductCode %s, got %s", productCode, info.ProductCode)
		}
		if info.ProductSerialNumber != productSerial {
			t.Errorf("Expected ProductSerialNumber %s, got %s", productSerial, info.ProductSerialNumber)
		}
		if info.IPAddress != ipAddress {
			t.Errorf("Expected IPAddress %s, got %s", ipAddress, info.IPAddress)
		}
		if info.MacAddress != macAddress {
			t.Errorf("Expected MacAddress %s, got %s", macAddress, info.MacAddress)
		}
		if info.DiscoveryMethod != "power_on" {
			t.Errorf("Expected DiscoveryMethod power_on, got %s", info.DiscoveryMethod)
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
		account := "A123"
		deviceId := "587A628A4042"
		macAddress := "AABBCCDDEEFF"
		ipAddress := "192.0.2.100"
		firmware := "27.0.6"

		// Pre-register device
		_ = ds.SaveDeviceInfo(account, deviceId, &models.ServiceDeviceInfo{
			DeviceID: deviceId,
			Name:     "TestDevice",
		})

		payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" ?>
		<device-data>
			<device id="%s">
				<serialnumber>P123</serialnumber>
				<firmware-version>%s</firmware-version>
				<product product_code="SoundTouch 10" type="5">
					<serialnumber>SN123</serialnumber>
				</product>
			</device>
			<diagnostic-data>
				<device-landscape>
					<rssi>Good</rssi>
					<macaddresses>
						<macaddress>%s</macaddress>
					</macaddresses>
					<ip-address>%s</ip-address>
				</device-landscape>
			</diagnostic-data>
		</device-data>`, deviceId, firmware, macAddress, ipAddress)

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
		events := ds.GetDeviceEvents(deviceId)
		found := false

		for _, e := range events {
			if e.Type == "customer-support-upload" {
				found = true
				if e.Data["firmware"] != firmware {
					t.Errorf("Expected firmware %s, got %v", firmware, e.Data["firmware"])
				}
				break
			}
		}

		if !found {
			t.Error("Customer support event not found in event log")
		}

		// Verify DeviceInfo was updated
		info, err := ds.GetDeviceInfo(account, deviceId)
		if err != nil {
			t.Fatalf("Failed to get device info: %v", err)
		}
		if info.IPAddress != ipAddress {
			t.Errorf("Expected updated IP %s, got %s", ipAddress, info.IPAddress)
		}
		if info.MacAddress != macAddress {
			t.Errorf("Expected updated MAC %s, got %s", macAddress, info.MacAddress)
		}
		if info.FirmwareVersion != firmware {
			t.Errorf("Expected updated firmware %s, got %s", firmware, info.FirmwareVersion)
		}
	})

	t.Run("AddRecent_Reproduction", func(t *testing.T) {
		account := "1234567"
		device := "001122334455"

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

	t.Run("MusicProviderIsEligible", func(t *testing.T) {
		path := "/marge/streaming/music/musicprovider/26/is_eligible"
		payload := `<?xml version = "1.0" encoding = "utf-8"?><account><accountId>12345</accountId></account>`

		res, err := http.Post(ts.URL+path, "application/vnd.bose.streaming-v1.1+xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		if ct := res.Header.Get("Content-Type"); ct != "application/vnd.bose.streaming-v1.1+xml" {
			t.Errorf("Expected Content-Type application/vnd.bose.streaming-v1.1+xml, got %v", ct)
		}

		body, _ := io.ReadAll(res.Body)
		expected := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><eligibility><isEligible>false</isEligible></eligibility>`
		if string(body) != expected {
			t.Errorf("Expected body %s, got %s", expected, string(body))
		}
	})

	t.Run("APIVersions", func(t *testing.T) {
		path := "/marge/streaming/resources/api_versions.xml"

		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		if ct := res.Header.Get("Content-Type"); ct != "text/xml" {
			t.Errorf("Expected Content-Type text/xml, got %v", ct)
		}

		body, _ := io.ReadAll(res.Body)
		if !strings.HasPrefix(string(body), "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n<marge ") {
			t.Errorf("Response body has incorrect header or root element: %s", body)
		}
		if !strings.Contains(string(body), `<api type="streaming">`) {
			t.Error("Response body missing streaming API")
		}
		if !strings.Contains(string(body), `<api type="customer">`) {
			t.Error("Response body missing customer API")
		}
		if !strings.Contains(string(body), `<api type="support">`) {
			t.Error("Response body missing support API")
		}
	})
}

func TestMargeGroupCRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-group-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	account := "ACC001"
	device1 := "AABBCCDDEEFF"
	device2 := "112233445566"

	groupXML := `<?xml version="1.0" encoding="UTF-8"?>
<group>
  <name>Living Room Stereo</name>
  <masterDeviceId>` + device1 + `</masterDeviceId>
  <roles>
    <groupRole><deviceId>` + device1 + `</deviceId><role>LEFT</role><ipAddress>192.0.2.10</ipAddress></groupRole>
    <groupRole><deviceId>` + device2 + `</deviceId><role>RIGHT</role><ipAddress>192.0.2.11</ipAddress></groupRole>
  </roles>
  <senderIPAddress>192.0.2.10</senderIPAddress>
</group>`

	var groupID string

	t.Run("GET device group returns empty group before creation", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/streaming/account/" + account + "/device/" + device1 + "/group")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", res.StatusCode)
		}
		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), "<group") {
			t.Errorf("Expected <group> element, got: %s", body)
		}
	})

	t.Run("POST group creates a new group and returns 201 with ID", func(t *testing.T) {
		res, err := http.Post(
			ts.URL+"/marge/streaming/account/"+account+"/group",
			"application/xml",
			bytes.NewBufferString(groupXML),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("Expected 201 Created, got %d: %s", res.StatusCode, body)
		}

		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), `<group `) {
			t.Errorf("Response missing <group> with id attr: %s", body)
		}

		// Parse out the group ID from the response XML
		type groupResp struct {
			ID string `xml:"id,attr"`
		}
		var gr groupResp
		if err := xml.Unmarshal(body, &gr); err != nil {
			t.Fatalf("Failed to unmarshal group response: %v", err)
		}
		if gr.ID == "" {
			t.Fatalf("Response group has no ID: %s", body)
		}
		groupID = gr.ID
	})

	t.Run("GET device group returns the group after creation", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/streaming/account/" + account + "/device/" + device1 + "/group")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", res.StatusCode)
		}
		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), "Living Room Stereo") {
			t.Errorf("Expected group name in response: %s", body)
		}
	})

	t.Run("POST group/{groupId} renames the group", func(t *testing.T) {
		if groupID == "" {
			t.Skip("No group ID from prior subtest")
		}
		modXML := `<group><name>Bedroom Stereo</name><masterDeviceId>` + device1 + `</masterDeviceId></group>`
		req, _ := http.NewRequest(http.MethodPost,
			ts.URL+"/marge/streaming/account/"+account+"/group/"+groupID,
			bytes.NewBufferString(modXML),
		)
		req.Header.Set("Content-Type", "application/xml")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("Expected 200, got %d: %s", res.StatusCode, body)
		}
		body, _ := io.ReadAll(res.Body)
		if !strings.Contains(string(body), "Bedroom Stereo") {
			t.Errorf("Expected updated name in response: %s", body)
		}
	})

	t.Run("DELETE group/{groupId} removes the group", func(t *testing.T) {
		if groupID == "" {
			t.Skip("No group ID from prior subtest")
		}
		req, _ := http.NewRequest(http.MethodDelete,
			ts.URL+"/marge/streaming/account/"+account+"/group/"+groupID,
			nil,
		)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("Expected 200, got %d: %s", res.StatusCode, body)
		}
	})

	t.Run("DELETE group/{groupId} returns 404 for missing group", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete,
			ts.URL+"/marge/streaming/account/"+account+"/group/9999999",
			nil,
		)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", res.StatusCode)
		}
	})

	t.Run("GET device group is empty after deletion", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/streaming/account/" + account + "/device/" + device1 + "/group")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		body, _ := io.ReadAll(res.Body)
		// Should be back to empty <group/>
		if strings.Contains(string(body), "Bedroom Stereo") {
			t.Errorf("Group should be gone after deletion: %s", body)
		}
	})
}

// TestMargeAddGroup_FromSpeakerCapture replays the exact request a SoundTouch
// 10 master sends when it forwards an addGroup to its configured Marge server
// while forming a stereo pair. The shape is taken verbatim from a live capture
// in issue #252; account ID and device IDs are anonymised:
//
//	POST /streaming/account/{account}/group/
//	Authorization: Bearer <token>
//	Content-Type:  application/vnd.bose.streaming-v1.2+xml
//	<group><masterDeviceId>...</masterDeviceId><name>TEST</name>
//	  <roles>
//	    <groupRole><deviceId>{master}</deviceId><role>LEFT</role></groupRole>
//	    <groupRole><deviceId>{slave}</deviceId><role>RIGHT</role></groupRole>
//	  </roles>
//	</group>
//
// Notable differences from CLI-side requests this codebase already tests:
//   - URL has a trailing slash ("/group/", not "/group")
//   - <groupRole> elements have no <ipAddress>
//   - <senderIPAddress> is absent (correct for the master-bound payload)
//   - Content-Type is the vendor-specific media type
//
// The speaker retries this POST every 15 s while in AddingMaster state; if
// AfterTouch doesn't accept it the group never completes and reverts to
// NoGroup after a timeout. This test pins down the exact wire contract so
// any future change that breaks it fails loudly.
func TestMargeAddGroup_FromSpeakerCapture(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	const (
		account     = "1234567"
		masterDevID = "001122334455"
		slaveDevID  = "AABBCCDDEEFF"
	)

	// Body matches the captured MargeClient payload structure verbatim --
	// no <senderIPAddress>, no per-role <ipAddress>, no <status>, no group id.
	reqBody := `<?xml version="1.0" encoding="UTF-8" ?><group><masterDeviceId>` + masterDevID +
		`</masterDeviceId><name>TEST</name><roles><groupRole><deviceId>` + masterDevID +
		`</deviceId><role>LEFT</role></groupRole><groupRole><deviceId>` + slaveDevID +
		`</deviceId><role>RIGHT</role></groupRole></roles></group>`

	url := ts.URL + "/streaming/account/" + account + "/group/"

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	// Headers copied from the captured CMargeHttpInterface::Post lines.
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(res.Body)
		t.Fatalf("POST %s: expected 201 Created, got %d. Body: %s", url, res.StatusCode, respBody)
	}

	if got := res.Header.Get("Content-Type"); got != "application/vnd.bose.streaming-v1.2+xml" {
		t.Errorf("response Content-Type = %q, want %q", got, "application/vnd.bose.streaming-v1.2+xml")
	}

	location := res.Header.Get("Location")
	if !strings.Contains(location, "/account/"+account+"/group/") {
		t.Errorf("Location header should reference the new group under account %s, got %q", account, location)
	}

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	var got models.Group
	if err := xml.Unmarshal(respBody, &got); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, respBody)
	}

	if got.MasterDeviceID != masterDevID {
		t.Errorf("response masterDeviceId = %q, want %q", got.MasterDeviceID, masterDevID)
	}

	if got.Name != "TEST" {
		t.Errorf("response name = %q, want %q", got.Name, "TEST")
	}

	if len(got.Roles.Roles) != 2 {
		t.Fatalf("response roles = %d, want 2", len(got.Roles.Roles))
	}
}
