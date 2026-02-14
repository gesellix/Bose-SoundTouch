package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"sort"
	"time"

	"github.com/urfave/cli/v2"
)

// Package-level variables for build information
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// sortCommands recursively sorts commands and their subcommands alphabetically
func sortCommands(commands []*cli.Command) {
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})

	// Recursively sort subcommands and flags
	for _, cmd := range commands {
		// Sort flags for this command
		if len(cmd.Flags) > 0 {
			sortFlags(cmd.Flags)
		}

		// Recursively sort subcommands
		if len(cmd.Subcommands) > 0 {
			sortCommands(cmd.Subcommands)
		}
	}
}

// sortFlags sorts a slice of flags alphabetically by name
func sortFlags(flags []cli.Flag) {
	sort.Slice(flags, func(i, j int) bool {
		// Get the flag names for comparison
		name1 := getFlagName(flags[i])
		name2 := getFlagName(flags[j])

		return name1 < name2
	})
}

// getFlagName extracts the primary name from a flag
func getFlagName(flag cli.Flag) string {
	switch f := flag.(type) {
	case *cli.StringFlag:
		return f.Name
	case *cli.IntFlag:
		return f.Name
	case *cli.BoolFlag:
		return f.Name
	case *cli.DurationFlag:
		return f.Name
	case *cli.StringSliceFlag:
		return f.Name
	default:
		// Fallback: try to get name using reflection or string representation
		flagStr := fmt.Sprintf("%v", flag)
		// This is a simple fallback - in practice, all flags should match the types above
		return flagStr
	}
}

// updateBuildInfo extracts version information from debug.BuildInfo and updates package variables
func updateBuildInfo() {
	if info, ok := debug.ReadBuildInfo(); ok {
		// Get version from module info
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}

		// Extract build settings
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				commit = setting.Value
			case "vcs.time":
				if t, err := time.Parse(time.RFC3339, setting.Value); err == nil {
					date = t.Format("2006-01-02_15:04:05")
				}
			}
		}
	}
}

func main() {
	updateBuildInfo()

	app := &cli.App{
		Name:  "soundtouch-cli",
		Usage: "Command-line interface for controlling Bose SoundTouch devices",
		Description: `⠎⠕⠥⠝⠙⠤⠞⠕⠥⠉⠓ A comprehensive CLI tool for interacting with Bose SoundTouch devices.
   Supports device discovery, playback control, volume/bass/balance adjustment,
   source selection, zone management, and more.`,
		Version: version,
		Authors: []*cli.Author{
			{
				Name: "Tobias Gesellchen, and the Bose-SoundTouch Contributors",
			},
		},
		Flags: CommonFlags,
		Commands: []*cli.Command{
			// Version commands
			{
				Name:    "version",
				Aliases: []string{"v"},
				Usage:   "Show detailed version information",
				Action:  showVersionInfo,
			},
			// Discovery commands
			{
				Name:    "discover",
				Aliases: []string{"d"},
				Usage:   "Discover SoundTouch devices on the network",
				Subcommands: []*cli.Command{
					{
						Name:   "devices",
						Usage:  "Discover and list SoundTouch devices",
						Action: discoverDevices,
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "all",
								Aliases: []string{"a"},
								Usage:   "Show detailed information for all devices",
							},
						},
					},
				},
			},
			// Device information commands
			{
				Name:    "info",
				Aliases: []string{"i"},
				Usage:   "Get device information",
				Action:  getDeviceInfo,
				Before:  RequireHost,
			},
			{
				Name:   "name",
				Usage:  "Get or set device name",
				Before: RequireHost,
				Subcommands: []*cli.Command{
					{
						Name:   "get",
						Usage:  "Get device name",
						Action: getDeviceName,
						Before: RequireHost,
					},
					{
						Name:   "set",
						Usage:  "Set device name",
						Action: setDeviceName,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "value",
								Aliases:  []string{"n"},
								Usage:    "New device name",
								Required: true,
							},
						},
						Before: RequireHost,
					},
				},
			},
			{
				Name:   "capabilities",
				Usage:  "Get device capabilities",
				Action: getCapabilities,
				Before: RequireHost,
			},
			{
				Name:    "supported-urls",
				Aliases: []string{"urls"},
				Usage:   "Get supported device endpoints",
				Action:  getSupportedURLs,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show complete endpoint list",
					},
					&cli.BoolFlag{
						Name:    "features",
						Aliases: []string{"f"},
						Usage:   "Show detailed feature mapping and CLI commands",
					},
				},
				Before: RequireHost,
			},
			{
				Name:    "analyze",
				Aliases: []string{"analysis"},
				Usage:   "Analyze device capabilities and provide recommendations",
				Action:  getDeviceAnalysis,
				Before:  RequireHost,
			},
			{
				Name:   "presets",
				Usage:  "Get configured presets",
				Action: getPresets,
				Before: RequireHost,
			},
			// Recent content commands
			{
				Name:    "recents",
				Aliases: []string{"recent"},
				Usage:   "Recently played content commands",
				Subcommands: []*cli.Command{
					{
						Name:   "list",
						Usage:  "List recently played content",
						Action: getRecents,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:  "limit",
								Usage: "Maximum number of items to display (0 for all)",
								Value: 10,
							},
							&cli.BoolFlag{
								Name:    "detailed",
								Aliases: []string{"d"},
								Usage:   "Show detailed information for each item",
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "filter",
						Usage:  "List recently played content with filters",
						Action: getRecentsFiltered,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "source",
								Aliases: []string{"s"},
								Usage:   "Filter by source (SPOTIFY, LOCAL_MUSIC, TUNEIN, etc.)",
							},
							&cli.StringFlag{
								Name:    "type",
								Aliases: []string{"t"},
								Usage:   "Filter by content type (track, station, playlist, album, presetable)",
							},
							&cli.IntFlag{
								Name:  "limit",
								Usage: "Maximum number of items to display (0 for all)",
								Value: 10,
							},
							&cli.BoolFlag{
								Name:    "detailed",
								Aliases: []string{"d"},
								Usage:   "Show detailed information for each item",
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "latest",
						Usage:  "Show only the most recent item",
						Action: getRecentsMostRecent,
						Before: RequireHost,
					},
					{
						Name:   "stats",
						Usage:  "Show statistics about recent content",
						Action: recentsStats,
						Before: RequireHost,
					},
				},
			},
			// Playback commands
			{
				Name:    "play",
				Aliases: []string{"p"},
				Usage:   "Playback control commands",
				Subcommands: []*cli.Command{
					{
						Name:   "now",
						Usage:  "Get current playback status",
						Action: getNowPlaying,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "verbose",
								Aliases: []string{"v"},
								Usage:   "Show detailed content information (including Spotify URIs)",
							},
						},
					},
					{
						Name:   "start",
						Usage:  "Start playback",
						Action: playCommand,
						Before: RequireHost,
					},
					{
						Name:   "pause",
						Usage:  "Pause playback",
						Action: pauseCommand,
						Before: RequireHost,
					},
					{
						Name:   "stop",
						Usage:  "Stop playback",
						Action: stopCommand,
						Before: RequireHost,
					},
					{
						Name:   "next",
						Usage:  "Next track",
						Action: nextCommand,
						Before: RequireHost,
					},
					{
						Name:   "prev",
						Usage:  "Previous track",
						Action: prevCommand,
						Before: RequireHost,
					},
				},
			},
			// Preset commands
			{
				Name:  "preset",
				Usage: "Preset management commands",
				Subcommands: []*cli.Command{
					{
						Name:   "store-current",
						Usage:  "Store currently playing content as preset",
						Action: storeCurrentPreset,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:     "slot",
								Usage:    "Preset slot number (1-6)",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "store",
						Usage:  "Store specific content as preset",
						Action: storePreset,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:     "slot",
								Usage:    "Preset slot number (1-6)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "source",
								Usage:    "Content source (SPOTIFY, TUNEIN, LOCAL_INTERNET_RADIO, etc.)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "location",
								Usage:    "Content location (URI, URL, or ID)",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "source-account",
								Usage: "Source account (username, device ID, etc.)",
							},
							&cli.StringFlag{
								Name:  "name",
								Usage: "Display name for the preset",
							},
							&cli.StringFlag{
								Name:  "type",
								Usage: "Content type (uri, stationurl, etc.)",
							},
							&cli.StringFlag{
								Name:  "artwork",
								Usage: "Artwork URL",
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "remove",
						Usage:  "Remove a preset",
						Action: removePreset,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:     "slot",
								Usage:    "Preset slot number (1-6)",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "select",
						Usage:  "Select and play a preset",
						Action: selectPresetNew,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:     "slot",
								Usage:    "Preset slot number (1-6)",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "list",
						Usage:  "List all presets",
						Action: listPresets,
						Before: RequireHost,
					},
				},
			},
			// Browse/Navigation commands
			{
				Name:    "browse",
				Aliases: []string{"nav"},
				Usage:   "Browse and navigate content sources",
				Subcommands: []*cli.Command{
					{
						Name:   "content",
						Usage:  "Browse content from a source",
						Action: browseContent,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Usage:    "Content source (TUNEIN, PANDORA, SPOTIFY, STORED_MUSIC)",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "source-account",
								Usage: "Source account (username, device ID, etc.)",
							},
							&cli.IntFlag{
								Name:  "start",
								Usage: "Starting item number",
								Value: 1,
							},
							&cli.IntFlag{
								Name:  "limit",
								Usage: "Number of items to return",
								Value: 20,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "menu",
						Usage:  "Browse content with menu navigation",
						Action: browseWithMenu,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Usage:    "Content source (PANDORA, etc.)",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "source-account",
								Usage: "Source account (required for some sources)",
							},
							&cli.StringFlag{
								Name:     "menu",
								Usage:    "Menu type (radioStations, etc.)",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "sort",
								Usage: "Sort order (dateCreated, etc.)",
								Value: "dateCreated",
							},
							&cli.IntFlag{
								Name:  "start",
								Usage: "Starting item number",
								Value: 1,
							},
							&cli.IntFlag{
								Name:  "limit",
								Usage: "Number of items to return",
								Value: 20,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "container",
						Usage:  "Browse into a container/directory",
						Action: browseContainer,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Usage:    "Content source",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "source-account",
								Usage: "Source account",
							},
							&cli.StringFlag{
								Name:     "location",
								Usage:    "Container location",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "type",
								Usage: "Container type",
							},
							&cli.IntFlag{
								Name:  "start",
								Usage: "Starting item number",
								Value: 1,
							},
							&cli.IntFlag{
								Name:  "limit",
								Usage: "Number of items to return",
								Value: 20,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "tunein",
						Usage:  "Browse TuneIn stations",
						Action: browseTuneIn,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "source-account",
								Usage: "TuneIn account (optional)",
							},
							&cli.IntFlag{
								Name:  "start",
								Usage: "Starting item number",
								Value: 1,
							},
							&cli.IntFlag{
								Name:  "limit",
								Usage: "Number of items to return",
								Value: 100,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "pandora",
						Usage:  "Browse Pandora stations",
						Action: browsePandora,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source-account",
								Usage:    "Pandora account (required)",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "stored-music",
						Usage:  "Browse stored music library",
						Action: browseStoredMusic,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source-account",
								Usage:    "Device ID (required)",
								Required: true,
							},
						},
						Before: RequireHost,
					},
				},
			},
			// Station commands
			{
				Name:    "station",
				Aliases: []string{"st"},
				Usage:   "Search and manage stations",
				Subcommands: []*cli.Command{
					{
						Name:   "search",
						Usage:  "Search for stations and content",
						Action: searchStations,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Usage:    "Search source (TUNEIN, PANDORA, SPOTIFY)",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "source-account",
								Usage: "Source account (required for Pandora/Spotify)",
							},
							&cli.StringFlag{
								Name:     "query",
								Aliases:  []string{"q"},
								Usage:    "Search query",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "search-tunein",
						Usage:  "Search TuneIn stations",
						Action: searchTuneIn,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "query",
								Aliases:  []string{"q"},
								Usage:    "Search query",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "search-pandora",
						Usage:  "Search Pandora stations",
						Action: searchPandora,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source-account",
								Usage:    "Pandora account (required)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "query",
								Aliases:  []string{"q"},
								Usage:    "Search query",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "search-spotify",
						Usage:  "Search Spotify content",
						Action: searchSpotify,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source-account",
								Usage:    "Spotify account (required)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "query",
								Aliases:  []string{"q"},
								Usage:    "Search query",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "add",
						Usage:  "Add station and play immediately",
						Action: addStation,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Usage:    "Station source (TUNEIN, PANDORA, SPOTIFY)",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "source-account",
								Usage: "Source account (required for some sources)",
							},
							&cli.StringFlag{
								Name:     "token",
								Usage:    "Station token (from search results)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "name",
								Usage:    "Station name",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "remove",
						Usage:  "Remove station from collection",
						Action: removeStation,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Usage:    "Station source",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "source-account",
								Usage: "Source account",
							},
							&cli.StringFlag{
								Name:     "location",
								Usage:    "Station location",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "type",
								Usage: "Station type",
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "list",
						Usage:  "List saved stations",
						Action: listStations,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Usage:    "Station source (TUNEIN, PANDORA)",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "source-account",
								Usage: "Source account (required for Pandora)",
							},
						},
						Before: RequireHost,
					},
				},
			},
			// Key commands
			{
				Name:    "key",
				Aliases: []string{"k"},
				Usage:   "Send key commands",
				Subcommands: []*cli.Command{
					{
						Name:   "send",
						Usage:  "Send generic key command",
						Action: sendKey,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "key",
								Aliases:  []string{"k"},
								Usage:    "Key name (PLAY, PAUSE, STOP, POWER, MUTE, etc.)",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "power",
						Usage:  "Send POWER key command",
						Action: powerCommand,
						Before: RequireHost,
					},
					{
						Name:   "mute",
						Usage:  "Send MUTE key command",
						Action: muteCommand,
						Before: RequireHost,
					},
					{
						Name:   "thumbs-up",
						Usage:  "Send THUMBS_UP key command",
						Action: thumbsUpCommand,
						Before: RequireHost,
					},
					{
						Name:   "thumbs-down",
						Usage:  "Send THUMBS_DOWN key command",
						Action: thumbsDownCommand,
						Before: RequireHost,
					},
					{
						Name:   "volume-up",
						Usage:  "Send VOLUME_UP key command",
						Action: volumeUpKey,
						Before: RequireHost,
					},
					{
						Name:   "volume-down",
						Usage:  "Send VOLUME_DOWN key command",
						Action: volumeDownKey,
						Before: RequireHost,
					},
				},
			},
			// Track info
			{
				Name:   "track",
				Usage:  "Get track information (WARNING: times out on real devices, use playback 'now' command instead)",
				Action: getTrackInfo,
				Before: RequireHost,
			},
			// Volume commands
			{
				Name:    "volume",
				Aliases: []string{"vol"},
				Usage:   "Volume control commands",
				Subcommands: []*cli.Command{
					{
						Name:   "get",
						Usage:  "Get current volume level",
						Action: getVolume,
						Before: RequireHost,
					},
					{
						Name:   "set",
						Usage:  "Set volume level",
						Action: setVolume,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:     "level",
								Aliases:  []string{"l"},
								Usage:    "Volume level (0-100)",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "up",
						Usage:  "Increase volume",
						Action: volumeUp,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "amount",
								Aliases: []string{"a"},
								Usage:   "Amount to increase (1-10)",
								Value:   2,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "down",
						Usage:  "Decrease volume",
						Action: volumeDown,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "amount",
								Aliases: []string{"a"},
								Usage:   "Amount to decrease (1-10)",
								Value:   2,
							},
						},
						Before: RequireHost,
					},
				},
			},
			// Source commands
			{
				Name:    "source",
				Aliases: []string{"src"},
				Usage:   "Audio source commands",
				Subcommands: []*cli.Command{
					{
						Name:   "list",
						Usage:  "List available audio sources",
						Action: listSources,
						Before: RequireHost,
					},
					{
						Name:   "select",
						Usage:  "Select an audio source",
						Action: selectSource,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Aliases:  []string{"s"},
								Usage:    "Source to select (SPOTIFY, BLUETOOTH, AUX, etc.)",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "account",
								Aliases: []string{"a"},
								Usage:   "Source account for streaming services (optional)",
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "spotify",
						Usage:  "Select Spotify source",
						Action: selectSpotify,
						Before: RequireHost,
					},
					{
						Name:   "bluetooth",
						Usage:  "Select Bluetooth source",
						Action: selectBluetooth,
						Before: RequireHost,
					},
					{
						Name:   "aux",
						Usage:  "Select AUX input source",
						Action: selectAux,
						Before: RequireHost,
					},
					{
						Name:   "internet-radio",
						Usage:  "Select internet radio stream (LOCAL_INTERNET_RADIO)",
						Action: selectLocalInternetRadio,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "location",
								Aliases:  []string{"l"},
								Usage:    "Stream location URL (direct stream or streamUrl format)",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "account",
								Aliases: []string{"a"},
								Usage:   "Source account (optional)",
							},
							&cli.StringFlag{
								Name:    "name",
								Aliases: []string{"n"},
								Usage:   "Station name",
							},
							&cli.StringFlag{
								Name:  "artwork",
								Usage: "Station artwork URL",
							},
						},
					},
					{
						Name:   "local-music",
						Usage:  "Select local music content (LOCAL_MUSIC)",
						Action: selectLocalMusic,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "location",
								Aliases:  []string{"l"},
								Usage:    "Content location (e.g., album:983, track:2579)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "account",
								Aliases:  []string{"a"},
								Usage:    "Source account GUID (required)",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "name",
								Aliases: []string{"n"},
								Usage:   "Content name",
							},
							&cli.StringFlag{
								Name:  "artwork",
								Usage: "Content artwork URL",
							},
						},
					},
					{
						Name:   "stored-music",
						Usage:  "Select stored music content (STORED_MUSIC)",
						Action: selectStoredMusic,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "location",
								Aliases:  []string{"l"},
								Usage:    "Content location ID (e.g., 6_a2874b5d_4f83d999)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "account",
								Aliases:  []string{"a"},
								Usage:    "Source account GUID (required)",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "name",
								Aliases: []string{"n"},
								Usage:   "Content name",
							},
							&cli.StringFlag{
								Name:  "artwork",
								Usage: "Content artwork URL",
							},
						},
					},
					{
						Name:   "content",
						Usage:  "Select content using ContentItem (advanced)",
						Action: selectContent,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Aliases:  []string{"s"},
								Usage:    "Content source (SPOTIFY, TUNEIN, LOCAL_INTERNET_RADIO, etc.)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "location",
								Aliases:  []string{"l"},
								Usage:    "Content location",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "account",
								Aliases: []string{"a"},
								Usage:   "Source account",
							},
							&cli.StringFlag{
								Name:    "name",
								Aliases: []string{"n"},
								Usage:   "Content name",
							},
							&cli.StringFlag{
								Name:    "type",
								Aliases: []string{"t"},
								Usage:   "Content type (uri, stationurl, album, track, etc.)",
							},
							&cli.StringFlag{
								Name:  "artwork",
								Usage: "Content artwork URL",
							},
							&cli.BoolFlag{
								Name:  "presetable",
								Usage: "Mark content as presetable",
								Value: true,
							},
						},
					},
					{
						Name:   "availability",
						Usage:  "Show service availability",
						Action: getServiceAvailability,
						Before: RequireHost,
					},
					{
						Name:   "compare",
						Usage:  "Compare sources and service availability",
						Action: compareSourcesAndAvailability,
						Before: RequireHost,
					},
					{
						Name:   "introspect",
						Usage:  "Get introspect data for a music service",
						Action: introspectService,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Aliases:  []string{"s"},
								Usage:    "Music service source (SPOTIFY, PANDORA, TUNEIN, etc.)",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "account",
								Aliases: []string{"a"},
								Usage:   "Source account name (optional)",
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "introspect-spotify",
						Usage:  "Get Spotify introspect data (convenience command)",
						Action: introspectSpotify,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "account",
								Aliases: []string{"a"},
								Usage:   "Spotify account name (optional)",
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "introspect-all",
						Usage:  "Get introspect data for all available services",
						Action: introspectAllServices,
						Before: RequireHost,
					},
				},
			},
			// Bass commands
			{
				Name:    "bass",
				Aliases: []string{"b"},
				Usage:   "Bass control commands",
				Subcommands: []*cli.Command{
					{
						Name:   "get",
						Usage:  "Get current bass level",
						Action: getBass,
						Before: RequireHost,
					},
					{
						Name:   "set",
						Usage:  "Set bass level",
						Action: setBass,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:     "level",
								Aliases:  []string{"l"},
								Usage:    "Bass level (-9 to 9)",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "up",
						Usage:  "Increase bass",
						Action: bassUp,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "amount",
								Aliases: []string{"a"},
								Usage:   "Amount to increase (1-5)",
								Value:   1,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "down",
						Usage:  "Decrease bass",
						Action: bassDown,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "amount",
								Aliases: []string{"a"},
								Usage:   "Amount to decrease (1-5)",
								Value:   1,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "capabilities",
						Usage:  "Get bass capabilities",
						Action: getBassCapabilities,
						Before: RequireHost,
					},
				},
			},
			// Balance commands
			{
				Name:    "balance",
				Aliases: []string{"bal"},
				Usage:   "Balance control commands",
				Subcommands: []*cli.Command{
					{
						Name:   "get",
						Usage:  "Get current balance level",
						Action: getBalance,
						Before: RequireHost,
					},
					{
						Name:   "set",
						Usage:  "Set balance level",
						Action: setBalance,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:     "level",
								Aliases:  []string{"l"},
								Usage:    "Balance level (-50 to 50, negative=left, positive=right)",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "left",
						Usage:  "Shift balance to the left",
						Action: balanceLeft,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "amount",
								Aliases: []string{"a"},
								Usage:   "Amount to shift left (1-10, default: 5)",
								Value:   5,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "right",
						Usage:  "Shift balance to the right",
						Action: balanceRight,
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "amount",
								Aliases: []string{"a"},
								Usage:   "Amount to shift right (1-10, default: 5)",
								Value:   5,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "center",
						Usage:  "Center the balance",
						Action: balanceCenter,
						Before: RequireHost,
					},
				},
			},
			// Clock commands
			{
				Name:    "clock",
				Aliases: []string{"time"},
				Usage:   "Clock and time commands",
				Subcommands: []*cli.Command{
					{
						Name:   "get",
						Usage:  "Get current time",
						Action: getClockTime,
						Before: RequireHost,
					},
					{
						Name:   "set",
						Usage:  "Set clock time",
						Action: setClockTime,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "time",
								Aliases:  []string{"t"},
								Usage:    "Time in HH:MM format or 'now' for current time",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "now",
						Usage:  "Set clock to current system time",
						Action: setClockTimeNow,
						Before: RequireHost,
					},
					{
						Name:  "display",
						Usage: "Clock display commands",
						Subcommands: []*cli.Command{
							{
								Name:   "get",
								Usage:  "Get display settings",
								Action: getClockDisplay,
								Before: RequireHost,
							},
							{
								Name:   "enable",
								Usage:  "Enable clock display",
								Action: enableClockDisplay,
								Before: RequireHost,
							},
							{
								Name:   "disable",
								Usage:  "Disable clock display",
								Action: disableClockDisplay,
								Before: RequireHost,
							},
							{
								Name:   "brightness",
								Usage:  "Set display brightness",
								Action: setClockDisplayBrightness,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:     "brightness",
										Aliases:  []string{"b"},
										Usage:    "Brightness level (low, medium, high, off)",
										Required: true,
									},
								},
								Before: RequireHost,
							},
							{
								Name:   "format",
								Usage:  "Set display format",
								Action: setClockDisplayFormat,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:     "format",
										Aliases:  []string{"f"},
										Usage:    "Time format (12 or 24)",
										Required: true,
									},
								},
								Before: RequireHost,
							},
						},
					},
				},
			},
			// Network commands
			{
				Name:    "network",
				Aliases: []string{"net"},
				Usage:   "Network information commands",
				Subcommands: []*cli.Command{
					{
						Name:   "info",
						Usage:  "Get network information",
						Action: getNetworkInfo,
						Before: RequireHost,
					},
					{
						Name:   "ping",
						Usage:  "Ping the device",
						Action: pingDevice,
						Before: RequireHost,
					},
					{
						Name:   "url",
						Usage:  "Get device base URL",
						Action: getDeviceURL,
						Before: RequireHost,
					},
				},
			},
			// Zone commands
			{
				Name:    "zone",
				Aliases: []string{"z"},
				Usage:   "Multi-room zone management commands",
				Subcommands: []*cli.Command{
					{
						Name:   "get",
						Usage:  "Get current zone configuration",
						Action: getZone,
						Before: RequireHost,
					},
					{
						Name:   "status",
						Usage:  "Get zone status",
						Action: getZoneStatus,
						Before: RequireHost,
					},
					{
						Name:   "members",
						Usage:  "List zone members",
						Action: getZoneMembers,
						Before: RequireHost,
					},
					{
						Name:   "create",
						Usage:  "Create a new zone",
						Action: createZone,
						Flags: []cli.Flag{
							&cli.StringSliceFlag{
								Name:     "members",
								Aliases:  []string{"m"},
								Usage:    "Member IP addresses",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "add",
						Usage:  "Add device to zone",
						Action: addToZone,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "member",
								Aliases:  []string{"m"},
								Usage:    "Member IP address to add",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "remove",
						Usage:  "Remove device from zone",
						Action: removeFromZone,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "member",
								Aliases:  []string{"m"},
								Usage:    "Member IP address to remove",
								Required: true,
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "dissolve",
						Usage:  "Dissolve the current zone",
						Action: dissolveZone,
						Before: RequireHost,
					},
					{
						Name:   "set",
						Usage:  "Set zone configuration",
						Action: setZoneConfig,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "master",
								Usage:    "Master device IP address",
								Required: true,
							},
							&cli.StringSliceFlag{
								Name:    "members",
								Aliases: []string{"m"},
								Usage:   "Member IP addresses",
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "add-slave",
						Usage:  "Add slave to zone (official API)",
						Action: addZoneSlave,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "master",
								Usage:    "Master device ID",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "slave",
								Usage:    "Slave device ID",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "slave-ip",
								Usage: "Slave device IP address (optional)",
							},
						},
						Before: RequireHost,
					},
					{
						Name:   "remove-slave",
						Usage:  "Remove slave from zone (official API)",
						Action: removeZoneSlave,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "master",
								Usage:    "Master device ID",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "slave",
								Usage:    "Slave device ID",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "slave-ip",
								Usage: "Slave device IP address (optional)",
							},
						},
						Before: RequireHost,
					},
				},
			},
			// Advanced Audio commands
			{
				Name:    "audio",
				Aliases: []string{"a"},
				Usage:   "Advanced audio control commands",
				Subcommands: []*cli.Command{
					// DSP Controls
					{
						Name:    "dsp",
						Aliases: []string{"d"},
						Usage:   "DSP audio control commands",
						Subcommands: []*cli.Command{
							{
								Name:   "get",
								Usage:  "Get current DSP audio controls",
								Action: getAudioDSPControls,
								Before: RequireHost,
							},
							{
								Name:   "set",
								Usage:  "Set DSP audio controls",
								Action: setAudioDSPControls,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  "mode",
										Usage: "Audio mode (NORMAL, DIALOG, SURROUND, MUSIC, MOVIE, etc.)",
									},
									&cli.IntFlag{
										Name:  "delay",
										Usage: "Video sync audio delay in milliseconds",
									},
								},
								Before: RequireHost,
							},
							{
								Name:   "mode",
								Usage:  "Set audio mode",
								Action: setAudioMode,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:     "mode",
										Usage:    "Audio mode (NORMAL, DIALOG, SURROUND, MUSIC, MOVIE, etc.)",
										Required: true,
									},
								},
								Before: RequireHost,
							},
							{
								Name:   "delay",
								Usage:  "Set video sync audio delay",
								Action: setVideoSyncDelay,
								Flags: []cli.Flag{
									&cli.IntFlag{
										Name:     "delay",
										Usage:    "Video sync audio delay in milliseconds",
										Required: true,
									},
								},
								Before: RequireHost,
							},
						},
					},
					// Tone Controls
					{
						Name:    "tone",
						Aliases: []string{"t"},
						Usage:   "Advanced tone control commands",
						Subcommands: []*cli.Command{
							{
								Name:   "get",
								Usage:  "Get current advanced tone controls",
								Action: getAudioToneControls,
								Before: RequireHost,
							},
							{
								Name:   "set",
								Usage:  "Set advanced tone controls",
								Action: setAudioToneControls,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  "bass",
										Usage: "Bass level (range varies by device)",
									},
									&cli.StringFlag{
										Name:  "treble",
										Usage: "Treble level (range varies by device)",
									},
								},
								Before: RequireHost,
							},
							{
								Name:   "bass",
								Usage:  "Set advanced bass level",
								Action: setAdvancedBass,
								Flags: []cli.Flag{
									&cli.IntFlag{
										Name:     "level",
										Usage:    "Bass level (range varies by device)",
										Required: true,
									},
								},
								Before: RequireHost,
							},
							{
								Name:   "treble",
								Usage:  "Set advanced treble level",
								Action: setAdvancedTreble,
								Flags: []cli.Flag{
									&cli.IntFlag{
										Name:     "level",
										Usage:    "Treble level (range varies by device)",
										Required: true,
									},
								},
								Before: RequireHost,
							},
						},
					},
					// Level Controls
					{
						Name:    "level",
						Aliases: []string{"l"},
						Usage:   "Speaker level control commands",
						Subcommands: []*cli.Command{
							{
								Name:   "get",
								Usage:  "Get current speaker level controls",
								Action: getAudioLevelControls,
								Before: RequireHost,
							},
							{
								Name:   "set",
								Usage:  "Set speaker level controls",
								Action: setAudioLevelControls,
								Flags: []cli.Flag{
									&cli.StringFlag{
										Name:  "front-center",
										Usage: "Front-center speaker level (range varies by device)",
									},
									&cli.StringFlag{
										Name:  "rear-surround",
										Usage: "Rear-surround speakers level (range varies by device)",
									},
								},
								Before: RequireHost,
							},
							{
								Name:   "front-center",
								Usage:  "Set front-center speaker level",
								Action: setFrontCenterLevel,
								Flags: []cli.Flag{
									&cli.IntFlag{
										Name:     "level",
										Usage:    "Front-center speaker level (range varies by device)",
										Required: true,
									},
								},
								Before: RequireHost,
							},
							{
								Name:   "rear-surround",
								Usage:  "Set rear-surround speakers level",
								Action: setRearSurroundLevel,
								Flags: []cli.Flag{
									&cli.IntFlag{
										Name:     "level",
										Usage:    "Rear-surround speakers level (range varies by device)",
										Required: true,
									},
								},
								Before: RequireHost,
							},
						},
					},
				},
			},
			// Speaker commands (TTS and URL playback)
			{
				Name:    "speaker",
				Aliases: []string{"sp"},
				Usage:   "Speaker notification and content playback commands",
				Subcommands: []*cli.Command{
					{
						Name:   "tts",
						Usage:  "Play a Text-To-Speech message",
						Action: playTTS,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "text",
								Aliases:  []string{"t"},
								Usage:    "Text message to speak",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "app-key",
								Aliases:  []string{"k"},
								Usage:    "Application key for the request",
								Required: true,
							},
							&cli.IntFlag{
								Name:    "volume",
								Aliases: []string{"v"},
								Usage:   "Volume level (0-100, 0 = current volume)",
								Value:   0,
							},
							&cli.StringFlag{
								Name:    "language",
								Aliases: []string{"l"},
								Usage:   "Language code (EN, DE, ES, FR, etc.)",
								Value:   "EN",
							},
						},
					},
					{
						Name:   "url",
						Usage:  "Play audio content from a URL",
						Action: playURL,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "url",
								Aliases:  []string{"u"},
								Usage:    "URL of the audio content to play",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "app-key",
								Aliases:  []string{"k"},
								Usage:    "Application key for the request",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "service",
								Aliases: []string{"s"},
								Usage:   "Service name (appears in NowPlaying artist field)",
								Value:   "URL Playback",
							},
							&cli.StringFlag{
								Name:    "message",
								Aliases: []string{"m"},
								Usage:   "Message description (appears in NowPlaying album field)",
								Value:   "Audio Content",
							},
							&cli.StringFlag{
								Name:    "reason",
								Aliases: []string{"r"},
								Usage:   "Reason or filename (appears in NowPlaying track field)",
							},
							&cli.IntFlag{
								Name:    "volume",
								Aliases: []string{"v"},
								Usage:   "Volume level (0-100, 0 = current volume)",
								Value:   0,
							},
						},
					},
					{
						Name:   "notify",
						Usage:  "Play a notification sound or local file",
						Action: playNotification,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "path",
								Aliases: []string{"p"},
								Usage:   "Device-local path to a PCM file (e.g. /opt/Bose/chimes/grouped.pcm)",
							},
						},
					},
					{
						Name:   "beep",
						Usage:  "Play a notification beep sound",
						Action: playNotificationBeep,
						Before: RequireHost,
					},
					{
						Name:   "help",
						Usage:  "Show detailed help about speaker functionality",
						Action: showSpeakerHelp,
					},
				},
			},
			// Account management commands
			{
				Name:    "account",
				Aliases: []string{"acc"},
				Usage:   "Music service account management commands",
				Subcommands: []*cli.Command{
					{
						Name:   "list",
						Usage:  "List configured music service accounts",
						Action: listMusicServiceAccounts,
						Before: RequireHost,
					},
					{
						Name:   "add",
						Usage:  "Add a music service account",
						Action: addMusicServiceAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Aliases:  []string{"s"},
								Usage:    "Music service source (SPOTIFY, PANDORA, AMAZON, DEEZER, IHEART, STORED_MUSIC)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Username or account identifier",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "password",
								Aliases: []string{"p"},
								Usage:   "Account password (not required for STORED_MUSIC)",
							},
							&cli.StringFlag{
								Name:    "name",
								Aliases: []string{"n"},
								Usage:   "Display name for the service",
							},
						},
					},
					{
						Name:   "remove",
						Usage:  "Remove a music service account",
						Action: removeMusicServiceAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "source",
								Aliases:  []string{"s"},
								Usage:    "Music service source (SPOTIFY, PANDORA, AMAZON, DEEZER, IHEART, STORED_MUSIC)",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Username or account identifier",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "name",
								Aliases: []string{"n"},
								Usage:   "Display name for the service",
							},
						},
					},
					{
						Name:   "add-spotify",
						Usage:  "Add a Spotify Premium account",
						Action: addSpotifyAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Spotify username/email",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "password",
								Aliases:  []string{"p"},
								Usage:    "Spotify password",
								Required: true,
							},
						},
					},
					{
						Name:   "remove-spotify",
						Usage:  "Remove a Spotify account",
						Action: removeSpotifyAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Spotify username/email to remove",
								Required: true,
							},
						},
					},
					{
						Name:   "add-pandora",
						Usage:  "Add a Pandora account",
						Action: addPandoraAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Pandora username",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "password",
								Aliases:  []string{"p"},
								Usage:    "Pandora password",
								Required: true,
							},
						},
					},
					{
						Name:   "remove-pandora",
						Usage:  "Remove a Pandora account",
						Action: removePandoraAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Pandora username to remove",
								Required: true,
							},
						},
					},
					{
						Name:   "add-nas",
						Usage:  "Add a network music library (NAS/UPnP)",
						Action: addStoredMusicAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "UPnP server GUID with /0 suffix (e.g., d09708a1-5953-44bc-a413-123456789012/0)",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "name",
								Aliases: []string{"n"},
								Usage:   "Display name for the music library",
								Value:   "Network Music Library",
							},
						},
					},
					{
						Name:   "remove-nas",
						Usage:  "Remove a network music library",
						Action: removeStoredMusicAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "UPnP server GUID with /0 suffix to remove",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "name",
								Aliases: []string{"n"},
								Usage:   "Display name for the music library",
								Value:   "Network Music Library",
							},
						},
					},
					{
						Name:   "add-amazon",
						Usage:  "Add an Amazon Music account",
						Action: addAmazonMusicAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Amazon Music username",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "password",
								Aliases:  []string{"p"},
								Usage:    "Amazon Music password",
								Required: true,
							},
						},
					},
					{
						Name:   "remove-amazon",
						Usage:  "Remove an Amazon Music account",
						Action: removeAmazonMusicAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Amazon Music username to remove",
								Required: true,
							},
						},
					},
					{
						Name:   "add-deezer",
						Usage:  "Add a Deezer Premium account",
						Action: addDeezerAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Deezer username",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "password",
								Aliases:  []string{"p"},
								Usage:    "Deezer password",
								Required: true,
							},
						},
					},
					{
						Name:   "remove-deezer",
						Usage:  "Remove a Deezer account",
						Action: removeDeezerAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "Deezer username to remove",
								Required: true,
							},
						},
					},
					{
						Name:   "add-iheart",
						Usage:  "Add an iHeartRadio account",
						Action: addIHeartRadioAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "iHeartRadio username",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "password",
								Aliases:  []string{"p"},
								Usage:    "iHeartRadio password",
								Required: true,
							},
						},
					},
					{
						Name:   "remove-iheart",
						Usage:  "Remove an iHeartRadio account",
						Action: removeIHeartRadioAccount,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "user",
								Aliases:  []string{"u"},
								Usage:    "iHeartRadio username to remove",
								Required: true,
							},
						},
					},
				},
			},
			// Token commands
			{
				Name:    "token",
				Aliases: []string{"t"},
				Usage:   "Bearer token management commands",
				Subcommands: []*cli.Command{
					{
						Name:   "request",
						Usage:  "Request a new bearer token from the device",
						Action: requestToken,
						Before: RequireHost,
					},
				},
			},
			// Events commands
			{
				Name:    "events",
				Aliases: []string{"e"},
				Usage:   "WebSocket event monitoring commands",
				Subcommands: []*cli.Command{
					{
						Name:   "subscribe",
						Usage:  "Subscribe to real-time device events via WebSocket",
						Action: eventSubscribe,
						Before: RequireHost,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "filter",
								Aliases: []string{"f"},
								Usage:   "Filter events by type (comma-separated): nowPlaying,volume,connection,preset,zone,bass,sdkInfo,userActivity",
							},
							&cli.DurationFlag{
								Name:    "duration",
								Aliases: []string{"d"},
								Usage:   "How long to listen for events (0 = infinite)",
								Value:   0,
							},
							&cli.BoolFlag{
								Name:  "no-reconnect",
								Usage: "Disable automatic reconnection on connection loss",
							},
							&cli.BoolFlag{
								Name:    "verbose",
								Aliases: []string{"v"},
								Usage:   "Enable verbose logging and detailed event information",
							},
						},
					},
				},
			},
		},
	}

	// Sort commands alphabetically (including subcommands and flags recursively)
	sortCommands(app.Commands)

	// Also sort global flags
	if len(app.Flags) > 0 {
		sortFlags(app.Flags)
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
