// Package tunein provides shared handlers for mocking the TuneIn
// (radiotime.com) upstream API, so integration tests for the BMX TuneIn
// endpoints do not depend on the live TuneIn service.
//
// It covers the OPML endpoints the service calls for station playback:
//   - GET /Tune.ashx?id=<guideID>&formats=...  -> stream URLs (JSON body[])
//   - GET /describe.ashx?id=<guideID>          -> station name + logo (OPML XML)
//
// Responses use only documentation-safe values (RFC-5737 192.0.2.0/24 hosts).
// Endpoints that are not yet mocked (navigate, search, profile contents) return
// 404 so a test that needs them fails loudly and we know to add a fixture; see
// tests/integration/http-client/TUNEIN-MOCK-MISSING.md.
package tunein

import (
	"fmt"
	"log"
	"net/http"
)

// NewTuneInHandler returns an http.Handler configured with the mocked TuneIn
// OPML endpoints.
func NewTuneInHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/Tune.ashx", HandleTune)
	mux.HandleFunc("/describe.ashx", HandleDescribe)
	mux.HandleFunc("/", HandleCatchAll)

	return mux
}

// HandleTune simulates TuneIn's Tune.ashx stream-resolution endpoint. The
// service parses the JSON body[] array for {url} entries
// (bmx.parseTuneInStreamBody); we return two documentation-safe stream URLs so
// the multi-stream failover path is exercised too.
func HandleTune(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	log.Printf("[TuneIn Mock] Tune.ashx id=%s formats=%s", sanitizeLog(id), sanitizeLog(r.URL.Query().Get("formats")))

	if id == "" {
		http.Error(w, `{"head":{"status":"400"}}`, http.StatusBadRequest)
		return
	}

	body := fmt.Sprintf(`{"head":{"status":"200"},"body":[`+
		`{"url":"http://192.0.2.20:8000/%s/stream-1.mp3","media_type":"mp3","reliability":99,"bitrate":128,"is_direct":true},`+
		`{"url":"http://192.0.2.20:8000/%s/stream-2.mp3","media_type":"mp3","reliability":95,"bitrate":128,"is_direct":true}`+
		`]}`, id, id)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// HandleDescribe simulates TuneIn's describe.ashx metadata endpoint. The service
// reads the first <outline> element's text + image attributes
// (bmx.TuneInDescribeMeta).
func HandleDescribe(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	log.Printf("[TuneIn Mock] describe.ashx id=%s", sanitizeLog(id))

	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<opml version="1">`+
		`<head><title>%s</title><status>200</status></head>`+
		`<body><outline type="object" text="Mock Radio %s" image="http://192.0.2.20:8000/%s/logo.png"/></body>`+
		`</opml>`, id, id, id)

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

// HandleCatchAll logs and 404s any TuneIn endpoint that is not mocked yet
// (navigate, search, profile contents), making the gap visible to a failing
// test rather than silently returning wrong data.
func HandleCatchAll(w http.ResponseWriter, r *http.Request) {
	log.Printf("[TuneIn Mock] UNMOCKED %s %s — add a fixture (see TUNEIN-MOCK-MISSING.md)",
		sanitizeLog(r.Method), sanitizeLog(r.URL.RequestURI()))
	http.Error(w, "tunein mock: endpoint not implemented", http.StatusNotFound)
}
