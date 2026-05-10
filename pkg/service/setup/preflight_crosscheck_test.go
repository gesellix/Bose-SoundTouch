package setup

import (
	"strings"
	"testing"
)

func TestParseGetpdoConfig_StandardLines(t *testing.T) {
	in := "margeServerUrl=http://example:8000\nbmxRegistryUrl=http://example:8000/bmx/registry/v1/services\n"

	got := parseGetpdoConfig(in)

	if got["margeServerUrl"] != "http://example:8000" {
		t.Errorf("margeServerUrl = %q, want http://example:8000", got["margeServerUrl"])
	}

	if got["bmxRegistryUrl"] != "http://example:8000/bmx/registry/v1/services" {
		t.Errorf("bmxRegistryUrl = %q", got["bmxRegistryUrl"])
	}
}

func TestParseGetpdoConfig_TolerantToNoise(t *testing.T) {
	in := "BoseShell\n-> getpdo CurrentSystemConfiguration\nmargeServerUrl=http://example:8000\nrandom line without equals\n  statsServerUrl  =  http://example:8000  \n-> "

	got := parseGetpdoConfig(in)

	if got["margeServerUrl"] != "http://example:8000" {
		t.Errorf("margeServerUrl = %q, want http://example:8000", got["margeServerUrl"])
	}

	if got["statsServerUrl"] != "http://example:8000" {
		t.Errorf("statsServerUrl = %q, want trimmed http://example:8000", got["statsServerUrl"])
	}

	if _, exists := got["random line without equals"]; exists {
		t.Errorf("non-key=value line should not be parsed")
	}
}

// TestParseGetpdoConfig_ProtobufTextRealDevice pins the parser to the
// live response captured from a SoundTouch 20 (FW 27.0.6.46330.5043500)
// against http://mac.fritz.box:8000/setup/summary. This is the format
// the parser actually has to handle in production — the prior
// key=value-only implementation returned an empty map for this input,
// which surfaced as empty "Current on Device" cells in the migration
// UI.
func TestParseGetpdoConfig_ProtobufTextRealDevice(t *testing.T) {
	in := `margeServerUrl {
  text: "https://streaming.bose.com"
}
statsServerUrl {
  text: "https://events.api.bosecm.com"
}
swUpdateUrl {
  text: "https://worldwide.bose.com/updates/soundtouch"
}
isZeroconfEnabled {
  text: true
}
usePandoraProductionServer {
  text: true
}
saveMargeCustomerReport {
  text: false
}
bmxRegistryUrl {
  text: "https://content.api.bose.io/bmx/registry/v1/services"
}

->OK
->`

	got := parseGetpdoConfig(in)

	want := map[string]string{
		"margeServerUrl":             "https://streaming.bose.com",
		"statsServerUrl":             "https://events.api.bosecm.com",
		"swUpdateUrl":                "https://worldwide.bose.com/updates/soundtouch",
		"bmxRegistryUrl":             "https://content.api.bose.io/bmx/registry/v1/services",
		"isZeroconfEnabled":          "true",
		"usePandoraProductionServer": "true",
		"saveMargeCustomerReport":    "false",
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestCrossCheckPreflights_AgreementProducesNoWarnings(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"}

	summary := &MigrationSummary{
		ParsedCurrentConfig: &PrivateCfg{
			MargeServerUrl: "http://example:8000",
			StatsServerUrl: "http://example:8000",
			SwUpdateUrl:    "http://example:8000/updates/soundtouch",
			BmxRegistryUrl: "http://example:8000/bmx/registry/v1/services",
		},
		TelnetVerifiedConfig: "margeServerUrl=http://example:8000\n" +
			"statsServerUrl=http://example:8000\n" +
			"swUpdateUrl=http://example:8000/updates/soundtouch\n" +
			"bmxRegistryUrl=http://example:8000/bmx/registry/v1/services\n",
	}

	m.crossCheckPreflights(summary)

	if len(summary.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none when both transports agree", summary.Warnings)
	}
}

func TestCrossCheckPreflights_MismatchProducesWarning(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"}

	// SSH-XML still shows the original cloud URL (envswitch wrote the
	// runtime layer but the on-device file hasn't been re-rendered).
	summary := &MigrationSummary{
		ParsedCurrentConfig: &PrivateCfg{
			MargeServerUrl: "https://streaming.bose.com",
		},
		TelnetVerifiedConfig: "margeServerUrl=http://example:8000\n",
	}

	m.crossCheckPreflights(summary)

	if len(summary.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one warning", summary.Warnings)
	}

	w := summary.Warnings[0]

	if !strings.Contains(w, "margeServerUrl") {
		t.Errorf("warning %q should name the field", w)
	}

	if !strings.Contains(w, "streaming.bose.com") || !strings.Contains(w, "example:8000") {
		t.Errorf("warning %q should quote both values", w)
	}
}

func TestCrossCheckPreflights_NoWarningWhenTelnetMissesField(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"}

	summary := &MigrationSummary{
		ParsedCurrentConfig: &PrivateCfg{
			MargeServerUrl: "http://example:8000",
			StatsServerUrl: "http://example:8000",
		},
		// getpdo only echoes margeServerUrl — statsServerUrl is silently
		// absent on this firmware. Absence is not a disagreement.
		TelnetVerifiedConfig: "margeServerUrl=http://example:8000\n",
	}

	m.crossCheckPreflights(summary)

	if len(summary.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none when a field is missing from one transport", summary.Warnings)
	}
}

func TestCrossCheckPreflights_OnlyOneTransportPresent(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"}

	t.Run("telnet only", func(t *testing.T) {
		summary := &MigrationSummary{
			TelnetVerifiedConfig: "margeServerUrl=http://example:8000\n",
		}
		m.crossCheckPreflights(summary)
		if len(summary.Warnings) != 0 {
			t.Errorf("Warnings = %v, want none when SSH didn't read the XML", summary.Warnings)
		}
	})

	t.Run("ssh only", func(t *testing.T) {
		summary := &MigrationSummary{
			ParsedCurrentConfig: &PrivateCfg{MargeServerUrl: "http://example:8000"},
		}
		m.crossCheckPreflights(summary)
		if len(summary.Warnings) != 0 {
			t.Errorf("Warnings = %v, want none when telnet didn't respond", summary.Warnings)
		}
	})
}
