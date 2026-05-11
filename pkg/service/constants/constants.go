// Package constants defines file names, directories, and common values used by the service layer.
package constants

import "strconv"

// SourceProvider represents a media source provider configuration.
type SourceProvider struct {
	ID        int
	Name      string
	Label     string
	CreatedOn string
	UpdatedOn string
}

const (
	// ProviderPandora is the identifier for Pandora.
	ProviderPandora = "PANDORA"
	// ProviderInternetRadio is the identifier for Internet Radio.
	ProviderInternetRadio = "INTERNET_RADIO"
	// ProviderOff is the identifier for Off.
	ProviderOff = "OFF"
	// ProviderLocal is the identifier for Local.
	ProviderLocal = "LOCAL"
	// ProviderAirplay is the identifier for AirPlay.
	ProviderAirplay = "AIRPLAY"
	// ProviderCuratedRadio is the identifier for Curated Radio.
	ProviderCuratedRadio = "CURRATED_RADIO"
	// ProviderStoredMusic is the identifier for Stored Music.
	ProviderStoredMusic = "STORED_MUSIC"
	// ProviderSlaveSource is the identifier for Slave Source.
	ProviderSlaveSource = "SLAVE_SOURCE"
	// ProviderAux is the identifier for Aux.
	ProviderAux = "AUX"
	// ProviderRecommendedInternetRadio is the identifier for Recommended Internet Radio.
	ProviderRecommendedInternetRadio = "RECOMMENDED_INTERNET_RADIO"
	// ProviderLocalInternetRadio is the identifier for Local Internet Radio.
	ProviderLocalInternetRadio = "LOCAL_INTERNET_RADIO"
	// ProviderGlobalInternetRadio is the identifier for Global Internet Radio.
	ProviderGlobalInternetRadio = "GLOBAL_INTERNET_RADIO"
	// ProviderHello is the identifier for Hello.
	ProviderHello = "HELLO"
	// ProviderDeezer is the identifier for Deezer.
	ProviderDeezer = "DEEZER"
	// ProviderSpotify is the identifier for Spotify.
	ProviderSpotify = "SPOTIFY"
	// ProviderIHeart is the identifier for iHeartRadio.
	ProviderIHeart = "IHEART"
	// ProviderSiriusXM is the identifier for SiriusXM.
	ProviderSiriusXM = "SIRIUSXM"
	// ProviderGooglePlayMusic is the identifier for Google Play Music.
	ProviderGooglePlayMusic = "GOOGLE_PLAY_MUSIC"
	// ProviderQQMusic is the identifier for QQMusic.
	ProviderQQMusic = "QQMUSIC"
	// ProviderAmazon is the identifier for Amazon Music.
	ProviderAmazon = "AMAZON"
	// ProviderLocalMusic is the identifier for Local Music Library.
	ProviderLocalMusic = "LOCAL_MUSIC"
	// ProviderWbmx is the identifier for WBMX.
	ProviderWbmx = "WBMX"
	// ProviderSoundcloud is the identifier for SoundCloud.
	ProviderSoundcloud = "SOUNDCLOUD"
	// ProviderTidal is the identifier for Tidal.
	ProviderTidal = "TIDAL"
	// ProviderTunein is the identifier for TuneIn Radio.
	ProviderTunein = "TUNEIN"
	// ProviderQPlay is the identifier for QPlay.
	ProviderQPlay = "QPLAY"
	// ProviderJuke is the identifier for Juke.
	ProviderJuke = "JUKE"
	// ProviderBbc is the identifier for BBC.
	ProviderBbc = "BBC"
	// ProviderDarfm is the identifier for DAR.fm.
	ProviderDarfm = "DARFM"
	// Provider7Digital is the identifier for 7digital.
	Provider7Digital = "7DIGITAL"
	// ProviderSaavn is the identifier for Saavn.
	ProviderSaavn = "SAAVN"
	// ProviderRdio is the identifier for Rdio.
	ProviderRdio = "RDIO"
	// ProviderPhoneMusic is the identifier for Phone Music.
	ProviderPhoneMusic = "PHONE_MUSIC"
	// ProviderAlexa is the identifier for Amazon Alexa.
	ProviderAlexa = "ALEXA"
	// ProviderRadioplayer is the identifier for Radioplayer.
	// RADIOPLAYER is deprecated: https://www.radioplayer.de/apps/bose.html
	ProviderRadioplayer = "RADIOPLAYER"
	// ProviderRadioDotCom is the identifier for Radio.com.
	ProviderRadioDotCom = "RADIO.COM"
	// ProviderRadioCom is the identifier for Radio.com (alternate).
	ProviderRadioCom = "RADIO_COM"
	// ProviderSiriusXmEverest is the identifier for SiriusXM Everest.
	ProviderSiriusXmEverest = "SIRIUSXM_EVEREST"
	// ProviderRadioBrowser is the identifier for Radio Browser.
	ProviderRadioBrowser = "RADIO_BROWSER"
	// ProviderBluetooth is the identifier for Bluetooth.
	ProviderBluetooth = "BLUETOOTH"
	// ProviderBmx is the identifier for BMX.
	ProviderBmx = "BMX"
	// ProviderNotification is the identifier for Notifications.
	ProviderNotification = "NOTIFICATION"
	// ProviderAuxIn is the identifier for AUX IN.
	ProviderAuxIn = "AUX_IN"
)

const (
	// PandoraProviderID is the provider identifier for Pandora.
	PandoraProviderID = 1
	// InternetRadioProviderID is the provider identifier for Internet Radio.
	InternetRadioProviderID = 2
	// OffProviderID is the provider identifier for Off.
	OffProviderID = 3
	// LocalProviderID is the provider identifier for Local.
	LocalProviderID = 4
	// AirplayProviderID is the provider identifier for AirPlay.
	AirplayProviderID = 5
	// CuratedRadioProviderID is the provider identifier for Curated Radio.
	CuratedRadioProviderID = 6
	// StoredMusicProviderID is the provider identifier for Stored Music.
	StoredMusicProviderID = 7
	// SlaveSourceProviderID is the provider identifier for Slave Source.
	SlaveSourceProviderID = 8
	// AuxProviderID is the provider identifier for Aux.
	AuxProviderID = 9
	// RecommendedInternetRadioProviderID is the provider identifier for Recommended Internet Radio.
	RecommendedInternetRadioProviderID = 10
	// LocalInternetRadioProviderID is the provider identifier for Local Internet Radio.
	LocalInternetRadioProviderID = 11
	// GlobalInternetRadioProviderID is the provider identifier for Global Internet Radio.
	GlobalInternetRadioProviderID = 12
	// HelloProviderID is the provider identifier for Hello.
	HelloProviderID = 13
	// DeezerProviderID is the provider identifier for Deezer.
	DeezerProviderID = 14
	// SpotifyProviderID is the provider identifier for Spotify.
	SpotifyProviderID = 15
	// IHeartProviderID is the provider identifier for iHeartRadio.
	IHeartProviderID = 16
	// SiriusXMProviderID is the provider identifier for SiriusXM.
	SiriusXMProviderID = 17
	// GooglePlayMusicProviderID is the provider identifier for Google Play Music.
	GooglePlayMusicProviderID = 18
	// QQMusicProviderID is the provider identifier for QQMusic.
	QQMusicProviderID = 19
	// AmazonProviderID is the provider identifier for Amazon Music.
	AmazonProviderID = 20
	// LocalMusicProviderID is the provider identifier for Local Music Library.
	LocalMusicProviderID = 21
	// WbmxProviderID is the provider identifier for WBMX.
	WbmxProviderID = 22
	// SoundcloudProviderID is the provider identifier for SoundCloud.
	SoundcloudProviderID = 23
	// TidalProviderID is the provider identifier for Tidal.
	TidalProviderID = 24
	// TuneinProviderID is the provider identifier for TuneIn Radio.
	TuneinProviderID = 25
	// QPlayProviderID is the provider identifier for QPlay.
	QPlayProviderID = 26
	// JukeProviderID is the provider identifier for Juke.
	JukeProviderID = 27
	// BbcProviderID is the provider identifier for BBC.
	BbcProviderID = 28
	// DarfmProviderID is the provider identifier for DAR.fm.
	DarfmProviderID = 29
	// SevenDigitalProviderID is the provider identifier for 7digital.
	SevenDigitalProviderID = 30
	// SaavnProviderID is the provider identifier for Saavn.
	SaavnProviderID = 31
	// RdioProviderID is the provider identifier for Rdio.
	RdioProviderID = 32
	// PhoneMusicProviderID is the provider identifier for Phone Music.
	PhoneMusicProviderID = 33
	// AlexaProviderID is the provider identifier for Amazon Alexa.
	AlexaProviderID = 34
	// RadioplayerProviderID is the provider identifier for Radioplayer.
	RadioplayerProviderID = 35
	// RadioDotComProviderID is the provider identifier for Radio.com.
	RadioDotComProviderID = 36
	// RadioComProviderID is the provider identifier for Radio.com (alternate).
	RadioComProviderID = 37
	// SiriusXmEverestProviderID is the provider identifier for SiriusXM Everest.
	SiriusXmEverestProviderID = 38
	// RadioBrowserProviderID is the provider identifier for Radio Browser.
	RadioBrowserProviderID = 39
	// BluetoothProviderID is the provider identifier for Bluetooth.
	BluetoothProviderID = 40
	// BmxProviderID is the provider identifier for BMX.
	BmxProviderID = 41
	// NotificationProviderID is the provider identifier for Notifications.
	NotificationProviderID = 42
	// AuxInProviderID is the provider identifier for AUX IN.
	AuxInProviderID = 43
)

// StaticProviders lists known source provider identifiers with their metadata.
var StaticProviders = []SourceProvider{
	{ID: PandoraProviderID, Name: ProviderPandora, Label: "Pandora", CreatedOn: "2012-09-19T12:43:00.000+00:00", UpdatedOn: "2012-09-19T12:43:00.000+00:00"},
	{ID: InternetRadioProviderID, Name: ProviderInternetRadio, Label: "Internet Radio", CreatedOn: "2012-09-19T12:43:00.000+00:00", UpdatedOn: "2012-09-19T12:43:00.000+00:00"},
	{ID: OffProviderID, Name: ProviderOff, Label: "Off", CreatedOn: "2012-10-22T16:03:00.000+00:00", UpdatedOn: "2012-10-22T16:03:00.000+00:00"},
	{ID: LocalProviderID, Name: ProviderLocal, Label: "Local", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: AirplayProviderID, Name: ProviderAirplay, Label: "AirPlay", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: CuratedRadioProviderID, Name: ProviderCuratedRadio, Label: "Curated Radio", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: StoredMusicProviderID, Name: ProviderStoredMusic, Label: "Stored Music", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: SlaveSourceProviderID, Name: ProviderSlaveSource, Label: "Slave Source", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: AuxProviderID, Name: ProviderAux, Label: "Aux", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: RecommendedInternetRadioProviderID, Name: ProviderRecommendedInternetRadio, Label: "Recommended Internet Radio", CreatedOn: "2013-01-10T09:45:00.000+00:00", UpdatedOn: "2013-01-10T09:45:00.000+00:00"},
	{ID: LocalInternetRadioProviderID, Name: ProviderLocalInternetRadio, Label: "Local Internet Radio", CreatedOn: "2013-01-10T09:45:00.000+00:00", UpdatedOn: "2013-01-10T09:45:00.000+00:00"},
	{ID: GlobalInternetRadioProviderID, Name: ProviderGlobalInternetRadio, Label: "Global Internet Radio", CreatedOn: "2013-01-10T09:45:00.000+00:00", UpdatedOn: "2013-01-10T09:45:00.000+00:00"},
	{ID: HelloProviderID, Name: ProviderHello, Label: "Hello", CreatedOn: "2014-03-17T15:30:07.000+00:00", UpdatedOn: "2014-03-17T15:30:07.000+00:00"},
	{ID: DeezerProviderID, Name: ProviderDeezer, Label: "Deezer", CreatedOn: "2014-03-17T15:30:27.000+00:00", UpdatedOn: "2014-03-17T15:30:27.000+00:00"},
	{ID: SpotifyProviderID, Name: ProviderSpotify, Label: "Spotify", CreatedOn: "2014-03-17T15:30:27.000+00:00", UpdatedOn: "2014-03-17T15:30:27.000+00:00"},
	{ID: IHeartProviderID, Name: ProviderIHeart, Label: "iHeartRadio", CreatedOn: "2014-03-17T15:30:27.000+00:00", UpdatedOn: "2014-03-17T15:30:27.000+00:00"},
	{ID: SiriusXMProviderID, Name: ProviderSiriusXM, Label: "SiriusXM", CreatedOn: "2014-12-04T19:49:55.000+00:00", UpdatedOn: "2014-12-04T19:49:55.000+00:00"},
	{ID: GooglePlayMusicProviderID, Name: ProviderGooglePlayMusic, Label: "Google Play Music", CreatedOn: "2014-12-04T19:49:55.000+00:00", UpdatedOn: "2014-12-04T19:49:55.000+00:00"},
	{ID: QQMusicProviderID, Name: ProviderQQMusic, Label: "QQMusic", CreatedOn: "2014-12-04T19:49:55.000+00:00", UpdatedOn: "2014-12-04T19:49:55.000+00:00"},
	{ID: AmazonProviderID, Name: ProviderAmazon, Label: "Amazon Music", CreatedOn: "2014-12-04T19:49:55.000+00:00", UpdatedOn: "2014-12-04T19:49:55.000+00:00"},
	{ID: LocalMusicProviderID, Name: ProviderLocalMusic, Label: "Local Music Library", CreatedOn: "2015-07-13T12:00:00.000+00:00", UpdatedOn: "2015-07-13T12:00:00.000+00:00"},
	{ID: WbmxProviderID, Name: ProviderWbmx, Label: "WBMX", CreatedOn: "2016-04-08T17:27:21.000+00:00", UpdatedOn: "2016-04-08T17:27:21.000+00:00"},
	{ID: SoundcloudProviderID, Name: ProviderSoundcloud, Label: "SoundCloud", CreatedOn: "2016-04-08T17:27:21.000+00:00", UpdatedOn: "2016-04-08T17:27:21.000+00:00"},
	{ID: TidalProviderID, Name: ProviderTidal, Label: "Tidal", CreatedOn: "2016-04-08T17:27:21.000+00:00", UpdatedOn: "2016-04-08T17:27:21.000+00:00"},
	{ID: TuneinProviderID, Name: ProviderTunein, Label: "TuneIn Radio", CreatedOn: "2016-04-08T17:27:21.000+00:00", UpdatedOn: "2016-04-08T17:27:21.000+00:00"},
	{ID: QPlayProviderID, Name: ProviderQPlay, Label: "QPlay", CreatedOn: "2016-06-17T18:00:54.000+00:00", UpdatedOn: "2016-06-17T18:00:54.000+00:00"},
	{ID: JukeProviderID, Name: ProviderJuke, Label: "Juke", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: BbcProviderID, Name: ProviderBbc, Label: "BBC", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: DarfmProviderID, Name: ProviderDarfm, Label: "DAR.fm", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: SevenDigitalProviderID, Name: Provider7Digital, Label: "7digital", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: SaavnProviderID, Name: ProviderSaavn, Label: "Saavn", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: RdioProviderID, Name: ProviderRdio, Label: "Rdio", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: PhoneMusicProviderID, Name: ProviderPhoneMusic, Label: "Phone Music", CreatedOn: "2016-10-26T14:42:49.000+00:00", UpdatedOn: "2016-10-26T14:42:49.000+00:00"},
	{ID: AlexaProviderID, Name: ProviderAlexa, Label: "Amazon Alexa", CreatedOn: "2017-12-04T19:18:47.000+00:00", UpdatedOn: "2017-12-04T19:18:47.000+00:00"},
	{ID: RadioplayerProviderID, Name: ProviderRadioplayer, Label: "Radioplayer", CreatedOn: "2019-05-28T18:21:20.000+00:00", UpdatedOn: "2019-05-28T18:21:20.000+00:00"},
	{ID: RadioDotComProviderID, Name: ProviderRadioDotCom, Label: "Radio.com", CreatedOn: "2019-05-28T18:21:41.000+00:00", UpdatedOn: "2019-05-28T18:21:41.000+00:00"},
	{ID: RadioComProviderID, Name: ProviderRadioCom, Label: "Radio.com", CreatedOn: "2019-06-13T17:30:47.000+00:00", UpdatedOn: "2019-06-13T17:30:47.000+00:00"},
	{ID: SiriusXmEverestProviderID, Name: ProviderSiriusXmEverest, Label: "SiriusXM Everest", CreatedOn: "2019-11-25T18:00:33.000+00:00", UpdatedOn: "2019-11-25T18:00:33.000+00:00"},
	{ID: RadioBrowserProviderID, Name: ProviderRadioBrowser, Label: "Radio Browser", CreatedOn: "2026-03-14T22:47:00.000+00:00", UpdatedOn: "2026-03-14T22:47:00.000+00:00"},
	{ID: BluetoothProviderID, Name: ProviderBluetooth, Label: "Bluetooth", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: BmxProviderID, Name: ProviderBmx, Label: "BMX", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: NotificationProviderID, Name: ProviderNotification, Label: "Notifications", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: AuxInProviderID, Name: ProviderAuxIn, Label: "AUX IN", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
}

// GetSourceLabel returns a user-friendly label for a source type.
func GetSourceLabel(sourceType string) string {
	for _, provider := range StaticProviders {
		if provider.Name == sourceType {
			return provider.Label
		}
	}

	return sourceType
}

// GetProviderName returns the human-readable name for a provider ID.
func GetProviderName(providerID string) string {
	id, err := strconv.Atoi(providerID)
	if err != nil {
		return providerID
	}

	for _, p := range StaticProviders {
		if p.ID == id {
			return p.Name
		}
	}

	return providerID
}

// GetProviderLabel returns the user-friendly label for a provider ID (e.g. "TuneIn Radio", "Spotify").
func GetProviderLabel(providerID string) string {
	id, err := strconv.Atoi(providerID)
	if err != nil {
		return ""
	}

	for _, p := range StaticProviders {
		if p.ID == id {
			return p.Label
		}
	}

	return ""
}

// GetProviders returns a list of known source provider names.
func GetProviders() []string {
	var providers []string
	for _, p := range StaticProviders {
		providers = append(providers, p.Name)
	}

	return providers
}

// Common file and path constants used by the datastore and setup logic.
const (
	DevicesDir     = "devices"
	DeviceInfoFile = "DeviceInfo.xml"
	PresetsFile    = "Presets.xml"
	RecentsFile    = "Recents.xml"
	SourcesFile    = "Sources.xml"

	// Speaker-protocol constants (HTTP port, paths, on-device file
	// locations) moved to github.com/gesellix/bose-soundtouch/pkg/speaker
	// so the client library and CLI can share them without depending on
	// the service package.

	// DateStr is the hardcoded date used in many Bose XML responses
	DateStr = "2012-09-19T12:43:00.000+00:00"

	// XMLHeader is the standard XML declaration for Bose SoundTouch responses
	XMLHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`

	// CredentialTypeToken is the standard token credential type.
	CredentialTypeToken = "token"
	// CredentialTypeTokenV3 is the token version 3 credential type, used for Spotify.
	CredentialTypeTokenV3 = "token_version_3"
)
