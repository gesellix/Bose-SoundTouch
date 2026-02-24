// Package migration provides device directory migration functionality.
// This package is designed to be easily removable in future releases once
// all devices have been migrated from serial-based to MAC-based directory structures.
//
// TODO: Remove this package after 3-4 releases when most devices are migrated.
package migration

import (
	"log"
	"os"
	"path/filepath"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// Config holds migration configuration
type Config struct {
	// Enabled controls whether migration is active
	Enabled bool
	// DryRun logs what would be migrated without actually doing it
	DryRun bool
}

// Manager handles device directory migrations
type Manager struct {
	datastore *datastore.DataStore
	config    Config
}

// NewManager creates a new migration manager
func NewManager(ds *datastore.DataStore, config Config) *Manager {
	return &Manager{
		datastore: ds,
		config:    config,
	}
}

// MigrateDevicesIfNeeded checks discovered devices and migrates any that need it
func (m *Manager) MigrateDevicesIfNeeded(existingDevices []models.ServiceDeviceInfo, targetDeviceID string) bool {
	if !m.config.Enabled {
		return false
	}

	migrated := false

	for i := range existingDevices {
		existing := &existingDevices[i]
		if existing.DeviceID != targetDeviceID {
			if m.config.DryRun {
				log.Printf("[MIGRATION DRY-RUN] Would migrate device directory: %s -> %s", existing.DeviceID, targetDeviceID)
			} else {
				log.Printf("[MIGRATION] Migrating device directory: %s -> %s", existing.DeviceID, targetDeviceID)

				if m.migrateDeviceDirectory(existing.AccountID, existing.DeviceID, targetDeviceID) {
					migrated = true
				}
			}
		}
	}

	return migrated
}

// migrateDeviceDirectory renames device directory from old ID to new ID
func (m *Manager) migrateDeviceDirectory(accountID, oldDeviceID, newDeviceID string) bool {
	// Use direct paths for migration - don't resolve through mappings
	// because mappings might point new ID back to old directory during migration
	accountDevicesDir := m.datastore.AccountDevicesDir(accountID)
	oldDir := filepath.Join(accountDevicesDir, oldDeviceID)
	newDir := filepath.Join(accountDevicesDir, newDeviceID)

	// Log directory contents before migration
	m.logDirectoryContents("Source directory", oldDir)

	// Check if old directory exists
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		log.Printf("[MIGRATION] Source directory %s does not exist, nothing to migrate", oldDir)
		return false
	}

	// Check if new directory already exists
	if _, err := os.Stat(newDir); err == nil {
		log.Printf("[MIGRATION] Target directory %s already exists, removing it first", newDir)

		if removeErr := os.RemoveAll(newDir); removeErr != nil {
			log.Printf("[MIGRATION ERROR] Failed to remove existing target directory: %v", removeErr)
			return false
		}
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(newDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		log.Printf("[MIGRATION ERROR] Failed to create parent directory %s: %v", parentDir, err)
		return false
	}

	// Rename the entire directory
	if err := os.Rename(oldDir, newDir); err != nil {
		log.Printf("[MIGRATION ERROR] Failed to rename directory from %s to %s: %v", oldDir, newDir, err)
		return false
	}

	log.Printf("[MIGRATION SUCCESS] Migrated device directory: %s -> %s", oldDeviceID, newDeviceID)
	m.logDirectoryContents("Migrated directory", newDir)

	return true
}

// logDirectoryContents logs the contents of a directory for debugging
func (m *Manager) logDirectoryContents(label, dirPath string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		log.Printf("[MIGRATION] %s (%s): Error reading - %v", label, dirPath, err)
		return
	}

	log.Printf("[MIGRATION] %s (%s): %d files", label, dirPath, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err == nil {
				log.Printf("[MIGRATION]   - %s (%d bytes)", entry.Name(), info.Size())
			} else {
				log.Printf("[MIGRATION]   - %s (size unknown)", entry.Name())
			}
		}
	}
}

// GetStats returns migration statistics
func (m *Manager) GetStats() Stats {
	// This could be extended to track migration metrics
	return Stats{
		Enabled: m.config.Enabled,
		DryRun:  m.config.DryRun,
	}
}

// Stats holds migration statistics
type Stats struct {
	Enabled bool
	DryRun  bool
}

// IsLegacyDeviceID checks if a device ID appears to be legacy (non-MAC format)
func IsLegacyDeviceID(deviceID string) bool {
	// Serial numbers typically start with I or K and are long
	if len(deviceID) > 15 && (deviceID[0] == 'I' || deviceID[0] == 'K') {
		return true
	}

	// IP addresses
	if isIPAddress(deviceID) {
		return true
	}

	// Assume MAC addresses are 12 hex characters
	if len(deviceID) == 12 && isHexString(deviceID) {
		return false // This is likely a MAC address
	}

	// Other formats are considered legacy
	return true
}

// isIPAddress checks if a string looks like an IP address
func isIPAddress(s string) bool {
	if len(s) < 7 || len(s) > 15 {
		return false
	}

	dotCount := 0

	for _, c := range s {
		if c == '.' {
			dotCount++
		} else if c < '0' || c > '9' {
			return false
		}
	}

	return dotCount == 3
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'A' || c > 'F') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}
