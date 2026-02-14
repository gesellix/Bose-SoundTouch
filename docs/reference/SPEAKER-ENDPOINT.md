# SoundTouch Speaker Endpoint Documentation

This document describes the implementation of the `/speaker` endpoint for Bose SoundTouch devices, which enables Text-To-Speech (TTS) notifications and URL content playback.

## Overview

The `/speaker` endpoint is used to play notification content on SoundTouch devices, including:
- Text-To-Speech messages using Google TTS
- Audio content from HTTP/HTTPS URLs
- Notification beeps (via `/playNotification` endpoint)

**Important**: This functionality is primarily supported by ST-10 (Series III) speakers. ST-300 and other models may not support this endpoint despite it appearing in their supported URLs.

## API Reference

### POST /speaker

Plays notification content on the speaker.

**Request Body:**
```xml
<play_info>
  <url>URL_TO_AUDIO_CONTENT</url>
  <app_key>YOUR_APPLICATION_KEY</app_key>
  <service>SERVICE_NAME</service>
  <message>MESSAGE_DESCRIPTION</message>
  <reason>REASON_OR_FILENAME</reason>
  <volume>VOLUME_LEVEL</volume> <!-- Optional: 0-100, omit for current volume -->
</play_info>
```

**Response:**
```xml
<?xml version="1.0" encoding="UTF-8" ?>
<status>/speaker</status>
```

### GET /playNotification

Plays a simple notification beep sound.

**Important**: This endpoint requires a GET request, not POST. Earlier versions of this client library incorrectly used POST and would fail with HTTP 400 status.

**Response:**
```xml
<?xml version="1.0" encoding="UTF-8" ?>
<status>/playNotification</status>
```

## Go Client Library Usage

### Text-To-Speech (TTS)

```go
package main

import (
    "log"
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    config := &client.Config{
        Host: "192.168.1.100",
        Port: 8090,
    }
    
    client := client.NewClient(config)
    
    // Play TTS at current volume
    err := client.PlayTTS("Hello, this is a test message", "YOUR_APP_KEY")
    if err != nil {
        log.Fatal(err)
    }
    
    // Play TTS at specific volume (70)
    err = client.PlayTTS("Volume test message", "YOUR_APP_KEY", 70)
    if err != nil {
        log.Fatal(err)
    }
}
```

### URL Content Playback

```go
func main() {
    config := &client.Config{
        Host: "192.168.1.100",
        Port: 8090,
    }
    
    client := client.NewClient(config)
    
    // Play audio from URL
    err := client.PlayURL(
        "https://example.com/audio.mp3",
        "YOUR_APP_KEY",
        "Music Service",
        "Song Title",
        "Artist Name",
        50, // volume level
    )
    if err != nil {
        log.Fatal(err)
    }
}
```

### Custom PlayInfo

```go
func main() {
    client := client.NewClient(config)
    
    // Create custom play info
    playInfo := models.NewPlayInfo(
        "https://example.com/audio.mp3",
        "YOUR_APP_KEY",
        "Custom Service",
        "Custom Message",
        "Custom Reason",
    ).SetVolume(60)
    
    err := client.PlayCustom(playInfo)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Notification Beep

```go
func main() {
    client := client.NewClient(config)
    
    // Uses GET request (fixed in v2025.02+)
    err := client.PlayNotificationBeep()
    if err != nil {
        log.Fatal(err)
    }
}
```

## CLI Usage

### Text-To-Speech

```bash
# Basic TTS (English)
soundtouch-cli speaker tts --text "Hello World" --app-key YOUR_KEY --host 192.168.1.100

# TTS with volume and language
soundtouch-cli speaker tts \
  --text "Bonjour le monde" \
  --app-key YOUR_KEY \
  --volume 70 \
  --language FR \
  --host 192.168.1.100
```

### URL Content Playback

```bash
# Basic URL playback
soundtouch-cli speaker url \
  --url "https://example.com/audio.mp3" \
  --app-key YOUR_KEY \
  --host 192.168.1.100

# URL playback with custom metadata
soundtouch-cli speaker url \
  --url "https://example.com/song.mp3" \
  --app-key YOUR_KEY \
  --service "My Music Service" \
  --message "Beautiful Song" \
  --reason "Artist Name" \
  --volume 60 \
  --host 192.168.1.100
```

### Notification Beep

```bash
soundtouch-cli speaker beep --host 192.168.1.100
```

### Help

```bash
# General speaker help
soundtouch-cli speaker --help

# Detailed functionality help
soundtouch-cli speaker help

# Command-specific help
soundtouch-cli speaker tts --help
soundtouch-cli speaker url --help
```

## Supported Languages for TTS

The following language codes are supported for Google TTS:

| Code | Language |
|------|----------|
| EN | English |
| DE | German |
| ES | Spanish |
| FR | French |
| IT | Italian |
| NL | Dutch |
| PT | Portuguese |
| RU | Russian |
| ZH | Chinese |
| JA | Japanese |
| KO | Korean |
| AR | Arabic |
| HI | Hindi |
| TH | Thai |

## Behavior Notes

1. **Volume Control**: If a volume is specified, the device will:
   - Switch to the specified volume for playback
   - Automatically restore the previous volume after playback completes
   - If volume is 0 or omitted, content plays at current volume

2. **Content Interruption**: 
   - Currently playing content is paused during notification playback
   - Original content resumes automatically after notification ends
   - If currently playing content is already a notification, you may get an error

3. **Multiroom Behavior**:
   - If the device is a zone master, notifications play on all zone members
   - Volume changes affect all devices in the zone

4. **Now Playing Display**:
   - Service name appears in the "artist" field
   - Message appears in the "album" field  
   - Reason appears in the "track" field
   - Custom artwork can be included in URL-based content

## Error Handling

Common errors and their meanings:

- **Device not found**: Check host/port configuration
- **Endpoint not supported**: Device doesn't support `/speaker` endpoint (common with ST-300)
- **Invalid app key**: App key is required for TTS and URL playback
- **Network timeout**: Check device connectivity
- **Invalid URL**: URL must be accessible and contain valid audio content

## App Key Requirements

Both TTS and URL playback require an `app_key` parameter. This appears to be used for:
- Request authentication/identification
- Rate limiting
- Service tracking

You'll need to provide your own application key. The format and generation method for valid app keys is not documented in the official API.

## Limitations

1. **Device Support**: Limited to specific SoundTouch models (primarily ST-10 Series III)
2. **Audio Formats**: Supported audio formats depend on device capabilities
3. **URL Requirements**: URLs must be publicly accessible (no authentication)
4. **TTS Length**: Very long TTS messages may be truncated
5. **Concurrent Playback**: Cannot play multiple notifications simultaneously

## Integration Examples

### Home Automation

```go
// Doorbell notification
client.PlayTTS("Someone is at the front door", "home-automation-key", 80)

// Security alert
client.PlayURL(
    "https://myserver.com/alerts/security-breach.mp3",
    "security-system-key", 
    "Security System",
    "Alert",
    "Motion detected in restricted area",
    100,
)
```

### Development/Testing

```bash
# Test connectivity
soundtouch-cli speaker beep --host 192.168.1.100

# Test TTS functionality  
soundtouch-cli speaker tts --text "Testing TTS functionality" --app-key test-key --host 192.168.1.100

# Test URL playback
soundtouch-cli speaker url --url "https://www.soundjay.com/misc/sounds/bell-ringing-05.wav" --app-key test-key --host 192.168.1.100
```

## Troubleshooting

1. **Command not found**: Ensure you're using a supported SoundTouch model
2. **No audio output**: Check volume levels and device status
3. **TTS not working**: Verify internet connectivity for Google TTS service
4. **URL content fails**: Ensure URL is accessible and contains valid audio
5. **Volume not restored**: May occur if device is powered off during playback

For more information, see the [SoundTouch WebServices API documentation](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API).