package models

import (
	"encoding/xml"
)

type Link struct {
	Href              string `json:"href" xml:"href,attr"`
	UseInternalClient string `json:"useInternalClient,omitempty" xml:"useInternalClient,attr,omitempty"`
}

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

type IconSet struct {
	DefaultAlbumArt string `json:"defaultAlbumArt,omitempty" xml:"defaultAlbumArt,omitempty"`
	LargeSvg        string `json:"largeSvg" xml:"largeSvg"`
	MonochromePng   string `json:"monochromePng" xml:"monochromePng"`
	MonochromeSvg   string `json:"monochromeSvg" xml:"monochromeSvg"`
	SmallSvg        string `json:"smallSvg" xml:"smallSvg"`
}

type Asset struct {
	Color            string  `json:"color" xml:"color"`
	Description      string  `json:"description" xml:"description"`
	Icons            IconSet `json:"icons" xml:"icons"`
	Name             string  `json:"name" xml:"name"`
	ShortDescription string  `json:"shortDescription,omitempty" xml:"shortDescription,omitempty"`
}

type Id struct {
	Name  string `json:"name" xml:"name"`
	Value int    `json:"value" xml:"value"`
}

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

type BmxResponse struct {
	Links         *Links    `json:"_links,omitempty" xml:"links,omitempty"`
	AskAgainAfter int       `json:"askAgainAfter" xml:"askAgainAfter"`
	BmxServices   []Service `json:"bmx_services" xml:"bmx_services>service"`
}

type Stream struct {
	Links             *Links `json:"_links,omitempty" xml:"links,omitempty"`
	BufferingTimeout  int    `json:"bufferingTimeout,omitempty" xml:"bufferingTimeout,omitempty"`
	ConnectingTimeout int    `json:"connectingTimeout,omitempty" xml:"connectingTimeout,omitempty"`
	HasPlaylist       bool   `json:"hasPlaylist" xml:"hasPlaylist"`
	IsRealtime        bool   `json:"isRealtime" xml:"isRealtime"`
	StreamUrl         string `json:"streamUrl" xml:"streamUrl"`
}

type Audio struct {
	HasPlaylist bool     `json:"hasPlaylist" xml:"hasPlaylist"`
	IsRealtime  bool     `json:"isRealtime" xml:"isRealtime"`
	MaxTimeout  int      `json:"maxTimeout,omitempty" xml:"maxTimeout,omitempty"`
	StreamUrl   string   `json:"streamUrl" xml:"streamUrl"`
	Streams     []Stream `json:"streams" xml:"streams>stream"`
}

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

type Track struct {
	Links      *Links `json:"_links,omitempty" xml:"links,omitempty"`
	IsSelected bool   `json:"isSelected" xml:"isSelected"`
	Name       string `json:"name" xml:"name"`
}

type BmxPodcastInfoResponse struct {
	Links           *Links  `json:"_links,omitempty" xml:"links,omitempty"`
	Name            string  `json:"name" xml:"name"`
	ShuffleDisabled bool    `json:"shuffleDisabled" xml:"shuffleDisabled"`
	RepeatDisabled  bool    `json:"repeatDisabled" xml:"repeatDisabled"`
	StreamType      string  `json:"streamType" xml:"streamType"`
	Tracks          []Track `json:"tracks" xml:"tracks>track"`
}

type SourceProvider struct {
	ID        int    `json:"id" xml:"id,attr"`
	CreatedOn string `json:"created_on" xml:"createdOn"`
	Name      string `json:"name" xml:"name"`
	UpdatedOn string `json:"updated_on" xml:"updatedOn"`
}

type ServiceContentItem struct {
	ID            string `json:"id" xml:"id,attr"`
	Name          string `json:"name" xml:"itemName"`
	Source        string `json:"source,omitempty" xml:"source,attr,omitempty"`
	Type          string `json:"type" xml:"type,attr"`
	Location      string `json:"location" xml:"location,attr"`
	SourceAccount string `json:"source_account,omitempty" xml:"sourceAccount,attr,omitempty"`
	SourceID      string `json:"source_id,omitempty" xml:"sourceid,omitempty"`
	IsPresetable  string `json:"is_presetable,omitempty" xml:"isPresetable,attr,omitempty"`
}

type ServicePreset struct {
	ServiceContentItem
	ContainerArt string `json:"container_art" xml:"containerArt"`
	CreatedOn    string `json:"created_on" xml:"createdOn"`
	UpdatedOn    string `json:"updated_on" xml:"updatedOn"`
}

type ServiceRecent struct {
	ServiceContentItem
	DeviceID     string `json:"device_id" xml:"deviceid"`
	UtcTime      string `json:"utc_time" xml:"utc_time"`
	ContainerArt string `json:"container_art,omitempty" xml:"containerArt,omitempty"`
}

type ConfiguredSource struct {
	DisplayName      string `json:"display_name" xml:"sourcename"`
	ID               string `json:"id" xml:"id,attr"`
	Secret           string `json:"secret" xml:"credential"`
	SecretType       string `json:"secret_type" xml:"credential_type,attr"`
	SourceKeyType    string `json:"source_key_type" xml:"sourceproviderid"`
	SourceKeyAccount string `json:"source_key_account" xml:"username"`
}

type ServiceDeviceInfo struct {
	DeviceID            string `json:"device_id" xml:"deviceID,attr"`
	ProductCode         string `json:"product_code" xml:"type"`
	DeviceSerialNumber  string `json:"device_serial_number" xml:"serialnumber"`
	ProductSerialNumber string `json:"product_serial_number" xml:"product_serial_number"`
	FirmwareVersion     string `json:"firmware_version" xml:"softwareVersion"`
	IPAddress           string `json:"ip_address" xml:"ipAddress"`
	Name                string `json:"name" xml:"name"`
}

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

type UsageStats struct {
	DeviceID   string                 `json:"deviceId" xml:"deviceId"`
	AccountID  string                 `json:"accountId" xml:"accountId"`
	Timestamp  string                 `json:"timestamp" xml:"timestamp"`
	EventType  string                 `json:"eventType" xml:"eventType"`
	Parameters map[string]interface{} `json:"parameters" xml:"parameters"`
}

type ErrorStats struct {
	DeviceID     string `json:"deviceId" xml:"deviceId"`
	ErrorCode    string `json:"errorCode" xml:"errorCode"`
	ErrorMessage string `json:"errorMessage" xml:"errorMessage"`
	Timestamp    string `json:"timestamp" xml:"timestamp"`
	Details      string `json:"details,omitempty" xml:"details,omitempty"`
}

type DeviceEvent struct {
	Type     string                 `json:"type"`
	Time     string                 `json:"time"`
	MonoTime int64                  `json:"monoTime"`
	Data     map[string]interface{} `json:"data"`
}
