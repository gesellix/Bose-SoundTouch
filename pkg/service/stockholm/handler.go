package stockholm

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Handler is the main entry point for the Stockholm frontend backend.
type Handler struct {
	cfg          *Config
	backendCfg   *BackendConfig
	state        *NativeState
	bridge       *Bridge
	stockholmDir string
}

// New initialises and returns a Stockholm Handler.
//
// stockholmDir is the path to the extracted Stockholm frontend (contains index.html).
// workspaceRoot is used to locate backend/state and backend/config directories.
// backendURL is the external URL of this service (used for config URL rewriting).
// basePath is the URL prefix at which the Stockholm UI is mounted (e.g. "/stockholm");
// pass "" to serve at the root.
func New(stockholmDir, workspaceRoot, backendURL, basePath string) (*Handler, error) {
	if _, err := os.Stat(stockholmDir); err != nil {
		return nil, fmt.Errorf("stockholm dir not found at %q: %w", stockholmDir, err)
	}

	cfg, err := LoadConfig(stockholmDir)
	if err != nil {
		return nil, fmt.Errorf("load stockholm config: %w", err)
	}

	if backendURL != "" {
		margeURL := firstNonEmpty(os.Getenv("MARGE_URL"), backendURL)

		authServiceURL := firstNonEmpty(os.Getenv("AUTH_SERVICE_URL"), backendURL)
		if err := RewriteConfigURLs(stockholmDir, backendURL, margeURL, authServiceURL); err != nil {
			log.Printf("[Stockholm] Warning: failed to rewrite config URLs: %v", err)
		}
	}

	backendCfg := LoadBackendConfig(workspaceRoot)

	stateDir := filepath.Join(workspaceRoot, "backend", "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	state := NewNativeState(stateDir)
	if err := state.Load(); err != nil {
		log.Printf("[Stockholm] Warning: failed to load native state: %v", err)
	}

	// Normalise basePath: no trailing slash, must start with "/" or be empty.
	if basePath != "" && !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}

	basePath = strings.TrimRight(basePath, "/")
	cfg.BasePath = basePath

	state.SeedFromEnv(cfg)

	bridge := newBridge(cfg, state)

	return &Handler{
		cfg:          cfg,
		backendCfg:   backendCfg,
		state:        state,
		bridge:       bridge,
		stockholmDir: stockholmDir,
	}, nil
}

// Mount registers all Stockholm routes on the given chi router.
// API routes (/api/native/*, /api/http-proxy) are registered under cfg.BasePath
// because the patched JS uses window.__stockholmBase as a prefix for all API calls.
// Static content is served under cfg.BasePath (e.g. /stockholm) if set,
// otherwise at the root.
func (h *Handler) Mount(r chi.Router) {
	apiBase := h.cfg.BasePath
	r.Post(apiBase+"/api/native/appSend", h.bridge.HandleAppSend)
	r.Get(apiBase+"/api/native/runQueue", h.bridge.HandleRunQueue)
	r.HandleFunc(apiBase+"/api/http-proxy", h.handleProxy)

	if h.cfg.BasePath != "" {
		// Redirect bare /stockholm to /stockholm/ so the browser sets the correct
		// base URL for relative asset references.
		r.Get(h.cfg.BasePath, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, h.cfg.BasePath+"/", http.StatusMovedPermanently)
		})
		// Strip the base path prefix before passing to handleStatic so that
		// resolveStaticFile sees paths like "/" or "/index.html", not "/stockholm/".
		// r.Route does NOT strip r.URL.Path, so we must use http.StripPrefix explicitly.
		stripped := http.StripPrefix(h.cfg.BasePath, http.HandlerFunc(h.handleStatic))
		r.Get(h.cfg.BasePath+"/", stripped.ServeHTTP)
		r.Head(h.cfg.BasePath+"/", stripped.ServeHTTP)
		r.Get(h.cfg.BasePath+"/*", stripped.ServeHTTP)
		r.Head(h.cfg.BasePath+"/*", stripped.ServeHTTP)
	} else {
		// Serve static content at the root (catch-all at the end).
		r.Get("/*", h.handleStatic)
		r.Head("/*", h.handleStatic)
		r.Get("/", h.handleStatic)
	}
}

func (h *Handler) handleProxy(w http.ResponseWriter, r *http.Request) {
	HandleProxy(w, r, h.cfg, h.state)
}

func (h *Handler) handleStatic(w http.ResponseWriter, r *http.Request) {
	ServeStatic(w, r, h.stockholmDir, h.backendCfg, h.state, h.cfg)
}

// HandleStatic is the exported form of handleStatic, needed when mounting the
// Stockholm static handler inside sub-routers (e.g. to resolve the /setup/ path
// collision between the management API and the Stockholm setup wizard pages).
func (h *Handler) HandleStatic(w http.ResponseWriter, r *http.Request) {
	ServeStatic(w, r, h.stockholmDir, h.backendCfg, h.state, h.cfg)
}

// Config returns the loaded Stockholm config (for integration with the proxy handler
// that may need to inject BMX/marge headers).
func (h *Handler) Config() *Config {
	return h.cfg
}

// State returns the NativeState (for integration with handlers that need to read
// auth tokens or account IDs).
func (h *Handler) State() *NativeState {
	return h.state
}
