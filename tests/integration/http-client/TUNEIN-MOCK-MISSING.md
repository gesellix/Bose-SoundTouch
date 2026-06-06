# TuneIn mock: missing upstream fixtures

The `mock-tunein` service (`cmd/mock-tunein`, `pkg/testutils/tunein`) lets the
integration suite exercise the BMX TuneIn endpoints without hitting the live
TuneIn (radiotime.com) service. The service is pointed at it in CI via
`TUNEIN_OPML_URL` / `TUNEIN_API_URL` (see `docker-compose.ci.yml`).

## Mocked today (hermetic)

- `GET /Tune.ashx?id=...&formats=...` (station stream resolution)
- `GET /describe.ashx?id=...` (station name + logo)

These back `GET /bmx/tunein/v1/playback/station/{stationID}`
(`tunein_playback_station.http`), which is therefore fully offline.

Local-only BMX routes (no upstream at all, already hermetic):
`/bmx/tunein/v1/favorite/{id}` (POST/DELETE), `/bmx/tunein/v1/token`,
`/bmx/tunein/v1/report`.

## NOT mocked yet — need real upstream captures

The following BMX TuneIn routes call `api.radiotime.com` / `opml.radiotime.com`
endpoints we do not have recorded upstream responses for. The mock returns 404
for these (so a test that needs them fails loudly). To cover them we need to
capture the **raw radiotime responses** (service -> TuneIn), not just the final
BMX responses the speaker received:

| BMX route (speaker-facing) | Upstream call the service makes | Needed capture |
|----------------------------|----------------------------------|----------------|
| `GET /bmx/tunein/v1/playback/episode/{id}` | `GET api.radiotime.com/profiles/{p<N>}/contents?version=1.3` then `Tune.ashx?id=<episode>` | `profiles/<id>/contents` JSON |
| `GET /bmx/tunein/v1/playback/episodes/{id}` | same profile-contents JSON | `profiles/<id>/contents` JSON |
| `GET /bmx/tunein/v1/navigate` | `GET opml.radiotime.com/?render=json` (and `Browse.ashx`) | OPML-as-JSON browse pages |
| `GET /bmx/tunein/v1/search` | `GET api.radiotime.com/profiles?fulltextsearch=true&version=1.3&query=...` | search-results JSON |
| `GET /bmx/tunein/v1/search/next` | opaque cursor URL from a prior search | search next-page JSON |

### How to collect

Run the service against live TuneIn with `RECORD_INTERACTIONS` on and the
proxy/recorder capturing the **upstream** category, drive the speaker (or the
`/api/tunein/*` UI) through podcast playback / browse / search, then copy the
sanitized radiotime responses here. Once captured, extend
`pkg/testutils/tunein/handlers.go` with the matching `/profiles*`, `/?render=json`
handlers and add the corresponding `.http` flows.

Keep captured fixtures sanitized (no real account ids/tokens); the radiotime
station/program ids themselves are public catalog ids and are fine to keep.
