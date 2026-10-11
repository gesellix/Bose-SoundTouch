---
title: "Decision: Subnet-Scan Discovery Fallback"
---
**Status:** accepted (implemented, awaiting reporter confirmation on real bridge-networked hardware)

## Context

`soundtouch-service` discovers speakers via SSDP multicast. In Docker's
default bridge network multicast never reaches the LAN, so discovery returns
nothing and users must add each speaker manually.

## Decision

Discovery is manual by default: a fresh install does nothing until someone
clicks "Scan Again" in the Admin UI (`POST /api/setup/discover`) or in the
embedded Player (`POST /api/control/discover`). Both run one SSDP search and
then always scan, even when SSDP finds speakers. Opt-in periodic discovery
(`DISCOVERY_ENABLED=true`) scans only when SSDP returns zero devices (or
errors). The scan is `discovery.ScanSubnets`: a bounded
unicast `GET http://<ip>:8090/info` over candidate IPv4 subnets.
The smallest option was chosen: no new dependencies, no ARP/ICMP tricks, no
new daemon. Accepted devices reuse `handleDiscoveredDevice`,
`mergeOverlappingDevices` and `notifyDevicesChanged`. Opt-in periodic
discovery that SSDP satisfies never scans.

## Security and safety

- Targets come from `DISCOVERY_SUBNETS` or from RFC1918 hints only. The admin
  request hint uses `RemoteAddr`, never forwarded headers, so a client cannot
  steer the scan at arbitrary networks. Explicit config may name any IPv4 CIDR. CIDRs in docs and tests (such as
  `203.0.113.0/24`, RFC 5737) are illustrative; supply your own real LAN CIDR.
- Hard caps, enforced even for oversized caller options: 1024 unique hosts
  before scheduling, 64 workers, 400 ms dial default, 2 s per probe default,
  60 s per pass, context cancellation throughout.
- Responses are untrusted: HTTP 200 only, 1 MiB body limit, strict XML root
  `info` with non-blank `deviceID`, no redirects, no proxy, and nothing after the
  root element except whitespace and comments. Logs carry duration, number of
  hosts checked and the prefixes scanned, not raw response content.
- These guarantees cover the initial scan probe only. Accepted speakers then go
  through `handleDiscoveredDevice`, whose inherited second `/info` fetch
  (`GetLiveDeviceInfo`) follows redirects, honours environment proxies, and
  has no body limit or caller context.
- One sweep at a time (atomic slot); no lock is held during network I/O.
- Two manual triggers force a scan: `POST /api/setup/discover` (and its alias
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

A /24 costs up to 254 short connections per sweep that finds no speakers.
Speakers on non-RFC1918 or unrelated subnets need `DISCOVERY_SUBNETS`. Not
verified against real bridge-networked hardware. Health diagnostics for this
feature are deliberately out of scope.
