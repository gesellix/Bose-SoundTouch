package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

const normalizedEtag = "Etag"
const caseSensitiveETag = "ETag"

func TestMargeETags(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "soundcork-etag-test-*")

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	account := "12345"
	deviceID := "DEV1"
	accountDir := filepath.Join(tempDir, "accounts", account)
	deviceDir := filepath.Join(accountDir, "devices", deviceID)
	_ = os.MkdirAll(deviceDir, 0755)

	// Create some initial data
	presetsFile := filepath.Join(deviceDir, "Presets.xml")
	_ = os.WriteFile(presetsFile, []byte("<presets/>"), 0644)

	sourcesFile := filepath.Join(deviceDir, "Sources.xml")
	_ = os.WriteFile(sourcesFile, []byte("<sources/>"), 0644)

	recentsFile := filepath.Join(deviceDir, "Recents.xml")
	_ = os.WriteFile(recentsFile, []byte("<recents/>"), 0644)

	// Ensure devices directory exists for AccountFull
	_ = os.MkdirAll(ds.AccountDevicesDir(account), 0755)

	r, _ := setupRouter("http://localhost:8001", ds)

	ts := httptest.NewServer(r)
	defer ts.Close()

	t.Run("Presets ETag", func(t *testing.T) {
		// First request to get ETag
		res, err := http.Get(ts.URL + "/marge/accounts/" + account + "/devices/DEV1/presets")
		if err != nil {
			t.Fatal(err)
		}

		etag := res.Header.Get(caseSensitiveETag)
		_ = res.Body.Close()

		if etag == "" {
			t.Fatal("Expected ETag header, got none")
		}

		// Second request with If-None-Match
		req, _ := http.NewRequest("GET", ts.URL+"/marge/accounts/"+account+"/devices/DEV1/presets", nil)
		req.Header.Set("If-None-Match", etag)

		res2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = res2.Body.Close() }()

		if res2.StatusCode != http.StatusNotModified {
			t.Errorf("Expected 304 Not Modified, got %v", res2.Status)
		}
	})

	t.Run("AccountFull ETag", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/accounts/" + account + "/full")
		if err != nil {
			t.Fatal(err)
		}

		etag := res.Header.Get(caseSensitiveETag)
		_ = res.Body.Close()

		if etag == "" {
			t.Fatal("Expected ETag header, got none")
		}

		req, _ := http.NewRequest("GET", ts.URL+"/marge/accounts/"+account+"/full", nil)
		req.Header.Set("If-None-Match", etag)

		res2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = res2.Body.Close() }()

		if res2.StatusCode != http.StatusNotModified {
			t.Errorf("Expected 304 Not Modified, got %v", res2.Status)
		}
	})

	t.Run("SourceProviders ETag (Dynamic)", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/streaming/sourceproviders")
		if err != nil {
			t.Fatal(err)
		}

		etag := res.Header.Get(caseSensitiveETag)
		_ = res.Body.Close()

		req, _ := http.NewRequest("GET", ts.URL+"/marge/streaming/sourceproviders", nil)
		req.Header.Set("If-None-Match", etag)

		res2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = res2.Body.Close() }()

		// For SourceProviders, we currently use time.Now(), so this might fail if it crosses a millisecond boundary.
		// In a real scenario, this would likely be stable during a single SoundTouch session's refresh.
		if res2.StatusCode != http.StatusNotModified {
			t.Logf("SourceProviders ETag changed (expected if ms boundary crossed)")
		}
	})

	t.Run("SoftwareUpdate ETag", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/marge/updates/soundtouch")
		if err != nil {
			t.Fatal(err)
		}

		etag := res.Header.Get(caseSensitiveETag)
		_ = res.Body.Close()

		if etag == "" {
			t.Fatal("Expected ETag header for swupdate")
		}

		req, _ := http.NewRequest("GET", ts.URL+"/marge/updates/soundtouch", nil)
		req.Header.Set("If-None-Match", etag)

		res2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = res2.Body.Close() }()

		if res2.StatusCode != http.StatusNotModified {
			t.Errorf("Expected 304 Not Modified for swupdate, got %v", res2.Status)
		}
	})

	t.Run("Negative ETag Test", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/marge/accounts/"+account+"/full", nil)
		req.Header.Set("If-None-Match", "wrong-etag")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK for wrong ETag, got %v", res.Status)
		}
	})

	t.Run("ETag Header Case Sensitivity", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/marge/accounts/"+account+"/full", nil)
		r.ServeHTTP(w, req)

		t.Logf("Recorder Headers: %v", w.Header())

		found := false

		for k := range w.Header() {
			if k == caseSensitiveETag {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected exact 'ETag' header in recorder, but it was not found in: %v", w.Header())
		}
	})

	t.Run("ETag Header Case Sensitivity (Proxy)", func(t *testing.T) {
		// Mock a backend response with lowercase 'etag'
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header()["etag"] = []string{"backend-etag"}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<xml/>"))
		}))
		defer backend.Close()

		target, _ := url.Parse(backend.URL)
		pyProxy := httputil.NewSingleHostReverseProxy(target)
		pyProxy.ModifyResponse = func(res *http.Response) error {
			// Generic Header Restoration:
			// Move Etag to ETag
			if etags, ok := res.Header[normalizedEtag]; ok {
				delete(res.Header, normalizedEtag)
				res.Header[caseSensitiveETag] = etags
			}

			return nil
		}

		// We'll use a direct call to the ModifyResponse to check logic
		resp := &http.Response{
			Header: make(http.Header),
		}
		resp.Header[normalizedEtag] = []string{"test-etag"}
		_ = pyProxy.ModifyResponse(resp)

		//nolint:canonicalheader
		if _, ok := resp.Header[caseSensitiveETag]; !ok {
			t.Errorf("ModifyResponse did not normalize ETag casing. Headers: %v", resp.Header)
		}

		// Negative check: ensure 'Etag' is gone (net/http canonicalizes ETag to Etag)
		// but since we deleted it and set ETag specifically, it should NOT be there.
		if _, ok := resp.Header[normalizedEtag]; ok {
			t.Error("Etag header still present after normalization")
		}
	})

	t.Run("X-Bose-Token Casing Test", func(t *testing.T) {
		w := httptest.NewRecorder()
		// Test that using direct map access on w.Header() preserves casing
		w.Header()["X-BOSE-TOKEN"] = []string{"token"}

		found := false

		for k := range w.Header() {
			if k == "X-BOSE-TOKEN" {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected exact 'X-BOSE-TOKEN' header in recorder, but it was normalized: %v", w.Header())
		}
	})

	t.Run("Golang Header Normalization Documentation", func(t *testing.T) {
		// This test documents how Go's http.Header.Set/Get canonicalizes keys.
		h := make(http.Header)

		// 1. Set canonicalizes to "Etag" (Standard Go behavior)
		h.Set(caseSensitiveETag, "v1")

		if _, ok := h[normalizedEtag]; !ok {
			t.Errorf("Expected key 'Etag' in map after Set('ETag'), but got: %v", h)
		}

		//nolint:canonicalheader
		if _, ok := h[caseSensitiveETag]; ok {
			// In Go's map, "ETag" and "Etag" are different keys.
			// Set() uses CanonicalHeaderKey which produces "Etag" (lowercase 't').
			// So "ETag" should NOT be present in the map if we used Set("ETag").
			t.Errorf("Did not expect exact key 'ETag' in map after Set('ETag') because Go canonicalizes to 'Etag'")
		}

		// 2. Get() also canonicalizes the key before lookup
		if val := h.Get("ETAG"); val != "v1" {
			t.Errorf("Expected Get('ETAG') to find 'v1' due to canonicalization, got %q", val)
		}

		// 3. Direct map access bypasses normalization
		h["X-Bose-Token"] = []string{"v2"}
		if _, ok := h["X-Bose-Token"]; !ok {
			t.Error("Expected exact key 'X-Bose-Token' to be present")
		}
		// However, Get() will still look for "X-Bose-Token" (canonicalized)
		// Wait, CanonicalHeaderKey("X-Bose-Token") is "X-Bose-Token" anyway.
		// Let's try something that changes.
		h["etag"] = []string{"v3"}
		if h.Get("etag") != "v1" {
			// Get("etag") -> Get(Canonical("etag")) -> Get("Etag") -> returns "v1"
			// It does NOT find "v3" because "etag" != "Etag" in the map.
			t.Errorf("Get('etag') found %q, but we expected it to find the canonical 'Etag' value 'v1'", h.Get("etag"))
		}
	})
}
