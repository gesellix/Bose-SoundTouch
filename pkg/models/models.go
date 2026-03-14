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
	Location        string `json:"location" xml:"location"`
	SourceAccount   string `json:"source_account,omitempty" xml:"sourceAccount,attr,omitempty"`
	SourceID        string `json:"source_id,omitempty" xml:"sourceid,omitempty"`
	IsPresetable    string `json:"is_presetable,omitempty" xml:"isPresetable,attr,omitempty"`
}

// ServicePreset represents a user-defined preset for quick access to media content.
type ServicePreset struct {
	ServiceContentItem
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
	DeviceID     string            `json:"device_id" xml:"deviceid,attr"`
	UtcTime      string            `json:"utc_time" xml:"utcTime,attr"`
	CreatedOn    string            `json:"created_on,omitempty" xml:"createdOn"`
	UpdatedOn    string            `json:"updated_on,omitempty" xml:"updatedOn"`
	ContainerArt string            `json:"container_art,omitempty" xml:"containerArt,omitempty"`
	SourceConfig *ConfiguredSource `json:"-" xml:"source,omitempty"`
	LastPlayedAt string            `json:"last_played_at,omitempty" xml:"lastplayedat"`
}

// ConfiguredSource represents a configured media source with authentication details.
type ConfiguredSource struct {
	XMLName     xml.Name `json:"-" xml:"source"`
	DisplayName string   `json:"display_name" xml:"name"`
	ID          string   `json:"id" xml:"id,attr"`
	Secret      string   `json:"secret" xml:"credential"`
	SecretType  string   `json:"secret_type" xml:"credential_type,attr"`
	SourceKey   struct {
		Type    string `xml:"type,attr"`
		Account string `xml:"account,attr"`
	} `json:"source_key" xml:"source_key"`
	Type string `xml:"type,attr"`

	// Parity fields
	CreatedOn        string `json:"created_on,omitempty" xml:"createdOn"`
	UpdatedOn        string `json:"updated_on,omitempty" xml:"updatedOn"`
	SourceProviderID string `json:"sourceproviderid,omitempty" xml:"sourceproviderid"`
	Username         string `json:"username,omitempty" xml:"username"`
	SourceName       string `json:"source_name,omitempty" xml:"sourcename"`
	SourceSettings   string `json:"-" xml:"sourceSettings"`

	// Legacy fields for backward compatibility in code if needed,
	// though it's better to update the code to use SourceKey.
	SourceKeyType    string `json:"source_key_type" xml:"-"`
	SourceKeyAccount string `json:"source_key_account" xml:"-"`
}

// MarshalXML implements the xml.Marshaler interface for custom XML encoding of ConfiguredSource.
func (s ConfiguredSource) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	type Alias ConfiguredSource

	a := struct {
		Alias
		Username       string `xml:"username"`
		SourceName     string `xml:"sourcename"`
		SourceSettings string `xml:"sourceSettings"`
	}{
		Alias: Alias(s),
	}
	a.Username = s.Username
	a.SourceName = s.SourceName
	// We want <sourceSettings/>
	a.SourceSettings = ""

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
	Type            string `xml:"type,attr"`
	Category        string `xml:"category,attr,omitempty"`
	SoftwareVersion string `xml:"firmware-version"`
	SerialNumber    string `xml:"serialnumber"`
	Label           string `xml:"componentlabel,omitempty"`
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
	ID         string `xml:"id,attr"`
	Type       string `xml:"type,attr"`
	CreatedOn  string `xml:"createdOn"`
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

// FullResponsePreset represents a preset specifically for the /full response.
type FullResponsePreset struct {
	ButtonNumber    string             `xml:"buttonNumber,attr"`
	ContainerArt    string             `xml:"containerArt"`
	ContentItemType string             `xml:"contentItemType"`
	CreatedOn       string             `xml:"createdOn"`
	Location        string             `xml:"location"`
	Name            string             `xml:"name"`
	Source          FullResponseSource `xml:"source"`
	UpdatedOn       string             `xml:"updatedOn"`
	Username        string             `xml:"username"`
}

// FullResponseRecent represents a recent item specifically for the /full response.
type FullResponseRecent struct {
	ID              string             `xml:"id,attr"`
	ContentItemType string             `xml:"contentItemType"`
	CreatedOn       string             `xml:"createdOn"`
	LastPlayedAt    string             `xml:"lastplayedat"`
	Location        string             `xml:"location"`
	Name            string             `xml:"name"`
	Source          FullResponseSource `xml:"source"`
	SourceID        string             `xml:"sourceid"`
	UpdatedOn       string             `xml:"updatedOn"`
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
	DeviceID        string               `xml:"deviceid,attr"`
	AttachedProduct *AttachedProduct     `xml:"attachedProduct"`
	CreatedOn       string               `xml:"createdOn"`
	FirmwareVersion string               `xml:"firmwareVersion"`
	IPAddress       string               `xml:"ipaddress"`
	Name            string               `xml:"name"`
	Presets         []FullResponsePreset `xml:"presets>preset"`
	Recents         []FullResponseRecent `xml:"recents>recent"`
	SerialNumber    string               `xml:"serialNumber"`
	UpdatedOn       string               `xml:"updatedOn"`
}

// AttachedProduct represents product information for a device.
type AttachedProduct struct {
	ProductCode  string             `xml:"product_code,attr"`
	Components   []ServiceComponent `xml:"components>component"`
	ProductLabel string             `xml:"productlabel"`
	SerialNumber string             `xml:"serialNumber"`
	UpdatedOn    string             `xml:"updatedOn"`
}

// ProviderSetting represents a single provider setting.
type ProviderSetting struct {
	BoseID     string `xml:"boseId"`
	KeyName    string `xml:"keyName"`
	Value      string `xml:"value"`
	ProviderID string `xml:"providerId"`
}
