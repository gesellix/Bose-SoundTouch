//go:build browsertest

// Package soundtouchweb browser-level regression tests for #649. These drive
// a real headless Chrome via chromedp (already a project dependency, used
// today for the doc-screenshot tool) instead of only asserting on the raw
// HTML/JS source. They are opt-in (build tag "browsertest", run via `make
// test-browser`) rather than part of the default `go test ./...`/`make
// check` path, since they require a Chrome/Chromium binary to be present --
// see CONTRIBUTING or the Makefile for how to run them locally or in CI.
package soundtouchweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
)

// newHeadlessChromeContext returns a context bound to a fresh headless
// Chrome instance, torn down automatically at the end of the test.
func newHeadlessChromeContext(t *testing.T) context.Context {
	t.Helper()

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			// CI runners commonly execute as a user without the namespace
			// permissions Chrome's sandbox needs; harmless to also set
			// locally.
			chromedp.Flag("no-sandbox", true),
			// chromedp's default is 20s; a loaded shared CI runner can be
			// slower than that to fork/exec Chrome and print its DevTools
			// websocket URL, which otherwise surfaces as a flaky "websocket
			// url timeout reached" test failure unrelated to the page under
			// test.
			chromedp.WSURLReadTimeout(45*time.Second),
		)...,
	)
	t.Cleanup(cancelAlloc)

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	t.Cleanup(cancelCtx)

	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(cancelTimeout)

	return ctx
}

// TestPlayerRendersNatively confirms the shipped page (native import maps,
// es-module-shims left uninjected) still renders in an ordinary modern
// browser -- i.e. that restoring import maps for #649 didn't break the
// common case for the vast majority of users who never need the shim.
func TestPlayerRendersNatively(t *testing.T) {
	app := NewWebApp()
	r := chi.NewRouter()
	app.Mount(r, nil)

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	ctx := newHeadlessChromeContext(t)

	var shimInjected bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(`.nav-discover-icon`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelectorAll('script[src*="es-module-shims"]').length > 0`, &shimInjected),
	); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}

	if shimInjected {
		t.Error("es-module-shims should not be injected on a browser with native import map support")
	}
}

// TestPlayerRendersUnderForcedShimMode exercises es-module-shims resolving the
// same import map and vendored files the real app uses. It does not emulate
// Safari or the production feature-detection loader; those require a target-
// browser canary. The test serves a page that forces es-module-shims into
// shimMode
// (see the library's README: shimMode is triggered by
// window.esmsInitOptions.shimMode or by using importmap-shim/module-shim
// script types), which routes every browser -- including this ordinary
// headless Chrome -- through the library's own polyfill resolution instead
// of native import map support.
func TestPlayerRendersUnderForcedShimMode(t *testing.T) {
	app := NewWebApp()
	r := chi.NewRouter()
	app.MountWeb(r, nil) // only need /app/static/* and /api/control/*

	const shimModePage = `<!doctype html>
<html lang="en">
<head>
<meta charset="UTF-8" />
<script>window.esmsInitOptions = { shimMode: true };</script>
<script src="/app/static/lib/es-module-shims.js"></script>
<script type="importmap-shim">
{
    "imports": {
        "preact": "/app/static/lib/preact.module.js",
        "preact/hooks": "/app/static/lib/preact-hooks.module.js",
        "htm": "/app/static/lib/htm.module.js"
    }
}
</script>
<link rel="stylesheet" href="/app/static/css/app.css" />
</head>
<body>
<div id="app"></div>
<script type="module-shim" src="/app/static/js/app.js"></script>
</body>
</html>`

	r.Get("/test-shim-mode", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(shimModePage))
	})

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	ctx := newHeadlessChromeContext(t)

	var rendered bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/test-shim-mode"),
		chromedp.WaitVisible(`.nav-discover-icon`, chromedp.ByQuery),
		chromedp.Evaluate(`document.getElementById('app').children.length > 0`, &rendered),
	); err != nil {
		t.Fatalf("chromedp run (forced shim mode): %v", err)
	}

	if !rendered {
		t.Error("app did not render under forced es-module-shims shim mode")
	}
}
