// Package soundtouch provides a comprehensive Go library, CLI tool, and local service for controlling and emulating Bose SoundTouch devices.
//
// This project implements the complete Bose SoundTouch Web API, enabling programmatic control
// of SoundTouch speakers including playback control, volume management, source selection,
// multiroom zone management, and real-time event monitoring.
//
// It also provides a local service (`soundtouch-service`) that can emulate the Bose Cloud,
// allowing for offline control and enhanced debugging through HTTP interaction recording.
//
// # Quick Start
//
// Install the library:
//
//	go get github.com/gesellix/bose-soundtouch
//
// Basic usage example:
//
//	package main
//
//	import (
//		"fmt"
//		"log"
//
//		"github.com/gesellix/bose-soundtouch/pkg/client"
//	)
//
//	func main() {
//		// Create a client for your SoundTouch device
//		config := &client.Config{
//			Host: "192.168.1.100",
//			Port: 8090,
//		}
//		client := client.NewClient(config)
//
//		// Get device information
//		info, err := client.GetInfo()
//		if err != nil {
//			log.Fatal(err)
//		}
//		fmt.Printf("Device: %s\n", info.Name)
//
//		// Control playback
//		err = client.Play()
//		if err != nil {
//			log.Fatal(err)
//		}
//	}
//
// # SoundTouch Service
//
// The `soundtouch-service` provides several advanced features:
//
//   - Bose Cloud Emulation: Allows speakers to work without an internet connection.
//   - HTTP Interaction Recording: Captures all traffic as IntelliJ-compatible .http files.
//   - Speaker Migration: Automated tools to redirect speakers to the local service.
//   - Web Interface: A management dashboard for proxy settings and speaker setup.
//
// Install the service:
//
//	go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-service@latest
//
// # CLI Tool
//
// The package includes a comprehensive CLI tool for device control:
//
//	# Install the CLI
//	go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-cli@latest
//
//	# Discover devices
//	soundtouch-cli discover devices
//
//	# Control a device
//	soundtouch-cli --host 192.168.1.100 play start
//
// # Supported Features
//
//   - ✅ Device Information & Capabilities
//   - ✅ Playback, Volume, Bass, and Balance Control
//   - ✅ Source Selection & Preset Management
//   - ✅ Real-time WebSocket Events
//   - ✅ Multiroom Zone Management
//   - ✅ Device Discovery (UPnP/SSDP and mDNS)
//   - ✅ Local Cloud Emulation (soundtouch-service)
//   - ✅ HTTP Traffic Recording & Sanitization
//   - ✅ Automated Speaker Migration & Revert
//
// # Package Structure
//
//   - client: HTTP client for SoundTouch Web API
//   - discovery: Device discovery using UPnP/SSDP and mDNS
//   - models: Data structures for API requests/responses
//   - service: Core logic for the soundtouch-service (proxy, recording, setup)
//   - cmd/soundtouch-cli: Command-line interface tool
//   - cmd/soundtouch-service: Local cloud emulation service
//
// # Implementation Notes
//
// This project is an independent effort to preserve the functionality of Bose SoundTouch
// devices and provide enhanced debugging and control capabilities. It is not
// affiliated with or endorsed by Bose Corporation.
//
// For detailed API documentation, examples, and advanced usage patterns, visit:
// https://pkg.go.dev/github.com/gesellix/bose-soundtouch
package soundtouch
