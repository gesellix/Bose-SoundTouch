# Merging Bose-SoundTouch-API into Bose-SoundTouch

This document outlines the plan to merge the [Bose-SoundTouch-API](https://github.com/gesellix/Bose-SoundTouch-API) project into this repository. The actual Go implementation in that repository is located in the `soundcork-go` subdirectory. The goal is to provide both a CLI (`soundtouch-cli`) and a service (`soundtouch-service`) from a single codebase.

## Goals

- [x] Maintain the existing `soundtouch-cli` functionality.
- [x] Introduce `soundtouch-service` as a new command (based on the `soundcork-go` project).
- [x] Consolidate shared logic (models, clients, discovery) into the `pkg/` directory.
- [x] Simplify maintenance by having a single Go module and shared CI/CD pipeline.

## Current Directory Structure

```text
.
├── cmd/
│   ├── soundtouch-cli/        # Existing CLI implementation
│   │   └── main.go
│   └── soundtouch-service/    # New service implementation (REST API / Websocket)
│       └── main.go
├── pkg/
│   ├── client/                # Shared SoundTouch API client
│   ├── models/                # Shared data models
│   ├── discovery/             # Shared device discovery logic
│   └── service/               # Service-specific logic (from Bose-SoundTouch-API)
│       ├── bmx/               # BMX service logic
│       ├── marge/             # Marge service logic
│       ├── datastore/         # Device and configuration storage
│       ├── proxy/             # Logging proxy logic
│       ├── setup/             # Device setup and migration logic
│       └── handlers/          # HTTP handlers (adapted from soundcork-go/soundcork-go)
│           └── soundcork/     # Embedded resources (index.html, media/, etc.)
├── docs/
│   └── MERGE_PROJECTS.md      # This document
├── go.mod
└── go.sum
```

## Step-by-Step Merge Status

### 1. Preparation
- [x] Review `go.mod` in both projects to identify dependency overlaps and conflicts.

### 2. Code Integration
- [x] **Models & Client**: Merged missing functionality from `soundcork-go/internal/models` into `pkg/models`. Renamed overlapping models to `Service*` (e.g., `ServiceContentItem`, `ServicePreset`).
- [x] **Service Logic**: Adapted internal packages from `soundcork-go/internal/` to `pkg/service/`.
- [x] **Handlers**: Moved and adapted HTTP handlers into `pkg/service/handlers/`.
- [x] **New Command**: Created `cmd/soundtouch-service/main.go` as the service entry point using `chi` router.
- [x] **Embedded Resources**: Integrated `index.html`, `bmx_services.json`, `swupdate.xml`, and `media/` folder into the binary using `//go:embed`.

### 3. Dependency Management
- [x] Update `go.mod` to include:
    - `github.com/go-chi/chi/v5`
    - `github.com/srwiley/oksvg` and `github.com/srwiley/rasterx`
    - `golang.org/x/crypto`
- [x] Run `go mod tidy` to clean up dependencies.

### 4. Shared Logic Refactoring
- [x] Identify common code between `soundtouch-cli` and the new service.
- [x] Move shared logic into `pkg/` to ensure both commands use the same underlying implementation.

### 5. Documentation & Examples
- [x] Update `README.md` to mention the new `soundtouch-service` command.
- [x] Add service-specific documentation in `docs/SOUNDTOUCH-SERVICE.md`.
- [x] Provide examples of how to run and interact with the service in `examples/service-demo/`.

### 6. CI/CD Updates
- [x] Update `.github/workflows/release.yml` to build and release the `soundtouch-service` binary alongside `soundtouch-cli`.
- [x] Update any test workflows to include tests for the service logic.

## Verification
- [x] `go build ./cmd/soundtouch-cli` works as expected.
- [x] `go build ./cmd/soundtouch-service` works as expected.
- [x] All tests pass: `go test ./...`.
- [x] Resources are correctly served from the embedded filesystem.
