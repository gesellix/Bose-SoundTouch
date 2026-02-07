package handlers

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"time"
)

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	version := "0.0.1"
	vcsRevision := ""
	vcsTime := ""
	vcsModified := ""

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}

		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				vcsRevision = setting.Value
			case "vcs.time":
				vcsTime = setting.Value
			case "vcs.modified":
				vcsModified = setting.Value
			}
		}
	}

	status := map[string]interface{}{
		"status":    "up",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   version,
	}
	if vcsRevision != "" {
		status["vcs_revision"] = vcsRevision
	}

	if vcsTime != "" {
		status["vcs_time"] = vcsTime
	}

	if vcsModified != "" {
		status["vcs_modified"] = vcsModified
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}
