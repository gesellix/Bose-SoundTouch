# Service Availability Example

This example demonstrates how to use the `GetServiceAvailability()` method to retrieve and analyze service availability from a Bose SoundTouch device. This information can be used to provide better user feedback about supported stations and sources.

## What is Service Availability?

The `/serviceAvailability` endpoint provides information about which music services and input sources are theoretically available on the device, along with reasons why certain services might be unavailable.

This is different from the `/sources` endpoint, which shows currently configured and ready sources. Service availability shows what's possible, while sources show what's currently set up.

## Running the Example

### Method 1: Command Line Argument
```bash
go run main.go 192.0.2.100
```

### Method 2: Environment Variable
```bash
SOUNDTOUCH_HOST=192.0.2.100 go run main.go
```

Replace `192.0.2.100` with your SoundTouch device's IP address.

## Example Output

```
============================================================
SOUNDTOUCH SERVICE AVAILABILITY REPORT
============================================================
Total Services: 13
Available Services: 9
Unavailable Services: 4

📱 AVAILABLE SERVICES:
  ✅ AirPlay
  ✅ Amazon Music
  ✅ Deezer
  ✅ iHeartRadio
  ✅ Internet Radio
  ✅ Local Music Library
  ✅ Pandora
  ✅ Spotify
  ✅ TuneIn Radio

❌ UNAVAILABLE SERVICES:
  ❌ Amazon Alexa
  ❌ Bluetooth (INVALID_SOURCE_TYPE)
  ❌ BMX
  ❌ Notifications

🎵 STREAMING SERVICES:
  ✅ Spotify
  ✅ Pandora
  ✅ TuneIn Radio
  ✅ Amazon Music
  ✅ Deezer
  ✅ iHeartRadio
  ✅ Internet Radio
  Summary: 7/7 streaming services available

🔗 LOCAL INPUT SERVICES:
  ❌ Bluetooth
  ✅ AirPlay
  ✅ Local Music Library
  Summary: 2/3 local services available
```

## Key Features Demonstrated

### 1. Service Availability Analysis
- Total service count and availability breakdown
- Categorization into streaming vs. local services
- Detailed status for each service type

### 2. User-Friendly Recommendations
- Smart suggestions based on available services
- Alternative recommendations when preferred services are unavailable
- Clear status indicators for popular services

### 3. Troubleshooting Information
- Specific reasons why services are unavailable
- Helpful tips for resolving common issues
- Service-specific guidance

### 4. Comparison with Configured Sources
- Side-by-side comparison with the `/sources` endpoint
- Identification of available but unconfigured services
- Guidance on setting up available services

## Use Cases

### Application Development
Use this information to:
- Show users which music services they can potentially use
- Provide helpful setup guidance for available but unconfigured services
- Display appropriate UI elements based on device capabilities
- Offer fallback options when preferred services are unavailable

### User Support
- Diagnose why certain services aren't working
- Provide specific troubleshooting steps
- Help users understand their device's capabilities
- Guide users through service setup

### Device Management
- Audit service capabilities across multiple devices
- Plan music service deployments
- Understand device limitations

## API Methods Used

This example demonstrates several key methods from the ServiceAvailability API:

```go
// Get service availability
serviceAvailability, err := client.GetServiceAvailability()

// Check specific services
hasSpotify := serviceAvailability.HasSpotify()
hasBluetooth := serviceAvailability.HasBluetooth()

// Get service details
spotifyService := serviceAvailability.GetServiceByType(models.ServiceTypeSpotify)
if spotifyService != nil && !spotifyService.IsAvailable {
    reason := spotifyService.GetReason()
}

// Get categorized services
streamingServices := serviceAvailability.GetStreamingServices()
localServices := serviceAvailability.GetLocalServices()

// Get availability counts
total := serviceAvailability.GetServiceCount()
available := serviceAvailability.GetAvailableServiceCount()
unavailable := serviceAvailability.GetUnavailableServiceCount()
```

## Integration Ideas

This functionality can be integrated into:
- Mobile apps to show service status
- Web dashboards for device management
- Setup wizards for new devices
- Troubleshooting tools
- Music service recommendation systems

## Notes

- Service availability may change based on device firmware, network connectivity, and account status
- Some services may show as available but require additional setup (like signing into streaming accounts)
- The `reason` field provides valuable context for why services are unavailable
- Always compare with the `/sources` endpoint for a complete picture of device capabilities