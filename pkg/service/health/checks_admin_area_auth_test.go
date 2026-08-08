package health

import "testing"

func TestAdminAreaAuthCheck_NoFindingWhenDecided(t *testing.T) {
	for _, mode := range []string{"enabled", "disabled"} {
		got := runAdminAreaAuthCheck(func() string { return mode })
		if len(got) != 0 {
			t.Errorf("mode=%q: expected no findings once decided, got %+v", mode, got)
		}
	}
}

func TestAdminAreaAuthCheck_NudgesWhenUnset(t *testing.T) {
	got := runAdminAreaAuthCheck(func() string { return "" })
	if len(got) != 1 {
		t.Fatalf("expected one finding for the unset default, got %+v", got)
	}

	if got[0].Severity != SeverityInfo {
		t.Errorf("expected SeverityInfo (visibility-only nudge, not a gate), got %v", got[0].Severity)
	}
}
