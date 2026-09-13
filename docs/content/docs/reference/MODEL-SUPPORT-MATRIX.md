---
title: "Model Support Matrix"
---
A living record of how individual SoundTouch models behave with AfterTouch,
built up from things actually observed on hardware.

**This table only claims what someone has verified.** Anything not tested is
marked `?` rather than inferred from a similar-looking model. Bose used
several different chassis designs across the SoundTouch line, and at least
one behaviour (LAN reachability, below) differs between them in a way that is
invisible from the outside. If you have a model that isn't filled in yet,
[the commands below](#how-to-fill-in-a-row) produce everything a row needs.

## What the columns mean

- **variant / moduleType**: the speaker's own identifiers, straight out of
  `/info`. `variant` is Bose's internal codename for the product; `moduleType`
  distinguishes chassis generations (`scm` and `sm2` are the two seen so far).
- **BCO**: whether the board carries a BCO co-processor (Bose's internal name
  for the SMSC Wi-Fi/Bluetooth combo chip that also handles AirPlay). Bose's
  own `has-bco` helper on the device is simply
  `[ "$(cat /proc/module_type)" = scm ]`.
- **`:8000` from LAN**: whether AfterTouch's own port is reachable from
  another machine on the network *without* any workaround.
- **Entry port**: when `:8000` isn't reachable, the port AfterTouch redirects
  to itself so the admin UI still works. See
  [LAN access on co-processor chassis](#lan-access-on-co-processor-chassis).

## Matrix

| Model               | variant  | moduleType | BCO | On-device install | `:8000` from LAN | Entry port      | Evidence                                                                                                   |
|---------------------|----------|------------|-----|-------------------|------------------|-----------------|------------------------------------------------------------------------------------------------------------|
| SoundTouch 20       | `spotty` | `scm`      | yes | works             | ✗ blocked       | `17008`         | verified on hardware 2026-08-16 (FW 27.0.6), redirect survives reboot                                      |
| SoundTouch 10       | ?        | ?          | ?   | reported working  | ?                | ?               | not tested for LAN reachability                                                                            |
| SoundTouch 30       | `mojo`   | ?          | ?   | reported working  | ✗ reported      | `17008`         | reported by a user (Series I, v0.112.0): redirect works, see [discussion 490][d490]                        |
| SoundTouch Portable | `taigan` | ?          | ?   | reported working  | ✗ reported      | `17008` or `82` | reported by a user (Series I, FW 27.0.6): both work, `82` stable across reboot, see [discussion 490][d490] |
| Wave / SA-4         | ?        | ?          | ?   | ?                 | ?                | ?               | not tested                                                                                                 |

Not every SoundTouch shares one firmware image, so treat a `?` as genuinely
unknown. In particular, do not assume a model is unaffected just because it is
newer or older than a model that is.

Rows marked *reported* come from users' own tests and have not been reproduced
by the maintainers yet.

[d490]: https://github.com/gesellix/Bose-SoundTouch/discussions/490

## LAN access on co-processor chassis

On chassis with a BCO co-processor, inbound LAN traffic reaches the speaker's
main Linux SoC only for a fixed set of Bose's *own* service ports. That list
appears to be compiled into the co-processor's firmware, and AfterTouch's
`:8000` is not on it, so a connection attempt never arrives at the SoC at
all. On a verified ST20, `tcpdump -i eth0` on the speaker recorded **zero
packets** for `:8000` while Bose's `:8090`, `:8091`, `:8200`, `:82`, `:8080`
and `:17000` all answered normally from the same client.

This is not a firewall, and not something AfterTouch can fix by binding
differently: the service already listens on `0.0.0.0:8000`, and the speaker's
`iptables` is empty (there is no `nft` or `ebtables` at all).

The on-device installer works around it by redirecting one of the relayed
ports to AfterTouch. **Credit for this technique goes to the
[STR / SoundTouch Reborn](https://github.com/JRpersonal/streborn) project**,
which documented and shipped it first (their agent uses the same entry port
for the same reason); finding their prior art is what turned this from an
apparent hardware dead end into a one-line fix:

```
iptables -t nat -I PREROUTING 1 ! -i lo -p tcp --dport 17008 -j REDIRECT --to-ports 8000
```

`17008` is Bose's `SoftwareUpdate` listener. Its cloud service no longer
exists, so taking over its inbound traffic costs nothing in practice. Only
external traffic is matched (`! -i lo`), so anything running on the speaker
still reaches AfterTouch on `:8000` exactly as before.

The rule is re-applied by the init script on every start, so it survives
reboots (confirmed on the ST20) without any background watchdog. It is
removed again on `stop` and on uninstall.

The redirect is applied automatically on chassis that need it, and configured
via `AFTERTOUCH_LAN_PORT` in `/opt/aftertouch/aftertouch.conf`:

| Value      | Effect                                                          |
|------------|-----------------------------------------------------------------|
| `auto`     | *(default)* redirect only where the co-processor blocks `:8000` |
| `none`     | never redirect; use an SSH tunnel instead                       |
| *(a port)* | always redirect that inbound port to AfterTouch                 |

Two caveats worth knowing:

- **Account linking still prefers the SSH tunnel.** Spotify only accepts
  `https://` or *loopback* OAuth redirect URIs, so `http://localhost:8000`
  through a tunnel works for linking where a plain LAN address does not.
- **The `streborn` project defaults to the same port** for the same reason. If
  you run both on one speaker, change `AFTERTOUCH_LAN_PORT`.
- **`17008` may be briefly unreliable on some models (reported, not
  reproduced).** On a SoundTouch Portable, a user saw the UI on `17008`
  intermittently stop responding while Bose's `Shepherd` supervisor restarted
  `SoftwareUpdate`, even though the `iptables` rule stayed in place. Port `82`
  (`PtsServer`) was stable across a full reboot. If you see this, set
  `AFTERTOUCH_LAN_PORT=82` and restart AfterTouch.

### Bose ports by variant

Listening Bose services, as reported on the speaker itself. Any of them is a
candidate entry port, but a port whose service is still in use (such as
`8090`, the SoundTouch API) must not be redirected.

| Port    | Service (`taigan`, SoundTouch Portable) | Notes                                         |
|---------|-----------------------------------------|-----------------------------------------------|
| `82`    | `PtsServer` ("xfer")                    | reported stable as an entry port              |
| `8080`  | `WebServer`                             | speaker WebSocket; in use                     |
| `8090`  | `BoseApp`                               | SoundTouch API; in use                        |
| `8200`  | `STSCertified`                          |                                               |
| `17000` | `CLIServer`                             | telnet CLI; used for migration and SSH unlock |
| `17004` | `BoseApp`                               |                                               |
| `17005` | `BoseApp`                               |                                               |
| `17008` | `SoftwareUpdate`                        | default entry port; periodically restarted    |
| `17018` | `LegacyProduct`                         | untested as an entry port                     |

Source: [discussion 490][d490] (reported on FW 27.0.6). Other variants may
differ.

## How to fill in a row

Run these from a machine on the same network (replace the address), then open
an issue or PR with the output:

```bash
# variant, moduleType, and whether an SCM/SMSC component is listed
curl -s http://<speaker-ip>:8090/info

# is AfterTouch's own port reachable directly? (only meaningful once
# AfterTouch is installed on the device)
curl -v --max-time 5 http://<speaker-ip>:8000/health

# which Bose ports the chassis relays at all
for p in 82 8080 8090 8091 8200 17000 17008 17018; do
  printf '%s: ' "$p"
  curl -s -o /dev/null -w '%{http_code}\n' --max-time 3 "http://<speaker-ip>:$p/" || echo unreachable
done
```

And on the speaker itself, if you have SSH access:

```bash
has-bco; echo "has-bco exit status: $?"   # 0 = BCO co-processor present
cat /proc/module_type /proc/variant
```
