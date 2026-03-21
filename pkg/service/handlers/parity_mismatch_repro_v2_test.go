package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestParityMismatchReproduction_V2(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-parity-repro-v2-*")
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

	t.Run("POST /recent parity with upstream example", func(t *testing.T) {
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
  <sourceSettings></sourceSettings>
  <updatedOn>2017-07-20T16:43:48.000+00:00</updatedOn>
  <username></username>
</source>
<sourceid>14774275</sourceid>
</recent>`

		res, err := http.Post(ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/recent", "application/xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		body, _ := io.ReadAll(res.Body)
		bodyStr := string(body)

		if !strings.HasPrefix(bodyStr, constants.XMLHeader) {
			t.Errorf("Missing or incorrect XML declaration: %s", bodyStr)
		}

		if !strings.Contains(bodyStr, `id="`) {
			t.Errorf("Missing recent id attribute")
		}

		if !strings.Contains(bodyStr, ".000+00:00") {
			t.Errorf("Date format mismatch. Expected .000+00:00. Body: %s", bodyStr)
		}

		if !strings.Contains(bodyStr, `sourceproviderid="25"`) {
			t.Errorf("sourceproviderid mismatch. Expected 25 in attribute. Body: %s", bodyStr)
		}

		if !strings.Contains(bodyStr, `secret="dummy-token-base64"`) {
			t.Errorf("Secret value mismatch in attribute. Body: %s", bodyStr)
		}

		if !strings.Contains(bodyStr, "<lastplayedat>2026-03-14T12:50:10.000+00:00</lastplayedat>") {
			t.Errorf("lastplayedat mismatch. Body: %s", bodyStr)
		}
	})
}
