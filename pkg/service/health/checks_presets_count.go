package health

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDPresetsCount is the registry id of the speaker-vs-service
// preset count check.
const CheckIDPresetsCount = "speaker_presets_count"

// speakerPresetsXML mirrors just enough of the speaker's :8090/presets
// XML to count slots. The schema is the same as on the service side
// but with <ContentItem> (capitalised) inside <preset>.
type speakerPresetsXML struct {
	XMLName xml.Name `xml:"presets"`
	Presets []struct {
		ID string `xml:"id,attr"`
	} `xml:"preset"`
}

// RegisterPresetsCountCheck registers a check that fetches each
// device's :8090/presets and compares the count against the
// service's Presets.xml. Useful as a one-step "is the speaker
// seeing the same presets the service thinks it has?" sanity
// check — the question that triggers issue #253, #269, #308,
// among others.
func RegisterPresetsCountCheck(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDPresetsCount,
		Title: "Speaker preset count matches service Presets.xml",
		Run: func() []Finding {
			return runPresetsCountCheck(ds)
		},
	})
}

func runPresetsCountCheck(ds *datastore.DataStore) []Finding {
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
		if dev.IPAddress == "" || dev.AccountID == "" || dev.DeviceID == "" {
			continue
		}

		findings = append(findings, comparePresetsForDevice(ds, dev.AccountID, dev.DeviceID, dev.IPAddress)...)
	}

	return findings
}

func comparePresetsForDevice(ds *datastore.DataStore, account, deviceID, ipAddress string) []Finding {
	probeURL := fmt.Sprintf("http://%s:8090/presets", ipAddress)
	return comparePresetsForDeviceWithURL(ds, account, deviceID, probeURL)
}

// comparePresetsForDeviceWithURL is the same but takes the URL
// directly; used by tests bound to an httptest.Server.
func comparePresetsForDeviceWithURL(ds *datastore.DataStore, account, deviceID, probeURL string) []Finding {
	target := Target{Account: account, Device: deviceID}

	servicePresets, err := ds.GetPresets(account, deviceID)
	if err != nil {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Could not read service-side Presets.xml.",
			Details:  err.Error(),
		}}
	}

	serviceCount := len(servicePresets)

	res := ProbeGet(context.Background(), probeURL, 2*time.Second)
	if !res.Reachable {
		return []Finding{{
			Severity: SeverityInfo,
			Target:   target,
			Message:  fmt.Sprintf("Couldn't fetch /presets from the speaker; can't compare. Service Presets.xml has %d entries.", serviceCount),
			ManualCommands: []ManualCommand{{
				Label:   "Fetch /presets from your network:",
				Command: res.CurlCommand,
				Hint:    "Compare the count and slot IDs against what AfterTouch has for this device.",
			}},
		}}
	}

	if res.Status != 200 {
		return []Finding{{
			Severity: SeverityInfo,
			Target:   target,
			Message:  fmt.Sprintf("Speaker /presets returned HTTP %d.", res.Status),
		}}
	}

	var parsed speakerPresetsXML
	if err := xml.Unmarshal(res.Body, &parsed); err != nil {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Speaker /presets reply isn't valid XML.",
			Details:  err.Error(),
		}}
	}

	speakerCount := countNonEmpty(parsed)

	if speakerCount == serviceCount {
		return nil
	}

	severity := SeverityInfo
	if speakerCount == 0 && serviceCount > 0 {
		// Speaker shows nothing while the service has presets —
		// this is the post-reset preset-loss class from
		// discussion #295 and #235.
		severity = SeverityWarning
	}

	return []Finding{{
		Severity: severity,
		Target:   target,
		Message: fmt.Sprintf(
			"Speaker shows %d preset slot(s); service Presets.xml has %d.",
			speakerCount, serviceCount,
		),
		Details: "If the speaker shows fewer than the service, a power-cycle or a sourcesUpdated notification usually re-syncs. If it shows more, the service may have stale entries or the speaker is still holding pre-migration state.",
	}}
}

// countNonEmpty returns the number of <preset> entries with a
// non-empty id. Empty slots in the speaker's response (e.g. the
// six fixed buttons with no programmed preset) are not counted.
func countNonEmpty(parsed speakerPresetsXML) int {
	n := 0

	for i := range parsed.Presets {
		if parsed.Presets[i].ID != "" {
			n++
		}
	}

	return n
}
