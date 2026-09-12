---
title: "Player: Sources and Selection State"
---
How the embedded web player (`soundtouch-player`, and the same UI served by
`soundtouch-service`) decides what a source button does, and how it decides
whether a selection worked.

Both questions have non-obvious answers, learned from real hardware. This page
records what the speaker actually does, so the behaviour is not re-derived or
accidentally undone.

## Not every advertised source can be selected

A speaker's `/sources` lists what it knows about, each with a `status`. The
player renders every `status="READY"` entry as a button. That set is not
uniform: some entries are **inputs**, some are **providers**.

An **input** can be selected on its own. `AUX`, `BLUETOOTH`, a `SPOTIFY`
entry with a real `sourceAccount`: `POST /select` with just the source and
account is meaningful, and the speaker resumes that input.

A **provider** cannot. `RADIO_BROWSER`, `TUNEIN` and `LOCAL_INTERNET_RADIO`
need a station **ContentItem carrying a `Location`** (see
`stations.ResolveContentItem`, which sets `type="stationurl"`). There is
nothing for the speaker to resume from the source name alone.

All three are confirmed on hardware: `RADIO_BROWSER` and
`LOCAL_INTERNET_RADIO` by the stub described below, `TUNEIN` by its resume
path playing the station as intended.

`STORED_MUSIC` is a third case: one entry per media server, its
`sourceAccount` being a server UDN. Selecting it identifies no track or
container.

## What a bare select does to a provider

The speaker does not refuse. It answers `200`, and parks on a stub
now-playing, while whatever was playing before **carries on**:

```xml
<nowPlaying deviceID="..." source="RADIO_BROWSER" sourceAccount="">
  <ContentItem source="RADIO_BROWSER" type="" location="" isPresetable="false">
    <itemName>RADIO_BROWSER</itemName>
  </ContentItem>
</nowPlaying>
```

Four things identify the stub: no `playStatus`, empty `type`, empty
`location`, and an `itemName` that just echoes the source name.

The speaker then reports that stub indefinitely. Observed on hardware: the
player showed RadioBrowser while Spotify was audible, and a naive readback
"confirmed" the selection because the reported source did match the one
requested. `LOCAL_INTERNET_RADIO` produces a byte-identical stub.

This is speaker behaviour, not something the player or the service can fix
after the fact. The only remedy is not to issue such a select.

## What the player does instead

| Source                 | Click behaviour                               | Resumes from Recents |
|------------------------|-----------------------------------------------|----------------------|
| `RADIO_BROWSER`        | resume newest station, else open RadioBrowser | yes                  |
| `TUNEIN`               | resume newest station, else open TuneIn       | yes                  |
| `LOCAL_INTERNET_RADIO` | open Play URL                                 | no                   |
| `STORED_MUSIC`         | open Library                                  | no                   |
| anything else          | `POST /select` as before                      | n/a                  |

Resuming replays the newest Recents entry for that source, using that entry's
own ContentItem: the real item the speaker was given, `Location` included.

**`LOCAL_INTERNET_RADIO` deliberately does not resume.** AfterTouch plays its
own one-shot audio through that source: TTS and the notification ding both go
out over `/custom/v1/playback/`. Its Recents therefore mix notifications with
stations, and on a test speaker the *only* entry was "AfterTouch ding", so
resuming played the ding. The announcement path is distinguishable from Play
URL's `bmx.BuildOrionLocation`, but it also carries CLI URL playback, and any
future audio-injecting feature would have to remember to stay clear of it.
Opening Play URL does not depend on classifying what is in Recents.

`ALEXA` is advertised `READY` too and is deliberately left alone: it cannot be
tested on the hardware available, and guessing at its behaviour risks breaking
a source that works today. The backstop below covers it instead.

### The backstop

The table above only covers sources known to need it, and a source list is
whatever the speaker chooses to advertise. So the readback additionally
refuses to *confirm* the stub itself, wherever it comes from: a now-playing
naming the requested source but with no `Location`, no `PlayStatus`, and an
`ItemName` equal to the source is reported as a failure.

All three conditions are required together. A physical input reports no
location and no item name of its own yet is genuinely playing, so any single
condition alone would reject real selections.

## How a selection is confirmed

`POST /select` returning `200` proves nothing: the speaker can reject a source
seconds later, surfacing as a transition to an error source
(`INVALID_SOURCE`, `*_ERROR`). So the player posts once, then watches.

- The **event stream** is the primary watcher. A `nowPlayingUpdated` event
  reports a late rejection as it happens, and the player turns it into a
  failure.
- **Bounded readbacks** at 2s, 5s and 10s are the fallback for a speaker whose
  events are not arriving. They stop as soon as a confirmation arrives *and*
  the readback reports a live event stream, so a confirmed selection normally
  costs one request rather than three. That signal is `webSocketConnected`,
  which reports the service's own socket to the speaker; it is opened lazily
  on first fetch or control of a device, so the very first click after
  loading one can still take all three.
- Readbacks use `GET /devices/{id}/now-playing`, which refreshes only
  `/now_playing`. The full device fetch runs a complete status poll: six
  sequential speaker calls plus `/getGroup` on a stereo-capable model, to
  answer one question, against a device that may be slow precisely because
  something is wrong.

Outcomes are `pending`, `provisional-confirmed`, `final-confirmed`,
`unverified` and `failed`, shown in a live region under the source list. A
confirmation from a push event is never retracted by a later failed readback.

### Definitive versus uncertain failures

A rejected write is not always proof the command never landed:

- **4xx** is produced before the service contacts the speaker (unknown device,
  unparseable body, empty source). The command provably never went out, so the
  failure is reported immediately.
- **5xx and transport errors** are ambiguous. `handleSourceControl` reports a
  failed `Client.SelectSource` through `sendControlResponse`, which maps any
  speaker-call error to 500, and a request that timed out *after* the speaker
  already switched looks identical to one it never received. The readbacks
  keep running and the reason is carried into whatever outcome they reach.

## Track skips: what can be verified

A skip is confirmed by watching *what is playing* change, and picking the wrong
field gets it wrong in both directions. `track` and `stationName` are the
obvious candidates and the wrong ones: a live stream rewrites its own title
while the same stream keeps playing, so a skip that did nothing looks
confirmed. `trackID` is a stable per-track identity (`spotify:track:...` in the
captured firmware responses), so it stays put through a title rewrite and moves
on a real skip.

Not every source reports one. A media library skips perfectly well while
reporting no `trackID` at all, and there its track metadata is the signal,
because a library's title changes only when the track really changes. Live
radio changes its title on its own and can skip nothing, so there no readback
can ever say anything, and the player settles the command on a write the
speaker accepted rather than holding the transport disabled for the whole
readback window only to report "unverified".

| What the speaker reports          | Player behaviour                  |
|-----------------------------------|-----------------------------------|
| a `trackID`                       | verify the skip against it        |
| no `trackID`, but `skipEnabled`   | verify against the track metadata |
| neither                           | settle on the accepted write      |

The middle row is why `skipEnabled` / `skipPreviousEnabled` (parsed as
`models.CanSkip` / `models.CanSkipPrevious`) matter: a media library changes
its track title only when the track really changes, and says so by claiming
skip support, while live radio rewrites its title with the same stream playing
on and claims no skip support at all. The claim is what tells the two apart,
and without it a whole source would be unverifiable. The metadata identity is
`track`, `artist`, `album` and the ContentItem's `location` together.

The flags are **not** used to enable or disable the skip buttons, deliberately:
they flap while a source buffers. The same Spotify track reported
`skipPreviousEnabled` false mid-buffer and true a few seconds later, and
Spotify reported no `trackID` at all during one buffering window, so buttons
driven by them would flicker.

### Previous-track restarts before it steps back

The first `PREV_TRACK` restarts the track that is playing; only a second press
moves to the previous one. Measured on a SoundTouch 10 against a media library:
at position 97s a press left the track and the queue offset untouched and reset
the position to 0, and a press five seconds later stepped from track 10 to
track 09. Pressing from a track that has just begun steps back immediately,
since there is nothing to restart: a walk back through a whole album, each
press landing at position 0 or 1, moved one track per press from 10 down to 01.

`NEXT_TRACK` has no such rule and always advances.

This matters twice over. It looks like a broken Previous button when a track is
well under way, and it defeats any confirmation based on identity, because a
restart changes no track, no `trackID` and no metadata. The player therefore
also accepts a play position that has moved backwards as a confirmed
`previous-track`. Nothing about it is specific to a source or a queue position:
there is no "first track of the queue" border, and an album selected as a
container keeps every track before the current one available.

### What the hardware reports

Measured on a SoundTouch 10 (server version 4) by walking its sources:

| Source                 | `trackID` | `skipEnabled` | `skipPreviousEnabled` | Skip confirmed by  |
|------------------------|-----------|---------------|-----------------------|--------------------|
| `SPOTIFY`              | yes       | yes           | yes                   | `trackID`          |
| `STORED_MUSIC`         | no        | yes           | yes                   | track metadata     |
| `TUNEIN`               | no        | no            | no                    | nothing: the write |
| `RADIO_BROWSER`        | no        | no            | no                    | nothing: the write |
| `AUX`                  | no        | no            | no                    | nothing: the write |
| `LOCAL_INTERNET_RADIO` | ?         | ?             | ?                     | ?                  |
| `BLUETOOTH`            | ?         | ?             | ?                     | ?                  |

`STORED_MUSIC` is the case the design turns on: it skips perfectly well and
says so, but reports no `trackID`, so a trackID-only rule would leave an entire
source unverifiable. It was measured against this repository's own
`cmd/example-dlna-server`; another media server may report a `trackID`, and
nothing guarantees either way. That is why a `trackID` is preferred wherever
one is present and the metadata path is only the fallback.

The reports are not stable moment to moment. Mid-buffer, the same Spotify track
reported `skipPreviousEnabled` false and, in a later buffering window, no
`trackID` at all. The player reads these only at the moment the button is
pressed, so a press inside such a window simply settles on the write.

Both passes below are worth repeating on other hardware, other firmware and
other media servers. The first collects what each source *reports*, by watching
while you switch sources on the speaker or in the Bose app:

```bash
soundtouch-cli --host <speaker> play capabilities --watch
```

It prints one row per observation and a new row whenever the report changes, so
one run covers every source you visit. A radio stream rewriting its own title
while the `trackID` stays put shows up here as a new row too, which is the
behaviour that rules the title out as a confirmation signal.

Reporting a `trackID` is not the same as that `trackID` moving when a skip
happens, and only the second is what the player relies on. The second pass
measures it, once per source, while that source is playing:

```bash
soundtouch-cli --host <speaker> play capabilities --probe-skip
```

That really does skip a track: it sends one `NEXT_TRACK`, watches for up to
`--probe-wait` (6s by default), prints the before and after rows, and states
whether the `trackID` moved, whether only the title moved, or whether nothing
changed. Add what you find to the table above.

Still open:

- `LOCAL_INTERNET_RADIO` and `BLUETOOTH` have not been observed at all.
- The probe pass has not been run per source. Reporting a `trackID` is not the
  same as that `trackID` moving on a skip, and the table's right-hand column is
  so far an inference from the first pass rather than a measurement.
- Media servers other than this repository's `cmd/example-dlna-server` may
  report a `trackID` for `STORED_MUSIC`, which would simply move that source
  onto the `trackID` path.

Two findings would change the player: a source reporting a `trackID` that does
not move across a real skip (the readback would have to stop trusting it
there), and a source that claims `skipEnabled` while rewriting its track
metadata on its own (that would break the metadata fallback the way the title
broke the original rule).

## Ordering: revisions and epochs

Status reaches the browser three ways — a full `devices` snapshot, a
`status_update` delta, and REST refreshes — with no inherent ordering. Two
fields fix that:

- `revision` advances on every projection, so a frame no newer than what the
  browser holds is dropped.
- `nowPlayingRevision` is the now-playing field's own generation. `revision`
  alone cannot answer "did now-playing actually change?", because any other
  field's merge advances it; a selection waiting for confirmation needs
  exactly that distinction.

Revisions are per-connection and restart at 0, so they are only comparable
within one **`epoch`**, which identifies the connection that produced the
status. Without it, a device backed by a fresh connection would publish
revisions the browser rejects forever, freezing that device's display until
reload. Epochs are seeded from the wall clock and forced strictly increasing,
so they keep rising across a service restart, and are in milliseconds because
the browser compares them as JSON numbers.

## Source inventory staleness

`sourcesStale` marks an inventory the speaker has stopped confirming; the
player keeps showing it but disables the buttons.

It is set after **two consecutive** failed `/sources` reads, not one. A single
dropped read is not evidence the list is wrong, and marking it stale
immediately disabled every source button on a transient hiccup. This mirrors
`offlineFailureThreshold`, which debounces connectivity the same way. A
successful read clears the marker and resets the count.

Successful reads remain ordered by generation, so an older one cannot
overwrite a newer one. Failures are not ordered: a failure carries no
inventory, so spending the generation on it would let a failed read discard a
concurrent successful one.

## Related

- [Source Selection Guide](SOURCE-SELECTION.md) — the `/select` endpoint and
  the client library
- [WebSocket Events](WEBSOCKET-EVENTS.md) — the event stream the confirmation
  relies on
- [Radio Browser](radio-browser.md) — the RadioBrowser provider
