// Package constants defines file names, directories, and common values used by the service layer.
package constants

// SourceProvider represents a media source provider configuration.
type SourceProvider struct {
	ID        int
	Name      string
	CreatedOn string
	UpdatedOn string
}

// StaticProviders lists known source provider identifiers with their metadata.
var StaticProviders = []SourceProvider{
	{ID: 1, Name: "PANDORA", CreatedOn: "2012-09-19T12:43:00.000+00:00", UpdatedOn: "2012-09-19T12:43:00.000+00:00"},
	{ID: 2, Name: "INTERNET_RADIO", CreatedOn: "2012-09-19T12:43:00.000+00:00", UpdatedOn: "2012-09-19T12:43:00.000+00:00"},
	{ID: 3, Name: "OFF", CreatedOn: "2012-10-22T16:03:00.000+00:00", UpdatedOn: "2012-10-22T16:03:00.000+00:00"},
	{ID: 4, Name: "LOCAL", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: 5, Name: "AIRPLAY", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: 6, Name: "CURRATED_RADIO", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: 7, Name: "STORED_MUSIC", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: 8, Name: "SLAVE_SOURCE", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: 9, Name: "AUX", CreatedOn: "2012-10-22T16:04:00.000+00:00", UpdatedOn: "2012-10-22T16:04:00.000+00:00"},
	{ID: 10, Name: "RECOMMENDED_INTERNET_RADIO", CreatedOn: "2013-01-10T09:45:00.000+00:00", UpdatedOn: "2013-01-10T09:45:00.000+00:00"},
	{ID: 11, Name: "LOCAL_INTERNET_RADIO", CreatedOn: "2013-01-10T09:45:00.000+00:00", UpdatedOn: "2013-01-10T09:45:00.000+00:00"},
	{ID: 12, Name: "GLOBAL_INTERNET_RADIO", CreatedOn: "2013-01-10T09:45:00.000+00:00", UpdatedOn: "2013-01-10T09:45:00.000+00:00"},
	{ID: 13, Name: "HELLO", CreatedOn: "2014-03-17T15:30:07.000+00:00", UpdatedOn: "2014-03-17T15:30:07.000+00:00"},
	{ID: 14, Name: "DEEZER", CreatedOn: "2014-03-17T15:30:27.000+00:00", UpdatedOn: "2014-03-17T15:30:27.000+00:00"},
	{ID: 15, Name: "SPOTIFY", CreatedOn: "2014-03-17T15:30:27.000+00:00", UpdatedOn: "2014-03-17T15:30:27.000+00:00"},
	{ID: 16, Name: "IHEART", CreatedOn: "2014-03-17T15:30:27.000+00:00", UpdatedOn: "2014-03-17T15:30:27.000+00:00"},
	{ID: 17, Name: "SIRIUSXM", CreatedOn: "2014-12-04T19:49:55.000+00:00", UpdatedOn: "2014-12-04T19:49:55.000+00:00"},
	{ID: 18, Name: "GOOGLE_PLAY_MUSIC", CreatedOn: "2014-12-04T19:49:55.000+00:00", UpdatedOn: "2014-12-04T19:49:55.000+00:00"},
	{ID: 19, Name: "QQMUSIC", CreatedOn: "2014-12-04T19:49:55.000+00:00", UpdatedOn: "2014-12-04T19:49:55.000+00:00"},
	{ID: 20, Name: "AMAZON", CreatedOn: "2014-12-04T19:49:55.000+00:00", UpdatedOn: "2014-12-04T19:49:55.000+00:00"},
	{ID: 21, Name: "LOCAL_MUSIC", CreatedOn: "2015-07-13T12:00:00.000+00:00", UpdatedOn: "2015-07-13T12:00:00.000+00:00"},
	{ID: 22, Name: "WBMX", CreatedOn: "2016-04-08T17:27:21.000+00:00", UpdatedOn: "2016-04-08T17:27:21.000+00:00"},
	{ID: 23, Name: "SOUNDCLOUD", CreatedOn: "2016-04-08T17:27:21.000+00:00", UpdatedOn: "2016-04-08T17:27:21.000+00:00"},
	{ID: 24, Name: "TIDAL", CreatedOn: "2016-04-08T17:27:21.000+00:00", UpdatedOn: "2016-04-08T17:27:21.000+00:00"},
	{ID: 25, Name: "TUNEIN", CreatedOn: "2016-04-08T17:27:21.000+00:00", UpdatedOn: "2016-04-08T17:27:21.000+00:00"},
	{ID: 26, Name: "QPLAY", CreatedOn: "2016-06-17T18:00:54.000+00:00", UpdatedOn: "2016-06-17T18:00:54.000+00:00"},
	{ID: 27, Name: "JUKE", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: 28, Name: "BBC", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: 29, Name: "DARFM", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: 30, Name: "7DIGITAL", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: 31, Name: "SAAVN", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: 32, Name: "RDIO", CreatedOn: "2016-08-01T13:53:40.000+00:00", UpdatedOn: "2016-08-01T13:53:40.000+00:00"},
	{ID: 33, Name: "PHONE_MUSIC", CreatedOn: "2016-10-26T14:42:49.000+00:00", UpdatedOn: "2016-10-26T14:42:49.000+00:00"},
	{ID: 34, Name: "ALEXA", CreatedOn: "2017-12-04T19:18:47.000+00:00", UpdatedOn: "2017-12-04T19:18:47.000+00:00"},
	{ID: 35, Name: "RADIOPLAYER", CreatedOn: "2019-05-28T18:21:20.000+00:00", UpdatedOn: "2019-05-28T18:21:20.000+00:00"},
	{ID: 36, Name: "RADIO.COM", CreatedOn: "2019-05-28T18:21:41.000+00:00", UpdatedOn: "2019-05-28T18:21:41.000+00:00"},
	{ID: 37, Name: "RADIO_COM", CreatedOn: "2019-06-13T17:30:47.000+00:00", UpdatedOn: "2019-06-13T17:30:47.000+00:00"},
	{ID: 38, Name: "SIRIUSXM_EVEREST", CreatedOn: "2019-11-25T18:00:33.000+00:00", UpdatedOn: "2019-11-25T18:00:33.000+00:00"},
	{ID: 39, Name: "RADIO_BROWSER", CreatedOn: "2026-03-14T22:47:00.000+00:00", UpdatedOn: "2026-03-14T22:47:00.000+00:00"},
}

// Providers lists known source provider identifiers used by Bose SoundTouch.
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
	"RADIO_BROWSER",
}

// Common file and path constants used by the datastore and setup logic.
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

	// XMLHeader is the standard XML declaration for Bose SoundTouch responses
	XMLHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
)
