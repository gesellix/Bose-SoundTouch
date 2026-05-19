package health

import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDSourcesXMLDiff is the registry id of the speaker-vs-service
// sources comparison check.
const CheckIDSourcesXMLDiff = "sources_xml_diff"

// speakerSourcesXML mirrors the speaker's /sources response.
// Schema example:
//
//	<sources deviceID="DEVICEID01">
//	  <sourceItem source="TUNEIN" status="READY" ... />
//	  <sourceItem source="AUX" sourceAccount="AUX" ... />AUX IN</sourceItem>
//	</sources>
//
// Note this is a *different* schema from the service-side
// Sources.xml, which is why we compare the set of source-key types
// rather than diffing the XML byte-for-byte.
type speakerSourcesXML struct {
	XMLName xml.Name `xml:"sources"`
	Items   []struct {
		Source string `xml:"source,attr"`
		Status string `xml:"status,attr"`
	} `xml:"sourceItem"`
}

// RegisterSourcesXMLDiff registers the sources_xml_diff check
// against r. For each known device it fetches the speaker's
// /sources, compares the source-type set against the service's
// Sources.xml, and emits findings for asymmetries.
func RegisterSourcesXMLDiff(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDSourcesXMLDiff,
		Title: "Speaker /sources matches service Sources.xml",
		Run: func() []Finding {
			return runSourcesXMLDiff(ds)
		},
	})
}

func runSourcesXMLDiff(ds *datastore.DataStore) []Finding {
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

		findings = append(findings, diffSourcesForDevice(ds, dev.AccountID, dev.DeviceID, dev.IPAddress)...)
	}

	return findings
}

func diffSourcesForDevice(ds *datastore.DataStore, account, deviceID, ipAddress string) []Finding {
	probeURL := fmt.Sprintf("http://%s:8090/sources", ipAddress)
	return diffSourcesForDeviceWithURL(ds, account, deviceID, ipAddress, probeURL)
}

// diffSourcesForDeviceWithURL is the same as diffSourcesForDevice
// but takes the speaker URL explicitly. Used by tests that point
// at an httptest.Server bound to a random port.
func diffSourcesForDeviceWithURL(ds *datastore.DataStore, account, deviceID, ipAddress, probeURL string) []Finding {
	target := Target{Account: account, Device: deviceID}

	// Service side: read configured sources (if Sources.xml is
	// missing, leave the set empty — the sources_xml_present check
	// already flags that case).
	serviceSet := map[string]bool{}

	if ds.HasConfiguredSources(account, deviceID) {
		sources, err := ds.GetConfiguredSources(account, deviceID)
		if err == nil {
			for i := range sources {
				if t := sources[i].SourceKey.Type; t != "" {
					serviceSet[t] = true
				}
			}
		}
	}

	res := ProbeGet(context.Background(), probeURL, 2*time.Second)
	if !res.Reachable {
		// Don't double-warn — the speaker_info_reachable check
		// will already have flagged unreachable speakers. Surface
		// only the manual command for this specific endpoint.
		return []Finding{{
			Severity: SeverityInfo,
			Target:   target,
			Message:  "Couldn't fetch /sources from the speaker; can't compare.",
			ManualCommands: []ManualCommand{{
				Label:   "Compare manually:",
				Command: res.CurlCommand,
				Hint:    "Paste the result here in a future revision, or just diff the source list against the service's Sources.xml by eye.",
			}},
		}}
	}

	if res.Status != 200 {
		return []Finding{{
			Severity: SeverityInfo,
			Target:   target,
			Message:  fmt.Sprintf("Speaker /sources returned HTTP %d.", res.Status),
		}}
	}

	speakerSet, parseErr := parseSpeakerSources(res.Body)
	if parseErr != nil {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Speaker /sources reply isn't valid XML.",
			Details:  parseErr.Error(),
		}}
	}

	missingOnSpeaker := setDifference(serviceSet, speakerSet)
	missingOnService := setDifference(speakerSet, serviceSet)

	var findings []Finding

	if len(missingOnSpeaker) > 0 {
		notifyCmd := fmt.Sprintf(
			"curl -sS -X POST 'http://%s:8090/notification' -H 'Content-Type: application/xml' -d '<updates deviceID=\"%s\"><sourcesUpdated/></updates>'",
			ipAddress, deviceID,
		)

		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Target:   target,
			Message: fmt.Sprintf(
				"Speaker is missing %d source type(s) the service advertises: %s.",
				len(missingOnSpeaker), strings.Join(missingOnSpeaker, ", "),
			),
			Details: "After a power-cycle the speaker fetches /full from the service and re-registers its source list. Forcing a sourcesUpdated notification triggers the same refresh without rebooting.",
			ManualCommands: []ManualCommand{{
				Label:   "Trigger a sources refresh on the speaker:",
				Command: notifyCmd,
				Hint:    "Run on a host that can reach the speaker on port 8090. A power-cycle is sometimes also required for the new source types to fully register (see docs/reference/radio-browser.md:21).",
			}},
		})
	}

	if len(missingOnService) > 0 {
		findings = append(findings, Finding{
			Severity: SeverityInfo,
			Target:   target,
			Message: fmt.Sprintf(
				"Speaker advertises %d source type(s) the service doesn't know about: %s.",
				len(missingOnService), strings.Join(missingOnService, ", "),
			),
			Details: "Usually harmless — the speaker can keep AUX or other local sources without the service knowing. But if a managed source is in this list, check the service Sources.xml.",
		})
	}

	return findings
}

func parseSpeakerSources(body []byte) (map[string]bool, error) {
	var parsed speakerSourcesXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	out := make(map[string]bool, len(parsed.Items))
	for i := range parsed.Items {
		if s := parsed.Items[i].Source; s != "" {
			out[s] = true
		}
	}

	return out, nil
}

// setDifference returns the keys in a that are not in b, sorted.
func setDifference(a, b map[string]bool) []string {
	out := make([]string, 0)

	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}

	sort.Strings(out)

	return out
}
