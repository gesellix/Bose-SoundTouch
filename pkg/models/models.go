// Package models defines data structures used for Bose SoundTouch API communication
// and service management. It includes types for BMX (Bose Media eXchange) services,
// device information, presets, recents, and other core data models.
package models

import (
	"encoding/xml"
)

// Link represents a navigational link with URL and client usage preferences.
type Link struct {
	Href              string `json:"href" xml:"href,attr"`
	UseInternalClient string `json:"useInternalClient,omitempty" xml:"useInternalClient,attr,omitempty"`
}

// Links contains various navigation links used by BMX services.
type Links struct {
	BmxLogout               *Link `json:"bmx_logout,omitempty" xml:"bmx_logout,omitempty"`
	BmxNavigate             *Link `json:"bmx_navigate,omitempty" xml:"bmx_navigate,omitempty"`
	BmxServicesAvailability *Link `json:"bmx_services_availability,omitempty" xml:"bmx_services_availability,omitempty"`
	BmxToken                *Link `json:"bmx_token,omitempty" xml:"bmx_token,omitempty"`
	Self                    *Link `json:"self,omitempty" xml:"self,omitempty"`
	BmxAvailability         *Link `json:"bmx_availability,omitempty" xml:"bmx_availability,omitempty"`
	BmxReporting            *Link `json:"bmx_reporting,omitempty" xml:"bmx_reporting,omitempty"`
	BmxFavorite             *Link `json:"bmx_favorite,omitempty" xml:"bmx_favorite,omitempty"`
	BmxNowPlaying           *Link `json:"bmx_nowplaying,omitempty" xml:"bmx_nowplaying,omitempty"`
	BmxTrack                *Link `json:"bmx_track,omitempty" xml:"bmx_track,omitempty"`
}

// IconSet represents a collection of icons with different sizes for media content.
type IconSet struct {
	DefaultAlbumArt string `json:"defaultAlbumArt,omitempty" xml:"defaultAlbumArt,omitempty"`
	LargeSvg        string `json:"largeSvg" xml:"largeSvg"`
	MonochromePng   string `json:"monochromePng" xml:"monochromePng"`
	MonochromeSvg   string `json:"monochromeSvg" xml:"monochromeSvg"`
	SmallSvg        string `json:"smallSvg" xml:"smallSvg"`
}

// Asset represents a media asset with URL and content type information.
type Asset struct {
	Color            string  `json:"color" xml:"color"`
	Description      string  `json:"description" xml:"description"`
	Icons            IconSet `json:"icons" xml:"icons"`
	Name             string  `json:"name" xml:"name"`
	ShortDescription string  `json:"shortDescription,omitempty" xml:"shortDescription,omitempty"`
}

// Id represents an identifier structure used in various API responses.
type Id struct {
	Name  string `json:"name" xml:"name"`
	Value int    `json:"value" xml:"value"`
}

// BmxService represents a Bose Media eXchange service configuration.
type BmxService struct {
	Links               *Links                 `json:"_links,omitempty" xml:"links,omitempty"`
	AskAdapter          bool                   `json:"askAdapter" xml:"askAdapter"`
	Assets              Asset                  `json:"assets" xml:"assets"`
	BaseUrl             string                 `json:"baseUrl" xml:"baseUrl"`
	SignupUrl           string                 `json:"signupUrl,omitempty" xml:"signupUrl,omitempty"`
	StreamTypes         []string               `json:"streamTypes" xml:"streamTypes>streamType"`
	AuthenticationModel map[string]interface{} `json:"authenticationModel" xml:"authenticationModel"`
	ID                  Id                     `json:"id" xml:"id"`
}

// BmxResponse represents a response from BMX services.
type BmxResponse struct {
	Links         *Links    `json:"_links,omitempty" xml:"links,omitempty"`
	AskAgainAfter int       `json:"askAgainAfter" xml:"askAgainAfter"`
	BmxServices   []Service `json:"bmx_services" xml:"bmx_services>service"`
}

// Stream represents audio stream information including URL and format details.
type Stream struct {
	Links             *Links `json:"_links,omitempty" xml:"links,omitempty"`
	BufferingTimeout  int    `json:"bufferingTimeout,omitempty" xml:"bufferingTimeout,omitempty"`
	ConnectingTimeout int    `json:"connectingTimeout,omitempty" xml:"connectingTimeout,omitempty"`
	HasPlaylist       bool   `json:"hasPlaylist" xml:"hasPlaylist"`
	IsRealtime        bool   `json:"isRealtime" xml:"isRealtime"`
	StreamUrl         string `json:"streamUrl" xml:"streamUrl"`
}

// Audio represents audio content metadata including format and quality information.
type Audio struct {
	HasPlaylist bool     `json:"hasPlaylist" xml:"hasPlaylist"`
	IsRealtime  bool     `json:"isRealtime" xml:"isRealtime"`
	MaxTimeout  int      `json:"maxTimeout,omitempty" xml:"maxTimeout,omitempty"`
	StreamUrl   string   `json:"streamUrl" xml:"streamUrl"`
	Streams     []Stream `json:"streams" xml:"streams>stream"`
}

// BmxPlaybackResponse represents a playback response from BMX services.
type BmxPlaybackResponse struct {
	Links  *Links `json:"_links,omitempty" xml:"links,omitempty"`
	Artist struct {
		Name string `json:"name,omitempty" xml:"name,omitempty"`
	} `json:"artist,omitempty" xml:"artist,omitempty"`
	Audio           Audio  `json:"audio" xml:"audio"`
	ImageUrl        string `json:"imageUrl" xml:"imageUrl"`
	IsFavorite      *bool  `json:"isFavorite,omitempty" xml:"isFavorite,omitempty"`
	Name            string `json:"name" xml:"name"`
	StreamType      string `json:"streamType" xml:"streamType"`
	Duration        int    `json:"duration,omitempty" xml:"duration,omitempty"`
	ShuffleDisabled bool   `json:"shuffle_disabled,omitempty" xml:"shuffleDisabled,omitempty"`
	RepeatDisabled  bool   `json:"repeat_disabled,omitempty" xml:"repeatDisabled,omitempty"`
}

// Track represents track information for media playback.
type Track struct {
	Links      *Links `json:"_links,omitempty" xml:"links,omitempty"`
	IsSelected bool   `json:"isSelected" xml:"isSelected"`
	Name       string `json:"name" xml:"name"`
}

// BmxPodcastInfoResponse represents podcast information from BMX services.
type BmxPodcastInfoResponse struct {
	Links           *Links  `json:"_links,omitempty" xml:"links,omitempty"`
	Name            string  `json:"name" xml:"name"`
	ShuffleDisabled bool    `json:"shuffleDisabled" xml:"shuffleDisabled"`
	RepeatDisabled  bool    `json:"repeatDisabled" xml:"repeatDisabled"`
	StreamType      string  `json:"streamType" xml:"streamType"`
	Tracks          []Track `json:"tracks" xml:"tracks>track"`
}

// SourceProvider represents a media source provider configuration.
type SourceProvider struct {
	ID        int    `json:"id" xml:"id,attr"`
	CreatedOn string `json:"created_on" xml:"createdOn"`
	Name      string `json:"name" xml:"name"`
	UpdatedOn string `json:"updated_on" xml:"updatedOn"`
}

// ServiceContentItem represents a media content item with source and location details.
type ServiceContentItem struct {
	ID              string `json:"id" xml:"id,attr"`
	Name            string `json:"name" xml:"name"`
	Source          string `json:"source,omitempty" xml:"source,attr,omitempty"`
	Type            string `json:"type" xml:"type,attr"`
	ContentItemType string `json:"content_item_type" xml:"contentItemType"`
	Location        string `json:"location,omitempty" xml:"location,attr,omitempty"`
	SourceAccount   string `json:"source_account" xml:"sourceAccount,attr"`
	SourceID        string `json:"source_id,omitempty" xml:"sourceid,omitempty"`
	IsPresetable    string `json:"is_presetable,omitempty" xml:"isPresetable,attr,omitempty"`
}

// ServicePreset represents a user-defined preset for quick access to media content.
type ServicePreset struct {
	ServiceContentItem
	ID           string            `json:"id,omitempty" xml:"id,attr"`
	ContainerArt string            `json:"container_art" xml:"containerArt"`
	CreatedOn    string            `json:"created_on" xml:"createdOn"`
	UpdatedOn    string            `json:"updated_on" xml:"updatedOn"`
	ButtonNumber string            `json:"button_number,omitempty" xml:"buttonNumber,attr,omitempty"`
	Username     string            `json:"-" xml:"username,omitempty"`
	SourceConfig *ConfiguredSource `json:"-" xml:"source,omitempty"`
}

// ServiceRecent represents recently played media content.
type ServiceRecent struct {
	XMLName xml.Name `json:"-" xml:"recent"`
	ServiceContentItem
	DeviceID     string            `json:"device_id" xml:"deviceID,attr"`
	UtcTime      string            `json:"utc_time" xml:"utcTime,attr"`
	CreatedOn    string            `json:"created_on,omitempty" xml:"createdOn"`
	UpdatedOn    string            `json:"updated_on,omitempty" xml:"updatedOn"`
	ContainerArt string            `json:"container_art,omitempty" xml:"containerArt,omitempty"`
	SourceConfig *ConfiguredSource `json:"-" xml:"source,omitempty"`
	LastPlayedAt string            `json:"last_played_at,omitempty" xml:"lastplayedat"`
	ContentItem  *struct {
		Source        string `xml:"source,attr"`
		Type          string `xml:"type,attr"`
		Location      string `xml:"location,attr"`
		SourceAccount string `xml:"sourceAccount,attr"`
		IsPresetable  string `xml:"isPresetable,attr"`
		ItemName      string `xml:"itemName"`
		ContainerArt  string `xml:"containerArt,omitempty"`
	} `xml:"contentItem,omitempty"`
}

// UnmarshalXML implements the xml.Unmarshaler interface to handle both nested and flat formats.
func (r *ServiceRecent) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type ContentItem struct {
		Source        string `xml:"source,attr"`
		Type          string `xml:"type,attr"`
		Location      string `xml:"location,attr"`
		SourceAccount string `xml:"sourceAccount,attr"`
		IsPresetable  string `xml:"isPresetable,attr"`
		ItemName      string `xml:"itemName"`
		ContainerArt  string `xml:"containerArt,omitempty"`
	}

	type Alias struct {
		XMLName xml.Name `xml:"recent"`
		ServiceContentItem
		DeviceID     string            `xml:"deviceID,attr"`
		UtcTime      string            `xml:"utcTime,attr"`
		ID           string            `xml:"id,attr"`
		CreatedOn    string            `xml:"createdOn,omitempty"`
		UpdatedOn    string            `xml:"updatedOn,omitempty"`
		ContainerArt string            `xml:"containerArt,omitempty"`
		SourceConfig *ConfiguredSource `xml:"source,omitempty"`
		LastPlayedAt string            `xml:"lastplayedat"`
		ContentItem  *ContentItem      `xml:"contentItem,omitempty"`
	}

	var a Alias
	if err := d.DecodeElement(&a, &start); err != nil {
		return err
	}

	r.DeviceID = a.DeviceID
	r.UtcTime = a.UtcTime
	r.ID = a.ID

	r.SourceID = a.SourceID
	if r.SourceID == "" {
		r.SourceID = a.SourceID
	}

	r.CreatedOn = a.CreatedOn
	r.UpdatedOn = a.UpdatedOn
	r.ContainerArt = a.ContainerArt
	r.SourceConfig = a.SourceConfig
	r.LastPlayedAt = a.LastPlayedAt
	// Ensure the embedded ServiceContentItem.ID is populated from the attribute
	r.ID = a.ID

	if a.ContentItem != nil {
		r.Source = a.ContentItem.Source
		r.Type = a.ContentItem.Type
		r.Location = a.ContentItem.Location
		r.SourceAccount = a.ContentItem.SourceAccount
		r.IsPresetable = a.ContentItem.IsPresetable

		r.Name = a.ContentItem.ItemName
		if a.ContentItem.ContainerArt != "" {
			r.ContainerArt = a.ContentItem.ContainerArt
		}
	} else {
		// Fallback for flat format: populate ContentItem fields from root fields
		r.Source = a.Source
		r.Type = a.Type
		r.Location = a.Location
		r.SourceAccount = a.SourceAccount
		r.IsPresetable = a.IsPresetable
		r.Name = a.Name
	}

	// Always ensure the nested struct is populated for MarshalXML
	r.ContentItem = &struct {
		Source        string `xml:"source,attr"`
		Type          string `xml:"type,attr"`
		Location      string `xml:"location,attr"`
		SourceAccount string `xml:"sourceAccount,attr"`
		IsPresetable  string `xml:"isPresetable,attr"`
		ItemName      string `xml:"itemName"`
		ContainerArt  string `xml:"containerArt,omitempty"`
	}{
		Source:        r.Source,
		Type:          r.Type,
		Location:      r.Location,
		SourceAccount: r.SourceAccount,
		IsPresetable:  r.IsPresetable,
		ItemName:      r.Name,
		ContainerArt:  r.ContainerArt,
	}

	return nil
}

// MarshalXML implements the xml.Marshaler interface for custom XML encoding of ServiceRecent.
func (r ServiceRecent) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	type ContentItem struct {
		Source        string `xml:"source,attr"`
		Type          string `xml:"type,attr"`
		Location      string `xml:"location,attr"`
		SourceAccount string `xml:"sourceAccount,attr"`
		IsPresetable  string `xml:"isPresetable,attr"`
		ItemName      string `xml:"itemName"`
		ContainerArt  string `xml:"containerArt,omitempty"`
	}

	type Alias struct {
		XMLName      xml.Name          `xml:"recent"`
		DeviceID     string            `xml:"deviceID,attr"`
		UtcTime      string            `xml:"utcTime,attr"`
		ID           string            `xml:"id,attr"`
		ContentItem  ContentItem       `xml:"contentItem"`
		CreatedOn    string            `xml:"createdOn,omitempty"`
		UpdatedOn    string            `xml:"updatedOn,omitempty"`
		LastPlayedAt string            `xml:"lastplayedat,omitempty"`
		SourceID     string            `xml:"sourceid,omitempty"`
		Source       *ConfiguredSource `xml:"source,omitempty"`
	}

	a := Alias{
		DeviceID: r.DeviceID,
		UtcTime:  r.UtcTime,
		ID:       r.ID,
		SourceID: r.SourceID,
		ContentItem: ContentItem{
			Source:        r.Source,
			Type:          r.Type,
			Location:      r.Location,
			SourceAccount: r.SourceAccount,
			IsPresetable:  r.IsPresetable,
			ItemName:      r.Name,
			ContainerArt:  r.ContainerArt,
		},
		CreatedOn:    r.CreatedOn,
		UpdatedOn:    r.UpdatedOn,
		LastPlayedAt: r.LastPlayedAt,
		Source:       r.SourceConfig,
	}

	if a.SourceID == "" && r.SourceID != "" {
		a.SourceID = r.SourceID
	}

	if r.ContentItem != nil {
		a.ContentItem.Source = r.ContentItem.Source
		a.ContentItem.Type = r.ContentItem.Type
		a.ContentItem.Location = r.ContentItem.Location
		a.ContentItem.SourceAccount = r.ContentItem.SourceAccount
		a.ContentItem.IsPresetable = r.ContentItem.IsPresetable

		a.ContentItem.ItemName = r.ContentItem.ItemName
		if r.ContentItem.ContainerArt != "" {
			a.ContentItem.ContainerArt = r.ContentItem.ContainerArt
		}
	}

	if a.Source == nil && r.SourceConfig != nil {
		a.Source = r.SourceConfig
	}

	if a.ContentItem.IsPresetable == "" {
		a.ContentItem.IsPresetable = "true"
	}

	start.Name.Local = "recent"

	return e.EncodeElement(a, start)
}

// ConfiguredSource represents a configured media source with authentication details.
type ConfiguredSource struct {
	XMLName     xml.Name `json:"-" xml:"source"`
	DisplayName string   `json:"display_name" xml:"displayName,attr,omitempty"`
	ID          string   `json:"id" xml:"id,attr,omitempty"`
	Secret      string   `json:"secret" xml:"secret,attr"`
	SecretType  string   `json:"secret_type" xml:"secretType,attr"`
	SourceKey   struct {
		Type    string `xml:"type,attr"`
		Account string `xml:"account,attr"`
	} `json:"source_key" xml:"sourceKey"`
	Type string `xml:"type,attr,omitempty"`

	// Parity fields
	CreatedOn        string `json:"created_on,omitempty" xml:"createdOn,attr,omitempty"`
	UpdatedOn        string `json:"updated_on,omitempty" xml:"updatedOn,attr,omitempty"`
	SourceProviderID string `json:"sourceproviderid,omitempty" xml:"sourceproviderid,attr,omitempty"`
	Username         string `json:"username,omitempty" xml:"-"`
	SourceName       string `json:"source_name,omitempty" xml:"-"`
	Name             string `json:"name,omitempty" xml:"-"`
	SourceSettings   string `json:"-" xml:"-"`
	Status           string `json:"status,omitempty" xml:"-"`

	// Legacy fields for backward compatibility in code if needed,
	// though it's better to update the code to use SourceKey.
	SourceKeyType    string `json:"source_key_type" xml:"-"`
	SourceKeyAccount string `json:"source_key_account" xml:"-"`
}

// MarshalXML implements the xml.Marshaler interface for custom XML encoding of ConfiguredSource.
func (s ConfiguredSource) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	type Alias struct {
		DisplayName      string `xml:"displayName,attr,omitempty"`
		Secret           string `xml:"secret,attr"`
		SecretType       string `xml:"secretType,attr"`
		ID               string `xml:"id,attr,omitempty"`
		Type             string `xml:"type,attr,omitempty"`
		CreatedOn        string `xml:"createdOn,attr,omitempty"`
		UpdatedOn        string `xml:"updatedOn,attr,omitempty"`
		SourceProviderID string `xml:"sourceproviderid,attr,omitempty"`
		SourceKey        struct {
			Type    string `xml:"type,attr"`
			Account string `xml:"account,attr"`
		} `xml:"sourceKey"`
	}

	a := Alias{
		DisplayName:      s.DisplayName,
		Secret:           s.Secret,
		SecretType:       s.SecretType,
		ID:               s.ID,
		Type:             s.Type,
		CreatedOn:        s.CreatedOn,
		UpdatedOn:        s.UpdatedOn,
		SourceProviderID: s.SourceProviderID,
	}
	a.SourceKey.Type = s.SourceKey.Type
	a.SourceKey.Account = s.SourceKey.Account

	start.Name.Local = "source"
	// Important: Clear automatically generated attributes from the start element
	// because we are using Alias to control attribute order and presence.
	start.Attr = nil

	return e.EncodeElement(a, start)
}

// ServiceDeviceInfo represents information about a SoundTouch device.
type ServiceDeviceInfo struct {
	DeviceID            string             `json:"device_id" xml:"deviceID,attr"`
	ProductCode         string             `json:"product_code" xml:"type"`
	DeviceSerialNumber  string             `json:"device_serial_number" xml:"serialnumber"`
	ProductSerialNumber string             `json:"product_serial_number" xml:"product_serial_number"`
	FirmwareVersion     string             `json:"firmware_version" xml:"softwareVersion"`
	IPAddress           string             `json:"ip_address" xml:"ipAddress"`
	Name                string             `json:"name" xml:"name"`
	MacAddress          string             `json:"mac_address,omitempty" xml:"-"`
	DiscoveryMethod     string             `json:"discovery_method,omitempty"`
	AccountID           string             `json:"account_id,omitempty"`
	Components          []ServiceComponent `json:"components,omitempty" xml:"-"`
}

// ServiceComponent represents a hardware or software component of a device.
type ServiceComponent struct {
	Type            string `json:"type" xml:"type,attr"`
	Category        string `json:"category,omitempty" xml:"category,attr,omitempty"`
	SoftwareVersion string `json:"firmware_version" xml:"firmware-version"`
	SerialNumber    string `json:"serial_number" xml:"serialnumber"`
	Label           string `json:"label,omitempty" xml:"componentlabel,omitempty"`
}

// ServiceAccountInfo represents account-level metadata.
type ServiceAccountInfo struct {
	AccountID         string            `json:"account_id"`
	PreferredLanguage string            `json:"preferred_language"`
	ProviderSettings  []ProviderSetting `json:"provider_settings"`
	IsPlaceholder     bool              `json:"is_placeholder,omitempty"`
}

// CustomerSupportDevice represents device information for customer support purposes.
type CustomerSupportDevice struct {
	ID              string `xml:"id,attr"`
	SerialNumber    string `xml:"serialnumber"`
	FirmwareVersion string `xml:"firmware-version"`
	Product         struct {
		ProductCode  string `xml:"product_code,attr"`
		Type         string `xml:"type,attr"`
		SerialNumber string `xml:"serialnumber"`
	} `xml:"product"`
}

// CustomerSupportRequest represents a customer support request with device and configuration details.
type CustomerSupportRequest struct {
	XMLName        xml.Name              `xml:"device-data"`
	Device         CustomerSupportDevice `xml:"device"`
	DiagnosticData struct {
		DeviceLandscape struct {
			RSSI                  string   `xml:"rssi"`
			GatewayIP             string   `xml:"gateway-ip-address"`
			IPAddress             string   `xml:"ip-address"`
			NetworkConnectionType string   `xml:"network-connection-type"`
			MacAddresses          []string `xml:"macaddresses>macaddress"`
		} `xml:"device-landscape"`
	} `xml:"diagnostic-data"`
}

// UsageStats represents usage statistics for the service.
type UsageStats struct {
	DeviceID   string                 `json:"deviceId" xml:"deviceId"`
	AccountID  string                 `json:"accountId" xml:"accountId"`
	Timestamp  string                 `json:"timestamp" xml:"timestamp"`
	EventType  string                 `json:"eventType" xml:"eventType"`
	Parameters map[string]interface{} `json:"parameters" xml:"parameters"`
}

// ErrorStats represents error statistics for monitoring and debugging.
type ErrorStats struct {
	DeviceID     string `json:"deviceId" xml:"deviceId"`
	ErrorCode    string `json:"errorCode" xml:"errorCode"`
	ErrorMessage string `json:"errorMessage" xml:"errorMessage"`
	Timestamp    string `json:"timestamp" xml:"timestamp"`
	Details      string `json:"details,omitempty" xml:"details,omitempty"`
}

// DeviceEvent represents an event that occurred on a device.
type DeviceEvent struct {
	Type     string                 `json:"type"`
	Time     string                 `json:"time"`
	MonoTime int64                  `json:"monoTime"`
	Data     map[string]interface{} `json:"data"`
}

// DeviceEventsRequest represents a request containing multiple device events (stapp/scmudc).
type DeviceEventsRequest struct {
	Envelope struct {
		MonoTime               int64  `json:"monoTime"`
		PayloadProtocolVersion string `json:"payloadProtocolVersion"`
		PayloadType            string `json:"payloadType"`
		ProtocolVersion        string `json:"protocolVersion"`
		Time                   string `json:"time"`
		UniqueID               string `json:"uniqueId"`
	} `json:"envelope"`
	Payload struct {
		DeviceInfo struct {
			BoseID          string `json:"boseID"`
			DeviceID        string `json:"deviceID"`
			DeviceType      string `json:"deviceType"`
			SoftwareVersion string `json:"softwareVersion"`
		} `json:"deviceInfo"`
		Events []struct {
			Data map[string]interface{} `json:"data"`
			Time string                 `json:"time"`
			Type string                 `json:"type"`
		} `json:"events"`
	} `json:"payload"`
}

// DeviceSettingsResponse represents device settings.
type DeviceSettingsResponse struct {
	XMLName  xml.Name        `xml:"deviceSettings"`
	Settings []DeviceSetting `xml:"deviceSetting"`
}

// DeviceSetting represents a single device setting.
type DeviceSetting struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}

// AccountProfileResponse represents a customer account profile.
type AccountProfileResponse struct {
	XMLName        xml.Name `xml:"customer"`
	AccountID      string   `xml:"accountID"`
	Email          string   `xml:"email"`
	FirstName      string   `xml:"firstName"`
	LastName       string   `xml:"lastName"`
	CountryCode    string   `xml:"countryCode"`
	LanguageCode   string   `xml:"languageCode"`
	Street         string   `xml:"street"`
	City           string   `xml:"city"`
	PostalCode     string   `xml:"postalCode"`
	State          string   `xml:"state"`
	Phone          string   `xml:"phone"`
	MarketingOptIn bool     `xml:"marketingOptIn"`
}

// ChangePasswordRequest represents a request to change the account password.
type ChangePasswordRequest struct {
	XMLName     xml.Name `xml:"passwordChange"`
	OldPassword string   `xml:"oldPassword"`
	NewPassword string   `xml:"newPassword"`
}

// EmailAddressResponse represents the account email address.
type EmailAddressResponse struct {
	XMLName xml.Name `xml:"emailAddress"`
	Email   string   `xml:",chardata"`
}

// FullResponseSource represents a configured media source specifically for the /full response.
// It follows the specific XML structure and field order of the upstream /full response.
type FullResponseSource struct {
	ID          string `json:"id" xml:"id,attr"`
	Type        string `json:"type" xml:"type,attr"`
	DisplayName string `json:"display_name,omitempty" xml:"displayName,attr,omitempty"`
	CreatedOn   string `json:"created_on" xml:"createdOn"`
	Credential  struct {
		Type  string `json:"type" xml:"type,attr"`
		Value string `json:"value" xml:",chardata"`
	} `json:"credential" xml:"credential"`
	Name             string `json:"name" xml:"name"`
	SourceProviderID string `json:"sourceproviderid" xml:"sourceproviderid"`
	SourceName       string `json:"source_name" xml:"sourcename"`
	SourceSettings   string `json:"source_settings" xml:"sourceSettings"`
	UpdatedOn        string `json:"updated_on" xml:"updatedOn"`
	Username         string `json:"username" xml:"username"`
	Account          string `json:"account,omitempty" xml:"account,attr,omitempty"`
	SourceLabel      string `json:"source_label" xml:"-"`
}

// FullResponsePreset represents a preset specifically for the /full response.
type FullResponsePreset struct {
	ButtonNumber    string             `json:"button_number" xml:"buttonNumber,attr"`
	ContainerArt    string             `json:"container_art" xml:"containerArt"`
	ContentItemType string             `json:"content_item_type" xml:"contentItemType"`
	CreatedOn       string             `json:"created_on" xml:"createdOn"`
	Location        string             `json:"location" xml:"location"`
	Name            string             `json:"name" xml:"name"`
	Source          FullResponseSource `json:"source" xml:"source"`
	UpdatedOn       string             `json:"updated_on" xml:"updatedOn"`
	Username        string             `json:"username" xml:"username"`
}

// FullResponseRecent represents a recent item specifically for the /full response.
type FullResponseRecent struct {
	ID              string             `json:"id" xml:"id,attr"`
	ContentItemType string             `json:"content_item_type" xml:"contentItemType"`
	CreatedOn       string             `json:"created_on" xml:"createdOn"`
	LastPlayedAt    string             `json:"last_played_at" xml:"lastplayedat"`
	Location        string             `json:"location" xml:"location"`
	Name            string             `json:"name" xml:"name"`
	Source          FullResponseSource `json:"source" xml:"source"`
	SourceID        string             `json:"source_id" xml:"sourceid"`
	UpdatedOn       string             `json:"updated_on" xml:"updatedOn"`
}

// AccountFullResponse represents the complete account XML structure.
type AccountFullResponse struct {
	XMLName           xml.Name             `xml:"account"`
	ID                string               `xml:"id,attr"`
	AccountStatus     string               `xml:"accountStatus"`
	Devices           []AccountDevice      `xml:"devices>device"`
	Mode              string               `xml:"mode"`
	PreferredLanguage string               `xml:"preferredLanguage"`
	ProviderSettings  []ProviderSetting    `xml:"providerSettings>providerSetting"`
	Sources           []FullResponseSource `xml:"sources>source"`
}

// AccountDevice represents a device in the account response.
type AccountDevice struct {
	DeviceID           string               `json:"device_id" xml:"deviceid,attr"`
	AttachedProduct    *AttachedProduct     `json:"attached_product" xml:"attachedProduct"`
	CreatedOn          string               `json:"created_on" xml:"createdOn"`
	FirmwareVersion    string               `json:"firmware_version" xml:"firmwareVersion"`
	IPAddress          string               `json:"ip_address" xml:"ipaddress"`
	Name               string               `json:"name" xml:"name"`
	Presets            []FullResponsePreset `json:"presets" xml:"presets>preset"`
	ProductCode        string               `json:"product_code" xml:"-"`
	Recents            []FullResponseRecent `json:"recents" xml:"recents>recent"`
	SerialNumber       string               `json:"serial_number" xml:"serialNumber"`
	DeviceSerialNumber string               `json:"device_serial_number,omitempty" xml:"-"`
	MacAddress         string               `json:"mac_address,omitempty" xml:"-"`
	DiscoveryMethod    string               `json:"discovery_method,omitempty" xml:"-"`
	UpdatedOn          string               `json:"updated_on" xml:"updatedOn"`
}

// AttachedProduct represents product information for a device.
type AttachedProduct struct {
	ProductCode  string             `json:"product_code" xml:"product_code,attr"`
	Components   []ServiceComponent `json:"components" xml:"components>component"`
	ProductLabel string             `json:"product_label" xml:"productlabel"`
	SerialNumber string             `json:"serial_number" xml:"serialnumber"`
	UpdatedOn    string             `json:"updated_on" xml:"updatedOn"`
}

// ProviderSetting represents a single provider setting.
type ProviderSetting struct {
	BoseID       string `json:"bose_id" xml:"boseId"`
	KeyName      string `json:"key_name" xml:"keyName"`
	Value        string `json:"value" xml:"value"`
	ProviderID   string `json:"provider_id" xml:"providerId"`
	ProviderName string `json:"provider_name,omitempty" xml:"-"`
}
