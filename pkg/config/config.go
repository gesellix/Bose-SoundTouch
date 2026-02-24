// Package config provides configuration management for the Bose SoundTouch Go library.
package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// Config holds configuration for the SoundTouch application
type Config struct {
	// Discovery settings
	DiscoveryTimeout time.Duration `env:"DISCOVERY_TIMEOUT" default:"5s"`
	UPnPEnabled      bool          `env:"UPNP_ENABLED" default:"true"`
	MDNSEnabled      bool          `env:"MDNS_ENABLED" default:"true"`

	// Preferred devices from .env file
	PreferredDevices []DeviceConfig `env:"PREFERRED_DEVICES"`

	// HTTP Client settings
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"10s"`
	UserAgent   string        `env:"USER_AGENT" default:"Bose-SoundTouch-Go-Client/1.0"`

	// Cache settings
	CacheEnabled bool          `env:"CACHE_ENABLED" default:"true"`
	CacheTTL     time.Duration `env:"CACHE_TTL" default:"30s"`

	// Migration settings (TODO: Remove after 3-4 releases when all devices are migrated)
	MigrationEnabled bool `env:"MIGRATION_ENABLED" default:"true"`
	MigrationDryRun  bool `env:"MIGRATION_DRY_RUN" default:"false"`
}

// DeviceConfig represents a configured SoundTouch device
type DeviceConfig struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		DiscoveryTimeout: 5 * time.Second,
		UPnPEnabled:      true,
		MDNSEnabled:      true,
		PreferredDevices: []DeviceConfig{},
		HTTPTimeout:      10 * time.Second,
		UserAgent:        "Bose-SoundTouch-Go-Client/1.0",
		MigrationEnabled: true, // TODO: Change to false after 3-4 releases
		MigrationDryRun:  false,
		CacheEnabled:     true,
		CacheTTL:         30 * time.Second,
	}
}

// LoadFromEnv loads configuration from environment variables and .env file
func LoadFromEnv() (*Config, error) {
	config := DefaultConfig()

	// Load .env file if it exists
	_ = loadDotEnv() // Don't fail if .env doesn't exist, just continue with defaults

	// Parse environment variables
	if timeout := os.Getenv("DISCOVERY_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			config.DiscoveryTimeout = d
		}
	}

	if upnp := os.Getenv("UPNP_ENABLED"); upnp != "" {
		config.UPnPEnabled = upnp == "true" || upnp == "1"
	}

	if mdns := os.Getenv("MDNS_ENABLED"); mdns != "" {
		config.MDNSEnabled = mdns == "true" || mdns == "1"
	}

	if timeout := os.Getenv("HTTP_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			config.HTTPTimeout = d
		}
	}

	if userAgent := os.Getenv("USER_AGENT"); userAgent != "" {
		config.UserAgent = userAgent
	}

	if cache := os.Getenv("CACHE_ENABLED"); cache != "" {
		config.CacheEnabled = cache == "true" || cache == "1"
	}

	if cacheTTL := os.Getenv("CACHE_TTL"); cacheTTL != "" {
		if d, err := time.ParseDuration(cacheTTL); err == nil {
			config.CacheTTL = d
		}
	}

	// Parse preferred devices
	devices, err := parsePreferredDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to parse preferred devices: %w", err)
	}

	config.PreferredDevices = devices

	return config, nil
}

// GetPreferredDevicesAsDiscovered converts configured devices to DiscoveredDevice format
func (c *Config) GetPreferredDevicesAsDiscovered() []*models.DiscoveredDevice {
	devices := make([]*models.DiscoveredDevice, 0, len(c.PreferredDevices))

	for _, device := range c.PreferredDevices {
		discovered := &models.DiscoveredDevice{
			Name:            device.Name,
			Host:            device.Host,
			Port:            device.Port,
			LastSeen:        time.Now(),
			DiscoveryMethod: "Configuration",
			APIBaseURL:      fmt.Sprintf("http://%s:%d/", device.Host, device.Port),
			InfoURL:         fmt.Sprintf("http://%s:%d/info", device.Host, device.Port),
			ConfigName:      device.Name,
		}
		devices = append(devices, discovered)
	}

	return devices
}

// loadDotEnv loads variables from .env file
func loadDotEnv() error {
	file, err := os.Open(".env")
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 {
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}

		// Set environment variable if not already set
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}

	return scanner.Err()
}

// parsePreferredDevices parses PREFERRED_DEVICES from environment
func parsePreferredDevices() ([]DeviceConfig, error) {
	devicesEnv := os.Getenv("PREFERRED_DEVICES")
	if devicesEnv == "" {
		return []DeviceConfig{}, nil
	}

	var devices []DeviceConfig

	// Split by semicolon for multiple devices
	deviceStrings := strings.Split(devicesEnv, ";")

	for _, deviceStr := range deviceStrings {
		deviceStr = strings.TrimSpace(deviceStr)
		if deviceStr == "" {
			continue
		}

		device, err := parseDeviceString(deviceStr)
		if err != nil {
			return nil, fmt.Errorf("invalid device configuration '%s': %w", deviceStr, err)
		}

		devices = append(devices, device)
	}

	return devices, nil
}

// parseDeviceString parses a single device string in format "name@host:port" or "host:port" or "host"
func parseDeviceString(deviceStr string) (DeviceConfig, error) {
	device := DeviceConfig{
		Port: 8090, // Default SoundTouch port
	}

	// Check if name is specified (name@host:port)
	if strings.Contains(deviceStr, "@") {
		parts := strings.SplitN(deviceStr, "@", 2)
		device.Name = strings.TrimSpace(parts[0])
		deviceStr = strings.TrimSpace(parts[1])
	}

	// Parse host:port or just host
	if strings.Contains(deviceStr, ":") {
		host, portStr, err := net.SplitHostPort(deviceStr)
		if err != nil {
			return device, fmt.Errorf("invalid host:port format: %w", err)
		}

		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			return device, fmt.Errorf("invalid port number: %s", portStr)
		}

		device.Host = host
		device.Port = port
	} else {
		device.Host = deviceStr
	}

	// Validate host
	if device.Host == "" {
		return device, fmt.Errorf("host cannot be empty")
	}

	// Set default name if not provided
	if device.Name == "" {
		device.Name = fmt.Sprintf("SoundTouch-%s", device.Host)
	}

	return device, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.DiscoveryTimeout <= 0 {
		return fmt.Errorf("discovery timeout must be positive")
	}

	if c.HTTPTimeout <= 0 {
		return fmt.Errorf("HTTP timeout must be positive")
	}

	if c.CacheTTL <= 0 {
		return fmt.Errorf("cache TTL must be positive")
	}

	for i, device := range c.PreferredDevices {
		if device.Host == "" {
			return fmt.Errorf("device %d: host cannot be empty", i)
		}

		if device.Port <= 0 || device.Port > 65535 {
			return fmt.Errorf("device %d: invalid port %d", i, device.Port)
		}
	}

	return nil
}
