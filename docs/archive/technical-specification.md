# Technical Specification - Enhanced State Management System

## Table of Contents

1. [System Architecture](#system-architecture)
2. [Data Models](#data-models)
3. [API Specifications](#api-specifications)
4. [File Format Specifications](#file-format-specifications)
5. [State Machine Definitions](#state-machine-definitions)
6. [Event Processing](#event-processing)
7. [Performance Requirements](#performance-requirements)
8. [Security Considerations](#security-considerations)
9. [Error Handling](#error-handling)
10. [Monitoring and Observability](#monitoring-and-observability)

## System Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    SoundTouch Service                       │
├─────────────────────────────────────────────────────────────┤
│  HTTP Router & Middleware                                   │
│  ├── Recorder Middleware                                    │
│  ├── Disparity Detection                                    │
│  └── Health Check Middleware                               │
├─────────────────────────────────────────────────────────────┤
│  Service Layer                                              │
│  ├── Account Manager          ├── Lifecycle Manager        │
│  ├── Migration Controller     ├── Data Source Router       │
│  ├── Event Processor         ├── Health Monitor           │
│  └── Export Manager          └── Analytics Engine         │
├─────────────────────────────────────────────────────────────┤
│  Data Layer                                                 │
│  ├── Enhanced DataStore      ├── Event Store              │
│  └── Configuration Store    └── Session Store             │
├─────────────────────────────────────────────────────────────┤
│  External Integrations                                      │
│  ├── Device Discovery         ├── BMX/TuneIn Services      │
│  └── SSH/Setup Manager                                   │
└─────────────────────────────────────────────────────────────┘
```

### Package Structure

```
pkg/service/
├── account/               # Account management
│   ├── manager.go
│   ├── persistence.go
│   └── validation.go
├── lifecycle/             # Device lifecycle management
│   ├── manager.go
│   ├── states.go
│   ├── transitions.go
│   └── events.go
├── migration/            # Enhanced migration (extends existing)
│   ├── controller.go
│   ├── strategies.go
│   └── progress.go
├── datasource/           # Data source routing
│   ├── router.go
│   ├── preferences.go
│   └── fallback.go
├── events/               # Event processing system
│   ├── processor.go
│   ├── queue.go
│   └── storage.go
├── health/               # System monitoring
│   ├── monitor.go
│   ├── metrics.go
│   └── alerts.go
└── export/               # Data export and backup
    ├── exporter.go
    ├── formats.go
    └── backup.go
```

## Data Models

### Account Model

```go
type Account struct {
    ID             string           `json:"id"`
    Name           string           `json:"name"`
    Email          string           `json:"email,omitempty"`
    CreatedAt      time.Time        `json:"created_at"`
    UpdatedAt      time.Time        `json:"updated_at"`
    Status         AccountStatus    `json:"status"`
    DeviceCount    int              `json:"device_count"`
    MigrationInfo  *MigrationInfo   `json:"migration_info,omitempty"`
    BoseAccountID  string           `json:"bose_account_id,omitempty"`
    DataSources    DataSourceConfig `json:"data_sources"`
    Settings       AccountSettings  `json:"settings"`
}

type AccountStatus string
const (
    AccountStatusActive    AccountStatus = "active"
    AccountStatusMigrating AccountStatus = "migrating"
    AccountStatusSuspended AccountStatus = "suspended"
    AccountStatusArchived  AccountStatus = "archived"
)

type MigrationInfo struct {
    StartedAt        time.Time `json:"started_at"`
    CompletedAt      *time.Time `json:"completed_at,omitempty"`
    DevicesMigrated  int       `json:"devices_migrated"`
    DevicesPending   int       `json:"devices_pending"`
    Strategy         string    `json:"strategy"`
    RollbackData     string    `json:"rollback_data,omitempty"`
}

type DataSourceConfig struct {
    Local       bool   `json:"local"`
    Primary     string `json:"primary"` // "local" or "bose"
}

type AccountSettings struct {
    AutoMigration    bool     `json:"auto_migration"`
    RetentionDays    int      `json:"retention_days"`
}
```

### Device Lifecycle Model

```go
type DeviceLifecycle struct {
    DeviceID     string            `json:"device_id"`
    AccountID    string            `json:"account_id"`
    State        DeviceState       `json:"state"`
    CreatedAt    time.Time         `json:"created_at"`
    UpdatedAt    time.Time         `json:"updated_at"`
    StateHistory []StateTransition `json:"state_history"`
    Metadata     DeviceMetadata    `json:"metadata"`
    DataSources  DataSourceConfig  `json:"data_sources"`
    Migration    *DeviceMigration  `json:"migration,omitempty"`
    Health       DeviceHealth      `json:"health"`
}

type DeviceState string
const (
    DeviceStateUnregistered DeviceState = "unregistered"
    DeviceStateDiscovered   DeviceState = "discovered"
    DeviceStateRegistering  DeviceState = "registering"
    DeviceStateActive       DeviceState = "active"
    DeviceStateMigrating    DeviceState = "migrating"
    DeviceStateOffline      DeviceState = "offline"
    DeviceStateError        DeviceState = "error"
    DeviceStateRetired      DeviceState = "retired"
)

type StateTransition struct {
    From      DeviceState `json:"from"`
    To        DeviceState `json:"to"`
    Timestamp time.Time   `json:"timestamp"`
    Reason    string      `json:"reason"`
    Source    string      `json:"source"`
    Context   map[string]interface{} `json:"context,omitempty"`
}

type DeviceMetadata struct {
    Name            string    `json:"name"`
    Type            string    `json:"type"`
    SerialNumber    string    `json:"serial_number"`
    FirmwareVersion string    `json:"firmware_version"`
    MACAddress      string    `json:"mac_address"`
    IPAddress       string    `json:"ip_address"`
    LastSeen        time.Time `json:"last_seen"`
    IsLegacyID      bool      `json:"is_legacy_id"`
    Capabilities    []string  `json:"capabilities,omitempty"`
}

type DeviceMigration struct {
    FromBoseAccount string     `json:"from_bose_account,omitempty"`
    MigratedAt      *time.Time `json:"migrated_at,omitempty"`
    Method          string     `json:"method"`
    RollbackAvailable bool     `json:"rollback_available"`
    DataPreserved   []string   `json:"data_preserved"`
}

type DeviceHealth struct {
    Status          string    `json:"status"` // "healthy", "warning", "error"
    LastCheck       time.Time `json:"last_check"`
    ResponseTime    int       `json:"response_time_ms"`
    Connectivity    string    `json:"connectivity"` // "online", "offline", "intermittent"
    ErrorCount      int       `json:"error_count"`
    LastError       string    `json:"last_error,omitempty"`
}
```

### Event Model

```go
type DeviceEvent struct {
    ID        string                 `json:"id"`
    DeviceID  string                 `json:"device_id"`
    AccountID string                 `json:"account_id"`
    Type      DeviceEventType        `json:"type"`
    Data      map[string]interface{} `json:"data"`
    Timestamp time.Time              `json:"timestamp"`
    Source    EventSource            `json:"source"`
    Processed bool                   `json:"processed"`
    Context   EventContext           `json:"context"`
}

type DeviceEventType string
const (
    EventTypeNowPlaying      DeviceEventType = "now_playing"
    EventTypePresetChanged   DeviceEventType = "preset_changed"
    EventTypeVolumeChanged   DeviceEventType = "volume_changed"
    EventTypeSourceChanged   DeviceEventType = "source_changed"
    EventTypeDeviceOnline    DeviceEventType = "device_online"
    EventTypeDeviceOffline   DeviceEventType = "device_offline"
    EventTypeZoneChanged     DeviceEventType = "zone_changed"
    EventTypeDisparityFound  DeviceEventType = "disparity_found"
    EventTypeMigrationStart  DeviceEventType = "migration_start"
    EventTypeMigrationEnd    DeviceEventType = "migration_end"
    EventTypeHealthCheck     DeviceEventType = "health_check"
    EventTypeErrorOccurred   DeviceEventType = "error_occurred"
)

type EventSource string
const (
    EventSourceWebSocket  EventSource = "websocket"
    EventSourceDiscovery  EventSource = "discovery"
    EventSourceSystem     EventSource = "system"
    EventSourceAPI        EventSource = "api"
    EventSourceUser       EventSource = "user"
)

type EventContext struct {
    RequestID    string                 `json:"request_id,omitempty"`
    UserAgent    string                 `json:"user_agent,omitempty"`
    IPAddress    string                 `json:"ip_address,omitempty"`
    Endpoint     string                 `json:"endpoint,omitempty"`
    Additional   map[string]interface{} `json:"additional,omitempty"`
}
```

### Disparity Model

```go
type Disparity struct {
    ID          string                 `json:"id"`
    Timestamp   time.Time              `json:"timestamp"`
    DeviceID    string                 `json:"device_id"`
    AccountID   string                 `json:"account_id"`
    Endpoint    string                 `json:"endpoint"`
    Type        DisparityType          `json:"type"`
    Severity    DisparitySeverity      `json:"severity"`
    LocalHash   string                 `json:"local_hash"`
    UpstreamHash string                `json:"upstream_hash"`
    Details     DisparityDetails       `json:"details"`
    Context     map[string]interface{} `json:"context"`
    Resolved    bool                   `json:"resolved"`
}

type DisparityType string
const (
    DisparityTypeContentMismatch  DisparityType = "content_mismatch"
    DisparityTypeStructureDiff    DisparityType = "structure_diff"
    DisparityTypeTimestampFormat  DisparityType = "timestamp_format"
    DisparityTypeFieldMissing     DisparityType = "field_missing"
    DisparityTypeValueMismatch    DisparityType = "value_mismatch"
    DisparityTypeCountMismatch    DisparityType = "count_mismatch"
)

type DisparitySeverity string
const (
    DisparitySeverityLow      DisparitySeverity = "low"
    DisparitySeverityMedium   DisparitySeverity = "medium"
    DisparitySeverityHigh     DisparitySeverity = "high"
    DisparitySeverityCritical DisparitySeverity = "critical"
)

type DisparityDetails struct {
    FieldPath     string      `json:"field_path"`
    LocalValue    interface{} `json:"local_value"`
    UpstreamValue interface{} `json:"upstream_value"`
    Description   string      `json:"description"`
}
```

## API Specifications

### Account Management APIs

#### Create Account
```http
POST /api/v1/accounts
Content-Type: application/json

{
  "name": "User Account",
  "email": "user@example.com",
  "settings": {
    "auto_migration": false,
    "retention_days": 30
  }
}

Response: 201 Created
{
  "id": "acc_12345",
  "name": "User Account",
  "email": "user@example.com",
  "created_at": "2024-01-20T10:00:00Z",
  "status": "active",
  "device_count": 0
}
```

#### Get Account
```http
GET /api/v1/accounts/{account_id}

Response: 200 OK
{
  "id": "acc_12345",
  "name": "User Account",
  "status": "active",
  "device_count": 2,
  "migration_info": {
    "started_at": "2024-01-18T09:00:00Z",
    "devices_migrated": 1,
    "devices_pending": 1
  },
  "data_sources": {
    "local": true,
    "primary": "bose"
  }
}
```

#### List Accounts
```http
GET /api/v1/accounts?status=active&limit=10&offset=0

Response: 200 OK
{
  "accounts": [...],
  "total": 5,
  "limit": 10,
  "offset": 0
}
```

### Device Lifecycle APIs

#### Register Device
```http
POST /api/v1/accounts/{account_id}/devices
Content-Type: application/json

{
  "device_id": "AABBCCDDEEFF",
  "name": "Living Room Speaker",
  "registration_type": "fresh"
}

Response: 201 Created
{
  "device_id": "AABBCCDDEEFF",
  "account_id": "acc_12345",
  "state": "registering",
  "created_at": "2024-01-20T10:00:00Z"
}
```

#### Get Device State
```http
GET /api/v1/accounts/{account_id}/devices/{device_id}/state

Response: 200 OK
{
  "device_id": "AABBCCDDEEFF",
  "account_id": "acc_12345",
  "state": "active",
  "metadata": {
    "name": "Living Room Speaker",
    "type": "SoundTouch 30",
    "last_seen": "2024-01-20T15:30:00Z"
  },
  "health": {
    "status": "healthy",
    "connectivity": "online",
    "response_time": 45
  }
}
```

#### Migrate Device
```http
POST /api/v1/accounts/{account_id}/devices/{device_id}/migrate
Content-Type: application/json

{
  "from_bose_account": "bose-acc-xyz",
  "preserve_data": true,
  "method": "gradual"
}

Response: 202 Accepted
{
  "migration_id": "mig_67890",
  "status": "started",
  "estimated_completion": "2024-01-20T11:00:00Z"
}
```

### Event APIs

#### Get Device Events
```http
GET /api/v1/accounts/{account_id}/devices/{device_id}/events?since=2024-01-20T00:00:00Z&type=now_playing&limit=50

Response: 200 OK
{
  "events": [
    {
      "id": "evt_12345",
      "type": "now_playing",
      "timestamp": "2024-01-20T15:30:00Z",
      "data": {
        "source": "SPOTIFY",
        "track": "Song Name",
        "artist": "Artist Name"
      }
    }
  ],
  "total": 125,
  "has_more": true
}
```

#### Stream Events
```http
GET /api/v1/accounts/{account_id}/devices/{device_id}/events/stream
Accept: text/event-stream

Response: 200 OK
Content-Type: text/event-stream

data: {"id":"evt_12346","type":"volume_changed","timestamp":"2024-01-20T15:31:00Z","data":{"volume":50}}

data: {"id":"evt_12347","type":"now_playing","timestamp":"2024-01-20T15:32:00Z","data":{"source":"TUNEIN"}}
```

### Monitoring APIs

#### System Health
```http
GET /api/v1/system/health

Response: 200 OK
{
  "status": "healthy",
  "timestamp": "2024-01-20T15:30:00Z",
  "services": {
    "account_manager": "healthy",
    "lifecycle_manager": "healthy",
    "event_processor": "healthy"
  },
  "statistics": {
    "total_accounts": 5,
    "total_devices": 12,
    "active_devices": 10,
    "events_processed_24h": 1547
  }
}
```

#### Disparity Analysis
```http
GET /api/v1/system/disparities?since=2024-01-20T00:00:00Z&severity=high

Response: 200 OK
{
  "disparities": [
    {
      "id": "disp_12345",
      "timestamp": "2024-01-20T14:30:00Z",
      "endpoint": "/v1/presets",
      "type": "count_mismatch",
      "severity": "high",
      "details": {
        "field_path": "preset_count",
        "local_value": 5,
        "upstream_value": 4
      }
    }
  ],
  "summary": {
    "total": 15,
    "by_severity": {
      "high": 2,
      "medium": 8,
      "low": 5
    }
  }
}
```

## File Format Specifications

### Account Metadata (account.json)
```json
{
  "version": "1.0",
  "id": "acc_12345",
  "name": "User Account",
  "email": "user@example.com",
  "created_at": "2024-01-20T10:00:00Z",
  "updated_at": "2024-01-20T15:30:00Z",
  "status": "active",
  "device_count": 2,
  "migration_info": {
    "started_at": "2024-01-18T09:00:00Z",
    "devices_migrated": 1,
    "devices_pending": 1,
    "strategy": "gradual"
  },
  "bose_account_id": "bose-original-id",
  "data_sources": {
    "local": true,
    "primary": "bose"
  },
  "settings": {
    "auto_migration": false,
    "retention_days": 30
  }
}
```

### Device Lifecycle (lifecycle.json)
```json
{
  "version": "1.0",
  "device_id": "AABBCCDDEEFF",
  "account_id": "acc_12345",
  "state": "active",
  "created_at": "2024-01-20T10:00:00Z",
  "updated_at": "2024-01-20T15:30:00Z",
  "state_history": [
    {
      "from": "unregistered",
      "to": "discovered",
      "timestamp": "2024-01-20T10:00:00Z",
      "reason": "mdns_discovery",
      "source": "discovery",
      "context": {
        "ip_address": "192.0.2.100",
        "discovery_method": "mdns"
      }
    },
    {
      "from": "discovered",
      "to": "active",
      "timestamp": "2024-01-20T10:05:00Z",
      "reason": "registration_complete",
      "source": "system"
    }
  ],
  "metadata": {
    "name": "Living Room Speaker",
    "type": "SoundTouch 30",
    "serial_number": "I6332527703739342000020",
    "firmware_version": "4.8.1.25341.2677643.1597353330",
    "mac_address": "AA:BB:CC:DD:EE:FF",
    "ip_address": "192.0.2.100",
    "last_seen": "2024-01-20T15:30:00Z",
    "is_legacy_id": false,
    "capabilities": ["multiroom", "bluetooth", "aux"]
  },
  "data_sources": {
    "presets": "local",
    "recents": "local",
    "sources": "local"
  },
  "migration": {
    "from_bose_account": "bose-acc-xyz",
    "migrated_at": "2024-01-18T14:30:00Z",
    "method": "gradual",
    "rollback_available": true,
    "data_preserved": ["presets", "recents", "sources"]
  },
  "health": {
    "status": "healthy",
    "last_check": "2024-01-20T15:30:00Z",
    "response_time": 45,
    "connectivity": "online",
    "error_count": 0
  }
}
```

### Event Log Format (events.log)
```
# SoundTouch Service Event Log - Device AABBCCDDEEFF
# Format: TIMESTAMP|EVENT_ID|EVENT_TYPE|SOURCE|DATA_JSON
# Version: 1.0

2024-01-20T15:30:00.123Z|evt_12345|now_playing|websocket|{"source":"SPOTIFY","track":"Song Name","artist":"Artist Name","album":"Album Name"}
2024-01-20T15:30:30.456Z|evt_12346|volume_changed|websocket|{"volume":45,"muted":false,"previous_volume":40}
2024-01-20T15:31:00.789Z|evt_12347|preset_selected|websocket|{"preset":1,"source":"SPOTIFY","location":"spotify:track:123abc"}
2024-01-20T15:32:00.345Z|evt_12349|health_check|system|{"response_time":42,"status":"healthy","connectivity":"online"}
```

### Disparity Log Format (disparities.log)
```
# SoundTouch Service Disparity Log
# Format: TIMESTAMP|DISPARITY_ID|DEVICE_ID|ACCOUNT_ID|ENDPOINT|TYPE|SEVERITY|DETAILS_JSON
# Version: 1.0

2024-01-20T15:31:15.012Z|disp_12345|AABBCCDDEEFF|acc_12345|/v1/presets|count_mismatch|medium|{"field_path":"preset_count","local_value":5,"upstream_value":4,"description":"Local has one additional preset"}
2024-01-20T15:32:45.678Z|disp_12346|AABBCCDDEEFF|acc_12345|/v1/recents|timestamp_format|low|{"field_path":"recent[0].utc_time","local_value":"2024-01-20T15:30:00Z","upstream_value":"1705761000","description":"Timestamp format difference"}
2024-01-20T15:35:20.901Z|disp_12347|B92C7B647B09|acc_12345|/v1/account/full|structure_diff|high|{"field_path":"device[1].ip_address","local_value":"present","upstream_value":"missing","description":"IP address field missing in upstream response"}
```

## State Machine Definitions

### Device State Transitions

```
Unregistered → Discovered (via discovery)
Discovered → Registering (via user action/auto-registration)
Registering → Active (via successful registration)
Registering → Error (via registration failure)
Active → Migrating (via migration start)
Active → Offline (via connectivity loss)
Migrating → Active (via migration success)
Migrating → Error (via migration failure)
Offline → Active (via connectivity restored)
Error → Active (via error resolution)
Any State → Retired (via explicit retirement)
```

### State Transition Rules

```go
var StateTransitionRules = map[DeviceState][]DeviceState{
    DeviceStateUnregistered: {DeviceStateDiscovered},
    DeviceStateDiscovered:   {DeviceStateRegistering, DeviceStateOffline},
    DeviceStateRegistering:  {DeviceStateActive, DeviceStateError},
    DeviceStateActive:       {DeviceStateMigrating, DeviceStateOffline, DeviceStateRetired},
    DeviceStateMigrating:    {DeviceStateActive, DeviceStateError},
    DeviceStateOffline:      {DeviceStateActive, DeviceStateError, DeviceStateRetired},
    DeviceStateError:        {DeviceStateActive, DeviceStateOffline, DeviceStateRetired},
    DeviceStateRetired:      {}, // Terminal state
}
```

### Transition Triggers

```go
type TransitionTrigger struct {
    Event     DeviceEventType
    Condition func(*DeviceLifecycle, *DeviceEvent) bool
    Target    DeviceState
    Reason    string
}

var TransitionTriggers = []TransitionTrigger{
    {
        Event:     EventTypeDeviceOnline,
        Condition: isOfflineDevice,
        Target:    DeviceStateActive,
        Reason:    "connectivity_restored",
    },
    {
        Event:     EventTypeDeviceOffline,
        Condition: isActiveDevice,
        Target:    DeviceStateOffline,
        Reason:    "connectivity_lost",
    },
    {
        Event:     EventTypeMigrationStart,
        Condition: isActiveDevice,
        Target:    DeviceStateMigrating,
        Reason:    "migration_initiated",
    },
    // ... additional triggers
}
```

## Event Processing

### Event Queue Implementation

```go
type EventQueue struct {
    buffer     chan DeviceEvent
    processors []EventProcessor
    storage    EventStorage
    config     EventQueueConfig
}

type EventQueueConfig struct {
    BufferSize      int           `json:"buffer_size"`
    ProcessorCount  int           `json:"processor_count"`
    FlushInterval   time.Duration `json:"flush_interval"`
    RetryAttempts   int           `json:"retry_attempts"`
    DeadLetterQueue bool          `json:"dead_letter_queue"`
}

type EventProcessor interface {
    ProcessEvent(event DeviceEvent) error
    CanHandle(eventType DeviceEventType) bool
}
```

### Event Processing Flow

```
Event Input → Validation → Queue → Processing → Storage → Notification
     ↓            ↓          ↓         ↓          ↓          ↓
  Websocket    Schema    Buffer    Parallel   Files    Webhooks
  Discovery    Check     Memory    Workers    Logs     SSE
  API Call     Format              Retry      DB       Metrics
  System       Enrich              DLQ
```

### Event Retention Policy

```go
type RetentionPolicy struct {
    EventType     DeviceEventType `json:"event_type"`
    RetentionDays int            `json:"retention_days"`
    MaxCount      int            `json:"max_count"`
    Compression   bool           `json:"compression"`
}

var DefaultRetentionPolicies = []RetentionPolicy{
    {EventTypeNowPlaying, 7, 1000, true},
    {EventTypeVolumeChanged, 1, 100, false},
    {EventTypeDisparityFound, 30, 10000, true},
    {EventTypeMigrationStart, 365, -1, false}, // Keep forever
    {EventTypeHealthCheck, 7, 1000, true},
}
```

### Development Requirements

#### KISS Principle (Keep It Simple, Stupid)
- Prioritize simplicity and readability over performance optimization
- Use standard Go idioms and patterns
- Avoid premature abstraction and optimization
- Build the simplest thing that works first

#### Quality Gates
Every change must pass these checks:
- `golangci-lint run --fix` - no linting issues
- `go test ./...` - all tests pass with no failures
- Integration tests verify existing functionality intact
- Code coverage maintained or improved

#### Testing Requirements
- Unit tests for all new functions
- Integration tests for modified workflows
- Regression tests for existing functionality
- Mock external dependencies appropriately

### Simplicity-First Performance Approach

| Aspect | Simple Approach | Optimization Only When Needed |
|--------|----------------|-------------------------------|
| Memory Usage | Direct file operations, minimal caching | Add caching if performance issues arise |
| CPU Usage | Synchronous processing initially | Add async processing if bottlenecks occur |
| Storage | Simple append operations | Add rotation/compression when files grow large |
| Networking | Reuse existing patterns | Optimize only if latency becomes problematic |

## Quality Assurance

### Testing Strategy

#### Unit Testing
```go
// Example test structure
func TestAccountManager_CreateAccount(t *testing.T) {
    tests := []struct {
        name    string
        input   CreateAccountRequest
        want    *Account
        wantErr bool
    }{
        {
            name: "valid account creation",
            input: CreateAccountRequest{Name: "Test Account"},
            want: &Account{Name: "Test Account", Status: "active"},
            wantErr: false,
        },
        {
            name: "empty name should fail",
            input: CreateAccountRequest{Name: ""},
            want: nil,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := manager.CreateAccount(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("CreateAccount() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            // Additional assertions...
        })
    }
}
```

#### Integration Testing
- Test with real file operations in temporary directories
- Verify HTTP endpoints work with actual HTTP requests
- Test interaction with existing WebSocket system
- Ensure existing XML endpoints remain functional

#### Quality Gates
```bash
# Required before each commit
golangci-lint run --fix
go test ./...
go test -race ./...

# Required before milestone completion
go test ./... -v -cover
go test -bench=. ./...
```

## Performance Requirements

### Response Time Targets
- Local API requests: < 100ms (95th percentile)
- Discovery time: < 5s for network scan

### Resource Constraints
- Memory usage: < 64MB for small deployments
- CPU usage: < 5% on dual-core ARM systems (idle)
- Storage: < 100MB for interaction logs (rotatable)

### Security Considerations

#### Simple Security Model
- Reuse existing authentication mechanisms
- Basic input validation with standard Go validation
- Simple file permissions (0755 for directories, 0644 for files)
- No complex authorization initially - build incrementally

#### Input Validation
```go
// Simple validation approach
func ValidateAccount(account *Account) error {
    if account.Name == "" {
        return errors.New("account name cannot be empty")
    }
    if len(account.Name) > 100 {
        return errors.New("account name too long")
    }
    if account.Email != "" && !isValidEmail(account.Email) {
        return errors.New("invalid email format")
    }
    return nil
}
```

## Error Handling

### Error Categories

```go
type ErrorCategory string
const (
    ErrorCategoryValidation ErrorCategory = "validation"
    ErrorCategorySystem     ErrorCategory = "system"
    ErrorCategoryNetwork    ErrorCategory = "network"
    ErrorCategoryStorage    ErrorCategory = "storage"
    ErrorCategoryTimeout    ErrorCategory = "timeout"
    ErrorCategoryAuth       ErrorCategory = "authentication"
)

type ServiceError struct {
    Code        string                 `json:"code"`
    Message     string                 `json:"message"`
    Category    ErrorCategory          `json:"category"`
    Timestamp   time.Time              `json:"timestamp"`
    Context     map[string]interface{} `json:"context"`
    Retryable   bool                   `json:"retryable"`
    Severity    string                 `json:"severity"`
}
```

### Error Recovery Strategies

1. **Transient Errors**: Exponential backoff retry (3 attempts)
2. **Storage Errors**: Graceful degradation with in-memory fallback
3. **Network Errors**: Circuit breaker pattern with fallback data
4. **Validation Errors**: Immediate response with detailed feedback
5. **System Errors**: Alerting and automatic recovery attempts

### Error Response Format

```json
{
  "error": {
    "code": "DEVICE_NOT_FOUND",
    "message": "Device with ID 'AABBCCDDEEFF' not found in account 'acc_12345'",
    "category": "validation",
    "timestamp": "2024-01-20T15:30:00Z",
    "context": {
      "account_id": "acc_12345",
      "device_id": "AABBCCDDEEFF",
      "request_id": "req_67890"
    },
    "retryable": false,
    "severity": "error"
  },
  "request_id": "req_67890"
}
```

## Monitoring and Observability

### Simple Monitoring Approach

#### Basic Health Check
```go
// Simple health check implementation
type HealthStatus struct {
    Status    string    `json:"status"`    // "healthy", "warning", "error"
    Timestamp time.Time `json:"timestamp"`
    Version   string    `json:"version"`
    Uptime    string    `json:"uptime"`
}

func (s *Server) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
    health := HealthStatus{
        Status:    "healthy",
        Timestamp: time.Now(),
        Version:   version,
        Uptime:    time.Since(startTime).String(),
    }

    // Simple checks
    if !s.canWriteToDataDir() {
        health.Status = "error"
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}
```

#### Leverage Existing Systems
- Extend existing parity mismatch logging for disparity detection
- Reuse existing interaction recording for request/response tracking
- Build upon current discovery and migration event logging
- Use existing WebSocket event system for device state changes

#### Simple Metrics
```go
// Basic counters - no complex metrics initially
type SimpleMetrics struct {
    AccountsCreated   int `json:"accounts_created"`
    DevicesActive     int `json:"devices_active"`
    EventsProcessed   int `json:"events_processed_today"`
    LastUpdate        time.Time `json:"last_update"`
}

// Update metrics in simple text file
func (m *SimpleMetrics) Save(dataDir string) error {
    data, err := json.MarshalIndent(m, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(filepath.Join(dataDir, "metrics.json"), data, 0644)
}
```

This technical specification provides comprehensive details for implementing the enhanced state management system while maintaining compatibility with existing SoundTouch service functionality and meeting the performance requirements for small hardware deployments.
