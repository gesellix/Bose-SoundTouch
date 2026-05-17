// Package main provides an example of using zone slave operations.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
)

func main() {
	// Configure your device
	deviceIP := "192.0.2.100" // Replace with your SoundTouch device IP

	// Create client
	soundtouchClient := client.NewClientFromHost(deviceIP)

	fmt.Println("🎵 Bose SoundTouch Zone Slave Operations Example")
	fmt.Println("==============================================")

	// Example 1: Add a slave to an existing zone using official /addZoneSlave endpoint
	fmt.Println("\n1. Adding slave to zone using official API...")

	masterDeviceID := "ABCD1234EFGH" // Replace with actual master device ID
	slaveDeviceID := "WXYZ5678IJKL"  // Replace with actual slave device ID
	slaveIP := "192.0.2.101"         // Replace with actual slave IP

	err := soundtouchClient.AddZoneSlave(masterDeviceID, slaveDeviceID, slaveIP)
	if err != nil {
		log.Printf("❌ Failed to add zone slave: %v", err)
	} else {
		fmt.Printf("✅ Successfully added slave '%s' to master '%s'\n", slaveDeviceID, masterDeviceID)
	}

	// Wait a moment for the zone change to take effect
	time.Sleep(2 * time.Second)

	// Example 2: Check zone status after adding slave
	fmt.Println("\n2. Checking zone status...")

	zone, err := soundtouchClient.GetZone()
	if err != nil {
		log.Printf("❌ Failed to get zone info: %v", err)
	} else {
		fmt.Printf("📡 Zone Status: %s\n", zone.String())
		fmt.Printf("   Total devices: %d\n", zone.GetTotalDeviceCount())

		for _, member := range zone.Members {
			fmt.Printf("   Member: %s (%s)\n", member.DeviceID, member.IP)
		}
	}

	// Example 3: Add slave by device ID only (without IP)
	fmt.Println("\n3. Adding another slave by device ID only...")

	anotherSlaveID := "PQRS9012MNOP" // Replace with actual device ID

	err = soundtouchClient.AddZoneSlaveByDeviceID(masterDeviceID, anotherSlaveID)
	if err != nil {
		log.Printf("❌ Failed to add zone slave by ID: %v", err)
	} else {
		fmt.Printf("✅ Successfully added slave '%s' to master '%s' (by ID only)\n", anotherSlaveID, masterDeviceID)
	}

	time.Sleep(2 * time.Second)

	// Example 4: Remove a slave from the zone using official /removeZoneSlave endpoint
	fmt.Println("\n4. Removing slave from zone using official API...")

	err = soundtouchClient.RemoveZoneSlave(masterDeviceID, slaveDeviceID, slaveIP)
	if err != nil {
		log.Printf("❌ Failed to remove zone slave: %v", err)
	} else {
		fmt.Printf("✅ Successfully removed slave '%s' from master '%s'\n", slaveDeviceID, masterDeviceID)
	}

	time.Sleep(2 * time.Second)

	// Example 5: Remove slave by device ID only
	fmt.Println("\n5. Removing another slave by device ID only...")

	err = soundtouchClient.RemoveZoneSlaveByDeviceID(masterDeviceID, anotherSlaveID)
	if err != nil {
		log.Printf("❌ Failed to remove zone slave by ID: %v", err)
	} else {
		fmt.Printf("✅ Successfully removed slave '%s' from master '%s' (by ID only)\n", anotherSlaveID, masterDeviceID)
	}

	// Example 6: Final zone status check
	fmt.Println("\n6. Final zone status...")

	finalZone, err := soundtouchClient.GetZone()
	if err != nil {
		log.Printf("❌ Failed to get final zone info: %v", err)
	} else {
		fmt.Printf("📡 Final Zone Status: %s\n", finalZone.String())

		if finalZone.IsStandalone() {
			fmt.Println("   Device is now standalone (no zone)")
		} else {
			fmt.Printf("   Zone has %d total devices\n", finalZone.GetTotalDeviceCount())
		}
	}

	// Example 7: Comparison with high-level zone API
	fmt.Println("\n7. Comparison: High-level zone API (enhanced functionality)...")
	fmt.Println("   For more complex zone operations, you can also use:")
	fmt.Printf("   - soundtouchClient.CreateZoneWithIPs(master, []string{slave1, slave2})\n")
	fmt.Printf("   - soundtouchClient.AddToZone(master, slave)\n")
	fmt.Printf("   - soundtouchClient.RemoveFromZone(master, slave)\n")
	fmt.Printf("   - soundtouchClient.DissolveZone(master)\n")

	fmt.Println("\n🎉 Zone slave operations example completed!")

	// Example 8: Error handling demonstration
	fmt.Println("\n8. Error handling example...")

	// Try to add a non-existent device to demonstrate error handling
	err = soundtouchClient.AddZoneSlave("INVALID123", "NOTFOUND456", "192.0.2.999")
	if err != nil {
		fmt.Printf("⚠️  Expected error for invalid operation: %v\n", err)
		fmt.Println("   This demonstrates proper error handling for invalid device IDs or IPs")
	}
}

// Notes for usage:
//
// 1. Replace the device IPs and IDs with your actual SoundTouch devices
// 2. Ensure devices are on the same network and powered on
// 3. The master device should be capable of creating zones
// 4. Zone slave operations require exact device IDs (MAC addresses)
// 5. IP addresses are optional but recommended for faster operations
//
// To get device IDs:
//   info, _ := soundtouchClient.GetDeviceInfo()
//   deviceID := info.DeviceID
//
// To discover devices on your network:
//   Use the discovery package or the soundtouch-cli discover command
//
// Official API endpoints implemented:
//   POST /addZoneSlave    - Add individual slave to existing zone
//   POST /removeZoneSlave - Remove individual slave from existing zone
//
// These complement the high-level zone management API:
//   GET /getZone - Get zone information
//   POST /setZone - Create/modify zones with multiple members
