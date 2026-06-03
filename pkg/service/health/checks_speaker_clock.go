package health

import (
	"fmt"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDSpeakerClock is the registry ID of the speaker clock skew check.
const CheckIDSpeakerClock = "speaker_clock"

// clockPlausibilityMin is the lower bound of the plausibility window (year 2000).
// Matches the lower bound used by ClockTimeRequest.Validate in pkg/models/clocktime.go.
const clockPlausibilityMin int64 = 946684800

// clockPlausibilityMax is the upper bound of the plausibility window (year 2100).
// Matches the upper bound used by ClockTimeRequest.Validate in pkg/models/clocktime.go.
const clockPlausibilityMax int64 = 4102444800

// clockStaleNTPThreshold is how long after the last NTP sync before we flag it
// as stale.
const clockStaleNTPThreshold = time.Hour

// setClockQuickFix is the QuickFix descriptor for the set_clock action.
// Attached to Warning and Error findings only (Info is harmless sub-5-minute drift).
var setClockQuickFix = QuickFix{
	ID:    "set_clock",
	Label: "Set clock from AfterTouch",
	Confirm: "Sets the speaker's clock to this server's current time. " +
		"If the speaker's NTP sync is failing, the clock may drift again or reset on reboot " +
		"— restoring time sync is the durable fix.",
}

// RegisterSpeakerClockCheck registers a per-device health check that
// compares each speaker's UTC epoch to the service's UTC epoch and surfaces
// findings when the skew is large enough to cause TLS certificate failures.
//
// clockFn must return the speaker's clock as epoch seconds (utc), its last
// NTP sync epoch (sync), and ok=false when /clockTime could not be read or
// had no usable UTC time.
//
// setFn sets the clock on the speaker at the given IP. It is called by the
// set_clock QuickFix. Passing nil disables the fix registration.
func RegisterSpeakerClockCheck(
	r *Registry,
	ds *datastore.DataStore,
	clockFn func(deviceIP string) (utc, sync int64, ok bool),
	setFn func(deviceIP string) error,
) {
	r.Register(Check{
		ID:    CheckIDSpeakerClock,
		Title: "Speaker clock accuracy",
		Run: func() []Finding {
			return runSpeakerClockCheck(ds, clockFn, time.Now())
		},
	})

	if setFn == nil {
		return
	}

	r.RegisterFix(CheckIDSpeakerClock, "set_clock", func(target Target) (string, error) {
		devices, err := ds.ListAllDevices()
		if err != nil {
			return "", fmt.Errorf("could not enumerate devices: %w", err)
		}

		var (
			ip      string
			devName string
		)

		for i := range devices {
			if devices[i].DeviceID == target.Device {
				ip = devices[i].IPAddress
				devName = displayName(devices[i].Name, devices[i].DeviceID)

				break
			}
		}

		if ip == "" {
			return "", fmt.Errorf("device %q not found or has no IP address", target.Device)
		}

		if err := setFn(ip); err != nil {
			return "", fmt.Errorf("set clock on %s: %w", devName, err)
		}

		now := time.Now().UTC().Format(time.RFC3339)

		return fmt.Sprintf(
			"Set clock on %s to %s. "+
				"If NTP is still failing the clock may drift again or reset on reboot "+
				"— restoring time sync is the durable fix.",
			devName, now,
		), nil
	})
}

func runSpeakerClockCheck(
	ds *datastore.DataStore,
	clockFn func(deviceIP string) (utc, sync int64, ok bool),
	now time.Time,
) []Finding {
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

	var findings []Finding

	for i := range devices {
		dev := &devices[i]
		if dev.IPAddress == "" {
			continue
		}

		utc, sync, ok := clockFn(dev.IPAddress)
		if !ok {
			// Reachability is speaker_info_reachable's job; do not duplicate.
			continue
		}

		target := Target{Account: dev.AccountID, Device: dev.DeviceID}
		name := displayName(dev.Name, dev.DeviceID)

		skew := now.Unix() - utc

		abs := skew
		if abs < 0 {
			abs = -abs
		}

		// Build common details lines for enrichment.
		speakerTimeStr := time.Unix(utc, 0).UTC().Format(time.RFC3339)
		serviceTimeStr := now.UTC().Format(time.RFC3339)

		var skewSign string
		if skew >= 0 {
			skewSign = fmt.Sprintf("+%ds", skew)
		} else {
			skewSign = fmt.Sprintf("%ds", skew)
		}

		details := fmt.Sprintf(
			"Speaker UTC: %s. Service UTC: %s. Signed skew: %s.",
			speakerTimeStr, serviceTimeStr, skewSign,
		)

		// NTP staleness note.
		ntpNote := ""
		if sync == 0 {
			ntpNote = " Speaker has never successfully synced with an NTP server — NTP is likely failing."
		} else if now.Unix()-sync > int64(clockStaleNTPThreshold.Seconds()) {
			lastSync := time.Unix(sync, 0).UTC().Format(time.RFC3339)
			ntpNote = fmt.Sprintf(" Speaker's last NTP sync was %s (more than 1 hour ago) — NTP is likely failing.", lastSync)
		}

		if ntpNote != "" {
			details += ntpNote
		}

		remediation := " Remediation: ensure the speaker can reach an NTP server" +
			" (UDP/123 outbound to a reachable time server). AfterTouch can also" +
			" push the current time via /clockTime (soundtouch-cli set-clock-time) if needed."
		details += remediation

		// Determine tier. The plausibility check is applied regardless of abs
		// magnitude: a speaker claiming year 2000 or year 2101 is an Error even
		// if the arithmetic skew happens to be small.
		outsidePlausibility := utc < clockPlausibilityMin || utc > clockPlausibilityMax

		switch {
		case outsidePlausibility || abs >= int64(24*time.Hour/time.Second):
			var msg string
			if outsidePlausibility {
				msg = fmt.Sprintf(
					"Device %s: speaker clock is implausible (speaker reads %s, service is %s)."+
						" HTTPS/TLS will fail: certificates appear not-yet-valid or expired,"+
						" so TuneIn/BMX content and any HTTPS source will break until time sync is restored.",
					name, speakerTimeStr, serviceTimeStr,
				)
			} else {
				msg = fmt.Sprintf(
					"Device %s: speaker clock is far off (speaker reads %s, service is %s)."+
						" HTTPS/TLS will fail: certificates appear not-yet-valid or expired,"+
						" so TuneIn/BMX content and any HTTPS source will break until time sync is restored.",
					name, speakerTimeStr, serviceTimeStr,
				)
			}

			findings = append(findings, Finding{
				Severity:   SeverityError,
				Target:     target,
				Message:    msg,
				Details:    details,
				QuickFixes: []QuickFix{setClockQuickFix},
			})

		case abs >= int64(5*time.Minute/time.Second):
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Target:   target,
				Message: fmt.Sprintf(
					"Device %s: clock is off by %s; may cause intermittent HTTPS/TLS failures.",
					name, formatDuration(abs),
				),
				Details:    details,
				QuickFixes: []QuickFix{setClockQuickFix},
			})

		case abs >= 60:
			findings = append(findings, Finding{
				Severity: SeverityInfo,
				Target:   target,
				Message: fmt.Sprintf(
					"Device %s: minor clock drift of %s; NTP sync may be loose.",
					name, formatDuration(abs),
				),
				Details: details,
			})

		default:
			// abs < 60s: healthy, no finding.
		}
	}

	return findings
}

// formatDuration renders a duration (given as unsigned seconds) as a
// human-readable string (e.g. "10m30s", "2h5m", "33d").
func formatDuration(secs int64) string {
	days := secs / 86400
	rem := secs % 86400
	hours := rem / 3600
	rem %= 3600
	minutes := rem / 60
	seconds := rem % 60

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}

		return fmt.Sprintf("%dd", days)
	}

	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}

		return fmt.Sprintf("%dh", hours)
	}

	if minutes > 0 {
		if seconds > 0 {
			return fmt.Sprintf("%dm%ds", minutes, seconds)
		}

		return fmt.Sprintf("%dm", minutes)
	}

	return fmt.Sprintf("%ds", seconds)
}
