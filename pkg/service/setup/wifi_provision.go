package setup

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// SpeakerSetupAP is the IP address a SoundTouch speaker assigns itself
// when in setup mode. Verified on ST10 (assigns 192.0.2.2 to the client
// via DHCP).
const SpeakerSetupAP = "192.0.2.1"

// DefaultWiFiSecurity matches what the official setup wizard sends for
// home networks. The device accepts this string for both WPA and WPA2.
const DefaultWiFiSecurity = "wpa_or_wpa2"

// PushWiFiCredentialsParams holds the inputs for PushWiFiCredentials.
type PushWiFiCredentialsParams struct {
	// APHost is the speaker's setup-mode address. Defaults to SpeakerSetupAP.
	APHost string
	// SSID and Password identify the home network to join.
	SSID     string
	Password string
	// Security defaults to DefaultWiFiSecurity.
	Security string
	// HTTPClient lets callers (mostly tests) override the transport. Nil
	// uses a 10-second default client.
	HTTPClient *http.Client
}

// PushWiFiCredentials POSTs an AddWirelessProfile XML to the speaker's
// setup-mode endpoint, instructing it to drop AP mode and join the named
// network. The caller must already be connected to the speaker's Wi-Fi.
//
// The speaker confirms the request before disconnecting; expect to lose
// the AP link within ~30 seconds.
//
// Empirically the first POST often races the speaker's setup endpoint
// readiness — the connection times out, then a second POST a few seconds
// later succeeds immediately. We retry once internally so the caller
// doesn't have to.
func PushWiFiCredentials(ctx context.Context, p PushWiFiCredentialsParams) error {
	if p.SSID == "" {
		return fmt.Errorf("PushWiFiCredentials: SSID is required")
	}

	host := p.APHost
	if host == "" {
		host = SpeakerSetupAP
	}

	security := p.Security
	if security == "" {
		security = DefaultWiFiSecurity
	}

	body := fmt.Sprintf(
		`<AddWirelessProfile><profile ssid="%s" password="%s" securityType="%s" /></AddWirelessProfile>`,
		xmlAttrEscape(p.SSID), xmlAttrEscape(p.Password), xmlAttrEscape(security),
	)

	hostPort := host
	if _, _, err := net.SplitHostPort(host); err != nil {
		hostPort = host + ":8090"
	}

	url := "http://" + hostPort + "/addWirelessProfile"

	httpClient := p.HTTPClient
	if httpClient == nil {
		// No client-side timeout: let the per-attempt sub-context
		// govern. The CLI passes a context deadline (default 30 s
		// in setupWiFiPushCmd) and a hard-coded 10 s here would
		// race it for no benefit.
		httpClient = &http.Client{}
	}

	// Per-attempt cap so a stuck first attempt doesn't burn the whole
	// budget. 12 s is well above the typical sub-second response time
	// when the endpoint is healthy, and the failure mode we're working
	// around (first attempt hangs until the deadline elapses) means
	// any value here is mostly a sub-budget for a stuck attempt.
	const perAttemptTimeout = 12 * time.Second
	// Pause between attempts gives the speaker's setup endpoint a
	// moment to finish whatever initialization the first POST kicked
	// off (the empirical workaround that motivated this retry).
	const interAttemptDelay = 2 * time.Second

	attempt := func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}

		req.Header.Set("Content-Type", "text/xml")

		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("POST %s: %w", url, err)
		}

		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("POST %s returned %d: %s", url, resp.StatusCode, strings.TrimSpace(string(respBody)))
		}

		return nil
	}

	// Two attempts: the second is silent on the wire when the first
	// already succeeded (returns at the first non-error), or carries
	// the recovery when the first failed.
	const maxAttempts = 2

	var lastErr error

	for i := 0; i < maxAttempts; i++ {
		if i > 0 {
			select {
			case <-time.After(interAttemptDelay):
			case <-ctx.Done():
				return fmt.Errorf("PushWiFiCredentials: %w (last attempt error: %w)", ctx.Err(), lastErr)
			}
		}

		attemptCtx, cancel := context.WithTimeout(ctx, perAttemptTimeout)
		err := attempt(attemptCtx)

		cancel()

		if err == nil {
			return nil
		}

		lastErr = err
	}

	return fmt.Errorf("PushWiFiCredentials: both attempts failed (last: %w)", lastErr)
}

// PollConfig governs the retry cadence of WaitForAP and WaitForOnline.
type PollConfig struct {
	// Interval between probes. Default 2 s.
	Interval time.Duration
	// Timeout is the total wall-clock budget. Default 5 min.
	Timeout time.Duration
}

func (c PollConfig) interval() time.Duration {
	if c.Interval <= 0 {
		return 2 * time.Second
	}

	return c.Interval
}

func (c PollConfig) timeout() time.Duration {
	if c.Timeout <= 0 {
		return 5 * time.Minute
	}

	return c.Timeout
}

// WaitForAP blocks until the speaker at apHost answers /info on its
// setup-mode HTTP endpoint, then returns the parsed info. apHost
// defaults to SpeakerSetupAP. The caller is expected to have switched
// the host machine to the speaker's setup-mode Wi-Fi network manually.
//
// HTTPGet is the dependency-injection point so tests can supply a fake
// without spinning a server on 192.0.2.1.
func WaitForAP(ctx context.Context, apHost string, cfg PollConfig, httpGet func(string) (*http.Response, error)) (*DeviceInfoXML, error) {
	if apHost == "" {
		apHost = SpeakerSetupAP
	}

	if httpGet == nil {
		client := &http.Client{Timeout: 3 * time.Second}
		httpGet = client.Get
	}

	deadline := time.Now().Add(cfg.timeout())

	hostPort := apHost
	if _, _, err := net.SplitHostPort(apHost); err != nil {
		hostPort = apHost + ":8090"
	}

	infoURL := "http://" + hostPort + "/info"

	for {
		info, err := tryFetchInfo(httpGet, infoURL)
		if err == nil && info != nil {
			return info, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("WaitForAP: %w", ctx.Err())
		case <-time.After(cfg.interval()):
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("WaitForAP: %s did not respond within %s", infoURL, cfg.timeout())
		}
	}
}

// tryFetchInfo performs one /info probe; returns nil on any failure so
// the polling loop can decide whether to retry.
func tryFetchInfo(httpGet func(string) (*http.Response, error), infoURL string) (*DeviceInfoXML, error) {
	resp, err := httpGet(infoURL)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	m := &Manager{}

	var info DeviceInfoXML
	if err := m.parseDeviceInfoXML(strings.NewReader(string(body)), &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// MDNSDiscoverer is the discovery-side capability WaitForOnline depends
// on. The real implementation is *discovery.MDNSDiscoveryService; tests
// inject a stub.
type MDNSDiscoverer interface {
	DiscoverDevices(ctx context.Context) ([]*models.DiscoveredDevice, error)
}

// WaitForOnline polls mDNS for a SoundTouch speaker matching the given
// substring (typically a device-ID suffix such as "DE4803"). It returns
// the speaker's IP address as soon as it reappears on the home network
// after a Wi-Fi provision.
//
// matcher is matched case-insensitively against DiscoveredDevice.Name,
// SerialNo, and Host. An empty matcher returns the first speaker seen.
func WaitForOnline(ctx context.Context, matcher string, cfg PollConfig, mdns MDNSDiscoverer) (*models.DiscoveredDevice, error) {
	if mdns == nil {
		mdns = discovery.NewMDNSDiscoveryService(cfg.interval())
	}

	deadline := time.Now().Add(cfg.timeout())
	needle := strings.ToLower(matcher)

	for {
		devs, _ := mdns.DiscoverDevices(ctx)

		for _, d := range devs {
			if needle == "" || matchesDevice(d, needle) {
				return d, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("WaitForOnline: %w", ctx.Err())
		case <-time.After(cfg.interval()):
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("WaitForOnline: no speaker matching %q discovered within %s", matcher, cfg.timeout())
		}
	}
}

func matchesDevice(d *models.DiscoveredDevice, needle string) bool {
	if d == nil {
		return false
	}

	for _, candidate := range []string{d.Name, d.SerialNo, d.Host, d.UPnPSerial} {
		if candidate != "" && strings.Contains(strings.ToLower(candidate), needle) {
			return true
		}
	}

	return false
}
