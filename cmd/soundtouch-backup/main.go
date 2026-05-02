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
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
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
