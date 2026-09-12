package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/urfave/cli/v2"
)

// The player has to decide, per source, whether a track skip can be verified
// at all: it confirms a skip by watching `trackID` change, because track and
// stationName move on their own while a live stream rewrites its metadata.
// Sources with no track concept report no trackID, and there nothing can be
// verified.
//
// Which sources report what is a question only hardware answers, and answering
// it means walking every source on a real speaker. This command exists to make
// that walk one pass instead of one `play now --verbose` per source and a
// manual diff: it prints one row per observation and, with --watch, a new row
// whenever the speaker reports something different, so switching sources on
// the speaker (or in the app) fills in the matrix as you go.
//
// See docs/content/docs/reference/PLAYER-SOURCE-BEHAVIOUR.md, "Track skips:
// what can be verified", for the table these rows are collected into.

// capabilityObservation is one speaker answer, reduced to the fields that
// decide whether a skip is verifiable.
type capabilityObservation struct {
	Source        string
	SourceAccount string
	PlayStatus    string
	StreamType    string
	TrackID       string
	Track         string
	CanSkip       bool
	CanSkipPrev   bool
	SeekSupported bool
}

func observeCapabilities(nowPlaying *models.NowPlaying) capabilityObservation {
	if nowPlaying == nil {
		return capabilityObservation{}
	}

	return capabilityObservation{
		Source:        nowPlaying.Source,
		SourceAccount: nowPlaying.SourceAccount,
		PlayStatus:    nowPlaying.PlayStatus.String(),
		StreamType:    nowPlaying.StreamType,
		TrackID:       nowPlaying.TrackID,
		Track:         nowPlaying.Track,
		CanSkip:       nowPlaying.CanSkip(),
		CanSkipPrev:   nowPlaying.CanSkipPrevious(),
		SeekSupported: nowPlaying.IsSeekSupported(),
	}
}

// skipVerdict states, for this observation, what the player can do with a
// skip command against this source. It is the whole point of the sweep.
func (o capabilityObservation) skipVerdict() string {
	switch {
	case o.Source == "" || o.Source == "STANDBY":
		return "n/a (nothing playing)"
	case o.TrackID != "":
		return "verifiable by trackID"
	case o.CanSkip || o.CanSkipPrev:
		return "claims skip, no trackID"
	default:
		return "no track concept"
	}
}

// reportsSameCapabilities compares only what the matrix records, so a row is
// not reprinted every poll while a track's elapsed time ticks along. A new
// trackID is a change: that is what a skip is expected to produce.
func reportsSameCapabilities(previous, current capabilityObservation) bool {
	return previous == current
}

func placeholder(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}

	return value
}

func capabilityHeader() []string {
	return []string{"TIME", "SOURCE", "ACCOUNT", "STATUS", "STREAMTYPE", "TRACKID", "SKIP", "SKIPPREV", "SEEK", "SKIP VERDICT"}
}

func capabilityRow(observed time.Time, o capabilityObservation) []string {
	return []string{
		observed.Format("15:04:05"),
		placeholder(o.Source),
		placeholder(o.SourceAccount),
		placeholder(o.PlayStatus),
		placeholder(o.StreamType),
		placeholder(o.TrackID),
		fmt.Sprintf("%t", o.CanSkip),
		fmt.Sprintf("%t", o.CanSkipPrev),
		fmt.Sprintf("%t", o.SeekSupported),
		o.skipVerdict(),
	}
}

// classifySkipProbe reports what a real skip did to the fields a readback
// could confirm it with. This is the decisive measurement: observing that a
// source reports a trackID says nothing until a skip has been seen to move it,
// and a source whose title changes while the trackID stays put is exactly the
// case that made the player's first confirmation rule wrong.
func classifySkipProbe(before, after capabilityObservation) string {
	switch {
	case before.Source != after.Source:
		return "source changed during the probe, so nothing can be concluded; rerun while one source stays put"
	case before.TrackID == "" && after.TrackID == "":
		return "no trackID before or after: a readback has nothing to verify, so the player settles on the write"
	case before.TrackID != after.TrackID:
		return "trackID changed: skips are verifiable on this source"
	case before.Track != after.Track:
		return "trackID unchanged while the title changed: the title moves on its own and cannot confirm a skip"
	default:
		return "nothing changed within the probe window: either the skip did not land, or this source reports no change for one"
	}
}

// probeSkip sends one NEXT_TRACK and reports what changed, polling rather than
// sleeping once, so a slow source is given its time without making a fast one
// wait for it.
func probeSkip(c *cli.Context, soundTouchClient skipProbeClient, before capabilityObservation,
	report func(capabilityObservation),
) error {
	if before.Source == "" || before.Source == "STANDBY" {
		return fmt.Errorf("nothing is playing, so a skip would measure nothing")
	}

	fmt.Printf("\nProbing: sending %s and watching what changes. This really does skip a track.\n\n",
		models.KeyNextTrack)

	if err := soundTouchClient.SendKeyPressOnly(models.KeyNextTrack); err != nil {
		return fmt.Errorf("failed to send next track command: %w", err)
	}

	wait := c.Duration("probe-wait")
	interval := c.Duration("interval")
	deadline := time.Now().Add(wait)
	after := before

	for time.Now().Before(deadline) {
		time.Sleep(interval)

		nowPlaying, err := soundTouchClient.GetNowPlaying()
		if err != nil {
			PrintWarning(fmt.Sprintf("readback failed: %v", err))

			continue
		}

		after = observeCapabilities(nowPlaying)
		if !reportsSameCapabilities(before, after) {
			break
		}
	}

	report(after)

	fmt.Printf("\nVerdict: %s\n", classifySkipProbe(before, after))

	return nil
}

// skipProbeClient is the slice of the SoundTouch client a probe needs.
type skipProbeClient interface {
	GetNowPlaying() (*models.NowPlaying, error)
	SendKeyPressOnly(key string) error
}

// playbackCapabilities polls now-playing and reports the skip-relevant
// capabilities, once or until the watch window closes.
func playbackCapabilities(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	soundTouchClient, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	PrintDeviceHeader("Observing playback capabilities", clientConfig.Host, clientConfig.Port)

	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, strings.Join(capabilityHeader(), "\t"))

	printObservation := func(o capabilityObservation) {
		fmt.Fprintln(writer, strings.Join(capabilityRow(time.Now(), o), "\t"))
		_ = writer.Flush()
	}

	nowPlaying, err := soundTouchClient.GetNowPlaying()
	if err != nil {
		return fmt.Errorf("failed to get now playing: %w", err)
	}

	previous := observeCapabilities(nowPlaying)
	printObservation(previous)

	if c.Bool("probe-skip") {
		return probeSkip(c, soundTouchClient, previous, printObservation)
	}

	if !c.Bool("watch") {
		return nil
	}

	interval := c.Duration("interval")
	duration := c.Duration("duration")

	fmt.Printf("\nWatching every %s. Switch sources on the speaker; a row is printed whenever the report changes.\n", interval)

	if duration > 0 {
		fmt.Printf("Stopping after %s. Press Ctrl+C to stop sooner.\n\n", duration)
	} else {
		fmt.Print("Press Ctrl+C to stop.\n\n")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if duration > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nObservation finished.")

			return nil
		case <-ticker.C:
			nowPlaying, err := soundTouchClient.GetNowPlaying()
			if err != nil {
				// A speaker that is briefly unreachable is not a reason to
				// abandon a sweep that may run for minutes.
				PrintWarning(fmt.Sprintf("readback failed: %v", err))

				continue
			}

			current := observeCapabilities(nowPlaying)
			if reportsSameCapabilities(previous, current) {
				continue
			}

			previous = current

			printObservation(current)
		}
	}
}
