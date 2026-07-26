package health

import "testing"

func TestSpotifyAccountLinkedCheck_NoFindingWhenNotConfigured(t *testing.T) {
	got := runSpotifyAccountLinkedCheck(false, 0)
	if len(got) != 0 {
		t.Errorf("expected no findings when Spotify isn't configured, got %+v", got)
	}
}

func TestSpotifyAccountLinkedCheck_NoFindingWhenLinked(t *testing.T) {
	got := runSpotifyAccountLinkedCheck(true, 1)
	if len(got) != 0 {
		t.Errorf("expected no findings when an account is linked, got %+v", got)
	}
}

func TestSpotifyAccountLinkedCheck_WarnsWhenConfiguredButUnlinked(t *testing.T) {
	got := runSpotifyAccountLinkedCheck(true, 0)
	if len(got) != 1 {
		t.Fatalf("expected one finding for configured-but-unlinked, got %+v", got)
	}

	if got[0].Severity != SeverityWarning {
		t.Errorf("expected SeverityWarning, got %v", got[0].Severity)
	}
}
