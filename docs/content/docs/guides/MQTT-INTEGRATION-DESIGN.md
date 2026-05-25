---
title: "MQTT Integration Design for SoundTouch Service"
---
## Overview

This document outlines the design for integrating MQTT support into the existing SoundTouch service to simulate AWS IoT Core functionality. The integration will provide real-time device communication, shadow state management, and prepare for the AWS IoT service shutdown in May 2026.

## Current Architecture Analysis

### Existing Service Structure
```
Bose-SoundTouch/
├── cmd/soundtouch-service/main.go     # Main service entry point
├── pkg/
│   ├── client/                        # HTTP client for devices
│   ├── config/                        # Configuration management
│   ├── discovery/                     # Device discovery (UPnP, mDNS)
│   ├── models/                        # Data structures
│   └── service/
│       ├── handlers/                  # HTTP request handlers
│       │   └── server.go             # Main server struct
│       ├── datastore/                # Data persistence
│       ├── proxy/                    # HTTP proxying
│       └── [other services]
```

### Key Components
- **Server Struct**: Central HTTP handler in `pkg/service/handlers/server.go`
- **Discovery Service**: UPnP/mDNS device discovery in `pkg/discovery/`
- **DataStore**: Device state persistence in `pkg/service/datastore/`
- **Device Models**: Data structures in `pkg/models/`

## MQTT Integration Design

### 1. New Package Structure

```
pkg/service/mqtt/
├── broker.go              # MQTT broker implementation
├── shadow.go              # AWS IoT Shadow simulation
├── auth.go                # Certificate-based authentication
├── topics.go              # Topic routing and handlers
├── bridge.go              # HTTP ↔ MQTT state bridging
├── config.go              # MQTT configuration
└── client.go              # MQTT client utilities
```

### 2. Core Components

#### A. MQTT Broker (`pkg/service/mqtt/broker.go`)
```go
package mqtt

import (
    "crypto/tls"
    "fmt"
    "log"
    "sync"
    
    "github.com/mochi-co/mqtt/v2"
    "github.com/mochi-co/mqtt/v2/hooks/auth"
    "github.com/mochi-co/mqtt/v2/listeners"
)

type Broker struct {
    server      *mqtt.Server
    shadowStore *ShadowStore
    bridge      *HTTPBridge
    authHook    *AuthHook
    config      *Config
    running     bool
    mu          sync.RWMutex
}

type Config struct {
    Enabled         bool     `json:"enabled"`
    Port           int      `json:"port"`
    TLSEnabled     bool     `json:"tls_enabled"`
    CertFile       string   `json:"cert_file"`
    KeyFile        string   `json:"key_file"`
    DeviceCertPath string   `json:"device_cert_path"`
    ShadowPersist  bool     `json:"shadow_persist"`
}

func NewBroker(config *Config) (*Broker, error) {
    server := mqtt.New(nil)
    
    shadowStore := NewShadowStore()
    authHook := NewAuthHook(config.DeviceCertPath)
    
    return &Broker{
        server:      server,
        shadowStore: shadowStore,
        authHook:    authHook,
        config:      config,
    }, nil
}

func (b *Broker) Start() error {
    // Add TLS listener
    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{b.loadServerCert()},
        ClientAuth:   tls.RequireAndVerifyClientCert,
        ClientCAs:    b.loadDeviceCAs(),
    }
    
    tcp := listeners.NewTCP("mqtt-tls", fmt.Sprintf(":%d", b.config.Port), &listeners.Config{
        TLSConfig: tlsConfig,
    })
    
    b.server.AddListener(tcp)
    
    // Add hooks
    b.server.AddHook(b.authHook, nil)
    b.server.AddHook(NewShadowHook(b.shadowStore), nil)
    
    return b.server.Serve()
}
```

#### B. Shadow State Management (`pkg/service/mqtt/shadow.go`)
```go
package mqtt

import (
    "encoding/json"
    "fmt"
    "sync"
    "time"
)

type ShadowStore struct {
    shadows map[string]*DeviceShadow
    mu      sync.RWMutex
}

type DeviceShadow struct {
    State struct {
        Desired  map[string]interface{} `json:"desired"`
        Reported map[string]interface{} `json:"reported"`
        Delta    map[string]interface{} `json:"delta,omitempty"`
    } `json:"state"`
    Version     int    `json:"version"`
    Timestamp   int64  `json:"timestamp"`
    ClientToken string `json:"clientToken,omitempty"`
}

func NewShadowStore() *ShadowStore {
    return &ShadowStore{
        shadows: make(map[string]*DeviceShadow),
    }
}

func (s *ShadowStore) UpdateShadow(clientID string, payload []byte) (*DeviceShadow, error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    var update DeviceShadow
    if err := json.Unmarshal(payload, &update); err != nil {
        return nil, err
    }
    
    shadow := s.shadows[clientID]
    if shadow == nil {
        shadow = &DeviceShadow{
            State: struct {
                Desired  map[string]interface{} `json:"desired"`
                Reported map[string]interface{} `json:"reported"`
                Delta    map[string]interface{} `json:"delta,omitempty"`
            }{
                Desired:  make(map[string]interface{}),
                Reported: make(map[string]interface{}),
                Delta:    make(map[string]interface{}),
            },
        }
        s.shadows[clientID] = shadow
    }
    
    // Update reported state
    if update.State.Reported != nil {
        for key, value := range update.State.Reported {
            shadow.State.Reported[key] = value
        }
    }
    
    // Update desired state
    if update.State.Desired != nil {
        for key, value := range update.State.Desired {
            shadow.State.Desired[key] = value
        }
    }
    
    // Calculate delta
    shadow.calculateDelta()
    shadow.Version++
    shadow.Timestamp = time.Now().Unix()
    shadow.ClientToken = update.ClientToken
    
    return shadow, nil
}

func (s *DeviceShadow) calculateDelta() {
    s.State.Delta = make(map[string]interface{})
    
    for key, desired := range s.State.Desired {
        if reported, exists := s.State.Reported[key]; !exists || reported != desired {
            s.State.Delta[key] = desired
        }
    }
    
    if len(s.State.Delta) == 0 {
        s.State.Delta = nil
    }
}
```

#### C. HTTP ↔ MQTT Bridge (`pkg/service/mqtt/bridge.go`)
```go
package mqtt

import (
    "encoding/json"
    "fmt"
    "log"
    
    "github.com/gesellix/bose-soundtouch/pkg/models"
    "github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

type HTTPBridge struct {
    shadowStore *ShadowStore
    dataStore   *datastore.DataStore
    deviceMap   map[string]string // clientID -> deviceID mapping
}

func NewHTTPBridge(shadowStore *ShadowStore, dataStore *datastore.DataStore) *HTTPBridge {
    return &HTTPBridge{
        shadowStore: shadowStore,
        dataStore:   dataStore,
        deviceMap:   make(map[string]string),
    }
}

// ShadowToHTTP converts MQTT shadow updates to HTTP API calls
func (b *HTTPBridge) ShadowToHTTP(clientID string, shadow *DeviceShadow) error {
    deviceID, exists := b.deviceMap[clientID]
    if !exists {
        log.Printf("Unknown device clientID: %s", clientID)
        return fmt.Errorf("unknown device: %s", clientID)
    }
    
    // Handle power state changes
    if powerState, ok := shadow.State.Reported["powerState"].(string); ok {
        if err := b.updateDevicePower(deviceID, powerState == "ON"); err != nil {
            return fmt.Errorf("power update failed: %w", err)
        }
    }
    
    // Handle volume changes
    if volume, ok := shadow.State.Reported["volume"].(float64); ok {
        if err := b.updateDeviceVolume(deviceID, int(volume)); err != nil {
            return fmt.Errorf("volume update failed: %w", err)
        }
    }
    
    // Handle source changes
    if source, ok := shadow.State.Reported["source"].(string); ok {
        if err := b.updateDeviceSource(deviceID, source); err != nil {
            return fmt.Errorf("source update failed: %w", err)
        }
    }
    
    return nil
}

// HTTPToShadow converts HTTP device state to MQTT shadow updates
func (b *HTTPBridge) HTTPToShadow(deviceID string, deviceInfo *models.DeviceInfo) error {
    clientID, exists := b.getClientIDForDevice(deviceID)
    if !exists {
        return nil // Device not connected via MQTT
    }
    
    // Create shadow state from device info
    shadowState := map[string]interface{}{
        "deviceState": "CONNECTED",
        "deviceID":    deviceInfo.DeviceID,
        "name":        deviceInfo.Name,
        "type":        deviceInfo.Type,
    }
    
    // Add additional state if available
    if status := b.getDeviceStatus(deviceID); status != nil {
        shadowState["powerState"] = status.PowerState
        shadowState["volume"] = status.Volume
        shadowState["source"] = status.Source
    }
    
    // Update shadow
    shadowUpdate := DeviceShadow{
        State: struct {
            Desired  map[string]interface{} `json:"desired"`
            Reported map[string]interface{} `json:"reported"`
            Delta    map[string]interface{} `json:"delta,omitempty"`
        }{
            Reported: shadowState,
        },
    }
    
    payload, _ := json.Marshal(shadowUpdate)
    _, err := b.shadowStore.UpdateShadow(clientID, payload)
    return err
}
```

### 3. Integration with Existing Server

#### A. Extend Server Struct (`pkg/service/handlers/server.go`)
```go
// Add to existing Server struct
type Server struct {
    // ... existing fields ...
    
    // New MQTT fields
    mqttBroker      *mqtt.Broker
    mqttEnabled     bool
    mqttConfig      *mqtt.Config
    deviceClientIDs map[string]string // deviceID -> clientID mapping
}

// New initialization method
func (s *Server) initMQTTBroker(config *mqtt.Config) error {
    if !config.Enabled {
        return nil
    }
    
    broker, err := mqtt.NewBroker(config)
    if err != nil {
        return fmt.Errorf("failed to create MQTT broker: %w", err)
    }
    
    // Set up HTTP ↔ MQTT bridge
    bridge := mqtt.NewHTTPBridge(broker.ShadowStore(), s.ds)
    broker.SetBridge(bridge)
    
    s.mqttBroker = broker
    s.mqttEnabled = true
    s.mqttConfig = config
    s.deviceClientIDs = make(map[string]string)
    
    return nil
}

// Start MQTT broker alongside HTTP server
func (s *Server) StartMQTT() error {
    if !s.mqttEnabled {
        return nil
    }
    
    go func() {
        if err := s.mqttBroker.Start(); err != nil {
            log.Printf("MQTT broker error: %v", err)
        }
    }()
    
    return nil
}
```

#### B. Configuration Integration (`cmd/soundtouch-service/main.go`)
```go
// Add to serviceConfig struct
type serviceConfig struct {
    // ... existing fields ...
    
    // New MQTT configuration fields
    mqttEnabled         bool     `mapstructure:"mqtt_enabled"`
    mqttPort           int      `mapstructure:"mqtt_port"`
    mqttTLSCert        string   `mapstructure:"mqtt_tls_cert"`
    mqttTLSKey         string   `mapstructure:"mqtt_tls_key"`
    mqttDeviceCertPath string   `mapstructure:"mqtt_device_cert_path"`
    mqttShadowPersist  bool     `mapstructure:"mqtt_shadow_persist"`
}

// Update main function to initialize MQTT
func main() {
    // ... existing initialization ...
    
    // Initialize MQTT if enabled
    if cfg.mqttEnabled {
        mqttConfig := &mqtt.Config{
            Enabled:         cfg.mqttEnabled,
            Port:           cfg.mqttPort,
            TLSEnabled:     true,
            CertFile:       cfg.mqttTLSCert,
            KeyFile:        cfg.mqttTLSKey,
            DeviceCertPath: cfg.mqttDeviceCertPath,
            ShadowPersist:  cfg.mqttShadowPersist,
        }
        
        if err := server.InitMQTTBroker(mqttConfig); err != nil {
            log.Fatalf("Failed to initialize MQTT broker: %v", err)
        }
        
        if err := server.StartMQTT(); err != nil {
            log.Fatalf("Failed to start MQTT broker: %v", err)
        }
        
        log.Printf("MQTT broker started on port %d", cfg.mqttPort)
    }
    
    // ... rest of existing main function ...
}
```

### 4. Enhanced Device Discovery

#### A. MQTT Device Discovery (`pkg/service/mqtt/discovery.go`)
```go
package mqtt

import (
    "log"
    "time"
    
    "github.com/gesellix/bose-soundtouch/pkg/models"
    "github.com/mochi-co/mqtt/v2/packets"
)

type DeviceDiscoveryHook struct {
    deviceRegistry map[string]*models.Device
    onDeviceFound  func(*models.Device)
}

func NewDeviceDiscoveryHook() *DeviceDiscoveryHook {
    return &DeviceDiscoveryHook{
        deviceRegistry: make(map[string]*models.Device),
    }
}

func (h *DeviceDiscoveryHook) ID() string {
    return "device-discovery"
}

func (h *DeviceDiscoveryHook) OnConnect(cl *packets.Client, pk packets.Packet) error {
    clientID := pk.Connect.ClientIdentifier
    
    log.Printf("MQTT device connected: %s", clientID)
    
    // Create device entry
    device := &models.Device{
        ID:           clientID,
        ClientID:     clientID,
        Name:         "MQTT Device",
        LastSeen:     time.Now(),
        MQTTOnline:   true,
        Source:       "mqtt",
    }
    
    h.deviceRegistry[clientID] = device
    
    if h.onDeviceFound != nil {
        h.onDeviceFound(device)
    }
    
    return nil
}

func (h *DeviceDiscoveryHook) OnDisconnect(cl *packets.Client, err error) {
    clientID := cl.ID
    
    log.Printf("MQTT device disconnected: %s", clientID)
    
    if device, exists := h.deviceRegistry[clientID]; exists {
        device.MQTTOnline = false
        device.LastSeen = time.Now()
    }
}
```

#### B. Integration with Existing Discovery (`pkg/discovery/mqtt.go`)
```go
package discovery

import (
    "context"
    "time"
    
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

type MQTTDiscovery struct {
    deviceRegistry map[string]*models.Device
    enabled        bool
}

func NewMQTTDiscovery() *MQTTDiscovery {
    return &MQTTDiscovery{
        deviceRegistry: make(map[string]*models.Device),
        enabled:        true,
    }
}

func (d *MQTTDiscovery) DiscoverDevices(ctx context.Context, timeout time.Duration) ([]*models.Device, error) {
    if !d.enabled {
        return []*models.Device{}, nil
    }
    
    var devices []*models.Device
    for _, device := range d.deviceRegistry {
        if device.MQTTOnline {
            devices = append(devices, device)
        }
    }
    
    return devices, nil
}

func (d *MQTTDiscovery) AddDevice(device *models.Device) {
    d.deviceRegistry[device.ClientID] = device
}

func (d *MQTTDiscovery) RemoveDevice(clientID string) {
    delete(d.deviceRegistry, clientID)
}
```

### 5. Configuration File Extensions

#### A. Default Configuration (`config.yaml`)
```yaml
# Existing configuration...

# MQTT Configuration
mqtt:
  enabled: false
  port: 8883
  tls:
    cert_file: "/etc/ssl/certs/soundtouch-mqtt.crt"
    key_file: "/etc/ssl/private/soundtouch-mqtt.key"
  
  # Device certificate validation
  device_certs:
    path: "/etc/soundtouch/device-certs"
    auto_load: true
    
  # Shadow state management
  shadow:
    persist: true
    ttl: 86400  # 24 hours
    
  # Bridge configuration
  bridge:
    enabled: true
    sync_interval: 30s
```

#### B. Environment Variable Support
```bash
# MQTT configuration via environment variables
SOUNDTOUCH_MQTT_ENABLED=true
SOUNDTOUCH_MQTT_PORT=8883
SOUNDTOUCH_MQTT_TLS_CERT=/path/to/cert.pem
SOUNDTOUCH_MQTT_TLS_KEY=/path/to/key.pem
SOUNDTOUCH_MQTT_DEVICE_CERT_PATH=/path/to/device/certs
SOUNDTOUCH_MQTT_SHADOW_PERSIST=true
```

### 6. API Extensions

#### A. MQTT Status Endpoints
```go
// Add to handlers
func (s *Server) handleMQTTStatus(c *gin.Context) {
    if !s.mqttEnabled {
        c.JSON(http.StatusNotImplemented, gin.H{
            "error": "MQTT not enabled",
        })
        return
    }
    
    status := gin.H{
        "enabled":     s.mqttEnabled,
        "port":        s.mqttConfig.Port,
        "connected_devices": len(s.deviceClientIDs),
        "shadow_count": s.mqttBroker.ShadowStore().Count(),
    }
    
    c.JSON(http.StatusOK, status)
}

// Device shadow endpoint
func (s *Server) handleDeviceShadow(c *gin.Context) {
    deviceID := c.Param("deviceId")
    clientID, exists := s.deviceClientIDs[deviceID]
    if !exists {
        c.JSON(http.StatusNotFound, gin.H{
            "error": "Device not connected via MQTT",
        })
        return
    }
    
    shadow := s.mqttBroker.ShadowStore().GetShadow(clientID)
    if shadow == nil {
        c.JSON(http.StatusNotFound, gin.H{
            "error": "Shadow not found",
        })
        return
    }
    
    c.JSON(http.StatusOK, shadow)
}
```

### 7. Testing Strategy

#### A. Unit Tests
```go
// pkg/service/mqtt/shadow_test.go
func TestShadowStore_UpdateShadow(t *testing.T) {
    store := NewShadowStore()
    
    payload := []byte(`{
        "state": {
            "reported": {
                "powerState": "ON",
                "volume": 25
            }
        }
    }`)
    
    shadow, err := store.UpdateShadow("test-client", payload)
    assert.NoError(t, err)
    assert.Equal(t, "ON", shadow.State.Reported["powerState"])
    assert.Equal(t, 25.0, shadow.State.Reported["volume"])
    assert.Equal(t, 1, shadow.Version)
}
```

#### B. Integration Tests
```go
// pkg/service/mqtt/integration_test.go
func TestMQTTBrokerIntegration(t *testing.T) {
    // Start test broker
    broker := setupTestBroker(t)
    go broker.Start()
    defer broker.Stop()
    
    // Connect test client
    client := mqtt.NewClient(mqtt.NewClientOptions().
        AddBroker("tls://localhost:8883").
        SetClientID("test-device"))
    
    // Test shadow operations
    testShadowUpdate(t, client)
    testShadowGet(t, client)
}
```

### 8. Migration Path

#### A. Gradual Rollout
1. **Phase 1**: Deploy MQTT broker alongside existing HTTP service (disabled by default)
2. **Phase 2**: Enable MQTT for testing with specific devices
3. **Phase 3**: Enable bidirectional HTTP ↔ MQTT bridging
4. **Phase 4**: Full MQTT support for all discovered devices
5. **Phase 5**: Prepare for AWS IoT shutdown (May 2026)

#### B. Backward Compatibility
- All existing HTTP API endpoints continue to work
- MQTT is purely additive functionality
- Devices can be discovered via HTTP even with MQTT enabled
- Configuration remains optional

### 9. Monitoring and Logging

#### A. MQTT Metrics
```go
type MQTTMetrics struct {
    ConnectedDevices   int64
    MessagesReceived   int64
    MessagesSent       int64
    ShadowUpdates      int64
    AuthenticationFails int64
    Uptime             time.Duration
}

func (b *Broker) GetMetrics() *MQTTMetrics {
    return &MQTTMetrics{
        ConnectedDevices:   int64(len(b.server.Clients)),
        MessagesReceived:   b.server.Stats.MessagesReceived,
        MessagesSent:       b.server.Stats.MessagesSent,
        ShadowUpdates:      b.shadowStore.UpdateCount(),
        AuthenticationFails: b.authHook.FailCount(),
        Uptime:            time.Since(b.startTime),
    }
}
```

#### B. Logging Integration
```go
import "github.com/sirupsen/logrus"

func (b *Broker) setupLogging() {
    log := logrus.WithFields(logrus.Fields{
        "component": "mqtt-broker",
        "port":      b.config.Port,
    })
    
    b.server.AddHook(&LoggingHook{logger: log}, nil)
}
```

### 10. Security Considerations

#### A. Certificate Validation
- Validate device certificates against known device list
- Implement certificate revocation checking
- Support certificate rotation

#### B. Access Control
- Restrict topic access per device certificate
- Implement rate limiting per client
- Monitor for unusual connection patterns

#### C. Data Protection
- Encrypt shadow data at rest
- Implement secure certificate storage
- Audit logging for security events

## Implementation Timeline

### Week 1: Core Infrastructure
- [ ] Create MQTT package structure
- [ ] Implement basic MQTT broker
- [ ] Add TLS configuration
- [ ] Basic shadow state management

### Week 2: Integration & Bridging  
- [ ] Integrate with existing Server struct
- [ ] Implement HTTP ↔ MQTT bridge
- [ ] Device discovery integration
- [ ] Configuration management

### Week 3: Testing & Polish
- [ ] Unit test coverage
- [ ] Integration testing
- [ ] Documentation updates
- [ ] Performance optimization

### Week 4: Deployment & Monitoring
- [ ] Docker container updates
- [ ] Monitoring and metrics
- [ ] Security hardening
- [ ] Production readiness

## Success Criteria

1. **Functional**: MQTT broker accepts device connections using extracted certificates
2. **Compatible**: All existing HTTP functionality continues to work unchanged  
3. **Performant**: MQTT operations don't impact HTTP API performance
4. **Secure**: Device authentication and authorization properly implemented
5. **Observable**: Comprehensive logging and metrics for MQTT operations
6. **Maintainable**: Clean separation of MQTT code from existing HTTP logic

This design provides a comprehensive path to add MQTT support while maintaining the existing architecture and ensuring smooth integration with current functionality.