package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestMargeRecentConsistencyAndIDParity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-recent-parity-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	account := "1234567"
	deviceID := "001122334455"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", deviceID)
	os.MkdirAll(deviceDir, 0755)

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	defer ts.Close()

	t.Run("POST recent creates consistent IDs and persists unknown sources", func(t *testing.T) {
		payload := `
<recent>
  <contentItemType>stationurl</contentItemType>
  <lastplayedat>2026-03-29T21:33:00+00:00</lastplayedat>
  <location>/v1/playback/station/s166521</location>
  <name>SMOOTH JAZZ</name>
  <sourceid>14774275</sourceid>
</recent>`

		expectedToken := datastore.GenerateSerialSecret("tunein")
		// Pre-configure source 14774275 as TUNEIN (ID 25)
		ds.SaveConfiguredSources(account, deviceID, []models.ConfiguredSource{
			{
				ID:               "14774275",
				SourceProviderID: "25",
				Type:             "Audio",
				DisplayName:      "TuneIn",
				Secret:           expectedToken,
				SecretType:       "token",
				SourceKey: struct {
					Type    string `xml:"type,attr"`
					Account string `xml:"account,attr"`
				}{
					Type: "TUNEIN",
				},
			},
		})

		// 1. POST /recent
		res, err := http.Post(ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/recent", "application/xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		postBody, _ := io.ReadAll(res.Body)
		postBodyStr := string(postBody)

		if res.StatusCode != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d. Body: %s", res.StatusCode, postBodyStr)
		}

		// Verify constant token for TUNEIN
		if !strings.Contains(postBodyStr, expectedToken) {
			t.Errorf("Response missing expected constant token for TuneIn. Body: %s", postBodyStr)
		}
		if !strings.Contains(postBodyStr, `<credential type="token">`) {
			t.Errorf("Response missing expected credential tag for TuneIn. Body: %s", postBodyStr)
		}

		// Verify ID format: YYMMDDXXX (9 digits)
		// Today's prefix:
		prefix := time.Now().UTC().Format("060102")
		idPattern := fmt.Sprintf(`id="%s`, prefix)
		if !strings.Contains(postBodyStr, idPattern) {
			t.Errorf("Response ID missing expected prefix %s. Body: %s", prefix, postBodyStr)
		}

		// Extract ID
		startIdx := strings.Index(postBodyStr, `id="`) + 4
		endIdx := strings.Index(postBodyStr[startIdx:], `"`) + startIdx
		recentID := postBodyStr[startIdx:endIdx]

		idInt, err := strconv.Atoi(recentID)
		if err != nil {
			t.Errorf("Recent ID is not an integer: %s", recentID)
		} else if idInt > 2147483647 {
			t.Errorf("Recent ID exceeds 32-bit signed integer range: %d", idInt)
		}

		// 2. GET /recents
		res2, err := http.Get(ts.URL + "/streaming/account/" + account + "/device/" + deviceID + "/recent")
		if err != nil {
			t.Fatal(err)
		}
		defer res2.Body.Close()

		getRecentsBody, _ := io.ReadAll(res2.Body)
		getRecentsStr := string(getRecentsBody)

		// 3. Verify consistency (Content identity, not structural XML identity)
		// POST response is flat, GET response is nested ServiceRecent.
		if !strings.Contains(getRecentsStr, `id="`+recentID+`"`) {
			t.Errorf("GET /recents missing ID %s. Body: %s", recentID, getRecentsStr)
		}
		if !strings.Contains(getRecentsStr, `SMOOTH JAZZ`) {
			t.Errorf("GET /recents missing Name 'SMOOTH JAZZ'. Body: %s", getRecentsStr)
		}
		if !strings.Contains(getRecentsStr, `<itemName>SMOOTH JAZZ</itemName>`) {
			t.Errorf("GET /recents should use nested <itemName> for ServiceRecent. Body: %s", getRecentsStr)
		}

		// 4. Verify source persistence
		// Check if source 14774275 was learned and is now in Sources.xml
		sources, err := ds.GetConfiguredSources(account, deviceID)
		if err != nil {
			t.Errorf("Failed to get configured sources: %v", err)
		}
		found := false
		for _, s := range sources {
			if s.ID == "14774275" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Source 14774275 was not learned and persisted")
		}
	})
}
