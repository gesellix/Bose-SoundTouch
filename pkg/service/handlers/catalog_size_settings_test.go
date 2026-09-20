package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// postSettings saves the settings form and returns the status, closing the
// body itself so no caller has to remember to.
func postSettings(t *testing.T, ts *httptest.Server, body map[string]any) int {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	res, err := http.Post(ts.URL+"/setup/settings", "application/json", bytes.NewBuffer(encoded))
	if err != nil {
		t.Fatalf("post: %v", err)
	}

	defer func() { _ = res.Body.Close() }()

	return res.StatusCode
}

// The catalog cap has three states on the wire, and each has to survive the
// settings form: omitted preserves, "" means "use the default", a number sets
// the cap, and 0 switches the catalog off (issue 754).
func TestCatalogSizeSettingRoundTrip(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	r, _ := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)

	defer ts.Close()

	if status := postSettings(t, ts, map[string]any{
		"server_url": "http://127.0.0.1:8000", "catalog_size": "250",
	}); status != http.StatusOK {
		t.Fatalf("expected OK, got %d", status)
	}

	persisted, err := ds.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}

	if persisted.CatalogSize == nil || *persisted.CatalogSize != 250 {
		t.Fatalf("CatalogSize = %v, want 250", persisted.CatalogSize)
	}

	// A form that does not know about the field must not reset it. That is
	// the shape of issue #589, and the reason the value is a pointer.
	if status := postSettings(t, ts, map[string]any{
		"server_url": "http://127.0.0.1:8000",
	}); status != http.StatusOK {
		t.Fatalf("expected OK, got %d", status)
	}

	persisted, _ = ds.GetSettings()
	if persisted.CatalogSize == nil || *persisted.CatalogSize != 250 {
		t.Fatalf("an omitted catalog_size reset the stored value: %v", persisted.CatalogSize)
	}

	// Zero is a real choice: the catalog is off, which is not the same as
	// never having configured one.
	if status := postSettings(t, ts, map[string]any{
		"server_url": "http://127.0.0.1:8000", "catalog_size": "0",
	}); status != http.StatusOK {
		t.Fatalf("expected OK, got %d", status)
	}

	persisted, _ = ds.GetSettings()
	if persisted.CatalogSize == nil || *persisted.CatalogSize != 0 {
		t.Fatalf("CatalogSize = %v, want it switched off", persisted.CatalogSize)
	}

	// Clearing the box returns to the default, which is nil rather than a
	// number, so a later change of default reaches installs that never chose.
	if status := postSettings(t, ts, map[string]any{
		"server_url": "http://127.0.0.1:8000", "catalog_size": "",
	}); status != http.StatusOK {
		t.Fatalf("expected OK, got %d", status)
	}

	persisted, _ = ds.GetSettings()
	if persisted.CatalogSize != nil {
		t.Fatalf("CatalogSize = %v, want unset", persisted.CatalogSize)
	}
}

func TestCatalogSizeSettingRejectsNonsense(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	r, _ := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)

	defer ts.Close()

	for _, value := range []string{"-1", "many"} {
		if status := postSettings(t, ts, map[string]any{
			"server_url": "http://127.0.0.1:8000", "catalog_size": value,
		}); status != http.StatusBadRequest {
			t.Errorf("catalog_size=%q: expected 400, got %d", value, status)
		}
	}

	if persisted, _ := ds.GetSettings(); persisted.CatalogSize != nil {
		t.Errorf("a rejected value was stored anyway: %v", persisted.CatalogSize)
	}
}
