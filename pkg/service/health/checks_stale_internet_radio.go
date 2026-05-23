package health

import (
	"fmt"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDStaleInternetRadio is the registry id of the stale INTERNET_RADIO
// stub source check.
const CheckIDStaleInternetRadio = "stale_internet_radio"

// FixIDRemoveInternetRadio is the registry id of the quick-fix that removes
// the stub INTERNET_RADIO source.
const FixIDRemoveInternetRadio = "remove_internet_radio"

// RegisterStaleInternetRadioCheck registers a check that detects the legacy
// INTERNET_RADIO stub source (empty credentials) left over from devices
// initialised before AfterTouch removed it from the default source list.
func RegisterStaleInternetRadioCheck(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDStaleInternetRadio,
		Title: "Stale INTERNET_RADIO stub source",
		Run: func() []Finding {
			return runStaleInternetRadioCheck(ds)
		},
	})

	r.RegisterFix(CheckIDStaleInternetRadio, FixIDRemoveInternetRadio, func(target Target) (string, error) {
		return fixRemoveInternetRadio(ds, target)
	})
}

func runStaleInternetRadioCheck(ds *datastore.DataStore) []Finding {
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

		sources, err := ds.GetConfiguredSources(dev.AccountID, dev.DeviceID)
		if err != nil {
			continue
		}

		for j := range sources {
			s := &sources[j]

			if s.SourceKeyType != constants.ProviderInternetRadio {
				continue
			}

			if s.Secret != "" || s.Credential.Value != "" {
				// Real user-configured INTERNET_RADIO source — don't touch it.
				continue
			}

			findings = append(findings, Finding{
				Severity: SeverityInfo,
				Target:   Target{Account: dev.AccountID, Device: dev.DeviceID},
				Message: fmt.Sprintf(
					"Device %s has a stub %s source (ID %s) with no credentials.",
					displayName(dev.Name, dev.DeviceID),
					constants.ProviderInternetRadio,
					s.ID,
				),
				Details: "AfterTouch no longer adds INTERNET_RADIO to new devices. " +
					"This entry is a leftover from an earlier version and can be safely removed. " +
					"The speaker will re-sync its source list on the next reconnect.",
				QuickFixes: []QuickFix{{
					ID:    FixIDRemoveInternetRadio,
					Label: fmt.Sprintf("Remove %s source (ID %s)", constants.ProviderInternetRadio, s.ID),
				}},
			})
		}
	}

	return findings
}

func fixRemoveInternetRadio(ds *datastore.DataStore, target Target) (string, error) {
	if target.Device == "" {
		return "", fmt.Errorf("device is required")
	}

	sources, err := ds.GetConfiguredSources(target.Account, target.Device)
	if err != nil {
		return "", fmt.Errorf("could not read sources: %w", err)
	}

	irID := ""

	for i := range sources {
		if sources[i].SourceKeyType == constants.ProviderInternetRadio &&
			sources[i].Secret == "" && sources[i].Credential.Value == "" {
			irID = sources[i].ID

			break
		}
	}

	if irID == "" {
		return "No stub INTERNET_RADIO source found — nothing to remove.", nil
	}

	if err := ds.DeleteSourceByID(target.Account, target.Device, irID); err != nil {
		return "", fmt.Errorf("delete source: %w", err)
	}

	return fmt.Sprintf("Removed stub %s source (ID %s) from device %s. The speaker will receive the updated source list on its next reconnect.", constants.ProviderInternetRadio, irID, target.Device), nil
}
