// Package main provides soundtouch-player, the LAN-resident web player for
// controlling Bose SoundTouch devices. It reaches speakers directly on the
// local network and optionally delegates cloud-only features (e.g. TTS) to a
// remote AfterTouch service via --service-url, which is why it stays useful
// when soundtouch-service runs off-LAN (e.g. in the cloud).
//
// It was previously named soundtouch-web; that name is no longer published.
// If you still run the binary under the old name, it prints a rename notice
// and otherwise behaves identically.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb"
	"github.com/go-chi/chi/v5"
	"github.com/urfave/cli/v2"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
	repoURL = "https://github.com/gesellix/bose-soundtouch"
)

func updateBuildInfo() {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Path != "" {
			repoURL = "https://" + info.Main.Path
		}

		// Only fall back to build info when the version was not injected via
		// -ldflags (i.e. still the "dev" default, e.g. `go install …@vX.Y.Z`).
		// This keeps an explicitly stamped release version from being clobbered
		// by a VCS pseudo-version (e.g. v0.0.0-… from a shallow checkout).
		if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}

		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				commit = setting.Value
			case "vcs.time":
				if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
					date = t.Format("2006-01-02 15:04:05")
				}
			}
		}
	}
}

// warnIfInvokedAsWeb prints a one-line deprecation notice when the binary is
// run under its old name (soundtouch-web). That name is no longer published,
// but anyone who renamed the binary still gets nudged to soundtouch-player.
func warnIfInvokedAsWeb() {
	if len(os.Args) == 0 {
		return
	}

	name := filepath.Base(os.Args[0])
	if name == "soundtouch-web" || name == "soundtouch-web.exe" {
		log.Println("notice: 'soundtouch-web' has been renamed to 'soundtouch-player'. " +
			"The 'soundtouch-web' name is no longer published; please switch to 'soundtouch-player'.")
	}
}

func main() {
	updateBuildInfo()
	warnIfInvokedAsWeb()

	app := &cli.App{
		Name:  "soundtouch-player",
		Usage: "LAN web player for controlling Bose SoundTouch devices",
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
			&cli.StringFlag{
				Name:    "service-url",
				Usage:   "AfterTouch service base URL (e.g. https://soundtouch.local). Required for custom stream URLs to work as presets via LOCAL_INTERNET_RADIO",
				EnvVars: []string{"SERVICE_URL"},
			},
			&cli.StringFlag{
				Name:    "service-ca",
				Usage:   "Path to the AfterTouch service CA certificate (PEM) to trust for server-side calls such as TTS. Typically the service's <dataDir>/certs/ca.crt. Appended to the system trust store",
				EnvVars: []string{"SERVICE_CA"},
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
				log.Printf("Resolved --bind %q to %s", sanitizeLog(rawBind), sanitizeLog(bindAddr))
			}

			rawIface := c.String("interface")
			manualHosts := c.StringSlice("devices")

			ifaceName := defaultDiscoveryInterface(rawIface, rawBind, bindAddr)
			if rawIface == "" && ifaceName != "" {
				log.Printf("Defaulting --interface to %q from --bind", sanitizeLog(ifaceName))
			}

			addr := ":" + port
			if bindAddr != "" {
				addr = bindAddr + ":" + port
			}

			// Create web app without templates (SPA mode)
			webApp := soundtouchweb.NewWebApp()
			webApp.Version = version
			webApp.Commit = commit
			webApp.Date = date
			webApp.RepoURL = repoURL
			webApp.ServiceURL = strings.TrimRight(c.String("service-url"), "/")

			if caPath := c.String("service-ca"); caPath != "" {
				client, err := soundtouchweb.NewServiceHTTPClient(caPath)
				if err != nil {
					log.Fatalf("--service-ca: %v", err)
				}

				webApp.ServiceClient = client

				log.Printf("Trusting AfterTouch service CA from %s", sanitizeLog(caPath))
			}

			discoveryService := soundtouchweb.NewDiscoveryService(ifaceName)

			// Discover devices on startup
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				webApp.BroadcastDiscoveryStatus("starting", webApp.DeviceCount())

				for _, host := range manualHosts {
					webApp.AddDeviceByHost(host, 8090, "manual")
				}

				webApp.DiscoverDevices(ctx, discoveryService)

				webApp.BroadcastDiscoveryStatus("completed", webApp.DeviceCount())
				webApp.BroadcastDeviceList()
			}()

			r := chi.NewRouter()
			webApp.Mount(r, discoveryService)

			log.Printf("AfterTouch Web UI starting on http://%s", sanitizeLog(addr))

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
