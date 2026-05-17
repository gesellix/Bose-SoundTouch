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
// payload from the log (line 36) — `deviceid="AABBCCDDEE02"`,
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
		accountID         = "3981561"
		deviceID          = "AABBCCDDEE02"
		oldName           = "Wohnzimmer"
		newName           = "Wohnzimmer SB"
		preExistingIP     = "192.168.0.109"
		preExistingPaired = "2017-02-07T11:13:03.000+00:00"
	)

	// 1. Seed datastore with the device under its original name and
	// a known pre-existing first-paired timestamp. The pre-existing
	// data models a long-paired device the user is now renaming —
	// CreatedOn must survive the PUT (real Bose preserves it
	// across renames; see parity capture at
	// data/parity_mismatches/1771797308__streaming_account_1000001_device_AABBCCDDEEFF.json).
	if err := ds.SaveDeviceInfo(accountID, deviceID, &models.ServiceDeviceInfo{
		DeviceID:  deviceID,
		AccountID: accountID,
		Name:      oldName,
		IPAddress: preExistingIP,
		CreatedOn: preExistingPaired,
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

	// Parity assertion: the pre-existing first-paired CreatedOn
	// must survive the rename. This is the load-bearing fix versus
	// the prior behaviour that rewrote `now()` on every PUT, and
	// matches what real Bose's pre-shutdown 200 OK responses
	// carried (see the parity capture referenced above).
	if !bytes.Contains(respBody, []byte(`<createdOn>`+preExistingPaired+`</createdOn>`)) {
		t.Errorf("response did not preserve pre-existing CreatedOn %q; body:\n%s", preExistingPaired, respBody)
	}

	// Parity assertion: the pre-existing IP address must survive
	// the rename. The request body doesn't carry an `<ipaddress>`,
	// so the datastore merge has to inject what was already on
	// disk rather than writing back empty.
	if !bytes.Contains(respBody, []byte(`<ipaddress>`+preExistingIP+`</ipaddress>`)) {
		t.Errorf("response did not preserve pre-existing IPAddress %q; body:\n%s", preExistingIP, respBody)
	}

	// Parity assertion: UpdatedOn refreshes. Don't pin the exact
	// value — it's "now()" — but assert it's present and
	// non-empty.
	if !bytes.Contains(respBody, []byte(`<updatedOn>`)) ||
		bytes.Contains(respBody, []byte(`<updatedOn></updatedOn>`)) {
		t.Errorf("response missing or empty <updatedOn>; body:\n%s", respBody)
	}

	// 4. Persistence assertion: the datastore now reflects the new
	// name AND keeps the original CreatedOn. This is what the
	// Bose App reads back on its next /streaming/account/.../full
	// poll, which is what closes the visible rename loop.
	persisted, err := ds.GetDeviceInfo(accountID, deviceID)
	if err != nil {
		t.Fatalf("read persisted device info: %v", err)
	}

	if persisted.Name != newName {
		t.Errorf("persisted Name = %q, want %q", persisted.Name, newName)
	}

	if persisted.CreatedOn != preExistingPaired {
		t.Errorf("persisted CreatedOn = %q, want %q (preserved across rename)", persisted.CreatedOn, preExistingPaired)
	}

	if persisted.IPAddress != preExistingIP {
		t.Errorf("persisted IPAddress = %q, want %q (preserved across rename)", persisted.IPAddress, preExistingIP)
	}

	if persisted.UpdatedOn == "" {
		t.Errorf("persisted UpdatedOn is empty; want a fresh timestamp from the rename")
	}
}

// TestIssue285_NewDeviceGetsRemoteAddrAndFreshTimestamps covers the
// "first-time registration" path on a PUT (which can happen if the
// speaker emits a rename before AfterTouch has ever heard of it).
// With no pre-existing datastore record:
//
//   - CreatedOn must be a fresh timestamp (no record to preserve).
//   - IPAddress must come from r.RemoteAddr (the inbound connection)
//     since the request body doesn't carry one.
//   - UpdatedOn must be the same fresh timestamp.
//
// Pairs with the parity-preservation assertions in the main test:
// existing records win, but new records seed sensibly instead of
// landing with empty CreatedOn / IPAddress.
func TestIssue285_NewDeviceGetsRemoteAddrAndFreshTimestamps(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "issue285-new-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	const (
		accountID = "1111111"
		deviceID  = "AABBCCDDEEFF"
		newName   = "Living Room SoundTouch"
	)

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)

	t.Cleanup(ts.Close)

	body := []byte(`<?xml version="1.0" encoding="UTF-8" ?>` +
		`<device deviceid="` + deviceID + `"><name>` + newName + `</name><macaddress>` + deviceID + `</macaddress></device>`)

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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT status = %d, want 200; body:\n%s", resp.StatusCode, respBody)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	// CreatedOn present and non-empty (will be "now()" since no
	// prior record existed).
	if !bytes.Contains(respBody, []byte(`<createdOn>`)) ||
		bytes.Contains(respBody, []byte(`<createdOn></createdOn>`)) {
		t.Errorf("first-registration response missing CreatedOn; body:\n%s", respBody)
	}

	// IPAddress should be the httptest connection's remote host
	// (127.0.0.1) since the body didn't carry one and there was
	// no existing record to preserve from.
	if !bytes.Contains(respBody, []byte(`<ipaddress>127.0.0.1</ipaddress>`)) {
		t.Errorf("first-registration response missing IPAddress from RemoteAddr; body:\n%s", respBody)
	}

	// Persistence: CreatedOn and IPAddress on disk too.
	persisted, err := ds.GetDeviceInfo(accountID, deviceID)
	if err != nil {
		t.Fatalf("read persisted device info: %v", err)
	}

	if persisted.CreatedOn == "" {
		t.Errorf("persisted CreatedOn is empty for new device; want a fresh timestamp")
	}

	if persisted.IPAddress != "127.0.0.1" {
		t.Errorf("persisted IPAddress = %q, want %q (from RemoteAddr)", persisted.IPAddress, "127.0.0.1")
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

	const urlDeviceID = "AABBCCDDEE02"

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

	// Mismatched body must be rejected *before* the upsert runs —
	// otherwise the datastore ends up with a row keyed on the body's
	// deviceID even though we return 400. Verify by reading both keys.
	if got, _ := ds.GetDeviceInfo("3981561", "DEADBEEFCAFE"); got != nil {
		t.Fatalf("body deviceID DEADBEEFCAFE was persisted despite 400 response: %+v", got)
	}

	if got, _ := ds.GetDeviceInfo("3981561", urlDeviceID); got != nil {
		t.Fatalf("URL deviceID %s was persisted despite 400 response: %+v", urlDeviceID, got)
	}
}
