---
title: "Bose SoundTouch Device Setup Flow"
sidebar:
  exclude: true
---
This document details the multi-step process required to fully set up a Bose SoundTouch device, as derived from the Stockholm firmware (`setup/js/`) analysis.

A complete setup flow involves a sequence of local (WebSocket) and cloud (HTTP) actions that move the device from a factory-reset state to a fully registered, functional system.

## 1. Local Coordination Stage (WebSocket)

Before a device can be controlled, it must be configured on the local network and named. These actions occur via a WebSocket connection to the device on port 8080.

### 1.1 Language Configuration (Optional)
If the device is in a factory-reset state, the UI typically ensures the device language matches the user's choice.
- **WebSocket Action**: `set_language`
- **Internal Logic**: `SetupWizard.js` handles this via `set_device_language`.

### 1.2 Network Configuration (WiFi)
Configures the device to connect to a specific wireless access point.
- **File Reference**: `setup/js/workflow_wifi_setup.js`
- **Logic**: Triggers a site survey, then sends SSID and credentials.
- **WebSocket Command**: `set_WIFI_OLED` or similar internal method calls to configure the network profile.

### 1.3 Device Naming (Rename Step)
Assigns a user-friendly name (e.g., "Living Room") to the device.
- **File Reference**: `setup/js/workflow_rename.js`
- **WebSocket Action**: `name`
- **XML Payload**:
    ```xml
    <name>Living Room</name>
    ```
- **Implementation**: The `RenameDevices.do_rename_devices()` function sends this to the device. The device then updates its local name and mDNS/SSDP broadcasts.

## 2. Cloud Interaction Stage (HTTP)

The device needs to be linked to a Bose "Marge" account to enable cloud-based features and music services.

### 2.1 Account Creation (Registration)
If a user doesn't have an account, the setup client creates one.
- **File Reference**: `setup/js/workflow_marge.js`
- **Cloud Endpoint**: `POST https://streaming.bose.com/streaming/account`
- **Payload**: XML containing name, email, password, and country.
- **Content-Type**: `application/vnd.bose.customer-v1.0+xml`

### 2.2 Cloud Authentication (Login)
The setup client must obtain a valid `accountId` and `userAuthToken` to pair the device.
- **File Reference**: `setup/js/workflow_marge.js`
- **Cloud Endpoint**: `POST https://streaming.bose.com/streaming/account/login`
- **Payload**: XML containing username and password.
- **Content-Type**: `application/vnd.bose.streaming-v1.2+xml`
- **Result**: Returns a session token in the `Credentials` response header and the user's `account ID` in the XML body.

## 3. Registration Bridge (WebSocket to Cloud)

This is the final "pairing" step where the client tells the device which account it belongs to.

### 3.1 Device Registration (The "Pair" Step)
The client sends the user's credentials to the device, which then registers itself with the cloud.
- **File Reference**: `setup/js/workflow_add_devices.js`
- **WebSocket Action**: `setMargeAccount`
- **XML Payload**:
    ```xml
    <PairDeviceWithAccount>
        <accountId>12345</accountId>
        <userAuthToken>jGwE... (truncated)</userAuthToken>
    </PairDeviceWithAccount>
    ```
- **Device Reaction**: Upon receiving this, the device makes its own outbound HTTP POST to the Marge service:
    `POST https://streaming.bose.com/{accountId}/devices`

## 4. Finalization

Once the registration is complete, the setup application (Stockholm) performs final cleanup. It's important to distinguish between **App State** (the Stockholm UI's persistent settings) and **Device State** (the physical speaker's configuration).

### 4.1 Exiting Setup Mode (App Settings)
The Stockholm app communicates with its "native container" (the WebView bridge on iOS/Android/Windows/macOS) using a `setData` command in **JSON format**. This is an internal message to the application's persistent storage, **not a network command sent to the physical speaker**.

This command tells the Stockholm app which page to load on startup, effectively marking the setup as complete in the UI.

- **Internal Command**: `setData`
- **Parameter**: `startupPage`
- **Normal Value**: `index.html` (Normal mode)
- **Setup Value**: `setup/index.html` (Setup mode)

**JSON Payload (Internal to Stockholm App)**:
```json
{
    "method": "setData",
    "params": {
        "name": "startupPage",
        "value": "index.html"
    }
}
```

**Other Common Internal Parameters**:
- `changeStartupPage`: Set to `false` after a successful setup or update.
- `tipsEnabled`: Set to `false` to suppress the "Getting Started" tutorials.
- `promptUpdate`: Set to `true` if a firmware update was deferred during setup.

### 4.2 Device Finalization
The physical speaker considers the setup "done" once it successfully processes the `<PairDeviceWithAccount>` XML message and completes its own handshake with the Marge cloud. There is no specific "Finalize" XML command sent to the speaker; the successful registration is the signal.

The `SetupWizard.js` calls `single_device_setup_done()` to trigger the internal `setData` updates described above. If these are not saved in the app's local storage, the Stockholm UI may return to the setup flow on next launch, even if the speaker is already paired.

---

## Summary of Scriptable Requirements

To automate a device setup using a custom tool (like `soundtouch-cli`), you must perform the following:
1.  **Configure WiFi**: (Assumed if device is reachable over IP).
2.  **Set Name**: Send the `<name>` WebSocket message (XML) to update the device identity.
3.  **Obtain Token**: Authenticate against the cloud service (Marge) via HTTP.
4.  **Pair Device**: Send the `<PairDeviceWithAccount>` WebSocket message (XML) with the account ID and token.

**Note**: The JSON `setData` commands are only necessary if you are building/controlling a version of the Stockholm UI itself. They are not required to configure the physical hardware.
