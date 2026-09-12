---
title: "soundtouch-player: remaining features"
sidebar:
  exclude: true
---
Four features complete the parity gap between soundtouch-player and the Stockholm
app's local-control functionality. Everything else in Stockholm (OAuth flows,
setup wizard, service account linking, onboarding, analytics) is cloud
infrastructure that is either shut down or already handled by soundtouch-service.

> **Shipped:** Saving the current content to a preset slot (slots 1–6) is
> already implemented — a ★ star button in the top-right corner of the Now
> Playing card opens a slot picker, and a **+** button on each preset tile
> saves to that slot directly.  See [the preset guide](../guides/PRESETS.md)
> for usage details.

---

## 1. Seek / scrub

The progress bar already renders `NowPlaying.Time.Position` / `NowPlaying.Time.Total`
with a live 1 s ticker. What's missing is the ability to click or drag it to seek.

**Device API:** `POST /seek` with body `<seek deviceID="…" type="TIME_VALUE"><time>30</time></seek>`

**Backend:**
- Add `POST /api/device-seek/{id}/{seconds}` handler in `handler.go`
- Guard on `NowPlaying.SeekSupported.Value` — return 400 if the stream doesn't
  support seeking (radio, for example)

**Frontend (`NowPlaying.js`):**
- Replace the static `<div class="progress-bar">` with a `<input type="range">`
- `onInput` updates local state for smooth scrubbing; `onChange` (pointer up)
  fires `api.seek(deviceId, seconds)`
- Pause the 1 s ticker while the user is dragging to avoid fighting the input

**Client method to add (or verify exists):**
```go
func (c *Client) Seek(positionSeconds int) error {
    // POST /seek
}
```

---

## 2. Favorites (device-native, distinct from presets)

> **Note:** This section is about the speaker's **built-in** `/favorites` API —
> a separate concept from the 6 preset slots.  Preset-slot saving (★ star /
> **+** button) is already shipped; the native Favorites API is not yet
> surfaced in soundtouch-player.

Mark or unmark the currently playing track as a device favourite directly from
the Now Playing card.  Unlike presets (maximum 6, numbered slots), the device
can hold a larger favourites list; support varies by source.

**Device API:**
- `GET /favorites` — returns `<favorites>` list
- `POST /favorites` — adds current content item as a favourite
- `DELETE /favorites/{id}` — removes a favourite by ID

**Backend:**
- `GET /api/device-favorites/{id}` — fetch favourites list
- `POST /api/device-favorites/{id}` — add current now-playing item as favourite
- `DELETE /api/device-favorites/{id}/{favId}` — remove a favourite

**Frontend:**
- Heart button (♡ / ♥) in `NowPlaying.js`, next to the source label
- On mount (or when `nowPlaying` changes) fetch favourites and check whether
  the current `ContentItem.Location` is already in the list
- Toggle on click; optimistic UI update before the round-trip

**Note:** Not all sources support favourites. Check
`NowPlaying.FavoriteEnabled` — if the field is nil/absent, hide the button.

---

## 3. Device settings panel

A lightweight settings page per device covering the two most useful knobs:
rename and network/firmware info.

**Device API:**
- `GET /info` — device info (already fetched; stored as `DeviceInfo`)
- `POST /name` with body `<name>New Name</name>` — rename the device
- `GET /networkInfo` — IP, MAC, SSID, signal strength
- `GET /swUpdateStatus` — current firmware version and whether an update is
  available (not all devices expose this)

**Backend:**
- `POST /api/device-rename/{id}` — body `{"name":"…"}`; calls `POST /name`
- `GET /api/device-network/{id}` — proxies `GET /networkInfo`
- Optionally `GET /api/device-update-status/{id}` — proxies `GET /swUpdateStatus`

**Frontend:**
- Small ⚙ icon button in `DeviceDetail`'s page header (next to the power button)
- Navigates to a new `page === 'settings'` state in `App`; passes `deviceId`
- `DeviceSettings.js` component: editable name field (save on blur/Enter),
  read-only network info card, optional firmware version badge
- Back button returns to `'device'` page

---

## 4. Stereo-pair presentation and lifecycle (shipped)

soundtouch-player projects a valid two-speaker stereo pair (formed via
`/addGroup` - see [issue #252](https://github.com/gesellix/Bose-SoundTouch/issues/252))
as one logical control target. This restores the single-entry presentation
expected by users while preserving both physical speakers in the service
registry.

**Device API:**
- `GET /getGroup` on each speaker — returns the current `<group>` with
  `<masterDeviceId>` + `<roles>` (each `<groupRole>` carries the speaker's
  deviceId, role `LEFT|RIGHT`, and ipAddress)
- Empty `<group/>` means the speaker is standalone
- Querying the master and slave returns the same `<group>` payload, so either
  side is sufficient to detect the pair

**Backend:**
- Poll `GET /getGroup` together with the other device status and consume
  `groupUpdated` events. A generation check prevents an older poll from
  overwriting a newer event.
- Collapse only an exact two-member `LEFT`/`RIGHT` group whose registered
  members agree on the group claim. Malformed, conflicting, or ambiguous data
  fails open and leaves the physical entries visible.
- Use the master speaker's existing registry key for the logical target, so
  controls continue to route through the master without changing the raw
  physical-device registry.
- Use the same projection for the REST device list and the global player
  WebSocket snapshot.

**Frontend:**
- Render one card using the shared member name or the group's name.
- Show pair availability as `Stereo pair n/2` and mark the card degraded when
  a member is unavailable or the group reports a non-OK state.
- Hide the single-device remove action on a projected pair. Standalone
  speakers continue to render as before.
- For standalone stereo-capable SoundTouch 10 speakers, offer pair creation
  with an explicit LEFT/master and RIGHT member.
- For an existing pair, offer rename and a separately confirmed dissolve
  action. A dissolve changes speaker group state; it does not delete either
  physical speaker from the player registry.

**Lifecycle safety:**
- A shared coordinator backs both the CLI and player. It freshly checks both
  speakers, their L/R capability, current group, and temporary-zone state
  before a mutation. Pair creation also requires one shared Marge account and
  backend.
- After both create candidates are freshly verified as physically standalone,
  a fail-closed, read-only persistence barrier checks for a stored group before
  either speaker is mutated. The embedded service searches every account by
  device ID; standalone player and CLI query the speakers' current Marge
  backend. Creation stops and reports the exact stale generation when any
  record remains; pre-create checks never delete it.
- Create sends the asymmetric master/slave payloads required by the speaker
  state machines, then freshly verifies that both members agree. A partial
  create is compensated only where the exact group generation returned by
  that speaker can be proven.
- Rename and dissolve update both physical speakers and report a degraded
  result, including per-member detail, instead of claiming success after a
  partial transition.
- Rename and dissolve carry the group ID displayed to the user and reject a
  stale request if either speaker now belongs to another generation.
- A degraded dissolve retains the last exact L/R topology for a bounded retry.
  The retry freshly verifies both physical identities and states, and stored
  persistence must match that full topology before it can be retired.
- Legacy Marge teardown callbacks without a group ID are acknowledged without
  deleting persistent state. After a verified physical dissolve, the embedded
  player retires the exact generation directly in its datastore; standalone
  player and CLI use the generation-aware endpoint derived from fresh speaker
  info. Physical verification and exact persistence cleanup share one
  coordinator lock, and a cleanup failure is returned as degraded.
- Retired group IDs leave their small XML snapshot in the datastore and
  active/retired IDs are reserved across all accounts, so an account move or
  stale request cannot match a later physical generation.
- Pair mutations are rejected while either member belongs to a temporary
  multi-room zone. The zone must be dissolved first.

!!! warning "Run lifecycle operations site-locally"
    Marge hostnames such as `unifi` are resolved from the caller's site, not
    from the speaker's site. Create, rename, and dissolve a pair through the
    Player/service deployment co-located with both speakers and their Marge
    backend; a cross-site registry entry is not a backend-routing mechanism.

!!! warning "Datastore downgrade boundary"
    Once this lifecycle has written a `Group_<id>.retired` snapshot, do not run an
    older service binary against the same datastore. Older allocators do not
    reserve these generation IDs globally and can reuse one. Restore both the
    binary and its pre-lifecycle datastore snapshot for a rollback, or upgrade
    forward.

---

## Decide later

| Feature                                | Reason                                                             |
|----------------------------------------|--------------------------------------------------------------------|
| Spotify / Pandora / Amazon browsing UI | Requires Bose cloud (shutting down); handled by soundtouch-service |
| Setup wizard (WiFi, Marge migration)   | Already in soundtouch-service setup flows                          |
| OAuth / login flows                    | Cloud-dependent; not needed for local network access               |
| AirPlay / Bluetooth pairing UI         | Device handles this independently; no SoundTouch Web API           |
| Onboarding, help, analytics            | Not relevant for a local control tool                              |
