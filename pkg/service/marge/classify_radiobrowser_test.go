package marge

import (
	"strconv"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

// TestClassifyAsRadioBrowser verifies that classifyAsRadioBrowser sets
// the expected fields on a ConfiguredSource.
func TestClassifyAsRadioBrowser(t *testing.T) {
	src := &models.ConfiguredSource{}

	classifyAsRadioBrowser(src)

	if src.SourceKey.Type != constants.ProviderRadioBrowser {
		t.Errorf("SourceKey.Type = %q, want %q", src.SourceKey.Type, constants.ProviderRadioBrowser)
	}

	if src.SourceKeyType != constants.ProviderRadioBrowser {
		t.Errorf("SourceKeyType = %q, want %q", src.SourceKeyType, constants.ProviderRadioBrowser)
	}

	if src.Type != "Audio" {
		t.Errorf("Type = %q, want Audio", src.Type)
	}

	if src.SecretType != constants.CredentialTypeToken {
		t.Errorf("SecretType = %q, want %q", src.SecretType, constants.CredentialTypeToken)
	}

	if src.Secret == "" {
		t.Error("expected Secret to be generated, got empty string")
	}

	if src.DisplayName != constants.ProviderRadioBrowser {
		t.Errorf("DisplayName = %q, want %q", src.DisplayName, constants.ProviderRadioBrowser)
	}
}

// TestClassifyAsRadioBrowser_PreservesExistingSecret verifies that a
// pre-existing secret is NOT overwritten.
func TestClassifyAsRadioBrowser_PreservesExistingSecret(t *testing.T) {
	src := &models.ConfiguredSource{Secret: "existing-secret"}

	classifyAsRadioBrowser(src)

	if src.Secret != "existing-secret" {
		t.Errorf("expected existing secret to be preserved, got %q", src.Secret)
	}
}

// TestClassifyLearnedSource_RadioBrowserByProviderID verifies that the
// classifyLearnedSource dispatcher routes to classifyAsRadioBrowser when
// sourceProviderID matches RadioBrowserProviderID (39).
func TestClassifyLearnedSource_RadioBrowserByProviderID(t *testing.T) {
	src := &models.ConfiguredSource{}

	classifyLearnedSource(src, "", "", strconv.Itoa(constants.RadioBrowserProviderID))

	if src.SourceKey.Type != constants.ProviderRadioBrowser {
		t.Errorf("expected RADIO_BROWSER from providerID 39, got %q", src.SourceKey.Type)
	}
}

// TestClassifyLearnedSource_RadioBrowserByLocation verifies that the dispatcher
// routes to classifyAsRadioBrowser when the location contains the RadioBrowser
// byuuid path segment, for both the relative form the native RADIO_BROWSER play
// path now emits and the legacy absolute form (#479).
func TestClassifyLearnedSource_RadioBrowserByLocation(t *testing.T) {
	locations := []string{
		"/stations/byuuid/abc-123", // native relative location
		"https://all.api.radio-browser.info/soundtouch/stations/byuuid/abc-123", // legacy absolute location
	}

	for _, location := range locations {
		src := &models.ConfiguredSource{}

		classifyLearnedSource(src, "", location, "")

		if src.SourceKey.Type != constants.ProviderRadioBrowser {
			t.Errorf("expected RADIO_BROWSER from byuuid location %q, got %q", location, src.SourceKey.Type)
		}
	}
}
