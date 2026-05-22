package handlers

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/health"
)

// addMargeHostToTLSFix is the FixFunc registered for the
// (CheckIDSpeakerMargeURL, FixIDAddMargeHostToTLS) pair. It re-probes
// the target device's <margeURL>, extracts the host portion, and
// appends it to the persisted Settings.TLSExtraHosts. A subsequent
// service restart picks up the change via the regular settings
// merge path in applyPersistedSettings (cmd/soundtouch-service/main.go).
//
// The re-probe is deliberate: the persisted Settings only become
// authoritative after the operator restarts AfterTouch, so reading
// the margeURL fresh from the speaker avoids racing a stale finding
// that was rendered before the speaker rebooted.
//
// Returns a success message that names the host and instructs the
// operator to restart the service. Returns an error if the device
// can't be located, the probe fails, or the marge URL is empty /
// unparseable.
func (s *Server) addMargeHostToTLSFix(target health.Target) (string, error) {
	if target.Device == "" {
		return "", fmt.Errorf("device is required")
	}

	deviceIP, err := s.resolveDeviceIDToIP(target.Device)
	if err != nil {
		return "", fmt.Errorf("locate device %s: %w", target.Device, err)
	}

	probeURL := fmt.Sprintf("http://%s:8090/info", deviceIP)

	margeHost, err := fetchMargeHostFromSpeaker(probeURL, 2*time.Second)
	if err != nil {
		return "", err
	}

	if margeHost == "" {
		return "", fmt.Errorf("speaker %s has no <margeURL>; nothing to add", target.Device)
	}

	persisted, err := s.ds.GetSettings()
	if err != nil {
		return "", fmt.Errorf("load settings: %w", err)
	}

	for _, existing := range persisted.TLSExtraHosts {
		if strings.EqualFold(strings.TrimSpace(existing), margeHost) {
			return fmt.Sprintf("%s is already in the persisted TLS hosts. Restart AfterTouch to regenerate the TLS certificate if you haven't already.", margeHost), nil
		}
	}

	persisted.TLSExtraHosts = append(persisted.TLSExtraHosts, margeHost)

	if err := s.ds.SaveSettings(persisted); err != nil {
		return "", fmt.Errorf("save settings: %w", err)
	}

	return fmt.Sprintf("Added %s to persisted TLS hosts (tls_extra_hosts). Restart AfterTouch for the TLS certificate to be regenerated and include this host in its SAN list.", margeHost), nil
}

// fetchMargeHostFromSpeaker probes the given speaker /info URL and
// returns the host portion of the <margeURL> XML element. Returns an
// empty string with no error when the speaker responds but doesn't
// carry a margeURL. Returns a non-nil error when the probe itself
// fails or the response can't be parsed.
func fetchMargeHostFromSpeaker(probeURL string, timeout time.Duration) (string, error) {
	res := health.ProbeGet(context.Background(), probeURL, timeout)
	if !res.Reachable {
		return "", fmt.Errorf("speaker probe failed: %s", res.Err)
	}

	if res.Status != 200 {
		return "", fmt.Errorf("speaker /info returned HTTP %d", res.Status)
	}

	var parsed struct {
		MargeURL string `xml:"margeURL"`
	}

	if err := xml.Unmarshal(res.Body, &parsed); err != nil {
		return "", fmt.Errorf("parse /info: %w", err)
	}

	if parsed.MargeURL == "" {
		return "", nil
	}

	u, err := url.Parse(parsed.MargeURL)
	if err != nil {
		return "", fmt.Errorf("parse margeURL %q: %w", parsed.MargeURL, err)
	}

	host := u.Hostname()
	if host == "" {
		host = strings.TrimSpace(parsed.MargeURL)
	}

	return host, nil
}
