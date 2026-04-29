package handlers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/amazon"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/miekg/dns"
)

// Server handles HTTP requests for the SoundTouch service.
type Server struct {
	ds                  *datastore.DataStore
	sm                  *setup.Manager
	mu                  sync.RWMutex
	serverURL           string
	httpsServerURL      string
	discovering         bool
	proxyRedact         bool
	proxyLogBody        bool
	recordEnabled       bool
	discoveryInterval   time.Duration
	discoveryEnabled    bool
	dnsEnabled          bool
	dnsUpstream         []string
	dnsBindAddr         string
	mirrorEnabled       bool
	mirrorEndpoints     []string
	skipMirrorEndpoints []string
	preferredSource     string
	internalPaths       []string
	shortcuts           map[string]int
	recorder            *proxy.Recorder
	dnsDiscovery        *discovery.DNSDiscovery
	UpstreamProxy       http.Handler
	Version             string
	Commit              string
	Date                string
	RepoURL             string
	mgmtUsername        string
	mgmtPassword        string
	spotifyClientID     string
	spotifyClientSecret string
	spotifyRedirectURI  string
	spotifyService      *spotify.Service
	amazonService       *amazon.Service
}

// RequestSnapshot represents an immutable snapshot of an HTTP request.
type RequestSnapshot struct {
	Method    string
	URL       *url.URL
	Headers   http.Header
	Body      []byte
	Host      string
	Timestamp time.Time
}

type ctxKey struct{ name string }

// SnapshotKey is the context key for the RequestSnapshot.
var SnapshotKey = &ctxKey{"request_snapshot"}

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// NewServer creates a new SoundTouch service server.
func NewServer(ds *datastore.DataStore, sm *setup.Manager, serverURL string, proxyRedact, proxyLogBody, recordEnabled bool) *Server {
	s := &Server{
		ds:                ds,
		sm:                sm,
		serverURL:         serverURL,
		proxyRedact:       proxyRedact,
		proxyLogBody:      proxyLogBody,
		recordEnabled:     recordEnabled,
		discoveryInterval: 5 * time.Minute,
		discoveryEnabled:  true,
	}

	return s
}

// SetVersionInfo sets the version information for the server.
func (s *Server) SetVersionInfo(version, commit, date, repoURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Version = version
	s.Commit = commit
	s.Date = date
	s.RepoURL = repoURL
}

// SetDiscoverySettings sets the discovery settings for the server.
func (s *Server) SetDiscoverySettings(interval time.Duration, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.discoveryInterval = interval
	s.discoveryEnabled = enabled
}

// parseUpstreamDNS splits a comma-separated string of DNS servers.
func parseUpstreamDNS(upstream string) []string {
	var upstreamList []string

	if upstream != "" {
		for _, u := range strings.Split(upstream, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				upstreamList = append(upstreamList, u)
			}
		}
	}

	return upstreamList
}

// getSystemDNS returns the DNS servers from /etc/resolv.conf.
func getSystemDNS() []string {
	config, _ := dns.ClientConfigFromFile("/etc/resolv.conf")
	if config != nil && len(config.Servers) > 0 {
		return config.Servers
	}

	return nil
}

// areUpstreamsEqual compares two slices of DNS server addresses.
func areUpstreamsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// SetDNSSettings sets the DNS discovery settings for the server.
func (s *Server) SetDNSSettings(enabled bool, upstream, bind string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldBind := s.dnsBindAddr
	oldUpstream := s.dnsUpstream

	s.dnsEnabled = enabled
	s.dnsBindAddr = bind

	upstreamList := parseUpstreamDNS(upstream)

	// Try to get system DNS if none provided
	if enabled && len(upstreamList) == 0 {
		upstreamList = getSystemDNS()
		if len(upstreamList) > 0 {
			log.Printf("[DNS] Using system DNS servers from /etc/resolv.conf: %v", upstreamList)
		}
	}

	s.dnsUpstream = upstreamList
	upstreamChanged := !areUpstreamsEqual(upstreamList, oldUpstream)

	if s.dnsDiscovery != nil {
		if !enabled || bind != oldBind || upstreamChanged {
			log.Printf("[DNS] Settings changed, stopping DNS discovery server")

			_ = s.dnsDiscovery.Shutdown()
			s.dnsDiscovery = nil
		}
	}

	if enabled && len(upstreamList) == 0 {
		log.Printf("[DNS] Cannot start DNS discovery server: upstream DNS is empty and no system DNS found")

		s.dnsEnabled = false

		return
	}

	if enabled && s.dnsDiscovery == nil {
		s.startDNSDiscovery(bind, upstreamList)
	}
}

func (s *Server) startDNSDiscovery(bind string, upstreamList []string) {
	log.Printf("[DNS] Starting DNS discovery server on %s", bind)

	u, _ := url.Parse(s.serverURL)

	serviceIP := u.Hostname()
	if serviceIP == "localhost" || serviceIP == "" {
		serviceIP = "127.0.0.1"
	}

	if s.sm != nil {
		serviceIP = s.sm.GetResolvedIP(serviceIP)
	}

	s.dnsDiscovery = discovery.NewDNSDiscovery(upstreamList, serviceIP)
	go func(d *discovery.DNSDiscovery, addr string) {
		if err := d.Start(addr); err != nil {
			log.Printf("Warning: DNS discovery server error: %v", err)
		}
	}(s.dnsDiscovery, bind)
}

// GetDNSRunning returns whether DNS discovery is active and its bind address.
func (s *Server) GetDNSRunning() (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.dnsDiscovery == nil {
		return false, ""
	}

	return s.dnsDiscovery.IsRunning(s.dnsBindAddr), s.dnsBindAddr
}

// SetDNSDiscoveries sets the initial DNS discoveries for the server.
func (s *Server) SetDNSDiscoveries(discoveries map[string]*discovery.DiscoveredHost) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dnsDiscovery != nil {
		s.dnsDiscovery.SetDiscovered(discoveries)
	}
}

// GetDNSDiscovery returns the current DNS discoveries.
func (s *Server) GetDNSDiscovery() map[string]*discovery.DiscoveredHost {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.dnsDiscovery == nil {
		return nil
	}

	return s.dnsDiscovery.GetDiscovered()
}

// SetShortcuts sets the request shortcuts for the server.
func (s *Server) SetShortcuts(shortcuts map[string]int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.shortcuts = shortcuts
}

// GetShortcuts returns the current request shortcuts.
func (s *Server) GetShortcuts() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.shortcuts
}

// GetDiscoverySettings returns the current discovery settings.
func (s *Server) GetDiscoverySettings() (time.Duration, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.discoveryInterval, s.discoveryEnabled
}

// SetHTTPServerURL sets the external HTTPS URL of the service.
func (s *Server) SetHTTPServerURL(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.httpsServerURL = url
}

// SetRecorder sets the recorder for the server.
func (s *Server) SetRecorder(r *proxy.Recorder) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.recorder = r
	if r != nil {
		r.Redact = s.proxyRedact
	}
}

// SetSpotifyConfig sets the Spotify OAuth configuration.
func (s *Server) SetSpotifyConfig(clientID, clientSecret, redirectURI string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.spotifyClientID = clientID
	s.spotifyClientSecret = clientSecret
	s.spotifyRedirectURI = redirectURI
}

// SetMgmtConfig sets the management API authentication credentials.
func (s *Server) SetMgmtConfig(username, password string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mgmtUsername = username
	s.mgmtPassword = password
}

// SetMirrorSettings sets the mirroring settings for the server.
func (s *Server) SetMirrorSettings(enabled bool, endpoints, skipEndpoints []string, preferredSource string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.mirrorEnabled = enabled
	s.mirrorEndpoints = endpoints
	s.skipMirrorEndpoints = skipEndpoints
	s.preferredSource = preferredSource
}

// SetInternalPaths sets the internal paths for the server.
func (s *Server) SetInternalPaths(paths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.internalPaths = paths
}

// SetAmazonService sets the Amazon OAuth service.
func (s *Server) SetAmazonService(as *amazon.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.amazonService = as
}

// IsAmazonConfigured returns whether Amazon Music integration is configured.
func (s *Server) IsAmazonConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.amazonService != nil
}

// SetSpotifyService sets the Spotify OAuth service.
func (s *Server) SetSpotifyService(ss *spotify.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.spotifyService = ss
}

// GetRecordEnabled returns whether recording is enabled.
func (s *Server) GetRecordEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.recordEnabled
}

// GetSettings returns the current server settings.
func (s *Server) GetSettings() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.serverURL, s.httpsServerURL
}

// IsSpotifyConfigured returns whether Spotify integration is configured.
func (s *Server) IsSpotifyConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.spotifyService != nil
}

// GetProxySettings returns the current proxy settings.
func (s *Server) GetProxySettings() (bool, bool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.proxyRedact, s.proxyLogBody, s.recordEnabled
}

// DiscoverDevices starts a background device discovery process.
//
//nolint:contextcheck
func (s *Server) DiscoverDevices(ctx context.Context) {
	s.discovering = true

	defer func() { s.discovering = false }()

	log.Println("Scanning for Bose devices...")

	// Use background context if none provided or if it's likely a request context
	if ctx == nil {
		ctx = context.Background()
	}

	// Always wrap in a timeout to prevent hanging forever
	discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	svc := discovery.NewService(10 * time.Second)

	devices, err := svc.DiscoverDevices(discoveryCtx)
	if err != nil {
		log.Printf("Discovery error: %v", err)
		return
	}

	for _, d := range devices {
		s.handleDiscoveredDevice(*d)
	}

	// Post-discovery cleanup: merge overlapping IP/Serial entries
	s.mergeOverlappingDevices()
}

// findExistingDeviceInfoByDeviceID looks for existing device info by deviceID
func (s *Server) findExistingDeviceInfoByDeviceID(deviceID string) *models.ServiceDeviceInfo {
	allDevices, err := s.ds.ListAllDevices()
	if err != nil {
		return nil
	}

	for i := range allDevices {
		device := &allDevices[i]
		if device.DeviceID == deviceID {
			return device
		}
	}

	return nil
}

// PrimeDeviceWithSpotify triggers a Spotify priming of the speaker if a Spotify account is linked.
func (s *Server) PrimeDeviceWithSpotify(deviceIP string) {
	s.mu.RLock()
	svc := s.spotifyService
	s.mu.RUnlock()

	if svc == nil {
		return
	}

	accounts := svc.GetAccounts()
	if len(accounts) == 0 {
		return
	}

	// We'll use the first linked account. In the future, we might want to let the user
	// pick or map accounts to speakers, but for now, we follow the "One linked account" model.
	accessToken, username, err := svc.GetFreshToken()
	if err != nil {
		log.Printf("[Spotify Watchdog] Failed to get fresh token for %s: %v", deviceIP, err)
		return
	}

	log.Printf("[Spotify Watchdog] Proactively priming %s with Spotify user %s", deviceIP, username)

	if err := s.pushSpotifyTokenToDevice(deviceIP, username, accessToken); err != nil {
		log.Printf("[Spotify Watchdog] Failed to prime %s: %v", deviceIP, err)
	} else {
		log.Printf("[Spotify Watchdog] Successfully primed %s", deviceIP)
	}
}

func (s *Server) pushSpotifyTokenToDevice(deviceIP, username, accessToken string) error {
	var zcURL string
	if _, _, err := net.SplitHostPort(deviceIP); err == nil {
		zcURL = fmt.Sprintf("http://%s/zc", deviceIP)
	} else {
		zcURL = fmt.Sprintf("http://%s:8200/zc", deviceIP)
	}

	return spotify.PushSpotifyCredentials(zcURL, username, accessToken)
}

func (s *Server) handleDiscoveredDevice(d models.DiscoveredDevice) {
	log.Printf("Discovered Bose device: %s at %s (Serial: %s)", d.Name, d.Host, d.SerialNo)

	// 1. Always fetch live device info from /info endpoint as the authoritative source
	liveInfo, err := s.sm.GetLiveDeviceInfo(d.Host)
	if err != nil {
		log.Printf("Failed to fetch live device info for %s at %s: %v", d.Name, d.Host, err)
		// Fallback to discovery info if /info is not available
		s.handleDiscoveredDeviceFallback(d)

		return
	}

	// 2. Use deviceID from /info as the canonical device identifier
	deviceID := liveInfo.DeviceID
	if deviceID == "" {
		log.Printf("No deviceID found in /info response for %s at %s, using fallback", d.Name, d.Host)
		s.handleDiscoveredDeviceFallback(d)

		return
	}

	log.Printf("Using deviceID '%s' from /info for device %s at %s", deviceID, d.Name, d.Host)

	// 3. Get account ID from live info or fallback to existing/default
	accountID := liveInfo.MargeAccountUUID
	if accountID == "" {
		// Try to find account ID from existing device entries
		if existing := s.findExistingDeviceInfoByDeviceID(deviceID); existing != nil {
			accountID = existing.AccountID
		}
	}

	if accountID == "" {
		accountID = "default"
	}

	// 4. Get primary MAC address from networkInfo
	macAddress := liveInfo.GetPrimaryMacAddress()

	// 5. Build complete device info from live data
	info := &models.ServiceDeviceInfo{
		DeviceID:            deviceID, // Use deviceID from /info (MAC address)
		AccountID:           accountID,
		Name:                liveInfo.Name,                             // Use name from /info
		IPAddress:           d.Host,                                    // IP from discovery
		MacAddress:          macAddress,                                // MAC from /info networkInfo
		DeviceSerialNumber:  liveInfo.SerialNumber,                     // Serial from components
		ProductCode:         liveInfo.Type + " " + liveInfo.ModuleType, // Type + ModuleType
		FirmwareVersion:     liveInfo.SoftwareVer,
		ProductSerialNumber: "", // Will be populated from components if available
		DiscoveryMethod:     d.DiscoveryMethod,
	}

	// 6. Extract product serial number from PackagedProduct component
	for _, comp := range liveInfo.Components {
		if comp.Category == "PackagedProduct" && comp.SerialNumber != "" {
			info.ProductSerialNumber = comp.SerialNumber
			break
		}
	}

	// 7. Save the updated device info
	if err := s.ds.SaveDeviceInfo(accountID, deviceID, info); err != nil {
		log.Printf("Failed to save device info for %s: %v", deviceID, err)
		return
	}

	// 8. Create default Sources.xml only when no sources file exists yet
	if !s.ds.HasConfiguredSources(accountID, deviceID) {
		if sources, err := s.ds.GetConfiguredSources(accountID, deviceID); err == nil {
			log.Printf("Creating default Sources.xml for device %s", deviceID)

			if err := s.ds.SaveConfiguredSources(accountID, deviceID, sources); err != nil {
				log.Printf("Failed to save default sources for %s: %v", deviceID, err)
			}
		}
	}

	log.Printf("Successfully saved device %s (%s) with MAC-based deviceID: %s", info.Name, d.Host, deviceID)
}

// handleDiscoveredDeviceFallback handles device discovery when /info endpoint is not available
func (s *Server) handleDiscoveredDeviceFallback(d models.DiscoveredDevice) {
	log.Printf("Using fallback discovery method for device: %s at %s", d.Name, d.Host)

	// Use discovery data as-is with the old logic
	existingID := s.findExistingDeviceID(d)

	deviceID := d.SerialNo
	if deviceID == "" {
		deviceID = d.Host
	}

	accountID := "default"
	if existing := s.findExistingDeviceInfo(d); existing != nil {
		accountID = existing.AccountID
	}

	info := &models.ServiceDeviceInfo{
		DeviceID:           deviceID,
		AccountID:          accountID,
		Name:               d.Name,
		IPAddress:          d.Host,
		DeviceSerialNumber: d.SerialNo,
		ProductCode:        d.ModelID,
		FirmwareVersion:    "0.0.0", // Unknown from discovery
		DiscoveryMethod:    d.DiscoveryMethod,
	}

	// If we had an IP-based entry and now have a Serial, clean up the IP-based entry
	if d.SerialNo != "" && existingID != "" && existingID != d.SerialNo {
		log.Printf("Device %s previously known as %s, migrating to serial-based ID %s", d.Name, existingID, d.SerialNo)
		_ = s.ds.RemoveDevice(accountID, existingID)
	}

	if err := s.ds.SaveDeviceInfo(accountID, deviceID, info); err != nil {
		log.Printf("Failed to save device info for %s: %v", deviceID, err)
		return
	}

	// Create default Sources.xml only when no sources file exists yet
	if !s.ds.HasConfiguredSources(accountID, deviceID) {
		if sources, err := s.ds.GetConfiguredSources(accountID, deviceID); err == nil {
			log.Printf("Creating default Sources.xml for device %s (fallback)", deviceID)

			if err := s.ds.SaveConfiguredSources(accountID, deviceID, sources); err != nil {
				log.Printf("Failed to save default sources for %s: %v", deviceID, err)
			}
		}
	}

	log.Printf("Successfully saved device %s (%s) with fallback deviceID: %s", info.Name, d.Host, deviceID)
}

func (s *Server) mergeOverlappingDevices() {
	allDevices, err := s.ds.ListAllDevices()
	if err != nil {
		return
	}

	// Group devices by IP
	byIP := make(map[string][]models.ServiceDeviceInfo)

	for i := range allDevices {
		dev := allDevices[i]
		if dev.IPAddress != "" {
			byIP[dev.IPAddress] = append(byIP[dev.IPAddress], dev)
		}
	}

	for ip, devices := range byIP {
		if len(devices) <= 1 {
			continue
		}

		// We have multiple entries for the same IP.
		// Try to find one with a Serial Number to be the master.
		var master *models.ServiceDeviceInfo

		for i := range devices {
			if devices[i].DeviceSerialNumber != "" {
				master = &devices[i]
				break
			}
		}

		if master == nil {
			// Fallback: look for one with DeviceID that isn't the IP
			for i := range devices {
				if devices[i].DeviceID != "" && devices[i].DeviceID != devices[i].IPAddress {
					master = &devices[i]
					break
				}
			}
		}

		if master == nil {
			// None have serials, just keep the first one
			continue
		}

		masterID := master.DeviceID
		if masterID == "" {
			masterID = master.DeviceSerialNumber
		}

		for i := range devices {
			dev := devices[i]
			devID := dev.DeviceID

			if devID == "" {
				devID = dev.IPAddress
			}

			if devID != masterID && dev.IPAddress == ip {
				log.Printf("Merging overlapping device entry %s into %s (IP: %s)", devID, masterID, ip)
				_ = s.ds.RemoveDevice(dev.AccountID, devID)
			}
		}
	}
}

func (s *Server) findExistingDeviceID(d models.DiscoveredDevice) string {
	info := s.findExistingDeviceInfo(d)
	if info != nil {
		return info.DeviceID
	}

	return ""
}

func (s *Server) findExistingDeviceInfo(d models.DiscoveredDevice) *models.ServiceDeviceInfo {
	allDevices, _ := s.ds.ListAllDevices()
	for i := range allDevices {
		known := allDevices[i]
		// Match by Serial
		if d.SerialNo != "" && (known.DeviceID == d.SerialNo || known.DeviceSerialNumber == d.SerialNo) {
			return &known
		}
		// Match by IP
		if d.Host != "" && known.IPAddress == d.Host {
			return &known
		}
	}

	return nil
}

func (s *Server) resolveDeviceIDToIP(deviceID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Try to find in Datastore
	devices, err := s.ds.ListAllDevices()
	if err == nil {
		for i := range devices {
			if devices[i].DeviceID == deviceID {
				return devices[i].IPAddress, nil
			}
		}
	}

	return "", fmt.Errorf("device not found: %s", deviceID)
}
