package constants

var Providers = []string{
	"PANDORA",
	"INTERNET_RADIO",
	"OFF",
	"LOCAL",
	"AIRPLAY",
	"CURRATED_RADIO",
	"STORED_MUSIC",
	"SLAVE_SOURCE",
	"AUX",
	"RECOMMENDED_INTERNET_RADIO",
	"LOCAL_INTERNET_RADIO",
	"GLOBAL_INTERNET_RADIO",
	"HELLO",
	"DEEZER",
	"SPOTIFY",
	"IHEART",
	"SIRIUSXM",
	"GOOGLE_PLAY_MUSIC",
	"QQMUSIC",
	"AMAZON",
	"LOCAL_MUSIC",
	"WBMX",
	"SOUNDCLOUD",
	"TIDAL",
	"TUNEIN",
	"QPLAY",
	"JUKE",
	"BBC",
	"DARFM",
	"7DIGITAL",
	"SAAVN",
	"RDIO",
	"PHONE_MUSIC",
	"ALEXA",
	"RADIOPLAYER",
	"RADIO.COM",
	"RADIO_COM",
	"SIRIUSXM_EVEREST",
}

const (
	DevicesDir     = "devices"
	DeviceInfoFile = "DeviceInfo.xml"
	PresetsFile    = "Presets.xml"
	RecentsFile    = "Recents.xml"
	SourcesFile    = "Sources.xml"

	SpeakerHTTPPort            = 8090
	SpeakerDeviceInfoPath      = "/info"
	SpeakerRecentsPath         = "/recents"
	SpeakerPresetsPath         = "/presets"
	SpeakerSourcesFileLocation = "/mnt/nv/BoseApp-Persistence/1/Sources.xml"

	// DateStr is the hardcoded date used in many Bose XML responses
	DateStr = "2012-09-19T12:43:00.000+00:00"
)
