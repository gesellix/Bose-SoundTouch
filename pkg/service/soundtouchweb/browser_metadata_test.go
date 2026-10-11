//go:build browsertest

package soundtouchweb

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/go-chi/chi/v5"
)

func newMetadataFixtureServer(t *testing.T, moduleScript string) *httptest.Server {
	t.Helper()

	r := chi.NewRouter()
	staticFS, err := fs.Sub(StaticFS, "static")
	if err != nil {
		t.Fatalf("open static fixture filesystem: %v", err)
	}
	r.Get("/app/static/*", http.StripPrefix("/app/static", http.FileServer(http.FS(staticFS))).ServeHTTP)
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

func TestNowPlayingLongTextDoesNotOverflow(t *testing.T) {
	const overflowFixture = `
import { h, render } from 'preact';
import { NowPlaying } from '/app/static/js/components/NowPlaying.js';
import { DeviceList } from '/app/static/js/components/DeviceList.js';
const track = 'RAOP-' + 'VeryLongUnbrokenTrackValue'.repeat(30);
const artist = 'Artist-' + 'UnbrokenArtistValue'.repeat(30);
const album = 'Album-' + 'UnbrokenAlbumValue'.repeat(30);
const deviceName = 'Speaker-' + 'UnbrokenDeviceName'.repeat(30);
const nowPlaying = { Source: 'AIRPLAY', SourceAccount: '', Track: track, Artist: artist, Album: album, PlayStatus: 'PLAY_STATE' };
const ordinaryTrack = 'A perfectly ordinary AirPlay title with forty characters';
const ordinaryArtist = 'An ordinary AirPlay artist';
const ordinaryAlbum = 'An ordinary AirPlay album title';
const ordinaryNowPlaying = { Source: 'AIRPLAY', Track: ordinaryTrack, Artist: ordinaryArtist, Album: ordinaryAlbum, PlayStatus: 'PLAY_STATE' };
const devices = { speaker: { info: { name: deviceName, type: 'SoundTouch' }, status: { isConnected: true, nowPlaying } } };
render(h('div', {},
  h('div', { class: 'device-detail' }, h(NowPlaying, { nowPlaying })),
  h('div', { class: 'ordinary-raop' }, h(NowPlaying, { nowPlaying: ordinaryNowPlaying })),
  h('div', { class: 'device-grid' }, h(DeviceList, { devices, onSelect() {}, onDiscover() {}, onRemove() {} }))
), document.getElementById('fixture'));
window.expectedTrack = track;
window.expectedArtist = artist;
window.expectedAlbum = album;
window.expectedDeviceName = deviceName;
window.expectedOrdinaryMetadata = [ordinaryTrack, ordinaryArtist, ordinaryAlbum];
`
	server := newMetadataFixtureServer(t, overflowFixture)
	ctx := newHeadlessChromeContext(t)

	for _, viewport := range []struct {
		name          string
		width, height int64
	}{
		{name: "desktop", width: 1200, height: 800},
		{name: "mobile", width: 320, height: 700},
	} {
		t.Run(viewport.name, func(t *testing.T) {
			var overflow []string
			var titlesComplete, detailsComplete, ordinaryDetailsComplete bool
			if err := chromedp.Do(ctx,
				chromedp.EmulateViewport(viewport.width, viewport.height),
				chromedp.Navigate(server.URL+"/fixture"),
				chromedp.WaitVisible(chromedp.CSS(`.track-title`)),
				evalInto(`[
  ...[...document.querySelectorAll('.app, .device-detail, .now-playing, .track-info, .device-grid, .device-card')]
    .filter(el => el.scrollWidth > el.clientWidth + 1)
    .map(el => el.className),
  ...(document.documentElement.scrollWidth > window.innerWidth + 1 ? ['document'] : [])
]`, &overflow),
				evalInto(`document.querySelector('.track-title').title === window.expectedTrack &&
	 document.querySelector('.track-artist').title === window.expectedArtist &&
	 document.querySelector('.track-album').title === window.expectedAlbum &&
	 document.querySelector('.device-name').title === window.expectedDeviceName &&
  document.querySelector('.now-playing-mini').title.startsWith(window.expectedTrack)`, &titlesComplete),
				chromedp.Click(chromedp.CSS(`.track-details summary`)),
				evalInto(`document.querySelector('.track-details').open &&
  document.querySelector('.track-details').textContent.includes(window.expectedTrack) &&
  document.querySelector('.track-details').textContent.includes(window.expectedArtist) &&
  document.querySelector('.track-details').textContent.includes(window.expectedAlbum)`, &detailsComplete),
				chromedp.Click(chromedp.CSS(`.ordinary-raop .track-details summary`)),
				evalInto(`document.querySelector('.ordinary-raop .track-details').open &&
  window.expectedOrdinaryMetadata.every(value => document.querySelector('.ordinary-raop .track-details').textContent.includes(value))`, &ordinaryDetailsComplete),
			); err != nil {
				t.Fatalf("measure now-playing layout: %v", err)
			}
			if len(overflow) != 0 {
				t.Errorf("overflowing elements: %v", overflow)
			}
			if !titlesComplete {
				t.Error("complete long values are not available through title attributes")
			}
			if !detailsComplete {
				t.Error("complete long metadata is not available through the touch disclosure")
			}
			if !ordinaryDetailsComplete {
				t.Error("ordinary-length RAOP metadata is not available through the touch disclosure")
			}
		})
	}
}
