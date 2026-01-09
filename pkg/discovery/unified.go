package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// UnifiedDiscoveryService combines SSDP and mDNS discovery methods
type UnifiedDiscoveryService struct {
	ssdpService *DiscoveryService
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
		ssdpService: NewDiscoveryServiceWithConfig(cfg),
		mdnsService: NewMDNSDiscoveryService(timeout),
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
			devices, err := u.ssdpService.DiscoverDevices(ctx)
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

// mergeDevices merges two device lists, avoiding duplicates based on host
func (u *UnifiedDiscoveryService) mergeDevices(existing, new []*models.DiscoveredDevice) []*models.DiscoveredDevice {
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
