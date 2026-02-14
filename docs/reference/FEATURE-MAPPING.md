# Feature Mapping Guide

This guide demonstrates the comprehensive endpoint-to-feature mapping system that helps you understand exactly what your SoundTouch device can do and how to use it effectively.

## Overview

The SoundTouch API client now includes intelligent feature mapping that:
- **Maps 103+ endpoints** to **15+ functional features**
- **Categorizes capabilities** by type (Core, Audio, Playback, etc.)
- **Identifies device limitations** and missing features
- **Provides personalized recommendations** based on your device
- **Shows exact CLI commands** for each supported feature

## Quick Start

### Basic Feature Overview
```bash
# Get device feature overview (default view)
soundtouch-cli --host 192.168.1.100 supported-urls

# Show detailed feature mapping with CLI commands
soundtouch-cli --host 192.168.1.100 supported-urls --features

# Show complete endpoint list
soundtouch-cli --host 192.168.1.100 supported-urls --verbose

# Get comprehensive device analysis with recommendations
soundtouch-cli --host 192.168.1.100 analyze
```

## Understanding Feature Categories

### ⚡ Core Features (Essential)
Basic device functionality required for operation:
- **Device Information** - Device details, name, identification
- **Device Capabilities** - Feature discovery and endpoint listing
- **Volume Control** - Audio volume management

### 🔊 Audio Features
Sound quality and audio processing:
- **Bass Control** - Bass level adjustment (-9 to +9)
- **Balance Control** - Left/right audio balance (-50 to +50)
- **Advanced Audio Controls** - DSP controls, tone controls, audio processing

### ▶️ Playback Features
Media playback and control:
- **Playback Control** - Play, pause, stop, track navigation
- **Track Information** - Currently playing metadata

### 📱 Sources Features
Audio source management:
- **Audio Sources** - Available sources and source selection
- **Service Availability** - Streaming service status

### 📻 Content Features
Content browsing and discovery:
- **Content Navigation** - Browse music libraries and streaming services
- **Station Management** - Add, remove, and manage radio stations

### ⭐ Preset Features
Favorite content management:
- **Preset Management** - Store and recall favorite content (1-6 slots)

### 🏠 Multiroom Features
Multi-speaker functionality:
- **Multiroom Zones** - Create and manage speaker groups

### 🌐 Network Features
Connectivity and networking:
- **Network Information** - Network configuration and status
- **Bluetooth Connectivity** - Bluetooth device management
- **AirPlay Support** - Apple AirPlay streaming

### ⚙️ System Features
Device system settings:
- **Clock and Time** - Device clock settings
- **Power Management** - Power state and standby control

## Device Analysis Examples

### Premium Device Example
```bash
$ soundtouch-cli --host 192.168.1.100 analyze

🔍 Device Capability Analysis:
  Device ID: 08DF1F0BA325
  Feature Coverage: 87% (13/15 features)
  Device Type: Premium SoundTouch Speaker (Full Feature Set)

✅ All essential features are supported

✅ Available Features (13):
    ⚡ Core: 3 features
    🔊 Audio: 3 features
    ▶️ Playback: 2 features
    📱 Sources: 2 features
    📻 Content: 2 features
    ⭐ Presets: 1 features

💡 Recommendations:
    🏠 This device supports multiroom - you can create speaker groups
       Try: soundtouch-cli zone create --master 192.168.1.100 --members <other-devices>
    ⭐ Save your favorite content as presets for quick access
       Try: soundtouch-cli preset store-current --slot 1
    📻 Browse and discover new content from streaming services
       Try: soundtouch-cli browse tunein, station search-tunein --query jazz
    🔧 Fine-tune your audio with advanced controls
       Try: soundtouch-cli audio dsp get, audio tone get

🚀 Common Commands for This Device:
    • Get device info: soundtouch-cli info get
    • Control volume: soundtouch-cli volume set --level 50
    • Check what's playing: soundtouch-cli play now
    • List audio sources: soundtouch-cli source list
    • Manage presets: soundtouch-cli preset list
    • Adjust bass: soundtouch-cli bass set --level 5
    • Create speaker group: soundtouch-cli zone create
    • Search content: soundtouch-cli station search-tunein --query "classic rock"
```

### Basic Device Example
```bash
$ soundtouch-cli --host 192.168.1.101 analyze

🔍 Device Capability Analysis:
  Device ID: 4C569D123456
  Feature Coverage: 53% (8/15 features)
  Device Type: Basic SoundTouch Speaker

✅ All essential features are supported

❌ Unavailable Features (7):
    • Advanced Audio Controls - DSP controls, tone controls, and audio processing
    • Station Management - Add, remove, and manage radio stations
    • Multiroom Zones - Create and manage speaker groups
    • Network Information - Network configuration and connectivity status
    • Bluetooth Connectivity - Bluetooth pairing and device management
    • AirPlay Support - Apple AirPlay streaming capability
    • Clock and Time - Device clock settings and time display

💡 Recommendations:
    ⭐ Save your favorite content as presets for quick access
       Try: soundtouch-cli preset store-current --slot 1
    📻 Browse and discover new content from streaming services
       Try: soundtouch-cli browse tunein, station search-tunein --query jazz
    ⚠️  No balance control available on this device
```

## Feature Mapping in Code

### Using the Feature Mapping API
```go
package main

import (
    "fmt"
    "github.com/gesellix/bose-soundtouch/pkg/client"
)

func analyzeDevice(host string) {
    // Create client
    c := client.NewClient(&client.Config{Host: host})
    
    // Get supported URLs with feature mapping
    supportedURLs, err := c.GetSupportedURLs()
    if err != nil {
        log.Fatal(err)
    }
    
    // Get device capabilities overview
    completeness, supported, total := supportedURLs.GetFeatureCompleteness()
    fmt.Printf("Device supports %d%% of features (%d/%d)\n", 
        completeness, supported, total)
    
    // Check specific capabilities
    if supportedURLs.HasMultiroomSupport() {
        fmt.Println("✅ Device can create multiroom zones")
    }
    
    if supportedURLs.HasAdvancedAudioSupport() {
        fmt.Println("✅ Device has advanced audio controls")
    }
    
    // Get missing essential features
    missing := supportedURLs.GetMissingEssentialFeatures()
    if len(missing) > 0 {
        fmt.Println("❌ Missing essential features:")
        for _, feature := range missing {
            fmt.Printf("   • %s\n", feature.Name)
        }
    }
    
    // Get features by category
    featuresByCategory := supportedURLs.GetFeaturesByCategory()
    for category, features := range featuresByCategory {
        fmt.Printf("%s: %d features available\n", category, len(features))
    }
    
    // Check for partial implementations
    partial := supportedURLs.GetPartiallyImplementedFeatures()
    for _, feature := range partial {
        fmt.Printf("⚠️ %s is partially supported\n", feature.Name)
    }
}
```

### Custom Feature Analysis
```go
// Check if device supports a specific workflow
func canDoAdvancedAudio(supportedURLs *models.SupportedURLsResponse) bool {
    requiredEndpoints := []string{
        "/audiodspcontrols",
        "/audioproducttonecontrols", 
        "/audioproductlevelcontrols",
    }
    
    for _, endpoint := range requiredEndpoints {
        if !supportedURLs.HasURL(endpoint) {
            return false
        }
    }
    return true
}

// Get device-specific recommendations
func getPersonalizedTips(supportedURLs *models.SupportedURLsResponse) []string {
    var tips []string
    
    if supportedURLs.HasURL("/presets") {
        tips = append(tips, "Set up presets for your favorite stations")
    }
    
    if supportedURLs.HasURL("/setZone") {
        tips = append(tips, "Create multiroom zones for whole-home audio")
    }
    
    if supportedURLs.HasURL("/search") && supportedURLs.HasURL("/addStation") {
        tips = append(tips, "Search and save new radio stations")
    }
    
    return tips
}
```

## CLI Command Reference by Feature

### Core Features
```bash
# Device Information
soundtouch-cli info get                    # Get device details
soundtouch-cli name get                    # Get device name
soundtouch-cli name set --value "Kitchen"  # Set device name

# Capabilities Discovery
soundtouch-cli capabilities                # Get device capabilities
soundtouch-cli supported-urls              # Get supported endpoints
soundtouch-cli supported-urls --features   # Get feature mapping
soundtouch-cli analyze                     # Full device analysis
```

### Audio Control
```bash
# Volume Control (Essential)
soundtouch-cli volume get                  # Get current volume
soundtouch-cli volume set --level 50       # Set volume to 50%
soundtouch-cli volume up                   # Increase volume
soundtouch-cli volume down                 # Decrease volume

# Bass Control
soundtouch-cli bass get                    # Get current bass level
soundtouch-cli bass set --level 3          # Set bass to +3
soundtouch-cli bass up                     # Increase bass
soundtouch-cli bass down                   # Decrease bass

# Balance Control
soundtouch-cli balance get                 # Get current balance
soundtouch-cli balance set --level 10      # Set balance +10 (right)
soundtouch-cli balance left                # Move balance left
soundtouch-cli balance right               # Move balance right

# Advanced Audio Controls
soundtouch-cli audio dsp get               # Get DSP settings
soundtouch-cli audio tone get              # Get tone controls
soundtouch-cli audio level get             # Get level controls
```

### Playback Control
```bash
# Basic Playback (Essential)
soundtouch-cli play start                  # Start playback
soundtouch-cli play stop                   # Stop playback  
soundtouch-cli play pause                  # Pause playback
soundtouch-cli play now                    # Get now playing info

# Key Commands
soundtouch-cli key send --key PLAY         # Send play key
soundtouch-cli key send --key NEXT_TRACK   # Next track
soundtouch-cli key send --key PREV_TRACK   # Previous track
soundtouch-cli key power                   # Power toggle
soundtouch-cli key mute                    # Mute toggle
```

### Source Management
```bash
# Audio Sources
soundtouch-cli source list                 # List available sources
soundtouch-cli source select --source SPOTIFY  # Select Spotify
soundtouch-cli source bluetooth            # Select Bluetooth  
soundtouch-cli source aux                  # Select AUX input

# Service Availability
soundtouch-cli source availability         # Check service status
soundtouch-cli source compare              # Compare sources vs availability
```

### Content & Stations
```bash
# Content Navigation
soundtouch-cli browse tunein               # Browse TuneIn content
soundtouch-cli browse pandora --source-account <account>  # Browse Pandora
soundtouch-cli browse spotify --source-account <account>  # Browse Spotify

# Station Management  
soundtouch-cli station search-tunein --query "jazz"       # Search TuneIn
soundtouch-cli station search-pandora --query "rock" --source-account <account>
soundtouch-cli station add --source TUNEIN --token <token> --name "Jazz FM"
soundtouch-cli station remove --source TUNEIN --location <location>
soundtouch-cli station list --source TUNEIN               # List saved stations
```

### Presets
```bash
# Preset Management
soundtouch-cli preset list                 # List all presets
soundtouch-cli preset select --slot 1      # Select preset 1
soundtouch-cli preset store-current --slot 1  # Store current as preset 1
soundtouch-cli preset remove --slot 1      # Remove preset 1
```

### Multiroom
```bash
# Zone Management
soundtouch-cli zone list                   # List current zones
soundtouch-cli zone create --master 192.168.1.100 --members 192.168.1.101,192.168.1.102
soundtouch-cli zone add --member 192.168.1.103    # Add member to zone
soundtouch-cli zone remove --member 192.168.1.103 # Remove from zone
```

## Feature Detection Patterns

### Checking Device Capabilities
```bash
# Quick capability check
soundtouch-cli supported-urls | grep "Feature Coverage"

# Essential features verification  
soundtouch-cli analyze | grep -A 5 "Missing Essential Features"

# Advanced features check
soundtouch-cli supported-urls --features | grep "Advanced Audio"

# Multiroom capability
soundtouch-cli supported-urls --features | grep "Multiroom"
```

### Device Classification
Based on feature support, devices are automatically classified:

- **Premium SoundTouch Speaker**: Multiroom + Advanced Audio + Full Feature Set
- **Standard SoundTouch Speaker**: Multiroom Capable + Core Features  
- **Basic SoundTouch Speaker**: Streaming + Presets + Core Features
- **Essential SoundTouch Device**: Core Playback Features Only
- **Limited SoundTouch Device**: Minimal Feature Set

## Troubleshooting with Feature Mapping

### Common Issues

**Issue**: "Command not working"
```bash
# Check if feature is supported
soundtouch-cli supported-urls --features | grep -i "bass control"
# If not listed, device doesn't support bass control
```

**Issue**: "Multiroom not available"  
```bash
# Verify multiroom support
soundtouch-cli analyze | grep "Multiroom"
# Check specific endpoints
soundtouch-cli supported-urls --verbose | grep -i zone
```

**Issue**: "Station search failing"
```bash
# Check content navigation support
soundtouch-cli source availability
# Verify streaming service status
soundtouch-cli supported-urls --features | grep "Content Navigation"
```

### Device Recommendations

The feature mapping system provides personalized recommendations:

- **Missing Balance Control**: "No balance control available on this device"
- **Multiroom Available**: "Create speaker groups with other devices"  
- **Advanced Audio**: "Fine-tune sound with DSP controls"
- **Limited Features**: "Consider upgrading for full functionality"

## Best Practices

1. **Always check device capabilities first** with `soundtouch-cli analyze`
2. **Use feature-specific commands** rather than trying unsupported features
3. **Check service availability** before attempting streaming operations
4. **Review recommendations** for optimal device usage
5. **Monitor feature completeness** to understand device limitations

This comprehensive feature mapping system ensures you get the most out of your SoundTouch device by understanding exactly what it can do and how to use it effectively.