package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func newLandingServer(t *testing.T, landing string) *Server {
	t.Helper()

	ds := datastore.NewDataStore(t.TempDir())
	if landing != "" {
		if err := ds.SaveSettings(datastore.Settings{
			ServerURL:      "http://127.0.0.1:8000",
			DefaultLanding: landing,
		}); err != nil {
			t.Fatalf("SaveSettings: %v", err)
		}
	}

	return NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)
}

func htmlGet(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Accept", "text/html")

	return req
}

// TestHandleRootChooser: with no configured default, a browser at "/"
// gets the neutral chooser linking to both surfaces.
func TestHandleRootChooser(t *testing.T) {
	server := newLandingServer(t, "")

	rec := httptest.NewRecorder()
	server.HandleRoot(rec, htmlGet("/"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{`href="/app"`, `href="/admin"`, "Player"} {
		if !strings.Contains(body, want) {
			t.Errorf("chooser body missing %q", want)
		}
	}

	// The chooser is not the admin console.
	if strings.Contains(body, "tab-settings") {
		t.Error("chooser unexpectedly served the admin console")
	}
}

// TestHandleRootRedirects: default_landing app/admin redirects "/" to the
// matching surface for browsers.
func TestHandleRootRedirects(t *testing.T) {
	for landing, want := range map[string]string{"app": "/app", "admin": "/admin"} {
		server := newLandingServer(t, landing)

		rec := httptest.NewRecorder()
		server.HandleRoot(rec, htmlGet("/"))

		if rec.Code != http.StatusFound {
			t.Errorf("landing=%q: status = %d; want 302", landing, rec.Code)
		}

		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("landing=%q: Location = %q; want %q", landing, got, want)
		}
	}
}

// TestHandleRootChooserOverride: "?chooser" forces the chooser even when a
// default redirect is configured, so the hub stays reachable.
func TestHandleRootChooserOverride(t *testing.T) {
	for _, landing := range []string{"app", "admin"} {
		server := newLandingServer(t, landing)

		rec := httptest.NewRecorder()
		server.HandleRoot(rec, htmlGet("/?chooser"))

		if rec.Code != http.StatusOK {
			t.Errorf("landing=%q with ?chooser: status = %d; want 200 (no redirect)", landing, rec.Code)
		}

		if !strings.Contains(rec.Body.String(), `href="/admin"`) {
			t.Errorf("landing=%q with ?chooser: body is not the chooser", landing)
		}
	}
}

// TestHandleRootJSONIgnoresLanding: API/speaker clients (non-HTML Accept)
// always get the version JSON, never a landing redirect, regardless of
// the configured default.
func TestHandleRootJSONIgnoresLanding(t *testing.T) {
	server := newLandingServer(t, "app")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "application/json")

	rec := httptest.NewRecorder()
	server.HandleRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (not a redirect)", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
}

// TestHandleAdminServesConsole: /admin serves the admin console itself.
func TestHandleAdminServesConsole(t *testing.T) {
	server := newLandingServer(t, "")

	rec := httptest.NewRecorder()
	server.HandleAdmin(rec, htmlGet("/admin"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "tab-settings") {
		t.Error("admin body did not contain the console markup")
	}
}
