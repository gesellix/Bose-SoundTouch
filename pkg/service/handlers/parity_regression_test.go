package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestMargeParityRegressions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-parity-regressions-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	account := "1234567"
	deviceID := "001122334455"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", deviceID)
	os.MkdirAll(deviceDir, 0755)

	// Mock Sources.xml matching the upstream example (source id 14774275)
	// One with "Other" and one with a specific name.
	sourcesXML := `
<sources>
  <source id="14774275" displayName="Other" secret="" secretType="Audio">
    <sourceKey type="TUNEIN" account=""/>
  </source>
  <source id="SPOT1" displayName="My Spotify" secret="token123" secretType="Audio">
    <sourceKey type="SPOTIFY" account="user123"/>
  </source>
</sources>`
	os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644)
	os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte("<recents></recents>"), 0644)

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	t.Run("POST recent with Other source - displayName should be 'Other' in attribute", func(t *testing.T) {
		payload := `
<recent>
  <contentItemType>stationurl</contentItemType>
  <lastplayedat>2026-03-14T12:50:10.000+00:00</lastplayedat>
  <location>/v1/playback/station/s104811</location>
  <name>1LIVE Chillout</name>
  <sourceid>14774275</sourceid>
</recent>`

		res, err := http.Post(ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/recent", "application/xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		body, _ := io.ReadAll(res.Body)
		bodyStr := string(body)

		// Check for standalone="yes"
		if !strings.Contains(bodyStr, `standalone="yes"`) {
			t.Errorf("Response missing standalone=\"yes\"")
		}

		// Check for displayName when it's "Other"
		if !strings.Contains(bodyStr, `displayName="Other"`) {
			t.Errorf("Expected displayName=\"Other\", but got: %s", bodyStr)
		}

		// Check for date format (should have .000+00:00)
		if !strings.Contains(bodyStr, ".000+00:00") {
			t.Errorf("Response date format mismatch, expected .000+00:00. Body: %s", bodyStr)
		}
	})

	t.Run("POST recent with named source - displayName should be preserved in attribute", func(t *testing.T) {
		payload := `
<recent>
  <contentItemType>track</contentItemType>
  <lastplayedat>2026-03-14T12:50:10.000+00:00</lastplayedat>
  <location>spotify:track:123</location>
  <name>Test Song</name>
  <sourceid>SPOT1</sourceid>
</recent>`

		res, err := http.Post(ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/recent", "application/xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		body, _ := io.ReadAll(res.Body)
		bodyStr := string(body)

		if !strings.Contains(bodyStr, `displayName="My Spotify"`) {
			t.Errorf("Expected displayName=\"My Spotify\", body: %s", bodyStr)
		}
	})
}
