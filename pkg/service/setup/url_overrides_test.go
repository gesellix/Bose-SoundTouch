package setup

import (
	"errors"
	"strings"
	"testing"
)

func TestApplyURLOverrides_NilSafety(t *testing.T) {
	// Should not panic on nil cfg or nil options.
	applyURLOverrides(nil, map[string]string{"marge_url": "x"})

	cfg := &PrivateCfg{}
	applyURLOverrides(cfg, nil)
}

func TestApplyURLOverrides_EmptyValueIsIgnored(t *testing.T) {
	cfg := &PrivateCfg{
		MargeServerUrl: "http://example:8000",
	}
	applyURLOverrides(cfg, map[string]string{"marge_url": ""})

	if cfg.MargeServerUrl != "http://example:8000" {
		t.Errorf("MargeServerUrl was overwritten by empty override: %q", cfg.MargeServerUrl)
	}
}

func TestApplyURLOverrides_AllFour(t *testing.T) {
	cfg := &PrivateCfg{
		MargeServerUrl: "default-marge",
		StatsServerUrl: "default-stats",
		SwUpdateUrl:    "default-sw",
		BmxRegistryUrl: "default-bmx",
	}

	applyURLOverrides(cfg, map[string]string{
		"marge_url":     "http://example:8000/marge",
		"stats_url":     "http://example:8000",
		"sw_update_url": "http://example:8000/updates/soundtouch",
		"bmx_url":       "http://example:8000/bmx/registry/v1/services",
	})

	if cfg.MargeServerUrl != "http://example:8000/marge" {
		t.Errorf("MargeServerUrl = %q", cfg.MargeServerUrl)
	}

	if cfg.StatsServerUrl != "http://example:8000" {
		t.Errorf("StatsServerUrl = %q", cfg.StatsServerUrl)
	}

	if cfg.SwUpdateUrl != "http://example:8000/updates/soundtouch" {
		t.Errorf("SwUpdateUrl = %q", cfg.SwUpdateUrl)
	}

	if cfg.BmxRegistryUrl != "http://example:8000/bmx/registry/v1/services" {
		t.Errorf("BmxRegistryUrl = %q", cfg.BmxRegistryUrl)
	}
}

// TestApplyURLOverrides_OverridesProxiedMode locks in the precedence
// rule: a literal *_url override wins over a self/proxied/original
// mode set on the same field. This is the load-bearing behaviour for
// the unified per-field URL editor in the Plan card — the user picked
// a URL and the migration honors it verbatim.
func TestApplyURLOverrides_OverridesProxiedMode(t *testing.T) {
	m := &Manager{}

	cfg := &PrivateCfg{
		MargeServerUrl: "http://example:8000", // canonical default
	}

	currentCfg := &PrivateCfg{
		MargeServerUrl: "https://streaming.bose.com",
	}

	options := map[string]string{
		"marge":     "proxied", // legacy mode
		"marge_url": "http://example:8000/marge",
	}

	m.applyProxyOptions(cfg, "http://proxy:8000", options, currentCfg)
	applyURLOverrides(cfg, options)

	if cfg.MargeServerUrl != "http://example:8000/marge" {
		t.Errorf("MargeServerUrl = %q, want literal override (not the /proxy/… form)", cfg.MargeServerUrl)
	}
}

// TestGetMigrationSummary_HonorsURLOverridesInPlannedConfig drives the
// PlannedConfig diff back from a real GetMigrationSummary to confirm
// the user's per-field URL overrides reach the planned XML the UI
// shows — closing the loop between the Plan card editor and the
// preview pane.
func TestGetMigrationSummary_HonorsURLOverridesInPlannedConfig(t *testing.T) {
	m, host, cleanup := telnetSummaryEnv(t, nil, &fakeTelnet{dialErr: errors.New("not the focus of this test")})
	defer cleanup()

	options := map[string]string{
		"marge_url": "http://example:8000/marge",
	}

	summary, err := m.GetMigrationSummary(host, "http://example:8000", "", options)
	if err != nil {
		t.Fatalf("GetMigrationSummary: %v", err)
	}

	if !strings.Contains(summary.PlannedConfig, "<margeServerUrl>http://example:8000/marge</margeServerUrl>") {
		t.Errorf("PlannedConfig should reflect marge_url override:\n%s", summary.PlannedConfig)
	}
}
