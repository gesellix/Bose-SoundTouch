# SoundTouch Troubleshooting Guide

**Complete guide to diagnosing and fixing common SoundTouch Go client issues**

This guide helps you quickly identify and resolve problems with the SoundTouch Go client library. Issues are organized by category with step-by-step solutions.

## 🚨 **Quick Diagnostics**

### Test Your Setup
Run these commands to quickly diagnose your setup:

```bash
# 1. Test discovery
go run ./cmd/soundtouch-cli -discover

# 2. Test specific device connection
go run ./cmd/soundtouch-cli -host 192.168.1.100 -info

# 3. Test basic controls
go run ./cmd/soundtouch-cli -host 192.168.1.100 -volume

# 4. Test network connectivity
ping 192.168.1.100
```

---

## 🔍 **Discovery Issues**

### ❌ "No devices found"

**Symptoms:**
```
🔍 Discovering SoundTouch devices...
❌ No devices found on the network
```

**Causes & Solutions:**

#### 1. **Network Configuration**
```bash
# Check if devices are on same network
ip route show default  # Your gateway
arp -a | grep -i bose   # Look for Bose devices
```

**Solution:** Ensure both your computer and SoundTouch are on the same subnet.

#### 2. **Firewall Issues**
```bash
# Check if firewall is blocking UPnP
sudo ufw status                    # Ubuntu
netsh advfirewall show allprofiles # Windows
```

**Solution:** Allow UPnP traffic (port 1900 UDP) or temporarily disable firewall.

#### 3. **Device Not Ready**
- Power cycle your SoundTouch device
- Wait 30 seconds for full boot
- Check device is connected to network (solid white LED)

#### 4. **Discovery Timeout Too Short**
```go
discoverer := discovery.NewDiscoverer(discovery.Config{
    Timeout: 30 * time.Second,  // Increase timeout
})
```

#### 5. **Use Manual IP**
```go
// Bypass discovery entirely
client := client.NewClientFromHost("192.168.1.100")
```

### ❌ "Discovery timeout"

**Symptoms:**
```
🔍 Discovering SoundTouch devices (timeout: 5s)...
❌ Discovery failed: context deadline exceeded
```

**Solutions:**

1. **Increase timeout:**
```go
discoverer := discovery.NewDiscoverer(discovery.Config{
    Timeout: 15 * time.Second,
})
```

2. **Check network performance:**
```bash
# Test network latency
ping -c 4 192.168.1.1

# Check for network congestion
iperf3 -c 192.168.1.1  # If iperf server available
```

3. **Use wired connection if possible**

---

## 🌐 **Connection Issues**

### ❌ "Connection refused"

**Symptoms:**
```go
Failed to connect: dial tcp 192.168.1.100:8090: connection refused
```

**Diagnostic Steps:**

#### 1. **Verify IP and Port**
```bash
# Test if port 8090 is open
telnet 192.168.1.100 8090
# OR
nc -zv 192.168.1.100 8090

# Scan for open ports
nmap -p 8080-8100 192.168.1.100
```

#### 2. **Check Device Status**
- Device LED should be solid white (connected)
- Blinking white = connecting
- Red = error state

#### 3. **Router/Network Issues**
```bash
# Check routing
traceroute 192.168.1.100

# Test basic connectivity
ping -c 4 192.168.1.100
```

### ❌ "Timeout" / "Context deadline exceeded"

**Symptoms:**
```go
Failed to get device info: context deadline exceeded
```

**Solutions:**

#### 1. **Increase Client Timeout**
```go
config := client.ClientConfig{
    Host:    "192.168.1.100",
    Port:    8090,
    Timeout: 30 * time.Second,  // Increase from default 10s
}
```

#### 2. **Check Network Latency**
```bash
# Test response time
ping -c 10 192.168.1.100

# Should be < 100ms typically
```

#### 3. **Device Performance Issues**
- Device may be overloaded
- Try power cycling the device
- Check for firmware updates via Bose app

### ❌ "No such host"

**Symptoms:**
```go
Failed to connect: dial tcp: lookup soundtouch.local: no such host
```

**Solutions:**

1. **Use IP instead of hostname:**
```go
client := client.NewClientFromHost("192.168.1.100")  // Not "soundtouch.local"
```

2. **Fix DNS/mDNS:**
```bash
# Test hostname resolution
nslookup soundtouch.local
dig soundtouch.local

# Install mDNS tools if needed (Linux)
sudo apt-get install avahi-utils
avahi-resolve -n soundtouch.local
```

---

## 🎵 **Playback Control Issues**

### ❌ "Play/Pause not working"

**Symptoms:**
- Commands succeed but no audio change
- Device shows wrong status

**Diagnostic Steps:**

#### 1. **Check Current Status**
```go
nowPlaying, err := client.GetNowPlaying()
if err == nil {
    fmt.Printf("Status: %s, Source: %s\n", 
        nowPlaying.PlayStatus, nowPlaying.Source)
}
```

#### 2. **Verify Source Selection**
```go
sources, err := client.GetSources()
if err == nil {
    for _, source := range sources.Sources {
        fmt.Printf("Source: %s, Status: %s\n", 
            source.Source, source.Status)
    }
}
```

**Solutions:**

1. **Select active source first:**
```go
client.SelectSpotify()
time.Sleep(2 * time.Second)  // Wait for source change
client.Play()
```

2. **Use key commands instead:**
```go
client.SendKey("PLAY")   // Instead of client.Play()
client.SendKey("PAUSE")  // Instead of client.Pause()
```

3. **Check device isn't in setup mode**

### ❌ "Source selection fails"

**Symptoms:**
```go
Failed to select source: API request failed with status 500
```

**Solutions:**

1. **Check source availability:**
```go
sources, _ := client.GetSources()
for _, source := range sources.Sources {
    if source.Source == "SPOTIFY" && source.Status == "READY" {
        // Source is available
        client.SelectSource("SPOTIFY", source.SourceAccount)
    }
}
```

2. **Account-specific sources:**
```go
// For streaming services, include account
client.SelectSource("SPOTIFY", "your_account_id")
```

3. **Use convenience methods:**
```go
client.SelectSpotify()    // Handles account automatically
client.SelectBluetooth()
client.SelectAux()
```

---

## 🔊 **Volume & Audio Issues**

### ❌ "Volume control not working"

**Symptoms:**
- Volume commands succeed but no change
- "Permission denied" errors

**Diagnostic Steps:**

#### 1. **Check Zone Status**
```go
zoneStatus, err := client.GetZoneStatus()
if err == nil {
    fmt.Printf("Zone Status: %s\n", zoneStatus)
}
```

**Solutions:**

1. **Zone Member Issue:**
```go
// Only zone master can control volume
if zoneStatus == "MEMBER" {
    fmt.Println("Device is zone member - only master controls volume")
    
    // Find and use master device
    zone, _ := client.GetZone()
    // Connect to master device using zone.Master ID
}
```

2. **Use Safe Volume Methods:**
```go
client.SetVolumeSafe(50)     // Clamps to valid range
client.IncreaseVolume(5)     // Incremental control
client.DecreaseVolume(5)
```

3. **Check Current Volume:**
```go
volume, _ := client.GetVolume()
fmt.Printf("Target: %d, Actual: %d, Muted: %t\n", 
    volume.TargetVolume, volume.ActualVolume, volume.Muted)
```

### ❌ "Bass/Balance control not supported"

**Symptoms:**
```go
Failed to set bass: API request failed with status 404
```

**Solutions:**

1. **Check device capabilities:**
```go
caps, err := client.GetCapabilities()
if err == nil {
    fmt.Printf("Bass capable: %t\n", caps.BassCapable)
}
```

2. **Use safe methods:**
```go
client.SetBassSafe(-5)       // Won't fail on unsupported devices
client.SetBalanceSafe(10)    // Falls back gracefully
```

3. **Device-specific features:**
- SoundTouch 10: Basic bass only
- SoundTouch 20/30: Full bass and balance
- Soundbar models: Advanced audio controls

---

## 🔔 **Speaker Notification Issues**

### ❌ "speaker beep" command fails with status 400

**Symptoms:**
```bash
$ go run ./cmd/soundtouch-cli --host 192.168.178.35 sp beep
Playing notification beep from 192.168.178.35:8090...
✗ Failed to play notification beep: API request failed with status 400
```

**Cause:**
This was a bug in earlier versions where the Go client incorrectly used POST instead of GET for the `/playNotification` endpoint.

**Solution:**
Update to the latest version. The fix changed the `PlayNotificationBeep()` method to use GET requests:

```go
// Fixed implementation (v2025.02+)
func (c *Client) PlayNotificationBeep() error {
    var status models.StationResponse
    return c.get("/playNotification", &status)
}
```

**Verification:**
Both commands should now work identically:
```bash
# CLI command
go run ./cmd/soundtouch-cli --host 192.168.178.35 sp beep

# Direct curl (for comparison)
curl http://192.168.178.35:8090/playNotification
```

### ❌ "speaker" commands not supported

**Symptoms:**
```
✗ Failed to play notification: endpoint not supported
```

**Causes & Solutions:**

#### 1. **Device Model Compatibility**
- ✅ **Supported**: SoundTouch 10 (ST-10), SoundTouch 20 (ST-20)  
- ❌ **Not Supported**: SoundTouch 300 (ST-300), older models

**Solution:** Verify device model with:
```bash
soundtouch-cli --host <device> info
```

#### 2. **Missing App Key (TTS/URL only)**
TTS and URL playback require an app key, but beep does not:
```bash
# Beep - no app key needed
soundtouch-cli --host <device> speaker beep

# TTS - app key required
soundtouch-cli --host <device> speaker tts --text "Hello" --app-key "your-key"
```

### ❌ "Device is busy" during notifications

**Symptoms:**
```
✗ Failed to play notification: device is busy
```

**Solutions:**

#### 1. **Wait for Current Notification to Complete**
Only one notification can play at a time. Wait a few seconds and retry.

#### 2. **Check Current Playback Status**
```go
nowPlaying, _ := client.GetNowPlaying()
fmt.Printf("Current source: %s, status: %s\n", 
    nowPlaying.Source, nowPlaying.PlayStatus)
```

---

## 📡 **WebSocket Issues**

### ❌ "WebSocket connection failed"

**Symptoms:**
```go
Failed to connect WebSocket: dial ws://192.168.1.100:8080/: connection refused
```

**Solutions:**

#### 1. **Verify WebSocket Port (8080)**
```bash
# WebSocket uses port 8080, not 8090
nc -zv 192.168.1.100 8080
```

#### 2. **Check Protocol Specification**
```go
// WebSocket client should auto-handle this
wsClient := client.NewWebSocketClient(nil)

// Manual connection (if needed)
url := "ws://192.168.1.100:8080/"
headers := http.Header{}
headers.Set("Sec-WebSocket-Protocol", "gabbo")
```

#### 3. **Connection Conflicts**
- Only one WebSocket connection per device
- Close other apps using SoundTouch
- Restart SoundTouch device if needed

### ❌ "WebSocket disconnects frequently"

**Symptoms:**
- Connection drops every few minutes
- Constant reconnection messages

**Solutions:**

1. **Increase ping interval:**
```go
config := client.DefaultWebSocketConfig()
config.PingInterval = 60 * time.Second    // Increase from 30s
config.PongTimeout = 20 * time.Second     // Increase timeout

wsClient := client.NewWebSocketClient(config)
```

2. **Check network stability:**
```bash
# Test for packet loss
ping -c 100 192.168.1.100 | grep loss
```

3. **Power management issues:**
```bash
# Disable WiFi power saving (Linux)
sudo iwconfig wlan0 power off

# Check Windows power management
powercfg -devicequery wake_armed
```

### ❌ "Events not received"

**Symptoms:**
- WebSocket connects but no events
- Missing volume/playback updates

**Solutions:**

1. **Verify event handlers:**
```go
wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
    fmt.Printf("Volume event received: %d\n", event.Volume.TargetVolume)
})

// Test by manually changing volume on device
```

2. **Check event parsing:**
```go
wsClient.OnUnknownEvent(func(event *models.WebSocketEvent) {
    fmt.Printf("Unknown event: %+v\n", event)
})
```

3. **Device activity required:**
- Events only sent when device state changes
- Try manual volume/source changes
- Check device isn't in standby

---

## 👥 **Multiroom Issues**

### ❌ "Zone creation fails"

**Symptoms:**
```go
Failed to create zone: API request failed with status 400
```

**Solutions:**

#### 1. **Check Device Compatibility**
```go
// Get device capabilities
caps, _ := client.GetCapabilities()
// Look for multiroom support

// Verify devices are on same network
for _, client := range clients {
    network, _ := client.GetNetworkInfo()
    fmt.Printf("Device IP: %s\n", network.GetConnectedInterface().IPAddress)
}
```

#### 2. **Correct Device IDs**
```go
// Get exact device IDs
info, _ := client.GetDeviceInfo()
masterID := info.DeviceID  // Use this, not MAC address

// Create zone with proper IDs
client.CreateZone(masterID, []string{member1ID, member2ID})
```

#### 3. **Sequential Zone Operations**
```go
// Don't create multiple zones simultaneously
client1.CreateZone(master1, []string{member1})
time.Sleep(2 * time.Second)
client2.CreateZone(master2, []string{member2})
```

### ❌ "Device won't join zone"

**Symptoms:**
- Zone creation succeeds but member doesn't join
- Member device shows as standalone

**Solutions:**

1. **Check device status:**
```go
status, _ := memberClient.GetZoneStatus()
fmt.Printf("Member status: %s\n", status)

if status == "STANDALONE" {
    // Device didn't join - check network/permissions
}
```

2. **Firmware compatibility:**
- Ensure all devices have recent firmware
- Update via Bose SoundTouch app
- Some very old devices don't support multiroom

3. **Network subnet issues:**
```bash
# Verify devices can reach each other
ping -c 4 member_device_ip
```

---

## 🔧 **Development & Debugging**

### Enable Detailed Logging

```go
import "log"

// Enable verbose HTTP logging
log.SetFlags(log.LstdFlags | log.Lshortfile)

// Custom HTTP client with debug
transport := &http.Transport{
    // Add debug transport if needed
}

config := client.ClientConfig{
    Host:    "192.168.1.100",
    Port:    8090,
    Timeout: 10 * time.Second,
}
```

### Debug WebSocket Events

```go
wsClient.OnUnknownEvent(func(event *models.WebSocketEvent) {
    log.Printf("Raw event: %+v", event)
})

// Enable WebSocket debug logging
config := client.DefaultWebSocketConfig()
config.Logger = &client.DefaultLogger{}  // Or custom logger
```

### Network Debugging Tools

```bash
# Capture SoundTouch traffic
sudo tcpdump -i any host 192.168.1.100 and port 8090

# Monitor WebSocket traffic  
sudo tcpdump -i any host 192.168.1.100 and port 8080

# HTTP debugging with curl
curl -v http://192.168.1.100:8090/info
curl -v http://192.168.1.100:8090/volume
```

---

## 📊 **Performance Issues**

### High Memory Usage

**Symptoms:**
- Go process memory keeps growing
- Out of memory errors in long-running apps

**Solutions:**

1. **Connection cleanup:**
```go
// Always close WebSocket connections
defer wsClient.Disconnect()

// Use connection pools for multiple devices
pool := NewConnectionPool(10, 5*time.Minute)
defer pool.Close()
```

2. **Goroutine leaks:**
```go
// Use context for cancellation
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Monitor goroutines
go func() {
    for {
        fmt.Printf("Goroutines: %d\n", runtime.NumGoroutine())
        time.Sleep(10 * time.Second)
    }
}()
```

### Slow Response Times

**Solutions:**

1. **Increase timeouts appropriately:**
```go
config := client.ClientConfig{
    Timeout: 15 * time.Second,  // Reasonable for network ops
}
```

2. **Use connection pooling:**
```go
// Reuse connections instead of creating new ones
pool := NewConnectionPool(5, 5*time.Minute)
client := pool.GetClient(host, port)
```

3. **Concurrent operations:**
```go
// Process multiple devices concurrently
var wg sync.WaitGroup
for _, client := range clients {
    wg.Add(1)
    go func(c *client.Client) {
        defer wg.Done()
        // Process device
    }(client)
}
wg.Wait()
```

---

## 🚨 **Emergency Procedures**

### Device Becomes Unresponsive

1. **Power cycle device:**
   - Unplug for 10 seconds
   - Reconnect and wait 30 seconds for boot

2. **Network reset:**
   - Hold Bluetooth and Volume Down for 10 seconds
   - Device will reset network settings

3. **Factory reset (last resort):**
   - Hold Power for 10 seconds while plugged in
   - Will lose all presets and settings

### Multiple Devices Acting Strange

1. **Check router:**
   - Restart router/access point
   - Check for firmware updates
   - Verify DHCP/IP assignment

2. **Network interference:**
   - Check for 2.4GHz interference
   - Try 5GHz WiFi if available
   - Check for microwave/Bluetooth interference

### App Crashes or Hangs

1. **Graceful shutdown:**
```go
// Always use context for cancellation
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Cleanup resources
defer func() {
    if wsClient != nil {
        wsClient.Disconnect()
    }
}()
```

2. **Resource monitoring:**
```go
// Monitor resource usage
go func() {
    var m runtime.MemStats
    for {
        runtime.ReadMemStats(&m)
        log.Printf("Alloc = %d KB, Sys = %d KB", m.Alloc/1024, m.Sys/1024)
        time.Sleep(30 * time.Second)
    }
}()
```

---

## 📋 **Diagnostic Checklist**

Use this checklist to systematically troubleshoot issues:

### Network Connectivity
- [ ] Device power LED is solid white
- [ ] Both devices on same network subnet  
- [ ] Firewall allows ports 8090 (HTTP) and 8080 (WebSocket)
- [ ] Can ping device IP address
- [ ] Can telnet to ports 8090 and 8080

### Device Status
- [ ] Device not in setup mode (solid white LED)
- [ ] Recent firmware version (check Bose app)
- [ ] Device responds to Bose app
- [ ] No other apps connected to device

### Code Configuration
- [ ] Correct IP address and ports
- [ ] Reasonable timeouts (10-30 seconds)
- [ ] Proper error handling
- [ ] Resource cleanup (defer statements)

### Multiroom Specific  
- [ ] All devices support multiroom
- [ ] Device IDs are correct (from GetDeviceInfo)
- [ ] Devices on same network subnet
- [ ] No existing zone conflicts

---

## 🛟 **Getting More Help**

### Information to Gather

When reporting issues, include:

```go
// Device information
info, _ := client.GetDeviceInfo()
fmt.Printf("Device: %s %s (ID: %s)\n", info.Type, info.Name, info.DeviceID)

// Network information
network, _ := client.GetNetworkInfo()
fmt.Printf("Network: %+v\n", network)

// Go version and OS
fmt.Printf("Go version: %s\n", runtime.Version())
fmt.Printf("OS: %s/%s\n", runtime.GOOS, runtime.GOARCH)
```

### Useful Commands

```bash
# System information
go version
uname -a  # Linux/macOS
systeminfo # Windows

# Network debugging
ip addr show        # Linux
ifconfig           # macOS
ipconfig /all      # Windows

# SoundTouch specific
go run ./cmd/soundtouch-cli -host <ip> -info
go run ./cmd/soundtouch-cli -host <ip> -network-info
```

### Support Resources

- **GitHub Issues**: Create detailed issue with logs and system info
- **Documentation**: Check `/docs` directory for specific topics
- **Examples**: Review `/examples` for working code patterns
- **CLI Tool**: Use built-in CLI for testing and debugging

Remember: Most issues are network-related. Start with basic connectivity testing before investigating code issues.