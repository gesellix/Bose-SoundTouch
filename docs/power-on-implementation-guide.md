# /power_on Implementation Guide

## Overview

This guide provides detailed technical specifications for implementing `/power_on` endpoint enhancements to reduce network dependency and improve device lifecycle management in the SoundTouch service.

## Current /power_on Handler Analysis

### Existing Implementation
Located in `pkg/service/handlers/handlers_marge.go`:

```go
func (s *Server) HandleMargePowerOn(w http.ResponseWriter, r *http.Request) {
    body, err := io.ReadAll(r.Body)
    if err != nil {
        log.Printf("[Marge] Failed to read power_on body: %v", err)
        w.WriteHeader(http.StatusOK)
        return
    }

    var req models.CustomerSupportRequest
    if err := xml.Unmarshal(body, &req); err != nil {
        log.Printf("[Marge] Failed to parse power_on body: %v", err)
        // Fallback to remote address
        if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
            go s.PrimeDeviceWithSpotify(host)
        }
        w.WriteHeader(http.StatusOK)
        return
    }

    deviceID := req.Device.ID
    deviceIP := req.DiagnosticData.DeviceLandscape.IPAddress

    log.Printf("[Marge] Device %s powered on (IP: %s)", deviceID, deviceIP)

    if deviceIP != "" {
        go s.PrimeDeviceWithSpotify(deviceIP)
    } else {
        // Fallback to remote address
        if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
            go s.PrimeDeviceWithSpotify(host)
        }
    }

    w.WriteHeader(http.StatusOK)
}
```

**Current Limitations:**
- Only extracts basic device ID and IP
- No device state management
- No data persistence
- No response payload
- Limited to Spotify priming

## Enhanced Implementation Design

### 1. Extended Data Models

#### Enhanced Power-On Request Model
```go
// PowerOnRequest represents the enhanced power_on request structure
type PowerOnRequest struct {
    XMLName        xml.Name `xml:"device-data"`
    Device         PowerOnDevice `xml:"device"`
    DiagnosticData DiagnosticData `xml:"diagnostic-data"`
}

type PowerOnDevice struct {
    ID              string `xml:"id,attr"`
    SerialNumber    string `xml:"serialnumber"`
    FirmwareVersion string `xml:"firmware-version"`
    Product         PowerOnProduct `xml:"product"`
}

type PowerOnProduct struct {
    ProductCode  string `xml:"product_code,attr"`
    Type         string `xml:"type,attr"`
    SerialNumber string `xml:"serialnumber"`
}

type DiagnosticData struct {
    DeviceLandscape DeviceLandscape `xml:"device-landscape"`
    NetworkData     NetworkData     `xml:"network-landscape>network-data"`
}

type DeviceLandscape struct {
    RSSI               string   `xml:"rssi"`
    GatewayIP          string   `xml:"gateway-ip-address"`
    MacAddresses       []string `xml:"macaddresses>macaddress"`
    IPAddress          string   `xml:"ip-address"`
    ConnectionType     string   `xml:"network-connection-type"`
}
```

#### Enhanced Response Model
```go
// PowerOnResponse represents the response sent back to the device
type PowerOnResponse struct {
    XMLName              xml.Name              `xml:"power-on-response"`
    Status               string                `xml:"status"`
    DeviceID             string                `xml:"device-id"`
    ConfigurationUpdates []ConfigurationUpdate `xml:"configuration-updates>update,omitempty"`
    MigrationInstructions *MigrationInstruction `xml:"migration,omitempty"`
    RegistrationRequired bool                  `xml:"registration-required,omitempty"`
    Timestamp            string                `xml:"timestamp"`
}

type ConfigurationUpdate struct {
    Type     string `xml:"type,attr"`
    Key      string `xml:"key"`
    Value    string `xml:"value"`
    Priority int    `xml:"priority,attr"`
}

type MigrationInstruction struct {
    Method    string            `xml:"method,attr"`
    TargetURL string            `xml:"target-url"`
    ProxyURL  string            `xml:"proxy-url,omitempty"`
    Options   map[string]string `xml:"options>option"`
}
```

### 2. Enhanced PowerOn Handler

```go
// HandleMargePowerOnEnhanced processes power_on requests with full device lifecycle management
func (s *Server) HandleMargePowerOnEnhanced(w http.ResponseWriter, r *http.Request) {
    startTime := time.Now()
    
    // Parse the power_on request
    powerOnReq, err := s.parsePowerOnRequest(r)
    if err != nil {
        s.handlePowerOnError(w, r, "Failed to parse request", err)
        return
    }

    // Process device information
    deviceInfo, isNewDevice, err := s.processDeviceFromPowerOn(powerOnReq)
    if err != nil {
        s.handlePowerOnError(w, r, "Failed to process device", err)
        return
    }

    // Build response based on device state
    response := s.buildPowerOnResponse(deviceInfo, isNewDevice, powerOnReq)

    // Log the interaction
    s.logPowerOnInteraction(deviceInfo, powerOnReq, response, startTime)

    // Send response
    if err := s.sendPowerOnResponse(w, response); err != nil {
        log.Printf("[PowerOn] Failed to send response for device %s: %v", deviceInfo.DeviceID, err)
    }
}
```

### 3. Device Processing Logic

```go
// processDeviceFromPowerOn handles device identification and data updates
func (s *Server) processDeviceFromPowerOn(req *PowerOnRequest) (*models.ServiceDeviceInfo, bool, error) {
    deviceMAC := req.Device.ID
    deviceIP := req.DiagnosticData.DeviceLandscape.IPAddress
    
    // Try to find existing device by MAC address (primary identifier)
    existingDevice, err := s.ds.GetDeviceByMAC(deviceMAC)
    if err != nil && err != datastore.ErrDeviceNotFound {
        return nil, false, fmt.Errorf("failed to lookup device: %w", err)
    }

    var deviceInfo *models.ServiceDeviceInfo
    isNewDevice := existingDevice == nil

    if isNewDevice {
        // Create new device record from power_on data
        deviceInfo = s.createDeviceFromPowerOn(req)
        
        // Store in datastore
        if err := s.ds.SaveDeviceInfo("", deviceMAC, deviceInfo); err != nil {
            return nil, false, fmt.Errorf("failed to save new device: %w", err)
        }
        
        log.Printf("[PowerOn] New device registered: %s (IP: %s, Model: %s)", 
                   deviceMAC, deviceIP, deviceInfo.ProductCode)
    } else {
        // Update existing device with power_on data
        deviceInfo = existingDevice
        s.updateDeviceFromPowerOn(deviceInfo, req)
        
        // Detect significant changes
        if s.hasSignificantChanges(existingDevice, deviceInfo) {
            log.Printf("[PowerOn] Device %s updated: IP %s->%s, FW %s->%s",
                       deviceMAC, existingDevice.IPAddress, deviceInfo.IPAddress,
                       existingDevice.FirmwareVersion, deviceInfo.FirmwareVersion)
        }
        
        // Save updated device info
        if err := s.ds.SaveDeviceInfo(deviceInfo.AccountID, deviceMAC, deviceInfo); err != nil {
            return nil, false, fmt.Errorf("failed to update device: %w", err)
        }
    }

    // Update device mappings for lookup optimization
    s.ds.UpdateDeviceMappings(*deviceInfo)
    
    return deviceInfo, isNewDevice, nil
}
```

### 4. Device Creation from Power-On Data

```go
// createDeviceFromPowerOn creates a new ServiceDeviceInfo from power_on request
func (s *Server) createDeviceFromPowerOn(req *PowerOnRequest) *models.ServiceDeviceInfo {
    now := time.Now()
    
    deviceInfo := &models.ServiceDeviceInfo{
        DeviceID:            req.Device.ID, // MAC address
        ProductCode:         req.Device.Product.ProductCode,
        DeviceSerialNumber:  req.Device.SerialNumber,
        ProductSerialNumber: req.Device.Product.SerialNumber,
        FirmwareVersion:     req.Device.FirmwareVersion,
        IPAddress:           req.DiagnosticData.DeviceLandscape.IPAddress,
        MacAddress:          req.Device.ID, // Primary MAC
        DiscoveryMethod:     "power_on",
        LastSeen:            now,
        CreatedAt:           now,
        UpdatedAt:           now,
    }

    // Generate default name if not provided
    if deviceInfo.Name == "" {
        deviceInfo.Name = s.generateDefaultDeviceName(deviceInfo)
    }

    // Add power_on specific metadata
    deviceInfo.Metadata = map[string]string{
        "rssi":            req.DiagnosticData.DeviceLandscape.RSSI,
        "gateway_ip":      req.DiagnosticData.DeviceLandscape.GatewayIP,
        "connection_type": req.DiagnosticData.DeviceLandscape.ConnectionType,
        "power_on_count":  "1",
    }

    // Store additional MAC addresses if available
    if len(req.DiagnosticData.DeviceLandscape.MacAddresses) > 1 {
        additionalMACs := make([]string, 0, len(req.DiagnosticData.DeviceLandscape.MacAddresses)-1)
        for _, mac := range req.DiagnosticData.DeviceLandscape.MacAddresses {
            if mac != req.Device.ID {
                additionalMACs = append(additionalMACs, mac)
            }
        }
        if len(additionalMACs) > 0 {
            deviceInfo.Metadata["additional_macs"] = strings.Join(additionalMACs, ",")
        }
    }

    return deviceInfo
}
```

### 5. Response Generation Logic

```go
// buildPowerOnResponse creates appropriate response based on device state
func (s *Server) buildPowerOnResponse(deviceInfo *models.ServiceDeviceInfo, isNewDevice bool, req *PowerOnRequest) *PowerOnResponse {
    response := &PowerOnResponse{
        Status:    "ok",
        DeviceID:  deviceInfo.DeviceID,
        Timestamp: time.Now().Format(time.RFC3339),
    }

    // Handle new device registration
    if isNewDevice {
        response.RegistrationRequired = deviceInfo.AccountID == ""
        
        // Add welcome configuration for new devices
        response.ConfigurationUpdates = []ConfigurationUpdate{
            {
                Type:     "welcome",
                Key:      "device_registered",
                Value:    "true",
                Priority: 1,
            },
        }
    }

    // Check if migration is needed
    if s.needsMigration(deviceInfo) {
        migration := s.getMigrationInstructions(deviceInfo)
        response.MigrationInstructions = migration
        
        log.Printf("[PowerOn] Migration required for device %s: %s", 
                   deviceInfo.DeviceID, migration.Method)
    }

    // Add any pending configuration updates
    pendingUpdates := s.getPendingConfigurationUpdates(deviceInfo)
    response.ConfigurationUpdates = append(response.ConfigurationUpdates, pendingUpdates...)

    return response
}
```

### 6. Device Lookup Enhancements

#### Enhanced DataStore Methods
```go
// GetDeviceByMAC finds a device by MAC address across all accounts
func (ds *DataStore) GetDeviceByMAC(macAddress string) (*models.ServiceDeviceInfo, error) {
    normalizedMAC := normalizeMAC(macAddress)
    
    // Check device mappings first (for performance)
    ds.idMutex.RLock()
    deviceID, exists := ds.deviceMappings[normalizedMAC]
    ds.idMutex.RUnlock()
    
    if exists {
        // Try to find device by mapped ID
        device, err := ds.findDeviceByID(deviceID)
        if err == nil {
            return device, nil
        }
    }

    // Fallback to full scan
    devices, err := ds.ListAllDevices()
    if err != nil {
        return nil, err
    }

    for _, device := range devices {
        if normalizeMAC(device.MacAddress) == normalizedMAC || 
           normalizeMAC(device.DeviceID) == normalizedMAC {
            return &device, nil
        }
        
        // Check additional MAC addresses in metadata
        if additionalMACs, exists := device.Metadata["additional_macs"]; exists {
            for _, mac := range strings.Split(additionalMACs, ",") {
                if normalizeMAC(mac) == normalizedMAC {
                    return &device, nil
                }
            }
        }
    }

    return nil, datastore.ErrDeviceNotFound
}
```

### 7. Migration Integration

```go
// needsMigration determines if device requires configuration migration
func (s *Server) needsMigration(deviceInfo *models.ServiceDeviceInfo) bool {
    if deviceInfo.AccountID == "" {
        return false // Cannot migrate without account
    }

    // Check if device is already migrated
    if s.sm != nil {
        summary, err := s.sm.GetMigrationSummary(deviceInfo.IPAddress, s.ServerURL, "", nil)
        if err == nil && summary.IsMigrated {
            return false
        }
    }

    return true
}

// getMigrationInstructions creates migration instructions for device
func (s *Server) getMigrationInstructions(deviceInfo *models.ServiceDeviceInfo) *MigrationInstruction {
    return &MigrationInstruction{
        Method:    "xml", // Default to XML-based migration
        TargetURL: s.ServerURL,
        Options: map[string]string{
            "marge":     "true",
            "stats":     "true",
            "sw_update": "true",
        },
    }
}
```

### 8. Error Handling and Fallbacks

```go
// handlePowerOnError provides graceful error handling with fallbacks
func (s *Server) handlePowerOnError(w http.ResponseWriter, r *http.Request, message string, err error) {
    log.Printf("[PowerOn] %s: %v", message, err)
    
    // Try to extract IP from request for fallback processing
    if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
        // Fallback to existing discovery mechanism
        go s.PrimeDeviceWithSpotify(host)
        log.Printf("[PowerOn] Falling back to legacy processing for IP %s", host)
    }

    // Always return 200 OK to avoid device retry loops
    w.WriteHeader(http.StatusOK)
}

// parsePowerOnRequest safely parses the power_on request with validation
func (s *Server) parsePowerOnRequest(r *http.Request) (*PowerOnRequest, error) {
    body, err := io.ReadAll(r.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read request body: %w", err)
    }

    if len(body) == 0 {
        return nil, fmt.Errorf("empty request body")
    }

    var req PowerOnRequest
    if err := xml.Unmarshal(body, &req); err != nil {
        return nil, fmt.Errorf("failed to parse XML: %w", err)
    }

    // Validate required fields
    if req.Device.ID == "" {
        return nil, fmt.Errorf("missing device ID")
    }

    if req.DiagnosticData.DeviceLandscape.IPAddress == "" {
        return nil, fmt.Errorf("missing device IP address")
    }

    return &req, nil
}
```

### 9. Logging and Monitoring

```go
// logPowerOnInteraction records detailed interaction logs for debugging
func (s *Server) logPowerOnInteraction(deviceInfo *models.ServiceDeviceInfo, req *PowerOnRequest, resp *PowerOnResponse, startTime time.Time) {
    duration := time.Since(startTime)
    
    log.Printf("[PowerOn] Device: %s, IP: %s, Duration: %v, Status: %s, NewDevice: %t, Migration: %t",
               deviceInfo.DeviceID,
               req.DiagnosticData.DeviceLandscape.IPAddress,
               duration,
               resp.Status,
               resp.RegistrationRequired,
               resp.MigrationInstructions != nil)

    // Store interaction for debugging (if enabled)
    if s.config.RecordInteractions {
        interaction := models.DeviceInteraction{
            Timestamp:   startTime,
            DeviceID:    deviceInfo.DeviceID,
            Type:        "power_on",
            Request:     req,
            Response:    resp,
            Duration:    duration,
            IPAddress:   req.DiagnosticData.DeviceLandscape.IPAddress,
            UserAgent:   r.Header.Get("User-Agent"),
        }
        
        if err := s.ds.SaveInteraction(interaction); err != nil {
            log.Printf("[PowerOn] Failed to save interaction: %v", err)
        }
    }
}
```

### 10. Configuration and Feature Flags

```go
// PowerOnConfig controls behavior of enhanced power_on processing
type PowerOnConfig struct {
    EnableEnhancedProcessing bool          `json:"enable_enhanced_processing"`
    AutoMigration           bool          `json:"auto_migration"`
    RecordInteractions      bool          `json:"record_interactions"`
    DefaultResponseTimeout  time.Duration `json:"default_response_timeout"`
    FallbackToLegacy       bool          `json:"fallback_to_legacy"`
}

// loadPowerOnConfig loads configuration with defaults
func loadPowerOnConfig() *PowerOnConfig {
    return &PowerOnConfig{
        EnableEnhancedProcessing: true,
        AutoMigration:           false, // Conservative default
        RecordInteractions:      false,
        DefaultResponseTimeout:  5 * time.Second,
        FallbackToLegacy:       true,
    }
}
```

## Testing Strategy

### 1. Unit Tests
```go
func TestHandleMargePowerOnEnhanced(t *testing.T) {
    tests := []struct {
        name           string
        requestBody    string
        existingDevice *models.ServiceDeviceInfo
        expectedStatus string
        expectMigration bool
    }{
        {
            name: "new_device_registration",
            requestBody: `<device-data><device id="A81B6A536A98">...</device></device-data>`,
            existingDevice: nil,
            expectedStatus: "ok",
            expectMigration: false,
        },
        {
            name: "existing_device_update",
            requestBody: `<device-data><device id="A81B6A536A98">...</device></device-data>`,
            existingDevice: &models.ServiceDeviceInfo{DeviceID: "A81B6A536A98"},
            expectedStatus: "ok",
            expectMigration: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### 2. Integration Tests
```go
func TestPowerOnDeviceLifecycle(t *testing.T) {
    // Test complete device lifecycle through power_on events
    // 1. New device power_on
    // 2. Device registration
    // 3. Configuration changes
    // 4. Migration
    // 5. Subsequent power_on events
}
```

### 3. Load Testing
```go
func BenchmarkPowerOnProcessing(b *testing.B) {
    // Benchmark power_on processing performance
    // Test concurrent device registrations
    // Measure response times
}
```

## Deployment Strategy

### Phase 1: Parallel Implementation
- Implement enhanced handler alongside existing handler
- Use feature flag to control which handler processes requests
- Maintain full backward compatibility

### Phase 2: Gradual Rollout
- Enable enhanced processing for subset of devices
- Monitor performance and error rates
- Collect metrics on data completeness

### Phase 3: Full Migration
- Default to enhanced processing for all devices
- Remove legacy fallbacks
- Optimize performance based on production data

## Monitoring and Metrics

### Key Metrics to Track
- Power-on event frequency per device
- New device registration rate via power_on
- Migration success rate via power_on response
- Response time distribution
- Error rates and types
- Data completeness metrics

### Alerting Thresholds
- Power-on processing failures > 5%
- Average response time > 2 seconds
- New device registration failures > 1%
- Migration instruction delivery failures > 2%

## Security Considerations

### Input Validation
- XML parsing security (prevent XXE attacks)
- Device ID format validation
- IP address validation
- Request size limits

### Authentication
- Device authentication via MAC address verification
- Request signing (if available)
- Rate limiting per device/IP

### Data Privacy
- Sensitive data handling in diagnostic information
- Logging data retention policies
- Compliance with data protection regulations