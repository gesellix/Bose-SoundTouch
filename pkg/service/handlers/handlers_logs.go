package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gesellix/bose-soundtouch/pkg/service/logbuf"
)

// logsResponse is the wire shape for GET /setup/logs. Entries is
// ordered by Seq ascending. NextSince is the highest Seq returned,
// suitable for the client's next poll's `since` parameter. Dropped
// is the count of entries the client missed because the ring
// evicted them before the client polled — a non-zero value
// signals the client to surface a gap to the operator.
type logsResponse struct {
	Entries   []logbuf.Entry `json:"entries"`
	NextSince uint64         `json:"nextSince"`
	Dropped   uint64         `json:"dropped"`
	Capacity  int            `json:"capacity"`
}

// HandleGetLogs returns recent log entries from the in-process
// ring buffer. Query parameters:
//
//	since  — return entries with Seq strictly greater than this
//	         value. Omitted or "0" → full snapshot.
//	limit  — cap the number of entries returned. Omitted → no cap
//	         beyond the buffer's capacity.
//
// When no log buffer is attached (e.g. tests, or the env opted
// out via SOUNDTOUCH_LOG_BUFFER_LINES=0), the response is an
// empty snapshot rather than an error so the UI degrades
// gracefully.
func (s *Server) HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	since, err := parseUint64Query(q.Get("since"), 0)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid since: "+err.Error())
		return
	}

	limit, err := parseIntQuery(q.Get("limit"), 0)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid limit: "+err.Error())
		return
	}

	resp := logsResponse{
		Entries:   []logbuf.Entry{},
		NextSince: since,
	}

	if buf := s.LogBuffer(); buf != nil {
		entries, nextSince, dropped := buf.Since(since, limit)
		resp.Entries = entries
		resp.NextSince = nextSince
		resp.Dropped = dropped
		resp.Capacity = buf.Capacity()
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func parseUint64Query(raw string, defaultVal uint64) (uint64, error) {
	if raw == "" {
		return defaultVal, nil
	}

	return strconv.ParseUint(raw, 10, 64)
}

func parseIntQuery(raw string, defaultVal int) (int, error) {
	if raw == "" {
		return defaultVal, nil
	}

	return strconv.Atoi(raw)
}
