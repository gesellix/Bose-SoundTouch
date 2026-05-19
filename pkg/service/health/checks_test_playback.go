package health

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// CheckIDTestPlayback is the registry id of the test-playback
// affordance. Unlike most checks, this one doesn't surface bugs —
// it exposes a one-click "play the AfterTouch ding on each
// speaker" button so operators can confirm a freshly migrated
// speaker actually emits sound, without depending on TuneIn or
// any external service.
const CheckIDTestPlayback = "playback_test"

// FixIDPlayDing identifies the quick-fix that pushes the ding URL
// to the speaker as a custom-radio ContentItem.
const FixIDPlayDing = "play_ding"

// DingMediaPath is the path the embedded WAV is served from by
// pkg/service/handlers (see static/media embed).
const DingMediaPath = "/media/aftertouch-ding.wav"

// RegisterTestPlaybackCheck registers the playback_test check and
// its play_ding quick fix. serverURLFn returns the externally
// reachable URL of this service — the speaker fetches the audio
// from "<serverURL><DingMediaPath>". A blank serverURL disables
// the check (the speaker would have nowhere to fetch from).
func RegisterTestPlaybackCheck(r *Registry, ds *datastore.DataStore, serverURLFn func() string) {
	r.Register(Check{
		ID:    CheckIDTestPlayback,
		Title: "Test playback (\"ding\")",
		Run: func() []Finding {
			return runTestPlaybackCheck(ds, serverURLFn())
		},
	})

	r.RegisterFix(CheckIDTestPlayback, FixIDPlayDing, func(target Target) (string, error) {
		return playDingOnDevice(ds, serverURLFn(), target)
	})
}

func runTestPlaybackCheck(ds *datastore.DataStore, serverURL string) []Finding {
	if ds == nil {
		return nil
	}

	if strings.TrimSpace(serverURL) == "" {
		return []Finding{{
			Severity: SeverityInfo,
			Message:  "No external server URL is configured, so speakers can't fetch the test ding.",
			Details:  "Set SERVER_URL (or --server-url) to an address the speaker can reach AfterTouch on, then refresh.",
		}}
	}

	devices, err := ds.ListAllDevices()
	if err != nil {
		return []Finding{{
			Severity: SeverityError,
			Message:  "Could not enumerate devices: " + err.Error(),
		}}
	}

	out := make([]Finding, 0, len(devices))

	for i := range devices {
		dev := &devices[i]
		if dev.IPAddress == "" || dev.DeviceID == "" {
			continue
		}

		out = append(out, Finding{
			Severity: SeverityInfo,
			Target:   Target{Account: dev.AccountID, Device: dev.DeviceID},
			Message:  fmt.Sprintf("Play the AfterTouch ding on %s.", displayName(dev.Name, dev.DeviceID)),
			Details:  fmt.Sprintf("Pushes %s%s to the speaker via a custom-radio ContentItem. Confirms migration is healthy end-to-end without depending on TuneIn or any external service.", serverURL, DingMediaPath),
			QuickFixes: []QuickFix{{
				ID:    FixIDPlayDing,
				Label: "Play ding",
			}},
			ManualCommands: []ManualCommand{{
				Label:   "Or trigger from the LAN:",
				Command: dingCurlCommand(dev.IPAddress, serverURL),
				Hint:    "Run from a host that can reach both AfterTouch (for the media URL) and the speaker (port 8090). Speaker will fetch the audio from the service.",
			}},
		})
	}

	return out
}

func playDingOnDevice(ds *datastore.DataStore, serverURL string, target Target) (string, error) {
	if strings.TrimSpace(serverURL) == "" {
		return "", fmt.Errorf("no external server URL is configured")
	}

	if target.Device == "" {
		return "", fmt.Errorf("device is required")
	}

	dev, err := ds.GetDeviceInfo(target.Account, target.Device)
	if err != nil || dev == nil {
		return "", fmt.Errorf("device %s not found in datastore", target.Device)
	}

	if dev.IPAddress == "" {
		return "", fmt.Errorf("device %s has no IP address recorded", target.Device)
	}

	mediaURL := serverURL + DingMediaPath
	contentItem := buildDingContentItem(mediaURL)
	selectURL := fmt.Sprintf("http://%s:8090/select", dev.IPAddress)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, selectURL, bytes.NewReader([]byte(contentItem)))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("post to speaker: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return "", fmt.Errorf("speaker returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return fmt.Sprintf("Pushed ding URL to %s. You should hear it within a second.", displayName(dev.Name, target.Device)), nil
}

func buildDingContentItem(mediaURL string) string {
	escaped := xmlAttrEscape(mediaURL)

	return fmt.Sprintf(
		`<ContentItem source="INTERNET_RADIO" type="stationurl" location="%s" sourceAccount="" isPresetable="false"><itemName>AfterTouch ding</itemName></ContentItem>`,
		escaped,
	)
}

func dingCurlCommand(speakerIP, serverURL string) string {
	mediaURL := serverURL + DingMediaPath
	body := buildDingContentItem(mediaURL)

	return fmt.Sprintf(
		"curl -sS -X POST 'http://%s:8090/select' -H 'Content-Type: application/xml' -d '%s'",
		speakerIP, strings.ReplaceAll(body, "'", `'\''`),
	)
}

// xmlAttrEscape replaces the five XML-significant characters in
// an attribute value. URL.QueryEscape would be too aggressive
// (escapes harmless characters and breaks the location URL the
// speaker needs to fetch verbatim).
func xmlAttrEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)

	return r.Replace(s)
}

func displayName(name, deviceID string) string {
	if name != "" {
		return name
	}

	return deviceID
}
