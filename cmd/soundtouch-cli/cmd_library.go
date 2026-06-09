// Package main — `soundtouch-cli library` command group.
//
// Three subcommands:
//
//   - library servers: discover DLNA media servers on the LAN, either via an
//     app-side SSDP sweep (default) or via the speaker's own list (--via-speaker).
//   - library browse: walk a DLNA ContentDirectory tree by UDN.
//   - library play: play a DLNA track on a speaker via native STORED_MUSIC playback.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/dlna"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/urfave/cli/v2"
)

// libraryCommand returns the top-level `library` command group.
func libraryCommand() *cli.Command {
	return &cli.Command{
		Name:  "library",
		Usage: "DLNA music library commands (server discovery, browse, play)",
		Subcommands: []*cli.Command{
			{
				Name:   "browse",
				Usage:  "Browse a DLNA ContentDirectory tree",
				Action: libraryBrowse,
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "count",
						Usage: "Page size (number of entries to request)",
						Value: 50,
					},
					&cli.StringFlag{
						Name:  "object",
						Usage: `ContentDirectory object ID to browse ("0" = root)`,
						Value: "0",
					},
					&cli.IntFlag{
						Name:  "start",
						Usage: "Page offset (starting index)",
						Value: 0,
					},
					&cli.DurationFlag{
						Name:  "timeout",
						Usage: "SSDP discovery + SOAP timeout",
						Value: 5 * time.Second,
					},
					&cli.StringFlag{
						Name:     "udn",
						Usage:    "UDN (uuid:...) of the DLNA media server to browse",
						Required: true,
					},
				},
			},
			{
				Name:   "play",
				Usage:  "Play a DLNA track on a speaker via native STORED_MUSIC playback",
				Action: libraryPlay,
				Before: RequireHost,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "art",
						Usage: "Container art URL (optional)",
					},
					&cli.StringFlag{
						Name:  "name",
						Usage: "Display name shown on the speaker (optional)",
					},
					&cli.StringFlag{
						Name:     "source-account",
						Usage:    "STORED_MUSIC source account (media-server UDN with /0 suffix, e.g. fa095ecc-e13e-40e7-8e6c-e0286d5bc000/0)",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "type",
						Usage: `ContentItem type: "track" or "dir"`,
						Value: "track",
					},
					&cli.StringFlag{
						Name:     "location",
						Usage:    "Object ID from a browse result (e.g. 5:audio5:part13:3171:5 TRACK)",
						Required: true,
					},
				},
			},
			{
				Name:   "servers",
				Usage:  "List DLNA media servers visible on the LAN",
				Action: libraryServers,
				Flags: []cli.Flag{
					&cli.DurationFlag{
						Name:  "timeout",
						Usage: "SSDP sweep timeout",
						Value: 5 * time.Second,
					},
					&cli.BoolFlag{
						Name:  "via-speaker",
						Usage: "Ask the speaker (--host required) instead of doing an app-side SSDP sweep",
					},
				},
			},
		},
	}
}

// libraryServers implements `library servers`.
func libraryServers(c *cli.Context) error {
	if c.Bool("via-speaker") {
		return libraryServersViaSpeaker(c)
	}

	return libraryServersAppSide(c)
}

// libraryServersAppSide runs an SSDP sweep from the CLI process itself.
func libraryServersAppSide(c *cli.Context) error {
	timeout := c.Duration("timeout")

	ctx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
	defer cancel()

	servers, err := discovery.DiscoverMediaServers(ctx, timeout)
	if err != nil {
		PrintError(fmt.Sprintf("SSDP discovery failed: %v", err))

		return err
	}

	if len(servers) == 0 {
		fmt.Println("No DLNA media servers found on the LAN.")

		return nil
	}

	fmt.Printf("Found %d DLNA media server(s):\n\n", len(servers))

	for _, srv := range servers {
		printAppSideServer(srv)
	}

	return nil
}

// libraryServersViaSpeaker asks the speaker for its own DLNA server list.
func libraryServersViaSpeaker(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	speakerClient, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create speaker client: %v", err))

		return err
	}

	resp, err := speakerClient.ListMediaServers()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to list media servers via speaker: %v", err))

		return err
	}

	if len(resp.MediaServers) == 0 {
		fmt.Println("Speaker reports no DLNA media servers.")

		return nil
	}

	fmt.Printf("Speaker reports %d DLNA media server(s):\n\n", len(resp.MediaServers))

	for i := range resp.MediaServers {
		printSpeakerServer(resp.MediaServers[i])
	}

	return nil
}

// printAppSideServer prints a single server discovered by the app-side sweep.
func printAppSideServer(srv discovery.MediaServer) {
	fmt.Printf("  Name:   %s\n", srv.FriendlyName)
	fmt.Printf("  Vendor: %s / %s\n", srv.Manufacturer, srv.ModelName)
	fmt.Printf("  UDN:    %s\n", srv.UDN)
	fmt.Printf("  CDS:    %s\n", srv.CDSControlURL)

	if srv.IconURL != "" {
		fmt.Printf("  Icon:   %s\n", srv.IconURL)
	}

	fmt.Println()
}

// printSpeakerServer prints a single server as reported by the speaker.
func printSpeakerServer(srv models.MediaServerInfo) {
	name := srv.FriendlyName
	if name == "" {
		name = "(unnamed)"
	}

	vendor := srv.Manufacturer

	if srv.ModelName != "" {
		if vendor != "" {
			vendor += " / " + srv.ModelName
		} else {
			vendor = srv.ModelName
		}
	}

	fmt.Printf("  Name:     %s\n", name)

	if vendor != "" {
		fmt.Printf("  Vendor:   %s\n", vendor)
	}

	fmt.Printf("  UDN:      %s\n", srv.ID)

	if srv.IP != "" {
		fmt.Printf("  IP:       %s\n", srv.IP)
	}

	if srv.Location != "" {
		fmt.Printf("  Location: %s\n", srv.Location)
	}

	fmt.Println()
}

// libraryBrowse implements `library browse`.
func libraryBrowse(c *cli.Context) error {
	udn := strings.TrimSpace(c.String("udn"))
	objectID := c.String("object")
	start := c.Int("start")
	count := c.Int("count")
	timeout := c.Duration("timeout")

	ctx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
	defer cancel()

	servers, err := discovery.DiscoverMediaServers(ctx, timeout)
	if err != nil {
		PrintError(fmt.Sprintf("SSDP discovery failed: %v", err))

		return err
	}

	var target *discovery.MediaServer

	for i := range servers {
		if servers[i].UDN == udn {
			target = &servers[i]

			break
		}
	}

	if target == nil {
		var udns []string

		for _, srv := range servers {
			udns = append(udns, fmt.Sprintf("  %s  (%s)", srv.UDN, srv.FriendlyName))
		}

		if len(udns) == 0 {
			PrintError(fmt.Sprintf("No server with UDN %q found; no servers discovered.", udn))
		} else {
			PrintError(fmt.Sprintf(
				"No server with UDN %q found.\nKnown servers:\n%s",
				udn, strings.Join(udns, "\n"),
			))
		}

		return fmt.Errorf("server %q not found", udn)
	}

	fmt.Printf("Browsing %q (object %q, offset %d, page %d)\n\n", target.FriendlyName, objectID, start, count)

	browseCtx, browseCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer browseCancel()

	result, err := dlna.Browse(browseCtx, *target, objectID, start, count)
	if err != nil {
		PrintError(fmt.Sprintf("Browse failed: %v", err))

		return err
	}

	fmt.Printf("TotalMatches: %d  Returned: %d\n\n", result.TotalMatches, result.Returned)

	for _, con := range result.Containers {
		fmt.Printf("  [dir]  %s  (id=%s, children=%d)\n", con.Title, con.ID, con.ChildCount)
	}

	for i := range result.Items {
		it := &result.Items[i]

		audio := ""
		if it.IsAudioItem() {
			audio = " [audio]"
		}

		meta := ""

		if it.Artist != "" || it.Album != "" {
			parts := []string{}
			if it.Artist != "" {
				parts = append(parts, it.Artist)
			}

			if it.Album != "" {
				parts = append(parts, it.Album)
			}

			meta = "  — " + strings.Join(parts, " / ")
		}

		dur := ""

		if it.DurationSec > 0 {
			m := it.DurationSec / 60
			s := it.DurationSec % 60
			dur = fmt.Sprintf("  [%d:%02d]", m, s)
		}

		fmt.Printf("  [item]%s  %s%s%s\n", audio, it.Title, meta, dur)

		if it.StreamURL != "" {
			fmt.Printf("         url: %s\n", it.StreamURL)
		}
	}

	return nil
}

// libraryPlay implements `library play` using native STORED_MUSIC playback.
func libraryPlay(c *cli.Context) error {
	sourceAccount := strings.TrimSpace(c.String("source-account"))
	location := strings.TrimSpace(c.String("location"))
	name := c.String("name")
	itemType := c.String("type")
	art := c.String("art")

	if sourceAccount == "" {
		PrintError("--source-account is required")

		return fmt.Errorf("--source-account is required")
	}

	if location == "" {
		PrintError("--location is required")

		return fmt.Errorf("--location is required")
	}

	clientConfig := GetClientConfig(c)

	speakerClient, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create speaker client: %v", err))

		return err
	}

	// Check that the STORED_MUSIC source for this account is READY before
	// attempting playback. Re-registering an already-READY account can flip
	// it to UNAVAILABLE, so we intentionally do NOT auto-register here.
	sources, err := speakerClient.GetSources()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to retrieve sources: %v", err))

		return err
	}

	ready := false

	for _, si := range sources.SourceItem {
		if si.Source == "STORED_MUSIC" && si.SourceAccount == sourceAccount {
			if si.Status.IsReady() {
				ready = true
			}

			break
		}
	}

	if !ready {
		host := clientConfig.Host
		PrintError(fmt.Sprintf(
			"STORED_MUSIC source account %q is not READY on the speaker.\n"+
				"Register it first:\n"+
				"  soundtouch-cli --host %s account add-nas --user %s --name <server-display-name>",
			sourceAccount, host, sourceAccount,
		))

		return fmt.Errorf("STORED_MUSIC source account %q not ready", sourceAccount)
	}

	PrintDeviceHeader("STORED_MUSIC play", clientConfig.Host, clientConfig.Port)
	fmt.Printf("  Source account: %s\n", sourceAccount)
	fmt.Printf("  Location:       %s\n", location)
	fmt.Printf("  Type:           %s\n", itemType)

	if name != "" {
		fmt.Printf("  Name:           %s\n", name)
	}

	if art != "" {
		fmt.Printf("  Art:            %s\n", art)
	}

	fmt.Println()

	// SelectStoredMusic does not set Type, so we build the ContentItem directly
	// so we can pass the correct type ("track" or "dir") to the speaker.
	ci := &models.ContentItem{
		Source:        "STORED_MUSIC",
		SourceAccount: sourceAccount,
		Location:      location,
		Type:          itemType,
		ItemName:      name,
		ContainerArt:  art,
		IsPresetable:  true,
	}

	if err = speakerClient.SelectContentItem(ci); err != nil {
		PrintError(fmt.Sprintf("Playback command failed: %v", err))

		return err
	}

	label := name
	if label == "" {
		label = location
	}

	PrintSuccess(fmt.Sprintf("Playing %q (STORED_MUSIC, location=%s)", label, location))

	return nil
}
