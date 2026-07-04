---
title: "Cloud Deployment Walkthrough"
---
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

## Client IP behind a proxy or load balancer

Behind a reverse proxy or load balancer, the connection AfterTouch sees comes
from the proxy, not from the speaker. A few handlers act on the source IP (for
example the Spotify priming triggered by `/marge/streaming/support/power_on`,
and the device IP AfterTouch records), so in a proxied setup you usually want
it to recover the real speaker IP from the `X-Forwarded-For` header.

Enable it in `data/settings.json`:

- Set `"trust_forwarded_headers": true`.
- Set `"trusted_proxy_cidrs"` to your proxy's own source IP range(s) **as
  AfterTouch sees them**, for example `["10.0.0.0/8"]`. It defaults to loopback
  (`127.0.0.0/8`, `::1/128`), which already covers a proxy on the same host.
  When the proxy runs in a separate Docker container, the address AfterTouch
  sees is usually the Docker bridge gateway/subnet (e.g. `172.16.0.0/12`), not
  the proxy's published address.

Make sure the proxy sets the header (nginx:
`proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`). Only
`X-Forwarded-For` is consulted (not `X-Real-IP` or `True-Client-IP`).

The trust decision is made on the **immediate TCP connection**: AfterTouch
reads `X-Forwarded-For` only when the connecting socket's own source IP is in
`trusted_proxy_cidrs`. That socket address is the real connection, so an
`X-Forwarded-For` header cannot forge it.

| Deployment                                                          | `trust_forwarded_headers` | Client IP AfterTouch uses                                                              |
|---------------------------------------------------------------------|---------------------------|----------------------------------------------------------------------------------------|
| Direct LAN / on-device (no proxy)                                   | `false` (default)         | the connecting socket's IP; `X-Forwarded-For` is ignored                               |
| Behind a proxy whose socket IP is in `trusted_proxy_cidrs`          | `true`                    | the rightmost `X-Forwarded-For` entry outside `trusted_proxy_cidrs` (the real speaker) |
| A direct connection whose socket IP is not in `trusted_proxy_cidrs` | `true`                    | the socket IP; `X-Forwarded-For` is ignored (spoofing protection)                      |

> **Do not enable `trust_forwarded_headers` on a flat LAN with no proxy.** A
> malicious speaker could then send `X-Forwarded-For` itself and spoof its
> source IP. A missing or unparseable header always falls back to the socket IP.

For terminating TLS at the proxy (serving the certificate on `:443`), see the
[reverse proxy section of the HTTPS guide](HTTPS-SETUP.md#reverse-proxy-optional).

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
[Downloads page](../downloads/_index.md).

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
AfterTouch serves via `/streaming/account/<id>/full`. If no `Sources.xml`
exists yet for the device on the server (which happens when the device
first checks in after service startup), AfterTouch never writes the default
sources and TuneIn is never registered.

> **Note:** "Data Sync" from the AfterTouch web UI will not work here — it
> requires AfterTouch to reach the speaker outbound, which is not possible
> from a cloud server. The fix is entirely server-side.

**Check whether TuneIn is missing** (run from your local machine):

```bash
curl -s http://192.0.2.1:8090/sources
```

If the output does not contain `source="TUNEIN"`, fix it in three steps:

### 1. Create `Sources.xml` on the server

Find the device's data directory on your server. With Docker it is inside
the volume, e.g. `/app/data/accounts/<accountID>/devices/<deviceID>/`.
Create `Sources.xml` there with the default source set:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<sources>
  <source id="10001" displayName="AUX IN" secret="" secretType="" type="Audio"
          createdOn="2015-03-11T19:12:38.000+00:00"
          updatedOn="2015-03-11T19:12:38.000+00:00">
    <sourceKey type="AUX" account="AUX"/>
  </source>
  <source id="10003" secret="" secretType="token" type="Audio"
          sourceproviderid="11"
          createdOn="2019-01-24T08:18:37.000+00:00"
          updatedOn="2019-02-03T18:35:45.000+00:00">
    <sourceKey type="LOCAL_INTERNET_RADIO" account=""/>
  </source>
  <source id="10004" secret="" secretType="token" type="Audio"
          sourceproviderid="25"
          createdOn="2017-07-20T16:43:48.000+00:00"
          updatedOn="2017-07-20T16:43:48.000+00:00">
    <sourceKey type="TUNEIN" account=""/>
  </source>
  <source id="10005" secret="" secretType="token" type="Audio"
          sourceproviderid="39"
          createdOn="2026-02-16T01:01:01.000+00:00"
          updatedOn="2026-02-16T01:01:01.000+00:00">
    <sourceKey type="RADIO_BROWSER" account=""/>
  </source>
</sources>
```

The empty `secret=""` fields are intentional — AfterTouch generates TuneIn
tokens on the fly from `pkg/service/marge/marge.go`.

With Docker you can copy the file in:

```bash
docker cp Sources.xml aftertouch:/app/data/accounts/<accountID>/devices/<deviceID>/Sources.xml
```

### 2. Notify the speaker (from your local machine)

Send a `sourcesUpdated` push to the speaker so it re-fetches `/full`:

```bash
curl -X POST http://192.0.2.1:8090/notification \
  -H "Content-Type: application/xml" \
  -d '<updates deviceID="<deviceID>"><sourcesUpdated/></updates>'
```

Replace `<deviceID>` with the speaker's device ID (visible in the AfterTouch
Devices tab or in the `Sources.xml` path above).

### 3. Power-cycle the speaker

This is the load-bearing step. The firmware only activates new source *types*
at boot — the runtime sync writes on-device state but does not complete
activation. The CLI `setup reboot` command is not sufficient; physically
unplug the speaker, wait 10 seconds, and plug it back in.

After the reboot, re-check:

```bash
curl -s http://192.0.2.1:8090/sources
```

`TUNEIN` should now appear as `READY`.

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
