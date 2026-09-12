---
title: "Getting Started"
weight: 1
---
Bose shut down the SoundTouch cloud on 6 May 2026, and your speakers lost the
parts that depended on it: the preset buttons, music-service browsing, and
stereo pairing on the SoundTouch 10. **AfterTouch** puts those back by running
the missing service on a machine in your own home.

This page is the short route from "my presets do nothing" to "my presets work
again". Each step links to the guide that covers it in full; read those when
something does not match your setup.

You need a machine that is always on (a Raspberry Pi, NAS, or home server), your
speakers on the same network, and roughly half an hour for the first speaker.

Developers looking for the Go client library want
[Quick start: the Go client library](GO-CLIENT-QUICKSTART.md) instead.

## Step 1: Run the service

`soundtouch-service` is the replacement for Bose's cloud. Run it wherever you
like, as long as your speakers can reach it: a Docker container on a home
server, a binary on a Raspberry Pi, or, if you have no always-on machine, on
one of the speakers themselves.

- [Which deployment is right for me?](DEPLOYMENT-OVERVIEW.md) compares the
  options in a paragraph each.
- [Migration Guide, Step 1](MIGRATION-GUIDE.md#step-1-install-and-start-the-service)
  has the exact commands for binaries, the install script, and Docker Compose.
- [Raspberry Pi](RASPBERRY-PI.md) and
  [On-Device Install](ON-DEVICE-INSTALL-WALKTHROUGH.md) are the two most common
  setups.

Whichever you pick, the service listens on **port 8000**. Open
`http://<your-server>:8000` and you should see the AfterTouch web UI. Use the
address your speakers will use too, not `localhost`.

## Step 2: Point your speaker at it

Your speaker still asks Bose's servers for its presets. Migration rewrites the
speaker's configuration so it asks yours instead.

Open the web UI, go to **Devices**, find your speaker, and use **Migrate**. For
most models this runs over the speaker's own diagnostic port, so there is
nothing to open up and no hardware to modify. Some models need SSH enabled from
a USB drive first, which the guide walks through.

- [Migration Guide](MIGRATION-GUIDE.md) is the full process, including what to
  do per model.
- [Migration and Safety](MIGRATION-SAFETY.md) covers rolling back, and is worth
  reading before you start if you are nervous.
- [Model Support Matrix](../reference/MODEL-SUPPORT-MATRIX.md) says what is
  known to work where.

After the speaker reboots, it should appear in the web UI as paired.

## Step 3: Check it worked

In the web UI, open your speaker. You should see what it is playing, volume and
bass controls, its sources, and six preset tiles. Press play on something: the
speaker responds within a second or two.

If the speaker does not appear, or a source is missing, start with
[Troubleshooting](TROUBLESHOOTING.md). The most common causes are the service
being unreachable at the address the speaker was given, and a speaker that has
not been rebooted since migration.

## Step 4: Make the preset buttons work again

This is the part most people came for. In the web UI you can browse internet
radio, search for a station, play it, and save it to one of the six slots with
the star button. Those are the same six buttons on top of the speaker.

[Get your preset buttons working again](PRESETS.md) walks through it, including
saving music from a NAS or USB drive, and the command-line equivalent if you
would rather script your setup.

## Where to go next

- [Connecting Music Services](MUSIC-SERVICES.md) for Spotify and the other
  services.
- [Play from a NAS or USB drive](dlna-music-library.md) for a local music
  library.
- [Survival Guide](SURVIVAL-GUIDE.md) for what the shutdown broke, what still
  works without AfterTouch, and what AfterTouch restores.
- [CLI Reference](CLI-REFERENCE.md) if you prefer the command line.

Something not working, or a step that did not match what you saw? Please open
an [issue](https://github.com/gesellix/Bose-SoundTouch/issues) or ask in
[Discussions](https://github.com/gesellix/Bose-SoundTouch/discussions). Reports
from setups we cannot test here are how this gets better.
