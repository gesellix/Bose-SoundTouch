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

func TestRevertTelnetURLs_DefaultsAndCommandOrder(t *testing.T) {
	urls := telnetURLs{
		Marge:       "https://streaming.bose.com",
		Stats:       "https://events.api.bosecm.com",
		SwUpdate:    "https://worldwide.bose.com/updates/soundtouch",
		BmxRegistry: "https://content.api.bose.io/bmx/registry/v1/services",
	}
	f := &fakeTelnet{responses: telnetResponses(urls, flatGetpdoResponse(urls))}
	m := newFakeTelnetManager(f)

	logs, err := m.RevertTelnetURLs("192.0.2.1", nil)
	if err != nil {
		t.Fatalf("RevertTelnetURLs: %v", err)
	}

	wantCommands := append(urls.Commands(), "getpdo CurrentSystemConfiguration")
	if !reflect.DeepEqual(f.commands, wantCommands) {
		t.Errorf("commands =\n%v\nwant\n%v", f.commands, wantCommands)
	}

	if !strings.Contains(logs, "URL configuration only") {
		t.Errorf("logs do not limit the operation to URL configuration:\n%s", logs)
	}
}

func TestRevertTelnetURLs_OverridesTakePrecedence(t *testing.T) {
	options := map[string]string{
		"marge_url":     "https://override.example/marge",
		"stats_url":     "https://override.example/stats",
		"sw_update_url": "https://override.example/update",
		"bmx_url":       "https://override.example/bmx",
	}
	want := telnetURLs{
		Marge:       options["marge_url"],
		Stats:       options["stats_url"],
		SwUpdate:    options["sw_update_url"],
		BmxRegistry: options["bmx_url"],
	}
	f := &fakeTelnet{responses: telnetResponses(want, flatGetpdoResponse(want))}
	m := newFakeTelnetManager(f)

	if _, err := m.RevertTelnetURLs("192.0.2.1", options); err != nil {
		t.Fatalf("RevertTelnetURLs: %v", err)
	}

	wantCommands := append(want.Commands(), "getpdo CurrentSystemConfiguration")
	if !reflect.DeepEqual(f.commands, wantCommands) {
		t.Errorf("commands =\n%v\nwant overrides\n%v", f.commands, wantCommands)
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
	urls := telnetURLs{
		Marge:       "http://example:8000/marge",
		Stats:       "http://example:8000",
		SwUpdate:    "http://example:8000/updates/soundtouch",
		BmxRegistry: "http://example:8000/bmx/registry/v1/services",
	}

	f := &fakeTelnet{responses: telnetResponses(urls, flatGetpdoResponse(urls))}
	m := newFakeTelnetManager(f)

	if _, err := m.migrateViaTelnet("192.0.2.1", urls); err != nil {
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
