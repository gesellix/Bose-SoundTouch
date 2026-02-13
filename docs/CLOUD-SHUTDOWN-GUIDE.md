### Bose Cloud Shutdown: Survival Guide for SoundTouch

With Bose's announcement of discontinuing cloud support for SoundTouch devices in May 2026, this project provides the necessary tools to keep your speakers fully functional using a local emulation service.

This guide explains how to set up the `soundtouch-service` to run your devices independently of Bose's servers.

---

### Supported Use Cases

1.  **Local Service Emulation**: The service emulates Bose's BMX (Bose Media eXchange) and Marge services, which handle content registries, presets, recents, and software update checks.
2.  **Traffic Redirection**: Tools are provided to redirect your speakers to this local service instead of `*.bose.com`.
3.  **Offline Operation**: Once redirected, the speakers function without needing to reach Bose's servers.
4.  **Preset & Recent Management**: Captures and stores presets and "recently played" items locally.

---

### Setup Steps

To set up your SoundTouch system for local-only operation, follow these steps:

#### 1. Install and Start the Service
Run the `soundtouch-service` on a machine that is always on (like a Raspberry Pi or a NAS) within your local network.

```bash
# Install the service
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-service@latest

# Start the service (defaults to http://localhost:8000)
soundtouch-service
```

#### 2. Access the Management UI
Open your web browser and navigate to the service's web interface:
`http://<your-server-ip>:8000/`, e.g. `http://localhost:8000/`

*Note: The service also supports a `/web/` path for management.*

#### 3. Enable SSH on Your Speakers
To migrate your speakers, the service needs SSH access. You can enable it by:
1. Creating an empty file named `remote_services` on a USB stick.
2. Inserting the USB stick into the SoundTouch speaker's service port.
3. Rebooting the speaker.
Once enabled, you can log in as `root` (no password).

#### 4. Discover and Sync Device Data
The web interface handles the entire process in a guided flow across four tabs:

*   **Step 1: Devices**: The service automatically scans for SoundTouch devices on your network. If a device is not found, you can manually add its IP address.
*   **Step 2: Data Sync**: Select your device and click "Start Sync". This will automatically fetch your presets, recents, and configured sources from the speaker and store them in the local `data/` directory.
*   **Step 3: Migration**: Choose your redirection method (XML Recommended) and click "Confirm Migration & Reboot".
*   **Step 4: Settings**: Configure global server URLs and proxy behavior (logging, redaction).

#### 5. Verify Your Local Data
Once migrated, your speaker will use the data captured during the Sync step.
*   The service stores data in the `data/` directory, organized by device serial number (e.g., `data/default/devices/<SERIAL>/`).
*   **Automatic Capture**: As you use the device (changing presets, playing new music), the service continues to "learn" and update your local files.

---

### Comparison with other implementations (soundcork)
Our implementation (`soundtouch-service`) is largely compatible with the Python-based `soundcork` project but offers several advantages:
- **Web UI**: Integrated management interface for discovery and migration.
- **Surgical Migration**: Uses XML-based redirection by default, which is less invasive than `/etc/hosts`.
- **Automated SSL**: Handles Root CA injection automatically for secure communication.
- **Proxy Support**: Can proxy requests to original Bose servers while "learning" your configuration.

---

### Alternative: DNS Redirection (No SSH)
If you prefer not to modify your speakers via SSH, you can use a local DNS server (like Pi-hole, AdGuard Home, or Unbound) to point the following domains to your local server's IP:

*   `bmx.bose.com`
*   `streaming.bose.com`
*   `updates.bose.com`
*   `stats.bose.com`
*   `content.api.bose.io`

*Note: DNS redirection for HTTPS services requires the speakers to trust your local service's SSL certificate. The SSH-based migration handles this automatically by injecting the CA.*

---
