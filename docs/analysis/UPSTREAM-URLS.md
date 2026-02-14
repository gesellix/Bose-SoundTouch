# Upstream URLs & Domains Analysis

This document provides a comprehensive overview of the upstream Bose cloud services and domains that SoundTouch devices communicate with. These details were gathered from firmware analysis of ST10/ST20 devices, binary string extraction, and community research from the **SoundCork** project (Issue #128).

## Core Service Domains

SoundTouch devices use a set of primary domains for their operation. These are often configurable via the `SoundTouchSdkPrivateCfg.xml` file.

| Service | Primary Domain | Purpose |
| :--- | :--- | :--- |
| **Marge** | `streaming.bose.com` | Account management, streaming source providers, and preset sync. |
| **BMX Registry** | `content.api.bose.io` | Bose Media eXchange service discovery and registry. |
| **Stats/Analytics** | `events.api.bosecm.com` | Telemetry, device events, and usage statistics. |
| **Software Update** | `worldwide.bose.com` | Firmware update checks and downloads (path: `/updates/soundtouch`). |
| **Voice/Alexa** | `voice.api.bose.io` | Token management for Amazon Alexa integration. |

## Internal & Development Domains

Analysis of device binaries (`BoseApp`, `IoT`) and community findings revealed several internal, integration, and development domains used by Bose.

### Marge & Auth Proxies
- `bose-test.apigee.net/margeproxy` (Integration/Test proxy)
- `bose-test.apigee.net/margeproxyefe`
- `streamingstg.bose.com` (Staging)
- `streamingintoauth.bose.com` (Internal Auth)
- `streamingefeintoauth.bose.com` (Internal EFE Auth)
- `streamingefeint.bose.com`

### BMX & Content Registry
- `test.content.api.bose.io`
- `content.api.bose.io/bmx/registry/v1/services`
- `test.content.api.bose.io/bmx/int-registry/v1/services`
- `test.content.api.bose.io/bmx/efe-registry/v1/services`

### Stats & Analytics
- `eventsdev.api.bosecm.com`
- `eventsefe.api.bosecm.com`
- `eventsdev.bosecm.com`

### Software Updates
- `worldwide.bose.com/updates/soundtouch-int`
- `worldwide.bose.com/updates/soundtouch-efe`

## Third-Party Services

Devices also communicate directly with third-party providers for specific features.

- **Pandora**: 
    - `device-tuner.pandora.com`
    - `device-tuner-beta.savagebeast.com`
- **Amazon AVS**:
    - `avs.na.amazonalexa.com`

## Hardcoded Validation (IsItBose)

As documented in [DEVICE-REDIRECT-METHODS.md](DEVICE-REDIRECT-METHODS.md#method-3-binary-patching), the `libBmxAccountHsm.so` library contains a hardcoded regex to validate these URLs:

`^https:\/\/bose-[a-zA-Z0-9\.\_\-\$\%]\+\.apigee\.net\/`

This regex ensures that certain critical services must reside on the `apigee.net` domain under a `bose-` prefix, unless patched.

## Configuration File References

On-device, these URLs are primarily managed in the following files:

1.  **`/opt/Bose/etc/SoundTouchSdkPrivateCfg.xml`**:
    *   `<margeServerUrl>`
    *   `<statsServerUrl>`
    *   `<swUpdateUrl>`
    *   `<bmxRegistryUrl>`
2.  **`/opt/Bose/etc/Voice.xml`**:
    *   `<TPDATokenUrl>` (Points to `voice.api.bose.io`)
3.  **`/opt/Bose/etc/HandCraftedWebServer-SoundTouch.xml`**:
    *   Contains internal local API mapping.

## Conclusion for Offline Operation

To achieve full offline operation or redirection to a custom service (like `soundtouch-service`), all of the above domains must either be redirected via DNS (`/etc/hosts`) or updated in the device's XML configuration files. For domains not exposed in XML, binary patching or DNS-level redirection is the only option.

---

## References
- [SoundCork Issue #128: Endpoint and URL Listing](https://github.com/deborahgu/soundcork/issues/128#issuecomment-3892933337)
- [Bose SoundTouch Web API v1.0 Specification](https://assets.bosecreative.com/m/496577402d128874/original/SoundTouch-Web-API.pdf)
