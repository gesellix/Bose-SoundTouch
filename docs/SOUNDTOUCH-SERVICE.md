# SoundTouch Service

The `soundtouch-service` is a companion service for Bose SoundTouch devices. It provides:
- A REST API for device management and discovery.
- Emulation of Bose backend services (BMX and Marge), allowing devices to work without an active internet connection to Bose servers.
- A logging proxy for inspecting device communication.
- A web interface for management.

## Installation

```bash
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-service@latest
```

## Running the Service

Simply run the binary:
```bash
soundtouch-service
```

### Configuration

The service can be configured via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Port to bind the service to | `8000` |
| `BIND_ADDR` | Network interface to bind to | all (ip4 and ip6) |
| `DATA_DIR` | Directory for persistent data (devices, stats) | `./data` |
| `SERVER_URL` | External URL of this service | `http://<hostname>:8000` |
| `REDACT_PROXY_LOGS` | Set to `false` to show sensitive data in proxy logs | `true` |
| `LOG_PROXY_BODY` | Set to `true` to log full request/reponse bodies | `false` |

## API Endpoints

### Discovery & Setup
- `GET /setup/devices`: List all discovered Bose devices.
- `POST /setup/discover`: Trigger a new network scan.
- `GET /setup/info/{deviceIP}`: Get detailed info for a specific device.
- `POST /setup/migrate/{deviceIP}`: Configure a device to use this service as its backend.

### BMX (Bose Music eXperience)
- `GET /bmx/registry/v1/services`: Service registry for the device.
- `GET /bmx/tunein/v1/playback/station/{stationID}`: TuneIn playback bridge.

### Marge (Account & Device Management)
- `GET /marge/streaming/sourceproviders`: List of available music services.
- `GET /marge/accounts/{account}/full`: Mock account information.
- `GET /marge/updates/soundtouch`: Mock software update endpoint.

### Proxy
- `GET /proxy/{targetURL}`: Proxy requests through the service with logging.

## Web Interface

Access the management interface at `http://localhost:8000/`. The interface allows you to view discovered devices and manage their settings.

## Persistent Data

By default, the service creates a `data/` directory in the current working directory. This directory contains:
- `default/devices/`: Configuration and state for each discovered device.
- `usage_stats.json`: Logged device usage statistics.
- `error_stats.json`: Logged device errors.
