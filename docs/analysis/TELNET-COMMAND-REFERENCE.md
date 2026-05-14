# Bose SoundTouch Telnet (Port 17000) Command Reference

A consolidated reference for the diagnostic shell that listens on TCP port
17000 across the SoundTouch line. Compiled from multiple community sources
to give a single map of what's been observed in the wild — useful both for
implementing automation against it (see
[TELNET-MIGRATION-METHOD.md](TELNET-MIGRATION-METHOD.md)) and for manual
recovery / WiFi setup.

> **Important caveat.** The command set is firmware-dependent. Anything that
> existed in firmware 1.x–7.x (`flarn2006`'s era) was progressively trimmed;
> some commands listed here have been removed on firmware 27.x. Where a
> command's availability is known to vary, the **Availability** column says so.

### Telnet via Docker (when not installed locally)

```shell
docker run --rm --name telnet -it --env IP=192.168.123.123 alpine:edge ash -c 'apk add -U busybox-extras && telnet $IP 17000'
```

## Sources

| #  | Source                                                                                                                                                                                                             | Era / focus                                                                                                                                                                                            |
|----|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| S1 | [flarn2006: "Hacking the Bose SoundTouch and its Linux insides"](https://flarn2006.blogspot.com/2014/09/hacking-bose-soundtouch-and-its-linux.html) (2014)                                                         | Firmware 1.x–7.x; root shell discovery, codenames                                                                                                                                                      |
| S2 | [Sam Hobbs: "Connect Bose SoundTouch 10 to WiFi using Linux Telnet"](https://samhobbs.co.uk/2016/01/connect-bose-soundtouch-10-wifi-using-linux-telnet) (2016)                                                     | ST 10 setup mode; `network`/`sys` families                                                                                                                                                             |
| S3 | [izndgroup: "Connect Bose SoundTouch 10 to WiFi"](https://technical.izndgroup.com/2021/02/connect-bose-soundtouch-10-to-wifi.html) (2021)                                                                          | Reissue of S2 with later-firmware notes                                                                                                                                                                |
| S4 | [sijeffrey/SoundTouch — `bose` script](https://github.com/sijeffrey/SoundTouch/blob/master/bose) (2017)                                                                                                            | `nc`-based remote-control script using `sys`/`ws`                                                                                                                                                      |
| S5 | [r/bose "SoundTouch telnet probing"](https://www.reddit.com/r/bose/comments/1o5zkym/soundtouch_telnet_probing/)                                                                                                    | Recent (post-EOS) probing on ST 10 firmware `27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29`; comments mirrored in [#221](https://github.com/gesellix/Bose-SoundTouch/issues/221) |
| S6 | Issue [#221](https://github.com/gesellix/Bose-SoundTouch/issues/221), [#236](https://github.com/gesellix/Bose-SoundTouch/issues/236), [deborahgu/soundcork#141](https://github.com/deborahgu/soundcork/issues/141) | The migration commands we already implement                                                                                                                                                            |

---

## Connecting to the shell

### From an already-on-network device

The shell binds to TCP port 17000 on every device family observed (ST 10/20/300, Wave III/IV, ST 520, SA-5 — see §"Firmware era notes" for caveats). No authentication.

```bash
# A no-op probe just to verify reach.
echo '' | nc -w 2 <device-ip> 17000

# Or interactively — works the same.
telnet <device-ip> 17000
```

The `bose` script (S4) goes one level lower and writes commands directly to a `/dev/tcp/<ip>/17000` redirection target instead of using `nc`. That's the same wire protocol with no library between.

### From a factory-fresh / WiFi-less device

Per S2/S3 — newer firmware may have closed this on some models:

1. **Enter setup mode.** Press and hold key **2** + **volume down** for 5 seconds until the WiFi LED turns amber.
2. **Connect your laptop to the speaker's open access point.** The speaker becomes its own AP.
3. **Telnet to `192.0.2.1` on port 17000.**

Once you've added a WiFi profile (see `network wifi profiles add` below) the speaker reboots into station mode and the AP goes away.

### Hardware key combinations on the device itself

| Combo             | Effect                                   | Source |
|-------------------|------------------------------------------|--------|
| `1` + volume-down | Factory reset                            | S2, S3 |
| `2` + volume-down | Setup mode (open WiFi AP at `192.0.2.1`) | S2, S3 |
| `3` + volume-down | Toggle WiFi / Bluetooth                  | S2, S3 |
| `4` + volume-down | Check for software updates               | S2, S3 |

---

## The `network` family — WiFi & interfaces

| Command                                                    | Purpose                                                                                                                   | Availability               | Source |
|------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|----------------------------|--------|
| `network wifi status`                                      | Current SSID, state (e.g. `WIFI_STATION_CONNECTED`), signal strength. Returns XML-like `<WiFiStatus SSID="…" state="…">`. | Wide                       | S2, S3 |
| `network wifi scan [<maxresults>]`                         | Site survey.                                                                                                              | Wide                       | S2     |
| `network wifi profiles info`                               | Lists stored WiFi profiles (passphrases shown encrypted).                                                                 | Wide                       | S2, S3 |
| `network wifi profiles add <ssid> <security> [<password>]` | Adds a WiFi network. `<security>` ∈ `none` \| `wep` \| `wpa_or_wpa2`.                                                     | Wide; setup-mode workhorse | S2, S3 |
| `network wifi profiles clear`                              | Wipes all stored profiles.                                                                                                | Wide                       | S2     |
| `network status`                                           | All interfaces and IP addresses.                                                                                          | Wide                       | S2, S3 |
| `network dhcp`                                             | Current DHCP interface info.                                                                                              | Wide                       | S2     |
| `network mode auto\|wifioff\|wifisetup`                    | Switch radio / setup-AP state.                                                                                            | Wide                       | S2     |

**Example session — adding a network from setup mode (S3):**

```
network wifi profiles add foobarHub wpa_or_wpa2 topsecret
```

The speaker stores the profile, drops the setup AP, and reboots into station mode.

---

## The `key` family — front-panel button emulation

Each `key …` command emulates a press of a physical button on the speaker
or remote. Confirmed working on ST 10 / FW `27.0.6.46330.5043500` (S5);
also visible on the ST 20/300/Wave captures in #221. Different from the
`sys presetkey N p` form (S4) — the `key prefix_N` shape on FW 27 is what
the device's own remote sends.

| Command                         | Effect                                                                                | Source |
|---------------------------------|---------------------------------------------------------------------------------------|--------|
| `key prefix_1` … `key prefix_6` | Triggers preset 1–6 (same as a remote preset press).                                  | S5     |
| `key play`                      | Begin / resume playback.                                                              | S5     |
| `key pause`                     | Pause playback.                                                                       | S5     |
| `key stop`                      | Stop playback (does **not** terminate the underlying stream).                         | S5     |
| `key prev`                      | Restart current song / previous track.                                                | S5     |
| `key next`                      | Next track.                                                                           | S5     |
| `key aux`                       | Toggle Bluetooth / AUX input.                                                         | S5     |
| `key power`                     | Echoes "OK" but no observable effect on FW 27.x — possibly handled at a higher layer. | S5     |

The S4 `bose` script's `sys presetkey N p` form still works, but `key prefix_N` is shorter and matches what the remote already does on FW 27.x.

---

## The `sys` family — system control & service URLs

The `sys` family is the one our migration uses (see §"What we use during migration"). Two distinct sub-syntaxes coexist:

- **Single-token verbs:** `sys reboot`, `sys volume`, `sys power`, etc.
- **`sys configuration <key> <value>` setters** that modify persisted runtime configuration. Used for the four service URLs (margeServerUrl, statsServerUrl, swUpdateUrl, bmxRegistryUrl).

| Command                                     | Purpose                                                                                                                                                                                       | Availability               | Source     |
|---------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------|------------|
| `sys reboot`                                | Restart the device.                                                                                                                                                                           | Wide                       | S2, S6     |
| `sys factorydefault`                        | Reset to factory defaults.                                                                                                                                                                    | Wide                       | S1, S2     |
| `sys ver`                                   | Firmware version string, e.g. `BoseApp version: 27.0.6.46330.5043500 …`.                                                                                                                      | Wide; confirmed on FW 27.x | S1, S5     |
| `sys power`                                 | Toggle power. Confirmed working on older firmware via S2/S4; on FW 27.x ST 10 the response is `OK` but with **no observable effect** — power state may be controlled elsewhere on that build. | Varies                     | S2, S4, S5 |
| `sys playpause`                             | Toggle playback.                                                                                                                                                                              | Wide                       | S2         |
| `sys stop`, `sys pause`                     | Accepted (return `OK`) but **no observable effect** on FW 27.x ST 10 — the working stop/pause path on that firmware is `key stop` / `key pause`.                                              | Wide / no-op               | S5         |
| `sys volume`                                | Print current volume. The S4 script parses the 5th token of the first line.                                                                                                                   | Wide                       | S2, S4, S5 |
| `sys volume <int>`                          | Set absolute volume to `<int>`.                                                                                                                                                               | Wide                       | S5         |
| `sys volume up <n>` / `sys volume down <n>` | Adjust volume by `<n>` (steps, not dB).                                                                                                                                                       | Wide                       | S4         |
| `sys volume <value> updateDisplay`          | Set absolute volume and update the front-panel display.                                                                                                                                       | Wide                       | S2         |
| `sys presetkey <1-6> p`                     | Trigger a preset (`p` = press). Older shape of `key prefix_<N>`.                                                                                                                              | Wide                       | S4         |
| `sys timeout inactivity disable` (or `off`) | Stop the auto-shutoff timer. May need to be sent twice.                                                                                                                                       | Wide                       | S1, S2     |
| `sys configuration` (no args)               | Returns the usage hint `sys configuration <XMLTag> <XMLValue>` — confirms the underlying setter is XML-tag-keyed.                                                                             | FW 27.x                    | S5         |
| `sys configuration bmxRegistryUrl <url>`    | Set the Bose Media eXchange registry URL.                                                                                                                                                     | Wide; **migration**        | S6         |
| `sys configuration statsServerUrl <url>`    | Set the telemetry/stats endpoint.                                                                                                                                                             | Wide; **migration**        | S6         |
| `sys configuration margeServerUrl <url>`    | Set the marge / streaming endpoint.                                                                                                                                                           | Wide; **migration**        | S6         |
| `sys configuration swUpdateUrl <url>`       | Set the software-update endpoint.                                                                                                                                                             | Wide; **migration**        | S6         |

Each `sys configuration` setter is reported by users to return `OK` on success. Wait for that token between commands (S6, `foob61451`).

---

## The `envswitch` family — parallel persistence layer

`envswitch` writes to a separate, lower-level persistence store that **wins on next reboot** if the corresponding `sys configuration` value differs. So our migration writes both — see TELNET-MIGRATION-METHOD.md §2.1.

| Command                                           | Purpose                                                                                       | Source  |
|---------------------------------------------------|-----------------------------------------------------------------------------------------------|---------|
| `envswitch boseurls set <margeUrl> <swUpdateUrl>` | Persist the marge and update URLs. **Two arguments**, in that order.                          | S6      |
| `envswitch accountid set <numeric-id>`            | Equivalent to the HTTP `/setMargeAccount` POST. Used as fallback in our `PairAccount` helper. | S6      |
| `envswitch accountid get`                         | Plausible by symmetry but **not yet confirmed** across firmwares; we probe it best-effort.    | (probe) |

---

## The `getpdo` family — read persisted configuration

`getpdo <selector>` prints the contents of a persisted-data-object. We use it as the verification step after writing URLs.

| Selector                            | Purpose                                                                                                                                                               | Source |
|-------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------|
| `getpdo CurrentSystemConfiguration` | Echoes the resolved URL set, including margeServerUrl/bmxRegistryUrl/statsServerUrl/swUpdateUrl. We grep our targetURL out of this to confirm a successful migration. | S6     |

---

## The `scm` family — service control

`scm` (System Control / Module manager) lets you inspect and restart internal services.

| Command                 | Purpose                                                                                  | Availability   | Source                                                                       |
|-------------------------|------------------------------------------------------------------------------------------|----------------|------------------------------------------------------------------------------|
| `scm list`              | List running services.                                                                   | Older firmware | S1                                                                           |
| `scm restart <service>` | Restart a service by name.                                                               | Older firmware | S1                                                                           |
| `scm uboot_ver`         | Print bootloader version (`U-Boot 2013.01.01-…`). Confirmed working on SA-5 with FW 9.x. | Older firmware | [deborahgu/soundcork#141](https://github.com/deborahgu/soundcork/issues/141) |

---

## Shell-unlock commands

These are the commands that gated SSH access on older firmware. Both have been progressively removed; on FW 27.x they generally do nothing useful.

| Command                     | Purpose                                                                                                                                                                                  | Availability     | Source                                                                           |
|-----------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------|----------------------------------------------------------------------------------|
| `remote_services on`        | Enable SSH on port 22. Volatile (re-enter after reboot). Response: `remote services on`. **Removed in FW 7.x+**.                                                                         | Old              | S1                                                                               |
| `local_services on`         | Alternative enablement; works on some firmware where `remote_services` was removed. SA-5 FW 9.x reports `local services on`, but this alone does not appear to grant SSH on most models. | Old, hit-or-miss | S1, [deborahgu/soundcork#141](https://github.com/deborahgu/soundcork/issues/141) |
| `demo enter` / `mode enter` | Unlocks demo / button-test mode (used historically to recover bricked units).                                                                                                            | Old              | S1                                                                               |

---

## The `ws` and `swupdate` families

| Command          | Purpose                                                                                                 | Availability | Source |
|------------------|---------------------------------------------------------------------------------------------------------|--------------|--------|
| `ws getpresets`  | Returns an XML list of presets — the S4 script parses the `<itemName>…<text>…` blocks to extract names. | Wide         | S4     |
| `swupdate abort` | Cancel a software update in progress.                                                                   | Wide         | S1     |

---

## `help`

Lists the commands available on the running firmware. **Frequently removed** on later firmware — returns `Command not found` on FW 27.x in many of the captures we have. Still worth probing once during preflight: a successful response is a quick way to enumerate what this specific build supports without trial-and-error.

---

## Device codenames (S1)

These show up in `getpdo`, `network status`, and SSH-side hostnames. Useful for matching captures to hardware.

| Codename | Hardware                                       |
|----------|------------------------------------------------|
| `lisa`   | Adapter (older speakers running Bose firmware) |
| `spotty` | SoundTouch 20                                  |
| `rhino`  | SoundTouch 10                                  |
| `mojo`   | SoundTouch 30                                  |
| `taigan` | SoundTouch Portable                            |

---

## Firmware era notes

- **Firmware 1.x–7.x** (S1 era): everything — `help`, `remote_services on`, full `scm`, and an in-shell login prompt. `flarn2006` documents the original Linux insides.
- **Firmware 8.x–14.x** (S2 era): `remote_services on` removed; `network`, `sys`, `envswitch`, `getpdo` still present. `local_services on` works on some Wave/SA-5 models.
- **Firmware 27.x** (S5/S6 era — the long-lived "frozen" build that survived through EOS): `help`, `remote_services on`, and `sys ver` removed in some builds; `sys configuration …` and `envswitch …` confirmed working on ST 10, ST 20, ST 300, Wave III, Wave IV. **This is the firmware our migration targets**. The Portable on more recent firmware drops further commands and is the hardest target.

  S5 enumerated the **top-level command roots** that don't return "Command not found" on a vanilla ST 10 (`rhino`) running `27.0.6.46330.5043500`:

  ```
  key
  net
  sys
  getpdo
  ```

  Notably absent from that probe: `network`, `envswitch`, `scm`, `ws`, `swupdate`, `remote_services`, `local_services`, `demo`, `mode`, `help`. **However**, other captures on the same firmware family (S6, ST 20 / Wave III / Wave IV) accept `envswitch …`, suggesting either per-model variation in the shipped command table or an SSH/role gate the S5 author didn't trip. Implementations that use `envswitch` should treat its absence as a recoverable preflight outcome (we already do).

  `net` is observed as a valid root by S5 but its sub-commands aren't enumerated; it may be a shorthand alias for `network` on FW 27.x ST 10.

---

## What we use during migration

For quick reference, the exact sequence our `pkg/service/setup.migrateViaTelnet` issues, all on the same connection, in this order:

```
sys configuration bmxRegistryUrl <serverURL>/bmx/registry/v1/services
sys configuration statsServerUrl <serverURL>
sys configuration margeServerUrl <serverURL>
sys configuration swUpdateUrl    <serverURL>/updates/soundtouch
envswitch boseurls set <serverURL> <serverURL>/updates/soundtouch
getpdo CurrentSystemConfiguration
```

Plus, when pairing a fresh device whose `:8090/setMargeAccount` is missing or wedged, the helper falls back to:

```
envswitch accountid set <7-digit-id>
```

Reboot is **not** part of these sequences — it stays a user-initiated action via the existing reboot button, which now accepts `?method=telnet|ssh` and sends `sys reboot` when telnet is picked.

---

## Out of scope here, but worth recording

- **Setup-mode WiFi onboarding via 192.0.2.1.** The community uses this to add a fresh device to a network without the Bose app. Our `soundtouch-service` does not currently automate this, but `network wifi profiles add` is the entry point if we ever do.
- **Direct preset / playback control via `sys`.** The S4 `bose` script demonstrates a viable headless remote-control path that does not need our marge emulation at all. Useful as a fallback for tooling on devices that refuse to talk to any cloud.
- **`scm restart <service>`.** Not used today, but a possible recovery primitive on older firmware where a stuck service blocks streaming.
