Endpoints observed in `bose-pairing-20260502-155542`, `bose-pairing-20260502-165549` mitm captures
and `20260502-170853-2882944` service interaction recording that are either served by external hosts
not redirected to soundtouch-service, or are third-party analytics calls that the service does not
need to handle.

## Redirected Bose hosts — implemented ✅

These hosts are now in the DNS redirect list and the service handles them natively.

| host                | request                                                              | handler                         | notes                                                                                                        |
|---------------------|----------------------------------------------------------------------|---------------------------------|--------------------------------------------------------------------------------------------------------------|
| streaming.bose.com  | `POST /streaming/music/musicprovider/{providerID}/trial/is_eligible` | `HandleMusicProviderIsEligible` | reuses same handler as `/is_eligible`; response: `<eligibility><isEligible>false</isEligible></eligibility>` |
| content.api.bose.io | `POST /bmx/tunein/v1/favorite/{stationID}`                           | `HandleTuneInFavorite`          | returns 202 Accepted `{}`; row 0247 in interactions_20260328-103522-477978.md                                |
| downloads.bose.com  | `GET /ced/soundtouch/mr4_22097fe2/index.xml`                         | `HandleCedStatic`               | firmware index XML; static file embedded from `static/ced/`                                                  |
| downloads.bose.com  | `GET /ced/soundtouch/mr4_22097fe2/relnotes/releasenotes_en.xml`      | `HandleCedStatic`               | firmware release notes; static file embedded                                                                 |
| downloads.bose.com  | `GET /ced/soundtouch/soundtouch_app_help/en/*.xml`                   | `HandleCedStatic`               | 10 in-app help XMLs embedded (gabbo_*, name_device, welcome, etc.)                                           |
| media.bose.io       | `GET /bmx-icons/tunein/monochromeSvg.svg`                            | `HandleBmxIcons`                | already in `static/media/bmx-icons/tunein/`                                                                  |
| media.bose.io       | `GET /bmx-icons/tunein/smallSvg.svg`                                 | `HandleBmxIcons`                | already in `static/media/bmx-icons/tunein/`                                                                  |
| media.bose.io       | `GET /bmx-icons/tunein/top-menu/*.png`                               | `HandleBmxIcons`                | 6 PNGs embedded (bubble, location, microphone, news, note, podcasts); speaker.png was 403 from CDN           |

## Stub implemented — requires AWS IoT integration to complete

| host              | request                   | handler                  | notes                                                                                                                                              |
|-------------------|---------------------------|--------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| voice.api.bose.io | `POST /alexa/certificate` | `HandleAlexaCertificate` | Returns 501; logs device MAC. Full impl needs AWS IoT: parse CSR + token, call RegisterThing/CreateKeysAndCertificate, return cert + iot_endpoint. |

## Third-party analytics (no action needed)

These are calls to third-party analytics services made by the Bose app itself.
soundtouch-service does not handle or intercept them.

| host                  | request                      | notes                                                              |
|-----------------------|------------------------------|--------------------------------------------------------------------|
| api.segment.io        | `POST /v1/i`                 | Segment analytics — identify event                                 |
| api.segment.io        | `POST /v1/p`                 | Segment analytics — page event                                     |
| api.segment.io        | `POST /v1/t`                 | Segment analytics — track event                                    |
| events.api.bosecm.com | `POST /v1/stapp/{accountId}` | Bose app telemetry; handled by service at `/v1/stapp/{deviceId}` ✅ |

## TuneIn / Spotify CDN (no action needed)

Image CDNs used by TuneIn browse and Spotify artwork — not Bose infrastructure.

| host                           | request pattern                   |
|--------------------------------|-----------------------------------|
| cdn-profiles.tunein.com        | `GET /{stationId}/images/logog.*` |
| cdn-albums.tunein.com          | `GET /gn/*.jpg`                   |
| cdn-radiotime-logos.tunein.com | `GET /s*.png`                     |
| i.scdn.co                      | `GET /image/{hash}`               |
