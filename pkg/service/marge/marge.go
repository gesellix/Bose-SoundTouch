// Package marge provides XML generation and data management for the Marge service,
// which handles SoundTouch device configuration, presets, recents, and account management.
package marge

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// DateStr is a fixed timestamp used in XML responses for consistency.
const DateStr = "2012-09-19T12:43:00.000+00:00"

// FormatTime formats a time according to the Bose SoundTouch standard.
func FormatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000+00:00")
}

// SourceProviders returns a list of available media source providers.
func SourceProviders() []models.SourceProvider {
	providers := make([]models.SourceProvider, 0, len(constants.StaticProviders))
	for _, p := range constants.StaticProviders {
		providers = append(providers, models.SourceProvider{
			ID:        p.ID,
			CreatedOn: p.CreatedOn,
			Name:      p.Name,
			UpdatedOn: p.UpdatedOn,
		})
	}

	return providers
}

// SourceProvidersXML represents the XML structure for source providers.
type SourceProvidersXML struct {
	XMLName   xml.Name                `xml:"sourceProviders"`
	Providers []models.SourceProvider `xml:"sourceprovider"`
}

// SourceProvidersToXML converts source providers to XML format.
func SourceProvidersToXML() ([]byte, error) {
	sp := SourceProvidersXML{
		Providers: SourceProviders(),
	}

	data, err := xml.MarshalIndent(sp, "", "    ")
	if err != nil {
		return nil, err
	}

	return append([]byte(constants.XMLHeader+"\n"), data...), nil
}

// ConfiguredSourceToXML converts a configured source to XML format.
func ConfiguredSourceToXML(cs models.ConfiguredSource) ([]byte, error) {
	// Use the model's own MarshalXML for consistent output
	data, err := xml.Marshal(cs)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// EscapeXML escapes special characters for XML.
func EscapeXML(s string) string {
	var b bytes.Buffer
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}

	return b.String()
}

// GetConfiguredSourceXML returns the XML representation of a configured source as a string.
func GetConfiguredSourceXML(cs models.ConfiguredSource) string {
	data, _ := ConfiguredSourceToXML(cs)
	return string(data)
}

// PrepareConfiguredSource sets up the source for XML marshaling.
func PrepareConfiguredSource(s *models.ConfiguredSource) {
	providerID := s.SourceProviderID
	tokenType := "token"

	if providerID == "" {
		for _, p := range constants.StaticProviders {
			if p.Name == s.SourceKeyType {
				providerID = strconv.Itoa(p.ID)
				break
			}
		}
	}

	if s.SecretType == "" {
		if s.SourceKeyType == "SPOTIFY" {
			tokenType = "token_version_3"
		}

		s.SecretType = tokenType
	}

	if providerID == "" {
		providerID = "0"
	}

	if s.CreatedOn == "" {
		s.CreatedOn = DateStr
	}

	if s.UpdatedOn == "" {
		s.UpdatedOn = DateStr
	}

	s.Type = "Audio"
	s.SourceProviderID = providerID

	if s.SourceName == "" && s.DisplayName != "Other" {
		s.SourceName = s.DisplayName
	}

	if s.SourceKeyType == "TUNEIN" {
		s.SourceName = ""
	}

	if s.Username == "" {
		s.Username = s.SourceKeyAccount
	}

	s.SourceSettings = ""
}

// PresetsToXML converts account presets to XML format for Marge responses.
func PresetsToXML(ds *datastore.DataStore, account, deviceID string) ([]byte, error) {
	presets, err := ds.GetPresets(account, deviceID)
	if err != nil {
		return nil, err
	}

	sources, err := ds.GetConfiguredSources(account, deviceID)
	if err != nil {
		return nil, err
	}

	type PresetsXML struct {
		XMLName xml.Name               `xml:"presets"`
		Presets []models.ServicePreset `xml:"preset"`
	}

	pxml := PresetsXML{
		Presets: make([]models.ServicePreset, 0, len(presets)),
	}

	for i := range presets {
		p := presets[i]

		p.ButtonNumber = p.ID
		if p.CreatedOn == "" {
			p.CreatedOn = DateStr
		}

		if p.UpdatedOn == "" {
			p.UpdatedOn = DateStr
		}

		// Find and prepare source
		// Priority 1: sourceID match
		// Priority 2: source and sourceAccount match
		sourceID := p.SourceID
		if sourceID == "" {
			sourceID = p.SourceID
		}

		for j := range sources {
			s := sources[j]
			if (sourceID != "" && s.ID == sourceID) ||
				(s.SourceKeyType == p.Source && s.SourceKeyAccount == p.SourceAccount) {
				// Use a new variable to avoid pointer-to-iterator-variable bug
				matchedSource := s
				PrepareConfiguredSource(&matchedSource)
				p.SourceConfig = &matchedSource

				break
			}
		}

		pxml.Presets = append(pxml.Presets, p)
	}

	data, err := xml.MarshalIndent(pxml, "", "  ")
	if err != nil {
		return nil, err
	}

	return append([]byte(constants.XMLHeader+"\n"), data...), nil
}

// RecentsToXML converts account recent items to XML format for Marge responses.
func RecentsToXML(ds *datastore.DataStore, account, deviceID string) ([]byte, error) {
	recents, err := ds.GetRecents(account, deviceID)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte(constants.XMLHeader + `<recents/>`), nil
		}

		return nil, err
	}

	type RecentsXML struct {
		XMLName xml.Name               `xml:"recents"`
		Recents []models.ServiceRecent `xml:"recent"`
	}

	rxml := RecentsXML{
		Recents: recents,
	}

	for i := range rxml.Recents {
		r := &rxml.Recents[i]
		if r.SourceConfig == nil && r.SourceID != "" {
			sources, _ := ds.GetConfiguredSources(account, deviceID)
			for j := range sources {
				s := sources[j]
				if s.ID == r.SourceID {
					// Use a new variable to avoid pointer-to-iterator-variable bug
					matchedSource := s
					r.SourceConfig = &matchedSource

					break
				}
			}
		}

		if r.SourceConfig != nil {
			PrepareConfiguredSource(r.SourceConfig)
		}

		if r.UtcTime != "" {
			if t, parseErr := strconv.ParseInt(r.UtcTime, 10, 64); parseErr == nil {
				r.LastPlayedAt = time.Unix(t, 0).UTC().Format("2006-01-02T15:04:05.000+00:00")
			}
		}
	}

	data, err := xml.MarshalIndent(rxml, "", "  ")
	if err != nil {
		return nil, err
	}

	// Parity: use self-closing tags for empty SourceSettings
	data = bytes.ReplaceAll(data, []byte("<sourceSettings></sourceSettings>"), []byte("<sourceSettings/>"))

	header := constants.XMLHeader

	return append([]byte(header+"\n"), data...), nil
}

// ProviderSettingsToXML generates provider settings XML for the specified account.
func ProviderSettingsToXML(account string) string {
	return constants.XMLHeader + fmt.Sprintf(`<providerSettings>
    <providerSetting>
      <boseId>%s</boseId>
      <keyName>ELIGIBLE_FOR_TRIAL</keyName>
      <value>false</value>
      <providerId>14</providerId>
    </providerSetting>
    <providerSetting>
      <boseId>%s</boseId>
      <keyName>STREAMING_QUALITY</keyName>
      <value>2</value>
      <providerId>15</providerId>
    </providerSetting>
  </providerSettings>`, EscapeXML(account), EscapeXML(account))
}

// SoftwareUpdateToXML generates software update configuration XML.
func SoftwareUpdateToXML() string {
	return constants.XMLHeader + `
<software_update>
<softwareUpdateLocation></softwareUpdateLocation>
</software_update>`
}

// CreateAccountDevice creates an AccountDevice model for the given account and device.
func CreateAccountDevice(ds *datastore.DataStore, account, deviceID string) (models.AccountDevice, error) {
	info, err := ds.GetDeviceInfo(account, deviceID)
	if err != nil {
		return models.AccountDevice{}, err
	}

	device := models.AccountDevice{
		DeviceID: deviceID,
		AttachedProduct: &models.AttachedProduct{
			ProductCode:  info.ProductCode,
			ProductLabel: info.ProductCode,
			SerialNumber: info.ProductSerialNumber,
			UpdatedOn:    DateStr,
		},
		CreatedOn:       DateStr,
		FirmwareVersion: info.FirmwareVersion,
		IPAddress:       info.IPAddress,
		Name:            info.Name,
		SerialNumber:    info.DeviceSerialNumber,
		UpdatedOn:       DateStr,
	}

	if device.SerialNumber == "" && info.DeviceID != "" {
		device.SerialNumber = info.DeviceID
	}

	if device.AttachedProduct.SerialNumber == "" && info.ProductSerialNumber != "" {
		device.AttachedProduct.SerialNumber = info.ProductSerialNumber
	} else if device.AttachedProduct.SerialNumber == "" && device.SerialNumber != "" {
		device.AttachedProduct.SerialNumber = device.SerialNumber
	}

	if len(info.Components) > 0 {
		for _, comp := range info.Components {
			device.AttachedProduct.Components = append(device.AttachedProduct.Components, models.ServiceComponent{
				Category:        comp.Category,
				SoftwareVersion: comp.SoftwareVersion,
				SerialNumber:    comp.SerialNumber,
			})
		}
	}

	sources, _ := ds.GetConfiguredSources(account, deviceID)
	presets, _ := ds.GetPresets(account, deviceID)
	recents, _ := ds.GetRecents(account, deviceID)

	device.Presets = mapPresetsToFullResponse(presets, sources)
	device.Recents = mapRecentsToFullResponse(recents, sources)

	return device, nil
}

func mapToFullResponseSource(s models.ConfiguredSource) models.FullResponseSource {
	fullSource := models.FullResponseSource{
		ID:               s.ID,
		Type:             s.Type,
		DisplayName:      s.DisplayName,
		CreatedOn:        s.CreatedOn,
		Name:             s.SourceKeyAccount,
		SourceProviderID: s.SourceProviderID,
		SourceName:       s.SourceName,
		SourceSettings:   "",
		UpdatedOn:        s.UpdatedOn,
		Username:         s.Username,
	}
	fullSource.Credential.Type = s.SecretType
	fullSource.Credential.Value = s.Secret

	if fullSource.Credential.Type == "" || fullSource.Credential.Type == "token" {
		if s.Type == "SPOTIFY" || s.SourceProviderID == "SPOTIFY" || s.SourceKeyType == "SPOTIFY" {
			fullSource.Credential.Type = "token_version_3"
		} else if fullSource.Credential.Type == "" {
			fullSource.Credential.Type = "token"
		}
	}

	if s.SourceKeyType == "TUNEIN" {
		fullSource.SourceName = ""
	}

	if fullSource.Username == "" {
		fullSource.Username = s.SourceKeyAccount
	}

	return fullSource
}

func mapPresetsToFullResponse(presets []models.ServicePreset, sources []models.ConfiguredSource) []models.FullResponsePreset {
	var fullPresets []models.FullResponsePreset

	for i := range presets {
		p := &presets[i]

		if p.CreatedOn == "" {
			p.CreatedOn = DateStr
		}

		if p.UpdatedOn == "" {
			p.UpdatedOn = DateStr
		}

		var matchedSource *models.ConfiguredSource

		for j := range sources {
			s := sources[j]
			if s.ID == p.SourceID || s.SourceKeyType == p.Source {
				// Use a new variable to avoid pointer-to-iterator-variable bug
				copySource := s
				PrepareConfiguredSource(&copySource)
				matchedSource = &copySource

				break
			}
		}

		fullPreset := models.FullResponsePreset{
			ButtonNumber:    p.ButtonNumber,
			ContainerArt:    p.ContainerArt,
			ContentItemType: p.ContentItemType,
			CreatedOn:       p.CreatedOn,
			Location:        p.Location,
			Name:            p.Name,
			UpdatedOn:       p.UpdatedOn,
			Username:        p.Username,
		}
		if matchedSource != nil {
			fullPreset.Source = mapToFullResponseSource(*matchedSource)
		}

		fullPresets = append(fullPresets, fullPreset)
	}

	return fullPresets
}

func mapRecentsToFullResponse(recents []models.ServiceRecent, sources []models.ConfiguredSource) []models.FullResponseRecent {
	var fullRecents []models.FullResponseRecent

	for i := range recents {
		r := &recents[i]
		if r.CreatedOn == "" {
			r.CreatedOn = DateStr
		}

		if r.UpdatedOn == "" {
			r.UpdatedOn = DateStr
		}

		var matchedSource *models.ConfiguredSource

		for j := range sources {
			s := sources[j]
			if s.ID == r.SourceID || s.SourceKeyType == r.Source {
				// Use a new variable to avoid pointer-to-iterator-variable bug
				copySource := s
				PrepareConfiguredSource(&copySource)
				matchedSource = &copySource

				break
			}
		}

		fullRecent := models.FullResponseRecent{
			ID:              r.ID,
			ContentItemType: r.ContentItemType,
			CreatedOn:       r.CreatedOn,
			LastPlayedAt:    r.LastPlayedAt,
			Location:        r.Location,
			Name:            r.Name,
			SourceID:        r.SourceID,
			UpdatedOn:       r.UpdatedOn,
		}
		if matchedSource != nil {
			fullRecent.Source = mapToFullResponseSource(*matchedSource)
		}

		fullRecents = append(fullRecents, fullRecent)
	}

	return fullRecents
}

// AccountFullToXML generates a complete account XML with devices, presets, and recents.
func AccountFullToXML(ds *datastore.DataStore, account string) ([]byte, error) {
	devicesDir := ds.AccountDevicesDir(account)

	entries, err := os.ReadDir(devicesDir)
	if err != nil {
		return nil, err
	}

	resp := models.AccountFullResponse{
		ID:                account,
		AccountStatus:     "OK",
		Mode:              "global",
		PreferredLanguage: "de",
	}

	for _, p := range constants.StaticProviders {
		switch p.Name {
		case "DEEZER":
			resp.ProviderSettings = append(resp.ProviderSettings, models.ProviderSetting{
				BoseID:     account,
				KeyName:    "ELIGIBLE_FOR_TRIAL",
				Value:      "false",
				ProviderID: strconv.Itoa(p.ID),
			})
		case "SPOTIFY":
			resp.ProviderSettings = append(resp.ProviderSettings, models.ProviderSetting{
				BoseID:     account,
				KeyName:    "STREAMING_QUALITY",
				Value:      "2",
				ProviderID: strconv.Itoa(p.ID),
			})
		}
	}

	if info, _ := ds.GetAccountInfo(account); info != nil {
		if info.PreferredLanguage != "" {
			resp.PreferredLanguage = info.PreferredLanguage
		}

		if len(info.ProviderSettings) > 0 {
			resp.ProviderSettings = info.ProviderSettings
		}
	}

	for i := range resp.ProviderSettings {
		ps := &resp.ProviderSettings[i]
		if ps.ProviderName == "" {
			ps.ProviderName = constants.GetProviderName(ps.ProviderID)
		}
	}

	var lastDeviceID string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		deviceID := entry.Name()
		lastDeviceID = deviceID

		var dev models.AccountDevice

		dev, err = CreateAccountDevice(ds, account, deviceID)
		if err != nil {
			continue
		}

		resp.Devices = append(resp.Devices, dev)
	}

	if lastDeviceID != "" {
		sources, _ := ds.GetConfiguredSources(account, lastDeviceID)
		for i := range sources {
			s := sources[i]
			PrepareConfiguredSource(&s)

			resp.Sources = append(resp.Sources, mapToFullResponseSource(s))
		}
	}

	data, err := xml.Marshal(resp)
	if err != nil {
		return nil, err
	}

	// Parity: use self-closing tags for empty components and sourceSettings
	data = bytes.ReplaceAll(data, []byte("<components></components>"), []byte("<components/>"))
	data = bytes.ReplaceAll(data, []byte("<sourceSettings> </sourceSettings>"), []byte("<sourceSettings/>"))
	data = bytes.ReplaceAll(data, []byte("<sourceSettings></sourceSettings>"), []byte("<sourceSettings/>"))
	data = bytes.ReplaceAll(data, []byte("<name></name>"), []byte("<name/>"))

	return append([]byte(constants.XMLHeader), data...), nil
}

// UpdatePreset updates or creates a preset for the specified account and device.
func UpdatePreset(ds *datastore.DataStore, account, device string, presetNumber int, sourceXML []byte) ([]byte, error) {
	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		return nil, err
	}

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		return nil, err
	}

	var newPresetElem struct {
		Name            string `xml:"name"`
		SourceID        string `xml:"sourceid"`
		Location        string `xml:"location"`
		ContentItemType string `xml:"contentItemType"`
		ContainerArt    string `xml:"containerArt"`
	}
	if err = xml.Unmarshal(sourceXML, &newPresetElem); err != nil {
		return nil, err
	}

	var matchingSrc *models.ConfiguredSource

	for i := range sources {
		if sources[i].ID == newPresetElem.SourceID {
			matchingSrc = &sources[i]
			break
		}
	}

	if matchingSrc == nil {
		return nil, fmt.Errorf("invalid account/source")
	}

	nowStr := strconv.FormatInt(time.Now().Unix(), 10)
	presetObj := models.ServicePreset{
		ServiceContentItem: models.ServiceContentItem{
			Name:            newPresetElem.Name,
			Source:          matchingSrc.SourceKeyType,
			Type:            newPresetElem.ContentItemType,
			Location:        newPresetElem.Location,
			SourceAccount:   matchingSrc.SourceKeyAccount,
			SourceID:        newPresetElem.SourceID,
			ContentItemType: newPresetElem.ContentItemType,
		},
		ID:           strconv.Itoa(presetNumber),
		ContainerArt: newPresetElem.ContainerArt,
		CreatedOn:    nowStr,
		UpdatedOn:    nowStr,
		ButtonNumber: strconv.Itoa(presetNumber),
	}

	// Ensure presets list is large enough
	for len(presets) < presetNumber {
		presets = append(presets, models.ServicePreset{})
	}

	presets[presetNumber-1] = presetObj

	if err = ds.SavePresets(account, device, presets); err != nil {
		return nil, err
	}

	// Return XML for the single preset
	PrepareConfiguredSource(matchingSrc)
	presetObj.SourceConfig = matchingSrc

	data, err := xml.Marshal(presetObj)
	if err != nil {
		return nil, err
	}

	return append([]byte(constants.XMLHeader), data...), nil
}

// AddRecent adds or updates a recent item for the specified account and device.
func AddRecent(ds *datastore.DataStore, account, device string, sourceXML []byte) ([]byte, error) {
	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	recents, err := ds.GetRecents(account, device)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	var newRecentElem struct {
		Name            string `xml:"name"`
		SourceID        string `xml:"sourceid"`
		Location        string `xml:"location"`
		ContentItemType string `xml:"contentItemType"`
		LastPlayedAt    string `xml:"lastplayedat"`
		Source          struct {
			ID               string `xml:"id,attr"`
			Type             string `xml:"type,attr"`
			SourceName       string `xml:"sourcename"`
			SourceProviderID string `xml:"sourceproviderid"`
			CreatedOn        string `xml:"createdOn"`
			UpdatedOn        string `xml:"updatedOn"`
			Credential       struct {
				Type  string `xml:"type,attr"`
				Value string `xml:",chardata"`
			} `xml:"credential"`
		} `xml:"source"`
	}
	if err := xml.Unmarshal(sourceXML, &newRecentElem); err != nil {
		return nil, err
	}

	sourceName := newRecentElem.Source.SourceName
	if sourceName == "" {
		// Some clients might send sourcename as a direct child of recent
		var altRecentElem struct {
			SourceName string `xml:"sourcename"`
		}

		_ = xml.Unmarshal(sourceXML, &altRecentElem)
		sourceName = altRecentElem.SourceName
	}

	matchingSrc, learned := learnSource(ds, account, device, sources, newRecentElem.SourceID, newRecentElem.Location, sourceName, newRecentElem.Source.Credential.Value, newRecentElem.Source.SourceProviderID, newRecentElem.Source.CreatedOn, newRecentElem.Source.UpdatedOn)
	if learned {
		// Re-fetch sources to ensure we have the newly learned one
		sources, _ = ds.GetConfiguredSources(account, device)
		matchingSrc = findMatchingSource(sources, newRecentElem.SourceID)
	}

	if matchingSrc == nil {
		matchingSrc = &models.ConfiguredSource{ID: newRecentElem.SourceID}
	} else if matchingSrc.ID == "" {
		matchingSrc.ID = newRecentElem.SourceID
	}

	// Ensure DisplayName and SourceName are consistent
	if matchingSrc.SourceName == "" && matchingSrc.DisplayName != "" {
		// Parity: for some services like TuneIn, sourcename should be empty
		if matchingSrc.DisplayName != "TuneIn" && matchingSrc.DisplayName != "Other" {
			matchingSrc.SourceName = matchingSrc.DisplayName
		}
	}

	if matchingSrc.DisplayName == "" && matchingSrc.SourceName != "" {
		matchingSrc.DisplayName = matchingSrc.SourceName
	}

	utcTime := parseLastPlayedAt(newRecentElem.LastPlayedAt)
	recentObj, recents := updateOrCreateRecent(recents, newRecentElem.Name, matchingSrc, newRecentElem.ContentItemType, newRecentElem.Location, device, utcTime)

	if err := ds.SaveRecents(account, device, recents); err != nil {
		return nil, err
	}

	return formatRecentResponse(recentObj, matchingSrc, recentObj.CreatedOn, utcTime), nil
}

func learnSource(ds *datastore.DataStore, account, device string, sources []models.ConfiguredSource, sourceID, location, sourceName, credentialValue, sourceProviderID, createdOn, updatedOn string) (*models.ConfiguredSource, bool) {
	matchingSrc := findMatchingSource(sources, sourceID)
	sourceLearned := false

	if matchingSrc == nil {
		matchingSrc = createLearnedSource(sourceID, location, sourceName, credentialValue, sourceProviderID, createdOn, updatedOn)
		sourceLearned = true
	} else {
		sourceLearned = updateSourceFields(matchingSrc, credentialValue, sourceName, sourceProviderID)
	}

	if sourceLearned {
		persistLearnedSource(ds, account, device, sources, matchingSrc)
	}

	return matchingSrc, sourceLearned
}

func createLearnedSource(sourceID, location, sourceName, credentialValue, sourceProviderID, createdOn, updatedOn string) *models.ConfiguredSource {
	displayName := sourceName
	// For TuneIn, we often see empty DisplayName/SourceName in recent items
	// if it's already a known source or if it's a generic TuneIn request.
	if displayName == "" && sourceID != "" {
		// Try to deduce from sourceID if it looks like a known service
		switch sourceID {
		case "14774275": // TuneIn
			displayName = "TuneIn"
		case "Spotify":
			displayName = "Spotify"
		}
	}

	src := &models.ConfiguredSource{
		ID:               sourceID,
		DisplayName:      displayName,
		SourceName:       sourceName,
		Secret:           credentialValue,
		SourceProviderID: sourceProviderID,
		CreatedOn:        createdOn,
		UpdatedOn:        updatedOn,
	}

	switch {
	case sourceProviderID == "25" || sourceID == "TUNEIN" || strings.Contains(location, "/v1/playback/station/"):
		src.SourceKey.Type = "TUNEIN"
		src.SourceKeyType = "TUNEIN"
		src.Type = "Audio"

		if src.DisplayName == "Other" || src.DisplayName == "TuneIn" {
			src.DisplayName = "TuneIn"
		}
	case strings.Contains(location, "spotify") || strings.Contains(location, "c3BvdGlme") || sourceID == "SPOTIFY":
		src.SourceKey.Type = "SPOTIFY"
		src.SourceKeyType = "SPOTIFY"
		src.Type = "Audio"
		src.SecretType = "token_version_3"

		if src.DisplayName == "Other" {
			src.DisplayName = "Spotify"
		}
	default:
		src.SourceKey.Type = "INVALID"
		src.SourceKeyType = "INVALID"
	}

	return src
}

func updateSourceFields(src *models.ConfiguredSource, credentialValue, sourceName, sourceProviderID string) bool {
	learned := false

	if credentialValue != "" && src.Secret == "" {
		src.Secret = credentialValue
		learned = true
	}

	if sourceName != "" && src.SourceName == "" {
		src.SourceName = sourceName
		learned = true
	}

	if sourceProviderID != "" && src.SourceProviderID == "" {
		src.SourceProviderID = sourceProviderID
		learned = true
	}

	return learned
}

func persistLearnedSource(ds *datastore.DataStore, account, device string, sources []models.ConfiguredSource, matchingSrc *models.ConfiguredSource) {
	updatedSources := make([]models.ConfiguredSource, len(sources))
	copy(updatedSources, sources)

	found := false

	for i := range updatedSources {
		if updatedSources[i].ID == matchingSrc.ID {
			updatedSources[i] = *matchingSrc
			found = true

			break
		}
	}

	if !found {
		updatedSources = append(updatedSources, *matchingSrc)
	}

	if err := ds.SaveConfiguredSources(account, device, updatedSources); err != nil {
		log.Printf("[MARGE_ERR] Failed to persist learned source for %s: %v", device, err)
	}
}

func updateOrCreateRecent(recents []models.ServiceRecent, name string, matchingSrc *models.ConfiguredSource, contentItemType, location, device string, utcTime int64) (*models.ServiceRecent, []models.ServiceRecent) {
	var recentObj *models.ServiceRecent

	for i := range recents {
		r := &recents[i]

		sourceMatch := false
		if matchingSrc != nil {
			sourceMatch = r.Source == matchingSrc.SourceKeyType && r.SourceAccount == matchingSrc.SourceKeyAccount
		}

		if sourceMatch && r.Location == location {
			recents[i].UtcTime = strconv.FormatInt(utcTime, 10)
			recents[i].UpdatedOn = FormatTime(time.Now())
			recentObj = &recents[i]
			// Move to front
			recents = append([]models.ServiceRecent{*recentObj}, append(recents[:i], recents[i+1:]...)...)

			return recentObj, recents
		}
	}

	recentObj = createNewRecent(recents, name, matchingSrc, contentItemType, location, device, utcTime)
	recentObj.UpdatedOn = FormatTime(time.Now())

	recents = append([]models.ServiceRecent{*recentObj}, recents...)
	if len(recents) > 10 {
		recents = recents[:10]
	}

	return recentObj, recents
}

func findMatchingSource(sources []models.ConfiguredSource, sourceID string) *models.ConfiguredSource {
	for i := range sources {
		if sources[i].ID == sourceID {
			return &sources[i]
		}
	}

	return nil
}

func parseLastPlayedAt(lastPlayedAt string) int64 {
	utcTime := time.Now().Unix()

	if lastPlayedAt != "" {
		if t, err := time.Parse(time.RFC3339, lastPlayedAt); err == nil {
			utcTime = t.Unix()
		} else if t, err := time.Parse("2006-01-02T15:04:05.000-07:00", lastPlayedAt); err == nil {
			utcTime = t.Unix()
		} else if t, err := time.Parse("2006-01-02T15:04:05.000Z", lastPlayedAt); err == nil {
			utcTime = t.Unix()
		}
	}

	return utcTime
}

func createNewRecent(recents []models.ServiceRecent, name string, matchingSrc *models.ConfiguredSource, contentItemType, location, device string, utcTime int64) *models.ServiceRecent {
	// Refined ID generation: YYMMDD (6 digits) + 3-digit counter (9 digits total, fits in 32-bit signed int).
	// Max value for 32-bit signed int is 2,147,483,647.
	// 260315999 is well within the limit.
	prefixStr := time.Now().UTC().Format("060102")
	// For testing parity with older logs, we might want to check if a specific date is requested.
	// But usually, we just use today.
	prefix, _ := strconv.Atoi(prefixStr)
	baseID := int64(prefix) * 1000

	maxCounter := 0

	for j := range recents {
		if id, err := strconv.Atoi(recents[j].ID); err == nil {
			if int64(id) >= baseID && int64(id) < baseID+1000 {
				counter := id % 1000
				if counter > maxCounter {
					maxCounter = counter
				}
			}
		}
	}

	newID := int(baseID) + maxCounter + 1

	r := &models.ServiceRecent{
		ServiceContentItem: models.ServiceContentItem{
			ID:              strconv.Itoa(newID),
			Name:            name,
			Type:            contentItemType,
			ContentItemType: contentItemType,
			Location:        location,
			IsPresetable:    "true",
		},
		DeviceID:  device,
		UtcTime:   strconv.FormatInt(utcTime, 10),
		CreatedOn: FormatTime(time.Now()),
	}

	if matchingSrc != nil {
		r.Source = matchingSrc.SourceKeyType
		r.SourceAccount = matchingSrc.SourceKeyAccount
		r.SourceID = matchingSrc.ID
	}

	return r
}

func formatRecentResponse(recentObj *models.ServiceRecent, matchingSrc *models.ConfiguredSource, createdOn string, utcTime int64) []byte {
	if matchingSrc != nil {
		PrepareConfiguredSource(matchingSrc)
		recentObj.SourceConfig = matchingSrc
	}

	recentObj.CreatedOn = createdOn
	recentObj.UpdatedOn = createdOn
	recentObj.UtcTime = strconv.FormatInt(utcTime, 10)
	recentObj.LastPlayedAt = time.Unix(utcTime, 0).UTC().Format("2006-01-02T15:04:05.000+00:00")

	data, _ := xml.MarshalIndent(recentObj, "", "  ")

	// Parity: use self-closing tags for empty SourceSettings
	data = bytes.ReplaceAll(data, []byte("<sourceSettings></sourceSettings>"), []byte("<sourceSettings/>"))

	header := constants.XMLHeader

	return append([]byte(header+"\n"), data...)
}

// AddDeviceToAccount adds a new device to the specified account.
func AddDeviceToAccount(ds *datastore.DataStore, account string, sourceXML []byte) ([]byte, error) {
	var newDeviceElem struct {
		DeviceID string `xml:"deviceid,attr"`
		Name     string `xml:"name"`
	}
	if err := xml.Unmarshal(sourceXML, &newDeviceElem); err != nil {
		return nil, err
	}

	info := &models.ServiceDeviceInfo{
		DeviceID: newDeviceElem.DeviceID,
		Name:     newDeviceElem.Name,
		// Other fields will be filled by discovery later or default
	}

	if err := ds.SaveDeviceInfo(account, newDeviceElem.DeviceID, info); err != nil {
		return nil, err
	}

	createdOn := FormatTime(time.Now())
	res := fmt.Sprintf(`<device deviceid="%s">`, EscapeXML(newDeviceElem.DeviceID))
	res += fmt.Sprintf(`<createdOn>%s</createdOn>`, EscapeXML(createdOn))
	res += `<ipaddress></ipaddress>`
	res += fmt.Sprintf(`<name>%s</name>`, EscapeXML(newDeviceElem.Name))
	res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, EscapeXML(createdOn))
	res += `</device>`

	header := constants.XMLHeader

	return append([]byte(header), []byte(res)...), nil
}

// RemoveDeviceFromAccount removes a device from the specified account.
func RemoveDeviceFromAccount(ds *datastore.DataStore, account, device string) error {
	return ds.RemoveDevice(account, device)
}
