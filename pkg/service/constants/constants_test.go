package constants

import (
	"testing"
)

func TestConstants(t *testing.T) {
	if DateStr == "" {
		t.Error("DateStr should not be empty")
	}
	if SpeakerHTTPPort != 8090 {
		t.Errorf("Expected SpeakerHTTPPort 8090, got %d", SpeakerHTTPPort)
	}
	if len(Providers) == 0 {
		t.Error("Providers should not be empty")
	}
}
