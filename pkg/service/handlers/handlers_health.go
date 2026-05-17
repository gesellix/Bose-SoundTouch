package handlers

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"time"
)

// buildVersionInfo extracts module version + VCS metadata from the
// runtime/debug build info, falling back to "0.0.1" when the binary
// wasn't built with module info (e.g. local `go run`). Shared between
// HandleHealth and HandleRoot so both endpoints report identical
// release context; keys present-when-non-empty so a `go run` response
// stays minimal instead of carrying empty strings.
func buildVersionInfo() map[string]string {
	info := map[string]string{"version": "0.0.1"}

	build, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}

	if build.Main.Version != "" && build.Main.Version != "(devel)" {
		info["version"] = build.Main.Version
	}

	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			if setting.Value != "" {
				info["vcs_revision"] = setting.Value
			}
		case "vcs.time":
			if setting.Value != "" {
				info["vcs_time"] = setting.Value
			}
		case "vcs.modified":
			if setting.Value != "" {
				info["vcs_modified"] = setting.Value
			}
		}
	}

	return info
}

// HandleHealth returns the health status of the service.
func (s *Server) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	status := map[string]interface{}{
		"status":    "up",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	for k, v := range buildVersionInfo() {
		status[k] = v
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
