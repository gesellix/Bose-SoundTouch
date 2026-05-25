---
title: "Stockholm Backend — Port Guide for Bose-SoundTouch (Go)"
sidebar:
  exclude: true
---
This document describes everything needed to integrate the
[krahl/soundcork-stockholm-app](https://github.com/krahl/soundcork-stockholm-app)
functionality into the Go service. It is written as a reference; nothing here
implies a specific file layout or package structure.

---

## Table of Contents

1. [What needs porting](#1-what-needs-porting)
2. [What becomes obsolete](#2-what-becomes-obsolete)
3. [Startup: Stockholm frontend preparation](#3-startup-stockholm-frontend-preparation)
4. [Native bridge — appSend / runQueue](#4-native-bridge--appsend--runqueue)
5. [State persistence — native-state.json](#5-state-persistence--native-statejson)
6. [HTTP proxy — /api/http-proxy](#6-http-proxy--apihttp-proxy)
7. [Browser bootstrap injection](#7-browser-bootstrap-injection)
8. [SSDP discovery](#8-ssdp-discovery)
9. [Config file structure (stockholm/json/config.json)](#9-config-file-structure-stockholmjsonconfigjson)
10. [Backend config (backend-config.json)](#10-backend-config-backend-configjson)
11. [Running as a plain process (no Docker)](#11-running-as-a-plain-process-no-docker)

---

## 1. What needs porting

| Component                                     | Java class / file                     | Notes                                                |
|-----------------------------------------------|---------------------------------------|------------------------------------------------------|
| Stockholm zip extraction + patch application  | `docker-entrypoint.sh`                | Shell; can be Go at startup                          |
| URL rewriting in `stockholm/json/config.json` | `update-urls.sh`                      | Shell + `jq`/`sed`; can be Go                        |
| Native bridge                                 | `NativeBridgeService`                 | Core; per-tab message queue                          |
| State persistence                             | `NativeBridgeService` (file I/O)      | JSON file read/written on every `setData`            |
| HTTP proxy                                    | `HttpProxyService`                    | CORS proxy + cloud header injection                  |
| Browser bootstrap injection                   | `BackendApplication` (static handler) | Injects `<script>` into `index.html`                 |
| SSDP speaker + media-server discovery         | `SsdpDiscoveryService`                | Already partially in Bose-SoundTouch                 |
| Config reading                                | `SoundcorkDataService`                | Reads `config.json` + `override.json`                |
| Backend config                                | `BackendConfig`                       | Single JSON file, only `frontendLoggingLevel` so far |

---

## 2. What becomes obsolete

When Bose-SoundTouch serves Stockholm directly, the following env vars and
concepts collapse because the Go service knows its own URLs:

| Variable                           | Why it disappears                          |
|------------------------------------|--------------------------------------------|
| `BACKEND_URL`                      | Go service knows its own base URL          |
| `STREAMING_URL`                    | Same — the marge path is internal          |
| `AUTH_SERVICE_URL`                 | Same — marge is a local handler            |
| `BACKEND_BIND_IP` / `BACKEND_PORT` | Replaced by existing `PORT` / `HTTPS_PORT` |
| `update-urls.sh`                   | Config rewriting becomes Go startup logic  |
| Custom CA cert via `keytool`       | Replaced by Bose-SoundTouch `certmanager`  |

What does **not** disappear:

- `MARGE_AUTH_TOKEN` / `MARGE_ACCOUNT_ID` — seeding initial session state
- Stockholm zip + versioned patch files — still needed as assets
- `PREFERRED_DEVICES` and other existing Bose-SoundTouch config

---

## 3. Startup: Stockholm frontend preparation

### 3a. Zip extraction

Source file: `docker-entrypoint.sh:prepare_stockholm()`

Look for `stockholm/index.html`. If absent:

1. Find the zip in `stockholm_zip/stockholm.zip` (preferred) or `stockholm.zip`
   alongside the binary.
2. Extract the zip into `stockholm/`.

### 3b. Versioned patch application

Patch files are named `stockholm-changes_v<N>.patch` and applied in ascending
order. The set is scanned dynamically at preparation time; today the upstream
project ships v1 (1 153 lines), v2 (1 475 lines), v3 (44 lines, `now_play.js`
fix), and v4 (68 lines, `app_comm.js` clientId polish). New versions added
upstream are picked up automatically — our code does not hardcode a list.

A marker file `stockholm/.soundcork-stockholm-app.json` tracks the last applied
version:
```json
{"project":"soundcork-stockholm-app","patchVersion":2}
```

Algorithm:

1. Read `patchVersion` from the marker (default 0).
2. For each `stockholm-changes_v<N>.patch` with N > current version, in order:
   - Strip hunks that don't touch `stockholm/` paths (the patch files include
     README and self-referential hunks).
   - Dry-run `patch -p1 -R` (reverse) to test if it's already applied.
   - Dry-run `patch -p1` (forward) to test if it can apply.
   - Apply with `patch -p1 --batch`.
   - Write the marker for version N.
3. For v1 only, run `prettier --write "stockholm/**/*.js"` before patching
   (the patch was generated against formatted source).

The `patch` and `prettier` (npm) binaries are required. In a container image
these are install-time dependencies. For a plain binary distribution they must
be present on the host.

### 3c. Copy update-urls.sh into place

Copy `update-urls.sh` to `stockholm/json/update-urls.sh` after extraction.
The script is called from that directory so relative paths work.

### 3d. Rewrite config.json URLs (replaces update-urls.sh)

`stockholm/json/config.json` stores most values base64-encoded under a
`"default"` key (`d0`…`d13`). `update-urls.sh` decodes, rewrites with `sed`,
and re-encodes.

When the Go service knows its own URLs at startup, it can do this in-process:

```
fields to rewrite (sed substitutions in the shell script):
  streaming.bose.com  → STREAMING_URL  (default: BACKEND_URL,  soundcork: BACKEND_URL/marge)
  events.api.bosecm.com → BACKEND_URL
  content.api.bose.io   → BACKEND_URL
  worldwide.bose.com    → BACKEND_URL
  downloads.bose.com    → BACKEND_URL
  d6 field              → AUTH_SERVICE_URL  (set via jq, not sed)
```

The Go equivalent:
1. Read `config.json`, base64-decode each value in `default`.
2. Replace the hostnames above.
3. Set `default.d6` to the auth service URL.
4. Re-encode all values in `default` as base64.
5. Write back.

---

## 4. Native bridge — appSend / runQueue

Source: `NativeBridgeService.java`

Stockholm communicates with the native layer through two HTTP endpoints. The
bridge emulates the Android `Native` object.

### Endpoints

```
POST /api/native/appSend?clientId=<id>   (or X-Stockholm-Client-Id header)
GET  /api/native/runQueue?clientId=<id>
```

`clientId` is a per-browser-tab identifier. Falls back to `"default"`.

### appSend request body

JSON:
```json
{"method":"<name>","params":{...},"id":<number or null>}
```

### runQueue response body

```json
{"messages": [<message>, ...] | null}
```

Each message is one of:

**Callback result** (response to a `getData`, `getConstant`, etc.):
```json
{"result":<value>,"error":<value or null>,"id":<id from request>}
```

**Push method** (unsolicited, e.g. device discovery results):
```json
{"method":"devices","params":[...],"id":null}
```

### Supported methods

| Method                                                                                                                                                        | Action                                                                                          |
|---------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------|
| `locale`, `htmlReady`, `stopHrmsUpdates`                                                                                                                      | No-op                                                                                           |
| `log`                                                                                                                                                         | Log `params.msg` at debug level                                                                 |
| `setData`                                                                                                                                                     | Store `params.name` → `params.value` in state; persist to disk                                  |
| `getData`                                                                                                                                                     | Return state value for `params.name`; empty string if absent                                    |
| `getLanStatus`                                                                                                                                                | Return `{"result":true,"error":null,"id":<id>}`                                                 |
| `getTimeZone`                                                                                                                                                 | Return `{"result":{"timezoneInfo":"<IANA zone>","timeFormat":"TIME_FORMAT_24HOUR_ID"}}`         |
| `getLegalDocPath`                                                                                                                                             | Return path string (see below)                                                                  |
| `getConstant`                                                                                                                                                 | Return `state["constant.<name>"]`; default for `"kilo"` is `"a7928d7b43dcd49f0af31e5aeed26458"` |
| `canPerformAutoAPSetup`                                                                                                                                       | Return `{"result":{"permission":false,"location":false}}`                                       |
| `getDeviceList`                                                                                                                                               | Run SSDP renderer discovery async; push incremental `"devices"` messages                        |
| `getHrmsList`                                                                                                                                                 | Run SSDP server discovery async; push `"servers"` message                                       |
| `getNetStats`, `getSSIDList`, `setSSID`, `updateSetting`, `oauth`, `downloadNewGui`, `installNewGui`, `sendLogs`, `socketCreate`, `socketSend`, `socketClose` | Return error `"unsupported"`                                                                    |

**getLegalDocPath logic:**
```
type=lcns               → "legal/platform_license.txt"
type=<blank>            → "legal/eula_en.txt"
type=<type>,lang=<lang> → "legal/<type>_<lang>.txt"   (lang defaults to "en")
```

### Async discovery pattern

`getDeviceList` and `getHrmsList` are fired asynchronously. Each discovered
device is pushed to the client queue immediately via `"devices"` / `"servers"`
method messages before the discovery is complete. The frontend polls
`/api/native/runQueue` continuously, so results arrive as they come in.

### Queue structure

One deque per `clientId`. `appSend` appends; `runQueue` drains the whole deque
atomically and returns all pending messages.

### State seeding from environment

On startup, read these env vars and write to state if present:

| Env var                                | State key        |
|----------------------------------------|------------------|
| `MARGE_AUTH_TOKEN` or `margeAuthToken` | `margeAuthToken` |
| `MARGE_ACCOUNT_ID` or `margeAccountID` | `margeAccountID` |

Also seed on first run:

| State key            | Value                                                           |
|----------------------|-----------------------------------------------------------------|
| `guid`               | Random UUID (hex, no dashes)                                    |
| `deviceGuid`         | Same UUID as `guid`                                             |
| `nativeFrameVersion` | Short version prefix extracted from `bose_app` in `config.json` |
| `frame_version`      | Full version from `bose_app`                                    |
| `authServer`         | `"0"`                                                           |
| `constant.kilo`      | `"a7928d7b43dcd49f0af31e5aeed26458"`                            |

---

## 5. State persistence — native-state.json

Source: `NativeBridgeService.loadState()` / `persistState()`

File path (relative to workspace root): `backend/state/native-state.json`

Format: flat JSON object, all values are strings.

```json
{
  "guid": "abc123...",
  "deviceGuid": "abc123...",
  "frame_version": "27.0.13",
  "nativeFrameVersion": "27.0.13",
  "authServer": "0",
  "margeAuthToken": "<token>",
  "margeAccountID": "1234567",
  "overrideMargeURL": "https://...",
  "overrideUpdateURL": "https://...",
  "constant.kilo": "a7928d7b43dcd49f0af31e5aeed26458",
  ... (arbitrary keys from setData calls)
}
```

Written on every `setData` call and on initial seeding. Read once at startup.

---

## 6. HTTP proxy — /api/http-proxy

Source: `HttpProxyService.java`

Stockholm makes all cloud API calls through this proxy to work around browser
CORS restrictions.

### Endpoint

```
<ANY METHOD> /api/http-proxy?url=<url-encoded target URL>
```

### Header filtering

**Blocked outbound (not forwarded to target):**
```
access-control-request-headers, access-control-request-method, connection,
content-length, cookie, forwarded, host, http2-settings, keep-alive, origin,
proxy-authenticate, proxy-authorization, referer, sec-ch-ua, sec-ch-ua-mobile,
sec-ch-ua-platform, sec-fetch-dest, sec-fetch-mode, sec-fetch-site,
sec-fetch-user, te, trailer, transfer-encoding, upgrade, x-forwarded-for,
x-forwarded-host, x-forwarded-port, x-forwarded-proto, x-real-ip,
x-requested-with
```

**Blocked inbound (not relayed to browser):**
```
access-control-allow-credentials, access-control-allow-headers,
access-control-allow-methods, access-control-allow-origin,
access-control-expose-headers, access-control-max-age, connection,
content-length, keep-alive, proxy-authenticate, proxy-authorization,
set-cookie, set-cookie2, te, trailer, transfer-encoding, upgrade
```

Also block HTTP/2 pseudo-headers (names starting with `:`).

Always add `Cache-Control: no-store` to the response.

### Backend-injected headers

Injected only if not already present in the request.

**BMX targets** (host is `content.api.bose.io`, `*.apigee.net`,
`bose-prod.apigee.net`, `test.content.api.bose.io`):
```
x-bmx-api-key: <encryptedBmxToken from config.json d7>
x-software-version: <bose_app version>
```

**Marge targets** (host ends with `.bose.com` or `.apigee.net` AND path
contains `/streaming/` or `/customer/`):
```
Accept:                       application/vnd.bose.streaming-v<N>+xml
                              (or customer variant if path contains /customer/)
Content-Type:                 same as Accept
ClientType:                   SOUNDTOUCH_COMPUTER_APP
GUID:                         <guid from state>
version_NativeFrameVersion:   <nativeFrameVersion from state>
version_StockholmVersion:     <bose_app version>
version_ProtocolVersion:      <bose_protocol version>
<margeServerKeyHeader>:       <margeServerKey>   (if config d13/d10 non-empty)
Authorization:                <margeAuthToken>   (not injected on login/environment endpoints)
```

Authorization is **not** injected for these paths:
- `*/streaming/account/login`
- `/streaming/account` or `/streaming/account/`
- `*/streaming/account/email/*/environment`
- `/customer/account/password/email/*`

### Login retry (environment switching)

After a login `POST` to `*/streaming/account/login`:

1. If the response XML contains `<status-code>4033</status-code>` (wrong
   region), parse the login request body for `<username>` and `<password>`.
2. Fetch `GET <same-origin><marge-prefix>/streaming/account/email/<email>/environment`
   with `Authorization: Basic <base64(email:password)>`.
3. Parse the environment response XML for `<streamingURL>` and `<updateURL>`.
4. Store both as `overrideMargeURL` / `overrideUpdateURL` in state.
5. Retry the original login against the new `streamingURL`.

Subsequent marge requests are automatically redirected to `overrideMargeURL`
via `SoundcorkDataService.overrideTarget()`.

### Session capture

After a successful login response (2xx):
- Extract `<account id="...">` from the response XML body → store as `margeAccountID`.
- Extract `Credentials` response header → store as `margeAuthToken`.

On any marge response:
- If there is a `Refresh` response header, store its value as `margeAuthToken`.

### Proxy loop detection

Reject requests whose target URL resolves to the proxy's own
`/api/http-proxy` endpoint. Considers both the direct bind address and the
externally visible address from `X-Forwarded-Host` / `X-Forwarded-Port` /
`Host` headers.

### Header value sanitisation

Drop header values that are `null`, `undefined`, or empty string (these can
come from the Stockholm JS).

---

## 7. Browser bootstrap injection

Source: `BackendApplication.StaticStockholmHandler`

On every request to `index.html` or `setup/index.html`, inject a `<script>`
block before `</head>`. The script is skipped if `window.StockholmBrowserBootstrap`
already exists.

The injected JSON payload:
```json
{
  "authServer": "<0–3, from state>",
  "guid": "<guid from state>",
  "nativeVersion": "<frame_version from state>",
  "frameConfig": {}
}
```

The script does four things:
1. Patches `window.getURLParams` to return `bootstrap.authServer`, `bootstrap.guid`,
   and `bootstrap.nativeVersion` for the keys `authServer`, `guid`, and
   `native_version` when the original function returns null.
2. Patches `window.getUserAgentValue` to return `bootstrap.guid` for `_app`
   when the original returns empty.
3. Sets `window.guid`, `window.frame_version`, `window.auth_server` from
   bootstrap values when they are empty.
4. Patches `window.settingsLoad` to merge `bootstrap.frameConfig` into the
   config object (keys `f<N>` → `d<N>`, base64-encoded, only if currently
   empty).

`authServer` is an integer string `"0"`–`"3"`. The Java code normalises to
`"0"` for any invalid value.

### Static file serving

Serve everything under `stockholm/` for all paths. Content types:

| Extension      | MIME type                               |
|----------------|-----------------------------------------|
| `.html`        | `text/html; charset=UTF-8`              |
| `.js`          | `application/javascript; charset=UTF-8` |
| `.css`         | `text/css; charset=UTF-8`               |
| `.json`        | `application/json; charset=UTF-8`       |
| `.xml`         | `application/xml; charset=UTF-8`        |
| `.svg`         | `image/svg+xml`                         |
| `.png`         | `image/png`                             |
| `.jpg`/`.jpeg` | `image/jpeg`                            |
| `.gif`         | `image/gif`                             |
| `.ttf`         | `font/ttf`                              |
| `.otf`         | `font/otf`                              |
| `.txt`         | `text/plain; charset=UTF-8`             |

Set `Cache-Control: no-store` on all responses.

For `HEAD` requests send headers only (no body, status -1 in content-length).
For 204/304 responses send no body.

Path traversal: reject any path that resolves outside `stockholm/`.

### Frontend logging cookie

Set a `Set-Cookie` header on every static response:
- If `frontendLoggingLevel > 0`:
  `stockholmFrontendLoggingLevel=<level>; Path=/; SameSite=Lax`
- Otherwise (clear it):
  `stockholmFrontendLoggingLevel=; Max-Age=0; Path=/; SameSite=Lax`

---

## 8. SSDP discovery

Source: `SsdpDiscoveryService.java`

Bose-SoundTouch already has SSDP/UPnP discovery in `pkg/discovery`. The
Stockholm bridge needs two specific discovery types with specific result shapes.

### Renderer discovery (speakers) — `getDeviceList`

Search target: `urn:schemas-upnp-org:device:MediaRenderer:1`

For each SSDP response, extract the `Location` header URL, take the `host`
part, then fetch `GET http://<host>:8090/info`.

Parse the XML response:
```xml
<info deviceID="AA:BB:CC:DD:EE:FF">
  ...
  <margeAccountUUID>1234567</margeAccountUUID>
  ...
</info>
```

- `deviceID` attribute → `uID` (uppercased)
- `margeAccountUUID` element text → `accountId`

Filter: if `margeAccountID` is set in state, only include speakers whose
`margeAccountUUID` matches.

Result payload per speaker:
```json
{"uID": "AA:BB:CC:DD:EE:FF", "ip": "192.0.2.10"}
```

Push incremental results as they arrive (push one device at a time via the
`"devices"` method message). At the end, if the list is empty, push an empty
`"devices"` message.

### Network interface selection for SSDP

Priority order: ethernet/en* > wifi/wl* > others.

Exclude: loopback, virtual, docker, vbox, vmware, hyper-v, bluetooth, teredo,
tunnel interfaces.
Require: IPv4 address, multicast support, interface up.

Try each interface in priority order; return results from the first one that
gets responses.

SSDP probe parameters:
- Multicast: `239.255.255.250:1900`
- 3 probes, 350 ms between probes
- 1 250 ms grace period after last probe
- `MX: 1`

### Media server discovery (HRMS) — `getHrmsList`

Search target: `urn:schemas-upnp-org:device:MediaServer:1`

No HTTP fetch needed — extract from SSDP response headers only.

Result payload per server:
```json
{"uID": "<usn uuid or host:port>", "ip": "<host>", "port": "<port>"}
```

`uID` is the UUID portion of the `USN` header (strip `uuid:` prefix and
anything after `::`). Fall back to `host:port` if USN is absent.

Push all results at once (no incremental push) via `"servers"` method message.

---

## 9. Config file structure (stockholm/json/config.json)

Source: `SoundcorkDataService.java`

The file has three top-level objects: `app_versions`, `api_versions`, `default`.

### app_versions

| Key             | Used as                                                                       |
|-----------------|-------------------------------------------------------------------------------|
| `bose_app`      | `soundcorkAppVersion` — also `x-software-version`, `version_StockholmVersion` |
| `bose_protocol` | `protocolVersion` — sent as `version_ProtocolVersion`                         |

### api_versions

| Key              | Used as                                                                  |
|------------------|--------------------------------------------------------------------------|
| `bose_streaming` | Streaming API version — builds `application/vnd.bose.streaming-v<N>+xml` |
| `bose_customer`  | Customer API version — builds `application/vnd.bose.customer-v<N>+xml`   |

### default (all values base64-encoded)

| Field | Content                      | Used as                                                      |
|-------|------------------------------|--------------------------------------------------------------|
| `d0`  | marge base URL               | `defaultMargeUrl` (redirected from `streaming.bose.com`)     |
| `d1`  | update base URL              | `defaultUpdateUrl` (redirected from `events.api.bosecm.com`) |
| `d3`  | BMX registry URL             | `defaultBmxRegistryUrl`                                      |
| `d6`  | auth service URL             | Written by `update-urls.sh` / `AUTH_SERVICE_URL`             |
| `d7`  | BMX API token                | `encryptedBmxToken` — injected as `x-bmx-api-key`            |
| `d8`  | BMX server alt URL           | stored but not currently used in header injection            |
| `d10` | marge server key             | injected as `<margeServerKeyHeader>` value on marge requests |
| `d13` | marge server key header name | the header name for d10                                      |

### override.json

Sits alongside `config.json` at `stockholm/json/override.json`. Currently only
`kilo` is read from it (not used in any live code path yet).

---

## 10. Backend config (backend-config.json)

Source: `BackendConfig.java`

File path: `backend/config/backend-config.json`

```json
{"frontendLoggingLevel": 2}
```

`frontendLoggingLevel`:
- `0` — disable frontend debug logging (clear the cookie)
- `> 0` — enable at that level (set cookie to the numeric value)

The Stockholm JS reads `stockholmFrontendLoggingLevel` from a cookie on load.

---

## 11. Running as a plain process (no Docker)

The entrypoint script does three things beyond launching the JVM. For a plain
process, do these steps once before running the binary:

### Step 1 — extract and patch Stockholm

```shell
# Requires: unzip, patch, npm/prettier@3.8.3
unzip stockholm_zip/stockholm.zip -d stockholm
npx prettier@3.8.3 --ignore-path /dev/null --write "stockholm/**/*.js"
# Apply every stockholm-changes_v<N>.patch that exists, in ascending order.
# Today the upstream ships v1..v4; new ones get picked up automatically when
# the upstream repo is re-cloned via `make build-stockholm-image`.
for patch in stockholm-changes_v*.patch; do
    patch -p1 --batch < "$patch"
done
```

Or run the Docker container once and copy the `stockholm/` directory out.

### Step 2 — rewrite URLs in config.json

```shell
cd stockholm/json
BACKEND_URL=http://localhost:8000 \
STREAMING_URL=http://localhost:8000/marge \   # soundcork only
AUTH_SERVICE_URL=http://localhost:8000/marge/ \
source update-urls.sh
cd ../..
```

For Bose-SoundTouch, this step disappears — the Go service rewrites config.json
in-process at startup.

### Step 3 — create state directory

```shell
mkdir -p backend/state
```

### Step 4 — run

```shell
# Java (current):
./gradlew run

# Go (future):
./soundtouch-service   # with appropriate env vars
```

The Java `resolveWorkspaceRoot()` searches for a `stockholm/` directory at CWD
or one level up. Run from the project root.

---

## Patches summary — what the Stockholm JS patches do

**v1** (the main patch, applied after prettier formatting):

- `stockholm/index.html` — adds `<meta>` charset and viewport tags
- `stockholm/js/app_comm.js` — rewrites `AppComm` to use the HTTP native bridge
  (`/api/native/appSend` + `/api/native/runQueue`) instead of Android native calls
- `stockholm/js/browser_http_proxy.js` — **new file** — implements the
  `stHttpProxy` function that routes all cloud API calls through `/api/http-proxy`
- `stockholm/js/browser_native_bridge.js` — **new file** — implements
  `window.Native` shim that calls the bridge endpoints
- `stockholm/js/main.js` — wires up the browser native bridge on load
- `stockholm/setup/index.html` — same charset/viewport fix
- `stockholm/setup/js/app_comm.js` — same AppComm bridge rewrite for the setup flow

**v2** (incremental fixes on top of v1):

- `stockholm/js/app_comm.js` — additional fixes and multi-tab `clientId` support
- `stockholm/js/browser_native_bridge.js` — minor fix
- `stockholm/js/main.js` — minor fix
- `stockholm/js/marge_comm.js` — fixes marge URL handling
- `stockholm/js/presets.js` — minor fix
- `stockholm/js/sources.js` — minor fix
- `stockholm/setup/js/app_comm.js` — same fixes as main app_comm.js

**v3** (44 lines):

- `stockholm/js/now_play.js` — small playback-state guard.

**v4** (68 lines):

- `stockholm/js/app_comm.js` — further `clientId` handling polish (localStorage persistence).
