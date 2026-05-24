package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/urfave/cli/v2"
)

// cloudCommand assembles the `soundtouch-cli cloud …` command group.
// All subcommands talk to the AfterTouch service (not the speaker directly)
// and require --service-url.
func cloudCommand() *cli.Command {
	return &cli.Command{
		Name:  "cloud",
		Usage: "Manage AfterTouch service data (sources, accounts, devices)",
		Subcommands: []*cli.Command{
			cloudSourceCmd(),
		},
	}
}

func cloudSourceCmd() *cli.Command {
	return &cli.Command{
		Name:  "source",
		Usage: "Manage sources stored in AfterTouch",
		Subcommands: []*cli.Command{
			cloudSourceRemoveCmd(),
		},
	}
}

func cloudSourceRemoveCmd() *cli.Command {
	return &cli.Command{
		Name:  "remove",
		Usage: "Remove a source from AfterTouch's datastore for a specific device",
		Flags: append(CloudCommonFlags,
			&cli.StringFlag{
				Name:     "account",
				Aliases:  []string{"a"},
				Usage:    "Account ID",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "device",
				Aliases:  []string{"d"},
				Usage:    "Device ID",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "id",
				Usage: "Source ID to remove (e.g. 10002)",
			},
			&cli.StringFlag{
				Name:    "type",
				Aliases: []string{"t"},
				Usage:   "Source type to remove (e.g. INTERNET_RADIO). Resolved to a canonical ID; fails if multiple sources share the type.",
			},
		),
		Action: cloudSourceRemove,
	}
}

// canonicalSourceID maps well-known SourceKeyType values to their canonical IDs.
// Used to resolve --type to an ID without requiring a round-trip GET.
// TODO We need to ensure that ids here are consistent with the ones used in the AfterTouch service.
var canonicalSourceID = map[string]string{
	"AUX":                  "10001",
	"INTERNET_RADIO":       "10002",
	"LOCAL_INTERNET_RADIO": "10003",
	"TUNEIN":               "10004",
	"RADIO_BROWSER":        "10005",
}

func cloudSourceRemove(c *cli.Context) error {
	serviceURL := strings.TrimRight(c.String("service-url"), "/")
	account := c.String("account")
	device := c.String("device")
	sourceID := c.String("id")
	sourceType := strings.ToUpper(c.String("type"))

	if sourceID == "" && sourceType == "" {
		return fmt.Errorf("one of --id or --type is required")
	}

	if sourceID != "" && sourceType != "" {
		return fmt.Errorf("only one of --id or --type may be given")
	}

	if sourceType != "" {
		id, ok := canonicalSourceID[sourceType]
		if !ok {
			return fmt.Errorf("unknown source type %q; use --id for non-canonical sources", sourceType)
		}

		sourceID = id
	}

	url := fmt.Sprintf("%s/setup/sources/%s/%s/%s", serviceURL, account, device, sourceID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNoContent {
		PrintSuccess(fmt.Sprintf("Removed source %s from device %s (account %s)", sourceID, device, account))

		if sourceType != "" {
			fmt.Printf("  Type: %s\n", sourceType)
		}

		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))

	return fmt.Errorf("service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
