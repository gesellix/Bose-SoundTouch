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

	// Health / liveness
	r.Get("/health", app.HandleHealth)

	// Player / control API. Per #451 this is the post-merge canonical shape:
	// device-scoped actions nest under devices/{id}/, so every direct child of
	// /api/control is a literal namespace (devices, tunein, radiobrowser,
	// version, discover) — no static-vs-param sibling, so routing never depends
	// on chi's static-over-param precedence.
	r.Route("/api/control", func(r chi.Router) {
		r.Get("/version", app.HandleAPIVersion)

		r.Post("/discover", func(w http.ResponseWriter, r *http.Request) {
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

		// One /devices subrouter holds both the list and the /{id} subtree (the
		// issue #285 single-subrouter lesson). Under /{id} every child is a
		// literal action.
		r.Route("/devices", func(r chi.Router) {
			r.Get("/", app.HandleAPIDevices)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", app.HandleAPIDevice)
				r.Post("/key/{key}", app.HandleDeviceKey)
				r.Post("/volume/{volume}", app.HandleDirectVolumeControl)
				r.Post("/power", app.HandleDevicePower)
				r.Get("/power-status", app.HandleDevicePowerStatus)
				r.Get("/recents", app.HandleDeviceRecents)
				r.Post("/play", app.HandleDevicePlay)
				r.Post("/play-url", app.HandlePlayURL)
				// Proxied to the AfterTouch service's /api/setup/tts/speak.
				r.Post("/speak", app.HandleAPISpeakText)
				// Generic key / preset / source / bass actions.
				r.Get("/action/{action}", app.HandleAPIControl)
				r.Post("/action/{action}", app.HandleAPIControl)
				r.Get("/ws", app.HandleDeviceWebSocket)

				r.Route("/zone", func(r chi.Router) {
					r.Get("/", app.HandleGetZone)
					r.Post("/add/{slaveId}", app.HandleZoneAdd)
					r.Post("/remove/{slaveId}", app.HandleZoneRemove)
					r.Post("/dissolve", app.HandleZoneDissolve)
					r.Post("/leave", app.HandleZoneLeave)
				})

				r.Post("/tunein/play", app.HandlePlayTuneIn)
				r.Post("/radiobrowser/play", app.HandlePlayRadioBrowser)
			})
		})

		// Browse / search (global, not device-scoped).
		r.Route("/tunein", func(r chi.Router) {
			r.Get("/search", app.HandleTuneInSearch)
			r.Get("/search/next", app.HandleTuneInSearchNext)
			r.Get("/navigate", app.HandleTuneInNavigate)
			r.Get("/navigate/*", app.HandleTuneInNavigate)
		})

		r.Route("/radiobrowser", func(r chi.Router) {
			r.Get("/search", app.HandleRadioBrowserSearch)
		})
	})

	// SPA — served under /app/*. The client navigates via component state
	// rather than the URL, so these entries only ensure deep links and
	// refreshes return index.html instead of 404. Per #451 this keeps the
	// whole web UI under one /app subtree, so folding -web into -service is an
	// additive mount.
	r.Get("/app", app.serveIndex)
	r.Get("/app/devices", app.serveIndex)
	r.Get("/app/device/*", app.serveIndex)
	r.Get("/app/tunein", app.serveIndex)
	r.Get("/app/radiobrowser", app.serveIndex)
	r.Get("/app/playurl", app.serveIndex)
	r.Get("/app/tts", app.serveIndex)

	// Standalone convenience: the bare root jumps into the app. When -web is
	// folded into -service, / instead serves a landing page (admin vs app) and
	// this redirect is replaced.
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app", http.StatusFound)
	})
}

func (app *WebApp) serveIndex(w http.ResponseWriter, _ *http.Request) {
	data, _ := StaticFS.ReadFile("static/index.html")

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write(data)
}
