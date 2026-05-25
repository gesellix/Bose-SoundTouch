---
title: "Bose SoundTouch — Community Tools for Post-EOL Preservation"
---
> **Context:** Bose announced the shutdown of SoundTouch cloud services, extended to **May 6, 2026**. On that date the official SoundTouch app will update to a local-only version. Bose has released the [SoundTouch Web API documentation](https://assets.bosecreative.com/m/496577402d128874/original/SoundTouch-Web-API.pdf) as open-source to enable community-driven development. This document surveys the active community projects, their feature coverage, and open development opportunities.

---

## What Bose Is Doing

After the May 6, 2026 shutdown, the following will **continue to work**:

- Streaming via Bluetooth, AirPlay, Spotify Connect, and AUX
- Local device control and grouping via an updated SoundTouch app
- Remote control features (play, pause, skip, volume)
- HDMI/optical connections on soundbars

The following will **stop working**:

- Physical and app-based presets
- In-app music service browsing (TuneIn, Pandora, etc.)
- Stereo pairing for SoundTouch 10
- Security and firmware updates

---

## Community Projects

### 1. soundcork
**[github.com/deborahgu/soundcork](https://github.com/deborahgu/soundcork)**
| | |
|---|---|
| Language | Python |
| License | MIT |
| Stars | 111 |
| Contributors | 8 |
| Commits | 287 |
| Status | Pre-alpha, actively developed |

A reverse-engineered intercept API that replaces the Bose cloud servers locally. Works by redirecting the speaker's internal `SoundTouchSdkPrivateCfg.xml` to a self-hosted FastAPI server, emulating the `marge` server (required for basic network functionality) and the `bmx` server (required for TuneIn). Deployable as a Docker container or systemd daemon. The most community-engaged project, with a dedicated discussion thread tracking Bose cloud service status.

---

### 2. Überböse API
**[github.com/julius-d/ueberboese-api](https://github.com/julius-d/ueberboese-api)**
| | |
|---|---|
| Language | Java (Spring Boot) |
| License | MIT |
| Stars | 10 |
| Contributors | 1 |
| Commits | 224 |
| Tags/Releases | 163 |
| Documentation | [julius-d.github.io/ueberboese-api](https://julius-d.github.io/ueberboese-api/) |

Reverse-engineers and rebuilds the Bose streaming HTTP API. Unique in publishing a machine-readable OpenAPI specification (`ueberboese-api.yaml`) and comprehensive request logging — making it the best research instrument for understanding what speakers actually call upstream. Implements Spotify OAuth integration and TuneIn. Companion to the Überböse App.

---

### 3. Überböse App
**[github.com/julius-d/ueberboese-app](https://github.com/julius-d/ueberboese-app)**
| | |
|---|---|
| Language | Flutter (Dart) |
| License | MIT |
| Latest version | 0.26.0 (March 2026) |
| Distribution | [F-Droid](https://f-droid.org/en/packages/io.github.juliusd.ueberboese.app/) |
| Platform | Android |

The only native installable phone app in the ecosystem. Pairs with the Überböse API server. Features: preset view/play/reprogram, multi-room zone management, volume control, now-playing display, Spotify authentication setup. Controls speakers directly via the local SoundTouch WebServices API (no server required for basic control).

---

### 4. SoundTouch Hybrid 2026
**[github.com/TJGigs/Bose-SoundTouch-Hybrid-2026](https://github.com/TJGigs/Bose-SoundTouch-Hybrid-2026)**
| | |
|---|---|
| Language | Node.js (JavaScript) |
| License | — |
| Stars | 1 |
| Commits | 3 (V1) / 12 (V3) |
| Status | Experimental / testing |

A self-hosted private cloud that emulates and replaces the Bose Cloud Service. Runs locally on a NAS or PC, intercepts the complex server handshakes needed to keep the SoundTouch infrastructure functional. Relies on **Music Assistant** for backend audio routing and provider aggregation. Features a setup wizard including USB config generation (`OverrideSdkPrivateCfg.xml`) and an on-screen Bose Cloud Emulation Setup guide. Targets users who want the broadest streaming provider support via Music Assistant's ecosystem.

---

### 5. OpenCloudTouch (OCT)
**[github.com/scheilch/opencloudtouch](https://github.com/scheilch/opencloudtouch)**
| | |
|---|---|
| Language | Python (FastAPI) + TypeScript (React) |
| License | Apache 2.0 |
| Stars | 9 |
| Commits | 313 |
| Latest release | v1.1.1 (April 12, 2026) |
| Documentation | GitHub Wiki (EN/DE) |

A single Docker container combining a FastAPI backend and React frontend. The most production-ready project in the ecosystem in terms of release discipline and deployment accessibility. Features: internet radio with full hardware preset support (buttons 1–6), responsive web UI, device discovery via SSDP/UPnP, multi-room zone management, BMX-compatible endpoints, TuneIn stream resolver, RadioBrowser as a built-in first-class search provider, and pre-built Raspberry Pi SD card images for Pi 3/4/5. Deployable on amd64, arm64, and arm/v7. Documented in English and German. Spotify and Music Assistant integration are on the roadmap.

---

### 6. AfterTouch
**[github.com/gesellix/Bose-SoundTouch](https://github.com/gesellix/Bose-SoundTouch)** by gesellix
| | |
|---|---|
| Language | Go |
| License | MIT |
| Stars | 16 |
| Contributors | 2 |
| Commits | 217 |
| Releases | 51 (latest: v0.28.0, Feb 15, 2026) |
| Documentation | [gesellix.github.io/Bose-SoundTouch](https://gesellix.github.io/Bose-SoundTouch/) |

The most comprehensive single toolkit in the ecosystem. Comprises three components: a Go library (importable package), a CLI (`soundtouch-cli`), and a local cloud emulation service (`soundtouch-service`). Covers the widest range of dimensions of any single project. Implements the complete Bose Spotify OAuth relay including surrogate secret generation and token refresh proxy. Includes a built-in DNS server for device redirection without SSH, HTTPS/custom CA injection, HTTP session recording, traffic proxy/logging, and a web management UI. Tested on real SoundTouch 10 and 20 hardware. Has a Patreon for ongoing support.

---

### 7. soundcork-stockholm-app
**[github.com/krahl/soundcork-stockholm-app](https://github.com/krahl/soundcork-stockholm-app)**
| | |
|---|---|
| Language | Java |
| License | — |
| Stars | 2 |
| Commits | 21 |
| Status | Active development, bugs expected |

A Java-based middleware that hosts the original Bose Stockholm frontend (extracted from the APK) in a local web browser at `http://127.0.0.1:8088/`. Bridges the Stockholm UI to local speakers via an HTTP proxy that resolves cross-origin issues, with SSDP-based device discovery and JSON state persistence. Unlike every other tool in the ecosystem, it runs the **official Bose UI** rather than a custom replacement — preserving the familiar Bose UX at the cost of requiring the Stockholm APK. Notable limitations: OAuth flows are unreliable, and WebSocket connections to speakers over HTTPS have blocking issues. Works alongside soundcork's backend for full cloud emulation.

---

### 8. jaas666/bose-soundtouch-web-api (Reference)
**[github.com/jaas666/bose-soundtouch-web-api](https://github.com/jaas666/bose-soundtouch-web-api)**

Community-maintained Markdown conversion of the official Bose SoundTouch Web API PDF (v1.0, January 7, 2026). Useful as a developer reference. Not a deployable tool.

---

## Feature Coverage Matrix

Legend: ● Yes/complete · ◑ Partial/planned · ○ No

| Dimension                                          | soundcork | Überböse API | Überböse App | ST Hybrid 2026 | OpenCloudTouch | AfterTouch | Stockholm App |
|----------------------------------------------------|:---------:|:------------:|:------------:|:--------------:|:--------------:|:----------:|:-------------:|
| **① App layer — local HTTP/WS control**            |           |              |              |                |                |            |               |
| Playback control (play/pause/vol)                  |     ○     |      ○       |      ●       |       ●        |       ●        |     ●      |       ●       |
| Preset view & trigger                              |     ○     |      ○       |      ●       |       ●        |       ●        |     ●      |       ●       |
| Preset write / reprogram                           |     ○     |      ○       |      ●       |       ●        |       ◑        |     ●      |       ●       |
| Multi-room zone management                         |     ○     |      ○       |      ●       |       ●        |       ●        |     ●      |       ●       |
| Now playing / status display                       |     ○     |      ○       |      ●       |       ●        |       ●        |     ●      |       ●       |
| Device discovery (SSDP/mDNS)                       |     ○     |      ○       |      ●       |       ○        |       ●        |     ●      |       ●       |
| WebSocket real-time events                         |     ○     |      ○       |      ◑       |       ●        |       ◑        |     ●      |       ◑       |
| **② Cloud/service layer — replaces Bose upstream** |           |              |              |                |                |            |               |
| Marge server emulation                             |     ●     |      ●       |      ○       |       ●        |       ○        |     ●      |       ○       |
| BMX / content registry                             |     ◑     |      ◑       |      ○       |       ●        |       ●        |     ●      |       ○       |
| Account / OAuth token relay                        |     ○     |      ●       |      ○       |       ◑        |       ○        |     ●      |       ◑       |
| Preset sync (cloud-side)                           |     ●     |      ●       |      ○       |       ●        |       ○        |     ●      |       ○       |
| Recents sync                                       |     ●     |      ◑       |      ○       |       ◑        |       ○        |     ●      |       ○       |
| Sources / device info persistence                  |     ●     |      ●       |      ○       |       ●        |       ○        |     ●      |       ○       |
| Stereo group CRUD (ST10 pairs)                     |     ●     |      ○       |      ○       |       ○        |       ○        |     ●      |       ○       |
| **③ Device redirection — USB/SSH setup**           |           |              |              |                |                |            |               |
| Setup wizard / guided redirect                     |     ◑     |      ◑       |      ○       |       ●        |       ●        |     ●      |       ○       |
| USB image / config generation                      |     ○     |      ○       |      ○       |       ●        |       ○        |     ◑      |       ○       |
| HTTPS / custom CA support                          |     ○     |      ○       |      ○       |       ○        |       ○        |     ●      |       ○       |
| **④ Streaming provider integration**               |           |              |              |                |                |            |               |
| Internet radio (RadioBrowser)                      |     ○     |      ◑       |      ◑       |       ◑        |       ●        |     ◑      |       ○       |
| TuneIn stream resolver                             |     ●     |      ●       |      ●       |       ●        |       ●        |     ●      |       ●       |
| Spotify OAuth / Connect                            |     ○     |      ●       |      ●       |       ◑        |       ◑        |     ●      |       ◑       |
| Pandora                                            |     ○     |      ○       |      ○       |       ○        |       ○        |     ●      |       ◑       |
| Music Assistant backend                            |     ○     |      ○       |      ○       |       ●        |       ◑        |     ○      |       ○       |
| **⑤ Mobile / native app**                          |           |              |              |                |                |            |               |
| Android app (installable)                          |     ○     |      ○       |      ●       |       ○        |       ○        |     ○      |       ○       |
| iOS app                                            |     ○     |      ○       |      ○       |       ○        |       ○        |     ○      |       ○       |
| Mobile-responsive web UI                           |     ○     |      ○       |      ○       |       ●        |       ●        |     ●      |       ●       |
| **⑥ Smart home / ecosystem integration**           |           |              |              |                |                |            |               |
| Home Assistant integration                         |     ○     |      ○       |      ○       |       ○        |       ○        |     ◑      |       ○       |
| Music Assistant integration                        |     ○     |      ○       |      ○       |       ●        |       ◑        |     ○      |       ○       |
| **⑦ CLI / automation tools**                       |           |              |              |                |                |            |               |
| CLI for scripting / automation                     |     ○     |      ○       |      ○       |       ○        |       ○        |     ●      |       ○       |
| Traffic proxy / API logging                        |     ◑     |      ●       |      ○       |       ○        |       ○        |     ●      |       ◑       |
| HTTP session recording                             |     ○     |      ○       |      ○       |       ○        |       ○        |     ●      |       ○       |
| **⑧ Library / SDK**                                |           |              |              |                |                |            |               |
| Importable library / package                       |     ○     |      ○       |      ○       |       ○        |       ○        |     ●      |       ○       |
| Published API spec / docs                          |     ○     |      ●       |      ○       |       ○        |       ○        |     ●      |       ○       |
| Docker deployment                                  |     ●     |      ●       |      ○       |       ●        |       ●        |     ●      |       ●       |
| Raspberry Pi SD card image                         |     ○     |      ○       |      ○       |       ○        |       ●        |     ○      |       ○       |

---

## Making AfterTouch the One-Stop Solution — Open Tasks

AfterTouch is the strongest single project across the service and developer layers. Its remaining gaps are on the consumer-facing and ecosystem-integration sides.

### Priority 1 — PWA installability

The web UI is already fully responsive — it has Bootstrap grid columns, `@media (max-width: 768px)` and `@media (max-width: 576px)` breakpoints, and a proper viewport meta tag. It works on iPhone and Android browsers today. What's missing is **installability**: no `manifest.json` and no service worker, so it cannot be added to the home screen as a standalone app. Adding these would close the iOS app gap ecosystem-wide (no project has an iOS app) at minimal effort.

### Priority 2 — RadioBrowser as a first-class provider

AfterTouch can proxy and play any stream URL, but there is no built-in station search. OpenCloudTouch's RadioBrowser integration is the reference. Tasks:
- Wire the [RadioBrowser API](https://www.radio-browser.info/) into the `soundtouch-web` web UI as a browsable/searchable source.
- Make discovered stations directly presetable to hardware buttons.
- This is the most common replacement for TuneIn for users who listened to internet radio via presets.

### Priority 3 — Raspberry Pi SD card image

OpenCloudTouch ships a flashable Pi image and it dramatically lowers the barrier for the most common "always-on local server" deployment. AfterTouch already has Docker and a web management UI; this is largely a CI/packaging task:
- Build a Pi image (using e.g. `pi-gen` or `rpi-imager`-compatible tooling) that boots directly into `soundtouch-service`.
- Auto-starts on boot, auto-discovers devices, opens the web UI on a known port.
- Target Pi 3/4/5 with amd64/arm64/arm/v7 variants (mirroring OCT's approach).

### Priority 4 — USB config generation in the web UI

AfterTouch modifies `SoundTouchSdkPrivateCfg.xml` via SSH (`pkg/service/setup/setup.go`) and documents the redirect process thoroughly, but does not yet generate the USB stick content for users without SSH access. A "prepare USB stick" button in the web UI would remove the last manual step:
- Generate `OverrideSdkPrivateCfg.xml` pre-populated with the running server's URL.
- Optionally include the custom CA certificate for HTTPS-capable devices.
- Surface alongside the existing guided migration wizard.

### Priority 5 — Music Assistant integration

SoundTouch Hybrid 2026 uses Music Assistant as its streaming backend, giving access to Apple Music, Deezer, local libraries, and many other providers. A formal Music Assistant **player provider** for AfterTouch would give power users a path to sources beyond Spotify, TuneIn, Pandora, and RadioBrowser. The Music Assistant community has an open discussion thread on this ([#4766](https://github.com/orgs/music-assistant/discussions/4766)).

### Priority 6 — DNS-based migration documentation

AfterTouch includes a built-in DNS server (`ENABLE_DNS_DISCOVERY`, `DNS_BIND_ADDR`, `DNS_UPSTREAM`) that intercepts `*.bose.com` queries and forwards everything else upstream — no Pi-hole, AdGuard, or any other external tool required. The ResolvConf migration path already treats DNS as a first-class option. The remaining gap is awareness: users unfamiliar with the project may not realise no external DNS infrastructure is needed. Tasks:
- Surface the built-in DNS server more prominently in the getting-started documentation.
- Document Pi-hole / AdGuard Home as an *alternative* for users who already run those, not a requirement.

### Priority 7 — MQTT integration

A design document exists (`docs/guides/MQTT-INTEGRATION-DESIGN.md`) but no code has been written. Implementing it would unlock home automation use cases without requiring the full Home Assistant stack — enabling triggers like "play preset 1 when front door opens" via any MQTT-capable automation platform.

---

## soundcork ↔ AfterTouch

soundcork and AfterTouch share the most functional overlap of any two projects in the ecosystem. For the implementation-level parity analysis and remaining tasks see [docs/PARITY-SOUNDCORK.md](../appendix/PARITY-SOUNDCORK.md).

### Architectural differences (not gaps)

These exist in soundcork but are deliberate architectural choices in AfterTouch, not missing features:

| Area                     | soundcork                             | AfterTouch                                                |
|--------------------------|---------------------------------------|-----------------------------------------------------------|
| Web UI                   | FastAPI + Jinja2 miniapp and admin UI | Separate `soundtouch-web` component (Go + plain HTML/JS)  |
| Direct device management | SSH/SCP access into speakers          | HTTP API only; no SSH                                     |
| Device discovery client  | Python `upnpclient` library           | mDNS + UPnP in Go, with dedicated DNS interception server |
| Token delivery           | Push (ZeroConf priming to port 8200)  | Pull (device calls back to fetch)                         |
| Persistence format       | Flat files                            | XML flat files + atomic writes                            |

### AfterTouch capabilities soundcork lacks

| Feature                                   | Notes                                                           |
|-------------------------------------------|-----------------------------------------------------------------|
| DNS server for device redirect            | Intercepts Bose domain queries; no Pi-hole required             |
| HTTPS / custom CA injection               | Full TLS with certificate generation and trust workflow         |
| HTTP interaction recording & replay       | Captures real device traffic for debugging and regression tests |
| Device migration (serial → MAC path)      | Handles legacy device ID formats automatically                  |
| Transparent proxy mode with upstream sync | Can mirror to real Bose cloud while running locally             |
| CLI (`soundtouch-cli`)                    | Scriptable control of speakers                                  |
| Importable Go library                     | `github.com/gesellix/bose-soundtouch/pkg/client`                |

---



### Ecosystem fragmentation vs. convergence

The community is currently covering different parts of the problem in parallel rather than converging. AfterTouch explicitly credits soundcork, Überböse, and SoundTouch Plus in its README and describes its `soundtouch-service` as "heavily inspired by SoundCork". There is an opportunity — and arguably a need — for these projects to formally coordinate: shared test fixtures, a common compatibility matrix against specific firmware versions, and agreed-on API contracts would all reduce duplicated effort.

### Firmware version sensitivity

The SoundTouch 10 is most dependent on Marge for basic network functionality; the 20 and 30 are somewhat more tolerant. Compatibility across firmware versions is not systematically documented anywhere. A community firmware compatibility matrix (model × firmware version × which emulation features work) would be high value and is currently missing.

### Security posture

All projects warn that speakers should only be used on a private, firewalled network after cloud shutdown. AfterTouch is the only project to implement HTTPS/custom CA, which matters if devices are ever on a network where traffic could be inspected. soundcork's SECURITY.md explicitly warns against running on open networks.

### No iOS app — a structural gap

The original SoundTouch app was iOS-first. Every community replacement is Android-only (Überböse App) or browser-based. This is the largest unaddressed user segment in the ecosystem.

### Bose's open-source move as a precedent

Bose's decision to release API documentation rather than simply shutting down is notable — it mirrors what Pebble users did themselves with Rebble after that shutdown, but here the manufacturer initiated it. This sets a useful precedent and gives the community a solid legal and technical foundation to build on.

### Related community resources

- [Bose SoundTouch Plus (Home Assistant component)](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus) — comprehensive HA integration by Todd Lucas, extensive API wiki
- [Bose SoundTouch Hook](https://github.com/CodeFinder2/bose-soundtouch-hook) — `LD_PRELOAD`-based reverse engineering framework used by AfterTouch for protocol research
- [Bose SoundTouch Web API (community Markdown)](https://github.com/jaas666/bose-soundtouch-web-api) — official API PDF converted to Markdown
- [Bose Wiki — SoundTouch App Alternatives](https://bose.fandom.com/wiki/SoundTouch_app_alternatives) — community-maintained living list of workarounds and projects
- [Reddit megathread — Bose alternatives](https://www.reddit.com/r/bose) — ongoing community discussion
- [Radio Browser](https://www.radio-browser.info/) — the free, community-maintained internet radio directory used as a TuneIn replacement

---

*Document compiled April 2026. Project details sourced directly from GitHub repositories and official documentation. Star counts, commit counts, and release dates reflect the state at time of writing and will change as projects evolve. soundcork-stockholm-app added April 2026.*
