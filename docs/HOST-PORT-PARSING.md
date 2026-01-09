# Host:Port Parsing Feature

This document describes the automatic host:port parsing functionality added to the SoundTouch CLI, which allows users to specify both host and port in a single `-host` flag.

## Overview

The SoundTouch CLI now supports parsing host and port combinations in the `-host` flag, making it more user-friendly and following common CLI patterns. Users can specify either just a host (using the default or `-port` flag) or a complete `host:port` combination.

## Usage Examples

### Basic Host:Port Format
```bash
# Specify host and port together
soundtouch-cli -host 192.168.1.100:8090 -info
soundtouch-cli -host 192.168.178.35:8090 -play
soundtouch-cli -host soundtouch.local:8090 -pause
```

### Traditional Separate Flags (Still Supported)
```bash
# Traditional separate host and port flags
soundtouch-cli -host 192.168.1.100 -port 8090 -info
soundtouch-cli -host 192.168.178.35 -port 8090 -play
```

### Precedence Rules
When both formats are used, the port specified in the host:port format takes precedence:
```bash
# Uses port 8090 from host:port, ignores -port 9999
soundtouch-cli -host 192.168.1.100:8090 -port 9999 -info
```

## Supported Formats

### IPv4 Addresses
```bash
# Standard IPv4 with port
soundtouch-cli -host 192.168.1.100:8090 -info

# IPv4 without port (uses default 8090)
soundtouch-cli -host 192.168.1.100 -info
```

### Hostnames
```bash
# Hostname with port
soundtouch-cli -host soundtouch.local:8090 -info
soundtouch-cli -host bose-kitchen:9000 -play

# Hostname without port (uses default)
soundtouch-cli -host soundtouch.local -info
```

### IPv6 Addresses
```bash
# IPv6 with port (requires brackets)
soundtouch-cli -host [::1]:8090 -info
soundtouch-cli -host [2001:db8::1]:8090 -play

# IPv6 without port
soundtouch-cli -host ::1 -info
```

## Implementation Details

### Parsing Function
The `parseHostPort()` function handles the parsing logic:

```go
func parseHostPort(hostPort string, defaultPort int) (string, int)
```

### Parsing Rules
1. **Contains colon**: Attempts to split using `net.SplitHostPort()`
2. **Valid port**: Port must be numeric and in range 1-65535
3. **Invalid port**: Falls back to original host and default port
4. **No colon**: Returns original input as host with default port
5. **Parse error**: Returns original input as host with default port

### Error Handling
The parser is designed to be forgiving and always return usable values:

- **Invalid port numbers**: Fall back to default port
- **Malformed input**: Return original input as host
- **Empty input**: Handle gracefully
- **Multiple colons**: Handled by `net.SplitHostPort()` error handling

## Test Coverage

### Unit Tests
Comprehensive test coverage in `cmd/soundtouch-cli/main_test.go`:

- ✅ IPv4 addresses with and without ports
- ✅ Hostnames with and without ports  
- ✅ IPv6 addresses with and without ports
- ✅ Invalid port handling
- ✅ Edge cases (empty strings, malformed input)
- ✅ Real-world SoundTouch scenarios

### Integration Tests
Tested with real SoundTouch devices:
- ✅ SoundTouch 10 (192.168.178.28:8090)
- ✅ SoundTouch 20 (192.168.178.35:8090)

## Benefits

### User Experience
- **Simplified syntax**: `host:port` is more intuitive than separate flags
- **Consistent with other tools**: Follows common CLI patterns
- **Backward compatible**: Existing scripts continue to work
- **Copy-paste friendly**: Can copy host:port from discovery output

### Development Benefits
- **Robust parsing**: Handles edge cases gracefully
- **Comprehensive tests**: Well-tested functionality
- **Clean implementation**: Uses Go standard library
- **Error resilience**: Falls back to sensible defaults

## Examples with Real Devices

### Discovery + Direct Usage
```bash
# Discover devices to find host:port
$ soundtouch-cli -discover
Found SoundTouch devices:
  My SoundTouch Device (192.168.1.10:8090) - SoundTouch 20

# Use discovered host:port directly
$ soundtouch-cli -host 192.168.1.10:8090 -play
```

### Different Port Scenarios
```bash
# Standard SoundTouch port
soundtouch-cli -host 192.168.1.100:8090 -info

# Custom port (if device configured differently)
soundtouch-cli -host 192.168.1.100:9000 -info

# Default port fallback
soundtouch-cli -host 192.168.1.100 -info  # Uses 8090
```

### Error Scenarios
```bash
# Invalid port - uses default 8090
soundtouch-cli -host 192.168.1.100:invalid -info

# Out of range port - uses default 8090  
soundtouch-cli -host 192.168.1.100:99999 -info

# Malformed input - treats as hostname
soundtouch-cli -host "malformed::input" -info
```

## CLI Help Output

The help text has been updated to reflect the new functionality:

```
Options:
  -host <ip>        SoundTouch device IP address (or host:port)
  -port <port>      SoundTouch device port (default: 8090)

Examples:
  soundtouch-cli -host 192.168.1.100 -info
  soundtouch-cli -host 192.168.1.100:8090 -info
  soundtouch-cli -host 192.168.1.100:8090 -pause
  soundtouch-cli -host 192.168.1.100:8090 -preset 1
```

## Technical Implementation

### Function Signature
```go
// parseHostPort splits a host:port string into separate host and port components
// If no port is specified, returns the original host and the provided default port
func parseHostPort(hostPort string, defaultPort int) (string, int)
```

### Key Features
- Uses Go's `net.SplitHostPort()` for robust parsing
- Validates port range (1-65535)
- Handles IPv6 addresses correctly with brackets
- Graceful fallback for all error conditions
- Preserves original host for malformed input

### Integration Points
The parsed values are used throughout the CLI:
- Device info commands
- Now playing queries
- Source management
- Key control commands
- All API endpoint interactions

## Future Enhancements

Potential improvements for the future:

1. **URL Format Support**: Support full URLs like `http://192.168.1.100:8090`
2. **Service Discovery**: Auto-detect port via service discovery protocols
3. **Configuration File**: Save frequently used host:port combinations
4. **Environment Variables**: Support `SOUNDTOUCH_HOST` with host:port format
5. **Validation**: More sophisticated host validation (DNS lookup, ping)

## Reference

- **Implementation**: `cmd/soundtouch-cli/main.go` (parseHostPort function)
- **Tests**: `cmd/soundtouch-cli/main_test.go`
- **Go Documentation**: `net.SplitHostPort()` for parsing logic
- **Standards**: Follows RFC 3986 for host:port format