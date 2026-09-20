package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func configuredSource(sourceType, account, displayName, secret string) ConfiguredSource {
	s := ConfiguredSource{DisplayName: displayName, Secret: secret, SecretType: "token"}
	s.SourceKey.Type = sourceType
	s.SourceKey.Account = account
	s.Credential.Type = "token"
	s.Credential.Value = secret

	return s
}

// The projection exists to keep credential material inside the service: a
// speaker's Sources.xml carries it, ConfiguredSource.Secret is tagged for
// JSON, and the player's API is unauthenticated (issue 663). AfterTouch's own
// `bs-` surrogate is a capability too -- whoever presents it is resolved to
// that linked account.
func TestSourceIdentityCarriesNoCredentialMaterial(t *testing.T) {
	const secret = "bs-6c58d056c2d35df85f57ad2334b0cdc4"

	identity := IdentityOfSource(configuredSource("SPOTIFY", "listener", "Spotify", secret))

	encoded, err := json.Marshal(identity)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, forbidden := range []string{secret, "secret", "credential", "token"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(forbidden)) {
			t.Fatalf("projection leaked %q: %s", forbidden, encoded)
		}
	}

	if identity.Type != "SPOTIFY" || identity.Account != "listener" || identity.DisplayName != "Spotify" {
		t.Fatalf("identity lost what it is for: %+v", identity)
	}
}

// Speakers that never learned the sourceKey element still carry the legacy
// flat fields, and a source with no identity at all must not masquerade as
// one belonging to every speaker.
func TestIdentityOfSourceFallsBackToTheFlatFields(t *testing.T) {
	legacy := ConfiguredSource{DisplayName: "fritz", SourceKeyType: "STORED_MUSIC", SourceKeyAccount: "uuid:aaaa/0"}

	identity := IdentityOfSource(legacy)
	if identity.Type != "STORED_MUSIC" || identity.Account != "uuid:aaaa/0" {
		t.Fatalf("legacy fields ignored: %+v", identity)
	}
}

func TestSourceAvailability(t *testing.T) {
	tests := map[string]string{
		// No credential at all: a media server is its DLNA UDN.
		"STORED_MUSIC": SourceAvailableToAdd,
		// AfterTouch mints these tokens itself.
		"TUNEIN":               SourceAvailableToAdd,
		"RADIO_BROWSER":        SourceAvailableToAdd,
		"LOCAL_INTERNET_RADIO": SourceAvailableToAdd,
		// The target speaker has to acquire its own credential.
		"SPOTIFY": SourceAvailableByLinking,
		"AMAZON":  SourceAvailableByLinking,
		"DEEZER":  SourceAvailableByLinking,
		// A socket is not somewhere else to be copied from.
		"AUX":       SourceAvailableLocalOnly,
		"BLUETOOTH": SourceAvailableLocalOnly,
		"PRODUCT":   SourceAvailableLocalOnly,
		"airplay":   SourceAvailableLocalOnly,
	}

	for sourceType, want := range tests {
		if got := SourceAvailability(sourceType); got != want {
			t.Errorf("SourceAvailability(%q) = %q, want %q", sourceType, got, want)
		}
	}
}

// One source across two speakers has to collapse to one entry, so the account
// comparison cannot be case-sensitive where the speaker is not.
func TestSourceIdentityKeyIgnoresCase(t *testing.T) {
	a := SourceIdentity{Type: "stored_music", Account: "UUID:AAAA/0"}
	b := SourceIdentity{Type: "STORED_MUSIC", Account: "uuid:aaaa/0"}

	if a.Key() != b.Key() {
		t.Fatalf("same source keyed differently: %q vs %q", a.Key(), b.Key())
	}

	other := SourceIdentity{Type: "STORED_MUSIC", Account: "uuid:bbbb/0"}
	if other.Key() == b.Key() {
		t.Fatal("two media servers collapsed into one")
	}
}
