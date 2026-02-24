// Package main provides a debug tool for analyzing device consolidation and migration scenarios.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: debug-consolidation <data-directory>")
		fmt.Println("Example: debug-consolidation /var/lib/soundtouch-service")
		os.Exit(1)
	}

	dataDir := os.Args[1]

	fmt.Printf("🔍 Analyzing device consolidation in: %s\n", dataDir)

	// Initialize datastore
	ds := datastore.NewDataStore(dataDir)

	// List all devices
	devices, err := ds.ListAllDevices()
	if err != nil {
		log.Fatalf("Failed to list devices: %v", err)
	}

	fmt.Printf("📱 Found %d device entries:\n", len(devices))

	for i := range devices {
		device := &devices[i]
		fmt.Printf("  %d. %s (Account: %s)\n", i+1, device.DeviceID, device.AccountID)
		fmt.Printf("     Name: %s\n", device.Name)
		fmt.Printf("     IP: %s, MAC: %s, Serial: %s\n",
			device.IPAddress, device.MacAddress, device.DeviceSerialNumber)

		// Check directory contents
		deviceDir := ds.AccountDeviceDir(device.AccountID, device.DeviceID)
		analyzeDeviceDirectory(deviceDir, device.DeviceID)
		fmt.Println()
	}

	// Group devices by potential physical device
	fmt.Println("🔄 Analyzing potential consolidation opportunities:")

	deviceGroups := groupDevicesByIdentity(devices)

	for i, group := range deviceGroups {
		if len(group) <= 1 {
			continue
		}

		fmt.Printf("  Group %d - %d entries for same physical device:\n", i+1, len(group))

		for i := range group {
			device := &group[i]
			deviceDir := ds.AccountDeviceDir(device.AccountID, device.DeviceID)
			fileCount := countFiles(deviceDir)
			fmt.Printf("    - %s (%d files)\n", device.DeviceID, fileCount)
		}

		// Recommend consolidation target
		macDevice := findMACBasedDevice(group)
		if macDevice != nil {
			fmt.Printf("    → Recommend keeping: %s (MAC-based)\n", macDevice.DeviceID)
		} else {
			fmt.Printf("    → No clear MAC-based target found\n")
		}

		fmt.Println()
	}
}

func analyzeDeviceDirectory(dirPath, deviceID string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		fmt.Printf("     Directory: %s (Error: %v)\n", dirPath, err)
		return
	}

	fmt.Printf("     Directory: %s (%d files)\n", dirPath, len(entries))

	// Check for important files
	importantFiles := []string{"DeviceInfo.xml", "Presets.xml", "Recents.xml", "Sources.xml"}
	for _, fileName := range importantFiles {
		filePath := filepath.Join(dirPath, fileName)
		if stat, err := os.Stat(filePath); err == nil {
			status := "✓"
			if stat.Size() == 0 {
				status = "⚠️ (empty)"
			} else if stat.Size() < 100 {
				status = "⚠️ (very small)"
			}

			fmt.Printf("       %s %s (%d bytes)\n", status, fileName, stat.Size())
		} else {
			fmt.Printf("       ❌ %s (missing)\n", fileName)
		}
	}

	// Check if deviceID looks like MAC address
	if isLikelyMACAddress(deviceID) {
		fmt.Printf("       📍 Device ID appears to be MAC address format\n")
	} else {
		fmt.Printf("       📍 Device ID appears to be %s format\n", guessIDType(deviceID))
	}
}

func countFiles(dirPath string) int {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0
	}

	count := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}

	return count
}

func groupDevicesByIdentity(devices []models.ServiceDeviceInfo) [][]models.ServiceDeviceInfo {
	var groups [][]models.ServiceDeviceInfo

	// Simple grouping by MAC address and serial number
	macGroups := make(map[string][]models.ServiceDeviceInfo)
	serialGroups := make(map[string][]models.ServiceDeviceInfo)
	ipGroups := make(map[string][]models.ServiceDeviceInfo)

	for i := range devices {
		device := &devices[i]
		// Group by MAC address
		if device.MacAddress != "" {
			macGroups[device.MacAddress] = append(macGroups[device.MacAddress], *device)
		}

		// Group by serial number
		if device.DeviceSerialNumber != "" {
			serialGroups[device.DeviceSerialNumber] = append(serialGroups[device.DeviceSerialNumber], *device)
		}

		// Group by IP address
		if device.IPAddress != "" {
			ipGroups[device.IPAddress] = append(ipGroups[device.IPAddress], *device)
		}
	}

	// Merge groups - prioritize MAC address grouping
	processed := make(map[string]bool)

	for _, macDevices := range macGroups {
		if len(macDevices) > 1 {
			groups = append(groups, macDevices)
			for i := range macDevices {
				processed[macDevices[i].DeviceID] = true
			}
		}
	}

	// Check for serial number groups not already processed
	for _, serialDevices := range serialGroups {
		if len(serialDevices) > 1 {
			unprocessed := []models.ServiceDeviceInfo{}

			for i := range serialDevices {
				if !processed[serialDevices[i].DeviceID] {
					unprocessed = append(unprocessed, serialDevices[i])
				}
			}

			if len(unprocessed) > 1 {
				groups = append(groups, unprocessed)
				for i := range unprocessed {
					processed[unprocessed[i].DeviceID] = true
				}
			}
		}
	}

	return groups
}

func findMACBasedDevice(devices []models.ServiceDeviceInfo) *models.ServiceDeviceInfo {
	for i := range devices {
		if isLikelyMACAddress(devices[i].DeviceID) {
			return &devices[i]
		}
	}

	return nil
}

func isLikelyMACAddress(id string) bool {
	// MAC addresses are typically 12 hex characters without separators
	// or 17 characters with separators (XX:XX:XX:XX:XX:XX)
	if len(id) == 12 {
		for _, c := range id {
			if (c < '0' || c > '9') && (c < 'A' || c > 'F') && (c < 'a' || c > 'f') {
				return false
			}
		}

		return true
	}

	return false
}

func guessIDType(id string) string {
	if len(id) > 15 && (id[0] == 'I' || id[0] == 'K') {
		return "serial number"
	}

	// Check if it looks like an IP address
	if len(id) >= 7 && len(id) <= 15 {
		dotCount := 0

		for _, c := range id {
			if c == '.' {
				dotCount++
			} else if c < '0' || c > '9' {
				break
			}
		}

		if dotCount == 3 {
			return "IP address"
		}
	}

	return "unknown"
}
