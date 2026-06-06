package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
)

// TestDualRouteEquivalence verifies the issue #451 step-1 aliasing invariant:
// each admin-tier route served at both its legacy path and the new /api/* path
// returns an identical response (same handler, same middleware). It fires the
// same request at the old and new path and asserts equal status + body.
//
// The cases use endpoints whose body does not embed per-request time/random
// values, so the only thing that can differ is the routing — which is exactly
// what we want to pin while the routes are dual-mounted.
func TestDualRouteEquivalence(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	server := handlers.NewServer(ds, nil, "http://localhost:8000", true, false, false)
	r := setupRouter(server, nil, nil)

	ts := httptest.NewServer(r)
	defer ts.Close()

	cases := []struct {
		method  string
		oldPath string
		newPath string
	}{
		{http.MethodGet, "/setup/version", "/api/setup/version"},
		{http.MethodGet, "/setup/settings", "/api/setup/settings"},
		{http.MethodGet, "/setup/tts/config", "/api/setup/tts/config"},
		{http.MethodGet, "/setup/logging-settings", "/api/setup/logging-settings"},
		{http.MethodGet, "/setup/interaction-stats", "/api/setup/interaction-stats"},
		{http.MethodGet, "/setup/dns-discoveries", "/api/setup/dns-discoveries"},
		// /mgmt is Basic-Auth'd; without credentials both paths must reject
		// identically — that pins the auth gate is mirrored onto /api/mgmt too.
		{http.MethodGet, "/mgmt/accounts/", "/api/mgmt/accounts/"},
		{http.MethodGet, "/mgmt/spotify/accounts", "/api/mgmt/spotify/accounts"},
		{http.MethodGet, "/mgmt/amazon/accounts", "/api/mgmt/amazon/accounts"},
	}

	for _, c := range cases {
		t.Run(c.method+" "+c.newPath, func(t *testing.T) {
			oldStatus, oldBody := doEquivReq(t, ts.URL, c.method, c.oldPath)
			newStatus, newBody := doEquivReq(t, ts.URL, c.method, c.newPath)

			if oldStatus != newStatus {
				t.Errorf("status mismatch for %s vs %s: old=%d new=%d", c.oldPath, c.newPath, oldStatus, newStatus)
			}

			if !bytes.Equal(oldBody, newBody) {
				t.Errorf("body mismatch for %s vs %s:\n old=%q\n new=%q", c.oldPath, c.newPath, oldBody, newBody)
			}
		})
	}
}

func doEquivReq(t *testing.T, base, method, path string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", path, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", path, err)
	}

	return resp.StatusCode, body
}
