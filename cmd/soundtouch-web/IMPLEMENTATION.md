# SoundTouch Web Implementation

## Overview

The `soundtouch-web` tool provides a modern single-page application (SPA) for controlling Bose SoundTouch devices. Built with a JSON API backend and client-side JavaScript rendering, it offers superior performance and eliminates template rendering issues.

## Architecture

### Single-Page Application Design

The architecture eliminates Go template dependencies and provides:
- **JSON API Backend**: Pure Go server returning only JSON responses
- **Client-Side Rendering**: JavaScript handles all HTML generation
- **WebSocket Real-time**: Bi-directional communication for live updates
- **Better Performance**: No server-side template processing
- **Easier Development**: Clear separation of frontend/backend concerns

### Core Components

#### 1. Main Application (`main.go`)
- **Entry Point**: Handles command-line arguments and application initialization
- **SPA Routing**: Serves static HTML file for all non-API routes
- **Device Discovery**: Automatic discovery of SoundTouch devices using unified discovery service
- **JSON API Server**: Configures API routes and serves the SPA
- **Context Management**: Proper context handling for timeouts and cancellation

#### 2. HTTP Handlers (`handlers/handlers.go`)
- **WebApp Structure**: Central application state management
- **JSON API Endpoints**: RESTful API returning only JSON responses
- **Device Control**: Device control with proper validation and error handling
- **Modular Design**: Separated control actions into focused functions

#### 3. WebSocket Support (`handlers/websocket.go`)
- **Real-time Updates**: Live device status streaming to web clients
- **Device WebSocket Connections**: Maintains persistent connections to SoundTouch devices
- **Event Handling**: Processes nowPlaying, volume, and connection state updates
- **Status Synchronization**: Keeps device status current across all connected clients

#### 4. Type Definitions (`webtypes/types.go`)
- **Device Management**: Structures for device connections and status
- **API Responses**: Standardized JSON response format
- **WebSocket Messages**: Real-time message types
- **Template Data**: HTML template data structures

### Key Features Implemented

#### Device Discovery & Management
- **Auto-discovery**: Finds SoundTouch devices on local network using mDNS/UPnP
- **Multi-device Support**: Manages multiple devices simultaneously
- **Connection Tracking**: Monitors device availability and connection status
- **Device Information**: Displays device details (name, type, IP address)

#### Real-time Control Interface
- **Now Playing**: Live track information with artwork display
- **Playback Controls**: Play/pause/stop/next/previous with visual feedback
- **Volume Control**: Real-time volume slider with mute functionality
- **Bass Adjustment**: Bass level control for supported devices
- **Preset Management**: Quick access to saved presets (1-6)
- **Source Selection**: Input switching (Spotify, TuneIn, Bluetooth, AUX, etc.)

#### Web Interface
- **Single-Page Application**: Self-contained HTML file with embedded CSS and JavaScript
- **Responsive Design**: Bootstrap 5-based UI optimized for desktop and mobile
- **Client-Side Routing**: JavaScript handles page navigation without page reloads
- **Dynamic Rendering**: All HTML generated client-side from JSON data
- **Real-time Updates**: WebSocket-powered live status updates
- **Performance Optimized**: Fast loading and no template rendering delays

#### API Endpoints
```
GET  /                          # SPA - serves static/index.html
GET  /api/devices              # List all devices (JSON)
GET  /api/device/{id}          # Get device info (JSON)
POST /api/discover             # Trigger device discovery
GET  /api/control/{id}/play    # Playback control
GET  /api/control/{id}/pause   # Pause playback
GET  /api/control/{id}/stop    # Stop playback
GET  /api/control/{id}/next    # Next track
GET  /api/control/{id}/previous # Previous track
POST /api/control/{id}/volume  # Set volume (JSON body)
GET  /api/control/{id}/mute    # Toggle mute
POST /api/control/{id}/bass    # Set bass level (JSON body)
GET  /api/control/{id}/preset?id=N # Select preset
GET  /api/control/{id}/source?name=X # Select source
```

#### WebSocket Events
- **Connection**: `ws://localhost:8080/ws`
- **Device Updates**: Real-time device list changes
- **Status Updates**: Live playback and volume changes
- **Connection Monitoring**: Device availability status

## Technical Implementation

### Frontend Architecture
- **Single HTML File**: Complete application in `static/index.html`
- **Embedded CSS**: Bootstrap 5 with custom Bose-inspired styling
- **Vanilla JavaScript**: No framework dependencies, fast performance
- **Client-Side Routing**: JavaScript manages page state without reloads
- **Dynamic Components**: HTML elements generated from JSON API responses

### Error Handling & Validation
- **Input Validation**: Proper bounds checking for volume (0-100) and bass (-9 to 9)
- **HTTP Status Codes**: Appropriate response codes for different error conditions
- **JSON Error Responses**: Structured error messages for API consumers
- **Client-Side Error Display**: JavaScript toast notifications for user feedback

### Code Quality
- **golangci-lint Compliance**: Passes all configured lint checks
- **Context Handling**: Proper context propagation and timeout management
- **Error Checking**: All JSON encoding/decoding operations checked
- **Type Safety**: Strong typing with dedicated type package
- **Test Coverage**: Comprehensive unit tests for handlers and types

### WebSocket Integration
- **Gabbo Protocol**: Native SoundTouch WebSocket protocol implementation
- **Event Processing**: Handles all documented SoundTouch WebSocket events
- **Connection Management**: Automatic reconnection and health monitoring
- **Bi-directional Communication**: Both status monitoring and device control

## Dependencies

### Core Libraries
- **chi v5**: HTTP router (inherited from existing codebase)
- **gorilla/websocket**: WebSocket implementation
- **Go standard library**: html/template, net/http, encoding/json

### Project Dependencies
- **pkg/client**: SoundTouch HTTP and WebSocket client library
- **pkg/discovery**: Device discovery service (mDNS/UPnP)
- **pkg/models**: XML/JSON data structures for SoundTouch API
- **pkg/config**: Configuration management

### Frontend Dependencies
- **Bootstrap 5**: CSS framework for responsive design
- **Bootstrap Icons**: Icon library for UI elements
- **Vanilla JavaScript**: No external JS frameworks, pure WebSocket implementation

## Build & Testing

### Build Commands
```bash
# Build the web application
cd cmd/soundtouch-web
go build -o soundtouch-web

# Build all project components (includes soundtouch-web)
make build

# Cross-platform builds
make build-all
```

### Testing
```bash
# Run unit tests
go test ./cmd/soundtouch-web/...

# Run with coverage
go test -cover ./cmd/soundtouch-web/...

# Lint checking
golangci-lint run cmd/soundtouch-web/...
```

### Development Server
```bash
# Run development server
cd cmd/soundtouch-web
go run main.go -port 8080

# Access the web interface
open http://localhost:8080
```

## Configuration

### Command Line Options
```bash
soundtouch-web [options]

Options:
  -port string    Web server port (default "8080")
  -host string    Specific device host for single-device mode (optional)
```

### File Structure
```
cmd/soundtouch-web/
├── main.go                    # Application entry point
├── soundtouch-web            # Built binary
├── handlers/
│   ├── handlers.go           # HTTP request handlers
│   ├── handlers_test.go      # Handler tests
│   └── websocket.go          # WebSocket functionality
├── webtypes/
│   ├── types.go              # Type definitions
│   └── types_test.go         # Type tests
├── templates/
│   ├── layout.html           # Base HTML layout
│   ├── index.html            # Device list page
│   └── device.html           # Device control page
├── static/
│   └── style.css             # Additional CSS styles
└── README.md                 # User documentation
```

## Browser Compatibility

### Supported Browsers
- **Chrome 80+** (recommended)
- **Firefox 75+**
- **Safari 13+**
- **Edge 80+**

### Required Features
- WebSocket support
- CSS Grid and Flexbox
- ES6 JavaScript features
- JSON API support

## Security Considerations

### Design Principles
- **Local Network Only**: Designed for trusted local network environments
- **No Authentication**: Assumes local network security
- **CORS Policy**: Restricted to same-origin requests
- **Input Validation**: All user inputs validated on server side

### Network Security
- **Port Usage**: Uses standard HTTP port (configurable)
- **WebSocket Security**: Same-origin WebSocket connections only
- **No External Dependencies**: All resources served locally

## Performance Characteristics

### Resource Usage
- **Memory**: Minimal footprint, scales with number of discovered devices
- **CPU**: Low usage, event-driven architecture
- **Network**: Efficient WebSocket connections, HTTP REST for control

### Scalability
- **Device Limits**: Designed for typical home networks (5-20 devices)
- **Concurrent Users**: Multiple browser sessions supported
- **Update Frequency**: Real-time updates without polling

## Future Enhancements

### Potential Features
- **Zone Management**: Multi-room audio control
- **Preset Programming**: Advanced preset configuration
- **Mobile PWA**: Progressive Web App for mobile installation
- **Theme Support**: Additional UI themes
- **Device Grouping**: Logical device organization

### Technical Improvements
- **Caching**: Enhanced device status caching
- **Compression**: WebSocket message compression
- **Persistence**: Device settings persistence
- **Metrics**: Usage analytics and performance monitoring

## Integration with Main Project

### Project Alignment
- **Consistent Architecture**: Follows established project patterns
- **Shared Libraries**: Leverages existing pkg/ modules
- **Build Integration**: Included in main Makefile targets
- **Documentation**: Consistent with project documentation standards

### Migration Path
- **Cloud Replacement**: Serves as local alternative to Bose cloud services
- **API Compatibility**: Maintains compatibility with existing SoundTouch APIs
- **User Experience**: Familiar interface for existing SoundTouch app users
- **Long-term Support**: Designed for continued operation post-2026

This implementation provides a robust, feature-complete web interface for SoundTouch device control, ensuring continued functionality beyond the official app's lifecycle while maintaining high code quality and user experience standards.
