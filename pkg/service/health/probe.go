package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProbeResult captures the outcome of an HTTP probe against an
// external endpoint (typically a speaker on the LAN). Reachable is
// true when the server-side fetch succeeded; otherwise CurlCommand
// holds a ready-to-paste fallback so the operator can run the same
// request from a machine that *can* reach the target.
//
// Body is only populated when Reachable is true; otherwise Err
// carries the underlying transport error.
type ProbeResult struct {
	URL         string
	Reachable   bool
	Status      int
	Body        []byte
	Err         string
	CurlCommand string
}

// ProbeGet issues an HTTP GET against rawURL using a short timeout.
// CurlCommand is populated in every result, regardless of success,
// so the UI can always offer "run this from your LAN" as a fallback
// or as a copy-the-actual-command affordance.
//
// This is intentionally a tiny wrapper around net/http rather than
// a fully-fledged probe abstraction: the slice is one of "we tried
// from here, here's what we got" plus "or run this elsewhere".
//
// ctx is required; pass context.Background() when no caller context
// is available.
func ProbeGet(ctx context.Context, rawURL string, timeout time.Duration) ProbeResult {
	result := ProbeResult{
		URL:         rawURL,
		CurlCommand: curlForGet(rawURL),
	}

	if _, err := url.Parse(rawURL); err != nil {
		result.Err = "invalid url: " + err.Error()
		return result
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		result.Err = err.Error()
		return result
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		result.Err = err.Error()
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		result.Status = resp.StatusCode
		result.Err = "read body: " + err.Error()

		return result
	}

	result.Reachable = true
	result.Status = resp.StatusCode
	result.Body = body

	return result
}

// curlForGet renders the curl command the operator would run to
// reproduce the GET from a different host. Single-quotes the URL
// so shell metacharacters in query strings don't confuse copy-paste.
func curlForGet(rawURL string) string {
	if strings.Contains(rawURL, "'") {
		// Extremely rare; fall back to double-quotes when the URL
		// itself contains a single quote.
		return fmt.Sprintf("curl -sS \"%s\"", rawURL)
	}

	return fmt.Sprintf("curl -sS '%s'", rawURL)
}
