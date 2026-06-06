package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/urfave/cli/v2"
)

// ttsCloudCmd is the `speaker tts-cloud` subcommand. Unlike `speaker tts`
// (which sends a Google Translate URL straight to the speaker), this routes
// through the AfterTouch service, which synthesizes the audio with the
// configured provider (e.g. Google Cloud TTS), hosts it, and plays it on the
// speaker. It therefore needs --service-url. Target the speaker with the global
// --host, or with --device (resolved to an IP by the service).
func ttsCloudCmd() *cli.Command {
	return &cli.Command{
		Name:  "tts-cloud",
		Usage: "Speak text via the AfterTouch service (Google Cloud TTS), synthesized server-side",
		Description: "Routes through the AfterTouch service (requires --service-url), which\n" +
			"synthesizes the audio with the configured provider, hosts it, and plays it\n" +
			"on the speaker. Target the speaker with the global --host or with --device.\n\n" +
			"Contrast with 'speaker tts', which sends a Google Translate URL directly to\n" +
			"the speaker without involving the service.",
		Flags: append(CloudCommonFlags,
			&cli.StringFlag{
				Name:     "text",
				Aliases:  []string{"t"},
				Usage:    "Text to speak",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "device",
				Aliases: []string{"d"},
				Usage:   "Target device ID (the service resolves it to an IP); alternative to --host",
			},
			&cli.StringFlag{
				Name:    "language",
				Aliases: []string{"l"},
				Usage:   "Language code (provider-specific; defaults to the service setting)",
			},
			&cli.StringFlag{
				Name:  "voice",
				Usage: "Voice name (Google Cloud TTS; ignored by the translate provider)",
			},
			&cli.IntFlag{
				Name:    "volume",
				Aliases: []string{"v"},
				Usage:   "Playback volume (0-100, 0 = service default; only honoured by --method speaker)",
			},
			&cli.StringFlag{
				Name:  "method",
				Usage: "Playback method: 'speaker' (/speaker notification, ducks+resumes, supports volume) or 'radio' (LOCAL_INTERNET_RADIO, no app_key, replaces source)",
				Value: "speaker",
			},
		),
		Action: ttsCloud,
	}
}

func ttsCloud(c *cli.Context) error {
	serviceURL := strings.TrimRight(c.String("service-url"), "/")
	device := c.String("device")
	host := c.String("host") // global flag

	if device == "" && host == "" {
		return fmt.Errorf("one of --host or --device is required")
	}

	payload := map[string]interface{}{"text": c.String("text")}
	if device != "" {
		payload["deviceId"] = device
	}

	if host != "" {
		payload["host"] = host
	}

	if l := c.String("language"); l != "" {
		payload["language"] = l
	}

	if v := c.String("voice"); v != "" {
		payload["voice"] = v
	}

	if c.IsSet("volume") {
		payload["volume"] = c.Int("volume")
	}

	if m := c.String("method"); m != "" {
		payload["method"] = m
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, serviceURL+"/api/setup/tts/speak", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	PrintSuccess(fmt.Sprintf("Spoke %q", c.String("text")))

	return nil
}
