package handlers

import (
	"io"
	"log"
	"net/http"
)

// unsupportedReportURL is where we ask operators to report a real client that
// depends on an endpoint we treat as unused.
const unsupportedReportURL = "https://github.com/gesellix/Bose-SoundTouch/issues"

// maxUnsupportedBodyLog caps how much of an unexpected request body we log.
const maxUnsupportedBodyLog = 2048

// HandleUnsupported is wired to frozen routes that no real speaker or app was
// ever observed using (verified against the recorded interaction corpus). It
// exists so the API surface the refactor (issue #451) has to preserve stays as
// small as possible: these routes are candidates for removal, but rather than
// drop them blind we make them fail loudly and observably.
//
// It returns 501 Not Implemented and logs the full request (method, path,
// query, client IP, user-agent and a capped, sanitized body) plus a message
// asking the operator to report it, so that if some device or old app actually
// relies on the route we find out and restore it instead of silently breaking
// it during the refactor.
func (s *Server) HandleUnsupported(w http.ResponseWriter, r *http.Request) {
	client := clientHost(r)

	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(io.LimitReader(r.Body, maxUnsupportedBodyLog))
	}

	log.Printf("[unsupported] %s %s%s called by client=%s ua=%q — this endpoint is treated as unused and returns 501. "+
		"If a speaker or app you rely on needs it, please report it at %s so we can keep it. body=%q",
		sanitizeLog(r.Method),
		sanitizeLog(r.URL.Path),
		sanitizeLog(querySuffix(r)),
		sanitizeLog(client),
		sanitizeLog(r.UserAgent()),
		unsupportedReportURL,
		sanitizeLog(string(body)),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte(`{"error":"not_implemented","message":"This endpoint is not implemented by AfterTouch. ` +
		`If your speaker or app depends on it, please report it at ` + unsupportedReportURL + `"}`))
}

// querySuffix returns "?<rawquery>" or "" so the log line shows the query
// without an empty trailing "?".
func querySuffix(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}

	return "?" + r.URL.RawQuery
}
