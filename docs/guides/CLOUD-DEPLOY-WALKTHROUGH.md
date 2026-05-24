# Cloud Deployment Walkthrough

A step-by-step guide to running AfterTouch on a VPS or cloud server and
pointing your local Bose SoundTouch speakers at it.

This scenario is useful when you don't have an always-on machine at home but
you already have a cloud server (Hetzner, DigitalOcean, Coolify, etc.).

For local deployments (Raspberry Pi, NAS, home server) see
[EXTERNAL-HOST-WALKTHROUGH.md](EXTERNAL-HOST-WALKTHROUGH.md).
For a comparison of all deployment options see
[DEPLOYMENT-OVERVIEW.md](DEPLOYMENT-OVERVIEW.md).

---

## How cloud deployment differs from a local host

|                   | Local host                                          | Cloud host                                               |
|-------------------|-----------------------------------------------------|----------------------------------------------------------|
| Speaker discovery | Automatic (mDNS on the LAN)                         | **Disabled** — cloud can't reach your LAN                |
| Speaker migration | Via the AfterTouch web UI                           | **Via `soundtouch-cli` on your local machine**           |
| URL scheme        | `http://` is fine on a LAN                          | **HTTPS required** — speakers validate the certificate   |
| Audio routing     | Stays on the LAN (AfterTouch only proxies metadata) | Stays on the LAN — audio never transits the cloud server |

> **Audio does not flow through AfterTouch.** The cloud server only handles
> authentication tokens and URL discovery. Music data goes directly between
> your LAN and the streaming provider.

---

## Security warning

The Marge API (the Bose-protocol endpoints on `/streaming/*`) has no
built-in authentication beyond account/device IDs. When AfterTouch is
internet-facing, those endpoints are reachable by anyone who knows the URL.

Minimum mitigations before going live:

- Enable **HTTP Basic Auth** on the management UI (set via `MGMT_USERNAME` /
  `MGMT_PASSWORD` or the `--mgmt-username` / `--mgmt-password` flags).
- Run AfterTouch **behind a reverse proxy** (Nginx, Caddy, Coolify, Traefik)
  and consider blocking the `/streaming/*` paths to all but your speaker's
  IP address at the proxy level if your server/firewall allows it.
- Do not expose the Docker socket or the data volume to untrusted processes.

---

## Step 1 — Deploy AfterTouch on your server

### Docker / Docker Compose (any VPS)

```yaml
# docker-compose.yml
services:
  aftertouch:
    image: ghcr.io/gesellix/bose-soundtouch:latest
    restart: unless-stopped
    ports:
      - "8000:8000"
      - "8443:8443"
    environment:
      SERVER_URL: "https://soundtouch.example.com"
      HTTPS_SERVER_URL: "https://soundtouch.example.com"
      DISCOVERY_ENABLED: "false"         # speakers aren't on the same network
      MGMT_USERNAME: "admin"
      MGMT_PASSWORD: "change_me!"        # change this
    volumes:
      - aftertouch-data:/app/data

volumes:
  aftertouch-data:
```

> **`DISCOVERY_ENABLED: "false"` is important.** Without it, AfterTouch
> tries mDNS/UPnP discovery every 5 minutes and logs timeouts, since no
> speakers are reachable from the cloud.

Replace `soundtouch.example.com` with your own domain. Run:

```bash
docker compose up -d
```

### Coolify

wimdeblauwe's working Coolify configuration (from
[discussion #295](https://github.com/gesellix/Bose-SoundTouch/discussions/295)):

- **Docker image:** `ghcr.io/gesellix/bose-soundtouch`
- **Tag:** `latest`
- **Custom docker options:** `--env SERVER_URL=https://soundtouch.example.com`
- **Port exposes:** `8000,8443`
- **Basic auth:** enabled in Coolify's settings
- **Persistent storage volume:** source `/opt/coolifydata/soundtouchdata` →
  destination `/app/data`

Coolify handles HTTPS and certificate renewal automatically via Let's Encrypt.

---

## Step 2 — Verify the service is reachable

```bash
curl https://soundtouch.example.com/health
```

You should get a JSON response with `"status":"ok"` and a version string.
If you get a certificate error, the HTTPS setup is not complete — fix that
before proceeding, because the speaker will reject an untrusted certificate.

---

## Step 3 — Migrate the speaker from your local machine

Because AfterTouch is in the cloud and cannot reach your speaker, the
migration must be driven from `soundtouch-cli` **running on your own machine
on the same LAN as the speaker**.

Download `soundtouch-cli` for your OS from the
[Releases page](https://github.com/gesellix/Bose-SoundTouch/releases).

### Check the migration plan first

```bash
soundtouch-cli --host 192.0.2.1 setup plan \
  --service-url="https://soundtouch.example.com"
```

Replace `192.0.2.1` with your speaker's LAN IP. The plan output shows what
will change and which migration method will be used.

### Run the migration

```bash
soundtouch-cli --host 192.0.2.1 setup migrate \
  --service-url="https://soundtouch.example.com" \
  --method=telnet
```

If `telnet` is not available on your speaker, omit `--method` to let the
CLI pick the best available method.

### Reboot the speaker

```bash
soundtouch-cli --host 192.0.2.1 setup reboot
```

Wait 2–3 minutes for the speaker to come back up. After the reboot, the
speaker contacts `soundtouch.example.com` instead of the Bose cloud.

---

## Step 4 — Open the AfterTouch Admin UI

Navigate to **`https://soundtouch.example.com`** in a browser. Because you
set `DISCOVERY_ENABLED=false`, the Devices list may be empty — the speaker
registered itself when it connected, but you need to add it manually first.

Add your speaker:
1. Go to the **Devices** tab.
2. Click **Add device manually**.
3. Enter your speaker's LAN IP (`192.0.2.1`) and click **Add**.

The speaker should appear in the list.

---

## Step 5 — Run Health QuickFixes

1. Click your speaker → **Health** tab.
2. Run or refresh the health checks.
3. Apply any QuickFixes shown, especially:

   | Warning                                         | Action                                      |
   |-------------------------------------------------|---------------------------------------------|
   | *Speaker reports an empty `<margeAccountUUID>`* | Click **Pair account** / **Apply QuickFix** |
   | *INTERNET_RADIO source is a stale stub*         | Click **Remove INTERNET_RADIO source**      |

4. Reboot the speaker after any QuickFix that requires it, then re-run health checks.

---

## Step 6 — TuneIn source registration (if TuneIn is missing)

On a freshly factory-reset speaker, TuneIn may not appear in the speaker's
source list — you'll see error `1005` (UNKNOWN_SOURCE_ERROR) when trying to
play a TuneIn station.

**Why this happens:** The speaker materialises its source list from what
AfterTouch serves via `/streaming/account/<id>/full`. If the device's
`Sources.xml` on the server doesn't include a TUNEIN entry, the speaker
never registers TuneIn as a source.

**Check whether TuneIn is missing:**

```bash
curl -s http://192.0.2.1:8090/sources
```

If the XML does not contain `source="TUNEIN"`, run the Health QuickFix for
missing sources (the health check reports it as a warning). If no QuickFix
appears, trigger a data sync from the AfterTouch web UI:

1. Click your speaker → **Data Sync** tab (or **Migration** tab).
2. Click **Sync** to push the default source set to the speaker.
3. Reboot the speaker.
4. Re-check `/sources` — `TUNEIN` should now be present.

After TuneIn appears:

```bash
soundtouch-cli --host 192.0.2.1 source tunein --station s10861 \
  --service-url="https://soundtouch.example.com"
sleep 5
soundtouch-cli --host 192.0.2.1 preset store-current --slot 1
```

---

## Step 7 — Set up preset buttons

### Via soundtouch-cli (from your local machine)

```bash
soundtouch-cli --host 192.0.2.1 source custom-radio \
  --url "https://stream.laut.fm/country-nonstop" \
  --name "Country Nonstop" \
  --service-url "https://soundtouch.example.com"
sleep 5
soundtouch-cli --host 192.0.2.1 preset store-current --slot 1
```

See the [On-Device Install Walkthrough](ON-DEVICE-INSTALL-WALKTHROUGH.md#step-9--store-custom-radio-streams-to-preset-buttons)
for six worked examples with public internet radio streams.

### Via the AfterTouch web UI

Use the **Radio Browser** or **TuneIn** tab in the Admin UI to find a
station, play it, then store it to a preset slot.

---

## What happens if AfterTouch goes offline?

The speaker caches its last-known source list and presets. If AfterTouch
becomes unreachable:

- **Preset buttons** that trigger locally-stored content (custom radio,
  cached TuneIn stations) may still work for a short time.
- **TuneIn / Spotify** require AfterTouch to resolve the token and URL —
  these stop working when the server is unreachable.
- **Audio does not route through AfterTouch** — once a stream URL is
  resolved, the speaker fetches audio directly from the provider.

To minimise downtime, use your cloud provider's restart policy
(`restart: unless-stopped` in Docker Compose, or Coolify's auto-restart).

---

## Troubleshooting

| Symptom                                         | First check                                                                                    |
|-------------------------------------------------|------------------------------------------------------------------------------------------------|
| Speaker shows certificate error                 | HTTPS certificate is not trusted — ensure your reverse proxy serves a valid Let's Encrypt cert |
| Migration fails with "connection refused"       | Speaker can't reach `soundtouch.example.com:443` — check your server's firewall                |
| Source TuneIn 1005 error                        | TuneIn not in speaker's source list — follow Step 6                                            |
| AfterTouch logs "discovery timeout" every 5 min | Set `DISCOVERY_ENABLED=false`                                                                  |
| Devices tab empty after migration               | Add the speaker manually by IP (Step 4)                                                        |
| `margeAccountUUID` still empty after QuickFix   | Re-run Health QuickFix and reboot again                                                        |

For more detail see [TROUBLESHOOTING.md](TROUBLESHOOTING.md) and
[MIGRATION-GUIDE.md](MIGRATION-GUIDE.md).

---

## References

- [discussion #295](https://github.com/gesellix/Bose-SoundTouch/discussions/295) —
  wimdeblauwe's field report deploying AfterTouch on a Hetzner VPS via Coolify;
  covers the TuneIn source registration issue in detail.
