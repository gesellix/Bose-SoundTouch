---
title: "Radio Browser"
---

## radio-browser.info

- https://www.radio-browser.info is a community driven radio station database.
- It provides an API to access the data and allows users to submit new stations or update existing ones.
- RadioBrowser provides a native SoundTouch-compatible API at `https://all.api.radio-browser.info/soundtouch`.

### Architecture

This service registers RadioBrowser in its BMX service registry (provider ID 39) pointing to RadioBrowser's SoundTouch API. The device discovers it from there and communicates directly with RadioBrowser for browsing and playback — the local service does not proxy streams.

The source is registered with type `RADIO_BROWSER` in the Marge sources list. The RadioBrowser provider ID (39) in the source entry identifies it as RadioBrowser within the BMX layer.

The device's own `Sources.xml` (`/mnt/nv/BoseApp-Persistence/1/Sources.xml`) must contain a `RADIO_BROWSER` entry for playback to work:

```xml
<source secret="" secretType="">
    <sourceKey type="RADIO_BROWSER" account="" />
</source>
```

**A device reboot is required after adding this entry.** The firmware only registers `RADIO_BROWSER` as a selectable source type during the boot-time `Sources.xml` load. The Marge runtime sync stores the source in the registry but does not complete the activation — without a reboot, selecting a `RADIO_BROWSER` station results in `INVALID_SOURCE`.

A reboot achieves two things in sequence:

1. The speaker fetches all sources from the soundtouch-service via a `/full` request, which updates the device-local `Sources.xml`.
2. The firmware initialises and registers the `RADIO_BROWSER` source type from that updated file.

The `INVALID_SOURCE_TYPE` message from the Bluetooth daemon visible in device logs (e.g. during `GET /serviceAvailability`) is informational noise and does not affect playback.

### Triggering a sources refresh without rebooting

Step 1 above (the `/full` fetch that updates `Sources.xml`) can be triggered independently by posting a `sourcesUpdated` notification directly to the speaker. This is useful for verifying that the soundtouch-service serves the correct sources list before committing to a full reboot:

```bash
curl -v -X POST http://<speaker-ip>:8090/notification \
     -H "Content-Type: application/xml" \
     -d '<updates deviceID="<deviceID>"><sourcesUpdated/></updates>'
```

Replace `<speaker-ip>` with your speaker's IP address and `<deviceID>` with its device ID (visible in `/info`). After this call the speaker re-fetches its sources from the service. Step 2 (source-type registration) still requires a reboot.

> **If a reboot doesn't help after an in-place migration:** on some speakers the radio source types (`LOCAL_INTERNET_RADIO`, `TUNEIN`, `RADIO_BROWSER`) never activate after an in-place migration, even though the entries are present in the device-local `Sources.xml` and you have rebooted and sent `sourcesUpdated`. The cause isn't fully understood; the confirmed remedy is a factory reset + re-migrate. See [Troubleshooting: Radio sources never activate after an in-place migration](../../guides/TROUBLESHOOTING/#radio-sources-after-migration).

### Search for stations

- Go to https://www.radio-browser.info and find a station you like.
- Click on the station and copy the UUID from the URL.
- e.g. `https://www.radio-browser.info/history/d28420a4-eccf-47a2-ace1-088c7e7cb7e0`

### Playing the station

```xml
<ContentItem
  source="RADIO_BROWSER"
  type="stationurl"
  isPresetable="true"
  location="/stations/byuuid/9610c454-0601-11e8-ae97-52543be04c81">
  <itemName>Radio Station Name</itemName>
  <containerArt></containerArt>
</ContentItem>
```

To start the radio stream replace `<uuid>` and `<soundtouch>` and run curl like this:

```bash
curl -d '<ContentItem source="RADIO_BROWSER" type="stationurl" location="/stations/byuuid/<uuid>"/>' <soundtouch>:8090/select
```

### BMX service registry entry

The entry in `pkg/service/handlers/static/bmx_services.json` that enables RadioBrowser:

```json
{
  "baseUrl": "https://all.api.radio-browser.info/soundtouch",
  "id": {
    "name": "RADIO_BROWSER",
    "value": 39
  },
  "streamTypes": ["liveRadio", "onDemand"],
  "authenticationModel": {
    "anonymousAccount": {
      "autoCreate": true,
      "enabled": true
    }
  }
}
```
