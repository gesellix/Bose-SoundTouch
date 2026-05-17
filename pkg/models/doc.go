// Package models provides data structures for Bose SoundTouch Web API requests and responses.
//
// This package contains all the XML/JSON data models used to communicate with SoundTouch
// devices. These structures handle serialization and deserialization of API data,
// WebSocket events, and device state information.
//
// # Core Data Structures
//
// The package includes models for all major SoundTouch API endpoints:
//
//   - DeviceInfo: Device information and capabilities
//   - NowPlaying: Current playback status and track information
//   - Volume: Volume levels and mute status
//   - Bass: Bass control settings (-9 to +9)
//   - Balance: Balance control settings (-50 to +50)
//   - Sources: Available audio sources (Spotify, Bluetooth, etc.)
//   - Presets: Configured preset buttons
//   - Zone: Multiroom zone configuration
//   - ClockTime/ClockDisplay: Device clock settings
//   - NetworkInfo: Network connectivity information
//
// # Example Usage
//
// Working with device information:
//
//	var info models.DeviceInfo
//	err := xml.Unmarshal(responseData, &info)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Device: %s (Type: %s)\n", info.Name, info.Type)
//
// Volume control:
//
//	volume := models.Volume{
//		ActualVolume:  50,
//		TargetVolume:  50,
//		Muted:        false,
//	}
//
// Creating zone configurations:
//
//	zone := models.Zone{
//		Master: "192.0.2.100",
//		Members: []models.ZoneMember{
//			{IPAddress: "192.0.2.101"},
//			{IPAddress: "192.0.2.102"},
//		},
//	}
//
// # WebSocket Events
//
// The package includes models for real-time WebSocket events:
//
//   - NowPlayingUpdated: Track changes and playback status
//   - VolumeUpdated: Volume and mute state changes
//   - ConnectionStateUpdated: Network connectivity changes
//   - ZoneUpdated: Multiroom zone configuration changes
//
// Example WebSocket event handling:
//
//	switch event := event.(type) {
//	case *models.NowPlayingUpdated:
//		fmt.Printf("Now playing: %s by %s\n", event.Track, event.Artist)
//	case *models.VolumeUpdated:
//		fmt.Printf("Volume: %d (Muted: %t)\n", event.ActualVolume, event.Muted)
//	}
//
// # XML Serialization
//
// Most models support XML marshaling/unmarshaling for API communication:
//
//	// Marshal to XML for API requests
//	data, err := xml.Marshal(volume)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Unmarshal from XML responses
//	var response models.DeviceInfo
//	err = xml.Unmarshal(xmlData, &response)
//	if err != nil {
//		log.Fatal(err)
//	}
//
// # Discovery Models
//
// Device discovery structures:
//
//	device := models.DiscoveredDevice{
//		Name:     "Living Room",
//		Host:     "192.0.2.100",
//		Port:     8090,
//		SerialNo: "AA123456789",
//		Location: "/device.xml",
//	}
//
// # Validation and Constraints
//
// Many models include validation logic and constraints:
//
//   - Volume: 0-100 range with mute support
//   - Bass: -9 to +9 range
//   - Balance: -50 (left) to +50 (right)
//   - Keys: Predefined key constants (PLAY, PAUSE, etc.)
//
// # Thread Safety
//
// All model structures are safe for concurrent read access. For write access
// in concurrent environments, appropriate synchronization should be used.
//
// # Compatibility
//
// These models are compatible with all SoundTouch device types including:
//   - SoundTouch 10, 20, 30 series
//   - SoundTouch Portable
//   - Wave SoundTouch music systems
//   - Other SoundTouch-enabled Bose speakers
package models
