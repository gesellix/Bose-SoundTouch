package soundtouchweb

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/go-chi/chi/v5"
)

// MountWeb registers the portable soundtouch-player surface on r: the embedded
// assets (/app/static/*), the control API (/api/control/*), and the SPA
// (/app/*). It is self-contained under /api/control and /app, registering
// nothing outside those subtrees (no /, no /health), so it can be mounted into
// another router (e.g. soundtouch-service) additively. The discovery service
// is reused by POST /api/control/discover for on-demand sweeps; pass the same
// instance used for startup discovery so settings (interface, timeout) stay
// consistent.
func (app *WebApp) MountWeb(r chi.Router, discoveryService *discovery.UnifiedDiscoveryService) {
	// Embedded assets, served under the /app subtree so nothing contends with a
	// host router's own /static (e.g. the Stockholm bridge's root catch-all).
	subFS, _ := fs.Sub(StaticFS, "static")
	r.Get("/app/static/*", http.StripPrefix("/app/static", http.FileServer(http.FS(subFS))).ServeHTTP)

	// Player / control API. Per #451 this is the post-merge canonical shape:
	// device-scoped actions nest under devices/{id}/, so every direct child of
	// /api/control is a literal namespace (version, ws, discover, devices,
	// providers) — no static-vs-param sibling, so routing never depends on
	// chi's static-over-param precedence.
	r.Route("/api/control", func(r chi.Router) {
		r.Get("/version", app.HandleAPIVersion)

		// App-wide event stream: device list, discovery status, per-device
		// status updates. The read/event half of the control surface (the
		// per-device socket lives at devices/{id}/ws).
		r.Get("/ws", app.HandleWebSocket)

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
				r.Delete("/", app.HandleDeleteDevice)
				r.Post("/key/{key}", app.HandleDeviceKey)
				r.Post("/volume/{volume}", app.HandleDirectVolumeControl)
				r.Post("/power", app.HandleDevicePower)
				r.Get("/power-status", app.HandleDevicePowerStatus)
				r.Get("/recents", app.HandleDeviceRecents)
				// Low-level "play this ContentItem" primitive (not a provider).
				r.Post("/play", app.HandleDevicePlay)
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

				// Play a result from a content provider on this device.
				// Browsable providers (tunein, radiobrowser) take a catalog item;
				// input providers (url, tts) take the raw input.
				r.Route("/providers", func(r chi.Router) {
					r.Post("/tunein/play", app.HandlePlayTuneIn)
					r.Post("/radiobrowser/play", app.HandlePlayRadioBrowser)
					r.Post("/url/play", app.HandlePlayURL)
					// Proxied to the AfterTouch service's /api/setup/tts/speak.
					r.Post("/tts/play", app.HandleAPISpeakText)
				})
			})
		})

		// Provider browse / search (global, not device-scoped). Only browsable
		// providers (a catalog you search/navigate) appear here; input
		// providers (url, tts) exist solely as a device play above.
		r.Route("/providers", func(r chi.Router) {
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
}

// Mount is the standalone soundtouch-player entry point: the portable web surface
// (see MountWeb) plus the standalone-only liveness endpoint and a / redirect
// into the app. soundtouch-service does not call this — it mounts MountWeb and
// keeps its own / (landing page) and /health.
func (app *WebApp) Mount(r chi.Router, discoveryService *discovery.UnifiedDiscoveryService) {
	app.MountWeb(r, discoveryService)

	// Health / liveness (standalone only).
	r.Get("/health", app.HandleHealth)

	// Standalone convenience: the bare root jumps into the app.
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app", http.StatusFound)
	})
}

func (app *WebApp) serveIndex(w http.ResponseWriter, _ *http.Request) {
	data, _ := StaticFS.ReadFile("static/index.html")

	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write(data)
}
