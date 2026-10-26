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
answer in #635. The player's `/app/static/lib/` route is subject to its
enable/disable gate, so the admin does not import from it.

## PR 1 asset decisions

Following Operator0382's instruction to proceed without waiting for a more
specific maintainer preference, PR 1 uses the two smallest reversible options:

1. Vendor a separate copy of preact, hooks, htm and es-module-shims in
   `/web/lib/` via `make update-static-deps`, including licenses and lockfile.
   This costs some binary size but avoids coupling the admin to the player
   gate and avoids changing the existing player route.
2. Revalidate every `/web/js/*` module and `/web/lib/*` dependency with
   `Cache-Control: no-cache` and content-based ETags. An entry-module query
   alone cannot invalidate cached child imports; no-cache forces a fresh
   validation even if the module URL stays stable. Keep existing style and
   legacy script hash queries. Do not apply this cache policy to the
   speaker-facing/cloud-compatible endpoints.

These are implementation choices, not a maintainer-endorsed public-cloud
security design. The first PR includes routing, cache, disabled-player, Node,
Go and Chromium regression coverage. Older Safari (iPadOS 15.8.8) needs
separate manual confirmation where hardware is available.

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
