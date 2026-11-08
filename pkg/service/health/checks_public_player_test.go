package health

import "testing"

func TestPublicNetworkPlayerCheck_GH663(t *testing.T) {
	if got := runPublicNetworkPlayerCheck(false); got != nil {
		t.Errorf("closed player: findings = %+v, want none", got)
	}

	got := runPublicNetworkPlayerCheck(true)
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Errorf("open player: findings = %+v, want one warning", got)
	}
}
