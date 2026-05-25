---
title: "Spotify on SoundTouch — Overview"
---
This is the entry point for understanding how Spotify works on a SoundTouch
speaker behind AfterTouch. Read this first; the deeper docs assume you already
have the mental model below.

> **Premium likely required.** As far as we know, Spotify Connect on
> SoundTouch only works with a Spotify Premium account — this matches our
> testing and matches what other SoundTouch-replacement projects report, but
> we have not exhaustively verified every account tier or region. None of the
> workarounds in this document change Spotify's account-tier requirements.

## Two completely separate Spotify paths

These are routinely confused. They share a speaker and a Spotify account, but
they ride on different infrastructure and fail for different reasons.

### 1. Spotify Connect (speaker-native, independent of AfterTouch)

- The speaker advertises itself on the LAN as a Spotify Connect endpoint
  (mDNS service `_spotify-connect._tcp`).
- You open the Spotify app on your phone or desktop, tap the Connect device
  picker, and select the SoundTouch.
- Audio streams directly from Spotify's CDN to the speaker. Token handling,
  session setup, and playback all happen between Spotify and the speaker.
- **AfterTouch is not involved.** It still works even if AfterTouch is
  offline.

This is the simplest path. If you only want to push playback from your phone,
you do not need to link Spotify to AfterTouch at all — see [Manual kick-start
alternative](#manual-kick-start-alternative) below.

### 2. OAuth-intercept path (managed by AfterTouch)

This is what enables features that originate **from the speaker**:

- Spotify presets on the speaker's buttons.
- Spotify playback from the Bose app's source picker.
- "Resume Spotify" after a power cycle without touching the Spotify app.

After Bose's cloud shutdown (May 2026), the speaker can no longer reach
Bose's OAuth server for Spotify token refresh. AfterTouch intercepts those
calls via DNS, brokers tokens with Spotify using your linked account, and
hands them back to the speaker.

The rest of this document describes that path.

## Setup at a glance

Full step-by-step is in
[docs/guides/MUSIC-SERVICES.md](../guides/MUSIC-SERVICES.md). Summary:

1. **Register a Spotify developer app** (one-time, by the AfterTouch operator).
2. **Configure AfterTouch** with the Client ID, Client Secret, and Redirect
   URI in the Settings tab.
3. **Authorize your Spotify account** via the Local Account tab — completes
   the OAuth flow and persists a long-lived refresh token to AfterTouch's
   datastore.
4. **Prime each speaker** so its source list and ZeroConf state know about
   Spotify.

After step 4, presets and Bose-app-initiated Spotify playback work.

## The DNS rewrite — easy to miss, breaks everything

Bose firmware does **not** read a separate OAuth server hostname from
configuration. It derives the OAuth host from the marge host by inserting
`oauth` into the first label:

| Purpose         | Hostname                  |
|-----------------|---------------------------|
| Marge / sources | `streaming.bose.com`      |
| OAuth refresh   | `streamingoauth.bose.com` |

**Both hostnames must resolve to AfterTouch.** AfterTouch's DNS server hijacks
both, but if you bypass that DNS server (e.g. by hard-coding only the marge
hostname in `/etc/hosts`, or by routing only one through a custom resolver),
token refresh will silently die while the speaker still pulls sources.
Symptom: the speaker briefly streams Spotify after priming, then stops at the
first token refresh ~1 hour later.

If you self-host AfterTouch at e.g. `aftertouch.lan`, the speaker derives
`aftertouchoauth.lan` and queries that hostname for token refresh. AfterTouch's
DNS server **auto-derives this alias** from the configured `--server-url` and
adds it to the hijack list automatically — the operator does not have to
configure it as long as speakers resolve names via AfterTouch's DNS server
(via DHCP, the `setup migrate --method=resolv` flow, or an external LAN DNS
that delegates to AfterTouch for these names). The implementation lives in
`pkg/discovery/dns.go` `DeriveOAuthHostnames`.

> **IP-based `--server-url` is incompatible with OAuth (both Spotify and Amazon
> Music).** The speaker's hostname construction appends `oauth` to the first
> label only, so `192.168.0.30` would produce `192oauth.168.0.30` — malformed,
> no DNS resolver will answer for it, and there is no clean workaround on the
> AfterTouch side. **Use a real LAN hostname** before configuring Spotify or
> Amazon Music. The Health-tab `oauth_target_reachable` check warns when this
> trap is wired up.

## End-to-end token lifecycle

What actually happens, from priming to steady-state playback:

1. **Operator links Spotify account.** OAuth flow stores
   `{user_id, refresh_token, bose_secret}` in `spotify/accounts.json`. The
   `bose_secret` is an opaque surrogate (e.g. `bs-deadbeef…`) that AfterTouch
   issues; the speaker only ever sees this surrogate, never the real Spotify
   refresh token.
2. **Priming runs.** Either on speaker `power_on`, on discovery, or on a
   manual `POST /mgmt/spotify/prime`. AfterTouch:
   - Resolves the speaker's currently-paired account via live `:8090/info`
     (`margeAccountUUID`).
   - Writes a `SPOTIFY` `ConfiguredSource` into marge under that account with
     `secret = bose_secret`, `secretType = token_version_3`.
   - POSTs `<updates><sourcesUpdated/></updates>` to the speaker's
     `:8090/notification`, causing the speaker to re-fetch
     `/streaming/account/{account}/full` and pick up the new source.
   - Optionally pushes a fresh access token to the speaker's ZeroConf
     endpoint (`:8200/zc?action=addUser`). This is best-effort — see
     [ZeroConf clientId and benign 404s](#zeroconf-clientid-and-benign-404s).
3. **Speaker pulls sources.** It now has a SPOTIFY entry with the surrogate
   as its credential. The speaker stores this; from its perspective the
   surrogate is the refresh token.
4. **Speaker uses Spotify.** When it needs a fresh access token (every ~1 h
   on Spotify's clock), it POSTs to
   `streamingoauth.bose.com/oauth/device/{deviceID}/music/musicprovider/15/token/cs3`
   with the surrogate.
5. **AfterTouch translates.** DNS hijack routes the request to AfterTouch,
   which looks up the surrogate, performs the real refresh against Spotify
   using the stored refresh token, and returns the resulting access token to
   the speaker.
6. **Speaker uses the access token** for Spotify Web API metadata calls
   (artwork, track lookups, playback container resolution).

Forensic details of the request shapes are in
[docs/reference/spotify-account-addition.md](../reference/spotify-account-addition.md).
The cryptographic specifics of the ZeroConf `addUser` blob are in
[spotify-priming-strategy.md](spotify-priming-strategy.md).

## ZeroConf clientId and benign 404s

`GET http://<speaker>:8200/zc?action=getInfo` returns, among other fields:

```json
"clientID": "79ebcb219e8e4e9a892e796607931810"
"tokenType": "accesstoken"
"activeUser": "<spotify-user-id-or-empty>"
```

That `clientID` is **Bose's official Spotify Connect partner client_id**,
baked into firmware. It is **not** the client_id of the developer app you
registered for AfterTouch — those are two unrelated OAuth apps, by design.
The Bose-baked one is what Spotify Connect uses when a Spotify mobile app
discovers the speaker on the LAN. The AfterTouch-registered one is what
brokers refresh tokens for the OAuth-intercept path. They never converge.

**Implication:** an access token AfterTouch obtained under its own client_id
is not directly usable as a Spotify Connect session token. Pushing it via
ZeroConf `addUser` is best-effort, and the speaker may respond with a `404`
and an empty body when its `activeUser` already matches the username being
pushed — that is the firmware's idiomatic "no transition required" signal,
not a failure. AfterTouch recognises this case (`zeroconf.ErrAddUserNoOp`)
and logs it as an expected no-op rather than an error.

A 404 **with a body**, or any other non-2xx, is treated as a real failure
and logged loudly with the response headers and body so it can be
diagnosed.

## Manual kick-start alternative

You can skip the OAuth setup entirely if you only want playback pushed from
the Spotify app:

1. Open the Spotify mobile/desktop app.
2. Start any track.
3. Open the Connect device picker, select the SoundTouch.

The speaker now holds an in-memory Spotify Connect session and can play
until next reboot. Presets and Bose-app-initiated Spotify playback will
still not work — those require the OAuth-intercept path — but Spotify-app-
initiated playback does.

## Troubleshooting quick reference

| Symptom                                            | Most likely cause                                                                              |
|----------------------------------------------------|------------------------------------------------------------------------------------------------|
| Preset stores then fails: "invalid SourceID"       | No `SPOTIFY` source in marge for the speaker's paired account. Re-run priming.                 |
| Preset stores fine; playback dies after ~1 hour    | `streamingoauth.bose.com` not pointed at AfterTouch (DNS rewrite gap).                         |
| Speaker has source but `Sources.xml` looks stale   | `<sourcesUpdated/>` notification did not reach the speaker. Re-run priming or POST it by hand. |
| ZeroConf `addUser` returns 404, empty body         | Benign no-op; speaker already has `activeUser` set. Marge path is authoritative.               |
| Spotify Connect device picker doesn't show speaker | Unrelated to AfterTouch; check the speaker's mDNS visibility on the LAN.                       |

## Where to go next

- **Setup walkthrough:** [docs/guides/MUSIC-SERVICES.md](../guides/MUSIC-SERVICES.md)
- **OAuth flow details (browser + mobile + endpoint table):** [spotify-oauth.md](spotify-oauth.md)
- **Priming strategy, ZeroConf DH protocol, deployment topologies:** [spotify-priming-strategy.md](spotify-priming-strategy.md)
- **Forensic request/response analysis from the Stockholm app:** [docs/reference/spotify-account-addition.md](../reference/spotify-account-addition.md)
