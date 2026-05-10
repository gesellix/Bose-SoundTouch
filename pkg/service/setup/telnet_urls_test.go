package setup

import (
	"reflect"
	"strings"
	"testing"
)

func TestDefaultTelnetURLs_DerivesAllFourFromBase(t *testing.T) {
	got := defaultTelnetURLs("http://example:8000")

	want := telnetURLs{
		Marge:       "http://example:8000",
		Stats:       "http://example:8000",
		SwUpdate:    "http://example:8000/updates/soundtouch",
		BmxRegistry: "http://example:8000/bmx/registry/v1/services",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("defaultTelnetURLs = %+v, want %+v", got, want)
	}
}

func TestTelnetURLsFromOptions_NilOptionsReturnsDefaults(t *testing.T) {
	got := telnetURLsFromOptions("http://example:8000", nil)
	want := defaultTelnetURLs("http://example:8000")

	if !reflect.DeepEqual(got, want) {
		t.Errorf("telnetURLsFromOptions(nil) = %+v, want defaults %+v", got, want)
	}
}

func TestTelnetURLsFromOptions_EmptyValueFallsBackToDefault(t *testing.T) {
	options := map[string]string{
		"marge_url": "", // empty override should be ignored
	}

	got := telnetURLsFromOptions("http://example:8000", options)

	if got.Marge != "http://example:8000" {
		t.Errorf("Marge with empty override = %q, want default", got.Marge)
	}
}

func TestTelnetURLsFromOptions_PerFieldOverrides(t *testing.T) {
	options := map[string]string{
		"marge_url":     "http://example:8000/marge", // soundcork-style
		"stats_url":     "",                          // ignored
		"sw_update_url": "http://example:8000/custom/updates",
		"bmx_url":       "http://example:8000/custom/bmx",
	}

	got := telnetURLsFromOptions("http://example:8000", options)

	if got.Marge != "http://example:8000/marge" {
		t.Errorf("Marge = %q, want override", got.Marge)
	}

	if got.Stats != "http://example:8000" {
		t.Errorf("Stats = %q, want default (empty override)", got.Stats)
	}

	if got.SwUpdate != "http://example:8000/custom/updates" {
		t.Errorf("SwUpdate = %q, want override", got.SwUpdate)
	}

	if got.BmxRegistry != "http://example:8000/custom/bmx" {
		t.Errorf("BmxRegistry = %q, want override", got.BmxRegistry)
	}
}

// TestTelnetURLs_Commands_EnvswitchTracksMargeAndSwUpdate is the load-bearing
// test for the soundcork case: if the user added /marge to Marge, the
// envswitch arg1 must follow the same suffix verbatim, otherwise the
// parallel persistence layer will revert margeServerUrl on next reboot
// (the very failure mode the user described as "envswitch silently
// restores my typo").
func TestTelnetURLs_Commands_EnvswitchTracksMargeAndSwUpdate(t *testing.T) {
	urls := telnetURLs{
		Marge:       "http://example:8000/marge",
		Stats:       "http://example:8000",
		SwUpdate:    "http://example:8000/updates/soundtouch",
		BmxRegistry: "http://example:8000/bmx/registry/v1/services",
	}

	cmds := urls.Commands()

	var envswitch string

	for _, c := range cmds {
		if strings.HasPrefix(c, "envswitch boseurls set ") {
			envswitch = c
			break
		}
	}

	if envswitch == "" {
		t.Fatalf("Commands missing envswitch boseurls set:\n%v", cmds)
	}

	wantEnv := "envswitch boseurls set http://example:8000/marge http://example:8000/updates/soundtouch"
	if envswitch != wantEnv {
		t.Errorf("envswitch =\n  %q\nwant\n  %q", envswitch, wantEnv)
	}
}

func TestMigrateViaTelnet_SoundcorkMargeSuffixPropagatesToEnvswitch(t *testing.T) {
	target := "http://example:8000"
	urls := telnetURLs{
		Marge:       "http://example:8000/marge",
		Stats:       "http://example:8000",
		SwUpdate:    "http://example:8000/updates/soundtouch",
		BmxRegistry: "http://example:8000/bmx/registry/v1/services",
	}

	// Build a happy-path responder that matches the *new* command set.
	resp := map[string]string{
		"sys configuration bmxRegistryUrl " + urls.BmxRegistry:       "OK\n",
		"sys configuration statsServerUrl " + urls.Stats:             "OK\n",
		"sys configuration margeServerUrl " + urls.Marge:             "OK\n",
		"sys configuration swUpdateUrl " + urls.SwUpdate:             "OK\n",
		"envswitch boseurls set " + urls.Marge + " " + urls.SwUpdate: "OK\n",
		"getpdo CurrentSystemConfiguration":                          "margeServerUrl=" + urls.Marge + "\n",
	}

	f := &fakeTelnet{responses: resp}
	m := newFakeTelnetManager(f)

	if _, err := m.migrateViaTelnet("192.0.2.1", target, urls); err != nil {
		t.Fatalf("migrateViaTelnet: %v", err)
	}

	wantEnvCmd := "envswitch boseurls set http://example:8000/marge http://example:8000/updates/soundtouch"

	var saw bool

	for _, c := range f.commands {
		if c == wantEnvCmd {
			saw = true
			break
		}
	}

	if !saw {
		t.Errorf("never sent expected envswitch command %q\nactual commands:\n%v", wantEnvCmd, f.commands)
	}
}
