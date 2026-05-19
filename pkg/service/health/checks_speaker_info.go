package health

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDSpeakerInfoReachable is the registry id of the speaker
// reachability check.
const CheckIDSpeakerInfoReachable = "speaker_info_reachable"

// speakerInfoXML mirrors only the fields we need from the
// speaker's :8090/info XML response. Duplicated here (rather than
// imported from pkg/service/setup) to keep the health package
// free of cross-package dependencies that would pull in SSH,
// telnet, certmgr, etc.
type speakerInfoXML struct {
	XMLName          xml.Name `xml:"info"`
	DeviceID         string   `xml:"deviceID,attr"`
	Name             string   `xml:"name"`
	MargeAccountUUID string   `xml:"margeAccountUUID"`
	MargeURL         string   `xml:"margeURL"`
}

// RegisterSpeakerInfoReachable registers the speaker_info_reachable
// check against r. The check iterates every known device in the
// datastore, probes its :8090/info endpoint, and emits findings
// for unreachable speakers and speakers paired with an empty
// margeAccountUUID (a known TPDA failure mode — see
// discussion #223).
func RegisterSpeakerInfoReachable(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDSpeakerInfoReachable,
		Title: "Speakers respond on :8090/info",
		Run: func() []Finding {
			return runSpeakerInfoReachable(ds)
		},
	})
}

func runSpeakerInfoReachable(ds *datastore.DataStore) []Finding {
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

	var findings []Finding

	for i := range devices {
		dev := &devices[i]
		if dev.IPAddress == "" {
			continue
		}

		findings = append(findings, probeAndAssessSpeaker(dev.AccountID, dev.DeviceID, dev.IPAddress)...)
	}

	return findings
}

func probeAndAssessSpeaker(account, deviceID, ipAddress string) []Finding {
	probeURL := fmt.Sprintf("http://%s:8090/info", ipAddress)
	return probeAndAssessSpeakerWithURL(account, deviceID, probeURL)
}

// probeAndAssessSpeakerWithURL is the same as probeAndAssessSpeaker
// but takes the full URL directly. Used by tests that need to point
// at an httptest.Server, since those bind to random ports rather
// than :8090.
func probeAndAssessSpeakerWithURL(account, deviceID, probeURL string) []Finding {
	target := Target{Account: account, Device: deviceID}

	res := ProbeGet(context.Background(), probeURL, 2*time.Second)

	if !res.Reachable {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Speaker /info is not reachable from this host.",
			Details:  "If AfterTouch is hosted off the speaker's LAN (e.g. behind a reverse proxy or in a cloud), the service can't reach the speaker directly. Run the command below from a host that can.",
			ManualCommands: []ManualCommand{{
				Label:   "Fetch /info from your network:",
				Command: res.CurlCommand,
				Hint:    "Paste the response into a bug report or compare margeAccountUUID/margeURL with what AfterTouch expects.",
			}},
		}}
	}

	if res.Status != 200 {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  fmt.Sprintf("Speaker returned HTTP %d for /info.", res.Status),
			Details:  "Expected 200. Either the speaker is in a transient state or the IP belongs to a different device now.",
		}}
	}

	var parsed speakerInfoXML
	if err := xml.Unmarshal(res.Body, &parsed); err != nil {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Speaker replied but the /info body is not valid XML.",
			Details:  "Parse error: " + err.Error(),
		}}
	}

	var out []Finding

	if parsed.MargeAccountUUID == "" {
		out = append(out, Finding{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Speaker reports an empty <margeAccountUUID>.",
			Details:  "The speaker is reachable but isn't bound to any Marge account. Playback selection will fail with INVALID_SOURCE until pairing completes. See discussion #223 for the full symptom chain.",
		})
	}

	if parsed.MargeURL == "" {
		out = append(out, Finding{
			Severity: SeverityInfo,
			Target:   target,
			Message:  "Speaker reports an empty <margeURL>.",
			Details:  "The speaker hasn't been told where the cloud lives. This usually clears up after the first successful /info request from the service.",
		})
	}

	return out
}
