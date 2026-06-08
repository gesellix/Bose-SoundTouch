package stations

import (
	"testing"
)

func TestResolveContentItem_TuneIn(t *testing.T) {
	item := PlayItem{
		Provider: ProviderTuneIn,
		Location: "/v1/playback/station/s123",
		Name:     "Jazz FM",
		Type:     "stationurl",
	}

	ci := ResolveContentItem(item)

	if ci.Source != "TUNEIN" {
		t.Errorf("expected Source TUNEIN, got %q", ci.Source)
	}

	if ci.Type != "stationurl" {
		t.Errorf("expected Type stationurl, got %q", ci.Type)
	}

	if ci.Location != item.Location {
		t.Errorf("expected Location %q, got %q", item.Location, ci.Location)
	}

	if ci.ItemName != "Jazz FM" {
		t.Errorf("expected ItemName Jazz FM, got %q", ci.ItemName)
	}

	if !ci.IsPresetable {
		t.Error("expected IsPresetable true")
	}
}

func TestResolveContentItem_TuneIn_DefaultType(t *testing.T) {
	// When Type is empty it should default to "stationurl".
	item := PlayItem{
		Provider: ProviderTuneIn,
		Location: "/v1/playback/station/s456",
		Name:     "Rock Radio",
	}

	ci := ResolveContentItem(item)

	if ci.Type != "stationurl" {
		t.Errorf("expected default Type stationurl, got %q", ci.Type)
	}
}

func TestResolveContentItem_TuneIn_ContainerArt(t *testing.T) {
	item := PlayItem{
		Provider:     ProviderTuneIn,
		Location:     "/v1/playback/station/s789",
		Name:         "Pop Radio",
		ContainerArt: "http://example.com/art.png",
	}

	ci := ResolveContentItem(item)

	if ci.ContainerArt != "http://example.com/art.png" {
		t.Errorf("expected ContainerArt set, got %q", ci.ContainerArt)
	}
}

func TestResolveContentItem_RadioBrowser(t *testing.T) {
	item := PlayItem{
		Provider: ProviderRadioBrowser,
		Location: "/stations/byuuid/abc-123",
		Name:     "Radio Paradise",
	}

	ci := ResolveContentItem(item)

	if ci.Source != "RADIO_BROWSER" {
		t.Errorf("expected Source RADIO_BROWSER, got %q", ci.Source)
	}

	if ci.Type != "stationurl" {
		t.Errorf("expected Type stationurl, got %q", ci.Type)
	}

	if ci.Location != item.Location {
		t.Errorf("expected Location %q, got %q", item.Location, ci.Location)
	}

	if ci.ItemName != "Radio Paradise" {
		t.Errorf("expected ItemName Radio Paradise, got %q", ci.ItemName)
	}

	if !ci.IsPresetable {
		t.Error("expected IsPresetable true")
	}
}

// TestResolveContentItem_SourceAccountGuard_EchoDropped verifies that a
// SourceAccount equal to the ContentItem Source (the placeholder value
// speakers echo back) is NOT forwarded.
func TestResolveContentItem_SourceAccountGuard_EchoDropped(t *testing.T) {
	// TuneIn: source name == "TUNEIN"; echoed SourceAccount must be dropped.
	item := PlayItem{
		Provider:      ProviderTuneIn,
		Location:      "/v1/playback/station/s111",
		Name:          "Example",
		SourceAccount: "TUNEIN", // echoed placeholder
	}

	ci := ResolveContentItem(item)

	if ci.SourceAccount != "" {
		t.Errorf("expected SourceAccount dropped, got %q", ci.SourceAccount)
	}
}

// TestResolveContentItem_SourceAccountGuard_RealAccountKept verifies that a
// real (non-placeholder) SourceAccount is forwarded to the ContentItem.
func TestResolveContentItem_SourceAccountGuard_RealAccountKept(t *testing.T) {
	item := PlayItem{
		Provider:      ProviderTuneIn,
		Location:      "/v1/playback/station/s222",
		Name:          "Example",
		SourceAccount: "real-user-token-xyz",
	}

	ci := ResolveContentItem(item)

	if ci.SourceAccount != "real-user-token-xyz" {
		t.Errorf("expected SourceAccount kept, got %q", ci.SourceAccount)
	}
}

// TestResolveContentItem_SourceAccountGuard_RadioBrowserEchoDropped checks the
// guard for RadioBrowser where Source is "RADIO_BROWSER".
func TestResolveContentItem_SourceAccountGuard_RadioBrowserEchoDropped(t *testing.T) {
	// SourceAccount == "RADIO_BROWSER" is the echo value — must be dropped.
	item := PlayItem{
		Provider:      ProviderRadioBrowser,
		Location:      "/stations/byuuid/xyz",
		Name:          "Test",
		SourceAccount: "RADIO_BROWSER",
	}

	ci := ResolveContentItem(item)

	if ci.SourceAccount != "" {
		t.Errorf("expected SourceAccount dropped for RADIO_BROWSER source, got %q", ci.SourceAccount)
	}
}

func TestSearch_UnknownProvider(t *testing.T) {
	_, err := Search("bogus", "query")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestSearchNext_UnknownProvider(t *testing.T) {
	_, err := SearchNext("bogus", "cursor")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestNavigate_RadioBrowserNotSupported(t *testing.T) {
	_, err := Navigate(ProviderRadioBrowser, "")
	if err == nil {
		t.Error("expected error for RadioBrowser navigate")
	}
}

func TestNavigate_UnknownProvider(t *testing.T) {
	_, err := Navigate("bogus", "")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}
