package health

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDPresetsConsistency is the registry id of the consistency check.
const CheckIDPresetsConsistency = "presets_recents_sources_consistency"

// FixIDDeleteOrphanAccountEntry is the QuickFix that removes a stale
// account directory for a device after the operator has confirmed the
// speaker isn't currently targeting it.
const FixIDDeleteOrphanAccountEntry = "delete_orphan_account_entry"

// FixIDReclassifyCanonicalSourceIDs is the QuickFix that rewrites
// non-canonical IDs on built-in radio sources back to their canonical
// values (TUNEIN→10004, INTERNET_RADIO→10002, …), and updates any
// preset/recent <sourceid> references to match. Offline-only — no
// speaker access required.
const FixIDReclassifyCanonicalSourceIDs = "reclassify_canonical_source_ids"

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

	r.RegisterFix(CheckIDPresetsConsistency, FixIDDeleteOrphanAccountEntry, func(target Target) (string, error) {
		return deleteOrphanAccountEntry(ds, target)
	})

	r.RegisterFix(CheckIDPresetsConsistency, FixIDReclassifyCanonicalSourceIDs, func(target Target) (string, error) {
		return reclassifyCanonicalSourceIDs(ds, target)
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

	// Surface orphan "default"-account entries for devices that are
	// also paired under a real account. The speaker pairs to one
	// account at a time; the leftover "default" record from the
	// pre-pair phase is stale state the operator can safely remove.
	findings = append(findings, detectOrphanDefaultEntries(ds, devices)...)

	for i := range devices {
		dev := &devices[i]
		if dev.AccountID == "" || dev.DeviceID == "" {
			continue
		}

		findings = append(findings, checkOneDeviceConsistency(ds, dev.AccountID, dev.DeviceID, dev.IPAddress)...)
	}

	return findings
}

// detectOrphanDefaultEntries flags devices that exist under multiple
// account directories. The speaker decides which account it belongs to
// via the URL of every PUT it sends; any other account entry on disk
// is leftover state from a previous pairing. The active account is
// the one ListAllDevices' dedup currently exposes (with "default"
// already deprioritised); the stale ones each get a finding with a
// confirm-gated QuickFix so the operator can delete them one at a
// time after verifying via the service log which account the speaker
// is actually targeting.
func detectOrphanDefaultEntries(ds *datastore.DataStore, paired []models.ServiceDeviceInfo) []Finding {
	activeAccount := map[string]string{} // deviceID -> the account ListAllDevices picked

	for i := range paired {
		if paired[i].DeviceID == "" {
			continue
		}

		activeAccount[paired[i].DeviceID] = paired[i].AccountID
	}

	var findings []Finding

	for deviceID, active := range activeAccount {
		allAccounts := ds.AllAccountsForDevice(deviceID)
		if len(allAccounts) <= 1 {
			continue
		}

		for _, acc := range allAccounts {
			if acc == active {
				continue
			}

			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Target:   Target{Account: acc, Device: deviceID},
				Message:  "Stale account entry: device " + deviceID + " also has state under account " + safeQuoteFinding(acc) + " — likely leftover from a previous pairing. The currently-active account is " + safeQuoteFinding(active) + ".",
				Details:  "Before deleting, verify the speaker isn't currently PUTting to account " + acc + " by checking the service log for /streaming/account/" + acc + "/device/" + deviceID + "/... entries.",
				QuickFixes: []QuickFix{{
					ID:      FixIDDeleteOrphanAccountEntry,
					Label:   "Delete stale entry",
					Confirm: "Permanently delete <data-dir>/accounts/" + acc + "/devices/" + deviceID + "/? This removes Presets.xml, Recents.xml, Sources.xml and DeviceInfo.xml for this stale pairing. The active account " + active + " is not touched.",
				}},
				ManualCommands: []ManualCommand{{
					Label:   "Or remove from a shell:",
					Command: "rm -rf <data-dir>/accounts/" + acc + "/devices/" + deviceID,
					Hint:    "Substitute <data-dir> with the service's actual data directory (typically /var/lib/soundtouch-service).",
				}},
			})
		}
	}

	return findings
}

func safeQuoteFinding(s string) string {
	if s == "" {
		return `""`
	}

	return `"` + s + `"`
}

// deleteOrphanAccountEntry removes accounts/<target.Account>/devices/<target.Device>/.
// Called only after the operator has clicked through the Confirm dialog
// that the QuickFix surfaces; the framework is the gatekeeper, so this
// just executes. Logs the action for auditability.
func deleteOrphanAccountEntry(ds *datastore.DataStore, target Target) (string, error) {
	if target.Account == "" || target.Device == "" {
		return "", fmt.Errorf("account and device are both required")
	}

	if target.Account == accountIDDefaultPlaceholder {
		// Allowed — "default" is a frequent orphan source — but log
		// the explicit case so misuse stands out.
		log.Printf("[Health] deleteOrphanAccountEntry: deleting the \"default\" placeholder entry for device %s; this is normal after pairing completed", target.Device)
	}

	path := ds.AccountDeviceDir(target.Account, target.Device)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("orphan directory %s no longer exists; nothing to do", path)
	}

	if err := os.RemoveAll(path); err != nil {
		return "", fmt.Errorf("delete %s: %w", path, err)
	}

	log.Printf("[Health] Removed orphan account entry %s (account=%s device=%s) at operator request",
		path, target.Account, target.Device)

	return fmt.Sprintf("Removed stale account entry %s for device %s.", target.Account, target.Device), nil
}

// accountIDDefaultPlaceholder mirrors datastore.accountIDDefault for
// the health package; kept here to avoid widening the datastore
// package's exported surface.
const accountIDDefaultPlaceholder = "default"

// reclassifiableSource captures a single built-in radio source whose
// on-disk ID drifted from the canonical value. Built up by
// findReclassifiableSources and consumed by reclassifyCanonicalSourceIDs.
type reclassifiableSource struct {
	OldID   string
	NewID   string
	KeyType string // TUNEIN / INTERNET_RADIO / …
	ProvID  string // canonical sourceproviderid (e.g. "25")
}

// canonicalIDByKeyType returns the canonical built-in source ID for one
// of the four well-known radio provider key types, or ("", "") for any
// other type. Mirrors datastore.getDefaultSources() and
// canonicalDefaultsByType in pkg/service/marge.
func canonicalIDByKeyType(keyType string) (id, providerID string) {
	switch keyType {
	case "INTERNET_RADIO":
		return "10002", "2"
	case "LOCAL_INTERNET_RADIO":
		return "10003", "11"
	case "TUNEIN":
		return "10004", "25"
	case "RADIO_BROWSER":
		return "10005", "39"
	}

	return "", ""
}

// findReclassifiableSources walks a ConsistencyView's sources and
// returns the entries that:
//   - have a SourceKeyType matching one of the four built-in radio
//     providers (the ones with a canonical ID),
//   - currently sit on a non-canonical ID, and
//   - would not collide with another source already at that canonical
//     ID.
//
// The collision check is intentionally strict — when two entries claim
// the same SourceKeyType, leaving them in place is safer than guessing
// which one is the "real" one and leaving the other broken.
func findReclassifiableSources(v ConsistencyView) []reclassifiableSource {
	usedIDs := map[string]bool{}

	for _, s := range v.Sources {
		if s.ID != "" {
			usedIDs[s.ID] = true
		}
	}

	var out []reclassifiableSource

	for _, s := range v.Sources {
		newID, providerID := canonicalIDByKeyType(s.Type)
		if newID == "" || s.ID == newID {
			continue
		}

		// Don't try to re-classify if the canonical ID is already
		// occupied by a different source — would create a collision
		// the rest of the codebase isn't prepared to handle.
		if usedIDs[newID] {
			continue
		}

		out = append(out, reclassifiableSource{
			OldID:   s.ID,
			NewID:   newID,
			KeyType: s.Type,
			ProvID:  providerID,
		})
	}

	return out
}

func reclassifyDetailMessage(in []reclassifiableSource) string {
	out := "Each preset binding by ID may end up bound to the wrong source after re-pair churn — exactly the GH-343 footprint. Re-classifying restores canonical IDs:\n"

	for _, r := range in {
		out += "  • " + r.KeyType + ": " + r.OldID + " → " + r.NewID + " (sourceproviderid " + r.ProvID + ")\n"
	}

	return out
}

func reclassifyConfirmDetail(in []reclassifiableSource) string {
	out := "Changes:"

	for _, r := range in {
		out += " " + r.KeyType + " " + r.OldID + "→" + r.NewID + ";"
	}

	return out
}

// reclassifyCanonicalSourceIDs is the QuickFix body for
// FixIDReclassifyCanonicalSourceIDs. Re-reads Sources.xml / Presets.xml
// / Recents.xml for the device, builds the old-ID → new-ID mapping for
// each eligible built-in radio source, rewrites the source IDs in
// Sources.xml plus any matching <sourceid> references in
// Presets.xml/Recents.xml, and persists all three. The datastore's
// SaveX helpers each use atomic-rename internally; a failure mid-way
// leaves earlier files updated but the operation as a whole is
// idempotent — re-running it produces the same result.
func reclassifyCanonicalSourceIDs(ds *datastore.DataStore, target Target) (string, error) {
	if target.Account == "" || target.Device == "" {
		return "", fmt.Errorf("account and device are both required")
	}

	view, err := loadServiceView(ds, target.Account, target.Device)
	if err != nil {
		return "", fmt.Errorf("load service state: %w", err)
	}

	plans := findReclassifiableSources(view)
	if len(plans) == 0 {
		return "Nothing to do — all built-in radio sources already on canonical IDs.", nil
	}

	rename := map[string]string{}
	canonicalProviderID := map[string]string{}

	for _, p := range plans {
		rename[p.OldID] = p.NewID
		canonicalProviderID[p.OldID] = p.ProvID
	}

	sources, err := ds.GetConfiguredSources(target.Account, target.Device)
	if err != nil {
		return "", fmt.Errorf("read Sources.xml: %w", err)
	}

	for i := range sources {
		if newID, ok := rename[sources[i].ID]; ok {
			log.Printf("[Health] Re-classify %s: id %s → %s (account=%s device=%s)",
				sources[i].SourceKeyType, sources[i].ID, newID, target.Account, target.Device)

			sources[i].ID = newID

			if provID := canonicalProviderID[sources[i].ID]; provID != "" {
				sources[i].SourceProviderID = provID
			}
		}
	}

	if saveErr := ds.SaveConfiguredSources(target.Account, target.Device, sources); saveErr != nil {
		return "", fmt.Errorf("save Sources.xml: %w", saveErr)
	}

	if err := rewritePresetSourceIDs(ds, target, rename); err != nil {
		return "", err
	}

	if err := rewriteRecentSourceIDs(ds, target, rename); err != nil {
		return "", err
	}

	return fmt.Sprintf("Re-classified %d source ID(s) for device %s (account %s). Presets/Recents references updated accordingly. The speaker will pick up the new IDs on its next /full fetch.",
		len(plans), target.Device, target.Account), nil
}

// rewritePresetSourceIDs walks Presets.xml and updates any <sourceid>
// that appears as a key in rename to the mapped value. Persists only
// when at least one preset changed. Silent on read errors (missing
// Presets.xml is a valid state — the operator just doesn't have any
// presets to update).
func rewritePresetSourceIDs(ds *datastore.DataStore, target Target, rename map[string]string) error {
	presets, err := ds.GetPresets(target.Account, target.Device)
	if err != nil {
		return fmt.Errorf("read Presets.xml: %w", err)
	}

	dirty := false

	for i := range presets {
		if newID, ok := rename[presets[i].SourceID]; ok {
			presets[i].SourceID = newID
			dirty = true
		}
	}

	if !dirty {
		return nil
	}

	if err := ds.SavePresets(target.Account, target.Device, presets); err != nil {
		return fmt.Errorf("save Presets.xml: %w", err)
	}

	return nil
}

// rewriteRecentSourceIDs is the recents-side twin of rewritePresetSourceIDs.
func rewriteRecentSourceIDs(ds *datastore.DataStore, target Target, rename map[string]string) error {
	recents, err := ds.GetRecents(target.Account, target.Device)
	if err != nil {
		return fmt.Errorf("read Recents.xml: %w", err)
	}

	dirty := false

	for i := range recents {
		if newID, ok := rename[recents[i].SourceID]; ok {
			recents[i].SourceID = newID
			dirty = true
		}
	}

	if !dirty {
		return nil
	}

	if err := ds.SaveRecents(target.Account, target.Device, recents); err != nil {
		return fmt.Errorf("save Recents.xml: %w", err)
	}

	return nil
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

	// GH-343-style detection: built-in radio sources sitting on
	// non-canonical IDs (the 2000001+i fallback that GetConfiguredSources
	// hands out when on-disk sources don't carry canonical IDs). Surface
	// as a finding with an offline QuickFix that rewrites both the
	// source IDs and the preset/recent <sourceid> references atomically.
	if reclassifiable := findReclassifiableSources(serviceView); len(reclassifiable) > 0 {
		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Sources.xml has " + plural(len(reclassifiable), "built-in radio source", "built-in radio sources") + " on non-canonical IDs (GH-343 trigger).",
			Details:  reclassifyDetailMessage(reclassifiable),
			QuickFixes: []QuickFix{{
				ID:      FixIDReclassifyCanonicalSourceIDs,
				Label:   "Re-assign canonical IDs",
				Confirm: "Rewrite " + plural(len(reclassifiable), "source ID", "source IDs") + " in Sources.xml and matching <sourceid> references in Presets.xml/Recents.xml? " + reclassifyConfirmDetail(reclassifiable) + " Offline operation — no speaker contact needed; the speaker will re-fetch /full on its own.",
			}},
		})
	}

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

	// No special resolution here anymore — datastore.GetPresets /
	// GetRecents self-heal the protocol-level "Audio" leak via
	// repairLeakedSource at load time. The persisted Source we read
	// already reflects the speaker's perspective.
	for i := range presets {
		view.Presets = append(view.Presets, ConsistencyPreset{
			Slot:     presets[i].ButtonNumber,
			Source:   presets[i].Source,
			SourceID: presets[i].SourceID,
			Location: presets[i].Location,
			Name:     presets[i].Name,
		})
	}

	for i := range recents {
		view.Recents = append(view.Recents, ConsistencyRecent{
			ID:       recents[i].ID,
			Source:   recents[i].Source,
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
