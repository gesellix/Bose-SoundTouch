package discovery

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// Service handles UPnP SSDP discovery of SoundTouch devices
type Service struct {
	timeout    time.Duration
	cache      map[string]*models.DiscoveredDevice
	cacheTTL   time.Duration
	mutex      sync.RWMutex
	config     *config.Config
	httpClient *http.Client
}

// NewService creates a new UPnP discovery service
func NewService(timeout time.Duration) *Service {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &Service{
		timeout:    timeout,
		cache:      make(map[string]*models.DiscoveredDevice),
		cacheTTL:   defaultCacheTTL,
		mutex:      sync.RWMutex{},
		config:     config.DefaultConfig(),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// NewServiceWithConfig creates a new discovery service with configuration
func NewServiceWithConfig(cfg *config.Config) *Service {
	timeout := cfg.DiscoveryTimeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	cacheTTL := cfg.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = defaultCacheTTL
	}

	return &Service{
		timeout:    timeout,
		cache:      make(map[string]*models.DiscoveredDevice),
		cacheTTL:   cacheTTL,
		mutex:      sync.RWMutex{},
		config:     cfg,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// DiscoverDevices discovers all SoundTouch devices on the network
func (d *Service) DiscoverDevices(ctx context.Context) ([]*models.DiscoveredDevice, error) {
	// Check cache first
	d.cleanupCache()

	cached := d.getCachedDevices()
	if len(cached) > 0 {
		return cached, nil
	}

	var allDevices []*models.DiscoveredDevice

	// Add configured devices first
	configuredDevices := d.getConfiguredDevices()
	allDevices = append(allDevices, configuredDevices...)

	// Perform UPnP discovery if enabled
	if d.config.UPnPEnabled {
		upnpDevices, err := d.PerformDiscovery(ctx)
		if err != nil {
			log.Printf("UPnP: Discovery failed: %v", err)
			// Don't fail completely if UPnP fails, just log and continue with configured devices
			// We'll just use configured devices
		} else {
			// Merge UPnP devices, avoiding duplicates
			allDevices = d.mergeDevices(allDevices, upnpDevices)
		}
	}

	// Update cache
	d.updateCache(allDevices)

	return allDevices, nil
}

// DiscoverDevice discovers a specific SoundTouch device by host
func (d *Service) DiscoverDevice(ctx context.Context, host string) (*models.DiscoveredDevice, error) {
	// Check cache first
	d.mutex.RLock()

	if device, exists := d.cache[host]; exists && time.Since(device.LastSeen) < d.cacheTTL {
		d.mutex.RUnlock()
		return device, nil
	}

	d.mutex.RUnlock()

	// Try to discover all devices and find the specific one
	devices, err := d.DiscoverDevices(ctx)
	if err != nil {
		return nil, err
	}

	for _, device := range devices {
		if device.Host == host {
			return device, nil
		}
	}

	return nil, fmt.Errorf("device with host %s not found", host)
}

// GetCachedDevices returns all cached devices that haven't expired
func (d *Service) GetCachedDevices() []*models.DiscoveredDevice {
	d.cleanupCache()
	return d.getCachedDevices()
}

// ClearCache clears the device cache
func (d *Service) ClearCache() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.cache = make(map[string]*models.DiscoveredDevice)
}

// PerformDiscovery performs the actual UPnP SSDP discovery
func (d *Service) PerformDiscovery(ctx context.Context) ([]*models.DiscoveredDevice, error) {
	log.Printf("UPnP: Starting SSDP discovery for '%s' with timeout %v", soundTouchURN, d.timeout)

	listener, err := d.setupUDPListener()
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = listener.Close()
	}()

	multicastAddr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		log.Printf("UPnP: Failed to resolve multicast address %s: %v", ssdpAddr, err)
		return nil, fmt.Errorf("failed to resolve multicast address: %w", err)
	}

	if err = d.sendMSearch(listener, multicastAddr); err != nil {
		return nil, err
	}

	// Listen for responses
	devices := make(map[string]*models.DiscoveredDevice)

	responseCount, err := d.listenForResponses(ctx, listener, devices)
	if err != nil {
		return nil, err
	}

	// Convert map to slice
	result := make([]*models.DiscoveredDevice, 0, len(devices))
	for _, device := range devices {
		result = append(result, device)
	}

	log.Printf("UPnP: Discovery completed. Processed %d responses, found %d unique devices", responseCount, len(result))

	for i, device := range result {
		log.Printf("UPnP: Device #%d: %s at %s:%d (UPnP Location: %s)", i+1, device.Name, device.Host, device.Port, device.UPnPLocation)
	}

	return result, nil
}

func (d *Service) setupUDPListener() (*net.UDPConn, error) {
	listenAddr, err := net.ResolveUDPAddr("udp4", ":0")
	if err != nil {
		log.Printf("UPnP: Failed to resolve listen address: %v", err)
		return nil, fmt.Errorf("failed to resolve listen address: %w", err)
	}

	listener, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		log.Printf("UPnP: Failed to create UDP listener: %v", err)
		return nil, fmt.Errorf("failed to create UDP listener: %w", err)
	}

	addr := listener.LocalAddr()

	localAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		_ = listener.Close()

		log.Printf("UPnP: Failed to cast local address to UDPAddr: %v", addr)

		return nil, fmt.Errorf("failed to cast local address to UDPAddr: %v", addr)
	}

	log.Printf("UPnP: Created UDP listener on %s", localAddr.String())

	return listener, nil
}

func (d *Service) sendMSearch(listener *net.UDPConn, multicastAddr *net.UDPAddr) error {
	msearchRequest := d.buildMSearchRequest()
	log.Printf("UPnP: Sending M-SEARCH request to %s:\n%s", ssdpAddr, strings.TrimSpace(msearchRequest))

	bytesWritten, err := listener.WriteToUDP([]byte(msearchRequest), multicastAddr)
	if err != nil {
		log.Printf("UPnP: Failed to send M-SEARCH request: %v", err)
		return fmt.Errorf("failed to send M-SEARCH: %w", err)
	}

	log.Printf("UPnP: Successfully sent M-SEARCH request (%d bytes)", bytesWritten)

	return nil
}

func (d *Service) listenForResponses(ctx context.Context, listener *net.UDPConn, devices map[string]*models.DiscoveredDevice) (int, error) {
	responseCount := 0

	// Set read deadline
	deadline := time.Now().Add(d.timeout)
	if err := listener.SetReadDeadline(deadline); err != nil {
		log.Printf("UPnP: Failed to set read deadline: %v", err)
		return 0, fmt.Errorf("failed to set read deadline: %w", err)
	}

	log.Printf("UPnP: Set read deadline to %v, now listening for responses...", deadline.Format("15:04:05.000"))

	buffer := make([]byte, 4096)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			log.Printf("UPnP: Discovery cancelled by context")
			return responseCount, ctx.Err()
		default:
			n, remoteAddr, err := listener.ReadFromUDP(buffer)
			if err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					log.Printf("UPnP: Read timeout reached after %v, stopping discovery", d.timeout)
					return responseCount, nil
				}

				log.Printf("UPnP: Error reading response: %v", err)

				return responseCount, fmt.Errorf("failed to read response: %w", err)
			}

			responseCount++
			responseText := string(buffer[:n])
			log.Printf("UPnP: Received response #%d (%d bytes) from %s:\n%s", responseCount, n, remoteAddr.String(), strings.TrimSpace(responseText))

			device, err := d.parseResponse(responseText)
			if err != nil {
				log.Printf("UPnP: Failed to parse response #%d from %s: %v", responseCount, remoteAddr.String(), err)
				continue // Skip invalid responses
			}

			if device != nil {
				log.Printf("UPnP: Successfully parsed device from response #%d: %s at %s:%d", responseCount, device.Name, device.Host, device.Port)
				devices[device.Host] = device
			} else {
				log.Printf("UPnP: Response #%d from %s did not contain a valid SoundTouch device", responseCount, remoteAddr.String())
			}
		}
	}

	return responseCount, nil
}

// buildMSearchRequest builds the M-SEARCH request for SoundTouch devices
func (d *Service) buildMSearchRequest() string {
	return fmt.Sprintf(
		"M-SEARCH * HTTP/1.1\r\n"+
			"HOST: %s\r\n"+
			"MAN: \"ssdp:discover\"\r\n"+
			"ST: %s\r\n"+
			"MX: %d\r\n"+
			"\r\n",
		ssdpAddr,
		soundTouchURN,
		int(d.timeout.Seconds()),
	)
}

// parseResponse parses UPnP SSDP response and extracts device information
func (d *Service) parseResponse(response string) (*models.DiscoveredDevice, error) {
	log.Printf("UPnP: Parsing response (%d chars): %.100s...", len(response), strings.ReplaceAll(response, "\r\n", "\\r\\n"))

	// Try both \r\n and \n line endings
	var lines []string
	if strings.Contains(response, "\r\n") {
		lines = strings.Split(response, "\r\n")
	} else {
		lines = strings.Split(response, "\n")
	}

	// Check if it's a valid HTTP response
	if len(lines) < 1 || !strings.HasPrefix(lines[0], "HTTP/1.1 200") {
		log.Printf("UPnP: Invalid HTTP response, first line: '%s'", lines[0])
		return nil, fmt.Errorf("invalid HTTP response")
	}

	log.Printf("UPnP: Valid HTTP response detected")

	headers := make(map[string]string)

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(strings.ToLower(parts[0]))
			value := strings.TrimSpace(parts[1])
			headers[key] = value
		}
	}

	log.Printf("UPnP: Parsed %d headers from response", len(headers))

	for key, value := range headers {
		log.Printf("UPnP: Header: %s = %s", key, value)
	}

	// Check if it's a SoundTouch device
	st, exists := headers["st"]
	if !exists {
		log.Printf("UPnP: No ST header found in response")
		return nil, fmt.Errorf("no ST header found")
	}

	log.Printf("UPnP: Found ST header: %s", st)

	// Accept both MediaRenderer and any device type for now - we'll validate it's a SoundTouch later
	if !strings.Contains(strings.ToLower(st), "mediarenderer") && !strings.Contains(strings.ToLower(st), "upnp:rootdevice") {
		log.Printf("UPnP: Device type '%s' is not a MediaRenderer, skipping", st)
		return nil, fmt.Errorf("not a MediaRenderer device")
	}

	log.Printf("UPnP: Device type '%s' is acceptable", st)

	location, exists := headers["location"]
	if !exists {
		log.Printf("UPnP: No Location header found in response")
		return nil, fmt.Errorf("no location header found")
	}

	log.Printf("UPnP: Found Location header: %s", location)

	// Extract device information from location URL
	device, err := d.parseLocationURL(location, headers["usn"])
	if err != nil {
		log.Printf("UPnP: Failed to parse location URL '%s': %v", location, err)
		return nil, fmt.Errorf("failed to parse location URL: %w", err)
	}

	log.Printf("UPnP: Successfully parsed device from location: %s at %s:%d", device.Name, device.Host, device.Port)

	// Try to get more device info from the location URL
	if err := d.EnrichDeviceInfo(device, location); err != nil {
		log.Printf("UPnP: Could not enrich device info from location '%s': %v", location, err)
		// Don't fail if we can't get additional info
		// The basic info from URL parsing should be sufficient
	} else {
		log.Printf("UPnP: Successfully enriched device info for %s", device.Name)
	}

	return device, nil
}

// parseLocationURL extracts basic device info from the location URL
func (d *Service) parseLocationURL(location, usn string) (*models.DiscoveredDevice, error) {
	log.Printf("UPnP: Parsing location URL: %s", location)

	// Parse the URL to extract host and port
	re := regexp.MustCompile(`http://([^:]+):(\d+)`)
	matches := re.FindStringSubmatch(location)

	if len(matches) < 2 {
		log.Printf("UPnP: Location URL '%s' does not match expected format http://host:port", location)
		return nil, fmt.Errorf("invalid location URL format")
	}

	host := matches[1]
	port := 8090 // Default SoundTouch port
	log.Printf("UPnP: Extracted host='%s', using default port=%d", host, port)

	device := &models.DiscoveredDevice{
		Host:            host,
		Port:            port,
		LastSeen:        time.Now(),
		Name:            fmt.Sprintf("SoundTouch-%s", host), // Default name
		DiscoveryMethod: "SSDP/UPnP",
		APIBaseURL:      fmt.Sprintf("http://%s:%d/", host, port),
		InfoURL:         fmt.Sprintf("http://%s:%d/info", host, port),
		UPnPLocation:    location,
		UPnPUSN:         usn,
	}

	return device, nil
}

// EnrichDeviceInfo tries to get additional device information from the device description
func (d *Service) EnrichDeviceInfo(device *models.DiscoveredDevice, location string) error {
	log.Printf("UPnP: Attempting to enrich device info by fetching %s", location)

	resp, err := d.httpClient.Get(location)
	if err != nil {
		log.Printf("UPnP: Failed to fetch device description from %s: %v", location, err)
		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	log.Printf("UPnP: Successfully fetched device description from %s (Status: %s)", location, resp.Status)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}

	var upnpRoot struct {
		XMLName xml.Name `xml:"root"`
		Device  struct {
			FriendlyName string `xml:"friendlyName"`
			ModelName    string `xml:"modelName"`
			SerialNumber string `xml:"serialNumber"`
		} `xml:"device"`
	}

	if err := xml.Unmarshal(data, &upnpRoot); err != nil {
		log.Printf("UPnP: Failed to parse device description from %s: %v", location, err)
		return err
	}

	if upnpRoot.Device.FriendlyName != "" {
		device.Name = upnpRoot.Device.FriendlyName
	}

	if upnpRoot.Device.ModelName != "" {
		device.ModelID = upnpRoot.Device.ModelName
	}

	if upnpRoot.Device.SerialNumber != "" {
		device.UPnPSerial = upnpRoot.Device.SerialNumber
	}

	log.Printf("UPnP: Enriched device info: Name='%s', Model='%s', UPnPSerial='%s'",
		device.Name, device.ModelID, device.UPnPSerial)

	return nil
}

// updateCache updates the device cache with discovered devices
func (d *Service) updateCache(devices []*models.DiscoveredDevice) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	for _, device := range devices {
		d.cache[device.Host] = device
	}
}

// getCachedDevices returns all valid cached devices (internal method)
func (d *Service) getCachedDevices() []*models.DiscoveredDevice {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	devices := make([]*models.DiscoveredDevice, 0, len(d.cache))
	for _, device := range d.cache {
		if time.Since(device.LastSeen) < d.cacheTTL {
			devices = append(devices, device)
		}
	}

	return devices
}

// cleanupCache removes expired devices from cache
func (d *Service) cleanupCache() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	for host, device := range d.cache {
		if time.Since(device.LastSeen) >= d.cacheTTL {
			delete(d.cache, host)
		}
	}
}

// getConfiguredDevices returns devices from configuration
func (d *Service) getConfiguredDevices() []*models.DiscoveredDevice {
	return d.config.GetPreferredDevicesAsDiscovered()
}

// mergeDevices merges two device lists, avoiding duplicates based on host
func (d *Service) mergeDevices(existing, newDevices []*models.DiscoveredDevice) []*models.DiscoveredDevice {
	hostSet := make(map[string]bool)
	result := make([]*models.DiscoveredDevice, 0, len(existing)+len(newDevices))

	// Add existing devices
	for _, device := range existing {
		if !hostSet[device.Host] {
			result = append(result, device)
			hostSet[device.Host] = true
		}
	}

	// Add new devices if not already present
	for _, device := range newDevices {
		if !hostSet[device.Host] {
			result = append(result, device)
			hostSet[device.Host] = true
		}
	}

	return result
}
