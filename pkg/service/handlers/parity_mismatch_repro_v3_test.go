package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestParityMismatchReproduction_V3(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "marge-test")
	defer os.RemoveAll(tempDir)
	ds := datastore.NewDataStore(tempDir)

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Upstream input for POST /recent (extracted from user description)
	// We'll use the same source metadata as provided in the upstream response
	// to see if we can "learn" it and echo it back correctly.
	requestBody := `
<recent>
  <contentItemType>stationurl</contentItemType>
  <location>/v1/playback/station/s104811</location>
  <name>1LIVE Chillout</name>
  <source id="14774275" type="Audio">
    <createdOn>2017-07-20T16:43:48.000+00:00</createdOn>
    <credential type="token">dummy-token-base64</credential>
    <sourceproviderid>25</sourceproviderid>
    <sourcename></sourcename>
    <sourceSettings/>
    <updatedOn>2017-07-20T16:43:48.000+00:00</updatedOn>
  </source>
  <sourceid>14774275</sourceid>
</recent>`

	account := "1234567"
	device := "001122334455"
	url := fmt.Sprintf("%s/streaming/account/%s/device/%s/recent", ts.URL, account, device)

	t.Run("POST /recent and check parity", func(t *testing.T) {
		res, err := http.Post(url, "application/xml", strings.NewReader(requestBody))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %v", res.Status)
		}

		body, _ := io.ReadAll(res.Body)
		bodyStr := string(body)
		t.Logf("POST /recent Response:\n%s\n", bodyStr)

		if !strings.Contains(bodyStr, constants.XMLHeader) {
			t.Error("Missing XML declaration with standalone=\"yes\"")
		}

		// 2. Large ID (YYMMDDxxx format)
		prefix := time.Now().UTC().Format("060102")
		if !strings.Contains(bodyStr, fmt.Sprintf(`id="%s`, prefix)) {
			t.Errorf("Recent ID missing expected prefix %s. Body: %s", prefix, bodyStr)
		}

		// 3. Date Formatting (.000+00:00)
		if !strings.Contains(bodyStr, `.000+00:00`) {
			t.Error("Dates are missing milliseconds or incorrect offset")
		}

		// 4. Source Learning
		// Check for provider ID 25
		if !strings.Contains(bodyStr, `sourceproviderid="25"`) {
			t.Errorf("Source provider ID mismatch: expected 25 for TuneIn in attribute. Body: %s", bodyStr)
		}
		// Check for credential
		if !strings.Contains(bodyStr, `secret="dummy-token-base64"`) {
			t.Errorf("Secret value was not preserved in attribute. Body: %s", bodyStr)
		}

		// 6. Indentation check (2 spaces)
		if !strings.Contains(bodyStr, "\n  <contentItem source=\"TUNEIN\"") {
			t.Errorf("Incorrect indentation for contentItem: expected 2 spaces. Body: %s", bodyStr)
		}
	})

	t.Run("Verify GET /recents consistency", func(t *testing.T) {
		recentsUrl := fmt.Sprintf("%s/streaming/account/%s/device/%s/recent", ts.URL, account, device)
		res, err := http.Get(recentsUrl)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		body, _ := io.ReadAll(res.Body)
		bodyStr := string(body)

		t.Logf("GET /recents Local Response:\n%s\n", bodyStr)

		if !strings.Contains(bodyStr, `sourceproviderid="25"`) {
			t.Error("Source provider ID missing in GET /recents")
		}
	})
}

func TestXMLWhitespaceInsensitivity(t *testing.T) {
	s := &Server{}
	local := []byte(constants.XMLHeader + `
<recent id="123">
  <name>Test</name>
</recent>`)
	upstream := []byte(constants.XMLHeader + `
<recent id="123">
    <name>Test</name>
</recent>`)

	if !s.compareXMLWhitespaceInsensitive(local, upstream) {
		t.Error("compareXMLWhitespaceInsensitive failed for simple whitespace difference")
	}

	upstreamNoSpaces := []byte(constants.XMLHeader + `<recent id="123"><name>Test</name></recent>`)
	if !s.compareXMLWhitespaceInsensitive(local, upstreamNoSpaces) {
		t.Error("compareXMLWhitespaceInsensitive failed for no-whitespace upstream")
	}
}
