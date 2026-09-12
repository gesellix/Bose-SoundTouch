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

Sources with no track concept report no `trackID` at all. There a readback can
never say anything, so the player settles the command on a write the speaker
accepted rather than holding the transport disabled for the whole readback
window only to report "unverified".

| What the speaker reports | Player behaviour             |
|--------------------------|------------------------------|
| a `trackID`              | verify the skip against it   |
| no `trackID`             | settle on the accepted write |

The firmware also reports `skipEnabled` and `skipPreviousEnabled` on
now-playing (parsed as `models.CanSkip` / `models.CanSkipPrevious`). The player
does not use them yet: they say whether a source can skip at all, which is a
question about whether to *offer* the buttons, not about how to verify a press.

### The unmeasured part

The only captured now-playing responses in the tree are Spotify, and both carry
`skipEnabled`, `skipPreviousEnabled` and a `trackID`. "Sources with no track
concept report no `trackID`" is therefore reasoned from the model, not measured
across sources. The failure mode if a source reports a `trackID` that never
changes across a skip is "unverified", not a false confirmation, which is the
safe direction to be wrong in.

Filling this in needs a speaker and two passes. The first collects what each
source *reports*, by watching while you switch sources on the speaker or in the
Bose app:

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
changed. Record both passes here:

| Source                 | `trackID` | `skipEnabled` | `skipPreviousEnabled` | Skip moves `trackID` | Skip verifiable |
|------------------------|-----------|---------------|-----------------------|----------------------|-----------------|
| `SPOTIFY`              | yes       | yes           | yes                   | ?                    | expected        |
| `TUNEIN`               | ?         | ?             | ?                     | ?                    | ?               |
| `RADIO_BROWSER`        | ?         | ?             | ?                     | ?                    | ?               |
| `LOCAL_INTERNET_RADIO` | ?         | ?             | ?                     | ?                    | ?               |
| `STORED_MUSIC`         | ?         | ?             | ?                     | ?                    | ?               |
| `BLUETOOTH`            | ?         | ?             | ?                     | ?                    | ?               |
| `AUX` / `PRODUCT`      | ?         | ?             | ?                     | ?                    | ?               |

The `SPOTIFY` row's first three columns come from the captured fixtures; its
probe has not been run either, so "skip verifiable" is an expectation until it
is.

Two findings would change the player: a source reporting a `trackID` that does
not change across a real skip (the readback would have to stop trusting it for
that source), and a source whose `skipEnabled` is absent while skipping works
anyway (which would rule out using the flag to hide the buttons).

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
