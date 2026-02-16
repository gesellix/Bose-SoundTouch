# SoundTouch Service

The `soundtouch-service` is a comprehensive local server that emulates Bose's cloud services, enabling offline SoundTouch device operation and advanced debugging capabilities. This service is particularly valuable given Bose's announcement that cloud support will end in May 2026.

## Overview

The service provides:

- **🏠 Local Service Emulation**: Complete BMX (Bose Media eXchange) and Marge service implementation
- **🔧 Device Migration**: Seamlessly migrate devices from Bose cloud to local services via XML config, `/etc/hosts`, or `/etc/resolv.conf`
- **🔍 DNS Discovery & Interception**: Built-in DNS server to discover unknown Bose endpoints and selectively intercept cloud traffic
- **📊 Traffic Proxying**: Inspect and log all device communications for debugging
- **🌐 Web Management UI**: Browser-based interface for device management
- **💾 Persistent Data**: Store device configurations, presets, and usage statistics
- **📝 HTTP Recording**: Persist all interactions as re-playable `.http` files
- **📥 Session Archiving**: Download entire interaction sessions as `.tar.gz` for offline analysis
- **🔍 Auto-Discovery**: Automatically detect and configure SoundTouch devices
- **🔒 Offline Operation**: Continue using full device functionality without internet
- **🔗 Bose Proxy & Soundcork Fallback**: Dynamic proxying with automatic fallback to local [SoundCork](https://github.com/deborahgu/soundcork) emulation if enabled

## Architecture

The service consists of several key components:

### BMX Services (Bose Media eXchange)
- **TuneIn Integration**: Direct playback of radio stations and podcasts
- **Service Registry**: Media service discovery and configuration
- **Playback Control**: Stream URL resolution and audio metadata

### Marge Services (Account & Device Management)  
- **Account Management**: User account simulation and device association
- **Preset Synchronization**: Cross-device preset storage and sync
- **Recent Items**: Playback history tracking and management
- **Configuration Management**: Device settings and preferences

### Discovery & Migration
- **Network Scanning**: UPnP/SSDP and mDNS device discovery
- **Device Analysis**: Configuration assessment and compatibility checking
- **Service Migration**: Automated configuration updates for local service usage
- **Health Monitoring**: Device connectivity and service status tracking

## Installation

### Install from Source
```bash
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-service@latest
```

### Build from Repository
```bash
git clone https://github.com/gesellix/bose-soundtouch.git
cd Bose-SoundTouch
go build -o soundtouch-service ./cmd/soundtouch-service
```

### Docker Support

You can run the SoundTouch service using Docker or Docker Compose. 

> **Note for macOS and Windows users**: The `--net host` option is only supported on Linux. On macOS and Windows, service discovery (mDNS, UPnP) will not work automatically within the container. You will need to manually enter your device's IP address in the management UI, and the service will communicate with it directly.

#### Using Docker

**Linux (with host networking for discovery):**
```bash
docker run -d \
  --name soundtouch-service \
  --network host \
  -v $(pwd)/data:/app/data \
  ghcr.io/gesellix/bose-soundtouch:latest
```

**macOS / Windows (with port mapping):**
```bash
docker run --rm -it \
  -p 8000:8000 -p 8443:8443 \
  -v $(pwd)/data:/app/data \
  --env SERVER_URL=http://soundtouch.local:8000 \
  --env HTTPS_SERVER_URL=https://soundtouch.local:8443 \
  ghcr.io/gesellix/bose-soundtouch:latest
```

> **Note**: The hostnames configured via `SERVER_URL` and `HTTPS_SERVER_URL` are automatically added as Subject Alternative Names (SAN) to the generated TLS certificate, ensuring valid SSL connections.

#### Using Docker Compose

Create a `docker-compose.yml` file:

```yaml
services:
  soundtouch-service:
    image: ghcr.io/gesellix/bose-soundtouch:latest
    container_name: soundtouch-service
    # Linux users: use host networking for device discovery
    # network_mode: host
    # macOS/Windows users: use port mapping (discovery will be manual)
    ports:
      - "8000:8000"
      - "8443:8443"
    environment:
      - PORT=8000
      - SERVER_URL=http://soundtouch.local:8000
      - HTTPS_SERVER_URL=https://soundtouch.local:8443
      - DATA_DIR=/app/data
    volumes:
      - soundtouch-data:/app/data
    restart: unless-stopped

volumes:
  soundtouch-data:
```

And run:

```bash
docker-compose up -d
```

## Quick Start

### 1. Start the Service

```bash
# Start with default settings (port 8000)
soundtouch-service
```

### 2. Access the Web Interface

Open your browser to `http://localhost:8000` to access the management interface.

### 3. Discover Devices

The service will automatically start discovering SoundTouch devices on your network. You can also trigger manual discovery from the web UI or API.

### 4. Migrate Devices

Use the web interface or API to migrate devices from Bose cloud services to your local instance.

## Configuration

### Configuration Precedence

The service supports multiple ways to configure its behavior. When multiple sources provide the same setting, the following precedence rules apply (highest to lowest):

1.  **`settings.json`**: Settings saved via the Web UI (stored in the data directory) take the highest precedence. This ensures that changes made in the browser persist across service restarts even if environment variables or flags change.
2.  **Environment Variables / CLI Flags**: If a setting is not present in `settings.json`, environment variables and flags are used.
3.  **Default Values**: If no configuration is provided, the service uses its built-in defaults.

> **Tip**: If you find that changes to environment variables are not taking effect, check the **Settings** tab in the Web UI or inspect the `settings.json` file in your data directory, as it might be overriding your manual configuration.

### Configuration Options

| Variable                           | Flag                       | Description                                                                                             | Default                   |
|------------------------------------|----------------------------|---------------------------------------------------------------------------------------------------------|---------------------------|
| `PORT`                             | `--port`, `-p`             | HTTP port to bind the service to                                                                        | `8000`                    |
| `BIND_ADDR`                        | `--bind`                   | Network interface to bind to                                                                            | all (ipv4 and ipv6)       |
| `DATA_DIR`                         | `--data-dir`               | Directory for persistent data                                                                           | `./data`                  |
| `SERVER_URL`                       | `--server-url`, `-s`       | External URL of this service                                                                            | `http://<hostname>:8000`  |
| `HTTPS_PORT`                       | `--https-port`             | HTTPS port to bind the service to                                                                       | `8443`                    |
| `HTTPS_SERVER_URL`                 | `--https-server-url`, `-S` | External HTTPS URL                                                                                      | `https://<hostname>:8443` |
| `PYTHON_BACKEND_URL`, `TARGET_URL` | `--target-url`             | URL for Python-based service components (legacy)                                                        | `http://localhost:8001`   |
| `REDACT_PROXY_LOGS`                | `--redact-logs`            | Redact sensitive data in proxy logs                                                                     | `true`                    |
| `LOG_PROXY_BODY`                   | `--log-bodies`             | Log full request/response bodies                                                                        | `false`                   |
| `RECORD_INTERACTIONS`              | `--record-interactions`    | Record HTTP interactions to disk                                                                        | `true`                    |
| `DISCOVERY_INTERVAL`               | `--discovery-interval`     | Device discovery interval                                                                               | `5m`                      |
| `ENABLE_DNS_DISCOVERY`             | `--dns-discovery`          | Enable DNS discovery server                                                                             | `false`                   |
| `DNS_UPSTREAM`                     | `--dns-upstream`           | Upstream DNS server for non-Bose queries                                                                | `8.8.8.8`                 |
| `DNS_BIND_ADDR`                    | `--dns-bind`               | Bind address for the DNS discovery server (standard port `:53` is required for `resolv.conf` migration) | `:53`                     |
| `DISCOVERY_DISABLED`               |                            | Disable automated device discovery                                                                      | `false`                   |

### Configuration Examples

```bash
# Custom port and data directory
PORT=9000 DATA_DIR=/home/user/soundtouch soundtouch-service

# External server with custom URL
SERVER_URL=https://my-soundtouch.example.com soundtouch-service --port 443

# Development mode with full logging
LOG_PROXY_BODY=true REDACT_PROXY_LOGS=false soundtouch-service
```

## Device Migration

### Understanding Migration

Device migration switches your SoundTouch devices from Bose's cloud services to your local service instance. This process:

1. **Backs up** existing device configuration
2. **Updates** device service URLs to point to your local server
3. **Maintains** all existing presets and settings
4. **Enables** offline operation and advanced debugging

### Migration Methods

#### Web Interface (Recommended)

1. Start the service: `soundtouch-service`
2. Open `http://localhost:8000`
3. Wait for device discovery to complete
4. Click "Migrate" next to each device
5. Monitor migration status in real-time

#### API Migration

```bash
# Get migration summary first
curl http://localhost:8000/setup/migration-summary/192.168.1.100

# Perform migration
curl -X POST http://localhost:8000/setup/migrate/192.168.1.100

# Verify migration status
curl http://localhost:8000/setup/devices
```

#### Advanced Migration Options

```bash
# Migration with proxy fallback for original services
curl -X POST "http://localhost:8000/setup/migrate/192.168.1.100?proxy_url=http://localhost:8000&marge=original&stats=original"

# Migration with custom target URL
curl -X POST "http://localhost:8000/setup/migrate/192.168.1.100?target_url=https://my-server.com:8000"
```

### Post-Migration Verification

After migration, verify the device is working correctly:

```bash
# Check device status
curl http://localhost:8000/setup/devices

# Test preset functionality
curl "http://192.168.1.100:8090/presets"

# Monitor device events (if needed)
curl "http://localhost:8000/events/192.168.1.100"
```

#### ResolvConf Migration (DHCP-Aware DNS Redirection)

The most robust and flexible DNS-based migration method. It utilizes the device's persistent `/mnt/nv/rc.local` script to inject a priority DNS hook into the system's DHCP configuration.

> **Note**: This method requires the DNS Discovery Server to be bound to **port 53** on your local IP and **actually running**. Most devices do not support custom DNS ports in `/etc/resolv.conf`. If you use a custom port for testing, remember to switch back to `:53` and ensure the server has successfully bound to it (check Settings for status) before the actual migration.

**Advantages:**
- **Discovery**: Automatically discover all Bose endpoints queried by the device.
- **Dynamic Interception**: Intercept new or unknown services without further device modifications.
- **Fail-Safe**: Falls back to the standard network DNS (provided by your router) if the Aftertouch service is unavailable.
- **DHCP Compatible**: Preserves your router's assigned search domain and secondary DNS servers.
- **Wildcard Support**: Seamlessly handles `*.bose.com` redirection via your local DNS server.
- **Persistent**: Survives reboots and DHCP renewals.

**How it works:**
1. **Configuration**: A custom file named `/mnt/nv/aftertouch.resolv.conf` is created on the device's persistent partition.
2. **Boot Hook**: On every boot, `/mnt/nv/rc.local` checks if the system's DHCP scripts (`/etc/udhcpc.d/50default` or `/opt/Bose/udhcpc.script`) have been patched.
3. **Surgical Patch**: If not patched, it injects a one-line check into the relevant DHCP scripts.
4. **Resolution**: Whenever the device acquires a DHCP lease, the scripts now read your `aftertouch.resolv.conf` first, placing your DNS server at the top of `/etc/resolv.conf` while keeping all other DHCP-provided settings.

**Setup:**
1. Enable SSH via the `remote_services` USB trick.
2. Create `/mnt/nv/aftertouch.resolv.conf` with your server details:
   ```text
   # Created by Aftertouch/SoundTouch-Service
   # Priority nameserver for Bose service redirection
   nameserver 192.168.1.XXX
   ```
3. Update `/mnt/nv/rc.local` with the idempotent patch:
   ```sh
   #!/bin/sh
   # Aftertouch DNS hook: prioritizes our custom nameserver if it exists
   HOOK_MARKER="/mnt/nv/aftertouch.resolv.conf"
   if [ -f "$HOOK_MARKER" ]; then
       # Patch 50default if it exists
       TARGET_FILE="/etc/udhcpc.d/50default"
       if [ -f "$TARGET_FILE" ] && ! grep -q "$HOOK_MARKER" "$TARGET_FILE"; then
           sed -i '/echo "search \$domain"/a \        [ -f '"$HOOK_MARKER"' ] && cat '"$HOOK_MARKER"' && dns=""' "$TARGET_FILE"
       fi
       # Patch udhcpc.script if it exists (e.g. SoundTouch 10)
       TARGET_SCRIPT="/opt/Bose/udhcpc.script"
       if [ -f "$TARGET_SCRIPT" ] && ! grep -q "$HOOK_MARKER" "$TARGET_SCRIPT"; then
           sed -i '/echo "search \$search_list # \$interface" >> \$RESOLV_CONF/a \                [ -f '"$HOOK_MARKER"' ] && cat '"$HOOK_MARKER"' >> '"\$RESOLV_CONF"' && dns=""' "$TARGET_SCRIPT"
       fi
   fi
   ```
4. Make the script executable: `chmod +x /mnt/nv/rc.local`.
5. Reboot the speaker.

### DNS Discovery Server

The SoundTouch service includes a built-in DNS server specifically designed for Bose devices.

#### How it Works
When enabled, the DNS server:
1. Receives DNS queries from migrated SoundTouch devices.
2. **Intercepts** known Bose domains (e.g., `api.bose.com`, `streaming.bose.com`, `bmx.bose.com`) and resolves them to the AfterTouch service IP.
3. **Logs** all other queries for discovery purposes, allowing you to identify new Bose cloud endpoints.
4. **Forwards** unknown or non-Bose queries to the configured upstream DNS server (default: `8.8.8.8`).

#### Configuration
You can enable and configure the DNS server via the Web UI or environment variables:
- `ENABLE_DNS_DISCOVERY=true`: Turns on the DNS server.
- `DNS_BIND_ADDR=:53`: The port to listen on (requires root privileges for port 53).
- `DNS_UPSTREAM=1.1.1.1`: Your preferred upstream DNS provider. **Note:** Ensure this is not set to the same address as the DNS server itself (loopback or local IP) to avoid forwarding loops. The server includes built-in loop prevention, but misconfiguration will cause forwarding to fail. DNS Discovery cannot be enabled if this setting is empty.

#### Manual Discovery via DNS
Even without migrating a device, you can use the DNS server to discover what a device is querying by manually setting your router's DNS or the device's DNS to point to the AfterTouch service.

## API Reference

### Discovery & Setup

#### `GET /setup/devices`
Lists all discovered SoundTouch devices with their current status.

**Response:**
```json
[
  {
    "device_id": "08DF1F0BA325",
    "name": "Living Room Speaker",
    "ip_address": "192.168.1.100",
    "product_code": "SoundTouch 20",
    "firmware_version": "19.0.5",
    "migrated": true,
    "last_seen": "2024-01-15T10:30:00Z"
  }
]
```

#### `POST /setup/discover`
Triggers immediate network device discovery.

#### `GET /setup/info/{deviceIP}`
Gets detailed device information and configuration.

#### `GET /setup/migration-summary/{deviceIP}`
Analyzes device configuration and provides migration preview.

**Response:**
```json
{
  "device_name": "Living Room Speaker",
  "device_model": "SoundTouch 20",
  "firmware_version": "19.0.5",
  "ssh_success": true,
  "current_config": "<?xml version=\"1.0\"?>...",
  "planned_config": "<?xml version=\"1.0\"?>...",
  "remote_services_enabled": false,
  "migration_required": true
}
```

#### `POST /setup/migrate/{deviceIP}`
Migrates device to use local services.

**Query Parameters:**
- `target_url`: Custom service URL (optional)
- `proxy_url`: Proxy URL for fallback (optional)  
- `marge`: Set to "original" to proxy Marge requests (optional)
- `stats`: Set to "original" to proxy stats requests (optional)
- `sw_update`: Set to "original" to proxy update requests (optional)
- `bmx`: Set to "original" to proxy BMX requests (optional)

### BMX Services (Bose Media eXchange)

#### `GET /bmx/registry/v1/services`
Returns available media services for device registration.

#### `GET /bmx/tunein/v1/playbook/station/{stationID}`
Provides TuneIn station playback information.

#### `GET /bmx/tunein/v1/podcast/{podcastID}`
Returns podcast episode information and playback URLs.

### Marge Services (Account & Device Management)

#### `GET /marge/streaming/sourceproviders`
Lists available music service providers.

#### `GET /marge/accounts/{account}/devices/any/presets`
Returns user presets for synchronization.

#### `GET /marge/accounts/{account}/devices/any/recents`
Returns recent playback items.

#### `PUT /marge/accounts/{account}/devices/{device}/presets/{slot}`
Updates a specific preset slot.

#### `POST /marge/streaming/support/addrecent`
Adds item to recent playback history.

#### `GET /marge/updates/soundtouch`
Returns software update configuration (disabled by default).

### Proxy Services

#### `GET /proxy/{encodedURL}`
Proxies requests to external services with logging.

**Example:**
```bash
# Proxy request to Bose services
curl "http://localhost:8000/proxy/aHR0cHM6Ly9hcGkuc291bmR0b3VjaC5ib3NlLmNvbS8="
```

### Health & Monitoring

#### `GET /health`
Returns service health status.

#### `GET /events/{deviceID}`
WebSocket endpoint for real-time device events.

#### `GET /stats/usage`
Returns usage statistics.

#### `GET /stats/errors`
Returns error statistics.

## Web Interface

### Overview

The web management interface provides a comprehensive dashboard for managing your SoundTouch devices:

**URL:** `http://localhost:8000/`

### Features

#### Device Dashboard
- **Device Discovery**: Real-time view of discovered devices
- **Migration Status**: Visual indicators of migration state
- **Device Health**: Connectivity and service status monitoring
- **Quick Actions**: One-click migration and configuration

#### Device Management
- **Configuration Viewer**: Inspect current and planned device configs
- **Migration Wizard**: Step-by-step device migration process
- **Backup Management**: View and restore configuration backups
- **Service Testing**: Test connectivity to local services

#### Monitoring & Debugging
- **Traffic Logs**: Real-time proxy request/response logging
- **Event Streaming**: Live device event monitoring
- **Statistics Dashboard**: Usage and error analytics
- **Debug Tools**: Device communication testing utilities

#### Interactions & Traffic Analysis
- **Traffic Overview**: View aggregate request counts for self-handled and proxied traffic.
- **Session Browsing**: Browse recorded interactions grouped by session.
- **Advanced Filtering**: Filter interactions by session, category (Self/Upstream), and timestamp.
- **Interaction Viewer**: View raw `.http` recording content directly in the browser.
- **Session Management**: Delete individual sessions or perform bulk cleanup to keep only recent sessions.
- **Session Download**: Download complete interaction sessions as `.tar.gz` archives for offline analysis or bug reports.
- **DNS Discoveries**: Real-time table of all hostnames discovered via the AfterTouch DNS server, categorized by interception status (Self/Upstream).

### Usage Tips

1. **First Time Setup**: The interface will guide you through initial device discovery
2. **Migration Monitoring**: Watch migration progress in real-time with detailed status updates
3. **Troubleshooting**: Use the debug tools to diagnose device connectivity issues
4. **Log Analysis**: Enable detailed logging for development and troubleshooting

## HTTP Interaction Recording

The service automatically records all HTTP interactions (both those handled locally and those proxied upstream) as `.http` files. These files are compatible with the [IntelliJ IDEA HTTP Client](https://www.jetbrains.com/help/idea/exploring-http-syntax.html).

### Key Features

- **Session Grouping**: All interactions from a single server session are stored in a dedicated directory named `{timestamp}-{pid}`.
- **Chronological Order**: Files are prefixed with a sequential number (e.g., `0001-`, `0002-`) to preserve the exact order of requests across the entire session.
- **Path-Based Structure**: Recordings are organized into subdirectories based on their URL path for better discoverability.
- **Automatic Sanitization**: Variable path segments like IP addresses, Device IDs, and Account IDs are automatically identified and replaced with placeholders (e.g., `{{ip}}`, `{{deviceId}}`). The original values are preserved as comments at the top of the recorded `.http` files for easy identification.
- **Re-playability**: An `http-client.env.json` file is generated for each session, allowing you to re-play the recorded requests immediately in IntelliJ IDEA.
- **Management UI**: The **5. Interactions** tab provides a built-in viewer and management tools for all recorded data.

### Configuration

#### Redaction

By default, the service redacts sensitive information from the recorded `.http` files, including:
- `Authorization` headers
- `Cookie` headers
- `X-Bose-Token` headers
- `X-Bose-Key` headers
- `Proxy-Authorization` headers

This behavior is controlled by the `--redact-logs` flag or the `REDACT_PROXY_LOGS` environment variable.

#### Custom Patterns

The service uses regex patterns to identify variable segments in URL paths. These patterns are loaded from `data/patterns.json`. You can add custom patterns to this file to support additional variable segments:

```json
[
  {
    "name": "MyVariable",
    "regexp": "^[0-9]{5}$",
    "replacement": "{myVar}"
  }
]
```

Variables found via these patterns will be:
1. Used as directory names in the `interactions/` folder.
2. Parameterized as `{{myVar}}` within the `.http` files.
3. Added to the `http-client.env.json` file with their actual values.

## Persistent Data

### Data Directory Structure

By default, the service creates a `data/` directory in the current working directory:

```
data/
├── accounts/
│   └── default/
│       ├── devices/
│       │   ├── {DEVICE_ID}/
│       │   │   ├── DeviceInfo.xml
│       │   │   └── config_backup_*.xml
│       │   └── ...
│       ├── Sources.xml
│       ├── Presets.xml
│       └── Recents.xml
├── interactions/
│   └── {SESSION_ID}/
│       ├── self/
│       │   └── {PATH}/
│       │       └── {SEQ}-{TIME}-{METHOD}.http
│       ├── upstream/
│       │   └── {PATH}/
│       │       └── {SEQ}-{TIME}-{METHOD}.http
│       └── http-client.env.json
├── dns/
│   └── discoveries.json
├── stats/
│   ├── usage/
│   │   └── *.json
│   └── error/
│       └── *.json
└── events/
    └── device_events_*.log
```

### Data Components

#### Device Data (`accounts/default/devices/{DEVICE_ID}/`)
- **DeviceInfo.xml**: Device metadata and capabilities
- **config_backup_*.xml**: Configuration backups before migration
- **presets.xml**: Device-specific preset configurations

#### Account Data (`accounts/default/`)
- **Sources.xml**: Configured music service providers
- **Presets.xml**: Cross-device preset synchronization
- **Recents.xml**: Recent playback history

#### DNS Data (`dns/`)
- **discoveries.json**: Persisted DNS discovery logs with hostname deduplication

#### Statistics (`stats/`)
- **usage/**: Device usage analytics and patterns
- **error/**: Error logs and diagnostic information

#### Events (`events/`)
- **device_events_*.log**: Device event history and debugging logs

#### HTTP Interactions (`interactions/`)
- **{SESSION_ID}/**: A unique directory per server run (format: `YYYYMMDD-HHMMSS-PID`).
- **self/**: Requests handled directly by the service.
- **upstream/**: Requests proxied to external Bose services.
- **{PATH}/**: Nested subdirectories reflecting the URL path (sanitized).
- **http-client.env.json**: IntelliJ IDEA HTTP Client environment file with session variables.
- **{SEQ}-{TIME}-{METHOD}.http**: Individual interaction recordings in standard HTTP Client format.

### Data Management

#### Backup Strategy
```bash
# Manual backup
cp -r data/ backup-$(date +%Y%m%d)/

# Automated backup (cron example)
0 2 * * * cp -r /path/to/data/ /backup/soundtouch-$(date +\%Y\%m\%d)/
```

#### Data Migration
```bash
# Moving to new server
tar czf soundtouch-data.tar.gz data/
# Transfer to new server
tar xzf soundtouch-data.tar.gz
```

#### Cleanup
```bash
# Clean old event logs (older than 30 days)
find data/events/ -name "*.log" -mtime +30 -delete

# Clean old statistics (older than 90 days)
find data/stats/ -name "*.json" -mtime +90 -delete
```

## API Endpoints

### Management UI
- **URL**: `http://localhost:8000/` or `http://localhost:8000/web/`
- **Description**: Browser-based guided flow for discovery, data sync, and migration.

### Setup API
- `GET /setup/devices`: List all known (auto-discovered and manual) devices.
- `POST /setup/devices`: Manually add a device by IP.
- `POST /setup/discover`: Trigger a new network discovery scan.
- `GET /setup/discovery-status`: Check if a scan is currently in progress.
- `POST /setup/sync/{deviceIP}`: Fetch presets, recents, and sources from a device.
- `GET /setup/summary/{deviceIP}`: Get a detailed migration readiness summary.
- `POST /setup/migrate/{deviceIP}`: Migrate a device using the specified method (XML/Hosts).
- `GET /setup/ca.crt`: Download the Root CA certificate for manual installation.

#### `GET /setup/interactions`
Lists recorded interactions with optional filtering.

**Query Parameters:**
- `session`: Filter by session ID (optional)
- `category`: Filter by category (`self` or `upstream`) (optional)
- `since`: Filter by timestamp (e.g., `2026-02-15 15:00:00`) (optional)

#### `GET /setup/interaction-stats`
Returns aggregate statistics about recorded interactions across all sessions.

#### `GET /setup/interaction-content?file={path}`
Returns the raw content of a specific recorded `.http` file.

#### `DELETE /setup/interactions/sessions/{sessionID}`
Deletes all recordings associated with a specific session.

#### `DELETE /setup/interactions/sessions?keep={N}`
Bulk cleanup: deletes all but the most recent `N` sessions.

### DNS Discovery API

#### `GET /setup/dns-discoveries`
Returns merged in-memory and persisted DNS discoveries, sorted by last seen timestamp.

#### `DELETE /setup/dns-discoveries`
Clears all recorded DNS discovery data from memory and disk.

### Emulated Services
- `/bmx/registry/v1/services`: BMX service registry.
- `/bmx/tunein/v1/*`: TuneIn radio emulation.
- `/marge/accounts/*`: Account and device management.
- `/marge/updates/soundtouch`: Software update emulation.
- `/proxy/*`: Logging proxy for original Bose services.

## Troubleshooting

### Common Issues

#### Device Not Discovered
```bash
# Check network connectivity
ping 192.168.1.100

# Trigger manual discovery
curl -X POST http://localhost:8000/setup/discover

# Check device accessibility
curl http://192.168.1.100:8090/info
```

#### Migration Failures
```bash
# Check SSH connectivity
ssh-keyscan 192.168.1.100

# Get migration summary
curl http://localhost:8000/setup/migration-summary/192.168.1.100

# Verify device configuration
curl http://192.168.1.100:8090/info
```

#### Service Connectivity Issues
```bash
# Test local service endpoints
curl http://localhost:8000/health
curl http://localhost:8000/bmx/registry/v1/services
curl http://localhost:8000/marge/streaming/sourceproviders
```

### Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
LOG_PROXY_BODY=true REDACT_PROXY_LOGS=false soundtouch-service
```

### Log Analysis

```bash
# Monitor service logs
tail -f /var/log/soundtouch-service.log

# Analyze proxy traffic
grep "PROXY" /var/log/soundtouch-service.log

# Check device events
ls -la data/events/
```

## Credits & Inspiration

This service implementation is based on and inspired by several excellent community projects:

### SoundCork
- **Project**: [SoundCork](https://github.com/deborahgu/soundcork)  
- **Authors**: Deborah Gu and contributors
- **Contribution**: The architecture and service emulation approach in this Go implementation is heavily based on SoundCork's pioneering Python implementation. SoundCork provided the foundation for understanding Bose's service architecture and migration strategies.

### ÜberBöse API
- **Project**: [ÜberBöse API](https://github.com/julius-d/ueberboese-api)
- **Author**: Julius D.
- **Contribution**: Advanced API endpoint discovery and implementation details that helped make this service more complete and robust.

We are grateful to these projects for paving the way and providing the research foundation that made this comprehensive service implementation possible.

## Advanced Usage

### Custom Service Integration

```go
// Example: Custom BMX service handler
package main

import (
    "net/http"
    "github.com/go-chi/chi/v5"
)

func customBMXHandler(w http.ResponseWriter, r *http.Request) {
    // Custom BMX service logic
    w.Header().Set("Content-Type", "application/json")
    w.Write([]byte(`{"custom": "service"}`))
}

func main() {
    r := chi.NewRouter()
    r.Get("/bmx/custom/endpoint", customBMXHandler)
    http.ListenAndServe(":8000", r)
}
```

### Integration with Home Assistant

```yaml
# configuration.yaml
soundtouch:
  - host: 192.168.1.100
    port: 8090
    name: "Living Room Speaker"
    
rest:
  - resource: "http://localhost:8000/setup/devices"
    scan_interval: 60
    sensor:
      - name: "SoundTouch Devices"
        value_template: "{{ value_json | length }}"
```

### Monitoring & Alerting

```bash
# Health check script
#!/bin/bash
response=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8000/health)
if [ $response != "200" ]; then
    echo "SoundTouch service is down!" | mail -s "Alert" admin@example.com
fi
```

## Security Considerations

- **Network Security**: The service binds to all interfaces by default. Consider using `BIND_ADDR=127.0.0.1` for localhost-only access.
- **SSH Access**: Migration requires SSH access to devices. Ensure your network security policies allow this.
- **Proxy Logging**: Disable `REDACT_PROXY_LOGS` only in development environments.
- **Data Protection**: The data directory contains device configurations and usage patterns. Secure appropriately.

## Performance Tuning

### Resource Usage
- **Memory**: ~50MB baseline + ~5MB per discovered device
- **CPU**: Minimal during steady state, ~10% during discovery/migration
- **Disk**: ~1MB per device configuration + logs

### Scaling Considerations
```bash
# For many devices, increase discovery interval
DISCOVERY_INTERVAL=10m soundtouch-service

# For high-traffic environments, consider reverse proxy
nginx -> soundtouch-service instances
```
