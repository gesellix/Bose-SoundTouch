// Package marge provides XML generation and data management for the Marge service,
// which handles SoundTouch device configuration, presets, recents, and account management.
package marge

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

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
	ensureTimestamps(s)
	ensureSourceType(s)
	ensureSourceProviderID(s)
	syncCredentials(s)
	syncLegacySourceKey(s)
}

func ensureTimestamps(s *models.ConfiguredSource) {
	if s.CreatedOn == "" {
		s.CreatedOn = constants.DateStr
	}

	if s.UpdatedOn == "" {
		s.UpdatedOn = constants.DateStr
	}
}

func ensureSourceType(s *models.ConfiguredSource) {
	// AUX must be normalized to Type="Audio" — the speaker rejects type="AUX"
	// (which the datastore previously synthesized from SourceKey.Type).
	// Bluetooth is left alone since its canonical Type isn't "Audio".
	if s.Type == "" || (s.SourceKey.Type != "" && s.SourceKey.Type != constants.ProviderBluetooth) {
		if s.SourceKey.Type == constants.ProviderAmazon {
			s.Type = constants.ProviderAmazon
		} else {
			s.Type = "Audio"
		}
	}
}

// canonicalProviderIDByName returns the canonical sourceproviderid for a
// symbolic source-key type ("TUNEIN", "SPOTIFY", ...) via constants.StaticProviders.
// Returns the provider ID as a decimal string and ok=true on match, ("", false) otherwise.
func canonicalProviderIDByName(name string) (string, bool) {
	for _, p := range constants.StaticProviders {
		if p.Name == name {
			return strconv.Itoa(p.ID), true
		}
	}

	return "", false
}

// HasResolvableProviderID reports whether s can be given a canonical
// sourceproviderid — it already carries one, or its source-key type maps to a
// constants.StaticProviders entry. Sources without one are device-local /
// transient slots (STORED_MUSIC_MEDIA_RENDERER, UPNP, the INVALID_SOURCE
// sentinel, ...) that a speaker rejects as INVALID_SOURCE (issue #334); they
// must not be persisted for, or served to, a speaker.
func HasResolvableProviderID(s models.ConfiguredSource) bool {
	if s.SourceProviderID != "" {
		return true
	}

	if _, ok := canonicalProviderIDByName(s.SourceKey.Type); ok {
		return true
	}

	if _, ok := canonicalProviderIDByName(s.SourceKeyType); ok {
		return true
	}

	return false
}

func ensureSourceProviderID(s *models.ConfiguredSource) {
	if s.SourceProviderID == "" && s.SourceKey.Type != "" {
		if id, ok := canonicalProviderIDByName(s.SourceKey.Type); ok {
			s.SourceProviderID = id
		}
	}
}

func syncCredentials(s *models.ConfiguredSource) {
	if s.SecretType == "" {
		if s.SourceKey.Type == constants.ProviderSpotify {
			s.SecretType = constants.CredentialTypeTokenV3
		} else {
			s.SecretType = constants.CredentialTypeToken
		}
	}

	if s.Credential.Type == "" {
		s.Credential.Type = s.SecretType
	}

	if s.Credential.Value == "" {
		s.Credential.Value = s.Secret
	}

	if s.Secret == "" && s.Credential.Value != "" {
		s.Secret = s.Credential.Value
	}

	if s.SecretType == "" && s.Credential.Type != "" {
		s.SecretType = s.Credential.Type
	}
}

func syncLegacySourceKey(s *models.ConfiguredSource) {
	if s.SourceKey.Type == "" && s.SourceKeyType != "" {
		s.SourceKey.Type = s.SourceKeyType
	}

	if s.SourceKey.Account == "" && s.SourceKeyAccount != "" {
		s.SourceKey.Account = s.SourceKeyAccount
	}
}

// PresetsXML is the XML wrapper for a list of presets.
type PresetsXML struct {
	XMLName xml.Name          `xml:"presets"`
	Presets []presetParityXML `xml:"preset"`
}

type presetParityXML struct {
	ButtonNumber    string                   `xml:"buttonNumber,attr,omitempty"`
	ContainerArt    string                   `xml:"containerArt"`
	ContentItemType string                   `xml:"contentItemType"`
	CreatedOn       string                   `xml:"createdOn"`
	Location        string                   `xml:"location"`
	Name            string                   `xml:"name"`
	Source          *models.ConfiguredSource `xml:"source,omitempty"`
	UpdatedOn       string                   `xml:"updatedOn"`
	Username        string                   `xml:"username"`
}

func (p presetParityXML) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	type Alias presetParityXML

	start.Name.Local = "preset"

	return e.EncodeElement(Alias(p), start)
}

// recentSourceUsername returns the account/username to emit in a recent's
// <source> block. The account (e.g. STORED_MUSIC's "<UDN>/0") is persisted in
// SourceKey.Account, not Username, which does not round-trip through the
// datastore — so fall back to SourceKeyAccount. Without this, STORED_MUSIC
// media-server recents get an empty <username>, the speaker falls back to the
// provider id (e.g. "7") as the account, and replay fails with INVALID_SOURCE.
// Mirrors the fallback in mapToFullResponseSource; TuneIn / Internet Radio /
// Local Internet Radio intentionally keep an empty username (parity).
func recentSourceUsername(src *models.ConfiguredSource) string {
	if src.Username != "" {
		return src.Username
	}

	switch src.SourceKeyType {
	case constants.ProviderTunein, constants.ProviderInternetRadio, constants.ProviderLocalInternetRadio:
		return ""
	}

	return src.SourceKeyAccount
}

func prepareRecentItemParitySource(src *models.ConfiguredSource) *models.RecentItemParitySource {
	sxml := &models.RecentItemParitySource{
		ID:               src.ID,
		Type:             src.Type,
		CreatedOn:        src.CreatedOn,
		UpdatedOn:        src.UpdatedOn,
		Name:             src.DisplayName,
		SourceProviderID: src.SourceProviderID,
		SourceName:       src.SourceName,
		Username:         recentSourceUsername(src),
		Credential: &models.RecentItemParityCredential{
			Type:  src.Credential.Type,
			Value: src.Credential.Value,
		},
	}

	if sxml.Name == "TuneIn" || sxml.Name == constants.ProviderLocalInternetRadio {
		sxml.Name = ""
	}

	secret := src.Secret
	if secret == "" {
		secret = src.Credential.Value
	}

	secretType := src.SecretType
	if secretType == "" {
		secretType = src.Credential.Type
	}

	if secretType == "" {
		secretType = constants.CredentialTypeToken
	}

	if sxml.Credential.Value == "" {
		sxml.Credential.Value = secret
	}

	if sxml.Credential.Type == "" {
		sxml.Credential.Type = secretType
	}

	if sxml.SourceName == "" {
		sxml.SourceName = src.SourceKeyType
	}

	if sxml.SourceName == "" {
		sxml.SourceName = sxml.Username
	}

	if sxml.Username == "" {
		sxml.Username = sxml.SourceName
	}

	if sxml.Name == "" {
		sxml.Name = sxml.SourceName
	}

	return sxml
}

func mapPresetToParityXML(p models.ServicePreset, sources []models.ConfiguredSource) *presetParityXML {
	// Find and prepare source
	matchedSource := findMatchingSourceForPreset(sources, p)
	if matchedSource != nil {
		PrepareConfiguredSource(matchedSource)
	}

	if p.ContentItemType == "" && p.Name == "" && p.Location == "" && (matchedSource == nil || matchedSource.ID == "") {
		return nil
	}

	p.ButtonNumber = p.ID
	if p.CreatedOn == "" {
		p.CreatedOn = constants.DateStr
	} else if t, e := strconv.ParseInt(p.CreatedOn, 10, 64); e == nil {
		p.CreatedOn = time.Unix(t, 0).UTC().Format("2006-01-02T15:04:05.000+00:00")
	}

	if p.UpdatedOn == "" {
		p.UpdatedOn = constants.DateStr
	} else if t, e := strconv.ParseInt(p.UpdatedOn, 10, 64); e == nil {
		p.UpdatedOn = time.Unix(t, 0).UTC().Format("2006-01-02T15:04:05.000+00:00")
	}

	username := p.Username
	if username == "" {
		username = p.Name
	}

	return &presetParityXML{
		ButtonNumber:    p.ButtonNumber,
		ContainerArt:    p.ContainerArt,
		ContentItemType: p.ContentItemType,
		CreatedOn:       p.CreatedOn,
		Location:        p.Location,
		Name:            p.Name,
		Source:          matchedSource,
		UpdatedOn:       p.UpdatedOn,
		Username:        username,
	}
}

// AccountPresetsToXML aggregates presets from all account devices and converts to XML format.
func AccountPresetsToXML(ds *datastore.DataStore, account string) ([]byte, error) {
	accountDir := ds.AccountDevicesDir(account)

	entries, err := ds.ReadDirUnderBase(accountDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte(constants.XMLHeader + "\n<presets/>"), nil
		}

		return nil, err
	}

	type presetsParityWrapper struct {
		XMLName xml.Name          `xml:"presets"`
		Presets []presetParityXML `xml:"preset"`
	}

	pxml := presetsParityWrapper{
		Presets: make([]presetParityXML, 0),
	}

	seenPresets := make(map[string]bool)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		deviceID := entry.Name()

		presets, gerr := ds.GetPresets(account, deviceID)
		if gerr != nil {
			continue
		}

		sources, serr := ds.GetConfiguredSources(account, deviceID)
		if serr != nil {
			continue
		}

		for i := range presets {
			p := presets[i]

			// Deduplicate by ID (which is the buttonNumber in our store)
			if seenPresets[p.ID] {
				continue
			}

			if pXML := mapPresetToParityXML(p, sources); pXML != nil {
				seenPresets[p.ID] = true

				pxml.Presets = append(pxml.Presets, *pXML)
			}
		}
	}

	// Sort by buttonNumber (optional but nice)
	sort.Slice(pxml.Presets, func(i, j int) bool {
		ni, _ := strconv.Atoi(pxml.Presets[i].ButtonNumber)
		nj, _ := strconv.Atoi(pxml.Presets[j].ButtonNumber)

		return ni < nj
	})

	data, err := xml.MarshalIndent(pxml, "", "  ")
	if err != nil {
		return nil, err
	}

	return append([]byte(constants.XMLHeader+"\n"), data...), nil
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

	type presetsParityWrapper struct {
		XMLName xml.Name          `xml:"presets"`
		Presets []presetParityXML `xml:"preset"`
	}

	pxml := presetsParityWrapper{
		Presets: make([]presetParityXML, 0, len(presets)),
	}

	for i := range presets {
		if pXML := mapPresetToParityXML(presets[i], sources); pXML != nil {
			pxml.Presets = append(pxml.Presets, *pXML)
		}
	}

	data, err := xml.MarshalIndent(pxml, "", "  ")
	if err != nil {
		return nil, err
	}

	return append([]byte(constants.XMLHeader+"\n"), data...), nil
}

func findMatchingSourceForPreset(sources []models.ConfiguredSource, p models.ServicePreset) *models.ConfiguredSource {
	// First try exact ID match
	if p.SourceID != "" {
		for j := range sources {
			if sources[j].ID == p.SourceID {
				return &sources[j]
			}
		}
	}

	// Then try type and account match
	for j := range sources {
		s := &sources[j]
		if (s.SourceKey.Type == p.Source && (p.SourceAccount == "" || s.SourceKey.Account == p.SourceAccount)) ||
			(s.SourceKeyType == p.Source && (p.SourceAccount == "" || s.SourceKeyAccount == p.SourceAccount)) {
			return s
		}
	}

	return nil
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

	type recentsParityXML struct {
		XMLName xml.Name `xml:"recents"`
		Recents []recent `xml:"recent"`
	}

	rxml := recentsParityXML{
		Recents: make([]recent, len(recents)),
	}

	sources, _ := ds.GetConfiguredSources(account, deviceID)

	for i := range recents {
		r := &recents[i]

		var matchingSrc *models.ConfiguredSource

		if r.SourceID != "" {
			matchingSrc = findMatchingSource(sources, r.SourceID)
		}

		if matchingSrc != nil {
			PrepareConfiguredSource(matchingSrc)
		} else if r.Source != "" {
			// Try to find by Source and SourceAccount if SourceID didn't match
			for j := range sources {
				if sources[j].SourceKeyType == r.Source && sources[j].SourceKeyAccount == r.SourceAccount {
					matchingSrc = &sources[j]
					PrepareConfiguredSource(matchingSrc)

					break
				}
			}
		}

		rxml.Recents[i] = recentToXML(r, matchingSrc)
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

type recent struct {
	ID              string                         `xml:"id,attr"`
	ContentItem     *contentItem                   `xml:"contentItem"`
	ContentItemType string                         `xml:"contentItemType"`
	CreatedOn       string                         `xml:"createdOn"`
	LastPlayedAt    string                         `xml:"lastplayedat"`
	Location        string                         `xml:"location"`
	Name            string                         `xml:"name"`
	Source          *models.RecentItemParitySource `xml:"source,omitempty"`
	SourceID        string                         `xml:"sourceid"`
	UpdatedOn       string                         `xml:"updatedOn"`
}

type contentItem struct {
	Source        string `xml:"source,attr"`
	Type          string `xml:"type,attr"`
	Location      string `xml:"location,attr"`
	SourceAccount string `xml:"sourceAccount,attr"`
	IsPresetable  string `xml:"isPresetable,attr"`
	ItemName      string `xml:"itemName"`
	ContainerArt  string `xml:"containerArt,omitempty"`
}

func recentToXML(r *models.ServiceRecent, matchingSrc *models.ConfiguredSource) recent {
	utcTime := int64(0)

	if r.UtcTime != "" {
		if t, parseErr := strconv.ParseInt(r.UtcTime, 10, 64); parseErr == nil {
			utcTime = t
		}
	}

	createdOn := r.CreatedOn
	if createdOn == "" {
		createdOn = FormatTime(time.Now())
	}

	updatedOn := r.UpdatedOn
	if updatedOn == "" {
		updatedOn = createdOn
	}

	lastPlayedAt := r.LastPlayedAt
	if lastPlayedAt == "" && utcTime > 0 {
		lastPlayedAt = time.Unix(utcTime, 0).UTC().Format("2006-01-02T15:04:05.000+00:00")
	}

	res := recent{
		ID:              r.ID,
		ContentItemType: r.ContentItemType,
		CreatedOn:       createdOn,
		UpdatedOn:       updatedOn,
		LastPlayedAt:    lastPlayedAt,
		Location:        r.Location,
		Name:            r.Name,
		SourceID:        r.SourceID,
		ContentItem: &contentItem{
			Source:        r.Source,
			Type:          r.Type,
			Location:      r.Location,
			SourceAccount: r.SourceAccount,
			IsPresetable:  r.IsPresetable,
			ItemName:      r.Name,
			ContainerArt:  r.ContainerArt,
		},
	}

	if matchingSrc != nil {
		res.Source = prepareRecentItemParitySource(matchingSrc)
	}

	return res
}

// ProviderSettingsToXML generates provider settings XML for the specified account.
func ProviderSettingsToXML(account string) string {
	type providerSetting struct {
		BoseID     string `xml:"boseId"`
		KeyName    string `xml:"keyName"`
		Value      string `xml:"value"`
		ProviderID string `xml:"providerId"`
	}

	type providerSettings struct {
		XMLName  xml.Name          `xml:"providerSettings"`
		Settings []providerSetting `xml:"providerSetting"`
	}

	payload := providerSettings{
		Settings: []providerSetting{
			{
				BoseID:     account,
				KeyName:    "ELIGIBLE_FOR_TRIAL",
				Value:      "false",
				ProviderID: "14",
			},
			{
				BoseID:     account,
				KeyName:    "STREAMING_QUALITY",
				Value:      "2",
				ProviderID: "15",
			},
		},
	}

	out, err := xml.Marshal(payload)
	if err != nil {
		return constants.XMLHeader + `<providerSettings></providerSettings>`
	}

	return constants.XMLHeader + string(out)
}

// SoftwareUpdateToXML generates software update configuration XML.
func SoftwareUpdateToXML() string {
	return constants.XMLHeader + `
<software_update>
<softwareUpdateLocation></softwareUpdateLocation>
</software_update>`
}

// APIVersionsToXML returns the XML response for Marge API versions.
func APIVersionsToXML() ([]byte, error) {
	resp := models.MargeAPIVersionsResponse{
		Version: "221",
		Project: "origin/master",
		Apis: []models.MargeAPI{
			{
				Type: "streaming",
				XML:  "application/vnd.bose.streaming-v1.0+xml",
				JSON: "application/vnd.bose.streaming-v1.0+json",
			},
			{
				Type: "customer",
				XML:  "application/vnd.bose.customer-v1.0+xml",
				JSON: "application/vnd.bose.customer-v1.0+json",
			},
			{
				Type: "support",
				XML:  "application/vnd.bose.support-v1.0+xml",
				JSON: "application/vnd.bose.support-v1.0+json",
			},
		},
	}

	output, err := xml.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}

	return append([]byte(constants.XMLHeader+"\n"), output...), nil
}

// CreateAccountDevice creates an AccountDevice model for the given account and device.
func CreateAccountDevice(ds *datastore.DataStore, account, deviceID string) (models.AccountDevice, error) {
	info, err := ds.GetDeviceInfo(account, deviceID)
	if err != nil {
		return models.AccountDevice{}, err
	}

	if info == nil {
		return models.AccountDevice{}, fmt.Errorf("device info not found")
	}

	device := models.AccountDevice{
		DeviceID: deviceID,
		AttachedProduct: &models.AttachedProduct{
			ProductCode:  info.ProductCode,
			ProductLabel: info.ProductCode,
			SerialNumber: info.ProductSerialNumber,
			UpdatedOn:    constants.DateStr,
		},
		CreatedOn:       constants.DateStr,
		FirmwareVersion: info.FirmwareVersion,
		IPAddress:       info.IPAddress,
		Name:            info.Name,
		SerialNumber:    info.DeviceSerialNumber,
		UpdatedOn:       constants.DateStr,
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
				Label:           comp.Label,
			})
		}
	}

	sources, err := ds.GetConfiguredSources(account, deviceID)
	if err != nil {
		return models.AccountDevice{}, err
	}

	presets, _ := ds.GetPresets(account, deviceID)
	recents, _ := ds.GetRecents(account, deviceID)

	device.Presets = mapPresetsToFullResponse(presets, sources)
	device.Recents = mapRecentsToFullResponse(recents, sources)

	return device, nil
}

func resolveSourceName(s models.ConfiguredSource) string {
	// Prefer human-readable names; fall back to the account ID only when nothing better is set.
	var name string

	switch {
	case s.SourceName != "":
		name = s.SourceName
	case s.DisplayName != "":
		name = s.DisplayName
	default:
		name = s.SourceKeyAccount
	}
	// FALLBACKS for common sources
	if name == "" {
		switch s.SourceKeyType {
		case constants.ProviderInternetRadio:
			name = constants.ProviderInternetRadio
		case constants.ProviderLocalInternetRadio:
			name = constants.ProviderLocalInternetRadio
		case constants.ProviderTunein:
			name = constants.ProviderTunein
		case constants.ProviderRadioBrowser:
			name = constants.ProviderRadioBrowser
		case constants.ProviderAux:
			name = constants.ProviderAux
		}
	}
	// FINAL fallback: name should not be empty if possible
	if name == "" {
		if s.ID != "" {
			name = s.ID
		} else if s.SourceProviderID != "" {
			name = s.SourceProviderID
		}
	}

	return name
}

func mapToFullResponseCredential(s models.ConfiguredSource, fullSource *models.FullResponseSource) {
	if s.Credential.Value != "" {
		fullSource.Credential.Value = s.Credential.Value
		fullSource.Credential.Type = s.Credential.Type
	}

	if fullSource.Credential.Value == "" && s.Secret != "" {
		fullSource.Credential.Value = s.Secret
		fullSource.Credential.Type = s.SecretType
	}

	if fullSource.Credential.Type == "" || fullSource.Credential.Type == constants.CredentialTypeToken {
		if s.Type == constants.ProviderSpotify || s.SourceProviderID == constants.ProviderSpotify || s.SourceKeyType == constants.ProviderSpotify {
			fullSource.Credential.Type = constants.CredentialTypeTokenV3
		} else if fullSource.Credential.Type == "" {
			fullSource.Credential.Type = constants.CredentialTypeToken
		}
	}
}

func mapToFullResponseSource(s models.ConfiguredSource) models.FullResponseSource {
	fullSource := models.FullResponseSource{
		ID:               s.ID,
		Type:             s.Type,
		DisplayName:      s.DisplayName,
		CreatedOn:        s.CreatedOn,
		Name:             resolveSourceName(s),
		SourceProviderID: s.SourceProviderID,
		SourceName:       s.SourceName,
		SourceSettings:   "",
		UpdatedOn:        s.UpdatedOn,
		Username:         s.Username,
	}

	mapToFullResponseCredential(s, &fullSource)

	if s.SourceKeyType == constants.ProviderTunein {
		fullSource.SourceName = ""
	}

	if fullSource.Username == "" && s.SourceKeyType != constants.ProviderTunein && s.SourceKeyType != constants.ProviderInternetRadio && s.SourceKeyType != constants.ProviderLocalInternetRadio {
		fullSource.Username = s.SourceKeyAccount
	}

	// SourceProviderID is a required protobuf field inside recents/preset
	// source blocks. A persisted source that lost its SourceKey.Type (e.g.
	// poisoned by an older "INVALID" classification) lands here with an
	// empty value, so fall back to the canonical default whose ID matches.
	if fullSource.SourceProviderID == "" && s.ID != "" {
		if def := canonicalProviderIDByID(s.ID); def != "" {
			fullSource.SourceProviderID = def
		}
	}

	return fullSource
}

// canonicalProviderIDByID returns the canonical SourceProviderID for one of
// the well-known built-in source IDs (10001..10005), or "" if the ID isn't
// recognised.
func canonicalProviderIDByID(id string) string {
	switch id {
	case "10002":
		return strconv.Itoa(constants.InternetRadioProviderID)
	case "10003":
		return strconv.Itoa(constants.LocalInternetRadioProviderID)
	case "10004":
		return strconv.Itoa(constants.TuneinProviderID)
	case "10005":
		return strconv.Itoa(constants.RadioBrowserProviderID)
	}

	return ""
}

// inferSourceKeyTypeFromLocation makes a best-effort guess at the source
// type from a preset/recent's location URL. Used *only* for diagnostic
// logs — never to decide which source to bind to. URL-pattern matching
// is fragile (a location format can be reused across providers; tests
// can't enumerate every shape Bose's firmware ever emits), so the
// inference is one-way visibility: when it disagrees with the actually-
// bound source, surface a log line so an operator can investigate.
// When it can't make a confident inference, returns "" and the caller
// silently accepts the bound source as-is.
func inferSourceKeyTypeFromLocation(location string) string {
	switch {
	case location == "":
		return ""
	case strings.HasPrefix(location, "/v1/playback/station/s"),
		strings.HasPrefix(location, "/v1/playback/episodes/t"):
		return constants.ProviderTunein
	case strings.HasPrefix(location, "/playback/container/"),
		strings.HasPrefix(location, "/playback/track/"):
		return constants.ProviderSpotify
	case strings.Contains(location, "/custom/v1/playback/"):
		return constants.ProviderLocalInternetRadio
	}

	return ""
}

// canonicalSourceKeyTypeByProviderID is the inverse of the canonical
// sourceproviderid mapping used in /full responses (25 = TuneIn, 11 =
// LocalInternetRadio, …). Used by syncPresets/syncRecents to project the
// upstream cloud /full perspective ("Audio" + providerid=25) back onto
// the speaker's perspective ("TUNEIN") at persist time, so the on-disk
// ServicePreset/ServiceRecent.Source matches what the speaker itself
// would write via setup.syncPresets (which is the source of truth).
//
// Returns "" for unknown / non-canonical provider IDs; the caller is
// expected to fall back to whatever upstream gave us.
func canonicalSourceKeyTypeByProviderID(providerID string) string {
	for i := range constants.StaticProviders {
		if strconv.Itoa(constants.StaticProviders[i].ID) == providerID {
			return constants.StaticProviders[i].Name
		}
	}

	return ""
}

// canonicalDefaultsByType returns the canonical (built-in) source ID and
// SourceProviderID for a well-known provider key type. Used to synthesise a
// minimum-viable <source> block when a preset references a source we no
// longer have in the configured-sources list (e.g. after a factory reset
// that hasn't repopulated the device's sources yet).
//
// Returns ("", "") for provider types whose IDs are account-specific
// (Spotify, Amazon) — those cannot be synthesised without losing
// per-account state, so callers must skip the preset instead.
func canonicalDefaultsByType(sourceKeyType string) (id, providerID string) {
	switch sourceKeyType {
	case constants.ProviderInternetRadio:
		return "10002", strconv.Itoa(constants.InternetRadioProviderID)
	case constants.ProviderLocalInternetRadio:
		return "10003", strconv.Itoa(constants.LocalInternetRadioProviderID)
	case constants.ProviderTunein:
		return "10004", strconv.Itoa(constants.TuneinProviderID)
	case constants.ProviderRadioBrowser:
		return "10005", strconv.Itoa(constants.RadioBrowserProviderID)
	}

	return "", ""
}

func mapPresetsToFullResponse(presets []models.ServicePreset, sources []models.ConfiguredSource) []models.FullResponsePreset {
	var fullPresets []models.FullResponsePreset

	for i := range presets {
		p := &presets[i]

		if p.CreatedOn == "" {
			p.CreatedOn = constants.DateStr
		}

		if p.UpdatedOn == "" {
			p.UpdatedOn = constants.DateStr
		}

		var matchedSource *models.ConfiguredSource
		// 1. Exact ID match — but only if the source's type is
		// compatible with what the preset claims. GH-343: a numeric
		// SourceID collision between e.g. a TuneIn preset and a
		// RADIOPLAYER source in the configured-sources list used to
		// silently rewrite the preset's source attribute to
		// RADIOPLAYER on the next /full sync. Strict-match refuses
		// the bind when the types disagree; the fallback below then
		// finds the right source (or synthesise/skip kicks in).
		if p.SourceID != "" {
			for j := range sources {
				if sources[j].ID == p.SourceID && sourceTypeCompatible(p.Source, sources[j].SourceKeyType) {
					copySource := sources[j]
					PrepareConfiguredSource(&copySource)
					matchedSource = &copySource

					break
				}

				if sources[j].ID == p.SourceID && !sourceTypeCompatible(p.Source, sources[j].SourceKeyType) {
					log.Printf("[Marge] /full: refusing cross-type bind for preset %s — preset.Source=%q but configured source id=%s has type %q; falling through to type-based match",
						sanitizeLog(p.ButtonNumber), sanitizeLog(p.Source), sanitizeLog(sources[j].ID), sanitizeLog(sources[j].SourceKeyType))
				}
			}
		}

		// 2. Fallback to type/account match if ID didn't match or was empty
		if matchedSource == nil {
			for j := range sources {
				s := sources[j]
				if s.SourceKeyType == p.Source && (p.SourceAccount == "" || s.SourceKeyAccount == p.SourceAccount) {
					copySource := s
					PrepareConfiguredSource(&copySource)
					matchedSource = &copySource

					break
				}
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
		if fullPreset.Username == "" {
			fullPreset.Username = p.Name
		}

		if fullPreset.ContentItemType == "" && p.Type != "" {
			fullPreset.ContentItemType = p.Type
		}

		if matchedSource != nil {
			fullPreset.Source = mapToFullResponseSource(*matchedSource)
			fullPresets = append(fullPresets, fullPreset)

			continue
		}

		// No configured source matches this preset. Emitting the preset
		// with an empty <source/> block produces a /full response whose
		// inner protobuf is missing required fields (sourceproviderid,
		// id, type, credential), which makes the speaker abort the whole
		// account sync and wipe its local presets. See GH-269.
		//
		// For the well-known built-in providers we can synthesise a
		// minimum-viable source block from canonical defaults; the
		// credential will be empty so play-time will fail loudly, but
		// the sync survives and other presets stay intact. For
		// account-bound providers (Spotify, Amazon) we can't synthesise
		// without losing state, so we skip the preset entirely — its
		// slot reverts to "Select a preset" until the source is
		// repopulated, which is recoverable; a wiped /presets is not.
		if synth, ok := synthesiseDefaultSourceForPreset(p); ok {
			log.Printf("[Marge] /full: synthesising canonical %s source (id=%s, providerid=%s) for preset %s — original source %q not in configured sources; preset will appear on the speaker but play-time may fail until the source is repopulated",
				sanitizeLog(p.Source), sanitizeLog(synth.ID), sanitizeLog(synth.SourceProviderID), sanitizeLog(p.ButtonNumber), sanitizeLog(p.SourceID))
			fullPreset.Source = synth
			fullPresets = append(fullPresets, fullPreset)

			continue
		}

		log.Printf("[Marge] /full: skipping preset %s — source %q (id=%q, account=%q) not in configured sources and not synthesisable; the speaker's preset slot will revert to empty until the source is repopulated",
			sanitizeLog(p.ButtonNumber), sanitizeLog(p.Source), sanitizeLog(p.SourceID), sanitizeLog(p.SourceAccount))
	}

	return fullPresets
}

// synthesiseDefaultSourceForPreset builds a minimum-viable FullResponseSource
// for a preset whose source is no longer in the configured-sources list,
// using canonical built-in defaults. Returns (zero, false) when the preset's
// provider type isn't one we can safely synthesise (e.g. Spotify, Amazon,
// where the ID and credential are account-bound).
func synthesiseDefaultSourceForPreset(p *models.ServicePreset) (models.FullResponseSource, bool) {
	return synthesiseDefaultSourceByType(p.Source)
}

// synthesiseDefaultSourceForRecent is the recent-side twin of
// synthesiseDefaultSourceForPreset. Same protobuf-required-field invariants
// apply to recents>recent>source.
func synthesiseDefaultSourceForRecent(r *models.ServiceRecent) (models.FullResponseSource, bool) {
	return synthesiseDefaultSourceByType(r.Source)
}

// synthesiseDefaultSourceByType is the shared helper behind the preset/recent
// synthesisers. Keep them as separate wrappers so callers stay readable and
// the call site signals which container we're patching up.
func synthesiseDefaultSourceByType(sourceKeyType string) (models.FullResponseSource, bool) {
	id, providerID := canonicalDefaultsByType(sourceKeyType)
	if id == "" || providerID == "" {
		return models.FullResponseSource{}, false
	}

	synth := models.FullResponseSource{
		ID:               id,
		Type:             "Audio",
		CreatedOn:        constants.DateStr,
		UpdatedOn:        constants.DateStr,
		SourceProviderID: providerID,
		Name:             sourceKeyType,
	}
	synth.Credential.Type = constants.CredentialTypeToken

	return synth, true
}

// findMatchingSourceForRecent resolves the configured source a recent
// references. Tries exact SourceID match (but only if the matched source's
// type is compatible with the recent's claimed Source — see GH-343), then
// SourceKeyType+account. Returns nil when nothing matches; caller falls
// back to synthesise/skip.
func findMatchingSourceForRecent(r *models.ServiceRecent, sources []models.ConfiguredSource) *models.ConfiguredSource {
	if r.SourceID != "" {
		for j := range sources {
			if sources[j].ID == r.SourceID && sourceTypeCompatible(r.Source, sources[j].SourceKeyType) {
				copySource := sources[j]
				PrepareConfiguredSource(&copySource)

				return &copySource
			}

			if sources[j].ID == r.SourceID && !sourceTypeCompatible(r.Source, sources[j].SourceKeyType) {
				log.Printf("[Marge] /full: refusing cross-type bind for recent id=%s — recent.Source=%q but configured source id=%s has type %q; falling through to type-based match",
					sanitizeLog(r.ID), sanitizeLog(r.Source), sanitizeLog(sources[j].ID), sanitizeLog(sources[j].SourceKeyType))
			}
		}
	}

	for j := range sources {
		s := sources[j]
		if s.SourceKeyType == r.Source && (r.SourceAccount == "" || s.SourceKeyAccount == r.SourceAccount) {
			copySource := s
			PrepareConfiguredSource(&copySource)

			return &copySource
		}
	}

	return nil
}

// sourceTypeCompatible decides whether a preset/recent whose original
// source is `claimed` can be bound to a configured source of type
// `configured`. Strict-match policy: identical types compatible; either
// side empty is treated as "no info, allow" (matches pre-fix behaviour for
// records that lost their Source attribute). Refuse when both are set and
// disagree — that's the GH-343 cross-type rewrite the speaker reflects.
func sourceTypeCompatible(claimed, configured string) bool {
	if claimed == "" || configured == "" {
		return true
	}

	return claimed == configured
}

func mapRecentsToFullResponse(recents []models.ServiceRecent, sources []models.ConfiguredSource) []models.FullResponseRecent {
	var fullRecents []models.FullResponseRecent

	for i := range recents {
		r := &recents[i]
		if r.CreatedOn == "" {
			r.CreatedOn = constants.DateStr
		}

		if r.UpdatedOn == "" {
			r.UpdatedOn = constants.DateStr
		}

		matchedSource := findMatchingSourceForRecent(r, sources)

		fullRecent := models.FullResponseRecent{
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
		if fullRecent.LastPlayedAt == "" && r.UtcTime != "" {
			if ut, err := strconv.ParseInt(r.UtcTime, 10, 64); err == nil {
				fullRecent.LastPlayedAt = time.Unix(ut, 0).UTC().Format("2006-01-02T15:04:05.000+00:00")
			}
		}

		if fullRecent.Username == "" {
			fullRecent.Username = r.Name
		}

		if fullRecent.ContentItemType == "" && r.Type != "" {
			fullRecent.ContentItemType = r.Type
		}

		if matchedSource != nil {
			fullRecent.Source = mapToFullResponseSource(*matchedSource)
			if fullRecent.SourceID == "" {
				fullRecent.SourceID = fullRecent.Source.ID
			}

			fullRecents = append(fullRecents, fullRecent)

			continue
		}

		// Same protobuf-required-field hazard as presets: emitting a
		// recent with an empty <source/> block makes the speaker abort
		// the whole /full sync. Mirror the preset behaviour — synthesise
		// for well-known radio providers, skip otherwise. Recents are
		// less load-bearing than presets, but a single broken recent can
		// still take the sync down.
		if synth, ok := synthesiseDefaultSourceForRecent(r); ok {
			log.Printf("[Marge] /full: synthesising canonical %s source (id=%s, providerid=%s) for recent id=%s — original source %q not in configured sources",
				sanitizeLog(r.Source), sanitizeLog(synth.ID), sanitizeLog(synth.SourceProviderID), sanitizeLog(r.ID), sanitizeLog(r.SourceID))

			fullRecent.Source = synth
			if fullRecent.SourceID == "" {
				fullRecent.SourceID = synth.ID
			}

			fullRecents = append(fullRecents, fullRecent)

			continue
		}

		log.Printf("[Marge] /full: skipping recent id=%s — source %q (id=%q, account=%q) not in configured sources and not synthesisable",
			sanitizeLog(r.ID), sanitizeLog(r.Source), sanitizeLog(r.SourceID), sanitizeLog(r.SourceAccount))
	}

	return fullRecents
}

func fillDefaultProviderSettings(account string, resp *models.AccountFullResponse) {
	for _, p := range constants.StaticProviders {
		switch p.Name {
		case constants.ProviderDeezer:
			resp.ProviderSettings = append(resp.ProviderSettings, models.ProviderSetting{
				BoseID:     account,
				KeyName:    "ELIGIBLE_FOR_TRIAL",
				Value:      "false",
				ProviderID: strconv.Itoa(p.ID),
			})
		case constants.ProviderSpotify:
			resp.ProviderSettings = append(resp.ProviderSettings, models.ProviderSetting{
				BoseID:     account,
				KeyName:    "STREAMING_QUALITY",
				Value:      "2",
				ProviderID: strconv.Itoa(p.ID),
			})
		}
	}
}

func fillAccountInfo(ds *datastore.DataStore, account string, resp *models.AccountFullResponse) {
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
}

func getAccountDevices(ds *datastore.DataStore, account string, entries []os.DirEntry) ([]models.AccountDevice, string) {
	var (
		devices      []models.AccountDevice
		lastDeviceID string
	)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		deviceID := entry.Name()
		lastDeviceID = deviceID

		dev, err := CreateAccountDevice(ds, account, deviceID)
		if err != nil {
			continue
		}

		if dev.Name == "" || dev.Name == " " {
			if deviceID != "" {
				dev.Name = deviceID
			} else {
				continue
			}
		}

		devices = append(devices, dev)
	}

	return devices, lastDeviceID
}

func getAccountSources(ds *datastore.DataStore, account, lastDeviceID string) []models.FullResponseSource {
	var (
		sources []models.ConfiguredSource
		err     error
	)

	if lastDeviceID != "" {
		sources, err = ds.GetConfiguredSources(account, lastDeviceID)
		if err == nil {
			sources = mergeDefaultSources(sources, ds.GetInitialSources())
		}
	} else {
		sources = ds.GetInitialSources()
	}

	if err != nil {
		return nil
	}

	var fullSources []models.FullResponseSource

	for i := range sources {
		s := sources[i]
		// Real Bose's /streaming/account/{a}/full never emitted AUX as
		// a cloud-side <source> (verified across 61 captured upstream
		// /full responses in scripts/android/captures/.../
		// parity_mismatches/). AUX is hardware-local — the speaker
		// enumerates it via isLocal=true in its own /sources response,
		// it doesn't need the cloud to list it. AfterTouch emitting a
		// malformed AUX entry here (with displayName=, empty
		// <credential>, non-empty <name>/<username>) is the suspected
		// trigger for issue #195: the speaker's source-reconciliation
		// code marks AUX as cloud-side inconsistent and refuses
		// dispatch, even though the local availability check reports
		// it READY. We still keep AUX in getDefaultSources() because
		// other call sites (default-sources init at startup, the
		// SoundTouch web UI source picker) rely on it; the filter
		// just keeps it out of /full's wire shape.
		if s.SourceKeyType == constants.ProviderAux {
			continue
		}

		PrepareConfiguredSource(&s)

		fs := mapToFullResponseSource(s)
		if fs.SourceProviderID == "" {
			log.Printf("[Marge] /full: omitting source name=%q (sourceKey type %q) — no resolvable sourceproviderid; would be rejected as INVALID_SOURCE",
				sanitizeLog(fs.Name), sanitizeLog(s.SourceKeyType))

			continue
		}

		fullSources = append(fullSources, fs)
	}

	return fullSources
}

// mergeDefaultSources returns sources in canonical order: defaults first (using
// the stored entry's credentials when a match is found), followed by any stored
// sources that have no matching default (e.g. Spotify, Amazon). This ensures
// that default sources always appear in a predictable order regardless of what
// is present in the device's on-disk Sources.xml.
// It does not persist — initializeDefaultSources handles persistence at startup.
func mergeDefaultSources(stored, defaults []models.ConfiguredSource) []models.ConfiguredSource {
	result := make([]models.ConfiguredSource, 0, len(stored)+len(defaults))

	for i := range defaults {
		found := -1

		for j := range stored {
			if stored[j].SourceKeyType == defaults[i].SourceKeyType {
				found = j
				break
			}
		}

		if found >= 0 {
			result = append(result, stored[found])
		} else {
			result = append(result, defaults[i])
		}
	}

	for i := range stored {
		isDefault := false

		for j := range defaults {
			if stored[i].SourceKeyType == defaults[j].SourceKeyType {
				isDefault = true
				break
			}
		}

		if !isDefault {
			result = append(result, stored[i])
		}
	}

	return result
}

// AccountSourcesToXML generates the account sources XML.
func AccountSourcesToXML(ds *datastore.DataStore, account string) ([]byte, error) {
	devicesDir := ds.AccountDevicesDir(account)

	entries, err := ds.ReadDirUnderBase(devicesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	_, lastDeviceID := getAccountDevices(ds, account, entries)
	resp := models.AccountSourcesResponse{
		Sources: getAccountSources(ds, account, lastDeviceID),
	}

	data, err := xml.Marshal(resp)
	if err != nil {
		return nil, err
	}

	// Parity: use self-closing tags and handle empty sourceproviderid
	data = bytes.ReplaceAll(data, []byte("<sourceSettings></sourceSettings>"), []byte("<sourceSettings/>"))
	data = bytes.ReplaceAll(data, []byte("<sourceproviderid></sourceproviderid>"), []byte(""))

	return append([]byte(constants.XMLHeader), data...), nil
}

// AccountDevicesToXML generates the account devices XML.
func AccountDevicesToXML(ds *datastore.DataStore, account string) ([]byte, error) {
	devicesDir := ds.AccountDevicesDir(account)

	entries, err := ds.ReadDirUnderBase(devicesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	devices, _ := getAccountDevices(ds, account, entries)

	var margeDevices []models.MargeAccountDevice

	for i := range devices {
		d := &devices[i]
		margeDevices = append(margeDevices, models.MargeAccountDevice{
			DeviceID:        d.DeviceID,
			AttachedProduct: d.AttachedProduct,
			CreatedOn:       d.CreatedOn,
			IPAddress:       d.IPAddress,
			Name:            d.Name,
			UpdatedOn:       d.UpdatedOn,
		})
	}

	resp := models.AccountDevicesResponse{
		Devices: margeDevices,
	}

	// Fill provider settings
	fullResp := &models.AccountFullResponse{}
	fillDefaultProviderSettings(account, fullResp)
	fillAccountInfo(ds, account, fullResp)
	resp.ProviderSettings = fullResp.ProviderSettings

	data, err := xml.Marshal(resp)
	if err != nil {
		return nil, err
	}

	// Parity: use self-closing tags for empty components
	data = bytes.ReplaceAll(data, []byte("<components></components>"), []byte("<components/>"))

	return append([]byte(constants.XMLHeader), data...), nil
}

// AccountFullToXML generates a complete account XML with devices, presets, and recents.
func AccountFullToXML(ds *datastore.DataStore, account string) ([]byte, error) {
	devicesDir := ds.AccountDevicesDir(account)

	resp := models.AccountFullResponse{
		ID:                account,
		AccountStatus:     "OK",
		Mode:              "global",
		PreferredLanguage: "en",
	}

	fillDefaultProviderSettings(account, &resp)
	fillAccountInfo(ds, account, &resp)

	entries, err := ds.ReadDirUnderBase(devicesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	devices, lastDeviceID := getAccountDevices(ds, account, entries)
	resp.Devices = devices
	resp.Sources = getAccountSources(ds, account, lastDeviceID)

	data, err := xml.Marshal(resp)
	if err != nil {
		return nil, err
	}

	// Parity: use self-closing tags for empty components and sourceSettings.
	// NOTE: do NOT strip empty <sourceproviderid> elements here — the speaker
	// decodes /full into a protobuf message where recents>recent>source>
	// sourceproviderid is a *required* field, so removing even an empty element
	// trips "missing required field" and aborts the whole account sync.
	data = bytes.ReplaceAll(data, []byte("<components></components>"), []byte("<components/>"))
	data = bytes.ReplaceAll(data, []byte("<sourceSettings></sourceSettings>"), []byte("<sourceSettings/>"))

	return append([]byte(constants.XMLHeader), data...), nil
}

// RemovePreset clears a preset for the specified account and device.
func RemovePreset(ds *datastore.DataStore, account, device string, presetNumber int) error {
	presets, err := ds.GetPresets(account, device)
	if err != nil {
		return err
	}

	if presetNumber < 1 || presetNumber > len(presets) {
		// Preset doesn't exist or index out of range, nothing to do
		return nil
	}

	presets[presetNumber-1] = models.ServicePreset{}

	return ds.SavePresets(account, device, presets)
}

// resolvePresetSource resolves the source a preset PUT is referencing,
// auto-adding a canonical built-in source when AfterTouch's per-device
// configured-sources list doesn't include it yet (GH-314 / GH-253). It
// returns the matched source plus the possibly-extended sources slice
// (since auto-add appends). Returns (nil, sources) when no match could be
// resolved — UpdatePreset turns that into a 500 with a diagnostic log line.
func resolvePresetSource(ds *datastore.DataStore, account, device string, sources []models.ConfiguredSource, sourceID string, presetNumber int) (*models.ConfiguredSource, []models.ConfiguredSource) {
	for i := range sources {
		if sources[i].ID == sourceID {
			return &sources[i], sources
		}
	}

	// Fallback: SourceID is the symbolic provider name (the speaker
	// sometimes sends e.g. <sourceid>TUNEIN</sourceid> instead of a
	// numeric ID); match by SourceKeyType.
	if sourceID == constants.ProviderInternetRadio || sourceID == constants.ProviderTunein || sourceID == constants.ProviderSpotify || sourceID == constants.ProviderAmazon {
		for i := range sources {
			if sources[i].SourceKeyType == sourceID {
				return &sources[i], sources
			}
		}
	}

	// Auto-add a canonical built-in source the speaker referenced but
	// AfterTouch hasn't been told about (post-factory-reset state). For
	// account-bound sources (Spotify, Amazon) we can't synthesise
	// credentials, so the caller will reject the PUT instead.
	if canonical, ok := ds.CanonicalSourceByID(sourceID); ok {
		log.Printf("[Marge] UpdatePreset(preset=%d): auto-adding canonical source id=%s type=%s providerid=%s — speaker referenced a built-in source not yet in AfterTouch's configured-sources list; saving so the preset can land",
			presetNumber, sanitizeLog(canonical.ID), sanitizeLog(canonical.SourceKeyType), sanitizeLog(canonical.SourceProviderID))

		sources = append(sources, canonical)
		if saveErr := ds.SaveConfiguredSources(account, device, sources); saveErr != nil {
			log.Printf("[Marge] UpdatePreset(preset=%d): SaveConfiguredSources after auto-add failed: %s — the preset will land but the source may not survive a service restart",
				presetNumber, sanitizeErr(saveErr))
		}

		return &sources[len(sources)-1], sources
	}

	return nil, sources
}

// UpdatePreset updates or creates a preset for the specified account and device.
func UpdatePreset(ds *datastore.DataStore, account, device string, presetNumber int, sourceXML []byte) ([]byte, error) {
	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		return nil, err
	}

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		presets = []models.ServicePreset{}
	}

	var newPresetElem struct {
		Name            string `xml:"name"`
		Username        string `xml:"username"`
		SourceID        string `xml:"sourceid"`
		Location        string `xml:"location"`
		ContentItemType string `xml:"contentItemType"`
		ContainerArt    string `xml:"containerArt"`
	}
	if err = xml.Unmarshal(sourceXML, &newPresetElem); err != nil {
		return nil, err
	}

	// The PUT body field carrying the human-readable name varies by client:
	// the speaker firmware sends it as <name>, the Stockholm mobile app sends
	// it as <username>. soundcork documents this divergence too. Prefer
	// <name>; fall back to <username> so app-initiated stores don't end up
	// with an empty preset name.
	if newPresetElem.Name == "" && newPresetElem.Username != "" {
		log.Printf("[Marge] UpdatePreset(preset=%d): using Stockholm <username> as preset name; speaker firmware would have sent <name>", presetNumber)

		newPresetElem.Name = newPresetElem.Username
	}

	matchingSrc, sources := resolvePresetSource(ds, account, device, sources, newPresetElem.SourceID, presetNumber)
	if matchingSrc == nil {
		log.Printf("[Marge] UpdatePreset(preset=%d): rejecting — source id=%q is neither in configured sources nor a canonical built-in we can auto-add. The speaker's long-press will appear to succeed locally but the preset will not survive a /full sync",
			presetNumber, sanitizeLog(newPresetElem.SourceID))

		return nil, fmt.Errorf("invalid account/source")
	}

	// Visibility-only: the speaker's PUT carries only <sourceid> — no
	// symbolic source name — so we can't strict-match at write time.
	// But if the preset's location URL has a recognisable pattern
	// (TuneIn /v1/playback/station/, Spotify /playback/container/,
	// LocalInternetRadio /custom/v1/playback/, …) and that pattern
	// disagrees with the matched source's SourceKeyType, the operator
	// is probably looking at the GH-343 footprint: a stale Sources.xml
	// entry shadowing the speaker's actual intent. Log so the operator
	// can re-trigger setup.syncSources without us guessing on their
	// behalf — the binding still happens as the speaker requested.
	if inferred := inferSourceKeyTypeFromLocation(newPresetElem.Location); inferred != "" && matchingSrc.SourceKeyType != "" && inferred != matchingSrc.SourceKeyType {
		log.Printf("[Marge] UpdatePreset(preset=%d): location URL %q suggests %s but matched source id=%s has SourceKeyType=%s — proceeding as the speaker requested, but the Sources.xml entry may be stale; consider re-running setup.syncSources from the speaker to refresh",
			presetNumber, sanitizeLog(newPresetElem.Location), sanitizeLog(inferred), sanitizeLog(matchingSrc.ID), sanitizeLog(matchingSrc.SourceKeyType))
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
		Username:     newPresetElem.Name,
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
	syncMatchingSource(matchingSrc, recentInput{
		Source: struct {
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
		}{
			ID:         matchingSrc.ID,
			SourceName: newPresetElem.Name,
		},
	})
	presetObj.SourceID = matchingSrc.ID
	presetObj.Username = newPresetElem.Name

	// Parity: return the single preset
	px := mapPresetToParityXML(presetObj, sources)

	data, err := xml.Marshal(px)
	if err != nil {
		return nil, err
	}

	return append([]byte(constants.XMLHeader), data...), nil
}

type recentInput struct {
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

func getSourceNameFromXML(sourceXML []byte, input recentInput) string {
	sourceName := input.Source.SourceName
	if sourceName == "" {
		// Some clients might send sourcename as a direct child of recent
		var altRecentElem struct {
			SourceName string `xml:"sourcename"`
		}

		_ = xml.Unmarshal(sourceXML, &altRecentElem)
		sourceName = altRecentElem.SourceName
	}

	return sourceName
}

func syncMatchingSource(matchingSrc *models.ConfiguredSource, input recentInput) {
	if matchingSrc == nil {
		return
	}

	if input.Source.ID != "" {
		matchingSrc.ID = input.Source.ID
	}

	if input.Source.Type != "" {
		matchingSrc.Type = input.Source.Type
	}

	// Ensure we use the latest secret from the input if it was just learned/updated
	if input.Source.Credential.Value != "" {
		matchingSrc.Secret = input.Source.Credential.Value
		matchingSrc.SecretType = input.Source.Credential.Type
	}

	if input.Source.CreatedOn != "" {
		matchingSrc.CreatedOn = input.Source.CreatedOn
	}

	if input.Source.UpdatedOn != "" {
		matchingSrc.UpdatedOn = input.Source.UpdatedOn
	}

	if matchingSrc.ID == "" {
		matchingSrc.ID = input.SourceID
	}

	if matchingSrc.SourceName == "" && matchingSrc.DisplayName != "" {
		// Parity: for some services like TuneIn, sourcename should be empty
		if !strings.EqualFold(matchingSrc.DisplayName, constants.ProviderTunein) && matchingSrc.DisplayName != "Other" {
			matchingSrc.SourceName = matchingSrc.DisplayName
		}
	}

	if matchingSrc.DisplayName == "" && matchingSrc.SourceName != "" {
		matchingSrc.DisplayName = matchingSrc.SourceName
	}

	if matchingSrc.Username == "" && matchingSrc.DisplayName != "" {
		// Parity: for some services like TuneIn, username should be empty
		if !strings.EqualFold(matchingSrc.DisplayName, constants.ProviderTunein) && matchingSrc.DisplayName != "Other" {
			matchingSrc.Username = matchingSrc.DisplayName
		}
	}
}

// AddRecent adds or updates a recent item for the specified account and device.
func AddRecent(ds *datastore.DataStore, account, device string, sourceXML []byte) ([]byte, error) {
	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		return nil, err
	}

	recents, err := ds.GetRecents(account, device)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	var input recentInput
	if err := xml.Unmarshal(sourceXML, &input); err != nil {
		return nil, err
	}

	sourceName := getSourceNameFromXML(sourceXML, input)

	// 1. learnSource handles persistence AND returns a matching source.
	matchingSrc, learned := learnSource(ds, account, device, sources, input.SourceID, input.Location, sourceName, input.Source.Credential.Value, input.Source.SourceProviderID, input.Source.CreatedOn, input.Source.UpdatedOn)

	// Parity: ensure generic tokens for TUNEIN and LOCAL_INTERNET_RADIO if missing.
	// This covers both learned and already existing sources.
	if (matchingSrc.SourceProviderID == strconv.Itoa(constants.TuneinProviderID) || matchingSrc.ID == constants.ProviderTunein || strings.Contains(input.Location, "/v1/playback/station/")) && matchingSrc.Secret == "" {
		matchingSrc.Secret = datastore.GenerateSerialSecret(strings.ToLower(constants.ProviderTunein))
		matchingSrc.SecretType = constants.CredentialTypeToken
	} else if matchingSrc.ID == constants.ProviderLocalInternetRadio && matchingSrc.Secret == "" {
		matchingSrc.Secret = datastore.GenerateSerialSecret("local-internet-radio")
		matchingSrc.SecretType = constants.CredentialTypeToken
	}

	if learned {
		// Re-fetch sources to ensure we have the newly learned one in the slice
		if updatedSources, err := ds.GetConfiguredSources(account, device); err == nil {
			sources = updatedSources
			_ = sources // avoid ineffectual assignment
		}
	}

	if matchingSrc == nil {
		// This should technically not happen as learnSource always returns something,
		// but we keep it as a fallback for safety.
		matchingSrc = &models.ConfiguredSource{
			ID:               input.SourceID,
			SourceProviderID: input.Source.SourceProviderID,
			Secret:           input.Source.Credential.Value,
			SecretType:       input.Source.Credential.Type,
			CreatedOn:        input.Source.CreatedOn,
			UpdatedOn:        input.Source.UpdatedOn,
		}
	}

	syncMatchingSource(matchingSrc, input)

	utcTime := parseLastPlayedAt(input.LastPlayedAt)
	recentObj, recents := updateOrCreateRecent(recents, input.Name, matchingSrc, input.ContentItemType, input.Location, device, utcTime)

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
		sourceLearned = updateSourceFields(matchingSrc, credentialValue, sourceName, sourceProviderID, createdOn, updatedOn)
	}

	if sourceLearned {
		// Ensure generic tokens for TUNEIN and LOCAL_INTERNET_RADIO if missing
		if (matchingSrc.SourceProviderID == strconv.Itoa(constants.TuneinProviderID) || matchingSrc.ID == constants.ProviderTunein || strings.Contains(location, "/v1/playback/station/")) && matchingSrc.Secret == "" {
			matchingSrc.Secret = datastore.GenerateSerialSecret(strings.ToLower(constants.ProviderTunein))
			matchingSrc.SecretType = constants.CredentialTypeToken
		} else if matchingSrc.ID == constants.ProviderLocalInternetRadio && matchingSrc.Secret == "" {
			matchingSrc.Secret = datastore.GenerateSerialSecret("local-internet-radio")
			matchingSrc.SecretType = constants.CredentialTypeToken
		}

		persistLearnedSource(ds, account, device, sources, matchingSrc)
	}

	return matchingSrc, sourceLearned
}

func createLearnedSource(sourceID, location, sourceName, credentialValue, sourceProviderID, createdOn, updatedOn string) *models.ConfiguredSource {
	displayName := sourceName
	if displayName == "" && sourceID != "" {
		switch sourceID {
		case constants.ProviderSpotify:
			displayName = constants.ProviderSpotify
		case constants.ProviderAmazon:
			displayName = "Amazon Music"
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

	classifyLearnedSource(src, sourceID, location, sourceProviderID)

	return src
}

func classifyLearnedSource(src *models.ConfiguredSource, sourceID, location, sourceProviderID string) {
	switch {
	case sourceProviderID == strconv.Itoa(constants.TuneinProviderID) || sourceID == constants.ProviderTunein || strings.Contains(location, "/v1/playback/station/"):
		classifyAsTuneIn(src)
	case sourceProviderID == strconv.Itoa(constants.LocalInternetRadioProviderID) || sourceID == constants.ProviderLocalInternetRadio || strings.Contains(location, "/custom/v1/playback/"):
		classifyAsLocalInternetRadio(src)
	case strings.Contains(location, "spotify") || strings.Contains(location, "c3BvdGlme") || sourceID == constants.ProviderSpotify:
		classifyAsSpotify(src)
	case strings.Contains(location, "amazon") || sourceID == constants.ProviderAmazon || sourceProviderID == strconv.Itoa(constants.AmazonProviderID):
		classifyAsAmazon(src)
	case sourceProviderID == strconv.Itoa(constants.RadioBrowserProviderID) || strings.Contains(location, "/stations/byuuid/"):
		classifyAsRadioBrowser(src)
	}
	// If we can't classify, leave SourceKey.Type empty so the canonical-by-ID
	// fallback in mapToFullResponseSource and the read-side applyCanonicalDefaults
	// still have a chance to repair it. Writing a literal "INVALID" used to lock
	// the source out of every repair path, producing a <source> block with no
	// <sourceproviderid> and breaking the speaker's protobuf required-field check.
}

func classifyAsTuneIn(src *models.ConfiguredSource) {
	src.SourceKey.Type = constants.ProviderTunein
	src.SourceKeyType = constants.ProviderTunein
	src.Type = "Audio"
	src.SecretType = constants.CredentialTypeToken

	if src.Secret == "" {
		src.Secret = datastore.GenerateSerialSecret(strings.ToLower(constants.ProviderTunein))
	}

	if src.DisplayName == "Other" || src.DisplayName == constants.ProviderTunein || src.DisplayName == "" {
		src.DisplayName = constants.ProviderTunein
	}
}

func classifyAsLocalInternetRadio(src *models.ConfiguredSource) {
	src.SourceKey.Type = constants.ProviderLocalInternetRadio
	src.SourceKeyType = constants.ProviderLocalInternetRadio
	src.Type = "Audio"
	src.SecretType = constants.CredentialTypeToken

	if src.Secret == "" {
		src.Secret = datastore.GenerateSerialSecret("local-internet-radio")
	}

	if src.DisplayName == "Other" || src.DisplayName == "Local Internet Radio" || src.DisplayName == "" {
		src.DisplayName = "Local Internet Radio"
	}
}

func classifyAsSpotify(src *models.ConfiguredSource) {
	src.SourceKey.Type = constants.ProviderSpotify
	src.SourceKeyType = constants.ProviderSpotify
	src.Type = "Audio"
	src.SecretType = constants.CredentialTypeTokenV3

	if src.DisplayName == "Other" {
		src.DisplayName = constants.ProviderSpotify
	}
}

func classifyAsAmazon(src *models.ConfiguredSource) {
	src.SourceKey.Type = constants.ProviderAmazon
	src.SourceKeyType = constants.ProviderAmazon
	src.Type = "Audio"
	src.SecretType = constants.CredentialTypeToken

	if src.DisplayName == "" || src.DisplayName == "Other" {
		src.DisplayName = "Amazon Music"
	}
}

func classifyAsRadioBrowser(src *models.ConfiguredSource) {
	src.SourceKey.Type = constants.ProviderRadioBrowser
	src.SourceKeyType = constants.ProviderRadioBrowser
	src.Type = "Audio"
	src.SecretType = constants.CredentialTypeToken

	if src.Secret == "" {
		src.Secret = datastore.GenerateSerialSecret(strings.ToLower(constants.ProviderRadioBrowser))
	}

	if src.DisplayName == "Other" || src.DisplayName == constants.ProviderRadioBrowser || src.DisplayName == "" {
		src.DisplayName = constants.ProviderRadioBrowser
	}
}

func updateSourceFields(src *models.ConfiguredSource, credentialValue, sourceName, sourceProviderID, createdOn, updatedOn string) bool {
	learned := false

	if credentialValue != "" && (src.Secret == "" || src.Secret != credentialValue) {
		src.Secret = credentialValue
		learned = true
	}

	if sourceName != "" && (src.SourceName == "" || src.SourceName != sourceName) {
		src.SourceName = sourceName
		learned = true
	}

	if sourceProviderID != "" && (src.SourceProviderID == "" || src.SourceProviderID != sourceProviderID) {
		src.SourceProviderID = sourceProviderID
		learned = true
	}

	if createdOn != "" && (src.CreatedOn == "" || src.CreatedOn != createdOn) {
		src.CreatedOn = createdOn
		learned = true
	}

	if updatedOn != "" && (src.UpdatedOn == "" || src.UpdatedOn != updatedOn) {
		src.UpdatedOn = updatedOn
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
		log.Printf("[MARGE_ERR] Failed to persist learned source for %s: %s", sanitizeLog(device), sanitizeErr(err))
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

			// Move the matched recent to the front. Copy the value out FIRST,
			// then rebuild the slice into a fresh backing array. The previous
			// in-place `append(recents[:i], recents[i+1:]...)` shuffle aliased
			// the shared backing array and overwrote index i, which both
			// corrupted the returned pointer (it pointed at the neighbour) and,
			// because Go does not specify evaluation order between `*recentObj`
			// and the inner append, could drop the matched recent and duplicate
			// its neighbour in the saved list.
			matched := recents[i]
			reordered := make([]models.ServiceRecent, 0, len(recents))
			reordered = append(reordered, matched)
			reordered = append(reordered, recents[:i]...)
			reordered = append(reordered, recents[i+1:]...)

			return &reordered[0], reordered
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
	// Create RecentItemParity for the flat web response
	res := models.RecentItemParity{
		ID:              recentObj.ID,
		ContentItemType: recentObj.ContentItemType,
		CreatedOn:       createdOn,
		UpdatedOn:       recentObj.UpdatedOn,
		LastPlayedAt:    time.Unix(utcTime, 0).UTC().Format("2006-01-02T15:04:05.000+00:00"),
		Location:        recentObj.Location,
		Name:            recentObj.Name,
		SourceID:        recentObj.SourceID,
	}

	if res.UpdatedOn == "" {
		res.UpdatedOn = createdOn
	}

	if matchingSrc != nil {
		PrepareConfiguredSource(matchingSrc)
		res.Source = &models.RecentItemParitySource{
			ID:               matchingSrc.ID,
			Type:             matchingSrc.Type,
			CreatedOn:        matchingSrc.CreatedOn,
			UpdatedOn:        matchingSrc.UpdatedOn,
			Name:             matchingSrc.DisplayName,
			SourceProviderID: matchingSrc.SourceProviderID,
			SourceName:       matchingSrc.SourceName,
			Username:         recentSourceUsername(matchingSrc),
		}

		if res.Source.Name == "TuneIn" || res.Source.Name == "LOCAL_INTERNET_RADIO" {
			res.Source.Name = ""
		}

		switch {
		case matchingSrc.Secret != "":
			res.Source.Credential = &models.RecentItemParityCredential{
				Type:  matchingSrc.SecretType,
				Value: matchingSrc.Secret,
			}
		case matchingSrc.Credential.Value != "":
			res.Source.Credential = &models.RecentItemParityCredential{
				Type:  matchingSrc.Credential.Type,
				Value: matchingSrc.Credential.Value,
			}
		default:
			res.Source.Credential = &models.RecentItemParityCredential{
				Type:  "token",
				Value: "",
			}
		}

		if res.Source.Credential.Value == "" && matchingSrc.Secret != "" {
			res.Source.Credential.Value = matchingSrc.Secret
		}

		if res.Source.Credential.Type == "" && matchingSrc.SecretType != "" {
			res.Source.Credential.Type = matchingSrc.SecretType
		}

		if res.Source.SourceName == "" {
			res.Source.SourceName = res.Source.Username
		}
	}

	data, _ := xml.MarshalIndent(res, "", "  ")

	// Parity: use self-closing tags for empty SourceSettings
	data = bytes.ReplaceAll(data, []byte("<sourceSettings></sourceSettings>"), []byte("<sourceSettings/>"))

	header := constants.XMLHeader

	return append([]byte(header+"\n"), data...)
}

// AddDeviceToAccount upserts a device record for the given account.
// Called by both the device-create (POST) and device-rename (PUT)
// handlers — the persistence layer doesn't distinguish; only the
// response status differs.
//
// remoteAddr is the speaker's address as seen by the HTTP server
// (r.RemoteAddr, "host:port"). When the request body doesn't carry
// an `<ipaddress>` and the datastore has no IP for this device yet,
// we fall back to remoteAddr's host portion. An empty remoteAddr
// is treated as "no fallback available" — never errors.
//
// Timestamps:
//   - CreatedOn is preserved from any existing datastore record so a
//     rename doesn't reset the "first paired in 2017" semantics real
//     Bose emits. New devices get CreatedOn = now() at first save.
//   - UpdatedOn is set to now() on every call.
//
// Returns the persisted deviceID and the marge XML response shape
// (`<device deviceid="…"><createdOn/><ipaddress/><name/><updatedOn/></device>`).
func AddDeviceToAccount(ds *datastore.DataStore, account string, sourceXML []byte, remoteAddr string) (string, []byte, error) {
	var newDeviceElem struct {
		DeviceID   string `xml:"deviceid,attr"`
		Name       string `xml:"name"`
		MACAddress string `xml:"macaddress"`
	}
	if err := xml.Unmarshal(sourceXML, &newDeviceElem); err != nil {
		return "", nil, err
	}

	now := FormatTime(time.Now())

	// Build the info to save. Empty fields are filled in by the
	// datastore's mergeWithExistingDeviceInfo (which preserves IP,
	// MAC, CreatedOn, etc.) before the write — so the precedence
	// here is "explicit > merged > remoteAddr fallback".
	info := &models.ServiceDeviceInfo{
		DeviceID:   newDeviceElem.DeviceID,
		Name:       newDeviceElem.Name,
		MacAddress: newDeviceElem.MACAddress,
		UpdatedOn:  now,
	}

	existing, _ := ds.GetDeviceInfo(account, newDeviceElem.DeviceID)

	// CreatedOn: preserve from existing record for renames; set
	// now() only on first registration (no prior record OR the
	// record has no CreatedOn — older AfterTouch installs may
	// have records without one).
	if existing != nil && existing.CreatedOn != "" {
		info.CreatedOn = existing.CreatedOn
	} else {
		info.CreatedOn = now
	}

	// IPAddress: prefer existing record's IP (the speaker may be
	// hitting us through a different network path right now, e.g.
	// SSH port-forward, and the persisted IP is the one other
	// flows like DNS hints care about). Fall back to the inbound
	// connection's remote address only when there's no existing
	// IP to preserve. Invalid remoteAddr leaves info.IPAddress
	// empty, which the merge then handles.
	if existing == nil || existing.IPAddress == "" {
		if remoteAddr != "" {
			if host, _, splitErr := net.SplitHostPort(remoteAddr); splitErr == nil {
				info.IPAddress = host
			}
		}
	}

	if err := ds.SaveDeviceInfo(account, newDeviceElem.DeviceID, info); err != nil {
		return "", nil, err
	}

	// Re-read the persisted record so the response XML reflects
	// the merged state (preserved CreatedOn, preserved IP if the
	// new info had none and the existing record did, etc.).
	persisted, err := ds.GetDeviceInfo(account, newDeviceElem.DeviceID)
	if err != nil {
		return "", nil, fmt.Errorf("re-read persisted device info: %w", err)
	}

	res := fmt.Sprintf(`<device deviceid="%s">`, EscapeXML(persisted.DeviceID))
	res += fmt.Sprintf(`<createdOn>%s</createdOn>`, EscapeXML(persisted.CreatedOn))
	res += fmt.Sprintf(`<ipaddress>%s</ipaddress>`, EscapeXML(persisted.IPAddress))
	res += fmt.Sprintf(`<name>%s</name>`, EscapeXML(persisted.Name))
	res += fmt.Sprintf(`<updatedOn>%s</updatedOn>`, EscapeXML(persisted.UpdatedOn))
	res += `</device>`

	header := constants.XMLHeader

	return persisted.DeviceID, append([]byte(header), []byte(res)...), nil
}

// RemoveDeviceFromAccount removes a device from the specified account.
func RemoveDeviceFromAccount(ds *datastore.DataStore, account, device string) error {
	return ds.RemoveDevice(account, device)
}

// AddSourceToAccount adds a new music source to the account.
// POST /streaming/account/{account}/source
func AddSourceToAccount(ds *datastore.DataStore, account string, sourceXML []byte) ([]byte, error) {
	var input struct {
		XMLName          xml.Name `xml:"source"`
		Username         string   `xml:"username"`
		SourceProviderID string   `xml:"sourceproviderid"`
		Credential       struct {
			Type  string `xml:"type,attr"`
			Value string `xml:",chardata"`
		} `xml:"credential"`
		SourceName string `xml:"sourcename"`
	}

	if err := xml.Unmarshal(sourceXML, &input); err != nil {
		return nil, fmt.Errorf("failed to unmarshal source XML: %w", err)
	}

	sourceID, err := AddSource(ds, account, input.Username, input.SourceProviderID, input.Credential.Value, input.Credential.Type, input.SourceName)
	if err != nil {
		return nil, err
	}

	resp := models.MargeAddSourceResponse{
		SourceID:         sourceID,
		SourceProviderID: input.SourceProviderID,
		CreatedOn:        FormatTime(time.Now()),
		UpdatedOn:        FormatTime(time.Now()),
	}

	res, _ := xml.Marshal(resp)
	header := constants.XMLHeader

	return append([]byte(header), res...), nil
}

// RemoveSourceFromAccount deletes the source with the given id from every device
// of the account, mirroring AddSource (which fans a source out to all devices).
// Idempotent: a source absent from a device is silently skipped.
func RemoveSourceFromAccount(ds *datastore.DataStore, account, sourceID string) error {
	devicesDir := ds.AccountDevicesDir(account)

	entries, err := ds.ReadDirUnderBase(devicesDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if delErr := ds.DeleteSourceByID(account, entry.Name(), sourceID); delErr != nil {
			return delErr
		}
	}

	return nil
}

// newSourceID returns a unique opaque source ID. SaveConfiguredSources dedups
// by ID, so it must be collision-free even for sources created in the same
// instant; it uses crypto/rand (64 bits) and falls back to a nanosecond
// timestamp only if the RNG ever fails.
func newSourceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "SRC_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	return "SRC_" + hex.EncodeToString(b[:])
}

// AddSource adds a new music source to the account and returns the generated source ID.
func AddSource(ds *datastore.DataStore, account, username, providerID, secret, secretType, sourceName string) (string, error) {
	now := time.Now()
	createdOn := FormatTime(now)
	// SaveConfiguredSources dedups by ID, so IDs must be unique even when two
	// sources are added in the same instant. Use a random ID rather than a
	// timestamp (which can collide on coarse clocks or rapid calls).
	sourceID := newSourceID()

	// List accounts directly from the account directory to be sure we find them.
	devicesDir := ds.AccountDevicesDir(account)
	entries, _ := ds.ReadDirUnderBase(devicesDir)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		devID := entry.Name()
		sources, _ := ds.GetConfiguredSources(account, devID)

		newSrc := models.ConfiguredSource{
			ID:               sourceID,
			SourceProviderID: providerID,
			Username:         username,
			Secret:           secret,
			SecretType:       secretType,
			SourceName:       sourceName,
			Name:             username,
			CreatedOn:        createdOn,
			UpdatedOn:        createdOn,
			Status:           "READY",
		}

		newSrc.SourceKey.Account = username

		switch providerID {
		case strconv.Itoa(constants.SpotifyProviderID):
			newSrc.SourceKey.Type = constants.ProviderSpotify
		case strconv.Itoa(constants.AmazonProviderID):
			newSrc.SourceKey.Type = constants.ProviderAmazon
		default:
			newSrc.SourceKey.Type = providerID
		}

		log.Printf("[Marge] Adding source %s (%s) for device %s", sanitizeLog(newSrc.SourceKey.Type), sanitizeLog(username), sanitizeLog(devID))

		PrepareConfiguredSource(&newSrc)

		// Update or append. Most providers are singletons (one account each), so
		// the same provider replaces the existing entry. STORED_MUSIC is the
		// exception: each DLNA media server is a separate account (username =
		// "<UDN>/0"), so it must only replace when the account also matches.
		// Otherwise registering a second media server overwrites the first, which
		// then vanishes from /full + /sources and the speaker drops it (only one
		// media server could ever stay registered).
		replaced := false

		for i := range sources {
			sameProvider := sources[i].SourceProviderID == providerID
			if providerID == strconv.Itoa(constants.StoredMusicProviderID) {
				// Match on the persisted account identity (SourceKey.Account),
				// not Username, which does not round-trip through the datastore.
				sameProvider = sameProvider && sources[i].SourceKey.Account == username
			}

			if sameProvider ||
				(providerID == strconv.Itoa(constants.SpotifyProviderID) && sources[i].SourceKey.Type == constants.ProviderSpotify) {
				sources[i] = newSrc
				replaced = true

				break
			}
		}

		if !replaced {
			sources = append(sources, newSrc)
		}

		_ = ds.SaveConfiguredSources(account, devID, sources)
	}

	return sourceID, nil
}
