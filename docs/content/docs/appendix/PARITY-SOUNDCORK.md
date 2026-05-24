---
title: "Parity Analysis: Bose-SoundTouch (Go) vs. SoundCork (Python)"
sidebar:
  exclude: true
---

# Parity Analysis: Bose-SoundTouch (Go) vs. SoundCork (Python)

This document provides a comparative analysis of the current Go implementation and the `deborahgu/soundcork` project, identifying functional gaps and potential improvements.

## 1. Core Architecture and Language
- **Bose-SoundTouch (Go)**: Uses `chi` for routing and `encoding/xml` for data. High performance, strong typing, and precise MIME type handling (`application/vnd.bose.streaming-v1.2+xml`).
- **SoundCork (Python)**: Uses `FastAPI` and `xml.etree.ElementTree`. Prioritizes flexibility and rapid prototyping of streaming service mocks.

## 2. Functional Comparison

| Feature              | Bose-SoundTouch (Go)                                                                                                  | SoundCork (Python)                                                                      |
|:---------------------|:----------------------------------------------------------------------------------------------------------------------|:----------------------------------------------------------------------------------------|
| **Group Management** | Full CRUD: `POST /group`, `POST /group/{id}`, `DELETE /group/{id}` with XML datastore persistence (`Group_{id}.xml`). | Active group management (`groups.py`), supporting `/addGroup` and stereo pairing logic. |
| **ZeroConf Priming** | Full DH key exchange + encrypted blob; fallback to `tokenType=accesstoken` for older firmware.                        | Simple `tokenType=accesstoken` push only; token expires after ~60 minutes.              |
| **BMX Services**     | Supports TuneIn, Orion, and custom streams.                                                                           | More modular `bmx_services.json` registry with broader mock support.                    |
| **Persistence**      | Mixed JSON/XML datastore.                                                                                             | Pure XML-based persistence per device/account.                                          |
| **Admin UI**         | CLI-based (`soundtouch-cli`) or API-driven.                                                                           | Draft Web UI for device discovery and account management (`admin.py`).                  |
| **Discovery**        | Integrated setup tools and SSDP/MDNS awareness.                                                                       | Leverages `bosesoundtouchapi` Python library for active discovery.                      |

## 3. Key Strengths of SoundCork
- **Group Pairing Logic**: Includes logic to manage master/slave relationships for SoundTouch 10 stereo pairs.
- **Service Extensibility**: JSON-based registry for BMX services makes it easier to mock multiple providers (SiriusXM, Spotify) without code changes.
- ~~**Mock Coverage**: Better coverage of "dummy" endpoints that respond with plausible XML (e.g., `customerSupport`).~~ **Addressed**: AfterTouch's `HandleNotFound` (registered via `r.NotFound`) logs every unimplemented endpoint as `[UNHANDLED]` and forwards the request to the Bose upstream via `HandleBoseProxy`. This provides at least the same coverage as static dummy responses, while also aiding discovery of new endpoints.

## 4. Suggested Implementation Steps for Bose-SoundTouch

### ✅ A. Implement Full Group Support (Completed)
- `POST /group`, `POST /group/{id}`, `DELETE /group/{id}` implemented in `pkg/service/handlers/handlers_marge.go`.
- Group CRUD persisted in XML datastore (`Group_{id}.xml`) via `pkg/service/datastore/datastore.go`.
- `GET /group` on device registration reads the group the device belongs to.

### ✅ B. Proper ZeroConf Spotify Blob (Completed)
- Full Spotify Connect ZeroConf protocol implemented in `pkg/service/spotify/zeroconf.go`.
- Flow: `getInfo` (fetch speaker DH public key) → 768-bit DH key exchange → AES-128-CTR encrypted `LoginCredentials` protobuf blob → `addUser`.
- Speaker can self-refresh credentials independently; no periodic re-priming needed for token expiry.
- Automatic fallback to `tokenType=accesstoken` if `getInfo` fails (older firmware without DH support).
- See `docs/concepts/spotify-priming-strategy.md` for full protocol details.
- **Remaining gap**: Background watchdog to re-prime devices that lose their session (reboot / power loss). Not required for token expiry on modern firmware; only needed for the "speaker rebooted and lost state" recovery path and for older firmware on the fallback path (~45 min token expiry).

### C. Modularize BMX Registry (Medium Priority)
- Extract the hardcoded service list in `HandleBMXRegistry` into an external `bmx-services.json` file.
- Allow users to customize which mocked services are advertised to the speaker.

### D. Enhanced Source Management (Medium Priority)
- Refine source learning logic to ensure all `sourceAccount` and `sourceName` metadata is correctly captured during synchronization, using patterns from `soundcork`'s `learnSource`.

### E. Basic Admin Web UI (Low Priority)
- Develop a minimal internal status page to list active accounts and connected devices, improving usability over raw API calls.

## 5. Summary

Group support and ZeroConf Spotify priming are now feature-complete in AfterTouch. The Go implementation is structurally more consistent with recent reference recordings (e.g., `buttonNumber`, detailed `components`). SoundCork's remaining functional advantages are:

- **BMX service extensibility**: the `bmx_services.json` registry makes it trivial to add or mock new streaming providers without code changes (step C above).
- **Group pairing logic**: master/slave relationship management for SoundTouch 10 stereo pairs goes beyond the CRUD AfterTouch implements.

For the broader ecosystem context (feature matrix across all community projects, AfterTouch open tasks, and cross-project observations) see [docs/analysis/bose-soundtouch-community-tools.md](../analysis/bose-soundtouch-community-tools.md).
