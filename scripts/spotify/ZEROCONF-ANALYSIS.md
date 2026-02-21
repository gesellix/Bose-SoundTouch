# ZeroConf Analysis - Spotify Connect Integration for Bose SoundTouch

## Overview

This document provides a comprehensive analysis of the Spotify Connect ZeroConf protocol as implemented by Bose SoundTouch speakers. ZeroConf enables seamless integration between Spotify clients and SoundTouch hardware without requiring manual configuration.

## What is ZeroConf in This Context?

ZeroConf (Zero Configuration) in the Bose SoundTouch ecosystem is a **Spotify Connect integration protocol** that allows Spotify clients (mobile apps, desktop applications) to discover and control SoundTouch speakers automatically. The speakers expose an HTTP API on **port 8200** that implements Spotify's official ZeroConf specification.

## Network Discovery

### mDNS/Bonjour Advertisement

SoundTouch speakers advertise themselves on the local network using:
- **Service Type**: `_spotify-connect._tcp`
- **Port**: 8200
- **TXT Record**: `CPath=/zc` (points to the ZeroConf endpoint)

This allows Spotify applications to automatically discover available speakers without manual configuration.

### Endpoint Structure

```
http://[SPEAKER_IP]:8200/zc?action=[ACTION]&[PARAMETERS]
```

Example: `http://192.168.1.100:8200/zc?action=getInfo`

## The getInfo Action

### Purpose

The `getInfo` action retrieves comprehensive device information and current status. This is the most commonly used ZeroConf action for:
- Device discovery and identification
- Checking Spotify authentication status
- Retrieving device capabilities
- Monitoring multiroom configurations

### Request Format

```http
GET http://[SPEAKER_IP]:8200/zc?action=getInfo&version=2.10.0
```

The `version` parameter is optional but recommended for compatibility.

### Response Properties

#### Mandatory Fields (Present in All Responses)

| Property | Type | Description |
|----------|------|-------------|
| `status` | Integer | Operation result code (101 = success) |
| `statusString` | String | Human-readable status description |
| `spotifyError` | Integer | Last Spotify SDK error code (0 = no error) |
| `responseSource` | String | Entity identifier (e.g., "Bose") |

#### Device Information Fields

| Property | Required | Type | Description |
|----------|----------|------|-------------|
| `version` | Yes | String | ZeroConf API version (e.g., "2.10.0") |
| `deviceID` | Yes | String | Unique device identifier (MAC-based) |
| `publicKey` | Yes | String | Device's public key for secure communication |
| `remoteName` | Yes | String | User-friendly device name shown in Spotify |
| `deviceType` | No | String | Device category (e.g., "SPEAKER") |
| `brandDisplayName` | Yes | String | Brand name displayed in Spotify apps |
| `modelDisplayName` | No | String | Model name for user display |
| `libraryVersion` | Yes | String | Spotify Connect library version |
| `resolverVersion` | Yes | String | DNS resolution version |
| `groupStatus` | Yes | String | Multiroom status: "NONE", "GROUP", or "SLAVE" |
| `tokenType` | Yes | String | Authentication token type ("accesstoken") |
| `clientID` | Yes | String | Spotify client identifier |
| `productID` | Yes | Integer | Spotify product identifier |
| `scope` | Yes | String | Permission scope (typically "streaming") |
| `availability` | Yes | String | Device availability status |

#### Status Fields

| Property | Required | Type | Description |
|----------|----------|------|-------------|
| `activeUser` | No | String | Currently logged-in Spotify username (if any) |

#### Advanced Fields (Optional)

| Property | Type | Description |
|----------|------|-------------|
| `aliases` | Array | Virtual devices for multiroom zones |
| `supported_drm_media_formats` | Array | Supported audio formats with DRM capabilities |
| `supported_capabilities` | Integer | Bitmasked device capabilities |

### Example Response

```json
{
    "status": 101,
    "statusString": "OK",
    "spotifyError": 0,
    "responseSource": "Bose",
    "version": "2.10.0",
    "deviceID": "0007F537F5ED",
    "deviceType": "SPEAKER",
    "remoteName": "Living Room Speaker",
    "publicKey": "BgIwVfz9ZXQG...",
    "brandDisplayName": "Bose",
    "modelDisplayName": "SoundTouch 30",
    "libraryVersion": "master-v3.15.1-g7890abcd",
    "resolverVersion": "1",
    "groupStatus": "NONE",
    "tokenType": "accesstoken",
    "clientID": "65b708073fc0480ea92a077233ca87bd",
    "productID": 0,
    "scope": "streaming",
    "availability": "",
    "activeUser": "spotify_username",
    "supported_drm_media_formats": [
        {"drm": 0, "formats": 35},
        {"drm": 1, "formats": 35},
        {"drm": 3, "formats": 1168}
    ],
    "supported_capabilities": 1
}
```

## Key Properties Analysis

### Critical Status Indicators

- **`activeUser`**: Most important field for determining if Spotify is active
  - Present and non-empty: Spotify is authenticated and ready
  - Empty or missing: No active Spotify session

- **`remoteName`**: The display name users see in Spotify Connect device lists
  - Should be descriptive and user-friendly
  - Can contain UTF-8 characters and special symbols

### Device Identification

- **`deviceID`**: Unique identifier for targeting specific speakers
  - Typically derived from MAC address
  - Used for device-specific API calls

- **`groupStatus`**: Critical for multiroom functionality
  - `"NONE"`: Standalone device
  - `"GROUP"`: Multiroom master/coordinator
  - `"SLAVE"`: Member of a multiroom group

### Display Properties

- **`brandDisplayName`** and **`modelDisplayName`**: Shown in Spotify client UIs
  - Should be marketing-appropriate names
  - Support UTF-8 for international markets

## Practical Usage Examples

### 1. Status Checking

```bash
# Check if Spotify is active
curl -s "http://192.168.1.100:8200/zc?action=getInfo" | \
  grep -o '"activeUser" *: *"[^"]*"' | \
  sed 's/"activeUser" *: *"//;s/"$//'
```

### 2. Device Discovery

```bash
# Get device name and ID
info=$(curl -s "http://192.168.1.100:8200/zc?action=getInfo")
device_name=$(echo "$info" | grep -o '"remoteName" *: *"[^"]*"' | sed 's/"remoteName" *: *"//;s/"$//')
device_id=$(echo "$info" | grep -o '"deviceID" *: *"[^"]*"' | sed 's/"deviceID" *: *"//;s/"$//')
```

### 3. Multiroom Detection

```bash
# Check multiroom status
group_status=$(curl -s "http://192.168.1.100:8200/zc?action=getInfo" | \
  grep -o '"groupStatus" *: *"[^"]*"' | \
  sed 's/"groupStatus" *: *"//;s/"$//')
```

## Authentication Flow

The ZeroConf API supports the `addUser` action for Spotify authentication:

```bash
curl -X POST "http://192.168.1.100:8200/zc" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "action=addUser&userName=${SPOTIFY_USER}&blob=${ACCESS_TOKEN}&clientKey=&tokenType=accesstoken"
```

### Token Requirements

- **Access Token**: Valid Spotify OAuth access token
- **Username**: Spotify username associated with the token
- **Token Type**: Always "accesstoken" for current implementations
- **Client Key**: Empty string for current protocol version

### Token Lifecycle

1. Tokens expire after 1 hour (3600 seconds)
2. Speakers must be re-primed after reboot
3. Use `getInfo` to verify successful authentication via `activeUser` field

## Security Considerations

### Communication Security

- **Protocol**: HTTP (plain text) is standard, HTTPS supported but optional
- **Network Scope**: Local network only (port 8200 typically not exposed externally)
- **Authentication**: Token-based, no permanent credentials stored

### Best Practices

1. **Token Management**: 
   - Never store long-lived tokens on devices
   - Implement token refresh mechanisms
   - Use centralized token servers when possible

2. **Network Security**:
   - Ensure port 8200 is not accessible from external networks
   - Consider HTTPS for enhanced security
   - Implement proper firewall rules

3. **Error Handling**:
   - Always check `status` and `spotifyError` fields
   - Implement retry mechanisms for network failures
   - Log authentication failures for debugging

## Integration Patterns

### Boot-time Automation

See `spotify-boot-primer.sh` for a complete example of:
1. Waiting for ZeroConf endpoint availability
2. Checking current authentication status
3. Fetching fresh tokens from a management server
4. Automatically priming speakers at startup

### Manual Priming

See `spotify-prime-speaker.sh` for standalone token injection:
1. Validate access tokens against Spotify API
2. Extract username from token metadata
3. Prime individual speakers
4. Verify successful authentication

### Monitoring and Health Checks

```bash
#!/bin/bash
# Health check script
SPEAKER_IP="192.168.1.100"
info=$(curl -sf --max-time 5 "http://${SPEAKER_IP}:8200/zc?action=getInfo" 2>/dev/null)

if [ $? -eq 0 ]; then
    active_user=$(echo "$info" | grep -o '"activeUser" *: *"[^"]*"' | sed 's/"activeUser" *: *"//;s/"$//')
    if [ -n "$active_user" ]; then
        echo "✅ Spotify active (user: $active_user)"
    else
        echo "⚠️  Speaker reachable but Spotify not active"
    fi
else
    echo "❌ Speaker unreachable"
fi
```

## Troubleshooting

### Common Issues

1. **Port 8200 Unreachable**
   - Check network connectivity
   - Verify speaker is powered on
   - Confirm IP address is correct

2. **Empty `activeUser` After Authentication**
   - Wait 2-5 seconds after `addUser` request
   - Verify access token is valid and not expired
   - Check `spotifyError` field for SDK errors

3. **Authentication Failures**
   - Ensure token has correct scopes
   - Verify username matches token owner
   - Check token expiration time

### Diagnostic Commands

```bash
# Test basic connectivity
curl -sf --max-time 5 "http://192.168.1.100:8200/zc?action=getInfo"

# Check detailed response
curl -s "http://192.168.1.100:8200/zc?action=getInfo" | jq .

# Monitor authentication status
while true; do
    active=$(curl -s "http://192.168.1.100:8200/zc?action=getInfo" | \
             grep -o '"activeUser" *: *"[^"]*"' | sed 's/"activeUser" *: *"//;s/"$//')
    echo "$(date): activeUser = '$active'"
    sleep 10
done
```

## References

- [Spotify ZeroConf API Documentation](https://developer.spotify.com/documentation/commercial-hardware/implementation/guides/zeroconf)
- [Bose SoundTouch Toolkit](https://github.com/gesellix/Bose-SoundTouch)
- Scripts in this directory:
  - `spotify-boot-primer.sh`: Automated boot-time priming
  - `spotify-prime-speaker.sh`: Manual speaker priming
  - `spotify-primer.conf.example`: Configuration template

---

*This analysis is based on Spotify's official ZeroConf specification and practical implementation experience with Bose SoundTouch speakers.*