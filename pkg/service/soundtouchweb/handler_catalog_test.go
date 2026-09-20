package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/catalog"
)

func decodeCatalog(t *testing.T, w *httptest.ResponseRecorder) CatalogPayload {
	t.Helper()

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool           `json:"success"`
		Data    CatalogPayload `json:"data"`
	}

	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success, got %s", w.Body.String())
	}

	return resp.Data
}

func TestHandleCatalogReturnsTheEntries(t *testing.T) {
	app := NewWebApp()
	app.CatalogEntries = func() []catalog.Entry {
		return []catalog.Entry{{
			Source: "TUNEIN", Location: "s12345", Name: "WDR 2",
			Origin: catalog.OriginPreset, LastSeen: time.Now().UTC(),
		}}
	}

	w := httptest.NewRecorder()
	app.HandleCatalog(w, httptest.NewRequest("GET", "/api/control/catalog", nil))

	payload := decodeCatalog(t, w)
	if !payload.Available {
		t.Error("expected the catalog to be reported as available")
	}

	if len(payload.Entries) != 1 || payload.Entries[0].Name != "WDR 2" {
		t.Fatalf("unexpected entries: %+v", payload.Entries)
	}
}

// Standalone soundtouch-player has no datastore behind it. "No catalog here"
// has to be tellable apart from "nothing seen yet", or the player would offer
// a pick list it can never fill.
func TestHandleCatalogWithoutAServiceReportsUnavailable(t *testing.T) {
	app := NewWebApp()

	w := httptest.NewRecorder()
	app.HandleCatalog(w, httptest.NewRequest("GET", "/api/control/catalog", nil))

	payload := decodeCatalog(t, w)
	if payload.Available {
		t.Error("expected available=false with no catalog hook wired")
	}

	if payload.Entries == nil {
		t.Error("entries must serialise as [] rather than null")
	}
}
