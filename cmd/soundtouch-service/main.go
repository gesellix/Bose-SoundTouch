// Package main provides the SoundTouch service daemon that acts as a proxy and management
// interface for Bose SoundTouch devices, providing Marge service emulation and device discovery.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/urfave/cli/v2"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func updateBuildInfo() {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}

		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				commit = setting.Value
			case "vcs.time":
				if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
					date = t.Format("2006-01-02_15:04:05")
				}
			}
		}
	}
}

func main() {
	updateBuildInfo()

	app := &cli.App{
		Name:  "soundtouch-service",
		Usage: "Local service for Bose SoundTouch cloud emulation and management",
		Description: `⠎⠕⠥⠝⠙⠤⠞⠕⠥⠉⠓ A local server that emulates Bose cloud services (BMX, Marge).
   It enables offline operation, device migration, and HTTP interaction recording.`,
		Version: version,
		Authors: []*cli.Author{
			{
				Name: "Tobias Gesellchen, and the Bose-SoundTouch Contributors",
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Usage:   "HTTP port to bind the service to",
				Value:   "8000",
				EnvVars: []string{"PORT"},
			},
			&cli.StringFlag{
				Name:    "bind",
				Usage:   "Network interface to bind to",
				EnvVars: []string{"BIND_ADDR"},
			},
			&cli.StringFlag{
				Name:    "data-dir",
				Usage:   "Directory for persistent data",
				Value:   "data",
				EnvVars: []string{"DATA_DIR"},
			},
			&cli.StringFlag{
				Name:    "server-url",
				Aliases: []string{"s"},
				Usage:   "External URL of this service",
				EnvVars: []string{"SERVER_URL"},
			},
			&cli.StringFlag{
				Name:    "https-port",
				Usage:   "HTTPS port to bind the service to",
				Value:   "8443",
				EnvVars: []string{"HTTPS_PORT"},
			},
			&cli.StringFlag{
				Name:    "https-server-url",
				Aliases: []string{"S"},
				Usage:   "External HTTPS URL",
				EnvVars: []string{"HTTPS_SERVER_URL"},
			},
			&cli.BoolFlag{
				Name:    "redact-logs",
				Usage:   "Redact sensitive data in proxy logs",
				Value:   true,
				EnvVars: []string{"REDACT_PROXY_LOGS"},
			},
			&cli.BoolFlag{
				Name:    "log-bodies",
				Usage:   "Log full request/response bodies",
				EnvVars: []string{"LOG_PROXY_BODY"},
			},
			&cli.BoolFlag{
				Name:    "record-interactions",
				Usage:   "Record HTTP interactions to disk",
				Value:   true,
				EnvVars: []string{"RECORD_INTERACTIONS"},
			},
			&cli.StringFlag{
				Name:    "discovery-interval",
				Usage:   "Device discovery interval",
				Value:   "5m",
				EnvVars: []string{"DISCOVERY_INTERVAL"},
			},
			&cli.BoolFlag{
				Name:    "dns-discovery",
				Usage:   "Enable DNS discovery server",
				EnvVars: []string{"ENABLE_DNS_DISCOVERY"},
			},
			&cli.StringFlag{
				Name:    "dns-upstream",
				Usage:   "Upstream DNS server(s) for non-Bose queries (comma-separated). If empty, /etc/resolv.conf is used.",
				Value:   "",
				EnvVars: []string{"DNS_UPSTREAM"},
			},
			&cli.StringFlag{
				Name:    "dns-bind",
				Usage:   "Bind address for the DNS discovery server",
				Value:   ":53",
				EnvVars: []string{"DNS_BIND_ADDR"},
			},
			&cli.StringFlag{
				Name:    "spotify-client-id",
				Usage:   "Spotify OAuth client ID",
				EnvVars: []string{"SPOTIFY_CLIENT_ID"},
			},
			&cli.StringFlag{
				Name:    "spotify-client-secret",
				Usage:   "Spotify OAuth client secret",
				EnvVars: []string{"SPOTIFY_CLIENT_SECRET"},
			},
			&cli.StringFlag{
				Name:    "spotify-redirect-uri",
				Usage:   "Spotify OAuth redirect URI",
				Value:   "ueberboese-login://spotify",
				EnvVars: []string{"SPOTIFY_REDIRECT_URI"},
			},
			&cli.StringFlag{
				Name:    "mgmt-username",
				Usage:   "Management API username for HTTP Basic Auth",
				Value:   "admin",
				EnvVars: []string{"MGMT_USERNAME"},
			},
			&cli.StringFlag{
				Name:    "mgmt-password",
				Usage:   "Management API password for HTTP Basic Auth",
				Value:   "change_me!",
				EnvVars: []string{"MGMT_PASSWORD"},
			},
			&cli.StringFlag{
				Name:    "base-url",
				Usage:   "External base URL for OAuth callbacks behind reverse proxy",
				EnvVars: []string{"BASE_URL"},
			},
			&cli.BoolFlag{
				Name:    "mirror-enabled",
				Usage:   "Enable background mirroring to Bose Cloud",
				EnvVars: []string{"MIRROR_ENABLED"},
			},
			&cli.StringSliceFlag{
				Name:    "mirror-endpoints",
				Usage:   "Endpoints to mirror to Bose Cloud (comma-separated or multiple flags)",
				EnvVars: []string{"MIRROR_ENDPOINTS"},
			},
			&cli.StringSliceFlag{
				Name:    "skip-mirror-endpoints",
				Usage:   "Endpoints to skip mirroring to Bose Cloud (comma-separated or multiple flags)",
				EnvVars: []string{"SKIP_MIRROR_ENDPOINTS"},
			},
			&cli.StringSliceFlag{
				Name:    "internal-paths",
				Usage:   "Paths for internal requests (comma-separated or multiple flags)",
				EnvVars: []string{"INTERNAL_PATHS"},
			},
			&cli.BoolFlag{
				Name:    "migration-enabled",
				Usage:   "Enable device directory migration from serial to MAC-based structure",
				Value:   true,
				EnvVars: []string{"MIGRATION_ENABLED"},
			},
			&cli.BoolFlag{
				Name:    "migration-dry-run",
				Usage:   "Log what would be migrated without actually doing it",
				EnvVars: []string{"MIGRATION_DRY_RUN"},
			},
			&cli.StringFlag{
				Name:    "preferred-source",
				Usage:   "Preferred source of truth (local or upstream)",
				Value:   "local",
				EnvVars: []string{"PREFERRED_SOURCE"},
			},
		},
		Action: func(c *cli.Context) error {
			config := loadConfig(c)
			ds := initDataStore(config.dataDir)

			persisted := applyPersistedSettings(ds, &config)

			if persisted.ServerURL == "" {
				log.Printf("Creating default settings.json in %s", config.dataDir)
				persisted = createDefaultSettings(ds, config)
			}

			// Recalculate domains if settings changed
			hostname, _ := os.Hostname()
			if hostname == "" {
				hostname = "localhost"
			}

			config.domains = getDomains(config.serverURL, config.httpsServerURL, hostname)

			cm := initCertificateManager(config.dataDir)
			sm := setup.NewManager(config.serverURL, ds, cm)
			sm.MgmtUsername = config.mgmtUsername
			sm.MgmtPassword = config.mgmtPassword
			server := handlers.NewServer(ds, sm, config.serverURL, config.redact, config.logBody, config.record)
			sm.GetDNSRunning = server.GetDNSRunning
			server.SetHTTPServerURL(config.httpsServerURL)
			server.SetVersionInfo(version, commit, date)
			server.SetDiscoverySettings(config.discoveryInterval, persisted.DiscoveryEnabled)
			server.SetDNSSettings(persisted.DNSEnabled, strings.Join(persisted.DNSUpstream, ","), persisted.DNSBindAddr)
			server.SetMirrorSettings(persisted.MirrorEnabled, persisted.MirrorEndpoints, persisted.SkipMirrorEndpoints, persisted.PreferredSource)
			server.SetInternalPaths(persisted.InternalPaths)
			server.SetSpotifyConfig(config.spotifyClientID, config.spotifyClientSecret, config.spotifyRedirectURI)
			server.SetMgmtConfig(config.mgmtUsername, config.mgmtPassword)

			if config.spotifyClientID != "" {
				spotifyService := spotify.NewSpotifyService(
					config.spotifyClientID,
					config.spotifyClientSecret,
					config.spotifyRedirectURI,
					config.dataDir,
				)
				server.SetSpotifyService(spotifyService)

				clientIDPrefix := config.spotifyClientID
				if len(clientIDPrefix) > 8 {
					clientIDPrefix = clientIDPrefix[:8]
				}

				log.Printf("Spotify service initialized (client ID: %s...)", clientIDPrefix)
			}

			// Load and set initial DNS discoveries
			dnsDiscoveries, err := ds.LoadDNSDiscoveries()
			if err == nil && len(dnsDiscoveries) > 0 {
				initial := make(map[string]*discovery.DiscoveredHost)
				for _, entry := range dnsDiscoveries {
					initial[entry.Hostname] = &discovery.DiscoveredHost{
						Hostname:      entry.Hostname,
						FirstSeen:     entry.FirstSeen,
						LastSeen:      entry.LastSeen,
						QueryCount:    entry.QueryCount,
						IsBoseService: entry.IsBoseService,
						IsIntercepted: entry.IsIntercepted,
						RemoteAddr:    entry.RemoteAddr,
					}
				}

				server.SetDNSDiscoveries(initial)
			}

			server.SetShortcuts(persisted.Shortcuts)

			for path, status := range persisted.Shortcuts {
				log.Printf("Warning: configured shortcut: %s -> %d", path, status)
			}

			recorder := proxy.NewRecorder(config.dataDir)
			recorder.Redact = config.redact
			patternsPath := filepath.Join(config.dataDir, "patterns.json")

			patterns, err := proxy.LoadPatterns(patternsPath)
			if err != nil {
				log.Printf("Warning: Failed to load patterns from %s: %v", patternsPath, err)
			}

			if len(patterns) == 0 {
				log.Printf("Creating default patterns at %s", patternsPath)

				patterns = proxy.DefaultPatterns()

				patternsData, jsonErr := json.MarshalIndent(patterns, "", "  ")
				if jsonErr != nil {
					log.Printf("Warning: Failed to marshal default patterns: %v", jsonErr)
				} else {
					_ = os.WriteFile(patternsPath, patternsData, 0644)
				}
			}

			if len(patterns) > 0 {
				recorder.Patterns = patterns
			}

			server.SetRecorder(recorder)

			tlsConfig, err := cm.GetServerTLSConfig(config.domains)
			if err != nil {
				log.Printf("Warning: Failed to setup TLS: %v", err)
			}

			startDeviceDiscovery(server)

			r := setupRouter(server)

			log.Printf("Go service starting on %s", config.serverURL)

			if tlsConfig != nil {
				startHTTPSServer(config.httpsAddr, r, tlsConfig, config.httpsServerURL)
			}

			return http.ListenAndServe(config.addr, r)
		},
		Commands: []*cli.Command{
			{
				Name:    "version",
				Aliases: []string{"v"},
				Usage:   "Show detailed version information",
				Action:  showVersionInfo,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func showVersionInfo(_ *cli.Context) error {
	fmt.Printf("%s version %s\n", os.Args[0], version)
	fmt.Printf("Build commit: %s\n", commit)
	fmt.Printf("Build date: %s\n", date)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	return nil
}

type serviceConfig struct {
	port                string
	bindAddr            string
	addr                string
	dataDir             string
	serverURL           string
	httpsServerURL      string
	httpsAddr           string
	redact              bool
	logBody             bool
	record              bool
	dnsEnabled          bool
	dnsUpstream         string
	dnsBind             string
	mirrorEnabled       bool
	mirrorEndpoints     []string
	skipMirrorEndpoints []string
	internalPaths       []string
	discoveryInterval   time.Duration
	domains             []string
	spotifyClientID     string
	spotifyClientSecret string
	spotifyRedirectURI  string
	mgmtUsername        string
	mgmtPassword        string
	migrationEnabled    bool
	migrationDryRun     bool
	preferredSource     string
}

func loadConfig(c *cli.Context) serviceConfig {
	port := c.String("port")
	bindAddr := c.String("bind")

	addr := bindAddr + ":" + port
	if bindAddr == "" {
		addr = ":" + port
	}

	dataDir := c.String("data-dir")

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	hostname = strings.ToLower(hostname)

	serverURL := c.String("server-url")
	if serverURL == "" {
		serverURL = "http://" + hostname + ":" + port
	}

	httpsPort := c.String("https-port")

	httpsAddr := bindAddr + ":" + httpsPort
	if bindAddr == "" {
		httpsAddr = ":" + httpsPort
	}

	httpsServerURL := c.String("https-server-url")
	if httpsServerURL == "" {
		httpsServerURL = "https://" + hostname + ":" + httpsPort
	}

	domains := getDomains(serverURL, httpsServerURL, hostname)

	redact := c.Bool("redact-logs")
	logBody := c.Bool("log-bodies")
	record := c.Bool("record-interactions")

	dnsEnabled := c.Bool("dns-discovery")
	dnsUpstream := c.String("dns-upstream")
	dnsBind := c.String("dns-bind")

	discoveryIntervalStr := c.String("discovery-interval")

	discoveryInterval, err := time.ParseDuration(discoveryIntervalStr)
	if err != nil {
		log.Printf("Warning: Failed to parse discovery interval %s, using default 5m: %v", discoveryIntervalStr, err)

		discoveryInterval = 5 * time.Minute
	}

	spotifyClientID := c.String("spotify-client-id")
	spotifyClientSecret := c.String("spotify-client-secret")
	spotifyRedirectURI := c.String("spotify-redirect-uri")
	mgmtUsername := c.String("mgmt-username")
	mgmtPassword := c.String("mgmt-password")
	mirrorEnabled := c.Bool("mirror-enabled")
	mirrorEndpoints := c.StringSlice("mirror-endpoints")
	skipMirrorEndpoints := c.StringSlice("skip-mirror-endpoints")
	internalPaths := c.StringSlice("internal-paths")
	migrationEnabled := c.Bool("migration-enabled")
	migrationDryRun := c.Bool("migration-dry-run")
	preferredSource := c.String("preferred-source")

	return serviceConfig{
		port:                port,
		bindAddr:            bindAddr,
		addr:                addr,
		dataDir:             dataDir,
		serverURL:           serverURL,
		httpsServerURL:      httpsServerURL,
		httpsAddr:           httpsAddr,
		redact:              redact,
		logBody:             logBody,
		record:              record,
		dnsEnabled:          dnsEnabled,
		dnsUpstream:         dnsUpstream,
		dnsBind:             dnsBind,
		mirrorEnabled:       mirrorEnabled,
		mirrorEndpoints:     mirrorEndpoints,
		skipMirrorEndpoints: skipMirrorEndpoints,
		internalPaths:       internalPaths,
		discoveryInterval:   discoveryInterval,
		domains:             domains,
		spotifyClientID:     spotifyClientID,
		spotifyClientSecret: spotifyClientSecret,
		spotifyRedirectURI:  spotifyRedirectURI,
		mgmtUsername:        mgmtUsername,
		mgmtPassword:        mgmtPassword,
		migrationEnabled:    migrationEnabled,
		migrationDryRun:     migrationDryRun,
		preferredSource:     preferredSource,
	}
}

func getDomains(serverURL, httpsServerURL, hostname string) []string {
	domainsMap := map[string]bool{
		// RFC-compliant wildcards for API patterns
		"*.api.bose.io":    true,
		"*.api.bosecm.com": true,
		// Core Bose domains (keep specific ones for clarity)
		"streaming.bose.com":      true,
		"updates.bose.com":        true,
		"stats.bose.com":          true,
		"bmx.bose.com":            true,
		"worldwide.bose.com":      true,
		"music.api.bose.com":      true,
		"streamingoauth.bose.com": true,
		"bosecm.com":              true,
		"bose.io":                 true,
		"bose-prod.apigee.net":    true,
		"bose-test.apigee.net":    true,
		// Local service domains
		setup.TestDomain: true,
		hostname:         true,
		"localhost":      true,
		"127.0.0.1":      true,
	}

	if u, err := url.Parse(serverURL); err == nil && u.Hostname() != "" {
		domainsMap[strings.ToLower(u.Hostname())] = true
	}

	if u, err := url.Parse(httpsServerURL); err == nil && u.Hostname() != "" {
		domainsMap[strings.ToLower(u.Hostname())] = true
	}

	domains := make([]string, 0, len(domainsMap))
	for d := range domainsMap {
		domains = append(domains, d)
	}

	return domains
}

func applyPersistedSettings(ds *datastore.DataStore, config *serviceConfig) datastore.Settings {
	persisted, err := ds.GetSettings()
	if err != nil {
		return datastore.Settings{}
	}

	// Only override CLI values if settings file exists
	// If no settings file exists, GetSettings returns empty Settings{} and we should preserve CLI values
	settingsPath := filepath.Join(ds.DataDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		return datastore.Settings{}
	}

	if persisted.ServerURL != "" {
		config.serverURL = persisted.ServerURL
	}

	if persisted.HTTPServerURL != "" {
		config.httpsServerURL = persisted.HTTPServerURL
	}

	if persisted.DiscoveryInterval != "" {
		if d, durErr := time.ParseDuration(persisted.DiscoveryInterval); durErr == nil {
			config.discoveryInterval = d
		}
	}

	config.redact = persisted.RedactLogs
	config.logBody = persisted.LogBodies
	config.record = persisted.RecordInteractions

	config.dnsEnabled = persisted.DNSEnabled
	if len(persisted.DNSUpstream) > 0 {
		config.dnsUpstream = strings.Join(persisted.DNSUpstream, ",")
	}

	if persisted.DNSBindAddr != "" {
		config.dnsBind = persisted.DNSBindAddr
	}

	config.mirrorEnabled = persisted.MirrorEnabled
	config.mirrorEndpoints = persisted.MirrorEndpoints
	config.skipMirrorEndpoints = persisted.SkipMirrorEndpoints
	config.preferredSource = persisted.PreferredSource
	config.internalPaths = persisted.InternalPaths

	return persisted
}

func createDefaultSettings(ds *datastore.DataStore, config serviceConfig) datastore.Settings {
	settings := datastore.Settings{
		ServerURL:           config.serverURL,
		HTTPServerURL:       config.httpsServerURL,
		RedactLogs:          config.redact,
		LogBodies:           config.logBody,
		RecordInteractions:  config.record,
		DiscoveryInterval:   config.discoveryInterval.String(),
		DiscoveryEnabled:    true,
		DNSEnabled:          config.dnsEnabled,
		DNSUpstream:         strings.Split(config.dnsUpstream, ","),
		DNSBindAddr:         config.dnsBind,
		MirrorEnabled:       config.mirrorEnabled,
		MirrorEndpoints:     config.mirrorEndpoints,
		SkipMirrorEndpoints: config.skipMirrorEndpoints,
		PreferredSource:     config.preferredSource,
		InternalPaths:       config.internalPaths,
		Shortcuts: map[string]int{
			"/.well-known/appspecific/com.chrome.devtools.json": http.StatusNotFound,
			"/sw.js": http.StatusNotFound,
		},
	}

	_ = ds.SaveSettings(settings)

	return settings
}

func initDataStore(dataDir string) *datastore.DataStore {
	ds := datastore.NewDataStore(dataDir)
	if err := ds.Initialize(); err != nil {
		log.Printf("Warning: Failed to initialize datastore: %v", err)
	}

	return ds
}

func initCertificateManager(dataDir string) *certmanager.CertificateManager {
	cm := certmanager.NewCertificateManager(filepath.Join(dataDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		log.Printf("Warning: Failed to ensure CA: %v", err)
	}

	return cm
}

func startDeviceDiscovery(server *handlers.Server) {
	go func() {
		for {
			currentInterval, enabled := server.GetDiscoverySettings()
			if enabled {
				server.DiscoverDevices(context.Background())
			}

			time.Sleep(currentInterval)
		}
	}()
}

func setupRouter(server *handlers.Server) *chi.Mux {
	r := chi.NewRouter()
	r.Use(server.SnapshotMiddleware)
	r.Use(server.OriginMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(server.ShortcutMiddleware)
	r.Use(server.MirrorMiddleware)
	r.Use(server.RecordMiddleware)

	r.Get("/", server.HandleRoot)
	r.Get("/health", server.HandleHealth)
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/media/favicon-braille.svg"
		server.HandleMedia()(w, r)
	})

	r.Get("/media/*", server.HandleMedia())
	r.Get("/web/*", server.HandleWeb())
	r.Get("/docs/*", server.HandleDocs)

	r.Route("/bmx", func(r chi.Router) {
		r.Get("/registry/v1/services", server.HandleBMXRegistry)

		r.Route("/tunein", func(r chi.Router) {
			r.Get("/v1/playback/station/{stationID}", server.HandleTuneInPlayback)
			r.Get("/v1/playback/episodes/{podcastID}", server.HandleTuneInPodcastInfo)
			r.Get("/v1/playback/episode/{podcastID}", server.HandleTuneInPlaybackPodcast)
			r.Post("/v1/token", server.HandleTuneInToken)
		})

		r.Post("/orion/v1/playback/station/{data}", server.HandleOrionPlayback)
	})

	r.Get("/custom/v1/playback/{encodedURL}", server.HandleCustomPlayback)

	r.Route("/streaming", func(r chi.Router) {
		r.Get("/sourceproviders", server.HandleMargeSourceProviders)
		r.Post("/account", server.HandleMargeCreateAccount)
		r.Post("/account/login", server.HandleMargeLogin)

		r.Route("/account/{account}", func(r chi.Router) {
			r.Get("/emailaddress", server.HandleMargeGetEmailAddress)
			r.Get("/full", server.HandleMargeAccountFull)
			r.Get("/provider_settings", server.HandleMargeProviderSettings)

			r.Route("/device", func(r chi.Router) {
				r.Post("/", server.HandleMargeAddDevice)
				r.Post("/{device}", server.HandleMargeAddDevice)
			})

			r.Route("/device/{device}", func(r chi.Router) {
				r.Get("/presets", server.HandleMargePresets)
				r.Post("/presets/{presetNumber}", server.HandleMargeUpdatePreset)
				r.Get("/recent", server.HandleMargeRecents)
				r.Post("/recent", server.HandleMargeAddRecent)

				r.Get("/group", server.HandleMargeDeviceGroup)
				r.Get("/group/", server.HandleMargeDeviceGroup)
				r.Get("/group/server", server.HandleMargeDeviceGroupServer)
				r.Get("/group/member", server.HandleMargeDeviceGroupMember)
			})
		})

		r.Get("/device/{device}/streaming_token", server.HandleMargeStreamingToken)

		r.Get("/device_setting/account/{account}/device/{device}/device_settings", server.HandleMargeGetDeviceSettings)
		r.Post("/device_setting/account/{account}/device/{device}/device_settings", server.HandleMargeUpdateDeviceSettings)

		r.Get("/software/update/account/{account}", server.HandleMargeSoftwareUpdate)

		r.Route("/support", func(r chi.Router) {
			r.Post("/power_on", server.HandleMargePowerOn)
			r.Post("/customersupport", server.HandleMargeCustomerSupport)
		})

		r.Route("/stats", func(r chi.Router) {
			r.Post("/usage", server.HandleUsageStats)
			r.Post("/error", server.HandleErrorStats)
		})
	})

	r.Route("/accounts", func(r chi.Router) {
		r.Route("/{account}", func(r chi.Router) {
			r.Get("/full", server.HandleMargeAccountFull)

			r.Post("/devices", server.HandleMargeAddDevice)

			r.Delete("/devices/{device}", server.HandleMargeRemoveDevice)
			r.Get("/devices/{device}/group", server.HandleMargeDeviceGroup)
			r.Get("/devices/{device}/group/", server.HandleMargeDeviceGroup)
			r.Get("/devices/{device}/group/server", server.HandleMargeDeviceGroupServer)
			r.Get("/devices/{device}/group/member", server.HandleMargeDeviceGroupMember)
			r.Get("/devices/{device}/presets", server.HandleMargePresets)
			r.Get("/devices/{device}/recents", server.HandleMargeRecents)

			r.Post("/devices/{device}/presets/{presetNumber}", server.HandleMargeUpdatePreset)
			r.Post("/devices/{device}/recents", server.HandleMargeAddRecent)
		})
	})

	r.Get("/updates/soundtouch", server.HandleMargeSoftwareUpdate)

	r.Route("/customer", func(r chi.Router) {
		r.Get("/account/{account}", server.HandleMargeAccountProfile)
		r.Post("/account/{account}", server.HandleMargeUpdateAccountProfile)
		r.Post("/account/{account}/password", server.HandleMargeChangePassword)
	})

	r.Route("/oauth", func(r chi.Router) {
		r.Post("/device/{deviceID}/music/musicprovider/{sourceID}/token/cs3", server.HandleBoseToken)
		r.Post("/device/{deviceID}/music/musicprovider/{sourceID}/token", server.HandleBoseLegacyToken)
		r.HandleFunc("/*", server.HandleBoseProxy)
	})

	r.Route("/v1", func(r chi.Router) {
		r.Post("/stapp/{deviceId}", server.HandleAppEvents)
		r.Post("/scmudc/{deviceId}", server.HandleAppEvents)
	})

	r.Route("/mgmt", func(r chi.Router) {
		// Browser OAuth callback — no auth required (Spotify redirects the
		// user's browser here directly). The authorization code is single-use,
		// short-lived, and useless without the client_secret.
		r.Get("/spotify/callback", server.HandleMgmtSpotifyCallback)

		// All other management endpoints require Basic Auth.
		r.Group(func(r chi.Router) {
			r.Use(server.BasicAuthMgmt())

			r.Route("/accounts", func(r chi.Router) {
				r.Get("/", server.HandleMgmtListAccounts)
				r.Get("/{accountId}", server.HandleMgmtAccountDetails)
				r.Post("/{accountId}/language", server.HandleMgmtUpdateAccountLanguage)
				r.Post("/{accountId}/provider-settings", server.HandleMgmtUpdateAccountProviderSetting)
				r.Get("/{accountId}/speakers", server.HandleMgmtListSpeakers)
			})

			r.Route("/spotify", func(r chi.Router) {
				r.Post("/init", server.HandleMgmtSpotifyInit)
				r.Post("/confirm", server.HandleMgmtSpotifyConfirm)
				r.Get("/accounts", server.HandleMgmtSpotifyAccounts)
				r.Get("/token", server.HandleMgmtSpotifyToken)
				r.Post("/entity", server.HandleMgmtSpotifyEntity)
				r.Post("/prime", server.HandleMgmtPrimeDevice)
			})

			r.Get("/devices/{deviceId}/events", server.HandleMgmtDeviceEvents)
		})
	})

	r.Get("/proxy/*", server.HandleProxyRequest)

	r.Route("/setup", func(r chi.Router) {
		r.Get("/devices", server.HandleListDiscoveredDevices)
		r.Post("/devices", server.HandleAddManualDevice)
		r.Delete("/devices/{deviceId}", server.HandleRemoveDevice)
		r.Post("/discover", server.HandleTriggerDiscovery)
		r.Get("/discovery-status", server.HandleGetDiscoveryStatus)
		r.Get("/settings", server.HandleGetSettings)
		r.Post("/settings", server.HandleUpdateSettings)
		r.Get("/info/{deviceId}", server.HandleGetDeviceInfo)
		r.Get("/summary/{deviceId}", server.HandleGetMigrationSummary)
		r.Post("/migrate/{deviceId}", server.HandleMigrateDevice)
		r.Post("/revert/{deviceId}", server.HandleRevertMigration)
		r.Post("/reboot/{deviceId}", server.HandleRebootDevice)
		r.Post("/trust-ca/{deviceId}", server.HandleTrustCACert)
		r.Post("/ensure-remote-services/{deviceId}", server.HandleEnsureRemoteServices)
		r.Post("/remove-remote-services/{deviceId}", server.HandleRemoveRemoteServices)
		r.Post("/backup/{deviceId}", server.HandleBackupConfig)
		r.Post("/sync/{deviceId}", server.HandleInitialSync)
		r.Post("/test-connection/{deviceId}", server.HandleTestConnection)
		r.Post("/test-hosts/{deviceId}", server.HandleTestHostsRedirection)
		r.Post("/test-dns/{deviceId}", server.HandleTestDNSRedirection)
		r.Get("/ca.crt", server.HandleGetCACert)
		r.Get("/proxy-settings", server.HandleGetProxySettings)
		r.Post("/proxy-settings", server.HandleUpdateProxySettings)
		r.Get("/version", server.HandleGetVersionInfo)
		r.Get("/interaction-stats", server.HandleGetInteractionStats)
		r.Get("/interactions", server.HandleListInteractions)
		r.Get("/interaction-content", server.HandleGetInteractionContent)
		r.Get("/parity-mismatches", server.HandleListParityMismatches)
		r.Delete("/parity-mismatches", server.HandleClearParityMismatches)
		r.Get("/interactions/sessions/{session}/download", server.HandleDownloadSession)
		r.Delete("/interactions/sessions/{session}", server.HandleDeleteSession)
		r.Delete("/interactions/sessions", server.HandleCleanupSessions)

		r.Get("/dns-discoveries", server.HandleGetDNSDiscoveries)
		r.Get("/dns-discoveries/download", server.HandleDownloadDNSDiscoveries)
		r.Delete("/dns-discoveries", server.HandleClearDNSDiscoveries)

		r.Get("/devices/{deviceId}/events", server.HandleGetDeviceEvents)
	})

	r.NotFound(server.HandleNotFound)

	return r
}

func startHTTPSServer(httpsAddr string, r http.Handler, tlsConfig *tls.Config, httpsServerURL string) {
	// Add custom error logging and connection state tracking
	tlsConfig.GetCertificate = func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		// log.Printf("[TLS] Certificate request for ServerName: %s", clientHello.ServerName)

		// Use the default certificate selection logic
		for _, cert := range tlsConfig.Certificates {
			if cert.Leaf != nil {
				for _, name := range cert.Leaf.DNSNames {
					if matchesDomain(name, clientHello.ServerName) {
						// log.Printf("[TLS] ✅ Serving certificate for %s (matched %s)", clientHello.ServerName, name)
						return &cert, nil
					}
				}
			}
		}

		// If no specific match, return the first certificate and log it
		if len(tlsConfig.Certificates) > 0 {
			// log.Printf("[TLS] ⚠️ No exact match for %s, using default certificate", clientHello.ServerName)
			return &tlsConfig.Certificates[0], nil
		}

		log.Printf("[TLS] ❌ No certificate available for %s", clientHello.ServerName)

		return nil, fmt.Errorf("no certificate available for %s", clientHello.ServerName)
	}

	httpsServer := &http.Server{
		Addr:      httpsAddr,
		Handler:   r,
		TLSConfig: tlsConfig,
		ErrorLog:  log.Default(), // Ensure error logging is enabled
	}

	log.Printf("Go service starting HTTPS on %s", httpsServerURL)

	go func() {
		listener, err := net.Listen("tcp", httpsAddr)
		if err != nil {
			log.Printf("[TLS] Failed to create listener: %v", err)
			return
		}

		tlsListener := tls.NewListener(listener, tlsConfig)

		// Wrap listener to log connection attempts
		wrappedListener := &loggingTLSListener{
			Listener: tlsListener,
		}

		if err := httpsServer.Serve(wrappedListener); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTPS server error: %v", err)
		}
	}()
}

// matchesDomain checks if a certificate domain (which may be a wildcard) matches a server name
func matchesDomain(certDomain, serverName string) bool {
	if certDomain == serverName {
		return true
	}

	// Handle wildcard certificates (only at the beginning of a label)
	if strings.HasPrefix(certDomain, "*.") {
		certBase := certDomain[2:] // Remove "*."

		// For *.api.bose.io to match events.api.bose.io but not test.content.api.bose.io
		// We need to ensure only one label is replaced by the wildcard
		if strings.HasSuffix(serverName, "."+certBase) {
			// Count dots to ensure we're not matching too many levels
			serverPrefix := strings.TrimSuffix(serverName, "."+certBase)
			if !strings.Contains(serverPrefix, ".") {
				return true
			}
		}

		// Also match the base domain (e.g., api.bose.io matches *.api.bose.io)
		if serverName == certBase {
			return true
		}
	}

	return false
}

// loggingTLSListener wraps a TLS listener to log connection attempts and handshake failures
type loggingTLSListener struct {
	net.Listener
}

func (l *loggingTLSListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// Wrap the connection to log TLS handshake results
	return &loggingTLSConn{
		Conn: conn,
		addr: conn.RemoteAddr(),
	}, nil
}

// loggingTLSConn wraps a TLS connection to log handshake failures
type loggingTLSConn struct {
	net.Conn
	addr            net.Addr
	handshakeLogged bool
}

func (c *loggingTLSConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)

	// Log TLS handshake failures on first read attempt
	if !c.handshakeLogged {
		c.handshakeLogged = true

		if err != nil {
			// Check if this looks like a TLS handshake failure
			if strings.Contains(err.Error(), "tls:") ||
				strings.Contains(err.Error(), "handshake") ||
				strings.Contains(err.Error(), "certificate") {
				log.Printf("[TLS] ❌ Handshake failed from %s: %v", c.addr, err)
			}
		}
	}

	return n, err
}
