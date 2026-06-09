---
title: "Playing Music from a DLNA / NAS Library"
---
This guide explains how to browse a DLNA/UPnP media server on your LAN (NAS,
FRITZ!Box, minidlna, Plex, etc.) and play its tracks on a SoundTouch speaker
using the speaker's native `STORED_MUSIC` source.

---

## How it works

The **speaker is the DLNA control point**, not AfterTouch. The flow is:

1. AfterTouch (or the CLI) discovers DLNA servers on the LAN via SSDP.
2. You register a server on the speaker with `setMusicServiceAccount`. This
   creates a `STORED_MUSIC` source entry on the speaker.
3. You browse the library through the speaker's `/navigate` endpoint. The
   speaker contacts the media server's ContentDirectory service and returns its
   own browse tokens (e.g. `4:cont2:150:0:0:` for folders, or a track token
   ending in `TRACK`).
4. You select a track or folder using `/select` with a `STORED_MUSIC`
   `ContentItem`. The speaker fetches the audio directly from the media server.
   AfterTouch does not proxy the audio stream.

> **Why use the speaker's browse tokens instead of raw DLNA object IDs?**
> The speaker's `/select` endpoint only accepts the tokens it produces via
> `/navigate`. Raw DLNA ContentDirectory object IDs (like `"64"`) are not
> accepted for playback.

---

## Prerequisites

- A DLNA/UPnP media server running on the same LAN as the speaker (e.g. NAS
  with minidlna, FRITZ!Box media server, Plex with DLNA enabled).
- The speaker and the media server must be on the same network segment so the
  speaker can reach the server directly for streaming.
- `soundtouch-cli` installed and able to reach the speaker (test with
  `soundtouch-cli --host 192.0.2.10 info`).

---

## Step 1: Discover servers on the LAN

Run an SSDP sweep from the machine where the CLI runs:

```bash
soundtouch-cli library servers
```

Sample output:

```
Found 1 DLNA media server(s):

  Name:   My Music Library
  Vendor: minidlna / MiniDLNA 1.3.3
  UDN:    uuid:00000000-0000-0000-0000-000000000000
  CDS:    http://192.0.2.20:8200/ctl/ContentDir
```

Note the **UDN** (the `uuid:...` string). You will need it in the next steps.

Alternatively, ask a specific speaker for its own DLNA list (the speaker runs
its own independent UPnP sweep):

```bash
soundtouch-cli --host 192.0.2.10 library servers --via-speaker
```

> The speaker's list and the app-side list may differ. The speaker reports only
> servers it has seen on its UPnP sweep, which can lag behind or miss servers
> that appear after the speaker boots.

---

## Step 2: Register the server on the speaker

The `STORED_MUSIC` source account is the bare UUID from the UDN with a `/0`
suffix appended. If the UDN from discovery is
`uuid:00000000-0000-0000-0000-000000000000`, the account string is
`00000000-0000-0000-0000-000000000000/0` (drop the `uuid:` prefix).

```bash
soundtouch-cli --host 192.0.2.10 account add-nas \
  --user 00000000-0000-0000-0000-000000000000/0 \
  --name "My Music Library"
```

Verify the source is visible:

```bash
soundtouch-cli --host 192.0.2.10 source list
```

The output should include a `STORED_MUSIC` entry with your display name and
status `READY`. If the status is `UNAVAILABLE`, wait 10-20 seconds and check
again; the speaker needs a moment to connect to the media server.

---

## Step 3: Browse the library

Get the top-level containers:

```bash
soundtouch-cli --host 192.0.2.10 browse stored-music \
  --source-account 00000000-0000-0000-0000-000000000000/0
```

Drill into a folder using a location token returned from the previous step:

```bash
soundtouch-cli --host 192.0.2.10 browse container \
  --source STORED_MUSIC \
  --source-account 00000000-0000-0000-0000-000000000000/0 \
  --location "4:cont2:150:0:0:" \
  --type dir
```

Repeat with `--type dir` for sub-folders, or `--type track` for track
containers. The `Location` values shown in browse output are the tokens to
pass to the next `browse container` or `library play` call.

---

## Step 4: Play a track

Pass the location token from a browse result to `library play`:

```bash
soundtouch-cli --host 192.0.2.10 library play \
  --source-account 00000000-0000-0000-0000-000000000000/0 \
  --location "5:audio5:part13:3171:5 TRACK" \
  --name "Track Title"
```

The `--name` flag sets the display name shown on the speaker's display and in
the web UI. It is optional but recommended for clarity.

The CLI checks that the `STORED_MUSIC` source is in `READY` state before
sending the play command. If it is not ready, it prints the `account add-nas`
command you need to run first.

---

## Player UI (BETA)

The soundtouch-player "Library" tab provides a browser-based interface for the
same workflow:

1. Open the player at `http://<aftertouch-host>:8000`.
2. Go to the **Library** tab.
3. Select the target speaker from the device list.
4. Click **Find servers** to run an SSDP sweep.
5. Click **Add** next to a server to register it on the speaker.
6. Open the server to browse folders and tracks.
7. Click a track to play it on the speaker.

> **BETA notice:** DLNA behavior varies across server implementations. Some
> servers expose non-standard browse trees or restrict access by IP. If a
> server appears in discovery but does not load in the library browser, check
> that the media server allows UPnP browsing from the speaker's IP address.

---

## Removing a server

```bash
soundtouch-cli --host 192.0.2.10 account remove-nas \
  --user 00000000-0000-0000-0000-000000000000/0
```

---

## Gotchas and limitations

**No password needed for STORED_MUSIC.** The registration only requires the
server UDN. The `--user` flag takes the bare UUID plus `/0`; no `--password`
flag is accepted or needed.

**Source becomes UNAVAILABLE after a speaker reboot.** The speaker re-runs its
UPnP sweep on startup. Until it rediscovers the media server (usually within
30 seconds), the `STORED_MUSIC` source shows as `UNAVAILABLE`. Wait for it to
return to `READY` before browsing or playing.

**`uuid:` prefix in UDN.** Discovery output may show the full UDN as
`uuid:00000000-0000-0000-0000-000000000000`. Drop the `uuid:` prefix when
passing it to `--user`; the account string is just the UUID plus `/0`.

**Format support.** The SoundTouch firmware decodes MP3 and AAC streams. HLS
(`.m3u8`) playlists and formats the firmware cannot decode (e.g. FLAC, ALAC,
OGG) will not play. If a track starts and immediately stops, the audio format
is likely unsupported.

**Do not re-register a READY source.** Calling `account add-nas` on a source
that is already `READY` can flip it to `UNAVAILABLE`. Check `source list`
first.
