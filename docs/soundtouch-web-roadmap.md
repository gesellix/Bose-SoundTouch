# soundtouch-web: remaining features

Three features complete the parity gap between soundtouch-web and the Stockholm
app's local-control functionality. Everything else in Stockholm (OAuth flows,
setup wizard, service account linking, onboarding, analytics) is cloud
infrastructure that is either shut down or already handled by soundtouch-service.

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

## 2. Favorites

Mark or unmark the currently playing track as a favourite directly from the
Now Playing card.

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

## Decide later

| Feature                                | Reason                                                             |
|----------------------------------------|--------------------------------------------------------------------|
| Spotify / Pandora / Amazon browsing UI | Requires Bose cloud (shutting down); handled by soundtouch-service |
| Setup wizard (WiFi, Marge migration)   | Already in soundtouch-service setup flows                          |
| OAuth / login flows                    | Cloud-dependent; not needed for local network access               |
| AirPlay / Bluetooth pairing UI         | Device handles this independently; no SoundTouch Web API           |
| Onboarding, help, analytics            | Not relevant for a local control tool                              |
