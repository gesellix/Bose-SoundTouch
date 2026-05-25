---
title: "Technical Proposal: External Service Provider Abstraction"
sidebar:
  exclude: true
---
This document outlines a strategy to refactor the SoundTouch Service's content handling into a modular provider-based system.

## 1. Problem Statement
Currently, content handling for BMX (Bose Media Exchange) services like TuneIn or RadioBrowser is deeply intertwined with the HTTP handlers and XML models. Adding a new content provider (e.g., Local Media, Podcast RSS) requires modifying several files and duplicating boilerplate code for HTTP requests and error handling.

## 2. Proposed Architecture

### 2.1 The Provider Interface
We define a generic `ContentProvider` interface that abstracts away the source-specific logic (API calls, data parsing).

```go
package provider

import "github.com/gesellix/bose-soundtouch/pkg/models"

type ContentProvider interface {
    // ID returns the unique identifier for this provider (e.g. "RADIO_BROWSER")
    ID() string

    // Resolve returns playback details for a given content identifier
    Resolve(id string) (*models.BmxPlaybackResponse, error)

    // Search allows finding content within this provider
    Search(query string) ([]models.ContentItem, error)
}
```

### 2.2 Provider Registry
A central registry in `soundtouch-service` manages the lifecycle and selection of providers.

```go
type Registry struct {
    providers map[string]ContentProvider
}

func (r *Registry) Register(p ContentProvider) { ... }
func (r *Registry) Get(id string) ContentProvider { ... }
```

## 3. Implementation Plan

### 3.1 Phase 1: Modularize RadioBrowser
1.  **Extract Logic**: Move current RadioBrowser logic from `bmx.go` into a new package `pkg/service/providers/radiobrowser`.
2.  **Add Failover**: Implement the **API Failover** logic inspired by OpenCloudTouch.
    -   Maintain a list of active RadioBrowser mirrors (e.g., `de1.api.radio-browser.info`, `nl1.api.radio-browser.info`).
    -   Implement a round-robin or health-based selection strategy.
3.  **Implements Interface**: Ensure the new package satisfies the `ContentProvider` interface.

### 3.2 Phase 2: Refactor BMX Handlers
-   Update `HandleTuneInPlayback` and `HandleOrionPlayback` to use the registry.
-   The handlers will look up the provider based on the request context or URL parameters and delegate the resolution.

### 3.3 Phase 3: Dynamic Service Advertising
-   Modify `HandleBMXRegistry` to dynamically generate the `bmx_services.json` content based on the currently registered and enabled providers.

## 4. Benefits
-   **Resilience**: Centralized error handling and failover strategies for all external APIs.
-   **Extensibility**: New services can be added by simply implementing the interface and registering them at startup.
-   **Testability**: Providers can be unit-tested in isolation without mocking the entire HTTP server stack.
-   **Unified UI**: A future Web UI can query the registry to show available content sources and their statuses.

## 5. Next Steps
1.  Refine the `ContentProvider` interface to include metadata (icons, user-friendly names).
2.  Create a prototype for the `radiobrowser` provider with failover support.
