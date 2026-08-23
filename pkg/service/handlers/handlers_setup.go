package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fmt"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/health"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/go-chi/chi/v5"
)

// HandleListDiscoveredDevices returns a list of all discovered devices.
func (s *Server) HandleListDiscoveredDevices(w http.ResponseWriter, _ *http.Request) {
	devices, err := s.ds.ListAllDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(devices); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleAddManualDevice adds a device manually by IP.
func (s *Server) HandleAddManualDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if body.IP == "" {
		http.Error(w, "IP address is required", http.StatusBadRequest)
		return
	}

	// Try to get live info
	liveInfo, err := s.sm.GetLiveDeviceInfo(body.IP)
	if err != nil {
		// Even if we can't get live info, we might want to add it?
		// But usually we need at least the serial for proper account management.
		http.Error(w, "Failed to reach device at "+body.IP+": "+err.Error(), http.StatusBadGateway)
		return
	}

	// Reuse handleDiscoveredDevice logic via a fake models.DiscoveredDevice
	d := models.DiscoveredDevice{
		Name:            liveInfo.Name,
		Host:            body.IP,
		ModelID:         liveInfo.Type,
		SerialNo:        liveInfo.SerialNumber,
		DiscoveryMethod: "manual",
	}

	s.handleDiscoveredDevice(d)
	s.mergeOverlappingDevices()

	// Let any observer (e.g. the embedded web UI) re-sync from the datastore.
	s.notifyDevicesChanged()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]bool{"ok": true}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleTriggerDiscovery triggers a new device discovery scan.
func (s *Server) HandleTriggerDiscovery(w http.ResponseWriter, _ *http.Request) {
	//nolint:contextcheck
	go s.DiscoverDevices(context.Background())

	w.WriteHeader(http.StatusAccepted)
}

// HandleGetDiscoveryStatus returns the current discovery status.
func (s *Server) HandleGetDiscoveryStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]bool{"discovering": s.discovering}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// RemoveDeviceByID removes the device with the given device ID (MAC) from
// the datastore, searching across all accounts. It returns whether a
// matching device was found. On a successful removal it notifies
// observers (e.g. the embedded web UI) so they re-sync — the symmetric
// counterpart to the notify in HandleAddManualDevice.
func (s *Server) RemoveDeviceByID(deviceID string) (bool, error) {
	devices, err := s.ds.ListAllDevices()
	if err != nil {
		return false, err
	}

	for i := range devices {
		if devices[i].DeviceID == deviceID {
			if err := s.ds.RemoveDevice(devices[i].AccountID, devices[i].DeviceID); err != nil {
				return false, err
			}

			// Let any observer (e.g. the embedded web UI) re-sync from the
			// datastore so the removal propagates to the player UI.
			s.notifyDevicesChanged()

			return true, nil
		}
	}

	return false, nil
}

// HandleRemoveDevice removes a device from the datastore.
func (s *Server) HandleRemoveDevice(w http.ResponseWriter, r *http.Request) {
	deviceId := chi.URLParam(r, "deviceId")
	if deviceId == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	found, err := s.RemoveDeviceByID(deviceId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !found {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]bool{"ok": true}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetSettings returns the current service settings.
func (s *Server) HandleGetSettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.mu.RLock()
	serverURL, httpsServerURL := s.serverURL, s.httpsServerURL
	httpsOverride := s.httpsOverride
	discoveryInterval := s.discoveryInterval.String()
	discoveryEnabled := s.discoveryEnabled
	// Read the update-check fields directly rather than via
	// GetUpdateCheckSettings(): that getter takes s.mu.RLock itself, and Go's
	// sync.RWMutex is not reentrant-safe against a concurrent writer.
	updateCheckInterval := s.updateCheckInterval.String()
	updateCheckEnabled := s.updateCheckEnabled
	dnsEnabled := s.dnsEnabled
	dnsUpstream := s.dnsUpstream
	dnsBindAddr := s.dnsBindAddr
	internalPaths := s.internalPaths
	redact, logBody, record := s.redactLogs, s.logBodies, s.recordEnabled
	adminAreaAuth := s.adminAreaAuth
	shortcuts := s.shortcuts
	spotifyConfigured := s.spotifyService != nil
	spotifyClientID := s.spotifyClientID
	spotifyClientSecret := s.spotifyClientSecret
	spotifyRedirectURI := s.spotifyRedirectURI
	amazonConfigured := s.amazonService != nil
	amazonClientID := s.amazonClientID
	amazonClientSecret := s.amazonClientSecret
	amazonRedirectURI := s.amazonRedirectURI
	ttsConfigured := s.ttsConfigured()
	ttsProvider := s.ttsProvider
	ttsGoogleAPIKey := s.ttsGoogleAPIKey
	ttsAppKey := s.ttsAppKey
	ttsLanguage := s.ttsLanguage
	ttsVoice := s.ttsVoice
	ttsVolume := s.ttsVolume
	s.mu.RUnlock()

	dnsRunning, actualBind := s.GetDNSRunning()

	defaultLanding := s.defaultLanding()

	var serverURLResolvedIP, serverURLResolveError string

	if ip, err := s.resolveServerURLIP(serverURL); err == nil {
		serverURLResolvedIP = ip
	} else {
		serverURLResolveError = err.Error()
	}

	httpsListenerPort := PortFromHTTPSServerURL(httpsServerURL)
	probe443 := Check443Reachability(httpsListenerPort, serverURL, s.resolveServerURLIP, ProbeDialTimeoutInline)

	// Mask secrets: return "***" if set so the UI can show "configured" without exposing the value.
	if spotifyClientSecret != "" {
		spotifyClientSecret = "***"
	}

	if amazonClientSecret != "" {
		amazonClientSecret = "***"
	}

	// Default the provider for display so the UI shows "translate" rather than
	// an empty selection when nothing has been configured yet.
	if ttsProvider == "" {
		ttsProvider = "translate"
	}

	if ttsGoogleAPIKey != "" {
		ttsGoogleAPIKey = "***"
	}

	if ttsAppKey != "" {
		ttsAppKey = "***"
	}

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"server_url":                    serverURL,
		"server_url_resolved_ip":        serverURLResolvedIP,
		"server_url_resolve_error":      serverURLResolveError,
		"https_server_url":              httpsServerURL,
		"https_server_url_override":     httpsOverride,
		"https_listener_port":           httpsListenerPort,
		"https_443_check_skipped":       probe443.Skipped,
		"https_443_not_applicable":      probe443.NotApplicable,
		"https_443_reason":              probe443.Reason,
		"tls_extra_hosts":               s.persistedTLSExtraHosts(),
		"tls_san_hosts":                 s.ExpectedHosts(),
		"https_443_localhost_reachable": probe443.Localhost.Reachable,
		"https_443_localhost_error":     probe443.Localhost.Error,
		"https_443_lan_reachable":       probe443.LAN.Reachable,
		"https_443_lan_error":           probe443.LAN.Error,
		"https_443_lan_host":            probe443.LANHost,
		"discovery_interval":            discoveryInterval,
		"discovery_enabled":             discoveryEnabled,
		"update_check_interval":         updateCheckInterval,
		"update_check_enabled":          updateCheckEnabled,
		"dns_enabled":                   dnsEnabled,
		"dns_running":                   dnsRunning,
		"dns_actual_bind":               actualBind,
		"dns_upstream":                  strings.Join(dnsUpstream, ","),
		"dns_bind_addr":                 dnsBindAddr,
		"internal_paths":                internalPaths,
		"redact_logs":                   redact,
		"log_bodies":                    logBody,
		"record_interactions":           record,
		"shortcuts":                     shortcuts,
		"spotify_configured":            spotifyConfigured,
		"spotify_client_id":             spotifyClientID,
		"spotify_client_secret":         spotifyClientSecret,
		"spotify_redirect_uri":          spotifyRedirectURI,
		"amazon_configured":             amazonConfigured,
		"amazon_client_id":              amazonClientID,
		"amazon_client_secret":          amazonClientSecret,
		"amazon_redirect_uri":           amazonRedirectURI,
		"tts_configured":                ttsConfigured,
		"tts_provider":                  ttsProvider,
		"tts_google_api_key":            ttsGoogleAPIKey,
		"tts_app_key":                   ttsAppKey,
		"tts_language":                  ttsLanguage,
		"tts_voice":                     ttsVoice,
		"tts_volume":                    ttsVolume,
		"default_landing":               defaultLanding,
		"admin_area_auth":               adminAreaAuth,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// parseDNSUpstreamList splits a comma-separated DNS upstream list into its
// trimmed, non-empty entries. Returns nil for an empty input.
func parseDNSUpstreamList(dnsUpstream string) []string {
	if dnsUpstream == "" {
		return nil
	}

	var upstreamList []string

	for _, u := range strings.Split(dnsUpstream, ",") {
		u = strings.TrimSpace(u)
		if u != "" {
			upstreamList = append(upstreamList, u)
		}
	}

	return upstreamList
}

// parseOptionalDuration parses a duration string that the client is allowed to
// omit. An empty value yields a zero duration and no error, so callers can
// treat "field omitted" as "keep the current value" while still rejecting a
// value that was supplied but is unparseable.
func parseOptionalDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}

	return time.ParseDuration(value)
}

// resolvePeriodicSetting computes the new (interval, enabled) pair for one of
// the background pollers (device discovery, update check) from a settings
// request. When the request omitted the interval, the current one is kept. A
// zero interval always forces the task off: both pollers treat zero as
// "always due", so leaving the task enabled would make their poll tick the
// work rate.
func resolvePeriodicSetting(
	currentInterval, requestedInterval time.Duration,
	requestedIntervalProvided, requestedEnabled bool,
) (time.Duration, bool) {
	interval := currentInterval
	if requestedIntervalProvided {
		interval = requestedInterval
	}

	if interval == 0 {
		return interval, false
	}

	return interval, requestedEnabled
}

// HandleUpdateSettings updates the service settings.
func (s *Server) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings struct {
		ServerURL              string         `json:"server_url"`
		HTTPSServerURLOverride *string        `json:"https_server_url_override"`
		DiscoveryInterval      string         `json:"discovery_interval"`
		DiscoveryEnabled       bool           `json:"discovery_enabled"`
		UpdateCheckInterval    string         `json:"update_check_interval"`
		UpdateCheckEnabled     bool           `json:"update_check_enabled"`
		DNSEnabled             bool           `json:"dns_enabled"`
		DNSUpstream            string         `json:"dns_upstream"`
		DNSBindAddr            string         `json:"dns_bind_addr"`
		InternalPaths          []string       `json:"internal_paths"`
		Shortcuts              map[string]int `json:"shortcuts"`
		SpotifyClientID        string         `json:"spotify_client_id"`
		SpotifyClientSecret    string         `json:"spotify_client_secret"`
		SpotifyRedirectURI     string         `json:"spotify_redirect_uri"`
		AmazonClientID         string         `json:"amazon_client_id"`
		AmazonClientSecret     string         `json:"amazon_client_secret"`
		AmazonRedirectURI      string         `json:"amazon_redirect_uri"`
		TTSProvider            string         `json:"tts_provider"`
		TTSGoogleAPIKey        string         `json:"tts_google_api_key"`
		TTSAppKey              string         `json:"tts_app_key"`
		TTSLanguage            string         `json:"tts_language"`
		TTSVoice               string         `json:"tts_voice"`
		TTSVolume              int            `json:"tts_volume"`
		TLSExtraHosts          *[]string      `json:"tls_extra_hosts"`
		DefaultLanding         string         `json:"default_landing"`
		AdminAreaAuth          string         `json:"admin_area_auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Normalise + validate the landing choice. Empty means "chooser".
	defaultLanding := strings.ToLower(strings.TrimSpace(settings.DefaultLanding))
	switch defaultLanding {
	case "", "chooser", "app", "admin":
	default:
		http.Error(w, "Invalid default_landing: must be chooser, app, or admin", http.StatusBadRequest)
		return
	}

	// Normalise + validate the admin-area auth mode. Empty means "unset"
	// (today: not enforced — see datastore.Settings.AdminAreaAuth, #419).
	adminAreaAuth, validAdminAreaAuth := NormalizeAdminAreaAuth(settings.AdminAreaAuth)
	if !validAdminAreaAuth {
		http.Error(w, "Invalid admin_area_auth: must be empty, enabled, or disabled", http.StatusBadRequest)
		return
	}

	if settings.DNSEnabled && settings.DNSUpstream == "" {
		// No strict requirement for DNSUpstream here as SetDNSSettings will
		// try to fall back to system DNS. We only log it if both are empty later.
		log.Printf("[DNS] DNS Discovery enabled without explicit upstreams, will try system DNS.")
	}

	// Strip a trailing slash before validating/persisting: it would otherwise
	// flow into the BMX registry base and produce "//bmx/..." playback requests
	// the router 404s.
	settings.ServerURL = NormalizeServerURL(settings.ServerURL)

	// Validate server_url: the same value the DNS server uses to derive its
	// intercept IP. Reject anything that does not resolve to a routable IP so
	// users see the error in the UI instead of getting a silently-broken setup
	// where DNS replies with `CNAME .` for every Bose hostname.
	if _, err := s.resolveServerURLIP(settings.ServerURL); err != nil {
		http.Error(w, "Invalid server_url: "+err.Error(), http.StatusBadRequest)
		return
	}

	interval, err := parseOptionalDuration(settings.DiscoveryInterval)
	if err != nil {
		http.Error(w, "Invalid discovery interval: "+err.Error(), http.StatusBadRequest)
		return
	}

	updateCheckInterval, err := parseOptionalDuration(settings.UpdateCheckInterval)
	if err != nil {
		http.Error(w, "Invalid update check interval: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()

	// Guard rail: refuse to enable the admin-area gate while the Management
	// API credentials are still the published default — that would let
	// anyone in with admin/change_me! anyway, just with extra friction.
	if blocksAdminAreaAuthEnable(adminAreaAuth, s.mgmtUsername, s.mgmtPassword) {
		s.mu.Unlock()
		http.Error(w, "Cannot enable admin_area_auth while Management API credentials are still the "+
			"published default (admin/change_me!). Set MGMT_USERNAME and MGMT_PASSWORD to your own "+
			"values first.", http.StatusBadRequest)

		return
	}

	s.adminAreaAuth = adminAreaAuth
	s.serverURL = settings.ServerURL
	// nil override = "field omitted, preserve"; recompute regardless, since
	// the Target Domain (which the derived URL follows) may have changed.
	s.applyHTTPSOverrideLocked(settings.HTTPSServerURLOverride)

	s.discoveryInterval, s.discoveryEnabled = resolvePeriodicSetting(
		s.discoveryInterval, interval, settings.DiscoveryInterval != "", settings.DiscoveryEnabled)

	s.updateCheckInterval, s.updateCheckEnabled = resolvePeriodicSetting(
		s.updateCheckInterval, updateCheckInterval, settings.UpdateCheckInterval != "", settings.UpdateCheckEnabled)

	s.dnsEnabled = settings.DNSEnabled
	s.dnsUpstream = parseDNSUpstreamList(settings.DNSUpstream)
	s.dnsBindAddr = settings.DNSBindAddr

	s.internalPaths = settings.InternalPaths

	if settings.Shortcuts != nil {
		s.shortcuts = settings.Shortcuts
	}

	if s.sm != nil {
		s.sm.ServerURL = settings.ServerURL
	}

	// Update music service credentials (empty or "***" means "unchanged").
	s.applyMusicServiceCredentials(
		settings.SpotifyClientID, settings.SpotifyClientSecret, settings.SpotifyRedirectURI,
		settings.AmazonClientID, settings.AmazonClientSecret, settings.AmazonRedirectURI,
	)

	// Update TTS config (empty/"***" secrets mean "unchanged").
	s.applyTTSConfig(
		settings.TTSProvider, settings.TTSGoogleAPIKey, settings.TTSAppKey,
		settings.TTSLanguage, settings.TTSVoice, settings.TTSVolume,
	)

	// Persist to datastore
	// Access fields directly since we already hold the lock
	currentRedact := s.redactLogs
	currentLogBody := s.logBodies
	currentRecord := s.recordEnabled
	// Persist the override (empty = derive), not the effective URL, so the
	// HTTPS URL keeps following the Target Domain across restarts.
	currentHTTPS := s.httpsOverride

	// Load the persisted settings first and overlay only the fields this
	// handler owns, instead of building a fresh struct from scratch. Fields
	// with no in-memory counterpart on Server (e.g. TrustForwardedHeaders,
	// TrustedProxyCIDRs, TuneInStreamFormats) are only ever set by hand-editing
	// settings.json; overwriting with a fresh struct would silently drop them
	// (issue #589).
	persisted, err := s.ds.GetSettings()
	if err != nil {
		s.mu.Unlock()
		http.Error(w, "Failed to load existing settings: "+err.Error(), http.StatusInternalServerError)

		return
	}

	// Resolve TLS extra hosts: nil pointer means "field omitted, preserve existing";
	// non-nil (even empty) means "replace with this list".
	resolvedTLSExtraHosts := persisted.TLSExtraHosts
	if settings.TLSExtraHosts != nil {
		resolvedTLSExtraHosts = normaliseTLSExtraHosts(*settings.TLSExtraHosts)
	}

	log.Printf("Saving updated settings to %s/settings.json", s.ds.DataDir)
	persisted.ServerURL = s.serverURL
	persisted.HTTPServerURL = currentHTTPS
	persisted.RedactLogs = currentRedact
	persisted.LogBodies = currentLogBody
	persisted.RecordInteractions = currentRecord
	persisted.DiscoveryInterval = s.discoveryInterval.String()
	persisted.DiscoveryEnabled = s.discoveryEnabled
	persisted.UpdateCheckInterval = s.updateCheckInterval.String()
	persisted.UpdateCheckEnabled = s.updateCheckEnabled
	persisted.DNSEnabled = s.dnsEnabled
	persisted.DNSUpstream = s.dnsUpstream
	persisted.DNSBindAddr = s.dnsBindAddr
	persisted.InternalPaths = s.internalPaths
	persisted.Shortcuts = s.shortcuts
	persisted.SpotifyClientID = s.spotifyClientID
	persisted.SpotifyClientSecret = s.spotifyClientSecret
	persisted.SpotifyRedirectURI = s.spotifyRedirectURI
	persisted.AmazonClientID = s.amazonClientID
	persisted.AmazonClientSecret = s.amazonClientSecret
	persisted.AmazonRedirectURI = s.amazonRedirectURI
	persisted.TTSProvider = s.ttsProvider
	persisted.TTSGoogleAPIKey = s.ttsGoogleAPIKey
	persisted.TTSAppKey = s.ttsAppKey
	persisted.TTSLanguage = s.ttsLanguage
	persisted.TTSVoice = s.ttsVoice
	persisted.TTSVolume = s.ttsVolume
	persisted.TLSExtraHosts = resolvedTLSExtraHosts
	persisted.DefaultLanding = defaultLanding
	persisted.AdminAreaAuth = s.adminAreaAuth
	err = s.ds.SaveSettings(persisted)

	dnsEnabled := s.dnsEnabled
	dnsUpstreamStr := strings.Join(s.dnsUpstream, ",")
	dnsBindAddr := s.dnsBindAddr
	reinitSpotify := s.spotifyClientID != ""
	reinitAmazon := s.amazonClientID != ""

	s.mu.Unlock()

	s.SetDNSSettings(dnsEnabled, dnsUpstreamStr, dnsBindAddr)

	if reinitSpotify {
		s.ReinitSpotifyService()
	}

	if reinitAmazon {
		s.ReinitAmazonService()
	}

	// TTS always has a usable provider (translate needs no credentials), so
	// rebuild unconditionally to pick up any provider/key/app-key change.
	s.ReinitTTSService()

	if err != nil {
		http.Error(w, "Failed to save settings: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Settings updated"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// NormalizeAdminAreaAuth trims/lowercases the admin-area auth mode and
// reports whether it's one of the three valid tri-state values ("", the
// unset default; "enabled"; "disabled" — see datastore.Settings.AdminAreaAuth,
// #419). Exported so main.go can apply the same validation to a persisted
// settings.json value at startup that HandleUpdateSettings applies on write.
func NormalizeAdminAreaAuth(v string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(v))

	switch normalized {
	case "", "enabled", "disabled":
		return normalized, true
	default:
		return "", false
	}
}

// blocksAdminAreaAuthEnable reports whether enabling the admin-area gate
// must be refused because the Management API credentials are still the
// published default (admin/change_me!) — enabling it in that state would
// give a false sense of security. Extracted from HandleUpdateSettings to
// keep its cyclomatic complexity in check.
func blocksAdminAreaAuthEnable(mode, mgmtUsername, mgmtPassword string) bool {
	return mode == "enabled" &&
		mgmtUsername == health.DefaultMgmtUsername && mgmtPassword == health.DefaultMgmtPassword
}

// normaliseTLSExtraHosts trims whitespace from each entry, drops empty
// values, and deduplicates while preserving the first occurrence's
// position. The settings endpoint applies this before persisting so the
// stored list is always canonical.
func normaliseTLSExtraHosts(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))

	for _, h := range in {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			continue
		}

		seen[h] = true

		out = append(out, h)
	}

	return out
}

// HandleGetDeviceInfo returns live information for a device.
func (s *Server) HandleGetDeviceInfo(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	info, err := s.sm.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(info); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetMigrationSummary returns a summary of the migration plan for a device.
func (s *Server) HandleGetMigrationSummary(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	targetURL := r.URL.Query().Get("target_url")
	proxyURL := r.URL.Query().Get("proxy_url")

	options := parseMigrationOptions(r.URL.Query())

	summary, err := s.sm.GetMigrationSummary(deviceIP, targetURL, proxyURL, options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(summary); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleMigrateDevice starts the migration process for a device.
func (s *Server) HandleMigrateDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device ID is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	targetURL := r.URL.Query().Get("target_url")
	proxyURL := r.URL.Query().Get("proxy_url")
	method := setup.MigrationMethod(r.URL.Query().Get("method"))

	options := parseMigrationOptions(r.URL.Query())

	output, err := s.sm.MigrateSpeaker(deviceIP, targetURL, proxyURL, options, method)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Migration started", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleRevertMigration reverts the migration for a device.
func (s *Server) HandleRevertMigration(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device ID is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	output, err := s.sm.RevertMigration(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Revert started", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetDNSDiscoveries returns recorded DNS discoveries.
func (s *Server) HandleGetDNSDiscoveries(w http.ResponseWriter, _ *http.Request) {
	result := s.getMergedDNSDiscoveries()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleDownloadDNSDiscoveries returns recorded DNS discoveries as a downloadable JSON file.
func (s *Server) HandleDownloadDNSDiscoveries(w http.ResponseWriter, _ *http.Request) {
	result := s.getMergedDNSDiscoveries()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dns-discoveries.json\"")

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(result); err != nil {
		log.Printf("Error encoding DNS discoveries for download: %v", err)
	}
}

func (s *Server) getMergedDNSDiscoveries() []datastore.DNSDiscoveryEntry {
	// 1. Get current in-memory discoveries
	inMemory := s.GetDNSDiscovery()

	// 2. Load persisted discoveries
	persisted, err := s.ds.LoadDNSDiscoveries()
	if err != nil {
		log.Printf("Warning: Failed to load DNS discoveries: %v", err)
	}

	// 3. Merge them
	merged := make(map[string]datastore.DNSDiscoveryEntry)
	for _, p := range persisted {
		merged[p.Hostname] = p
	}

	for hostname, h := range inMemory {
		m, exists := merged[hostname]
		if !exists || h.LastSeen.After(m.LastSeen) {
			merged[hostname] = datastore.DNSDiscoveryEntry{
				Hostname:      h.Hostname,
				FirstSeen:     h.FirstSeen,
				LastSeen:      h.LastSeen,
				QueryCount:    h.QueryCount,
				IsBoseService: h.IsBoseService,
				IsIntercepted: h.IsIntercepted,
				RemoteAddr:    h.RemoteAddr,
			}
		} else if h.QueryCount > m.QueryCount {
			// If exists and persisted is newer (rare but possible), update query count if higher
			m.QueryCount = h.QueryCount
			merged[hostname] = m
		}
	}

	// Convert to slice
	result := make([]datastore.DNSDiscoveryEntry, 0, len(merged))
	for _, entry := range merged {
		result = append(result, entry)
	}

	// Sort by last seen descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].LastSeen.After(result[j].LastSeen)
	})

	// 4. Update persistence with merged results
	if err := s.ds.SaveDNSDiscoveries(result); err != nil {
		log.Printf("Warning: Failed to persist merged DNS discoveries: %v", err)
	}

	return result
}

// HandleClearDNSDiscoveries clears recorded DNS discoveries.
func (s *Server) HandleClearDNSDiscoveries(w http.ResponseWriter, _ *http.Request) {
	// 1. Clear in-memory
	s.SetDNSDiscoveries(make(map[string]*discovery.DiscoveredHost))

	// 2. Clear persistence
	if err := s.ds.ClearDNSDiscoveries(); err != nil {
		http.Error(w, "Failed to clear DNS discoveries: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]bool{"ok": true}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleTrustCACert injects the local Root CA into the device's shared trust store.
func (s *Server) HandleTrustCACert(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device ID is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	output, err := s.sm.TrustCACert(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Root CA trusted", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleEnsureRemoteServices ensures that remote services are configured on a device.
func (s *Server) HandleEnsureRemoteServices(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device ID is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	output, err := s.sm.EnsureRemoteServices(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Remote services enabled", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleRemoveRemoteServices removes remote services configuration from a device.
func (s *Server) HandleRemoveRemoteServices(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device ID is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	output, err := s.sm.RemoveRemoteServices(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Remote services removed", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleBackupConfig creates a backup of the device configuration.
func (s *Server) HandleBackupConfig(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device ID is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	output, err := s.sm.BackupConfig(deviceIP)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Config backed up", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetLoggingSettings returns the current proxy settings.
func (s *Server) HandleGetLoggingSettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	redact, logBody, record := s.GetLoggingSettings()

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"redact":   redact,
		"log_body": logBody,
		"record":   record,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetCACert returns the Root CA certificate.
func (s *Server) HandleGetCACert(w http.ResponseWriter, _ *http.Request) {
	caCertPath := s.sm.Crypto.GetCACertPath()

	content, err := os.ReadFile(caCertPath)
	if err != nil {
		http.Error(w, "Failed to read CA certificate", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", "attachment; filename=soundtouch-ca.crt")
	_, _ = w.Write(content)
}

// HandleUpdateLoggingSettings updates the proxy settings.
func (s *Server) HandleUpdateLoggingSettings(w http.ResponseWriter, r *http.Request) {
	var settings struct {
		Redact  bool `json:"redact"`
		LogBody bool `json:"log_body"`
		Record  bool `json:"record"`
	}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.redactLogs = settings.Redact
	s.logBodies = settings.LogBody
	s.recordEnabled = settings.Record

	if s.recorder != nil {
		s.recorder.Redact = settings.Redact
	}

	// Persist to datastore
	// Access fields directly since we already hold the lock
	// Persist the HTTPS override (empty = derive), not the effective URL.
	serverURL, httpsOverride := s.serverURL, s.httpsOverride
	discoveryInterval := s.discoveryInterval.String()
	discoveryEnabled := s.discoveryEnabled

	// Load the persisted settings first and overlay only the fields this
	// handler owns, instead of building a fresh struct from scratch. This
	// handler's DTO only ever covers 3 of ~25 fields, so a from-scratch
	// struct used to reset everything else (credentials, DNS config,
	// TrustForwardedHeaders/TrustedProxyCIDRs, ...) to its zero value on
	// every save (issue #589).
	persisted, err := s.ds.GetSettings()
	if err != nil {
		s.mu.Unlock()
		http.Error(w, "Failed to load existing settings: "+err.Error(), http.StatusInternalServerError)

		return
	}

	persisted.ServerURL = serverURL
	persisted.HTTPServerURL = httpsOverride
	persisted.RedactLogs = s.redactLogs
	persisted.LogBodies = s.logBodies
	persisted.RecordInteractions = s.recordEnabled
	persisted.DiscoveryInterval = discoveryInterval
	persisted.DiscoveryEnabled = discoveryEnabled
	persisted.Shortcuts = s.shortcuts

	log.Printf("Saving updated proxy settings to %s/settings.json", s.ds.DataDir)
	err = s.ds.SaveSettings(persisted)
	s.mu.Unlock()

	if err != nil {
		http.Error(w, "Failed to save settings: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Proxy settings updated"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleTestHostsRedirection performs a preliminary check for /etc/hosts redirection.
func (s *Server) HandleTestHostsRedirection(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	targetURL := r.URL.Query().Get("target_url")
	if targetURL == "" {
		targetURL = s.serverURL
	}

	output, err := s.sm.TestHostsRedirection(deviceIP, targetURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Return 200 but ok: false so UI can show the output

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      false,
			"message": err.Error(),
			"output":  output,
		}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Hosts redirection test successful",
		"output":  output,
	}); encodeErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleTestDNSRedirection performs a check for DNS redirection to the AfterTouch service.
func (s *Server) HandleTestDNSRedirection(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	targetURL := r.URL.Query().Get("target_url")
	if targetURL == "" {
		targetURL = s.serverURL
	}

	output, err := s.sm.TestDNSRedirection(deviceIP, targetURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Return 200 but ok: false so UI can show the output

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      false,
			"message": err.Error(),
			"output":  output,
		}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "DNS redirection test successful",
		"output":  output,
	}); encodeErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleInitialSync fetches presets, recents and sources from the device
// and saves them to the datastore.
//
// If applying the fetched presets/recents would shrink what's already
// stored, the sync is not applied — the response comes back 409 with the
// diff describing what would be removed — unless the caller passes
// ?confirmed=true, in which case it's applied unconditionally. Every call
// re-fetches live from the speaker at that moment (see
// setup.SyncDeviceData), so a confirmed retry re-checks current reality
// rather than replaying a possibly-stale earlier response.
func (s *Server) HandleInitialSync(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		http.Error(w, "Missing deviceId", http.StatusBadRequest)
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	confirmed := r.URL.Query().Get("confirmed") == "true"

	result, err := s.sm.SyncDeviceData(deviceIP, confirmed)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !result.Applied {
		w.WriteHeader(http.StatusConflict)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	if encodeErr := json.NewEncoder(w).Encode(result); encodeErr != nil {
		log.Printf("HandleInitialSync: failed to encode result for device %s: %s", sanitizeLog(deviceID), sanitizeErr(encodeErr))
	}
}

// HandleRebootDevice reboots a device.
func (s *Server) HandleRebootDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": "Device ID is required"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error()}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	method := setup.RebootMethod(r.URL.Query().Get("method"))

	output, err := s.sm.Reboot(deviceIP, method)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "message": err.Error(), "output": output}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Reboot started", "output": output}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleTestConnection performs a connection check from the device to the server.
func (s *Server) HandleTestConnection(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		http.Error(w, "Device ID is required", http.StatusBadRequest)
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	targetURL := r.URL.Query().Get("target_url")
	if targetURL == "" {
		http.Error(w, "Target URL is required", http.StatusBadRequest)
		return
	}

	useExplicitCA := r.URL.Query().Get("use_explicit_ca") == "true"

	output, err := s.sm.TestConnection(deviceIP, targetURL, useExplicitCA)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Return 200 but ok: false so UI can show the output

		if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      false,
			"message": err.Error(),
			"output":  output,
		}); encodeErr != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "Connection test successful",
		"output":  output,
	}); encodeErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleGetVersionInfo returns version information for the service.
func (s *Server) HandleGetVersionInfo(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	version := s.Version
	commit := s.Commit
	date := s.Date
	repoURL := s.RepoURL
	s.mu.RUnlock()

	var dataDir string

	if s.ds != nil && s.ds.DataDir != "" {
		// Resolve to absolute: the default ("data") and any relative
		// --data-dir/DATA_DIR value are otherwise ambiguous without knowing
		// the process's working directory at startup.
		if abs, err := filepath.Abs(s.ds.DataDir); err == nil {
			dataDir = abs
		} else {
			dataDir = s.ds.DataDir
		}
	}

	w.Header().Set("Content-Type", "application/json")

	var (
		releaseURL string
		commitURL  string
	)

	if commit != "" && commit != "unknown" {
		commitURL = fmt.Sprintf("%s/commit/%s", repoURL, commit)
	}

	// Release version: should point to the release, e.g. https://github.com/gesellix/Bose-SoundTouch/releases/tag/v0.58.0
	// "dirty" versions don't get a release link (only the commit).
	if version != "" && version != "dev" && version != "(devel)" && !strings.Contains(version, "dirty") {
		releaseURL = fmt.Sprintf("%s/releases/tag/%s", repoURL, version)
	}

	// Opt-in periodic update check (#591) — UpdateCheckResult is nil-safe and
	// returns the zero value (Available: false) when the check was never
	// enabled, which is the common case.
	updateCheck := s.UpdateCheckResult()

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"version":            version,
		"commit":             commit,
		"date":               date,
		"repo_url":           repoURL,
		"release_url":        releaseURL,
		"commit_url":         commitURL,
		"data_dir":           dataDir,
		"update_available":   updateCheck.Available,
		"latest_version":     updateCheck.LatestVersion,
		"latest_release_url": updateCheck.ReleaseURL,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetInteractionStats returns statistics about recorded interactions.
func (s *Server) HandleGetInteractionStats(w http.ResponseWriter, _ *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	stats, err := s.recorder.GetInteractionStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleListInteractions returns a list of recorded interactions.
func (s *Server) HandleListInteractions(w http.ResponseWriter, r *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	session := r.URL.Query().Get("session")
	category := r.URL.Query().Get("category")
	since := r.URL.Query().Get("since")

	interactions, err := s.recorder.ListInteractions(session, category, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(interactions); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleGetInteractionContent returns the raw content of a recorded interaction.
func (s *Server) HandleGetInteractionContent(w http.ResponseWriter, r *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	file := r.URL.Query().Get("file")
	if file == "" {
		http.Error(w, "File parameter is required", http.StatusBadRequest)
		return
	}

	content, err := s.recorder.GetInteractionContent(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write(content)
}

// HandleDeleteSession deletes a recorded interaction session.
func (s *Server) HandleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	session := chi.URLParam(r, "session")
	if session == "" {
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	if err := s.recorder.DeleteSession(session); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok": true}`))
}

// HandleCleanupSessions deletes all but the most recent N sessions.
func (s *Server) HandleCleanupSessions(w http.ResponseWriter, r *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	keep := 10

	keepStr := r.URL.Query().Get("keep")
	if keepStr != "" {
		if k, err := strconv.Atoi(keepStr); err == nil {
			keep = k
		}
	}

	if err := s.recorder.CleanupSessions(keep); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok": true}`))
}

// HandleDownloadSession returns a .tar.gz archive of a recorded interaction session.
func (s *Server) HandleDownloadSession(w http.ResponseWriter, r *http.Request) {
	if s.recorder == nil {
		http.Error(w, "Recorder not initialized", http.StatusServiceUnavailable)
		return
	}

	session := chi.URLParam(r, "session")
	if session == "" {
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.tar.gz\"", session))

	if err := s.recorder.ArchiveSession(session, w); err != nil {
		log.Printf("Error archiving session %s: %s", sanitizeLog(session), sanitizeErr(err))
		// Since we already set headers, if we have an error here it might be partially written.
		// But for now, simple error handling.
		return
	}
}

// HandleDeleteSource removes a source from a device's Sources.xml.
// DELETE /setup/sources/{account}/{device}/{sourceID}
func (s *Server) HandleDeleteSource(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")
	device := chi.URLParam(r, "device")
	sourceID := chi.URLParam(r, "sourceID")

	if account == "" || device == "" || sourceID == "" {
		http.Error(w, "account, device, and sourceID are required", http.StatusBadRequest)

		return
	}

	if err := s.ds.DeleteSourceByID(account, device, sourceID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
