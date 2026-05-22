package health

import (
	"strings"
	"testing"
)

func TestOAuthTargetCheck_NoFindingForHostnameServerURL(t *testing.T) {
	dnsRunning := func() (bool, string) { return true, ":53" }

	got := runOAuthTargetReachableCheck("https://aftertouch.lan:8443", dnsRunning)
	if len(got) != 0 {
		t.Errorf("expected no findings for hostname-based serverURL, got %+v", got)
	}
}

func TestOAuthTargetCheck_WarnsForIPv4ServerURL(t *testing.T) {
	dnsRunning := func() (bool, string) { return true, ":53" }

	got := runOAuthTargetReachableCheck("https://192.168.0.30:8443", dnsRunning)
	if len(got) != 1 {
		t.Fatalf("expected one finding for IP-based serverURL, got %+v", got)
	}

	if got[0].Severity != SeverityWarning {
		t.Errorf("expected SeverityWarning, got %v", got[0].Severity)
	}

	if !strings.Contains(got[0].Message, "192oauth.168.0.30") {
		t.Errorf("expected the malformed example host in the message, got %q", got[0].Message)
	}

	if len(got[0].ManualCommands) == 0 {
		t.Errorf("expected at least one ManualCommand pointing at the fix")
	}
}

func TestOAuthTargetCheck_HintReflectsDNSRunningState(t *testing.T) {
	dnsRunning := func() (bool, string) { return false, "" }

	got := runOAuthTargetReachableCheck("https://10.0.0.5:8443", dnsRunning)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}

	if !strings.Contains(got[0].Details, "DNS hijack server isn't currently running") {
		t.Errorf("expected DNS-not-running fallback hint in Details, got %q", got[0].Details)
	}
}

func TestOAuthTargetCheck_EmptyOrUnparseableIsNoOp(t *testing.T) {
	dnsRunning := func() (bool, string) { return true, ":53" }

	for _, url := range []string{"", "  ", ":::not a url"} {
		got := runOAuthTargetReachableCheck(url, dnsRunning)
		if len(got) != 0 {
			t.Errorf("expected no findings for %q, got %+v", url, got)
		}
	}
}

func TestExampleMalformedOAuthHost(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"192.168.0.30", "192oauth.168.0.30"},
		{"10.0.0.5", "10oauth.0.0.5"},
		{"aftertouch", "aftertouchoauth"},
	}

	for _, c := range cases {
		if got := exampleMalformedOAuthHost(c.in); got != c.want {
			t.Errorf("exampleMalformedOAuthHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
