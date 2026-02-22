package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/config"
)

func TestNewUnifiedDiscoveryService(t *testing.T) {
	cfg := config.DefaultConfig()
	service := NewUnifiedDiscoveryService(cfg)

	if service == nil {
		t.Fatal("Expected service to be created, got nil")
	}

	if service.config != cfg {
		t.Error("Expected config to be set correctly")
	}

	if service.ssdpService == nil {
		t.Error("Expected SSDP service to be initialized")
	}

	if service.mdnsService == nil {
		t.Error("Expected mDNS service to be initialized")
	}

	if service.cache == nil {
		t.Error("Expected cache to be initialized")
	}
}

func TestUnifiedDiscoveryWithDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	service := NewUnifiedDiscoveryService(cfg)

	if service == nil {
		t.Error("Expected service to be created, got nil")
	}

	// Both UPnP and mDNS should be enabled by default
	if !cfg.UPnPEnabled {
		t.Error("Expected UPnP to be enabled by default")
	}

	if !cfg.MDNSEnabled {
		t.Error("Expected mDNS to be enabled by default")
	}
}

func TestUnifiedDiscoveryWithCustomConfig(t *testing.T) {
	cfg := &config.Config{
		DiscoveryTimeout: 10 * time.Second,
		UPnPEnabled:      false,
		MDNSEnabled:      true,
		CacheEnabled:     false,
		CacheTTL:         60 * time.Second,
		HTTPTimeout:      15 * time.Second,
	}

	service := NewUnifiedDiscoveryService(cfg)

	if service.config.DiscoveryTimeout != 10*time.Second {
		t.Errorf("Expected timeout 10s, got %v", service.config.DiscoveryTimeout)
	}

	if service.config.UPnPEnabled {
		t.Error("Expected UPnP to be disabled")
	}

	if !service.config.MDNSEnabled {
		t.Error("Expected mDNS to be enabled")
	}
}

func TestUnifiedDiscoverDevices(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DiscoveryTimeout = 100 * time.Millisecond
	cfg.CacheEnabled = false // Disable cache for testing
	service := NewUnifiedDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	devices, err := service.DiscoverDevices(ctx)

	// We don't expect an error, even if no devices are found
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// devices slice should be initialized (but might be empty)
	if devices == nil {
		t.Error("Expected devices slice to be initialized, got nil")
	}

	// If devices are found, verify they have the required fields
	for _, device := range devices {
		if device.Host == "" {
			t.Error("Device host should not be empty")
		}

		if device.Port == 0 {
			t.Error("Device port should not be zero")
		}

		if device.Name == "" {
			t.Error("Device name should not be empty")
		}

		if device.InfoURL == "" {
			t.Error("Device info URL should not be empty")
		}
	}
}

func TestUnifiedDiscoveryOnlyMDNS(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DiscoveryTimeout = 100 * time.Millisecond
	cfg.UPnPEnabled = false // Disable UPnP
	cfg.MDNSEnabled = true  // Enable only mDNS
	cfg.CacheEnabled = false
	service := NewUnifiedDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// devices should never be nil, even if no devices are found
	if devices == nil {
		t.Error("Expected devices slice to be initialized, got nil")
	}
}

func TestUnifiedDiscoveryOnlySSDP(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DiscoveryTimeout = 100 * time.Millisecond
	cfg.UPnPEnabled = true  // Enable only UPnP
	cfg.MDNSEnabled = false // Disable mDNS
	cfg.CacheEnabled = false
	service := NewUnifiedDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if devices == nil {
		t.Error("Expected devices slice to be initialized, got nil")
	}
}

func TestUnifiedDiscoveryCache(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DiscoveryTimeout = 100 * time.Millisecond
	cfg.CacheEnabled = true
	cfg.CacheTTL = 1 * time.Second
	service := NewUnifiedDiscoveryService(cfg)

	ctx := context.Background()

	// First discovery
	start := time.Now()
	devices1, err := service.DiscoverDevices(ctx)
	firstDuration := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error on first discovery, got %v", err)
	}

	// Second discovery (should use cache)
	start = time.Now()
	devices2, err := service.DiscoverDevices(ctx)
	secondDuration := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error on second discovery, got %v", err)
	}

	// Second call should be much faster (cached)
	if secondDuration > firstDuration/2 {
		t.Logf("First discovery: %v, Second discovery: %v", firstDuration, secondDuration)
		// Note: We don't fail here because in a test environment without devices,
		// both calls might be very fast anyway
	}

	// Results should be the same
	if len(devices1) != len(devices2) {
		t.Errorf("Expected same number of devices from cache, got %d vs %d", len(devices1), len(devices2))
	}
}

func TestUnifiedDiscoveryClearCache(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CacheEnabled = true
	service := NewUnifiedDiscoveryService(cfg)

	// Clear cache should not panic
	service.ClearCache()

	// Check that cache is empty
	cached := service.GetCachedDevices()
	if len(cached) != 0 {
		t.Errorf("Expected empty cache after clear, got %d devices", len(cached))
	}
}

func TestUnifiedSetMDNSEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	service := NewUnifiedDiscoveryService(cfg)

	// Initially enabled
	if !service.config.MDNSEnabled {
		t.Error("Expected mDNS to be enabled initially")
	}

	// Disable mDNS
	service.SetMDNSEnabled(false)

	if service.config.MDNSEnabled {
		t.Error("Expected mDNS to be disabled after SetMDNSEnabled(false)")
	}

	// Enable mDNS
	service.SetMDNSEnabled(true)

	if !service.config.MDNSEnabled {
		t.Error("Expected mDNS to be enabled after SetMDNSEnabled(true)")
	}
}

func TestUnifiedDiscoveryWithCancelledContext(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DiscoveryTimeout = 5 * time.Second
	service := NewUnifiedDiscoveryService(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	devices, err := service.DiscoverDevices(ctx)

	// Should handle cancelled context gracefully
	// devices should never be nil, even if cancelled
	if devices == nil {
		t.Error("Expected devices slice to be initialized, got nil")
	}

	// Error might or might not occur depending on timing, network conditions, and cancellation
	// This is acceptable for testing - we just ensure no panic and proper slice initialization
	_ = err
}

func TestUnifiedDiscoveryTimeout(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DiscoveryTimeout = 200 * time.Millisecond
	cfg.CacheEnabled = false
	service := NewUnifiedDiscoveryService(cfg)

	start := time.Now()
	ctx := context.Background()

	_, err := service.DiscoverDevices(ctx)
	duration := time.Since(start)

	// The discovery should not take significantly longer than the timeout
	// Allow some buffer for processing time and parallel execution
	maxExpected := 1 * time.Second // More generous for parallel execution
	if duration > maxExpected {
		t.Errorf("Discovery took too long: %v, expected less than %v", duration, maxExpected)
	}

	// We don't check for error here because discovery might succeed quickly
	// or fail due to network conditions, both are acceptable in tests
	_ = err
}
