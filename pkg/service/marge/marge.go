package marge

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

const DateStr = "2012-09-19T12:43:00.000+00:00"

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

type SourceProvidersXML struct {
	XMLName   xml.Name                `xml:"sourceProviders"`
	Providers []models.SourceProvider `xml:"sourceProvider"`
}

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
		SourceSettings   string `xml:"sourcesettings"`
		UpdatedOn        string `xml:"updatedOn"`
		Username         string `xml:"username"`
	}

	providerID := 0

	for i, p := range constants.Providers {
		if p == cs.SourceKeyType {
			providerID = i + 1
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
	sxml.Credential.Type = "token"
	sxml.Credential.Value = cs.Secret

	return xml.Marshal(sxml)
}

func GetConfiguredSourceXML(cs models.ConfiguredSource) string {
	providerID := 0

	for i, p := range constants.Providers {
		if p == cs.SourceKeyType {
			providerID = i + 1
			break
		}
	}

	return fmt.Sprintf(`<source id="%s" type="Audio"><createdOn>%s</createdOn><credential type="token">%s</credential><name>%s</name><sourceproviderid>%d</sourceproviderid><sourcename>%s</sourcename><sourcesettings></sourcesettings><updatedOn>%s</updatedOn><username>%s</username></source>`,
		cs.ID, DateStr, cs.Secret, cs.SourceKeyAccount, providerID, cs.DisplayName, DateStr, cs.SourceKeyAccount)
}

func PresetsToXML(ds *datastore.DataStore, account string) ([]byte, error) {
	presets, err := ds.GetPresets(account)
	if err != nil {
		return nil, err
	}

	sources, err := ds.GetConfiguredSources(account)
	if err != nil {
		return nil, err
	}

	res := `<presets>`
	for _, p := range presets {
		res += fmt.Sprintf(`<preset buttonNumber="%s">`, p.ID)
		res += fmt.Sprintf(`<containerArt>%s</containerArt>`, p.ContainerArt)
		res += fmt.Sprintf(`<contentItemType>%s</contentItemType>`, p.Type)
		res += fmt.Sprintf(`<createdOn>%s</createdOn>`, DateStr)
		res += fmt.Sprintf(`<location>%s</location>`, p.Location)
		res += fmt.Sprintf(`<name>%s</name>`, p.Name)

		// Content Item Source
		found := false

		for _, s := range sources {
			if s.ID == p.SourceID || (s.SourceKeyType == p.Source && s.SourceKeyAccount == p.SourceAccount) {
				res += GetConfiguredSourceXML(s)
				found = true

				break
			}
		}

		if !found {
			// This might happen if source is not found
		}

		res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, DateStr)
		res += `</preset>`
	}

	res += `</presets>`

	return append([]byte(xml.Header), []byte(res)...), nil
}

func RecentsToXML(ds *datastore.DataStore, account string) ([]byte, error) {
	recents, err := ds.GetRecents(account)
	if err != nil {
		return nil, err
	}

	sources, err := ds.GetConfiguredSources(account)
	if err != nil {
		return nil, err
	}

	res := `<recents>`

	for _, r := range recents {
		lastPlayed := ""
		if sec, err := strconv.ParseInt(r.UtcTime, 10, 64); err == nil {
			lastPlayed = time.Unix(sec, 0).Format(time.RFC3339)
		}

		res += fmt.Sprintf(`<recent id="%s">`, r.ID)
		res += fmt.Sprintf(`<contentItemType>%s</contentItemType>`, r.Type)
		res += fmt.Sprintf(`<createdOn>%s</createdOn>`, DateStr)
		res += fmt.Sprintf(`<lastplayedat>%s</lastplayedat>`, lastPlayed)
		res += fmt.Sprintf(`<location>%s</location>`, r.Location)
		res += fmt.Sprintf(`<name>%s</name>`, r.Name)

		found := false

		for _, s := range sources {
			if s.ID == r.SourceID || (s.SourceKeyType == r.Source && s.SourceKeyAccount == r.SourceAccount) {
				res += GetConfiguredSourceXML(s)
				found = true

				break
			}
		}

		if !found {
		}

		res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, DateStr)
		res += `</recent>`
	}

	res += `</recents>`

	return append([]byte(xml.Header), []byte(res)...), nil
}

func ProviderSettingsToXML(account string) string {
	return fmt.Sprintf(`<providerSettings><providerSetting><boseId>%s</boseId><keyName>ELIGIBLE_FOR_TRIAL</keyName><value>true</value><providerId>14</providerId></providerSetting></providerSettings>`, account)
}

func SoftwareUpdateToXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><software_update><softwareUpdateLocation></softwareUpdateLocation></software_update>`
}

func AccountFullToXML(ds *datastore.DataStore, account string) ([]byte, error) {
	devicesDir := ds.AccountDevicesDir(account)

	entries, err := os.ReadDir(devicesDir)
	if err != nil {
		return nil, err
	}

	res := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><account id="%s"><accountStatus>OK</accountStatus><devices>`, account)
	lastDeviceID := ""

	for _, entry := range entries {
		if entry.IsDir() {
			deviceID := entry.Name()
			lastDeviceID = deviceID

			info, err := ds.GetDeviceInfo(account, deviceID)
			if err != nil {
				continue
			}

			res += fmt.Sprintf(`<device deviceid="%s">`, deviceID)
			res += fmt.Sprintf(`<attachedProduct product_code="%s"><components/><productlabel>%s</productlabel><serialnumber>%s</serialnumber></attachedProduct>`,
				info.ProductCode, info.ProductCode, info.ProductSerialNumber)
			res += fmt.Sprintf(`<createdOn>%s</createdOn>`, DateStr)
			res += fmt.Sprintf(`<firmwareVersion>%s</firmwareVersion>`, info.FirmwareVersion)
			res += fmt.Sprintf(`<ipaddress>%s</ipaddress>`, info.IPAddress)
			res += fmt.Sprintf(`<name>%s</name>`, info.Name)

			presets, _ := PresetsToXML(ds, account)
			if len(presets) > len(xml.Header) {
				res += string(presets[len(xml.Header):]) // strip header
			}

			recents, _ := RecentsToXML(ds, account)
			if len(recents) > len(xml.Header) {
				res += string(recents[len(xml.Header):]) // strip header
			}

			res += fmt.Sprintf(`<serialnumber>%s</serialnumber>`, info.DeviceSerialNumber)
			res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, DateStr)
			res += `</device>`
		}
	}

	res += `</devices><mode>global</mode><preferredLanguage>en</preferredLanguage>`
	res += ProviderSettingsToXML(account)

	if lastDeviceID != "" {
		sources, _ := ds.GetConfiguredSources(account)

		res += `<sources>`
		for _, s := range sources {
			res += GetConfiguredSourceXML(s)
		}

		res += `</sources>`
	}

	res += `</account>`

	return []byte(res), nil
}

func UpdatePreset(ds *datastore.DataStore, account string, device string, presetNumber int, sourceXML []byte) ([]byte, error) {
	sources, err := ds.GetConfiguredSources(account)
	if err != nil {
		return nil, err
	}

	presets, err := ds.GetPresets(account)
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

	for _, s := range sources {
		if s.ID == newPresetElem.SourceID {
			matchingSrc = &s
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

	if err := ds.SavePresets(account, presets); err != nil {
		return nil, err
	}

	// Return XML for the single preset
	res := fmt.Sprintf(`<preset buttonNumber="%s">`, presetObj.ID)
	res += fmt.Sprintf(`<containerArt>%s</containerArt>`, presetObj.ContainerArt)
	res += fmt.Sprintf(`<contentItemType>%s</contentItemType>`, presetObj.Type)
	res += fmt.Sprintf(`<createdOn>%s</createdOn>`, DateStr)
	res += fmt.Sprintf(`<location>%s</location>`, presetObj.Location)
	res += fmt.Sprintf(`<name>%s</name>`, presetObj.Name)
	res += GetConfiguredSourceXML(*matchingSrc)
	res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, DateStr)
	res += `</preset>`

	return append([]byte(xml.Header), []byte(res)...), nil
}

func AddRecent(ds *datastore.DataStore, account string, device string, sourceXML []byte) ([]byte, error) {
	sources, err := ds.GetConfiguredSources(account)
	if err != nil {
		return nil, err
	}

	recents, err := ds.GetRecents(account)
	if err != nil {
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

	var matchingSrc *models.ConfiguredSource

	for _, s := range sources {
		if s.ID == newRecentElem.SourceID {
			matchingSrc = &s
			break
		}
	}

	if matchingSrc == nil {
		return nil, fmt.Errorf("invalid account/source")
	}

	utcTime := time.Now().Unix()

	if newRecentElem.LastPlayedAt != "" {
		if t, err := time.Parse(time.RFC3339, newRecentElem.LastPlayedAt); err == nil {
			utcTime = t.Unix()
		}
	}

	// Find existing
	var recentObj *models.ServiceRecent

	createdOn := DateStr

	for i, r := range recents {
		if r.Source == matchingSrc.SourceKeyType && r.Location == newRecentElem.Location && r.SourceAccount == matchingSrc.SourceKeyAccount {
			recents[i].UtcTime = strconv.FormatInt(utcTime, 10)
			recentObj = &recents[i]
			// Moving to front means we need to handle its original createdOn
			// In bose emulation, we often use fixed dates, but let's try to be consistent
			// If we had a real createdOn, we'd use it here.

			// Move to front
			recents = append([]models.ServiceRecent{*recentObj}, append(recents[:i], recents[i+1:]...)...)

			break
		}
	}

	if recentObj == nil {
		maxID := 0
		for _, r := range recents {
			if id, err := strconv.Atoi(r.ID); err == nil && id > maxID {
				maxID = id
			}
		}

		recentObj = &models.ServiceRecent{
			ServiceContentItem: models.ServiceContentItem{
				ID:            strconv.Itoa(maxID + 1),
				Name:          newRecentElem.Name,
				Source:        matchingSrc.SourceKeyType,
				Type:          newRecentElem.ContentItemType,
				Location:      newRecentElem.Location,
				SourceAccount: matchingSrc.SourceKeyAccount,
				SourceID:      newRecentElem.SourceID,
				IsPresetable:  "true",
			},
			DeviceID: device,
			UtcTime:  strconv.FormatInt(utcTime, 10),
		}
		createdOn = time.Now().Format(time.RFC3339)

		recents = append([]models.ServiceRecent{*recentObj}, recents...)
		if len(recents) > 10 {
			recents = recents[:10]
		}
	}

	if err := ds.SaveRecents(account, recents); err != nil {
		return nil, err
	}

	lastPlayed := time.Unix(utcTime, 0).Format(time.RFC3339)
	res := fmt.Sprintf(`<recent id="%s">`, recentObj.ID)
	res += fmt.Sprintf(`<contentItemType>%s</contentItemType>`, recentObj.Type)
	res += fmt.Sprintf(`<createdOn>%s</createdOn>`, createdOn)
	res += fmt.Sprintf(`<lastplayedat>%s</lastplayedat>`, lastPlayed)
	res += fmt.Sprintf(`<location>%s</location>`, recentObj.Location)
	res += fmt.Sprintf(`<name>%s</name>`, recentObj.Name)
	res += GetConfiguredSourceXML(*matchingSrc)
	res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, DateStr)
	res += `</recent>`

	return append([]byte(xml.Header), []byte(res)...), nil
}

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
	res := fmt.Sprintf(`<device deviceid="%s">`, newDeviceElem.DeviceID)
	res += fmt.Sprintf(`<createdOn>%s</createdOn>`, createdOn)
	res += `<ipaddress></ipaddress>`
	res += fmt.Sprintf(`<name>%s</name>`, newDeviceElem.Name)
	res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, createdOn)
	res += `</device>`

	return append([]byte(xml.Header), []byte(res)...), nil
}

func RemoveDeviceFromAccount(ds *datastore.DataStore, account string, device string) error {
	return ds.RemoveDevice(account, device)
}
