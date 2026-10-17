---
title: "Decision: Subnet-Scan Discovery"
---
**Status:** accepted (implemented, awaiting reporter confirmation on real bridge-networked hardware)

## Context

`soundtouch-service` discovers speakers via SSDP multicast. In Docker's
default bridge network multicast never reaches the LAN, so discovery returns
nothing and users must add each speaker manually.

## Decision

Clicking "Scan Again" in the Admin UI (`POST /api/setup/discover`) or in the
embedded Player (`POST /api/control/discover`) runs one SSDP search and then
always a subnet scan, even when SSDP finds speakers. The periodic background
sweep (`DISCOVERY_ENABLED`, `DISCOVERY_INTERVAL`) stays SSDP-only and never
scans: a scan probes up to 1024 hosts, and nobody should get that on a timer
without asking for it.

The scan is `discovery.ScanSubnets`: a bounded unicast
`GET http://<ip>:8090/info` over candidate IPv4 subnets. The smallest option
was chosen: no new dependencies, no ARP/ICMP tricks, no new daemon. Accepted
devices reuse `handleDiscoveredDevice`, `mergeOverlappingDevices` and
`notifyDevicesChanged`.

### Existing installs

Existing installs keep `discovery_enabled` from `settings.json`, and their
periodic sweep behaves exactly as before (SSDP only). The scan is new and only
runs on a click, so no migration is needed.

### Scheduled scan

`DISCOVERY_SCAN_INTERVAL` (also on the Settings page) runs the same scan on a
timer, independent of the periodic SSDP sweep. It defaults to `0` (off).
At least `1h` is recommended but not enforced; a shorter value logs a hint.
Known speakers are re-checked at their stored IPs anyway, so a frequent scan
only finds new speakers sooner, and a /24 sweep every few minutes is what
router intrusion detection tends to flag. The timer restarts at service
start, after each scan and whenever the interval changes, so enabling it
never fires a scan right away.

Existing installs have no `discovery_scan_interval` key in `settings.json`
and read it as off (or as the env value, if one is set), so they need no
migration either.

### Deployment modes

- `public-network`: only explicit `DISCOVERY_SUBNETS` are scanned. The
  interface-derived `/24` there is the hosting provider's network, and probing
  it looks like a port scan.
- `on-device`: the scan runs with 8 parallel probes instead of 64, because the
  speaker's CPU also serves its own BMX and marge traffic. A click-triggered
  scan keeps no state, so it does not fall under the on-device opt-in rule for
  caches and persisted data.

## Security and safety

- Targets come from `DISCOVERY_SUBNETS` or from RFC1918 hints only. The
  clicking client's hint is the address resolved by the client-IP middleware:
  `X-Forwarded-For` only from a trusted proxy, otherwise the socket peer, so a
  client cannot steer the scan at arbitrary networks with a forged header.
  Explicit config may name any IPv4 CIDR. CIDRs in docs and tests (such as
  `203.0.113.0/24`, RFC 5737) are illustrative; supply your own real LAN CIDR.
- Hard caps, enforced even for oversized caller options: 1024 unique hosts
  before scheduling, 64 workers, 400 ms dial default, 2 s per probe default,
  60 s per pass, context cancellation throughout.
- The host budget is shared round-robin across prefixes, so a large first
  prefix cannot crowd out a smaller one listed after it. A truncated scan logs
  at Warn.
- Responses are untrusted: HTTP 200 only, 64 KiB body limit (a real `/info` is
  under 2 KB), strict XML root `info` with non-blank `deviceID`, no redirects,
  no proxy, and nothing after the root element except whitespace and
  comments. Logs carry duration, number of hosts checked and the prefixes
  scanned, not raw response content.
- These guarantees cover the initial scan probe only. Accepted speakers then go
  through `handleDiscoveredDevice`, whose inherited second `/info` fetch
  (`GetLiveDeviceInfo`) follows redirects, honours environment proxies, and
  has no body limit or caller context.
- One sweep at a time (atomic slot); no lock is held during network I/O.
- Two manual triggers scan: `POST /api/setup/discover` (and its alias
  `POST /setup/discover`), admin-gated only when admin authentication is
  enabled (`BasicAuthAdmin` lets requests through otherwise), and the embedded
  Player's `POST /api/control/discover`, behind the existing Player gate.
- A Player click that finds the slot taken publishes no status: the active
  sweep owns "discovering", and the rejected click never reports completed.
- The Player's 90 s deadline budgets the search phases. The device list is
  checked against it before each device is handled and the Player is told
  "failed" if it expired, but a device lookup already in flight may still
  finish afterwards (it has its own 5 s timeout).

## Consequences

A /24 costs up to 254 short connections per click. Speakers on non-RFC1918 or
unrelated subnets need `DISCOVERY_SUBNETS`. Not verified against real
bridge-networked hardware. Health diagnostics for this feature are
deliberately out of scope.
