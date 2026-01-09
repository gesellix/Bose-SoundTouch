# System Endpoints Documentation

This document provides comprehensive documentation for the system management endpoints in the Bose SoundTouch Go client library.

## Overview

The system endpoints provide access to device-level configuration and diagnostic information including:

- **Clock/Time Management**: Device time settings and clock display configuration
- **Network Information**: Network interfaces, connectivity status, and diagnostics

## Clock/Time Management Endpoints

### GET /clockTime

Retrieves the current device time with timezone and UTC timestamp information.

**Response Structure:**
```xml
<clockTime zone="America/New_York" utc="1609459200">2021-01-01 00:00:00</clockTime>
```

**Go Usage:**
```go
clockTime, err := client.GetClockTime()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Device time: %s\n", clockTime.GetTimeString())
fmt.Printf("UTC timestamp: %d\n", clockTime.GetUTC())
fmt.Printf("Timezone: %s\n", clockTime.GetZone())
```

**CLI Usage:**
```bash
soundtouch-cli -host 192.168.1.100 -clock-time
```

### POST /clockTime

Sets the device time using either current system time or a specific UTC timestamp.

**Request Structure:**
```xml
<clockTime zone="UTC" utc="1609459200">2021-01-01 00:00:00</clockTime>
```

**Go Usage:**
```go
// Set to current system time
err := client.SetClockTimeNow()

// Set to specific timestamp
request := models.NewClockTimeRequestUTC(1609459200)
err := client.SetClockTime(request)

// Set to specific time
t := time.Date(2021, 1, 1, 12, 0, 0, 0, time.UTC)
request := models.NewClockTimeRequest(t)
err := client.SetClockTime(request)
```

**CLI Usage:**
```bash
# Set to current system time
soundtouch-cli -host 192.168.1.100 -set-clock-time now

# Set to specific Unix timestamp
soundtouch-cli -host 192.168.1.100 -set-clock-time 1609459200
```

### GET /clockDisplay

Retrieves clock display configuration including enabled state, format, brightness, and auto-dim settings.

**Response Structure:**
```xml
<clockDisplay deviceID="ABCD1234EFGH" enabled="true" format="24" brightness="75" autoDim="true" timeZone="America/New_York">Clock Display</clockDisplay>
```

**Go Usage:**
```go
clockDisplay, err := client.GetClockDisplay()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Enabled: %v\n", clockDisplay.IsEnabled())
fmt.Printf("Format: %s\n", clockDisplay.GetFormatDescription())
fmt.Printf("Brightness: %d%% (%s)\n", clockDisplay.GetBrightness(), clockDisplay.GetBrightnessLevel())
fmt.Printf("Auto-Dim: %v\n", clockDisplay.IsAutoDimEnabled())
```

**CLI Usage:**
```bash
soundtouch-cli -host 192.168.1.100 -clock-display
```

### POST /clockDisplay

Configures clock display settings including enable/disable, format, brightness, and auto-dim behavior.

**Request Structure:**
```xml
<clockDisplay enabled="true" format="24" brightness="75" autoDim="false" timeZone="UTC"></clockDisplay>
```

**Go Usage:**
```go
// Enable clock display
err := client.EnableClockDisplay()

// Disable clock display
err := client.DisableClockDisplay()

// Set brightness
err := client.SetClockDisplayBrightness(75)

// Set format
err := client.SetClockDisplayFormat(models.ClockFormat24Hour)

// Complex configuration
request := models.NewClockDisplayRequest().
    SetEnabled(true).
    SetFormat(models.ClockFormat24Hour).
    SetBrightness(75).
    SetAutoDim(false)
err := client.SetClockDisplay(request)
```

**CLI Usage:**
```bash
# Enable/disable clock display
soundtouch-cli -host 192.168.1.100 -enable-clock
soundtouch-cli -host 192.168.1.100 -disable-clock

# Set clock format (12, 24, auto)
soundtouch-cli -host 192.168.1.100 -clock-format 24

# Set brightness (0-100)
soundtouch-cli -host 192.168.1.10 -clock-brightness 75
```

**Clock Formats:**
- `12`: 12-hour format with AM/PM
- `24`: 24-hour format
- `auto`: System default format

**Brightness Levels:**
- `0`: Off
- `1-25`: Low
- `26-50`: Medium  
- `51-75`: High
- `76-100`: Maximum

## Network Information Endpoints

### GET /networkInfo

Retrieves comprehensive network information including interfaces, connectivity status, and statistics.

**Response Structure:**

#### WiFi Device Response (Real API)
```xml
<networkInfo wifiProfileCount="2">
<interfaces>
<interface type="WIFI_INTERFACE" name="wlan0" macAddress="AA:BB:CC:DD:EE:FF" ipAddress="192.168.1.10" ssid="MyHomeNetwork" frequencyKHz="5500000" state="NETWORK_WIFI_CONNECTED" signal="EXCELLENT_SIGNAL" mode="STATION"/>
<interface type="WIFI_INTERFACE" name="wlan1" macAddress="AA:BB:CC:DD:EE:01" state="NETWORK_WIFI_DISCONNECTED"/>
</interfaces>
</networkInfo>
```

#### Ethernet Device Response (Real API)
```xml
<networkInfo wifiProfileCount="3">
<interfaces>
<interface type="ETHERNET_INTERFACE" name="eth0" macAddress="AA:BB:CC:DD:EE:FF" ipAddress="192.168.1.10" state="NETWORK_ETHERNET_CONNECTED"/>
</interfaces>
</networkInfo>
```

**Go Usage:**
```go
networkInfo, err := client.GetNetworkInfo()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Device ID: %s\n", networkInfo.GetDeviceID())
fmt.Printf("Interfaces: %d\n", len(networkInfo.GetInterfaces()))

// Check connectivity
if networkInfo.HasWiFi() {
    fmt.Println("WiFi available")
}
if networkInfo.HasEthernet() {
    fmt.Println("Ethernet available")
}

// Iterate through interfaces
for i, iface := range networkInfo.GetInterfaces() {
    fmt.Printf("Interface %d:\n", i+1)
    fmt.Printf("  Type: %s\n", iface.GetType())
    fmt.Printf("  MAC: %s\n", iface.GetMacAddress())
    fmt.Printf("  IP: %s\n", iface.GetIPAddress())
    fmt.Printf("  Active: %v\n", iface.IsActive())
}

// WiFi profile information (if available)
if profile := networkInfo.GetWiFiProfile(); profile != nil {
    fmt.Printf("WiFi SSID: %s\n", profile.GetSSID())
    fmt.Printf("Security: %s\n", profile.GetSecurity())
    fmt.Printf("Signal: %d%% (%s)\n", profile.GetSignalStrength(), profile.GetSignalLevel())
}

// Network statistics (if available)
if stats := networkInfo.GetStatistics(); stats != nil {
    sent, received := stats.GetFormattedBytes()
    fmt.Printf("Data: %s sent, %s received\n", sent, received)
}
```

**CLI Usage:**
```bash
soundtouch-cli -host 192.168.1.100 -network-info
```

**CLI Output Example (WiFi Device):**
```
Network Information:
  WiFi Profiles: 2
  Total Interfaces: 2
  Active Interfaces: 1

Interface 1:
  • Type: WIFI_INTERFACE (wlan0)
  • MAC Address: AA:BB:CC:DD:EE:FF
  • IP Address: 192.168.1.10
  • State: WiFi Connected ✓
  • SSID: MyHomeNetwork
  • Signal: Excellent (90%)
  • Frequency: 5.5 GHz (5GHz)
  • Mode: Station (Client)
  • Summary: MyHomeNetwork (Excellent, 5GHz)

Interface 2:
  • Type: WIFI_INTERFACE (wlan1)
  • MAC Address: AA:BB:CC:DD:EE:01
  • State: WiFi Disconnected
  • Summary: Disconnected

Active WiFi Connection:
  • Network: MyHomeNetwork (Excellent, 5GHz)
  • Interface: wlan0
  • IP Address: 192.168.1.10

Connectivity Summary:
  ✓ WiFi Available (Connected)
  ✗ No Ethernet
```

## Interface Types

### Real SoundTouch Types
- **WIFI_INTERFACE**: WiFi network interfaces with SSID, signal strength, and frequency data
- **ETHERNET_INTERFACE**: Wired Ethernet network interfaces

### Interface States
- **NETWORK_WIFI_CONNECTED**: WiFi interface is connected to a network
- **NETWORK_WIFI_DISCONNECTED**: WiFi interface is not connected
- **NETWORK_ETHERNET_CONNECTED**: Ethernet interface is connected
- **NETWORK_ETHERNET_DISCONNECTED**: Ethernet interface is not connected

### Signal Levels
- **EXCELLENT_SIGNAL**: 90% quality
- **GOOD_SIGNAL**: 70% quality  
- **FAIR_SIGNAL**: 50% quality
- **POOR_SIGNAL**: 30% quality
- **virtual**: Virtual network interface

## Data Models

### ClockTime
```go
type ClockTime struct {
    XMLName xml.Name `xml:"clockTime"`
    Zone    string   `xml:"zone,attr,omitempty"`
    UTC     int64    `xml:"utc,attr,omitempty"`
    Value   string   `xml:",chardata"`
}
```

**Key Methods:**
- `GetTime() (time.Time, error)`: Parse time value
- `GetTimeString() string`: Formatted time string
- `IsEmpty() bool`: Check if time data is available

### ClockDisplay
```go
type ClockDisplay struct {
    XMLName    xml.Name `xml:"clockDisplay"`
    DeviceID   string   `xml:"deviceID,attr,omitempty"`
    Enabled    bool     `xml:"enabled,attr,omitempty"`
    Format     string   `xml:"format,attr,omitempty"`
    Brightness int      `xml:"brightness,attr,omitempty"`
    AutoDim    bool     `xml:"autoDim,attr,omitempty"`
    TimeZone   string   `xml:"timeZone,attr,omitempty"`
    Value      string   `xml:",chardata"`
}
```

**Key Methods:**
- `IsEnabled() bool`: Check if clock display is enabled
- `GetFormat() string`: Get format (12/24/auto)
- `GetBrightness() int`: Get brightness (0-100)
- `GetBrightnessLevel() string`: Get descriptive level

### NetworkInformation
```go
type NetworkInformation struct {
    XMLName          xml.Name          `xml:"networkInfo"`
    WifiProfileCount int               `xml:"wifiProfileCount,attr,omitempty"`
    Interfaces       NetworkInterfaces `xml:"interfaces"`
}

type NetworkInterfaces struct {
    XMLName    xml.Name           `xml:"interfaces"`
    Interfaces []NetworkInterface `xml:"interface"`
}
```

**Key Methods:**
- `GetWifiProfileCount() int`: Get number of WiFi profiles
- `GetInterfaces() []NetworkInterface`: Get all interfaces
- `GetActiveInterfaces() []NetworkInterface`: Get connected interfaces only
- `GetConnectedWiFiInterface() *NetworkInterface`: Get active WiFi interface
- `GetConnectedEthernetInterface() *NetworkInterface`: Get active Ethernet interface
- `HasWiFi() bool`: Check WiFi availability
- `HasEthernet() bool`: Check Ethernet availability

### NetworkInterface
```go
type NetworkInterface struct {
    XMLName      xml.Name `xml:"interface"`
    Type         string   `xml:"type,attr"`
    Name         string   `xml:"name,attr,omitempty"`
    MacAddress   string   `xml:"macAddress,attr,omitempty"`
    IPAddress    string   `xml:"ipAddress,attr,omitempty"`
    SSID         string   `xml:"ssid,attr,omitempty"`
    FrequencyKHz int      `xml:"frequencyKHz,attr,omitempty"`
    State        string   `xml:"state,attr,omitempty"`
    Signal       string   `xml:"signal,attr,omitempty"`
    Mode         string   `xml:"mode,attr,omitempty"`
}
```

**Key Methods:**
- `IsConnected() bool`: Check if interface is connected
- `IsWiFi() bool`: Check if WiFi interface (WIFI_INTERFACE)
- `IsEthernet() bool`: Check if Ethernet interface (ETHERNET_INTERFACE)
- `GetFrequencyGHz() float64`: Get WiFi frequency in GHz
- `GetFrequencyBand() string`: Get frequency band (2.4GHz/5GHz)
- `GetSignalQuality() int`: Get signal strength percentage (0-100)
- `GetSignalDescription() string`: Get readable signal level
- `GetNetworkSummary() string`: Get connection summary
- `ValidateIP() bool`: Validate IP address format
- `ValidateMAC() bool`: Validate MAC address format

## Error Handling

All system endpoints include comprehensive error handling:

### Common Error Cases
- **Network errors**: Connection timeouts, unreachable device
- **HTTP errors**: 404 (endpoint not found), 500 (server error)
- **Validation errors**: Invalid input parameters
- **XML parsing errors**: Malformed responses

### Error Response Format
```go
type APIError struct {
    XMLName xml.Name `xml:"error"`
    Code    int      `xml:"code,attr"`
    Message string   `xml:",chardata"`
}
```

### Best Practices
1. Always check for errors before using response data
2. Use validation methods for input parameters
3. Handle network timeouts gracefully
4. Log detailed error information for debugging

## Validation and Safety Features

### Clock Time Validation
- UTC timestamps must be between year 2000 and 2100
- Time formats are automatically validated and parsed
- Timezone information is preserved when available

### Clock Display Validation
- Brightness values are clamped to 0-100 range
- Format values are validated (12/24/auto only)
- Configuration changes require at least one parameter

### Network Info Safety
- IP and MAC address validation methods provided
- Interface status checking with multiple criteria
- Graceful handling of missing optional fields

## Integration Examples

### Complete Device Setup
```go
func setupDevice(host string) error {
    client := client.NewClientFromHost(host)
    
    // Set device time to current time
    if err := client.SetClockTimeNow(); err != nil {
        return fmt.Errorf("failed to set time: %w", err)
    }
    
    // Configure clock display
    err := client.SetClockDisplayFormat(models.ClockFormat24Hour)
    if err != nil {
        return fmt.Errorf("failed to set clock format: %w", err)
    }
    
    err = client.SetClockDisplayBrightness(50)
    if err != nil {
        return fmt.Errorf("failed to set brightness: %w", err)
    }
    
    err = client.EnableClockDisplay()
    if err != nil {
        return fmt.Errorf("failed to enable clock: %w", err)
    }
    
    // Check network connectivity
    networkInfo, err := client.GetNetworkInfo()
    if err != nil {
        return fmt.Errorf("failed to get network info: %w", err)
    }
    
    activeInterfaces := networkInfo.GetActiveInterfaces()
    if len(activeInterfaces) == 0 {
        return fmt.Errorf("no active network interfaces found")
    }
    
    log.Printf("Device setup complete with %d active network interfaces", len(activeInterfaces))
    return nil
}
```

### Network Diagnostics
```go
func diagnoseNetwork(host string) error {
    client := client.NewClientFromHost(host)
    
    networkInfo, err := client.GetNetworkInfo()
    if err != nil {
        return fmt.Errorf("failed to get network info: %w", err)
    }
    
    fmt.Printf("Network Diagnostics for %s:\n", networkInfo.GetDeviceID())
    
    interfaces := networkInfo.GetInterfaces()
    activeCount := len(networkInfo.GetActiveInterfaces())
    
    fmt.Printf("Total Interfaces: %d\n", len(interfaces))
    fmt.Printf("Active Interfaces: %d\n", activeCount)
    
    if activeCount == 0 {
        fmt.Println("⚠️  WARNING: No active network interfaces")
        return nil
    }
    
    for i, iface := range interfaces {
        fmt.Printf("\nInterface %d:\n", i+1)
        fmt.Printf("  Type: %s\n", iface.GetType())
        fmt.Printf("  MAC: %s", iface.GetMacAddress())
        if !iface.ValidateMAC() {
            fmt.Printf(" ❌ Invalid")
        }
        fmt.Printf("\n")
        
        fmt.Printf("  IP: %s", iface.GetIPAddress())
        if !iface.ValidateIP() {
            fmt.Printf(" ❌ Invalid")
        }
        fmt.Printf("\n")
        
        fmt.Printf("  Status: %s", iface.GetStatus())
        if iface.IsActive() {
            fmt.Printf(" ✅ Active")
        } else {
            fmt.Printf(" ❌ Inactive")
        }
        fmt.Printf("\n")
    }
    
    return nil
}
```

## CLI Integration

All system endpoints are fully integrated into the CLI tool with comprehensive help and examples:

```bash
# View all system commands
soundtouch-cli -help

# Device time management
soundtouch-cli -host 192.168.1.100 -clock-time
soundtouch-cli -host 192.168.1.100 -set-clock-time now

# Clock display configuration  
soundtouch-cli -host 192.168.1.100 -clock-display
soundtouch-cli -host 192.168.1.100 -enable-clock
soundtouch-cli -host 192.168.1.100 -clock-format 24
soundtouch-cli -host 192.168.1.100 -clock-brightness 75

# Network diagnostics
soundtouch-cli -host 192.168.1.100 -network-info
```

## Testing

Comprehensive test coverage includes:

- **Unit Tests**: All models and validation logic
- **Integration Tests**: HTTP client behavior with mock servers  
- **Real Device Tests**: Verified against actual SoundTouch hardware
- **Edge Case Tests**: Error conditions and invalid inputs
- **CLI Tests**: Command parsing and output formatting

## API Compatibility

The system endpoints implementation is designed to be:

- **Backward Compatible**: No breaking changes to existing APIs
- **Forward Compatible**: Extensible for future SoundTouch features
- **Robust**: Graceful handling of missing or unknown fields
- **Realistic**: Based on actual device response structures

The implementation supports both the basic SoundTouch response format (SCM/SMSC interfaces) and potential enhanced formats with additional network details.