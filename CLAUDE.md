# CLAUDE.md

Entry point for any Claude Code (or human) session working on this
repository. Read it before touching code.

## What this project is

Go library and toolset for controlling Bose SoundTouch speakers via
the local network API, plus a local cloud-service emulator. Bose
discontinued the SoundTouch cloud — this project keeps existing
speakers usable without it.

**Module:** `github.com/gesellix/bose-soundtouch`

Key binaries:

- `soundtouch-cli` — command-line control of one or more speakers
  (status, play, presets, groups, migration, …).
- `soundtouch-service` — replacement for `streaming.bose.com`
  and the `bmx` services, default port `8000`.
- `soundtouch-web` — Web UI for Radio browsing and device control.
- `soundtouch-backup` — Helper for on-device backup and restore.

Per-session pickup notes live in two local files at the repo root (they are `.gitignore`d and only exist if created during a session):

- `NEXT.md` — current "pick up here" log of open items.
- `DONE.md` — archive of recently resolved items.

## How a new session should start

1. Read this file.
2. Read `NEXT.md` if it's present — that's where running context lives.
3. Skim `README.md` for the user-facing pitch.
4. Skim `docs/` for the area you're touching. Long-form notes
   (analysis, guides, troubleshooting) live there, not in the code.
5. Run `make check` once to confirm the local environment compiles,
   vets, and tests cleanly.

## Build, test, run

```bash
# Build
make build          # All binaries
make build-cli      # Just CLI
make build-service  # Just service
make build-web      # Just web UI
make build-all      # Cross-platform builds (Linux, macOS, Windows)
make install        # Install to $GOPATH/bin

# Quality
make test           # Unit tests
make test-coverage  # Coverage reports
make check          # fmt + vet + test
make lint           # golangci-lint

# Development
make dev-service    # Run local service on port 8000
make dev-discover   # Discover devices on the LAN
make dev-info HOST=<ip>  # Get device info

# Docker
make docker-build
make docker-run-host
```

**Pre-push quality gate:** `make lint` (golangci-lint) must be clean
before `git push`. CI runs it on every PR; running it locally first
saves a round-trip. `make check` covers `lint` is its own target —
combine as needed.

## Integration tests

The `.http` integration tests under `tests/integration/http-client/`
run via `make test-http-client`, which spins up the service plus
support mocks (`spotify-mock`, `amazon-mock`) using
`docker-compose.yml` + `docker-compose.ci.yml`, executes the suite
through the JetBrains HTTP client image, then tears the stack down.
Requires Docker.

The compose CI override mounts `tests/integration/testdata/` into the
service container as its persistent data dir. That directory is
listed in `tests/.gitignore` — it's local developer state, not source.

**Treat the testdata dir as debug evidence, not disposable scratch.**
When a fixture or schema change makes the old state stale (e.g.
post-anonymisation, the previous run's IPs no longer match the
assertions), don't `rm -rf` it — archive it:

```bash
make test-http-client-rotate   # renames testdata/ → testdata_<timestamp>/
make test-http-client          # fresh run on a clean slate
```

The rotate target is non-destructive (it moves, never deletes) and
opt-in (no other target invokes it). Old archives stay around for
retrospective diffing whenever something goes sideways.

## Project structure

```
cmd/
  soundtouch-cli/      # CLI tool for device control
  soundtouch-service/  # Local cloud service emulator
  soundtouch-web/      # Web UI (TuneIn browser, device control)
  soundtouch-backup/   # On-device backup helper
  example-*/           # Usage examples
pkg/
  client/              # HTTP + WebSocket client for the SoundTouch Web API
  models/              # XML/JSON data structures
  discovery/           # Device discovery (mDNS + UPnP, unified interface)
  config/              # Configuration management
  service/
    bmx/               # Bose Media eXchange service emulation
    marge/             # Device-management service emulation
    handlers/          # HTTP request handlers (pkg/service/handlers/)
    proxy/             # HTTP proxy with request recording
    datastore/         # Persistent device data storage
    certmanager/       # TLS certificate management
    setup/             # Device migration and configuration
    spotify/           # Spotify integration
    stockholm/         # Optional Stockholm frontend bridge
    soundtouchweb/     # SoundTouch Web UI service logic
examples/              # Feature demonstration programs
docs/                  # Long-form analysis, guides, troubleshooting
.junie/                # Communication-style guidelines (see below)
```

## Key technologies

- **Go 1.26.3+**
- **chi v5** — HTTP router
- **gorilla/websocket** — WebSocket for real-time events
- **hashicorp/mdns** — mDNS device discovery
- **miekg/dns** — DNS operations and a custom DNS server
- **urfave/cli/v2** — CLI framework

## Architecture notes

- `pkg/client` is the core library for device API calls (HTTP + WebSocket).
- `pkg/service` is the local cloud replacement; routes wire to the
  handlers in `pkg/service/handlers/` via chi middleware.
- Discovery supports both mDNS and UPnP/SSDP behind a unified interface.
- The SoundTouch Web API uses XML on the wire; internal service-to-service
  messages use JSON.
- Tests cover unit, integration, parity (local vs. official Bose API
  recordings), and regression. Reproducer tests should be refactored
  into permanent regression or documentation tests rather than deleted.

## Load-bearing gotchas

### `ETag` header literal must stay capitalised

Bose speakers emit the response header with exact capitalisation
`ETag`. Go's `http.Header.Set` canonicalises to `Etag` (lowercase `t`).
Real speakers parse strictly — `Etag` is rejected. The codebase
deliberately bypasses the canonicalisation path; do **not** rewrite
the string literal `"ETag"` to `"Etag"` anywhere in `pkg/service/handlers/`
or in tests.

The contrast is encoded in two named constants in
`pkg/service/handlers/handlers_etag_test.go`:

```go
const normalizedEtag    = "Etag" // what http.Header.Set produces
const caseSensitiveETag = "ETag" // what the speaker actually expects
```

Linter suppressions on the canonical-header check live alongside the
test code. Static-analysis warnings about `"ETag"` are expected;
don't "fix" them.

### Destructive git or filesystem actions need explicit confirmation

`git reset --hard`, `git checkout` that would overwrite local changes,
`git clean -fd`, `rm -rf` on non-build paths, `git stash drop` — all
should be proposed in writing with their consequences before running,
unless the user has already authorised that specific action in this
session. Prefer reversible alternatives (`git stash` over
`git reset --hard`).

## What never goes into this repo

This repository is public. The following must never be committed:

- **Real LAN IPs** of personal networks. Use RFC-5737 documentation
  ranges in examples and fixtures: `192.0.2.0/24`, `198.51.100.0/24`,
  `203.0.113.0/24`.
- **Real MAC addresses** or speaker device IDs from anyone's actual
  hardware. Use `AA:BB:CC:DD:EE:FF` or `DEVICEID01` style placeholders.
- **Bose account IDs**, serial numbers, or tokens belonging to anyone
  other than the committer's own test devices — and even those should
  be sanitised before publication when feasible.
- **Bose firmware binaries, NAND dumps, or decompiled Bose code.**
- **Wi-Fi SSIDs or credentials**, captured or otherwise.
- **Network captures, traces, or logs** that include data from
  accounts or devices other than your own test hardware.
- **Personal identifiers**: real names of speakers ("LivingRoom",
  custom device names), private email addresses, household member
  names visible in source IDs.

If you spot any of the above already in the tree, treat it as a
sanitisation task: stop, flag it to the maintainer, propose a
remediation commit before continuing.

## Disclaimers

"SoundTouch" and "Bose" are registered trademarks of Bose Corporation.
This project is an unofficial, community-built effort, not affiliated
with, endorsed by, or authorised by Bose.

## Communication style

When working with a human user in this repo:

- **Prioritise direct answers** to the question being asked, even when
  it sits outside the current task or project context. Don't divert
  back to whatever you were doing when the user asks something else.
- **Don't substitute assumptions for real information.** When something
  is unclear, ask or check, rather than guessing and proceeding.

These principles also apply to other AI assistants pointed at this
repo. Tool-specific config dirs (e.g. `.junie/`, `.claude/`) should
defer to this file as the source of truth instead of carrying their
own copies.
