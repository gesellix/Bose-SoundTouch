---
title: "Spotify Account Addition Implementation Status"
---
To fully replace Bose cloud services for the Spotify account addition flow in the "Stockholm" SoundTouch application, the following routes have been implemented in the `soundtouch-service`:

## 1. OAuth Token Exchange (Bose Cloud)

The Stockholm background worker (in `worker_common.js` and `spotify_worker.js`) performs a token exchange using an authorization code.

*   **Route**: `POST /oauth/account/{account}/music/musicprovider/{sourceID}/token/cs`
*   **Purpose**: To exchange the Spotify authorization code for a Bose-mediated token.
*   **Implementation**: `HandleBoseAccountToken` in `pkg/service/handlers/handlers_oauth.go`.
*   **Registration**: Registered in `cmd/soundtouch-service/main.go` under the `/oauth` route group.

## 2. Cloud Source Registration (Marge Service)

The SoundTouch application registers a new music source (e.g., Spotify) with the Bose cloud profile.

*   **Route**: `POST /streaming/account/{account}/source`
*   **Purpose**: To add the new source (username, credentials, display name) to the user's emulated cloud profile.
*   **Implementation**: `HandleMargeAddSource` in `pkg/service/handlers/handlers_marge.go`.
*   **Registration**: Registered in `cmd/soundtouch-service/main.go` under the `/streaming` route group.
*   **Payload Format**: XML `application/vnd.bose.streaming-v1.1+xml` containing `<source>` with `<username>`, `<sourceproviderid>`, and `<credential type="token_version_3">`.

## 3. Redirect Handling (Browser to App)

The `soundtouch://` deep link redirect URI is handled by the management interface which provides the OAuth callback.

*   **Callback Route**: `GET /mgmt/spotify/callback`
*   **Implementation**: `HandleMgmtSpotifyCallback` in `pkg/service/handlers/handlers_mgmt.go`.
*   **Confirmation Route**: `POST /mgmt/spotify/confirm` (used by mobile apps for deep-link codes).
*   **Implementation**: `HandleMgmtSpotifyConfirm` in `pkg/service/handlers/handlers_mgmt.go`.

## Implementation Details

1.  **Marge Add Source**:
    *   `HandleMargeAddSource` in `pkg/service/handlers/handlers_marge.go` parses the incoming XML and persists the new source to the `DataStore` for the corresponding account.

2.  **OAuth Account Token Exchange**:
    *   `HandleBoseAccountToken` in `pkg/service/handlers/handlers_oauth.go` supports the `/oauth/account/.../token/cs` path.
    *   It responds with a JSON payload including `access_token` and `token_type` "Bearer" after exchanging the code via `ExchangeCodeAndStore`.

3.  **Router Registration**:
    *   These paths are registered in `cmd/soundtouch-service/main.go` within the `/streaming`, `/oauth`, and `/mgmt` route blocks.
