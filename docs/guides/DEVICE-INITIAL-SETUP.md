# SoundTouch Device Initial Setup Variants

Based on community research from the **SoundCork** and **ÜberBöse API** projects, as well as analysis of the Stockholm firmware (`firmware/Stockholm/.../setup/`), this document outlines the methods used for the "out-of-the-box" setup of SoundTouch devices.

## Setup Overview

Initial setup is the process of connecting a new or factory-reset device to a local Wi-Fi network and a Bose (or custom) account. This is distinct from the "Migration" process (handled by `soundtouch-service`), which redirects an already-configured device to a new server.

---

## 1. Bluetooth Low Energy (BLE) Setup
Used by most modern SoundTouch devices (ST-10, ST-20/30 Series III, SoundTouch 300).

- **Mechanism**: The SoundTouch app communicates with the device over BLE to exchange Wi-Fi credentials.
- **Protocol**: Internal research refers to this as the **Gabbo** protocol (see `gabbo_setup_bco.js` in firmware).
- **Process**:
  1. Put the device in setup mode (usually by holding the '2' and '-' buttons).
  2. The app discovers the device via BLE.
  3. The app sends the Wi-Fi SSID and Password to the device.
  4. The device connects to Wi-Fi and disables BLE setup.

---

## 2. Access Point (AP) Mode / Web Setup
The classic "failover" or "alternate" setup method.

- **Mechanism**: The device creates its own Wi-Fi network (SSID: `Bose SoundTouch ...` or `Bose Home Speaker ...`).
- **IP Address**: Typically `192.168.1.1` or `10.0.0.1` (device-side).
- **Web Interface**: The device hosts a web server on port 80.
- **Process**:
  1. Connect a PC/Phone to the device's Wi-Fi.
  2. Open a browser to `http://192.168.1.1`.
  3. The device serves `setup.html`, which redirects to a setup wizard (`setup/index.html`).
  4. Use the `gabbo_wifi` form to select a network and enter credentials.

---

## 3. Wireless Accessory Configuration (WAC)
Specific to Apple iOS devices.

- **Mechanism**: Uses Apple's MFi/WAC protocol to pass Wi-Fi settings from an iPhone/iPad directly to the device without manual password entry.
- **Status**: Detected automatically by iOS when a new SoundTouch device is in setup mode.

---

## 4. USB Setup (Legacy)
Primarily used for older SoundTouch Series I and II devices or as a last resort.

- **Mechanism**: Physical connection via Micro-USB to a computer running the SoundTouch Setup application.
- **Process**:
  1. Connect USB cable.
  2. The desktop app communicates via a proprietary HID or Serial-over-USB protocol.
  3. The app pushes Wi-Fi credentials.
  4. References to this exist in the firmware as `lost_USB_connection` and `connect_device` (see `setup_wizard.xml`).

---

## Technical Details: The "Gabbo" Protocol
The Stockholm firmware contains references to a communication layer called **Gabbo**.
- **File**: `setup/js/gabbo_setup_bco.js`
- **Function**: Handles the state machine for Wi-Fi connection, account pairing, and error handling during setup.
- **Relationship**: It appears to be an internal wrapper for the messages sent between the setup client (App or Browser) and the device firmware.

## Redirection during Setup
While the `soundtouch-service` focuses on migrating existing devices, a truly "clean" setup to a custom service would require:
1. Intercepting the initial account pairing request.
2. Providing a mock "Marge" service that accepts any credentials.
3. Patching the `SoundTouchSdkPrivateCfg.xml` during or immediately after the Wi-Fi connection phase.

---

## Comparison: Initial Setup vs. Migration

| Feature | Initial Setup | Migration (soundtouch-service) |
| :--- | :--- | :--- |
| **Connectivity** | BLE, AP Mode, USB, WAC | Ethernet/Wi-Fi (existing) |
| **Credentials** | Required (SSID/Pass) | Not required (uses existing) |
| **Access** | Web UI / App protocol | SSH (root) |
| **Primary File** | `setup/index.html` | `SoundTouchSdkPrivateCfg.xml` |
| **Use Case** | Out-of-the-box / Reset | Redirecting active devices |
