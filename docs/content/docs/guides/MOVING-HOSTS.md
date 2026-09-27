---
title: "Moving AfterTouch to Another Host"
---
This guide walks through moving `soundtouch-service` from one machine to
another, for example a Raspberry Pi that's dying, a NAS you're retiring, or a
switch from a Pi installer to Docker. Done in the right order, your speakers
don't need to notice the move at all.

For the initial migration from Bose's cloud, see the
[Migration Guide](MIGRATION-GUIDE.md). This guide assumes AfterTouch is
already running somewhere and your speakers are already migrated to it.

---

## Before you start

- **Stop the service on the old host first**, so nothing changes on disk
  while you copy.
- **Copy the whole data directory**, not just a config file. It holds
  everything AfterTouch knows: your speakers, presets, recents, sources,
  `settings.json`, `catalog.json`, and a `certs/` folder with the certificate
  authority your speakers were given during migration.
- **Keep the address the speakers know, if you can.** That's the difference
  between "nothing to do on the speakers" and "re-migrate every speaker".

The rest of this guide expands on each of those.

---

## Step 1: Stop the service on the old host

- **Raspberry Pi installer**: `sudo systemctl stop soundtouch-service`
- **Docker**: `docker compose stop soundtouch-service` (or `docker stop
  soundtouch-service` for a plain `docker run` container)
- **On-device install**: `/etc/init.d/aftertouch stop`
- **Plain binary**: stop the process (Ctrl-C, or however you're supervising it)

Stopping it isn't strictly required for the copy to succeed, but it avoids
copying a datastore file mid-write.

---

## Step 2: Copy the whole data directory

The data directory is `DATA_DIR`. Its default location depends on how you
installed AfterTouch:

| Install method            | Default data directory                    |
|----------------------------|--------------------------------------------|
| Raspberry Pi installer     | `/var/lib/soundtouch-service`               |
| Docker Compose / `docker run` | `/app/data` inside the container (a named volume by default, or a bind mount if you set one) |
| On-device install          | `/mnt/nv/aftertouch/data`                   |
| Plain binary                | `data/` next to the binary (relative to wherever you launched it), unless you passed `--data-dir` / `DATA_DIR` |

Check your own env file or compose file if you overrode `DATA_DIR` at
install time: the table above is the out-of-the-box default.

Inside, it holds:

- The **datastore**: your speakers, presets, recently played items,
  sources, `settings.json` (Target Domain and other UI settings), and
  `catalog.json` (the preset catalog).
- **`certs/`**: `ca.crt` and `ca.key` (the certificate authority your
  speakers were given during migration) and `server.crt`/`server.key` (the
  leaf certificate the service presents, signed by that CA).

Copy the whole directory to the new host, preserving structure:

```bash
# Example: over SSH from the old host to the new one
rsync -a /var/lib/soundtouch-service/ newhost:/var/lib/soundtouch-service/
```

For a **Docker named volume**, copy it via a throwaway container rather than
reaching into Docker's internal storage directly:

```bash
docker run --rm \
  -v soundtouch-data:/from \
  -v /path/to/backup:/to \
  alpine sh -c "cp -a /from/. /to/"
# ... move /path/to/backup to the new host, then on the new host:
docker run --rm \
  -v soundtouch-data:/to \
  -v /path/to/backup:/from \
  alpine sh -c "cp -a /from/. /to/"
```

If you use a host bind mount instead (`./data:/app/data`), just copy that
directory like any other.

**Ownership**: the Raspberry Pi installer runs the service as the
`soundtouch` system user and expects the data directory to be owned by
`soundtouch:soundtouch`. After copying onto a fresh Pi install, fix
ownership before starting the service:

```bash
sudo chown -R soundtouch:soundtouch /var/lib/soundtouch-service
```

The default Docker image still runs as `root`, so a bind-mounted directory
needs no ownership change today.

### Why the whole directory, and not just the config

Without `certs/`, the new host generates a brand-new certificate authority
the moment it starts (`EnsureCA` only creates one if `ca.crt`/`ca.key` are
missing). Your speakers still trust the *old* CA, not this new one, so HTTPS
to the new host fails until they're migrated again. Bring `certs/` along and
this doesn't happen: the new host keeps using the CA your speakers already
trust, and it can still generate a fresh leaf certificate under that same CA
if the address changes (see the next step).

---

## Step 3: Keep the address the speakers know, if you can

If `SERVER_URL` / **Target Domain** is a hostname (for example a name your
router hands out, or an entry in your own DNS), move that name to point at
the new host instead of the old one. Once you copied `certs/` in Step 2,
this is enough on its own:

- The service automatically issues a new leaf certificate for that hostname
  under the CA you copied (the existing cert only gets regenerated when it
  doesn't already cover the requested address).
- Every already-migrated speaker keeps using the exact same address and
  keeps trusting the same CA, so no re-migration is needed.

This is by far the easiest way to move hosts. It works whether the name
comes from your router's DHCP reservations, a static `/etc/hosts` entry, or
your own DNS server.

---

## Step 4: If the address changes

If the new host's address (hostname or IP) is different from the old one,
each speaker has to be told about it explicitly:

1. Set `SERVER_URL` (or **Settings → Target Domain**) on the new host to
   its new address.
2. For each speaker, open the **Migrate** tab in the AfterTouch admin UI and
   re-run migration (or use `soundtouch-cli setup migrate`, see the
   [CLI reference](CLI-REFERENCE.md)).
3. Restart each speaker (power-cycle it) once migration completes.

Since you copied `certs/` in Step 2, the CA your speakers already trust is
the same one the new host is using, so the certificate side of migration
should show as already trusted: you're only re-pointing the address, not
re-installing a new root of trust. If you skipped copying `certs/`, the
migration wizard's CA check compares the actual certificate, not just a
label, so it will correctly detect the mismatch and reinstall the CA as
part of this same re-migration.

Settings alone never reaches an already-migrated speaker; see
[Changing Target Domain doesn't change what a speaker actually
uses](TROUBLESHOOTING.md#settings-vs-migrate). The re-migrate step above is
required, not optional, whenever the address changes.

---

## Play URL radio presets and an address change {#the-catch-play-url-radio-presets-store-the-old-address}

Radio presets created with the player's **Play URL** feature, or with
`soundtouch-cli preset`, used to store the full AfterTouch address inside
the preset itself, for example:

```
http://<old-host>:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=...
```

After an address change those presets still point at the old host and
don't play.

This is fixed in the next release
([issue 769](https://github.com/gesellix/Bose-SoundTouch/issues/769)): new
Play URL presets store a relative location, `/station?data=...`, and the
speaker adds AfterTouch's address from its service registry, the same way
it does for **TuneIn** and **Radio Browser** presets. They follow AfterTouch
to its new address once the speaker has been re-migrated (Step 4).

Presets saved by an older version still carry the old address. The
**Health** tab lists them per speaker ("Internet Radio presets use relative
station locations") and offers **Store as relative locations**, which
stores the affected slots on the speaker again with the relative location,
keeping their names, artwork and slots. Run it on the new host after the
move (the old host doesn't need to be reachable), or on the old host before
you move. It needs AfterTouch to reach the speaker; if it can't, the finding
also lists a `curl` command per slot to run from a machine that can. See
[Play URL radio presets stop playing after moving AfterTouch](TROUBLESHOOTING.md#play-url-presets-after-move).

NAS/DLNA (stored-music) playback doesn't go through AfterTouch at all, so
it's unaffected by any of this.

---

## Moving between install types

Because every install method stores the same thing in its data directory,
switching install types (for example, Raspberry Pi installer to Docker) is
the same procedure as moving hosts: copy the data directory's contents into
whatever location the new install method expects (see the table in Step 2),
set `SERVER_URL`/`DATA_DIR` to match, and start the service there instead.
Watch the two install-specific details above: ownership (`soundtouch:soundtouch`
for the Pi installer) and the data-directory path (each method defaults to a
different one).

---

## What to check afterward

- **Health tab** in the AfterTouch admin UI: confirm it's green and run any
  QuickFixes it suggests.
- **A TuneIn station**: play one to confirm radio sources still resolve.
- **A Play URL preset**: if the address changed and the Health tab lists it
  with an old address, run its **Store as relative locations** fix (see
  above), then play it.

---

## Rolling back

If something isn't working on the new host, you can go back:

- **If you kept the same address** (moved a hostname, or haven't re-migrated
  any speaker yet): just start the service on the old host again. Nothing
  on the speakers changed, so they resume talking to it immediately.
- **If you already re-migrated speakers to the new address**: either
  re-migrate them back to the old host's address, or point the address back
  at the old host and re-migrate again. Once a speaker has been migrated to
  a specific address, only another migration changes that.
