// Package discovery provides automatic network discovery of Bose SoundTouch devices.
//
// This package implements both UPnP/SSDP (Universal Plug and Play) and mDNS/Bonjour
// discovery protocols to automatically find SoundTouch devices on your local network.
// It provides a unified interface that combines both discovery methods for maximum
// device detection reliability.
//
// # Basic Usage
//
// Discover all SoundTouch devices on your network:
//
//	import (
//		"context"
//		"time"
//		"github.com/gesellix/bose-soundtouch/pkg/discovery"
//	)
//
//	ctx := context.Background()
//	timeout := 5 * time.Second
//
//	devices, err := discovery.DiscoverDevices(ctx, timeout)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	for _, device := range devices {
//		fmt.Printf("Found: %s at %s:%d\n", device.Name, device.Host, device.Port)
//	}
//
// # Advanced Discovery
//
// Use specific discovery methods or configure advanced options:
//
//	// Create a unified discovery service
//	service, err := discovery.NewUnifiedDiscoveryService()
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Discover with caching (devices cached for 5 minutes)
//	devices, err := service.DiscoverWithCache(ctx, timeout)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Use only UPnP/SSDP discovery
//	ssdpDevices, err := service.DiscoverUPnP(ctx, timeout)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Use only mDNS discovery
//	mdnsDevices, err := service.DiscoverMDNS(ctx, timeout)
//	if err != nil {
//		log.Fatal(err)
//	}
//
// # Discovery Methods
//
// The package supports two discovery protocols:
//
//   - UPnP/SSDP: Discovers devices advertising UPnP services
//   - mDNS/Bonjour: Discovers devices using multicast DNS
//
// The unified service automatically combines results from both methods and
// deduplicates devices found through multiple protocols.
//
// # Device Information
//
// Discovered devices contain comprehensive information:
//
//	for _, device := range devices {
//		fmt.Printf("Device: %s\n", device.Name)
//		fmt.Printf("Host: %s:%d\n", device.Host, device.Port)
//		fmt.Printf("MAC: %s\n", device.MACAddress)
//		fmt.Printf("Method: %s\n", device.DiscoveryMethod)
//		fmt.Printf("URL: %s\n", device.BaseURL)
//	}
//
// # Caching
//
// The discovery service includes intelligent caching to avoid repeated network
// scans. Devices are cached for a configurable TTL (default: 5 minutes).
//
// # Error Handling
//
// Discovery operations may encounter various network conditions:
//
//	devices, err := discovery.DiscoverDevices(ctx, timeout)
//	if err != nil {
//		// Handle discovery errors
//		fmt.Printf("Discovery failed: %v\n", err)
//		return
//	}
//
//	if len(devices) == 0 {
//		fmt.Println("No SoundTouch devices found on the network")
//	}
//
// # Configuration
//
// Discovery behavior can be customized through configuration:
//
//	// Custom timeout for individual discovery methods
//	service := &discovery.UnifiedDiscoveryService{
//		CacheTTL: 10 * time.Minute,  // Cache devices for 10 minutes
//	}
package discovery

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// UnifiedDiscoveryService combines SSDP and mDNS discovery methods
type UnifiedDiscoveryService struct {
	ssdpService *Service
	mdnsService *MDNSDiscoveryService
	config      *config.Config
	cache       map[string]*models.DiscoveredDevice
	cacheTTL    time.Duration
	mutex       sync.RWMutex
}

// NewUnifiedDiscoveryService creates a new unified discovery service
func NewUnifiedDiscoveryService(cfg *config.Config) *UnifiedDiscoveryService {
	timeout := cfg.DiscoveryTimeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	cacheTTL := cfg.CacheTTL
	if cacheTTL == 0 {
		cacheTTL = defaultCacheTTL
	}

	return &UnifiedDiscoveryService{
		ssdpService: NewServiceWithConfig(cfg),
		mdnsService: NewMDNSDiscoveryServiceWithInterface(timeout, cfg.DiscoveryInterface),
		config:      cfg,
		cache:       make(map[string]*models.DiscoveredDevice),
		cacheTTL:    cacheTTL,
		mutex:       sync.RWMutex{},
	}
}

// DiscoverDevices discovers SoundTouch devices using both SSDP and mDNS
func (u *UnifiedDiscoveryService) DiscoverDevices(ctx context.Context) ([]*models.DiscoveredDevice, error) {
	// Check cache first
	u.cleanupCache()

	cached := u.getCachedDevices()
	if len(cached) > 0 && u.config.CacheEnabled {
		return cached, nil
	}

	// Initialize devices slice to ensure it's never nil
	allDevices := make([]*models.DiscoveredDevice, 0)

	// Add configured devices first
	configuredDevices := u.getConfiguredDevices()
	allDevices = append(allDevices, configuredDevices...)

	// Use channels to collect results from both discovery methods
	ssdpChan := make(chan []*models.DiscoveredDevice, 1)
	mdnsChan := make(chan []*models.DiscoveredDevice, 1)

	var wg sync.WaitGroup

	// Start SSDP discovery if enabled
	if u.config.UPnPEnabled {
		wg.Add(1)

		go func() {
			defer wg.Done()

			// Use PerformDiscovery directly to avoid double-adding configured devices
			devices, err := u.ssdpService.PerformDiscovery(ctx)
			if err == nil {
				ssdpChan <- devices
			} else {
				ssdpChan <- nil
			}
		}()
	} else {
		ssdpChan <- nil
	}

	// Start mDNS discovery if enabled
	if u.config.MDNSEnabled {
		wg.Add(1)

		go func() {
			defer wg.Done()

			devices, err := u.mdnsService.DiscoverDevices(ctx)
			if err == nil {
				mdnsChan <- devices
			} else {
				mdnsChan <- nil
			}
		}()
	} else {
		mdnsChan <- nil
	}

	// Wait for both discovery methods to complete
	wg.Wait()

	// Collect results from both methods
	if ssdpDevices := <-ssdpChan; ssdpDevices != nil {
		allDevices = u.mergeDevices(allDevices, ssdpDevices)
	}

	if mdnsDevices := <-mdnsChan; mdnsDevices != nil {
		allDevices = u.mergeDevices(allDevices, mdnsDevices)
	}

	// Update cache
	u.updateCache(allDevices)

	// Ensure we always return a non-nil slice
	if allDevices == nil {
		allDevices = make([]*models.DiscoveredDevice, 0)
	}

	return allDevices, nil
}

// DiscoverDevice discovers a specific SoundTouch device by host
func (u *UnifiedDiscoveryService) DiscoverDevice(ctx context.Context, host string) (*models.DiscoveredDevice, error) {
	// Check cache first
	u.mutex.RLock()

	if device, exists := u.cache[host]; exists && time.Since(device.LastSeen) < u.cacheTTL {
		u.mutex.RUnlock()
		return device, nil
	}

	u.mutex.RUnlock()

	// Try to discover all devices and find the specific one
	devices, err := u.DiscoverDevices(ctx)
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
func (u *UnifiedDiscoveryService) GetCachedDevices() []*models.DiscoveredDevice {
	u.cleanupCache()
	return u.getCachedDevices()
}

// ClearCache clears the device cache
func (u *UnifiedDiscoveryService) ClearCache() {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.cache = make(map[string]*models.DiscoveredDevice)
}

// SetMDNSEnabled enables or disables mDNS discovery
func (u *UnifiedDiscoveryService) SetMDNSEnabled(enabled bool) {
	u.config.MDNSEnabled = enabled
}

// updateCache updates the device cache with discovered devices
func (u *UnifiedDiscoveryService) updateCache(devices []*models.DiscoveredDevice) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	for _, device := range devices {
		u.cache[device.Host] = device
	}
}

// getCachedDevices returns all valid cached devices (internal method)
func (u *UnifiedDiscoveryService) getCachedDevices() []*models.DiscoveredDevice {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	devices := make([]*models.DiscoveredDevice, 0, len(u.cache))
	for _, device := range u.cache {
		if time.Since(device.LastSeen) < u.cacheTTL {
			devices = append(devices, device)
		}
	}

	return devices
}

// cleanupCache removes expired devices from cache
func (u *UnifiedDiscoveryService) cleanupCache() {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	for host, device := range u.cache {
		if time.Since(device.LastSeen) >= u.cacheTTL {
			delete(u.cache, host)
		}
	}
}

// getConfiguredDevices returns devices from configuration
func (u *UnifiedDiscoveryService) getConfiguredDevices() []*models.DiscoveredDevice {
	return u.config.GetPreferredDevicesAsDiscovered()
}

// mergeDevices merges two device lists, combining protocol-specific data when same device found via multiple methods
func (u *UnifiedDiscoveryService) mergeDevices(existing, newDevices []*models.DiscoveredDevice) []*models.DiscoveredDevice {
	deviceMap := make(map[string]*models.DiscoveredDevice)

	// Add existing devices to map
	for _, device := range existing {
		deviceMap[device.Host] = device
	}

	// Merge new devices, combining protocol-specific data for duplicates
	for _, newDevice := range newDevices {
		if existingDevice, exists := deviceMap[newDevice.Host]; exists {
			// Same device found via different protocol - merge the data
			mergedDevice := u.mergeDeviceData(existingDevice, newDevice)
			deviceMap[newDevice.Host] = mergedDevice
		} else {
			// New device
			deviceMap[newDevice.Host] = newDevice
		}
	}

	// Convert map back to slice
	result := make([]*models.DiscoveredDevice, 0, len(deviceMap))
	for _, device := range deviceMap {
		result = append(result, device)
	}

	return result
}

// mergeDeviceData combines data from two DiscoveredDevice instances representing the same physical device
func (u *UnifiedDiscoveryService) mergeDeviceData(existing, newDevice *models.DiscoveredDevice) *models.DiscoveredDevice {
	// Start with the existing device as base
	merged := *existing

	// Update last seen to the most recent
	if newDevice.LastSeen.After(existing.LastSeen) {
		merged.LastSeen = newDevice.LastSeen
	}

	// Prefer more descriptive names
	merged.Name = u.pickBestName(existing, newDevice)

	// Combine discovery methods
	if !strings.Contains(merged.DiscoveryMethod, newDevice.DiscoveryMethod) {
		merged.DiscoveryMethod = merged.DiscoveryMethod + "+" + newDevice.DiscoveryMethod
	}

	// Merge protocol-specific data
	u.mergeProtocolData(&merged, newDevice)

	// Merge metadata if it exists
	u.mergeMetadata(&merged, newDevice)

	// Keep model info if available
	u.mergeModelInfo(&merged, newDevice)

	return &merged
}

func (u *UnifiedDiscoveryService) pickBestName(existing, newDevice *models.DiscoveredDevice) string {
	// mDNS usually has better names than SSDP
	switch {
	case newDevice.DiscoveryMethod == "mDNS/Bonjour" && existing.DiscoveryMethod == "SSDP/UPnP":
		return newDevice.Name
	case existing.DiscoveryMethod == "Configuration":
		// Keep user-configured name
		return existing.Name
	case newDevice.DiscoveryMethod == "Configuration":
		return newDevice.Name
	default:
		return existing.Name
	}
}

func (u *UnifiedDiscoveryService) mergeProtocolData(merged, newDevice *models.DiscoveredDevice) {
	if newDevice.UPnPLocation != "" {
		merged.UPnPLocation = newDevice.UPnPLocation
	}

	if newDevice.UPnPUSN != "" {
		merged.UPnPUSN = newDevice.UPnPUSN
	}

	if newDevice.MDNSHostname != "" {
		merged.MDNSHostname = newDevice.MDNSHostname
	}

	if newDevice.MDNSService != "" {
		merged.MDNSService = newDevice.MDNSService
	}

	if newDevice.ConfigName != "" {
		merged.ConfigName = newDevice.ConfigName
	}
}

func (u *UnifiedDiscoveryService) mergeMetadata(merged, newDevice *models.DiscoveredDevice) {
	if merged.Metadata == nil {
		merged.Metadata = make(map[string]string)
	}

	if newDevice.Metadata != nil {
		for k, v := range newDevice.Metadata {
			merged.Metadata[k] = v
		}
	}
}

func (u *UnifiedDiscoveryService) mergeModelInfo(merged, newDevice *models.DiscoveredDevice) {
	if newDevice.ModelID != "" && merged.ModelID == "" {
		merged.ModelID = newDevice.ModelID
	}

	if newDevice.SerialNo != "" && merged.SerialNo == "" {
		merged.SerialNo = newDevice.SerialNo
	}
}
