// Package main provides a web UI for controlling Bose SoundTouch devices.
package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/handlers"
	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/go-chi/chi/v5"
)

//go:embed static
var staticFS embed.FS

var (
	port = flag.String("port", "8080", "Web server port")
	_    = flag.String("host", "", "Specific SoundTouch device host (optional)")
)

func main() {
	flag.Parse()

	// Create web app without templates (SPA mode)
	app := handlers.NewWebApp()

	// Initialize discovery service
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Printf("Failed to load config: %v, using defaults", err)

		cfg = config.DefaultConfig()
	}

	cfg.DiscoveryTimeout = 10 * time.Second
	cfg.CacheEnabled = true

	discoveryService := discovery.NewUnifiedDiscoveryService(cfg)

	// Discover devices on startup
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Broadcast discovery start
		app.BroadcastDiscoveryStatus("starting", len(app.Devices))

		discoverDevices(ctx, app, discoveryService)

		// Broadcast discovery completion and updated device list
		app.BroadcastDiscoveryStatus("completed", len(app.Devices))
		app.BroadcastDeviceList()
	}()

	// Setup HTTP routes
	r := setupRoutes(app, discoveryService)

	// Start web server
	log.Printf("SoundTouch Web UI starting on http://localhost:%s", *port)
	log.Fatal(http.ListenAndServe(":"+*port, r))
}

func setupRoutes(app *handlers.WebApp, discoveryService *discovery.UnifiedDiscoveryService) *chi.Mux {
	r := chi.NewRouter()

	// Static assets (embedded in binary)
	subFS, _ := fs.Sub(staticFS, "static")
	r.Get("/static/*", http.StripPrefix("/static", http.FileServer(http.FS(subFS))).ServeHTTP)

	// Serve index.html for SPA routes
	serveIndex := func(w http.ResponseWriter, _ *http.Request) {
		data, _ := staticFS.ReadFile("static/index.html")

		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(data)
	}

	// WebSocket endpoint
	r.Get("/ws", app.HandleWebSocket)

	// API endpoints
	r.Get("/api/devices", app.HandleAPIDevices)
	r.Get("/api/device/{id}", app.HandleAPIDevice)
	r.Post("/api/discover", func(w http.ResponseWriter, r *http.Request) {
		app.HandleAPIDiscover(w, r)
		// Trigger discovery
		//nolint:contextcheck // Context is created within goroutine
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Broadcast discovery start
			app.BroadcastDiscoveryStatus("starting", len(app.Devices))

			discoverDevices(ctx, app, discoveryService)

			// Broadcast discovery completion and updated device list
			app.BroadcastDiscoveryStatus("completed", len(app.Devices))
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
	r.Get("/api/device-ws/{id}", app.HandleDeviceWebSocket)

	// SPA routes - serve index.html for client-side routing
	r.Get("/", serveIndex)
	r.Get("/devices", serveIndex)
	r.Get("/device/*", serveIndex)

	return r
}

func discoverDevices(ctx context.Context, app *handlers.WebApp, discoveryService *discovery.UnifiedDiscoveryService) {
	log.Println("Starting device discovery...")

	devices, err := discoveryService.DiscoverDevices(ctx)
	if err != nil {
		log.Printf("Discovery failed: %v", err)
		app.BroadcastDiscoveryStatus("failed", len(app.Devices))

		return
	}

	log.Printf("Found %d devices", len(devices))

	for _, device := range devices {
		deviceID := device.Host // Use host as unique ID for now

		// Skip if we already have this device
		if _, exists := app.Devices[deviceID]; exists {
			app.Devices[deviceID].LastSeen = time.Now()
			continue
		}

		// Create new device connection
		clientConfig := &client.Config{
			Host:    device.Host,
			Port:    device.Port,
			Timeout: 10 * time.Second,
		}

		soundTouchClient := client.NewClient(clientConfig)

		// Get device info
		deviceInfo, err := soundTouchClient.GetDeviceInfo()
		if err != nil {
			log.Printf("Failed to get device info for %s: %v", device.Host, err)
			continue
		}

		// Create device connection
		conn := &webtypes.DeviceConnection{
			Client:     soundTouchClient,
			DeviceInfo: deviceInfo,
			LastSeen:   time.Now(),
			Status: webtypes.DeviceStatus{
				IsConnected:  false,
				LastActivity: time.Now(),
			},
		}

		// Initial status fetch asynchronously to avoid blocking discovery
		go app.UpdateDeviceStatus(deviceID, conn)

		app.Devices[deviceID] = conn

		log.Printf("Added device: %s (%s) at %s", deviceInfo.Name, deviceInfo.Type, device.Host)
	}
}
