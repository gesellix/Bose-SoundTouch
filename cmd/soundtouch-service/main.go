// Package main provides the SoundTouch service daemon that acts as a proxy and management
// interface for Bose SoundTouch devices, providing Marge service emulation and device discovery.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
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
				Name:    "target-url",
				Usage:   "URL for Python-based service components (legacy)",
				Value:   "http://localhost:8001",
				EnvVars: []string{"PYTHON_BACKEND_URL", "TARGET_URL"},
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
		},
		Action: func(c *cli.Context) error {
			config := loadConfig(c)
			ds := initDataStore(config.dataDir)

			// Load settings from datastore
			persisted, _ := ds.GetSettings()
			if persisted.ServerURL != "" {
				config.serverURL = persisted.ServerURL
			}

			if persisted.ProxyURL != "" {
				config.targetURL = persisted.ProxyURL
			}

			if persisted.HTTPServerURL != "" {
				config.httpsServerURL = persisted.HTTPServerURL
			}

			config.redact = persisted.RedactLogs || config.redact
			config.logBody = persisted.LogBodies || config.logBody
			config.record = persisted.RecordInteractions || config.record

			// Recalculate domains if settings changed
			hostname, _ := os.Hostname()
			if hostname == "" {
				hostname = "localhost"
			}

			config.domains = getDomains(config.serverURL, config.httpsServerURL, hostname)

			cm := initCertificateManager(config.dataDir)
			sm := setup.NewManager(config.serverURL, ds, cm)
			server := handlers.NewServer(ds, sm, config.serverURL, config.redact, config.logBody, config.record)
			server.SetHTTPServerURL(config.httpsServerURL)

			recorder := proxy.NewRecorder(config.dataDir)
			recorder.Redact = config.redact
			patternsPath := filepath.Join(config.dataDir, "patterns.json")

			patterns, err := proxy.LoadPatterns(patternsPath)
			if err == nil && len(patterns) > 0 {
				recorder.Patterns = patterns
			} else if err != nil {
				log.Printf("Warning: Failed to load patterns from %s: %v", patternsPath, err)
			}

			server.SetRecorder(recorder)

			tlsConfig, err := cm.GetServerTLSConfig(config.domains)
			if err != nil {
				log.Printf("Warning: Failed to setup TLS: %v", err)
			}

			pyProxy := setupPythonProxy(config.targetURL, config.redact, config.logBody, recorder, server)

			startDeviceDiscovery(server, config.discoveryInterval)

			r := setupRouter(server, pyProxy)

			log.Printf("Go service starting on %s, proxying to %s", config.serverURL, config.targetURL)

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
	port              string
	bindAddr          string
	addr              string
	targetURL         string
	dataDir           string
	serverURL         string
	httpsServerURL    string
	httpsAddr         string
	redact            bool
	logBody           bool
	record            bool
	discoveryInterval time.Duration
	domains           []string
}

func loadConfig(c *cli.Context) serviceConfig {
	port := c.String("port")
	bindAddr := c.String("bind")

	addr := bindAddr + ":" + port
	if bindAddr == "" {
		addr = ":" + port
	}

	targetURL := c.String("target-url")
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

	discoveryIntervalStr := c.String("discovery-interval")

	discoveryInterval, err := time.ParseDuration(discoveryIntervalStr)
	if err != nil {
		log.Printf("Warning: Failed to parse discovery interval %s, using default 5m: %v", discoveryIntervalStr, err)

		discoveryInterval = 5 * time.Minute
	}

	return serviceConfig{
		port:              port,
		bindAddr:          bindAddr,
		addr:              addr,
		targetURL:         targetURL,
		dataDir:           dataDir,
		serverURL:         serverURL,
		httpsServerURL:    httpsServerURL,
		httpsAddr:         httpsAddr,
		redact:            redact,
		logBody:           logBody,
		record:            record,
		discoveryInterval: discoveryInterval,
		domains:           domains,
	}
}

func getDomains(serverURL, httpsServerURL, hostname string) []string {
	domainsMap := map[string]bool{
		"streaming.bose.com":  true,
		"updates.bose.com":    true,
		"stats.bose.com":      true,
		"bmx.bose.com":        true,
		"content.api.bose.io": true,
		setup.TestDomain:      true,
		hostname:              true,
		"localhost":           true,
		"127.0.0.1":           true,
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

func setupPythonProxy(targetURL string, redact, logBody bool, recorder *proxy.Recorder, server *handlers.Server) *httputil.ReverseProxy {
	target, err := url.Parse(targetURL)
	if err != nil {
		log.Fatalf("Failed to parse target URL: %v", err)
	}

	pyProxy := httputil.NewSingleHostReverseProxy(target)
	pyProxy.ModifyResponse = func(res *http.Response) error {
		if etags, ok := res.Header["Etag"]; ok {
			delete(res.Header, "Etag")
			res.Header["ETag"] = etags
		}

		currentLp := proxy.NewLoggingProxy(target.String(), redact)
		currentLp.LogBody = logBody
		currentLp.RecordEnabled = server.GetRecordEnabled()
		currentLp.SetRecorder(recorder)
		currentLp.LogResponse(res)

		return nil
	}

	originalPyDirector := pyProxy.Director
	pyProxy.Director = func(req *http.Request) {
		originalPyDirector(req)

		currentLp := proxy.NewLoggingProxy(target.String(), redact)
		currentLp.LogBody = logBody
		currentLp.RecordEnabled = server.GetRecordEnabled()
		currentLp.SetRecorder(recorder)
		currentLp.LogRequest(req)
	}

	return pyProxy
}

func startDeviceDiscovery(server *handlers.Server, interval time.Duration) {
	go func() {
		for {
			server.DiscoverDevices(context.Background())
			time.Sleep(interval)
		}
	}()
}

func setupRouter(server *handlers.Server, pyProxy *httputil.ReverseProxy) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
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
		r.Get("/tunein/v1/playback/station/{stationID}", server.HandleTuneInPlayback)
		r.Get("/tunein/v1/playback/episodes/{podcastID}", server.HandleTuneInPodcastInfo)
		r.Get("/tunein/v1/playback/episode/{podcastID}", server.HandleTuneInPlaybackPodcast)
		r.Post("/orion/v1/playback/station/{data}", server.HandleOrionPlayback)
	})

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

	r.Route("/streaming/stats", func(r chi.Router) {
		r.Post("/usage", server.HandleUsageStats)
		r.Post("/error", server.HandleErrorStats)
	})

	r.Get("/proxy/*", server.HandleProxyRequest)

	r.Route("/setup", func(r chi.Router) {
		r.Get("/devices", server.HandleListDiscoveredDevices)
		r.Post("/devices", server.HandleAddManualDevice)
		r.Post("/discover", server.HandleTriggerDiscovery)
		r.Get("/discovery-status", server.HandleGetDiscoveryStatus)
		r.Get("/settings", server.HandleGetSettings)
		r.Post("/settings", server.HandleUpdateSettings)
		r.Get("/info/{deviceIP}", server.HandleGetDeviceInfo)
		r.Get("/summary/{deviceIP}", server.HandleGetMigrationSummary)
		r.Post("/migrate/{deviceIP}", server.HandleMigrateDevice)
		r.Post("/revert/{deviceIP}", server.HandleRevertMigration)
		r.Post("/reboot/{deviceIP}", server.HandleRebootDevice)
		r.Post("/trust-ca/{deviceIP}", server.HandleTrustCACert)
		r.Post("/ensure-remote-services/{deviceIP}", server.HandleEnsureRemoteServices)
		r.Post("/remove-remote-services/{deviceIP}", server.HandleRemoveRemoteServices)
		r.Post("/backup/{deviceIP}", server.HandleBackupConfig)
		r.Post("/sync/{deviceIP}", server.HandleInitialSync)
		r.Post("/test-connection/{deviceIP}", server.HandleTestConnection)
		r.Post("/test-hosts/{deviceIP}", server.HandleTestHostsRedirection)
		r.Get("/ca.crt", server.HandleGetCACert)
		r.Get("/proxy-settings", server.HandleGetProxySettings)
		r.Post("/proxy-settings", server.HandleUpdateProxySettings)
		r.Get("/devices/{deviceId}/events", server.HandleGetDeviceEvents)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		pyProxy.ServeHTTP(w, r)
	})

	return r
}

func startHTTPSServer(httpsAddr string, r http.Handler, tlsConfig *tls.Config, httpsServerURL string) {
	httpsServer := &http.Server{
		Addr:      httpsAddr,
		Handler:   r,
		TLSConfig: tlsConfig,
	}

	log.Printf("Go service starting HTTPS on %s", httpsServerURL)

	go func() {
		if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTPS server error: %v", err)
		}
	}()
}
