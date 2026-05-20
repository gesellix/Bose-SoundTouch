package health

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// suggestAccountForPairing picks an account ID to pre-fill into the
// "Complete pairing" QuickFix's Target.Account. Returns the first
// real-looking (7-digit) account directory that already contains the
// device's deviceID on disk — typical scenario: AfterTouch and the
// speaker were partially paired earlier, the speaker forgot but our
// datastore remembers. Returns "" when no such account exists, in
// which case the executor generates a fresh ID at click time.
func suggestAccountForPairing(ds *datastore.DataStore, deviceID string) string {
	if ds == nil {
		return ""
	}

	for _, acc := range ds.AllAccountsForDevice(deviceID) {
		if isSevenDigitAccountID(acc) {
			return acc
		}
	}

	return ""
}

// isSevenDigitAccountID mirrors setup.IsValidAccountID without
// importing the setup package (which would pull in SSH/telnet/certmgr
// transitively — see the boundary comment near speakerInfoXML).
func isSevenDigitAccountID(s string) bool {
	if len(s) != 7 {
		return false
	}

	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}

	return true
}

// buildEmptyMargeFinding constructs the "Speaker reports an empty
// <margeAccountUUID>" finding with a QuickFix that completes pairing
// in-place plus a ManualCommand fallback the operator can run from a
// shell on the speaker's LAN. The executor is registered separately by
// the caller that has setup.Manager access.
//
// Target.Account intentionally carries the *pair-with* account
// (whichever we picked from disk), not the current binding — that's
// what the fix executor needs at click time, and the framework passes
// Finding.Target through to the FixFunc verbatim. The UI grouping
// follows Target.Account too; that's a minor side effect we accept.
func buildEmptyMargeFinding(ds *datastore.DataStore, _ Target, deviceID, ipAddress string) Finding {
	suggested := suggestAccountForPairing(ds, deviceID)

	confirm := "Pair this speaker with a Marge account so playback selection stops failing with INVALID_SOURCE? "

	if suggested != "" {
		confirm += "AfterTouch will reuse the existing account " + suggested + " (already on disk for this device)."
	} else {
		confirm += "AfterTouch will generate a fresh 7-digit account ID for this speaker (private deployment — the ID is opaque)."
	}

	cliAccount := suggested
	if cliAccount == "" {
		cliAccount = "<7-digit-account-id>"
	}

	return Finding{
		Severity: SeverityWarning,
		Target:   Target{Account: suggested, Device: deviceID},
		Message:  "Speaker reports an empty <margeAccountUUID>.",
		Details:  "The speaker is reachable but isn't bound to any Marge account. Playback selection will fail with INVALID_SOURCE until pairing completes. See discussion #223 and issue #329.",
		QuickFixes: []QuickFix{{
			ID:      FixIDCompleteSpeakerPairing,
			Label:   "Complete pairing",
			Confirm: confirm,
		}},
		ManualCommands: []ManualCommand{{
			Label:   "Or pair from a host on the speaker's LAN:",
			Command: fmt.Sprintf("soundtouch-cli setup pair --host=%s --mode=bare --account=%s", ipAddress, cliAccount),
			Hint:    "The --mode=bare path sends just setMargeAccount without the full state machine; sufficient on FW 27.0.6.",
		}},
	}
}

// CheckIDSpeakerInfoReachable is the registry id of the speaker
// reachability check.
const CheckIDSpeakerInfoReachable = "speaker_info_reachable"

// FixIDCompleteSpeakerPairing is the QuickFix that completes pairing
// on a speaker that reports an empty <margeAccountUUID> — i.e. the
// speaker is reachable and configured to point at AfterTouch but
// hasn't been bound to a Marge account, so every playback selection
// fails with INVALID_SOURCE (see #329, discussion #223).
//
// The executor lives in pkg/service/handlers/server.go (where
// setup.Manager is available); the constant is defined here so the
// check that emits the finding stays self-contained.
const FixIDCompleteSpeakerPairing = "complete_speaker_pairing"

// speakerInfoXML mirrors only the fields we need from the
// speaker's :8090/info XML response. Duplicated here (rather than
// imported from pkg/service/setup) to keep the health package
// free of cross-package dependencies that would pull in SSH,
// telnet, certmgr, etc.
type speakerInfoXML struct {
	XMLName          xml.Name `xml:"info"`
	DeviceID         string   `xml:"deviceID,attr"`
	Name             string   `xml:"name"`
	MargeAccountUUID string   `xml:"margeAccountUUID"`
	MargeURL         string   `xml:"margeURL"`
}

// RegisterSpeakerInfoReachable registers the speaker_info_reachable
// check against r. The check iterates every known device in the
// datastore, probes its :8090/info endpoint, and emits findings
// for unreachable speakers and speakers paired with an empty
// margeAccountUUID (a known TPDA failure mode — see
// discussion #223).
func RegisterSpeakerInfoReachable(r *Registry, ds *datastore.DataStore) {
	r.Register(Check{
		ID:    CheckIDSpeakerInfoReachable,
		Title: "Speakers respond on :8090/info",
		Run: func() []Finding {
			return runSpeakerInfoReachable(ds)
		},
	})
}

func runSpeakerInfoReachable(ds *datastore.DataStore) []Finding {
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

		findings = append(findings, probeAndAssessSpeaker(ds, dev.AccountID, dev.DeviceID, dev.IPAddress)...)
	}

	return findings
}

func probeAndAssessSpeaker(ds *datastore.DataStore, account, deviceID, ipAddress string) []Finding {
	probeURL := fmt.Sprintf("http://%s:8090/info", ipAddress)
	return probeAndAssessSpeakerWithURL(ds, account, deviceID, ipAddress, probeURL)
}

// probeAndAssessSpeakerWithURL is the same as probeAndAssessSpeaker
// but takes the full URL directly. Used by tests that need to point
// at an httptest.Server, since those bind to random ports rather
// than :8090. ds and ipAddress feed the QuickFix attached to the
// empty-margeAccountUUID finding (suggestion for an existing
// account-on-disk, and the CLI ManualCommand fallback).
func probeAndAssessSpeakerWithURL(ds *datastore.DataStore, account, deviceID, ipAddress, probeURL string) []Finding {
	target := Target{Account: account, Device: deviceID}

	res := ProbeGet(context.Background(), probeURL, 2*time.Second)

	if !res.Reachable {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Speaker /info is not reachable from this host.",
			Details:  "If AfterTouch is hosted off the speaker's LAN (e.g. behind a reverse proxy or in a cloud), the service can't reach the speaker directly. Run the command below from a host that can.",
			ManualCommands: []ManualCommand{{
				Label:   "Fetch /info from your network:",
				Command: res.CurlCommand,
				Hint:    "Paste the response into a bug report or compare margeAccountUUID/margeURL with what AfterTouch expects.",
			}},
		}}
	}

	if res.Status != 200 {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  fmt.Sprintf("Speaker returned HTTP %d for /info.", res.Status),
			Details:  "Expected 200. Either the speaker is in a transient state or the IP belongs to a different device now.",
		}}
	}

	var parsed speakerInfoXML
	if err := xml.Unmarshal(res.Body, &parsed); err != nil {
		return []Finding{{
			Severity: SeverityWarning,
			Target:   target,
			Message:  "Speaker replied but the /info body is not valid XML.",
			Details:  "Parse error: " + err.Error(),
		}}
	}

	var out []Finding

	if parsed.MargeAccountUUID == "" {
		out = append(out, buildEmptyMargeFinding(ds, target, deviceID, ipAddress))
	}

	if parsed.MargeURL == "" {
		out = append(out, Finding{
			Severity: SeverityInfo,
			Target:   target,
			Message:  "Speaker reports an empty <margeURL>.",
			Details:  "The speaker hasn't been told where the cloud lives. This usually clears up after the first successful /info request from the service.",
		})
	}

	return out
}
