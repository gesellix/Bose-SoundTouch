# Introspect Endpoint Example

This example demonstrates how to use the `/introspect` endpoint to get detailed information about music service states and capabilities on your SoundTouch device.

## What is the Introspect Endpoint?

The introspect endpoint provides detailed information about music services (like Spotify, Pandora, TuneIn) including:

- **Service State**: Active, Inactive, or InactiveUnselected
- **User Information**: Associated account names
- **Playback Status**: Currently playing content and URIs
- **Service Capabilities**: Skip, seek, resume support
- **Token Information**: Authentication token status
- **Content History**: History size limits
- **Subscription Details**: Premium/free account status

## Usage

```bash
# Basic usage - check Spotify status
go run main.go -host 192.168.1.100

# Check specific service with account
go run main.go -host 192.168.1.100 -source SPOTIFY -account "your_spotify_username"

# Check Pandora service
go run main.go -host 192.168.1.100 -source PANDORA

# Check TuneIn radio
go run main.go -host 192.168.1.100 -source TUNEIN

# Custom timeout
go run main.go -host 192.168.1.100 -timeout 5s
```

## Command Line Options

- `-host` - **Required**: SoundTouch device IP address
- `-source` - Music service to introspect (default: `SPOTIFY`)
  - Supported: `SPOTIFY`, `PANDORA`, `TUNEIN`, `AMAZON`, `DEEZER`, etc.
- `-account` - Source account name (optional)
- `-timeout` - Request timeout (default: `10s`)

## Example Output

```
Getting introspect data for SPOTIFY

=== SPOTIFY Service Introspect Data ===
State: InactiveUnselected
User: SpotifyConnectUserName
Currently Playing: false
Current Content: 
Shuffle Mode: OFF
Subscription Type: 

=== Service State ===
❌ Service is INACTIVE

=== Service Capabilities ===
❌ Skip Previous not supported
❌ Seek not supported
✅ Resume supported
✅ Data collection enabled

=== Content History ===
Max History Size: 10 items

=== Technical Details ===
Token Last Changed: 1702566495 seconds
Token Microseconds: 427884
Play Status State: 2
Received Playback Request: false

=== Service Availability Check ===
✅ Spotify is available on this device

Done!
```

## Understanding the Output

### Service States
- **Active**: Service is currently selected and active
- **Inactive**: Service is available but not currently active
- **InactiveUnselected**: Service is available but never been used

### Capabilities
- **Skip Previous**: Can skip to previous track
- **Seek**: Can seek within tracks (scrub timeline)
- **Resume**: Can resume paused playback
- **Data Collection**: Service collects usage analytics

### Technical Fields
- **Token Last Changed**: Unix timestamp of last authentication
- **Play Status State**: Internal playback state code
- **Current URI**: Unique identifier for currently playing content

## Common Use Cases

### 1. Check if Spotify is Logged In
```go
response, err := client.Introspect("SPOTIFY", "")
if err != nil {
    log.Fatal(err)
}

if response.HasUser() && response.IsActive() {
    fmt.Println("Spotify is logged in and active")
} else {
    fmt.Println("Spotify needs authentication or activation")
}
```

### 2. Verify Service Capabilities Before Playback Control
```go
response, err := client.IntrospectSpotify("")
if err != nil {
    log.Fatal(err)
}

if response.SupportsSeek() {
    // Safe to use seek controls
    fmt.Println("Seek controls available")
}

if response.SupportsSkipPrevious() {
    // Safe to use previous track
    fmt.Println("Previous track control available")
}
```

### 3. Monitor Service Health
```go
response, err := client.Introspect("PANDORA", "my_pandora_user")
if err != nil {
    log.Fatal(err)
}

if !response.IsActive() {
    fmt.Println("Pandora service needs activation")
}

if response.HasSubscription() {
    fmt.Printf("Premium account: %s\n", response.SubscriptionType)
}
```

## Related API Methods

- `client.GetServiceAvailability()` - Check which services are available
- `client.SelectSource(source, account)` - Activate a music service
- `client.GetNowPlaying()` - Get current playback information

## Error Handling

The introspect endpoint may fail if:
- Service is not supported on the device
- Invalid source name provided
- Network connectivity issues
- Device is in standby mode

Always check for errors and handle gracefully:

```go
response, err := client.Introspect("SPOTIFY", "")
if err != nil {
    if strings.Contains(err.Error(), "failed to get introspect data") {
        fmt.Println("Service may not be configured or available")
        return
    }
    log.Fatal(err)
}
```

## Integration with Other Examples

This introspect data is useful before:
- [Preset Management](../preset-management/) - Verify service state before storing presets
- [Content Selection](../../docs/reference/SOURCE-SELECTION.md) - Check capabilities before switching sources
- [Zone Management](../../docs/reference/ZONE-MANAGEMENT.md) - Ensure all devices support the service

## API Documentation

For complete API documentation, see:
- [API Reference](../../docs/reference/API-ENDPOINTS.md)
- [Service Availability Implementation](../../docs/SERVICE-AVAILABILITY-IMPLEMENTATION.md)
