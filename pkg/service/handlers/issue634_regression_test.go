package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

// TestIssue634_NonNumericMargeAccountUUIDDoesNotLoseDevice reproduces
// https://github.com/gesellix/Bose-SoundTouch/issues/634
//
// A SoundTouch 10 had SSH enabled via the USB-stick method (rather than
// AfterTouch's own telnet-based enable-ssh flow) and, when discovered,
// reported a `margeAccountUUID` of `stick@local` instead of the usual
// 7-digit numeric Bose account ID. `handleDiscoveredDevice`
// (pkg/service/handlers/server.go) passes MargeAccountUUID straight
// through to DataStore.SaveDeviceInfo, which used to reject anything
// containing "@" as an "invalid account ID" via isSafeIdentifier's
// strict alnum-only allowlist. The device was never persisted at all.
//
// The fix widened datastore.IsSafeIdentifier to accept any device-reported
// identifier that's safe to use as a path component / XML value /
// telnet-command token, rather than requiring Bose's own 7-digit numeric
// format. setup's separate, stricter 7-digit-only IsValidAccountID was
// deleted outright in favor of calling datastore.IsSafeIdentifier directly
// everywhere an account ID needs validating — one validator, not two. So
// handleDiscoveredDevice needed no changes: it already passed
// MargeAccountUUID through unmodified, and now the datastore accepts it.
//
// What this test locks in:
//
//   - A speaker reporting a non-numeric margeAccountUUID is saved
//     under that account verbatim (not coerced to "default" — "default"
//     remains reserved for a genuinely empty/unpaired margeAccountUUID).
//
// What this test would catch if it flipped:
//
//   - If IsSafeIdentifier's allowlist regresses to reject "@" again,
//     GetDeviceInfo below would error with "invalid account ID" instead
//     of returning the device — the #634 symptom.
func TestIssue634_NonNumericMargeAccountUUIDDoesNotLoseDevice(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "issue634-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	const deviceInfoXML = `<info deviceID="001122334455">
<name>Kitchen SoundTouch</name>
<type>SoundTouch 10</type>
<margeAccountUUID>stick@local</margeAccountUUID>
<components>
<component>
<componentCategory>SCM</componentCategory>
<softwareVersion>27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29</softwareVersion>
<serialNumber>I6332527703739342000020</serialNumber>
</component>
<component>
<componentCategory>PackagedProduct</componentCategory>
<softwareVersion>27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29</softwareVersion>
<serialNumber>069231P63364828AE</serialNumber>
</component>
</components>
<margeURL>https://streaming.bose.com</margeURL>
<networkInfo type="SCM">
<macAddress>001122334455</macAddress>
<ipAddress>203.0.113.10</ipAddress>
</networkInfo>
<moduleType>sm2</moduleType>
<variant>rhino</variant>
<variantMode>normal</variantMode>
<countryCode>US</countryCode>
<regionCode>US</regionCode>
</info>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, deviceInfoXML)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	deviceIP := server.URL[len("http://"):]
	ds := datastore.NewDataStore(tempDir)
	sm := setup.NewManager(server.URL, ds, nil)

	srv := NewServer(ds, sm, server.URL, false, false, false)

	discoveredDevice := models.DiscoveredDevice{
		Host:            deviceIP,
		Name:            "Legacy Discovery Name",
		ModelID:         "SoundTouch 10",
		SerialNo:        "",
		DiscoveryMethod: "UPnP",
	}

	t.Logf("Test scenario: /info reports non-numeric margeAccountUUID %q", "stick@local")

	srv.handleDiscoveredDevice(discoveredDevice)

	const (
		expectedAccountID = "stick@local"
		expectedDeviceID  = "001122334455"
	)

	deviceInfo, err := ds.GetDeviceInfo(expectedAccountID, expectedDeviceID)
	if err != nil {
		t.Fatalf("device was not saved under account %q: %v (this is the #634 symptom — "+
			"SaveDeviceInfo rejects the raw margeAccountUUID as an invalid account ID)",
			expectedAccountID, err)
	}

	if deviceInfo.Name != "Kitchen SoundTouch" {
		t.Errorf("Name = %q, want %q", deviceInfo.Name, "Kitchen SoundTouch")
	}
}
