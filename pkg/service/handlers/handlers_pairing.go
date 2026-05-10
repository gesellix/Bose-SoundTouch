package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/go-chi/chi/v5"
)

// accountIDSuggestionsResponse is the body of GET /setup/account-id-suggestions/{deviceId}.
// `current` is the device's existing margeAccountUUID (empty when the device is fresh / factory-reset).
// `known` is the list of accountIDs already present in the local datastore, so the UI can offer
// the user a way to re-attach a fresh device to an existing account.
type accountIDSuggestionsResponse struct {
	Current string   `json:"current"`
	Known   []string `json:"known"`
}

// HandleAccountIDSuggestions returns the device's current account ID (from
// :8090/info, empty if unset) plus the list of account IDs already present
// in the local datastore.
func (s *Server) HandleAccountIDSuggestions(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "Device ID is required")
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	resp := accountIDSuggestionsResponse{}

	if info, err := s.sm.GetLiveDeviceInfo(deviceIP); err == nil {
		resp.Current = info.MargeAccountUUID
	}

	if known, err := s.ds.ListAccounts(); err == nil {
		resp.Known = known
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// pairAccountResponse is the body of POST /setup/pair-account/{deviceId}.
type pairAccountResponse struct {
	OK     bool                    `json:"ok"`
	Result setup.PairAccountResult `json:"result"`
	Output string                  `json:"output"`
	Error  string                  `json:"error,omitempty"`
}

// HandlePairAccount associates the device with the supplied 7-digit account ID,
// trying HTTP /setMargeAccount first and falling back to telnet
// `envswitch accountid set`.
//
// Query params:
//   - account_id (required) — must pass setup.IsValidAccountID
func (s *Server) HandlePairAccount(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "Device ID is required")
		return
	}

	accountID := r.URL.Query().Get("account_id")
	if !setup.IsValidAccountID(accountID) {
		writeJSONError(w, http.StatusBadRequest, "account_id must be exactly 7 digits")
		return
	}

	deviceIP, err := s.resolveDeviceIDToIP(deviceID)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	var t setup.TelnetClient

	if s.sm.NewTelnet != nil {
		t = s.sm.NewTelnet(deviceIP)
		if dialErr := t.Dial(); dialErr != nil {
			// Telnet not reachable — fall through with t=nil so PairAccount
			// can decide based on HTTP availability alone.
			t = nil
		} else {
			defer func() { _ = t.Close() }()
		}
	}

	result, output, err := s.sm.PairAccount(deviceIP, accountID, t)

	w.Header().Set("Content-Type", "application/json")

	body := pairAccountResponse{
		OK:     err == nil,
		Result: result,
		Output: output,
	}

	if err != nil {
		body.Error = err.Error()

		w.WriteHeader(http.StatusInternalServerError)
	}

	if encErr := json.NewEncoder(w).Encode(body); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// jsonErrorBody is the static shape of error responses from this file.
// Avoiding map[string]interface{} keeps errchkjson satisfied: the typed
// struct guarantees encoding can't fail with a runtime type error.
type jsonErrorBody struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// writeJSONError is a small helper for the handlers in this file to keep
// error wiring out of the happy path. It mirrors what the rest of the
// package does inline.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(jsonErrorBody{OK: false, Message: message}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
