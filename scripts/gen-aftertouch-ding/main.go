// Offline generator for the AfterTouch "ding" WAV. Thin CLI
// wrapper around pkg/service/ding so the same renderer used at
// runtime (HandleDing on GET /media/aftertouch-ding.wav) can
// also be invoked from the shell — handy for previewing parameter
// tweaks or producing a one-off file for sharing.
//
// Run:
//
//	go run ./scripts/gen-aftertouch-ding > ding.wav
//	go run ./scripts/gen-aftertouch-ding -o ding.wav
//	go run ./scripts/gen-aftertouch-ding -pitch-high 1200 -o ding.wav
//
// All flags fall back to defaults defined in pkg/service/ding;
// pass only the knobs you want to override.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/gesellix/bose-soundtouch/pkg/service/ding"
)

func main() {
	var (
		outPath string
		opts    ding.Options
	)

	flag.StringVar(&outPath, "o", "", "output WAV path; default stdout")

	flag.IntVar(&opts.SampleRate, "sample-rate", 0, "Hz; 0 → default 22050")
	flag.Float64Var(&opts.PitchHigh, "pitch-high", 0, "Hz, top row; 0 → default 880 (A5)")
	flag.Float64Var(&opts.PitchMid, "pitch-mid", 0, "Hz, mid row; 0 → default 659.25 (E5)")
	flag.Float64Var(&opts.PitchLow, "pitch-low", 0, "Hz, bottom row; 0 → default 440 (A4)")
	flag.Float64Var(&opts.ChirpDuration, "chirp-sec", 0, "chirp duration; 0 → default 0.25")
	flag.Float64Var(&opts.GapDuration, "gap-sec", 0, "between-chirp gap; 0 → default 0.10")
	flag.Float64Var(&opts.AttackDuration, "attack-sec", 0, "fade-in; 0 → default 0.020")
	flag.Float64Var(&opts.ReleaseDuration, "release-sec", 0, "fade-out; 0 → default 0.060")
	flag.Float64Var(&opts.Peak, "peak", 0, "0..1 headroom; 0 → default 0.85")

	flag.Parse()

	data := ding.Render(opts)

	var w io.Writer = os.Stdout

	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gen-aftertouch-ding: create %s: %v\n", outPath, err)
			os.Exit(1)
		}

		defer func() { _ = f.Close() }()
		w = f
	}

	if _, err := w.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "gen-aftertouch-ding: write: %v\n", err)
		os.Exit(1)
	}
}
