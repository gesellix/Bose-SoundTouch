package health

import (
	"fmt"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDOrionPaths is the registry id of the dead-orion-URL
// detector.
const CheckIDOrionPaths = "orion_paths_in_presets"

// orionPathFragment is the dead Bose cloud path that lingers in
// presets saved before the May 2026 shutdown. Anything matching
// this in a preset Location will be fetched against the dead
// public host instead of routing through BMX, so playback fails.
const orionPathFragment = "content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion"

// RegisterOrionPathsCheck registers a passive scan over every
// device's service-side Presets.xml looking for entries whose
// Location contains the dead Bose cloud orion path. Recurring
// pattern from #218 and #224: presets that worked pre-shutdown
// but silently fail post-migration because the location still
// references content.api.bose.io.
//
// Pure filesystem read; no probes. Always runs.
func RegisterOrionPathsCheck(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDOrionPaths,
		Title: "Presets don't reference the dead Bose cloud orion path",
		Run: func() []Finding {
			return runOrionPathsCheck(ds)
		},
	})
}

func runOrionPathsCheck(ds *datastore.DataStore) []Finding {
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

		presets, err := ds.GetPresets(dev.AccountID, dev.DeviceID)
		if err != nil {
			continue
		}

		hits := findOrionHits(presets)
		if len(hits) == 0 {
			continue
		}

		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Target:   Target{Account: dev.AccountID, Device: dev.DeviceID},
			Message: fmt.Sprintf(
				"%d preset(s) reference the dead Bose orion path; playback will fail until rewritten: %s.",
				len(hits), strings.Join(hits, ", "),
			),
			Details: "The Bose cloud shut down in May 2026, but presets created before then still carry `content.api.bose.io/.../orion` URLs in their <location>. Recurring debug pattern from #218 and #224. The fix is to rewrite the Location to the BMX-relative form (or simply re-create the preset against TUNEIN / RADIO_BROWSER).",
			ManualCommands: []ManualCommand{{
				Label: "Strip the dead host from Presets.xml on the service host:",
				Command: fmt.Sprintf(
					"sed -i 's|https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion||g' /app/data/accounts/%s/devices/%s/Presets.xml",
					dev.AccountID, dev.DeviceID,
				),
				Hint: "Adjust /app/data to your actual data dir. The leading `https://...orion` is stripped so the remaining /v1/playback/... path is BMX-relative and routes through this service.",
			}},
		})
	}

	return findings
}

// findOrionHits returns a sorted slice of preset slot labels (or
// preset names when the slot is missing) that contain the dead
// orion path. Returned in input order.
func findOrionHits(presets []models.ServicePreset) []string {
	var hits []string

	for i := range presets {
		loc := presets[i].Location
		if loc == "" || !strings.Contains(loc, orionPathFragment) {
			continue
		}

		label := presets[i].ID
		if label == "" {
			label = presets[i].Name
		}

		if label == "" {
			label = "(unnamed)"
		}

		hits = append(hits, label)
	}

	return hits
}
