package discovery

import (
	"context"
	"fmt"
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

// DiscoveryService handles UPnP SSDP discovery of SoundTouch devices
type DiscoveryService struct {
	timeout  time.Duration
	cache    map[string]*models.DiscoveredDevice
	cacheTTL time.Duration
	mutex    sync.RWMutex
	config   *config.Config
}

// NewDiscoveryService creates a new UPnP discovery service
func NewDiscoveryService(timeout time.Duration) *DiscoveryService {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &DiscoveryService{
		timeout:  timeout,
		cache:    make(map[string]*models.DiscoveredDevice),
		cacheTTL: defaultCacheTTL,
		mutex:    sync.RWMutex{},
		config:   config.DefaultConfig(),
	}
}

// NewDiscoveryServiceWithConfig creates a new discovery service with configuration
func NewDiscoveryServiceWithConfig(cfg *config.Config) *DiscoveryService {
	timeout := cfg.DiscoveryTimeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	cacheTTL := cfg.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = defaultCacheTTL
	}

	return &DiscoveryService{
		timeout:  timeout,
		cache:    make(map[string]*models.DiscoveredDevice),
		cacheTTL: cacheTTL,
		mutex:    sync.RWMutex{},
		config:   cfg,
	}
}

// DiscoverDevices discovers all SoundTouch devices on the network
func (d *DiscoveryService) DiscoverDevices(ctx context.Context) ([]*models.DiscoveredDevice, error) {
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
		upnpDevices, err := d.performDiscovery(ctx)
		if err != nil {
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
func (d *DiscoveryService) DiscoverDevice(ctx context.Context, host string) (*models.DiscoveredDevice, error) {
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
func (d *DiscoveryService) GetCachedDevices() []*models.DiscoveredDevice {
	d.cleanupCache()
	return d.getCachedDevices()
}

// ClearCache clears the device cache
func (d *DiscoveryService) ClearCache() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.cache = make(map[string]*models.DiscoveredDevice)
}

// performDiscovery performs the actual UPnP SSDP discovery
func (d *DiscoveryService) performDiscovery(ctx context.Context) ([]*models.DiscoveredDevice, error) {
	log.Printf("UPnP: Starting SSDP discovery for '%s' with timeout %v", soundTouchURN, d.timeout)

	// Create UDP connection for multicast
	conn, err := net.Dial("udp", ssdpAddr)
	if err != nil {
		log.Printf("UPnP: Failed to create UDP connection to %s: %v", ssdpAddr, err)
		return nil, fmt.Errorf("failed to create UDP connection: %w", err)
	}
	defer conn.Close()

	log.Printf("UPnP: Successfully connected to SSDP multicast address %s", ssdpAddr)

	// Send M-SEARCH request
	msearchRequest := d.buildMSearchRequest()
	log.Printf("UPnP: Sending M-SEARCH request:\n%s", strings.TrimSpace(msearchRequest))

	bytesWritten, err := conn.Write([]byte(msearchRequest))
	if err != nil {
		log.Printf("UPnP: Failed to send M-SEARCH request: %v", err)
		return nil, fmt.Errorf("failed to send M-SEARCH: %w", err)
	}
	log.Printf("UPnP: Successfully sent M-SEARCH request (%d bytes)", bytesWritten)

	// Listen for responses
	devices := make(map[string]*models.DiscoveredDevice)
	responseCount := 0

	// Set read deadline
	deadline := time.Now().Add(d.timeout)
	if err := conn.SetReadDeadline(deadline); err != nil {
		log.Printf("UPnP: Failed to set read deadline: %v", err)
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}
	log.Printf("UPnP: Set read deadline to %v, now listening for responses...", deadline.Format("15:04:05.000"))

	buffer := make([]byte, 4096)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			log.Printf("UPnP: Discovery cancelled by context")
			return nil, ctx.Err()
		default:
			n, err := conn.Read(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					log.Printf("UPnP: Read timeout reached after %v, stopping discovery", d.timeout)
					break // Timeout reached, stop reading
				}
				log.Printf("UPnP: Error reading response: %v", err)
				return nil, fmt.Errorf("failed to read response: %w", err)
			}

			responseCount++
			responseText := string(buffer[:n])
			log.Printf("UPnP: Received response #%d (%d bytes):\n%s", responseCount, n, strings.TrimSpace(responseText))

			device, err := d.parseResponse(responseText)
			if err != nil {
				log.Printf("UPnP: Failed to parse response #%d: %v", responseCount, err)
				continue // Skip invalid responses
			}

			if device != nil {
				log.Printf("UPnP: Successfully parsed device from response #%d: %s at %s:%d", responseCount, device.Name, device.Host, device.Port)
				devices[device.Host] = device
			} else {
				log.Printf("UPnP: Response #%d did not contain a valid SoundTouch device", responseCount)
			}
		}
	}

	// Convert map to slice
	result := make([]*models.DiscoveredDevice, 0, len(devices))
	for _, device := range devices {
		result = append(result, device)
	}

	log.Printf("UPnP: Discovery completed. Processed %d responses, found %d unique devices", responseCount, len(result))
	for i, device := range result {
		log.Printf("UPnP: Device #%d: %s at %s:%d (Location: %s)", i+1, device.Name, device.Host, device.Port, device.Location)
	}

	return result, nil
}

// buildMSearchRequest builds the M-SEARCH request for SoundTouch devices
func (d *DiscoveryService) buildMSearchRequest() string {
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
func (d *DiscoveryService) parseResponse(response string) (*models.DiscoveredDevice, error) {
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
	device, err := d.parseLocationURL(location)
	if err != nil {
		log.Printf("UPnP: Failed to parse location URL '%s': %v", location, err)
		return nil, fmt.Errorf("failed to parse location URL: %w", err)
	}
	log.Printf("UPnP: Successfully parsed device from location: %s at %s:%d", device.Name, device.Host, device.Port)

	// Try to get more device info from the location URL
	if err := d.enrichDeviceInfo(device, location); err != nil {
		log.Printf("UPnP: Could not enrich device info from location '%s': %v", location, err)
		// Don't fail if we can't get additional info
		// The basic info from URL parsing should be sufficient
	} else {
		log.Printf("UPnP: Successfully enriched device info for %s", device.Name)
	}

	return device, nil
}

// parseLocationURL extracts basic device info from the location URL
func (d *DiscoveryService) parseLocationURL(location string) (*models.DiscoveredDevice, error) {
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
		Host:     host,
		Port:     port,
		Location: location,
		LastSeen: time.Now(),
		Name:     fmt.Sprintf("SoundTouch-%s", host), // Default name
	}

	return device, nil
}

// enrichDeviceInfo tries to get additional device information from the device description
func (d *DiscoveryService) enrichDeviceInfo(device *models.DiscoveredDevice, location string) error {
	log.Printf("UPnP: Attempting to enrich device info by fetching %s", location)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(location)
	if err != nil {
		log.Printf("UPnP: Failed to fetch device description from %s: %v", location, err)
		return err
	}
	defer resp.Body.Close()

	log.Printf("UPnP: Successfully fetched device description from %s (Status: %s)", location, resp.Status)

	// For now, we'll keep it simple and not parse the full UPnP device description
	// This can be enhanced later to extract more detailed device information
	return nil
}

// updateCache updates the device cache with discovered devices
func (d *DiscoveryService) updateCache(devices []*models.DiscoveredDevice) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	for _, device := range devices {
		d.cache[device.Host] = device
	}
}

// getCachedDevices returns all valid cached devices (internal method)
func (d *DiscoveryService) getCachedDevices() []*models.DiscoveredDevice {
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
func (d *DiscoveryService) cleanupCache() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	for host, device := range d.cache {
		if time.Since(device.LastSeen) >= d.cacheTTL {
			delete(d.cache, host)
		}
	}
}

// getConfiguredDevices returns devices from configuration
func (d *DiscoveryService) getConfiguredDevices() []*models.DiscoveredDevice {
	return d.config.GetPreferredDevicesAsDiscovered()
}

// mergeDevices merges two device lists, avoiding duplicates based on host
func (d *DiscoveryService) mergeDevices(existing, new []*models.DiscoveredDevice) []*models.DiscoveredDevice {
	hostSet := make(map[string]bool)
	result := make([]*models.DiscoveredDevice, 0, len(existing)+len(new))

	// Add existing devices
	for _, device := range existing {
		if !hostSet[device.Host] {
			result = append(result, device)
			hostSet[device.Host] = true
		}
	}

	// Add new devices if not already present
	for _, device := range new {
		if !hostSet[device.Host] {
			result = append(result, device)
			hostSet[device.Host] = true
		}
	}

	return result
}
