// Package main — `soundtouch-cli library` command group.
//
// Three subcommands:
//
//   - library servers: discover DLNA media servers on the LAN, either via an
//     app-side SSDP sweep (default) or via the speaker's own list (--via-speaker).
//   - library browse: walk a DLNA ContentDirectory tree by UDN.
//   - library play: send a track URL to a speaker using one of the four
//     existing playback paths so we can A/B which mode works for DLNA streams.
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
				Usage:  "Play a DLNA track URL on a speaker",
				Action: libraryPlay,
				Before: RequireHost,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "art",
						Usage: "Album art URL (optional)",
					},
					&cli.StringFlag{
						Name:  "mode",
						Usage: "Playback mode: local-internet-radio, local-music, stored-music, content-item",
						Value: "local-internet-radio",
					},
					&cli.StringFlag{
						Name:  "name",
						Usage: "Track / item name shown on the speaker display",
						Value: "DLNA Track",
					},
					&cli.StringFlag{
						Name:     "url",
						Usage:    "Stream URL to play (required)",
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

// libraryPlay implements `library play`.
func libraryPlay(c *cli.Context) error {
	streamURL := c.String("url")
	name := c.String("name")
	art := c.String("art")
	mode := c.String("mode")

	clientConfig := GetClientConfig(c)

	speakerClient, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create speaker client: %v", err))

		return err
	}

	PrintDeviceHeader("DLNA play", clientConfig.Host, clientConfig.Port)
	fmt.Printf("  Mode: %s\n", mode)
	fmt.Printf("  URL:  %s\n", streamURL)
	fmt.Printf("  Name: %s\n", name)

	if art != "" {
		fmt.Printf("  Art:  %s\n", art)
	}

	fmt.Println()

	switch mode {
	case "local-internet-radio":
		err = speakerClient.SelectLocalInternetRadio(streamURL, "", name, art)
	case "local-music":
		err = speakerClient.SelectLocalMusic(streamURL, "", name, art)
	case "stored-music":
		err = speakerClient.SelectStoredMusic(streamURL, "", name, art)
	case "content-item":
		item := &models.ContentItem{
			Source:       "LOCAL_MUSIC",
			Type:         "track",
			Location:     streamURL,
			ItemName:     name,
			ContainerArt: art,
			IsPresetable: true,
		}

		err = speakerClient.SelectContentItem(item)
	default:
		return fmt.Errorf("unknown --mode %q; valid values: local-internet-radio, local-music, stored-music, content-item", mode)
	}

	if err != nil {
		PrintError(fmt.Sprintf("Playback command failed: %v", err))

		return err
	}

	PrintSuccess(fmt.Sprintf("Playback started via mode=%s", mode))

	return nil
}
