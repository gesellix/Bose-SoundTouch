package models

import "strings"

// SourceIdentity is a source stripped to what identifies it: enough to say
// "this speaker has Spotify as listener@example" or "this one knows the
// MiniDLNA server", and nothing else.
//
// It exists because ConfiguredSource carries the credential material a
// speaker's Sources.xml holds -- including a `secret` field that is tagged
// for JSON -- and none of that may leave the service. The player's control
// API is unauthenticated (issue 663), so a projection is the only safe shape
// to hand it. See sourceIdentityLeakGuard in the tests.
type SourceIdentity struct {
	// Type is the sourceKey type: SPOTIFY, STORED_MUSIC, TUNEIN, ...
	Type string `json:"type"`
	// Account is the sourceKey account: a user name, a DLNA UDN, or empty
	// for sources that have no account.
	Account string `json:"account,omitempty"`
	// DisplayName is what the owner recognises ("MiniDLNA", "fritz").
	DisplayName string `json:"display_name,omitempty"`
	// Availability says what it would take to give this source to a speaker
	// that does not have it. See the SourceAvailable* constants.
	Availability string `json:"availability,omitempty"`
	// Devices lists the device ids that have this source.
	Devices []string `json:"devices,omitempty"`
}

// What it takes to give a speaker a source it does not have.
const (
	// SourceAvailableToAdd: AfterTouch can add it itself. Either there is no
	// credential at all (a media server, identified by its DLNA UDN) or the
	// service mints the token (TuneIn, Radio Browser, Local Internet Radio).
	SourceAvailableToAdd = "addable"
	// SourceAvailableByLinking: a music service whose credential the target
	// speaker has to acquire for itself, by linking the account there. The
	// credential cannot be carried across for it.
	SourceAvailableByLinking = "link-required"
	// SourceAvailableLocalOnly: a physical input or a device-local source.
	// It is not somewhere else to be copied from; every speaker has its own.
	SourceAvailableLocalOnly = "local-only"
)

// localOnlySources are inputs and device-local sources: a speaker either has
// the socket or it does not, and nothing about another speaker's copy helps.
var localOnlySources = map[string]bool{
	"AUX":                         true,
	"BLUETOOTH":                   true,
	"HDMI":                        true,
	"OPTICAL":                     true,
	"PRODUCT":                     true,
	"AIRPLAY":                     true,
	"ALEXA":                       true,
	"NOTIFICATION":                true,
	"UPNP":                        true,
	"STORED_MUSIC_MEDIA_RENDERER": true,
}

// mintedSources are the ones AfterTouch issues the token for itself, so it can
// hand them to any speaker without anyone linking anything.
var mintedSources = map[string]bool{
	"TUNEIN":               true,
	"RADIO_BROWSER":        true,
	"LOCAL_INTERNET_RADIO": true,
}

// SourceAvailability classifies a source by what it would take to give it to a
// speaker that does not have it.
func SourceAvailability(sourceType string) string {
	upper := strings.ToUpper(strings.TrimSpace(sourceType))

	switch {
	case localOnlySources[upper]:
		return SourceAvailableLocalOnly
	case mintedSources[upper]:
		return SourceAvailableToAdd
	// A media server is identified by its DLNA UDN and carries no credential,
	// so adding it is naming it.
	case upper == "STORED_MUSIC", upper == "LOCAL_MUSIC":
		return SourceAvailableToAdd
	default:
		return SourceAvailableByLinking
	}
}

// IdentityOfSource projects a configured source down to its identity.
// Deliberately field by field rather than by copying the struct: a future
// field on ConfiguredSource must not become visible here by default.
func IdentityOfSource(source ConfiguredSource) SourceIdentity {
	sourceType := source.SourceKey.Type
	if sourceType == "" {
		sourceType = source.SourceKeyType
	}

	account := source.SourceKey.Account
	if account == "" {
		account = source.SourceKeyAccount
	}

	return SourceIdentity{
		Type:         strings.TrimSpace(sourceType),
		Account:      strings.TrimSpace(account),
		DisplayName:  strings.TrimSpace(source.DisplayName),
		Availability: SourceAvailability(sourceType),
	}
}

// Key identifies a source across speakers: its type plus its account.
func (s SourceIdentity) Key() string {
	return strings.ToUpper(s.Type) + "\x00" + strings.ToLower(s.Account)
}

// Label is the best human name for a source: its display name where a speaker
// recorded one, otherwise its type.
func (s SourceIdentity) Label() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}

	return s.Type
}
