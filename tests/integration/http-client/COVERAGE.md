# Speaker-contract coverage (.http integration suite)

This is the coverage checklist for the JetBrains `.http` integration suite
(`make test-http-client`). Its purpose is the regression net described in
`docs/content/docs/architecture/API-ROUTE-LAYOUT.md` ("Regression safety:
contract tests from the frozen recordings"): pin the **frozen speaker contract**
(category 1a/1b routes) so the staged route refactor for issue #451 cannot
silently change the wire.

## Method

The inventory below was mined (read-only) from real recorded speaker traffic
(`User-Agent: Bose_Lisa/*` / `Bose/*`, `self/` category) across the local
recording corpus (`_/backup/*`, `tests/integration/testdata/interactions/`).
Recordings are used as a **reference for route + request/response shape only**;
no raw recorded bodies (which carry real ids/IPs/MACs/tokens) are committed.
Authored flows use the RFC-5737 / placeholder values from `http-client.env.json`.

TuneIn upstream (radiotime.com) is served by `mock-tunein` in CI (see
`TUNEIN-MOCK-MISSING.md`), so the suite has no live external dependency.

Variable path segments are templated: `{stationID}`, `{episodeID}`, `{hash}`,
`{encodedURL}`, `{provider}/{file}`.

Legend: ✅ covered · ⬜ gap · 〰️ partial (some status/variant uncovered).

**Enforced by:** `TestFrozenRouteContractCoverage`
(`cmd/soundtouch-service/coverage_guard_test.go`). It walks the router for
frozen-contract routes, matches each against the `.http` request lines, and
golden-files the set of *uncovered* frozen routes
(`cmd/soundtouch-service/testdata/frozen_routes_uncovered.txt`). Adding a new
frozen route without a test, or a test that newly covers one, changes that set
and fails the test, so this checklist can't silently drift from the code.

## Frozen speaker routes

| Method      | Route                                              | Status(es) observed | Covered by                                                   | State                                              |
|-------------|----------------------------------------------------|---------------------|--------------------------------------------------------------|----------------------------------------------------|
| GET         | `/streaming/account/{a}/full`                      | 200, 304            | `get_full_account.http`, `get_full_account_conditional.http` | ✅                                                  |
| GET         | `/streaming/account/{a}/devices`                   | 200                 | `get_account_devices.http`                                   | ✅                                                  |
| GET         | `/streaming/account/{a}/sources`                   | 200                 | `get_account_sources.http`                                   | ✅                                                  |
| GET         | `/streaming/account/{a}/presets/all`               | 200                 | `get_account_presets.http`                                   | ✅                                                  |
| GET         | `/streaming/account/{a}/provider_settings`         | 200                 | `get_provider_settings.http`                                 | ✅                                                  |
| POST        | `/streaming/account` (+ `/login`)                  | 201/200             | `create_account.http`                                        | ✅                                                  |
| POST        | `/streaming/account/{a}/device/`                   | 201                 | `register_device.http`                                       | ✅                                                  |
| PUT         | `/streaming/account/{a}/device/{d}`                | 200, 401            | `rename_device.http`                                         | 〰️ (401 gap)                                       |
| DELETE      | `/streaming/account/{a}/device/{d}`                | 200                 | `unregister_device.http`                                     | ✅                                                  |
| GET         | `/streaming/account/{a}/device/{d}/group/`         | 200                 | `get_group.http`                                             | ✅                                                  |
| GET         | `/streaming/account/{a}/device/{d}/presets`        | 200, 304            | `get_presets.http`, `get_presets_conditional.http`           | ✅                                                  |
| PUT         | `/streaming/account/{a}/device/{d}/preset/{n}`     | 200                 | `set_preset_5/6.http`                                        | ✅                                                  |
| DELETE      | `/streaming/account/{a}/device/{d}/preset/{n}`     | 200                 | `delete_preset_6.http`                                       | ✅                                                  |
| POST        | `/streaming/account/{a}/device/{d}/recent`         | 201                 | `post_recent.http`                                           | ✅                                                  |
| GET         | `/streaming/account/{a}/device/{d}/recents`        | 200                 | `get_recents.http`                                           | ✅                                                  |
| POST        | `/streaming/account/{a}/source`                    | 200                 | `set_preset_5.http`                                          | ✅                                                  |
| POST        | `/streaming/account/{a}/group/`                    | 201                 | `create_group.http`                                          | ✅                                                  |
| DELETE      | `/streaming/account/{a}/group/`                    | 200                 | `delete_group.http`                                          | ✅ (account-level teardown)                         |
| DELETE      | `/streaming/account/{a}/group/{id}`                | 200                 | `delete_group.http`                                          | ✅                                                  |
| GET         | `/streaming/device/{d}/streaming_token`            | 200                 | `get_streaming_token.http`                                   | ✅                                                  |
| GET         | `/streaming/software/update/account/{a}`           | 200                 | `get_software_update.http`                                   | ✅                                                  |
| GET         | `/streaming/sourceproviders`                       | 200                 | `get_sourceproviders.http`                                   | ✅                                                  |
| GET         | `/streaming/resources/api_versions.xml`            | 200                 | `get_api_versions.http`                                      | ✅                                                  |
| POST        | `/streaming/support/power_on`                      | 200                 | `power_on.http`                                              | ✅                                                  |
| POST        | `/streaming/support/customersupport`               | 200                 | `customer_support.http`                                      | ✅                                                  |
| POST        | `/streaming/music/musicprovider/{id}/is_eligible`  | 200                 | `post_musicprovider_is_eligible.http`                        | ✅                                                  |
| POST        | `/accounts/{a}/devices`                            | 201                 | `register_device.http`                                       | ✅                                                  |
| DELETE      | `/accounts/{a}/devices/{d}`                        | 200                 | `unregister_device.http`                                     | ✅                                                  |
| GET         | `/updates/soundtouch`                              | 200                 | `get_soundtouch_updates.http`                                | ✅                                                  |
| GET         | `/v1/auth`                                         | 200, 403, 404       | `get_speaker_auth.http`                                      | ✅ (200; 403/404 probe/edge)                        |
| POST        | `/v1/scmudc/{d}`                                   | 200                 | `post_scmudc_event.http`                                     | ✅                                                  |
| GET         | `/v1/blacklist/{d}`                                | 405                 | `get_blacklist.http`                                         | ✅ (currently ignored: 405 stub)                    |
| POST        | `/alexa/certificate`                               | 501 (rare 200)      | `post_alexa_certificate.http`                                | ✅ (currently ignored: 501 stub)                    |
| GET         | `/bmx/registry/v1/services`                        | 200                 | `get_bmx_services.http`                                      | ✅                                                  |
| GET         | `/bmx/registry/v1/servicesAvailability`            | 200                 | `get_bmx_services_availability.http`                         | ✅                                                  |
| POST        | `/bmx/tunein/v1/token`                             | 200                 | `tunein_playback_station.http`                               | ✅                                                  |
| GET         | `/bmx/tunein/v1/playback/station/{stationID}`      | 200, 401            | `tunein_playback_station.http`                               | ✅ (offline via mock-tunein)                        |
| GET         | `/bmx/tunein/v1/playback/episode(s)/{episodeID}`   | 200                 | —                                                            | ⬜ (needs mock fixture, see TUNEIN-MOCK-MISSING.md) |
| POST        | `/bmx/tunein/v1/report`                            | 200                 | `post_tunein_report.http`                                    | ✅                                                  |
| POST/DELETE | `/bmx/tunein/v1/favorite/{stationID}`              | 202                 | `tunein_favorite.http`                                       | ✅ (local-only)                                     |
| GET         | `/core02/svc-bmx-adapter-orion/prod/orion/station` | 200                 | `get_orion_station.http`                                     | ✅                                                  |
| GET         | `/custom/v1/playback/{encodedURL}`                 | 200                 | `get_custom_playback.http`                                   | ✅                                                  |
| GET         | `/media/aftertouch-ding.wav`                       | 200 (binary)        | `get_media_ding.http`                                        | ✅                                                  |
| GET         | `/media/bmx-icons/{provider}/{file}`               | 200 (binary)        | `get_bmx_icon.http`                                          | ✅                                                  |
| GET         | `/media/tts/{hash}.mp3`                            | 200, 404 (binary)   | —                                                            | ⬜ (depends on prior TTS)                           |
| GET         | `/ced/soundtouch/.../index.xml`                    | 200                 | `get_ced_index.http`                                         | ✅ (absent paths 404)                               |
| POST        | `/oauth/device/{d}/.../15/token/cs3`               | 200                 | `post_oauth_token.http`                                      | ✅                                                  |
| POST        | `/oauth/device/{d}/.../20/token/cs1`               | 200                 | `post_oauth_token_amazon.http`                               | ✅                                                  |

## App / provisioning surface (app-called, not the speaker data-plane)

The SoundTouch app (not the speaker) drives these. They are part of the frozen
contract but a different audience; shapes are taken from `_/mitm` where a capture
exists, otherwise from the handler (the current responses are canned / stubs).

| Method | Route                                 | Status | Covered by                   | Source                   |
|--------|---------------------------------------|--------|------------------------------|--------------------------|
| GET    | `/streaming/account/{a}/emailaddress` | 200    | `get_emailaddress.http`      | `_/mitm` capture         |
| GET    | `/customer/account/{a}`               | 200    | `get_customer_profile.http`  | handler (canned profile) |
| POST   | `/customer/account/{a}`               | 200    | `post_customer_profile.http` | handler (stub accept)    |
| POST   | `/customer/account/{a}/password`      | 200    | `post_customer_profile.http` | handler (stub accept)    |
| POST   | `/streaming/account/login`            | 200    | `create_account.http`        | `_/mitm` capture         |

## Not observed (lower priority / no fixture)

- `/bmx/tunein/v1/navigate`, `/search`, `/search/next` — registered (frozen),
  but in the corpus the speaker uses `/playback/*`; the search/navigate layer is
  driven by the app/UI (`/api/tunein/*`), not the speaker. No upstream fixture
  yet, see TUNEIN-MOCK-MISSING.md.
- `/core02/svc-bmx-adapter-siriusxm-*` — registered, but not present in this
  corpus (no SiriusXM device). Left as a known blank.

## Remaining gaps

- `/bmx/tunein/v1/playback/episode(s)/{id}` — needs a captured radiotime
  profile-contents fixture for the mock (see TUNEIN-MOCK-MISSING.md).
- `/media/tts/{hash}.mp3` — returns 200 only after a TTS has been generated
  (otherwise a 404 miss). Needs a prior `/setup/tts/speak` step to be a
  deterministic 200.
- PUT-device `401` (rename with a mismatched/blocked payload) — the only
  remaining status variant on an otherwise-covered route.
