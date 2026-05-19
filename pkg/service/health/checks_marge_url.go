package health

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDSpeakerMargeURL is the registry id of the Marge-URL
// consistency check.
const CheckIDSpeakerMargeURL = "speaker_marge_url"

// RegisterSpeakerMargeURLCheck registers the speaker_marge_url
// check. For each device it probes /info, extracts <margeURL>, and
// compares the hostname against the service's expected-hosts list
// (serverURL host + httpsServerURL host + --tls-extra-host values).
// If they don't match, the speaker is talking to a different
// endpoint than this service thinks it serves — usually a sign
// that AfterTouch was reconfigured after the speaker was
// migrated, or that the speaker is pointed at the wrong DNS name.
//
// expectedHostsFn is a closure so the check picks up config
// changes without re-registration (today these change only at
// restart, but the closure costs nothing).
func RegisterSpeakerMargeURLCheck(r *Registry, ds *datastore.DataStore, expectedHostsFn func() []string) {
	r.Register(Check{
		ID:    CheckIDSpeakerMargeURL,
		Title: "Speaker <margeURL> matches AfterTouch's configured hosts",
		Run: func() []Finding {
			return runSpeakerMargeURLCheck(ds, expectedHostsFn)
		},
	})
}

func runSpeakerMargeURLCheck(ds *datastore.DataStore, expectedHostsFn func() []string) []Finding {
	if ds == nil {
		return nil
	}

	devices, err := ds.ListAllDevices()
	if err != nil {
		return []Finding{{
			Severity: SeverityError,
			Message:  "Could not enumerate devices: " + err.Error(),
		}}
	}

	expected := normaliseHosts(expectedHostsFn())

	var findings []Finding

	for i := range devices {
		dev := &devices[i]
		if dev.IPAddress == "" || dev.DeviceID == "" {
			continue
		}

		findings = append(findings, assessMargeURLForDevice(dev.AccountID, dev.DeviceID, dev.IPAddress, expected)...)
	}

	return findings
}

func assessMargeURLForDevice(account, deviceID, ipAddress string, expected map[string]bool) []Finding {
	probeURL := fmt.Sprintf("http://%s:8090/info", ipAddress)
	return assessMargeURLForDeviceWithURL(account, deviceID, probeURL, expected)
}

// assessMargeURLForDeviceWithURL is the same but takes the URL
// directly. Used by tests bound to an httptest.Server.
func assessMargeURLForDeviceWithURL(account, deviceID, probeURL string, expected map[string]bool) []Finding {
	target := Target{Account: account, Device: deviceID}

	res := ProbeGet(context.Background(), probeURL, 2*time.Second)
	if !res.Reachable || res.Status != 200 {
		// speaker_info_reachable already covers these cases.
		return nil
	}

	var parsed speakerInfoXML
	if err := xml.Unmarshal(res.Body, &parsed); err != nil {
		return nil
	}

	if parsed.MargeURL == "" {
		return nil
	}

	margeHost := hostFromURL(parsed.MargeURL)
	if margeHost == "" {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  fmt.Sprintf("Speaker reports an unparseable <margeURL>: %q", parsed.MargeURL),
		}}
	}

	if expected[margeHost] {
		return nil
	}

	return []Finding{{
		Severity: SeverityWarning,
		Target:   target,
		Message: fmt.Sprintf(
			"Speaker is pointed at %s, which isn't in the service's configured hosts.",
			parsed.MargeURL,
		),
		Details: fmt.Sprintf(
			"Configured hosts: %s. If the speaker should reach this service via %q, restart with `--tls-extra-host=%s` so the served TLS cert covers it. Otherwise, re-migrate the speaker to the correct URL.",
			joinHosts(expected), margeHost, margeHost,
		),
		ManualCommands: []ManualCommand{{
			Label:   "Add the speaker's expected hostname to AfterTouch's TLS cert:",
			Command: fmt.Sprintf("soundtouch-service --tls-extra-host=%s …", margeHost),
			Hint:    "Append to your existing service command-line / env (TLS_EXTRA_HOST). Requires a restart.",
		}},
	}}
}

func normaliseHosts(in []string) map[string]bool {
	out := make(map[string]bool, len(in))

	for _, h := range in {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "" {
			continue
		}

		// Accept either bare host or URL-style input — be lenient
		// since the registration call site may evolve.
		if hostOnly := hostFromURL(h); hostOnly != "" {
			out[hostOnly] = true
		} else {
			out[h] = true
		}
	}

	return out
}

func hostFromURL(raw string) string {
	if raw == "" {
		return ""
	}

	if !strings.Contains(raw, "://") {
		// Treat as a bare host. Strip an optional :port suffix.
		if i := strings.IndexByte(raw, ':'); i >= 0 {
			return strings.ToLower(strings.TrimSpace(raw[:i]))
		}

		return strings.ToLower(strings.TrimSpace(raw))
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return strings.ToLower(u.Hostname())
}

func joinHosts(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	if len(keys) == 0 {
		return "(none configured)"
	}

	return strings.Join(keys, ", ")
}
