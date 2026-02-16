## radio-browser.info

- https://www.radio-browser.info is a community driven radio station database.
- It provides an API to access the data and allows users to submit new stations or update existing ones.

### Search for stations

- Go to https://www.radio-browser.info and find a station you like.
- Click on the station and copy the UUID from the URL.
- e.g. `https://www.radio-browser.info/history/d28420a4-eccf-47a2-ace1-088c7e7cb7e0`

### RADIO_BROWSER

- This project supports source type RADIO_BROWSER to play radio stations.
- Set the `location` attribute to `/stations/byuuid/{UUID}`.

```xml
<ContentItem
  source="RADIO_BROWSER"
  type="stationurl"
  isPresetable="true"
  location="/stations/byuuid/9610c454-0601-11e8-ae97-52543be04c81">
  <itemName>RADIO_BROWSER</itemName>
  <containerArt></containerArt>
</ContentItem>
```

### Playing the station

To start the radio stream replace `<uuid>` and `<soundtouch>` and run curl like this:

```bash
curl -d '<ContentItem source="RADIO_BROWSER" type="stationurl" location="/stations/byuuid/<uuid>"/>' <soundtouch>:8090/select
```
