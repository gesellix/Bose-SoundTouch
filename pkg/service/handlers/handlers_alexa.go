package handlers

import (
	"encoding/json"
	"log"
	"net/http"
)

// HandleAlexaCertificate handles POST /alexa/certificate.
//
// The speaker sends a CSR (PEM, URL-form-encoded as "csr") and a JSON "data" field
// containing a Bearer token, device MAC address, device type, and AWS region.
// The real voice.api.bose.io endpoint forwards the CSR to AWS IoT, which signs it
// and returns a device certificate, the account's IoT endpoint URL, and a client ID.
// The speaker uses these to establish a persistent MQTT connection to Alexa IoT.
//
// Full implementation requires an AWS IoT integration:
//   - Parse the CSR from the form body
//   - Exchange it via the AWS IoT CreateKeysAndCertificate or RegisterThing API
//   - Return {"certificatePem": "...", "iot_endpoint": "...", "client_id": "..."}
//
// Until implemented, Alexa voice control will not work after cloud shutdown.
func (s *Server) HandleAlexaCertificate(w http.ResponseWriter, r *http.Request) {
	device := ""

	if err := r.ParseForm(); err == nil {
		if data := r.FormValue("data"); data != "" {
			var d struct {
				Device string `json:"device"`
			}
			if err := json.Unmarshal([]byte(data), &d); err == nil {
				device = d.Device
			}
		}
	}

	log.Printf("[alexa] certificate provisioning not implemented (device=%s); Alexa voice control requires AWS IoT integration", device)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte(`{"error":"not_implemented","message":"Alexa IoT certificate provisioning requires AWS IoT integration. See voice.api.bose.io /alexa/certificate handler."}`))
}
