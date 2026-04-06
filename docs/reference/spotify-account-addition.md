# Spotify Account Addition Technical Reference

This document details the exact network requests performed by the Bose SoundTouch "Stockholm" application and the SoundTouch speaker when adding a new Spotify account. This information is based on analysis of the Stockholm firmware version `27.0.13-4277-8963611`.

## Flow Overview

1.  **User Authorization Initiation**: The app opens the system browser to Spotify's authorization page.
2.  **Redirect Handling**: After authorization, Spotify redirects back to the app via a custom URI scheme, delivering an authorization `code`.
3.  **OAuth Token Exchange**: The app sends this `code` to the background worker, which exchanges it for a Bose-mediated token.
4.  **Cloud Source Registration**: The app registers the Spotify account as a "source" in the user's Bose Cloud (Marge) profile.
5.  **Local Device Sync**: The app notifies the local SoundTouch speaker about the new source, which then updates its internal configuration.

---

## 0. User Authorization Initiation

The process begins in the Stockholm UI when the user selects Spotify to add a new account.

### Request Details (App to Browser)
- **Action**: Open System Browser
- **Base URL**: `[SPOTIFY_AUTH_URL]` (e.g., `https://accounts.spotify.com/authorize`)
- **Query Parameters**:
    - `client_id`: Bose Spotify Client ID
    - `response_type`: `code`
    - `redirect_uri`: `http://localhost` (often used as a placeholder or specifically handled by the app's internal webview/proxy)
    - `scope`: `user-read-private user-read-email ...`
    - `state`: A base64-encoded JSON object containing metadata, e.g., `{"service": "SPOTIFY"}`.

### Redirect (Browser to App)
Upon successful login and authorization, Spotify redirects the browser to a URL that the SoundTouch app intercepts.

- **URL Format**: `soundtouch://bose/musicservice/spotify/login?code=[AUTH_CODE]&state=[STATE]`
- **App Action**: The `UIMain` component (in `ui_main.js`) handles this "deep link". It extracts the `code` from the query parameters and prepares to send it to the background worker.

---

## 1. OAuth Token Exchange (Bose Cloud)

After the UI intercepts the redirect and extracts the `code`, it sends a `createOAuthAccountRequest` to the background `SpotifyWorker`. The worker then performs the exchange for a Bose-mediated token.

### What is a "Bose-mediated token"?
The "Bose-mediated token" is a token issued by the Bose OAuth proxy. When the app (or device) requests a token via `oauth.streaming.bose.com`, Bose's service performs the actual OAuth2 exchange with Spotify.

- **It is not directly a Spotify refresh token**: Instead, it is a Bose-issued token that *represents* the underlying Spotify session.
- **Token Version 3**: Modern firmware uses `token_version_3`, which signifies that the device doesn't store the raw Spotify tokens but instead uses a Bose-specific "secret" that the Bose Cloud uses to fetch fresh Spotify access tokens on the device's behalf.
- **Access vs Refresh**: The initial response from the `.../token/cs` endpoint typically contains an `access_token` (valid for ~1 hour) and a `token_type: "Bearer"`. The Bose cloud service manages the persistent refresh token internally.

### Internal Message (UI to Worker)
- **Message Type**: `createOAuthAccountRequest`
- **Payload**:
    ```json
    {
      "source": "SPOTIFY",
      "code": "[AUTH_CODE_FROM_REDIRECT]",
      "credentialType": "token_version_3"
    }
    ```

### Outgoing Request (Worker to Bose OAuth Proxy)
- **Endpoint**: `https://oauth.streaming.bose.com/oauth/account/[ACCOUNT_ID]/music/musicprovider/15/token/cs`
- **Method**: `POST`
- **Headers**:
    - `Content-Type: application/json`
    - `Accept: application/json`
    - `Authorization: Bearer [SESSION_TOKEN]` (The user's Bose account session token)

### Payload (JSON)
```json
{
  "grant_type": "authorization_code",
  "code": "[AUTH_CODE_FROM_SPOTIFY]",
  "redirect_uri": "http://localhost"
}
```

### curl Example
```bash
curl -X POST "https://oauth.streaming.bose.com/oauth/account/[ACCOUNT_ID]/music/musicprovider/15/token/cs" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer [SESSION_TOKEN]" \
     -d '{
       "grant_type": "authorization_code",
       "code": "[AUTH_CODE_FROM_SPOTIFY]",
       "redirect_uri": "http://localhost"
     }'
```

---

## 2. Cloud Source Registration (Marge)

The app now registers the Spotify account with the Bose "Marge" service. This makes the source available across all devices linked to the same Bose account.

### Request Details
- **Endpoint**: `https://streaming.bose.com/streaming/account/[ACCOUNT_ID]/source`
- **Method**: `POST`
- **Headers**:
    - `Content-Type: application/vnd.bose.streaming-v1.1+xml`
    - `Authorization: [MARGE_TOKEN]`
    - `GUID: [DEVICE_GUID]`
    - `ClientType: Stockholm`

### Payload (XML)
```xml
<?xml version="1.0" encoding="UTF-8"?>
<source>
    <username>[SPOTIFY_USER_ID]</username>
    <sourceproviderid>15</sourceproviderid>
    <credential type="token_version_3">[SECRET_TOKEN_OBTAINED_IN_STEP_1]</credential>
    <sourcename>[DISPLAY_NAME_E_G_EMAIL]</sourcename>
</source>
```

### curl Example
```bash
curl -X POST "https://streaming.bose.com/streaming/account/[ACCOUNT_ID]/source" \
     -H "Content-Type: application/vnd.bose.streaming-v1.1+xml" \
     -H "Authorization: [MARGE_TOKEN]" \
     -d '<?xml version="1.0" encoding="UTF-8"?><source><username>[USER]</username><sourceproviderid>15</sourceproviderid><credential type="token_version_3">[TOKEN]</credential><sourcename>[NAME]</sourcename></source>'
```

---

### Local Device Sync (LISA API)

The app notifies the physical SoundTouch speaker about the new source. This is usually done via the device's management API on port 8090.

#### Modern Flow (OAuth)
- **Endpoint**: `http://[DEVICE_IP]:8090/setMusicServiceOAuthAccount`
- **Method**: `POST`
- **Payload**:
```xml
<OAuthCredentials source="SPOTIFY" displayName="[DISPLAY_NAME]">
    <user>[SPOTIFY_USER_ID]</user>
    <code>[AUTH_CODE_OR_TOKEN]</code>
    <version>token_version_3</version>
</OAuthCredentials>
```

#### Marge-Sync Notification (Fall-back)
If the speaker returns `1029 UNKNOWN_ACTION_ERROR`, it signifies the LISA API version is too old for the OAuth flow. Stockholm-based firmware often expects the account to be registered in Marge first, followed by a notification to sync.
- **Endpoint**: `http://[DEVICE_IP]:8090/notification`
- **Method**: `POST`
- **Payload**:
```xml
<updates deviceID="[DEVICE_UID]">
    <sourcesUpdated></sourcesUpdated>
</updates>
```

#### Legacy Flow (Fall-back)
For older firmware that doesn't use Marge for Spotify:
- **Endpoint**: `http://[DEVICE_IP]:8090/setMusicServiceAccount`
- **Method**: `POST`
- **Payload**:
```xml
<credentials source="SPOTIFY" displayName="Spotify Premium">
    <user>[USER]</user>
    <pass>[TOKEN]</pass>
</credentials>
```

---

## Implementation in SoundTouch-Service

This project implements the "Bose-mediated token" flow as follows:

1.  **Surrogate Secrets**: When a user links their Spotify account via `soundtouch-service`, the service generates a 32-character hex string (a "Bose Secret").
2.  **Marge & LISA registration**: This secret is sent to the speaker and stored in the emulated Marge cloud as the `credential`. The raw Spotify refresh token never leaves the server.
3.  **Token Refresh Proxy**: When the speaker needs a fresh Spotify `access_token`, it calls the `soundtouch-service` proxy (`/oauth/device/.../token/cs3`) providing this secret. The server maps the secret back to the actual Spotify account, performs the refresh with Spotify, and returns a fresh short-lived `access_token` to the speaker.

---

## Placeholders and Constants

| Placeholder       | Description                                         |
|:------------------|:----------------------------------------------------|
| `[ACCOUNT_ID]`    | The internal Bose account ID (UUID).                |
| `[SESSION_TOKEN]` | Temporary token from Bose login.                    |
| `[MARGE_TOKEN]`   | Persistent authorization token for Marge services.  |
| `[DEVICE_GUID]`   | Unique identifier for the controller app instance.  |
| `[DEVICE_IP]`     | Local IP address of the SoundTouch speaker.         |
| `15`              | Constant `sourceproviderid` for Spotify.            |
| `token_version_3` | Credential type for modern OAuth2 Spotify accounts. |

---

## Resulting Persistence

Once these requests succeed, the device updates its `/mnt/nv/BoseApp-Persistence/1/Sources.xml` file:

```xml
<source displayName="user@example.com" secret="[SECRET_BLOB]" secretType="token_version_3">
    <sourceKey type="SPOTIFY" account="user" />
</source>
```
