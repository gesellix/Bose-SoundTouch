package bmx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRadioBrowserSearch(t *testing.T) {
	// Mock RadioBrowser API
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[
			{
				"name": "Radio Paradise",
				"stationuuid": "123-456",
				"favicon": "http://example.com/favicon.png",
				"country": "USA",
				"tags": "eclectic,rock"
			}
		]`)
	}))
	defer ts.Close()

	// Use the mock server
	originalBaseURL := radioBrowserBaseURL
	radioBrowserBaseURL = ts.URL
	defer func() { radioBrowserBaseURL = originalBaseURL }()

	resp, err := RadioBrowserSearch("Paradise")
	if err != nil {
		t.Fatalf("RadioBrowserSearch failed: %v", err)
	}

	if len(resp.BmxSections) == 0 || len(resp.BmxSections[0].Items) == 0 {
		t.Fatal("expected items in response")
	}

	item := resp.BmxSections[0].Items[0]
	if item.Name != "Radio Paradise" {
		t.Errorf("expected name 'Radio Paradise', got %q", item.Name)
	}

	// The playback href must be the relative /stations/byuuid/<uuid> form so a
	// RADIO_BROWSER select resolves against the BMX-registry base URL (#479).
	if item.Links == nil || item.Links.BmxPlayback == nil {
		t.Fatal("expected a bmx_playback link on the station item")
	}

	if want := "/stations/byuuid/123-456"; item.Links.BmxPlayback.Href != want {
		t.Errorf("expected playback href %q, got %q", want, item.Links.BmxPlayback.Href)
	}
}

// makeStationsJSON returns a JSON array of n station objects.
func makeStationsJSON(n int) string {
	stations := make([]string, n)
	for i := 0; i < n; i++ {
		stations[i] = fmt.Sprintf(`{"name":"Station %d","stationuuid":"uuid-%d","favicon":"","country":"DE","tags":"pop"}`, i, i)
	}

	return "[" + strings.Join(stations, ",") + "]"
}

func TestRadioBrowserSearchPage_FullPage_HasNext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, makeStationsJSON(radioBrowserPageSize))
	}))
	defer ts.Close()

	originalBaseURL := radioBrowserBaseURL
	radioBrowserBaseURL = ts.URL
	defer func() { radioBrowserBaseURL = originalBaseURL }()

	resp, err := RadioBrowserSearchPage("test", 0)
	if err != nil {
		t.Fatalf("RadioBrowserSearchPage failed: %v", err)
	}

	if len(resp.BmxSections) == 0 {
		t.Fatal("expected sections")
	}

	section := resp.BmxSections[0]

	if len(section.Items) != radioBrowserPageSize {
		t.Errorf("expected %d items, got %d", radioBrowserPageSize, len(section.Items))
	}

	if section.Links == nil || section.Links.BmxNext == nil {
		t.Fatal("expected BmxNext link on full page")
	}

	if !strings.Contains(section.Links.BmxNext.Href, "cursor=") {
		t.Errorf("expected cursor in BmxNext href, got %q", section.Links.BmxNext.Href)
	}
}

func TestRadioBrowserSearchPage_ShortPage_NoNext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, makeStationsJSON(5)) // fewer than radioBrowserPageSize
	}))
	defer ts.Close()

	originalBaseURL := radioBrowserBaseURL
	radioBrowserBaseURL = ts.URL
	defer func() { radioBrowserBaseURL = originalBaseURL }()

	resp, err := RadioBrowserSearchPage("test", 0)
	if err != nil {
		t.Fatalf("RadioBrowserSearchPage failed: %v", err)
	}

	if len(resp.BmxSections) == 0 {
		t.Fatal("expected sections")
	}

	section := resp.BmxSections[0]

	if section.Links != nil && section.Links.BmxNext != nil {
		t.Errorf("expected no BmxNext link on short page, got %q", section.Links.BmxNext.Href)
	}
}

func TestRadioBrowserSearchNext_CursorRoundTrip(t *testing.T) {
	// Build a cursor manually to verify RadioBrowserSearchNext decodes it correctly.
	cursorData := radioBrowserCursor{Query: "jazz", NextOffset: 20}
	cursorJSON, err := json.Marshal(cursorData)
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(cursorJSON)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the offset was forwarded in the URL.
		if !strings.Contains(r.URL.RawQuery, "offset=20") {
			t.Errorf("expected offset=20 in query, got %q", r.URL.RawQuery)
		}

		if !strings.Contains(r.URL.RawQuery, "name=jazz") {
			t.Errorf("expected name=jazz in query, got %q", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, makeStationsJSON(3))
	}))
	defer ts.Close()

	originalBaseURL := radioBrowserBaseURL
	radioBrowserBaseURL = ts.URL
	defer func() { radioBrowserBaseURL = originalBaseURL }()

	resp, err := RadioBrowserSearchNext(encoded)
	if err != nil {
		t.Fatalf("RadioBrowserSearchNext failed: %v", err)
	}

	if len(resp.BmxSections) == 0 || len(resp.BmxSections[0].Items) != 3 {
		t.Errorf("expected 3 items, got response: %+v", resp)
	}
}

func TestRadioBrowserSearchNext_InvalidCursor(t *testing.T) {
	_, err := RadioBrowserSearchNext("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid cursor")
	}
}

func TestRadioBrowserSearchNext_EmptyQueryCursor(t *testing.T) {
	// A cursor with an empty query should be rejected.
	cursorData := radioBrowserCursor{Query: "", NextOffset: 20}
	cursorJSON, err := json.Marshal(cursorData)
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(cursorJSON)

	_, err = RadioBrowserSearchNext(encoded)
	if err == nil {
		t.Error("expected error for cursor with empty query")
	}
}

func TestRadioBrowserSearch_Real(t *testing.T) {
	if os.Getenv("RADIOBROWSER_INTEGRATION") == "" {
		t.Skip("skipping live network test; set RADIOBROWSER_INTEGRATION=1 to run")
	}
	query := "Deutschlandfunk Kultur"
	resp, err := RadioBrowserSearch(query)
	if err != nil {
		t.Fatalf("RadioBrowserSearch failed: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if len(resp.BmxSections) == 0 {
		t.Fatal("expected at least one section")
	}

	found := false
	for _, section := range resp.BmxSections {
		if section.Name == "Stations" && len(section.Items) > 0 {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected to find Stations section with items")
	}
}
