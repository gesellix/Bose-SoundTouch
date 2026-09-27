package marge

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

const (
	orionRelative = "/station?data=eyJuYW1lIjoiRG9jIn0%3D"
	orionAbsolute = "http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=eyJuYW1lIjoiRG9jIn0%3D"
)

// A recent saved with the absolute Orion location (before issue 769) and the
// same station played again with the relative one stay a single recent.
func TestUpdateOrCreateRecent_OrionAbsoluteAndRelativeAreOneRecent(t *testing.T) {
	src := &models.ConfiguredSource{}
	src.SourceKeyType = "LOCAL_INTERNET_RADIO"

	var old models.ServiceRecent
	old.ID = "1"
	old.Name = "Doc"
	old.Source = "LOCAL_INTERNET_RADIO"
	old.Location = orionAbsolute

	recentObj, out := updateOrCreateRecent([]models.ServiceRecent{old}, "Doc", src, "stationurl", orionRelative, "DEVICEID01", 12345)

	if len(out) != 1 {
		t.Fatalf("recents = %s, want one entry", names(out))
	}

	if recentObj.ID != "1" || out[0].Location != orionRelative {
		t.Fatalf("recent = %+v, want the existing entry with the reported (relative) location", out[0])
	}
}

func TestBanksAgree_OrionAbsoluteAndRelativeAgree(t *testing.T) {
	preset := func(location string) models.ServicePreset {
		var p models.ServicePreset
		p.ButtonNumber = "1"
		p.Source = "LOCAL_INTERNET_RADIO"
		p.Location = location
		p.ContentItemType = "stationurl"

		return p
	}

	if !banksAgree([]models.ServicePreset{preset(orionAbsolute)}, []models.ServicePreset{preset(orionRelative)}) {
		t.Fatal("banks holding one Orion station in both spellings must agree")
	}

	if banksAgree([]models.ServicePreset{preset(orionRelative)}, []models.ServicePreset{preset("/station?data=other")}) {
		t.Fatal("banks holding different Orion stations must not agree")
	}
}
