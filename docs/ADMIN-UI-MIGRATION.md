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

## Verification record for PR 1

Recorded so a reviewer can tell what was actually executed from what was
assumed. Rebased onto `main` at `5eea6da6`; environment: Go 1.27.4,
golangci-lint 2.14.0 (built with go1.27.2), Node 26.12.1, Chrome 149, 28
cores, Linux. `CGO_ENABLED=0` throughout, because `scripts/extract-ws`
imports gopacket/pcap and no system `pcap.h` is present; `-race` is therefore
not covered, since race instrumentation requires CGO.

| Gate | Result |
|---|---|
| `gofmt -s -l .` | clean, no files listed |
| `go vet ./...` | exit 0 |
| `go test ./...` | 39 packages ok, 0 failures |
| `golangci-lint run` | 0 issues |
| `node --test pkg/service/handlers/frontend_test/*.test.mjs` | 5 tests, 5 pass |
| `go test -tags browsertest ./pkg/service/handlers/...` | 6 of 6 runs green, ~5.0s each |
| `go test -tags browsertest ./pkg/service/soundtouchweb/...` | **not reliably green, see below** |

All browser runs quoted here used `-count=1`. Go caches test results per
package, so a run without it can report `ok` from an earlier invocation and
proves nothing about the current tree — an earlier `ok 119.986s` for the
player suite was such a cached result and was withdrawn.

`make check` and `make test-http-client` were not run and must not be run on
a host that also serves production: the target chains compose with the
hardcoded `container_name: soundtouch-service` on ports 8000/8443 and builds
the `latest` tag.

### Live verification

Against a separate test stack on port 18000, built from `git archive HEAD`
with `--target soundtouch-service` and pinned to a unique tag, never `latest`
or a released version. Note the target: the last stage in the Dockerfile is
`soundtouch-player`, so a plain `docker build` produces the player image,
which answers `/health` but 404s every `/web/*` path.

- `/admin` returns 200; the import map resolves, the module entry point runs,
  and the Preact logs panel renders. No console errors, no warnings, no
  uncaught exceptions.
- Real buffered log lines render as text: the `#logs-view` element had zero
  child *elements*, so speaker-supplied strings never became markup.
- Every `/web/js/*` and `/web/lib/*` module answers with
  `Cache-Control: no-cache` and an ETag equal to the SHA-256 of the served
  file; a matching `If-None-Match` yields 304, a non-matching one 200.
  `/web/css/style.css` keeps its previous behaviour, confirming the policy is
  scoped to modules.
- The upstream `env_controlled` hints (issue 744) survive this migration,
  verified in both directions: with no variable set the API returns `{}` and
  the Settings tab renders zero hints; with `TTS_LANGUAGE` set the API
  returns `{"tts_language":"TTS_LANGUAGE"}` and exactly one hint renders on
  the `tts-language` field.

Pre-existing and not addressed here: `/web/*` is routed with `r.Get` only, so
a `HEAD` request answers 405. Header checks must use GET.

## Browser matrix

Manually confirmed by Operator0382 on the migrated Logs tab, no anomalies:

| Browser | Platform | Result |
|---|---|---|
| Firefox | desktop | clean |
| Firefox Developer Edition | desktop | clean |
| Brave | desktop | clean |
| Safari 26.6.1 | macOS | clean |
| headless Chrome 149 | Linux, automated | clean |

**This does not close the gap #649 was about.** Import maps shipped in Safari
16.4, so every browser in the table above resolves bare specifiers natively
and never fetches `es-module-shims.js` at all. The fallback path is therefore
still unexercised by any of these results. The browser that needs it is
Safari 15 on iPadOS 15.8.8, which is the configuration that rendered the
player blank in #649 and which is not available here. Treat the shim as
untested until someone confirms it on such a device; a green modern-Safari
run is not evidence about it.

## Known flakiness in the player browser suite

The player's browser suite fails intermittently on this machine, and it does
so **independently of this migration**. Recorded here because a reviewer
seeing a red run should not attribute it to the admin changes, and because
"all gates green" would be false.

Full-package runs, all with `-count=1`:

| Tree | Runs | Failures |
|---|---|---|
| this branch (`e821f93f`) | 5 | 2 |
| `main` at `5eea6da6`, without these commits | 3 | 2 |

Tests observed failing:

- `TestZoneVolumeDelayedResultCannotOverwriteNewerProjection/member`
  (`browser_zone_volume_test.go`), failing after ~30s, i.e. hitting a poll
  timeout rather than asserting a wrong value. Observed on both trees.
- `TestScopedSoundSettingsDiscreteWritesAndHierarchy`
  (`browser_sound_settings_test.go`), failing in under a second. Observed on
  unmodified `main`.

Both live in `pkg/service/soundtouchweb`, which this PR does not touch. The
admin half of the suite, `pkg/service/handlers` including
`TestAdminLogsBrowser`, was green in 6 of 6 consecutive runs.

Running a single test in isolation with `-run` passed 4 of 4 times on both
trees, so the failure needs the full suite's concurrency to reproduce.
Isolated reruns are not a valid way to dismiss it.

The cause is **not** established. A 30-second timeout in a test that waits
for a projected state change is consistent with a genuine race in the
player's zone-volume reconciliation as well as with scheduling pressure from
the parallel suite; nothing here distinguishes the two. Worth its own issue
rather than a footnote in a frontend PR, and worth resolving before anyone
relies on this suite as a release gate.
