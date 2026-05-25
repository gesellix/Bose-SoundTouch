package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/go-chi/chi/v5"
)

// validatePathID ensures that an identifier is safe to use as a single path component.
func validatePathID(id string) bool {
	if id == "" {
		return false
	}

	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return false
	}

	if strings.Contains(id, "..") {
		return false
	}

	return true
}

// HandleMgmtAccountDetails returns full details for an account for the Web UI.
func (s *Server) HandleMgmtAccountDetails(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountId")
	if !validatePathID(accountID) {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	// 1. Get account info
	accountInfo, err := s.ds.GetAccountInfo(accountID)
	if err != nil {
		log.Printf("[Mgmt] Failed to get account info for %s: %s", sanitizeLog(accountID), sanitizeErr(err))
		accountInfo = &models.ServiceAccountInfo{AccountID: accountID}
	}

	// Enrich provider settings with names
	for i := range accountInfo.ProviderSettings {
		s := &accountInfo.ProviderSettings[i]
		if s.ProviderName == "" {
			s.ProviderName = constants.GetProviderName(s.ProviderID)
		}
	}

	// 2. List all devices for this account
	allDevices, err := s.ds.ListAllDevices()
	if err != nil {
		log.Printf("[Mgmt] Failed to list devices: %v", err)
	}

	accountDevices := make([]deviceDetail, 0)

	for i := range allDevices {
		d := &allDevices[i]
		if d.AccountID != accountID {
			continue
		}

		detail := s.getDeviceDetail(accountID, d)
		accountDevices = append(accountDevices, detail)
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"account": accountInfo,
		"devices": accountDevices,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode account details: %v", err)
	}
}

// HandleMgmtUpdateAccountLanguage updates the preferred language for an account.
func (s *Server) HandleMgmtUpdateAccountLanguage(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountId")
	if !validatePathID(accountID) {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Language string `json:"language"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Language != "en" && req.Language != "de" {
		http.Error(w, "Language must be 'en' or 'de'", http.StatusBadRequest)
		return
	}

	// 1. Load current account info
	accountInfo, err := s.ds.GetAccountInfo(accountID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Update language
	accountInfo.AccountID = accountID // Ensure ID is correct
	accountInfo.PreferredLanguage = req.Language
	accountInfo.IsPlaceholder = false

	// 3. Save account info
	if err := s.ds.SaveAccountInfo(accountID, accountInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleMgmtUpdateAccountProviderSetting updates a specific provider setting for an account.
func (s *Server) HandleMgmtUpdateAccountProviderSetting(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountId")
	if !validatePathID(accountID) {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	var req struct {
		ProviderID string `json:"provider_id"`
		Key        string `json:"key"`
		Value      string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ProviderID == "" || req.Key == "" {
		http.Error(w, "provider_id and key are required", http.StatusBadRequest)
		return
	}

	// 1. Load current account info
	accountInfo, err := s.ds.GetAccountInfo(accountID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 2. Update the specific setting
	found := false

	for i, setting := range accountInfo.ProviderSettings {
		if setting.ProviderID == req.ProviderID && setting.KeyName == req.Key {
			accountInfo.ProviderSettings[i].Value = req.Value
			found = true

			break
		}
	}

	if !found {
		// If not found, we can choose to add it or return error.
		// For now, let's return an error as we expect to edit existing ones.
		http.Error(w, "Provider setting not found", http.StatusNotFound)
		return
	}

	accountInfo.AccountID = accountID // Ensure ID is correct
	accountInfo.IsPlaceholder = false

	// 3. Save account info
	if err := s.ds.SaveAccountInfo(accountID, accountInfo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

type deviceDetail struct {
	models.AccountDevice
	Presets    []models.FullResponsePreset `json:"presets,omitempty"`
	Recents    []models.FullResponseRecent `json:"recents,omitempty"`
	Sources    []models.FullResponseSource `json:"sources,omitempty"`
	Components []models.ServiceComponent   `json:"components,omitempty"`
}

func (s *Server) getDeviceDetail(accountID string, d *models.ServiceDeviceInfo) deviceDetail {
	detail := deviceDetail{
		AccountDevice: models.AccountDevice{
			DeviceID:           d.DeviceID,
			FirmwareVersion:    d.FirmwareVersion,
			IPAddress:          d.IPAddress,
			Name:               d.Name,
			ProductCode:        d.ProductCode,
			SerialNumber:       d.DeviceSerialNumber,
			DeviceSerialNumber: d.DeviceSerialNumber,
			MacAddress:         d.MacAddress,
			DiscoveryMethod:    d.DiscoveryMethod,
		},
	}

	// We also have AttachedProduct which has Components
	detail.AttachedProduct = &models.AttachedProduct{
		SerialNumber: d.DeviceSerialNumber,
		ProductCode:  d.ProductCode,
		ProductLabel: d.Name,
		Components:   d.Components,
	}
	detail.Components = d.Components

	// Fetch sources
	var configuredSources []models.ConfiguredSource

	sources, err := s.ds.GetConfiguredSources(accountID, d.DeviceID)
	if err == nil {
		configuredSources = sources
		for j := range sources {
			fs := mapToFullResponseSource(&sources[j])
			if fs.Type == "" && fs.Name == "" && fs.DisplayName == "" {
				log.Printf("[Mgmt] Skipping empty source for device %s", sanitizeLog(d.DeviceID))
				continue
			}

			detail.Sources = append(detail.Sources, fs)
		}
	}

	// Fetch presets
	if presets, err := s.ds.GetPresets(accountID, d.DeviceID); err == nil {
		for j := range presets {
			detail.Presets = append(detail.Presets, mapToFullResponsePreset(&presets[j], configuredSources))
		}
	}

	detail.AccountDevice.Presets = detail.Presets

	// Fetch recents
	if recents, err := s.ds.GetRecents(accountID, d.DeviceID); err == nil {
		for j := range recents {
			detail.Recents = append(detail.Recents, mapToFullResponseRecent(&recents[j], configuredSources))
		}
	}

	detail.AccountDevice.Recents = detail.Recents

	return detail
}

func mapToFullResponseSource(src *models.ConfiguredSource) models.FullResponseSource {
	fs := models.FullResponseSource{
		ID:               src.ID,
		Type:             src.Type,
		DisplayName:      src.DisplayName,
		Name:             src.DisplayName,
		Username:         src.Username,
		SourceName:       src.SourceName,
		SourceProviderID: src.SourceProviderID,
		CreatedOn:        src.CreatedOn,
		UpdatedOn:        src.UpdatedOn,
		Account:          src.SourceKey.Account,
		SourceLabel:      constants.GetSourceLabel(src.Type),
		ProviderLabel:    constants.GetProviderLabel(src.SourceProviderID),
		SourceSettings:   src.SourceSettings,
	}
	fs.Credential.Value = src.Secret
	fs.Credential.Type = src.SecretType

	// If DisplayName is generic (e.g. "Audio") and we have a more specific Account name, use it.
	if fs.DisplayName == fs.Type && fs.Account != "" {
		fs.DisplayName = fs.Account
		fs.Name = fs.Account
	}

	// Provide fallback for Name and SourceName if missing
	switch {
	case fs.Name != "":
		// Name already set to DisplayName
	case fs.Account != "":
		fs.Name = fs.Account
	case fs.SourceLabel != "":
		fs.Name = fs.SourceLabel
	default:
		fs.Name = fs.Type
	}

	if fs.SourceName == "" {
		fs.SourceName = fs.Name
	}

	if fs.DisplayName == "" {
		fs.DisplayName = fs.Name
	}

	return fs
}

func mapToFullResponsePreset(p *models.ServicePreset, configuredSources []models.ConfiguredSource) models.FullResponsePreset {
	fp := models.FullResponsePreset{
		ButtonNumber:    p.ButtonNumber,
		ContainerArt:    p.ContainerArt,
		ContentItemType: p.ContentItemType,
		CreatedOn:       p.CreatedOn,
		Location:        p.Location,
		Name:            p.Name,
		UpdatedOn:       p.UpdatedOn,
		Username:        p.Username,
	}
	if fp.Username == "" {
		fp.Username = p.Name
	}

	if fp.Name == "" {
		fp.Name = p.Name
	}

	if fp.CreatedOn == "" && p.CreatedOn != "" {
		fp.CreatedOn = p.CreatedOn
	}

	// Attempt to find matching source in configuredSources
	found := false

	for k := range configuredSources {
		src := &configuredSources[k]
		if src.SourceKey.Type == p.Source && (src.SourceKey.Account == p.SourceAccount || p.SourceAccount == "") {
			fp.Source = mapToFullResponseSource(src)
			found = true

			break
		}
	}

	if !found && p.Source != "" {
		// Create a dummy source for UI purposes if not found in configured sources
		dummy := &models.ConfiguredSource{
			Type: p.Source,
		}
		dummy.SourceKey.Type = p.Source
		dummy.SourceKey.Account = p.SourceAccount
		fp.Source = mapToFullResponseSource(dummy)
	}

	return fp
}

func mapToFullResponseRecent(r *models.ServiceRecent, configuredSources []models.ConfiguredSource) models.FullResponseRecent {
	fr := models.FullResponseRecent{
		ID:              r.ID,
		ContentItemType: r.ContentItemType,
		CreatedOn:       r.CreatedOn,
		LastPlayedAt:    r.LastPlayedAt,
		Location:        r.Location,
		Name:            r.Name,
		SourceID:        r.SourceID,
		UpdatedOn:       r.UpdatedOn,
		Username:        r.Username,
	}
	if fr.LastPlayedAt == "" && r.UtcTime != "" {
		if ut, err := strconv.ParseInt(r.UtcTime, 10, 64); err == nil {
			fr.LastPlayedAt = time.Unix(ut, 0).UTC().Format("2006-01-02T15:04:05.000+00:00")
		}
	}

	if fr.Username == "" {
		fr.Username = r.Name
	}

	if fr.Name == "" {
		fr.Name = r.Name
	}

	if fr.CreatedOn == "" && r.CreatedOn != "" {
		fr.CreatedOn = r.CreatedOn
	} else if fr.CreatedOn == "" && r.UtcTime != "" {
		fr.CreatedOn = r.UtcTime
	}

	// Attempt to find matching source in configuredSources
	found := false

	for k := range configuredSources {
		src := &configuredSources[k]
		if src.SourceKey.Type == r.Source && (src.SourceKey.Account == r.SourceAccount || r.SourceAccount == "") {
			fr.Source = mapToFullResponseSource(src)
			if fr.SourceID == "" {
				fr.SourceID = fr.Source.ID
			}

			found = true

			break
		}
	}

	if !found && r.Source != "" {
		// Create a dummy source for UI purposes if not found in configured sources
		dummy := &models.ConfiguredSource{
			Type: r.Source,
		}
		dummy.SourceKey.Type = r.Source
		dummy.SourceKey.Account = r.SourceAccount

		fr.Source = mapToFullResponseSource(dummy)
		if fr.SourceID == "" {
			fr.SourceID = fr.Source.ID
		}
	}

	return fr
}

// HandleMgmtListAccounts returns a list of all account IDs in the datastore.
func (s *Server) HandleMgmtListAccounts(w http.ResponseWriter, _ *http.Request) {
	accounts, err := s.ds.ListAccounts()
	if err != nil {
		log.Printf("[Mgmt] Failed to list accounts: %v", err)

		accounts = []string{"default"}
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"accounts": accounts,
	}); err != nil {
		log.Printf("[Mgmt] Failed to encode accounts: %v", err)
	}
}
