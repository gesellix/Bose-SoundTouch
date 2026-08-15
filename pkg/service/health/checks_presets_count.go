package health

import (
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDPresetsCount is the registry id of the speaker-vs-service
// preset count check.
const CheckIDPresetsCount = "speaker_presets_count"

// FixIDRestorePresetsToSpeaker is the quick-fix that replays the
// service's stored presets onto the speaker via its :8090/storePreset
// endpoint, without requiring a reboot or re-entering them by hand.
const FixIDRestorePresetsToSpeaker = "restore_presets_to_speaker"

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

	r.RegisterFix(CheckIDPresetsCount, FixIDRestorePresetsToSpeaker, func(target Target) (string, error) {
		return restorePresetsToSpeaker(ds, target)
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

	var quickFixes []QuickFix

	if speakerCount == 0 && serviceCount > 0 {
		// Speaker shows nothing while the service has presets —
		// the post-reset preset-loss pattern confirmed in #614
		// (reboot and/or Sync leaving the speaker's own preset
		// slots empty while the service's Presets.xml is untouched).
		severity = SeverityWarning
		quickFixes = []QuickFix{{
			ID:      FixIDRestorePresetsToSpeaker,
			Label:   "Restore presets to speaker",
			Confirm: "This pushes AfterTouch's stored presets onto the speaker's own preset slots, one at a time. Doesn't require a reboot.",
		}}
	}

	return []Finding{{
		Severity: severity,
		Target:   target,
		Message: fmt.Sprintf(
			"Speaker shows %d preset slot(s); service Presets.xml has %d.",
			speakerCount, serviceCount,
		),
		Details:    "If the speaker shows fewer than the service, a sourcesUpdated notification sometimes re-syncs it. Don't power-cycle as a fix for this — it has itself been reported to wipe the speaker's presets (#614), so it may make things worse. If the speaker shows more than the service, the service may have stale entries or the speaker is still holding pre-migration state.",
		QuickFixes: quickFixes,
	}}
}

// restorePresetsToSpeaker replays every preset in the service's
// Presets.xml onto the live speaker via :8090/storePreset, one slot
// at a time. Unlike Sync (which only ever reads from the speaker),
// this is the one direction that can put presets back after they've
// been wiped, without needing to re-enter them by hand — see #614.
func restorePresetsToSpeaker(ds *datastore.DataStore, target Target) (string, error) {
	if target.Account == "" || target.Device == "" {
		return "", fmt.Errorf("account and device are required")
	}

	dev, err := ds.GetDeviceInfo(target.Account, target.Device)
	if err != nil || dev == nil {
		return "", fmt.Errorf("device %s not found in datastore", target.Device)
	}

	if dev.IPAddress == "" {
		return "", fmt.Errorf("device %s has no IP address recorded", target.Device)
	}

	presets, err := ds.GetPresets(target.Account, target.Device)
	if err != nil {
		return "", fmt.Errorf("read service Presets.xml: %w", err)
	}

	if len(presets) == 0 {
		return "", fmt.Errorf("service has no presets recorded for %s", target.Device)
	}

	c := client.NewClientFromHost(dev.IPAddress)

	restored := 0

	var failures []string

	for i := range presets {
		p := &presets[i]

		slot, atoiErr := strconv.Atoi(p.ID)
		if atoiErr != nil || slot < 1 || slot > 6 {
			failures = append(failures, fmt.Sprintf("slot %q: invalid preset id", p.ID))
			continue
		}

		isPresetable, _ := strconv.ParseBool(p.IsPresetable)

		ci := &models.ContentItem{
			Source:        p.Source,
			Type:          p.Type,
			Location:      p.Location,
			SourceAccount: p.SourceAccount,
			IsPresetable:  isPresetable,
			ItemName:      p.Name,
			ContainerArt:  p.ContainerArt,
		}

		if storeErr := c.StorePreset(slot, ci); storeErr != nil {
			failures = append(failures, fmt.Sprintf("slot %d: %v", slot, storeErr))
			continue
		}

		restored++
	}

	if restored == 0 {
		return "", fmt.Errorf("failed to restore any presets: %s", strings.Join(failures, "; "))
	}

	msg := fmt.Sprintf("Restored %d/%d preset(s) to %s.", restored, len(presets), displayName(dev.Name, target.Device))
	if len(failures) > 0 {
		msg += " Some slots failed: " + strings.Join(failures, "; ")
	}

	return msg, nil
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
