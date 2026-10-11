package setup

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/marge"
)

// MigrationDataNotReadyError means migration was refused because the service
// cannot prove that its rendered account data preserves the speaker's presets.
type MigrationDataNotReadyError struct {
	Reason string
	Action string
}

func (e *MigrationDataNotReadyError) Error() string {
	action := e.Action
	if action == "" {
		action = "Run Data Sync for this device and retry migration."
	}

	return fmt.Sprintf("Migration data is not ready: %s. %s", e.Reason, action)
}

func migrationDataNotReadyf(format string, args ...any) error {
	return &MigrationDataNotReadyError{Reason: fmt.Sprintf(format, args...)}
}

// checkMigrationDataReady proves that redirecting the speaker to this service
// will not replace its live presets with missing, stale, or filtered account
// data. Every operation in this check is read-only.
//
// It returns warnings for conditions worth telling the user about but not
// worth refusing over, and an error only for the ones it can actually prove.
func (m *Manager) checkMigrationDataReady(deviceIP string) ([]string, error) {
	if m.DataStore == nil {
		// CLI callers do not own the service datastore and cannot enforce this
		// check. ExecuteInitPlan is a separate onboarding flow which establishes
		// account state only after its intentional URL rewrite.
		return nil, nil
	}

	info, err := m.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		return nil, migrationDataNotReadyf("cannot read live /info: %v", err)
	}

	deviceID := strings.TrimSpace(info.DeviceID)
	accountID := strings.TrimSpace(info.MargeAccountUUID)

	if deviceID == "" {
		return nil, migrationDataNotReadyf("live /info has no deviceID")
	}

	if accountID == "" {
		// An unpaired speaker, typically factory-reset, has no account data to
		// preserve, so there is nothing for this check to compare and nothing
		// to lose. Refusing here would also break the documented onboarding
		// order: the admin UI migrates first and pairs afterwards (see the
		// "Pairing runs after the URL flip" comment in the setup page), and
		// MIGRATION-GUIDE.md tells the user to Generate an account ID on a
		// factory-reset device. Data Sync cannot unblock it either, since it
		// files an account-less device under "default", which never matches an
		// empty live account.
		return nil, nil
	}

	if !datastore.IsSafeIdentifier(accountID) || !datastore.IsSafeIdentifier(deviceID) {
		return nil, migrationDataNotReadyf("live /info contains an invalid account or device identifier")
	}

	persistedInfo, err := m.DataStore.GetExactDeviceInfo(accountID, deviceID)
	if err != nil {
		// Report the cause: a malformed file or an I/O error is not a missing
		// sync, and Data Sync is then the wrong remedy to suggest.
		return nil, migrationDataNotReadyf("cannot read DeviceInfo.xml under account %q and device %q: %v", accountID, deviceID, err)
	}

	if persistedInfo.DeviceID != deviceID {
		return nil, migrationDataNotReadyf("persisted DeviceInfo.xml identifies device %q instead of %q", persistedInfo.DeviceID, deviceID)
	}

	snapshot, err := m.DataStore.ReadPresetSnapshot(accountID, deviceID)
	if err != nil {
		return nil, migrationDataNotReadyf("cannot read the persisted preset snapshot: %v", err)
	}

	if snapshot.State != datastore.PresetSnapshotValid {
		return nil, migrationDataNotReadyf("persisted Presets.xml is %s", snapshot.State)
	}

	if snapshot.NeedsRewrite {
		return nil, migrationDataNotReadyf("persisted Presets.xml uses a legacy format that must be refreshed")
	}

	livePresets, err := m.fetchLivePresets(deviceIP)
	if err != nil {
		return nil, migrationDataNotReadyf("cannot read live /presets: %v", err)
	}

	fullXML, err := marge.AccountFullToXMLReadOnly(m.DataStore, accountID)
	if err != nil {
		return nil, migrationDataNotReadyf("cannot render account /full: %v", err)
	}

	fullPresets, accountDeviceCount, err := migrationFullPresets(fullXML, deviceID)
	if err != nil {
		return nil, migrationDataNotReadyf("rendered account /full is incomplete: %v", err)
	}

	var warnings []string

	// Not a refusal. One account holding every speaker in the household is the
	// normal Bose topology, so blocking it would block most setups, and issue
	// #614 concluded the shared-account preset wipe is empirical rather than a
	// proven mechanism with an open root cause. A stale duplicate entry left
	// by a failed /info read or a DHCP lease change would also trip it on a
	// genuinely single-speaker setup. Say what was found and let the user
	// decide.
	if accountDeviceCount != 1 {
		warnings = append(warnings, fmt.Sprintf(
			"migrating into an account that contains %d devices; some firmware has been reported to wipe presets after a reboot-triggered resync of a shared account (issue #614, root cause open)",
			accountDeviceCount))
	}

	persisted := migrationPresetIdentities(snapshot.Presets)
	live := migrationPresetIdentities(livePresets)

	if mismatch := compareMigrationPresets("persisted snapshot", persisted, "rendered /full", fullPresets); mismatch != nil {
		return nil, mismatch.err()
	}

	// A speaker can report two presets on one button. Data Sync keeps one per
	// button, so the live list stays one longer than /full and re-syncing can
	// never close the gap. Only the speaker can resolve it (discussion 708).
	if slot := firstDuplicateMigrationSlot(live); slot != "" {
		return nil, &MigrationDataNotReadyError{
			Reason: fmt.Sprintf("the speaker reports more than one preset on button %s", slot),
			Action: fmt.Sprintf("Store or clear preset %s on the speaker, run Data Sync, then retry migration.", slot),
		}
	}

	if mismatch := compareMigrationPresets("live /presets", live, "rendered /full", fullPresets); mismatch != nil {
		return nil, mismatch.err()
	}

	return warnings, nil
}

type migrationPresetIdentity struct {
	Slot     string
	Name     string
	Location string
}

func migrationPresetIdentities(presets []models.ServicePreset) []migrationPresetIdentity {
	result := make([]migrationPresetIdentity, 0, len(presets))
	for i := range presets {
		slot := presets[i].ButtonNumber
		if slot == "" {
			slot = presets[i].ID
		}

		// Clearing a slot through the Marge API leaves a zero-value entry in
		// the list (RemovePreset assigns models.ServicePreset{}), persisted as
		// <preset id="">. The rendered /full drops it, so counting it here
		// would report a mismatch for a datastore that is perfectly in sync.
		// An empty slot carries no identity to compare either way.
		if slot == "" {
			continue
		}

		result = append(result, migrationPresetIdentity{
			Slot:     slot,
			Name:     presets[i].Name,
			Location: presets[i].Location,
		})
	}

	return result
}

// firstDuplicateMigrationSlot returns the first slot that occurs more than
// once, or "" when every slot is unique.
func firstDuplicateMigrationSlot(presets []migrationPresetIdentity) string {
	seen := make(map[string]bool, len(presets))
	for _, preset := range presets {
		if seen[preset.Slot] {
			return preset.Slot
		}

		seen[preset.Slot] = true
	}

	return ""
}

func migrationFullPresets(fullXML []byte, deviceID string) ([]migrationPresetIdentity, int, error) {
	var full struct {
		Devices []struct {
			DeviceID string `xml:"deviceid,attr"`
			Presets  []struct {
				Slot     string `xml:"buttonNumber,attr"`
				Name     string `xml:"name"`
				Location string `xml:"location"`
			} `xml:"presets>preset"`
		} `xml:"devices>device"`
	}

	if err := xml.Unmarshal(fullXML, &full); err != nil {
		return nil, 0, fmt.Errorf("malformed XML: %w", err)
	}

	for i := range full.Devices {
		if full.Devices[i].DeviceID != deviceID {
			continue
		}

		presets := make([]migrationPresetIdentity, 0, len(full.Devices[i].Presets))
		for _, preset := range full.Devices[i].Presets {
			presets = append(presets, migrationPresetIdentity{
				Slot:     preset.Slot,
				Name:     preset.Name,
				Location: preset.Location,
			})
		}

		return presets, len(full.Devices), nil
	}

	return nil, len(full.Devices), fmt.Errorf("target device %q is missing", deviceID)
}

// migrationPresetMismatch describes why two preset views disagree.
type migrationPresetMismatch struct {
	Reason string
	// DroppedByFull marks the case where the speaker's own view holds a slot
	// the rendered /full does not. mapPresetsToFullResponse omits a preset
	// whose source is absent from the account's configured sources and cannot
	// be synthesised, so this is not a stale snapshot and re-syncing cannot
	// fix it: the source itself has to come back.
	DroppedByFull bool
}

func (m *migrationPresetMismatch) err() error {
	if !m.DroppedByFull {
		return migrationDataNotReadyf("%s", m.Reason)
	}

	return &MigrationDataNotReadyError{
		Reason: m.Reason,
		Action: "The rendered account omits a preset whose music service source is missing, so Data Sync cannot restore it. " +
			"Re-link or repopulate that source for this account, then retry migration.",
	}
}

func compareMigrationPresets(leftName string, left []migrationPresetIdentity, rightName string, right []migrationPresetIdentity) *migrationPresetMismatch {
	if len(left) != len(right) {
		return &migrationPresetMismatch{
			Reason:        fmt.Sprintf("%s has %d preset(s), but %s has %d", leftName, len(left), rightName, len(right)),
			DroppedByFull: len(left) > len(right),
		}
	}

	leftBySlot, problem := indexMigrationPresets(leftName, left)
	if problem != "" {
		return &migrationPresetMismatch{Reason: problem}
	}

	rightBySlot, problem := indexMigrationPresets(rightName, right)
	if problem != "" {
		return &migrationPresetMismatch{Reason: problem}
	}

	for slot, leftPreset := range leftBySlot {
		rightPreset, ok := rightBySlot[slot]
		if !ok {
			return &migrationPresetMismatch{
				Reason:        fmt.Sprintf("preset slot %s from %s is missing from %s", slot, leftName, rightName),
				DroppedByFull: true,
			}
		}

		if leftPreset.Name != rightPreset.Name || leftPreset.Location != rightPreset.Location {
			return &migrationPresetMismatch{
				Reason: fmt.Sprintf("preset slot %s differs between %s and %s", slot, leftName, rightName),
			}
		}
	}

	return nil
}

func indexMigrationPresets(name string, presets []migrationPresetIdentity) (map[string]migrationPresetIdentity, string) {
	bySlot := make(map[string]migrationPresetIdentity, len(presets))
	for _, preset := range presets {
		if preset.Slot == "" {
			return nil, fmt.Sprintf("%s contains a preset without a slot", name)
		}

		if _, exists := bySlot[preset.Slot]; exists {
			return nil, fmt.Sprintf("%s contains duplicate preset slot %s", name, preset.Slot)
		}

		bySlot[preset.Slot] = preset
	}

	return bySlot, ""
}
