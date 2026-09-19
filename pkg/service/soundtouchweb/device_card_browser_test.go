//go:build browsertest

package soundtouchweb

import (
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

func TestDeviceCardMarksNowPlayingOfOfflineSpeakerAsLastKnown(t *testing.T) {
	const (
		onlineID  = "192.0.2.20"
		offlineID = "192.0.2.21"
	)

	app := NewWebApp()
	for _, entry := range []DeviceEntry{
		projectionDevice(onlineID, "online-device", "Online Room", true, nil),
		projectionDevice(offlineID, "offline-device", "Offline Room", false, nil),
	} {
		entry.Device.UpdateStatus(func(status *webtypes.DeviceStatus) {
			status.NowPlaying = &models.NowPlaying{
				Source:     "STORED_MUSIC",
				Track:      "Frozen Track",
				Artist:     "Cached Artist",
				PlayStatus: "PLAY_STATE",
			}
		})
		app.AddDevice(entry.ID, entry.Device)
	}

	router := chi.NewRouter()
	app.MountWeb(router, nil)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	ctx := newHeadlessChromeContext(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/app"),
		chromedp.WaitVisible(".device-card", chromedp.ByQuery),
		chromedp.Poll(`(() => {
			const card = name => Array.from(document.querySelectorAll('.device-card')).find(c =>
				c.querySelector('.device-name')?.textContent.trim() === name);
			const online = card('Online Room')?.querySelector('.now-playing-mini');
			const offline = card('Offline Room')?.querySelector('.now-playing-mini');
			return online && offline &&
				!online.classList.contains('unconfirmed') &&
				online.querySelector('.play-status').textContent.trim() === '▶' &&
				offline.classList.contains('unconfirmed') &&
				offline.querySelector('.play-status').textContent.trim() === 'Last known:' &&
				offline.title.includes('went offline') &&
				offline.textContent.includes('Frozen Track');
		})()`, nil),
	); err != nil {
		t.Fatalf("offline speaker's now playing was presented as live: %v", err)
	}
}
