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

// ttsCommand assembles the `soundtouch-cli tts …` command group. Unlike
// `speaker tts` (which talks to a speaker directly using the Google Translate
// URL), these subcommands call the AfterTouch service, which synthesizes audio
// with the configured provider (e.g. Google Cloud TTS) and plays it on a
// speaker. They require --service-url.
func ttsCommand() *cli.Command {
	return &cli.Command{
		Name:  "tts",
		Usage: "Text-to-speech via the AfterTouch service (Google Cloud TTS or Google Translate)",
		Description: "Sends text to the AfterTouch service, which synthesizes audio (or builds a\n" +
			"direct URL) and plays it on a speaker via the /speaker endpoint.\n\n" +
			"This differs from 'speaker tts', which talks to a speaker directly using\n" +
			"the Google Translate URL. Use 'tts speak' for the service's configured\n" +
			"provider (e.g. Google Cloud TTS).",
		Subcommands: []*cli.Command{
			ttsSpeakCmd(),
		},
	}
}

func ttsSpeakCmd() *cli.Command {
	return &cli.Command{
		Name:  "speak",
		Usage: "Synthesize text and play it on a speaker via the AfterTouch service",
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
				Usage:   "Target device ID (the service resolves it to an IP)",
			},
			&cli.StringFlag{
				Name:  "speaker-host",
				Usage: "Target speaker IP/hostname (alternative to --device)",
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
				Usage: "Playback method: 'radio' (LOCAL_INTERNET_RADIO, no app_key, replaces source) or 'speaker' (/speaker notification, ducks+resumes, supports volume)",
				Value: "radio",
			},
		),
		Action: ttsSpeak,
	}
}

func ttsSpeak(c *cli.Context) error {
	serviceURL := strings.TrimRight(c.String("service-url"), "/")
	device := c.String("device")
	speakerHost := c.String("speaker-host")

	if device == "" && speakerHost == "" {
		return fmt.Errorf("one of --device or --speaker-host is required")
	}

	payload := map[string]interface{}{"text": c.String("text")}
	if device != "" {
		payload["deviceId"] = device
	}

	if speakerHost != "" {
		payload["host"] = speakerHost
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

	req, err := http.NewRequest(http.MethodPost, serviceURL+"/setup/tts/speak", bytes.NewReader(body))
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
