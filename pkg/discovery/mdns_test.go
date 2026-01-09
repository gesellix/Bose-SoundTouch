package discovery

import (
	"context"
	"testing"
	"time"
)

func TestNewMDNSDiscoveryService(t *testing.T) {
	// Test with custom timeout
	service := NewMDNSDiscoveryService(10 * time.Second)
	if service.timeout != 10*time.Second {
		t.Errorf("Expected timeout 10s, got %v", service.timeout)
	}

	// Test with zero timeout (should use default)
	service = NewMDNSDiscoveryService(0)
	if service.timeout != defaultTimeout {
		t.Errorf("Expected default timeout %v, got %v", defaultTimeout, service.timeout)
	}
}

func TestMDNSDiscoverDevices(t *testing.T) {
	service := NewMDNSDiscoveryService(2 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Note: This test will attempt actual mDNS discovery
	// In a real network environment, this might find actual SoundTouch devices
	// In a test environment without devices, it should return an empty slice
	devices, _ := service.DiscoverDevices(ctx)

	// devices slice should be initialized (but might be empty)
	// We don't fail on errors as they may be due to network conditions in test environment
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
		if device.Location == "" {
			t.Error("Device location should not be empty")
		}
	}
}

func TestMDNSServiceEntryToDevice(t *testing.T) {
	service := NewMDNSDiscoveryService(5 * time.Second)

	// Test with nil entry
	device := service.serviceEntryToDevice(nil)
	if device != nil {
		t.Error("Expected nil device for nil entry")
	}

	// Note: Testing with actual zeroconf.ServiceEntry would require
	// creating mock objects or using a testing framework that can
	// create proper ServiceEntry instances. For now, we test the nil case.
}

func TestMDNSDiscoveryTimeout(t *testing.T) {
	service := NewMDNSDiscoveryService(100 * time.Millisecond)

	start := time.Now()
	ctx := context.Background()

	_, err := service.DiscoverDevices(ctx)
	duration := time.Since(start)

	// The discovery should not take significantly longer than the timeout
	// Allow some buffer for processing time
	maxExpected := 200 * time.Millisecond
	if duration > maxExpected {
		t.Errorf("Discovery took too long: %v, expected less than %v", duration, maxExpected)
	}

	// We don't check for error here because mDNS discovery might succeed quickly
	// or fail due to network conditions, both are acceptable in tests
	_ = err
}

func TestMDNSDiscoveryWithCancelledContext(t *testing.T) {
	service := NewMDNSDiscoveryService(5 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	devices, err := service.DiscoverDevices(ctx)

	// Should handle cancelled context gracefully
	// devices should never be nil, even if cancelled
	if devices == nil {
		t.Error("Expected devices slice to be initialized, got nil")
	}

	// Error might or might not occur depending on timing and network conditions
	// This is acceptable for testing - we just ensure no panic and proper slice initialization
	_ = err
}
