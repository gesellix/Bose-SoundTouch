package setup

import (
	"bytes"
	"encoding/pem"
	"fmt"
	"strings"
)

// validateCABundleBytes walks bundle as a sequence of PEM-encoded
// CERTIFICATE blocks and asserts the framing is structurally intact:
// every BEGIN marker has a matching END marker, every block decodes
// as a valid PEM block, and no stray non-PEM/non-comment content
// appears between blocks. We deliberately do NOT call
// x509.ParseCertificate on the block bytes — that would reject
// legitimate Mozilla CCADB entries (negative serial numbers, ancient
// certificates from the 2000s that fail strict RFC 5280 enforcement
// in Go 1.23+), and the failure mode this check exists to defend
// against (issue #262, a corrupted CA bundle on disk) shows up at
// the PEM-framing layer, not at the x509 layer.
//
// Returns the parsed block count on success.
func validateCABundleBytes(bundle []byte) (int, error) {
	if len(bundle) == 0 {
		return 0, fmt.Errorf("CA bundle is empty")
	}

	const (
		beginMarker = "-----BEGIN CERTIFICATE-----"
		endMarker   = "-----END CERTIFICATE-----"
	)

	beginCount := bytes.Count(bundle, []byte(beginMarker))

	endCount := bytes.Count(bundle, []byte(endMarker))
	if beginCount != endCount {
		return 0, fmt.Errorf("PEM framing mismatch: %d BEGIN markers, %d END markers", beginCount, endCount)
	}

	rest := bundle

	count := 0

	for {
		var block *pem.Block

		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		count++

		if block.Type != "CERTIFICATE" {
			return count, fmt.Errorf("PEM block %d has type %q, want CERTIFICATE", count, block.Type)
		}

		if len(block.Bytes) == 0 {
			return count, fmt.Errorf("PEM block %d has empty body", count)
		}
	}

	if count == 0 {
		return 0, fmt.Errorf("CA bundle contains no PEM CERTIFICATE blocks")
	}

	if count != beginCount {
		return count, fmt.Errorf("decoded %d PEM blocks but found %d BEGIN markers (suggests a block has unparseable base64 body)", count, beginCount)
	}

	if trail := bytes.TrimSpace(rest); len(trail) > 0 {
		// Tolerate anything that's just whitespace, comments, or our
		// own sentinel lines — but reject stray non-PEM bytes that
		// don't fall on a block boundary. Comment lines (starting
		// with `#`) are allowed because CALabel is one.
		for _, raw := range bytes.Split(trail, []byte("\n")) {
			line := bytes.TrimSpace(raw)
			if len(line) == 0 {
				continue
			}

			if bytes.HasPrefix(line, []byte("#")) {
				continue
			}

			return count, fmt.Errorf("trailing non-PEM content after block %d: %q", count, line)
		}
	}

	return count, nil
}

// validateAfterTouchLabelBracketing asserts the AfterTouch CALabel
// sentinel appears exactly twice in bundle (open + close), and that
// exactly one CERTIFICATE block sits between the two occurrences.
// Used as a post-upload check to detect transport truncation that
// either drops the closing sentinel or drops the certificate body
// between them.
func validateAfterTouchLabelBracketing(bundle []byte) error {
	count := strings.Count(string(bundle), CALabel)
	if count != 2 {
		return fmt.Errorf("AfterTouch CA label %q appears %d times, want exactly 2 (open + close)", CALabel, count)
	}

	parts := strings.SplitN(string(bundle), CALabel, 3)
	if len(parts) != 3 {
		// Shouldn't reach here given the count check above, but
		// defend against malformed input that splits unexpectedly.
		return fmt.Errorf("AfterTouch CA label %q does not bracket cleanly", CALabel)
	}

	bracketed := parts[1]

	if strings.Count(bracketed, "-----BEGIN CERTIFICATE-----") != 1 {
		return fmt.Errorf("expected exactly one BEGIN CERTIFICATE between AfterTouch CA labels, found %d",
			strings.Count(bracketed, "-----BEGIN CERTIFICATE-----"))
	}

	if strings.Count(bracketed, "-----END CERTIFICATE-----") != 1 {
		return fmt.Errorf("expected exactly one END CERTIFICATE between AfterTouch CA labels, found %d",
			strings.Count(bracketed, "-----END CERTIFICATE-----"))
	}

	return nil
}

// stripAfterTouchEntriesResult is the structured outcome of
// stripAfterTouchEntries — non-fatal anomalies surface as fields so
// the caller can decide whether to log them or surface them in the
// migration UI.
type stripAfterTouchEntriesResult struct {
	// CleanedBundle is the bundle content with every AfterTouch entry
	// (each `# AfterTouch` sentinel pair and the cert lines between
	// them) removed.
	CleanedBundle string

	// RemovedEntries counts the number of complete sentinel pairs
	// stripped. >1 means an earlier release added our CA more than
	// once and we just collapsed the duplicates; the caller should
	// log this so the user knows their bundle was cleaned up.
	RemovedEntries int

	// UnpairedSentinel is true when the input had an odd number of
	// AfterTouch sentinel lines — a sign of a previous truncated or
	// botched install. The trailing "open" sentinel and anything that
	// follows it (until EOF) gets dropped along with the orphaned
	// half of a pair; that may silently drop legitimate non-AfterTouch
	// content that happened to sit after the truncation point, which
	// is why we surface this as a structured anomaly rather than
	// just logging it.
	UnpairedSentinel bool
}

// stripAfterTouchEntries removes every CALabel sentinel line from
// bundle and every line between paired sentinels (i.e. the
// previously-injected AfterTouch CA payload). It's the line-walking
// equivalent of "strip our own entry"; the caller appends a fresh
// entry afterward.
//
// The implementation tolerates the multi-entry case explicitly —
// older AfterTouch releases are reported to have appended the CA on
// every install without stripping the previous one, so the live
// bundle on long-lived devices may carry several copies. We strip
// them all and let the caller log the cleanup count.
func stripAfterTouchEntries(bundle string) stripAfterTouchEntriesResult {
	lines := strings.Split(bundle, "\n")

	var (
		out             []string
		inOurCA         bool
		removedEntries  int
		unpairedTrailer bool
	)

	for _, line := range lines {
		if strings.Contains(line, CALabel) {
			if inOurCA {
				// closing sentinel — one full entry consumed
				removedEntries++
			}

			inOurCA = !inOurCA

			continue
		}

		if !inOurCA {
			out = append(out, line)
		}
	}

	if inOurCA {
		// Loop ended with an open bracket — trailing content was
		// dropped along with the unpaired opening sentinel. The
		// (truncated) entry doesn't count as "removed" because no
		// closing sentinel ever marked it complete.
		unpairedTrailer = true
	}

	cleaned := strings.Join(out, "\n")
	if cleaned != "" && !strings.HasSuffix(cleaned, "\n") {
		cleaned += "\n"
	}

	return stripAfterTouchEntriesResult{
		CleanedBundle:    cleaned,
		RemovedEntries:   removedEntries,
		UnpairedSentinel: unpairedTrailer,
	}
}
