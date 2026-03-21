package marge

import (
	"fmt"
	"log"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// SyncFromAccountFull synchronizes the local datastore with the data from an AccountFullResponse.
func SyncFromAccountFull(ds *datastore.DataStore, resp *models.AccountFullResponse) error {
	accountID := resp.ID

	if accountID == "" {
		return fmt.Errorf("account ID is missing in response")
	}

	log.Printf("[SYNC] Starting synchronization for account %s", accountID)
	// 0. Update Account Metadata
	syncAccountInfo(ds, accountID, resp)

	for i := range resp.Devices {
		dev := &resp.Devices[i]

		deviceID := dev.DeviceID
		if deviceID == "" {
			continue
		}

		log.Printf("[SYNC] Synchronizing device %s (Account: %s)", deviceID, accountID)

		// 1. Update Device Info
		syncDeviceInfo(ds, accountID, dev)

		// 2. Update Configured Sources for this device
		syncConfiguredSources(ds, accountID, deviceID, resp.Sources, dev)

		// 3. Update Presets
		syncPresets(ds, accountID, deviceID, dev.Presets)

		// 4. Update Recents
		syncRecents(ds, accountID, deviceID, dev.Recents)
	}

	log.Printf("[SYNC] Synchronization completed for account %s", accountID)

	return nil
}

func syncAccountInfo(ds *datastore.DataStore, accountID string, resp *models.AccountFullResponse) {
	info := &models.ServiceAccountInfo{
		AccountID:         accountID,
		PreferredLanguage: resp.PreferredLanguage,
		ProviderSettings:  resp.ProviderSettings,
	}

	if err := ds.SaveAccountInfo(accountID, info); err != nil {
		log.Printf("[SYNC_ERR] Failed to save account info for %s: %v", accountID, err)
	}
}

func syncDeviceInfo(ds *datastore.DataStore, accountID string, dev *models.AccountDevice) {
	deviceID := dev.DeviceID
	existingInfo, _ := ds.GetDeviceInfo(accountID, deviceID)

	info := &models.ServiceDeviceInfo{
		DeviceID:           deviceID,
		AccountID:          accountID,
		IPAddress:          dev.IPAddress,
		DeviceSerialNumber: dev.SerialNumber,
		FirmwareVersion:    dev.FirmwareVersion,
		Name:               dev.Name,
		DiscoveryMethod:    "sync_full",
	}
	if dev.AttachedProduct != nil {
		info.ProductCode = dev.AttachedProduct.ProductCode

		info.ProductSerialNumber = dev.AttachedProduct.SerialNumber
		for _, comp := range dev.AttachedProduct.Components {
			info.Components = append(info.Components, models.ServiceComponent{
				Category:        comp.Category,
				SoftwareVersion: comp.SoftwareVersion,
				SerialNumber:    comp.SerialNumber,
			})
		}
	}

	// If the name is empty in the upstream response, try to preserve the local name
	if existingInfo != nil {
		info.MacAddress = existingInfo.MacAddress
		if info.IPAddress == "" {
			info.IPAddress = existingInfo.IPAddress
		}
	}

	if info.Name == "" {
		if existingInfo != nil && existingInfo.Name != "" {
			info.Name = existingInfo.Name
			log.Printf("[SYNC_DEBUG] Preserved local name '%s' for device %s", info.Name, deviceID)
		} else {
			log.Printf("[SYNC_DEBUG] Name is empty for device %s in upstream and no local name found", deviceID)
		}
	} else {
		log.Printf("[SYNC_DEBUG] Upstream name for device %s is '%s'", deviceID, info.Name)
	}

	// If the name is still empty, try to find a name from other devices in the same account or globally
	if info.Name == "" {
		allDevices, _ := ds.ListAllDevices()
		for i := range allDevices {
			d := &allDevices[i]
			if d.DeviceID == deviceID && d.Name != "" {
				info.Name = d.Name
				log.Printf("[SYNC_DEBUG] Recovered name '%s' for device %s from global search", info.Name, deviceID)

				break
			}
		}
	}

	if err := ds.SaveDeviceInfo(accountID, deviceID, info); err != nil {
		log.Printf("[SYNC_ERR] Failed to save device info for %s: %v", deviceID, err)
	}
}

func syncConfiguredSources(ds *datastore.DataStore, accountID, deviceID string, sources []models.FullResponseSource, dev *models.AccountDevice) {
	// We'll use the account-level sources from the response as a base.
	var deviceSources []models.ConfiguredSource

	// Track seen sources to avoid duplicates
	seen := make(map[string]bool)

	// 1. Add sources from the account-level sources list
	for i := range sources {
		s := &sources[i]
		if s.ID != "" && seen[s.ID] {
			continue
		}

		dsrc := mapFullSourceToConfiguredSource(*s)
		deviceSources = append(deviceSources, dsrc)

		if s.ID != "" {
			seen[s.ID] = true
		}
	}

	// 2. Add sources from presets if they are not already in the list
	for i := range dev.Presets {
		p := &dev.Presets[i]
		if p.Source.ID != "" && !seen[p.Source.ID] {
			dsrc := mapFullSourceToConfiguredSource(p.Source)
			deviceSources = append(deviceSources, dsrc)
			seen[p.Source.ID] = true
		}
	}

	// 3. Add sources from recents if they are not already in the list
	for i := range dev.Recents {
		r := &dev.Recents[i]
		if r.Source.ID != "" && !seen[r.Source.ID] {
			dsrc := mapFullSourceToConfiguredSource(r.Source)
			deviceSources = append(deviceSources, dsrc)
			seen[r.Source.ID] = true
		}
	}

	if err := ds.SaveConfiguredSources(accountID, deviceID, deviceSources); err != nil {
		log.Printf("[SYNC_ERR] Failed to save sources for %s: %v", deviceID, err)
	}
}

func syncPresets(ds *datastore.DataStore, accountID, deviceID string, presetsSource []models.FullResponsePreset) {
	var presets []models.ServicePreset

	for i := range presetsSource {
		p := &presetsSource[i]
		preset := models.ServicePreset{
			ServiceContentItem: models.ServiceContentItem{
				ContentItemType: p.ContentItemType,
				Location:        p.Location,
				Name:            p.Name,
				Source:          p.Source.Type,
				SourceID:        p.Source.ID,
				SourceAccount:   p.Source.Username,
			},
			ButtonNumber: p.ButtonNumber,
			ID:           p.ButtonNumber,
			CreatedOn:    p.CreatedOn,
			UpdatedOn:    p.UpdatedOn,
			ContainerArt: p.ContainerArt,
			SourceConfig: &models.ConfiguredSource{
				ID:               p.Source.ID,
				Type:             p.Source.Type,
				CreatedOn:        p.Source.CreatedOn,
				UpdatedOn:        p.Source.UpdatedOn,
				SourceName:       p.Source.SourceName,
				DisplayName:      p.Source.Name,
				Name:             p.Source.Name,
				SourceProviderID: p.Source.SourceProviderID,
				Secret:           p.Source.Credential.Value,
				SecretType:       p.Source.Credential.Type,
				Username:         p.Source.Username,
			},
		}
		presets = append(presets, preset)
	}

	if err := ds.SavePresets(accountID, deviceID, presets); err != nil {
		log.Printf("[SYNC_ERR] Failed to save presets for %s: %v", deviceID, err)
	}
}

func syncRecents(ds *datastore.DataStore, accountID, deviceID string, recentsSource []models.FullResponseRecent) {
	var recents []models.ServiceRecent

	for i := range recentsSource {
		r := &recentsSource[i]
		recent := models.ServiceRecent{
			ServiceContentItem: models.ServiceContentItem{
				ID:              r.ID,
				ContentItemType: r.ContentItemType,
				Location:        r.Location,
				Name:            r.Name,
				Source:          r.Source.Type,
				SourceID:        r.Source.ID,
				SourceAccount:   r.Source.Username,
			},
			CreatedOn:    r.CreatedOn,
			UpdatedOn:    r.UpdatedOn,
			LastPlayedAt: r.LastPlayedAt,
			SourceConfig: &models.ConfiguredSource{
				ID:               r.Source.ID,
				Type:             r.Source.Type,
				CreatedOn:        r.Source.CreatedOn,
				UpdatedOn:        r.Source.UpdatedOn,
				SourceName:       r.Source.SourceName,
				DisplayName:      r.Source.Name,
				Name:             r.Source.Name,
				SourceProviderID: r.Source.SourceProviderID,
				Secret:           r.Source.Credential.Value,
				SecretType:       r.Source.Credential.Type,
				Username:         r.Source.Username,
			},
		}
		recents = append(recents, recent)
	}

	if err := ds.SaveRecents(accountID, deviceID, recents); err != nil {
		log.Printf("[SYNC_ERR] Failed to save recents for %s: %v", deviceID, err)
	}
}

func mapFullSourceToConfiguredSource(s models.FullResponseSource) models.ConfiguredSource {
	dsrc := models.ConfiguredSource{
		ID:               s.ID,
		Type:             s.Type,
		CreatedOn:        s.CreatedOn,
		UpdatedOn:        s.UpdatedOn,
		SourceName:       s.SourceName,
		DisplayName:      s.DisplayName,
		Name:             s.Name,
		SourceProviderID: s.SourceProviderID,
		Secret:           s.Credential.Value,
		SecretType:       s.Credential.Type,
		Username:         s.Username,
		SourceSettings:   s.SourceSettings,
	}

	if dsrc.DisplayName == "" {
		dsrc.DisplayName = s.Name
	}

	if dsrc.Name == "" {
		dsrc.Name = s.DisplayName
	}

	dsrc.SourceKey.Type = s.Type
	dsrc.SourceKey.Account = s.Username

	return dsrc
}

// LogSyncDiff logs inconsistencies found between local state and upstream /full response.
// This is useful for debugging and verification.
func LogSyncDiff(ds *datastore.DataStore, resp *models.AccountFullResponse) {
	accountID := resp.ID
	for i := range resp.Devices {
		dev := &resp.Devices[i]
		deviceID := dev.DeviceID
		localPresets, _ := ds.GetPresets(accountID, deviceID)

		if len(localPresets) != len(dev.Presets) {
			log.Printf("[SYNC_DIFF] Preset count mismatch for %s: local=%d, upstream=%d", deviceID, len(localPresets), len(dev.Presets))
		}

		// Compare presets by button number
		for i := range dev.Presets {
			up := &dev.Presets[i]

			var found bool

			for j := range localPresets {
				lp := &localPresets[j]
				if lp.ButtonNumber == up.ButtonNumber {
					found = true

					if lp.Location != up.Location {
						log.Printf("[SYNC_DIFF] Preset %s location mismatch for %s: local=%s, upstream=%s", up.ButtonNumber, deviceID, lp.Location, up.Location)
					}

					break
				}
			}

			if !found {
				log.Printf("[SYNC_DIFF] Preset %s missing locally for %s", up.ButtonNumber, deviceID)
			}
		}
	}
}
