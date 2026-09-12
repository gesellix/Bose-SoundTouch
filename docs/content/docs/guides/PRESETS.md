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
