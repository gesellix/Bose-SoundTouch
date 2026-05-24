# Device-Local Install: Four User Journeys

> **Looking for how to actually install AfterTouch?**
> See the [Deployment Overview](../guides/DEPLOYMENT-OVERVIEW.md) for user-friendly
> step-by-step guides for both deployment scenarios (external host and on-device).
>
> This document is an **architectural analysis** — user journeys, install patterns,
> technology tradeoffs, and future directions. It is aimed at contributors and
> project planning, not at end users.

---

A user-journey-shaped view of where AfterTouch sits today and where it could go. The same speaker, the same constraints, but four different audiences with non-overlapping needs:

1. **Initial setup / install** — getting AfterTouch onto a fresh or freshly-orphaned speaker.
2. **Less-technical admin** — migration, maintenance, and recovery without a terminal.
3. **Daily usage** — playing music, switching presets, on the couch or on the phone.
4. **Automation** — driving the speaker from scripts, home automation, schedules.

Each journey is served by a different surface (CLI, web UI, GUI app, REST). Some surfaces serve more than one journey; some journeys are served badly today. This doc is informational; nothing here is a roadmap commitment.

Cross-cutting reference material — lessons from `GameTec-live/soundtouch-tiny`, plus a per-surface capability map — lives in the appendix.

---

## Journey 1: Initial setup / install

**Who.** Someone with a Bose speaker whose cloud just died. Could be technical (knows what SSH is) or not (knows what a USB stick is). Wants the speaker to play Internet Radio again with minimum fuss.

**Goal.** Get an AfterTouch instance reachable from the speaker, whether that instance lives on a separate host or on the speaker itself.

**Surfaces.** Shell (today), GUI installer (planned), pre-flashed stick (commercial offering, hypothetical).

### The three install patterns

#### Pattern A — External host

A separate machine (Raspberry Pi, NAS, always-on laptop) runs `soundtouch-service`. Speakers point at it via DNS rewrite at the router. No code on the speaker, no firmware risk.

- **Pros:** zero invasiveness, easy update (single host), unified for many speakers, no per-speaker storage limit.
- **Cons:** requires an always-on host on the LAN, DNS rewrite at router scope, single point of failure.

#### Pattern B — SSH-curl on-device (current `scripts/on-device-install/`)

User SSHes in once, pipes the installer. Installs to `/mnt/nv/aftertouch`, symlinks `/opt/aftertouch`, registers `/etc/init.d/aftertouch` via `update-rc.d`. Daemon serves `:8000` on the speaker's own LAN address.

- **Pros:** no separate host, per-speaker isolation, survives router replacement.
- **Cons:** SSH required for install and updates, ~12 MB binary stresses tiny rootfs partitions, no in-process restart on crash, some firmware images bind only loopback (issue #196).

#### Pattern C — Stick-driven on-device (*not* implemented here)

USB stick holds binary + bootstrap scripts. First install needs SSH (placing `/mnt/nv/rc.local`). After that, the NAND `rc.local` auto-syncs from any stick inserted with newer files. Stick can also carry one-shot configs (`wlan.conf`, `region.conf`, `name.conf`) consumed and wiped during boot.

- **Pros:** post-bootstrap updates need no SSH, stick wipe behavior keeps credentials short-lived, watchdog inside the bootstrap script restarts the agent on crash without a reboot.
- **Cons:** first install still needs SSH; FAT32 stick on the speaker is unreliable for writes; user has to keep a stick around.

### The technical underpinning: `/mnt/nv/rc.local`

Both pattern C and any "shepherd-less" install on stock firmware depend on a single line in the stock init scripts:

```
# /etc/init.d/shelby_local, start case
[ -x /mnt/nv/rc.local ] && /mnt/nv/rc.local
```

`shelby_local` is a stock Bose SysV script. Its `start` case fires at every boot from an `S`-symlink in `rcS.d/` (the misleading `K99shelby_local` symlink in `rc1.d/` is the *shutdown* path — same script, different case). `/mnt/nv` is the persistent read-write NAND partition; `rc.local` is intentionally exposed as an extension point. By the time it runs, rootfs is mounted read-only, `/mnt/nv` is read-write, network is configured, and `/media/sda1` is *typically* mounted by udev if a USB stick is present — but the mount is asynchronous and races the hook (polling for up to 30 s is one way to handle this).

**Stock firmware does not auto-copy anything from a USB stick into `/mnt/nv/rc.local`.** Inserting a stick alone is not enough. There is no udev rule, no autorun convention, no `shelby_usb` branch that handles this; `shelby_usb` only manages USB ethernet-gadget mode (`g_ether`) and the `microbswitch` helper on certain variants.

Placement happens one of two ways:

1. **Manual SSH bootstrap, once.** Shell access (via the `remote_services` stick trick) runs an installer that writes `/mnt/nv/rc.local`, makes it executable, and exits. After that single SSH session, the stick is no longer required to *trigger* anything — the NAND copy fires on every boot.
2. **Self-update from a newer stick, after step 1.** Once `/mnt/nv/rc.local` exists *and contains the self-update logic*, inserting a stick with a newer `rc.local` (compared by mtime) lets the running NAND copy overwrite itself for the next boot. This gives the stick its "repair channel" property.

**The very first placement requires SSH.** Any zero-SSH install would need either a different stock-firmware hook (we have not found one usable across SoundTouch variants) or a custom firmware image. The `remote_services` stick is the only stick-content convention the stock firmware honors out of the box, and all it does is enable `sshd`.

### App-driven install (the missing middle)

The SSH session does **not** have to be a human SSH session. `pkg/ssh` (`NewClient`, `Run`, `ReadFile`, `ReadDir`, `UploadContent`) is already used by `pkg/service/setup/` to drive migration probes; the same primitives can drive an installer. The user never sees a terminal.

User-visible flow:

1. User runs an admin app on their laptop or phone.
2. App walks them through preparing a `remote_services` stick — or writes one for them, if it can reach the host's USB subsystem.
3. User inserts the stick into the speaker and power-cycles it. Stock firmware's `sshd` starts.
4. App discovers the speaker via mDNS, dials SSH, runs the installer steps that today live behind `curl ... \| sh`. No `ssh` invocation, no `rw &&`, no copy-pasted IP.
5. App verifies `curl http://<box>:8000` from inside the speaker via SSH and surfaces a clear success / failure state.
6. App optionally removes `remote_services` from the stick and reboots the speaker, closing the SSH backdoor automatically.

Mapping each step to existing code:

| Step                | Today's installer                                                       | App equivalent (`pkg/ssh`)                                                                                  |
|---------------------|-------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| Remount rootfs rw   | `mount -o remount,rw /` (inside init script)                            | `Client.Run("mount -o remount,rw /")`                                                                       |
| Make NAND dir       | `mkdir -p $INSTALL_DIR`                                                 | `Client.Run("mkdir -p /mnt/nv/aftertouch")`                                                                 |
| Download binary     | `curl -sSL ... -o binary`                                               | local download on the app side, then `Client.UploadContent(bytes, "/mnt/nv/aftertouch/aftertouch-service")` |
| Mark executable     | `chmod +x`                                                              | `Client.Run("chmod +x ...")`                                                                                |
| Symlink `/opt`      | `ln -sf $INSTALL_DIR /opt/aftertouch`                                   | `Client.Run("ln -sf ...")`                                                                                  |
| Install init script | `curl ... -o /etc/init.d/aftertouch && update-rc.d aftertouch defaults` | `Client.UploadContent` + `Client.Run`                                                                       |
| Start               | `/etc/init.d/aftertouch start`                                          | `Client.Run("/etc/init.d/aftertouch start")`                                                                |
| Verify listener     | `curl -fsS http://localhost:8000` inside the box                        | `Client.Run("curl -fsS http://localhost:8000")`                                                             |

No new SSH plumbing required. The pieces already exist for the setup probes.

### Storage budget

The on-device patterns share one hard constraint: storage. ST20 stock rootfs has ~4 MB free (issue #268); even with `/mnt/nv` (~30 MB free) the budget is tight, and a second binary for safe OTA updates doubles it. This is the primary motivation for a slimmer `soundtouch-service-mini` build target — see the appendix.

### Open decisions for this journey

- Do we keep pattern B as the technical-user path while building a Gio admin app for the rest?
- Do we add a pattern-C-style "register a stick-update hook in `/mnt/nv/rc.local`" option as an opt-in, so users who do want a repair stick get one?
- Pre-flashed sticks shipped as a kit: in scope or out?

---

## Journey 2: Less-technical admin (migration + maintenance)

**Who.** The person who already has AfterTouch installed somewhere and now needs to do something *after* install. They are comfortable opening apps and clicking buttons; they are not comfortable opening a terminal. The whole-household admin: parent, partner, roommate doing it for the household.

**Goal.** Migrate a speaker to a new AfterTouch instance, update the agent, view what's going on, recover a stuck device, change WLAN credentials, reapply config after factory reset — all without SSH.

**Surfaces.** GUI admin app (Gio, planned), `soundtouch-service` embedded web UI (today, technical-leaning), CLI (today, technical-only).

### What "admin" covers in practice

- **Migration of a new (or factory-reset) speaker** to an AfterTouch instance: rewrite the server URLs in `/mnt/nv/persistence.json`, restart the device, verify it talks to us.
- **Agent update on an on-device install** (pattern B or C): push a new binary, restart, verify.
- **Status and diagnostics**: is `aftertouch` running, is `:8000` listening, did the last preset save succeed, what does syslog say?
- **Recovery**: speaker is stuck (won't respond to web UI, won't pair, lost WLAN). Today this almost always means SSH; with `pkg/ssh` behind a GUI, it can mean "click 'Diagnose' in the app."
- **Bulk operations**: do all of the above across several speakers at once.
- **Configuration drift**: WLAN password changed, region changed, speaker name changed, hosts file got rewritten — restore the AfterTouch overlay.

### How the GUI admin app shape would serve this

Same `pkg/ssh` primitives as Journey 1's installer, applied to post-install tasks. mDNS discovers all speakers on the LAN; the app fans operations out across them; SSH-driven actions stay hidden behind buttons. On a phone, the same app is the "speakers are unreachable, what now" diagnostic tool from another room.

Where today's surfaces fall short for this user:

- `soundtouch-service` web UI assumes the service is running and reachable. It cannot recover a broken installation or a stuck device.
- CLI works but presumes terminal comfort.
- The setup wizard in `soundtouch-service` handles initial migration well, but reapplying after factory reset is not first-class — see `docs/analysis/FACTORY-RESET-PROTOCOL.md`.

### Open decisions for this journey

- Does the admin app subsume the service web UI's admin tab, or do they coexist (admin app = onboarding + recovery; service web UI = ongoing operations once everything is healthy)?
- WASM as a fallback surface: today's service web UI is browser-accessible from anywhere. Does a Gio admin app sacrifice that, or do we ship both?
- Multi-household / multi-speaker: how much does the admin app need to know about distinguishing speakers vs distinguishing AfterTouch instances?

---

## Journey 3: Daily usage

**Who.** Anyone in the household using the speaker. Children pressing a preset button. The user opening a phone to switch from kitchen to living room. Guests asked to "just put on some jazz." Zero awareness of AfterTouch as a thing; the speaker is the speaker.

**Goal.** Music plays. Pressing preset 3 gives them what preset 3 should give them. Skipping a station, adjusting volume, browsing for a new station — all fast, no friction.

**Surfaces.** Physical preset buttons (always there), `soundtouch-web` (today), mobile app (Journey 2 admin app's daily-use mode), WASM-served browser UI (planned), Bose app while it still functions, voice assistants where wired up.

### What this layer needs to be good at

- **Preset playback works first try, every time.** The reliability bar is "is the kitchen radio still working?" Anything that fails on cold boot or after a Wi-Fi outage breaks the user's trust in the whole system.
- **Switching stations quickly**, including discovery of new ones (e.g. `radio-browser.info`-style search).
- **Volume and play / pause from any device the user has in hand.** Phone in pocket, laptop on table, browser tab open — all should work.
- **Multi-room awareness** if the household has more than one speaker: which speaker is playing what, can I send this to the bedroom.
- **Looking good.** This is the surface that gets seen daily by non-technical users. Visual polish matters more here than anywhere else in the stack.

### How surfaces map

- `soundtouch-web`: primary daily UI for desktop browsers and (responsively) for tablets. This is already shipped.
- Mobile app: daily-use mode of the same Gio app that handles admin. Capability split — admin features only show up when the user is in admin mode.
- WASM: same Gio app, served from `soundtouch-service` to anyone on the LAN. The "I forgot which device my login is on, just open a browser" fallback.
- Physical preset buttons: handled at the agent level (the Bose firmware fires them; AfterTouch or the on-device agent reacts).

### Open decisions for this journey

- Do we keep `soundtouch-web` as a separate codebase (HTML/JS), or does it become a Gio WASM build sharing code with the admin app?
- Mobile app store distribution: TestFlight for iOS (gated, slow), Play Store for Android (faster, AAB only), F-Droid as an open-source-friendly side path.
- Multi-user state: presets per-user vs per-household. Out of scope here, but the daily surface is where it gets felt.

---

## Journey 4: Automation

**Who.** The same household, but acting through code: a Home Assistant config, a NodeRED flow, a cron job, a shell script, a webhook from a smart doorbell. The user is not present at the speaker; they want music to start when something else happens.

**Goal.** Headless, scriptable control. "Play preset 2 at 7:00 every weekday." "When the kids' bedtime alarm fires, fade volume to zero." "If I get home and the speaker is on, switch to my dinner playlist."

**Surfaces.** `soundtouch-cli` (today), REST endpoints on `soundtouch-service` (today), MQTT bridge / webhook outputs (hypothetical), Home Assistant integration (community).

### What this layer needs to be good at

- **Stable, versioned API surface.** Scripts and home automation flows live for years; breaking changes are expensive for users.
- **CLI that works in pipelines.** Exit codes, machine-readable output (JSON), stable flag names. The reverse of the daily UI: zero polish, full predictability.
- **Discoverability of capabilities.** Users need to find out what's possible (`soundtouch-cli help`, openapi spec on the service, examples in the docs).
- **Idempotency.** Calling "set volume to 40" twice should not result in volume 80. Calling "switch to preset 3" when already on preset 3 should be a no-op.

### How surfaces map

- `soundtouch-cli`: the canonical surface for scripted control. Already covers most of the API.
- `soundtouch-service` REST endpoints: same surface, network-accessible. Used by `soundtouch-web` and by third-party automation.
- Home Assistant: external integration; track but do not own.
- Webhooks / MQTT: not present today; would let speakers participate in event-driven flows. Out of scope for a first pass; worth a separate design doc when demand surfaces.

### Open decisions for this journey

- Stability commitments for the CLI and REST API: do we adopt semver for the public surface separately from the service version?
- Authentication for the REST surface when exposed beyond loopback: needed before any internet exposure is sane.
- OpenAPI / typed-client output for the service: nice-to-have for integration developers.

---

## Appendix: which surface serves which journey

| Surface                            | Journey 1 (install) | Journey 2 (admin) | Journey 3 (daily) | Journey 4 (automation) |
|------------------------------------|---------------------|-------------------|-------------------|------------------------|
| `soundtouch-cli`                   | partial (today)     | partial (today)   | no                | primary                |
| `soundtouch-service` web UI        | wizard portion      | primary           | partial           | indirect (REST)        |
| `soundtouch-web`                   | no                  | no                | primary           | no                     |
| GUI admin app (Gio, planned)       | primary             | primary           | mobile mode       | no                     |
| Pre-flashed stick (hypothetical)   | primary             | recovery          | no                | no                     |
| Physical preset buttons            | no                  | no                | primary           | no                     |
| Home Assistant / webhooks (future) | no                  | no                | no                | primary                |

The diagonal isn't full because some journeys lack a polished surface today (Journey 1 mostly works but is shell-only; Journey 2 has gaps for recovery scenarios). The journey frame is what tells us *which* gaps to fill first.

## Appendix: per-surface capability constraints

The Gio admin app, if built, can target Windows / macOS / Linux / iOS / Android / WASM from one codebase. Each target has hard constraints:

- **WASM (browser).** Post-install REST control, device list and status, preset editing, station search. No mDNS (browsers cannot do raw multicast — fall back to manual IP entry or a backend bridge); no raw TCP, so no SSH and no install; no block-device access, so no stick writing. This is the "I just want to use my speakers" surface, equivalent to today's `soundtouch-web`.
- **Mobile iOS.** Everything WASM does, plus Bonjour-based mDNS, plus full SSH client (so app-driven install and recovery work). No FAT32 stick writing — iOS has no filesystem-level block device access for third-party apps. Best paired with a pre-flashed stick or a friend's desktop install for the bootstrap.
- **Mobile Android.** Same as iOS, plus FAT32 stick writing *if* the user grants USB-OTG host permission. UX caveat: most users will not know what USB host mode is.
- **Desktop (Gio).** Full capability set. mDNS, SSH-driven install, FAT32 stick writing via standard block-device APIs, post-install control, recovery. The primary onboarding surface.

The pattern to follow is to write code so each capability degrades automatically based on what the runtime actually offers, rather than gating with build tags.

## Appendix: lessons from adjacent projects

### soundtouch-tiny (GameTec-live)

Minimal on-device cloud replacement: Internet Radio + TuneIn proxy + optional presets. Go stdlib only, small binary. Inspired by AfterTouch but trimmed. The author offered collaboration in PR #292.

This is the gap a **`soundtouch-service-mini` build target** would fill. The full `soundtouch-service` is justified for the external-host pattern (Pattern A) where space is not pressed; on-device (patterns B and C) the calculus is different — many users only need Internet Radio because that's the surface most affected by the cloud shutdown.

A mini build target in this repo would look like:

- same codebase, different `cmd/` entry point,
- compiled with only the packages needed for Internet Radio + TuneIn shim + presets,
- no Spotify, no parity tests, no setup wizard, no Bose-protocol-level proxy,
- target size: under 4 MB so it fits the rootfs without `/mnt/nv` gymnastics, leaving room for a second binary for safe updates.

Open questions before committing:

1. Collaborate upstream with soundtouch-tiny, or build our own mini that shares code with the full service?
2. Where to draw the feature line — "Internet Radio only" is clear; "Spotify too" would already blow the budget on ST20.
3. Mini ships via Pattern B (SSH-curl) or Pattern C (stick)?
4. Full service and mini service coexisting on the same LAN — mDNS service name, port choice, web UI port.

### Wails vs Gio

Both are Go. Different tradeoffs:

- **Wails v2**: bundles a WebView per OS, frontend is HTML/CSS/JS. Faster to a working UI if the team is comfortable with HTML. Targets Windows / macOS / Linux. No mobile, no WASM.
- **Gio**: immediate-mode pure-Go UI. Smaller binaries, no WebView dependency. Targets Windows / macOS / Linux / iOS / Android / WASM. Steeper UI learning curve, mitigated by `gio-mw`.

The deciding factor is **mobile + WASM** (Journey 2 and Journey 3), not desktop alone. If "use a phone to set up a speaker" or "open the admin tool from any browser" is on the roadmap, Wails does not get us there.

## Appendix: documentation gap to close

Separate user-facing material to produce when we are ready (not in this comparison doc):

- **The `/mnt/nv/rc.local` hook** explained in user terms: what it does, when it fires, when *not* to use it, how to remove it cleanly. Bridges Journey 1 and Journey 2.
- **Hooks we already maintain** at OS level: resolv.conf stability, `/etc/hosts` overlay, anything in `pkg/service/setup/` that touches device state. Reference, not narrative. Journey 2 troubleshooting.
- **Storage budget per model**: rootfs free, `/mnt/nv` free, where the binary lands, which path applies to which ST model. Journey 1 sizing.
- **Decision matrix**: external host vs on-device vs mini, plus "do I need Spotify? do I need migration? do I want one host or per-speaker isolation?" Journey 1 entry point.
- **Stick file conventions**: what the `remote_services` stick does today, what we *might* add (presets / wlan / region) if we build a stick-driven path, and how that interacts with FAT credentials residency. Journey 1.
- **Automation cookbook**: example Home Assistant config, example shell scripts, common pitfalls. Journey 4.

## Cross-references

- AfterTouch installer: `scripts/on-device-install/install.sh`, `scripts/on-device-install/aftertouch` (init script), `scripts/on-device-install/README.md`.
- AfterTouch SSH client: `pkg/ssh/ssh.go` (`NewClient`, `Run`, `ReadFile`, `ReadDir`, `UploadContent`), already used by `pkg/service/setup/`.
- Storage limitations: issue #268 (ST20 rootfs free space), issue #196 (loopback-only bind), issue #250 (status reports running but unreachable).
- soundtouch-tiny: `https://github.com/GameTec-live/soundtouch-tiny`, raised in PR #292 (`https://github.com/gesellix/Bose-SoundTouch/pull/292`).
- opencloudtouch parallel discussion: `https://github.com/scheilch/opencloudtouch/discussions/201`.
- Existing parity doc shape: `docs/PARITY-OPENCLOUDTOUCH.md` is the precedent for cross-project comparison documents.
