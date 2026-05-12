package setup

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// applyProbeToSummary populates the SSH-derived fields of a
// MigrationSummary directly from a batched speakerProbe. Mirrors what
// the per-helper path (checkCurrentConfig + checkRemoteServices +
// checkCACertTrusted + the inline resolv read) used to do across
// multiple SSH dials.
//
// One subtle difference from the legacy path: the original
// checkCurrentConfig has a fallback that reads the file via base64 when
// `cat` returns empty but the file has size > 0. The batched script
// already does base64 for every file, so that fallback is implicit —
// any readable file appears in probe.Files.
func (m *Manager) applyProbeToSummary(
	summary *MigrationSummary,
	probe *speakerProbe,
	plannedCfg *PrivateCfg,
	proxyURL, targetURL string,
	options map[string]string,
) {
	summary.SSHSuccess = probe.SSHOK

	if !probe.SSHOK && probe.Err != nil {
		summary.CurrentConfig = fmt.Sprintf("SSH connection failed: %v", probe.Err)
	}

	// Current SoundTouchSdkPrivateCfg.xml (+ .original backup, if any)
	if cfg, ok := probe.Files[SoundTouchSdkPrivateCfgPath]; ok && cfg != "" {
		summary.CurrentConfig = cfg
		fmt.Printf("Current config from %s (length: %d):\n%q\n", probeDeviceTagFor(summary), len(cfg), cfg)

		var currentCfg PrivateCfg
		if xml.Unmarshal([]byte(cfg), &currentCfg) == nil {
			summary.ParsedCurrentConfig = &currentCfg

			if proxyURL == "" {
				proxyURL = targetURL
			}

			if options != nil {
				m.applyProxyOptions(plannedCfg, proxyURL, options, &currentCfg)
			}
		}
	}

	if orig, ok := probe.Files[SoundTouchSdkPrivateCfgPath+".original"]; ok && orig != "" {
		summary.OriginalConfig = orig
	}

	// /etc/resolv.conf — cached so checkIsMigratedFromProbe doesn't dial again
	if resolv, ok := probe.Files["/etc/resolv.conf"]; ok {
		summary.CurrentResolvConf = resolv
	}

	// remote_services markers (SSH enablement state)
	for _, loc := range []string{"/etc/remote_services", "/mnt/nv/remote_services", "/tmp/remote_services"} {
		if probe.Exists[loc] {
			summary.RemoteServicesFound = append(summary.RemoteServicesFound, loc)
			summary.RemoteServicesEnabled = true

			if loc != "/tmp/remote_services" {
				summary.RemoteServicesPersistent = true
			}
		}
	}

	// CA trust: check the ca-bundle for our injection label first; fall
	// back to matching the cert payload itself when Manager.Crypto is set
	// (web-UI/in-process path; the remote CLI has no Crypto so this stays
	// false until install-ca is run).
	if bundle, ok := probe.Files["/etc/pki/tls/certs/ca-bundle.crt"]; ok && bundle != "" {
		switch {
		case strings.Contains(bundle, CALabel):
			summary.CACertTrusted = true
		case m.Crypto != nil:
			if caCertPEM, err := os.ReadFile(m.Crypto.GetCACertPath()); err == nil {
				for _, line := range strings.Split(string(caCertPEM), "\n") {
					if line == "" || strings.Contains(line, "BEGIN CERTIFICATE") || strings.Contains(line, "END CERTIFICATE") {
						continue
					}

					if strings.Contains(bundle, line) {
						summary.CACertTrusted = true
					}

					break
				}
			}
		}
	}
}

// probeDeviceTagFor returns a short identifier for the device used in the
// "Current config from …" log line. We keep the legacy log shape so any
// downstream log scraping continues to work.
func probeDeviceTagFor(summary *MigrationSummary) string {
	if summary.DeviceID != "" {
		return summary.DeviceID
	}

	return "unknown"
}

// checkIsMigratedFromProbe is the probe-driven equivalent of
// checkIsMigrated. Unlike the legacy path it makes no fresh SSH dials —
// every file it inspects came from the single batched probe.
//
// Behavioural note: the legacy isResolvConfMigrated has a third fallback
// that DNS-resolves the target host via SSH (`getent` on the device) so
// it can match the *resolved* IP against resolv.conf. The probe path
// skips that — it would force a second SSH dial just for the corner
// case where the device's resolv.conf has the resolved IP but neither
// the marker comment nor the target hostname. In practice the hook file
// or marker comment is always present, so this is acceptable.
func (m *Manager) checkIsMigratedFromProbe(summary *MigrationSummary, probe *speakerProbe) {
	summary.TelnetMigrated = m.isTelnetMigrated(summary)

	if probe.SSHOK {
		summary.XMLMigrated = m.isXMLMigrated(summary)
		summary.HostsMigrated = isHostsMigratedFromProbe(probe, summary)
		summary.ResolvMigrated = m.isResolvConfMigratedFromProbe(probe, summary)
	}

	summary.IsMigrated = summary.TelnetMigrated ||
		summary.XMLMigrated ||
		summary.HostsMigrated ||
		summary.ResolvMigrated
}

func isHostsMigratedFromProbe(probe *speakerProbe, summary *MigrationSummary) bool {
	hostsContent, ok := probe.Files["/etc/hosts"]
	if !ok {
		return false
	}

	boseDomains := []string{
		"streaming.bose.com",
		"updates.bose.com",
		"stats.bose.com",
		"bmx.bose.com",
	}

	for _, domain := range boseDomains {
		if strings.Contains(hostsContent, domain) && summary.CACertTrusted {
			return true
		}
	}

	return false
}

func (m *Manager) isResolvConfMigratedFromProbe(probe *speakerProbe, summary *MigrationSummary) bool {
	if probe.Exists["/mnt/nv/aftertouch.resolv.conf"] {
		return summary.CACertTrusted
	}

	if summary.CurrentResolvConf == "" {
		return false
	}

	if strings.Contains(summary.CurrentResolvConf, "# Priority nameserver for Bose service redirection") && summary.CACertTrusted {
		return true
	}

	parsedTarget, err := url.Parse(m.ServerURL)
	if err != nil {
		return false
	}

	targetHost := parsedTarget.Hostname()
	if strings.Contains(summary.CurrentResolvConf, targetHost) && summary.CACertTrusted {
		return true
	}

	return false
}
