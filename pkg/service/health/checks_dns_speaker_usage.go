package health

import (
	"fmt"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDDNSSpeakerUsage is the registry ID of the check that verifies
// speakers are actually resolving Bose hostnames through AfterTouch's DNS
// interceptor rather than via the operator's own resolvers, which would
// route content requests to the decommissioned Bose cloud.
const CheckIDDNSSpeakerUsage = "dns_speaker_usage"

// probeDNSPathQuickFix is the QuickFix descriptor attached to per-device
// info findings for speakers not yet observed resolving through AfterTouch.
// The probe is silent and quick, so no confirmation dialog is needed.
var probeDNSPathQuickFix = QuickFix{
	ID:    "probe_dns_path",
	Label: "Test DNS path",
}

// RegisterDNSSpeakerUsageCheck registers a check that cross-references the
// set of known speaker IPs against the set of non-loopback clients that have
// sent intercepted-hostname queries to AfterTouch's DNS server. It surfaces:
//
//   - An info finding per device that has not yet been observed resolving a
//     Bose hostname through AfterTouch. Each finding carries a "Test DNS path"
//     QuickFix so the operator can run an active probe on demand.
//
// Devices whose IP is present in the querier set are confirmed (no finding
// is emitted for them).
// Reuses the existing DNSStatusFunc type from checks_dns_sanity.go.
func RegisterDNSSpeakerUsageCheck(
	r *Registry,
	ds *datastore.DataStore,
	statusFn DNSStatusFunc,
	clientIPsFn func() map[string]time.Time,
) {
	r.Register(Check{
		ID:    CheckIDDNSSpeakerUsage,
		Title: "Speakers resolve Bose hostnames through AfterTouch",
		Run: func() []Finding {
			return runDNSSpeakerUsageCheck(ds, statusFn, clientIPsFn)
		},
	})
}

func runDNSSpeakerUsageCheck(
	ds *datastore.DataStore,
	statusFn DNSStatusFunc,
	clientIPsFn func() map[string]time.Time,
) []Finding {
	running, _ := statusFn()
	if !running {
		// dns_sanity already covers the "DNS not running" case; emit nothing
		// here to avoid duplicating that finding.
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

	queriers := clientIPsFn()

	var findings []Finding

	for i := range devices {
		dev := &devices[i]
		if dev.IPAddress == "" {
			continue
		}

		if _, seen := queriers[dev.IPAddress]; seen {
			// Confirmed: this device has queried an intercepted hostname.
			continue
		}

		name := displayName(dev.Name, dev.DeviceID)

		findings = append(findings, Finding{
			Severity: SeverityInfo,
			Target:   Target{Account: dev.AccountID, Device: dev.DeviceID},
			Message: fmt.Sprintf(
				"Device %s (%s) is not yet confirmed to resolve Bose hostnames through AfterTouch. "+
					"Use 'Test DNS path' to check now.",
				name, dev.IPAddress,
			),
			Details: "This device's IP has not appeared as a querier for intercepted " +
				"Bose hostnames. It may simply not have played a TuneIn stream since the " +
				"last restart, or it may be using a different DNS resolver. " +
				"Click 'Test DNS path' to run an active probe that sends a silent " +
				"notification and waits for the speaker to call back through AfterTouch's DNS.",
			QuickFixes: []QuickFix{probeDNSPathQuickFix},
		})
	}

	return findings
}
