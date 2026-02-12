# Undocumented Community Features & API Discoveries
This document captures advanced API endpoints and device behaviors discovered by the SoundTouch community through reverse engineering projects like **SoundCork** and **ÜberBöse API**. These features are not documented in the official Bose SoundTouch Web API v1.0 but are crucial for full device emulation and offline operation.
## Cloud Emulation (Marge/BMX) Discoveries
While the local `/8090` API is well-documented, the cloud-side service emulation reveals deeper device integration points.
### 1. Stereo Pairing & Cloud-Side Grouping
SoundCork has pioneered the emulation of "Marge" group endpoints, which differ from the local `/getGroup` API. These are primarily used for persistent configurations like **Stereo Pairs** (e.g., two ST-10s).
- **GET** `/marge/streaming/account/{account}/device/{device}/group`
  Returns `<group/>` if ungrouped, or full group configuration for stereo pairs.
- **POST** `/marge/streaming/account/{account}/group`
  Creates a new group (returns a 7-digit group ID). Used for initial pairing.
- **DELETE** `/marge/streaming/account/{account}/group/{group}`
  Dissolves a group configuration.
### 2. Device Analytics & Event Reporting
Devices report real-time telemetry to the cloud. Intercepting these provides a window into device usage without polling.
- **Endpoint**: `POST /v1/scmudc/{deviceId}`
- **Function**: Submits event data including `play-state-changed`, `preset-pressed`, `power-pressed`, `source-state-changed`, and `art-changed` (Metadata updates). This endpoint was first extensively documented in the **ÜberBöse API** specification.
### 3. Power-On Lifecycle
When a SoundTouch device boots or "powers on" (distinct from waking from standby), it contacts specific support endpoints.
- **Endpoint**: `POST /streaming/support/power_on`
- **Behavior**: Reports device serial number, IP address, and diagnostic data.
- **Critical Finding**: SoundTouch devices fetch `TUNEIN` and `LOCAL_INTERNET_RADIO` source availability from the cloud **ONLY at boot time**. If the cloud is unreachable during a hard reboot (power cycle), these sources will disappear from the device's `/sources` list and become unavailable, even if the local API is working. This behavior was analyzed and reported by the **ÜberBöse API** project (Issue #3).
### 4. OAuth & Service Tokens
Integration with music services (Spotify, Pandora, etc.) involves specific token management endpoints.
- **Endpoint**: `POST /oauth/device/{deviceId}/music/musicprovider/{providerId}/token/{tokenType}`
- **Usage**: Used to refresh or validate session tokens for cloud-based music providers.
## Community-Driven Extensions
The community is working on extending SoundTouch functionality beyond its original design.
### 1. Radio-Browser.info Integration
There is an active effort to add `radio-browser.info` as a native `sourceprovider`. This would allow devices to browse a massive directory of thousands of stations without relying on the TuneIn cloud service.
- **Status**: Research phase in SoundCork (Issue #150).
- **Implementation**: Requires adding a new source provider entry in the emulated `/streaming/sourceproviders` response.
### 2. Stockholm Internal App Analysis
Deep analysis of the Stockholm (device firmware) internal web application reveals a set of internal AJAX/XML calls used by the device's own control interface.
- **Internal Domains**: `Marge` (XML-based) and `Gabbo` (App-send based).
- **Reference**: See SoundCork Issue #128 for a comprehensive list of internal JS controllers and their functions.
### 3. ETag Case-Sensitivity Bug
The SoundTouch device firmware has a case-sensitivity bug regarding HTTP `ETag` headers.
- **Discovery**: SoundCork Issue #129.
- **Detail**: The device expects the `ETag` header to be exactly title-cased. If a server returns `etag` (lowercase), the device fails to use it for `If-None-Match` requests, breaking efficient preset synchronization.
- **Solution**: Force title-casing of the header via a reverse proxy like Nginx or mitmproxy.
## References
- [SoundCork GitHub Repo](https://github.com/deborahgu/soundcork)
- [ÜberBöse API Spec](https://github.com/julius-d/ueberboese-api)
- [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- [IsItBose Regex Research](https://github.com/deborahgu/soundcork/issues/62#issuecomment-3610563908)
