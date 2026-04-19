# SoundTouch Web UI

A modern single-page web application (SPA) for controlling Bose SoundTouch devices. Built with a JSON API backend and client-side JavaScript rendering for superior performance and maintainability.

## Architecture

```
Browser → Static HTML → JavaScript → JSON API → Go Server
                     ↓
               Client-Side Rendering
```

### Key Benefits
- **Better Performance**: No server-side template processing overhead
- **Improved Maintainability**: Clear separation between frontend (JavaScript) and backend (Go)
- **Real-time Experience**: Smooth client-side updates without page reloads
- **Mobile Ready**: The JSON API can power both this web interface and mobile applications

## Features

Based on captured WebSocket interactions and device API capabilities, this web UI provides:

### Device Management
- **Auto-discovery** of SoundTouch devices on the network
- **Real-time status monitoring** via WebSocket connections
- **Multi-device support** with centralized control
- **Connection status** indicators and health monitoring

### Playback Control
- **Play/Pause/Stop/Next/Previous** controls
- **Now playing information** with artwork, track details, and progress
- **Real-time updates** of playback state changes
- **Source selection** from available inputs (Spotify, TuneIn, Bluetooth, AUX, etc.)

### Audio Controls
- **Volume control** with real-time slider updates
- **Mute/Unmute** functionality
- **Bass adjustment** (on supported models)
- **Audio level monitoring** and statistics

### Preset Management
- **6 preset buttons** with visual feedback
- **Preset content display** showing station/playlist names
- **One-click preset selection**

### Advanced Features
- **WebSocket real-time updates** for instant state synchronization
- **Responsive design** optimized for desktop and mobile
- **Dark mode support** (auto-detects system preference)
- **Accessibility features** (keyboard navigation, screen reader support)
- **Network statistics** and device health monitoring

## Screenshots

### Main Device Overview
The main page shows all discovered devices with their current status, now-playing information, and quick controls.

### Detailed Device Control
Individual device pages provide full control over:
- Detailed now-playing information with artwork
- Comprehensive audio controls (volume, bass)
- Full preset and source selection
- Real-time status updates

## Installation

### Prerequisites
- Go 1.21 or later
- Access to SoundTouch devices on the same network
- Modern web browser with WebSocket support

### Building
```bash
# From project root
make build

# Or manually
cd cmd/soundtouch-web
go build -o soundtouch-web
```

### Running
```bash
# Run with default settings (port 8080)
./soundtouch-web

# Specify custom port
./soundtouch-web -port 8888

# Connect to specific device
./soundtouch-web -host 192.168.1.100
```

### Command Line Options
```
-port string    Web server port (default "8080")
-host string    Specific SoundTouch device host (optional, enables single-device mode)
-help          Show help information
```

## Usage

### Accessing the Interface
1. Start the application
2. Open your web browser and navigate to `http://localhost:8080`
3. Click "Discover Devices" to find SoundTouch devices on your network
4. Click on any device for detailed control, or use quick controls from the main page

### Device Discovery
The application automatically discovers SoundTouch devices using:
- **mDNS discovery** for local network devices
- **UPnP/SSDP discovery** as fallback
- **Manual device addition** via IP address

### Real-time Updates
The interface maintains WebSocket connections to each device for instant updates of:
- Now playing information and artwork
- Volume and audio settings changes
- Playback status (play/pause/stop)
- Connection status and device health

### Responsive Design
- **Desktop**: Full-featured interface with side-by-side panels
- **Tablet**: Optimized layout with touch-friendly controls
- **Mobile**: Stacked interface with gesture support

## API Endpoints

The web UI exposes a REST API for programmatic control:

### Device Management
```
GET  /api/devices           # List all discovered devices
GET  /api/device/{id}       # Get specific device info
POST /api/discover          # Trigger device discovery
```

### Device Control
```
GET  /api/control/{id}/play        # Start playback
GET  /api/control/{id}/pause       # Pause playback
GET  /api/control/{id}/stop        # Stop playback
GET  /api/control/{id}/next        # Next track
GET  /api/control/{id}/previous    # Previous track
POST /api/control/{id}/volume      # Set volume (body: {"level": 50})
GET  /api/control/{id}/mute        # Mute audio
GET  /api/control/{id}/unmute      # Unmute audio
POST /api/control/{id}/bass        # Set bass (body: {"level": 0})
GET  /api/control/{id}/preset?id=1 # Select preset
GET  /api/control/{id}/source?name=SPOTIFY # Select source
```

### WebSocket Events
Connect to `/ws` for real-time updates:
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');
ws.onmessage = function(event) {
    const data = JSON.parse(event.data);
    // Handle device updates, status changes, etc.
};
```

## Architecture

### Single-Page Application Architecture
- **JSON API Backend**: Go server providing RESTful endpoints
- **Client-Side Rendering**: JavaScript handles all UI rendering
- **WebSocket Real-time**: Bi-directional real-time communication
- **No Template Dependencies**: Eliminates server-side template issues

### Backend Components
- **Discovery Service**: Finds and manages SoundTouch devices
- **WebSocket Manager**: Maintains real-time connections to devices
- **JSON API Server**: RESTful interface returning only JSON
- **Device Manager**: Tracks device state and health

### Frontend Components
- **Bootstrap 5**: Modern responsive UI framework
- **Vanilla JavaScript**: No framework dependencies, fast loading
- **WebSocket Client**: Real-time bidirectional communication
- **Dynamic Rendering**: Client-side HTML generation from JSON

### Communication Flow
1. **SPA Loading**: Single HTML file with embedded CSS and JavaScript
2. **JSON API**: Device discovery and control via REST endpoints
3. **WebSocket (Device)**: Real-time status updates from SoundTouch devices
4. **WebSocket (Browser)**: Real-time UI updates to web clients
5. **Client Rendering**: JavaScript dynamically creates all UI elements

## Development

### Project Structure
```
cmd/soundtouch-web/
├── main.go              # Application entry point and SPA routing
├── handlers/            # HTTP and WebSocket handlers
│   ├── handlers.go     # JSON API endpoints
│   └── websocket.go    # WebSocket management
├── webtypes/           # Type definitions
│   └── types.go        # Request/response types
├── static/             # Static assets
│   ├── index.html      # Single-page application
│   └── js/             # Legacy JS files (reference)
├── templates/          # Legacy templates (unused in SPA)
└── README.md          # This file
```

### Adding New Features
1. **API Endpoints**: Add new JSON routes in `setupRoutes()` and `handlers.go`
2. **WebSocket Events**: Extend event handlers in WebSocket client
3. **UI Components**: Add JavaScript rendering functions in `static/index.html`
4. **Device Controls**: Implement new control commands and update client-side handlers

### Testing
```bash
# Unit tests
go test ./...

# Manual testing with multiple devices
./soundtouch-web -port 8080

# API testing
curl http://localhost:8080/api/devices
```

## WebSocket Protocol Analysis

This UI is based on extensive analysis of captured SoundTouch WebSocket interactions, including:

### Message Types Implemented
- **SoundTouchSdkInfo**: Initial handshake and version info
- **nowPlayingUpdated**: Real-time track information
- **volumeUpdated**: Audio level changes
- **recentsUpdated**: Recently played items
- **userActivityUpdate**: User interaction notifications

### Request/Response Patterns
- **Device Information**: System details and capabilities
- **Audio Controls**: Volume, bass, mute controls
- **Playback Control**: Play/pause/stop/skip commands
- **Source Selection**: Input switching (Spotify, TuneIn, etc.)
- **Preset Management**: Saved station/playlist access

### Gabbo Protocol Features
- **Persistent Connections**: Maintains long-lived WebSocket connections
- **Request Correlation**: Uses request IDs for response matching
- **Real-time Events**: Instant updates for all device state changes
- **Bi-directional Control**: Both status monitoring and device control

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
- Responsive CSS media queries

## Security Considerations

- **Local Network Only**: Designed for local network device control
- **No Authentication**: Assumes trusted local network environment
- **CORS Policy**: Restricted to same-origin requests
- **WebSocket Security**: Uses same-origin WebSocket connections

## Troubleshooting

### Common Issues

**Devices Not Found**
- Ensure devices are on the same network
- Check firewall settings (ports 8090, 8080)
- Click "Discover Devices" button to trigger discovery

**WebSocket Connection Failed**
- Verify device supports WebSocket connections
- Check browser console for connection errors
- Refresh the page to reconnect WebSocket

**Control Commands Not Working**
- Check device is powered on and connected
- Verify device is not in exclusive mode (e.g., Spotify Connect active)
- Look for error notifications in the UI

**Page Shows Template Errors**
- This has been fixed in the SPA implementation
- Ensure you're accessing the correct URL (localhost:8080)
- Clear browser cache if you see old template-based content

### Debug Mode
Add verbose logging by setting environment variable:
```bash
export DEBUG=true
./soundtouch-web
```

## Contributing

This web UI is part of the larger SoundTouch Go library project. See the main project README for contribution guidelines.

### Architecture Benefits
The new SPA approach provides:
- **Better Performance**: No server-side template rendering
- **Easier Development**: Clear separation of frontend/backend
- **Mobile Ready**: Same JSON API can power mobile apps
- **Scalable**: Single-page app architecture

### Feature Requests
Based on WebSocket interaction analysis, potential future features:
- Zone/multi-room management
- Clock display control
- Software update management
- Advanced preset programming
- Progressive Web App (PWA) features

## License

Same as the parent project - see main repository LICENSE file.

## Acknowledgments

- Built on the comprehensive SoundTouch Go library
- UI design inspired by modern audio control interfaces
- WebSocket protocol reverse-engineered from captured device interactions
- Bootstrap and Bootstrap Icons for responsive design components
