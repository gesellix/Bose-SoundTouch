# Parity Analysis: Bose-SoundTouch (Go) vs. SoundCork (Python)

This document provides a comparative analysis of the current Go implementation and the `deborahgu/soundcork` project, identifying functional gaps and potential improvements.

## 1. Core Architecture and Language
- **Bose-SoundTouch (Go)**: Uses `chi` for routing and `encoding/xml` for data. High performance, strong typing, and precise MIME type handling (`application/vnd.bose.streaming-v1.2+xml`).
- **SoundCork (Python)**: Uses `FastAPI` and `xml.etree.ElementTree`. Prioritizes flexibility and rapid prototyping of streaming service mocks.

## 2. Functional Comparison

| Feature              | Bose-SoundTouch (Go)                             | SoundCork (Python)                                                                      |
|:---------------------|:-------------------------------------------------|:----------------------------------------------------------------------------------------|
| **Group Management** | Placeholder handlers (return `<group/>` or 404). | Active group management (`groups.py`), supporting `/addGroup` and stereo pairing logic. |
| **BMX Services**     | Supports TuneIn, Orion, and custom streams.      | More modular `bmx_services.json` registry with broader mock support.                    |
| **Persistence**      | Mixed JSON/XML datastore.                        | Pure XML-based persistence per device/account.                                          |
| **Admin UI**         | CLI-based (`soundtouch-cli`) or API-driven.      | Draft Web UI for device discovery and account management (`admin.py`).                  |
| **Discovery**        | Integrated setup tools and SSDP/MDNS awareness.  | Leverages `bosesoundtouchapi` Python library for active discovery.                      |

## 3. Key Strengths of SoundCork
- **Group Pairing Logic**: Includes logic to manage master/slave relationships for SoundTouch 10 stereo pairs.
- **Service Extensibility**: JSON-based registry for BMX services makes it easier to mock multiple providers (SiriusXM, Spotify) without code changes.
- **Mock Coverage**: Better coverage of "dummy" endpoints that respond with plausible XML (e.g., `customerSupport`).

## 4. Suggested Implementation Steps for Bose-SoundTouch

### A. Implement Full Group Support (High Priority)
- Add logic to `pkg/service/marge` to handle `/addGroup` and `/updateGroup`.
- Persist group memberships in the datastore to allow speakers to function as stereo pairs or multi-room zones.

### B. Modularize BMX Registry (Medium Priority)
- Extract the hardcoded service list in `HandleBMXRegistry` into an external `bmx-services.json` file.
- Allow users to customize which mocked services are advertised to the speaker.

### C. Enhanced Source Management (Medium Priority)
- Refine source learning logic to ensure all `sourceAccount` and `sourceName` metadata is correctly captured during synchronization, using patterns from `soundcork`'s `learnSource`.

### D. Basic Admin Web UI (Low Priority)
- Develop a minimal internal status page to list active accounts and connected devices, improving usability over raw API calls.

## 5. Summary
While our Go implementation is structurally more consistent with recent reference recordings (e.g., `buttonNumber`, detailed `components`), SoundCork provides better coverage of multi-device coordination (Groups) and service emulation (BMX) that we should adopt for a more complete offline experience.
