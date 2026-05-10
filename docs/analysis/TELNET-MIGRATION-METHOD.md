# Telnet (Port 17000) Migration Method — Analysis

This document captures the use cases, community findings, and feasibility analysis
for adding a **Telnet/port 17000** migration path to `soundtouch-service` as a
peer of the existing XML and DNS-based methods. The `/etc/hosts` method stays
deprecated and is intentionally kept off the visible UI options.

> **Sources** — community discussion synthesised from
> [gesellix/Bose-SoundTouch#221](https://github.com/gesellix/Bose-SoundTouch/issues/221),
> [gesellix/Bose-SoundTouch#236](https://github.com/gesellix/Bose-SoundTouch/issues/236),
> [scheilch/opencloudtouch#167](https://github.com/scheilch/opencloudtouch/issues/167),
> [deborahgu/soundcork#228](https://github.com/deborahgu/soundcork/issues/228),
> [deborahgu/soundcork#141](https://github.com/deborahgu/soundcork/issues/141),
> the post-EOS walkthrough PDF in `docs/`,
> [Bose SoundTouch Telnet Probing thread](https://www.reddit.com/r/bose/comments/1o5zkym/soundtouch_telnet_probing/),
> and [flarn2006's blog post on hacking SoundTouch](https://flarn2006.blogspot.com/2014/09/hacking-bose-soundtouch-and-its-linux.html).

---

## 1. Why a third method is needed

The two currently shipped methods both have hard preconditions that block real
users:

| Method                                  | Preconditions                                                | Failure modes seen in the wild                                                                                                                                                         |
|-----------------------------------------|--------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **XML** (`SoundTouchSdkPrivateCfg.xml`) | SSH/root access — needs `remote_services` USB unlock first   | Some firmware revisions (e.g. SA-5, ST520, latest ST Portable) refuse the USB unlock entirely; `remote_services on` was removed from the telnet command set in firmware 7.x and later. |
| **DNS** (`resolv.conf` priority hook)   | SSH/root access; service must own port 53 on the LAN gateway | Won't fit users behind ISP routers they can't reconfigure; still requires the device to be SSH-reachable to write the hook.                                                            |

The community has demonstrated a **third path that needs no SSH at all**:
the device's built-in **diagnostic Telnet shell on TCP port 17000** accepts
configuration commands that change exactly the same fields the XML method would.

### 1.1 Confirmed user reports (firmware 27.0.6.46330.5043500 unless noted)

| Reporter           | Hardware                  | Outcome                                                                                                     |
|--------------------|---------------------------|-------------------------------------------------------------------------------------------------------------|
| `foob61451` (#221) | ST 10, ST 20 (non-rooted) | All four URLs persisted via `sys configuration …`; `envswitch boseurls set …` survived `sys reboot`.        |
| `bveenker` (#221)  | Wave III                  | URLs accepted; presets work after pairing via `/setMargeAccount` (see §3).                                  |
| `stephan48` (#221) | Wave IV                   | Telnet:1700 + USB stick `remote_services` did **not** work; **port 17000 telnet** worked for all four URLs. |
| `mcdona1d` (#141)  | ST 20, ST 300             | Confirmed working with `sys configuration …` + `envswitch …` + `sys reboot`.                                |
| `TJGigs` (#228)    | ST 20 ×2, ST 10           | Wraps telnet:17000 into an admin "Smart Inject" tool; uses `sys reboot` over telnet to nudge devices.       |

So the method is plausible across **at least ST 10/20/300 and Wave III/IV** on
the most common firmware that survived the EOS cut, **without the USB unlock
dance** that newer firmware refuses.

---

## 2. The Telnet:17000 command set we rely on

### 2.1 URL configuration (the migration payload)

The sequence we send for `soundtouch-service` (community-validated in #221, #141):

```
sys configuration bmxRegistryUrl http://<service-host>:8000/bmx/registry/v1/services
sys configuration statsServerUrl http://<service-host>:8000
sys configuration margeServerUrl http://<service-host>:8000
sys configuration swUpdateUrl    http://<service-host>:8000/updates/soundtouch
envswitch boseurls set http://<service-host>:8000 http://<service-host>:8000/updates/soundtouch
getpdo CurrentSystemConfiguration
```

`sys reboot` is **not** part of this sequence. The migration flow only writes
configuration — the reboot is user-initiated via the existing reboot button in
the web UI, mirroring what XML/DNS migration already does. See §6.2 for how
that button gains a `?method=ssh|telnet` selector.

Three important details from the discussion:

1. **`sys configuration` alone is not enough.** `stephan48` reported that
   without the `envswitch boseurls set …` line his typo in `bmxRegistryUrl` was
   silently restored on reboot — i.e. there is a parallel "envswitch" persistence
   layer that wins on next boot if you don't also write to it. **We must always
   issue both.**
2. **margeServerUrl path is bare for `soundtouch-service`.** We mount the marge
   endpoints at the **root** of port 8000, matching what the existing XML
   migration writes (`Manager.migrateViaXML` in `pkg/service/setup/setup.go`
   sets `MargeServerUrl: targetURL` without any suffix). Some community
   recipes appended `/marge` because they were targeting
   [`deborahgu/soundcork`](https://github.com/deborahgu/soundcork), which
   routes marge under that sub-path. **For our service: bare URL. For users
   redirecting to soundcork: append `/marge`** to both `margeServerUrl` and
   the first argument of `envswitch boseurls set`.
3. **Each command must be sent one at a time, waiting for the device's `OK`
   response** before sending the next one (`foob61451`'s explicit warning).

### 2.2 Account pairing fallback

`envswitch accountid set <numeric-id>` was reported by `bveenker` (#221) as an
in-band equivalent to the HTTP `/setMargeAccount` call, useful when the
`/setMargeAccount` endpoint is missing on the firmware (see §3).

### 2.3 Probing / preflight

- A bare TCP connect to `<deviceIP>:17000` answers (no auth) on devices we care
  about.
- Useful read-only verification command: `getpdo CurrentSystemConfiguration` —
  prints the URLs after the changes have been applied so we can verify before
  rebooting.
- `sys reboot` is the trigger that re-reads both layers.

### 2.4 What Telnet:17000 cannot do

- It does **not** install a custom CA. So if a user wants HTTPS rather than HTTP
  redirection to our service (the DNS-method scenario, where `resolv.conf`
  redirection collides with the device's TLS validation unless our root CA is
  trusted on the device), telnet alone won't cover it. This is fine for our
  default flow, which uses plain `http://` URLs to the service's port 8000.
- It does not give us a way to read or write `Sources.xml` (third-party
  account credentials) — that still requires SSH, but for a migration we don't
  actually need it.

---

## 3. The `/setMargeAccount` problem (issue #236, #228)

### 3.1 What it is

A factory-reset speaker has an empty `<margeAccountUUID/>` in `:8090/info`. The
marge endpoints fail with 502 / unhandled until that field is populated, which
is why several users (#221, #236) saw **everything except AUX** broken after
migration:

```
POST http://<deviceIP>:8090/setMargeAccount
Content-Type: application/xml

<PairDeviceWithAccount>
  <accountId>1234567</accountId>
  <userAuthToken>soundcorkdoesntcare</userAuthToken>
</PairDeviceWithAccount>
```

The values are not validated by the local service, so any numeric `accountId`
will work — soundcork's runbook (#228) literally calls the token
`soundcorkdoesntcare` to make the point.

### 3.2 Why it's broken in practice

There are **three independent failure modes** observed:

| Symptom                                                         | Cause                                                                                                                 | Detection                                                                                                           |
|-----------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------|
| Endpoint returns 404 / "not implemented"                        | Newer firmware (e.g. some BST20 Portable, latest ST Portable) drops the endpoint entirely.                            | `GET /supportedURLs` does **not** list `/setMargeAccount` in `<URL location="…"/>`.                                 |
| Endpoint hangs (no response / socket stays open)                | "Broken state" the user explicitly called out — endpoint advertised, but handler is wedged.                           | Caller has to time out; we currently have no timeout, so the request appears to hang the migration UI indefinitely. |
| `POST /marge/streaming/support/power_on` → 502 unhandled (#236) | Device keeps polling marge after migration but no `margeAccountUUID` was ever assigned, so all subsequent calls fail. | `:8090/info` shows `<margeAccountUUID/>` empty after reboot.                                                        |

### 3.3 Required handling

Per the user's brief, the migration logic must:

1. **Probe** `GET http://<deviceIP>:8090/supportedURLs` and check whether
   `/setMargeAccount` is in the list **before** trying to POST it.
2. **Time-bound** the POST aggressively (e.g. ≤5s connect + ≤10s read) and treat
   anything over the budget as a failure rather than waiting indefinitely.
3. On either failure mode, **fall back** to the telnet equivalent
   `envswitch accountid set <id>` over the same `pkg/telnet` connection used
   for the URL flip. Reboot stays a user-initiated action (§6.2).
4. If telnet:17000 is **also** unreachable, surface a clear "your firmware does
   not support unattended pairing — please pair manually via the official Bose
   app *before* it goes EOS, or open SSH and use the XML method" error rather
   than leaving the device in a half-migrated state.

### 3.4 Where the `<id>` comes from

The device's current account ID is already discoverable through endpoints we
control:

- **`GET :8090/info`** returns `<margeAccountUUID>…</margeAccountUUID>`. If it
  is non-empty the device is already paired — **reuse that ID**, do not
  reassign. Our local marge accepts any ID, so the existing one is fine.
- If it is empty (factory reset), the user picks one in the UI:
  1. **Pick from existing accounts.** The setup UI lists IDs returned by
     `DataStore.ListAccounts()` so a user can re-attach a fresh device to an
     account that already has presets/recents/sources.
  2. **Enter manually.** Free-form text input, validated as **exactly 7
     numeric digits** (the format every Bose-cloud-issued ID has had in the
     captures we've seen, and the format the wider community uses in their
     recipes).
  3. **Randomize.** A "Generate" button that picks a 7-digit number and
     re-rolls if it collides with an existing account in the local datastore.
- **Telnet read-back (best-effort).** `envswitch accountid get` is plausible by
  symmetry with `envswitch accountid set` (#221) but is not yet confirmed
  across firmwares. We will probe it during preflight; if it returns a value
  we cross-check it against `:8090/info` and warn on mismatch.

This means the user is never *forced* to invent a number — the common path is
"the device already has an ID, reuse it" — and the manual/randomize controls
only show up when the device is genuinely fresh.

---

## 4. Port 17000 availability

The diagnostic shell is gated by firmware build and product family. Anecdotally:

- ST 10 / ST 20 / ST 300 / Wave III / Wave IV on FW 27.0.6 → **open**.
- SA-5 with FW 9.x → some commands present (`local_services on`) but
  **no `remote_services on`** and no SSH on FW 9.0.43.23466 (#141).
- Modern firmware on some Portables → endpoint set has shrunk further.

Because of this, we cannot assume port 17000 is reachable. The migration flow
must:

1. **Probe** with a TCP connect to `<deviceIP>:17000`, with a tight timeout
   (≤2s). A successful TCP handshake is necessary but not sufficient — some
   hardened firmware closes the port immediately.
2. **Banner check.** After connecting, read whatever the device sends within
   ~1s. The diagnostic shell prints a small banner (firmware-dependent); a
   blank read or an immediate close means we should treat it as "telnet not
   usable" and disable the option.
3. **Capability check.** Issue a no-op like `getpdo CurrentSystemConfiguration`
   and look for any non-empty response. If the device replies "Command not
   found" we abort and suggest XML or DNS instead.
4. **Surface state to the UI.** The migration form should grey out the Telnet
   option when the probe fails and show *why* (closed, banner missing,
   command rejected) instead of letting the user click into a dead end.

---

## 5. Implementation feasibility — Telnet client in Go

This is a feasibility check only; no code is written yet.

### 5.1 Protocol

"Telnet" on port 17000 is effectively a line-oriented plain-TCP shell. The
device prints a small prompt (`->` in the SA-5 captures from #141) and reads
newline-terminated commands. There is **no** real Telnet option negotiation
(no `IAC`/`DO`/`WILL` exchanges visible in the wild captures), so we don't
need `golang.org/x/crypto/ssh`-class machinery.

### 5.2 Standard-library only

A minimal client is just `net.DialTimeout("tcp", host+":17000", 2*time.Second)` +
`bufio.Scanner` + `time.Time`-based deadlines on `Conn`. No third-party Telnet
library is needed; `github.com/reiver/go-telnet` would be overkill and adds
maintenance surface for no benefit. This matches the project's KISS principle
in `docs/CLAUDE.md` §3.

### 5.3 Cross-platform compatibility

`net.Dial` over TCP works identically on Windows, macOS, Linux and (with
limitations on listening) WASM. WASM-side: `soundtouch-service` runs server-side
anyway, so this only matters for `soundtouch-cli`, where TCP dial works in any
target other than browser-WASM — an acceptable carve-out documented separately.

### 5.4 Concurrency / safety

Each migration is a single goroutine driving one device. The client must:

- enforce per-command response deadlines so a wedged device cannot stall the
  migration UI (mirrors the `/setMargeAccount` requirement);
- abort the rest of the sequence on the first non-`OK` response so we don't
  half-write configuration;
- always close the socket on error.

### 5.5 Testing strategy

We can test without a real speaker by spinning up a `net.Listen("tcp", "127.0.0.1:0")`
in the test, scripting it to consume our commands and emit canned `OK`/error
responses. That gives us deterministic coverage for:

- happy path (all four URLs accepted),
- single-command failure → sequence aborts, no further commands sent,
- "command not found" on `envswitch …` → fallback path exercised,
- TCP closed mid-stream → migration aborts cleanly,
- read deadline triggers when the device hangs (the broken-state simulation).

The repo already follows the "real device responses preferred, mock servers
otherwise" rule (see `docs/CLAUDE.md` §1, §8). The tests above are the mock-server
half of that pattern.

### 5.6 Where it lives

The protocol client is **a standalone package**, not buried inside
`pkg/service/setup`, so it can be reused from CLI tools, future setup wizards,
and tests without dragging the migration manager in:

```
pkg/telnet/                        # NEW reusable package
  client.go                        #   Dial / SendCommand / Probe / Close
  client_test.go                   #   mock-server tests against a net.Listen

pkg/service/setup/
  telnet_migration.go              # NEW thin wrapper that imports pkg/telnet
                                   # and runs the URL config sequence
  marge_pairing.go                 # NEW /setMargeAccount probe + post + telnet
                                   # `envswitch accountid set` fallback
  setup.go                         # add MigrationMethodTelnet const + case
```

UI plumbing is `pkg/service/handlers/web/index.html` (option list) and
`pkg/service/handlers/web/js/script.js` (`toggleMigrationMethod()`). The
deprecated `hosts` option is already hidden from the dropdown when we ship
this; we just add a `telnet` option next to `xml`/`resolv`.

### 5.7 Verdict

**Feasible and small.** Estimated scope: ~200 lines of client code in
`pkg/telnet`, ~300 lines of tests, plus a `MigrationMethodTelnet` branch in
`Manager.MigrateSpeaker`, plus the preflight probe described in §4 and the
`/setMargeAccount` guarding described in §3.

---

## 6. Decisions made (was: open questions)

1. **Account-ID generation.** Resolved — see §3.4. The migration form reads
   `:8090/info` first; if `margeAccountUUID` is non-empty it is reused.
   Otherwise the UI offers (a) pick from `DataStore.ListAccounts()`,
   (b) manual entry validated as 7 numeric digits, (c) a "Generate" button
   that randomizes a 7-digit number and re-rolls on collision.
2. **Reboot policy.** Migration writes configuration only — it does **not**
   issue `sys reboot` itself. Reboot stays user-initiated via the existing
   reboot button in the web UI, the same way XML/DNS migration already works.
   That button's endpoint (`POST /setup/reboot/{deviceId}`,
   `Manager.Reboot(deviceIP)`) gains an optional `?method=ssh|telnet` query
   parameter; default stays `ssh` so existing behavior is preserved. The
   button itself uses a plain `confirm()` dialog before firing.
3. **CA / HTTPS story.** Telnet has no way to install a custom CA. Documented
   as an explicit limitation: telnet method = HTTP-only redirect to our
   service. Users who need end-to-end TLS must use the XML or DNS method.
   *Possible future enhancement* — a hybrid "install CA via SSH/XML, then drive
   the URL flip via Telnet" path. Feasibility unknown; not in this iteration.

---

## 7. Summary of what changes when this lands

- **New reusable package `pkg/telnet`** — sibling of `pkg/ssh`, line-oriented
  TCP client with `Dial`, `SendCommand`, `Probe`, `Close`, all deadline-driven.
  No external dependencies, usable from CLI, service, and tests.
- **New `MigrationMethodTelnet = "telnet"`** constant in `pkg/service/setup/setup.go`
  plus a `migrateViaTelnet` branch in `Manager.MigrateSpeaker`.
- **New `pkg/service/setup/telnet_migration.go`** orchestrating the URL
  configuration sequence (§2.1) on top of `pkg/telnet`. Configuration only —
  no `sys reboot` here.
- **New `pkg/service/setup/marge_pairing.go`** with `PairAccount(deviceIP, id)`:
  probes `/supportedURLs`, time-bounded `POST /setMargeAccount`, falls back to
  telnet `envswitch accountid set <id>` on missing/wedged endpoint.
- **`Manager.Reboot` and `HandleRebootDevice` gain a method selector** —
  signature changes to `Reboot(deviceIP string, method RebootMethod) (string, error)`
  with `RebootMethodSSH` (default, today's behavior) and `RebootMethodTelnet`
  (sends `sys reboot` over a fresh `pkg/telnet` connection). Handler reads
  `?method=ssh|telnet` from the query string.
- **`MigrationSummary` gains** `TelnetReachable`, `TelnetBanner`,
  `TelnetCommandsAccepted`, `SetMargeAccountSupported`, `CurrentAccountID`,
  `KnownAccountIDs` so the UI can show preflight outcomes and offer reuse.
- **UI** — `web/index.html` dropdown gets a `telnet` option (greyed out when
  preflight fails) and a new pane for picking/entering/randomizing a 7-digit
  account ID when `:8090/info` reports an empty `margeAccountUUID`. The
  existing reboot button gets a method selector (radio or dropdown) wired to
  the new query param, with `confirm()` before firing. The legacy `hosts`
  option stays out of the dropdown (deprecated).
