package soundtouchweb

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/go-chi/chi/v5"
)

// Mount registers all routes (static, WebSocket, REST) on r. The
// discovery service is reused by the POST /api/discover handler to
// trigger an on-demand sweep — pass the same instance you used for
// startup discovery so settings (interface, timeout) stay consistent.
func (app *WebApp) Mount(r chi.Router, discoveryService *discovery.UnifiedDiscoveryService) {
	// Static assets (embedded in binary)
	subFS, _ := fs.Sub(StaticFS, "static")
	r.Get("/static/*", http.StripPrefix("/static", http.FileServer(http.FS(subFS))).ServeHTTP)

	// WebSocket endpoint
	r.Get("/ws", app.HandleWebSocket)

	// API endpoints
	r.Get("/api/devices", app.HandleAPIDevices)
	r.Get("/api/device/{id}", app.HandleAPIDevice)
	r.Get("/api/version", app.HandleAPIVersion)
	r.Post("/api/discover", func(w http.ResponseWriter, r *http.Request) {
		app.HandleAPIDiscover(w, r)

		// Trigger discovery
		//nolint:contextcheck // Context is created within goroutine
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			app.BroadcastDiscoveryStatus("starting", app.DeviceCount())

			app.DiscoverDevices(ctx, discoveryService)

			app.BroadcastDiscoveryStatus("completed", app.DeviceCount())
			app.BroadcastDeviceList()
		}()
	})

	// Device control endpoints (GET for most actions, POST for volume/bass)
	r.Get("/api/control/{id}/{action}", app.HandleAPIControl)
	r.Post("/api/control/{id}/{action}", app.HandleAPIControl)

	// TuneIn browse, search, and playback
	r.Get("/api/tunein/search", app.HandleTuneInSearch)
	r.Get("/api/tunein/navigate", app.HandleTuneInNavigate)
	r.Get("/api/tunein/navigate/*", app.HandleTuneInNavigate)
	r.Post("/api/tunein/play/{id}", app.HandlePlayTuneIn)

	// Enhanced device control endpoints
	r.Post("/api/device-key/{id}/{key}", app.HandleDeviceKey)
	r.Post("/api/device-volume/{id}/{volume}", app.HandleDirectVolumeControl)
	r.Post("/api/device-power/{id}", app.HandleDevicePower)
	r.Get("/api/device-power-status/{id}", app.HandleDevicePowerStatus)
	r.Get("/api/device-recents/{id}", app.HandleDeviceRecents)
	r.Post("/api/device-play/{id}", app.HandleDevicePlay)
	r.Get("/api/zone/{id}", app.HandleGetZone)
	r.Post("/api/zone/{id}/add/{slaveId}", app.HandleZoneAdd)
	r.Post("/api/zone/{id}/remove/{slaveId}", app.HandleZoneRemove)
	r.Post("/api/zone/{id}/dissolve", app.HandleZoneDissolve)
	r.Post("/api/zone/{id}/leave", app.HandleZoneLeave)
	r.Get("/api/device-ws/{id}", app.HandleDeviceWebSocket)

	// RadioBrowser search
	r.Get("/api/radiobrowser/search", app.HandleRadioBrowserSearch)
	r.Post("/api/radiobrowser/play/{id}", app.HandlePlayRadioBrowser)

	// SPA routes — serve index.html for client-side routing
	r.Get("/", app.serveIndex)
	r.Get("/devices", app.serveIndex)
	r.Get("/device/*", app.serveIndex)
	r.Get("/tunein", app.serveIndex)
	r.Get("/radiobrowser", app.serveIndex)
}

func (app *WebApp) serveIndex(w http.ResponseWriter, _ *http.Request) {
	data, _ := StaticFS.ReadFile("static/index.html")

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write(data)
}
