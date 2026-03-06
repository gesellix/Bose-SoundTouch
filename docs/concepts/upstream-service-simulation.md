# Upstream Bose Service Simulation - State Management Concept

## Overview

This document outlines the concept for simulating and replacing upstream Bose services with enhanced state management capabilities. The goal is to create a comprehensive local replacement that can handle device lifecycles, account management, and state synchronization while maintaining compatibility with existing SoundTouch devices.

## Use Cases

### Case 0: Account Management
- **Explicit Account Creation**: Accounts must be created through deliberate action (web UI, API call)
- **Mirror-Enhanced Creation**: Account creation can be enriched using mirrored data from upstream Bose endpoints when devices make requests
- **Data Recording**: Passively record account information during normal device operations for future use

### Case 1a: Fresh Device Registration
- Initial setup/registration of a factory-reset or new device
- Device has no prior Bose account association
- Full local initialization with default configurations

### Case 1b: Device Migration from Bose Account
- Migrate existing registered device from Bose services to local management
- Preserve existing device data (presets, recents, sources)
- Support gradual migration while maintaining Bose compatibility
- Mirror Bose account data for seamless transition

### Case 2: Device Lifecycle and State Management
- Track and manage device lifecycle states and activities
- Maintain internal state based on incoming events from devices
- Detect disparities between local and upstream behavior
- Provide visibility into state changes and system health

## Architecture Principles

### 1. **Text-Based Storage for Debugging**
- Maintain all state in human-readable text formats (XML, JSON, plain text)
- Use small, focused files for each data aspect
- Enable easy debugging and manual inspection
- Optimize for small hardware deployments (Raspberry Pi Zero 2W)

### 2. **Mirror-First Strategy**
- Keep mirror functionality active as long as possible
- Primary source switches from upstream to local only during:
  - Explicit migration
  - Sufficient local data accumulation
  - Upstream service unavailability
- Record and mirror as much data as possible, even if not immediately used

### 3. **Disparity Detection**
- Track differences between local and upstream responses
- Log discrepancies for analysis and improvement
- Provide visibility into implementation gaps
- Support parity testing and validation

### 4. **Event-Driven State Management**
- Process device events asynchronously
- Track comprehensive event history in text files
- Support event replay and analysis
- Minimize noise while capturing important state changes

## Enhanced Data Structure

### Account Management

```
data/
├── accounts/
│   ├── {account-id}/
│   │   ├── account.json           # Account metadata
│   ├── account-events.log     # High-level account behavior tracking
│   │   ├── devices/
│   │   │   └── {device-id}/
│   │   │       ├── lifecycle.json    # Device state and history
│   │   │       ├── info.xml         # Device information
│   │   │       ├── presets.xml      # Device presets
│   │   │       ├── recents.xml      # Recent plays
│   │   │       ├── sources.xml      # Configured sources
│   │   │       └── events.log       # Device event history
│   │   └── sessions/
│   │       └── {session-id}/        # Recorded interaction sessions
└── system/
    ├── discovery.log                # Device discovery events
    └── migration.log               # Migration activities
```

### Account Metadata Format

```json
{
  "id": "account-12345",
  "name": "User Account",
  "email": "user@example.com",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-20T15:45:00Z",
  "status": "active",
  "device_count": 3,
  "migration_status": {
    "started_at": "2024-01-18T09:00:00Z",
    "devices_migrated": 1,
    "devices_pending": 2,
    "mirror_active": true
  },
  "bose_account_id": "bose-original-id",
  "data_sources": {
    "local": true,
    "bose_mirror": true,
    "primary": "bose"
  }
}
```

### Device Lifecycle Format

```json
{
  "device_id": "A81B6A536A98",
  "account_id": "account-12345",
  "state": "active",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-20T16:22:00Z",
  "state_history": [
    {
      "from": "unregistered",
      "to": "registering",
      "timestamp": "2024-01-15T10:30:00Z",
      "reason": "fresh_device_setup",
      "source": "discovery"
    },
    {
      "from": "registering",
      "to": "active",
      "timestamp": "2024-01-15T10:35:00Z",
      "reason": "registration_complete",
      "source": "system"
    }
  ],
  "metadata": {
    "name": "Living Room Speaker",
    "type": "SoundTouch 30",
    "serial_number": "I6332527703739342000020",
    "firmware_version": "4.8.1.25341.2677643.1597353330",
    "mac_address": "A8:1B:6A:53:6A:98",
    "ip_address": "192.168.1.100",
    "last_seen": "2024-01-20T16:20:00Z",
    "is_legacy_id": false
  },
  "data_sources": {
    "presets": "local",
    "recents": "mirror_primary", 
    "sources": "local"
  },
  "migration": {
    "from_bose_account": "bose-account-xyz",
    "migrated_at": "2024-01-18T14:30:00Z",
    "method": "gradual",
    "rollback_available": true
  }
}
```

### Event Log Format

```
# Device Events Log - A81B6A536A98
# Format: TIMESTAMP|EVENT_TYPE|SOURCE|DATA

2024-01-20T16:15:00Z|now_playing|websocket|{"source":"SPOTIFY","track":"Song Name","artist":"Artist Name"}
2024-01-20T16:15:30Z|volume_changed|websocket|{"volume":45,"muted":false}
2024-01-20T16:16:00Z|preset_selected|websocket|{"preset":1,"source":"SPOTIFY","location":"spotify:track:123"}
2024-01-20T16:18:00Z|disparity_detected|mirror|{"endpoint":"/v1/account/full","local_hash":"abc123","upstream_hash":"def456"}
2024-01-20T16:20:00Z|device_online|discovery|{"ip":"192.168.1.100","method":"mdns"}
```

### Disparity Log Format

```
# Parity Analysis Log
# Format: TIMESTAMP|ENDPOINT|DEVICE|ACCOUNT|DISPARITY_TYPE|DETAILS

2024-01-20T16:18:00Z|/v1/account/full|A81B6A536A98|account-12345|content_mismatch|preset_count:local=5,upstream=4
2024-01-20T16:19:15Z|/v1/presets|A81B6A536A98|account-12345|xml_structure|missing_container_art_in_local
2024-01-20T16:20:30Z|/v1/recents|A81B6A536A98|account-12345|timestamp_format|local=RFC3339,upstream=custom
```

## Implementation Strategy

### Phase 1: Enhanced State Tracking

1. **Account Management Service**
   - Explicit account creation API
   - Mirror-enhanced account initialization
   - Account status and migration tracking

2. **Device Lifecycle Manager**
   - Comprehensive state machine for device lifecycle
   - Event-driven state transitions
   - Text-based state persistence

3. **Enhanced Mirror System**
   - Extended mirroring with disparity detection
   - Selective data source switching
   - Parity analysis and logging

### Phase 2: Gradual Migration Support

1. **Migration Controller**
   - Device-by-device migration orchestration
   - Rollback capability with state preservation
   - Migration progress tracking

2. **Dual-Source Data Management**
   - Smart routing between local and upstream data
   - Graceful fallback mechanisms
   - Data source preference management

3. **State Synchronization**
   - Bidirectional sync capabilities
   - Conflict resolution strategies
   - Sync status monitoring

### Phase 3: Advanced Analytics

1. **Disparity Analysis Engine**
   - Automated disparity detection and classification
   - Trend analysis and reporting
   - Implementation gap identification

2. **System Health Monitoring**
   - Device connectivity monitoring
   - Service availability tracking
   - Performance metrics collection

3. **Data Export and Backup**
   - Account data export for migration
   - Incremental backup strategies
   - Data integrity verification

## API Enhancements

### Account Management APIs

```http
# Create account explicitly
POST /api/v1/accounts
Content-Type: application/json

{
  "name": "User Account",
  "email": "user@example.com"
}

# Get account with migration status
GET /api/v1/accounts/{account-id}

# Initiate account migration from Bose
POST /api/v1/accounts/{account-id}/migrate
Content-Type: application/json

{
  "bose_account_id": "bose-original-id",
  "strategy": "gradual"
}
```

### Device Lifecycle APIs

```http
# Register fresh device
POST /api/v1/accounts/{account-id}/devices
Content-Type: application/json

{
  "device_id": "A81B6A536A98",
  "name": "Living Room Speaker",
  "registration_type": "fresh"
}

# Get device state and lifecycle
GET /api/v1/accounts/{account-id}/devices/{device-id}/state

# Migrate device from Bose account
POST /api/v1/accounts/{account-id}/devices/{device-id}/migrate
Content-Type: application/json

{
  "from_bose_account": "bose-account-xyz",
  "preserve_data": true
}
```

### Monitoring and Analysis APIs

```http
# Get disparity analysis
GET /api/v1/system/disparities?since=2024-01-20T00:00:00Z

# Get migration status
GET /api/v1/system/migration/status

# Export account data
GET /api/v1/accounts/{account-id}/export
```

## Integration with Existing Services

### Enhanced Marge Service

- Integrate lifecycle information into account responses
- Add migration status to device listings
- Support dual-source data routing
- Include disparity metadata in responses

### Enhanced BMX Service

- Track content source preferences by account
- Mirror and compare content recommendations
- Log streaming behavior for analysis
- Support gradual source migration

### Discovery Service Integration

- Link discovered devices to lifecycle manager
- Trigger lifecycle state transitions on discovery events
- Support both fresh registration and migration flows
- Handle legacy device ID migration automatically

## Performance Considerations

### Simplicity First (KISS Principle)

- Favor simple, readable code over premature optimization
- Use straightforward algorithms and data structures
- Minimize complexity in favor of maintainability
- Build incrementally with small, testable changes

### Quality Assurance

- Complete test coverage for all new functionality
- Comprehensive linting with `golangci-lint run --fix` 
- Full test suite execution `go test ./...` for each milestone
- Integration tests with existing functionality

### File Management

- Simple line-based append operations for logs
- Basic log rotation when needed
- Direct file operations without complex caching
- Straightforward data persistence

## Development Principles

### KISS (Keep It Simple, Stupid)
- Prioritize simplicity and readability over performance optimization
- Use standard Go idioms and patterns
- Avoid premature abstraction and optimization
- Build the simplest thing that works first

### Quality First
- Every milestone must pass `golangci-lint run --fix` without issues
- Complete test suite must pass `go test ./...` before proceeding
- Integration tests ensure existing functionality remains intact
- Code coverage should be maintained or improved

### Incremental Development
- Make small, focused changes that can be easily reviewed
- Each step should be independently testable and valuable
- Maintain backward compatibility throughout development
- Enable rollback at any point in the process

### Leverage Existing Systems
- Reuse existing interaction recording for request/response tracking
- Build upon current parity mismatch detection system
- Extend existing datastore and handler patterns
- Integrate with established discovery and migration workflows

## Future Enhancements

Future improvements should maintain the simplicity-first approach:

1. **Enhanced Web Interface**
   - Simple dashboard for account and device management
   - Basic migration progress tracking
   - Straightforward device health monitoring

2. **Extended Logging**
   - Additional high-level behavior tracking
   - Simple analytics based on existing parity data
   - Enhanced debugging information

3. **Community Integration**
   - Standardized data export formats
   - Simple reporting mechanisms
   - Clear documentation for community contributions

This concept provides a solid, maintainable foundation for replacing Bose's upstream services. The emphasis on simplicity, existing system reuse, and comprehensive testing ensures reliable functionality while maintaining the debugging capabilities needed for small hardware deployments.