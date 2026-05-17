package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

// captureStdout runs fn and returns whatever it wrote to os.Stdout.
// renderSourceTable prints directly via fmt.Print* — this lets us assert
// on its output without restructuring the renderer to take an io.Writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	os.Stdout = w

	done := make(chan struct{})
	buf := &bytes.Buffer{}

	go func() {
		_, _ = io.Copy(buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()

	os.Stdout = orig
	<-done

	return buf.String()
}

func TestRenderSourceTable_AlignsColumnsAndDedupsDisplayName(t *testing.T) {
	items := []models.SourceItem{
		// displayName != account → kept as "AUX (AUX IN)"
		{Source: "AUX", SourceAccount: "AUX", DisplayName: "AUX IN", Status: "READY", IsLocal: true, MultiroomAllowed: true},
		// displayName == account → dropped (would otherwise duplicate the next column)
		{Source: "AMAZON", SourceAccount: "amzn1.account.AFKTQOUNVZL7ODQCF4STPAAMVMPA", DisplayName: "amzn1.account.AFKTQOUNVZL7ODQCF4STPAAMVMPA", Status: "READY", MultiroomAllowed: true},
		// No displayName at all, no account
		{Source: "BLUETOOTH", Status: "UNAVAILABLE", IsLocal: true, MultiroomAllowed: true},
		// Long source name, no catalog entry → provider#?
		{Source: "STORED_MUSIC_MEDIA_RENDERER", SourceAccount: "StoredMusicUserName", DisplayName: "StoredMusicUserName", Status: "UNAVAILABLE", MultiroomAllowed: true},
	}

	out := captureStdout(t, func() { renderSourceTable(items) })

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d output lines, want 4:\n%s", len(lines), out)
	}

	// (1) AUX keeps "(AUX IN)" because it differs from both source and account.
	if !strings.Contains(lines[0], "AUX (AUX IN)") {
		t.Errorf("AUX line should keep displayName parenthesis: %q", lines[0])
	}

	// (2) AMAZON drops "(amzn1…)" because displayName equals sourceAccount.
	if strings.Contains(lines[1], "(amzn1.account") {
		t.Errorf("AMAZON line should drop displayName when it duplicates account: %q", lines[1])
	}

	// (3) provider#? for the uncatalogued source.
	if !strings.Contains(lines[3], "provider#?") {
		t.Errorf("uncatalogued source should be tagged provider#?: %q", lines[3])
	}

	// (4) Column starts must align across all rows — find the column index
	//     where "status=" appears in each line; they should all match.
	statusCols := make([]int, len(lines))
	for i, l := range lines {
		statusCols[i] = strings.Index(l, "status=")
		if statusCols[i] < 0 {
			t.Fatalf("line %d missing status= column: %q", i, l)
		}
	}

	for i := 1; i < len(statusCols); i++ {
		if statusCols[i] != statusCols[0] {
			t.Errorf("status= column misaligned: line 0 at col %d, line %d at col %d\n%s",
				statusCols[0], i, statusCols[i], out)
		}
	}

	// (5) account= column should likewise align across all rows.
	accountCols := make([]int, len(lines))
	for i, l := range lines {
		accountCols[i] = strings.Index(l, "account=")
		if accountCols[i] < 0 {
			t.Fatalf("line %d missing account= column: %q", i, l)
		}
	}

	for i := 1; i < len(accountCols); i++ {
		if accountCols[i] != accountCols[0] {
			t.Errorf("account= column misaligned: line 0 at col %d, line %d at col %d\n%s",
				accountCols[0], i, accountCols[i], out)
		}
	}
}

func TestRenderSourceTable_EmptyShowsNonePlaceholder(t *testing.T) {
	out := captureStdout(t, func() { renderSourceTable(nil) })
	if !strings.Contains(out, "(none)") {
		t.Errorf("expected (none) placeholder for empty list, got: %q", out)
	}
}

func TestRecommendMigrationMethod_PrefersTelnet(t *testing.T) {
	method, reason := recommendMigrationMethod("http://aftertouch.local:8000", &setup.MigrationSummary{
		TelnetReachable: true,
		SSHSuccess:      true,
	})

	if method != setup.MigrationMethodTelnet {
		t.Errorf("method = %q, want telnet (simplest path when telnet works)", method)
	}

	if !strings.Contains(reason, "Telnet") {
		t.Errorf("reason should mention Telnet: %q", reason)
	}
}

func TestRecommendMigrationMethod_HTTPSAddsCaveatToTelnet(t *testing.T) {
	_, reason := recommendMigrationMethod("https://aftertouch.local:8443", &setup.MigrationSummary{
		TelnetReachable: true,
	})

	if !strings.Contains(reason, "install-ca") {
		t.Errorf("HTTPS service URL should flag the CA-install caveat in the reason: %q", reason)
	}
}

func TestRecommendMigrationMethod_FallsBackToResolvWhenTelnetDown(t *testing.T) {
	method, _ := recommendMigrationMethod("http://aftertouch.local:8000", &setup.MigrationSummary{
		TelnetReachable: false,
		SSHSuccess:      true,
	})

	if method != setup.MigrationMethodResolvConf {
		t.Errorf("method = %q, want resolv (DNS redirect via SSH)", method)
	}
}

func TestRecommendMigrationMethod_EmptyWhenNoTransport(t *testing.T) {
	method, _ := recommendMigrationMethod("http://aftertouch.local:8000", &setup.MigrationSummary{
		TelnetReachable: false,
		SSHSuccess:      false,
	})

	if method != "" {
		t.Errorf("method = %q, want empty when no transport works", method)
	}
}

func TestBuildPlanSteps_NoOpWhenAlreadyMigratedAndPaired(t *testing.T) {
	summary := &setup.MigrationSummary{IsMigrated: true, IsPaired: true, TelnetMigrated: true}
	inspect := &setup.InspectReport{Info: &setup.DeviceInfoXML{DeviceID: "AABBCCDDEEFF"}}

	steps := buildPlanSteps("192.0.2.42", "http://aftertouch.local:8000", "", true, false, inspect, summary)

	if len(steps) != 0 {
		t.Errorf("expected no steps for fully-set-up device, got %d:\n%v", len(steps), steps)
	}
}

func TestBuildPlanSteps_RecommendsPairWhenMigratedButUnpaired(t *testing.T) {
	summary := &setup.MigrationSummary{IsMigrated: true, IsPaired: false, TelnetMigrated: true, TelnetReachable: true}
	inspect := &setup.InspectReport{Info: &setup.DeviceInfoXML{DeviceID: "AABBCCDDEEFF"}}

	steps := buildPlanSteps("192.0.2.42", "http://aftertouch.local:8000", "", true, false, inspect, summary)

	if len(steps) != 1 {
		t.Fatalf("expected exactly the pair step, got %d:\n%v", len(steps), steps)
	}

	if !strings.Contains(steps[0].cmd, "setup pair") {
		t.Errorf("expected pair command, got %q", steps[0].cmd)
	}
}

func TestBuildPlanSteps_MigrateRebootThenPairWhenFresh(t *testing.T) {
	summary := &setup.MigrationSummary{TelnetReachable: true, SSHSuccess: false, IsPaired: false}
	inspect := &setup.InspectReport{Info: &setup.DeviceInfoXML{DeviceID: "AABBCCDDEEFF"}}

	steps := buildPlanSteps("192.0.2.42", "http://aftertouch.local:8000", "", true, false, inspect, summary)

	// migrate → reboot → pair. The reboot step exists because envswitch's
	// parallel-persistence layer only fully wins on the next boot, and we
	// want the new URLs locked in before pairing posts to the speaker.
	if len(steps) != 3 {
		t.Fatalf("expected migrate+reboot+pair, got %d steps:\n%v", len(steps), steps)
	}

	if !strings.Contains(steps[0].cmd, "setup migrate") || !strings.Contains(steps[0].cmd, "method=telnet") {
		t.Errorf("step 1 should be telnet migrate, got %q", steps[0].cmd)
	}

	if !strings.Contains(steps[1].cmd, "setup reboot") {
		t.Errorf("step 2 should be reboot, got %q", steps[1].cmd)
	}

	if !strings.Contains(steps[2].cmd, "setup pair") {
		t.Errorf("step 3 should be pair, got %q", steps[2].cmd)
	}
}

func TestBuildPlanSteps_DNSMethodPrependsCAInstall(t *testing.T) {
	// Telnet down, SSH up, CA not yet trusted → plan must install-ca
	// before applying the resolv migration.
	summary := &setup.MigrationSummary{
		TelnetReachable: false,
		SSHSuccess:      true,
		CACertTrusted:   false,
		IsPaired:        false,
	}
	inspect := &setup.InspectReport{Info: &setup.DeviceInfoXML{DeviceID: "X"}}

	steps := buildPlanSteps("192.0.2.42", "http://aftertouch.local:8000", "", false, false, inspect, summary)

	if len(steps) < 2 {
		t.Fatalf("expected at least install-ca + migrate, got %d steps:\n%v", len(steps), steps)
	}

	if !strings.Contains(steps[0].cmd, "install-ca") {
		t.Errorf("install-ca should come first when DNS method is chosen and CA is not trusted, got %q", steps[0].cmd)
	}

	if !strings.Contains(steps[1].cmd, "method=resolv") {
		t.Errorf("step 2 should be resolv migrate, got %q", steps[1].cmd)
	}
}

func TestBuildPlanSteps_ResetModeIncludesManualNetworkSwitches(t *testing.T) {
	inspect := &setup.InspectReport{
		Info: &setup.DeviceInfoXML{DeviceID: "506583DE4803"},
		Network: &models.NetworkInformation{
			Interfaces: models.NetworkInterfaces{
				Interfaces: []models.NetworkInterface{
					{Type: "WIFI_INTERFACE", SSID: "MyHomeNetwork"},
				},
			},
		},
	}
	summary := &setup.MigrationSummary{IsMigrated: true, IsPaired: true} // doesn't matter in reset mode

	steps := buildPlanSteps("192.0.2.42", "http://aftertouch.local:8000", "", true, true, inspect, summary)

	// Expected sequence in --reset mode:
	//   factory-reset, manual AP switch, wait-ap, wifi-push, manual home switch,
	//   wait-online, migrate, pair (8 steps).
	if len(steps) < 7 {
		t.Fatalf("expected at least 7 steps in --reset mode, got %d:\n%v", len(steps), steps)
	}

	manualCount := 0
	for _, s := range steps {
		if s.manual {
			manualCount++
		}
	}

	if manualCount < 2 {
		t.Errorf("expected at least 2 manual steps for the Wi-Fi switches, got %d", manualCount)
	}

	if !strings.Contains(steps[0].cmd, "factory-reset") {
		t.Errorf("step 1 must be factory-reset, got %q", steps[0].cmd)
	}

	// wifi-push step should default to the inspected SSID
	foundWiFi := false

	for _, s := range steps {
		if strings.Contains(s.cmd, "wifi-push") && strings.Contains(s.cmd, "MyHomeNetwork") {
			foundWiFi = true
			break
		}
	}

	if !foundWiFi {
		t.Errorf("expected wifi-push step to default to inspected SSID 'MyHomeNetwork'")
	}

	// wait-online --match should use the deviceID suffix
	foundMatch := false

	for _, s := range steps {
		if strings.Contains(s.cmd, "wait-online") && strings.Contains(s.cmd, "--match=DE4803") {
			foundMatch = true
			break
		}
	}

	if !foundMatch {
		t.Errorf("expected wait-online step to use --match=DE4803 from deviceID suffix")
	}
}
