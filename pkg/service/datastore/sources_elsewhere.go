package datastore

import (
	"sort"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// SourcesElsewhere returns the sources this service knows from other speakers
// that the given device does not have, newest question first: what could this
// speaker be given that its neighbours already have? (issue 754)
//
// Identity only. The stored Sources.xml carries the credential material a
// speaker keeps -- including AfterTouch's own `bs-` surrogate, which resolves
// to a linked account for whoever presents it -- and none of it may leave the
// service. models.IdentityOfSource is the projection; nothing here passes a
// ConfiguredSource outward.
//
// Device-local sources (inputs, AirPlay, the media renderer) are left out
// entirely: every speaker has its own, so another speaker's copy says nothing.
func (ds *DataStore) SourcesElsewhere(account, device string) ([]models.SourceIdentity, error) {
	mine, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		return nil, err
	}

	held := make(map[string]bool, len(mine))
	for i := range mine {
		held[models.IdentityOfSource(mine[i]).Key()] = true
	}

	accounts, err := ds.ListAccounts()
	if err != nil {
		return nil, err
	}

	byKey := map[string]*models.SourceIdentity{}

	for _, acc := range accounts {
		devices, dirErr := ds.ReadDirUnderBase(ds.AccountDevicesDir(acc))
		if dirErr != nil {
			continue
		}

		for _, entry := range devices {
			if !entry.IsDir() || (acc == account && entry.Name() == device) {
				continue
			}

			sources, srcErr := ds.GetConfiguredSources(acc, entry.Name())
			if srcErr != nil {
				continue
			}

			for i := range sources {
				collectSourceIdentity(byKey, held, models.IdentityOfSource(sources[i]), entry.Name())
			}
		}
	}

	out := make([]models.SourceIdentity, 0, len(byKey))
	for _, identity := range byKey {
		out = append(out, *identity)
	}

	// Stable order: what we could add first, then by name, so the list does
	// not reshuffle between reads.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Availability != out[j].Availability {
			return out[i].Availability == models.SourceAvailableToAdd
		}

		return strings.ToLower(out[i].Label()) < strings.ToLower(out[j].Label())
	})

	return out, nil
}

func collectSourceIdentity(byKey map[string]*models.SourceIdentity, held map[string]bool, identity models.SourceIdentity, device string) {
	if identity.Type == "" || identity.Availability == models.SourceAvailableLocalOnly {
		return
	}

	key := identity.Key()
	if held[key] {
		return
	}

	existing, seen := byKey[key]
	if !seen {
		identity.Devices = []string{device}
		byKey[key] = &identity

		return
	}

	// A display name only one speaker bothered to record is still the best
	// name we have for the source.
	if existing.DisplayName == "" {
		existing.DisplayName = identity.DisplayName
	}

	for _, known := range existing.Devices {
		if known == device {
			return
		}
	}

	existing.Devices = append(existing.Devices, device)
}
