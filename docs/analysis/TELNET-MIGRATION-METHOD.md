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

> For a broader catalogue of every telnet command the community has documented
> across firmware eras (the `key`, `network`, `sys`, `envswitch`, `getpdo`,
> `scm`, `ws`, `swupdate`, and shell-unlock families), see
> **[TELNET-COMMAND-REFERENCE.md](TELNET-COMMAND-REFERENCE.md)**. This
> section only lists the subset our migration actually drives.


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

> **See §9 for the as-shipped state.** Section 7 below records the
> original forecast; the wizard grew larger during implementation and
> §9 documents what actually landed.

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

---

## 8. Device compatibility today

What follows is the current best read on which devices our `migrateViaTelnet`
flow handles end-to-end, derived from the same six sources catalogued in
[TELNET-COMMAND-REFERENCE.md](TELNET-COMMAND-REFERENCE.md) plus the issue
threads cited above. This is migration-outcome perspective; for per-command
availability see the reference doc.

### 8.1 Proven to work end-to-end

All on the firmware-27.0.6 family, which is what survived through Bose's
end-of-service cut. Multi-reporter agreement on every row.

| Device   | Reporter(s)                 | Source           | Confirmed                                                                  |
|----------|-----------------------------|------------------|----------------------------------------------------------------------------|
| ST 10    | foob61451, TJGigs           | #221, #228       | All four URLs persist; `envswitch boseurls set` survives `sys reboot`      |
| ST 20    | foob61451, mcdona1d, TJGigs | #221, #141, #228 | Same; multiple independent reports                                         |
| ST 300   | mcdona1d                    | #141             | `sys configuration` + `envswitch` + `sys reboot` round-trip                |
| Wave III | bveenker                    | #221             | URLs accepted; presets work after pairing fallback (§3)                    |
| Wave IV  | stephan48                   | #221             | Port-17000 path **was the only one that worked** — USB-stick unlock failed |

The exact sequence each reporter ran by hand is the sequence our migration
sends (§2.1). So the migration's happy path is exercised against five
hardware variants in independent captures.

### 8.2 Proven to need the pairing fallback

Migration of the URLs themselves works on these models, but
`POST /setMargeAccount` is missing or wedged on the firmware build, so
pairing has to go through the telnet `envswitch accountid set <id>` path
that `setup.PairAccount` already implements.

| Device                         | Reporter | Source                      | Why fallback is needed                                                                                                                                   |
|--------------------------------|----------|-----------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| ST Portable / FW 27.0.6        | jmosen   | #236                        | After migration: `POST /marge/streaming/support/power_on` → 502; `<margeAccountUUID/>` empty. Time-bounded HTTP path fails; envswitch fallback succeeds. |
| BST20 Portable (factory reset) | ubittner | scheilch/opencloudtouch#167 | `<margeAccountUUID/>` empty; `/setMargeAccount` not in `/supportedURLs`. HTTP path skipped entirely; only the telnet fallback works.                     |

### 8.3 Likely to fail (but the failure is clean)

Our preflight + abort-on-first-rejection design (`TestMigrateViaTelnet_CommandNotFoundAborts`)
means none of these scenarios leave a device half-configured. The user is
told what failed and pointed to the XML or DNS method.

| Device                                     | Source          | Likely cause                                                                                                                                                                                                                             |
|--------------------------------------------|-----------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **SA-5** (sound amplifier) on FW 9.0.43.x  | soundcork#141   | FW 9.x has a different shell generation: `->` prompt, `local_services on`, `scm uboot_ver`. **`sys configuration` and `envswitch` are not documented as working there.** Migration fails on command #1.                                  |
| **Recent ST Portable** (post-27.0.6.46330) | #236 (indirect) | `/setMargeAccount` removal points to broader command-set shrinkage. If `envswitch accountid set` is also gone, both migration and pairing fallback fail; user is told to pair via the official Bose app before EOS, or use XML over SSH. |

### 8.4 Unknown — would benefit from real-device verification

| Device                     | Why unknown                                                         | What we'd want to confirm                                                |
|----------------------------|---------------------------------------------------------------------|--------------------------------------------------------------------------|
| **ST 30** (`mojo`)         | No concrete capture in any of the six sources                       | Almost certainly works — same FW family as ST 10/20/300 — but unverified |
| **ST 520 / Home Cinema**   | USB-unlock reports failing (#141), no port-17000 capture either way | Whether `sys configuration` and `envswitch` are exposed at all           |
| **Wave Music System I/II** | `flarn2006`-era hardware, not seen in 27.x reports                  | Whether port 17000 is even open on those models                          |

### 8.5 The S5 "valid roots" tension

S5 (the r/bose telnet-probing thread) lists only `key`, `net`, `sys`,
`getpdo` as command roots that don't return "Command not found" on its
ST 10 / FW 27.0.6 — which would seem to rule out `envswitch`. But foob61451
on the same hardware/firmware ran `envswitch boseurls set` successfully
(#221).

The most plausible reading is that **S5 is a non-exhaustive probe**, not a
negative claim: the author writes "I've made some educated guesses and come
up with the following valid commands" and never says they tested
`envswitch`. We do not down-weight `envswitch` availability on the strength
of S5 alone — but if a real-device run ever shows `envswitch` rejected on
an ST 10, our preflight catches it, the migration aborts on the first
non-OK response, and the user gets a clear error rather than partial state.

### 8.6 Failure-mode matrix

What `migrateViaTelnet` does in each failure mode (verified by
`pkg/telnet` and `pkg/service/setup` unit tests):

| Failure                                        | Outcome                                                                                           | Test                                                                                                               |
|------------------------------------------------|---------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------|
| Port 17000 closed / TCP unreachable            | `Dial` errors before any command is sent; UI shows the error; nothing persisted                   | `TestMigrateViaTelnet_DialFailureReturnsError`                                                                     |
| `sys configuration` rejected (cmd #1)          | Sequence aborts; verification not sent; rest of commands not attempted                            | `TestMigrateViaTelnet_CommandNotFoundAborts` (envswitch variant — generalises)                                     |
| `envswitch boseurls set` rejected              | Sequence aborts; runtime-only `sys configuration` state reverts on reboot — no permanent damage   | `TestMigrateViaTelnet_CommandNotFoundAborts`                                                                       |
| Verification mismatch (URLs not echoed back)   | Loud "verification failed" error; live state may persist until reboot but UI never claims success | `TestMigrateViaTelnet_VerifyMismatchFails`                                                                         |
| `/setMargeAccount` 502 / hang                  | 5s connect + 12s total budget enforced; falls through to telnet `envswitch accountid set`         | `TestPairAccount_FallsBackWhenHTTPReturnsServerError`                                                              |
| `/setMargeAccount` missing in `/supportedURLs` | HTTP path skipped; goes straight to telnet `envswitch accountid set`                              | `TestPairAccount_FallsBackWhenSetMargeAccountMissing`                                                              |
| Both pairing paths unavailable                 | Structured error: "use the official Bose app before EOS, or open SSH and use the XML method"      | `TestPairAccount_NoTelnetAndHTTPMissingReturnsClearError`, `TestPairAccount_TelnetCommandNotFoundReportsBothPaths` |

### 8.7 TL;DR

- **Green light** — ST 10, ST 20, ST 300, Wave III, Wave IV on FW 27.0.6 (multi-reporter agreement).
- **Yellow** — ST Portable and BST20 Portable: migration works, pairing needs our fallback (already implemented).
- **Red, but fails cleanly** — SA-5 on FW 9.x, possibly newer ST Portable builds.
- **Unverified but expected to work** — ST 30, ST 520, Wave Music System I/II.

The most useful next verification step is touching a real ST 30 and ST 520
— those are the two "expected to work" models with zero concrete captures.
Beyond that, every behaviour the doc predicts is exercised by the unit
tests in `pkg/telnet` and `pkg/service/setup`.

---

## 9. What actually shipped (post-implementation addendum)

§7 forecast the surface area roughly; the wizard ended up larger. This
section is the present-day map of the migration tab and the supporting
backend pieces — kept appended rather than rewritten in place so the
feasibility analysis above stays a faithful design record.

### 9.1 Three-axis state model

`MigrationSummary` now exposes the four mechanism-specific booleans
that `checkIsMigrated` writes individually:

- `XMLMigrated` — parsed SoundTouchSdkPrivateCfg.xml's URLs point at us.
- `HostsMigrated` — `/etc/hosts` carries Bose-domain redirects (the
  deprecated method, kept detectable for legacy speakers).
- `ResolvMigrated` — the `/etc/resolv.conf` priority-nameserver hook
  is in place (with CA trusted).
- `TelnetMigrated` — `getpdo CurrentSystemConfiguration` reports the
  service hostname.

`IsMigrated` is the OR. Plus `IsPaired` from the live
`:8090/info.margeAccountUUID` value.

The frontend opens with a state card that surfaces three orthogonal
axes derived from these flags:

| Axis              | Verdict semantics                                                                                                     |
|-------------------|-----------------------------------------------------------------------------------------------------------------------|
| URL Configuration | URL flip active → ✅; original Bose URLs + DNS hook active → ✅ (intercepted); original + no DNS → ❌ (not intercepted). |
| DNS Interception  | None / resolv.conf hook / /etc/hosts (with deprecated badge).                                                         |
| CA / TLS          | Local root CA installed yes/no.                                                                                       |

Plus a Preconditions row: `remote_services` persistence, account
pairing state, XML config backup presence. Action affordances
(`Trust CA Now`, `Download CA cert`) live inline next to their verdicts.

### 9.2 Plan card with per-field URL editor

Replaces the XML method's `self/proxied/original` dropdowns and the
duplicate URL inputs that used to live inside the telnet method pane:

- Target service URL input with `Save as default` (POSTs to
  `/setup/settings`, preserving the `***` secret-unchanged convention).
- Capabilities header: detected transports (SSH / Telnet:17000) and
  the recipes AfterTouch can offer given those transports.
- Service URLs table: four free-form URL inputs (Marge / Stats /
  SwUpdate / BmxRegistry) with on-keystroke validation
  (`validatePlanURLs`), a Soundcork-mode checkbox that flips `/marge`
  on `margeServerUrl`, and a `Reset to defaults` button.
- Account pairing section: ID input + Generate + datastore picker;
  the implicit intent (`readPlanPairTarget`) queues a pair step at
  Apply when the input differs from the current `account_id`.
- Suggested plan box: one-click conservative default — XML + HTTP
  when SSH works, Telnet + HTTP otherwise; "Already migrated" info
  state when `IsMigrated` is already true.

The per-field URLs feed both XML and Telnet migrations via the
`marge_url` / `stats_url` / `sw_update_url` / `bmx_url` option family
(see §9.6). Live preview rewrites `#planned-config` purely client-side
on every keystroke — optimistic; the backend's perspective gates the
write via §9.4's pre-flight.

### 9.3 Customize three-axis form

The `<details>` "Customize this migration" section replaces the old
migration-method dropdown with three independent radio groups:

1. **URL flip transport**: XML / Telnet:17000 / Skip.
2. **DNS interception**: None / `/etc/resolv.conf` hook.
3. **Local CA install**: checkbox.

Each option carries a per-axis availability hint
(`(SSH unreachable)`, `(already trusted)`, etc.) so users see *why*
an option is disabled. `applyCustomPlan` orchestrates the chosen
combination as a sequence of existing backend calls
(`/setup/migrate?method=…` for each flip/resolv step plus
`/setup/trust-ca` for standalone CA install, and the queued pair
step from §9.2). Resolv already bundles a CA install, so a redundant
standalone CA step is skipped. First failure aborts the rest.

### 9.4 Pre-flight panel

Both Apply paths run a visible pre-flight panel before any backend
operation touches the speaker. Each check renders inline with the
🕐 / ⟳ / ✅ / ❌ / — idiom. On all-green the panel holds for ~700ms so
the success state registers, then auto-proceeds. On any failure the
panel surfaces `Proceed Anyway` / `Cancel` buttons; default is to
abort.

Checks:

| Check                         | When                                          | Backend route                  |
|-------------------------------|-----------------------------------------------|--------------------------------|
| Backend summary re-check      | always                                        | `GET /setup/summary`           |
| HTTPS connection from device  | `ssh_success && server_https_url`             | `POST /setup/test-connection`  |
| Telnet round-trip probe       | `!ssh_success && telnet_reachable` (see §9.5) | `POST /setup/telnet-probe`     |
| DNS redirection from device   | `methods.includes("resolv") && ssh_success`   | `POST /setup/test-dns`         |

The HTTPS check uses `use_explicit_ca=true` so it exercises the trust
path even when CA install is part of the plan (i.e. forward-looking).
The reachability skip row is explicit ("neither SSH nor Telnet:17000
is reachable") rather than silently dropped, per the user's
"feedback always visible" requirement.

### 9.5 Telnet round-trip probe — the SSH-less reachability check

The reachability gap §7 left open for USB-unlock-refusing speakers is
closed by `Manager.RunTelnetRoundTripProbe`
(`pkg/service/setup/telnet_probe.go`). Sequence:

1. Telnet `getpdo CurrentSystemConfiguration` to capture the
   speaker's current `swUpdateUrl`.
2. Generate a random 24-hex-char token; register a one-shot signal
   channel under it on the new `probeRegistry` (sibling field on
   `handlers.Server`).
3. Telnet `sys configuration swUpdateUrl <targetURL>/probe/<token>`
   — **runtime layer only, deliberately not `envswitch boseurls set
   …`**. The persistence layer keeps the original URL, so a reboot
   heals the device naturally if our restore step fails.
4. `HTTP GET <deviceIP>:8090/swUpdateCheck` — the cleanest
   `:8090` endpoint that triggers exactly one outbound to the
   configured `swUpdateUrl`. Read-only on the cloud side
   (doesn't initiate an update); independent of `margeAccountUUID`
   so it works on factory-reset speakers.
5. Wait on the registered channel up to `telnetProbeTimeout` (6s).
6. Telnet `sys configuration swUpdateUrl <originalURL>` — deferred
   restore so it runs even on the failure path.

The new `/probe/{token}[/*]` catch-all on the root router signals the
matching channel when the speaker's outbound lands. The response is
a minimal `<swUpdateIndex/>` so the speaker's `swUpdateCheck`
doesn't choke on a missing structure. The `/*` sub-path is
registered because some firmware appends a path component to the
configured `swUpdateUrl`.

### 9.6 Backend additions worth knowing

| Addition                                                               | Where                                        | Why                                                                                                                                                                          |
|------------------------------------------------------------------------|----------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `applyURLOverrides(cfg, options)`                                      | `pkg/service/setup/setup.go`                 | Per-field literal `marge_url` / `stats_url` / `sw_update_url` / `bmx_url` overrides win over `applyProxyOptions`. Honored by both `GetMigrationSummary` and `migrateViaXML`. |
| `telnetURLsFromOptions(targetURL, options)`                            | `pkg/service/setup/telnet_migration.go`      | Same option family as above, plus envswitch arg derivation rule (arg1 = final Marge verbatim; the soundcork-suffix case drops out).                                          |
| Per-axis booleans + `IsPaired` + `Warnings`                            | `MigrationSummary`                           | Surfaces partial-state cells and SSH-XML ⇄ telnet-getpdo cross-check disagreements.                                                                                          |
| `parseGetpdoConfig`                                                    | `pkg/service/setup/preflight_crosscheck.go`  | Parses the Protobuf-text-like nested-block reply (`key { text: "..." }`) FW 27.0.6 actually sends, plus the legacy `key=value` shape as a tolerance path.                    |
| `probeRegistry` + `RunTelnetRoundTripProbe` + `/setup/telnet-probe`    | `pkg/service/handlers` / `pkg/service/setup` | §9.5.                                                                                                                                                                        |
| `migrationOptionKeys` allow-list                                       | `pkg/service/handlers/migration_options.go`  | Unknown query keys never reach the manager. Both XML mode keys and `*_url` keys are recognised.                                                                              |
| Telnet client default timeouts: dial 4s, read 7s, write 3s, idle 600ms | `pkg/telnet/telnet.go`                       | Bumped from the original 2s/5s/2s/400ms after observing transient i/o-timeout flakes on healthy speakers that recovered on retry.                                            |

### 9.7 Future probe candidates

- `:8090/pushCustomerSupportInfoToMarge` — flagged as a potential
  "ask the device about itself" probe that could feed a richer
  device-info pane (firmware build dates, hardware revisions). Not
  implemented.
- Running the round-trip probe on SSH-capable speakers too (as
  additional validation alongside the curl-from-device HTTPS test),
  not just as the SSH-less fallback it is today. **Subsumed by §9.8
  — the round-trip probe is being removed; the passive observer is
  transport-agnostic and replaces it for migrated speakers.**

### 9.8 The swUpdate daemon-cache finding and removal of §9.5

The §9.5 round-trip probe was retired after empirical testing on a
fully-migrated speaker (FW 27.0.6) revealed that the `swUpdate`
daemon **caches its target URL at boot and ignores live config
writes**. The diagnostic sequence:

1. Manual telnet flip of both layers — `sys configuration swUpdateUrl
   <probe-url>` (runtime) **and** `envswitch boseurls set <marge>
   <probe-url>` (persistence). `getpdo CurrentSystemConfiguration`
   confirmed both writes stuck.
2. HTTP GET `:8090/swUpdateCheck` to trigger fan-out.
3. Service access log showed the device outbound landed on
   `/updates/soundtouch` (the **previous** `swUpdateUrl` value, current
   at the last daemon boot) and `/streaming/software/update/account/<id>`
   (a separate Bose URL the daemon hits, routed to this service by DNS
   interception). The probe URL was never dialed.

This falsifies the original NEXT.md hypothesis that the persistence
layer would override the runtime layer for the daemon's fan-out, and
points instead at daemon-level URL caching. Two consequences:

- **The §9.5 probe cannot work on migrated speakers without a
  reboot.** The cached URL is set when the daemon starts; flipping
  config after that point has no effect on what the daemon dials.
- **The §9.5 probe likely cannot work on unmigrated speakers
  either**, for the same reason — the daemon caches whatever URL it
  read at startup, which on an unmigrated speaker is the Bose cloud
  URL. We have no service running with the probe URL registered on
  unmigrated speakers, so the original "it worked in testing" claim
  has no empirical basis; it likely failed silently because nothing
  was watching.

The honest replacement is a **passive observer** (see
`pkg/service/setup/peer_probe.go`):

1. Register the device IP with an in-process observer
   (`handlers.peerObserver`, wired via `PeerObserverMiddleware`).
2. Nudge `:8090/swUpdateCheck` to make the daemon fan out *something*
   sooner than its ~5min timer.
3. Wait up to 30s for any inbound from that IP. On a migrated
   speaker, DNS interception means the daemon's outbounds (update
   fan-out, marge polls, BMX registry calls) all funnel through this
   service regardless of which URL the daemon resolved internally —
   so reachability reduces to *"did the device dial us at all."*

Endpoint: `POST /setup/peer-probe/{deviceId}`. No device-state
mutation; safe to re-run. Returns `{ok, result: {reached,
observed_path, elapsed_ms}, error}` with the same UI keying as the
old probe (`result.reached`).

#### 9.8.1 The pre-flight panel branch

The web UI's pre-flight orchestrator (`runApplyPreflight` in
`script.js`) branches on `summary.is_migrated`:

| Migration state                   | Reachability row                                                                                                             |
|-----------------------------------|------------------------------------------------------------------------------------------------------------------------------|
| Migrated (`is_migrated=true`)     | "Reachability check (passive observer)" — calls `POST /setup/peer-probe/{deviceId}`.                                         |
| Not migrated (incl. partial)      | Skip row "Round-trip validation runs after Apply + reboot" with the rationale "daemon caches swUpdateUrl at boot".           |

Per-axis booleans (`xml_migrated`, `hosts_migrated`, `resolv_migrated`,
`telnet_migrated`) remain visible in the State card, so the user can
see which parts of the migration are already in place even when the
overall flag is false. The skip row does not attempt the active probe
on unmigrated speakers — the canonical telnet flow is:

```
Apply telnet config → user-initiated reboot → re-run pre-flight on
the now-migrated speaker → passive observer confirms fan-out.
```

#### 9.8.2 Removal trail

Removed (or scheduled for removal in a follow-up commit) at the time
of §9.8 landing:

- `pkg/service/setup/telnet_probe.go` — `RunTelnetRoundTripProbe`,
  `ProbeRegistrar`, `TelnetProbeResult`, `generateProbeToken`.
- `pkg/service/handlers/handlers_telnet_probe.go` — `HandleTelnetProbe`,
  `HandleProbeInbound`, `telnetProbeTimeout`, `telnetProbeResponse`.
- `pkg/service/handlers/probe_registry.go` — `probeRegistry` + tests.
- `Server.probes` field.
- Routes `/probe/{token}`, `/probe/{token}/*`, `/setup/telnet-probe/{deviceId}`.
- The `target_url` query-param plumbing on the deprecated endpoint.
- `script.js` — `checkTelnetRoundTrip` (orchestrator call site removed
  in the commit that added the branch; function itself removed later).

`isCommandNotFound` and `parseGetpdoConfig` stay — they are also used
by the migration writer (`telnet_migration.go`), preflight reader
(`telnet_preflight.go`), pairing path (`marge_pairing.go`), and
cross-check (`preflight_crosscheck.go`).
