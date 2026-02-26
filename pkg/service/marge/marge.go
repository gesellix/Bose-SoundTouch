// Package marge provides XML generation and data management for the Marge service,
// which handles SoundTouch device configuration, presets, recents, and account management.
package marge

import (
	"bytes"
	"encoding/xml"
	"fmt"
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

// SourceProviders returns a list of available media source providers.
func SourceProviders() []models.SourceProvider {
	providers := make([]models.SourceProvider, len(constants.Providers))
	for i, name := range constants.Providers {
		providers[i] = models.SourceProvider{
			ID:        i + 1,
			CreatedOn: DateStr,
			Name:      name,
			UpdatedOn: DateStr,
		}
	}

	return providers
}

// SourceProvidersXML represents the XML structure for source providers.
type SourceProvidersXML struct {
	XMLName   xml.Name                `xml:"sourceProviders"`
	Providers []models.SourceProvider `xml:"sourceProvider"`
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

	return append([]byte(xml.Header), data...), nil
}

// ConfiguredSourceToXML converts a configured source to XML format.
func ConfiguredSourceToXML(cs models.ConfiguredSource) ([]byte, error) {
	type SourceXML struct {
		XMLName    xml.Name `xml:"source"`
		ID         string   `xml:"id,attr"`
		Type       string   `xml:"type,attr"`
		CreatedOn  string   `xml:"createdOn"`
		Credential struct {
			Type  string `xml:"type,attr"`
			Value string `xml:",chardata"`
		} `xml:"credential"`
		Name             string `xml:"name"`
		SourceProviderID string `xml:"sourceproviderid"`
		SourceName       string `xml:"sourcename"`
		SourceSettings   string `xml:"sourceSettings"`
		UpdatedOn        string `xml:"updatedOn"`
		Username         string `xml:"username"`
	}

	providerID := 0
	tokenType := "token"

	for i, p := range constants.Providers {
		if p == cs.SourceKeyType {
			providerID = i + 1

			if p == "SPOTIFY" {
				tokenType = "token_version_3"
			}

			break
		}
	}

	sxml := SourceXML{
		ID:               cs.ID,
		Type:             "Audio",
		CreatedOn:        DateStr,
		Name:             cs.SourceKeyAccount,
		SourceProviderID: strconv.Itoa(providerID),
		SourceName:       cs.DisplayName,
		UpdatedOn:        DateStr,
		Username:         cs.SourceKeyAccount,
	}
	sxml.Credential.Type = tokenType
	sxml.Credential.Value = cs.Secret

	return xml.Marshal(sxml)
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
	providerID := 0
	tokenType := "token"

	for i, p := range constants.Providers {
		if p == cs.SourceKeyType {
			providerID = i + 1

			if p == "SPOTIFY" {
				tokenType = "token_version_3"
			}

			break
		}
	}

	return fmt.Sprintf(`<source id="%s" type="Audio"><createdOn>%s</createdOn><credential type="%s">%s</credential><name>%s</name><sourceproviderid>%d</sourceproviderid><sourcename>%s</sourcename><sourceSettings></sourceSettings><updatedOn>%s</updatedOn><username>%s</username></source>`,
		EscapeXML(cs.ID), DateStr, EscapeXML(tokenType), EscapeXML(cs.Secret), EscapeXML(cs.SourceKeyAccount), providerID, EscapeXML(cs.DisplayName), DateStr, EscapeXML(cs.SourceKeyAccount))
}

// PresetsToXML converts account presets to XML format for Marge responses.
func PresetsToXML(ds *datastore.DataStore, account, device string) ([]byte, error) {
	presets, err := ds.GetPresets(account, device)
	if err != nil {
		return nil, err
	}

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		return nil, err
	}

	res := `<presets>`

	for i := range presets {
		p := &presets[i]
		res += fmt.Sprintf(`<preset buttonNumber="%s">`, EscapeXML(p.ID))
		res += fmt.Sprintf(`<containerArt>%s</containerArt>`, EscapeXML(p.ContainerArt))
		res += fmt.Sprintf(`<contentItemType>%s</contentItemType>`, EscapeXML(p.Type))
		res += fmt.Sprintf(`<createdOn>%s</createdOn>`, DateStr)
		res += fmt.Sprintf(`<location>%s</location>`, EscapeXML(p.Location))
		res += fmt.Sprintf(`<name>%s</name>`, EscapeXML(p.Name))

		// Content Item Source
		for j := range sources {
			s := sources[j]
			if s.ID == p.SourceID || (s.SourceKeyType == p.Source && s.SourceKeyAccount == p.SourceAccount) {
				res += GetConfiguredSourceXML(s)
				break
			}
		}

		res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, DateStr)
		res += `</preset>`
	}

	res += `</presets>`

	return append([]byte(xml.Header), []byte(res)...), nil
}

// RecentsToXML converts account recent items to XML format for Marge responses.
func RecentsToXML(ds *datastore.DataStore, account, device string) ([]byte, error) {
	recents, err := ds.GetRecents(account, device)
	if err != nil {
		return nil, err
	}

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		return nil, err
	}

	res := `<recents>`

	for i := range recents {
		r := &recents[i]

		lastPlayed := ""
		if sec, err := strconv.ParseInt(r.UtcTime, 10, 64); err == nil {
			lastPlayed = time.Unix(sec, 0).Format(time.RFC3339)
		}

		res += fmt.Sprintf(`<recent id="%s">`, EscapeXML(r.ID))
		res += fmt.Sprintf(`<contentItemType>%s</contentItemType>`, EscapeXML(r.Type))
		res += fmt.Sprintf(`<createdOn>%s</createdOn>`, DateStr)
		res += fmt.Sprintf(`<lastplayedat>%s</lastplayedat>`, EscapeXML(lastPlayed))
		res += fmt.Sprintf(`<location>%s</location>`, EscapeXML(r.Location))
		res += fmt.Sprintf(`<name>%s</name>`, EscapeXML(r.Name))

		// Content Item Source
		sourceID := ""

		for j := range sources {
			s := sources[j]
			if s.ID == r.SourceID || (s.SourceKeyType == r.Source && s.SourceKeyAccount == r.SourceAccount) {
				res += GetConfiguredSourceXML(s)
				sourceID = s.ID

				break
			}
		}

		if sourceID != "" {
			res += fmt.Sprintf(`<sourceid>%s</sourceid>`, EscapeXML(sourceID))
		}

		res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, DateStr)
		res += `</recent>`
	}

	res += `</recents>`

	return append([]byte(xml.Header), []byte(res)...), nil
}

// ProviderSettingsToXML generates provider settings XML for the specified account.
func ProviderSettingsToXML(account string) string {
	return xml.Header + fmt.Sprintf(`<providerSettings>
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
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><software_update><softwareUpdateLocation></softwareUpdateLocation></software_update>`
}

// AccountFullToXML generates a complete account XML with devices, presets, and recents.
func AccountFullToXML(ds *datastore.DataStore, account string) ([]byte, error) {
	devicesDir := ds.AccountDevicesDir(account)

	entries, err := os.ReadDir(devicesDir)
	if err != nil {
		return nil, err
	}

	res := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><account id="%s"><accountStatus>OK</accountStatus><devices>`, EscapeXML(account))
	lastDeviceID := ""

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		deviceID := entry.Name()
		lastDeviceID = deviceID

		info, err := ds.GetDeviceInfo(account, deviceID)
		if err != nil {
			continue
		}

		res += fmt.Sprintf(`<device deviceid="%s">`, EscapeXML(deviceID))

		res += fmt.Sprintf(`<attachedProduct product_code="%s">`, EscapeXML(info.ProductCode))
		if len(info.Components) > 0 {
			res += `<components>`
			for _, comp := range info.Components {
				res += fmt.Sprintf(`<component type="%s"><componentlabel>%s</componentlabel><firmware-version>%s</firmware-version><serialnumber>%s</serialnumber></component>`,
					EscapeXML(comp.Category), EscapeXML(comp.Category), EscapeXML(comp.SoftwareVersion), EscapeXML(comp.SerialNumber))
			}

			res += `</components>`
		} else {
			res += `<components/>`
		}

		res += fmt.Sprintf(`<productlabel>%s</productlabel><serialnumber>%s</serialnumber></attachedProduct>`,
			EscapeXML(info.ProductCode), EscapeXML(info.ProductSerialNumber))
		res += fmt.Sprintf(`<createdOn>%s</createdOn>`, DateStr)
		res += fmt.Sprintf(`<firmwareVersion>%s</firmwareVersion>`, EscapeXML(info.FirmwareVersion))
		res += fmt.Sprintf(`<ipaddress>%s</ipaddress>`, EscapeXML(info.IPAddress))
		res += fmt.Sprintf(`<name>%s</name>`, EscapeXML(info.Name))

		presets, _ := PresetsToXML(ds, account, deviceID)
		if len(presets) > len(xml.Header) {
			res += string(presets[len(xml.Header):]) // strip header
		}

		recents, _ := RecentsToXML(ds, account, deviceID)
		if len(recents) > len(xml.Header) {
			res += string(recents[len(xml.Header):]) // strip header
		}

		res += `</device>`
	}

	res += `</devices><mode>global</mode><preferredLanguage>en</preferredLanguage>`
	res += ProviderSettingsToXML(account)

	if lastDeviceID != "" {
		sources, _ := ds.GetConfiguredSources(account, lastDeviceID)

		res += `<sources>`
		for j := range sources {
			res += GetConfiguredSourceXML(sources[j])
		}

		res += `</sources>`
	}

	res += `</account>`

	return []byte(res), nil
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
	if err := xml.Unmarshal(sourceXML, &newPresetElem); err != nil {
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
			ID:            strconv.Itoa(presetNumber),
			Name:          newPresetElem.Name,
			Source:        matchingSrc.SourceKeyType,
			Type:          newPresetElem.ContentItemType,
			Location:      newPresetElem.Location,
			SourceAccount: matchingSrc.SourceKeyAccount,
			SourceID:      newPresetElem.SourceID,
		},
		ContainerArt: newPresetElem.ContainerArt,
		CreatedOn:    nowStr,
		UpdatedOn:    nowStr,
	}

	// Ensure presets list is large enough
	for len(presets) < presetNumber {
		presets = append(presets, models.ServicePreset{})
	}

	presets[presetNumber-1] = presetObj

	if err := ds.SavePresets(account, device, presets); err != nil {
		return nil, err
	}

	// Return XML for the single preset
	res := fmt.Sprintf(`<preset buttonNumber="%s">`, EscapeXML(presetObj.ID))
	res += fmt.Sprintf(`<containerArt>%s</containerArt>`, EscapeXML(presetObj.ContainerArt))
	res += fmt.Sprintf(`<contentItemType>%s</contentItemType>`, EscapeXML(presetObj.Type))
	res += fmt.Sprintf(`<createdOn>%s</createdOn>`, DateStr)
	res += fmt.Sprintf(`<location>%s</location>`, EscapeXML(presetObj.Location))
	res += fmt.Sprintf(`<name>%s</name>`, EscapeXML(presetObj.Name))
	res += GetConfiguredSourceXML(*matchingSrc)
	res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, DateStr)
	res += `</preset>`

	return append([]byte(xml.Header), []byte(res)...), nil
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
	}
	if err := xml.Unmarshal(sourceXML, &newRecentElem); err != nil {
		return nil, err
	}

	matchingSrc := findMatchingSource(sources, newRecentElem.SourceID)
	if matchingSrc == nil {
		// If we don't have a matching source, try to guess or create a virtual one.
		// For Spotify, the location usually starts with /playback/container/c3...
		// which is a base64 encoded spotify: URI.
		if strings.Contains(newRecentElem.Location, "spotify") || newRecentElem.SourceID == "SPOTIFY" {
			matchingSrc = &models.ConfiguredSource{
				ID:          newRecentElem.SourceID,
				DisplayName: "Spotify",
			}
			matchingSrc.SourceKey.Type = "SPOTIFY"
			matchingSrc.SourceKeyType = "SPOTIFY"
		} else {
			// fallback to a generic source if we can't guess
			matchingSrc = &models.ConfiguredSource{
				ID:          newRecentElem.SourceID,
				DisplayName: "Other",
			}
			matchingSrc.SourceKey.Type = "INVALID"
			matchingSrc.SourceKeyType = "INVALID"
		}
	}

	utcTime := parseLastPlayedAt(newRecentElem.LastPlayedAt)

	// Find existing
	var recentObj *models.ServiceRecent

	createdOn := DateStr

	for i := range recents {
		r := &recents[i]
		if r.Source == matchingSrc.SourceKeyType && r.Location == newRecentElem.Location && r.SourceAccount == matchingSrc.SourceKeyAccount {
			recents[i].UtcTime = strconv.FormatInt(utcTime, 10)
			recentObj = &recents[i]

			// Move to front
			recents = append([]models.ServiceRecent{*recentObj}, append(recents[:i], recents[i+1:]...)...)

			break
		}
	}

	if recentObj == nil {
		recentObj = createNewRecent(recents, newRecentElem.Name, matchingSrc, newRecentElem.ContentItemType, newRecentElem.Location, device, utcTime)
		createdOn = time.Now().Format(time.RFC3339)

		recents = append([]models.ServiceRecent{*recentObj}, recents...)
		if len(recents) > 10 {
			recents = recents[:10]
		}
	}

	if err := ds.SaveRecents(account, device, recents); err != nil {
		return nil, err
	}

	return formatRecentResponse(recentObj, matchingSrc, createdOn, utcTime), nil
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
		}
	}

	return utcTime
}

func createNewRecent(recents []models.ServiceRecent, name string, matchingSrc *models.ConfiguredSource, contentItemType, location, device string, utcTime int64) *models.ServiceRecent {
	maxID := 0
	for j := range recents {
		if id, err := strconv.Atoi(recents[j].ID); err == nil && id > maxID {
			maxID = id
		}
	}

	return &models.ServiceRecent{
		ServiceContentItem: models.ServiceContentItem{
			ID:            strconv.Itoa(maxID + 1),
			Name:          name,
			Source:        matchingSrc.SourceKeyType,
			Type:          contentItemType,
			Location:      location,
			SourceAccount: matchingSrc.SourceKeyAccount,
			SourceID:      matchingSrc.ID,
			IsPresetable:  "true",
		},
		DeviceID: device,
		UtcTime:  strconv.FormatInt(utcTime, 10),
	}
}

func formatRecentResponse(recentObj *models.ServiceRecent, matchingSrc *models.ConfiguredSource, createdOn string, utcTime int64) []byte {
	lastPlayed := time.Unix(utcTime, 0).Format(time.RFC3339)
	res := fmt.Sprintf(`<recent id="%s">`, EscapeXML(recentObj.ID))
	res += fmt.Sprintf(`<contentItemType>%s</contentItemType>`, EscapeXML(recentObj.Type))
	res += fmt.Sprintf(`<createdOn>%s</createdOn>`, EscapeXML(createdOn))
	res += fmt.Sprintf(`<lastplayedat>%s</lastplayedat>`, EscapeXML(lastPlayed))
	res += fmt.Sprintf(`<location>%s</location>`, EscapeXML(recentObj.Location))
	res += fmt.Sprintf(`<name>%s</name>`, EscapeXML(recentObj.Name))
	res += GetConfiguredSourceXML(*matchingSrc)
	res += fmt.Sprintf(`<sourceid>%s</sourceid>`, EscapeXML(matchingSrc.ID))
	res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, DateStr)
	res += `</recent>`

	return append([]byte(xml.Header), []byte(res)...)
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

	createdOn := time.Now().Format(time.RFC3339)
	res := fmt.Sprintf(`<device deviceid="%s">`, EscapeXML(newDeviceElem.DeviceID))
	res += fmt.Sprintf(`<createdOn>%s</createdOn>`, EscapeXML(createdOn))
	res += `<ipaddress></ipaddress>`
	res += fmt.Sprintf(`<name>%s</name>`, EscapeXML(newDeviceElem.Name))
	res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, EscapeXML(createdOn))
	res += `</device>`

	return append([]byte(xml.Header), []byte(res)...), nil
}

// RemoveDeviceFromAccount removes a device from the specified account.
func RemoveDeviceFromAccount(ds *datastore.DataStore, account, device string) error {
	return ds.RemoveDevice(account, device)
}
