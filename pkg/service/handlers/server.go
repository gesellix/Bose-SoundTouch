package handlers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/miekg/dns"
)

// Server handles HTTP requests for the SoundTouch service.
type Server struct {
	ds                   *datastore.DataStore
	sm                   *setup.Manager
	mu                   sync.RWMutex
	serverURL            string
	soundcorkURL         string
	httpsServerURL       string
	discovering          bool
	proxyRedact          bool
	proxyLogBody         bool
	recordEnabled        bool
	discoveryInterval    time.Duration
	discoveryEnabled     bool
	dnsEnabled           bool
	dnsUpstream          []string
	dnsBindAddr          string
	enableSoundcorkProxy bool
	shortcuts            map[string]int
	recorder             *proxy.Recorder
	dnsDiscovery         *discovery.DNSDiscovery
	UpstreamProxy        http.Handler
	Version              string
	Commit               string
	Date                 string
	mgmtUsername         string
	mgmtPassword         string
	spotifyClientID      string
	spotifyClientSecret  string
	spotifyRedirectURI   string
	spotifyService       *spotify.Service
}

// NewServer creates a new SoundTouch service server.
func NewServer(ds *datastore.DataStore, sm *setup.Manager, serverURL string, proxyRedact, proxyLogBody, recordEnabled, enableSoundcorkProxy bool) *Server {
	s := &Server{
		ds:                   ds,
		sm:                   sm,
		serverURL:            serverURL,
		soundcorkURL:         "http://localhost:8001",
		proxyRedact:          proxyRedact,
		proxyLogBody:         proxyLogBody,
		recordEnabled:        recordEnabled,
		enableSoundcorkProxy: enableSoundcorkProxy,
		discoveryInterval:    5 * time.Minute,
	}

	return s
}

// SetVersionInfo sets the version information for the server.
func (s *Server) SetVersionInfo(version, commit, date string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Version = version
	s.Commit = commit
	s.Date = date
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

// SetSoundcorkURL sets the URL for the Soundcork backend.
func (s *Server) SetSoundcorkURL(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.soundcorkURL = url
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
func (s *Server) GetSettings() (string, string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.serverURL, s.soundcorkURL, s.httpsServerURL
}

// IsSpotifyConfigured returns whether Spotify integration is configured.
func (s *Server) IsSpotifyConfigured() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.spotifyService != nil
}

// GetProxySettings returns the current proxy settings.
func (s *Server) GetProxySettings() (bool, bool, bool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.proxyRedact, s.proxyLogBody, s.recordEnabled, s.enableSoundcorkProxy
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
	// ZeroConf API endpoint on the speaker
	var zcURL string
	if _, _, err := net.SplitHostPort(deviceIP); err == nil {
		// If port is specified (e.g. in tests), keep it but usually it's just IP
		zcURL = fmt.Sprintf("http://%s/zc", deviceIP)
	} else {
		// If no port specified, default to 8200
		zcURL = fmt.Sprintf("http://%s:8200/zc", deviceIP)
	}

	data := url.Values{}
	data.Set("action", "addUser")
	data.Set("userName", username)
	data.Set("blob", accessToken)
	data.Set("clientKey", "")
	data.Set("tokenType", "accesstoken")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.PostForm(zcURL, data)
	if err != nil {
		return fmt.Errorf("POST to %s failed: %w", zcURL, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST to %s returned status %d: %s", zcURL, resp.StatusCode, string(body))
	}

	return nil
}

func (s *Server) handleDiscoveredDevice(d models.DiscoveredDevice) {
	log.Printf("Discovered Bose device: %s at %s (Serial: %s)", d.Name, d.Host, d.SerialNo)

	// 1. Check if we already have this device
	existingID := s.findExistingDeviceID(d)

	// Use SerialNo if available, otherwise fallback to IP for the datastore directory name
	if d.SerialNo == "" {
		// If serial is missing from discovery, try to fetch it from :8090/info
		log.Printf("Serial number missing for %s at %s, attempting live info fetch...", d.Name, d.Host)

		liveInfo, err := s.sm.GetLiveDeviceInfo(d.Host)
		if err == nil && liveInfo.SerialNumber != "" {
			d.SerialNo = liveInfo.SerialNumber
			log.Printf("Successfully retrieved serial number %s for %s via live info", d.SerialNo, d.Host)
		}
	}

	deviceID := d.SerialNo
	if deviceID == "" {
		deviceID = d.Host
	}

	accountID := ""

	if liveInfo, err := s.sm.GetLiveDeviceInfo(d.Host); err == nil {
		if liveInfo.MargeAccountUUID != "" {
			accountID = liveInfo.MargeAccountUUID
		}

		if liveInfo.SerialNumber != "" {
			d.SerialNo = liveInfo.SerialNumber
			deviceID = d.SerialNo
		}
	}

	if accountID == "" {
		// Try to find account ID from existing device entries if live info failed
		if existing := s.findExistingDeviceInfo(d); existing != nil {
			accountID = existing.AccountID
		}
	}

	if accountID == "" {
		accountID = "default"
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
		log.Printf("Failed to save device info: %v", err)
	}
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
