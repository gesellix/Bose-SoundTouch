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
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

func newPlayerFixtureServer(t *testing.T, moduleScript string, configure func(chi.Router)) *httptest.Server {
	t.Helper()

	r := chi.NewRouter()
	staticFS, err := fs.Sub(StaticFS, "static")
	if err != nil {
		t.Fatalf("open static fixture filesystem: %v", err)
	}
	r.Get("/app/static/*", http.StripPrefix("/app/static", http.FileServer(http.FS(staticFS))).ServeHTTP)
	configure(r)
	r.Get("/fixture", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<!doctype html>
<html><head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<script type="importmap">{"imports":{"preact":"/app/static/lib/preact.module.js","preact/hooks":"/app/static/lib/preact-hooks.module.js","htm":"/app/static/lib/htm.module.js"}}</script>
<link rel="stylesheet" href="/app/static/css/app.css">
</head><body><div class="app"><div id="fixture"></div></div>
<script type="module">%s</script></body></html>`, moduleScript)
	})

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	return server
}

// providerFixtureScript renders a RADIO_BROWSER source, which the speaker
// advertises READY but cannot act on without a station ContentItem.
const providerFixtureScript = `
import { h, render } from 'preact';
import { Sources } from '/app/static/js/components/Sources.js';
window.navigated = [];
render(h(Sources, {
  deviceId: 'speaker',
  status: {
    revision: 1,
    nowPlayingRevision: 1,
    sources: { SourceItem: [
      { Source: 'RADIO_BROWSER', SourceAccount: '', DisplayName: 'RadioBrowser', Status: 'READY' },
      { Source: 'LOCAL_INTERNET_RADIO', SourceAccount: '', DisplayName: 'Local Radio', Status: 'READY' },
      { Source: 'STORED_MUSIC', SourceAccount: 'fa095ecc-e13e-40e7-8e6c-e0286d5bc000/0', DisplayName: 'fritz', Status: 'READY' },
    ] },
    nowPlaying: { Source: 'SPOTIFY', SourceAccount: 'someone' },
  },
  onNavigate: page => window.navigated.push(page),
  readbackDelays: [100, 250, 500],
}), document.getElementById('fixture'));
`

const sourceFixtureScript = `
import { h, render } from 'preact';
import { Sources } from '/app/static/js/components/Sources.js';
const sources = [
  { Source: 'AUX', SourceAccount: 'AUX1', DisplayName: 'Aux 1', Status: 'READY' },
  { Source: 'PRODUCT', SourceAccount: '', DisplayName: 'Product', Status: 'READY' },
  { Source: 'SPOTIFY', SourceAccount: 'spotify-user', DisplayName: 'Spotify', Status: 'READY' },
];
let revision = 0;
window.renderStatus = (source = 'STANDBY', account = '', sourcesStale = false, deviceId = 'speaker', revisionOverride = null, sourceItems = sources) => {
  const nextRevision = revisionOverride ?? ++revision;
  return render(h(Sources, {
    deviceId,
    status: { revision: nextRevision, nowPlayingRevision: nextRevision, sources: { SourceItem: sourceItems }, sourcesStale, nowPlaying: { Source: source, SourceAccount: account } },
    readbackDelays: [100, 250, 500],
  }), document.getElementById('fixture'));
};
window.renderStatus();
`

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
// outageProxy is a TCP proxy in front of the test server that can be taken
// down and brought back at the same address.
//
// Simulating an outage needs both halves: refusing new connections AND
// severing the established ones. A server that merely stops accepting leaves
// an open WebSocket running, and Chrome's offline emulation does not close it
// either, so neither reproduces a service that went away.
type outageProxy struct {
	listener net.Listener
	backend  string

	mu    sync.Mutex
	up    bool
	conns []net.Conn
}

func newOutageProxy(t *testing.T, backend string) *outageProxy {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	p := &outageProxy{listener: listener, backend: backend, up: true}
	t.Cleanup(func() {
		_ = listener.Close()
		p.setUp(false)
	})

	go p.serve()

	return p
}

func (p *outageProxy) url() string { return "http://" + p.listener.Addr().String() }

func (p *outageProxy) serve() {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}

		p.mu.Lock()
		serving := p.up
		p.mu.Unlock()

		if !serving {
			_ = client.Close()

			continue
		}

		upstream, err := net.Dial("tcp", p.backend)
		if err != nil {
			_ = client.Close()

			continue
		}

		p.mu.Lock()
		p.conns = append(p.conns, client, upstream)
		p.mu.Unlock()

		go func() { _, _ = io.Copy(upstream, client) }()
		go func() { _, _ = io.Copy(client, upstream) }()
	}
}

// setUp brings the proxy down or back. Going down also drops every connection
// already established, which is what makes an open WebSocket notice.
func (p *outageProxy) setUp(up bool) {
	p.mu.Lock()
	p.up = up

	conns := p.conns
	p.conns = nil
	p.mu.Unlock()

	if up {
		return
	}

	for _, c := range conns {
		_ = c.Close()
	}
}

// TestPlayerSurvivesAServiceOutage: the player used to reload itself five
// seconds after the socket closed, which cannot work while the service is
// down, since the document is served by that same service. The tab landed on
// the browser's error page and everything the page held was lost.
//
// It must now stay up, say so, and recover on its own. The epoch on each
// status is what makes that safe: a restarted service publishes revisions
// from 0 again, and without the epoch the browser would reject them forever.
func TestPlayerSurvivesAServiceOutage(t *testing.T) {
	app := NewWebApp()
	r := chi.NewRouter()
	app.Mount(r, nil)

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	proxy := newOutageProxy(t, backendURL.Host)
	ctx := newHeadlessChromeContext(t)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(proxy.url()+"/app"),
		chromedp.WaitVisible(`#app`, chromedp.ByQuery),
		// The socket must be up before taking it away.
		chromedp.Poll(`document.querySelector('.connection-banner') === null`, nil),
	); err != nil {
		t.Fatalf("connect before the outage: %v", err)
	}

	proxy.setUp(false)

	var bannerAfterOutage, stillLoaded string
	if err := chromedp.Run(ctx,
		chromedp.Poll(`document.querySelector('.connection-banner') !== null`, nil),
		chromedp.Text(`.connection-banner`, &bannerAfterOutage, chromedp.ByQuery),
		// The page is still the player, not the browser's error page.
		chromedp.Evaluate(`document.querySelector('#app') !== null ? 'loaded' : 'gone'`, &stillLoaded),
	); err != nil {
		t.Fatalf("detect the outage: %v", err)
	}

	if !strings.Contains(bannerAfterOutage, "Reconnecting") {
		t.Errorf("banner during outage = %q, want it to say it is reconnecting", bannerAfterOutage)
	}
	if stillLoaded != "loaded" {
		t.Error("player did not survive the outage")
	}

	proxy.setUp(true)

	var navigations int
	if err := chromedp.Run(ctx,
		// Recovers on its own, with no interaction.
		chromedp.Poll(`document.querySelector('.connection-banner') === null`, nil),
		chromedp.Evaluate(`performance.getEntriesByType('navigation').length`, &navigations),
	); err != nil {
		t.Fatalf("recover after the outage: %v", err)
	}

	// Still the document that weathered the outage, not a reloaded one.
	if navigations != 1 {
		t.Errorf("navigation entries = %d, want 1: the player reloaded instead of reconnecting", navigations)
	}
}

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

func TestFrontendDeviceStateRejectsNonNewerStatusRevisions(t *testing.T) {
	const revisionFixture = `
import { mergeDevicesSnapshot, mergeStatusUpdate } from '/app/static/js/app.js';
const current = {
  speaker: {
    info: { name: 'Current' },
    status: { revision: 5, sourcesStale: false, nowPlaying: { Track: 'new' } },
  },
};
const snapshot = mergeDevicesSnapshot(current, {
  speaker: {
    info: { name: 'Renamed' },
    stereoPair: { id: 'stale-pair' },
    status: { revision: 4, nowPlaying: { Track: 'old' } },
  },
  added: { info: { name: 'Added' }, status: { revision: 1 } },
});
const equal = mergeStatusUpdate(snapshot, 'speaker', { revision: 5, nowPlaying: { Track: 'equal' } });
const older = mergeStatusUpdate(equal, 'speaker', { revision: 3, nowPlaying: { Track: 'older' } });
const newer = mergeStatusUpdate(older, 'speaker', { revision: 6, nowPlaying: { Track: 'newest' } });
const stale = mergeStatusUpdate(newer, 'speaker', {
  revision: 6,
  sourcesStale: true,
  nowPlaying: { Track: 'must not replace canonical state' },
});
const adversarial = Object.fromEntries([
  ['__proto__', { status: { revision: 1, nowPlaying: { Track: 'old proto' } } }],
  ['constructor', { status: { revision: 1, nowPlaying: { Track: 'old constructor' } } }],
]);
const protoUpdated = mergeStatusUpdate(adversarial, '__proto__', {
  revision: 2,
  nowPlaying: { Track: 'new proto' },
});
const constructorUpdated = mergeStatusUpdate(protoUpdated, 'constructor', {
  revision: 2,
  nowPlaying: { Track: 'new constructor' },
});
window.revisionChecks = {
  snapshotKeptStatus: snapshot.speaker.status.revision === 5 && snapshot.speaker.status.nowPlaying.Track === 'new',
  // A losing snapshot entry is rejected whole: its stereoPair is derived from
  // the same older status, so mixing the two would describe a pair the newer
  // status already dissolved.
  snapshotKeptEntryWhole: snapshot.speaker.info.name === 'Current',
  snapshotAcceptsUnseenDevices: snapshot.added.status.revision === 1,
  equalRejected: equal === snapshot,
  olderRejected: older === equal,
  newerAccepted: newer !== older && newer.speaker.status.revision === 6 && newer.speaker.status.nowPlaying.Track === 'newest',
  staleAtEqualRevisionRejected: stale === newer,
  staleAtNewerRevisionAccepted: (() => {
    const applied = mergeStatusUpdate(newer, 'speaker', {
      revision: 7,
      sourcesStale: true,
      nowPlaying: { Track: 'newest' },
    });
    return applied !== newer && applied.speaker.status.sourcesStale === true;
  })(),
  staleCannotClearAtEqualRevision: mergeStatusUpdate(stale, 'speaker', {
    revision: 6,
    sourcesStale: false,
  }) === stale,
  snapshotDroppedStaleProjection: snapshot.speaker.stereoPair === undefined,
  unknownRejected: mergeStatusUpdate(newer, 'unknown', { revision: 99 }) === newer,
  // A fresh DeviceConnection for the same id restarts revisions at 0. Without
  // the epoch check this frame loses the revision comparison and the tab stays
  // pinned to the old status forever.
  newerEpochAcceptedDespiteLowerRevision: (() => {
    const epoched = mergeStatusUpdate(newer, 'speaker', {
      epoch: 100,
      revision: 40,
      nowPlaying: { Track: 'epoch one' },
    });
    const reconnected = mergeStatusUpdate(epoched, 'speaker', {
      epoch: 101,
      revision: 0,
      nowPlaying: { Track: 'epoch two' },
    });
    return reconnected !== epoched &&
      reconnected.speaker.status.nowPlaying.Track === 'epoch two';
  })(),
  olderEpochRejected: (() => {
    const epochOne = mergeStatusUpdate(newer, 'speaker', {
      epoch: 100,
      revision: 40,
      nowPlaying: { Track: 'epoch one' },
    });
    const epochTwo = mergeStatusUpdate(epochOne, 'speaker', {
      epoch: 101,
      revision: 0,
      nowPlaying: { Track: 'epoch two' },
    });
    // A frame still in flight from the replaced connection, carrying a high
    // revision from its own sequence, must not win.
    return mergeStatusUpdate(epochTwo, 'speaker', {
      epoch: 100,
      revision: 99,
      nowPlaying: { Track: 'late frame from the old connection' },
    }) === epochTwo;
  })(),
  adversarialIDsAreOwnDataProperties:
    Object.prototype.hasOwnProperty.call(constructorUpdated, '__proto__') &&
    Object.prototype.hasOwnProperty.call(constructorUpdated, 'constructor'),
  adversarialIDsKeepObjectPrototype: Object.getPrototypeOf(constructorUpdated) === Object.prototype,
  adversarialIDsUpdateOnlyTheirRecords:
    constructorUpdated.__proto__.status.nowPlaying.Track === 'new proto' &&
    constructorUpdated.constructor.status.nowPlaying.Track === 'new constructor' &&
    Object.prototype.polluted === undefined,
};
`
	server := newPlayerFixtureServer(t, revisionFixture, func(chi.Router) {})
	ctx := newHeadlessChromeContext(t)

	var checks map[string]bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.Poll(`window.revisionChecks !== undefined`, nil),
		chromedp.Evaluate(`window.revisionChecks`, &checks),
	); err != nil {
		t.Fatalf("exercise frontend revision state owner: %v", err)
	}
	for name, passed := range checks {
		if !passed {
			t.Errorf("revision check %s failed", name)
		}
	}
}

// TestSourceExpiryDisablesCommandsOnNewerRevision: a stale marker pushed over
// the socket must reach the buttons and make them unclickable, without the
// projection ever showing the source as selected.
func TestSourceExpiryDisablesCommandsOnNewerRevision(t *testing.T) {
	const sourceExpiryFixture = `
import { h, render } from 'preact';
import { mergeStatusUpdate } from '/app/static/js/app.js';
import { Sources } from '/app/static/js/components/Sources.js';
const ready = [{ Source: 'AUX', SourceAccount: 'AUX1', DisplayName: 'Aux 1', Status: 'READY' }];
let devices = {
  speaker: {
    status: {
      revision: 5,
      nowPlayingRevision: 5,
      sourcesStale: false,
      sources: { SourceItem: ready },
      nowPlaying: { Source: 'STANDBY', SourceAccount: '' },
    },
  },
};
function redraw() {
  render(h(Sources, {
    deviceId: 'speaker',
    status: devices.speaker.status,
    readbackDelays: [100],
  }), document.getElementById('fixture'));
}
window.expireSources = () => {
  devices = mergeStatusUpdate(devices, 'speaker', {
    revision: 6,
    nowPlayingRevision: 5,
    sourcesStale: true,
    sources: { SourceItem: ready },
    nowPlaying: { Source: 'STANDBY', SourceAccount: '' },
  });
  redraw();
};
redraw();
`

	var mu sync.Mutex
	writes := 0
	server := newPlayerFixtureServer(t, sourceExpiryFixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	var trackSource string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Evaluate(`window.expireSources()`, nil),
		chromedp.Poll(`document.querySelector('.source-btn')?.disabled === true`, nil),
		chromedp.Evaluate(`document.querySelector('.source-btn').click()`, nil),
		chromedp.Sleep(50*time.Millisecond),
		chromedp.Evaluate(`document.querySelector('.source-btn').classList.contains('active') ? 'AUX' : 'STANDBY'`, &trackSource),
	); err != nil {
		t.Fatalf("apply derived source expiry: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if writes != 0 || trackSource != "STANDBY" {
		t.Fatalf("stale source expiry writes=%d projected source=%q, want 0 and STANDBY", writes, trackSource)
	}
}

func TestSourceSelectionUsesOneWriteAndAbsoluteReadbacks(t *testing.T) {
	type sourceRun struct {
		body      webtypes.SourceRequest
		startedAt time.Time
		readTimes []time.Duration
	}

	var mu sync.Mutex
	var runs []sourceRun
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, req *http.Request) {
			var body webtypes.SourceRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			mu.Lock()
			runs = append(runs, sourceRun{body: body, startedAt: time.Now()})
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			if body.Source == "SPOTIFY" {
				// 500 is how a failed Client.SelectSource surfaces. It does not
				// prove the speaker ignored the command, so the readbacks below
				// must still run.
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: false, Error: "source rejected"})
				return
			}
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			run := &runs[len(runs)-1]
			run.readTimes = append(run.readTimes, time.Since(run.startedAt))
			readCount := len(run.readTimes)
			body := run.body
			mu.Unlock()

			nowPlaying := map[string]string{"Source": "STANDBY", "SourceAccount": ""}
			if body.Source == "AUX" && readCount >= 2 {
				nowPlaying = map[string]string{"Source": body.Source, "SourceAccount": body.Account}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"status": map[string]any{"revision": readCount + 1, "nowPlayingRevision": readCount + 1, "nowPlaying": nowPlaying},
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var immediatelyPending bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.source-btn:nth-child(1)').getAttribute('aria-busy') === 'true'`, &immediatelyPending),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selected'`, nil),
		chromedp.Click(`.source-btn:nth-child(2)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selection unverified'`, nil),
		chromedp.Click(`.source-btn:nth-child(3)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-btn:nth-child(3)').classList.contains('unverified')`, nil),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selection unverified: source rejected'`, nil),
	); err != nil {
		t.Fatalf("exercise source commands: %v", err)
	}
	if !immediatelyPending {
		t.Error("source command did not expose pending state immediately")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(runs) != 3 {
		t.Fatalf("source writes = %d, want exactly one for each of 3 commands", len(runs))
	}
	wantBodies := []webtypes.SourceRequest{
		{Source: "AUX", Account: "AUX1"},
		{Source: "PRODUCT", Account: ""},
		{Source: "SPOTIFY", Account: "spotify-user"},
	}
	for i, want := range wantBodies {
		if runs[i].body != want {
			t.Errorf("write %d body = %+v, want %+v", i, runs[i].body, want)
		}
	}
	if got := len(runs[0].readTimes); got != 3 {
		t.Errorf("confirmed command readbacks = %d, want 3", got)
	}
	if got := len(runs[1].readTimes); got != 3 {
		t.Errorf("unverified command readbacks = %d, want 3", got)
	}
	if got := len(runs[2].readTimes); got != 3 {
		t.Errorf("transport-uncertain write readbacks = %d, want 3", got)
	}
	for i, want := range []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond} {
		got := runs[0].readTimes[i]
		if got < want-40*time.Millisecond || got > want+150*time.Millisecond {
			t.Errorf("confirmed readback %d at %s, want absolute deadline near %s", i, got, want)
		}
	}
	for i, want := range []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond} {
		got := runs[1].readTimes[i]
		if got < want-40*time.Millisecond || got > want+150*time.Millisecond {
			t.Errorf("unverified readback %d at %s, want absolute deadline near %s", i, got, want)
		}
	}
}

// TestSourceSelectionStopsReadbacksOnceTheEventStreamConfirms: when the
// speaker's event stream is live it will report a late rejection on its own,
// so a confirmed selection must not keep polling. This is the difference
// between one readback per source tap and three.
// TestProviderSourceResumesMostRecentStation: RADIO_BROWSER is advertised
// READY but is not a selectable input. A bare /select strands the speaker on a
// stub now-playing (empty type and location, no playStatus) while the previous
// audio keeps playing, and the speaker then reports that stub indefinitely.
// Playing the most recent station for the source sends a real ContentItem.
func TestProviderSourceResumesMostRecentStation(t *testing.T) {
	var mu sync.Mutex
	var played map[string]any
	selects := 0
	server := newPlayerFixtureServer(t, providerFixtureScript, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"Items":[
				{"ID":1,"ContentItem":{"Source":"SPOTIFY","Location":"spotify:track:x"}},
				{"ID":2,"ContentItem":{"Source":"RADIO_BROWSER","Type":"stationurl",
					"Location":"/station/abc","ItemName":"Some Station","IsPresetable":true}}
			]}}`))
		})
		r.Post("/api/control/devices/speaker/play", func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			_ = json.NewDecoder(req.Body).Decode(&played)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			selects++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":{"revision":9,"nowPlayingRevision":9,` +
				`"webSocketConnected":true,"nowPlaying":{"Source":"RADIO_BROWSER","SourceAccount":""}}}}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	var navigated []string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selected'`, nil),
		chromedp.Evaluate(`window.navigated`, &navigated),
	); err != nil {
		t.Fatalf("exercise provider source with a recent station: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if selects != 0 {
		t.Errorf("provider source issued %d bare selects, want 0", selects)
	}
	if len(navigated) != 0 {
		t.Errorf("navigated to %v, want to stay and play the recent station", navigated)
	}
	// A Location is the whole point: without it the speaker gets the same stub.
	if played["location"] != "/station/abc" || played["source"] != "RADIO_BROWSER" ||
		played["type"] != "stationurl" || played["itemName"] != "Some Station" {
		t.Errorf("played ContentItem = %+v, want the recent RADIO_BROWSER station", played)
	}
}

// TestProviderSourceWithoutRecentsNavigatesInstead: with nothing to resume, the
// only options are stranding the speaker or sending the user somewhere useful.
func TestProviderSourceWithoutRecentsNavigatesInstead(t *testing.T) {
	var mu sync.Mutex
	writes := 0
	server := newPlayerFixtureServer(t, providerFixtureScript, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"Items":[
				{"ID":1,"ContentItem":{"Source":"SPOTIFY","Location":"spotify:track:x"}}
			]}}`))
		})
		r.Post("/api/control/devices/speaker/play", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
		})
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
		})
	})

	ctx := newHeadlessChromeContext(t)
	var navigated []string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`window.navigated.length === 1`, nil),
		chromedp.Evaluate(`window.navigated`, &navigated),
	); err != nil {
		t.Fatalf("exercise provider source without recents: %v", err)
	}

	if len(navigated) != 1 || navigated[0] != "radiobrowser" {
		t.Errorf("navigated = %v, want [radiobrowser]", navigated)
	}

	// LOCAL_INTERNET_RADIO never resumes, so it goes straight to Play URL.
	var localRadioNav []string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`window.navigated = []`, nil),
		chromedp.Click(`.source-btn:nth-child(2)`, chromedp.ByQuery),
		chromedp.Poll(`window.navigated.length === 1`, nil),
		chromedp.Evaluate(`window.navigated`, &localRadioNav),
	); err != nil {
		t.Fatalf("exercise LOCAL_INTERNET_RADIO without recents: %v", err)
	}
	if len(localRadioNav) != 1 || localRadioNav[0] != "playurl" {
		t.Errorf("navigated = %v, want [playurl]", localRadioNav)
	}

	mu.Lock()
	defer mu.Unlock()
	if writes != 0 {
		t.Errorf("issued %d writes for a provider with nothing to resume, want 0", writes)
	}
}

// TestProviderSourceNavigatesWhenRecentsFail: a recents lookup that errors must
// not fall through to the bare select this whole path exists to avoid.
func TestProviderSourceNavigatesWhenRecentsFail(t *testing.T) {
	var mu sync.Mutex
	writes := 0
	server := newPlayerFixtureServer(t, providerFixtureScript, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		})
		r.Post("/api/control/devices/speaker/play", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
		})
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
		})
	})

	ctx := newHeadlessChromeContext(t)
	var navigated []string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`window.navigated.length === 1`, nil),
		chromedp.Evaluate(`window.navigated`, &navigated),
	); err != nil {
		t.Fatalf("exercise provider source with failing recents: %v", err)
	}

	if len(navigated) != 1 || navigated[0] != "radiobrowser" {
		t.Errorf("navigated = %v, want [radiobrowser]", navigated)
	}

	mu.Lock()
	defer mu.Unlock()
	if writes != 0 {
		t.Errorf("issued %d writes after a failed recents lookup, want 0", writes)
	}
}

// TestStubNowPlayingIsNotReportedAsSuccess: PROVIDER_SOURCES only covers the
// sources known to produce the stub. For any other, the readback must not
// confirm a now-playing that names the source but reports nothing playing:
// no location, no play status, item name echoing the source.
func TestStubNowPlayingIsNotReportedAsSuccess(t *testing.T) {
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":{"revision":9,"nowPlayingRevision":9,` +
				`"webSocketConnected":true,"nowPlaying":{"Source":"AUX","SourceAccount":"AUX1",` +
				`"PlayStatus":"","ContentItem":{"Source":"AUX","Type":"","Location":"",` +
				`"ItemName":"AUX","IsPresetable":false}}}}}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	var statusText string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-btn:nth-child(1)').classList.contains('failed')`, nil),
		chromedp.Text(`.source-command-status`, &statusText, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("exercise stub now-playing readback: %v", err)
	}

	if !strings.Contains(statusText, "nothing playing") {
		t.Errorf("status = %q, want it to report that nothing is playing", statusText)
	}
}

// TestPlayingSourceWithoutLocationStillConfirms guards the backstop's own
// blast radius: a physical input reports no location, and must still confirm
// as long as the speaker says it is playing.
func TestPlayingSourceWithoutLocationStillConfirms(t *testing.T) {
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":{"revision":9,"nowPlayingRevision":9,` +
				`"webSocketConnected":true,"nowPlaying":{"Source":"AUX","SourceAccount":"AUX1",` +
				`"PlayStatus":"PLAY_STATE","ContentItem":{"Source":"AUX","Type":"","Location":"",` +
				`"ItemName":"AUX","IsPresetable":false}}}}}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selected'`, nil),
	); err != nil {
		t.Fatalf("exercise playing source with no location: %v", err)
	}
}

// TestLocalInternetRadioNeverResumes: AfterTouch plays its own TTS and the
// notification ding through LOCAL_INTERNET_RADIO, so that source's Recents mix
// one-shot audio with stations. Observed on real hardware: resuming its newest
// entry played the "AfterTouch ding". It must open Play URL instead, even when
// a perfectly resumable entry exists.
func TestLocalInternetRadioNeverResumes(t *testing.T) {
	var mu sync.Mutex
	recentsFetches := 0
	writes := 0
	server := newPlayerFixtureServer(t, providerFixtureScript, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			recentsFetches++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"Items":[
				{"ID":1,"ContentItem":{"Source":"LOCAL_INTERNET_RADIO","Type":"stationurl",
					"Location":"https://host/custom/v1/playback/abc?name=AfterTouch+ding",
					"ItemName":"AfterTouch ding","IsPresetable":true}}
			]}}`))
		})
		r.Post("/api/control/devices/speaker/play", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
		})
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
		})
	})

	ctx := newHeadlessChromeContext(t)
	var navigated []string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(2)`, chromedp.ByQuery),
		chromedp.Poll(`window.navigated.length === 1`, nil),
		chromedp.Evaluate(`window.navigated`, &navigated),
	); err != nil {
		t.Fatalf("exercise LOCAL_INTERNET_RADIO with a resumable recent: %v", err)
	}

	if len(navigated) != 1 || navigated[0] != "playurl" {
		t.Errorf("navigated = %v, want [playurl]", navigated)
	}

	mu.Lock()
	defer mu.Unlock()
	if writes != 0 {
		t.Errorf("issued %d writes, want 0: this source never plays anything on click", writes)
	}
	// Not merely ignored: the lookup is skipped, so a slow /recents cannot
	// delay opening the page.
	if recentsFetches != 0 {
		t.Errorf("fetched recents %d times, want 0", recentsFetches)
	}
}

// TestStoredMusicOpensTheLibrary: a STORED_MUSIC entry names a media server,
// not something to play, so selecting it identifies no track or container.
// Browsing is the only meaningful action.
func TestStoredMusicOpensTheLibrary(t *testing.T) {
	var mu sync.Mutex
	writes := 0
	server := newPlayerFixtureServer(t, providerFixtureScript, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"Items":[
				{"ID":1,"ContentItem":{"Source":"STORED_MUSIC","Type":"dir",
					"SourceAccount":"fa095ecc-e13e-40e7-8e6c-e0286d5bc000/0",
					"Location":"/music/album/1","ItemName":"Some Album","IsPresetable":true}}
			]}}`))
		})
		r.Post("/api/control/devices/speaker/play", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
		})
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
		})
	})

	ctx := newHeadlessChromeContext(t)
	var navigated []string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(3)`, chromedp.ByQuery),
		chromedp.Poll(`window.navigated.length === 1`, nil),
		chromedp.Evaluate(`window.navigated`, &navigated),
	); err != nil {
		t.Fatalf("exercise STORED_MUSIC source: %v", err)
	}

	if len(navigated) != 1 || navigated[0] != "library" {
		t.Errorf("navigated = %v, want [library]", navigated)
	}

	mu.Lock()
	defer mu.Unlock()
	// Even with a resumable album in Recents, this source only ever browses.
	if writes != 0 {
		t.Errorf("issued %d writes, want 0", writes)
	}
}

func TestSourceSelectionStopsReadbacksOnceTheEventStreamConfirms(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":{"revision":9,"nowPlayingRevision":9,` +
				`"webSocketConnected":true,"nowPlaying":{"Source":"AUX","SourceAccount":"AUX1"}}}}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selected'`, nil),
		// Outlast the remaining readback deadlines (250ms and 500ms here).
		chromedp.Sleep(900*time.Millisecond),
	); err != nil {
		t.Fatalf("exercise event-stream-confirmed selection: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads != 1 {
		t.Errorf("readbacks with a live event stream = %d, want 1", reads)
	}
}

// TestSourceSelectionKeepsReadbacksWithoutAnEventStream is the other half:
// with no event stream to watch for a late rejection, the readbacks are the
// only watcher and must run to the end of their window.
func TestSourceSelectionKeepsReadbacksWithoutAnEventStream(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			readCount := reads
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"success":true,"data":{"status":{"revision":%d,"nowPlayingRevision":%d,`+
				`"webSocketConnected":false,"nowPlaying":{"Source":"AUX","SourceAccount":"AUX1"}}}}`,
				readCount+8, readCount+8)
		})
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selected'`, nil),
		chromedp.Sleep(300*time.Millisecond),
	); err != nil {
		t.Fatalf("exercise selection without an event stream: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads != len([]int{100, 250, 500}) {
		t.Errorf("readbacks without an event stream = %d, want 3", reads)
	}
}

func TestSourceSelectionTreatsSelfAccountAsOmitted(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { Sources } from '/app/static/js/components/Sources.js';
const sourceItems = [{ Source: 'AUX', SourceAccount: 'AUX', DisplayName: 'AUX IN', Status: 'READY' }];
render(h(Sources, {
  deviceId: 'speaker',
  status: {
    revision: 1,
    nowPlayingRevision: 1,
    nowPlaying: { Source: 'STANDBY', SourceAccount: '' },
    sources: { SourceItem: sourceItems },
  },
  readbackDelays: [100, 250, 500],
}), document.getElementById('fixture'));
`

	var mu sync.Mutex
	writes := 0
	var request webtypes.SourceRequest
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			writes++
			_ = json.NewDecoder(req.Body).Decode(&request)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":{"revision":2,"nowPlayingRevision":2,"nowPlaying":{"Source":"AUX","SourceAccount":""}}}}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	var active bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selected'`, nil),
		chromedp.Evaluate(`document.querySelector('.source-btn').classList.contains('active')`, &active),
	); err != nil {
		t.Fatalf("select source whose self-account is omitted by now_playing: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if writes != 1 || request != (webtypes.SourceRequest{Source: "AUX", Account: "AUX"}) {
		t.Fatalf("writes=%d request=%+v, want one AUX/AUX write", writes, request)
	}
	if !active {
		t.Fatal("source with omitted self-account was not projected active")
	}
}

func TestSourceSelectionReadbacksDoNotWaitForSlowWriteResponse(t *testing.T) {
	var mu sync.Mutex
	startedAt := time.Time{}
	var readTimes []time.Duration
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			startedAt = time.Now()
			mu.Unlock()
			time.Sleep(700 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			readTimes = append(readTimes, time.Since(startedAt))
			readCount := len(readTimes)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"success":true,"data":{"status":{"revision":%d,"nowPlayingRevision":%d,"nowPlaying":{"Source":"STANDBY","SourceAccount":""}}}}`, readCount+1, readCount+1)
		})
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selection unverified'`, nil),
	); err != nil {
		t.Fatalf("exercise slow source response: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(readTimes) != 3 {
		t.Fatalf("readbacks = %d, want 3", len(readTimes))
	}
	for i, want := range []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond} {
		if got := readTimes[i]; got < want-40*time.Millisecond || got > want+150*time.Millisecond {
			t.Errorf("readback %d at %s, want absolute deadline near %s", i, got, want)
		}
	}
}

// TestSourceSelectionDefinitiveRefusalFailsImmediately: a 4xx is produced
// before AfterTouch ever calls the speaker, so the command provably never went
// out. There is nothing for the readbacks to confirm; report it at once, with
// the server's reason, instead of polling for the full readback window.
func TestSourceSelectionDefinitiveRefusalFailsImmediately(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"error":"Device not found"}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":{"revision":2,"nowPlayingRevision":2,"nowPlaying":{"Source":"STANDBY","SourceAccount":""}}}}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	var statusText string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-btn').classList.contains('failed')`, nil),
		chromedp.Text(`.source-command-status`, &statusText, chromedp.ByQuery),
		// Well past every readback deadline in this fixture (100/250/500ms), so
		// a zero read count means the failure came from the write itself.
		chromedp.Sleep(700*time.Millisecond),
	); err != nil {
		t.Fatalf("exercise definitively refused source write: %v", err)
	}

	if !strings.Contains(statusText, "Source selection failed") ||
		!strings.Contains(statusText, "Device not found") {
		t.Errorf("status = %q, want the failure and the server's reason", statusText)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads != 0 {
		t.Errorf("readbacks after a definitive refusal = %d, want 0", reads)
	}
}

func TestSourceSelectionLaterFirmwareErrorOverridesProvisionalConfirmation(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			read := reads
			mu.Unlock()

			source := "AUX"
			account := "AUX1"
			if read == 2 {
				source = "INVALID_SOURCE"
				account = ""
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"success":true,"data":{"status":{"revision":%d,"nowPlayingRevision":%d,"nowPlaying":{"Source":%q,"SourceAccount":%q}}}}`, read+1, read+1, source, account)
		})
	})

	ctx := newHeadlessChromeContext(t)
	var provisional bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selected, confirming'`, nil),
		chromedp.Evaluate(`document.querySelector('.source-btn:nth-child(1)').classList.contains('provisional-confirmed')`, &provisional),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selection failed: INVALID_SOURCE'`, nil),
	); err != nil {
		t.Fatalf("exercise provisional source rejection: %v", err)
	}
	if !provisional {
		t.Error("early matching readback did not expose provisional confirmation")
	}

	mu.Lock()
	defer mu.Unlock()
	if reads != 2 {
		t.Errorf("readbacks = %d, want provisional match followed by authoritative rejection", reads)
	}
}

// TestSourceSelectionKeepsPushConfirmationWhenReadbacksFail: a
// nowPlayingUpdated event is authoritative evidence the speaker switched.
// Once it has confirmed the selection, the readback window closing without a
// matching read must not retract that and report "unverified".
func TestSourceSelectionKeepsPushConfirmationWhenReadbacksFail(t *testing.T) {
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		// Every readback fails, so only the pushed status can confirm anything.
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		})
	})

	ctx := newHeadlessChromeContext(t)
	var statusText string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		// The speaker reports the switch before the first readback deadline.
		chromedp.Evaluate(`window.renderStatus('AUX', 'AUX1')`, nil),
		chromedp.Poll(`document.querySelector('.source-btn').classList.contains('provisional-confirmed')`, nil),
		// Outlast the last readback deadline (500ms in this fixture).
		chromedp.Sleep(900*time.Millisecond),
		chromedp.Text(`.source-command-status`, &statusText, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("exercise push-confirmed selection with failing readbacks: %v", err)
	}

	if statusText != "Source selected" {
		t.Errorf("status = %q, want the push confirmation to stand as %q", statusText, "Source selected")
	}
}

func TestSourceSelectionRejectsReadbackWithOnlyNewerAggregateRevision(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":{"revision":99,"nowPlayingRevision":1,"nowPlaying":{"Source":"AUX","SourceAccount":"AUX1"}}}}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selection unverified'`, nil),
	); err != nil {
		t.Fatalf("exercise stale source readback: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads != 3 {
		t.Errorf("readbacks = %d, want all 3 after stale matching responses", reads)
	}
}

func TestSourceSelectionTreatsFirmwareErrorSourceAsFailed(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			read := reads
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"success":true,"data":{"status":{"revision":%d,"nowPlayingRevision":%d,"nowPlaying":{"Source":"INVALID_SOURCE","SourceAccount":""}}}}`, read+1, read+1)
		})
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selection failed: INVALID_SOURCE'`, nil),
	); err != nil {
		t.Fatalf("exercise firmware source rejection: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads != 1 {
		t.Errorf("readbacks = %d, want 1 after authoritative firmware rejection", reads)
	}
}

func TestSourceSelectionLaterAuthoritativeSourceClearsFinalProjection(t *testing.T) {
	transitions := []struct {
		name, source, account string
		activeButtons         int
	}{
		{name: "airplay", source: "AIRPLAY"},
		{name: "spotify", source: "SPOTIFY", account: "spotify-user", activeButtons: 1},
		{name: "standby", source: "STANDBY"},
		{name: "invalid source", source: "INVALID_SOURCE"},
	}

	for _, transition := range transitions {
		t.Run(transition.name, func(t *testing.T) {
			var mu sync.Mutex
			reads := 0
			server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
				r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"success":true}`))
				})
				r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
					mu.Lock()
					reads++
					revision := reads + 1
					mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintf(w, `{"success":true,"data":{"status":{"revision":%d,"nowPlayingRevision":%d,"nowPlaying":{"Source":"AUX","SourceAccount":"AUX1"}}}}`, revision, revision)
				})
			})

			ctx := newHeadlessChromeContext(t)
			var commandCleared, auxInactive bool
			var activeButtons int
			if err := chromedp.Run(ctx,
				chromedp.Navigate(server.URL+"/fixture"),
				chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
				chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
				chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selected'`, nil),
				chromedp.Evaluate(fmt.Sprintf(`window.renderStatus(%q, %q, false, 'speaker', 100)`, transition.source, transition.account), nil),
				chromedp.Poll(`document.querySelector('.source-command-status').textContent === ''`, nil),
				chromedp.Evaluate(`document.querySelector('.source-command-status').textContent === ''`, &commandCleared),
				chromedp.Evaluate(`!document.querySelector('.source-btn:nth-child(1)').classList.contains('active')`, &auxInactive),
				chromedp.Evaluate(`document.querySelectorAll('.source-btn.active').length`, &activeButtons),
			); err != nil {
				t.Fatalf("apply later authoritative source: %v", err)
			}
			if !commandCleared || !auxInactive || activeButtons != transition.activeButtons {
				t.Errorf("projection after %s: cleared=%v auxInactive=%v activeButtons=%d want=%d",
					transition.source, commandCleared, auxInactive, activeButtons, transition.activeButtons)
			}
		})
	}
}

func TestSourceReadbackPublishesThroughAppDeviceState(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { mergeStatusUpdate } from '/app/static/js/app.js';
import { NowPlaying } from '/app/static/js/components/NowPlaying.js';
import { Sources } from '/app/static/js/components/Sources.js';
const sourceItems = [{ Source: 'AUX', SourceAccount: 'AUX1', DisplayName: 'Aux 1', Status: 'READY' }];
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { status: {
    revision: 1,
    nowPlayingRevision: 1,
    nowPlaying: { Source: 'STANDBY' },
    sources: { SourceItem: sourceItems },
  } } });
  const status = devices.speaker.status;
  return h('div', {},
    h(NowPlaying, { nowPlaying: status.nowPlaying }),
    h(Sources, {
      deviceId: 'speaker',
      status,
      readbackDelays: [100, 250, 500],
      onStatusReadback: next => setDevices(previous => mergeStatusUpdate(previous, 'speaker', next)),
    }),
  );
}
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			revision := reads + 1
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"status": map[string]any{
					"revision":           revision,
					"nowPlayingRevision": revision,
					"nowPlaying": map[string]string{
						"Source": "AUX", "SourceAccount": "AUX1", "Track": "Confirmed track",
					},
					"sources": map[string]any{"SourceItem": []map[string]string{
						{"Source": "AUX", "SourceAccount": "AUX1", "DisplayName": "Aux 1", "Status": "READY"},
					}},
				},
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var title string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.track-title')?.textContent === 'Confirmed track'`, nil),
		chromedp.Text(`.track-title`, &title, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selected'`, nil),
	); err != nil {
		t.Fatalf("publish source readback through app state: %v", err)
	}
	if title != "Confirmed track" {
		t.Fatalf("NowPlaying title = %q, want confirmed readback", title)
	}
}

func TestSourceSelectionResetsWhenDeviceChanges(t *testing.T) {
	var mu sync.Mutex
	writes := 0
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	var statusText string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Evaluate(`window.renderStatus('STANDBY', '', false, 'other-speaker')`, nil),
		chromedp.Sleep(600*time.Millisecond),
		chromedp.Text(`.source-command-status`, &statusText, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("switch source component device: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if statusText != "" || writes != 1 {
		t.Errorf("device switch left command state=%q or writes=%d, want cleared state and one original write", statusText, writes)
	}
}

func TestSourceSelectionFencesOlderReadbackAndStatus(t *testing.T) {
	var mu sync.Mutex
	writes := map[string]int{}
	currentSource := ""
	readRevision := 10
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, req *http.Request) {
			var body webtypes.SourceRequest
			_ = json.NewDecoder(req.Body).Decode(&body)
			mu.Lock()
			writes[body.Source]++
			currentSource = body.Source
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			source := currentSource
			readRevision++
			revision := readRevision
			mu.Unlock()
			if source == "AUX" {
				time.Sleep(400 * time.Millisecond)
			}
			w.Header().Set("Content-Type", "application/json")
			responseSource := "STANDBY"
			responseAccount := ""
			if source == "AUX" {
				responseSource = source
				responseAccount = "AUX1"
			}
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"status": map[string]any{
					"revision":           revision,
					"nowPlayingRevision": revision,
					"nowPlaying": map[string]string{
						"Source": responseSource, "SourceAccount": responseAccount,
					},
				},
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var outcome, productClass string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Click(`.source-btn:nth-child(1)`, chromedp.ByQuery),
		chromedp.Sleep(140*time.Millisecond),
		chromedp.Click(`.source-btn:nth-child(2)`, chromedp.ByQuery),
		chromedp.Evaluate(`window.renderStatus('AUX', 'AUX1')`, nil),
		chromedp.Poll(`document.querySelector('.source-command-status').textContent === 'Source selection unverified'`, nil),
		chromedp.Text(`.source-command-status`, &outcome, chromedp.ByQuery),
		chromedp.AttributeValue(`.source-btn:nth-child(2)`, "class", &productClass, nil, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("exercise source generation fence: %v", err)
	}

	if outcome != "Source selection unverified" || !strings.Contains(productClass, "unverified") {
		t.Errorf("newer outcome overwritten: status=%q class=%q", outcome, productClass)
	}
	mu.Lock()
	defer mu.Unlock()
	if writes["AUX"] != 1 || writes["PRODUCT"] != 1 {
		t.Errorf("writes = %#v, want one AUX and one PRODUCT write", writes)
	}
}

func TestStaleSourcesRemainVisibleButCannotBeSelected(t *testing.T) {
	var mu sync.Mutex
	writes := 0
	server := newPlayerFixtureServer(t, sourceFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/action/source", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			writes++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
	})

	ctx := newHeadlessChromeContext(t)
	var sourceCount, disabledCount int
	var staleText, staleRole string
	var described bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.source-btn`, chromedp.ByQuery),
		chromedp.Evaluate(`window.renderStatus('STANDBY', '', true)`, nil),
		chromedp.WaitVisible(`#source-stale-status`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelectorAll('.source-btn').length`, &sourceCount),
		chromedp.Evaluate(`document.querySelectorAll('.source-btn:disabled').length`, &disabledCount),
		chromedp.Text(`#source-stale-status`, &staleText, chromedp.ByQuery),
		chromedp.AttributeValue(`#source-stale-status`, "role", &staleRole, nil, chromedp.ByQuery),
		chromedp.Evaluate(`[...document.querySelectorAll('.source-btn')].every(button => button.getAttribute('aria-describedby') === 'source-stale-status')`, &described),
		chromedp.Evaluate(`document.querySelector('.source-btn').click()`, nil),
		chromedp.Sleep(50*time.Millisecond),
	); err != nil {
		t.Fatalf("render stale source cache: %v", err)
	}

	mu.Lock()
	staleWrites := writes
	mu.Unlock()
	if sourceCount != 3 || disabledCount != sourceCount {
		t.Errorf("stale sources: rendered=%d disabled=%d, want all 3 retained and disabled", sourceCount, disabledCount)
	}
	if staleText != "Source list out of date" || staleRole != "status" || !described {
		t.Errorf("stale indication: text=%q role=%q described=%v", staleText, staleRole, described)
	}
	if staleWrites != 0 {
		t.Errorf("stale source selection issued %d writes, want 0", staleWrites)
	}

	var enabledCount int
	var staleIndicatorMissing bool
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`window.renderStatus('STANDBY', '', false)`, nil),
		chromedp.Poll(`document.querySelectorAll('.source-btn:disabled').length === 0`, nil),
		chromedp.Evaluate(`document.querySelectorAll('.source-btn:not(:disabled)').length`, &enabledCount),
		chromedp.Evaluate(`document.querySelector('#source-stale-status') === null`, &staleIndicatorMissing),
	); err != nil {
		t.Fatalf("render refreshed source cache: %v", err)
	}
	if enabledCount != sourceCount || !staleIndicatorMissing {
		t.Errorf("fresh sources: enabled=%d want=%d staleIndicatorMissing=%v", enabledCount, sourceCount, staleIndicatorMissing)
	}

	var emptyInventoryHidden, staleEmptyAnnounced bool
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`window.renderStatus('STANDBY', '', false, 'speaker', null, [])`, nil),
		chromedp.Poll(`document.querySelector('.sources-section') === null`, nil),
		chromedp.Evaluate(`document.querySelector('.sources-section') === null`, &emptyInventoryHidden),
		// A stale empty inventory still has something to say: we know the list
		// is untrustworthy, as opposed to simply not having read one yet.
		chromedp.Evaluate(`window.renderStatus('STANDBY', '', true, 'speaker', null, [])`, nil),
		chromedp.WaitVisible(`#source-stale-status`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#source-stale-status').textContent.trim() === 'Source list out of date'`, &staleEmptyAnnounced),
	); err != nil {
		t.Fatalf("render missing source inventory: %v", err)
	}
	if !emptyInventoryHidden || !staleEmptyAnnounced {
		t.Errorf("empty inventory: hidden=%v staleAnnounced=%v", emptyInventoryHidden, staleEmptyAnnounced)
	}
}
