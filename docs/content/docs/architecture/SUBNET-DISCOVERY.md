---
title: "Decision: Subnet-Scan Discovery Fallback"
---
**Status:** accepted (implemented, awaiting reporter confirmation on real bridge-networked hardware)

## Context

`soundtouch-service` discovers speakers via SSDP multicast. In Docker's
default bridge network multicast never reaches the LAN, so discovery returns
nothing and users must add each speaker manually.

## Decision

When SSDP returns zero devices (or errors), or an admin asks via
`POST /setup/discover?scan=subnet`, run `discovery.ScanSubnets`: a bounded
unicast `GET http://<ip>:8090/info` over candidate IPv4 subnets.
The smallest option was chosen: no new dependencies, no ARP/ICMP tricks, no
new daemon. Accepted devices reuse `handleDiscoveredDevice`,
`mergeOverlappingDevices` and `notifyDevicesChanged`. Periodic discovery that
SSDP satisfies never scans.

## Security and safety

- Targets come from `DISCOVERY_SUBNETS` or from RFC1918 hints only. The admin
  request hint uses `RemoteAddr`, never forwarded headers, so a client cannot
  steer the scan at arbitrary networks. Explicit config may name any IPv4 CIDR.
- Hard caps: 1024 unique hosts before scheduling, 64 workers, 400 ms dial,
  2 s per probe, 60 s per pass, context cancellation throughout.
- Responses are untrusted: HTTP 200 only, 1 MiB body limit, strict XML root
  `info` with non-blank `deviceID`, no redirects, no proxy. Logs carry counts
  and sanitised values, not raw response content.
- One sweep at a time (atomic slot); no lock is held during network I/O.
- Only the admin-gated `/setup/discover` can trigger a forced scan.

## Consequences

A /24 costs up to 254 short connections per sweep that finds no speakers.
Speakers on non-RFC1918 or unrelated subnets need `DISCOVERY_SUBNETS`. Not
verified against real bridge-networked hardware. Health diagnostics for this
feature are deliberately out of scope.
