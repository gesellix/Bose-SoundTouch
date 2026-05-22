package health

import (
	"fmt"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDDefaultAccountNonBoseDevices is the registry id of the
// non-Bose-default-account-devices check.
const CheckIDDefaultAccountNonBoseDevices = "default_account_non_bose_devices"

// FixIDEvictDefaultNonBoseDevice removes a non-Bose UPnP device from
// the "default" account directory. Implemented by the existing
// DataStore.RemoveDevice — this constant ties it to the finding it
// remediates.
const FixIDEvictDefaultNonBoseDevice = "evict_default_non_bose_device"

// RegisterDefaultAccountNonBoseDevicesCheck registers the
// non-Bose-default-account-devices health check. Walks the entries
// under data/accounts/default/devices/ and flags any whose
// DeviceInfo.xml model/type doesn't look like a Bose SoundTouch
// product. These are leftover discovery hits from the LAN's broader
// UPnP MediaRenderer population — LG TVs, Onkyo / Yamaha receivers,
// Dreambox tuners — that responded to our generic
// `urn:schemas-upnp-org:device:MediaRenderer:1` M-SEARCH.
//
// Each flagged entry comes with an "Evict" QuickFix that removes the
// device's data directory; the live discovery filter (see
// `pkg/discovery/upnp.go isBoseUPnPDevice`) prevents the entry from
// being re-created on the next scan.
//
// Bose devices that still live under "default" (e.g. a fresh speaker
// before pairing completes) are intentionally ignored here — that's
// the consistency check's domain.
func RegisterDefaultAccountNonBoseDevicesCheck(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDDefaultAccountNonBoseDevices,
		Title: "Default-account devices are SoundTouch speakers",
		Run: func() []Finding {
			return runDefaultAccountNonBoseDevicesCheck(ds)
		},
	})

	r.RegisterFix(
		CheckIDDefaultAccountNonBoseDevices,
		FixIDEvictDefaultNonBoseDevice,
		func(target Target) (string, error) {
			if target.Device == "" {
				return "", fmt.Errorf("device is required")
			}

			if err := ds.RemoveDevice("default", target.Device); err != nil {
				return "", fmt.Errorf("remove default/%s: %w", target.Device, err)
			}

			return fmt.Sprintf("Evicted %s from the default account. If it returns on the next scan, AfterTouch's discovery filter needs an update — please file a bug.", target.Device), nil
		},
	)
}

func runDefaultAccountNonBoseDevicesCheck(ds *datastore.DataStore) []Finding {
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
		if dev.AccountID != "default" {
			continue
		}

		if looksLikeSoundTouch(dev) {
			continue
		}

		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Target:   Target{Account: "default", Device: dev.DeviceID},
			Message: fmt.Sprintf(
				"Non-Bose device %q (type=%q) is stored under the default account.",
				labelForDevice(dev), dev.ProductCode,
			),
			Details: "Likely a UPnP MediaRenderer (TV / AV receiver / set-top box) that answered AfterTouch's generic discovery probe. " +
				"Evict it via the QuickFix; the discovery filter introduced alongside this check (#269/#359) prevents it from being re-created.",
			QuickFixes: []QuickFix{{
				ID:      FixIDEvictDefaultNonBoseDevice,
				Label:   "Evict from default account",
				Confirm: fmt.Sprintf("This will delete data/accounts/default/devices/%s/ and all its contents. The device entry was created by AfterTouch's discovery; no real speaker state is affected.", dev.DeviceID),
			}},
		})
	}

	return findings
}

// looksLikeSoundTouch returns true when the device's ProductCode /
// Name suggests it's a Bose SoundTouch product. The signal we have on
// disk is the `<type>` element from /info, which Bose devices populate
// with strings like "SoundTouch 10 sm2" or just "SoundTouch"; non-Bose
// devices populate it with their own model name ("HT-R695", "dm920",
// "OLED55G2", …). Case-insensitive substring match — the on-disk file
// preserves whatever the device emitted, so we don't normalise.
func looksLikeSoundTouch(dev *models.ServiceDeviceInfo) bool {
	if dev == nil {
		return false
	}

	hay := strings.ToLower(dev.ProductCode + " " + dev.Name)

	return strings.Contains(hay, "soundtouch") || strings.Contains(hay, "wave music system")
}

func labelForDevice(dev *models.ServiceDeviceInfo) string {
	if dev.Name != "" {
		return dev.Name
	}

	if dev.DeviceID != "" {
		return dev.DeviceID
	}

	return "(unnamed)"
}
