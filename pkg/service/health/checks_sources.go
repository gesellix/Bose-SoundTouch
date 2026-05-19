package health

import (
	"fmt"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckSourcesXMLPresent is the built-in check for missing
// Sources.xml on paired devices. Background:
// initializeDefaultSources in cmd/soundtouch-service/main.go only
// runs at startup over devices that already exist on disk, so a
// device that first checks in *after* boot never gets its default
// Sources.xml materialised. The speaker then absorbs whatever the
// service serves on /streaming/account/{id}/full — usually without
// TUNEIN — and playback fails with 1005 long after migration
// looked successful. See discussion #295 for the trace.
const (
	CheckIDSourcesXMLPresent  = "sources_xml_present"
	FixIDCreateDefaultSources = "create_default_sources"
)

// RegisterSourcesXMLPresent registers the sources_xml_present
// check and its create_default_sources quick fix against r,
// binding both to ds.
func RegisterSourcesXMLPresent(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDSourcesXMLPresent,
		Title: "Default sources are materialised on disk",
		Run: func() []Finding {
			return runSourcesXMLPresent(ds)
		},
	})

	r.RegisterFix(CheckIDSourcesXMLPresent, FixIDCreateDefaultSources, func(target Target) (string, error) {
		return fixCreateDefaultSources(ds, target)
	})
}

func runSourcesXMLPresent(ds *datastore.DataStore) []Finding {
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

		if ds.HasConfiguredSources(dev.AccountID, dev.DeviceID) {
			continue
		}

		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Target:   Target{Account: dev.AccountID, Device: dev.DeviceID},
			Message:  "Sources.xml is missing for this device.",
			Details:  "The /streaming/account/{id}/full response will not advertise the default sources (TUNEIN, RADIO_BROWSER, AUX). Playback may fail with error 1005 until a Sources.xml is materialised.",
			QuickFixes: []QuickFix{
				{
					ID:    FixIDCreateDefaultSources,
					Label: "Create default Sources.xml",
				},
			},
		})
	}

	return findings
}

func fixCreateDefaultSources(ds *datastore.DataStore, target Target) (string, error) {
	if ds == nil {
		return "", fmt.Errorf("datastore unavailable")
	}

	if target.Account == "" || target.Device == "" {
		return "", fmt.Errorf("account and device are required")
	}

	defaults := ds.GetDefaultSources()
	if err := ds.SaveConfiguredSources(target.Account, target.Device, defaults); err != nil {
		return "", fmt.Errorf("save Sources.xml: %w", err)
	}

	return fmt.Sprintf("Wrote default Sources.xml for %s", target.Device), nil
}
