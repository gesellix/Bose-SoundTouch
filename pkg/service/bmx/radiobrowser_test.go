package bmx

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
}

func TestRadioBrowserSearch_Real(t *testing.T) {
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
