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

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestIssue285_RenamePutAcceptedAndPersisted reproduces the rename
// loop documented in issue #285:
//
//	https://github.com/gesellix/Bose-SoundTouch/issues/285
//
// When a user renames an ST10 via the Bose App or via
// `soundtouch-cli name set`, the speaker fires PUT
// /streaming/account/{accountID}/device/{deviceID} with a body of
// the form:
//
//	<device deviceid="…"><name>NEW</name><macaddress>…</macaddress></device>
//
// Before this commit the router only registered POST for that path;
// PUT fell through to the chi router's default handling and the
// speaker observed HTTP 502 (captured verbatim in
// _/i285/Rename.log:38: "SimpleURLFetcher: retry needed, Curl 0,
// http 502, retries remaining 0"). The speaker retried in a loop
// and the Bose App showed the rename spinning indefinitely.
//
// The fixture at testdata/issue285/rename_request.xml is the exact
// payload from the log (line 36) — `deviceid="884AEAEEBD27"`,
// `<name>Wohnzimmer SB</name>`. The test:
//
//  1. Pre-seeds the datastore with a device record under the
//     reporter's accountID + deviceID so the PUT is updating, not
//     creating.
//  2. Replays the rename PUT.
//  3. Asserts:
//     - HTTP 200 (NOT 201; this is an update, not a create — speakers
//     observed 502 before, so any 2xx is the headline fix, but
//     pinning 200 protects against accidentally returning 201
//     which would change the Location-header contract).
//     - Response body carries the new name verbatim.
//     - Persisted Sources/DeviceInfo on disk reflects the new name.
//
// When future work decides to preserve `createdOn` across updates
// (currently AddDeviceToAccount rewrites both timestamps), update
// the test to also assert that — the rename request from the log
// does NOT carry a createdOn, so any value our marge response
// emits is purely our choice and should be stable.
func TestIssue285_RenamePutAcceptedAndPersisted(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "issue285-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	const (
		accountID = "3981561"
		deviceID  = "884AEAEEBD27"
		oldName   = "Wohnzimmer"
		newName   = "Wohnzimmer SB"
	)

	// 1. Seed datastore with the device under its original name —
	// modelling a pre-existing paired device the user is now
	// renaming.
	if err := ds.SaveDeviceInfo(accountID, deviceID, &models.ServiceDeviceInfo{
		DeviceID:  deviceID,
		AccountID: accountID,
		Name:      oldName,
		IPAddress: "192.168.0.109",
	}); err != nil {
		t.Fatalf("seed datastore: %v", err)
	}

	// 2. Spin up the router and replay the captured rename PUT.
	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)

	t.Cleanup(ts.Close)

	body, err := os.ReadFile(filepath.Join("testdata", "issue285", "rename_request.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Sanity-check the fixture before trusting any downstream
	// assertion against it.
	if !bytes.Contains(body, []byte(`deviceid="`+deviceID+`"`)) {
		t.Fatalf("fixture missing expected deviceid=%q; got:\n%s", deviceID, body)
	}

	if !bytes.Contains(body, []byte(`<name>`+newName+`</name>`)) {
		t.Fatalf("fixture missing expected new name %q; got:\n%s", newName, body)
	}

	req, err := http.NewRequest(http.MethodPut,
		ts.URL+"/streaming/account/"+accountID+"/device/"+deviceID,
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	req.Header.Set("Content-Type", "application/xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 3. Headline assertion: the speaker observed 502 before — any
	// 2xx fixes the loop. Pin 200 specifically so we don't drift
	// into 201/Created (which would change the Location-header
	// contract POST gets).
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT status = %d, want 200; body:\n%s", resp.StatusCode, respBody)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	// Response shape: <device …><name>NEW</name>…</device>
	if !bytes.Contains(respBody, []byte(`deviceid="`+deviceID+`"`)) {
		t.Errorf("response missing deviceid=%q; body:\n%s", deviceID, respBody)
	}

	if !bytes.Contains(respBody, []byte(`<name>`+newName+`</name>`)) {
		t.Errorf("response missing new name %q; body:\n%s", newName, respBody)
	}

	if strings.Contains(string(respBody), `<name>`+oldName+`</name>`) {
		t.Errorf("response still carries old name %q; body:\n%s", oldName, respBody)
	}

	// 4. Persistence assertion: the datastore now reflects the new
	// name. This is what the Bose App reads back on its next
	// /streaming/account/.../full poll, which is what closes the
	// visible rename loop.
	persisted, err := ds.GetDeviceInfo(accountID, deviceID)
	if err != nil {
		t.Fatalf("read persisted device info: %v", err)
	}

	if persisted.Name != newName {
		t.Errorf("persisted Name = %q, want %q", persisted.Name, newName)
	}
}

// TestIssue285_RenamePutRejectsMismatchedDeviceID pins the safety
// check: if the speaker (or a bug elsewhere) ever sends a PUT with
// a body whose `deviceid="…"` doesn't match the URL's `{device}`
// segment, we refuse with 400 rather than silently re-key the
// persisted record under the wrong account/device.
func TestIssue285_RenamePutRejectsMismatchedDeviceID(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "issue285-mismatch-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)

	t.Cleanup(ts.Close)

	const urlDeviceID = "884AEAEEBD27"

	// Body claims a different deviceID than the URL.
	body := []byte(`<?xml version="1.0" encoding="UTF-8" ?>` +
		`<device deviceid="DEADBEEFCAFE"><name>Rogue</name><macaddress>DEADBEEFCAFE</macaddress></device>`)

	req, err := http.NewRequest(http.MethodPut,
		ts.URL+"/streaming/account/3981561/device/"+urlDeviceID,
		bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	req.Header.Set("Content-Type", "application/xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT status = %d, want 400; body:\n%s", resp.StatusCode, respBody)
	}
}
