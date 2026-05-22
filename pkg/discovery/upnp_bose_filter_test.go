package discovery

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestIsBoseUPnPDevice(t *testing.T) {
	cases := []struct {
		name string
		dev  *models.DiscoveredDevice
		want bool
	}{
		{
			name: "Bose manufacturer wins",
			dev:  &models.DiscoveredDevice{Manufacturer: "Bose Corporation", ModelID: "Generic"},
			want: true,
		},
		{
			name: "SoundTouch model wins even without manufacturer",
			dev:  &models.DiscoveredDevice{Manufacturer: "", ModelID: "SoundTouch 30 sm2"},
			want: true,
		},
		{
			name: "Case-insensitive manufacturer",
			dev:  &models.DiscoveredDevice{Manufacturer: "BOSE CORP"},
			want: true,
		},
		{
			name: "LG TV rejected",
			dev:  &models.DiscoveredDevice{Manufacturer: "LG Electronics", ModelID: "OLED55G2"},
			want: false,
		},
		{
			name: "Onkyo AVR rejected",
			dev:  &models.DiscoveredDevice{Manufacturer: "Onkyo Corporation", ModelID: "HT-R695"},
			want: false,
		},
		{
			name: "Dreambox rejected",
			dev:  &models.DiscoveredDevice{Manufacturer: "Dream Multimedia", ModelID: "dm920"},
			want: false,
		},
		{
			name: "Empty fields rejected",
			dev:  &models.DiscoveredDevice{},
			want: false,
		},
		{
			name: "Nil rejected",
			dev:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBoseUPnPDevice(tc.dev); got != tc.want {
				t.Errorf("got %v, want %v (dev=%+v)", got, tc.want, tc.dev)
			}
		})
	}
}

func TestIsSoundTouchServiceName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"Bose-Wohnzimmer._soundtouch._tcp.local.", true},
		{"SoundTouch-Stick._soundtouchstick._tcp.local.", true},
		{"NewSpeaker._bose-soundtouch._tcp.local.", true},
		{"PrinterA._ipp._tcp.local.", false},
		{"TV._smarttv._tcp.local.", false},
		{"", false},
		// Case-insensitive: firmware might emit mixed-case
		{"Speaker._SoundTouch._tcp.local.", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSoundTouchServiceName(tc.name); got != tc.want {
				t.Errorf("isSoundTouchServiceName(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
