# Bose SoundTouch Cloud API Emulation (Marge/BMX/Stats)

This document describes the cloud-emulation APIs provided by the SoundTouch service. These APIs mimic the Bose cloud services (Marge, BMX, Stats) that SoundTouch devices and the SoundTouch controller application (Stockholm) interact with.

## Marge API (Account & Configuration)

Base path: `/marge`

### GET /streaming/sourceproviders
Retrieves a list of available streaming source providers.

### GET /accounts/{accountId}/full
Retrieves the full account configuration including sources, presets, and devices.

### GET /streaming/account/{accountId}/emailaddress
Retrieves the email address associated with the account.

### GET /streaming/device_setting/account/{accountId}/device/{deviceId}/device_settings
Retrieves settings for a specific device (e.g., clock format).

### POST /streaming/device_setting/account/{accountId}/device/{deviceId}/device_settings
Updates settings for a specific device.

### POST /accounts/{accountId}/devices/{deviceId}/presets/{presetNumber}
Updates a preset for a device.

### POST /accounts/{accountId}/devices/{deviceId}/recents
Adds an item to the device's recently played history.

### POST /accounts/{accountId}/devices
Adds a device to the account.

### DELETE /accounts/{accountId}/devices/{deviceId}
Removes a device from the account.

## Customer API (Profile & Password)

Base path: `/customer`

### GET /account/{accountId}
Retrieves the customer account profile.

### POST /account/{accountId}
Updates the customer account profile.

### POST /account/{accountId}/password
Changes the account password.

## Analytics & Stats API

Base path: `/v1` (App Events) or `/streaming/stats` (Device Stats)

### POST /v1/stapp/{deviceId}
Endpoint called by Bose SoundTouch mobile and web applications (Stockholm) to submit event data.

### POST /v1/scmudc/{deviceId}
Endpoint equivalent to `/v1/stapp/{deviceId}` sometimes used by apps or devices.

### POST /streaming/stats/usage
Endpoint used by physical devices to report usage statistics.

### POST /streaming/stats/error
Endpoint used by physical devices to report error statistics.

## BMX API (Streaming & Registry)

Base path: `/bmx`

### GET /registry/v1/services
Retrieves the registry of available streaming services.

### GET /tunein/v1/playback/station/{stationID}
Retrieves playback information for a TuneIn station.
