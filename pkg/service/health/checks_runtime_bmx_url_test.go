package health

import "testing"

func TestRuntimeBmxURL_WarnsWhenStillOnBoseCloud(t *testing.T) {
	got := assessRuntimeBmxURL("1000001", "DEVICEID01", "192.0.2.10",
		"https://content.api.bose.io/bmx/registry/v1/services")
	if len(got) != 1 {
		t.Fatalf("expected 1 finding for a Bose-cloud bmx URL, got %d: %+v", len(got), got)
	}

	f := got[0]
	if f.Severity != SeverityWarning {
		t.Errorf("expected SeverityWarning, got %q", f.Severity)
	}
	if f.Target.Account != "1000001" || f.Target.Device != "DEVICEID01" {
		t.Errorf("unexpected target: %+v", f.Target)
	}
	if len(f.ManualCommands) != 1 {
		t.Fatalf("expected a re-migrate manual command, got %+v", f.ManualCommands)
	}
}

func TestRuntimeBmxURL_NoFindingWhenPointingAtAfterTouch(t *testing.T) {
	got := assessRuntimeBmxURL("1000001", "DEVICEID01", "192.0.2.10",
		"http://192.0.2.10:8000/bmx/registry/v1/services")
	if len(got) != 0 {
		t.Errorf("expected no findings for an AfterTouch bmx URL, got %+v", got)
	}
}

func TestRuntimeBmxURL_NoFindingWhenEmpty(t *testing.T) {
	if got := assessRuntimeBmxURL("1000001", "DEVICEID01", "192.0.2.10", ""); len(got) != 0 {
		t.Errorf("expected no findings for an empty bmx URL, got %+v", got)
	}
}

func TestIsBoseCloudHost(t *testing.T) {
	cloud := []string{
		"content.api.bose.io",
		"streaming.bose.com",
		"events.api.bosecm.com",
		"worldwide.bose.com",
		"bose.com",
	}
	for _, h := range cloud {
		if !isBoseCloudHost(h) {
			t.Errorf("expected %q to be a Bose cloud host", h)
		}
	}

	local := []string{
		"aftertouch.local",
		"192.0.2.10",
		"",
		"notbose.example.com",
		"bose.io.evil.example.com",
	}
	for _, h := range local {
		if isBoseCloudHost(h) {
			t.Errorf("expected %q NOT to be a Bose cloud host", h)
		}
	}
}
