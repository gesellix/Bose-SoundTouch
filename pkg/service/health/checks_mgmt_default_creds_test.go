package health

import "testing"

func TestMgmtDefaultCredentialsCheck_NoFindingWhenChanged(t *testing.T) {
	for _, tc := range []struct{ username, password string }{
		{"admin", "somethingElse"},
		{"someoneElse", "change_me!"},
		{"customadmin", "customsecret"},
	} {
		got := runMgmtDefaultCredentialsCheck(tc.username, tc.password)
		if len(got) != 0 {
			t.Errorf("expected no findings for %+v, got %+v", tc, got)
		}
	}
}

func TestMgmtDefaultCredentialsCheck_WarnsWhenStillDefault(t *testing.T) {
	got := runMgmtDefaultCredentialsCheck(DefaultMgmtUsername, DefaultMgmtPassword)
	if len(got) != 1 {
		t.Fatalf("expected one finding for unchanged default credentials, got %+v", got)
	}

	if got[0].Severity != SeverityInfo {
		t.Errorf("expected SeverityInfo (visibility-only nudge, not a gate), got %v", got[0].Severity)
	}
}
