package main

import (
	"strings"
	"testing"
)

// custom-radio must store a relative Orion station location (issue 769), so a
// preset saved from it keeps working when AfterTouch changes its address.
func TestCustomRadioLocationIsRelative(t *testing.T) {
	loc := customRadioLocation("Test Station", "", "http://example.com/stream.mp3")

	if !strings.HasPrefix(loc, "/station?data=") {
		t.Fatalf("location = %q; want the relative /station?data=... form", loc)
	}

	if strings.Contains(loc, "://") {
		t.Errorf("location = %q; must not carry a host", loc)
	}
}
