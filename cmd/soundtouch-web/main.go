// Package main provides a web UI for controlling Bose SoundTouch devices.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/handlers"
	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
)

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
	setupRoutes(app, discoveryService)

	// Start web server
	log.Printf("SoundTouch Web UI starting on http://localhost:%s", *port)
	log.Fatal(http.ListenAndServe(":"+*port, nil))
}

func setupRoutes(app *handlers.WebApp, discoveryService *discovery.UnifiedDiscoveryService) {
	// Static files - try both relative paths
	staticDir := "cmd/soundtouch-web/static/"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		staticDir = "static/"
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// WebSocket endpoint
	http.HandleFunc("/ws", app.HandleWebSocket)

	// API endpoints
	http.HandleFunc("/api/devices", app.HandleAPIDevices)
	http.HandleFunc("/api/device/", app.HandleAPIDevice)
	http.HandleFunc("/api/discover", func(w http.ResponseWriter, r *http.Request) {
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

	// Device control endpoints
	http.HandleFunc("/api/control/", app.HandleAPIControl)

	// Enhanced device control endpoints with specific patterns
	http.HandleFunc("/api/device-key/", app.HandleDeviceKey)
	http.HandleFunc("/api/device-volume/", app.HandleDirectVolumeControl)
	http.HandleFunc("/api/device-power/", app.HandleDevicePower)
	http.HandleFunc("/api/device-power-status/", app.HandleDevicePowerStatus)
	http.HandleFunc("/api/device-ws/", app.HandleDeviceWebSocket)

	// SPA routes - serve index.html for specific routes only
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Serve the SPA index.html file for root path
		spaPath := staticDir + "index.html"
		http.ServeFile(w, r, spaPath)
	})

	// Additional SPA routes for client-side routing
	http.HandleFunc("/devices", func(w http.ResponseWriter, r *http.Request) {
		spaPath := staticDir + "index.html"
		http.ServeFile(w, r, spaPath)
	})

	http.HandleFunc("/device/", func(w http.ResponseWriter, r *http.Request) {
		spaPath := staticDir + "index.html"
		http.ServeFile(w, r, spaPath)
	})
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
