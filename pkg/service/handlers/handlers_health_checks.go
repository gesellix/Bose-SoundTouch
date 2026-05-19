package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/health"
)

// healthChecksResponse is the wire shape for GET /setup/health.
type healthChecksResponse struct {
	GeneratedAt string               `json:"generatedAt"`
	Checks      []health.CheckResult `json:"checks"`
}

// healthFixRequest is the wire shape for POST /setup/health/fix.
// Target locates the entity the fix should act on; an empty
// Account/Device pair is allowed for service-wide fixes.
type healthFixRequest struct {
	CheckID string        `json:"checkId"`
	FixID   string        `json:"fixId"`
	Target  health.Target `json:"target"`
}

type healthFixResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// HandleHealthChecks runs every registered health check and
// returns the current findings. Safe to poll: checks are
// expected to be cheap (filesystem stats, in-memory lookups).
func (s *Server) HandleHealthChecks(w http.ResponseWriter, _ *http.Request) {
	if s.healthRegistry == nil {
		writeJSONError(w, http.StatusInternalServerError, "health registry not initialized")
		return
	}

	resp := healthChecksResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Checks:      s.healthRegistry.RunAll(),
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// HandleHealthFix dispatches a quick-fix identified by
// (checkId, fixId) against the supplied target. Returns 404 when
// the fix isn't registered (typically a stale UI), 400 on a
// malformed body, and 500 when the fix itself fails. The success
// message comes from the FixFunc and is forwarded to the UI.
func (s *Server) HandleHealthFix(w http.ResponseWriter, r *http.Request) {
	if s.healthRegistry == nil {
		writeJSONError(w, http.StatusInternalServerError, "health registry not initialized")
		return
	}

	var req healthFixRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.CheckID == "" || req.FixID == "" {
		writeJSONError(w, http.StatusBadRequest, "checkId and fixId are required")
		return
	}

	msg, err := s.healthRegistry.RunFix(req.CheckID, req.FixID, req.Target)
	if err != nil {
		if errors.Is(err, health.ErrFixNotFound) {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}

		writeJSONError(w, http.StatusInternalServerError, err.Error())

		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(healthFixResponse{OK: true, Message: msg}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
