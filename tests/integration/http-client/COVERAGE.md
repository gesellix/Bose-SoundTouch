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

Variable path segments are templated: `{stationID}`, `{episodeID}`, `{hash}`,
`{encodedURL}`, `{provider}/{file}`.

Legend: ✅ covered · ⬜ gap · 〰️ partial (some status/variant uncovered).

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
| GET         | `/v1/auth`                                         | 200, 403, 404       | —                                                            | ⬜ (Go unit: `auth_probe_test.go`)                  |
| POST        | `/v1/scmudc/{d}`                                   | 200                 | —                                                            | ⬜                                                  |
| GET         | `/v1/blacklist/{d}`                                | 405                 | —                                                            | ⬜ (edge)                                           |
| POST        | `/alexa/certificate`                               | 501 (rare 200)      | —                                                            | ⬜ (edge)                                           |
| GET         | `/bmx/registry/v1/services`                        | 200                 | `get_bmx_services.http`                                      | ✅                                                  |
| POST        | `/bmx/tunein/v1/token`                             | 200                 | `tunein_playback_station.http`                               | ✅                                                  |
| GET         | `/bmx/tunein/v1/playback/station/{stationID}`      | 200, 401            | `tunein_playback_station.http`                               | ✅ (offline via mock-tunein)                        |
| GET         | `/bmx/tunein/v1/playback/episode(s)/{episodeID}`   | 200                 | —                                                            | ⬜ (needs mock fixture, see TUNEIN-MOCK-MISSING.md) |
| POST        | `/bmx/tunein/v1/report`                            | 200                 | `post_tunein_report.http`                                    | ✅                                                  |
| POST/DELETE | `/bmx/tunein/v1/favorite/{stationID}`              | 202                 | `tunein_favorite.http`                                       | ✅ (local-only)                                     |
| GET         | `/core02/svc-bmx-adapter-orion/prod/orion/station` | 200                 | —                                                            | ⬜                                                  |
| GET         | `/custom/v1/playback/{encodedURL}`                 | 200                 | —                                                            | ⬜                                                  |
| GET         | `/media/aftertouch-ding.wav`                       | 200 (binary)        | —                                                            | ⬜                                                  |
| GET         | `/media/bmx-icons/{provider}/{file}`               | 200 (binary)        | —                                                            | ⬜                                                  |
| GET         | `/media/tts/{hash}.mp3`                            | 200, 404 (binary)   | —                                                            | ⬜ (depends on prior TTS)                           |
| GET         | `/ced/utilities/audio/take5.mp3`                   | 404                 | —                                                            | ⬜ (edge: CED static miss)                          |
| POST        | `/oauth/device/{d}/.../15/token/cs3`               | 200                 | `post_oauth_token.http`                                      | ✅                                                  |
| POST        | `/oauth/device/{d}/.../20/token/cs1`               | 200                 | `post_oauth_token_amazon.http`                               | ✅                                                  |

## Not observed from the speaker (lower priority / different audience)

- `/bmx/tunein/v1/navigate`, `/search`, `/search/next` — registered (frozen),
  but in the corpus the speaker uses `/playback/*`; the search/navigate layer is
  driven by the app/UI (`/api/tunein/*`), not the speaker. Covered conceptually,
  no speaker recording to replay.
- `/core02/svc-bmx-adapter-siriusxm-*` — registered, but not present in this
  corpus (no SiriusXM device). Left as a known blank.
- App / provisioning surface (`/customer/account*`, account profile/password,
  `/streaming/account/login` beyond create) — app-called, not the speaker
  data-plane; out of scope for the speaker-contract net.

## Gap-fill priority

1. **High (pure service, no external dep):** `/v1/auth`, `/v1/scmudc/{d}`,
   `/core02/.../orion/station`, `/custom/v1/playback/{encodedURL}`,
   `/bmx/tunein/v1/report`, `/media/aftertouch-ding.wav`,
   `/media/bmx-icons/{...}`, group delete lifecycle.
2. **Medium (TuneIn live dep, like the existing playback test):**
   `/bmx/tunein/v1/playback/episode(s)/{id}`, `/bmx/tunein/v1/favorite/{id}`.
3. **Low / edge (still open):** PUT-device 401, `/v1/blacklist` 405,
   `/ced/*` 404, `/alexa/certificate` 501, `/media/tts/{hash}`. Quirky-status
   pins; add on demand.
