package setup

import (
	"bufio"
	"encoding/base64"
	"strings"
)

// speakerProbe is the result of a single batched SSH round-trip that
// gathers everything GetMigrationSummary needs in one go. Without it,
// the summary makes ~8 sequential SSH dials; pkg/ssh opens a fresh
// TCP+SSH handshake on every Run(), and SoundTouch firmware accepts
// only legacy crypto so each handshake is ~500 ms–1 s. Batching collapses
// that to one handshake.
type speakerProbe struct {
	// SSHOK reports whether the batched probe completed successfully.
	// false implies SSH is unreachable, auth failed, or the script
	// errored — in all cases GetMigrationSummary falls back to its
	// non-SSH paths (telnet preflight, HTTPS probe, etc).
	SSHOK bool

	// Files maps absolute device paths to their decoded contents.
	// Missing keys mean the file did not exist or could not be read.
	Files map[string]string

	// Exists is the set of probe paths that exist on the device (for
	// directories or non-text files we only need a yes/no signal).
	Exists map[string]bool

	// Err carries the underlying SSH error if SSHOK is false.
	Err error
}

// Probe paths used by the batched script. Keep this list in lockstep
// with the consumers in GetMigrationSummary.
var (
	probeFilePaths = []string{
		SoundTouchSdkPrivateCfgPath,               // current XML config
		SoundTouchSdkPrivateCfgPath + ".original", // backup XML config
		"/etc/resolv.conf",                        // DNS resolver
		"/etc/hosts",                              // hostname overrides
		"/etc/pki/tls/certs/ca-bundle.crt",        // CA trust store
	}

	probeExistsPaths = []string{
		"/etc/remote_services",    // SSH-enablement marker (persistent)
		"/mnt/nv/remote_services", // SSH-enablement marker (persistent, NV)
		"/tmp/remote_services",    // SSH-enablement marker (volatile)
		"/mnt/nv/aftertouch.resolv.conf",
	}
)

// probeSpeakerSSH runs one shell script over a single SSH connection
// and parses the result into a speakerProbe. The script emits framed
// blocks per file (base64-encoded so newlines/binary don't break the
// parser) and EXISTS lines per probe path.
func (m *Manager) probeSpeakerSSH(deviceIP string) *speakerProbe {
	probe := &speakerProbe{
		Files:  make(map[string]string),
		Exists: make(map[string]bool),
	}

	if m.NewSSH == nil {
		return probe
	}

	script := buildSpeakerProbeScript(probeFilePaths, probeExistsPaths)

	client := m.NewSSH(deviceIP)

	output, err := client.Run(script)
	if err != nil {
		probe.Err = err
		return probe
	}

	parseSpeakerProbe(probe, output)

	return probe
}

// buildSpeakerProbeScript composes the POSIX-sh script that does all the
// probes in one execution. Kept separate so tests can verify the script
// shape without having to mock an SSH transport.
func buildSpeakerProbeScript(filePaths, existsPaths []string) string {
	var b strings.Builder

	b.WriteString("echo '@SSH_OK@'\n")

	for _, p := range filePaths {
		b.WriteString("if [ -f '")
		b.WriteString(p)
		b.WriteString("' ]; then\n")
		b.WriteString("  echo '@FILE@")
		b.WriteString(p)
		b.WriteString("@'\n")
		b.WriteString("  base64 < '")
		b.WriteString(p)
		b.WriteString("' 2>/dev/null | tr -d '\\n'\n")
		b.WriteString("  echo\n")
		b.WriteString("  echo '@END@'\n")
		b.WriteString("fi\n")
	}

	for _, p := range existsPaths {
		b.WriteString("if [ -e '")
		b.WriteString(p)
		b.WriteString("' ]; then echo '@EXISTS@")
		b.WriteString(p)
		b.WriteString("@'; fi\n")
	}

	return b.String()
}

// parseSpeakerProbe parses the script's stdout into the probe struct.
// The format is line-oriented:
//
//	@SSH_OK@                 — sentinel: script ran to completion
//	@FILE@<path>@            — start-of-file marker
//	<base64 contents>        — exactly one line of base64 (no newlines)
//	@END@                    — end-of-file marker
//	@EXISTS@<path>@          — path-exists assertion
//
// We tolerate any other lines as stray output and skip them.
func parseSpeakerProbe(probe *speakerProbe, output string) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var (
		inFile      bool
		currentPath string
		b64         strings.Builder
	)

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case line == "@SSH_OK@":
			probe.SSHOK = true

		case strings.HasPrefix(line, "@FILE@") && strings.HasSuffix(line, "@"):
			currentPath = strings.TrimSuffix(strings.TrimPrefix(line, "@FILE@"), "@")
			inFile = true

			b64.Reset()

		case line == "@END@":
			if inFile && currentPath != "" {
				if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64.String())); err == nil {
					probe.Files[currentPath] = string(decoded)
				}
			}

			inFile = false
			currentPath = ""

			b64.Reset()

		case strings.HasPrefix(line, "@EXISTS@") && strings.HasSuffix(line, "@"):
			path := strings.TrimSuffix(strings.TrimPrefix(line, "@EXISTS@"), "@")
			probe.Exists[path] = true

		default:
			if inFile {
				b64.WriteString(line)
			}
		}
	}
}
