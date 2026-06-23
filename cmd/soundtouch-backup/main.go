// Package main implements the soundtouch-backup tool for backing up Bose SoundTouch
// cloud account data and local speaker filesystem files.
package main

import (
	"log"
	"os"
	"runtime/debug"

	"github.com/urfave/cli/v2"
)

var version = "dev"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		// Only fall back to build info when the version was not injected via
		// -ldflags (i.e. still the "dev" default, e.g. `go install …@vX.Y.Z`).
		// This keeps an explicitly stamped release version from being clobbered
		// by a VCS pseudo-version (e.g. v0.0.0-… from a shallow checkout).
		if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
}

func main() {
	app := &cli.App{
		Name:    "soundtouch-backup",
		Usage:   "Back up Bose SoundTouch account and speaker data",
		Version: version,
		Commands: []*cli.Command{
			allCommand(),
			cloudCommand(),
			localCommand(),
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
