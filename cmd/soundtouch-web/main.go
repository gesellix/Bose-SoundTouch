// Package main provides a web UI for controlling Bose SoundTouch devices.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/handlers"
	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/go-chi/chi/v5"
	"github.com/urfave/cli/v2"
)

//go:embed static
var staticFS embed.FS

func main() {
	app := &cli.App{
		Name:  "soundtouch-web",
		Usage: "Web UI for controlling Bose SoundTouch devices",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "HTTP port to listen on",
				Value:   "8080",
				EnvVars: []string{"PORT"},
			},
			&cli.StringFlag{
				Name:    "bind",
				Usage:   "Address for the HTTP listener: host, IP, or local interface name (e.g. eth0). Leave empty to listen on all interfaces",
				EnvVars: []string{"BIND_ADDR"},
			},
			&cli.StringFlag{
				Name:    "interface",
				Usage:   "Network interface name (e.g. eth0) for mDNS and UPnP device discovery. Defaults to the --bind interface name when one was given; leave empty otherwise to auto-pick",
				EnvVars: []string{"DISCOVERY_INTERFACE"},
			},
			&cli.StringSliceFlag{
				Name:    "devices",
				Usage:   "SoundTouch device IP address(es) to add manually (can be specified multiple times)",
				EnvVars: []string{"SOUNDTOUCH_DEVICES"},
			},
		},
		Action: func(c *cli.Context) error {
			port := c.String("port")
			rawBind := c.String("bind")

			bindAddr, err := resolveBindAddr(rawBind)
			if err != nil {
				log.Fatal(err)
			}

			if rawBind != "" && bindAddr != rawBind {
				log.Printf("Resolved --bind %q to %s", rawBind, bindAddr)
			}

			rawIface := c.String("interface")
			manualHosts := c.StringSlice("devices")

			ifaceName := defaultDiscoveryInterface(rawIface, rawBind, bindAddr)
			if rawIface == "" && ifaceName != "" {
				log.Printf("Defaulting --interface to %q from --bind", ifaceName)
			}

			addr := ":" + port
			if bindAddr != "" {
				addr = bindAddr + ":" + port
			}

			// Create web app without templates (SPA mode)
			webApp := handlers.NewWebApp()

			// Initialize discovery service
			cfg, err := config.LoadFromEnv()
			if err != nil {
				log.Printf("Failed to load config: %v, using defaults", err)

				cfg = config.DefaultConfig()
			}

			cfg.DiscoveryTimeout = 10 * time.Second
			cfg.CacheEnabled = true

			if ifaceName != "" {
				cfg.DiscoveryInterface = ifaceName
			}

			discoveryService := discovery.NewUnifiedDiscoveryService(cfg)

			// Discover devices on startup
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				webApp.BroadcastDiscoveryStatus("starting", webApp.DeviceCount())

				for _, host := range manualHosts {
					addDevice(webApp, host, 8090, "manual")
				}

				discoverDevices(ctx, webApp, discoveryService)

				webApp.BroadcastDiscoveryStatus("completed", webApp.DeviceCount())
				webApp.BroadcastDeviceList()
			}()

			r := setupRoutes(webApp, discoveryService)

			log.Printf("SoundTouch Web UI starting on http://%s", addr)

			return http.ListenAndServe(addr, r)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

// defaultDiscoveryInterface picks the interface name to use for mDNS/UPnP
// discovery. An explicit --interface always wins; otherwise, when --bind was
// given an interface name (i.e. resolveBindAddr substituted an IP for it),
// that name is reused so the common single-interface case "just works".
// Returns the empty string when there is nothing to propagate, leaving the
// discovery service to auto-pick.
func defaultDiscoveryInterface(rawInterface, rawBind, resolvedBind string) string {
	if rawInterface != "" {
		return rawInterface
	}

	if rawBind != "" && rawBind != resolvedBind {
		return rawBind
	}

	return ""
}

// resolveBindAddr returns the address to bind the HTTP listener to.
//
// If bindAddr names a local network interface, the interface's single IPv4
// address is returned. When no IPv4 is present, the function falls back to the
// interface's single non-link-local IPv6 address (wrapped in brackets so it
// composes correctly with ":port"). Ambiguous interfaces (multiple addresses
// in the chosen family) or interfaces with no usable address produce an error,
// so misconfiguration surfaces immediately instead of becoming an obscure DNS
// lookup failure at listen time.
//
// If bindAddr is not an interface name — including the empty string, a host
// name, or a literal IP — it is returned unchanged.
func resolveBindAddr(bindAddr string) (string, error) {
	// A lookup failure here just means bindAddr isn't an interface name
	// (it's a host, IP, or empty); fall through to pass-through.
	iface, _ := net.InterfaceByName(bindAddr)
	if iface == nil {
		return bindAddr, nil
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("--bind %q: failed to list addresses for interface: %w", bindAddr, err)
	}

	var ipv4, ipv6 []net.IP

	for _, addr := range addrs {
		var ip net.IP

		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		if ip == nil {
			continue
		}

		if v4 := ip.To4(); v4 != nil {
			ipv4 = append(ipv4, v4)
		} else if !ip.IsLinkLocalUnicast() {
			// Skip IPv6 link-local (fe80::); it requires a zone ID and
			// can't be used as a plain "[ip]:port" listen address.
			ipv6 = append(ipv6, ip)
		}
	}

	switch {
	case len(ipv4) == 1:
		return ipv4[0].String(), nil
	case len(ipv4) > 1:
		return "", fmt.Errorf("--bind %q: interface has multiple IPv4 addresses (%v); specify one directly", bindAddr, ipv4)
	case len(ipv6) == 1:
		return "[" + ipv6[0].String() + "]", nil
	case len(ipv6) > 1:
		return "", fmt.Errorf("--bind %q: interface has multiple IPv6 addresses (%v); specify one directly", bindAddr, ipv6)
	default:
		return "", fmt.Errorf("--bind %q: interface has no usable IPv4 or IPv6 address", bindAddr)
	}
}

// addDevice registers a SoundTouch device with the WebApp by fetching
// its /info and creating a DeviceConnection. The source label
// ("manual" or "discovered") appears in log lines so the operator can
// tell apart entries that came from --devices from those found via
// mDNS/UPnP. If the host is already known, the existing entry's
// LastSeen is bumped and the function returns without re-fetching.
func addDevice(app *handlers.WebApp, host string, port int, source string) {
	// Fast path: skip the network call if we already know this host.
	if app.TouchDevice(host) {
		return
	}

	c := client.NewClient(&client.Config{
		Host:    host,
		Port:    port,
		Timeout: 10 * time.Second,
	})

	info, err := c.GetDeviceInfo()
	if err != nil {
		log.Printf("Failed to fetch device info from %s (%s): %v", host, source, err)
		return
	}

	conn := &webtypes.DeviceConnection{
		Client:     c,
		DeviceInfo: info,
		LastSeen:   time.Now(),
		Status: webtypes.DeviceStatus{
			IsConnected:  false,
			LastActivity: time.Now(),
		},
	}
	if !app.AddDevice(host, conn) {
		// Lost a race — another goroutine inserted the same host
		// between TouchDevice and AddDevice. AddDevice bumped LastSeen
		// on the existing entry; discard our conn.
		return
	}

	go app.UpdateDeviceStatus(host, conn)

	log.Printf("Added %s device %s (%s) at %s:%d", source, info.Name, info.Type, host, port)
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
			app.BroadcastDiscoveryStatus("starting", app.DeviceCount())

			discoverDevices(ctx, app, discoveryService)

			// Broadcast discovery completion and updated device list
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
		app.BroadcastDiscoveryStatus("failed", app.DeviceCount())

		return
	}

	log.Printf("Found %d devices", len(devices))

	for _, device := range devices {
		addDevice(app, device.Host, device.Port, "discovered")
	}
}
