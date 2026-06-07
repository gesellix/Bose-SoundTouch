---
title: "API Route Layout and Refactoring Plan"
---

> **Tracking issue:** [#451 "Merge soundtouch-player into soundtouch-service"](https://github.com/gesellix/Bose-SoundTouch/issues/451).
> This document is the architectural reference for the staged API refactoring
> that precedes (and enables) that merge.

## Why this exists

`soundtouch-service` and `soundtouch-player` are two binaries with two routers.
We want to:

1. Restructure our own routes into a layout that can stay stable.
2. Eventually fold `soundtouch-player` into `soundtouch-service` (one binary).
3. Stop leaking frontend (SPA) routes into the backend API.
4. Make **cloud / remote-host a first-class, clean deployment**, not just LAN /
   on-device. This is a primary motivation: we consolidate the API *in a way
   that* closes the trust and auth gaps a public deployment exposes, rather than
   just merging two binaries. Enforced auth is therefore a real requirement, not
   an afterthought.

Before moving anything, every route has to be classified by **whether we are
free to move it**, and that depends on **who the client is**. A route the
speaker firmware calls is frozen forever; an internal admin route is ours to
reshape.

## Classification criteria

Classify by client audience, then by what pins the path:

| Category                           | Client                            | Free to move?                                                                                                  |
|------------------------------------|-----------------------------------|----------------------------------------------------------------------------------------------------------------|
| **(1a) Frozen, firmware-pinned**   | Speaker firmware                  | No, ever. The path is hardcoded in the speaker (or relative to a base it fetches from us).                     |
| **(1b) Frozen, externally-pinned** | OAuth providers (Spotify/Amazon)  | Only with provider re-registration + device re-priming. Treat as frozen unless that cost is paid deliberately. |
| **(2) Service-internal**           | The admin/setup UI                | Yes, freely. These are ours.                                                                                   |
| **(3) Web/control**                | The control UI (soundtouch-player)   | Yes, freely.                                                                                                   |
| **(4) Frontend (SPA)**             | Browser, client-side routing      | Should not be enumerated in the backend at all (see `/app/*` below).                                           |
| **(Infra)**                        | Humans, monitoring, the SPA shell | Conventionally stable; collision-prone at merge time.                                                          |

Two refinements that matter in practice:

- **"Must stay" is not one thing.** (1a) is immovable; (1b) is movable but
  coordinated. Do not lump OAuth callbacks in with firmware paths.
- **The merge-overlap bucket is smaller than it looks.** Verified against the
  two routers, only **`/` is a true collision** (service `HandleRoot` vs the web
  app's `serveIndex`); resolve it with a small **landing page** at `/` that lets
  the user pick Admin/Setup (service) or the App (web). **`/health` is a merge,
  not a clash** (both define it; standardise on the service's richer body, which
  carries version + timestamp, and confirm nothing depends on the web's
  `{"status":"ok","version"}` shape). **`/ws` and `/static/*` do not collide at
  all** — the service registers neither, so bringing the web's in is purely
  additive. TuneIn is **not** in this bucket either: `/bmx/tunein/*` (speaker <->
  BMX integration, frozen) and `/api/tunein/*` (the player's generalized radio
  search/play, ours to change) are two different layers.

  **Resolve overlaps structurally, before merging, not behind a flag.** A
  conditional "only register the web routes when opt-in is on" does not fix a
  collision — it just hides it while the flag is off, and the double-registration
  returns when it's on. Do not rely on chi to detect or warn about it. Clean up
  `/` (and the `/health` merge) up front so the merged router is unambiguous
  regardless of the flag. The opt-in (below) exists only to let people optionally
  run the merged variant and give feedback, not as a collision guard.

## What pins the frozen routes (evidence)

- The speaker fetches BMX content, marge/streaming data, software updates, and
  CED config from hostnames it has hardcoded (or from a base URL we hand it).
  `/ced/*` mirrors `downloads.bose.com/ced/soundtouch/...`; `/bmx`, `/core02`,
  `/streaming`, `/accounts`, `/customer`, `/oauth`, `/v1` mirror the Bose cloud
  contract.
- Persisted device data embeds absolute service URLs. Presets store
  `LOCAL_INTERNET_RADIO`/Orion locations like
  `https://.../core02/svc-bmx-adapter-orion/prod/orion/station?data=...`, and
  the BMX registry advertises `{MEDIA_SERVER}/media` and `/bmx-icons`. So
  `/media`, `/bmx-icons`, `/custom`, and `/core02` are effectively part of the
  firmware-facing contract: a speaker that stored a preset will replay that
  exact URL later. They cannot move without rewriting persisted state on every
  device.

## Service routes (`soundtouch-service`)

Grouped by prefix. The authoritative enumerated list is the router golden file
`cmd/soundtouch-service/testdata/router_routes.txt`.

| Prefix                                                                                             | Category              | Client                                | Movable?                                 |
|----------------------------------------------------------------------------------------------------|-----------------------|---------------------------------------|------------------------------------------|
| `/streaming/*`                                                                                     | (1a) frozen           | Speaker (marge / streaming.bose.com)  | No                                       |
| `/accounts/*`                                                                                      | (1a) frozen           | Speaker (marge, alternate paths)      | No                                       |
| `/customer/account/*`                                                                              | (1a) frozen           | Speaker                               | No                                       |
| `/bmx/*` (registry + tunein)                                                                       | (1a) frozen           | Speaker (BMX)                         | No                                       |
| `/core02/svc-bmx-adapter-*` (Orion, SiriusXM)                                                      | (1a) frozen           | Speaker (BMX adapters)                | No                                       |
| `/oauth/*/token`<br>`/oauth/*/token/cs`<br>`/oauth/*/token/cs1`<br>`/oauth/*/token/cs3`            | (1a) frozen           | Speaker (music tokens)                | No                                       |
| `/custom/v1/playback/*`                                                                            | (1a) frozen           | Speaker (LOCAL_INTERNET_RADIO / ding) | No                                       |
| `/bmx-icons/*`<br>`/media/*`<br>`/media/aftertouch-ding.wav`<br>`/media/tts/*`                     | (1a) frozen           | Speaker (advertised base)             | No                                       |
| `/streaming/resources/api_versions.xml`<br>`/streaming/software/update/*`<br>`/updates/soundtouch` | (1a) frozen           | Speaker (SW update)                   | No                                       |
| `/v1/auth`<br>`/v1/blacklist/*`<br>`/v1/scmudc/*`<br>`/v1/stapp/*`                                 | (1a) frozen           | Speaker                               | No                                       |
| `/alexa/certificate`                                                                               | (1a) frozen           | Speaker / AWS                         | No                                       |
| `/ced/*`                                                                                           | (1a) frozen           | Speaker (mirrors downloads.bose.com)  | No                                       |
| `/mgmt/amazon/callback`<br>`/mgmt/spotify/callback`                                                | (1b) frozen, external | OAuth providers                       | Only with re-registration                |
| `/setup/*` (~40 routes)                                                                            | (2) service-internal  | Admin UI                              | Yes                                      |
| `/mgmt/*` (except the callbacks above)                                                             | (2) service-internal  | Admin UI                              | Yes                                      |
| `/web/*` (`HandleWeb`)                                                                             | (4) frontend          | Browser (admin SPA)                   | Yes; already the clean catch-all pattern |
| `/`<br>`/docs/*`<br>`/favicon.ico`<br>`/health`                                                    | (Infra)               | Humans / monitoring                   | Keep stable by convention                |

## Web routes (`soundtouch-player`)

Defined in `pkg/service/soundtouchweb/mount.go`. Not currently mounted inside
the service; it is a separate binary.

| Group                                                                                    | Category        | Note                                                                                                                                                                |
|------------------------------------------------------------------------------------------|-----------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `/api/*` (devices, control, tunein, zone, radiobrowser, play-url, device-speak)          | (3) web/control | Freely restructurable                                                                                                                                               |
| `/health`<br>`/static/*`<br>`/ws`                                                        | (Infra)         | `/health` is a merge (standardise on the service's body); `/static/*` and `/ws` are additive (the service registers neither)                                        |
| `/`<br>`/device/*`<br>`/devices`<br>`/playurl`<br>`/radiobrowser`<br>`/tts`<br>`/tunein` | (4) frontend    | `/` is the one true collision (-> landing page); the rest move under `/app/*`. The anti-pattern: each SPA route enumerated in the backend, all serving `index.html` |

## Deployment scenarios, reachability, and trust boundaries

The client-audience axis tells you *who* calls a route. The deployment tells you
whether that caller can actually reach it and whether the surrounding network
can be trusted. AfterTouch runs in materially different places, and that decides
which routes are even *meaningful* and what the trust boundary is.

### Actors (the original Bose model)

The original Bose architecture had three actors, and our route surface still
reflects all three:

| Actor   | Where                                        | Role                                                                                                                                           |
|---------|----------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| Speaker | Local (the device)                           | Calls the cloud for its data-plane (`/full`, presets, sources, software update, tokens) and is provisioned by the app.                         |
| App     | Local (phone / desktop), **in-between**      | Creates the account, adds a speaker to an account, and teaches the speaker its cloud/marge credentials. **Authenticates itself** to the cloud. |
| Cloud   | External / public (what AfterTouch replaces) | Serves the speaker data-plane and the app's account/provisioning calls.                                                                        |

Two things matter for our design:

- **The app is deployment-agnostic.** It does not care whether the cloud (our
  service) runs locally or in a datacenter; it talks to whatever cloud endpoint
  it is pointed at. So the **deployment modes below are about where the *cloud*
  role runs**, orthogonal to the app actor.
- **AfterTouch's own tooling currently plays the app's role.** Account creation
  and "teach the speaker its marge account" are done by our migration tooling
  (today via the speaker's local WebSocket `setMargeAccount`), i.e. we are the
  provisioning agent. But the app-facing *cloud* endpoints still exist in the
  surface (account create/login, add device, profile, password, groups), and a
  real app pointed at us would use them. They are part of the frozen contract,
  but their caller and trust story differ from the speaker's data-plane (see
  below).

### Deployment topologies (where the cloud role runs)

This is descriptive (where it runs), distinct from the `deployment-mode`
*parameter* below (the security posture). They correlate but are kept separate so
an operator is not locked into one because of the other.

| Topology            | Where                                     | Reaches speakers directly? | Speaker reaches it?                       |
|---------------------|-------------------------------------------|----------------------------|-------------------------------------------|
| On-device           | On the speaker itself                     | Itself only                | Yes (loopback / LAN)                      |
| LAN host            | Raspberry Pi / Docker on the home network | Yes (same LAN)             | Yes                                       |
| Cloud / remote host | External host, not on the speaker LAN     | No                         | Yes (speaker calls out over the internet) |

### Two planes: speaker-direct vs data-plane

Routes fall into two reachability planes that behave very differently across
deployments:

- **Speaker-direct (control plane):** the service opens a connection *to* the
  speaker's local API (`:8090`) right now. Discovery, migration, reboot,
  test-connection, peer-probe, and the entire `soundtouch-player`
  control/zone/volume/key/TTS-to-speaker surface. These only work where the host
  shares the LAN with the speaker. **In a cloud deployment they are dead weight**,
  and any UI that shows them is misleading.
- **Data-plane (cloud replacement):** something calls the *service*, which works
  in every deployment because the caller reaches in. Two callers live here:
  - **Speaker-polled:** the speaker fetches its own data (`/full`, sources,
    presets, recents, provider/device settings, software update, streaming
    token, stats). No user auth; the speaker is identified by account/device.
  - **App / provisioning-called:** the app (or, today, our own tooling acting as
    the app) creates accounts, logs in, adds/updates/removes devices, edits the
    profile/password, and manages groups. In the original model the app
    **authenticates itself** here, so these endpoints carry an auth dimension the
    speaker's polling does not. They are deployment-agnostic: the app reaches the
    cloud wherever it runs.

So a cloud deployment is essentially the data-plane (both callers) plus
server-side state management (accounts, presets, provider credentials,
diagnostics of stored data). The interactive "do something to a speaker now"
features (both the player and migration) need LAN proximity.

Consequence for the migration tooling (ref the #451 discussion): migration is
**recurring**, not one-shot (you add a speaker later too), and it is
**LAN-bound**. That argues for migration as a local mode/tool you run on the LAN
when needed, rather than always-on code in a cloud binary that could never use
it.

### Trust zones and the current state

The trust zones, mapped to the actors above, and today barely any is guarded:

| Zone               | Routes                                                                                                                  | Client auth today                                                                                                                                        | Should be                                                                                          |
|--------------------|-------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| Speaker contract   | frozen (1a), speaker-polled                                                                                             | None (no user login; the app_key is validated but is not user auth)                                                                                      | None, but network-segmentable; in cloud these are necessarily public so the speaker can reach them |
| App / provisioning | account create/login, add/update/remove device, profile, password, groups (`/streaming/account*`, `/customer/account*`) | None enforced (we accept; the app's self-auth from the original model is not required)                                                                   | Authenticated in cloud: an open provisioning surface lets anyone create accounts or attach devices |
| Admin / setup      | `/mgmt/*` (non-callback)<br>`/setup/*`<br>`/web/*`                                                                      | `/mgmt/*` has single-credential HTTP Basic Auth; **`/setup/*` has none** (explicit "LAN-trust" premise); the Basic Auth even leaks behind a proxy (#419) | Authenticated always; mandatory in cloud                                                           |
| Control / player   | `/api/control/*` (post-merge)                                                                                           | None                                                                                                                                                     | Optional auth; low blast radius                                                                    |

The "LAN-trust" premise is defensible on a home LAN but **invalid in the cloud**:
`/setup/*` (migration, DNS redirect, trust roots / cert state, account data,
diagnostics, recovery) is wide open, and so is the app/provisioning surface
(anyone could create an account or attach a device). On a public host both are a
real exposure. Closing these gaps is a prerequisite for treating cloud as a
supported deployment.

### Requirements this drives

- **Authentication** on everything user-facing, actually enforced (not
  bypassable behind a proxy, see #419). Mandatory for cloud; offered and
  recommended for LAN.
- **Authorization tiers** by blast radius (the "authority boundary" from the
  #451 landing-page note): a low-privilege user may open the player, while
  setup/mgmt (trust roots, migration, accounts) require admin. The landing page
  is where that boundary is made explicit.
- **Deployment-aware surface:** in cloud mode, hide/disable the speaker-direct
  features (they cannot work) and require auth on the rest; in LAN/on-device
  mode, expose the full surface.

These requirements are why the `/api/*` split below is grouped by trust tier:
applying an auth (and later authz) middleware to a whole group is a one-liner,
whereas per-route auth is what produced today's patchy coverage.

### The `deployment-mode` parameter (private / shared / public)

The security posture is an explicit parameter, **default `private`**. It is a
preset over the per-tier auth machinery, not separate architecture: each value
just sets which trust tiers require auth.

| Tier (caller)                | private       | shared                | public          |
|------------------------------|---------------|-----------------------|-----------------|
| Speaker contract             | none (frozen) | none                  | none            |
| Control / player             | open          | open                  | **auth**        |
| Admin / setup + provisioning | open (opt-in) | **auth (min. Basic)** | **auth**        |
| Speaker-direct features      | on            | on                    | hidden/disabled |

- **private** (default): free-for-all, maximum insecurity, security is opt-in.
  Matches a trusted single-owner LAN or on-device.
- **public**: opt-out of security. Everything user-facing requires auth; the
  surrounding network is untrusted (cloud / internet). Speaker-direct features
  are hidden (they cannot work off-LAN anyway).
- **shared**: at least Basic Auth on the structural / admin routes, while the
  player stays open. The multi-user trusted-LAN case (guests, kids, roommates):
  daily playback without a login, but infrastructure is protected.

The **Speaker-direct features** row is UI gating, not auth. "Speaker-direct"
means actions that reach a speaker on the LAN (discover, migrate, reboot,
volume/play, zone). In `public` the UI hides or disables them because they cannot
work off-LAN, so we do not show buttons that only fail; in `private` / `shared`
they are shown. The UI derives this from the mode.

**Provisioning is treated like admin**, not like the player: creating accounts
and attaching devices is structural / high-blast-radius, so it shares the admin
trust tier (protected in `shared` and `public`). The mechanism may still be Marge
self-auth, but the *requirement* matches admin.

It is a **monotone ladder**: private -> shared adds admin (+ provisioning) auth;
shared -> public additionally locks the player.

**Is `shared` necessary, or just `public`?** It is necessary and distinct. The
only difference between shared and public is the **player tier**: shared keeps it
open (trusted network, frictionless household use), public locks it (untrusted
network). Collapsing them forces either a password on the daily-use player at
home, or an open player on the internet. The cost of keeping `shared` is near
zero once tier-based auth exists (it is just "admin required, player optional"),
so it earns its place as the trusted-LAN-with-privilege-split preset.

Note this 3-value enum compresses two orthogonal axes, network trust (private /
shared = trusted; public = untrusted) and the player/admin privilege split. That
is a deliberate usability simplification over a toggle matrix; if the presets
ever feel too coarse, the underlying per-tier toggles are the escape hatch.

**Configuration and lockout-safety.** `deployment-mode` is set like the other
config: CLI flag, env var, and persisted setting, with the same precedence as
the rest (e.g. like `server-url`). Because of that, the host operator always has
an out-of-band path: even if a mode change in the UI would lock them out, they
can reset it via env / flag / the settings file on the host. The service must
also not let a setting strand its owner: if a mode requires auth but no
credential / provider is configured yet, warn and keep a way in (refuse to apply,
fall back, or allow a loopback/on-host admin bypass) rather than hard-locking the
admin surface.

### Auth posture: opt-none -> opt-in -> opt-out?

The maturity path over releases, which the `deployment-mode` parameter then
expresses per posture:

- **Today: "opt-none".** Auth is not even opt-in. `/mgmt` has a single Basic-Auth
  credential (and it leaks behind a proxy, #419); `/setup` and the
  app/provisioning surface have nothing. There is effectively no usable way to
  turn real auth on. This is `private` before `private` is even a choice.
- **0.x: prepare opt-in.** Make auth something an operator *can* enable
  (enforced, not proxy-bypassable; covering the whole admin tier, ideally the
  provisioning surface too) and introduce the `deployment-mode` parameter so
  `shared` / `public` become selectable. The default stays `private` (security
  off) so existing LAN setups are undisturbed.
- **1.x: default still `private`?** The parameter exists, but whether the
  shipped default should ever move off `private` is the open call. A cloud-first
  stance argues for stricter defaults; the home-LAN majority argues for keeping
  `private`. Because the posture is now an explicit parameter, the default can
  stay `private` while operators opt into `shared` / `public`, so there is no
  need for a hard global flip.

### Auth mechanisms

Three identities, three mechanisms, only the last two are ours to shape:

- **Speaker -> data-plane: fixed, not ours to change.** The speaker authenticates
  with a long-lived **Marge account token**, provisioned as an account ID + auth
  token (`SetMargeAccount(accountID, authToken)`,
  `pkg/service/setup/init_plan.go`); it is *not* given an email/password. This is
  part of the frozen contract, so no new auth mechanism can be imposed on the
  speaker.
- **Admin -> admin UI: HTTP Basic Auth to start, pluggable later.** We begin with
  Basic Auth as the single admin mechanism, but structure it behind one boundary
  so additional providers (OIDC, etc.) are easy to add. None of this ever reaches
  a speaker; it is purely our app's auth.
- **User -> web app: Marge auth, delegated.** A human (not a speaker) signs into
  the player/control UI with their Marge account, and that authentication
  **delegates to the existing Marge routes** (`/streaming/account/login` and the
  app/provisioning surface). "User auth" thus reuses the same account the speaker
  belongs to, rather than a separate user store.
- **Native / non-browser clients (CLI, desktop or mobile app, automation) ->
  service.** A whole client class, not just the CLI. Talking to a *speaker's*
  local API needs no service auth; talking to *our service*
  (cloud/admin/provisioning routes) makes them authenticated clients. Interactive
  native clients do OIDC the standard way (RFC 8252, "OAuth 2.0 for Native
  Apps"): a loopback `localhost:<port>` redirect (CLI / desktop) or a private-use
  URI-scheme redirect (`app://callback`, mobile); the system browser runs the
  flow and the client exchanges the code for a token. The case that still needs a
  **non-interactive** credential (issued token / API key, or a device-code /
  client-credentials grant) is **headless** automation: CI, scripts, no browser.
  Requirements on the provider abstraction: (a) support both an interactive path
  (browser, including native loopback / custom-scheme redirects) and a headless
  token path, and (b) allow registering those redirect URIs (the same
  externally-pinned concern as the Spotify/Amazon callbacks).

**Mental model: Marge is an auth provider, like EntraID would be.** The UI auth
sits behind one provider abstraction, and Marge is simply one provider
implementation (the built-in / legacy one) alongside Basic Auth and future OIDC
providers (EntraID, Google, ...). "Sign in with your Marge account" is the same
pattern as "Sign in with EntraID": the app delegates to the provider. Basic Auth,
Marge, and any OIDC provider all implement the same interface, so they are
interchangeable and additive.

Design rule: keep the UI auth pluggable behind that single provider boundary so
new providers slot in without touching the speaker contract (which is not a
provider and never changes) or the Marge delegation.

### Identity in logs

Request logs should carry the resolved caller identity as context, **but only
where the request actually exposes one** (do not fabricate an id the protocol did
not send):

- **Authenticated UI / native / headless clients:** once auth lands, log the
  principal (provider subject / username / client id).
- **Speakers:** there is no single speaker login, so it depends on the route.
  Many marge/streaming routes embed `{account}` / `{device}` in the path (also
  `/v1/scmudc/{deviceId}`, `/v1/stapp/{deviceId}`), so the device/account is
  available and worth logging. Others (BMX content like `/bmx/tunein/...`,
  `/v1/auth`) carry only a token / app_key or nothing identifying; log what is
  present and otherwise leave it blank rather than guessing.
- **Unauthenticated:** mark as anonymous.

Caveats: sanitise the value before logging (the existing log-injection guard,
`sanitizeLog` / `sanitizeErr`), and remember these ids (account / device /
principal) are sensitive, so they must follow the existing log redaction on
diagnostic export, not leak into shared bundles.

## Target layout

```
# Frozen compat layer (top-level, never reshape):
/streaming /accounts /customer /bmx /core02 /oauth /custom
/media /bmx-icons /updates /v1 /alexa /ced

# Our JSON API (everything movable lives here, grouped BY TRUST TIER so
# auth/authz middleware applies per group, not per route):
/api/setup/*       (today: /setup/*)            -> admin tier: auth required
/api/mgmt/*        (today: /mgmt/*, no callbacks) -> admin tier: auth required
/api/control/*     (today: soundtouch-player /api/*) -> player tier: auth optional
/api/devices ...

# OAuth provider callbacks (externally-pinned; freeze in place,
# or move only with provider re-registration):
/mgmt/spotify/callback, /mgmt/amazon/callback

# Frontend (one role-gated app, single catch-all, no per-route registration):
/app/*   (the unified app; role/auth decides Player vs Setup visibility)
/web/*   (legacy admin UI; retired once /app/* subsumes it)

# Infra:
/health /metrics /ws
```

### The `/app/*` pattern

The service's admin UI already does the right thing: `/web/*` is one catch-all
(`HandleWeb`), not one route per page. The `soundtouch-player` SPA routes
(`mount.go`, the `/`, `/devices`, `/tunein`, ... block) are the legacy
anti-pattern. The target:

- **`/app/*`** is a single catch-all that returns `index.html`. The browser does
  client-side routing within `/app/`. No frontend path appears in the backend
  router.
- **`/api/*`** serves data only.
- Static assets live under a fixed prefix (e.g. `/app/static/*`).

This keeps the backend API free of frontend routes while still avoiding any
need for server-side SPA routing config.

### One app, role-gated (not two apps)

Decision: converge to a **single app** under `/app/*`; role/auth decides what a
user sees (Player vs Setup are views of one app, not separate apps). This is the
natural expression of the trust tiers, removes the duplicated shell / device
handling the two frontends carry today, and lets them share device list and
state (the data-sharing win from the #451 discussion). `/web/*` is retired once
`/app/*` subsumes it.

Two things make this safe:

- **Size (the on-device concern): "one app" is not "one eager bundle."**
  Code-split the heavy Setup/Admin surface (migration, certs, DNS, diagnostics,
  the ~4.8k-line `script.js`) into a **lazily loaded chunk** that loads only when
  an admin navigates there, so the Player path stays light. If size ever gets
  tight on-device, a **build tag / flag** can produce a player-only variant that
  does not embed the Setup chunk at all. The combined *embedded* size is likely
  to *drop*, not grow, since two separate apps duplicate more than one modular
  app does; the only real risk is naive eager bundling. Guard it with a
  bundle-size / route-count acceptance check (per the #451 discussion): measure
  first.
- **Role-gating is UX, not security.** Hiding the Setup views from non-admins is
  convenience only. The real boundary stays the **server-side auth middleware**
  on the admin / provisioning tiers, otherwise someone just loads the chunk and
  calls the routes directly.

## Regression safety: contract tests from the frozen recordings

Build the regression net **before** touching routes. We already record
interactions (`RECORD_INTERACTIONS`) and have a large collection; frozen and
sanitised, that collection becomes a contract suite that proves the refactor
preserves behavior. It is stronger than the router golden file
(`router_routes.txt`), which only checks that routes are registered, not what
they return.

Two directions, matching the two consumers:

- **Speaker contract (highest value): provider-side replay.** The speaker is a
  consumer we do *not* control (it is Bose firmware), so this is not classic
  consumer-driven Pact: the speaker's real recorded traffic *is* the contract.
  Replay each recorded request against the service and assert the response still
  matches (body and headers). This pins category-1 byte-for-byte, exactly the
  invariant the refactor must not break, and it catches subtle wire details a
  route reshuffle could disturb (for example the case-sensitive `ETag` header).
  It aligns with the existing parity tests (local vs official Bose recordings).
- **CLI / `/api/*` contract (optional): consumer-driven Pact.** The CLI is a
  consumer we *do* control, so real Pact fits: the CLI declares expectations and
  the service verifies them. Most useful once the new `/api/*` shape exists, and
  to assert **dual-routing equivalence** (old and new path satisfy the same
  contract). Lower priority, since this surface is intentionally changing in 0.x.

We are not starting from zero: the existing `tests/integration/http-client/*.http`
suite (run in CI via `make test-http-client` against the service plus the
spotify/amazon mocks) is already a near-consumer-driven contract from the
speaker's perspective. The requests carry the firmware user-agent
(`Bose_Lisa/27.0.6`) and assert status, content-type, and XML structure of the
marge/streaming/BMX routes. It is not literally Pact (no consumer/provider broker
or generated pacts), but it is functionally the speaker contract, and it already
asserts structure and invariants rather than raw bytes, which is exactly the
matcher approach that keeps contracts non-flaky. The natural path is to treat
this suite as the seed and broaden it with the frozen recordings, rather than
inventing a new harness.

How it de-risks the rebuild:

- Pins the frozen speaker contract so a route reshuffle cannot silently alter the
  wire.
- During dual-routing, runs the same contract against both old and new paths to
  prove the alias is faithful.
- Becomes the gate: the refactor lands only when the contract suite is green.

Caveats:

- **Sanitise before freezing.** Recordings carry real IPs, MACs, account /
  device ids, and tokens; per the repo rules they must be anonymised (the
  existing testdata anonymisation / rotation) before they become committed
  fixtures.
- **Match, do not byte-compare blindly.** Legitimately dynamic fields
  (timestamps, tokens, generated ids, ETag *values*) need normalisation /
  matchers, or the contracts go flaky. Freeze structure and invariants, not the
  volatile bits.

## Staged migration

Everything below happens **within 0.x**. 1.x is only the cutover (removal). The
frozen speaker/app contract routes (category 1) are out of scope throughout: they
never move, so none of the aliasing / redirect / deprecation machinery touches
them.

### Route-transition track (0.x)

1. **Add the new routes, switch the service admin UI to them, alias the old
   paths.** Mount `/setup/*` and `/mgmt/*` under the new `/api/*` grouping (chi
   `Route`/`Mount`; carve it so `/api/control/*` fits later) and point
   `script.js` at the new paths.
   - **Use aliasing (dual-mount), not HTTP redirects, for our own routes:**
     register the same handler at both the old and new path. It avoids the
     client-following and method/body pitfalls of redirects (a redirect would
     have to be 307/308 to keep a POST body) and is a no-break upgrade for any
     lagging client.
   - **Does this work for speaker/legacy routes? No, and it is not needed.** We
     never move frozen routes, and a fixed speaker firmware cannot be assumed to
     follow a redirect on its marge/BMX calls (untested; do not rely on it). This
     step is about our movable routes only.
   - **Exclude** `/mgmt/spotify/callback` and `/mgmt/amazon/callback` (1b):
     freeze, or move only with a deliberate provider re-registration.
2. **First, migrate `soundtouch-player` in place to the target API shape.** Before
   touching the service, restructure the standalone `-web` binary's own routes to
   what they should be *after* the merge: the control API under `/api/control/*`
   and the SPA under `/app/*` (with `/ws` as e.g. `/api/control/ws`). Unlike the
   service, this is a **direct migration, not a dual-mount, and with no
   deprecation signal**: `-web`'s only client is its own bundled frontend, served
   and reloaded from the same binary, so there are no out-of-band callers to keep
   compatible — restructure the routes and update the frontend in lockstep, in
   small commits, and a stale tab is fixed by a reload. (The careful
   add-alias-then-deprecate dance is reserved for `-service`, which is central and
   serves callers we do not control.) The payoff: by the time we merge, `-web`'s
   routes already match the target and don't overlap the service's namespaces, so
   the merge below is a near-additive mount.

3. **Fold `soundtouch-player` into the service.** Bring the (already target-shaped)
   control API in as `/api/control/*` and the UI under `/app/*` (one role-gated
   app, see above).
   The actual overlap to clean up (verified) is small: only **`/`** truly
   collides, so replace the two competing root handlers with a **landing page**
   that routes the user to Admin/Setup or the App; **`/health`** is a merge
   (keep the service's richer body); **`/ws`** and **`/static/*`** are additive
   (the service registers neither, so no collision). Do this cleanup
   structurally and verify it (a test that builds the merged router and asserts
   no double-registration) rather than hiding overlaps behind the opt-in flag.
   Keep the two TuneIn layers separate (frozen `/bmx/tunein/*` vs the player's
   `/api/control/*` radio feature).
   - **Ship the merged variant behind an opt-in flag (default off).** Its sole
     purpose is to let people optionally run the combined binary and give
     feedback; it is **not** a collision guard and **not** a security boundary on
     its own. Until the auth track lands, default-off keeps the merged app/control
     surface from being exposed unless an operator deliberately enables it. The
     flag follows the same CLI/env/persisted precedence as `server-url`, and is
     the seam the `deployment-mode` parameter later subsumes.
4. **Deprecate the `soundtouch-player` binary.** It keeps working in 0.x but prints
   a startup deprecation warning (along the lines of "this binary is removed in
   1.x, use soundtouch-service") so its removal is no surprise.
5. **Warn on old-route hits in the service, observably.** When a deprecated path
   is called, log a deprecation warning **and** count it (a metric / signal), so
   the 1.x removal is data-driven: a route is only cut once it has gone quiet
   across real deployments, not on a guess. *(Done for the `/setup` and `/mgmt`
   legacy paths via `DeprecatedRouteMiddleware`; extends to any future aliased
   route.)*

### Auth track (0.x, parallel)

- Group `/setup/*` + `/mgmt/*` (+ provisioning) into one admin tier and apply a
  single auth middleware, replacing today's per-route gap (`/mgmt` has Basic
  Auth, `/setup` has none).
- Make auth enforceable behind a reverse proxy (close #419), not dependent on a
  header a proxy can strip.
- Land the `deployment-mode` parameter (private / shared / public) with its
  lockout-safety, and the speaker-direct UI gating.
- Authorization (player vs admin tiers) can follow authentication; design the
  groups now so it slots in without another reshuffle.

### Before 1.x: definition of done

1.x removes the old routes and the deprecated binary, so all of this must be true
in a 0.x release first:

- **Auth / `deployment-mode` actually shipped** and opt-in works. This is the
  cloud-first motivation; without it 1.x has no payoff.
- **Every client we ship moved off the old paths:** the admin UI, the merged
  app, the **CLI**, the **HTTP-client integration tests**, **docs and examples**,
  any reverse-proxy guide. The 0.x dual-routing is their migration window, but
  someone has to actually move them.
- **Old-route usage has gone quiet** in the step-4 signal (do not remove blind).
- **A deprecation window of at least one release** where the warnings were live.
- **A user-facing migration note / changelog entry.**
- The router golden file (`router_routes.txt`) and the contract suite (above)
  kept green throughout; they are the regression guards.

### 1.x cutover

Remove the obsolete routes and retire `soundtouch-player`. Per the versioning
section, this is the only point where anything is removed; the frozen
speaker/app routes stay.

## Versioning and the 1.x cutover

We do **not** version our own API in the path (`/api/v1/...`). In practice path
versioning buys little; its one real benefit is explicitness, and it can be
retrofitted later if a hard break ever forces it. Either way, a `/v1` -> `/v2`
bump does not remove the need to be careful when changing or breaking a route.

(The frozen `/v1/*` routes in the tables above are Bose's firmware contract, not
our versioning. They are unrelated.)

Versioning lives at the **release level (semver)** instead:

- **0.x (now):** the API may evolve. When a route moves, the **old and new paths
  stay live at the same time** (the alias/redirect layer from step 1). Every
  release stays a no-break upgrade, which gives users time to follow.
- **1.x (the cutover):** the release where we settle on the better API. At 1.x we
  **remove the obsolete routes**. That is the only point where an old route
  disappears.

Why this is low-risk: the service and the frontend(s) it serves ship in **one
binary**. A user updates the service and reloads the browser tab; the reloaded
SPA is the client for the new API, so the two always match, with no window where
an old frontend talks to a new backend.

Caveat: this holds for the clients we ship (the bundled UIs). Out-of-band callers
that hardcode paths (the CLI, user scripts, reverse-proxy rules, the HTTP-client
integration tests) must follow by 1.x as well; the 0.x dual-routing is precisely
the window that lets them. The frozen speaker/app contract routes are never
removed, 1.x included.

## Open questions

- Lockout-safety mechanism: which of refuse-to-apply / fall-back / loopback-on-
  host bypass we use when a mode requires auth but none is configured yet.
- The form of the non-interactive credential for native / headless clients:
  issued token, API key, device-code, or client-credentials grant.
- The shipped default at 1.x: stay `private`, or move to a stricter default
  (the parameter lets operators opt in regardless, so no hard flip is forced).
