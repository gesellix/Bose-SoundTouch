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
  <contentItemType>tracklisturl</contentItemType>
  <lastplayedat>2026-03-14T21:33:22.000+00:00</lastplayedat>
  <location>/playback/container/c3BvdGlmeTphbGJ1bTo3RjUwdWg3b0dpdG1BRVNjUktWNnBE</location>
  <name>Terminal Caribe</name>
  <sourceid>10863533</sourceid>
</recent>`

		// 1. POST /recent
		res, err := http.Post(ts.URL+"/streaming/account/"+account+"/device/"+deviceID+"/recent", "application/xml", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d", res.StatusCode)
		}

		postBody, _ := io.ReadAll(res.Body)
		postBodyStr := string(postBody)

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

		// 3. Verify consistency
		// Use a whitespace-insensitive comparison
		clean := func(s string) string {
			if strings.HasPrefix(s, "<?xml") {
				if idx := strings.Index(s, "?>"); idx != -1 {
					s = s[idx+2:]
				}
			}
			var result strings.Builder
			inTag := false
			for i := 0; i < len(s); i++ {
				c := s[i]
				if c == '<' {
					inTag = true
					result.WriteByte(c)
				} else if c == '>' {
					inTag = false
					result.WriteByte(c)
				} else if inTag {
					result.WriteByte(c)
				} else {
					if c != ' ' && c != '\n' && c != '\r' && c != '\t' {
						result.WriteByte(c)
					}
				}
			}
			return strings.TrimSpace(result.String())
		}

		if !strings.Contains(clean(getRecentsStr), clean(postBodyStr)) {
			t.Errorf("GET /recents does not contain the same XML as POST /recent response.\nPOST: %s\nGET: %s", postBodyStr, getRecentsStr)
		}

		// 4. Verify source persistence
		// Check if source 10863533 was learned and is now in Sources.xml
		sources, err := ds.GetConfiguredSources(account, deviceID)
		if err != nil {
			t.Errorf("Failed to get configured sources: %v", err)
		}
		found := false
		for _, s := range sources {
			if s.ID == "10863533" {
				found = true
				if s.SourceKeyType != "SPOTIFY" {
					t.Errorf("Learned source should be SPOTIFY based on location, got %s", s.SourceKeyType)
				}
				break
			}
		}
		if !found {
			t.Errorf("Source 10863533 was not learned and persisted")
		}
	})
}
