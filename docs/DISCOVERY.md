# SoundTouch Device Discovery

This document describes the various methods available for discovering Bose SoundTouch devices on your network.

## Overview

The SoundTouch library supports multiple discovery methods to find devices on your network:

1. **Configuration-based discovery** - Manually configured devices in `.env` file
2. **UPnP/SSDP discovery** - Automatic discovery using Universal Plug and Play protocol
3. **mDNS/Bonjour discovery** - Automatic discovery using multicast DNS

## Discovery Methods

### 1. Configuration-based Discovery

You can manually configure known devices in your `.env` file:

```bash
PREFERRED_DEVICES=Living Room:192.168.1.100:8090,Kitchen:192.168.1.101:8090
```

This method is:
- ✅ Most reliable
- ✅ Fastest (no network scanning)
- ✅ Works in all network configurations
- ❌ Requires manual setup

### 2. UPnP/SSDP Discovery

Uses the Universal Plug and Play protocol to discover devices automatically. This is enabled by default.

Configuration:
```bash
UPNP_ENABLED=true  # Default: true
```

This method is:
- ✅ Widely supported by SoundTouch devices
- ✅ Standard protocol
- ❌ May be blocked by some firewalls/networks
- ❌ Requires multicast support

### 3. mDNS/Bonjour Discovery

Uses multicast DNS to discover devices advertising the `_soundtouch._tcp` service.

Configuration:
```bash
MDNS_ENABLED=true  # Default: true
```

This method is:
- ✅ Works well in home networks
- ✅ Apple/Bonjour compatible
- ❌ Depends on device advertising the service
- ❌ May not work in corporate networks
- ❌ Requires multicast support

## Configuration Options

### Environment Variables

```bash
# Discovery timeouts
DISCOVERY_TIMEOUT=5s        # How long to wait for discovery

# Protocol enablement
UPNP_ENABLED=true          # Enable UPnP/SSDP discovery
MDNS_ENABLED=true          # Enable mDNS/Bonjour discovery

# Caching
CACHE_ENABLED=true         # Enable discovery result caching
CACHE_TTL=30s             # How long to cache results

# Manual device configuration
PREFERRED_DEVICES=Name:IP:Port,Name2:IP2:Port2
```

### .env File Example

```bash
# Discovery settings
DISCOVERY_TIMEOUT=10s
UPNP_ENABLED=true
MDNS_ENABLED=true

# Cache settings
CACHE_ENABLED=true
CACHE_TTL=60s

# Known devices (fastest method)
PREFERRED_DEVICES=Living Room:192.168.1.100:8090,Kitchen:192.168.1.101:8090,Bedroom:192.168.1.102
```

## Usage Examples

### CLI Discovery

```bash
# Discover all devices using all methods
./soundtouch-cli -discover

# Discover with custom timeout
./soundtouch-cli -discover -timeout 10s

# Show detailed device information
./soundtouch-cli -discover-all
```

### Programmatic Usage

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/gesellix/bose-soundtouch/pkg/config"
    "github.com/gesellix/bose-soundtouch/pkg/discovery"
)

func main() {
    // Load configuration
    cfg, err := config.LoadFromEnv()
    if err != nil {
        cfg = config.DefaultConfig()
    }

    // Create unified discovery service
    discoveryService := discovery.NewUnifiedDiscoveryService(cfg)

    // Discover devices
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    devices, err := discoveryService.DiscoverDevices(ctx)
    if err != nil {
        fmt.Printf("Discovery failed: %v\n", err)
        return
    }

    // Print results
    for _, device := range devices {
        fmt.Printf("Found: %s at %s:%d\n", device.Name, device.Host, device.Port)
    }
}
```

### mDNS-only Discovery

```go
package main

import (
    "context"
    "time"

    "github.com/gesellix/bose-soundtouch/pkg/discovery"
)

func main() {
    // Create mDNS-only discovery service
    mdnsService := discovery.NewMDNSDiscoveryService(5 * time.Second)

    ctx := context.Background()
    devices, err := mdnsService.DiscoverDevices(ctx)
    
    // Handle results...
}
```

## Troubleshooting

### No devices found

1. **Check network connectivity**: Ensure devices are on the same network
2. **Verify multicast support**: Some corporate networks block multicast traffic
3. **Try configuration-based discovery**: Add devices manually to `.env` file
4. **Increase timeout**: Some networks may be slow
5. **Check firewall settings**: Ensure UDP multicast is allowed

### Slow discovery

1. **Enable caching**: Set `CACHE_ENABLED=true`
2. **Use manual configuration**: Fastest method for known devices
3. **Disable unused protocols**: Turn off UPnP or mDNS if not needed
4. **Reduce timeout**: If you know devices respond quickly

### mDNS specific issues

mDNS discovery may fail if:
- Devices don't advertise `_soundtouch._tcp` service
- Network blocks multicast DNS (port 5353)
- IPv6 is misconfigured (common error: "no route to host")

### UPnP specific issues

UPnP discovery may fail if:
- Network blocks SSDP multicast (239.255.255.250:1900)
- Devices don't respond to M-SEARCH requests
- Corporate firewalls block UPnP traffic

## Discovery Flow

The unified discovery service uses this flow:

1. **Check cache** (if enabled and not expired)
2. **Load configured devices** from environment/config
3. **Start parallel discovery**:
   - UPnP/SSDP discovery (if enabled)
   - mDNS discovery (if enabled)
4. **Merge results** (removing duplicates by IP)
5. **Update cache** for future requests
6. **Return combined device list**

## Device Information

Each discovered device includes:

- `Name`: Device name (from config or auto-detected)
- `Host`: IP address
- `Port`: Port number (usually 8090)
- `Location`: Full device URL
- `LastSeen`: When the device was discovered

## Performance Considerations

- **Caching**: Enabled by default, reduces repeated network scanning
- **Parallel discovery**: UPnP and mDNS run simultaneously
- **Timeouts**: Balance between speed and completeness
- **Configuration priority**: Manual config devices are added first

## Security Notes

- Discovery traffic is sent over multicast (inherently insecure)
- No authentication is performed during discovery
- Device communication after discovery may require authentication
- Consider network segmentation for IoT devices

## API Reference

### Core Types

```go
type DiscoveredDevice struct {
    Name     string
    Host     string
    Port     int
    Location string
    LastSeen time.Time
}
```

### Services

- `UnifiedDiscoveryService`: Uses all available discovery methods
- `DiscoveryService`: UPnP/SSDP only (legacy)
- `MDNSDiscoveryService`: mDNS/Bonjour only

### Methods

- `DiscoverDevices(ctx)`: Discover all devices
- `DiscoverDevice(ctx, host)`: Find specific device
- `GetCachedDevices()`: Get cached results
- `ClearCache()`: Clear discovery cache