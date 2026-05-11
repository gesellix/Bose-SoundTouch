package constants

import (
	"testing"
)

func TestConstants(t *testing.T) {
	if DateStr == "" {
		t.Error("DateStr should not be empty")
	}

	if len(GetProviders()) == 0 {
		t.Error("Providers should not be empty")
	}
}
