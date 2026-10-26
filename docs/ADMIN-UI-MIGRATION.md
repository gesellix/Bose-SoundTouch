# Admin UI migration (#635)

This document records the phase-1 architecture and review gates for replacing
`pkg/service/handlers/web/js/script.js` tab by tab with htm/Preact. It is not a
security approval for hosting the admin area on the public Internet.

## Scope and boundary

The existing `/admin` page and nine tab containers remain while individual tabs
are migrated. The initial PR moves the read-only Logs tab and establishes the
minimum module-loading/test infrastructure; Devices and device details are next.
This deliberately differs from the initial suggestion in the issue to start
with Devices, so the more sensitive tab does not also have to establish the
runtime foundation. APIs under `/api/setup` and `/api/mgmt` stay unchanged.
No browser build step is added: Go builds from embedded, vendored assets.

Do not share player components or styles during phase 1, per the maintainer's
answer in #635. Sharing the vendored library files through a new independent
static route is a separate *unresolved* decision; the existing
`/app/static/lib/` route is subject to the player's enable/disable gate.

## Open decisions (required before the first implementation PR)

1. Should the admin bundle have its own `/web/lib/` library copy populated
   by `make update-static-deps`, or should the player and admin share a new,
   deliberately ungated vendor-asset route? An admin page must still load
   when the player is disabled.
2. What is the cache-invalidation policy for the admin ES module graph?
   Existing cache-busting covers only `script.js` and `style.css`. A version
   on the entry module alone cannot invalidate already-cached child imports.
   Candidates: versioned URLs for the entire graph, `Cache-Control: no-cache`
   with ETags on `/web/js/*`, or versioned asset paths.

No asset-route/cache implementation or approval is implied until the
maintainer answers these questions. The first PR will include routing,
module-graph, disabled-player, Node, Go and Chromium regression coverage;
older Safari (iPadOS 15.8.8) also needs separate manual confirmation where
hardware is available.

## Public-cloud safety gate

`BasicAuthAdmin` is opt-in: without enabled admin-area auth, `/admin` and
some setup endpoints remain accessible by design. Do not represent the UI
migration as a complete public-cloud security solution. Before the Local
Account tab, agree with the maintainer how to separate admin authentication
and action authorization from speaker/player access to the Bose-compatible
cloud API, including browser-origin and CSRF considerations. Do not block
speaker or player routes in an incidental frontend change.

## Contribution gates

For each PR, inspect the diff; run the relevant Node and Chromium tests in
addition to `make check` and `make lint`; review independently; test the
actual integrated commit. Operator0382 reads and understands the complete
diff before creating a PR and performs the read-only comparison on real
speaker hardware. Never copy real IPs, IDs, names, accounts, credentials or
logs into code, fixtures, commits, PRs or external review packets. Use only
RFC-5737 example addresses and placeholder identifiers.

The production v0.138.1 container, its volume, DNS interception and speaker
configuration are out of scope. A separate read-only test container needs
Operator0382's approval and must not run migration, sync, pairing or
Spotify/Amazon priming against a copied production `server_url`.
