package health

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDPresetsConsistency is the registry id of the consistency check.
const CheckIDPresetsConsistency = "presets_recents_sources_consistency"

// speakerPresetsConsistencyXML mirrors enough of :8090/presets to extract
// slot id, source/location and itemName for cross-side comparison.
type speakerPresetsConsistencyXML struct {
	XMLName xml.Name `xml:"presets"`
	Presets []struct {
		ID          string `xml:"id,attr"`
		ContentItem struct {
			Source        string `xml:"source,attr"`
			SourceAccount string `xml:"sourceAccount,attr"`
			Location      string `xml:"location,attr"`
			ItemName      string `xml:"itemName"`
		} `xml:"ContentItem"`
	} `xml:"preset"`
}

// speakerRecentsConsistencyXML mirrors :8090/recents.
type speakerRecentsConsistencyXML struct {
	XMLName xml.Name `xml:"recents"`
	Recents []struct {
		DeviceID    string `xml:"deviceID,attr"`
		UtcTime     string `xml:"utcTime,attr"`
		ID          string `xml:"id,attr"`
		ContentItem struct {
			Source        string `xml:"source,attr"`
			SourceAccount string `xml:"sourceAccount,attr"`
			Location      string `xml:"location,attr"`
			ItemName      string `xml:"itemName"`
		} `xml:"contentItem"`
	} `xml:"recent"`
}

// speakerSourcesConsistencyXML mirrors :8090/sources.
type speakerSourcesConsistencyXML struct {
	XMLName xml.Name `xml:"sources"`
	Items   []struct {
		Source        string `xml:"source,attr"`
		SourceAccount string `xml:"sourceAccount,attr"`
	} `xml:"sourceItem"`
}

// RegisterPresetsConsistencyCheck registers the cross-reference check.
// For every paired device with a known IP, it builds two ConsistencyViews
// (speaker, service), runs the internal-consistency pass on each, then
// the cross-side pass — and surfaces every detected issue as a Finding so
// the operator can drill into "why aren't my presets behaving" without
// reading service logs or curl-ing XML by hand.
func RegisterPresetsConsistencyCheck(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDPresetsConsistency,
		Title: "Presets, recents and sources cross-reference consistently",
		Run: func() []Finding {
			return runPresetsConsistencyCheck(ds)
		},
	})
}

func runPresetsConsistencyCheck(ds *datastore.DataStore) []Finding {
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
		if dev.AccountID == "" || dev.DeviceID == "" {
			continue
		}

		findings = append(findings, checkOneDeviceConsistency(ds, dev.AccountID, dev.DeviceID, dev.IPAddress)...)
	}

	return findings
}

func checkOneDeviceConsistency(ds *datastore.DataStore, account, deviceID, ipAddress string) []Finding {
	target := Target{Account: account, Device: deviceID}

	serviceView, err := loadServiceView(ds, account, deviceID)
	if err != nil {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Could not read service-side state for consistency check.",
			Details:  err.Error(),
		}}
	}

	var findings []Finding

	// Internal consistency is meaningful only on the service side —
	// the speaker manages its own preset/source coherence locally,
	// and its /sources list deliberately omits streaming sources
	// (which would always trigger spurious "dangling" findings).
	findings = append(findings, issuesToFindings(target, CheckInternalConsistency(serviceView), SeverityWarning)...)

	if ipAddress == "" {
		findings = append(findings, Finding{
			Severity: SeverityInfo,
			Target:   target,
			Message:  "No IP recorded for device; skipping speaker-side consistency check. Service-side internal consistency was checked.",
		})

		return findings
	}

	speakerView, probeIssue := loadSpeakerView(ipAddress)
	if probeIssue != nil {
		findings = append(findings, Finding{
			Severity:       SeverityInfo,
			Target:         target,
			Message:        "Couldn't fetch speaker XML for cross-side comparison; service-side internal consistency was still checked.",
			Details:        probeIssue.Detail,
			ManualCommands: probeIssue.ManualCommands,
		})

		return findings
	}

	// "Unsynced device" short-circuit: when the service has no
	// presets / recents / sources for a device that the speaker
	// clearly does have all three for, emit one consolidated
	// finding instead of a torrent of per-slot mismatches.
	if isServiceUnsynced(serviceView) && !isSpeakerEmpty(speakerView) {
		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Device has speaker state (presets, recents, sources) but service has nothing for it — looks like a missed pair/sync. Click \"Sync\" in the device tab or factory-reset and re-pair.",
		})

		return findings
	}

	findings = append(findings, issuesToFindings(target, CheckCrossSide(speakerView, serviceView), SeverityWarning)...)

	return findings
}

func isServiceUnsynced(v ConsistencyView) bool {
	return len(v.Presets) == 0 && len(v.Recents) == 0 && len(v.Sources) == 0
}

func isSpeakerEmpty(v ConsistencyView) bool {
	return len(v.Presets) == 0 && len(v.Recents) == 0 && len(v.Sources) == 0
}

func issuesToFindings(target Target, issues []ConsistencyIssue, severity Severity) []Finding {
	if len(issues) == 0 {
		return nil
	}

	out := make([]Finding, 0, len(issues))
	for _, iss := range issues {
		out = append(out, Finding{
			Severity: severity,
			Target:   target,
			Message:  string(iss.Kind) + " (" + iss.Side + "): " + iss.Detail,
		})
	}

	return out
}

func loadServiceView(ds *datastore.DataStore, account, deviceID string) (ConsistencyView, error) {
	presets, err := ds.GetPresets(account, deviceID)
	if err != nil {
		return ConsistencyView{}, fmt.Errorf("read presets: %w", err)
	}

	recents, err := ds.GetRecents(account, deviceID)
	if err != nil {
		return ConsistencyView{}, fmt.Errorf("read recents: %w", err)
	}

	sources, err := ds.GetConfiguredSources(account, deviceID)
	if err != nil {
		return ConsistencyView{}, fmt.Errorf("read sources: %w", err)
	}

	view := ConsistencyView{Label: "service"}

	// Build sourceID -> SourceKeyType index up front so we can resolve
	// the *effective* source for each preset/recent. syncPresets and
	// syncRecents in sync.go currently persist the upstream
	// FullResponseSource.Type ("Audio") into ServicePreset.Source,
	// which is meaningless next to the speaker's actual source name.
	// Resolve via SourceID so the cross-side diff compares like with
	// like; fall back to the persisted Source string when the SourceID
	// is empty or doesn't resolve.
	sourceByID := make(map[string]string, len(sources))
	for i := range sources {
		if sources[i].ID != "" && sources[i].SourceKeyType != "" {
			sourceByID[sources[i].ID] = sources[i].SourceKeyType
		}
	}

	resolveSource := func(persisted, sourceID string) string {
		if sourceID != "" {
			if resolved := sourceByID[sourceID]; resolved != "" {
				return resolved
			}
		}

		return persisted
	}

	for i := range presets {
		view.Presets = append(view.Presets, ConsistencyPreset{
			Slot:     presets[i].ButtonNumber,
			Source:   resolveSource(presets[i].Source, presets[i].SourceID),
			SourceID: presets[i].SourceID,
			Location: presets[i].Location,
			Name:     presets[i].Name,
		})
	}

	for i := range recents {
		view.Recents = append(view.Recents, ConsistencyRecent{
			ID:       recents[i].ID,
			Source:   resolveSource(recents[i].Source, recents[i].SourceID),
			SourceID: recents[i].SourceID,
			Location: recents[i].Location,
			Name:     recents[i].Name,
		})
	}

	for i := range sources {
		view.Sources = append(view.Sources, ConsistencySource{
			ID:      sources[i].ID,
			Type:    sources[i].SourceKeyType,
			Account: sources[i].SourceKeyAccount,
		})
	}

	return view, nil
}

// probeFailure captures everything we know about a failed speaker probe
// so we can render a single Info finding instead of three separate ones
// when the speaker is just unreachable.
type probeFailure struct {
	Detail         string
	ManualCommands []ManualCommand
}

func loadSpeakerView(ipAddress string) (ConsistencyView, *probeFailure) {
	view := ConsistencyView{Label: "speaker"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	presetsRes := ProbeGet(ctx, fmt.Sprintf("http://%s:8090/presets", ipAddress), 2*time.Second)
	if !presetsRes.Reachable {
		return view, &probeFailure{
			Detail: "speaker /presets unreachable: " + presetsRes.Err,
			ManualCommands: []ManualCommand{
				{Label: "From a host on the speaker's LAN, fetch /presets:", Command: presetsRes.CurlCommand},
				{Label: "And /recents:", Command: fmt.Sprintf("curl -sS http://%s:8090/recents", ipAddress)},
				{Label: "And /sources:", Command: fmt.Sprintf("curl -sS http://%s:8090/sources", ipAddress)},
				{Label: "Compare the three side-by-side against AfterTouch's stored state.", Command: ""},
			},
		}
	}

	if presetsRes.Status == 200 {
		var parsed speakerPresetsConsistencyXML
		if err := xml.Unmarshal(presetsRes.Body, &parsed); err == nil {
			for i := range parsed.Presets {
				p := parsed.Presets[i]
				if p.ID == "" {
					continue
				}

				view.Presets = append(view.Presets, ConsistencyPreset{
					Slot:     p.ID,
					Source:   p.ContentItem.Source,
					Location: p.ContentItem.Location,
					Name:     p.ContentItem.ItemName,
				})
			}
		}
	}

	recentsRes := ProbeGet(ctx, fmt.Sprintf("http://%s:8090/recents", ipAddress), 2*time.Second)
	if recentsRes.Reachable && recentsRes.Status == 200 {
		var parsed speakerRecentsConsistencyXML
		if err := xml.Unmarshal(recentsRes.Body, &parsed); err == nil {
			for i := range parsed.Recents {
				r := parsed.Recents[i]
				if r.ID == "" {
					continue
				}

				view.Recents = append(view.Recents, ConsistencyRecent{
					ID:       r.ID,
					Source:   r.ContentItem.Source,
					Location: r.ContentItem.Location,
					Name:     r.ContentItem.ItemName,
				})
			}
		}
	}

	sourcesRes := ProbeGet(ctx, fmt.Sprintf("http://%s:8090/sources", ipAddress), 2*time.Second)
	if sourcesRes.Reachable && sourcesRes.Status == 200 {
		var parsed speakerSourcesConsistencyXML
		if err := xml.Unmarshal(sourcesRes.Body, &parsed); err == nil {
			seen := map[string]bool{}

			for i := range parsed.Items {
				key := parsed.Items[i].Source + "|" + parsed.Items[i].SourceAccount
				if parsed.Items[i].Source == "" || seen[key] {
					continue
				}

				seen[key] = true

				view.Sources = append(view.Sources, ConsistencySource{
					Type:    parsed.Items[i].Source,
					Account: parsed.Items[i].SourceAccount,
				})
			}
		}
	}

	return view, nil
}

// Compile-time guard: the models package must keep ServicePreset's
// embedded ServiceContentItem layout so that the field access in
// loadServiceView stays valid. Spotting a type change here is cheaper
// than via a runtime check.
var _ = models.ServicePreset{}.ButtonNumber
