package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

// bodyETag is the ETag for a served list: a hash of the bytes the speaker
// gets.
//
// It used to be the stored file's modification time (presets, recents) or a
// hash of the stored files (/full). Neither changes when only the served form
// does: a fix in how a list is rendered, a repair applied on read, or, for
// presets, a change in the sources each preset pulls its source block from.
// A speaker that cached a broken list then got 304 and kept it, even after
// an update that renders it correctly (issue 697). A hash of the body changes
// whenever what the speaker would get changes, and only then.
//
// The body must therefore be byte-stable for unchanged data: no "now"
// timestamps in it.
func bodyETag(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// writeWithBodyETag answers 304 when the speaker already holds this exact
// body, and sends it with its ETag otherwise.
func writeWithBodyETag(w http.ResponseWriter, r *http.Request, contentType string, body []byte) {
	etag := bodyETag(body)

	// Bose speakers expect the exact "ETag" capitalisation; see
	// handlers_etag_test.go.
	w.Header()["ETag"] = []string{etag}

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
}
