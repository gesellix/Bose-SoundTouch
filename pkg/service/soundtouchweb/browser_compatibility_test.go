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
	"io/fs"
	"net/http"
	"net/http/httptest"
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
				r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
		r.Get("/api/control/devices/speaker", func(w http.ResponseWriter, _ *http.Request) {
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
