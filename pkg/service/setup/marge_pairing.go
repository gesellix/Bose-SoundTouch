package setup

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// PairAccountTimeouts bounds every step of the pairing call so a wedged
// device cannot stall the migration UI indefinitely.
const (
	supportedURLsTimeout = 3 * time.Second
	setMargeAccountConn  = 5 * time.Second
	setMargeAccountTotal = 12 * time.Second
	// wsEscalationStep bounds each WebSocket step of the escalation below.
	wsEscalationStep = 8 * time.Second
)

// wsSettleDelay gives the firmware a moment to act on the WS pairing before
// the state is re-read. runPairBare uses the same 2 s. A var so tests do not
// have to sit through it.
var wsSettleDelay = 2 * time.Second

// PairAccountResult records what was attempted, so the UI can show a
// breadcrumb of which path actually succeeded (or that both failed).
type PairAccountResult struct {
	SetMargeAccountSupported bool   `json:"set_marge_account_supported"`
	HTTPAttempted            bool   `json:"http_attempted"`
	HTTPError                string `json:"http_error,omitempty"`
	TelnetAttempted          bool   `json:"telnet_attempted"`
	TelnetError              string `json:"telnet_error,omitempty"`

	// StuckInSetup records that the account was written but the speaker was
	// still reporting source="SETUP" afterwards, which is the condition the
	// WebSocket escalation exists for. WSAttempted/WSError/LeftSetup describe
	// that escalation. LeftSetup is only meaningful when WSAttempted is true.
	StuckInSetup bool   `json:"stuck_in_setup"`
	WSAttempted  bool   `json:"ws_attempted"`
	WSError      string `json:"ws_error,omitempty"`
	LeftSetup    bool   `json:"left_setup"`

	Method string `json:"method"` // "http" | "telnet" | "ws" | ""
}

// PairAccount associates the speaker at deviceIP with accountID. It tries
// the device's HTTP /setMargeAccount endpoint first; on missing endpoint or
// any time-bounded failure it falls back to a telnet
// `envswitch accountid set <id>` over the supplied client. If telnet is nil
// or also fails, PairAccount returns a structured error explaining the next
// step a user can take.
//
// Both of those paths write the account ID without going through the
// firmware's own SETUP state machine, and there is reason to think that is
// not always enough: a speaker can end up with a populated
// margeAccountUUID while its firmware stays in source="SETUP", refusing to
// play anything (issue #646). So on success PairAccount checks whether the
// speaker actually left SETUP, and if it did not, escalates to the
// WebSocket setMargeAccount that `soundtouch-cli setup pair --mode=bare`
// uses, which is the path with positive precedent for moving a speaker out
// of that state.
//
// The escalation is deliberately gated on the observed failure rather than
// reordered ahead of the existing paths. Adding it as another fallback arm
// would not have helped: the case being targeted is one where the HTTP call
// *succeeds*, so a fallback chain never reaches it.
func (m *Manager) PairAccount(ctx context.Context, deviceIP, accountID string, t TelnetClient) (PairAccountResult, string, error) {
	var (
		result PairAccountResult
		logs   strings.Builder
	)

	if !datastore.IsSafeIdentifier(accountID) {
		return result, "", fmt.Errorf("invalid account ID %q: must be a non-empty, path-safe identifier", accountID)
	}

	supported, supportedErr := m.probeSetMargeAccount(deviceIP)
	result.SetMargeAccountSupported = supported

	switch {
	case supportedErr != nil:
		fmt.Fprintf(&logs, "supportedURLs probe failed: %v\n", supportedErr)
	case supported:
		logs.WriteString("supportedURLs lists /setMargeAccount — trying HTTP\n")
	default:
		logs.WriteString("supportedURLs does NOT list /setMargeAccount — skipping HTTP, going straight to telnet\n")
	}

	if supported {
		result.HTTPAttempted = true

		if err := m.postSetMargeAccount(deviceIP, accountID); err != nil {
			result.HTTPError = err.Error()

			fmt.Fprintf(&logs, "HTTP /setMargeAccount failed: %v\n", err)
		} else {
			result.Method = "http"

			logs.WriteString("HTTP /setMargeAccount succeeded\n")

			m.escalateIfStuckInSetup(ctx, deviceIP, accountID, &result, &logs)

			return result, logs.String(), nil
		}
	}

	if t == nil {
		return result, logs.String(), errors.New(
			"pairing failed: HTTP /setMargeAccount unavailable and no telnet client supplied — " +
				"open the official Bose app and pair manually before EOS, or use the SSH-based XML method")
	}

	result.TelnetAttempted = true

	// Safe to concatenate: datastore.IsSafeIdentifier (checked above) rejects
	// any whitespace or control characters, so accountID can't smuggle extra
	// tokens into this single-line telnet command.
	cmd := "envswitch accountid set " + accountID

	resp, err := t.SendCommand(cmd)
	if err != nil {
		result.TelnetError = err.Error()

		return result, logs.String(), fmt.Errorf("HTTP unavailable and telnet fallback failed: %w", err)
	}

	if isCommandNotFound(resp) {
		result.TelnetError = "envswitch accountid: command not found on this firmware"

		return result, logs.String(), errors.New(
			"pairing failed: HTTP /setMargeAccount missing AND telnet `envswitch accountid` rejected — " +
				"firmware does not expose either pairing path")
	}

	fmt.Fprintf(&logs, "Telnet %q → %s\n", cmd, strings.TrimRight(resp, "\r\n"))

	result.Method = "telnet"

	m.escalateIfStuckInSetup(ctx, deviceIP, accountID, &result, &logs)

	return result, logs.String(), nil
}

// nowPlayingXML is the slice of /now_playing this package cares about: the
// source attribute, which reads "SETUP" while the firmware is still in
// onboarding.
type nowPlayingXML struct {
	XMLName xml.Name `xml:"nowPlaying"`
	Source  string   `xml:"source,attr"`
}

// readNowPlayingSource returns the speaker's current source, e.g. "STANDBY",
// "TUNEIN", or "SETUP".
func (m *Manager) readNowPlayingSource(deviceIP string) (string, error) {
	url := fmt.Sprintf("http://%s:8090/now_playing", deviceIP)
	if _, _, err := net.SplitHostPort(deviceIP); err == nil {
		url = fmt.Sprintf("http://%s/now_playing", deviceIP)
	}

	resp, err := m.httpGet(url)
	if err != nil {
		return "", fmt.Errorf("fetch now_playing from %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	var np nowPlayingXML
	if err := xml.NewDecoder(resp.Body).Decode(&np); err != nil {
		return "", fmt.Errorf("decode now_playing from %s: %w", url, err)
	}

	return np.Source, nil
}

// escalateIfStuckInSetup runs after the account ID has been written by one of
// the paths above. If the speaker still reports source="SETUP" it drives the
// WebSocket setMargeAccount, which goes through the firmware's own setup state
// machine rather than around it.
//
// It never returns an error: the pairing itself already succeeded, and a
// speaker that cannot be coaxed out of SETUP is not a reason to report the
// pairing as failed. Everything observed lands in result and logs instead, so
// a field report says which path ran and whether it helped.
func (m *Manager) escalateIfStuckInSetup(ctx context.Context, deviceIP, accountID string, result *PairAccountResult, logs *strings.Builder) {
	source, err := m.readNowPlayingSource(deviceIP)
	if err != nil {
		fmt.Fprintf(logs, "post-pair SETUP check failed (continuing): %v\n", err)
		return
	}

	if !strings.EqualFold(source, "SETUP") {
		fmt.Fprintf(logs, "post-pair source=%q — speaker is out of SETUP\n", source)
		return
	}

	result.StuckInSetup = true

	logs.WriteString("post-pair source=\"SETUP\" — account written but firmware still onboarding, " +
		"escalating to the WebSocket setMargeAccount\n")

	if err := m.pairOverWebSocket(ctx, deviceIP, accountID); err != nil {
		result.WSAttempted = true
		result.WSError = err.Error()

		fmt.Fprintf(logs, "WebSocket setMargeAccount failed: %v\n", err)

		return
	}

	result.WSAttempted = true
	result.Method = "ws"

	logs.WriteString("WebSocket setMargeAccount succeeded\n")

	time.Sleep(wsSettleDelay)

	switch after, err := m.readNowPlayingSource(deviceIP); {
	case err != nil:
		fmt.Fprintf(logs, "could not re-read source after escalation: %v\n", err)
	case strings.EqualFold(after, "SETUP"):
		logs.WriteString("speaker still reports SETUP after the escalation\n")
	default:
		result.LeftSetup = true

		fmt.Fprintf(logs, "speaker left SETUP after the escalation (source=%q)\n", after)
	}
}

// pairOverWebSocket sends setMargeAccount through the speaker's own setup
// WebSocket. The payload is deliberately minimal (account ID plus the default
// auth token, no boseServer extras): this is an escalation of the same request
// the HTTP endpoint just took, not a migration, and it should not quietly
// rewrite the speaker's configured server URLs as a side effect.
func (m *Manager) pairOverWebSocket(ctx context.Context, deviceIP, accountID string) error {
	info, err := m.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		return fmt.Errorf("read /info for device ID: %w", err)
	}

	if info.DeviceID == "" {
		return errors.New("device reported no deviceID, cannot route WebSocket messages")
	}

	session, err := m.NewSession(deviceIP, info.DeviceID, wsEscalationStep)
	if err != nil {
		return fmt.Errorf("dial setup WebSocket: %w", err)
	}

	defer func() { _ = session.Close() }()

	stepCtx, cancel := context.WithTimeout(ctx, wsEscalationStep+2*time.Second)
	defer cancel()

	if err := session.SetMargeAccount(stepCtx, accountID, ""); err != nil {
		return fmt.Errorf("setMargeAccount over WebSocket: %w", err)
	}

	return nil
}

// EnsureMargeAccountPaired reads the device's /info and, if margeAccountUUID
// is empty (a genuinely unpaired, factory-reset device), pairs it via
// PairAccount using wantAccountID if given, otherwise a freshly generated ID.
// See #515 comment 5230833551: on an unpaired device, margeServerUrl is
// reportedly never polled at all, so the boseurls SSH-enable injection has no
// read cycle to fire on regardless of command delay — pairing first gives it
// one. accountID is empty when GetLiveDeviceInfo itself fails; otherwise it
// is either the device's existing margeAccountUUID (alreadyPaired=true) or
// the account ID just paired with.
func (m *Manager) EnsureMargeAccountPaired(ctx context.Context, deviceIP, wantAccountID string, t TelnetClient) (accountID string, alreadyPaired bool, logs string, err error) {
	info, infoErr := m.GetLiveDeviceInfo(deviceIP)
	if infoErr != nil {
		return "", false, "", fmt.Errorf("read /info: %w", infoErr)
	}

	if info.MargeAccountUUID != "" {
		return info.MargeAccountUUID, true, "", nil
	}

	target := wantAccountID
	if target == "" {
		generated, genErr := GenerateAccountID(nil)
		if genErr != nil {
			return "", false, "", fmt.Errorf("generate account id: %w", genErr)
		}

		target = generated
	} else if !datastore.IsSafeIdentifier(target) {
		return "", false, "", fmt.Errorf("invalid account id %q: must be a non-empty, path-safe identifier", target)
	}

	_, pairLogs, pairErr := m.PairAccount(ctx, deviceIP, target, t)
	if pairErr != nil {
		return target, false, pairLogs, pairErr
	}

	return target, false, pairLogs, nil
}

// probeSetMargeAccount fetches /supportedURLs and reports whether
// /setMargeAccount is in the listing.
func (m *Manager) probeSetMargeAccount(deviceIP string) (bool, error) {
	url := buildDeviceURL(deviceIP, "/supportedURLs")

	client := &http.Client{Timeout: supportedURLsTimeout}

	resp, err := client.Get(url)
	if err != nil {
		return false, fmt.Errorf("GET %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", url, err)
	}

	var doc struct {
		URLs []struct {
			Location string `xml:"location,attr"`
		} `xml:"URL"`
	}

	if err := xml.Unmarshal(body, &doc); err != nil {
		// Fallback to substring match — some firmwares return a slightly
		// different XML root that Go's strict parser refuses.
		return strings.Contains(string(body), "/setMargeAccount"), nil
	}

	for _, u := range doc.URLs {
		if u.Location == "/setMargeAccount" {
			return true, nil
		}
	}

	return false, nil
}

// postSetMargeAccount sends the pairing XML body to the device's
// /setMargeAccount endpoint with bounded timeouts.
func (m *Manager) postSetMargeAccount(deviceIP, accountID string) error {
	url := buildDeviceURL(deviceIP, "/setMargeAccount")

	// accountID is XML-escaped rather than interpolated raw:
	// datastore.IsSafeIdentifier already excludes '<', '>', '&', '\'', '"'
	// (see #634), but escaping here too means this stays well-formed even
	// if that gate is ever bypassed.
	var escapedAccountID bytes.Buffer
	if err := xml.EscapeText(&escapedAccountID, []byte(accountID)); err != nil {
		return fmt.Errorf("escape account ID: %w", err)
	}

	body := fmt.Sprintf(
		`<PairDeviceWithAccount><accountId>%s</accountId><userAuthToken>aftertouch</userAuthToken></PairDeviceWithAccount>`,
		escapedAccountID.String(),
	)

	client := &http.Client{
		Timeout: setMargeAccountTotal,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: setMargeAccountConn}).DialContext,
			ResponseHeaderTimeout: setMargeAccountTotal - setMargeAccountConn,
		},
	}

	resp, err := client.Post(url, "application/xml", strings.NewReader(body))
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

// ConfigurationStatus values reported by GET /soundTouchConfigurationStatus.
// See issue #615: a speaker can be reachable, named, and already
// account-paired yet still report SOUNDTOUCH_NOT_CONFIGURED, which leaves
// the firmware nagging the owner to install the Bose app. Only a full pass
// through the WebSocket setup state machine (ExecuteInitPlan) clears it.
const (
	ConfigurationStatusConfigured    = "SOUNDTOUCH_CONFIGURED"
	ConfigurationStatusNotConfigured = "SOUNDTOUCH_NOT_CONFIGURED"
)

// ReadConfigurationStatus fetches /soundTouchConfigurationStatus and returns
// its raw status attribute (e.g. "SOUNDTOUCH_CONFIGURED").
func (m *Manager) ReadConfigurationStatus(deviceIP string) (string, error) {
	url := buildDeviceURL(deviceIP, "/soundTouchConfigurationStatus")

	client := &http.Client{Timeout: supportedURLsTimeout}

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}

	var doc struct {
		Status string `xml:"status,attr"`
	}

	if err := xml.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("parse %s: %w", url, err)
	}

	return doc.Status, nil
}

// PreflightInitPlan reports whether ExecuteInitPlan should be run against
// deviceIP, gated on the two conditions from issue #615: /setMargeAccount
// must be listed in /supportedURLs, and the device's current
// /soundTouchConfigurationStatus must be exactly SOUNDTOUCH_NOT_CONFIGURED.
// needed=false with a nil error means "already configured, nothing to do."
// Any other outcome (unsupported route, unrecognised status value) is
// treated as unknown and returned as an error rather than guessed at.
func (m *Manager) PreflightInitPlan(deviceIP string) (needed bool, status string, err error) {
	supported, probeErr := m.probeSetMargeAccount(deviceIP)
	if probeErr != nil {
		return false, "", fmt.Errorf("supportedURLs probe: %w", probeErr)
	}

	if !supported {
		return false, "", errors.New("/setMargeAccount is not listed in /supportedURLs — device does not support this pairing path")
	}

	status, err = m.ReadConfigurationStatus(deviceIP)
	if err != nil {
		return false, "", fmt.Errorf("read /soundTouchConfigurationStatus: %w", err)
	}

	switch status {
	case ConfigurationStatusConfigured:
		return false, status, nil
	case ConfigurationStatusNotConfigured:
		return true, status, nil
	default:
		return false, status, fmt.Errorf("unexpected /soundTouchConfigurationStatus value %q", status)
	}
}

// buildDeviceURL builds a URL for a SoundTouch device's HTTP API. If
// deviceIP already includes a port (test scenarios using httptest) it is
// reused as-is; otherwise the canonical port 8090 is appended.
func buildDeviceURL(deviceIP, path string) string {
	if _, _, err := net.SplitHostPort(deviceIP); err == nil {
		return "http://" + deviceIP + path
	}

	return "http://" + deviceIP + ":8090" + path
}

// GenerateAccountID returns a fresh 7-digit account ID that does not collide
// with any value in known. It uses crypto/rand and re-rolls on collision.
func GenerateAccountID(known []string) (string, error) {
	taken := make(map[string]bool, len(known))
	for _, k := range known {
		taken[k] = true
	}

	const maxAttempts = 32

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// 7-digit space starts at 1_000_000 to avoid leading zeros, ending at
		// 9_999_999. Range size is 9_000_000.
		n, err := rand.Int(rand.Reader, big.NewInt(9_000_000))
		if err != nil {
			return "", fmt.Errorf("crypto/rand: %w", err)
		}

		candidate := fmt.Sprintf("%07d", n.Int64()+1_000_000)
		if !taken[candidate] {
			return candidate, nil
		}
	}

	return "", errors.New("could not generate a non-colliding account ID after 32 attempts")
}
