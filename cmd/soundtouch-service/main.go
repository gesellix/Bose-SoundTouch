// Package main provides the SoundTouch service daemon that acts as a proxy and management
// interface for Bose SoundTouch devices, providing Marge service emulation and device discovery.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/service/amazon"
	"github.com/gesellix/bose-soundtouch/pkg/service/bmx"
	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/gesellix/bose-soundtouch/pkg/service/logbuf"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/gesellix/bose-soundtouch/pkg/service/stockholm"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

func initializeDefaultSources(ds *datastore.DataStore) {
	// Ensure default sources exist for all known devices on startup
	allDevices, _ := ds.ListAllDevices()
	for i := range allDevices {
		dev := &allDevices[i]
		if sources, errGet := ds.GetConfiguredSources(dev.AccountID, dev.DeviceID); errGet == nil {
			log.Printf("Initializing default Sources.xml for existing device %s", sanitizeLog(dev.DeviceID))

			// Find default sources and merge them if missing or outdated tokens.
			// claimed tracks which stored sources have already been matched by a default,
			// so two defaults with the same SourceKeyType but different SourceProviderIDs
			// (e.g. INTERNET_RADIO/2 and INTERNET_RADIO/39) are treated as distinct entries.
			defaults := ds.GetInitialSources()
			modified := false
			claimed := make(map[int]bool)

			for i := range defaults {
				def := defaults[i]
				foundIdx := -1

				for j := range sources {
					if claimed[j] || sources[j].SourceKeyType != def.SourceKeyType {
						continue
					}
					// When both sides have a providerID, require it to match.
					if def.SourceProviderID != "" && sources[j].SourceProviderID != "" && sources[j].SourceProviderID != def.SourceProviderID {
						continue
					}

					foundIdx = j

					break
				}

				if foundIdx >= 0 {
					claimed[foundIdx] = true

					if sources[foundIdx].Secret == "" && def.Secret != "" {
						log.Printf("Initializing missing token for source %s on device %s", sanitizeLog(def.SourceKeyType), sanitizeLog(dev.DeviceID))
						sources[foundIdx].Secret = def.Secret
						sources[foundIdx].SecretType = def.SecretType
						modified = true
					}
				} else {
					log.Printf("Adding missing default source %s (providerID=%s) to device %s", sanitizeLog(def.SourceKeyType), sanitizeLog(def.SourceProviderID), sanitizeLog(dev.DeviceID))
					sources = append(sources, def)
					modified = true
				}
			}

			if modified {
				if errSave := ds.SaveConfiguredSources(dev.AccountID, dev.DeviceID, sources); errSave != nil {
					log.Printf("Failed to save updated sources for %s: %v", sanitizeLog(dev.DeviceID), errSave)
				}
			}
		}
	}
}

func initMusicServices(config serviceConfig, server *handlers.Server) {
	if config.spotifyClientID != "" {
		spotifyService := spotify.NewSpotifyService(
			config.spotifyClientID,
			config.spotifyClientSecret,
			config.spotifyRedirectURI,
			config.dataDir,
		)
		if config.spotifyTokenURL != "" || config.spotifyAPIBase != "" {
			spotifyService.SetEndpoints(config.spotifyTokenURL, config.spotifyAPIBase)
		}

		if err := spotifyService.Load(); err != nil {
			log.Printf("[Spotify] Failed to load accounts: %v", err)
		}

		server.SetSpotifyService(spotifyService)

		clientIDPrefix := config.spotifyClientID
		if len(clientIDPrefix) > 8 {
			clientIDPrefix = clientIDPrefix[:8]
		}

		log.Printf("Spotify service initialized (client ID: %s...)", sanitizeLog(clientIDPrefix))
	}

	if config.amazonClientID != "" {
		amazonService := amazon.NewAmazonService(
			config.amazonClientID,
			config.amazonClientSecret,
			config.amazonRedirectURI,
			config.dataDir,
		)
		if config.amazonTokenURL != "" || config.amazonProfileURL != "" {
			amazonService.SetEndpoints(config.amazonTokenURL, config.amazonProfileURL)
		}

		if err := amazonService.Load(); err != nil {
			log.Printf("[Amazon] Failed to load accounts: %v", err)
		}

		server.SetAmazonService(amazonService)

		clientIDPrefix := config.amazonClientID
		if len(clientIDPrefix) > 8 {
			clientIDPrefix = clientIDPrefix[:8]
		}

		log.Printf("Amazon Music service initialized (client ID: %s...)", sanitizeLog(clientIDPrefix))
	}
}

// initTTSService loads the text-to-speech configuration onto the server and
// builds the running service. The provider construction and (re)build logic
// lives on the server so the settings UI can re-apply changes at runtime; see
// handlers.Server.ReinitTTSService.
func initTTSService(config serviceConfig, server *handlers.Server) {
	server.SetTTSConfig(
		config.ttsProvider,
		config.ttsGoogleAPIKey,
		config.ttsGoogleEndpoint,
		config.ttsAppKey,
		config.ttsLanguage,
		config.ttsVoice,
		config.ttsVolume,
	)
	server.ReinitTTSService()
}

// logBufferCapacityFromEnv reads SOUNDTOUCH_LOG_BUFFER_LINES and
// returns a positive capacity. Invalid or unset values fall back
// to the default; a value of 0 or negative is treated as "disable"
// and returns 0 so the caller can skip wiring the buffer.
func logBufferCapacityFromEnv(defaultCap int) int {
	raw := os.Getenv("SOUNDTOUCH_LOG_BUFFER_LINES")
	if raw == "" {
		return defaultCap
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("[Logs] Invalid SOUNDTOUCH_LOG_BUFFER_LINES=%q, using default %d", sanitizeLog(raw), defaultCap)
		return defaultCap
	}

	if v < 0 {
		return 0
	}

	return v
}

func main() {
	updateBuildInfo()

	// Mirror log output to an in-memory ring buffer so the admin
	// UI can show a live trace. Stderr keeps receiving every line
	// verbatim — the buffer is a second sink, not a replacement.
	// Installing this before the cli.Action runs means every
	// log.Printf from initialisation onwards is captured.
	var logBuf *logbuf.Buffer

	if bufCap := logBufferCapacityFromEnv(2000); bufCap > 0 {
		logBuf = logbuf.New(bufCap)
		log.SetOutput(io.MultiWriter(os.Stderr, logBuf))
	}

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
			&cli.BoolFlag{
				Name:    "discovery-enabled",
				Usage:   "Enable periodic device discovery",
				Value:   true,
				EnvVars: []string{"DISCOVERY_ENABLED"},
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
				Usage:   "Spotify OAuth redirect URI (defaults to <server-url>/mgmt/spotify/callback)",
				EnvVars: []string{"SPOTIFY_REDIRECT_URI"},
			},
			&cli.StringFlag{
				Name:    "spotify-token-url",
				Usage:   "Spotify OAuth token URL (for testing)",
				EnvVars: []string{"SPOTIFY_TOKEN_URL"},
			},
			&cli.StringFlag{
				Name:    "spotify-api-base",
				Usage:   "Spotify API base URL (for testing)",
				EnvVars: []string{"SPOTIFY_API_BASE"},
			},
			&cli.StringFlag{
				Name:    "amazon-client-id",
				Usage:   "Amazon LWA OAuth client ID",
				EnvVars: []string{"AMAZON_CLIENT_ID"},
			},
			&cli.StringFlag{
				Name:    "amazon-client-secret",
				Usage:   "Amazon LWA OAuth client secret",
				EnvVars: []string{"AMAZON_CLIENT_SECRET"},
			},
			&cli.StringFlag{
				Name:    "amazon-redirect-uri",
				Usage:   "Amazon LWA OAuth redirect URI (defaults to <server-url>/mgmt/amazon/callback)",
				EnvVars: []string{"AMAZON_REDIRECT_URI"},
			},
			&cli.StringFlag{
				Name:    "amazon-token-url",
				Usage:   "Amazon LWA token URL (for testing)",
				EnvVars: []string{"AMAZON_TOKEN_URL"},
			},
			&cli.StringFlag{
				Name:    "amazon-profile-url",
				Usage:   "Amazon LWA profile URL (for testing)",
				EnvVars: []string{"AMAZON_PROFILE_URL"},
			},
			&cli.StringFlag{
				Name:    "tunein-opml-url",
				Usage:   "TuneIn OPML base URL, covering Tune.ashx/describe.ashx/navigate (for testing / local mock; defaults to opml.radiotime.com)",
				EnvVars: []string{"TUNEIN_OPML_URL"},
			},
			&cli.StringFlag{
				Name:    "tunein-api-url",
				Usage:   "TuneIn API base URL, covering search and profile contents (for testing / local mock; defaults to api.radiotime.com)",
				EnvVars: []string{"TUNEIN_API_URL"},
			},
			&cli.StringFlag{
				Name:    "tts-provider",
				Usage:   "Text-to-speech provider: 'translate' (Google Translate, no credentials, default) or 'google-cloud' (Google Cloud TTS, needs an API key). Empty falls back to translate; leave unset to let a value saved in the settings UI take effect",
				EnvVars: []string{"TTS_PROVIDER"},
			},
			&cli.StringFlag{
				Name:    "tts-google-api-key",
				Usage:   "Google Cloud Text-to-Speech API key (required when --tts-provider=google-cloud)",
				EnvVars: []string{"TTS_GOOGLE_API_KEY"},
			},
			&cli.StringFlag{
				Name:    "tts-google-endpoint",
				Usage:   "Google Cloud TTS synthesize endpoint override (for testing)",
				EnvVars: []string{"TTS_GOOGLE_ENDPOINT"},
			},
			&cli.StringFlag{
				Name:    "tts-language",
				Usage:   "Default TTS language code. Provider-specific: 'EN'/'DE' for translate, BCP-47 like 'en-US' for google-cloud",
				EnvVars: []string{"TTS_LANGUAGE"},
			},
			&cli.StringFlag{
				Name:    "tts-voice",
				Usage:   "Default Google Cloud TTS voice name (e.g. en-US-Neural2-C); ignored by the translate provider",
				EnvVars: []string{"TTS_VOICE"},
			},
			&cli.StringFlag{
				Name:    "tts-app-key",
				Usage:   "Bose /speaker app_key used to play TTS notifications on speakers",
				EnvVars: []string{"TTS_APP_KEY"},
			},
			&cli.IntFlag{
				Name:    "tts-volume",
				Usage:   "Default TTS playback volume (0-100, 0 = keep current volume)",
				Value:   0,
				EnvVars: []string{"TTS_VOLUME"},
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
			&cli.StringSliceFlag{
				Name:    "internal-paths",
				Usage:   "Paths for internal requests (comma-separated or multiple flags)",
				EnvVars: []string{"INTERNAL_PATHS"},
			},
			&cli.StringSliceFlag{
				Name:    "tls-extra-host",
				Usage:   "Additional DNS name or IP to include in the server TLS certificate SAN list (repeatable)",
				EnvVars: []string{"TLS_EXTRA_HOST"},
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
				Name:    "stockholm-dir",
				Usage:   "Path to the extracted Stockholm frontend directory (enables Stockholm UI when set)",
				EnvVars: []string{"STOCKHOLM_DIR"},
			},
			&cli.StringFlag{
				Name:    "stockholm-base-path",
				Usage:   "URL prefix under which the Stockholm UI is served (e.g. /stockholm). Empty serves at root.",
				Value:   "/stockholm",
				EnvVars: []string{"STOCKHOLM_BASE_PATH"},
			},
		},
		Action: func(c *cli.Context) error {
			config := loadConfig(c)
			ds := initDataStore(config.dataDir)

			// Detect a genuinely fresh data dir by the ABSENCE of settings.json,
			// not by an empty server_url. A hand-authored settings.json (e.g. one
			// that only sets trust_forwarded_headers and leaves server_url to the
			// --server-url flag) exists but has no server_url; keying the "first
			// run" default-write off server_url would treat it as fresh and
			// clobber the operator's file, dropping fields createDefaultSettings
			// doesn't know about.
			settingsExisted := settingsFileExists(config.dataDir)

			persisted := applyPersistedSettings(ds, &config)

			if !settingsExisted {
				log.Printf("Creating default settings.json in %s", sanitizeLog(config.dataDir))
				log.Printf("Data directory %s looks empty (first run). If you did NOT expect this "+
					"(e.g. after recreating a Docker container), your previous settings, datastore and "+
					"CA were not persisted; mount a persistent volume at the data dir (Docker: "+
					"-v <volume>:/app/data) so device state and the CA survive restarts. A lost CA "+
					"forces re-migrating speakers and re-trusting the new CA.",
					sanitizeLog(config.dataDir))
				persisted = createDefaultSettings(ds, config)
			}

			// Recalculate domains if settings changed
			hostname, _ := os.Hostname()
			if hostname == "" {
				hostname = "localhost"
			}

			config.domains = getDomains(config.serverURL, config.httpsServerURL, hostname, config.tlsExtraHosts)

			cm := initCertificateManager(config.dataDir, config.hostname)
			sm := setup.NewManager(config.serverURL, ds, cm)
			sm.MgmtUsername = config.mgmtUsername
			sm.MgmtPassword = config.mgmtPassword
			server := handlers.NewServer(ds, sm, config.serverURL, config.redact, config.logBody, config.record)
			sm.GetDNSRunning = server.GetDNSRunning
			server.SetLogBuffer(logBuf)
			server.SetHTTPServerURL(config.httpsServerURL)
			server.SetHTTPSListenAddr(config.httpsAddr)
			server.SetExpectedHosts(config.domains)
			server.SetVersionInfo(version, commit, date, repoURL)
			server.SetDiscoverySettings(config.discoveryInterval, config.discoveryEnabled)
			server.SetDNSSettings(persisted.DNSEnabled, strings.Join(persisted.DNSUpstream, ","), persisted.DNSBindAddr)
			server.SetInternalPaths(persisted.InternalPaths)
			server.SetSpotifyConfig(config.spotifyClientID, config.spotifyClientSecret, config.spotifyRedirectURI)
			server.SetAmazonConfig(config.amazonClientID, config.amazonClientSecret, config.amazonRedirectURI)
			server.SetMgmtConfig(config.mgmtUsername, config.mgmtPassword)

			initMusicServices(config, server)
			initTTSService(config, server)

			// Redirect TuneIn upstream calls when overridden (e.g. to a local
			// mock in integration tests); empty values keep the real hosts.
			if config.tuneInOpmlURL != "" || config.tuneInAPIURL != "" {
				bmx.SetTuneInEndpoints(config.tuneInOpmlURL, config.tuneInAPIURL)
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
				log.Printf("Warning: configured shortcut: %s -> %d", sanitizeLog(path), status)
			}

			recorder := proxy.NewRecorder(config.dataDir)
			recorder.Redact = config.redact
			patternsPath := filepath.Join(config.dataDir, "patterns.json")

			patterns, err := proxy.LoadPatterns(patternsPath)
			if err != nil {
				log.Printf("Warning: Failed to load patterns from %s: %v", sanitizeLog(patternsPath), err)
			}

			if len(patterns) == 0 {
				log.Printf("Creating default patterns at %s", sanitizeLog(patternsPath))

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

			initializeDefaultSources(ds)

			startDeviceDiscovery(server)

			var stockholmHandler *stockholm.Handler

			if config.stockholmDir != "" {
				sh, shErr := stockholm.New(config.stockholmDir, config.dataDir, config.serverURL, config.stockholmBasePath)
				if shErr != nil {
					log.Printf("Warning: Failed to initialise Stockholm handler: %v", shErr)
				} else {
					stockholmHandler = sh

					log.Printf("Stockholm frontend enabled from %s", sanitizeLog(config.stockholmDir))
				}
			}

			// Embedded web UI (soundtouch-player): LAN control UI under /app, control
			// API under /api/control. Same LAN-trust tier as /setup, no auth.
			// Server-side self-calls (TTS proxy) use the service's own loopback
			// HTTP listener so they never depend on TLS / the service CA.
			loopbackHost := config.bindAddr
			if loopbackHost == "" {
				loopbackHost = "127.0.0.1"
			}

			internalURL := "http://" + net.JoinHostPort(loopbackHost, config.port)
			webApp := newEmbeddedWebApp(server, config.serverURL, internalURL, ds)

			r := setupRouter(server, stockholmHandler, webApp)

			// Bind the listener before logging so we print the true
			// effective port (handles :0 and catches "address already
			// in use" before the TLS goroutine launches).
			ln, err := net.Listen("tcp", config.addr)
			if err != nil {
				return fmt.Errorf("failed to listen on %s: %w", config.addr, err)
			}

			log.Printf("Go service listening on %s (configured: %s, server URL: %s)",
				ln.Addr().String(), sanitizeLog(config.addr), sanitizeLog(config.serverURL))

			// TLS cert generation can be slow on constrained hardware; run it in the
			// background so the HTTP server is available immediately.
			log.Printf("HTTPS setup running in background; %s will be available shortly", sanitizeLog(config.httpsServerURL))

			go func() {
				tlsConfig, err := cm.GetServerTLSConfig(config.domains)
				if err != nil {
					log.Printf("Warning: Failed to setup TLS: %v", err)
					return
				}

				startHTTPSServer(config.httpsAddr, r, tlsConfig, config.httpsServerURL)

				runHTTPSPreflight(config.httpsServerURL, config.serverURL, config.dnsEnabled, server.ResolveServerURLIPForPreflight)
			}()

			return http.Serve(ln, r)
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
	hostname            string
	serverURL           string
	httpsServerURL      string
	httpsAddr           string
	redact              bool
	logBody             bool
	record              bool
	dnsEnabled          bool
	dnsUpstream         string
	dnsBind             string
	internalPaths       []string
	tlsExtraHosts       []string
	discoveryEnabled    bool
	discoveryInterval   time.Duration
	domains             []string
	spotifyClientID     string
	spotifyClientSecret string
	spotifyRedirectURI  string
	spotifyTokenURL     string
	spotifyAPIBase      string
	amazonClientID      string
	amazonClientSecret  string
	amazonRedirectURI   string
	amazonTokenURL      string
	amazonProfileURL    string
	tuneInOpmlURL       string
	tuneInAPIURL        string
	mgmtUsername        string
	mgmtPassword        string
	ttsProvider         string
	ttsGoogleAPIKey     string
	ttsGoogleEndpoint   string
	ttsLanguage         string
	ttsVoice            string
	ttsAppKey           string
	ttsVolume           int
	migrationEnabled    bool
	migrationDryRun     bool
	stockholmDir        string
	stockholmBasePath   string
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
	// Strip a trailing slash so it cannot leak into the BMX registry base or the
	// margeServerUrl/bmxRegistryUrl pushed to speakers during migration.
	serverURL = handlers.NormalizeServerURL(serverURL)

	httpsPort := c.String("https-port")

	httpsAddr := bindAddr + ":" + httpsPort
	if bindAddr == "" {
		httpsAddr = ":" + httpsPort
	}

	httpsServerURL := c.String("https-server-url")
	if httpsServerURL == "" {
		httpsServerURL = "https://" + hostname + ":" + httpsPort
	}

	tlsExtraHosts := c.StringSlice("tls-extra-host")
	domains := getDomains(serverURL, httpsServerURL, hostname, tlsExtraHosts)

	redact := c.Bool("redact-logs")
	logBody := c.Bool("log-bodies")
	record := c.Bool("record-interactions")

	dnsEnabled := c.Bool("dns-discovery")
	dnsUpstream := c.String("dns-upstream")
	dnsBind := c.String("dns-bind")

	discoveryEnabled := c.Bool("discovery-enabled")
	discoveryIntervalStr := c.String("discovery-interval")

	discoveryInterval, err := time.ParseDuration(discoveryIntervalStr)
	if err != nil {
		log.Printf("Warning: Failed to parse discovery interval %s, using default 5m: %v", sanitizeLog(discoveryIntervalStr), err)

		discoveryInterval = 5 * time.Minute
	}

	spotifyClientID := c.String("spotify-client-id")
	spotifyClientSecret := c.String("spotify-client-secret")
	spotifyRedirectURI := c.String("spotify-redirect-uri")
	spotifyTokenURL := c.String("spotify-token-url")
	spotifyAPIBase := c.String("spotify-api-base")
	amazonClientID := c.String("amazon-client-id")
	amazonClientSecret := c.String("amazon-client-secret")
	amazonRedirectURI := c.String("amazon-redirect-uri")
	amazonTokenURL := c.String("amazon-token-url")
	amazonProfileURL := c.String("amazon-profile-url")
	tuneInOpmlURL := c.String("tunein-opml-url")
	tuneInAPIURL := c.String("tunein-api-url")
	mgmtUsername := c.String("mgmt-username")
	mgmtPassword := c.String("mgmt-password")
	ttsProvider := c.String("tts-provider")
	ttsGoogleAPIKey := c.String("tts-google-api-key")
	ttsGoogleEndpoint := c.String("tts-google-endpoint")
	ttsLanguage := c.String("tts-language")
	ttsVoice := c.String("tts-voice")
	ttsAppKey := c.String("tts-app-key")
	ttsVolume := c.Int("tts-volume")
	internalPaths := c.StringSlice("internal-paths")
	migrationEnabled := c.Bool("migration-enabled")
	migrationDryRun := c.Bool("migration-dry-run")
	stockholmDir := c.String("stockholm-dir")
	stockholmBasePath := c.String("stockholm-base-path")

	return serviceConfig{
		port:                port,
		bindAddr:            bindAddr,
		addr:                addr,
		dataDir:             dataDir,
		hostname:            hostname,
		serverURL:           serverURL,
		httpsServerURL:      httpsServerURL,
		httpsAddr:           httpsAddr,
		redact:              redact,
		logBody:             logBody,
		record:              record,
		dnsEnabled:          dnsEnabled,
		dnsUpstream:         dnsUpstream,
		dnsBind:             dnsBind,
		internalPaths:       internalPaths,
		tlsExtraHosts:       tlsExtraHosts,
		discoveryEnabled:    discoveryEnabled,
		discoveryInterval:   discoveryInterval,
		domains:             domains,
		spotifyClientID:     spotifyClientID,
		spotifyClientSecret: spotifyClientSecret,
		spotifyRedirectURI:  spotifyRedirectURI,
		spotifyTokenURL:     spotifyTokenURL,
		spotifyAPIBase:      spotifyAPIBase,
		amazonClientID:      amazonClientID,
		amazonClientSecret:  amazonClientSecret,
		amazonRedirectURI:   amazonRedirectURI,
		amazonTokenURL:      amazonTokenURL,
		amazonProfileURL:    amazonProfileURL,
		tuneInOpmlURL:       tuneInOpmlURL,
		tuneInAPIURL:        tuneInAPIURL,
		mgmtUsername:        mgmtUsername,
		mgmtPassword:        mgmtPassword,
		ttsProvider:         ttsProvider,
		ttsGoogleAPIKey:     ttsGoogleAPIKey,
		ttsGoogleEndpoint:   ttsGoogleEndpoint,
		ttsLanguage:         ttsLanguage,
		ttsVoice:            ttsVoice,
		ttsAppKey:           ttsAppKey,
		ttsVolume:           ttsVolume,
		migrationEnabled:    migrationEnabled,
		migrationDryRun:     migrationDryRun,
		stockholmDir:        stockholmDir,
		stockholmBasePath:   stockholmBasePath,
	}
}

func getDomains(serverURL, httpsServerURL, hostname string, extraHosts []string) []string {
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
		"downloads.bose.com":      true,
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

	// The speaker firmware constructs the OAuth host by appending `oauth`
	// to the first label of the streaming hostname (see issue #337 and
	// pkg/discovery/dns.go DeriveOAuthHostnames). The DNS hijack catches
	// it; the TLS cert must also cover it, otherwise the speaker rejects
	// the handshake and Spotify / Amazon Music OAuth dies before reaching
	// AfterTouch. Derive once from each of serverURL and httpsServerURL —
	// they typically share a hostname but a multi-homed deployment may
	// differ.
	for _, h := range discovery.DeriveOAuthHostnames(serverURL) {
		domainsMap[h] = true
	}

	for _, h := range discovery.DeriveOAuthHostnames(httpsServerURL) {
		domainsMap[h] = true
	}

	// Explicit overrides / additions for multi-homed hosts, reverse proxies,
	// or browsing the admin UI via a LAN IP that isn't part of serverURL.
	for _, h := range extraHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			domainsMap[h] = true
		}
	}

	domains := make([]string, 0, len(domainsMap))
	for d := range domainsMap {
		domains = append(domains, d)
	}

	return domains
}

// settingsFileExists reports whether a settings.json is already present in the
// data dir. It's the first-run discriminator: an existing file (even an
// incomplete, hand-authored one) must never be overwritten by the default
// seed, while a truly empty data dir gets defaults plus the lost-volume notice.
func settingsFileExists(dataDir string) bool {
	if dataDir == "" {
		return false
	}

	_, err := os.Stat(filepath.Join(dataDir, "settings.json"))

	return err == nil
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
		config.serverURL = handlers.NormalizeServerURL(persisted.ServerURL)
	}

	if persisted.HTTPServerURL != "" {
		config.httpsServerURL = persisted.HTTPServerURL
	}

	config.discoveryEnabled = persisted.DiscoveryEnabled
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

	config.internalPaths = persisted.InternalPaths

	// CLI/env args take precedence; only apply persisted credentials when not set via CLI.
	applyPersistedMusicServiceCredentials(config, persisted)

	config.tlsExtraHosts = mergeTLSExtraHosts(config.tlsExtraHosts, persisted.TLSExtraHosts)

	return persisted
}

// mergeTLSExtraHosts merges the CLI/env-supplied hosts with the persisted
// list. CLI/env wins (so an operator who pinned a host via systemd unit
// always sees it applied); persisted values are additive. Returns a
// deduplicated, order-preserving slice with CLI/env entries first.
func mergeTLSExtraHosts(cli, persisted []string) []string {
	seen := make(map[string]bool, len(cli)+len(persisted))
	out := make([]string, 0, len(cli)+len(persisted))

	for _, h := range cli {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			continue
		}

		seen[h] = true

		out = append(out, h)
	}

	for _, h := range persisted {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			continue
		}

		seen[h] = true

		out = append(out, h)
	}

	return out
}

// applyPersistedMusicServiceCredentials fills in music service credentials from persisted
// settings when they have not been supplied via CLI flags or environment variables.
func applyPersistedMusicServiceCredentials(config *serviceConfig, persisted datastore.Settings) {
	if config.spotifyClientID == "" {
		config.spotifyClientID = persisted.SpotifyClientID
	}

	if config.spotifyClientSecret == "" {
		config.spotifyClientSecret = persisted.SpotifyClientSecret
	}

	if config.spotifyRedirectURI == "" {
		config.spotifyRedirectURI = persisted.SpotifyRedirectURI
	}

	if config.amazonClientID == "" {
		config.amazonClientID = persisted.AmazonClientID
	}

	if config.amazonClientSecret == "" {
		config.amazonClientSecret = persisted.AmazonClientSecret
	}

	if config.amazonRedirectURI == "" {
		config.amazonRedirectURI = persisted.AmazonRedirectURI
	}

	if config.ttsProvider == "" {
		config.ttsProvider = persisted.TTSProvider
	}

	if config.ttsGoogleAPIKey == "" {
		config.ttsGoogleAPIKey = persisted.TTSGoogleAPIKey
	}

	if config.ttsAppKey == "" {
		config.ttsAppKey = persisted.TTSAppKey
	}

	if config.ttsLanguage == "" {
		config.ttsLanguage = persisted.TTSLanguage
	}

	if config.ttsVoice == "" {
		config.ttsVoice = persisted.TTSVoice
	}

	if config.ttsVolume == 0 {
		config.ttsVolume = persisted.TTSVolume
	}
}

func createDefaultSettings(ds *datastore.DataStore, config serviceConfig) datastore.Settings {
	settings := datastore.Settings{
		ServerURL:          config.serverURL,
		HTTPServerURL:      config.httpsServerURL,
		RedactLogs:         config.redact,
		LogBodies:          config.logBody,
		RecordInteractions: config.record,
		DiscoveryEnabled:   config.discoveryEnabled,
		DiscoveryInterval:  config.discoveryInterval.String(),
		DNSEnabled:         config.dnsEnabled,
		DNSUpstream:        strings.Split(config.dnsUpstream, ","),
		DNSBindAddr:        config.dnsBind,
		InternalPaths:      config.internalPaths,
		Shortcuts: map[string]int{
			"/.well-known/appspecific/com.chrome.devtools.json": http.StatusNotFound,
			"/sw.js": http.StatusNotFound,
		},
	}

	_ = ds.SaveSettings(settings)

	return settings
}

func initDataStore(dataDir string) *datastore.DataStore {
	warnIfDataDirNotWritable(dataDir)

	ds := datastore.NewDataStore(dataDir)
	if err := ds.Initialize(); err != nil {
		log.Printf("Warning: Failed to initialize datastore: %v", err)
	}

	return ds
}

// warnIfDataDirNotWritable probes the data dir and logs an actionable message
// when the process can't write to it. The common cause is running the
// container as non-root (uid 65532) while a bind-mounted host directory is
// owned by someone else; without this the failure would surface later as a
// cryptic permission error deep in a save. It only warns: the datastore's own
// resilience handles the degraded state.
func warnIfDataDirNotWritable(dataDir string) {
	if dataDir == "" {
		return
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Printf("WARNING: data dir %s cannot be created: %v", sanitizeLog(dataDir), err)
		logDataDirChownHint(dataDir)

		return
	}

	probe := filepath.Join(dataDir, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		log.Printf("WARNING: data dir %s is not writable: %v", sanitizeLog(dataDir), err)
		logDataDirChownHint(dataDir)

		return
	}

	_ = os.Remove(probe)
}

// logDataDirChownHint prints the one-time fix for a non-writable bind-mounted
// data dir, using the process's own uid. Skipped where uid is unavailable
// (e.g. Windows), where the hint wouldn't apply.
func logDataDirChownHint(dataDir string) {
	uid := os.Getuid()
	if uid < 0 {
		return
	}

	log.Printf("         The service runs as uid %d. If you bind-mounted a host directory as the data dir, "+
		"make it writable once: chown -R %d:%d %s", uid, uid, uid, sanitizeLog(dataDir))
}

func initCertificateManager(dataDir, hostname string) *certmanager.CertificateManager {
	cm := certmanager.NewCertificateManager(filepath.Join(dataDir, "certs"))

	cm.CommonName = hostname
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

// newEmbeddedWebApp builds the soundtouch-player application for embedding in the
// service router: release metadata from the build vars, the service's public
// ServiceURL (used by Play URL for speaker-fetched stream URLs and shown in the
// UI), a loopback InternalServiceURL for the player's own server-side self-calls
// (the TTS proxy) so they never depend on TLS or the service CA, and device
// state sourced entirely from the service.
//
// The web UI shares the service's discovery rather than running its own (the
// datastore is the single source of truth): ExtraDeviceHosts reads it,
// TriggerDiscovery runs the service sweep on a UI-initiated "discover", and the
// devices-changed hook re-syncs the UI registry whenever the service's
// discovery or a manual add changes the set.
func newEmbeddedWebApp(server *handlers.Server, serverURL, internalURL string, ds *datastore.DataStore) *soundtouchweb.WebApp {
	webApp := soundtouchweb.NewWebApp()
	webApp.Version = version
	webApp.Commit = commit
	webApp.Date = date
	webApp.RepoURL = repoURL
	webApp.ServiceURL = strings.TrimRight(serverURL, "/")

	// The player's own server-side calls (the TTS proxy hits
	// /api/setup/tts/speak) go to the service's loopback HTTP listener, not the
	// public ServiceURL. That avoids the "service doesn't trust its own CA"
	// x509 failure entirely: loopback is plain HTTP, so it needs no CA and
	// works on HTTP and HTTPS deployments alike — and before the CA is even
	// generated. ServiceURL stays the public URL because Play URL bakes it into
	// stream URLs the speaker fetches and the UI displays it.
	webApp.InternalServiceURL = internalURL

	webApp.ExtraDeviceHosts = func() []string {
		devices, listErr := ds.ListAllDevices()
		if listErr != nil {
			log.Printf("web UI: failed to list devices from datastore: %v", listErr)
			return nil
		}

		hosts := make([]string, 0, len(devices))
		for i := range devices {
			if devices[i].IPAddress != "" {
				hosts = append(hosts, devices[i].IPAddress)
			}
		}

		return hosts
	}

	// UI "discover" runs the service's sweep, not a second mDNS stack.
	webApp.TriggerDiscovery = server.DiscoverDevices

	// A removal from the player UI cascades to the datastore (the single
	// source of truth), so the device does not reappear on the next re-sync.
	webApp.RemoveDeviceHook = func(deviceID string) error {
		_, err := server.RemoveDeviceByID(deviceID)
		return err
	}

	// Keep the UI registry live as the service discovers or devices are added.
	server.SetDevicesChangedHook(func() {
		webApp.SeedExtraDevices()
		webApp.BroadcastDeviceList()
	})

	go func() {
		// Project the current device set into the UI; the devices-changed hook
		// and the service's periodic discovery keep it current from here on.
		webApp.SeedExtraDevices()
		webApp.BroadcastDeviceList()
	}()

	return webApp
}

func setupRouter(server *handlers.Server, stockholmHandler *stockholm.Handler, webApp *soundtouchweb.WebApp) *chi.Mux {
	r := chi.NewRouter()

	// CleanPath collapses duplicate slashes ("//bmx/..." -> "/bmx/...") and
	// resolves . / .. before routing. Defensive net for the double-slash
	// playback bug: even if a misconfigured base URL hands a speaker a "//bmx"
	// path, it still reaches the right handler instead of 404ing. Runs first so
	// every downstream middleware and the recorder see the cleaned path.
	r.Use(middleware.CleanPath)

	// ClientIPMiddleware must run before any handler that reads the client IP —
	// SnapshotMiddleware captures the request, and several handlers
	// (HandleMargePowerOn, etc.) inspect the source IP via middleware.GetClientIP.
	// Always wired: at minimum the socket peer is recorded; when
	// TrustForwardedHeaders is on and the peer is trusted, XFF is resolved.
	r.Use(server.ClientIPMiddleware())

	r.Use(server.SnapshotMiddleware)
	r.Use(server.OriginMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(server.PeerObserverMiddleware)
	r.Use(server.ShortcutMiddleware)
	r.Use(server.RecordMiddleware)

	r.Get("/", server.HandleRoot)
	r.Get("/admin", server.HandleAdmin)
	r.Get("/health", server.HandleHealth)
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		// The favicon lives in the embedded web/img bundle, not under
		// static/media — HandleMedia would 404. HandleWeb serves from
		// webFS at its native path.
		r.URL.Path = "/web/img/favicon-braille.svg"
		server.HandleWeb()(w, r)
	})

	r.Get("/media/aftertouch-ding.wav", server.HandleDing)
	// Synthesized TTS clips (Google Cloud provider). Served before the
	// /media/* wildcard so the {id} param route takes precedence.
	r.Get("/media/tts/{id}", server.HandleTTSMedia)
	r.Get("/media/*", server.HandleMedia())
	r.Get("/bmx-icons/*", server.HandleBmxIcons())
	r.Get("/ced/*", server.HandleCedStatic())
	r.Get("/web/*", server.HandleWeb())
	r.Post("/alexa/certificate", server.HandleAlexaCertificate)
	r.Get("/docs/*", server.HandleDocs)

	r.Route("/bmx", func(r chi.Router) {
		r.Get("/registry/v1/services", server.HandleBMXRegistry)
		r.Get("/registry/v1/servicesAvailability", server.HandleBMXServicesAvailability)

		r.Route("/tunein", func(r chi.Router) {
			// Bare service descriptor (the registry's `self` link for TuneIn).
			r.Get("/", server.HandleTuneInService)
			r.Get("/v1/playback/station/{stationID}", server.HandleTuneInPlayback)
			r.Get("/v1/playback/episodes/{podcastID}", server.HandleTuneInPodcastInfo)
			r.Get("/v1/playback/episode/{podcastID}", server.HandleTuneInPlaybackPodcast)
			r.Post("/v1/token", server.HandleTuneInToken)
			r.Post("/v1/report", server.HandleTuneInReport)
			r.Get("/v1/navigate", server.HandleTuneInNavigate)
			r.Get("/v1/navigate/*", server.HandleTuneInNavigate)
			r.Get("/v1/search", server.HandleTuneInSearch)
			r.Get("/v1/search/next", server.HandleTuneInSearchNext)
			r.Post("/v1/favorite/{stationID}", server.HandleTuneInFavorite)
			r.Delete("/v1/favorite/{stationID}", server.HandleTuneInDeleteFavorite)
		})
	})

	// Orion (LOCAL_INTERNET_RADIO) lives at the top level — the BMX registry
	// advertises baseUrl `{BMX_SERVER}/core02/svc-bmx-adapter-orion/prod/orion`
	// (no `/bmx/` prefix; verified against the upstream capture in
	// pkg/service/handlers/static/bmx_services_ustream.json), so speakers
	// reach the token + station endpoints at exactly these paths under
	// either DNS-interception or URL-flip migration.
	r.Get("/core02/svc-bmx-adapter-orion/prod/orion", server.HandleOrionService)
	r.Post("/core02/svc-bmx-adapter-orion/prod/orion/token", server.HandleOrionToken)
	r.Get("/core02/svc-bmx-adapter-orion/prod/orion/station", server.HandleOrionPlayback)

	// SiriusXM lives at the top level by the same convention. bmx_services.json
	// advertises baseUrl `{BMX_SERVER}/core02/svc-bmx-adapter-siriusxm-everest-eco1/prod/live-adapter`
	// (no /bmx/ prefix), so speakers reach this exact path under either
	// migration mode. The bare path returns the service descriptor (matches
	// soundcork main.py:805); sub-paths advertised by the descriptor's _links
	// (/availability, /navigate, /token, /logout) currently log + 404 so
	// future implementation work has visibility into real speaker calls.
	r.HandleFunc("/core02/svc-bmx-adapter-siriusxm-everest-eco1/prod/live-adapter", server.HandleSiriusXMLiveAdapter)
	r.HandleFunc("/core02/svc-bmx-adapter-siriusxm-everest-eco1/prod/live-adapter/*", server.HandleSiriusXMLiveAdapterSubpath)

	r.Get("/custom/v1/playback/{encodedURL}", server.HandleCustomPlayback)

	r.Route("/streaming", func(r chi.Router) {
		r.Get("/sourceproviders", server.HandleMargeSourceProviders)
		r.Post("/account", server.HandleMargeCreateAccount)
		r.Post("/account/login", server.HandleMargeLogin)
		r.Post("/account/{account}/source", server.HandleMargeAddSource)
		r.Delete("/account/{account}/source/{sourceID}", server.HandleMargeDeleteSource)

		r.Route("/account/{account}", func(r chi.Router) {
			r.Get("/emailaddress", server.HandleMargeGetEmailAddress)
			r.Get("/full", server.HandleMargeAccountFull)
			r.Get("/sources", server.HandleMargeAccountSources)
			r.Get("/devices", server.HandleMargeAccountDevices)
			r.Get("/presets", server.HandleMargeAccountPresets)
			r.Get("/presets/all", server.HandleMargeAccountPresets)
			r.Get("/provider_settings", server.HandleMargeProviderSettings)

			// All `/device` routes share one chi subrouter. Two
			// overlapping subrouters (`/device` + `/device/{device}`)
			// caused chi's radix-tree resolver to bind a runtime
			// request to the more-specific prefix even when only the
			// less-specific subrouter had a matching method handler,
			// producing the [UNHANDLED] → upstream-proxy fall-through
			// behind issue #285's first-attempted fix. One subrouter
			// keeps every device-scoped path resolvable; see
			// TestPUTRenameRoutesToLocalHandler for the regression
			// against the production router.
			r.Route("/device", func(r chi.Router) {
				r.Post("/", server.HandleMargeAddDevice)
				r.Post("/{device}", server.HandleMargeAddDevice)
				// PUT is the rename / update path — speakers fire
				// this against PUT /streaming/account/{a}/device/{d}
				// when the user renames via Bose App or
				// `soundtouch-cli name set`. Issue #285.
				r.Put("/{device}", server.HandleMargeUpdateDevice)
				r.Delete("/{device}", server.HandleMargeRemoveDevice)

				r.Get("/{device}/presets", server.HandleMargePresets)
				r.Post("/{device}/presets/{presetNumber}", server.HandleMargeUpdatePreset)
				r.Put("/{device}/preset/{presetNumber}", server.HandleMargeUpdatePreset)
				r.Delete("/{device}/preset/{presetNumber}", server.HandleMargeRemovePreset)
				r.Get("/{device}/recent", server.HandleMargeRecents)
				r.Get("/{device}/recents", server.HandleMargeRecents)
				r.Post("/{device}/recent", server.HandleMargeAddRecent)

				r.Get("/{device}/group", server.HandleMargeDeviceGroup)
				r.Get("/{device}/group/", server.HandleMargeDeviceGroup)
				r.Get("/{device}/group/server", server.HandleMargeDeviceGroupServer)
				r.Get("/{device}/group/member", server.HandleMargeDeviceGroupMember)
			})

			// Speakers POST to /group/ (with trailing slash) when forwarding
			// the addGroup payload to Marge during stereo-pair formation --
			// see issue #252. Register both forms so chi accepts either.
			// Speakers POST to /group/ (with trailing slash) when forwarding
			// the addGroup payload to Marge during stereo-pair formation --
			// see issue #252. Register both forms so chi accepts either.
			r.Post("/group", server.HandleMargeAddGroup)
			r.Post("/group/", server.HandleMargeAddGroup)
			r.Post("/group/{groupId}", server.HandleMargeModifyGroup)
			r.Delete("/group/{groupId}", server.HandleMargeDeleteGroup)
			// Speakers send DELETE /group/ (no group ID, trailing slash) during
			// stereo-pair teardown; master and slave use their own account IDs
			// so each deletes its own copy.
			r.Delete("/group", server.HandleMargeDeleteAccountGroups)
			r.Delete("/group/", server.HandleMargeDeleteAccountGroups)
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

		r.Route("/music", func(r chi.Router) {
			r.Route("/musicprovider/{providerID}", func(r chi.Router) {
				r.Post("/is_eligible", server.HandleMusicProviderIsEligible)
				r.Post("/trial/is_eligible", server.HandleMusicProviderIsEligible)
			})
		})

		r.Get("/resources/api_versions.xml", server.HandleMargeAPIVersions)
	})

	// The /accounts/* group mirrored /streaming/account/* for compatibility, but
	// no speaker or app was ever observed using this prefix in the recording
	// corpus (the integration tests that exercised it were migrated onto the
	// /streaming equivalents). The whole mirror is therefore treated as unused
	// and stubbed (HandleUnsupported): it logs + 501s so any real-world use
	// surfaces instead of being silently dropped, leaving the prefix a clean
	// removal candidate for the #451 refactor.
	r.Route("/accounts", func(r chi.Router) {
		r.Route("/{account}", func(r chi.Router) {
			r.Get("/full", server.HandleUnsupported)
			r.Get("/sources", server.HandleUnsupported)
			r.Get("/devices", server.HandleUnsupported)

			r.Post("/devices", server.HandleUnsupported)

			r.Delete("/devices/{device}", server.HandleUnsupported)
			r.Get("/devices/{device}/group", server.HandleUnsupported)
			r.Get("/devices/{device}/group/", server.HandleUnsupported)
			r.Get("/devices/{device}/group/server", server.HandleUnsupported)
			r.Get("/devices/{device}/group/member", server.HandleUnsupported)

			r.Post("/group", server.HandleUnsupported)
			r.Post("/group/", server.HandleUnsupported)
			r.Post("/group/{groupId}", server.HandleUnsupported)
			r.Delete("/group/{groupId}", server.HandleUnsupported)
			r.Delete("/group", server.HandleUnsupported)
			r.Delete("/group/", server.HandleUnsupported)
			r.Get("/devices/{device}/presets", server.HandleUnsupported)
			r.Get("/devices/{device}/recents", server.HandleUnsupported)

			r.Post("/devices/{device}/presets/{presetNumber}", server.HandleUnsupported)
			r.Post("/devices/{device}/recents", server.HandleUnsupported)
		})
	})

	r.Get("/updates/soundtouch", server.HandleMargeSoftwareUpdate)

	r.Route("/customer", func(r chi.Router) {
		r.Get("/account/{account}", server.HandleMargeAccountProfile)
		r.Post("/account/{account}", server.HandleMargeUpdateAccountProfile)
		r.Post("/account/{account}/password", server.HandleMargeChangePassword)
	})

	r.Route("/oauth", func(r chi.Router) {
		r.Post("/device/{deviceID}/music/musicprovider/{sourceID}/token", server.HandleBoseLegacyToken)
		r.Post("/account/{account}/music/musicprovider/{sourceID}/token/cs", server.HandleBoseAccountToken)
		r.Post("/device/{deviceID}/music/musicprovider/{sourceID}/token/cs1", server.HandleBoseToken)
		r.Post("/device/{deviceID}/music/musicprovider/{sourceID}/token/cs3", server.HandleBoseToken)
	})

	r.Route("/v1", func(r chi.Router) {
		r.Post("/stapp/{deviceId}", server.HandleAppEvents)
		r.Post("/scmudc/{deviceId}", server.HandleAppEvents)
		// Return 405 Method Not Allowed as the upstream behavior also returns 405
		r.Get("/blacklist/{deviceId}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
		})
		// app_key validation for the /speaker notification endpoint. Real Bose
		// validated the app_key against its cloud; as the cloud replacement we
		// accept it (200). A 404 here makes the speaker report "invalid app key"
		// (HandleInvalidAppKeyCb) and refuse TTS/URL notifications.
		// When an active DNS-path probe is running (POST /setup/health/dns-path-probe),
		// a matching probe nonce returns 403 instead so no audio plays.
		r.Get("/auth", server.HandleSpeakerAuth)
	})

	// Management API (admin tier). Registered under both /mgmt (legacy) and
	// /api/mgmt (new canonical — issue #451 route-transition step 1) from one
	// shared registration so the two paths stay byte-identical; both carry the
	// same Basic Auth. The browser OAuth callbacks are externally-pinned
	// (provider redirect URIs) and therefore stay at /mgmt only, not aliased.
	mountMgmtAuthed := func(r chi.Router) {
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

		r.Route("/amazon", func(r chi.Router) {
			r.Post("/init", server.HandleMgmtAmazonInit)
			r.Post("/confirm", server.HandleMgmtAmazonConfirm)
			r.Get("/accounts", server.HandleMgmtAmazonAccounts)
			r.Get("/token", server.HandleMgmtAmazonToken)
			r.Post("/prime", server.HandleMgmtPrimeDeviceAmazon)
		})

		r.Get("/devices/{deviceId}/events", server.HandleMgmtDeviceEvents)
	}

	r.Route("/mgmt", func(r chi.Router) {
		// Browser OAuth callbacks — no auth required (provider redirects the
		// user's browser here directly). The authorization code is single-use,
		// short-lived, and useless without the client_secret. Not aliased under
		// /api/mgmt (externally-pinned redirect URIs).
		r.Get("/spotify/callback", server.HandleMgmtSpotifyCallback)
		r.Get("/amazon/callback", server.HandleMgmtAmazonCallback)

		// All other management endpoints require Basic Auth. On the legacy mount
		// they also carry the deprecation signal (counts + one-time warning); the
		// callbacks above are excluded (externally-pinned, not deprecated).
		r.Group(func(r chi.Router) {
			r.Use(server.BasicAuthMgmt())
			r.Use(server.DeprecatedRouteMiddleware)
			mountMgmtAuthed(r)
		})
	})

	r.Route("/api/mgmt", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(server.BasicAuthMgmt())
			mountMgmtAuthed(r)
		})
	})

	// Setup / admin API (admin tier). Registered under both /setup (legacy) and
	// /api/setup (new canonical) from one shared registration. The Stockholm
	// setup-wizard static catch-all is a frontend concern and stays under /setup
	// only — /api/setup serves data only.
	mountSetupAPI := func(r chi.Router) {
		r.Get("/devices", server.HandleListDiscoveredDevices)
		r.Post("/devices", server.HandleAddManualDevice)
		r.Delete("/devices/{deviceId}", server.HandleRemoveDevice)
		r.Post("/discover", server.HandleTriggerDiscovery)
		r.Get("/discovery-status", server.HandleGetDiscoveryStatus)
		r.Get("/settings", server.HandleGetSettings)
		r.Post("/settings", server.HandleUpdateSettings)
		// TTS lives under /setup (LAN-trust, like the rest of the integration
		// surface and Play URL), not /mgmt: the API key is already configured
		// via /setup/settings, and -web/CLI reach this without mgmt credentials.
		r.Post("/tts/speak", server.HandleTTSSpeak)
		r.Get("/tts/config", server.HandleTTSConfig)
		r.Get("/info/{deviceId}", server.HandleGetDeviceInfo)
		r.Get("/summary/{deviceId}", server.HandleGetMigrationSummary)
		r.Post("/migrate/{deviceId}", server.HandleMigrateDevice)
		r.Post("/revert/{deviceId}", server.HandleRevertMigration)
		r.Post("/reboot/{deviceId}", server.HandleRebootDevice)
		r.Get("/account-id-suggestions/{deviceId}", server.HandleAccountIDSuggestions)
		r.Post("/pair-account/{deviceId}", server.HandlePairAccount)
		// Passive peer-reachability probe. Registers a device IP with the
		// in-process observer, nudges :8090/swUpdateCheck, and waits for any
		// inbound from that IP. Used post-migration where the daemon caches its
		// swUpdateUrl at boot and the active round-trip can't reach it without a
		// reboot.
		r.Post("/peer-probe/{deviceId}", server.HandlePeerProbe)
		r.Post("/trust-ca/{deviceId}", server.HandleTrustCACert)
		r.Post("/ensure-remote-services/{deviceId}", server.HandleEnsureRemoteServices)
		r.Post("/remove-remote-services/{deviceId}", server.HandleRemoveRemoteServices)
		r.Post("/backup/{deviceId}", server.HandleBackupConfig)
		r.Post("/sync/{deviceId}", server.HandleInitialSync)
		r.Post("/test-connection/{deviceId}", server.HandleTestConnection)
		r.Post("/test-hosts/{deviceId}", server.HandleTestHostsRedirection)
		r.Post("/test-dns/{deviceId}", server.HandleTestDNSRedirection)
		r.Get("/ca.crt", server.HandleGetCACert)
		r.Get("/logging-settings", server.HandleGetLoggingSettings)
		r.Post("/logging-settings", server.HandleUpdateLoggingSettings)
		r.Get("/version", server.HandleGetVersionInfo)
		r.Get("/interaction-stats", server.HandleGetInteractionStats)
		r.Get("/interactions", server.HandleListInteractions)
		r.Get("/interaction-content", server.HandleGetInteractionContent)
		r.Get("/interactions/sessions/{session}/download", server.HandleDownloadSession)
		r.Delete("/interactions/sessions/{session}", server.HandleDeleteSession)
		r.Delete("/interactions/sessions", server.HandleCleanupSessions)

		r.Get("/dns-discoveries", server.HandleGetDNSDiscoveries)
		r.Get("/dns-discoveries/download", server.HandleDownloadDNSDiscoveries)
		r.Delete("/dns-discoveries", server.HandleClearDNSDiscoveries)
		r.Delete("/sources/{account}/{device}/{sourceID}", server.HandleDeleteSource)

		r.Get("/devices/{deviceId}/events", server.HandleGetDeviceEvents)
		r.Get("/device-summary/{deviceId}", server.HandleDeviceSummary)

		r.Get("/health", server.HandleHealthChecks)
		r.Post("/health/fix", server.HandleHealthFix)
		r.Post("/health/dns-path-probe", server.HandleDNSPathProbe)
		r.Get("/export/diagnostic", server.HandleExportDiagnostic)
		r.Get("/logs", server.HandleGetLogs)
	}

	r.Route("/setup", func(r chi.Router) {
		// Legacy admin API: same handlers as /api/setup, plus the deprecation
		// signal (counts + one-time warning). Scoped to the API routes only — the
		// Stockholm wizard catch-all below is frontend, not a deprecated API path.
		r.Group(func(r chi.Router) {
			r.Use(server.DeprecatedRouteMiddleware)
			mountSetupAPI(r)
		})

		// Serve Stockholm setup wizard pages for paths not matched by the
		// management API. The Stockholm frontend has a setup/ directory that must
		// be accessible at /setup/*. Frontend-only — not mirrored under /api/setup.
		if stockholmHandler != nil {
			r.Get("/*", stockholmHandler.HandleStatic)
			r.Get("/", stockholmHandler.HandleStatic)
		}
	})

	r.Route("/api/setup", func(r chi.Router) {
		mountSetupAPI(r)
	})

	// Embedded web UI: control API under /api/control and the SPA under /app
	// (LAN-trust, like /setup). Additive — nothing here collides with the
	// service's own /, /health, or /static. The web app shares the service's
	// discovery (nil discovery service here), so it runs no mDNS of its own.
	// Skipped when nil, e.g. unit tests that only exercise the service surface.
	if webApp != nil {
		webApp.MountWeb(r, nil)
	}

	if stockholmHandler != nil {
		stockholmHandler.Mount(r)
	}

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

		log.Printf("[TLS] ❌ No certificate available for %s", sanitizeLog(clientHello.ServerName))

		return nil, fmt.Errorf("no certificate available for %s", clientHello.ServerName)
	}

	httpsServer := &http.Server{
		Addr:      httpsAddr,
		Handler:   r,
		TLSConfig: tlsConfig,
		ErrorLog:  log.Default(), // Ensure error logging is enabled
	}

	go func() {
		listener, err := net.Listen("tcp", httpsAddr)
		if err != nil {
			log.Printf("[TLS] Failed to create listener: %v", err)
			return
		}

		log.Printf("Go service listening HTTPS on %s (server URL: %s)",
			listener.Addr().String(), sanitizeLog(httpsServerURL))

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

// runHTTPSPreflight checks whether speakers' implicit :443 target reaches
// AfterTouch. Runs after the HTTPS listener has had a moment to come up; if
// the listener is already on :443 the check is skipped. Emits a single WARN
// log line with actionable guidance when either probe fails.
//
// Only runs when dnsEnabled is true: the :443 reachability only matters when
// speakers are reaching AfterTouch via intercepted Bose hostnames (i.e. the
// DNS migration method). For direct SDK-override migration the speaker
// connects to the configured https-port directly, so :443 is irrelevant.
// Users with external DNS interception (Pi-hole, router rules) can still see
// the live result on /setup/settings even when this startup warn is silent.
func runHTTPSPreflight(httpsServerURL, serverURL string, dnsEnabled bool, resolver func(string) (string, error)) {
	if !dnsEnabled {
		return
	}

	port := handlers.PortFromHTTPSServerURL(httpsServerURL)
	if port == 0 {
		// Can't determine the listener port — be silent rather than misleading.
		return
	}

	// Give the listener a head start so a successful bind beats the probe.
	time.Sleep(2 * time.Second)

	res := handlers.Check443Reachability(port, serverURL, resolver, handlers.ProbeDialTimeoutStartup)

	guidance := handlers.FormatPreflightGuidance(port, res)
	if guidance == "" {
		switch {
		case res.Skipped:
			// Listener already on :443 — nothing to say.
		case res.NotApplicable:
			log.Printf("HTTPS pre-flight: :443 check skipped — %s", sanitizeLog(res.Reason))
		default:
			log.Printf("HTTPS pre-flight: :443 reachable at localhost and %s ✓", sanitizeLog(res.LANHost))
		}

		return
	}

	log.Print(guidance)
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
				log.Printf("[TLS] ❌ Handshake failed from %s: %v", sanitizeLog(c.addr.String()), err)
			}
		}
	}

	return n, err
}
