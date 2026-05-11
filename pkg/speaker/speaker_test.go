package speaker

import "testing"

// Sanity-check the well-known values — a wrong number here would silently
// break every transport and is cheap to guard against.
func TestSpeakerConstants(t *testing.T) {
	if HTTPPort != 8090 {
		t.Errorf("HTTPPort = %d, want 8090", HTTPPort)
	}

	cases := map[string]string{
		"DeviceInfoPath":           DeviceInfoPath,
		"PresetsPath":              PresetsPath,
		"RecentsPath":              RecentsPath,
		"SourcesFileLocation":      SourcesFileLocation,
		"GroupServiceFileLocation": GroupServiceFileLocation,
	}

	for name, val := range cases {
		if val == "" {
			t.Errorf("%s is empty", name)
		}
	}
}
