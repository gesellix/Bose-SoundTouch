# Experiment: Does bare `setMargeAccount` work outside the SETUP bracket?

## Why we are doing this

Our captured pairing flow (`docs/reference/DEVICE-PAIRING-FLOW.md`) shows the official Bose app always sends `setMargeAccount` *inside* a `SETUP_START` → `SETUP_ENTER` → `SETUP_LEAVE` state-machine bracket over WebSocket. The question this experiment answers:

> If we open a WebSocket to a factory-reset speaker and send **only** `setMargeAccount` — no surrounding setupState messages — does the device honor it and write its persistence files (`SystemConfigurationDB.xml`, `Sources.xml`) cleanly?

The answer determines the shape of `PairAccount`:

- **If YES:** `PairAccount` becomes uniform: WebSocket-first, HTTP `/setMargeAccount` second, telnet `envswitch accountid set` third. One function, one ordering, all callers.
- **If NO:** WebSocket pairing is only meaningful inside the full state machine. Factory-reset path uses the state machine; re-pair path keeps today's HTTP→telnet ordering.

## Preconditions

- A SoundTouch speaker that has been **factory-reset** and joined to the test Wi-Fi.
- Speaker reachable on `:8090` (HTTP API) and `:8080` (WebSocket).
- Speaker's runtime marge URL already points at AfterTouch (run the existing telnet URL rewrite first — otherwise the device's downstream POST will land on the dead Bose cloud and we will not be able to distinguish "WS message refused" from "downstream cloud failed").
- A free 7-digit account ID — for example, generated via `setup.GenerateAccountID(nil)`.

## Step 0 — Baseline

```bash
DEVICE=192.168.x.x
curl -s http://$DEVICE:8090/info     | xmllint --format -
curl -s http://$DEVICE:8090/sources  | xmllint --format -
curl -s http://$DEVICE:8090/presets  | xmllint --format -
```

Record:

- `<margeAccountUUID>` — expect empty on a factory-reset device.
- `<margeURL>` — expect the AfterTouch URL (preflight already applied).
- `<sources>` — expect a minimal list.
- `<presets>` — expect `<presets/>`.

## Step 1 — Send bare `setMargeAccount` over WebSocket

Build the CLI once:

```bash
make build
```

Then run the bare path against the speaker:

```bash
DEVICE=192.168.x.x
./build/soundtouch-cli setup pair --host=$DEVICE --account=1234567 --mode=bare
```

What it does:

1. Reads `/info` to discover `deviceID`, logs the pre-state.
2. Opens a WebSocket to `$DEVICE:8080` with the `gabbo` subprotocol.
3. Sends exactly one frame — the `setMargeAccount` envelope — **without** any preceding `SETUP_START`/`SETUP_ENTER`.
4. Reads frames for up to `--step-timeout=8s` (configurable), looking for an ack referencing our `requestID`.
5. Closes the WebSocket, waits 2 s, re-reads `/info`, prints whether `margeAccountUUID` now equals our supplied ID.

The exact frame sent (built by `setup.SetupSession.SetMargeAccount`):

```xml
<msg><header deviceID="DEVICE_ID" url="setMargeAccount" method="POST"><request requestID="1"/></header><body>
  <PairDeviceWithAccount>
    <accountId>1234567</accountId>
    <userAuthToken>Bearer aftertouch</userAuthToken>
  </PairDeviceWithAccount>
</body></msg>
```

Outcomes the CLI will surface:

- `Device accepted bare pairing.` (post-`/info` shows our ID) → **bare path works**.
- `setMargeAccount: device rejected setMargeAccount: …` → device returned an `<error>` body → **bare path refused explicitly**.
- `setMargeAccount: await ack for setMargeAccount: …` (timeout or EOF) → **bare path refused silently**.
- `Device did NOT persist the pairing — bare path likely refused silently.` → ack received but persistence didn't follow.

## Step 2 — Record outcome

After step 1 (regardless of which branch happened):

```bash
sleep 2
curl -s http://$DEVICE:8090/info | grep margeAccountUUID
```

| Observed result                                                              | Verdict                     |
|------------------------------------------------------------------------------|-----------------------------|
| `<margeAccountUUID>1234567</margeAccountUUID>` appears                       | **YES** — Option 1 wins     |
| `<margeAccountUUID></margeAccountUUID>` still empty, no error frame received | Refused silently → **NO**   |
| Error frame returned (e.g. `<error name="UNSUPPORTED_STATE"/>`)              | Refused explicitly → **NO** |
| Device drops the WebSocket connection without replying                       | Refused → **NO**            |

If verdict is YES, also verify the device wrote persistence cleanly. Reboot the device, then:

```bash
ssh root@$DEVICE 'cat /mnt/nv/BoseApp-Persistence/1/SystemConfigurationDB.xml'
ssh root@$DEVICE 'cat /mnt/nv/BoseApp-Persistence/1/Sources.xml'
curl -s http://$DEVICE:8090/info | grep margeAccountUUID
```

The UUID must still be present after reboot, and `SystemConfigurationDB.xml` must contain `<AccountUUID>1234567</AccountUUID>`. If it survives reboot, **YES** is confirmed.

## Step 3 — Control: full state machine

Factory-reset the same speaker again and run the full state machine — the same CLI, `--mode=full`:

```bash
./build/soundtouch-cli setup pair --host=$DEVICE --account=1234567 --mode=full
```

This drives `setup.Manager.ExecuteInitPlan` with `SkipURLRewrite=true`, which runs:

```
SETUP_START
SETUP_IDENTIFY_DEVICE_ENTER
language sysLanguage=2
SETUP_ENTER
SETUP_IDENTIFY_DEVICE_LEAVE
setMargeAccount …
SETUP_LEAVE
pushCustomerSupportInfoToMarge
```

The CLI logs every step with status. Confirm `/info`, persistence, and reboot-survival checks pass. If the bare path failed but the full path succeeds, the SETUP bracket is load-bearing — a follow-up bisect (e.g. `SETUP_START + setMargeAccount + SETUP_LEAVE` only) tells us *which* surrounding messages the firmware actually requires.

## Full reset-and-rebuild loop

Once the bare/full question is decided, the loop for repeated experiments is:

```bash
# 0. Speaker is currently on home Wi-Fi at $DEVICE.
#    Capture deviceID-suffix + current SSID first so wait-online and
#    wifi-push have the right inputs.
./build/soundtouch-cli setup inspect --host=$DEVICE
./build/soundtouch-cli setup factory-reset --host=$DEVICE

# 1. Manually switch this host to the speaker's AP (Bose SoundTouch XXXX).
#    macOS: networksetup -setairportnetwork en0 "Bose SoundTouch XXXX"

./build/soundtouch-cli setup wait-ap
./build/soundtouch-cli setup wifi-push --ssid="$HOME_SSID" --pass="$HOME_PASS"

# 2. Manually switch this host back to home Wi-Fi.

./build/soundtouch-cli setup wait-online --match=DE4803          # deviceID suffix from /info before reset
# (note the new IP from the "Speaker discovered" line)

NEW_IP=192.168.x.y
./build/soundtouch-cli setup migrate --host=$NEW_IP --service-url=http://aftertouch.local:8000   # default --method=telnet

# Optional, if you want the DNS-redirect path instead of (or alongside) telnet envswitch:
#   1. ./build/soundtouch-cli setup ssh-check --host=$NEW_IP            # USB-stick procedure if 22 is closed
#   2. ./build/soundtouch-cli setup install-ca --host=$NEW_IP --service-url=http://aftertouch.local:8000
#   3. ./build/soundtouch-cli setup migrate --host=$NEW_IP --service-url=http://aftertouch.local:8000 --method=resolv
./build/soundtouch-cli setup pair --host=$NEW_IP --mode=bare   # or --mode=full
```

The two manual lines are user-side Wi-Fi switches that can't be automated portably. The `wait-ap` and `wait-online` subcommands poll for the corresponding network state, so timing them is hands-off.

## Recording the result

Append to this file under `## Results`:

```
- Date: YYYY-MM-DD
- Firmware: 27.x.x
- Model: ST10 / ST20 / ST30 / ST300
- Bare setMargeAccount accepted: yes/no
- Persistence written: yes/no
- Survives reboot: yes/no
- Notes: ...
```

One row per device tested. Once two devices on different firmware confirm the same verdict, we treat it as decided.

## Results

- Date: 2026-05-13
- Firmware: 27.0.6.46330.5043500 (build epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29)
- Model: SoundTouch 10 (deviceID A81B6A536A98)
- Bare setMargeAccount accepted: **yes** — pre-/info margeAccountUUID="" → post-/info margeAccountUUID="1111111"
- Persistence written: **yes** — device materialized 14-entry Sources.xml on its own
- Survives reboot: **yes** — `setup inspect` after `setup reboot` shows margeAccountUUID still 1111111
- Notes: After bare pairing, the speaker did the full post-pairing handshake against AfterTouch (POST /streaming/support/power_on, GET /streaming/sourceproviders, GET /streaming/account/{id}/full, group/, provider_settings). No SETUP_START/SETUP_ENTER/SETUP_LEAVE was ever sent. Verdict: bare path is functionally equivalent to the full state machine on this firmware.

### Implication for the codebase

- `pkg/service/setup/setup_session.go` keeps the full state machine for completeness, but
- `pkg/service/setup/init_plan.go`'s default could be simplified to "send setMargeAccount only" once we have one more confirming run on a different model.
- The OCT issue-167 SSH-XML seeding workaround is **not required**.

### Appendix — SystemConfigurationDB.xml comparison

Post-experiment we compared the device-written `/mnt/nv/BoseApp-Persistence/1/SystemConfigurationDB.xml` from the bare-paired speaker against two SSH backups taken from speakers originally paired by the official Bose app (account 3230304, devices `A_Sound_Machine` and `Sound_Machinechen`). The diff is much smaller than expected — only two fields differ, and neither is set by the pairing protocol itself:

| Field                    | Bare-paired (1111111)                      | Real-Bose-paired (3230304) | Set by                                                                                                    |
|--------------------------|--------------------------------------------|----------------------------|-----------------------------------------------------------------------------------------------------------|
| `DeviceName`             | `Bose SoundTouch 536A98` (factory default) | `Sound Machinechen`        | `name` WS message — only sent in `--mode=full`                                                            |
| `AccountAssociatedEMail` | empty                                      | **empty**                  | Never populated, even by real Bose                                                                        |
| `AccountUUID`            | `1111111`                                  | `3230304`                  | `setMargeAccount` — both paths set it                                                                     |
| `Locale`                 | empty                                      | **empty**                  | Never populated, even by real Bose                                                                        |
| `acctMode`               | `global`                                   | `global`                   | Firmware-default; no protocol path observed to change it                                                  |
| `isMultiDeviceAccount`   | `false`                                    | `true`                     | Derived from the cloud's `/streaming/account/{id}/full` response — count of `<devices>` > 1 flips it true |
| `margeAuthServerToken`   | empty                                      | **empty**                  | Never populated, even by real Bose                                                                        |
| `Password`               | (encrypted blob)                           | (encrypted blob)           | Device-local key; expected to differ                                                                      |

Three of the seven informational fields are empty even after a real-Bose pairing — the firmware simply doesn't populate `AccountAssociatedEMail`, `Locale`, or `margeAuthServerToken` from the pairing flow. So bare pairing isn't missing any field that real pairing fills.

The two genuinely different fields:

- **`DeviceName`** — pure UX. Settable any time post-pair via `name` POST (`soundtouch-cli name set --value=…`) or by sending the `name` WS message during `--mode=full` pairing.
- **`isMultiDeviceAccount`** — not a pairing concern. It's derived from the account's device count on AfterTouch's side; flips to `true` automatically the next time the speaker refreshes account state if a second speaker has been paired to the same account.

So the experiment's YES verdict stands unqualified: bare `setMargeAccount` produces a `SystemConfigurationDB.xml` functionally equivalent to one written by the official pairing flow.
