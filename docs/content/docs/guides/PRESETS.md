---
title: "Get your preset buttons working again"
# Kept so links to the old location keep working. This page used to live at
# docs/appendix/PRESET-QUICKSTART. Hugo builds a redirect at every alias, so
# add one whenever a page moves or is renamed, and never remove an old entry.
aliases:
  - /docs/appendix/PRESET-QUICKSTART/
---
The six buttons on top of your speaker are the first thing the Bose shutdown
took away: pressing one used to ask Bose's servers what to play. With
AfterTouch running and your speaker migrated, they work again, and you decide
what goes in each slot.

This guide uses the web UI, which is the shortest path. The command line does
the same things and is covered further down, for anyone who would rather script
their setup.

Before you start, you need a migrated speaker that appears in the web UI. If
that is not the case yet, see [Getting Started](GETTING-STARTED.md).

## Save a radio station

1. Open the web UI at `http://<your-server>:8000` and pick your speaker.
2. Find something to play. The **TuneIn** page browses stations by category,
   and the **RadioBrowser** page searches
   [a community directory](../reference/radio-browser.md) of internet stations.
   Both let you play a result straight to a speaker, so there is no need to
   hunt for stream URLs by hand.
3. Press play and let the station start.
4. Press the **★** at the top right of the now-playing card, then pick a slot
   from **1 2 3 4 5 6**.

The star turns gold once the current content is saved in a slot, and its
tooltip names which one. An empty preset tile reads **Save current here** and
saves to that slot when you click it, which is quicker when you already know
where you want it.

Press the matching button on the speaker. It should start playing within a few
seconds.

## Save music from a NAS or USB drive

Music from a DLNA server works the same way, and does not need to be playing
first.

1. Open the **Library** page and pick your media server. If none is listed,
   set one up first: see [Play from a NAS or USB drive](dlna-music-library.md).
2. Browse to an album, a folder, or a single track.
3. Press the **★** on that row and pick a slot.

Nothing is interrupted: the item is saved by name, so whatever is playing keeps
playing. A saved folder plays from its first track when you recall it, and
skip forward and back move through it.

## Save what is already playing

Anything the speaker can play and mark as presetable can go in a slot, whatever
its source: a Spotify album, a TuneIn station, a track from your library. Play
it, press the **★**, pick a slot.

Live inputs cannot be saved, because there is nothing to recall: Bluetooth, AUX
and AirPlay have no address to store. The star is greyed out for those.

## What a preset remembers

A preset stores what to play, not the audio: a source, a location within that
source, and a name. Two consequences worth knowing:

- **Artwork comes from the source.** Library items get their cover from your
  media server, so a preset tile can be blank if that server is unreachable
  from your browser. Presets saved before v0.131.0 keep their blank artwork
  until you save them again; there is no way to fill them in retroactively.
- **A preset can go stale.** If a media server reindexes, or a station
  disappears from a directory, the stored location may no longer resolve. Save
  it again to fix it.

## Change a slot without playing anything

The **✎** next to each preset tile opens the slot editor. It lists what
AfterTouch has seen this household play or store, so filling a slot is picking
from that list rather than finding the station again first.

From the editor you can:

- **Fill the slot** from the list. Type in the filter box to narrow it.
- **Rename** what is in the slot. The name is yours; it does not have to match
  what the station calls itself.
- **Move it to another slot.** Press a slot number under "Move to". A slot that
  already holds something is marked, says what it would replace, and needs a
  second press.
- **Empty the slot.** The station stays on the list, so this loses the slot's
  contents, not the station.

Nothing here needs the speaker to be playing, and none of it involves editing
files. If you have been recovering presets by hand from `Presets.xml` or over
SSH, this replaces that.

### Content another speaker has

The list is shared across your speakers, so it also offers things only one of
them can play: an album on a media server this speaker has not discovered, or a
service linked on another one. Those entries are dimmed and say why, for
example "not on this speaker's Library accounts".

You can still pick one. The marking is based on the source list the player last
read, which can be a few minutes old, so AfterTouch checks again against the
speaker before writing and refuses with what is missing, naming the sources the
speaker does have. To use such an entry here, add that media server or link that
service on this speaker first, then pick it again.

### The list, and what is on it

The list is a catalog of what AfterTouch has seen: every preset it has stored
and everything that has played, across all your speakers, whichever one it
happened on. That is what makes it useful for two jobs at once.

- **Getting something back.** A slot that gets emptied, by you or by anything
  else, leaves its station on the list. Putting it back is picking it again.
- **Copying between speakers.** Open the editor on the second speaker and pick
  the same entry.

An entry already sitting in a slot is still listed, and says which slot it is
in. Putting the same station in two slots is allowed; it is sometimes what you
want.

The list holds the 100 most recently seen entries, which costs about 45 KB. To
change that, set `catalog_size` in `settings.json`:

```json
{ "catalog_size": 250 }
```

When the list is full, stations that merely played are dropped before ones that
were saved as a preset, so what you are most likely to want back stays longest.

Setting it to `0` turns the catalog off and discards what is stored, which is
worth knowing if you run AfterTouch on the speaker itself and want to keep the
flash volume as quiet as possible. Leaving it out keeps the default of 100.

### When AfterTouch and the speaker disagree

Above the preset tiles, a warning appears if what AfterTouch has stored for a
speaker does not match what the speaker reports, for example "AfterTouch stores
8 presets for this speaker, 2 of which it can never play". On a healthy setup
there is nothing there.

Open it and you see the stored rows as stored, each saying why it cannot be
played: no such button on this speaker, no button number at all, or nothing to
play. Remove them one at a time, or use the button that removes all the rows
the speaker cannot play.

This matters beyond tidiness. A stored list with more rows than the speaker has
buttons is why:

- **"Sync Data" is refused as destructive.** Importing the speaker's six
  presets would shrink the stored eight, and a shrinking import is refused
  unless you confirm it.
- **A preset you just saved comes back as the old one.** AfterTouch serves its
  stored list to the speaker, so the extra rows keep winning.

Where a button holds different things on the two sides, it is listed with both,
and you pick one:

- **Keep ours** puts what AfterTouch stores onto the speaker, so the speaker
  catches up now instead of at its next fetch.
- **Take the speaker's** stores what the speaker has, so AfterTouch stops
  handing back the old entry.

Either way the other one stays in the list you fill slots from, so a choice can
be undone. This is the piece "Sync Data" cannot do: that imports a speaker's
whole list at once, and refuses outright when it would shrink what is stored.

**Remove** deletes the row from what AfterTouch stores. It presses nothing on
the speaker, and it does not lose the station: that stays in the list you pick
from, so you can put it straight back into a slot.

This is the one edit that goes to AfterTouch rather than to the speaker. The
rows exist only in AfterTouch, and some of them name no button the speaker
could be asked about.

## Presets on several speakers

Speakers that share one AfterTouch account share their presets: saving or
clearing a preset on one applies it to the others. How quickly they show it
depends on your setup, see "When the other speakers pick it up" below.

What happens by default:

- **One speaker, or speakers that already hold the same presets**: sharing is
  on, so a change reaches all of them.
- **A speaker added later with no presets**: it adopts the account's presets.
- **Speakers that already hold different presets**: nothing is overwritten.
  AfterTouch leaves them as they are until you say which way it should go.

To change that for an account:

```bash
curl -X POST http://192.0.2.10:8000/api/mgmt/accounts/<accountId>/preset-sync \
  -H 'Content-Type: application/json' -d '{"preset_sync": "on"}'
```

`on` always shares a change, `off` never overwrites another speaker's preset
(a speaker with no presets still adopts them, since nothing is lost that way),
and `auto` is the default described above.

### Sharing a preset is not the same as "Sync Data"

The two move in opposite directions, which is easy to mix up:

|                                        | What it does                                            | Reaches other speakers                   |
|----------------------------------------|---------------------------------------------------------|------------------------------------------|
| Saving or clearing one preset          | writes that slot, on the speaker and in AfterTouch      | yes, this is the sharing described above |
| "Sync Data" (admin UI) or `setup sync` | imports one speaker's whole preset list into AfterTouch | no, it stays on that speaker             |

An import takes the list exactly as that one speaker reports it at that
moment. If that list is shorter or out of date, handing it to every other
speaker would spread the loss, so an import is never shared. AfterTouch even
refuses an import that would shrink what it already holds, unless you confirm
it.

### When the other speakers pick it up

Storing the preset in AfterTouch is the sharing. The speakers fetch their own
presets, so each one picks the change up by itself: at its next fetch, when it
is switched on or rebooted, or when you press "Refresh sources on speaker".

Where AfterTouch can reach the speakers, it also nudges them, and the change
shows up within seconds. That nudge is an accelerator, not the mechanism: if
AfterTouch runs somewhere it cannot reach them (a public cloud host, for
example), the presets still arrive, just whenever the speakers next ask.

## From the command line

`soundtouch-cli` talks to the speaker directly, and is the right tool for
repeatable setups. Point `--host` at the speaker, not at the service.

### See what is in the slots

```bash
soundtouch-cli --host 192.0.2.100 preset list
```

### Save what is playing

```bash
soundtouch-cli --host 192.0.2.100 preset store-current --slot 1
```

### Save specific content

```bash
# A TuneIn station
soundtouch-cli --host 192.0.2.100 preset store \
  --slot 3 \
  --source TUNEIN \
  --location "/v1/playback/station/s33828" \
  --name "Example Radio"

# A Spotify album
soundtouch-cli --host 192.0.2.100 preset store \
  --slot 2 \
  --source SPOTIFY \
  --location "spotify:album:4aawyAB9vmqN3uQ7FjRGTy" \
  --name "An Album"
```

### Save a direct stream URL

A raw `.mp3` or `.aac` stream needs one extra flag. The speaker's radio module
fetches the preset's location and expects a station description in return, not
audio, so AfterTouch has to serve that description. `--service-url` tells the
CLI where your service lives, and the stream URL is wrapped for you:

```bash
soundtouch-cli --host 192.0.2.100 preset store \
  --slot 4 \
  --source LOCAL_INTERNET_RADIO \
  --location "https://stream.example.com/jazz" \
  --name "Jazz Stream" \
  --service-url https://soundtouch.local
```

Without `--service-url` the preset is stored but the speaker cannot play it.

### Recall and remove

```bash
soundtouch-cli --host 192.0.2.100 preset select --slot 1
soundtouch-cli --host 192.0.2.100 preset remove --slot 6
```

## When a preset does not work

**The star is greyed out.** The current source cannot be saved. Bluetooth, AUX
and AirPlay never can; switch to a station or a library item first.

**The button does nothing on the speaker.** Check the slot actually holds
something (`preset list`, or look at the tiles). If it does, the stored location
may have gone stale, so save it again. For a `LOCAL_INTERNET_RADIO` preset
stored from the command line, check it was stored with `--service-url`.

**A slot emptied itself.** One cause was found and fixed in v0.132.0 (presets
were addressed by list position rather than by button number, so one could
overwrite another), and a second candidate fix shipped in the same release for
library presets whose account is a server name. If you are on an older version,
upgrade and save the preset again. If it still happens, please
[open an issue](https://github.com/gesellix/Bose-SoundTouch/issues): that is
worth knowing about.

**Nothing happens at all.** Work through [Troubleshooting](TROUBLESHOOTING.md)
first; a speaker that is not reaching your service cannot recall anything.

## Going further

- [Preset Management](../reference/PRESET-MANAGEMENT.md) is the reference: the
  ContentItem shape, what the speaker stores, and where it is fragile.
- [CLI Reference](CLI-REFERENCE.md) documents every `preset` flag.
- [WebSocket Events](../reference/WEBSOCKET-EVENTS.md) covers reacting to
  preset changes in your own code.
- [Preset management example](https://github.com/gesellix/Bose-SoundTouch/tree/main/examples/preset-management)
  is the Go version.
