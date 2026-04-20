package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestMargeAccountETagNoDevice tests ETag correctness on the account-level endpoints
// when no ?device= query parameter is provided (the "no-device" code path in GetETagForAccount).
//
// These tests are specifically for the bug where:
//   - GetETagForAccount returns "" when the devices directory does not exist
//   - A missing If-None-Match header also produces "" via Header.Get
//   - The equality check "" == "" causes a false 304 on the very first request
func TestMargeAccountETagNoDevice(t *testing.T) {
	newServer := func(t *testing.T) (ts *httptest.Server, tempDir string) {
		t.Helper()
		tempDir, _ = os.MkdirTemp("", "st-etag-nodev-*")
		t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
		ds := datastore.NewDataStore(tempDir)
		r, _ := setupRouter("http://localhost:8001", ds)
		ts = httptest.NewServer(r)
		t.Cleanup(ts.Close)
		return ts, tempDir
	}

	writeDeviceFiles := func(t *testing.T, tempDir, account, deviceID string) string {
		t.Helper()
		deviceDir := filepath.Join(tempDir, "accounts", account, "devices", deviceID)
		_ = os.MkdirAll(deviceDir, 0755)
		_ = os.WriteFile(filepath.Join(deviceDir, "Presets.xml"), []byte("<presets/>"), 0644)
		_ = os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte("<sources/>"), 0644)
		_ = os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte("<recents/>"), 0644)
		return filepath.Join(deviceDir, "Presets.xml")
	}

	// Bug: GetETagForAccount returns "" when the devices directory does not exist.
	// Handler then checks: r.Header.Get("If-None-Match") == "" which is true on any
	// request without the header, causing a 304 before the client has any cached content.
	t.Run("first request without If-None-Match never returns 304 when devices dir is missing", func(t *testing.T) {
		ts, _ := newServer(t)
		// Deliberately no directories created for this account.
		res, err := http.Get(ts.URL + "/marge/accounts/no-devices-account/full")
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()

		if res.StatusCode == http.StatusNotModified {
			t.Error("got 304 on first request without If-None-Match — empty ETag matched empty header value")
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %v", res.Status)
		}
	})

	t.Run("ETag is non-empty even when devices dir does not exist", func(t *testing.T) {
		ts, _ := newServer(t)
		res, err := http.Get(ts.URL + "/marge/accounts/no-devices-account/full")
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()

		if etag := res.Header.Get("ETag"); etag == "" {
			t.Error("ETag must not be empty string — empty ETag causes false 304 on first request")
		}
	})

	t.Run("ETag is non-empty when devices dir exists but contains no device subdirs", func(t *testing.T) {
		ts, tempDir := newServer(t)
		account := "empty-devices-account"
		_ = os.MkdirAll(filepath.Join(tempDir, "accounts", account, "devices"), 0755)

		res, err := http.Get(ts.URL + "/marge/accounts/" + account + "/full")
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()

		if etag := res.Header.Get("ETag"); etag == "" {
			t.Error("ETag must not be empty string when devices dir exists but is empty")
		}
	})

	t.Run("ETag is stable across multiple requests when data does not change", func(t *testing.T) {
		ts, tempDir := newServer(t)
		account := "stable-etag-account"
		writeDeviceFiles(t, tempDir, account, "DEV1")
		url := ts.URL + "/marge/accounts/" + account + "/full"

		res1, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		etag1 := res1.Header.Get("ETag")
		_ = res1.Body.Close()

		res2, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		etag2 := res2.Header.Get("ETag")
		_ = res2.Body.Close()

		if etag1 == "" {
			t.Fatal("ETag must not be empty")
		}
		if etag1 != etag2 {
			t.Errorf("ETag changed between identical requests: %q → %q", etag1, etag2)
		}
	})

	t.Run("304 flow works correctly with no device param", func(t *testing.T) {
		ts, tempDir := newServer(t)
		account := "304-flow-account"
		writeDeviceFiles(t, tempDir, account, "DEV1")
		url := ts.URL + "/marge/accounts/" + account + "/full"

		res1, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		etag := res1.Header.Get("ETag")
		_ = res1.Body.Close()

		if etag == "" {
			t.Fatal("expected non-empty ETag from first request")
		}

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("If-None-Match", etag)
		res2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res2.Body.Close() }()

		if res2.StatusCode != http.StatusNotModified {
			t.Errorf("expected 304 with valid ETag, got %v", res2.Status)
		}
	})

	t.Run("ETag changes when file content changes", func(t *testing.T) {
		ts, tempDir := newServer(t)
		account := "changing-data-account"
		presetsFile := writeDeviceFiles(t, tempDir, account, "DEV1")
		url := ts.URL + "/marge/accounts/" + account + "/full"

		res1, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		etag1 := res1.Header.Get("ETag")
		_ = res1.Body.Close()

		_ = os.WriteFile(presetsFile, []byte(`<presets><preset id="1"/></presets>`), 0644)

		res2, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		etag2 := res2.Header.Get("ETag")
		_ = res2.Body.Close()

		if etag1 == etag2 {
			t.Errorf("ETag did not change after file modification: %q", etag1)
		}
	})

	t.Run("stale ETag returns 200 after data change", func(t *testing.T) {
		ts, tempDir := newServer(t)
		account := "stale-etag-account"
		presetsFile := writeDeviceFiles(t, tempDir, account, "DEV1")
		url := ts.URL + "/marge/accounts/" + account + "/full"

		res1, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		etag1 := res1.Header.Get("ETag")
		_ = res1.Body.Close()

		_ = os.WriteFile(presetsFile, []byte(`<presets><preset id="1"/></presets>`), 0644)

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("If-None-Match", etag1)
		res2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res2.Body.Close() }()

		if res2.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for stale ETag after data change, got %v", res2.Status)
		}
	})

	// The /full, /sources, and /devices endpoints all call GetETagForAccount(account, "")
	// so they should return the same ETag for the same account state.
	t.Run("ETag is consistent across full, sources, and devices endpoints", func(t *testing.T) {
		ts, tempDir := newServer(t)
		account := "consistent-etag-account"
		writeDeviceFiles(t, tempDir, account, "DEV1")

		get := func(path string) string {
			res, err := http.Get(ts.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			etag := res.Header.Get("ETag")
			_ = res.Body.Close()
			return etag
		}

		etagFull := get("/marge/accounts/" + account + "/full")
		etagSources := get("/marge/accounts/" + account + "/sources")
		etagDevices := get("/marge/accounts/" + account + "/devices")

		if etagFull == "" {
			t.Fatal("ETag from /full must not be empty")
		}
		if etagFull != etagSources {
			t.Errorf("/full ETag %q != /sources ETag %q", etagFull, etagSources)
		}
		if etagFull != etagDevices {
			t.Errorf("/full ETag %q != /devices ETag %q", etagFull, etagDevices)
		}
	})
}
