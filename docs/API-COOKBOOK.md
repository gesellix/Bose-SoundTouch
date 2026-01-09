# SoundTouch API Cookbook

**Real-world patterns, recipes, and best practices for the SoundTouch Go client**

This cookbook provides practical solutions to common SoundTouch integration challenges. Each recipe includes working code, error handling, and production considerations.

## 📋 **Table of Contents**

- [Device Management](#device-management)
- [Playback Control](#playback-control)
- [Volume & Audio](#volume--audio)
- [Real-time Monitoring](#real-time-monitoring)
- [Multiroom Coordination](#multiroom-coordination)
- [Error Handling](#error-handling)
- [Performance Optimization](#performance-optimization)
- [Production Patterns](#production-patterns)

---

## 🖥️ **Device Management**

### Recipe: Robust Device Discovery

**Problem**: Reliably find all SoundTouch devices on the network with fallback options.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/discovery"
)

func discoverAllDevices(timeout time.Duration) ([]*client.Client, error) {
    discoverer := discovery.NewDiscoverer(discovery.Config{
        Timeout: timeout,
    })

    fmt.Printf("🔍 Discovering SoundTouch devices (timeout: %v)...\n", timeout)
    
    devices, err := discoverer.DiscoverDevices()
    if err != nil {
        return nil, fmt.Errorf("discovery failed: %w", err)
    }

    if len(devices) == 0 {
        return nil, fmt.Errorf("no devices found on network")
    }

    // Create clients for each device
    var clients []*client.Client
    for _, device := range devices {
        config := client.ClientConfig{
            Host:    device.Host,
            Port:    device.Port,
            Timeout: 5 * time.Second,
        }
        
        c := client.NewClient(config)
        
        // Verify connectivity
        if _, err := c.Ping(); err != nil {
            log.Printf("⚠️  Device at %s:%d not responding: %v", device.Host, device.Port, err)
            continue
        }
        
        clients = append(clients, c)
    }

    fmt.Printf("✅ Found %d responsive device(s)\n", len(clients))
    return clients, nil
}

// Usage with fallback to known IPs
func getDevicesWithFallback() []*client.Client {
    // Try discovery first
    clients, err := discoverAllDevices(10 * time.Second)
    if err == nil && len(clients) > 0 {
        return clients
    }

    log.Printf("Discovery failed: %v, trying known IPs...", err)

    // Fallback to known IP addresses
    knownIPs := []string{"192.168.1.100", "192.168.1.101", "192.168.1.102"}
    
    var clients []*client.Client
    for _, ip := range knownIPs {
        c := client.NewClientFromHost(ip)
        if _, err := c.Ping(); err == nil {
            clients = append(clients, c)
            log.Printf("✅ Connected to device at %s", ip)
        }
    }

    return clients
}
```

### Recipe: Device Health Monitoring

**Problem**: Monitor device connectivity and automatically reconnect.

```go
type DeviceMonitor struct {
    client      *client.Client
    deviceName  string
    healthy     bool
    lastSeen    time.Time
    retryCount  int
    maxRetries  int
    checkInterval time.Duration
    callbacks   DeviceCallbacks
}

type DeviceCallbacks struct {
    OnHealthy   func(deviceName string)
    OnUnhealthy func(deviceName string, err error)
    OnReconnect func(deviceName string)
}

func NewDeviceMonitor(c *client.Client, name string) *DeviceMonitor {
    return &DeviceMonitor{
        client:        c,
        deviceName:    name,
        maxRetries:    3,
        checkInterval: 30 * time.Second,
    }
}

func (dm *DeviceMonitor) Start(ctx context.Context) {
    ticker := time.NewTicker(dm.checkInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            dm.checkHealth()
        }
    }
}

func (dm *DeviceMonitor) checkHealth() {
    err := dm.client.Ping()
    
    if err == nil {
        if !dm.healthy {
            dm.healthy = true
            dm.retryCount = 0
            dm.lastSeen = time.Now()
            if dm.callbacks.OnHealthy != nil {
                dm.callbacks.OnHealthy(dm.deviceName)
            }
            if dm.callbacks.OnReconnect != nil && dm.retryCount > 0 {
                dm.callbacks.OnReconnect(dm.deviceName)
            }
        }
        dm.lastSeen = time.Now()
        return
    }

    // Device is unhealthy
    dm.healthy = false
    dm.retryCount++
    
    if dm.callbacks.OnUnhealthy != nil {
        dm.callbacks.OnUnhealthy(dm.deviceName, err)
    }

    log.Printf("⚠️  Device %s unhealthy (attempt %d/%d): %v", 
        dm.deviceName, dm.retryCount, dm.maxRetries, err)
}

func (dm *DeviceMonitor) IsHealthy() bool {
    return dm.healthy && time.Since(dm.lastSeen) < 2*dm.checkInterval
}
```

---

## 🎵 **Playback Control**

### Recipe: Smart Play/Pause Toggle

**Problem**: Implement intelligent play/pause that works regardless of current state.

```go
func smartPlayPause(c *client.Client) error {
    // Get current status
    nowPlaying, err := c.GetNowPlaying()
    if err != nil {
        return fmt.Errorf("failed to get playback status: %w", err)
    }

    switch nowPlaying.PlayStatus {
    case "PLAY_STATE":
        fmt.Println("⏸️  Pausing playback...")
        return c.Pause()
    
    case "PAUSE_STATE", "STOP_STATE":
        fmt.Println("▶️  Resuming playback...")
        return c.Play()
    
    case "BUFFERING_STATE":
        fmt.Println("📡 Device is buffering, waiting...")
        // Wait a bit and try again
        time.Sleep(2 * time.Second)
        return smartPlayPause(c)
    
    default:
        fmt.Printf("🤔 Unknown play status: %s, trying play...\n", nowPlaying.PlayStatus)
        return c.Play()
    }
}
```

### Recipe: Playlist Navigation with Validation

**Problem**: Navigate playlists safely with boundary checking and retry logic.

```go
type PlaylistNavigator struct {
    client      *client.Client
    maxRetries  int
    retryDelay  time.Duration
}

func NewPlaylistNavigator(c *client.Client) *PlaylistNavigator {
    return &PlaylistNavigator{
        client:     c,
        maxRetries: 3,
        retryDelay: time.Second,
    }
}

func (pn *PlaylistNavigator) NextTrack() error {
    return pn.executeWithRetry(func() error {
        return pn.client.NextTrack()
    }, "next track")
}

func (pn *PlaylistNavigator) PreviousTrack() error {
    return pn.executeWithRetry(func() error {
        return pn.client.PrevTrack()
    }, "previous track")
}

func (pn *PlaylistNavigator) executeWithRetry(operation func() error, operationName string) error {
    var lastErr error
    
    for attempt := 1; attempt <= pn.maxRetries; attempt++ {
        lastErr = operation()
        if lastErr == nil {
            return nil
        }
        
        log.Printf("⚠️  %s attempt %d failed: %v", operationName, attempt, lastErr)
        
        if attempt < pn.maxRetries {
            time.Sleep(pn.retryDelay)
        }
    }
    
    return fmt.Errorf("%s failed after %d attempts: %w", operationName, pn.maxRetries, lastErr)
}

func (pn *PlaylistNavigator) GetTrackInfo() (string, error) {
    nowPlaying, err := pn.client.GetNowPlaying()
    if err != nil {
        return "", err
    }
    
    info := "Unknown Track"
    if nowPlaying.Track != "" && nowPlaying.Artist != "" {
        info = fmt.Sprintf("%s - %s", nowPlaying.Artist, nowPlaying.Track)
    } else if nowPlaying.Track != "" {
        info = nowPlaying.Track
    } else if nowPlaying.StationName != "" {
        info = nowPlaying.StationName
    }
    
    return info, nil
}
```

---

## 🔊 **Volume & Audio**

### Recipe: Gradual Volume Transitions

**Problem**: Smoothly transition volume levels without jarring jumps.

```go
type VolumeController struct {
    client        *client.Client
    stepSize      int
    stepDelay     time.Duration
    maxVolume     int
    warningLevel  int
}

func NewVolumeController(c *client.Client) *VolumeController {
    return &VolumeController{
        client:       c,
        stepSize:     2,
        stepDelay:    100 * time.Millisecond,
        maxVolume:    80, // Safety limit
        warningLevel: 70,
    }
}

func (vc *VolumeController) FadeIn(targetVolume int, duration time.Duration) error {
    if targetVolume > vc.maxVolume {
        return fmt.Errorf("target volume %d exceeds safety limit %d", targetVolume, vc.maxVolume)
    }

    current, err := vc.getCurrentVolume()
    if err != nil {
        return err
    }

    return vc.transitionVolume(current, targetVolume, duration)
}

func (vc *VolumeController) FadeOut(duration time.Duration) error {
    current, err := vc.getCurrentVolume()
    if err != nil {
        return err
    }

    return vc.transitionVolume(current, 0, duration)
}

func (vc *VolumeController) transitionVolume(from, to int, duration time.Duration) error {
    if from == to {
        return nil
    }

    steps := abs(to - from)
    if steps == 0 {
        return nil
    }

    stepDuration := duration / time.Duration(steps)
    direction := 1
    if to < from {
        direction = -1
    }

    current := from
    for current != to {
        if direction > 0 && current < to {
            current = min(current+vc.stepSize, to)
        } else if direction < 0 && current > to {
            current = max(current-vc.stepSize, to)
        }

        if err := vc.client.SetVolume(current); err != nil {
            return fmt.Errorf("failed to set volume to %d: %w", current, err)
        }

        if current >= vc.warningLevel {
            fmt.Printf("⚠️  High volume warning: %d\n", current)
        }

        if current != to {
            time.Sleep(stepDuration)
        }
    }

    return nil
}

func (vc *VolumeController) getCurrentVolume() (int, error) {
    volume, err := vc.client.GetVolume()
    if err != nil {
        return 0, err
    }
    return volume.TargetVolume, nil
}

// Helper functions
func abs(x int) int {
    if x < 0 { return -x }
    return x
}

func min(a, b int) int {
    if a < b { return a }
    return b
}

func max(a, b int) int {
    if a > b { return a }
    return b
}
```

### Recipe: Audio Profile Management

**Problem**: Save and restore audio settings for different scenarios.

```go
type AudioProfile struct {
    Name     string `json:"name"`
    Volume   int    `json:"volume"`
    Bass     int    `json:"bass"`
    Balance  int    `json:"balance"`
    Source   string `json:"source,omitempty"`
}

type ProfileManager struct {
    client   *client.Client
    profiles map[string]AudioProfile
}

func NewProfileManager(c *client.Client) *ProfileManager {
    return &ProfileManager{
        client:   c,
        profiles: make(map[string]AudioProfile),
    }
}

func (pm *ProfileManager) CreateProfile(name string) error {
    // Get current settings
    volume, err := pm.client.GetVolume()
    if err != nil {
        return fmt.Errorf("failed to get volume: %w", err)
    }

    bass, err := pm.client.GetBass()
    if err != nil {
        return fmt.Errorf("failed to get bass: %w", err)
    }

    balance, err := pm.client.GetBalance()
    if err != nil {
        // Balance might not be supported, use default
        balance = &models.Balance{TargetBalance: 0}
    }

    nowPlaying, err := pm.client.GetNowPlaying()
    source := ""
    if err == nil {
        source = nowPlaying.Source
    }

    profile := AudioProfile{
        Name:    name,
        Volume:  volume.TargetVolume,
        Bass:    bass.TargetBass,
        Balance: balance.TargetBalance,
        Source:  source,
    }

    pm.profiles[name] = profile
    fmt.Printf("✅ Profile '%s' saved: Vol=%d, Bass=%d, Balance=%d\n", 
        name, profile.Volume, profile.Bass, profile.Balance)

    return nil
}

func (pm *ProfileManager) ApplyProfile(name string) error {
    profile, exists := pm.profiles[name]
    if !exists {
        return fmt.Errorf("profile '%s' not found", name)
    }

    fmt.Printf("🎛️  Applying profile '%s'...\n", name)

    // Apply settings in order
    if err := pm.client.SetVolume(profile.Volume); err != nil {
        return fmt.Errorf("failed to set volume: %w", err)
    }

    if err := pm.client.SetBassSafe(profile.Bass); err != nil {
        log.Printf("Warning: failed to set bass: %v", err)
    }

    if err := pm.client.SetBalanceSafe(profile.Balance); err != nil {
        log.Printf("Warning: failed to set balance: %v", err)
    }

    if profile.Source != "" {
        if err := pm.client.SelectSource(profile.Source, ""); err != nil {
            log.Printf("Warning: failed to select source %s: %v", profile.Source, err)
        }
    }

    fmt.Printf("✅ Profile '%s' applied successfully\n", name)
    return nil
}

func (pm *ProfileManager) ListProfiles() []string {
    var names []string
    for name := range pm.profiles {
        names = append(names, name)
    }
    return names
}

// Predefined profiles
func (pm *ProfileManager) CreateDefaultProfiles() {
    // Movie profile
    pm.profiles["movie"] = AudioProfile{
        Name:    "movie",
        Volume:  60,
        Bass:    3,
        Balance: 0,
    }

    // Music profile
    pm.profiles["music"] = AudioProfile{
        Name:    "music",
        Volume:  50,
        Bass:    1,
        Balance: 0,
    }

    // Night profile (low volume)
    pm.profiles["night"] = AudioProfile{
        Name:    "night",
        Volume:  20,
        Bass:    -1,
        Balance: 0,
    }

    fmt.Println("✅ Default profiles created: movie, music, night")
}
```

---

## 📡 **Real-time Monitoring**

### Recipe: Event-Driven State Manager

**Problem**: Keep track of device state changes and trigger actions based on events.

```go
type StateManager struct {
    client      *client.Client
    wsClient    *client.WebSocketClient
    state       DeviceState
    mutex       sync.RWMutex
    subscribers []StateSubscriber
}

type DeviceState struct {
    Volume      int
    Muted       bool
    PlayStatus  string
    CurrentTrack string
    CurrentArtist string
    Source      string
    LastUpdate  time.Time
}

type StateSubscriber interface {
    OnStateChange(oldState, newState DeviceState)
}

func NewStateManager(c *client.Client) *StateManager {
    sm := &StateManager{
        client: c,
        state:  DeviceState{LastUpdate: time.Now()},
    }

    // Initialize WebSocket client
    sm.wsClient = c.NewWebSocketClient(nil)
    sm.setupEventHandlers()

    return sm
}

func (sm *StateManager) setupEventHandlers() {
    sm.wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
        sm.mutex.Lock()
        oldState := sm.state
        sm.state.Volume = event.Volume.TargetVolume
        sm.state.Muted = event.Volume.Muted
        sm.state.LastUpdate = time.Now()
        newState := sm.state
        sm.mutex.Unlock()

        sm.notifySubscribers(oldState, newState)
    })

    sm.wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
        sm.mutex.Lock()
        oldState := sm.state
        sm.state.PlayStatus = event.NowPlaying.PlayStatus
        sm.state.CurrentTrack = event.NowPlaying.Track
        sm.state.CurrentArtist = event.NowPlaying.Artist
        sm.state.Source = event.NowPlaying.Source
        sm.state.LastUpdate = time.Now()
        newState := sm.state
        sm.mutex.Unlock()

        sm.notifySubscribers(oldState, newState)
    })
}

func (sm *StateManager) Start() error {
    return sm.wsClient.Connect()
}

func (sm *StateManager) Stop() error {
    return sm.wsClient.Disconnect()
}

func (sm *StateManager) GetState() DeviceState {
    sm.mutex.RLock()
    defer sm.mutex.RUnlock()
    return sm.state
}

func (sm *StateManager) Subscribe(subscriber StateSubscriber) {
    sm.mutex.Lock()
    defer sm.mutex.Unlock()
    sm.subscribers = append(sm.subscribers, subscriber)
}

func (sm *StateManager) notifySubscribers(oldState, newState DeviceState) {
    sm.mutex.RLock()
    subscribers := make([]StateSubscriber, len(sm.subscribers))
    copy(subscribers, sm.subscribers)
    sm.mutex.RUnlock()

    for _, subscriber := range subscribers {
        go subscriber.OnStateChange(oldState, newState)
    }
}

// Example subscriber: Auto-pause when volume muted
type AutoPauseSubscriber struct {
    client *client.Client
}

func (aps *AutoPauseSubscriber) OnStateChange(oldState, newState DeviceState) {
    if !oldState.Muted && newState.Muted && newState.PlayStatus == "PLAY_STATE" {
        log.Println("🔇 Volume muted, auto-pausing...")
        aps.client.Pause()
    } else if oldState.Muted && !newState.Muted && newState.PlayStatus == "PAUSE_STATE" {
        log.Println("🔊 Volume unmuted, auto-resuming...")
        aps.client.Play()
    }
}
```

---

## 👥 **Multiroom Coordination**

### Recipe: Party Mode Controller

**Problem**: Synchronize multiple speakers for whole-house audio.

```go
type PartyModeController struct {
    masterClient *client.Client
    allClients   []*client.Client
    zoneActive   bool
    masterID     string
}

func NewPartyModeController(clients []*client.Client) (*PartyModeController, error) {
    if len(clients) == 0 {
        return nil, fmt.Errorf("no clients provided")
    }

    // Use first client as master
    master := clients[0]
    info, err := master.GetDeviceInfo()
    if err != nil {
        return nil, fmt.Errorf("failed to get master device info: %w", err)
    }

    return &PartyModeController{
        masterClient: master,
        allClients:   clients,
        masterID:     info.DeviceID,
    }, nil
}

func (pmc *PartyModeController) StartPartyMode() error {
    fmt.Println("🎉 Starting party mode...")

    // Get all device IDs
    var memberIDs []string
    for i, client := range pmc.allClients[1:] { // Skip master
        info, err := client.GetDeviceInfo()
        if err != nil {
            log.Printf("⚠️  Failed to get info for device %d: %v", i+1, err)
            continue
        }
        memberIDs = append(memberIDs, info.DeviceID)
    }

    if len(memberIDs) == 0 {
        return fmt.Errorf("no member devices available")
    }

    // Create zone
    err := pmc.masterClient.CreateZone(pmc.masterID, memberIDs)
    if err != nil {
        return fmt.Errorf("failed to create zone: %w", err)
    }

    pmc.zoneActive = true
    fmt.Printf("✅ Party mode active with %d speakers\n", len(memberIDs)+1)

    // Set reasonable volume for all speakers
    return pmc.SetPartyVolume(40)
}

func (pmc *PartyModeController) StopPartyMode() error {
    if !pmc.zoneActive {
        return nil
    }

    fmt.Println("🛑 Stopping party mode...")
    
    err := pmc.masterClient.DissolveZone()
    if err != nil {
        return fmt.Errorf("failed to dissolve zone: %w", err)
    }

    pmc.zoneActive = false
    fmt.Println("✅ Party mode stopped, speakers are now independent")
    return nil
}

func (pmc *PartyModeController) SetPartyVolume(level int) error {
    if !pmc.zoneActive {
        return fmt.Errorf("party mode not active")
    }

    // Only master controls volume in a zone
    return pmc.masterClient.SetVolume(level)
}

func (pmc *PartyModeController) PlayPartyPlaylist() error {
    if !pmc.zoneActive {
        return fmt.Errorf("party mode not active")
    }

    // Example: Select Spotify and play
    if err := pmc.masterClient.SelectSpotify(); err != nil {
        return fmt.Errorf("failed to select Spotify: %w", err)
    }

    time.Sleep(time.Second) // Give source change time to process

    return pmc.masterClient.Play()
}

func (pmc *PartyModeController) GetZoneStatus() (string, error) {
    zone, err := pmc.masterClient.GetZone()
    if err != nil {
        return "", err
    }

    if zone.IsStandalone() {
        return "No zone active", nil
    }

    return fmt.Sprintf("Zone active: master=%s, members=%d", 
        zone.Master, len(zone.Members)), nil
}
```

---

## ⚠️ **Error Handling**

### Recipe: Resilient Operation Wrapper

**Problem**: Handle network issues, temporary failures, and device state conflicts gracefully.

```go
type ResilientClient struct {
    client       *client.Client
    maxRetries   int
    baseDelay    time.Duration
    maxDelay     time.Duration
    backoffRate  float64
}

func NewResilientClient(c *client.Client) *ResilientClient {
    return &ResilientClient{
        client:      c,
        maxRetries:  3,
        baseDelay:   time.Second,
        maxDelay:    10 * time.Second,
        backoffRate: 2.0,
    }
}

func (rc *ResilientClient) ExecuteWithRetry(operation func() error, operationName string) error {
    var lastErr error
    delay := rc.baseDelay

    for attempt := 1; attempt <= rc.maxRetries; attempt++ {
        lastErr = operation()
        if lastErr == nil {
            if attempt > 1 {
                log.Printf("✅ %s succeeded on attempt %d", operationName, attempt)
            }
            return nil
        }

        if !rc.isRetryableError(lastErr) {
            return fmt.Errorf("%s failed (non-retryable): %w", operationName, lastErr)
        }

        log.Printf("⚠️  %s attempt %d/%d failed: %v", operationName, attempt, rc.maxRetries, lastErr)

        if attempt < rc.maxRetries {
            log.Printf("🔄 Retrying in %v...", delay)
            time.Sleep(delay)
            delay = time.Duration(float64(delay) * rc.backoffRate)
            if delay > rc.maxDelay {
                delay = rc.maxDelay
            }
        }
    }

    return fmt.Errorf("%s failed after %d attempts: %w", operationName, rc.maxRetries, lastErr)
}

func (rc *ResilientClient) isRetryableError(err error) bool {
    errStr := err.Error()
    
    // Network-related errors
    retryablePatterns := []string{
        "connection refused",
        "timeout",
        "temporary failure",
        "network is unreachable",
        "no such host",
        "connection reset",
        "500", // Server errors
        "502", // Bad gateway
        "503", // Service unavailable
    }

    for _, pattern := range retryablePatterns {
        if strings.Contains(strings.ToLower(errStr), pattern) {
            return true
        }
    }

    return false
}

// Wrapper methods with automatic retry
func (rc *ResilientClient) SetVolume(level int) error {
    return rc.ExecuteWithRetry(func() error {
        return rc.client.SetVolume(level)
    }, "SetVolume")
}

func (rc *ResilientClient) Play() error {
    return rc.ExecuteWithRetry(func() error {
        return rc.client.Play()
    }, "Play")
}

func (rc *ResilientClient) SelectSource(source, account string) error {
    return rc.ExecuteWithRetry(func() error {
        return rc.client.SelectSource(source, account)
    }, "SelectSource")
}
```

---

## 🚀 **Performance Optimization**

### Recipe: Connection Pool Manager

**Problem**: Efficiently manage connections to multiple devices without resource waste.

```go
type ConnectionPool struct {
    clients    map[string]*client.Client
    mutex      sync.RWMutex
    maxIdle    int
    timeout    time.Duration
    lastUsed   map[string]time.Time
    cleanup    *time.Ticker
    stopChan   chan struct{}
}

func NewConnectionPool(maxIdle int, timeout time.Duration) *ConnectionPool {
    cp := &ConnectionPool{
        clients:  make(map[string]*client.Client),
        maxIdle:  maxIdle,
        timeout:  timeout,
        lastUsed: make(map[string]time.Time),
        stopChan: make(chan struct{}),
    }

    // Start cleanup goroutine
    cp.cleanup = time.NewTicker(timeout / 2)
    go cp.cleanupLoop()

    return cp
}

func (cp *ConnectionPool) GetClient(host string, port int) *client.Client {
    key := fmt.Sprintf("%s:%d", host, port)
    
    cp.mutex.RLock()
    if client, exists := cp.clients[key]; exists {
        cp.mutex.RUnlock()
        cp.mutex.Lock()
        cp.lastUsed[key] = time.Now()
        cp.mutex.Unlock()
        return client
    }
    cp.mutex.RUnlock()

    // Create new client
    config := client.ClientConfig{
        Host:    host,
        Port:    port,
        Timeout: 10 * time.Second,
    }
    
    newClient := client.NewClient(config)

    cp.mutex.Lock()
    cp.clients[key] = newClient
    cp.lastUsed[key] = time.Now()
    cp.mutex.Unlock()

    return newClient
}

func (cp *ConnectionPool) cleanupLoop() {
    for {
        select {
        case <-cp.cleanup.C:
            cp.cleanupIdleConnections()
        case <-cp.stopChan:
            return
        }
    }
}

func (cp *ConnectionPool) cleanupIdleConnections() {
    cp.mutex.Lock()
    defer cp.mutex.Unlock()

    now := time.Now()
    var toDelete []string

    for key, lastUsed := range cp.lastUsed {
        if now.Sub(lastUsed) > cp.timeout {
            toDelete = append(toDelete, key)
        }
    }

    for _, key := range toDelete {
        delete(cp.clients, key)
        delete(cp.lastUsed, key)
        log.Printf("🗑️  Cleaned up idle connection: %s", key)
    }
}

func (cp *ConnectionPool) Close() {
    close(cp.stopChan)
    cp.cleanup.Stop()
    
    cp.mutex.Lock()
    defer cp.mutex.Unlock()
    
    // Clean up all connections
    for key := range cp.clients {
        delete(cp.clients, key)
        delete(cp.lastUsed, key)
    }
}

func (cp *ConnectionPool) Stats() (active int, idle int) {
    cp.mutex.RLock()
    defer cp.mutex.RUnlock()
    
    now := time.Now()
    active = 0
    idle = 0
    
    for _, lastUsed := range cp.lastUsed {
        if now.Sub(lastUsed) < time.Minute {
            active++
        } else {
            idle++
        }
    }
    
    return
}
```

---

## 🏭 **Production Patterns**

### Recipe: Configuration Management

**Problem**: Manage different environments and settings cleanly.

```go
type Config struct {
    DeviceHosts    []string      `json:"device_hosts"`
    DiscoveryTimeout time.Duration `json:"discovery_timeout"`
    RequestTimeout   time.Duration `json:"request_timeout"`
    MaxRetries       int           `json:"max_retries"`
    LogLevel         string        `json:"log_level"`
    Features         FeatureFlags  `json:"features"`
}

type FeatureFlags struct {
    AutoDiscovery bool `json:"auto_discovery"`
    HealthCheck   bool `json:"health_check"`
    PartyMode     bool `json:"party_mode"`
    Volu