package setup

import (
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
)

// PairAccountTimeouts bounds every step of the pairing call so a wedged
// device cannot stall the migration UI indefinitely.
const (
	supportedURLsTimeout = 3 * time.Second
	setMargeAccountConn  = 5 * time.Second
	setMargeAccountTotal = 12 * time.Second
)

// PairAccountResult records what was attempted, so the UI can show a
// breadcrumb of which path actually succeeded (or that both failed).
type PairAccountResult struct {
	SetMargeAccountSupported bool   `json:"set_marge_account_supported"`
	HTTPAttempted            bool   `json:"http_attempted"`
	HTTPError                string `json:"http_error,omitempty"`
	TelnetAttempted          bool   `json:"telnet_attempted"`
	TelnetError              string `json:"telnet_error,omitempty"`
	Method                   string `json:"method"` // "http" | "telnet" | ""
}

// PairAccount associates the speaker at deviceIP with accountID. It tries
// the device's HTTP /setMargeAccount endpoint first; on missing endpoint or
// any time-bounded failure it falls back to a telnet
// `envswitch accountid set <id>` over the supplied client. If telnet is nil
// or also fails, PairAccount returns a structured error explaining the next
// step a user can take.
func (m *Manager) PairAccount(deviceIP, accountID string, t TelnetClient) (PairAccountResult, string, error) {
	var (
		result PairAccountResult
		logs   strings.Builder
	)

	if !IsValidAccountID(accountID) {
		return result, "", fmt.Errorf("invalid account ID %q: must be exactly 7 digits", accountID)
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

			return result, logs.String(), nil
		}
	}

	if t == nil {
		return result, logs.String(), errors.New(
			"pairing failed: HTTP /setMargeAccount unavailable and no telnet client supplied — " +
				"open the official Bose app and pair manually before EOS, or use the SSH-based XML method")
	}

	result.TelnetAttempted = true

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

	return result, logs.String(), nil
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

	body := fmt.Sprintf(
		`<PairDeviceWithAccount><accountId>%s</accountId><userAuthToken>aftertouch</userAuthToken></PairDeviceWithAccount>`,
		accountID,
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

// buildDeviceURL builds a URL for a SoundTouch device's HTTP API. If
// deviceIP already includes a port (test scenarios using httptest) it is
// reused as-is; otherwise the canonical port 8090 is appended.
func buildDeviceURL(deviceIP, path string) string {
	if _, _, err := net.SplitHostPort(deviceIP); err == nil {
		return "http://" + deviceIP + path
	}

	return "http://" + deviceIP + ":8090" + path
}

// IsValidAccountID reports whether s is a syntactically valid SoundTouch
// account ID — exactly 7 numeric digits, the format used by every
// Bose-cloud-issued ID we have observed in captures.
func IsValidAccountID(s string) bool {
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
