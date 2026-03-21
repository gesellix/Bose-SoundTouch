package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestParityMismatchReproduction_New(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-parity-reproduce-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	account := "1234567"
	deviceID := "001122334455"

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Upstream example payload for POST /recent
	payload := `
<recent>
  <contentItemType>stationurl</contentItemType>
  <lastplayedat>2026-03-14T12:50:10.000+00:00</lastplayedat>
  <location>/v1/playback/station/s104811</location>
  <name>1LIVE Chillout</name>
  <source id="14774275" type="Audio">
    <createdOn>2017-07-20T16:43:48.000+00:00</createdOn>
    <credential type="token">dummy-token-base64</credential>
    <name></name>
    <sourceproviderid>25</sourceproviderid>
    <sourcename></sourcename>
    <sourceSettings/>
    <updatedOn>2017-07-20T16:43:48.000+00:00</updatedOn>
    <username></username>
  </source>
  <sourceid>14774275</sourceid>
  <updatedOn>2026-03-14T12:50:14.221+00:00</updatedOn>
</recent>`

	t.Run("POST /recent should learn source details and respond with parity", func(t *testing.T) {
		res, err := http.Post(ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/recent", "application/xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d", res.StatusCode)
		}

		body, _ := io.ReadAll(res.Body)
		bodyStr := string(body)

		t.Logf("Response Body:\n%s\n", bodyStr)

		// Verification points:
		// 1. Standalone="yes"
		if !strings.Contains(bodyStr, `standalone="yes"`) {
			t.Errorf("Missing standalone=\"yes\"")
		}

		// 2. Millisecond precision in dates
		if !strings.Contains(bodyStr, ".000+00:00") && !strings.Contains(bodyStr, ".221+00:00") {
			// Note: FormatTime always uses .000+00:00 for now, but it's acceptable.
			// The key is it MUST have milliseconds and +00:00 offset.
			t.Errorf("Date format mismatch, expected .000+00:00. Body: %s", bodyStr)
		}

		// 3. SourceProviderID learned (25)
		if !strings.Contains(bodyStr, `sourceproviderid="25"`) {
			t.Errorf("SourceProviderID was not learned from POST, expected 25 in attribute. Body: %s", bodyStr)
		}

		// 4. Credential learned
		if !strings.Contains(bodyStr, `secret="dummy-token-base64"`) {
			t.Errorf("Secret was not learned from POST in attribute. Body: %s", bodyStr)
		}

		// 6. Source CreatedOn/UpdatedOn learned
		if !strings.Contains(bodyStr, `createdOn="2017-07-20T16:43:48.000+00:00"`) {
			t.Errorf("Source CreatedOn was not learned from POST in attribute. Body: %s", bodyStr)
		}
	})

	t.Run("Subsequent GET /recents should also show learned source details", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/streaming/account/" + account + "/device/" + deviceID + "/recent")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		body, _ := io.ReadAll(res.Body)
		bodyStr := string(body)

		if !strings.Contains(bodyStr, `sourceproviderid="25"`) {
			t.Errorf("GET /recents missing learned sourceproviderid 25 in attribute. Body: %s", bodyStr)
		}
	})
}
