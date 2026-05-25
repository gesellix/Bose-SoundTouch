---
title: "SCMUDC Events Analysis"
sidebar:
  exclude: true
---
## Overview

SCMUDC (Sound Control Management Usage Data Collection) events are telemetry data sent from SoundTouch devices to `events.api.bosecm.com` via `/v1/scmudc/{deviceId}` endpoints. These events track user interactions and device behaviors for analytics and monitoring.

## Event Origins

Analysis of recorded interactions reveals three distinct origins for device events:

### 1. `"gabbo"` - SoundTouch App (Mobile/Desktop)
- **Source**: Remote control via SoundTouch mobile/desktop applications
- **Frequency**: Highest (primary control method)
- **Event Types**: User-initiated actions through app interface
- **Button Abstraction**: App UI elements (not physical buttons)

**Common Events**:
- `power-pressed` → Power on/off via app
- `play-pressed` → Play control
- `pause-pressed` → Pause control  
- `skip-forward-pressed` → Next track
- `stop-pressed` → Stop playback

### 2. `"console"` - Device Hardware Controls
- **Source**: Physical buttons and controls on the speaker device
- **Frequency**: Lower (secondary control method)
- **Event Types**: Direct hardware interaction
- **Physical Controls**: Actual buttons, knobs, or touch interfaces on device

**Common Events**:
- `preset-pressed` → Physical preset buttons (PRESET_1, PRESET_5, etc.)
- `power-pressed` → Hardware power button

### 3. `"device"` - Internal System Actions
- **Source**: Device's internal software systems
- **Frequency**: Automatic responses to user actions
- **Event Types**: System-generated events, content playback
- **Rich Content**: Base64-encoded XML with detailed metadata

**Common Events**:
- `play-item` → Automatic content playback responses
- `preset-assigned` → System preset assignments

## Event Data Structure

### Standard Button Events (gabbo/console)
```json
{
  "data": {
    "buttonId": "POWER|PLAY|PAUSE|PRESET_5|etc",
    "origin": "gabbo|console"
  },
  "type": "power-pressed|play-pressed|pause-pressed|preset-pressed|etc"
}
```

### Device Content Events
```json
{
  "data": {
    "contentItem": "PD94bWwgdmVyc2lvbj0...", // Base64-encoded XML
    "origin": "device",
    "preset": "none|P1|P5|etc"
  },
  "type": "play-item|preset-assigned"
}
```

## Content Item Structure

Device events include Base64-encoded XML with rich content metadata:

```xml
<ContentItem source="SPOTIFY" type="tracklisturl" 
             location="/playback/container/c3BvdGlmeTpwbGF5bGlzdDox..." 
             sourceAccount="gesellix" isPresetable="true">
    <itemName>Billie Eilish - bad guy (instrumental version)</itemName>
    <containerArt>https://i.scdn.co/image/ab67616d0000b273...</containerArt>
</ContentItem>
```

**Key Fields**:
- `source`: Music service (SPOTIFY, PANDORA, etc.)
- `itemName`: Track/playlist/station name
- `sourceAccount`: User account on the service
- `location`: Service-specific content identifier
- `containerArt`: Album/playlist artwork URL
- `isPresetable`: Whether content can be saved as preset

## Usage Patterns

### Control Method Preferences
1. **Primary**: SoundTouch App (`gabbo`) - Most frequent interactions
2. **Secondary**: Device Hardware (`console`) - Occasional direct control
3. **Automatic**: Internal System (`device`) - Background responses

### Event Flow
1. User triggers action via app or hardware
2. Device processes request and begins playback
3. Device sends content event with full metadata
4. System continues tracking playback state

## Telemetry Insights

### User Behavior Analytics
- **Interface Preference**: App vs. hardware control usage ratios
- **Feature Usage**: Most/least used controls and functions
- **Content Patterns**: Music service preferences, playlist usage

### Device Health Monitoring
- **Interaction Frequency**: Normal vs. abnormal usage patterns
- **Error Detection**: Failed commands or unusual event sequences
- **Performance**: Response times between user action and system response

### Service Integration Analysis
- **Music Services**: Spotify dominance, other service usage
- **Account Mapping**: User accounts across different services
- **Content Types**: Music vs. radio vs. podcast preferences

## Data Quality Notes

- All events include comprehensive device information (deviceID, serialNumber, softwareVersion)
- Timestamps include both UTC time and device monotonic time
- Events are batched and sent with consistent protocol versioning
- Content metadata is rich and includes artwork URLs for UI enhancement

## Security Considerations

- Events include user account information and listening habits
- Device serial numbers and unique identifiers are transmitted
- Content location data could reveal usage patterns
- Data should be handled according to privacy regulations

## Technical Implementation Notes

- Endpoint: `POST /v1/scmudc/{deviceId}`
- Protocol Version: 3.1 (current)
- Encoding: JSON with Base64-encoded XML payloads
- Authentication: Bearer token authorization
- Content-Type: `text/json; charset=utf-8`
