package handlers

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

// Server handles HTTP requests for the SoundTouch service.
type Server struct {
	ds                *datastore.DataStore
	sm                *setup.Manager
	mu                sync.RWMutex
	serverURL         string
	proxyURL          string
	httpsServerURL    string
	discovering       bool
	proxyRedact       bool
	proxyLogBody      bool
	recordEnabled     bool
	discoveryInterval time.Duration
	discoveryEnabled  bool
	shortcuts         map[string]int
	recorder          *proxy.Recorder
	Version           string
	Commit            string
	Date              string
}

// NewServer creates a new SoundTouch service server.
func NewServer(ds *datastore.DataStore, sm *setup.Manager, serverURL string, proxyRedact, proxyLogBody, recordEnabled bool) *Server {
	return &Server{
		ds:                ds,
		sm:                sm,
		serverURL:         serverURL,
		proxyURL:          serverURL,
		proxyRedact:       proxyRedact,
		proxyLogBody:      proxyLogBody,
		recordEnabled:     recordEnabled,
		discoveryInterval: 5 * time.Minute,
	}
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
	s.recorder = r
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

	return s.serverURL, s.proxyURL, s.httpsServerURL
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
