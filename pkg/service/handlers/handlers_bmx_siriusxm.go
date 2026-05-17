// Package handlers — SiriusXM BMX adapter (logging stub).
//
// bmx_services.json declares SIRIUSXM_EVEREST at
// `{BMX_SERVER}/core02/svc-bmx-adapter-siriusxm-everest-eco1/prod/live-adapter`,
// and bmx_services_availability.json lists it as available — so speakers
// that try SiriusXM hit this path. The bare URL returns the service
// descriptor; sub-paths advertised by the descriptor's _links
// (/availability, /token, /navigate, /logout, plus the playback paths
// the speaker discovers via navigate) currently log + 404 so we have
// visibility into real speaker calls for the next implementation pass.
//
// Reference: deborahgu/soundcork main.py:805 takes the same shape —
// returns the SiriusXM service descriptor from the BMX services array
// (hardcoded index 2). We select by id.name instead of array index.
package handlers

import (
	"log"
	"net/http"
)

// HandleSiriusXMLiveAdapter returns the SIRIUSXM_EVEREST service descriptor
// from bmx_services.json for the bare live-adapter base URL.
func (s *Server) HandleSiriusXMLiveAdapter(w http.ResponseWriter, r *http.Request) {
	log.Printf("[BMX SiriusXM] %s %s ua=%q auth=%q query=%q",
		r.Method, r.URL.Path, r.UserAgent(),
		r.Header.Get("Authorization"), r.URL.RawQuery)

	svc, err := extractBMXService(bmxServicesJSON, "SIRIUSXM_EVEREST")
	if err != nil {
		log.Printf("[BMX SiriusXM] failed to extract service descriptor: %v", err)
		http.Error(w, "service descriptor unavailable", http.StatusInternalServerError)

		return
	}

	body := s.applyBMXTemplate(string(svc))

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// HandleSiriusXMLiveAdapterSubpath logs and 404s any unimplemented sub-path
// under the SiriusXM live-adapter. Visibility for the next implementation
// pass — the _links in the descriptor publish /availability, /token,
// /navigate, /logout; playback URLs come dynamically from navigate.
func (s *Server) HandleSiriusXMLiveAdapterSubpath(w http.ResponseWriter, r *http.Request) {
	log.Printf("[BMX SiriusXM] UNIMPLEMENTED %s %s ua=%q auth=%q query=%q",
		r.Method, r.URL.Path, r.UserAgent(),
		r.Header.Get("Authorization"), r.URL.RawQuery)

	http.Error(w, "not implemented", http.StatusNotFound)
}
