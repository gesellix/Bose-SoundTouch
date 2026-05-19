package handlers

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/health"
	"github.com/go-chi/chi/v5"
)

// deviceSummary is the wire shape for GET /setup/device-summary/{deviceId}.
// Each sub-section is independently populated so partial failures
// (e.g. speaker unreachable but service-side state available)
// still produce a useful payload.
type deviceSummary struct {
	Device      deviceSummaryDevice  `json:"device"`
	Speaker     deviceSummarySpeaker `json:"speaker"`
	Service     deviceSummaryService `json:"service"`
	Pairing     deviceSummaryPairing `json:"pairing"`
	GeneratedAt string               `json:"generated_at"`
}

type deviceSummaryDevice struct {
	DeviceID        string `json:"device_id"`
	AccountID       string `json:"account_id"`
	Name            string `json:"name,omitempty"`
	IPAddress       string `json:"ip_address,omitempty"`
	ProductCode     string `json:"product_code,omitempty"`
	FirmwareVersion string `json:"firmware_version,omitempty"`
	SerialNumber    string `json:"serial_number,omitempty"`
	MacAddress      string `json:"mac_address,omitempty"`
}

type probeOutcome struct {
	Reachable   bool   `json:"reachable"`
	StatusCode  int    `json:"status_code,omitempty"`
	Err         string `json:"error,omitempty"`
	CurlCommand string `json:"curl_command,omitempty"`
}

type deviceSummarySpeaker struct {
	Info    speakerInfoSummary    `json:"info"`
	Sources speakerSourcesSummary `json:"sources"`
	Presets speakerPresetsSummary `json:"presets"`
}

type speakerInfoSummary struct {
	probeOutcome

	Name             string `json:"name,omitempty"`
	Type             string `json:"type,omitempty"`
	MargeAccountUUID string `json:"marge_account_uuid,omitempty"`
	MargeURL         string `json:"marge_url,omitempty"`
}

type speakerSourcesSummary struct {
	probeOutcome

	Types []string `json:"types,omitempty"`
}

type speakerPresetsSummary struct {
	probeOutcome

	IDs []string `json:"ids,omitempty"`
}

type deviceSummaryService struct {
	ServerURL          string   `json:"server_url,omitempty"`
	ExpectedHosts      []string `json:"expected_hosts,omitempty"`
	SourcesXMLPresent  bool     `json:"sources_xml_present"`
	ServiceSourceTypes []string `json:"service_source_types,omitempty"`
	PresetsXMLPresent  bool     `json:"presets_xml_present"`
	ServicePresetCount int      `json:"service_preset_count"`
}

type deviceSummaryPairing struct {
	Paired                 bool   `json:"paired"`
	SpeakerMargeHost       string `json:"speaker_marge_host,omitempty"`
	MargeURLMatchesService bool   `json:"marge_url_matches_service"`
}

// HandleDeviceSummary returns a one-shot aggregate of the speaker
// state (/info + /sources + /presets) plus the service-side
// equivalents, plus the pairing inference. Useful as a "what
// does this speaker think is true right now" probe — bundles
// what operators today fetch piecewise across several issues'
// debug threads.
//
// Probes run concurrently. Partial failures don't break the
// response: each sub-section carries its own reachability /
// error fields, and the curl command is always populated so the
// UI can render a paste-helper when the server can't reach the
// speaker (cloud-deployment topology).
func (s *Server) HandleDeviceSummary(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "deviceId")
	if deviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "deviceId is required")
		return
	}

	devices, err := s.ds.ListAllDevices()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list devices: "+err.Error())
		return
	}

	var match *struct {
		account string
		device  string
		ip      string
		name    string
		product string
		fw      string
		serial  string
		mac     string
	}

	for i := range devices {
		d := &devices[i]
		if d.DeviceID == deviceID {
			match = &struct {
				account string
				device  string
				ip      string
				name    string
				product string
				fw      string
				serial  string
				mac     string
			}{
				account: d.AccountID,
				device:  d.DeviceID,
				ip:      d.IPAddress,
				name:    d.Name,
				product: d.ProductCode,
				fw:      d.FirmwareVersion,
				serial:  d.DeviceSerialNumber,
				mac:     d.MacAddress,
			}

			break
		}
	}

	if match == nil {
		writeJSONError(w, http.StatusNotFound, "device not found: "+deviceID)
		return
	}

	summary := deviceSummary{
		Device: deviceSummaryDevice{
			DeviceID:        match.device,
			AccountID:       match.account,
			Name:            match.name,
			IPAddress:       match.ip,
			ProductCode:     match.product,
			FirmwareVersion: match.fw,
			SerialNumber:    match.serial,
			MacAddress:      match.mac,
		},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Service-side: cheap, no probes.
	serverURL, _ := s.GetSettings()
	summary.Service.ServerURL = serverURL
	summary.Service.ExpectedHosts = s.ExpectedHosts()

	if s.ds.HasConfiguredSources(match.account, match.device) {
		summary.Service.SourcesXMLPresent = true

		if sources, err := s.ds.GetConfiguredSources(match.account, match.device); err == nil {
			seen := map[string]bool{}

			for i := range sources {
				t := sources[i].SourceKey.Type
				if t != "" && !seen[t] {
					seen[t] = true

					summary.Service.ServiceSourceTypes = append(summary.Service.ServiceSourceTypes, t)
				}
			}
		}
	}

	if presets, err := s.ds.GetPresets(match.account, match.device); err == nil {
		summary.Service.ServicePresetCount = len(presets)
		summary.Service.PresetsXMLPresent = len(presets) > 0
	}

	// Speaker-side: probe concurrently.
	if match.ip != "" {
		probeContext, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()

		var wg sync.WaitGroup

		wg.Add(3)

		go func() {
			defer wg.Done()

			summary.Speaker.Info = fetchSpeakerInfo(probeContext, match.ip)
		}()

		go func() {
			defer wg.Done()

			summary.Speaker.Sources = fetchSpeakerSources(probeContext, match.ip)
		}()

		go func() {
			defer wg.Done()

			summary.Speaker.Presets = fetchSpeakerPresets(probeContext, match.ip)
		}()

		wg.Wait()
	}

	// Pairing inference: derive from what we've learned.
	if summary.Speaker.Info.Reachable {
		summary.Pairing.Paired = summary.Speaker.Info.MargeAccountUUID != ""
		summary.Pairing.SpeakerMargeHost = hostFromURL(summary.Speaker.Info.MargeURL)
		summary.Pairing.MargeURLMatchesService = pairingHostMatches(
			summary.Pairing.SpeakerMargeHost,
			summary.Service.ExpectedHosts,
		)
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(summary); err != nil {
		http.Error(w, "encode: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

func fetchSpeakerInfo(ctx context.Context, ip string) speakerInfoSummary {
	url := fmt.Sprintf("http://%s:8090/info", ip)
	res := health.ProbeGet(ctx, url, 3*time.Second)

	out := speakerInfoSummary{
		probeOutcome: probeOutcome{
			Reachable:   res.Reachable,
			StatusCode:  res.Status,
			Err:         res.Err,
			CurlCommand: res.CurlCommand,
		},
	}

	if !res.Reachable || res.Status != 200 {
		return out
	}

	var parsed struct {
		XMLName          xml.Name `xml:"info"`
		Name             string   `xml:"name"`
		Type             string   `xml:"type"`
		MargeAccountUUID string   `xml:"margeAccountUUID"`
		MargeURL         string   `xml:"margeURL"`
	}

	if err := xml.Unmarshal(res.Body, &parsed); err != nil {
		out.Err = "parse: " + err.Error()
		return out
	}

	out.Name = parsed.Name
	out.Type = parsed.Type
	out.MargeAccountUUID = parsed.MargeAccountUUID
	out.MargeURL = parsed.MargeURL

	return out
}

func fetchSpeakerSources(ctx context.Context, ip string) speakerSourcesSummary {
	url := fmt.Sprintf("http://%s:8090/sources", ip)
	res := health.ProbeGet(ctx, url, 3*time.Second)

	out := speakerSourcesSummary{
		probeOutcome: probeOutcome{
			Reachable:   res.Reachable,
			StatusCode:  res.Status,
			Err:         res.Err,
			CurlCommand: res.CurlCommand,
		},
	}

	if !res.Reachable || res.Status != 200 {
		return out
	}

	var parsed struct {
		XMLName xml.Name `xml:"sources"`
		Items   []struct {
			Source string `xml:"source,attr"`
		} `xml:"sourceItem"`
	}

	if err := xml.Unmarshal(res.Body, &parsed); err != nil {
		out.Err = "parse: " + err.Error()
		return out
	}

	seen := map[string]bool{}

	for i := range parsed.Items {
		s := parsed.Items[i].Source
		if s != "" && !seen[s] {
			seen[s] = true
			out.Types = append(out.Types, s)
		}
	}

	return out
}

func fetchSpeakerPresets(ctx context.Context, ip string) speakerPresetsSummary {
	url := fmt.Sprintf("http://%s:8090/presets", ip)
	res := health.ProbeGet(ctx, url, 3*time.Second)

	out := speakerPresetsSummary{
		probeOutcome: probeOutcome{
			Reachable:   res.Reachable,
			StatusCode:  res.Status,
			Err:         res.Err,
			CurlCommand: res.CurlCommand,
		},
	}

	if !res.Reachable || res.Status != 200 {
		return out
	}

	var parsed struct {
		XMLName xml.Name `xml:"presets"`
		Presets []struct {
			ID string `xml:"id,attr"`
		} `xml:"preset"`
	}

	if err := xml.Unmarshal(res.Body, &parsed); err != nil {
		out.Err = "parse: " + err.Error()
		return out
	}

	for i := range parsed.Presets {
		if id := parsed.Presets[i].ID; id != "" {
			out.IDs = append(out.IDs, id)
		}
	}

	return out
}

func hostFromURL(raw string) string {
	if raw == "" {
		return ""
	}

	// Trim scheme prefix
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(raw, scheme) {
			raw = raw[len(scheme):]
			break
		}
	}

	// Trim path
	if i := strings.IndexByte(raw, '/'); i >= 0 {
		raw = raw[:i]
	}

	// Trim port
	if i := strings.IndexByte(raw, ':'); i >= 0 {
		raw = raw[:i]
	}

	return strings.ToLower(strings.TrimSpace(raw))
}

func pairingHostMatches(host string, expected []string) bool {
	if host == "" {
		return false
	}

	host = strings.ToLower(host)

	for _, h := range expected {
		if strings.ToLower(strings.TrimSpace(h)) == host {
			return true
		}
	}

	return false
}
