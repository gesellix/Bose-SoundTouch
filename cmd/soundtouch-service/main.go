// Package main provides the SoundTouch service daemon that acts as a proxy and management
// interface for Bose SoundTouch devices, providing Marge service emulation and device discovery.
package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/crypto"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	config := loadConfig()
	ds := initDataStore(config.dataDir)
	cm := initCertificateManager(config.dataDir)
	sm := setup.NewManager(config.serverURL, ds, cm)
	server := handlers.NewServer(ds, sm, config.serverURL, config.redact, config.logBody)

	tlsConfig, err := cm.GetServerTLSConfig(config.domains)
	if err != nil {
		log.Printf("Warning: Failed to setup TLS: %v", err)
	}

	pyProxy := setupPythonProxy(config.targetURL, config.redact, config.logBody)

	startDeviceDiscovery(server)

	r := setupRouter(server, pyProxy)

	log.Printf("Go service starting on %s, proxying to %s", config.serverURL, config.targetURL)

	if tlsConfig != nil {
		startHTTPSServer(config.httpsAddr, r, tlsConfig, config.httpsServerURL)
	}

	log.Fatal(http.ListenAndServe(config.addr, r))
}

type serviceConfig struct {
	port           string
	bindAddr       string
	addr           string
	targetURL      string
	dataDir        string
	serverURL      string
	httpsServerURL string
	httpsAddr      string
	redact         bool
	logBody        bool
	domains        []string
}

func loadConfig() serviceConfig {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	bindAddr := os.Getenv("BIND_ADDR")

	addr := bindAddr + ":" + port
	if bindAddr == "" {
		addr = ":" + port
	}

	targetURL := os.Getenv("PYTHON_BACKEND_URL")
	if targetURL == "" {
		targetURL = "http://localhost:8001"
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "localhost"
		}

		serverURL = "http://" + strings.ToLower(hostname) + ":" + port
	}

	httpsPort := os.Getenv("HTTPS_PORT")
	if httpsPort == "" {
		httpsPort = "8443"
	}

	httpsAddr := bindAddr + ":" + httpsPort
	if bindAddr == "" {
		httpsAddr = ":" + httpsPort
	}

	httpsServerURL := os.Getenv("HTTPS_SERVER_URL")
	if httpsServerURL == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "localhost"
		}

		httpsServerURL = "https://" + strings.ToLower(hostname) + ":" + httpsPort
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	hostname = strings.ToLower(hostname)

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

	return serviceConfig{
		port:           port,
		bindAddr:       bindAddr,
		addr:           addr,
		targetURL:      targetURL,
		dataDir:        dataDir,
		serverURL:      serverURL,
		httpsServerURL: httpsServerURL,
		httpsAddr:      httpsAddr,
		redact:         os.Getenv("REDACT_PROXY_LOGS") != "false",
		logBody:        os.Getenv("LOG_PROXY_BODY") == "true",
		domains:        domains,
	}
}

func initDataStore(dataDir string) *datastore.DataStore {
	ds := datastore.NewDataStore(dataDir)
	if err := ds.Initialize(); err != nil {
		log.Printf("Warning: Failed to initialize datastore: %v", err)
	}

	return ds
}

func initCertificateManager(dataDir string) *crypto.CertificateManager {
	cm := crypto.NewCertificateManager(filepath.Join(dataDir, "certs"))
	if err := cm.EnsureCA(); err != nil {
		log.Printf("Warning: Failed to ensure CA: %v", err)
	}

	return cm
}

func setupPythonProxy(targetURL string, redact, logBody bool) *httputil.ReverseProxy {
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

	return pyProxy
}

func startDeviceDiscovery(server *handlers.Server) {
	go func() {
		for {
			server.DiscoverDevices(context.Background())
			time.Sleep(5 * time.Minute)
		}
	}()
}

func setupRouter(server *handlers.Server, pyProxy *httputil.ReverseProxy) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", server.HandleRoot)
	r.Get("/health", server.HandleHealth)
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/media/favicon-braille.svg"
		server.HandleMedia()(w, r)
	})

	r.Get("/media/*", server.HandleMedia())
	r.Get("/web/*", server.HandleWeb())

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
		r.Post("/discover", server.HandleTriggerDiscovery)
		r.Get("/discovery-status", server.HandleGetDiscoveryStatus)
		r.Get("/settings", server.HandleGetSettings)
		r.Get("/info/{deviceIP}", server.HandleGetDeviceInfo)
		r.Get("/summary/{deviceIP}", server.HandleGetMigrationSummary)
		r.Post("/migrate/{deviceIP}", server.HandleMigrateDevice)
		r.Post("/ensure-remote-services/{deviceIP}", server.HandleEnsureRemoteServices)
		r.Post("/remove-remote-services/{deviceIP}", server.HandleRemoveRemoteServices)
		r.Post("/backup/{deviceIP}", server.HandleBackupConfig)
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
