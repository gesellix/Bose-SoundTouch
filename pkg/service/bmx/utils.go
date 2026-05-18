package bmx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var defaultClient = &http.Client{Timeout: 10 * time.Second}

func fetchJSONMap(client *http.Client, fetchURL string, allowedHosts map[string]bool) (map[string]interface{}, error) {
	result, err := fetchJSONGeneric(client, fetchURL, allowedHosts)
	if err != nil {
		return nil, err
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected map[string]interface{}, got %T", result)
	}

	return m, nil
}

func fetchJSONGeneric(client *http.Client, fetchURL string, allowedHosts map[string]bool) (interface{}, error) {
	if allowedHosts != nil {
		if !isHostAllowed(fetchURL, allowedHosts) {
			return nil, fmt.Errorf("URL host not in allowed list: %s", fetchURL)
		}
	}

	resp, err := client.Get(fetchURL)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch failed with status %d: %s", resp.StatusCode, fetchURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func isHostAllowed(rawURL string, allowedHosts map[string]bool) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return allowedHosts[u.Hostname()]
}

func decodeBase64URI(encoded string) (string, error) {
	// Clean up input for base64 decoding (remove potential whitespace or prefixes)
	encoded = strings.TrimSpace(encoded)

	// Attempt URL-safe decoding first
	b, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		// Try again with padding if missing
		padding := len(encoded) % 4
		if padding > 0 {
			padded := encoded + strings.Repeat("=", 4-padding)
			b, err = base64.URLEncoding.DecodeString(padded)
		}
	}

	if err != nil {
		// Attempt standard decoding
		b, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			padding := len(encoded) % 4
			if padding > 0 {
				padded := encoded + strings.Repeat("=", 4-padding)
				b, err = base64.StdEncoding.DecodeString(padded)
			}
		}
	}

	if err != nil {
		// Try raw (no padding) decoding specifically
		b, err = base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(encoded)
		}
	}

	if err != nil {
		// FINAL DESPERATE ATTEMPT: decode by hand or check if it's just plain text
		// (though it shouldn't be). Some tests might be passing "illegal base64 data"
		// on purpose to test error handling? No, the tests themselves are failing.
		return "", err
	}

	return string(b), nil
}
