// Package main provides the SoundTouch service daemon that acts as a proxy and management
// interface for Bose SoundTouch devices, providing Marge service emulation and device discovery.
package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	bindAddr := os.Getenv("BIND_ADDR")
	// If BIND_ADDR is explicitly set, use it. Otherwise, bind to all interfaces (IPv4 and IPv6).
	addr := bindAddr + ":" + port
	if bindAddr == "" {
		addr = ":" + port
	}

	targetURL := os.Getenv("PYTHON_BACKEND_URL")
	if targetURL == "" {
		targetURL = "http://localhost:8001"
	}

	target, err := url.Parse(targetURL)
	if err != nil {
		log.Fatalf("Failed to parse target URL: %v", err)
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	ds := datastore.NewDataStore(dataDir)
	if err := ds.Initialize(); err != nil {
		log.Printf("Warning: Failed to initialize datastore: %v", err)
	}

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		// Try to guess the server URL
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "localhost"
		}

		serverURL = "http://" + strings.ToLower(hostname) + ":" + port
	}

	sm := setup.NewManager(serverURL, ds)

	redact := os.Getenv("REDACT_PROXY_LOGS") != "false"
	logBody := os.Getenv("LOG_PROXY_BODY") == "true"

	server := handlers.NewServer(ds, sm, serverURL, redact, logBody)

	pyProxy := httputil.NewSingleHostReverseProxy(target)
	pyProxy.ModifyResponse = func(res *http.Response) error {
		// Generic Header Preservation:
		// Go's net/http canonicalizes headers (e.g., ETag becomes Etag).
		// We ensure ETag specifically uses uppercase 'T' as some Bose devices are case-sensitive.
		if etags, ok := res.Header["Etag"]; ok {
			delete(res.Header, "Etag")
			res.Header["ETag"] = etags
		}
		// Also restore other potentially sensitive headers if needed, but for now we focus on ETag
		// as it's the most common culprit.

		currentLp := proxy.NewLoggingProxy(target.String(), redact)
		currentLp.LogBody = logBody
		currentLp.LogResponse(res)

		return nil
	}
	originalPyDirector := pyProxy.Director
	pyProxy.Director = func(req *http.Request) {
		originalPyDirector(req)

		currentLp := proxy.NewLoggingProxy(target.String(), redact)
		currentLp.LogBody = logBody
		currentLp.LogRequest(req)
	}

	// Phase 5: Device Discovery
	go func() {
		for {
			server.DiscoverDevices(context.Background())
			time.Sleep(5 * time.Minute)
		}
	}()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Phase 2: Root endpoint implemented in Go
	r.Get("/", server.HandleRoot)
	r.Get("/health", server.HandleHealth)
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/media/favicon-braille.svg"
		server.HandleMedia()(w, r)
	})

	// Phase 2: Static file serving for /media
	r.Get("/media/*", server.HandleMedia())

	// Phase 3: BMX endpoints
	r.Route("/bmx", func(r chi.Router) {
		r.Get("/registry/v1/services", server.HandleBMXRegistry)
		r.Get("/tunein/v1/playback/station/{stationID}", server.HandleTuneInPlayback)
		r.Get("/tunein/v1/playback/episodes/{podcastID}", server.HandleTuneInPodcastInfo)
		r.Get("/tunein/v1/playback/episode/{podcastID}", server.HandleTuneInPlaybackPodcast)
		r.Post("/orion/v1/playback/station/{data}", server.HandleOrionPlayback)
	})

	// Phase 4: Marge endpoints
	r.Route("/marge", func(r chi.Router) {
		r.Get("/streaming/sourceproviders", server.HandleMargeSourceProviders)
		r.Get("/accounts/{account}/full", server.HandleMargeAccountFull)
		r.Post("/streaming/support/power_on", server.HandleMargePowerOn)
		r.Get("/updates/soundtouch", server.HandleMargeSoftwareUpdate)
		r.Get("/accounts/{account}/devices/{device}/presets", server.HandleMargePresets)
		r.Post("/accounts/{account}/devices/{device}/presets/{presetNumber}", server.HandleMargeUpdatePreset)
		r.Post("/accounts/{account}/devices/{device}/recents", server.HandleMargeAddRecent)
		r.Post("/accounts/{account}/devices", server.HandleMargeAddDevice)
		r.Delete("/accounts/{account}/devices/{device}", server.HandleMargeRemoveDevice)
		r.Get("/streaming/account/{account}/provider_settings", server.HandleMargeProviderSettings)
		r.Get("/streaming/device/{device}/streaming_token", server.HandleMargeStreamingToken)
		r.Post("/streaming/support/customersupport", server.HandleMargeCustomerSupport)
	})

	// Phase 10: Stats endpoints
	r.Route("/streaming/stats", func(r chi.Router) {
		r.Post("/usage", server.HandleUsageStats)
		r.Post("/error", server.HandleErrorStats)
	})

	// Proxy route integrated into main router
	r.Get("/proxy/*", server.HandleProxyRequest)

	// Phase 7: Setup and Discovery endpoints
	r.Route("/setup", func(r chi.Router) {
		r.Get("/devices", server.HandleListDiscoveredDevices)
		r.Post("/discover", server.HandleTriggerDiscovery)
		r.Get("/discovery-status", server.HandleGetDiscoveryStatus)
		r.Get("/settings", server.HandleGetSettings)
		r.Get("/info/{deviceIP}", server.HandleGetDeviceInfo)
		r.Get("/summary/{deviceIP}", server.HandleGetMigrationSummary)
		r.Post("/migrate/{deviceIP}", server.HandleMigrateDevice)
		r.Post("/ensure-remote-services/{deviceIP}", server.HandleEnsureRemoteServices)
		r.Post("/backup/{deviceIP}", server.HandleBackupConfig)
		r.Get("/proxy-settings", server.HandleGetProxySettings)
		r.Post("/proxy-settings", server.HandleUpdateProxySettings)
		r.Get("/devices/{deviceId}/events", server.HandleGetDeviceEvents)
	})

	// Delegation Logic: Proxy everything else to Python
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		pyProxy.ServeHTTP(w, r)
	})

	log.Printf("Go service starting on %s, proxying to %s", addr, targetURL)
	log.Fatal(http.ListenAndServe(addr, r))
}
