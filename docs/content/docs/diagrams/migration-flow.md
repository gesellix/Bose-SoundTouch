---
title: "Migration Flow Diagrams"
---
This document specifies the diagrams needed for the migration guide, with descriptions that can be used to create actual visual diagrams.

## 1. Overall Migration Process Flow

### Description
A flowchart showing the complete migration journey from start to finish.

### Elements
```
[Start] → [Install SoundTouch Service] → [Create Account] → [Prepare Devices]
    ↓
[Enable Remote Services] → [Discover Devices] → [Register Devices]
    ↓
[Start Migration] → [Data Collection Phase] → [Testing Phase] → [Full Local Phase]
    ↓
[Verify Migration] → [Complete] → [Post-Migration Setup]
```

### Decision Points
- Multiple devices? → Repeat device steps
- Migration issues? → Rollback option
- All devices complete? → Account fully migrated

### Color Coding
- **Blue**: Service setup steps
- **Green**: Successful completion states
- **Orange**: In-progress/testing states
- **Red**: Error handling/rollback paths
- **Gray**: Optional steps

## 2. Network Topology Diagram

### Description
Shows the network layout with Raspberry Pi, router, and SoundTouch devices.

### Components
```
Internet Cloud
    ↑↓ (Optional - during migration)
Home Router (192.0.2.1)
    ├── Raspberry Pi (192.0.2.10) [SoundTouch Service]
    ├── Living Room Speaker (192.0.2.100)
    ├── Kitchen Speaker (192.0.2.101)
    ├── Bedroom Speaker (192.0.2.102)
    └── Office Speaker (192.0.2.103)
```

### Connections
- **Solid lines**: Active connections
- **Dashed lines**: Migration-phase connections to Bose cloud
- **Thick lines**: Primary data flow to local service

## 3. Device State Lifecycle

### Description
State machine showing device progression through migration phases.

### States and Transitions
```
[Unregistered] → [Discovered] → [Registered] → [Migrating]
                                                    ↓
[Active - Local Only] ← [Active - Testing] ← [Active - Data Collection]
    ↑                                              ↓
[Error/Rollback] ← ← ← ← ← ← ← ← ← ← ← ← ← ← ← ← ← [Migration Failed]
```

### State Descriptions
- **Unregistered**: Device not known to service
- **Discovered**: Found on network, remote services enabled
- **Registered**: Added to account, ready for migration
- **Migrating - Data Collection**: Building local database
- **Migrating - Testing**: Using local service with fallback
- **Active - Local Only**: Full independence achieved
- **Error/Rollback**: Issues detected, can revert to Bose

## 4. Data Flow During Migration

### Description
Shows how data flows between components during different migration phases.

### Phase 1 - Data Collection
```
SoundTouch Device → Bose Cloud Services
                ↓ (mirror)
            Local Service (collecting data)
```

### Phase 2 - Testing
```
SoundTouch Device ↔ Local Service (primary)
                ↕ (fallback when needed)
            Bose Cloud Services
```

### Phase 3 - Full Local
```
SoundTouch Device ↔ Local Service (only)

Bose Cloud Services (disconnected)
```

### Data Types
- **Presets**: Station favorites and custom sources
- **Recents**: Play history and recently accessed content
- **Sources**: Configured music services (Spotify, etc.)
- **Device Config**: Network settings, capabilities, metadata

## 5. Migration Timeline Visualization

### Description
Gantt-chart style timeline showing typical migration schedule.

### Timeline (7-day example)
```
Day 1-2: Data Collection Phase
         ████████████████████████████████████████
         
Day 3-4: Data Validation
               ████████████████████████████
               
Day 5-6: Testing Phase  
                     ████████████████████████
                     
Day 7+:  Full Local Operation
                           ████████████████████████→
```

### Parallel Activities
- Multiple devices can be in different phases
- Service continues operating throughout
- User can interact normally during process

## 6. Service Architecture Overview

### Description
High-level architecture showing enhanced SoundTouch service components.

### Components
```
Web Dashboard ← → HTTP API ← → REST Endpoints
      ↑               ↑              ↑
      └─── User ──────┼──── Devices ─┘
                      ↓
              Service Core
              ├── Account Manager
              ├── Device Lifecycle
              ├── Event Processor
              ├── Migration Controller
              └── Data Store
                      ↓
              File System Storage
              ├── accounts/
              ├── devices/
              ├── sessions/ (existing)
              └── system/
```

### External Integrations
- **Bose Cloud** (during migration)
- **Music Services** (Spotify, TuneIn, etc.)
- **Discovery Services** (mDNS, UPnP)

## 7. Error Handling and Rollback Flow

### Description
Decision tree for handling migration issues and rollback scenarios.

### Error Detection
```
Migration Issue Detected
    ├── Device Unresponsive → Retry → Success/Rollback
    ├── Data Corruption → Restore from Backup → Continue/Rollback  
    ├── Service Unavailable → Wait/Restart → Continue/Rollback
    └── User Dissatisfaction → Manual Rollback → Restore Bose Config
```

### Rollback Process
```
[Rollback Initiated]
    ↓
[Disable Local Services]
    ↓
[Restore Original Device Config]
    ↓
[Re-enable Bose Services]
    ↓
[Verify Functionality]
    ↓
[Rollback Complete]
```

## Implementation Notes

### For Diagram Creation
1. Use consistent colors as specified in main color scheme
2. Include clear labels for all components
3. Show directional flow with appropriate arrows
4. Use standard flowchart symbols where applicable
5. Ensure text is readable at various sizes

### Tools Recommended
- **Lucidchart**: Professional flowcharts and network diagrams
- **Draw.io**: Free online diagram tool
- **Miro**: Collaborative whiteboarding
- **PlantUML**: Code-based diagram generation

### File Naming Convention
- `migration-flow-overview.svg` - Overall process flow
- `network-topology.svg` - Network layout
- `device-lifecycle.svg` - State machine
- `data-flow-phases.svg` - Data flow during migration
- `migration-timeline.svg` - Timeline visualization
- `service-architecture.svg` - System architecture
- `error-rollback-flow.svg` - Error handling

### Accessibility
- Include alt-text descriptions
- Use patterns/textures in addition to colors
- Ensure sufficient contrast
- Provide text-based versions for screen readers