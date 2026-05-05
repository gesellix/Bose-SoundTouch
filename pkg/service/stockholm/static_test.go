package stockholm

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- injectBootstrap ----

func TestInjectBootstrap_InjectsBeforeHead(t *testing.T) {
	html := []byte(`<html><head><title>T</title></head><body></body></html>`)
	state := NewNativeState(t.TempDir())
	cfg := &Config{AppVersion: "27.0"}

	got := string(injectBootstrap(html, state, cfg))

	if !strings.Contains(got, "window.StockholmBrowserBootstrap") {
		t.Error("expected bootstrap script to be injected")
	}

	scriptEnd := strings.Index(got, "</script>")
	headEnd := strings.Index(got, "</head>")

	if scriptEnd < 0 || headEnd < 0 || scriptEnd > headEnd {
		t.Error("expected bootstrap script to appear before </head>")
	}
}

func TestInjectBootstrap_Idempotent(t *testing.T) {
	html := []byte(`<html><head><script>window.StockholmBrowserBootstrap = {};</script></head><body></body></html>`)
	state := NewNativeState(t.TempDir())
	cfg := &Config{}

	got := injectBootstrap(html, state, cfg)

	if strings.Count(string(got), "StockholmBrowserBootstrap") != 1 {
		t.Error("expected bootstrap not to be injected a second time")
	}
}

func TestInjectBootstrap_NoHeadTag_ReturnsUnchanged(t *testing.T) {
	html := []byte(`<html><body>no head here</body></html>`)
	state := NewNativeState(t.TempDir())
	cfg := &Config{}

	got := injectBootstrap(html, state, cfg)

	if string(got) != string(html) {
		t.Error("expected html to be returned unchanged when </head> is absent")
	}
}

// ---- isBootstrapTarget ----

func TestIsBootstrapTarget(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"index.html", true},
		{"INDEX.HTML", true},
		{"setup/index.html", true},
		{"SETUP/INDEX.HTML", true},
		{"js/app.js", false},
		{"css/main.css", false},
		{"", false},
	}

	for _, tc := range cases {
		if got := isBootstrapTarget(tc.path); got != tc.want {
			t.Errorf("isBootstrapTarget(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// ---- resolveStaticFile ----

func TestResolveStaticFile_Normal(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "app.js"), []byte("js"), 0644)

	file, rel, err := resolveStaticFile("/app.js", dir)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(file, "app.js") {
		t.Errorf("expected file path to end with app.js, got %q", file)
	}

	if rel != "app.js" {
		t.Errorf("expected rel = %q, got %q", "app.js", rel)
	}
}

func TestResolveStaticFile_RootMapsToIndexHTML(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>"), 0644)

	file, rel, err := resolveStaticFile("/", dir)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(file, "index.html") {
		t.Errorf("expected file path to end with index.html, got %q", file)
	}

	if rel != "index.html" {
		t.Errorf("expected rel = %q, got %q", "index.html", rel)
	}
}

func TestResolveStaticFile_DirectoryMapsToIndexHTML(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "setup")
	_ = os.MkdirAll(subDir, 0755)
	_ = os.WriteFile(filepath.Join(subDir, "index.html"), []byte("<html>"), 0644)

	file, rel, err := resolveStaticFile("/setup", dir)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(file, filepath.Join("setup", "index.html")) {
		t.Errorf("expected file path to end with setup/index.html, got %q", file)
	}

	if rel != "setup/index.html" {
		t.Errorf("expected rel = %q, got %q", "setup/index.html", rel)
	}
}

func TestResolveStaticFile_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()

	_, _, err := resolveStaticFile("/../../../etc/passwd", dir)

	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
}

// ---- ServeStatic integration ----

func TestServeStatic_ServesHTMLWithBootstrap(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"),
		[]byte(`<html><head></head><body></body></html>`), 0644)

	state := NewNativeState(t.TempDir())
	cfg := &Config{AppVersion: "27.0"}
	backendCfg := &BackendConfig{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	ServeStatic(rec, req, dir, backendCfg, state, cfg)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "StockholmBrowserBootstrap") {
		t.Error("expected bootstrap to be injected in served HTML")
	}
}

func TestServeStatic_NotFound(t *testing.T) {
	dir := t.TempDir()
	state := NewNativeState(t.TempDir())
	cfg := &Config{}
	backendCfg := &BackendConfig{}

	req := httptest.NewRequest(http.MethodGet, "/nonexistent.js", nil)
	rec := httptest.NewRecorder()

	ServeStatic(rec, req, dir, backendCfg, state, cfg)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestServeStatic_HeadReturnsNoBody(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"),
		[]byte(`<html><head></head><body></body></html>`), 0644)

	state := NewNativeState(t.TempDir())
	cfg := &Config{}
	backendCfg := &BackendConfig{}

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rec := httptest.NewRecorder()

	ServeStatic(rec, req, dir, backendCfg, state, cfg)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for HEAD, got %d bytes", rec.Body.Len())
	}
}

func TestServeStatic_FrontendLoggingCookieSet(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"),
		[]byte(`<html><head></head><body></body></html>`), 0644)

	state := NewNativeState(t.TempDir())
	cfg := &Config{}
	backendCfg := &BackendConfig{FrontendLoggingLevel: 2}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	ServeStatic(rec, req, dir, backendCfg, state, cfg)

	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "stockholmFrontendLoggingLevel=2") {
		t.Errorf("expected logging cookie with level 2, got %q", cookie)
	}
}
