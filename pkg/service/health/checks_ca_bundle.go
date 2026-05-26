package health

import (
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDSpeakerCABundle is the registry ID of the speaker CA bundle
// integrity check.
const CheckIDSpeakerCABundle = "speaker_ca_bundle"

// FixIDRestoreAndInjectCA is the registry ID of the fix that restores
// the .original factory backup and re-injects the AfterTouch CA.
// Used when check (1) — original certs still present — fails.
const FixIDRestoreAndInjectCA = "restore_and_inject_ca"

// FixIDInjectCACert is the registry ID of the fix that injects the
// AfterTouch CA into the speaker's bundle.
// Used when check (2) — AfterTouch CA present — fails.
const FixIDInjectCACert = "inject_ca_cert"

// caLabel is the AfterTouch sentinel written by TrustCACertFromBytes.
// Keep in sync with setup.CALabel in pkg/service/setup/setup.go.
// The health package deliberately avoids importing setup to keep its
// transitive dependency surface small.
const caLabel = "# AfterTouch"

// RegisterSpeakerCABundleCheck registers a per-device health check that
// verifies the integrity of the CA bundle on each speaker. probeFn must
// return the live bundle content, the .original backup content, and
// whether SSH succeeded. An empty bundle string means the file was
// absent on the device.
func RegisterSpeakerCABundleCheck(
	r *Registry,
	ds *datastore.DataStore,
	probeFn func(deviceIP string) (current, original string, sshOK bool),
) {
	r.Register(Check{
		ID:    CheckIDSpeakerCABundle,
		Title: "Speaker CA bundle integrity",
		Run: func() []Finding {
			return runSpeakerCABundleCheck(ds, probeFn)
		},
	})
}

func runSpeakerCABundleCheck(
	ds *datastore.DataStore,
	probeFn func(deviceIP string) (current, original string, sshOK bool),
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

		current, original, sshOK := probeFn(dev.IPAddress)

		target := Target{Account: dev.AccountID, Device: dev.DeviceID}
		name := displayName(dev.Name, dev.DeviceID)

		if !sshOK {
			findings = append(findings, Finding{
				Severity: SeverityInfo,
				Target:   target,
				Message:  fmt.Sprintf("Device %s: SSH unavailable — CA bundle integrity cannot be verified", name),
				Details:  "SSH access is required to inspect the on-device CA bundle. Enable SSH via USB stick or the remote_services panel, run the check again, then disable it.",
			})

			continue
		}

		// Check (1): every PEM block from .original must be present in current.
		if original == "" {
			findings = append(findings, Finding{
				Severity: SeverityWarning,
				Target:   target,
				Message:  fmt.Sprintf("Device %s: original CA bundle backup not found on speaker", name),
				Details:  "The .original backup is created by `setup install-ca` on first injection. Run `soundtouch-cli setup install-ca` to inject the AfterTouch CA and establish the backup in one step.",
			})
			// Fall through to check (2) — we still know whether the AfterTouch CA
			// is in the current bundle even without the backup.
		} else {
			missing := missingPEMBlocks(original, stripCALabel(current))
			if len(missing) > 0 {
				findings = append(findings, Finding{
					Severity: SeverityError,
					Target:   target,
					Message: fmt.Sprintf(
						"Device %s: %d original CA certificate(s) missing from current bundle",
						name, len(missing),
					),
					Details: "The live CA bundle has fewer certificates than the factory backup. " +
						"External HTTPS connections (Spotify, Amazon Music, firmware updates) may fail. " +
						"Use the fix to restore the factory bundle and re-inject the AfterTouch CA.",
					QuickFixes: []QuickFix{{
						ID:    FixIDRestoreAndInjectCA,
						Label: "Restore original bundle and re-inject AfterTouch CA",
						Confirm: fmt.Sprintf(
							"This will overwrite the live CA bundle on speaker %s with its factory backup, "+
								"then re-inject the AfterTouch CA. SSH access is required.",
							name,
						),
					}},
				})
			}
		}

		// Check (2): AfterTouch CA must be present in the current bundle.
		if !strings.Contains(current, caLabel) {
			findings = append(findings, Finding{
				Severity: SeverityError,
				Target:   target,
				Message:  fmt.Sprintf("Device %s: AfterTouch CA certificate not found in speaker's bundle", name),
				Details: "Without the AfterTouch CA, the speaker will reject AfterTouch's TLS certificate " +
					"and migration is effectively inactive. Use the fix to install the CA.",
				QuickFixes: []QuickFix{{
					ID:    FixIDInjectCACert,
					Label: "Install AfterTouch CA on speaker",
					Confirm: fmt.Sprintf(
						"This will inject the AfterTouch CA certificate into the CA bundle on speaker %s. SSH access is required.",
						name,
					),
				}},
			})
		}
	}

	return findings
}

// stripCALabel removes AfterTouch-labelled blocks from bundle so that
// the containment check compares only the original factory entries.
// This mirrors the sentinel-scan in setup.stripAfterTouchEntries without
// importing that package.
func stripCALabel(bundle string) string {
	var out strings.Builder

	inBlock := false

	for _, line := range strings.Split(bundle, "\n") {
		if line == caLabel {
			inBlock = !inBlock
			continue
		}

		if !inBlock {
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}

	return out.String()
}

// missingPEMBlocks returns the number of PEM blocks present in want that
// are absent from have. Comparison is by raw DER bytes so header
// differences (e.g. different Proc-Type lines) are ignored.
func missingPEMBlocks(want, have string) [][]byte {
	haveSet := pemDERSet(have)

	var missing [][]byte

	rest := []byte(want)

	for {
		var block *pem.Block

		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		if _, ok := haveSet[string(block.Bytes)]; !ok {
			missing = append(missing, block.Bytes)
		}
	}

	return missing
}

// pemDERSet decodes all PEM blocks from s and returns a set of their
// DER (raw Bytes) values for O(1) membership tests.
func pemDERSet(s string) map[string]struct{} {
	set := make(map[string]struct{})
	rest := []byte(s)

	for {
		var block *pem.Block

		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		set[string(block.Bytes)] = struct{}{}
	}

	return set
}
