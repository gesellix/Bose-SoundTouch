package health

import (
	"fmt"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDRuntimeBmxURLStale is the registry id of the runtime BMX-URL
// staleness check.
const CheckIDRuntimeBmxURLStale = "runtime_bmx_url_stale"

// RegisterRuntimeBmxURLStaleCheck registers a check that reads each reachable
// speaker's *runtime* bmxRegistryUrl (from its on-device
// SoundTouchSdkPrivateCfg.xml via SSH, or `getpdo` over telnet) and flags
// speakers still pointed at the shut-down Bose cloud.
//
// Radio source types (TUNEIN / RADIO_BROWSER / LOCAL_INTERNET_RADIO) are
// delivered to the speaker through the BMX registry, so a speaker whose
// bmxRegistryUrl still names the Bose cloud can never mount them. That is the
// most common "radio missing after migration" cause. The service's own /sources
// listing is correct, which is why the existing sources_xml_diff check only
// reports the symptom ("missing 3 source types"); this check reports the
// per-device cause so the operator knows exactly which speakers need
// re-migrating.
//
// readBmxURL reads a speaker's runtime bmxRegistryUrl by IP, returning the URL
// and whether the runtime config could be read at all. It lives in the handlers
// layer because the health package deliberately avoids importing the SSH/telnet
// and setup packages (see the boundary comments in server.go).
//
// dnsRunningFn reports whether this service is running its own DNS interception.
// This is the false-positive guard: under a DNS-based migration (AfterTouch
// acting as the speaker's DNS server) a cloud URL is EXPECTED, because the
// redirect happens at the DNS layer rather than by rewriting the on-device URL,
// so when our DNS is running the check stays silent. The router-DNS variant (the
// LAN's DNS points at AfterTouch without our DNS server running) is called out
// as a known exception in the finding text rather than suppressed, since we
// cannot detect it here.
func RegisterRuntimeBmxURLStaleCheck(r *Registry, ds *datastore.DataStore, readBmxURL func(ip string) (string, bool), dnsRunningFn func() bool) {
	r.Register(Check{
		ID:    CheckIDRuntimeBmxURLStale,
		Title: "Speaker runtime bmxRegistryUrl points at AfterTouch, not the Bose cloud",
		Run: func() []Finding {
			return runRuntimeBmxURLStaleCheck(ds, readBmxURL, dnsRunningFn)
		},
	})
}

func runRuntimeBmxURLStaleCheck(ds *datastore.DataStore, readBmxURL func(ip string) (string, bool), dnsRunningFn func() bool) []Finding {
	if ds == nil || readBmxURL == nil {
		return nil
	}

	// When this service intercepts DNS, a speaker legitimately keeps the Bose
	// cloud hostnames in its on-device config (they resolve to AfterTouch), so a
	// cloud URL is not evidence of a stale migration. Don't second-guess it.
	if dnsRunningFn != nil && dnsRunningFn() {
		return nil
	}

	devices, err := ds.ListAllDevices()
	if err != nil {
		return []Finding{{
			Severity: SeverityError,
			Message:  "Could not enumerate devices: " + err.Error(),
		}}
	}

	var findings []Finding

	for i := range devices {
		dev := &devices[i]
		if dev.IPAddress == "" || dev.DeviceID == "" {
			continue
		}

		bmxURL, ok := readBmxURL(dev.IPAddress)
		if !ok || bmxURL == "" {
			// Couldn't read the runtime config (speaker offline, or neither SSH
			// nor telnet available). speaker_info_reachable / sources_xml_diff
			// cover the reachability angle; nothing to assert here.
			continue
		}

		findings = append(findings, assessRuntimeBmxURL(dev.AccountID, dev.DeviceID, dev.IPAddress, bmxURL)...)
	}

	return findings
}

// assessRuntimeBmxURL is the pure, per-device core: given a speaker's runtime
// bmxRegistryUrl, it returns a warning finding when the URL still names the Bose
// cloud. Split out from the datastore iteration so it can be unit-tested
// directly (mirrors assessMargeURLForDeviceWithURL).
func assessRuntimeBmxURL(account, deviceID, ipAddress, bmxURL string) []Finding {
	host := hostFromURL(bmxURL)
	if host == "" || !isBoseCloudHost(host) {
		return nil
	}

	return []Finding{{
		Severity: SeverityWarning,
		Target:   Target{Account: account, Device: deviceID},
		Message:  fmt.Sprintf("Speaker's runtime BMX registry URL still points at the Bose cloud (%s).", host),
		Details:  "Radio source types (TUNEIN, RADIO_BROWSER, LOCAL_INTERNET_RADIO) are fetched from the BMX registry, so while this URL names the shut-down Bose cloud the speaker can never mount them, even though the service's own /sources listing looks correct. Re-migrate this speaker so its runtime bmxRegistryUrl points at AfterTouch, then reboot it. (Exception: if you migrate via DNS, meaning AfterTouch acts as the speaker's DNS server or your router points the LAN's DNS at AfterTouch, a cloud URL is expected and this warning can be ignored.)",
		ManualCommands: []ManualCommand{{
			Label:   "Re-migrate this speaker (the telnet method rewrites all runtime URLs):",
			Command: fmt.Sprintf("soundtouch-cli --host %s setup migrate --method telnet --service-url http://<aftertouch-host>:8000", ipAddress),
			Hint:    "Replace <aftertouch-host> with a LAN-resolvable name or IP of this service. Reboot the speaker afterwards so it reloads the new config.",
		}},
	}}
}

// isBoseCloudHost reports whether host belongs to one of Bose's (shut-down)
// cloud domains: content.api.bose.io, streaming.bose.com, events.api.bosecm.com,
// worldwide.bose.com, and the like. A domain-suffix match keeps it robust
// against the various sub-domains seen across firmware versions.
func isBoseCloudHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}

	for _, domain := range []string{"bose.io", "bose.com", "bosecm.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}

	return false
}
