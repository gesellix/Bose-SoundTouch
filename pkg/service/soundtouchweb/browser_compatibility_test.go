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
	"strconv"
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

func TestDiscreteCommandsUseOneWriteAndBoundedReadbacks(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail, mergeStatusUpdate } from '/app/static/js/app.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE', ShuffleSetting: 'SHUFFLE_OFF', RepeatSetting: 'REPEAT_OFF', Track: 'First track' },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [] },
  sources: { SourceItem: [] },
};
let publishStatus;
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  publishStatus = status => setDevices(previous => mergeStatusUpdate(previous, 'speaker', status));
  return h(DeviceDetail, {
    deviceId: 'speaker',
    devices,
    onBack: () => {},
    commandReadbackDelays: [100, 250, 500],
    onStatusReadback: (deviceId, status) => setDevices(previous => mergeStatusUpdate(previous, deviceId, status)),
  });
}
window.publishStatus = status => publishStatus(status);
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	mode := ""
	reads := map[string]int{}
	powerWrites := 0
	fullReads := 0
	nowPlayingReads := 0
	var keys []string
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/power", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			mode = "power"
			powerWrites++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Post("/api/control/devices/speaker/key/{key}", func(w http.ResponseWriter, req *http.Request) {
			key := chi.URLParam(req, "key")
			mu.Lock()
			mode = map[string]string{
				"PAUSE": "pause", "MUTE": "mute", "SHUFFLE_ON": "shuffle", "REPEAT_ALL": "repeat",
				"NEXT_TRACK": "next", "PREV_TRACK": "previous",
			}[key]
			keys = append(keys, key)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		readback := func(kind string) http.HandlerFunc {
			return func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				currentMode := mode
				reads[currentMode]++
				if kind == "full" {
					fullReads++
				} else {
					nowPlayingReads++
				}
				read := reads[currentMode]
				mu.Unlock()

				revisionBase := map[string]int{
					"power": 1, "pause": 100, "mute": 200, "shuffle": 300, "repeat": 400,
					"next": 500, "previous": 600,
				}[currentMode]
				revision := read + revisionBase
				nowPlayingRevision := revision
				source := "PRODUCT"
				playStatus := "PLAY_STATE"
				shuffle := "SHUFFLE_OFF"
				repeat := "REPEAT_OFF"
				muted := false
				track := "First track"
				if currentMode == "power" && read >= 2 {
					source = "STANDBY"
					playStatus = "STOP_STATE"
				} else if currentMode == "pause" {
					if read >= 2 {
						playStatus = "STOP_STATE"
					}
				} else if currentMode == "mute" {
					nowPlayingRevision = 200
					playStatus = "PAUSE_STATE"
					if read >= 2 {
						muted = true
					}
				} else if currentMode == "shuffle" {
					playStatus = "PAUSE_STATE"
					if read >= 2 {
						shuffle = "SHUFFLE_ON"
					}
				} else if currentMode == "repeat" {
					playStatus = "PAUSE_STATE"
					shuffle = "SHUFFLE_ON"
					if read >= 2 {
						repeat = "REPEAT_ALL"
					}
				} else if currentMode == "next" {
					playStatus = "PAUSE_STATE"
					shuffle = "SHUFFLE_ON"
					repeat = "REPEAT_ALL"
					if read >= 2 {
						track = "Second track"
					}
				} else if currentMode == "previous" {
					playStatus = "PAUSE_STATE"
					shuffle = "SHUFFLE_ON"
					repeat = "REPEAT_ALL"
					track = "Second track"
					if read >= 2 {
						track = "First track"
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
					"status": map[string]any{
						"revision": revision, "nowPlayingRevision": nowPlayingRevision,
						"nowPlaying": map[string]string{
							"Source": source, "PlayStatus": playStatus,
							"ShuffleSetting": shuffle, "RepeatSetting": repeat, "Track": track,
						},
						"volume": map[string]any{"ActualVolume": 20, "MuteEnabled": muted},
					},
				}})
			}
		}
		r.Get("/api/control/devices/speaker", readback("full"))
		r.Get("/api/control/devices/speaker/now-playing", readback("now-playing"))
		r.Get("/api/control/devices/speaker/zone", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/zone/candidates", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{}})
		})
		r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{"Items": []any{}}})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var powerPending, pausePending, mutePending, shufflePending, repeatPending bool
	var nextFreedTransport bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.page-header .command-btn`, chromedp.ByQuery),
		chromedp.Click(`.page-header .command-btn`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.page-header .command-btn').getAttribute('aria-busy') === 'true'`, &powerPending),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Device powered off'`, nil),
		chromedp.Evaluate(`window.publishStatus({revision: 100, nowPlayingRevision: 100, nowPlaying: {Source: 'PRODUCT', PlayStatus: 'PLAY_STATE', ShuffleSetting: 'SHUFFLE_OFF', RepeatSetting: 'REPEAT_OFF'}, volume: {ActualVolume: 20, MuteEnabled: false}})`, nil),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === ''`, nil),
		chromedp.Click(`.play-btn`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.play-btn').getAttribute('aria-busy') === 'true'`, &pausePending),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Playback paused'`, nil),
		chromedp.Evaluate(`window.publishStatus({revision: 200, nowPlayingRevision: 200, nowPlaying: {Source: 'PRODUCT', PlayStatus: 'PAUSE_STATE', ShuffleSetting: 'SHUFFLE_OFF', RepeatSetting: 'REPEAT_OFF'}, volume: {ActualVolume: 20, MuteEnabled: false}})`, nil),
		chromedp.Click(`.mute-btn`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.mute-btn').getAttribute('aria-busy') === 'true'`, &mutePending),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Audio muted'`, nil),
		chromedp.Evaluate(`window.publishStatus({revision: 300, nowPlayingRevision: 300, nowPlaying: {Source: 'PRODUCT', PlayStatus: 'PAUSE_STATE', ShuffleSetting: 'SHUFFLE_OFF', RepeatSetting: 'REPEAT_OFF'}, volume: {ActualVolume: 20, MuteEnabled: true}})`, nil),
		chromedp.Click(`.shuffle-btn`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.shuffle-btn').getAttribute('aria-busy') === 'true'`, &shufflePending),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Shuffle enabled'`, nil),
		chromedp.Evaluate(`window.publishStatus({revision: 400, nowPlayingRevision: 400, nowPlaying: {Source: 'PRODUCT', PlayStatus: 'PAUSE_STATE', ShuffleSetting: 'SHUFFLE_ON', RepeatSetting: 'REPEAT_OFF'}, volume: {ActualVolume: 20, MuteEnabled: true}})`, nil),
		chromedp.Click(`.repeat-btn`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.repeat-btn').getAttribute('aria-busy') === 'true'`, &repeatPending),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Repeat all enabled'`, nil),
		chromedp.Evaluate(`window.publishStatus({revision: 500, nowPlayingRevision: 500, nowPlaying: {Source: 'PRODUCT', PlayStatus: 'PAUSE_STATE', ShuffleSetting: 'SHUFFLE_ON', RepeatSetting: 'REPEAT_ALL', Track: 'First track'}, volume: {ActualVolume: 20, MuteEnabled: true}})`, nil),
		chromedp.Poll(`document.querySelector('.track-title').textContent === 'First track'`, nil),
		// Track skips settle on their write, so they confirm without a
		// readback and free the transport again immediately.
		chromedp.Click(`.next-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Next track started'`, nil),
		chromedp.Evaluate(`document.querySelector('.next-btn').disabled === false`, &nextFreedTransport),
		chromedp.Evaluate(`window.publishStatus({revision: 600, nowPlayingRevision: 600, nowPlaying: {Source: 'PRODUCT', PlayStatus: 'PAUSE_STATE', ShuffleSetting: 'SHUFFLE_ON', RepeatSetting: 'REPEAT_ALL', Track: 'Second track'}, volume: {ActualVolume: 20, MuteEnabled: true}})`, nil),
		chromedp.Poll(`document.querySelector('.track-title').textContent === 'Second track'`, nil),
		chromedp.Click(`.previous-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Previous track started'`, nil),
	); err != nil {
		t.Fatalf("exercise bounded discrete commands: %v", err)
	}
	if !powerPending || !pausePending || !mutePending || !shufflePending || !repeatPending {
		t.Errorf("pending state: power=%v pause=%v mute=%v shuffle=%v repeat=%v, want all true",
			powerPending, pausePending, mutePending, shufflePending, repeatPending)
	}
	if !nextFreedTransport {
		t.Error("the transport stayed disabled after a track skip settled on its write")
	}

	mu.Lock()
	defer mu.Unlock()
	if powerWrites != 1 {
		t.Errorf("power writes = %d, want 1", powerWrites)
	}
	if got, want := fmt.Sprint(keys), "[PAUSE MUTE SHUFFLE_ON REPEAT_ALL NEXT_TRACK PREV_TRACK]"; got != want {
		t.Errorf("key writes = %s, want %s", got, want)
	}
	for _, command := range []string{"power", "pause", "mute", "shuffle", "repeat"} {
		if reads[command] != 3 {
			t.Errorf("%s readbacks = %d, want 3", command, reads[command])
		}
	}
	for _, command := range []string{"next", "previous"} {
		if reads[command] != 0 {
			t.Errorf("%s readbacks = %d, want 0 for a command that settles on its write", command, reads[command])
		}
	}
	if fullReads != 3 || nowPlayingReads != 12 {
		t.Errorf("readback endpoints: full=%d now-playing=%d, want 3 and 12", fullReads, nowPlayingReads)
	}
}

func TestPauseConfirmationAcceptsFirmwareStopWithoutConfirmingStandby(t *testing.T) {
	const fixture = `
import { matchesCommand } from '/app/static/js/discreteCommand.js';
const pause = {action: 'pause'};
window.pauseConfirmationChecks = {
  paused: matchesCommand({nowPlaying: {Source: 'PRODUCT', PlayStatus: 'PAUSE_STATE'}}, pause),
  stoppedSource: matchesCommand({nowPlaying: {Source: 'LOCAL_INTERNET_RADIO', PlayStatus: 'STOP_STATE'}}, pause),
  standby: !matchesCommand({nowPlaying: {Source: 'STANDBY', PlayStatus: 'STOP_STATE'}}, pause),
  emptySource: !matchesCommand({nowPlaying: {Source: '', PlayStatus: 'STOP_STATE'}}, pause),
};
`

	server := newPlayerFixtureServer(t, fixture, func(chi.Router) {})
	ctx := newHeadlessChromeContext(t)

	var checks map[string]bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.Poll(`window.pauseConfirmationChecks !== undefined`, nil),
		chromedp.Evaluate(`window.pauseConfirmationChecks`, &checks),
	); err != nil {
		t.Fatalf("exercise pause confirmation matrix: %v", err)
	}
	for name, passed := range checks {
		if !passed {
			t.Errorf("pause confirmation check %s failed", name)
		}
	}
}

// discreteCommandFixtureScript renders a device detail page whose readback
// delays are compressed to [100, 250, 500]ms, and exposes window.publishStatus
// so a test can play the part of the event stream.
const discreteCommandFixtureScript = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail, mergeStatusUpdate } from '/app/static/js/app.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE' },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [] },
  sources: { SourceItem: [] },
};
let publishStatus;
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  publishStatus = status => setDevices(previous => mergeStatusUpdate(previous, 'speaker', status));
  return h(DeviceDetail, {
    deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500],
    onStatusReadback: (deviceId, status) => setDevices(previous => mergeStatusUpdate(previous, deviceId, status)),
  });
}
window.publishStatus = status => publishStatus(status);
render(h(Fixture), document.getElementById('fixture'));
`

// registerDeviceDetailSideRoutes answers the requests the device detail page
// makes besides the command under test, so they neither 404 nor interfere.
func registerDeviceDetailSideRoutes(r chi.Router) {
	r.Get("/api/control/devices/speaker/zone", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
	})
	r.Get("/api/control/devices/speaker/zone/candidates", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{}})
	})
	r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{"Items": []any{}}})
	})
}

// TestDiscreteCommandStopsReadbacksOnceTheEventStreamConfirms: with a live
// event stream the first matching readback settles the command, which frees
// the transport again instead of holding every button disabled until the last
// readback deadline (10s in production). The no-event-stream half is covered
// by TestDiscreteCommandsUseOneWriteAndBoundedReadbacks, whose readbacks all
// report webSocketConnected as absent and so run to the end of the window.
func TestDiscreteCommandStopsReadbacksOnceTheEventStreamConfirms(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, discreteCommandFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/key/{key}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":{"revision":9,"nowPlayingRevision":9,` +
				`"webSocketConnected":true,"nowPlaying":{"Source":"PRODUCT","PlayStatus":"PAUSE_STATE"}}}}`))
		})
		registerDeviceDetailSideRoutes(r)
	})

	ctx := newHeadlessChromeContext(t)
	var transportEnabled bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.play-btn`, chromedp.ByQuery),
		chromedp.Click(`.play-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Playback paused'`, nil),
		chromedp.Evaluate(`document.querySelector('.play-btn').disabled === false`, &transportEnabled),
		// Outlast the remaining readback deadlines (250ms and 500ms here).
		chromedp.Sleep(900*time.Millisecond),
	); err != nil {
		t.Fatalf("exercise event-stream-confirmed command: %v", err)
	}

	if !transportEnabled {
		t.Error("transport stayed disabled after the event stream confirmed the command")
	}

	mu.Lock()
	defer mu.Unlock()
	if reads != 1 {
		t.Errorf("readbacks with a live event stream = %d, want 1", reads)
	}
}

// trackSkipFixtureScript renders the device detail page for a source that
// reports a trackID, which is what makes a skip verifiable at all.
const trackSkipFixtureScript = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail, mergeStatusUpdate } from '/app/static/js/app.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: { Source: 'SPOTIFY', PlayStatus: 'PLAY_STATE', Track: 'First track', TrackID: 'track-1' },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [] },
  sources: { SourceItem: [] },
};
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  return h(DeviceDetail, {
    deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500],
    onStatusReadback: (deviceId, status) => setDevices(previous => mergeStatusUpdate(previous, deviceId, status)),
  });
}
render(h(Fixture), document.getElementById('fixture'));
`

// trackSkipServer answers a NEXT_TRACK write and then reports the given
// trackID and track title back, with a revision that always advances.
func trackSkipServer(t *testing.T, reads *int, mu *sync.Mutex, trackID string, rollingTitle bool) *httptest.Server {
	t.Helper()

	return newPlayerFixtureServer(t, trackSkipFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/key/{key}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			*reads++
			read := *reads
			mu.Unlock()
			track := "First track"
			if rollingTitle {
				track = fmt.Sprintf("Rolling stream title %d", read)
			}
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"status": map[string]any{
					"revision": read + 1, "nowPlayingRevision": read + 1,
					"nowPlaying": map[string]any{
						"Source": "SPOTIFY", "PlayStatus": "PLAY_STATE",
						"Track": track, "TrackID": trackID,
					},
					"volume": map[string]any{"ActualVolume": 20, "MuteEnabled": false},
				},
			}})
		})
		registerDeviceDetailSideRoutes(r)
	})
}

// TestTrackSkipConfirmsByTrackID: where the speaker reports a trackID, a skip
// is genuinely verifiable, and the readbacks confirm it.
func TestTrackSkipConfirmsByTrackID(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := trackSkipServer(t, &reads, &mu, "track-2", false)

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.next-btn`, chromedp.ByQuery),
		chromedp.Click(`.next-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Next track started'`, nil),
	); err != nil {
		t.Fatalf("exercise a verifiable track skip: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads == 0 {
		t.Error("a skip against a source with a trackID was not verified by readback")
	}
}

// TestTrackSkipIsNotConfirmedByRollingStreamMetadata: a live stream rewrites
// its own track title while the same track keeps playing. That is not a skip,
// and confirming on it would report a skip that never happened as done. The
// trackID is what stays put.
func TestTrackSkipIsNotConfirmedByRollingStreamMetadata(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := trackSkipServer(t, &reads, &mu, "track-1", true)

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.next-btn`, chromedp.ByQuery),
		chromedp.Click(`.next-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Next-track command unverified'`, nil),
	); err != nil {
		t.Fatalf("exercise a skip against rolling stream metadata: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads != 3 {
		t.Errorf("readbacks = %d, want all 3 while the trackID never changed", reads)
	}
}

// libraryFixtureScript renders the device detail page for a source shaped like
// STORED_MUSIC as measured on hardware: it claims skipEnabled but reports no
// trackID, so only its track metadata can confirm a skip.
const libraryFixtureScript = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail, mergeStatusUpdate } from '/app/static/js/app.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: {
    Source: 'STORED_MUSIC', PlayStatus: 'PLAY_STATE',
    Track: 'First track', Artist: 'An artist', Album: 'An album',
    SkipEnabled: {}, SkipPreviousEnabled: {},
  },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [] },
  sources: { SourceItem: [] },
};
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  return h(DeviceDetail, {
    deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500],
    onStatusReadback: (deviceId, status) => setDevices(previous => mergeStatusUpdate(previous, deviceId, status)),
  });
}
render(h(Fixture), document.getElementById('fixture'));
`

// TestTrackSkipConfirmsByMetadataWhereTheSourceClaimsSkip: a media library
// changes its track title only when the track really changes, and says so by
// claiming skipEnabled. Measured on hardware: STORED_MUSIC reports
// skipEnabled and skipPreviousEnabled with no trackID at all, so refusing to
// verify without a trackID would leave a whole source unverifiable.
func TestTrackSkipConfirmsByMetadataWhereTheSourceClaimsSkip(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, libraryFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/key/{key}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			read := reads
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"status": map[string]any{
					"revision": read + 1, "nowPlayingRevision": read + 1,
					"nowPlaying": map[string]any{
						"Source": "STORED_MUSIC", "PlayStatus": "PLAY_STATE",
						"Track": "Second track", "Artist": "An artist", "Album": "An album",
						"SkipEnabled": map[string]any{}, "SkipPreviousEnabled": map[string]any{},
					},
					"volume": map[string]any{"ActualVolume": 20, "MuteEnabled": false},
				},
			}})
		})
		registerDeviceDetailSideRoutes(r)
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.next-btn`, chromedp.ByQuery),
		chromedp.Click(`.next-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Next track started'`, nil),
	); err != nil {
		t.Fatalf("exercise a library skip confirmed by metadata: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads == 0 {
		t.Error("a skip against a source claiming skipEnabled was not verified by readback")
	}
}

// TestTrackSkipSettlesOnWriteWhereTheSourceClaimsNoSkip: live radio rewrites
// its own title while the same stream plays on, and claims no skip support.
// Measured on hardware: TUNEIN and RADIO_BROWSER report neither a trackID nor
// skipEnabled. Trusting the title there would confirm skips that never
// happened, so nothing is read back at all.
func TestTrackSkipSettlesOnWriteWhereTheSourceClaimsNoSkip(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail, mergeStatusUpdate } from '/app/static/js/app.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: {
    Source: 'TUNEIN', PlayStatus: 'PLAY_STATE',
    Track: 'Rolling stream title', StationName: 'A station',
  },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [] },
  sources: { SourceItem: [] },
};
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  return h(DeviceDetail, {
    deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500],
    onStatusReadback: (deviceId, status) => setDevices(previous => mergeStatusUpdate(previous, deviceId, status)),
  });
}
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	reads := 0
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/key/{key}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			reads++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"status": map[string]any{
					"revision": 9, "nowPlayingRevision": 9,
					"nowPlaying": map[string]any{
						"Source": "TUNEIN", "PlayStatus": "PLAY_STATE",
						"Track": "A different rolling title", "StationName": "A station",
					},
					"volume": map[string]any{"ActualVolume": 20, "MuteEnabled": false},
				},
			}})
		})
		registerDeviceDetailSideRoutes(r)
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.next-btn`, chromedp.ByQuery),
		chromedp.Click(`.next-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Next track started'`, nil),
		// Outlast every readback deadline in the fixture.
		chromedp.Sleep(900*time.Millisecond),
	); err != nil {
		t.Fatalf("exercise a skip against a source claiming no skip support: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if reads != 0 {
		t.Errorf("readbacks = %d, want 0 where a rolling title is the only thing that changes", reads)
	}
}

// TestPreviousTrackConfirmsARestart: measured on a SoundTouch 10, the first
// PREV_TRACK restarts the current track and only a second press steps back.
// The restart changes no identity, so without reading the play position a
// working Previous would report "unverified" every time.
func TestPreviousTrackConfirmsARestart(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail, mergeStatusUpdate } from '/app/static/js/app.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: {
    Source: 'STORED_MUSIC', PlayStatus: 'PLAY_STATE',
    Track: 'Something To Believe', Artist: 'An artist', Album: 'An album',
    Time: { Total: 0, Position: 97 },
    SkipEnabled: {}, SkipPreviousEnabled: {},
  },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [] },
  sources: { SourceItem: [] },
};
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  return h(DeviceDetail, {
    deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500],
    onStatusReadback: (deviceId, status) => setDevices(previous => mergeStatusUpdate(previous, deviceId, status)),
  });
}
render(h(Fixture), document.getElementById('fixture'));
`

	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/key/{key}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		// The same track, back at the beginning: a restart, not a step back.
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"status": map[string]any{
					"revision": 5, "nowPlayingRevision": 5,
					"nowPlaying": map[string]any{
						"Source": "STORED_MUSIC", "PlayStatus": "PLAY_STATE",
						"Track": "Something To Believe", "Artist": "An artist", "Album": "An album",
						"Time":        map[string]any{"Total": 0, "Position": 2},
						"SkipEnabled": map[string]any{}, "SkipPreviousEnabled": map[string]any{},
					},
					"volume": map[string]any{"ActualVolume": 20, "MuteEnabled": false},
				},
			}})
		})
		registerDeviceDetailSideRoutes(r)
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.previous-btn`, chromedp.ByQuery),
		chromedp.Click(`.previous-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Previous track started'`, nil),
	); err != nil {
		t.Fatalf("exercise a previous-track restart: %v", err)
	}
}

// TestRecentsBorrowArtworkFromAMatchingPreset: the speaker sends no artwork
// with /recents (measured on a SoundTouch 10: every preset carried
// containerArt and not one of ten recents did), so the player borrows it from
// a preset for the same content. The recents entry here also carries the
// echoed source-name account the speaker writes there, which is what a naive
// comparison against the preset's empty account would miss.
func TestRecentsBorrowArtworkFromAMatchingPreset(t *testing.T) {
	const art = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"

	fixture := `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail } from '/app/static/js/app.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: { Source: 'TUNEIN', PlayStatus: 'PLAY_STATE' },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [{ ID: 1, ContentItem: {
    Source: 'TUNEIN', SourceAccount: '', Location: '/v1/playback/station/s6634',
    ItemName: 'A Station', ContainerArt: '` + art + `',
  } }] },
  sources: { SourceItem: [] },
};
function Fixture() {
  const [devices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  return h(DeviceDetail, { deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500] });
}
render(h(Fixture), document.getElementById('fixture'));
`

	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{"Items": []any{
				// No ContainerArt, and the account the speaker echoes back.
				map[string]any{"ID": "recent-1", "ContentItem": map[string]any{
					"Source": "TUNEIN", "SourceAccount": "TUNEIN",
					"Location": "/v1/playback/station/s6634", "ItemName": "A Station",
				}},
				map[string]any{"ID": "recent-2", "ContentItem": map[string]any{
					"Source": "TUNEIN", "SourceAccount": "TUNEIN",
					"Location": "/v1/playback/station/s999999", "ItemName": "Not A Preset",
				}},
			}}})
		})
		r.Get("/api/control/devices/speaker/zone", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/zone/candidates", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{}})
		})
	})

	ctx := newHeadlessChromeContext(t)

	var borrowed string

	var fallbacks int

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.recent-item`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.recent-item img.recent-art')?.getAttribute('src') || ''`, &borrowed),
		// The entry with no matching preset keeps the source-icon placeholder.
		chromedp.Evaluate(`document.querySelectorAll('.recent-art-empty').length`, &fallbacks),
	); err != nil {
		t.Fatalf("exercise recents artwork borrowing: %v", err)
	}

	if borrowed != art {
		t.Errorf("recents artwork = %q, want the matching preset's art", borrowed)
	}

	if fallbacks != 1 {
		t.Errorf("source-icon fallbacks = %d, want 1 for the entry with no matching preset", fallbacks)
	}
}

// TestEmptyPresetSlotStaysSavableWhileACommandIsPending: an empty slot's
// tile is a save gesture, not a playback command, so an in-flight command
// has no reason to disable it -- and the sibling star button, which saves the
// same thing, stays enabled throughout anyway.
func TestEmptyPresetSlotStaysSavableWhileACommandIsPending(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail, mergeStatusUpdate } from '/app/static/js/app.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE' },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [{ ID: 1, ContentItem: { Source: 'TUNEIN', Location: '/station/preset', ItemName: 'Preset station' } }] },
  sources: { SourceItem: [] },
};
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  return h(DeviceDetail, {
    deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500],
    onStatusReadback: (deviceId, status) => setDevices(previous => mergeStatusUpdate(previous, deviceId, status)),
  });
}
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	storedSlots := []string{}
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/action/preset", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/action/storepreset", func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			storedSlots = append(storedSlots, req.URL.Query().Get("id"))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		// Never matches the preset, so the command stays pending for the whole
		// readback window while the empty slot is clicked.
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"status": map[string]any{
					"revision": 1, "nowPlayingRevision": 1,
					"nowPlaying": map[string]any{"Source": "PRODUCT", "PlayStatus": "PLAY_STATE"},
					"volume":     map[string]any{"ActualVolume": 20, "MuteEnabled": false},
				},
			}})
		})
		registerDeviceDetailSideRoutes(r)
	})

	ctx := newHeadlessChromeContext(t)
	var savableEnabled bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.preset-slot.savable`, chromedp.ByQuery),
		chromedp.Click(`.preset-slot:not(.empty)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Starting preset'`, nil),
		chromedp.Evaluate(`document.querySelector('.preset-slot.savable').disabled === false`, &savableEnabled),
		chromedp.Click(`.preset-slot.savable`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
	); err != nil {
		t.Fatalf("exercise empty preset slot during a pending command: %v", err)
	}

	if !savableEnabled {
		t.Error("the empty slot's save gesture was disabled while a command was pending")
	}

	mu.Lock()
	defer mu.Unlock()
	if got, want := fmt.Sprint(storedSlots), "[2]"; got != want {
		t.Errorf("stored preset slots = %s, want %s", got, want)
	}
}

// TestControlsStayLiveOnAnUnpolledSpeaker: a speaker that is offline or has
// not been polled yet reports no state, so nothing can be reconciled -- but
// the power button and the transport must still work. That is exactly the
// speaker you would want to power cycle from the UI.
func TestControlsStayLiveOnAnUnpolledSpeaker(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail } from '/app/static/js/app.js';
function Fixture() {
  const [devices] = useState({ speaker: { info: { name: 'Speaker' } } });
  return h(DeviceDetail, {
    deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500],
  });
}
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	powerWrites := 0
	var keys []string
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/power", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			powerWrites++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Post("/api/control/devices/speaker/key/{key}", func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			keys = append(keys, chi.URLParam(req, "key"))
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		})
		registerDeviceDetailSideRoutes(r)
	})

	ctx := newHeadlessChromeContext(t)
	var powerEnabled, transportEnabled bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.page-header .command-btn`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.page-header .command-btn').disabled === false`, &powerEnabled),
		chromedp.Evaluate(`document.querySelector('.play-btn').disabled === false`, &transportEnabled),
		chromedp.Click(`.page-header .command-btn`, chromedp.ByQuery),
		chromedp.Click(`.play-btn`, chromedp.ByQuery),
		// Both writes are fire-and-forget, so give them a moment to land.
		chromedp.Sleep(300*time.Millisecond),
	); err != nil {
		t.Fatalf("exercise controls on an unpolled speaker: %v", err)
	}

	if !powerEnabled || !transportEnabled {
		t.Errorf("enabled state: power=%v transport=%v, want both true", powerEnabled, transportEnabled)
	}

	mu.Lock()
	defer mu.Unlock()
	if powerWrites != 1 {
		t.Errorf("power writes = %d, want 1", powerWrites)
	}
	if got, want := fmt.Sprint(keys), "[PLAY]"; got != want {
		t.Errorf("key writes = %s, want %s", got, want)
	}
}

// TestDiscreteCommandConfirmsAcrossAReconnectEpoch: revisions restart at 0
// when a device id is backed by a new DeviceConnection or the service is
// restarted, so they only compare within one epoch. Comparing them raw made
// every readback after a mid-command reconnect look older than the revision
// the command started at, and the command could never confirm.
func TestDiscreteCommandConfirmsAcrossAReconnectEpoch(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail, mergeStatusUpdate } from '/app/static/js/app.js';
const initialStatus = {
  epoch: 1,
  revision: 50,
  nowPlayingRevision: 50,
  nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE' },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [] },
  sources: { SourceItem: [] },
};
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  return h(DeviceDetail, {
    deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500],
    onStatusReadback: (deviceId, status) => setDevices(previous => mergeStatusUpdate(previous, deviceId, status)),
  });
}
render(h(Fixture), document.getElementById('fixture'));
`

	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/key/{key}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		// The connection was re-established mid-command: a new epoch, whose
		// revisions start again from 0.
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":{"epoch":2,"revision":3,"nowPlayingRevision":3,` +
				`"nowPlaying":{"Source":"PRODUCT","PlayStatus":"PAUSE_STATE"}}}}`))
		})
		registerDeviceDetailSideRoutes(r)
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.play-btn`, chromedp.ByQuery),
		chromedp.Click(`.play-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Playback paused'`, nil),
	); err != nil {
		t.Fatalf("exercise command confirmation across a reconnect: %v", err)
	}
}

// TestDiscreteCommandKeepsPushConfirmationWhenReadbacksFail: a command the
// event stream already confirmed must not be retracted by a later readback
// that fails. Announcing "unverified" for a pause the speaker demonstrably
// performed is worse than saying nothing.
func TestDiscreteCommandKeepsPushConfirmationWhenReadbacksFail(t *testing.T) {
	server := newPlayerFixtureServer(t, discreteCommandFixtureScript, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/key/{key}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		})
		// Every readback fails, so only the pushed status can confirm anything.
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		})
		registerDeviceDetailSideRoutes(r)
	})

	ctx := newHeadlessChromeContext(t)
	var statusText string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.play-btn`, chromedp.ByQuery),
		chromedp.Click(`.play-btn`, chromedp.ByQuery),
		// The speaker reports the pause before the first readback deadline.
		chromedp.Evaluate(`window.publishStatus({revision: 100, nowPlayingRevision: 100, nowPlaying: {Source: 'PRODUCT', PlayStatus: 'PAUSE_STATE'}, volume: {ActualVolume: 20, MuteEnabled: false}})`, nil),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Playback paused, confirming'`, nil),
		// Outlast the last readback deadline (500ms in this fixture).
		chromedp.Sleep(900*time.Millisecond),
		chromedp.Text(`.discrete-command-status`, &statusText, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("exercise push-confirmed command with failing readbacks: %v", err)
	}

	if statusText != "Playback paused" {
		t.Errorf("status = %q, want the push confirmation to stand as %q", statusText, "Playback paused")
	}
}

// TestPlayConfirmationAcceptsBuffering: BUFFERING_STATE is a real firmware
// play status (models.PlayStatusBuffering), and internet radio sits in it for
// seconds at a time. Requiring PLAY_STATE reported a working play command as
// unverified and held the transport disabled for the whole readback window.
func TestPlayConfirmationAcceptsBuffering(t *testing.T) {
	const fixture = `
import { matchesCommand } from '/app/static/js/discreteCommand.js';
const play = {action: 'play'};
window.playConfirmationChecks = {
  playing: matchesCommand({nowPlaying: {Source: 'TUNEIN', PlayStatus: 'PLAY_STATE'}}, play),
  buffering: matchesCommand({nowPlaying: {Source: 'TUNEIN', PlayStatus: 'BUFFERING_STATE'}}, play),
  paused: !matchesCommand({nowPlaying: {Source: 'TUNEIN', PlayStatus: 'PAUSE_STATE'}}, play),
  standby: !matchesCommand({nowPlaying: {Source: 'STANDBY', PlayStatus: 'BUFFERING_STATE'}}, play),
  emptySource: !matchesCommand({nowPlaying: {Source: '', PlayStatus: 'BUFFERING_STATE'}}, play),
};
`

	server := newPlayerFixtureServer(t, fixture, func(chi.Router) {})
	ctx := newHeadlessChromeContext(t)

	var checks map[string]bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.Poll(`window.playConfirmationChecks !== undefined`, nil),
		chromedp.Evaluate(`window.playConfirmationChecks`, &checks),
	); err != nil {
		t.Fatalf("exercise play confirmation matrix: %v", err)
	}
	for name, passed := range checks {
		if !passed {
			t.Errorf("play confirmation check %s failed", name)
		}
	}
}

func TestPresetAndRecentCommandsUseOneWriteAndBoundedReadbacks(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { DeviceDetail, mergeStatusUpdate } from '/app/static/js/app.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE' },
  volume: { ActualVolume: 20, MuteEnabled: false },
  presets: { Preset: [{ ID: 1, ContentItem: { Source: 'TUNEIN', Location: '/station/preset', ItemName: 'Preset station' } }] },
  sources: { SourceItem: [] },
};
let publishStatus;
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { name: 'Speaker' }, status: initialStatus } });
  publishStatus = status => setDevices(previous => mergeStatusUpdate(previous, 'speaker', status));
  return h(DeviceDetail, {
    deviceId: 'speaker', devices, onBack: () => {}, commandReadbackDelays: [100, 250, 500],
    onStatusReadback: (deviceId, status) => setDevices(previous => mergeStatusUpdate(previous, deviceId, status)),
  });
}
window.publishStatus = status => publishStatus(status);
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	mode := ""
	reads := map[string]int{}
	presetWrites := 0
	recentWrites := 0
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/action/preset", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			mode = "preset"
			presetWrites++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Post("/api/control/devices/speaker/play", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			mode = "recent"
			recentWrites++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			currentMode := mode
			reads[currentMode]++
			read := reads[currentMode]
			mu.Unlock()

			revisionBase := map[string]int{"preset": 1, "recent": 100}[currentMode]
			source := "PRODUCT"
			location := ""
			name := "Original"
			if currentMode == "preset" && read >= 2 {
				source, location, name = "TUNEIN", "/station/preset", "Preset station"
			} else if currentMode == "recent" && read >= 2 {
				source, location, name = "LOCAL_INTERNET_RADIO", "/station/recent", "Recent station"
			}
			revision := revisionBase + read
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"status": map[string]any{
					"revision": revision, "nowPlayingRevision": revision,
					"nowPlaying": map[string]any{
						"Source": source, "PlayStatus": "PLAY_STATE",
						"ContentItem": map[string]string{"Source": source, "Location": location, "ItemName": name},
					},
					"volume": map[string]any{"ActualVolume": 20, "MuteEnabled": false},
				},
			}})
		})
		r.Get("/api/control/devices/speaker/zone", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/zone/candidates", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{}})
		})
		r.Get("/api/control/devices/speaker/recents", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{"Items": []any{
				map[string]any{"ID": "recent-1", "ContentItem": map[string]any{
					"Source": "LOCAL_INTERNET_RADIO", "Location": "/station/recent", "ItemName": "Recent station",
				}},
			}}})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var presetPending, recentPending bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.preset-slot:not(.empty)`, chromedp.ByQuery),
		chromedp.Click(`.preset-slot:not(.empty)`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.preset-slot:not(.empty)').getAttribute('aria-busy') === 'true'`, &presetPending),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Preset started'`, nil),
		chromedp.Evaluate(`window.publishStatus({revision: 100, nowPlayingRevision: 100, nowPlaying: {Source: 'PRODUCT', PlayStatus: 'PLAY_STATE'}, volume: {ActualVolume: 20, MuteEnabled: false}, presets: {Preset: [{ID: 1, ContentItem: {Source: 'TUNEIN', Location: '/station/preset', ItemName: 'Preset station'}}]}})`, nil),
		chromedp.WaitVisible(`.recent-item`, chromedp.ByQuery),
		chromedp.Click(`.recent-item`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.recent-item').getAttribute('aria-busy') === 'true'`, &recentPending),
		chromedp.Poll(`document.querySelector('.discrete-command-status').textContent === 'Recent item started'`, nil),
	); err != nil {
		t.Fatalf("exercise preset and recent command state: %v", err)
	}
	if !presetPending || !recentPending {
		t.Errorf("pending state: preset=%v recent=%v, want both true", presetPending, recentPending)
	}

	mu.Lock()
	defer mu.Unlock()
	if presetWrites != 1 || recentWrites != 1 {
		t.Errorf("writes: preset=%d recent=%d, want 1 each", presetWrites, recentWrites)
	}
	if reads["preset"] != 3 || reads["recent"] != 3 {
		t.Errorf("readbacks: preset=%d recent=%d, want 3 each", reads["preset"], reads["recent"])
	}
}

func TestProviderAndLibraryPlaybackUseOneWriteAndBoundedReadbacks(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState, useRef } from 'preact/hooks';
import { mergeStatusUpdate } from '/app/static/js/app.js';
import { ContentPlaybackCommand } from '/app/static/js/components/ContentPlaybackCommand.js';
import { TuneInBrowser } from '/app/static/js/components/TuneInBrowser.js';
import { RadioBrowser } from '/app/static/js/components/RadioBrowser.js';
import { PlayURL } from '/app/static/js/components/PlayURL.js';
import { Library } from '/app/static/js/components/Library.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE' },
  volume: { ActualVolume: 20, MuteEnabled: false },
};
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { device_id: 'DEVICE1', name: 'Speaker', ip_address: '192.0.2.1' }, status: initialStatus } });
  const [request, setRequest] = useState(null);
  const [busy, setBusy] = useState(false);
  const [showTuneIn, setShowTuneIn] = useState(true);
  const owner = useRef({ generation: 0, busy: false });
  const onStatusReadback = (deviceId, status, info) => setDevices(previous => {
    if (info?.device_id && previous[deviceId]?.info?.device_id !== info.device_id) return previous;
    return mergeStatusUpdate(previous, deviceId, status);
  });
  const onPlaybackRequest = next => {
    if (owner.current.busy) return false;
    owner.current.busy = true;
    owner.current.generation += 1;
    setBusy(true);
    setRequest({ ...next, key: 'command-' + owner.current.generation, targetIdentity: devices[next.deviceId]?.info?.device_id });
    return true;
  };
  const onStateChange = state => {
    owner.current.busy = state.busy;
    setBusy(state.busy);
  };
  const onClear = () => {
    if (!owner.current.busy) setRequest(null);
  };
  window.hideTuneIn = () => setShowTuneIn(false);
  const props = { devices, onPlaybackRequest, playbackBusy: busy, commandReadbackDelays: [100, 250, 500] };
  return h('div', {}, [
    request ? h(ContentPlaybackCommand, { key: request.key, request, devices, onStatusReadback, onStateChange, onClear }) : null,
    showTuneIn ? h('section', { id: 'tunein' }, h(TuneInBrowser, props)) : null,
    h('section', { id: 'radio' }, h(RadioBrowser, props)),
    h('section', { id: 'url' }, h(PlayURL, { ...props, serverServiceUrl: 'http://aftertouch.test' })),
    h('section', { id: 'library' }, h(Library, props)),
  ]);
}
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	mode := ""
	reads := map[string]int{}
	writes := map[string]int{}
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/providers/tunein/navigate", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"bmx_sections": []any{
					map[string]any{
						"name": "Stations",
						"items": []any{
							map[string]any{
								"name": "TuneIn station",
								"_links": map[string]any{"bmx_playback": map[string]string{
									"href": "/tunein/station", "type": "stationurl",
								}},
							},
						},
					},
				},
			}})
		})
		r.Get("/api/control/providers/radiobrowser/search", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"bmx_sections": []any{
					map[string]any{
						"name": "Stations",
						"items": []any{
							map[string]any{
								"name": "RadioBrowser station",
								"_links": map[string]any{"bmx_playback": map[string]string{
									"href": "/radiobrowser/station", "type": "stationurl",
								}},
							},
						},
					},
				},
			}})
		})
		r.Get("/api/control/devices/speaker/library/servers", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: []any{
				map[string]any{"udn": "uuid:library", "name": "Media server", "ready": true},
			}})
		})
		r.Get("/api/control/devices/speaker/library/browse", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"entries": []any{map[string]any{
					"name": "Library track", "location": "/library/track", "type": "track", "playable": true,
				}},
			}})
		})

		registerWrite := func(command string) http.HandlerFunc {
			return func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				mode = command
				writes[command]++
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
			}
		}
		r.Post("/api/control/devices/speaker/providers/tunein/play", registerWrite("tunein"))
		r.Post("/api/control/devices/speaker/providers/radiobrowser/play", registerWrite("radiobrowser"))
		r.Post("/api/control/devices/speaker/providers/url/play", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			mode = "url"
			writes["url"]++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]string{
				"source": "LOCAL_INTERNET_RADIO", "location": "/orion/fixture", "itemName": "Fixture stream",
			}})
		})
		r.Post("/api/control/devices/speaker/library/play", registerWrite("library"))
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			currentMode := mode
			reads[currentMode]++
			read := reads[currentMode]
			mu.Unlock()

			type expectedContent struct {
				source   string
				account  string
				location string
				name     string
			}
			expected := map[string]expectedContent{
				"tunein":       {source: "TUNEIN", location: "/tunein/station", name: "TuneIn station"},
				"radiobrowser": {source: "RADIO_BROWSER", location: "/radiobrowser/station", name: "RadioBrowser station"},
				"url":          {source: "LOCAL_INTERNET_RADIO", location: "/orion/fixture", name: "Fixture stream"},
				"library":      {source: "STORED_MUSIC", account: "uuid:library/0", location: "/library/track", name: "Library track"},
			}[currentMode]
			base := map[string]int{"tunein": 10, "radiobrowser": 20, "url": 30, "library": 40}[currentMode]
			revision := base + read
			source, account, location, name := "PRODUCT", "", "", "Original"
			if read >= 2 {
				source, account, location, name = expected.source, expected.account, expected.location, expected.name
			}
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"info": map[string]string{"device_id": "DEVICE1"},
				"status": map[string]any{
					"revision": revision, "nowPlayingRevision": revision,
					"nowPlaying": map[string]any{
						"Source": source, "SourceAccount": account, "PlayStatus": "PLAY_STATE",
						"ContentItem": map[string]string{
							"Source": source, "SourceAccount": account, "Location": location, "ItemName": name,
						},
					},
					"volume": map[string]any{"ActualVolume": 20, "MuteEnabled": false},
				},
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var radioDisabledWhilePending bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`#tunein .tunein-play-btn`, chromedp.ByQuery),
		chromedp.SetValue(`#radio .tunein-search-input`, "station", chromedp.ByQuery),
		chromedp.Click(`#radio .btn-primary`, chromedp.ByQuery),
		chromedp.WaitVisible(`#radio .tunein-play-btn`, chromedp.ByQuery),
		chromedp.Click(`#tunein .tunein-play-btn`, chromedp.ByQuery),
		chromedp.Click(`#tunein .picker-device-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#radio .tunein-play-btn')?.disabled === true`, nil),
		chromedp.Evaluate(`document.querySelector('#radio .tunein-play-btn').disabled`, &radioDisabledWhilePending),
		chromedp.Evaluate(`window.hideTuneIn()`, nil),
		chromedp.Poll(`document.querySelector('.content-command-status')?.classList.contains('final-confirmed')`, nil),

		chromedp.Click(`#radio .tunein-play-btn`, chromedp.ByQuery),
		chromedp.Click(`#radio .picker-device-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status')?.classList.contains('final-confirmed')`, nil),

		chromedp.SetValue(`#url input[type="url"]`, "http://stream.test/audio", chromedp.ByQuery),
		chromedp.SetValue(`#url input[type="text"]`, "Fixture stream", chromedp.ByQuery),
		chromedp.Click(`#url .tunein-toolbar .btn-primary`, chromedp.ByQuery),
		chromedp.Click(`#url .picker-device-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status')?.classList.contains('final-confirmed')`, nil),

		chromedp.WaitVisible(`#library .tunein-item`, chromedp.ByQuery),
		chromedp.Click(`#library .tunein-item`, chromedp.ByQuery),
		chromedp.WaitVisible(`#library .tunein-play-btn`, chromedp.ByQuery),
		chromedp.Click(`#library .tunein-play-btn`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status')?.classList.contains('final-confirmed')`, nil),
	); err != nil {
		t.Fatalf("exercise provider and library playback command state: %v", err)
	}
	if !radioDisabledWhilePending {
		t.Error("a second provider command remained enabled while the first command was pending")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, command := range []string{"tunein", "radiobrowser", "url", "library"} {
		if writes[command] != 1 {
			t.Errorf("%s writes = %d, want 1", command, writes[command])
		}
		if reads[command] != 3 {
			t.Errorf("%s readbacks = %d, want 3", command, reads[command])
		}
	}
}

func TestContentPlaybackCommandFencesTargetsAndClassifiesOutcomes(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { api } from '/app/static/js/api.js';
import { mergeStatusUpdate } from '/app/static/js/app.js';
import { ContentPlaybackCommand } from '/app/static/js/components/ContentPlaybackCommand.js';
const initialStatus = {
  revision: 1,
  nowPlayingRevision: 1,
  nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE' },
  volume: { ActualVolume: 20, MuteEnabled: false },
};
function Fixture() {
  const [devices, setDevices] = useState({ speaker: { info: { device_id: 'DEVICE1', name: 'Speaker' }, status: initialStatus } });
  const [request, setRequest] = useState(null);
  const commandRequest = (scenario, targetIdentity = devices.speaker.info.device_id) => ({
    key: scenario + '-' + Date.now(),
    deviceId: 'speaker',
    targetIdentity,
    action: 'tunein',
    readbackDelays: [100, 250, 500],
    invoke: () => api.tuneInPlayChecked('speaker', { location: '/' + scenario, type: 'stationurl', name: scenario }),
    expected: { source: 'TUNEIN', location: '/' + scenario, itemName: scenario },
  });
  const start = scenario => setRequest(commandRequest(scenario));
  const startWithReplacement = () => {
    const targetIdentity = devices.speaker.info.device_id;
    setRequest(commandRequest('pretarget', targetIdentity));
    setDevices(previous => ({
      ...previous,
      speaker: { ...previous.speaker, info: { ...previous.speaker.info, device_id: 'DEVICE2' } },
    }));
  };
  const onStatusReadback = (deviceId, status, info) => setDevices(previous => {
    if (info?.device_id && previous[deviceId]?.info?.device_id !== info.device_id) return previous;
    return mergeStatusUpdate(previous, deviceId, status);
  });
  window.replaceTarget = () => setDevices(previous => ({
    ...previous,
    speaker: { ...previous.speaker, info: { ...previous.speaker.info, device_id: 'DEVICE2' } },
  }));
  window.restoreTarget = () => setDevices(previous => ({
    ...previous,
    speaker: { ...previous.speaker, info: { ...previous.speaker.info, device_id: 'DEVICE1' }, status: initialStatus },
  }));
  window.publishNewer = () => setDevices(previous => mergeStatusUpdate(previous, 'speaker', {
    revision: 100,
    nowPlayingRevision: 100,
    nowPlaying: { Source: 'PRODUCT', PlayStatus: 'PLAY_STATE', Track: 'Newer external state' },
    volume: { ActualVolume: 20, MuteEnabled: false },
  }));
  return h('div', {}, [
    h('button', { id: 'reject', onClick: () => start('reject') }, 'Reject'),
    h('button', { id: 'target', onClick: () => start('target') }, 'Target'),
    h('button', { id: 'pretarget', onClick: startWithReplacement }, 'Pre-write target'),
    h('button', { id: 'indefinite', onClick: () => start('indefinite') }, 'Indefinite'),
    h('button', { id: 'stale', onClick: () => start('stale') }, 'Stale'),
    request ? h(ContentPlaybackCommand, {
      key: request.key,
      request,
      devices,
      onStatusReadback,
      onClear: () => setRequest(null),
    }) : null,
  ]);
}
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	mode := ""
	reads := map[string]int{}
	writes := map[string]int{}
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Post("/api/control/devices/speaker/providers/tunein/play", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Location string `json:"location"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			currentMode := strings.TrimPrefix(body.Location, "/")
			mu.Lock()
			mode = currentMode
			writes[currentMode]++
			mu.Unlock()
			// 4xx is the only definitive refusal: the service produces it
			// before any speaker call, so nothing was sent onward. 5xx is not
			// proof of anything, since a request that timed out after the
			// speaker already acted looks exactly like one it never received.
			if currentMode == "reject" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: false, Error: "catalog rejected"})
				return
			}
			if currentMode == "indefinite" {
				http.Error(w, "speaker call failed", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
		r.Get("/api/control/devices/speaker/now-playing", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			currentMode := mode
			reads[currentMode]++
			read := reads[currentMode]
			mu.Unlock()
			revision := read + 1
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"info": map[string]string{"device_id": "DEVICE1"},
				"status": map[string]any{
					"revision": revision, "nowPlayingRevision": revision,
					"nowPlaying": map[string]any{
						"Source": "TUNEIN", "PlayStatus": "PLAY_STATE",
						"ContentItem": map[string]string{
							"Source": "TUNEIN", "Location": "/" + currentMode, "ItemName": currentMode,
						},
					},
					"volume": map[string]any{"ActualVolume": 20, "MuteEnabled": false},
				},
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.Click(`#reject`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status')?.classList.contains('failed')`, nil),
		chromedp.Poll(`document.querySelector('.content-command-error')?.textContent === 'catalog rejected'`, nil),
		chromedp.Click(`.content-command-status button`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status') === null`, nil),

		chromedp.Click(`#target`, chromedp.ByQuery),
		chromedp.WaitVisible(`.content-command-status.pending`, chromedp.ByQuery),
		chromedp.Evaluate(`window.replaceTarget()`, nil),
		chromedp.Poll(`document.querySelector('.content-command-status')?.classList.contains('failed')`, nil),
		chromedp.Poll(`document.querySelector('.content-command-error')?.textContent === 'Playback target changed'`, nil),
		chromedp.Click(`.content-command-status button`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status') === null`, nil),
		chromedp.Evaluate(`window.restoreTarget()`, nil),

		chromedp.Click(`#pretarget`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status')?.classList.contains('failed')`, nil),
		chromedp.Poll(`document.querySelector('.content-command-error')?.textContent === 'Playback target changed'`, nil),
		chromedp.Click(`.content-command-status button`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status') === null`, nil),
		chromedp.Evaluate(`window.restoreTarget()`, nil),

		// A 5xx says nothing about whether the speaker acted, so the readbacks
		// must keep verifying and can still confirm the command.
		chromedp.Click(`#indefinite`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status')?.classList.contains('final-confirmed')`, nil),
		chromedp.Click(`.content-command-status button`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.content-command-status') === null`, nil),

		chromedp.Click(`#stale`, chromedp.ByQuery),
		chromedp.WaitVisible(`.content-command-status.pending`, chromedp.ByQuery),
		chromedp.Evaluate(`window.publishNewer()`, nil),
		chromedp.Poll(`document.querySelector('.content-command-status')?.classList.contains('unverified')`, nil),
	); err != nil {
		t.Fatalf("exercise negative content playback outcomes: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, command := range []string{"reject", "target", "indefinite", "stale"} {
		if writes[command] != 1 {
			t.Errorf("%s writes = %d, want 1", command, writes[command])
		}
	}
	if reads["reject"] != 0 {
		t.Errorf("rejected command readbacks = %d, want 0 after definite rejection", reads["reject"])
	}
	if reads["stale"] != 3 {
		t.Errorf("stale command readbacks = %d, want 3", reads["stale"])
	}
	if writes["pretarget"] != 0 {
		t.Errorf("pre-write target replacement writes = %d, want 0", writes["pretarget"])
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

// TestLibraryRowSavesNamedContentToAPreset covers the issue 700 save button:
// a Library row stores itself into a slot by naming its content, so nothing
// has to start playing first. It asserts the request body the row sends (a
// folder with no type, a track with one), the inline save feedback, and that
// a row already occupying a slot says so.
func TestLibraryRowSavesNamedContentToAPreset(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { Library } from '/app/static/js/components/Library.js';
const status = {
  revision: 1,
  presets: { Preset: [{ ID: 3, ContentItem: { Source: 'STORED_MUSIC', Location: '5:audio5:part13:3171:5 TRACK' } }] },
};
function Fixture() {
  const [devices] = useState({ speaker: { info: { device_id: 'DEVICE1', name: 'Speaker' }, status } });
  return h('section', { id: 'library' }, h(Library, { devices, onPlaybackRequest: () => true }));
}
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	var stored []string
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/library/servers", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: []any{
				map[string]any{"udn": "uuid:library", "name": "Media server", "ready": true, "account": "uuid:library/0"},
			}})
		})
		r.Get("/api/control/devices/speaker/library/browse", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"entries": []any{
					map[string]any{"name": "Some Album", "location": "1$7$0", "type": "dir", "isDir": true, "isPresetable": true},
					map[string]any{"name": "Great Song", "location": "5:audio5:part13:3171:5 TRACK", "type": "track", "playable": true, "isPresetable": true},
				},
			}})
		})
		r.Post("/api/control/devices/speaker/preset/{slot}", func(w http.ResponseWriter, req *http.Request) {
			body, _ := io.ReadAll(req.Body)
			mu.Lock()
			stored = append(stored, chi.URLParam(req, "slot")+" "+string(body))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var trackStarTitle string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`#library .tunein-item`, chromedp.ByQuery),
		// The first list is the registered servers; open it to get the rows.
		chromedp.Click(`#library .tunein-item`, chromedp.ByQuery),
		chromedp.WaitVisible(`#library .library-preset-btn`, chromedp.ByQuery),

		// Row 1 is the folder: save it to slot 4 and wait for the ✓.
		chromedp.Click(`#library .tunein-item:nth-child(1) .library-preset-btn`, chromedp.ByQuery),
		chromedp.Click(`#library .tunein-item:nth-child(1) .preset-picker-slot:nth-child(4)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#library .tunein-item:nth-child(1) .preset-picker-slot.saved')?.textContent === '✓'`, nil),

		// Row 2 is a single track, and it already occupies slot 3, so its star
		// is marked and says which slot without the popover being opened.
		chromedp.Evaluate(`document.querySelector('#library .tunein-item:nth-child(2) .library-preset-btn').title`, &trackStarTitle),
		chromedp.Click(`#library .tunein-item:nth-child(2) .library-preset-btn`, chromedp.ByQuery),
		chromedp.Click(`#library .tunein-item:nth-child(2) .preset-picker-slot:nth-child(1)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#library .tunein-item:nth-child(2) .preset-picker-slot.saved') !== null`, nil),
	); err != nil {
		t.Fatalf("save library rows as presets: %v", err)
	}

	var mappedClass bool
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector('#library .tunein-item:nth-child(2) .library-preset-btn').classList.contains('mapped')`, &mappedClass),
	); err != nil {
		t.Fatalf("read mapped star: %v", err)
	}
	if !mappedClass {
		t.Error("a row already saved in a slot should have a mapped star")
	}
	if !strings.Contains(trackStarTitle, "preset 3") {
		t.Errorf("mapped star title = %q, want it to name preset 3", trackStarTitle)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(stored) != 2 {
		t.Fatalf("store requests = %d (%v), want 2", len(stored), stored)
	}
	// The folder: type is sent empty, which is how the speaker stores a
	// container itself (issue 700 hardware run).
	for _, want := range []string{`4 `, `"source":"STORED_MUSIC"`, `"sourceAccount":"uuid:library/0"`, `"location":"1$7$0"`, `"type":""`, `"itemName":"Some Album"`} {
		if !strings.Contains(stored[0], want) {
			t.Errorf("folder store request should contain %q, got %s", want, stored[0])
		}
	}
	for _, want := range []string{`1 `, `"location":"5:audio5:part13:3171:5 TRACK"`, `"type":"track"`} {
		if !strings.Contains(stored[1], want) {
			t.Errorf("track store request should contain %q, got %s", want, stored[1])
		}
	}
}

// TestNowPlayingStarSavesThroughTheSharedPicker renders the now-playing card
// in Chrome and saves the current content to a slot. The card's star was
// extracted into the shared PresetPicker for issue 700 and nothing exercised
// it in a browser, so two module-level mistakes (a missing import, and the
// card rendering the shared picker directly with the wrapper's props) went
// unnoticed: both leave the star gone or inert at runtime.
func TestNowPlayingStarSavesThroughTheSharedPicker(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { NowPlaying } from '/app/static/js/components/NowPlaying.js';
const nowPlaying = {
  Source: 'STORED_MUSIC', PlayStatus: 'PLAY_STATE', Track: 'Great Song',
  ContentItem: { Source: 'STORED_MUSIC', Location: '5:audio5:part13:3171:5 TRACK', ItemName: 'Great Song' },
};
const presets = { Preset: [{ ID: 2, ContentItem: { Source: 'STORED_MUSIC', Location: '5:audio5:part13:3171:5 TRACK' } }] };
render(h(NowPlaying, { nowPlaying, deviceId: 'speaker', presets }), document.getElementById('fixture'));
`

	var mu sync.Mutex
	var slots []string
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/action/storepreset", func(w http.ResponseWriter, req *http.Request) {
			mu.Lock()
			slots = append(slots, req.URL.Query().Get("id"))
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var starTitle string
	var mapped bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`.now-playing-fav-btn`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.now-playing-fav-btn').title`, &starTitle),
		chromedp.Evaluate(`document.querySelector('.now-playing-fav-btn').classList.contains('mapped')`, &mapped),
		chromedp.Click(`.now-playing-fav-btn`, chromedp.ByQuery),
		chromedp.Click(`.preset-picker-slot:nth-child(5)`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.preset-picker-slot.saved')?.textContent === '✓'`, nil),
	); err != nil {
		t.Fatalf("save the now-playing content as a preset: %v", err)
	}

	if !mapped || !strings.Contains(starTitle, "preset 2") {
		t.Errorf("now-playing star: mapped=%v title=%q, want it marked and naming preset 2", mapped, starTitle)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(slots) != 1 || slots[0] != "5" {
		t.Errorf("storepreset calls = %v, want one for slot 5", slots)
	}
}

// TestLibraryPagesThroughALargeFolder covers issue 583: a folder with more
// entries than one page carries showed only the first page, with nothing to
// say there was more. The speaker does page correctly (measured on a
// SoundTouch 10: a 484-entry folder answers start=201 with the next slice and
// reports totalItems=484 on every page), so the truncation was ours.
//
// The fixture returns 3 entries per request regardless of the requested count,
// which is what a media server is free to do, and reports a total of 7.
func TestLibraryPagesThroughALargeFolder(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { Library } from '/app/static/js/components/Library.js';
function Fixture() {
  const [devices] = useState({ speaker: { info: { device_id: 'DEVICE1', name: 'Speaker' }, status: { revision: 1 } } });
  return h('section', { id: 'library' }, h(Library, { devices, onPlaybackRequest: () => true }));
}
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	var starts []string
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		r.Get("/api/control/devices/speaker/library/servers", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: []any{
				map[string]any{"udn": "uuid:library", "name": "Media server", "ready": true},
			}})
		})
		r.Get("/api/control/devices/speaker/library/browse", func(w http.ResponseWriter, req *http.Request) {
			start := req.URL.Query().Get("start")
			if start == "" {
				start = "1"
			}

			mu.Lock()
			starts = append(starts, start)
			mu.Unlock()

			offset, _ := strconv.Atoi(start)

			entries := []any{}
			for i := offset; i < offset+3 && i <= 7; i++ {
				// Directories, so the last step can browse into one and
				// check that a new listing starts empty.
				entries = append(entries, map[string]any{
					"name": fmt.Sprintf("Folder %02d", i), "location": fmt.Sprintf("/t/%d", i),
					"type": "dir", "isDir": true,
				})
			}

			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"entries": entries, "totalItems": 7,
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var firstCount int
	var firstLabel string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`#library .tunein-item`, chromedp.ByQuery),
		chromedp.Click(`#library .tunein-item`, chromedp.ByQuery),
		chromedp.WaitVisible(`#library .library-more`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelectorAll('#library .tunein-list .tunein-item').length`, &firstCount),
		chromedp.Text(`#library .library-more .tunein-item-desc`, &firstLabel, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("browse the first page: %v", err)
	}

	if firstCount != 3 || firstLabel != "3 of 7" {
		t.Errorf("first page: %d rows labelled %q, want 3 rows labelled \"3 of 7\"", firstCount, firstLabel)
	}

	var total int
	var moreGone bool
	if err := chromedp.Run(ctx,
		chromedp.Click(`#library .library-more button`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('#library .tunein-list .tunein-item').length === 6`, nil),
		chromedp.Click(`#library .library-more button`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('#library .tunein-list .tunein-item').length === 7`, nil),
		chromedp.Evaluate(`document.querySelectorAll('#library .tunein-list .tunein-item').length`, &total),
		// Everything is listed, so the row that offers more is gone.
		chromedp.Evaluate(`document.querySelector('#library .library-more') === null`, &moreGone),
	); err != nil {
		t.Fatalf("page through the folder: %v", err)
	}

	if total != 7 || !moreGone {
		t.Errorf("after paging: %d rows, load-more gone = %v; want 7 and true", total, moreGone)
	}

	// Browsing into a row must start a fresh listing rather than appending to
	// the folder we were in.
	var afterDescend int
	if err := chromedp.Run(ctx,
		chromedp.Click(`#library .tunein-list .tunein-item`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('#library .tunein-list .tunein-item').length === 3`, nil),
		chromedp.Evaluate(`document.querySelectorAll('#library .tunein-list .tunein-item').length`, &afterDescend),
	); err != nil {
		t.Fatalf("browse into a row after paging: %v", err)
	}

	if afterDescend != 3 {
		t.Errorf("a new listing shows %d rows, want 3 (the accumulated pages must not carry over)", afterDescend)
	}

	mu.Lock()
	defer mu.Unlock()

	if want := []string{"1", "4", "7", "1"}; len(starts) != len(want) {
		t.Fatalf("browse starts = %v, want %v", starts, want)
	} else {
		for i := range want {
			if starts[i] != want[i] {
				t.Errorf("browse start %d = %q, want %q (%v)", i, starts[i], want[i], starts)
			}
		}
	}
}

// TestLibraryRefreshReportsWhatTheSpeakerNowHas covers the issue 580 gesture.
// A registered media server sometimes vanishes from one speaker's source list
// while other speakers still see it, and the only recovery was a reboot. The
// Refresh button asks the speaker to re-read its accounts and then shows what
// it has, and says which of the three things happened, because a refresh that
// changes nothing otherwise looks identical to one that never ran.
func TestLibraryRefreshReportsWhatTheSpeakerNowHas(t *testing.T) {
	const fixture = `
import { h, render } from 'preact';
import { useState } from 'preact/hooks';
import { Library } from '/app/static/js/components/Library.js';
function Fixture() {
  const [devices] = useState({ speaker: { info: { device_id: 'DEVICE1', name: 'Speaker' }, status: { revision: 1 } } });
  return h('section', { id: 'library' }, h(Library, { devices, onPlaybackRequest: () => true }));
}
render(h(Fixture), document.getElementById('fixture'));
`

	var mu sync.Mutex
	refreshes := 0
	server := newPlayerFixtureServer(t, fixture, func(r chi.Router) {
		// The speaker starts out having lost its media server, which is the
		// state the issue describes.
		r.Get("/api/control/devices/speaker/library/servers", func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: []any{}})
		})
		r.Post("/api/control/devices/speaker/library/servers/refresh", func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			refreshes++
			found := refreshes > 1
			mu.Unlock()

			servers := []any{}
			if found {
				servers = append(servers, map[string]any{
					"udn": "uuid:library", "name": "Media server", "ready": true, "registered": true,
				})
			}

			_ = json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]any{
				"servers": servers, "refreshed": true,
			}})
		})
	})

	ctx := newHeadlessChromeContext(t)
	var emptyNote string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/fixture"),
		chromedp.WaitVisible(`#library .tunein-toolbar`, chromedp.ByQuery),
		// First refresh: still nothing, and the UI says where to go next.
		chromedp.Evaluate(`[...document.querySelectorAll('#library .tunein-toolbar button')].find(b => b.textContent.trim() === 'Refresh').click()`, nil),
		chromedp.WaitVisible(`#library .library-refresh-note`, chromedp.ByQuery),
		chromedp.Text(`#library .library-refresh-note`, &emptyNote, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("refresh with nothing registered: %v", err)
	}

	if !strings.Contains(emptyNote, "no media server") {
		t.Errorf("note after an empty refresh = %q, want it to say the speaker reports no media server", emptyNote)
	}

	var foundNote string
	var serverRows int
	if err := chromedp.Run(ctx,
		// Second refresh: the speaker has it again, so it is listed.
		chromedp.Evaluate(`[...document.querySelectorAll('#library .tunein-toolbar button')].find(b => b.textContent.trim() === 'Refresh').click()`, nil),
		chromedp.Poll(`document.querySelectorAll('#library .tunein-item').length === 1`, nil),
		chromedp.Evaluate(`document.querySelectorAll('#library .tunein-item').length`, &serverRows),
		chromedp.Text(`#library .library-refresh-note`, &foundNote, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("refresh that finds the server: %v", err)
	}

	if serverRows != 1 || !strings.Contains(foundNote, "Found 1 server") {
		t.Errorf("after the second refresh: %d rows, note %q; want 1 row and a found note", serverRows, foundNote)
	}

	mu.Lock()
	defer mu.Unlock()

	if refreshes != 2 {
		t.Errorf("refresh calls = %d, want 2", refreshes)
	}
}
