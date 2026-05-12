# What a SoundTouch speaker does during factory reset

Observed live on ST10 firmware `27.0.6.46330.5043500` (build `epdbuild.trunk.hepdswbld04.2022-08-04`) on 2026-05-12, by running `soundtouch-cli setup factory-reset` and tailing the speaker's `logread` over SSH. The trace is preserved at `_/logs/factory-reset.txt` for reference.

## Sequence

1. **Telnet receives `sys factorydefault`.** The diagnostic shell on port 17000 accepts the command and acknowledges. Some firmwares close the socket as part of the reboot — our CLI's `setup factory-reset` tolerates that as success.

2. **Speaker DELETEs itself from its marge account.** Before wiping anything, the firmware does:
   ```
   [MargeStateAssociated] HandleRemoveDeviceRequest - Removing this device from the user's Marge account
   [MargeClient]          RemoveDevice calling Marge Server with https://streaming.bose.com/streaming/account/{accountId}/device/{deviceId}
   [MargeClient]          RemoveDeviceCB - Device removed from the user's Marge account
   [MargeStateAssociated] HandleRemoveDeviceRequestSuccessCB, Marge returned: {"ok": true}
   ```
   AfterTouch already handles this — `HandleMargeRemoveDevice` (`pkg/service/handlers/handlers_marge.go:633`) routed via `r.Delete("/device/{device}", …)` in `cmd/soundtouch-service/main.go:955`. The handler calls `marge.RemoveDeviceFromAccount(s.ds, account, device)` and prunes the device from the datastore.

3. **Speaker notifies its LAN peers.** Two HTTP POSTs to each known peer at `:8090/notification`:
   ```
   [NotificationSender] SendNotifyLisas_: URL: >>http://192.168.123.122:8090/notification<<, m_msgdata.size(58)
   [SimpleURLFetcher]   multipart/form-data text/xml
   ```
   ~58 bytes of `multipart/form-data` carrying `text/xml`. "Lisas" is the firmware's internal term for LAN peers (devices on the same account on the same network segment). AfterTouch is **not** on this path — it's pure peer-to-peer over the LAN. Peers presumably refresh their account info as a result.

4. **Local state teardown.** Bluetooth pairings cleared (`BTRemoteDeviceAccess::ClearPrevPairedList`), zone/group state torn down, all source proxies disconnected (`STSAccountProxy::Disconnect Requested` × many).

5. **Persistence cleanup.** Logs, core dumps wiped (`FactoryDefault: Clearing the CoreDump and BoseLogs … rm -rf /mnt/nv/BoseLog/*`). Notably **NOT wiped**: `/mnt/nv/aftertouch.resolv.conf`, `/mnt/nv/rc.local`'s Aftertouch hook, and `/mnt/nv/BoseApp-Persistence/1/SystemConfigurationDB.xml`. The reset only touches log directories and account-specific persistence under the same `/mnt/nv/BoseApp-Persistence/1/` tree.

6. **Reboot into setup mode.** Speaker drops Wi-Fi, comes back as its own AP `Bose SoundTouch XXXX` on 192.0.2.1.

## Implications for migration ordering

The DELETE in step 2 only reaches AfterTouch if the speaker's `margeURL` already points at AfterTouch *at the moment of reset*. A speaker still pointing at `streaming.bose.com` sends it into the void → AfterTouch keeps a stale `account/{id}/device/{id}` entry until someone manually prunes it.

Therefore for a clean datastore lifecycle on an already-Bose-paired speaker:

1. Migrate URLs first (`setup migrate --method=resolv` or `--method=telnet`).
2. Reboot to apply.
3. Factory reset.
4. Re-provision.

`soundtouch-cli setup plan --reset` currently runs factory-reset first (optimal for already-on-AfterTouch speakers); both `setup plan --reset` and `setup factory-reset` print a one-line note explaining the ordering tradeoff so users can pick the right sequence for their starting state.

## Implications for AfterTouch behaviour

- The DELETE handler is already correct; no changes needed.
- AfterTouch is invisible to the LAN-peer notification step — that's just LAN HTTP between speakers.
- If you build a "consolidate account" / "migrate fleet" feature later, the peer-notification channel is the propagation path the firmware uses internally; AfterTouch doesn't need to do anything analogous.
- The persistence layer at `/mnt/nv/` is **factory-reset-resistant**. Our DNS-redirect migration (`setup migrate --method=resolv`) writes there specifically so AfterTouch routing survives a reset. This is intentional — the user can factory-reset a speaker freely without re-running migration.

## Open questions

- Are there other peer endpoints the firmware POSTs to besides `:8090/notification`? Worth checking on a 3-speaker LAN.
- Does the `:8090/notification` payload format match the format used for play-as-notification audio pushes, or is it a distinct message shape? The "size(58)" byte count is too small for an audio URL but big enough for an XML envelope with an event type.

If you want either of these answered, capture two synchronised `logread -f` streams from two LAN speakers while one is being reset.

## Runbook — reset & re-provision an ST10 on AfterTouch

End-to-end command sequence used during the 2026-05-12 bare-pairing experiment, recorded verbatim from the test session. Replace IPs, SSID, password, service URL, and account ID with your own. Two manual Wi-Fi switches happen between `factory-reset` and `wifi-push` (host joins the speaker's AP) and again between `wifi-push` and `wait-online` (host re-joins home Wi-Fi).

```bash
# === 1. Reconnaissance — confirm what state the speaker is in before touching it. ===

# Identity, network, sources, presets.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup inspect

# Green/red status across every migration axis (SSH, telnet, CA, pairing, …).
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup verify \
  --service-url=https://soundtouch.fritz.box

# What `setup plan --reset` would recommend, so you can preview the sequence.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup plan \
  --service-url=https://soundtouch.fritz.box --reset


# === 2. Reset and Wi-Fi re-provisioning. ===

# Tell the speaker to wipe itself. Speaker drops Wi-Fi and reboots into AP mode.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup factory-reset

# Manual: switch this host to the speaker's setup AP.
#   macOS: networksetup -setairportnetwork en0 "Bose SoundTouch XXXX"

# Poll 192.0.2.1:8090/info until the speaker answers (interval=2s, timeout=5m).
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup wait-ap

# Push home Wi-Fi credentials. NOTE the single-quoted password: zsh expands `!`
# inside double quotes as history-expansion and will refuse the command.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup wifi-push \
  --ssid="wifi-name" --pass='a.secure!password'

# Manual: switch host back to home Wi-Fi.
#   macOS: networksetup -setairportnetwork en0 "wifi-name" 'a.secure!password'

# mDNS-poll for the speaker on the home network, matched by deviceID suffix
# (which survives the reset since it's the MAC). Returns the new IP.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup wait-online --match=536A98


# === 3. Clock, migrate, pair. From here on use the new IP wait-online reported. ===

# Set the speaker's wall-clock. `clock set --time=now` fails on FW 27;
# `clock now` is the working subcommand.
go run ./cmd/soundtouch-cli --host 192.168.123.123 clock now

# Reboot to clear any half-initialized resolver / NTP state from the wifi-push flap.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup reboot

# Apply DNS-redirect migration: routes *.bose.com to AfterTouch and installs its CA.
# Idempotent; safe to re-run.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup migrate \
  --service-url=https://soundtouch.fritz.box --method=resolv

# Reboot again so the envswitch parallel-persistence layer and the resolv hook
# both take effect on the next boot.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup reboot

# Pair the device with an AfterTouch account — bare experiment variant.
# Drop --mode=bare and add --name=… / --language=… for the full state-machine variant.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup pair \
  --mode=bare --account=1111111 --service-url='https://soundtouch.fritz.box'


# === 4. Verify. ===

# Reboot to verify persistence survives.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup reboot

# Snapshot the result. margeAccountUUID should still equal --account, and Sources
# should list ~14 entries (TUNEIN, RADIO_BROWSER, LOCAL_INTERNET_RADIO,
# SPOTIFY slots, AIRPLAY, etc.) materialized by the firmware.
go run ./cmd/soundtouch-cli --host 192.168.123.123 setup inspect
```

Total wall-clock for the above on this hardware: roughly 5 minutes including the two manual Wi-Fi switches and three reboots.
