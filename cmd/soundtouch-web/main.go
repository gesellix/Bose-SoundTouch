// Package main provides a web UI for controlling Bose SoundTouch devices.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
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

		if info.Main.Version != "" && info.Main.Version != "(devel)" {
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

func main() {
	updateBuildInfo()

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
			&cli.StringFlag{
				Name:    "service-url",
				Usage:   "AfterTouch service base URL (e.g. https://soundtouch.local). Required for custom stream URLs to work as presets via LOCAL_INTERNET_RADIO",
				EnvVars: []string{"SERVICE_URL"},
			},
			&cli.StringFlag{
				Name:    "mgmt-username",
				Usage:   "AfterTouch management API username, used to proxy TTS to the service's /mgmt endpoints",
				Value:   "admin",
				EnvVars: []string{"MGMT_USERNAME"},
			},
			&cli.StringFlag{
				Name:    "mgmt-password",
				Usage:   "AfterTouch management API password, used to proxy TTS to the service's /mgmt endpoints",
				EnvVars: []string{"MGMT_PASSWORD"},
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
			webApp.MgmtUsername = c.String("mgmt-username")
			webApp.MgmtPassword = c.String("mgmt-password")

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
