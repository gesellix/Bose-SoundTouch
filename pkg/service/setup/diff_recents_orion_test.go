package setup

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// A stored recent with the absolute Orion location and a fetched one with the
// relative location of the same station (issue 769) is not a removal.
func TestDiffRecents_OrionAbsoluteAndRelativeAreOneEntry(t *testing.T) {
	recent := func(name, location string) models.ServiceRecent {
		var r models.ServiceRecent
		r.Name = name
		r.Source = "LOCAL_INTERNET_RADIO"
		r.Location = location

		return r
	}

	current := []models.ServiceRecent{
		recent("Doc", "http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=eyJ9"),
		recent("Gone", "/station?data=gone"),
	}
	incoming := []models.ServiceRecent{recent("Doc", "/station?data=eyJ9")}

	diff := diffRecents(current, incoming)
	if len(diff.Removed) != 1 || diff.Removed[0] != "Gone" {
		t.Fatalf("removed = %v, want only [Gone]", diff.Removed)
	}
}
