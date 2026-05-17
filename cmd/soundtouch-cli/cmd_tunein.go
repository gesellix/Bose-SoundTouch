// Package main — `soundtouch-cli source tunein` subcommand.
//
// Convenience shortcut for the verbose `source content --source TUNEIN
// --type … --location …` pattern. Picks the right Type + location template
// from the TuneIn guide-ID prefix, optionally fetches name + artwork from
// TuneIn's describe endpoint, then calls the same SelectContentItem path
// the generic `source content` command uses.
//
// Implements #226.
package main

import (
	"fmt"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/bmx"
	"github.com/urfave/cli/v2"
)

// tuneInKind captures the three guide-ID shapes the SoundTouch firmware
// distinguishes; each picks a different Bose `/v1/playback/...` location
// template and a different ContentItem Type.
type tuneInKind struct {
	flag      string // CLI flag name (`station`, `episode`, `program`)
	prefix    string // single-letter guide-ID prefix (`s`, `e`, `p`)
	location  string // printf template, %s = guide ID
	itemType  string // ContentItem.Type the speaker expects
	humanName string // user-facing kind label for log lines
}

var tuneInKinds = []tuneInKind{
	{flag: "station", prefix: "s", location: "/v1/playback/station/%s", itemType: "stationurl", humanName: "live station"},
	{flag: "episode", prefix: "e", location: "/v1/playback/episode/%s", itemType: "stationurl", humanName: "podcast episode"},
	{flag: "program", prefix: "p", location: "/v1/playback/episodes/%s", itemType: "tracklisturl", humanName: "podcast program"},
}

// resolveTuneInKind picks a kind from the CLI flags. Exactly one of
// --station / --episode / --program must be set, OR --id with a prefix we
// recognise. Returns the kind plus the bare guide ID.
func resolveTuneInKind(c *cli.Context) (*tuneInKind, string, error) {
	// Explicit kind flags take precedence over --id.
	var picked *tuneInKind

	var id string

	for i, k := range tuneInKinds {
		v := c.String(k.flag)
		if v == "" {
			continue
		}

		if picked != nil {
			return nil, "", fmt.Errorf("only one of --station, --episode, --program may be set")
		}

		picked = &tuneInKinds[i]
		id = v
	}

	if picked != nil {
		return picked, strings.TrimSpace(id), nil
	}

	// Fall back to --id with prefix auto-detect.
	raw := strings.TrimSpace(c.String("id"))
	if raw == "" {
		return nil, "", fmt.Errorf("one of --station, --episode, --program, or --id is required")
	}

	if raw == "" {
		return nil, "", fmt.Errorf("--id is empty")
	}

	for i, k := range tuneInKinds {
		if strings.HasPrefix(raw, k.prefix) {
			return &tuneInKinds[i], raw, nil
		}
	}

	return nil, "", fmt.Errorf("--id %q has no recognised TuneIn prefix; use --station/--episode/--program explicitly", raw)
}

// playTuneIn is the action wired into `soundtouch-cli source tunein`.
func playTuneIn(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	kind, id, err := resolveTuneInKind(c)
	if err != nil {
		return err
	}

	name := c.String("name")
	artwork := c.String("artwork")

	// Optional metadata enrichment — only fetch if the user hasn't already
	// supplied both, and they haven't asked us to skip it.
	if !c.Bool("no-lookup") && (name == "" || artwork == "") {
		fetchedName, fetchedLogo, lookupErr := bmx.TuneInDescribeMeta(id)
		if lookupErr != nil {
			// Non-fatal: the speaker can resolve the title itself; just
			// note the failure so an operator sees what went wrong.
			fmt.Printf("  Note: TuneIn describe lookup failed (%v); proceeding without enrichment.\n", lookupErr)
		} else {
			if name == "" {
				name = fetchedName
			}

			if artwork == "" {
				artwork = fetchedLogo
			}
		}
	}

	if name == "" {
		// Fall back to a sensible non-empty default so the speaker's
		// now-playing UI doesn't show a blank source label.
		name = "TuneIn"
	}

	contentItem := &models.ContentItem{
		Source:       "TUNEIN",
		Type:         kind.itemType,
		Location:     fmt.Sprintf(kind.location, id),
		ItemName:     name,
		ContainerArt: artwork,
		IsPresetable: true,
	}

	PrintDeviceHeader("Playing TuneIn "+kind.humanName, clientConfig.Host, clientConfig.Port)
	fmt.Printf("  ID:       %s\n", id)
	fmt.Printf("  Location: %s\n", contentItem.Location)
	fmt.Printf("  Type:     %s\n", contentItem.Type)
	fmt.Printf("  Name:     %s\n", contentItem.ItemName)

	if contentItem.ContainerArt != "" {
		fmt.Printf("  Artwork:  %s\n", contentItem.ContainerArt)
	}

	if err := client.SelectContentItem(contentItem); err != nil {
		return fmt.Errorf("failed to select TuneIn content: %w", err)
	}

	PrintSuccess("TuneIn content selected")

	return nil
}
