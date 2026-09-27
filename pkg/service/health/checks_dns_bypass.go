package health

import (
	"fmt"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDDNSBypassRisk is the registry ID of the check that warns when the
// operator has DNS interception ("Enable DNS Discovery Server" in Settings)
// turned on, but a specific speaker shows positive evidence of resolving
// Bose hostnames through its own resolver instead of through AfterTouch.
//
// HandleBMXRegistry (pkg/service/handlers/handlers_bmx.go) keys
// {BMX_SERVER} off the DNS-enabled *setting* alone, not off whether the DNS
// listener is actually reachable from any given speaker's network path. So
// a speaker that was never DNS-migrated (no aftertouch.resolv.conf hook, no
// AfterTouch nameserver in /etc/resolv.conf) still gets handed
// "https://content.api.bose.io" for TuneIn and the other BMX-delivered
// services, the shut-down Bose cloud. That is exactly what happened in
// issue #728: a SoundTouch 30 with DNS Discovery on, but never migrated,
// hit BMX_HTTP_ERROR 4501 (Apigee "ApplicationNotFound") on every TuneIn
// request, and nothing in the Health tab flagged it at the time.
const CheckIDDNSBypassRisk = "dns_bypass_risk"

// dnsBypassTroubleshootingURL is the anchor for the dedicated
// troubleshooting entry added alongside this check.
const dnsBypassTroubleshootingURL = "https://gesellix.github.io/Bose-SoundTouch/docs/guides/TROUBLESHOOTING/#tunein-4501-dns"

// DNSEnabledFunc reports whether the operator has turned on "Enable DNS
// Discovery Server" in Settings. This is deliberately independent of
// DNSStatusFunc (whether the DNS listener actually bound to a port):
// HandleBMXRegistry reads the persisted *setting*, not the listener's
// runtime state, when deciding whether to hand out the Bose-cloud BMX base
// URL, so the setting, not the listener state, is what actually gates the
// risk this check reports on.
type DNSEnabledFunc func() bool

// SpeakerDNSResolutionFunc probes a single speaker's DNS configuration over
// SSH in one round trip and reports whether it currently resolves through
// AfterTouch.
//
// sshOK is false when SSH is unreachable or unauthenticated; callers must
// then treat usesAfterTouch and hookInstalled as undefined, not as "false",
// SSH being unavailable is not evidence of a bypass.
//
// usesAfterTouch means the speaker's /etc/resolv.conf currently lists
// expectedIP as a nameserver. hookInstalled means the AfterTouch
// DNS-migration hook file is present on the speaker (either the current
// /mnt/nv/soundtouch-service/aftertouch.resolv.conf or the legacy
// /mnt/nv/aftertouch.resolv.conf, see migrateViaResolvConf in
// pkg/service/setup/setup.go). The hook file is checked in addition to the
// live resolv.conf because the udhcpc hook only re-applies the AfterTouch
// nameserver on DHCP renewal; a speaker that hasn't renewed its lease since
// migrating can have a hook installed while /etc/resolv.conf still shows
// the ISP's nameservers.
type SpeakerDNSResolutionFunc func(deviceIP, expectedIP string) (usesAfterTouch, hookInstalled, sshOK bool)

// RegisterDNSBypassRiskCheck registers a check that, for each known device,
// determines whether it actually resolves Bose hostnames through AfterTouch
// while DNS interception is enabled. Signals are tried strongest first:
//
//  1. SSH probe (resolutionFn): reads the speaker's own /etc/resolv.conf and
//     checks for the AfterTouch DNS-migration hook file. This is a positive,
//     authoritative signal in either direction, so it is trusted on its own:
//     confirmed-using-us emits nothing, confirmed-bypassing emits a warning.
//  2. DNS-query observation (clientIPsFn, from the dns_speaker_usage check's
//     querier set): used only when SSH is unavailable or didn't run. A
//     speaker that has queried an intercepted hostname is confirmed; one that
//     hasn't gets a weak info finding, because "never queried" is ambiguous
//     (it may simply not have played a TuneIn stream since the last restart)
//     rather than positive evidence of a bypass.
//
// DNS interception being off (dnsEnabledFn returns false) short-circuits the
// whole check to no findings: HandleBMXRegistry hands out this service's own
// address in that case, so there is nothing to warn about regardless of how
// any given speaker resolves hostnames.
func RegisterDNSBypassRiskCheck(
	r *Registry,
	ds *datastore.DataStore,
	dnsEnabledFn DNSEnabledFunc,
	expectedIPFn ExpectedIPFunc,
	clientIPsFn func() map[string]time.Time,
	resolutionFn SpeakerDNSResolutionFunc,
) {
	r.Register(Check{
		ID:    CheckIDDNSBypassRisk,
		Title: "Speakers with DNS Discovery on resolve Bose hostnames through AfterTouch",
		Run: func() []Finding {
			return runDNSBypassRiskCheck(ds, dnsEnabledFn, expectedIPFn, clientIPsFn, resolutionFn)
		},
	})
}

func runDNSBypassRiskCheck(
	ds *datastore.DataStore,
	dnsEnabledFn DNSEnabledFunc,
	expectedIPFn ExpectedIPFunc,
	clientIPsFn func() map[string]time.Time,
	resolutionFn SpeakerDNSResolutionFunc,
) []Finding {
	if dnsEnabledFn == nil || !dnsEnabledFn() {
		// DNS interception is off: HandleBMXRegistry hands out the
		// service's own address regardless of any speaker's resolver, so
		// there is nothing to warn about.
		return nil
	}

	if ds == nil {
		return nil
	}

	devices, err := ds.ListAllDevices()
	if err != nil {
		return []Finding{{
			Severity: SeverityError,
			Message:  "Could not enumerate devices: " + err.Error(),
		}}
	}

	var expectedIP string
	if expectedIPFn != nil {
		expectedIP = expectedIPFn()
	}

	queriers := map[string]time.Time{}
	if clientIPsFn != nil {
		queriers = clientIPsFn()
	}

	var findings []Finding

	for i := range devices {
		dev := &devices[i]
		if dev.IPAddress == "" {
			continue
		}

		findings = append(findings, assessDNSBypassRisk(
			dev.AccountID, dev.DeviceID, dev.Name, dev.IPAddress,
			expectedIP, queriers, resolutionFn,
		)...)
	}

	return findings
}

// assessDNSBypassRisk is the pure, per-device core: given the strongest
// available signal, it decides whether this device's DNS path is confirmed,
// confirmed-bypassed, or unconfirmed. Split out from the datastore iteration
// so it can be unit-tested directly.
func assessDNSBypassRisk(
	account, deviceID, deviceName, ipAddress, expectedIP string,
	queriers map[string]time.Time,
	resolutionFn SpeakerDNSResolutionFunc,
) []Finding {
	target := Target{Account: account, Device: deviceID}
	name := displayName(deviceName, deviceID)

	if resolutionFn != nil {
		usesAfterTouch, hookInstalled, sshOK := resolutionFn(ipAddress, expectedIP)
		if sshOK {
			if usesAfterTouch || hookInstalled {
				// Confirmed: this speaker resolves through us (or has the
				// migration hook installed and will on its next DHCP
				// renewal). SSH is authoritative here, so we stop:
				// no need to fall back to the weaker query signal.
				return nil
			}

			return []Finding{{
				Severity: SeverityWarning,
				Target:   target,
				Message: fmt.Sprintf(
					"Device %s (%s) has DNS Discovery enabled but does not resolve Bose hostnames through AfterTouch.",
					name, ipAddress,
				),
				Details: "Its /etc/resolv.conf has no AfterTouch nameserver and no AfterTouch DNS-migration hook " +
					"file was found. Because DNS Discovery is on, HandleBMXRegistry still hands this speaker " +
					"\"https://content.api.bose.io\" as the base URL for TuneIn and the other BMX-delivered " +
					"services, the shut-down Bose cloud, so those requests fail (BMX_HTTP_ERROR 4501, " +
					"Apigee \"ApplicationNotFound\"). Pick ONE way out: turn off \"Enable DNS Discovery Server\" " +
					"in Settings, or run the DNS (resolv.conf) migration for this speaker. Either way, reboot " +
					"the speaker afterward: it keeps its old service list until then. See " +
					dnsBypassTroubleshootingURL + ".",
				ManualCommands: []ManualCommand{{
					Label: "Migrate this speaker via DNS (resolv.conf) instead of toggling Settings:",
					Command: fmt.Sprintf(
						"soundtouch-cli --host %s setup migrate --method resolv --service-url http://<aftertouch-host>:8000",
						ipAddress,
					),
					Hint: "Replace <aftertouch-host> with a LAN-resolvable name or IP of this service. " +
						"Don't also toggle DNS Discovery off if you run this. Pick one remedy, not both, " +
						"then reboot the speaker.",
				}},
			}}
		}
	}

	// No SSH signal available (or none was attempted): fall back to the
	// weaker DNS-query observation. This cannot distinguish "never played
	// anything since restart" from "bypasses us", so it stays info-level.
	if _, observed := queriers[ipAddress]; observed {
		return nil
	}

	return []Finding{{
		Severity: SeverityInfo,
		Target:   target,
		Message: fmt.Sprintf(
			"Device %s (%s) has DNS Discovery enabled; it's not yet confirmed whether it resolves Bose "+
				"hostnames through AfterTouch.",
			name, ipAddress,
		),
		Details: "This device's IP has not appeared as a querier for intercepted Bose hostnames, and no SSH " +
			"probe of its /etc/resolv.conf was possible (SSH unreachable, or not attempted). It may simply " +
			"not have played a TuneIn stream since the last restart, or it may be resolving Bose hostnames " +
			"through a different resolver, in which case TuneIn and the other BMX-delivered services fail " +
			"with BMX_HTTP_ERROR 4501 against the shut-down Bose cloud. Use 'Test DNS path' to check now, " +
			"or see " + dnsBypassTroubleshootingURL + " if TuneIn is actually failing.",
		QuickFixes: []QuickFix{probeDNSPathQuickFix},
	}}
}
