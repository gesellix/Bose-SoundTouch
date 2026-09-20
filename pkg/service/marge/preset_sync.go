package marge

import (
	"log"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// Preset sync modes, stored per account as ServiceAccountInfo.PresetSync.
//
// Real Bose kept one preset bank per account, so speakers of a household
// shared it (issue 495). AfterTouch stores presets per device, which is safer
// for setups whose speakers deliberately differ, so sharing is a policy rather
// than a given.
const (
	// PresetSyncAuto shares a write when nothing diverging would be
	// overwritten: a single speaker, speakers that already agree, or a
	// speaker with no presets at all. It is the default.
	PresetSyncAuto = "auto"
	// PresetSyncOn always shares a write with the account's other speakers.
	PresetSyncOn = "on"
	// PresetSyncOff never overwrites another speaker's preset. A speaker
	// with no presets still adopts the account's, because nothing is lost
	// that way (maintainer's decision, 2026-09-20).
	PresetSyncOff = "off"
)

// PresetSyncMode reads the account's mode, defaulting to PresetSyncAuto.
func PresetSyncMode(ds *datastore.DataStore, account string) string {
	info, err := ds.GetAccountInfo(account)
	if err != nil || info == nil {
		return PresetSyncAuto
	}

	switch strings.ToLower(strings.TrimSpace(info.PresetSync)) {
	case PresetSyncOn:
		return PresetSyncOn
	case PresetSyncOff:
		return PresetSyncOff
	default:
		return PresetSyncAuto
	}
}

// presetIdentity is what makes two preset banks "the same" for sync
// purposes: the slot plus what it plays. Names and artwork are presentation
// and must not make two banks look diverging.
type presetIdentity struct {
	source        string
	sourceAccount string
	location      string
	itemType      string
}

// presetButton is the slot a preset occupies: its button number, or its id
// when only that is set.
func presetButton(p models.ServicePreset) string {
	if button := strings.TrimSpace(p.ButtonNumber); button != "" {
		return button
	}

	return strings.TrimSpace(p.ID)
}

func identityOf(p models.ServicePreset) presetIdentity {
	return presetIdentity{
		source:        strings.TrimSpace(p.Source),
		sourceAccount: strings.TrimSpace(p.SourceAccount),
		location:      strings.TrimSpace(p.Location),
		itemType:      strings.TrimSpace(p.ContentItemType),
	}
}

func presetIsEmpty(p models.ServicePreset) bool {
	id := identityOf(p)

	return id.source == "" && id.location == ""
}

// bankIsEmpty reports whether a device holds no usable preset at all, which
// is the case for a speaker that was just added to the account.
func bankIsEmpty(presets []models.ServicePreset) bool {
	for i := range presets {
		if !presetIsEmpty(presets[i]) {
			return false
		}
	}

	return true
}

// banksAgree compares two preset banks by slot and content, ignoring names
// and artwork.
func banksAgree(left, right []models.ServicePreset) bool {
	byButton := func(presets []models.ServicePreset) map[string]presetIdentity {
		out := make(map[string]presetIdentity, len(presets))

		for i := range presets {
			if presetIsEmpty(presets[i]) {
				continue
			}

			button := presetButton(presets[i])
			if button == "" {
				continue
			}

			out[button] = identityOf(presets[i])
		}

		return out
	}

	leftByButton, rightByButton := byButton(left), byButton(right)
	if len(leftByButton) != len(rightByButton) {
		return false
	}

	for button, identity := range leftByButton {
		if rightByButton[button] != identity {
			return false
		}
	}

	return true
}

// PresetSyncTargets lists the devices of an account that should receive a
// preset write made on sourceDevice, following the account's mode.
//
// A device with no presets always adopts (nothing is overwritten). A device
// that holds presets is written only when the mode allows overwriting: always
// with PresetSyncOn, and with PresetSyncAuto as long as the account's
// speakers did not already disagree.
//
// before is the source device's bank as it was *before* the write that is
// being propagated. Judging divergence on the new bank would misread every
// ordinary write as a disagreement, since the source by then already differs
// in the written slot.
func PresetSyncTargets(ds *datastore.DataStore, account, sourceDevice string,
	before []models.ServicePreset) []models.ServiceDeviceInfo {
	if ds == nil || account == "" || sourceDevice == "" {
		return nil
	}

	all, err := ds.ListAllDevices()
	if err != nil {
		return nil
	}

	candidates := make([]models.ServiceDeviceInfo, 0, len(all))

	for i := range all {
		if all[i].AccountID != account || all[i].DeviceID == sourceDevice {
			continue
		}

		candidates = append(candidates, all[i])
	}

	if len(candidates) == 0 {
		return nil
	}

	mode := PresetSyncMode(ds, account)

	// In auto, a single diverging speaker turns sharing off for the whole
	// account: the owner has to say which bank wins.
	overwrite := mode == PresetSyncOn
	if mode == PresetSyncAuto {
		overwrite = true

		for i := range candidates {
			presets, presetErr := ds.GetPresetsReadOnly(account, candidates[i].DeviceID)
			if presetErr != nil {
				continue
			}

			if !bankIsEmpty(presets) && !banksAgree(before, presets) {
				overwrite = false

				break
			}
		}
	}

	targets := make([]models.ServiceDeviceInfo, 0, len(candidates))

	for i := range candidates {
		presets, presetErr := ds.GetPresetsReadOnly(account, candidates[i].DeviceID)
		if presetErr != nil {
			continue
		}

		if overwrite || bankIsEmpty(presets) {
			targets = append(targets, candidates[i])
		}
	}

	return targets
}

// PropagatePresetWrite copies the slot just written on sourceDevice to the
// account's other devices, following PresetSyncTargets. It returns the
// devices actually written, so the caller can nudge exactly those.
//
// Only single-slot writes travel this way. A whole-list harvest (setup sync)
// stays per device: that list can be shorter or stale, which is what the
// destructive-sync guard from issue 697 protects against.
func PropagatePresetWrite(ds *datastore.DataStore, account, sourceDevice string, presetNumber int,
	before []models.ServicePreset) []models.ServiceDeviceInfo {
	button := strconv.Itoa(presetNumber)

	targets := PresetSyncTargets(ds, account, sourceDevice, before)
	if len(targets) == 0 {
		return nil
	}

	sourcePresets, err := ds.GetPresetsReadOnly(account, sourceDevice)
	if err != nil {
		return nil
	}

	var written models.ServicePreset

	for i := range sourcePresets {
		if presetButton(sourcePresets[i]) == button {
			written = sourcePresets[i]

			break
		}
	}

	if presetIsEmpty(written) {
		return nil
	}

	written.ButtonNumber = button
	written.ID = button

	applied := make([]models.ServiceDeviceInfo, 0, len(targets))

	for i := range targets {
		if _, mutateErr := ds.MutatePresets(account, targets[i].DeviceID,
			func(presets []models.ServicePreset) ([]models.ServicePreset, error) {
				return upsertPresetByButton(presets, written), nil
			}); mutateErr != nil {
			log.Printf("[PresetSync] %s -> %s slot %s: %v",
				sanitizeLog(sourceDevice), sanitizeLog(targets[i].DeviceID), button, mutateErr)

			continue
		}

		applied = append(applied, targets[i])
	}

	return applied
}

// PropagatePresetRemoval clears the same slot on the account's other
// devices, so a cleared preset does not come back from a speaker that still
// holds it.
func PropagatePresetRemoval(ds *datastore.DataStore, account, sourceDevice string, presetNumber int,
	before []models.ServicePreset) []models.ServiceDeviceInfo {
	button := strconv.Itoa(presetNumber)

	targets := PresetSyncTargets(ds, account, sourceDevice, before)
	if len(targets) == 0 {
		return nil
	}

	applied := make([]models.ServiceDeviceInfo, 0, len(targets))

	for i := range targets {
		// Clear by button, not by list position: RemovePreset indexes the
		// stored slice, and a sibling's bank can hold the same slot at a
		// different position.
		if _, err := ds.MutatePresets(account, targets[i].DeviceID,
			func(presets []models.ServicePreset) ([]models.ServicePreset, error) {
				for j := range presets {
					if presetButton(presets[j]) == button {
						presets[j] = models.ServicePreset{ButtonNumber: button, ID: button}
					}
				}

				return presets, nil
			}); err != nil {
			log.Printf("[PresetSync] clear %s slot %s: %v", sanitizeLog(targets[i].DeviceID), button, err)

			continue
		}

		applied = append(applied, targets[i])
	}

	return applied
}
