package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

// DeviceMigrationDiagnostic provides detailed analysis of device migration scenarios
type DeviceMigrationDiagnostic struct {
	server *Server
}

// NewDeviceMigrationDiagnostic creates a new diagnostic instance
func NewDeviceMigrationDiagnostic(server *Server) *DeviceMigrationDiagnostic {
	return &DeviceMigrationDiagnostic{server: server}
}

// DiagnoseDeviceMigration analyzes why a specific device might not be migrating correctly
func (d *DeviceMigrationDiagnostic) DiagnoseDeviceMigration(deviceIP string) error {
	log.Printf("=== Device Migration Diagnostic for %s ===", deviceIP)

	// 1. Fetch live device info
	liveInfo, err := d.server.sm.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		log.Printf("❌ Failed to fetch /info from %s: %v", deviceIP, err)
		return fmt.Errorf("cannot fetch /info from %s: %w", deviceIP, err)
	}

	log.Printf("✅ Successfully fetched /info from %s", deviceIP)
	log.Printf("   Device ID (MAC): %s", liveInfo.DeviceID)
	log.Printf("   Device Name: %s", liveInfo.Name)
	log.Printf("   Product: %s %s", liveInfo.Type, liveInfo.ModuleType)
	log.Printf("   Account: %s", liveInfo.MargeAccountUUID)
	log.Printf("   Component Serial: %s", liveInfo.SerialNumber)
	log.Printf("   Primary MAC: %s", liveInfo.GetPrimaryMacAddress())

	// 2. List all existing devices
	allDevices, err := d.server.ds.ListAllDevices()
	if err != nil {
		log.Printf("❌ Failed to list devices: %v", err)
		return fmt.Errorf("failed to list devices: %w", err)
	}

	log.Printf("\n📋 Found %d existing devices in datastore:", len(allDevices))

	devicesByAccount := make(map[string][]models.ServiceDeviceInfo)

	for i := range allDevices {
		device := &allDevices[i]
		devicesByAccount[device.AccountID] = append(devicesByAccount[device.AccountID], *device)
	}

	for accountID, devices := range devicesByAccount {
		log.Printf("   Account %s: %d devices", accountID, len(devices))

		for i := range devices {
			device := &devices[i]
			log.Printf("     %d. %s", i+1, device.DeviceID)
			log.Printf("        Name: %s", device.Name)
			log.Printf("        IP: %s", device.IPAddress)
			log.Printf("        Serial: %s", device.DeviceSerialNumber)
			log.Printf("        MAC: %s", device.MacAddress)
			log.Printf("        Product: %s", device.ProductCode)
			log.Printf("        Discovery: %s", device.DiscoveryMethod)
		}
	}

	// 3. Simulate discovery and check matching
	log.Printf("\n🔍 Testing migration candidate matching:")

	// Test different discovery scenarios
	testDiscoveries := []models.DiscoveredDevice{
		{
			Host:            deviceIP,
			Name:            "Current Discovery",
			SerialNo:        "",
			DiscoveryMethod: "Manual",
		},
		{
			Host:            deviceIP,
			Name:            "With Live Serial",
			SerialNo:        liveInfo.SerialNumber,
			DiscoveryMethod: "UPnP",
		},
	}

	// Add test with different IPs that might match existing devices
	seenIPs := make(map[string]bool)

	for i := range allDevices {
		device := &allDevices[i]
		if device.IPAddress != "" && device.IPAddress != deviceIP && !seenIPs[device.IPAddress] {
			seenIPs[device.IPAddress] = true
			testDiscoveries = append(testDiscoveries, models.DiscoveredDevice{
				Host:            device.IPAddress,
				Name:            "Previous IP Test",
				SerialNo:        "",
				DiscoveryMethod: "Test",
			})
		}
	}

	for i := range testDiscoveries {
		testDiscovery := &testDiscoveries[i]
		log.Printf("\n   Test Scenario %d: %s (IP: %s, Serial: %s)",
			i+1, testDiscovery.Name, testDiscovery.Host, testDiscovery.SerialNo)

		matches := d.server.findAllExistingDeviceVariants(*testDiscovery, liveInfo)
		if len(matches) == 0 {
			log.Printf("     ❌ No migration candidates found")
		} else {
			log.Printf("     ✅ Found %d migration candidate(s):", len(matches))

			for i := range matches {
				match := &matches[i]
				if match.DeviceID == liveInfo.DeviceID {
					log.Printf("       - %s ⚠️  (already uses target MAC)", match.DeviceID)
				} else {
					log.Printf("       - %s", match.DeviceID)
				}
			}
		}
	}

	// 4. Detailed matching analysis
	log.Printf("\n🔬 Detailed Matching Analysis:")
	log.Printf("   Looking for devices that should match MAC %s...", liveInfo.DeviceID)

	potentialMatches := d.findPotentialMatches(allDevices, liveInfo)
	if len(potentialMatches) == 0 {
		log.Printf("   ❌ No potential matches found")
		log.Printf("\n💡 Recommendations:")
		log.Printf("   - This appears to be a completely new device")
		log.Printf("   - Device will be created with MAC-based ID: %s", liveInfo.DeviceID)
		log.Printf("   - Account: %s", liveInfo.MargeAccountUUID)
	} else {
		log.Printf("   ✅ Found %d potential match(es):", len(potentialMatches))

		for i := range potentialMatches {
			d.explainMatch(potentialMatches[i], liveInfo)
		}

		log.Printf("\n💡 Migration Recommendations:")

		for i := range potentialMatches {
			match := potentialMatches[i]
			if match.DeviceID != liveInfo.DeviceID {
				log.Printf("   - Migrate %s → %s", match.DeviceID, liveInfo.DeviceID)
				log.Printf("     Reason: %s", d.getMatchReason(match, liveInfo))
			}
		}
	}

	log.Printf("\n=== End Diagnostic ===")

	return nil
}

// findPotentialMatches finds devices that could potentially be the same device
func (d *DeviceMigrationDiagnostic) findPotentialMatches(allDevices []models.ServiceDeviceInfo, liveInfo *setup.DeviceInfoXML) []models.ServiceDeviceInfo {
	var matches []models.ServiceDeviceInfo

	for i := range allDevices {
		device := &allDevices[i]
		if d.couldBeMatch(*device, liveInfo) {
			matches = append(matches, *device)
		}
	}

	return matches
}

// couldBeMatch determines if a device could potentially be the same physical device
func (d *DeviceMigrationDiagnostic) couldBeMatch(device models.ServiceDeviceInfo, liveInfo *setup.DeviceInfoXML) bool {
	// 1. Serial number match
	if liveInfo.SerialNumber != "" && device.DeviceSerialNumber == liveInfo.SerialNumber {
		return true
	}

	// 2. DeviceID is the serial
	if liveInfo.SerialNumber != "" && device.DeviceID == liveInfo.SerialNumber {
		return true
	}

	// 3. MAC address match
	primaryMAC := liveInfo.GetPrimaryMacAddress()
	if primaryMAC != "" && device.MacAddress == primaryMAC {
		return true
	}

	// 4. DeviceID is already the MAC
	if device.DeviceID == liveInfo.DeviceID {
		return true
	}

	// 5. Name and product similarity
	if liveInfo.Name != "" && device.Name == liveInfo.Name {
		expectedProduct := liveInfo.Type + " " + liveInfo.ModuleType
		if device.ProductCode == expectedProduct ||
			device.ProductCode == liveInfo.Type ||
			strings.Contains(device.ProductCode, liveInfo.Type) ||
			strings.Contains(expectedProduct, device.ProductCode) {
			return true
		}
	}

	// 6. Check if device product serial matches any component
	for _, comp := range liveInfo.Components {
		if comp.SerialNumber != "" && device.ProductSerialNumber == comp.SerialNumber {
			return true
		}
	}

	return false
}

// explainMatch provides detailed explanation of why a device matches
func (d *DeviceMigrationDiagnostic) explainMatch(device models.ServiceDeviceInfo, liveInfo *setup.DeviceInfoXML) {
	log.Printf("     📋 Device: %s", device.DeviceID)
	log.Printf("        Account: %s", device.AccountID)
	log.Printf("        Name: %s → %s", device.Name, liveInfo.Name)
	log.Printf("        IP: %s", device.IPAddress)
	log.Printf("        Serial: %s → %s", device.DeviceSerialNumber, liveInfo.SerialNumber)
	log.Printf("        MAC: %s → %s", device.MacAddress, liveInfo.GetPrimaryMacAddress())
	log.Printf("        Product: %s → %s %s", device.ProductCode, liveInfo.Type, liveInfo.ModuleType)

	reasons := d.getMatchReasons(device, liveInfo)
	for _, reason := range reasons {
		log.Printf("        ✅ %s", reason)
	}
}

// getMatchReason gets the primary reason for a match
func (d *DeviceMigrationDiagnostic) getMatchReason(device models.ServiceDeviceInfo, liveInfo *setup.DeviceInfoXML) string {
	reasons := d.getMatchReasons(device, liveInfo)
	if len(reasons) > 0 {
		return reasons[0]
	}

	return "Unknown match reason"
}

// getMatchReasons gets all reasons why a device matches
func (d *DeviceMigrationDiagnostic) getMatchReasons(device models.ServiceDeviceInfo, liveInfo *setup.DeviceInfoXML) []string {
	var reasons []string

	// Serial number matches
	if liveInfo.SerialNumber != "" && device.DeviceSerialNumber == liveInfo.SerialNumber {
		reasons = append(reasons, "Device serial number matches")
	}

	if liveInfo.SerialNumber != "" && device.DeviceID == liveInfo.SerialNumber {
		reasons = append(reasons, "DeviceID matches component serial")
	}

	// MAC address matches
	primaryMAC := liveInfo.GetPrimaryMacAddress()
	if primaryMAC != "" && device.MacAddress == primaryMAC {
		reasons = append(reasons, "MAC address matches")
	}

	if device.DeviceID == liveInfo.DeviceID {
		reasons = append(reasons, "DeviceID matches (already migrated)")
	}

	// Name and product
	if liveInfo.Name != "" && device.Name == liveInfo.Name {
		expectedProduct := liveInfo.Type + " " + liveInfo.ModuleType
		if device.ProductCode == expectedProduct || device.ProductCode == liveInfo.Type {
			reasons = append(reasons, "Name and product match exactly")
		} else if strings.Contains(device.ProductCode, liveInfo.Type) || strings.Contains(expectedProduct, device.ProductCode) {
			reasons = append(reasons, "Name and product similar")
		}
	}

	// Component serials
	for _, comp := range liveInfo.Components {
		if comp.SerialNumber != "" && device.ProductSerialNumber == comp.SerialNumber {
			reasons = append(reasons, fmt.Sprintf("Product serial matches %s component", comp.Category))
		}
	}

	return reasons
}

// SimulateFullMigration simulates what would happen if migration ran for this device
func (d *DeviceMigrationDiagnostic) SimulateFullMigration(deviceIP string) error {
	log.Printf("=== Migration Simulation for %s ===", deviceIP)

	// Fetch device info
	liveInfo, err := d.server.sm.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		return fmt.Errorf("cannot fetch device info: %w", err)
	}

	// Simulate discovery
	discovery := models.DiscoveredDevice{
		Host:            deviceIP,
		Name:            "Simulated Discovery",
		SerialNo:        "",
		DiscoveryMethod: "Manual",
	}

	log.Printf("Target Device ID: %s", liveInfo.DeviceID)
	log.Printf("Target Account: %s", liveInfo.MargeAccountUUID)

	// Find existing variants
	existingDevices := d.server.findAllExistingDeviceVariants(discovery, liveInfo)

	if len(existingDevices) == 0 {
		log.Printf("✨ This would be a NEW device:")
		log.Printf("   Directory: accounts/%s/devices/%s/", liveInfo.MargeAccountUUID, liveInfo.DeviceID)
	} else {
		log.Printf("🔄 This would MIGRATE %d existing device(s):", len(existingDevices))

		for i := range existingDevices {
			existing := &existingDevices[i]
			if existing.DeviceID != liveInfo.DeviceID {
				log.Printf("   %s → %s", existing.DeviceID, liveInfo.DeviceID)
				log.Printf("     From: accounts/%s/devices/%s/", existing.AccountID, existing.DeviceID)
				log.Printf("     To:   accounts/%s/devices/%s/", liveInfo.MargeAccountUUID, liveInfo.DeviceID)
			} else {
				log.Printf("   %s (already correct)", existing.DeviceID)
			}
		}
	}

	log.Printf("=== End Simulation ===")

	return nil
}

// AnalyzeExistingDevices provides an overview of all devices and potential migration issues
func (d *DeviceMigrationDiagnostic) AnalyzeExistingDevices() error {
	log.Printf("=== Device Migration Analysis ===")

	allDevices, err := d.server.ds.ListAllDevices()
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	log.Printf("📊 Total devices in datastore: %d", len(allDevices))

	// Categorize devices
	var (
		macBasedDevices    []models.ServiceDeviceInfo
		ipBasedDevices     []models.ServiceDeviceInfo
		serialBasedDevices []models.ServiceDeviceInfo
		unknownDevices     []models.ServiceDeviceInfo
	)

	for i := range allDevices {
		device := &allDevices[i]

		deviceID := device.DeviceID
		switch {
		case isMACAddress(deviceID):
			macBasedDevices = append(macBasedDevices, *device)
		case isIPAddress(deviceID):
			ipBasedDevices = append(ipBasedDevices, *device)
		case isSerialNumber(deviceID):
			serialBasedDevices = append(serialBasedDevices, *device)
		default:
			unknownDevices = append(unknownDevices, *device)
		}
	}

	log.Printf("\n📋 Device ID Categories:")
	log.Printf("   ✅ MAC-based: %d (target format)", len(macBasedDevices))
	log.Printf("   🔄 IP-based: %d (needs migration)", len(ipBasedDevices))
	log.Printf("   🔄 Serial-based: %d (needs migration)", len(serialBasedDevices))
	log.Printf("   ❓ Unknown format: %d", len(unknownDevices))

	if len(ipBasedDevices) > 0 {
		log.Printf("\n🔄 IP-based devices (migration candidates):")

		for i := range ipBasedDevices {
			device := &ipBasedDevices[i]
			log.Printf("   %s (%s)", device.DeviceID, device.Name)
		}
	}

	if len(serialBasedDevices) > 0 {
		log.Printf("\n🔄 Serial-based devices (migration candidates):")

		for i := range serialBasedDevices {
			device := &serialBasedDevices[i]
			log.Printf("   %s (%s)", device.DeviceID, device.Name)
		}
	}

	if len(unknownDevices) > 0 {
		log.Printf("\n❓ Unknown format devices:")

		for i := range unknownDevices {
			device := &unknownDevices[i]
			log.Printf("   %s (%s)", device.DeviceID, device.Name)
		}
	}

	log.Printf("=== End Analysis ===")

	return nil
}

// Helper functions
func isMACAddress(s string) bool {
	// AABBCCDDEEFF format
	if len(s) == 12 {
		return isHexOnly(s)
	}

	// AA:BB:CC:DD:EE:FF or AA-BB-CC-DD-EE-FF format
	if len(s) == 17 && (strings.Contains(s, ":") || strings.Contains(s, "-")) {
		s = strings.ReplaceAll(s, "-", ":")

		parts := strings.Split(s, ":")
		if len(parts) != 6 {
			return false
		}

		for _, part := range parts {
			if len(part) != 2 || !isHexOnly(part) {
				return false
			}
		}

		return true
	}

	return false
}

func isHexOnly(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'A' || r > 'F') && (r < 'a' || r > 'f') {
			return false
		}
	}

	return true
}

func isIPAddress(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}

		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}

	return true
}

func isSerialNumber(s string) bool {
	// Heuristic: serial numbers are typically alphanumeric and longer than MAC addresses
	if len(s) < 10 || len(s) > 30 {
		return false
	}

	hasLetter := false
	hasDigit := false

	for _, r := range s {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			return false // Contains non-alphanumeric characters
		}
	}

	return hasLetter && hasDigit
}
