# Connecting Music Services (Spotify & Amazon Music)

This guide explains how to link your Spotify or Amazon Music account to AfterTouch so your speakers can stream music from those services.

> For Spotify, a higher-level mental model of how the integration works —
> Spotify Connect vs. AfterTouch's OAuth-intercept path, the
> `streamingoauth.bose.com` DNS gotcha, and the token lifecycle — is in
> [docs/concepts/spotify-overview.md](../concepts/spotify-overview.md).
> Read that if priming or playback isn't behaving as you'd expect.

---

## How it works

Connecting a music service happens in three separate steps, each done once:

1. **Register a developer app** with Spotify or Amazon (one-time setup by the person running AfterTouch).
2. **Authorize your personal account** so AfterTouch can access your music library.
3. **Prime your speaker** so the speaker itself learns about the source.

Each step is described below. If someone else is hosting AfterTouch for you, step 1 may already be done — ask them.

---

## Who registers the developer app?

The developer app is what allows AfterTouch to talk to Spotify's or Amazon's servers on behalf of users. It requires registering an account on a developer portal.

**If you run AfterTouch for yourself only:** You register one app and use it yourself.

**If you run AfterTouch for a group** (e.g., your household): You register one app, configure it in AfterTouch, and everyone who uses your AfterTouch instance shares it. They never see your app credentials — those stay on your server. However, they do need to trust you, since their music account tokens are stored by your AfterTouch installation.

**If you don't trust the AfterTouch operator:** Run your own AfterTouch instance and register your own app. That way everything stays under your control.

---

## Spotify

### Step 1: Register a Spotify developer app

1. Go to [developer.spotify.com/dashboard](https://developer.spotify.com/dashboard) and log in with your Spotify account.
2. Click **Create app**.
3. Give it any name and description (e.g., "My AfterTouch").
4. Under **Redirect URIs**, add:
   ```
   http://<your-aftertouch-ip>:8000/mgmt/spotify/callback
   ```
   Replace `<your-aftertouch-ip>:8000` with the address of your AfterTouch server.
5. Save the app.
6. Open the app's settings and note down the **Client ID** and **Client Secret**.

### Step 2: Enter credentials in AfterTouch

1. Open the AfterTouch web interface and go to the **Settings** tab.
2. Scroll to **Spotify Integration**.
3. Enter your **Client ID**, **Client Secret**, and the **Redirect URI** you registered above.
4. Click **Save Settings**.

The status should change to **Active**.

### Step 3: Authorize your Spotify account

1. Go to the **Local Account** tab (tab 7).
2. Click **Connect Spotify to this Account**.
3. A Spotify login window opens. Log in and grant permission.
4. When the window closes, your account is linked. You should see your Spotify username appear.

### Step 4: Prime your speaker

After authorizing, each speaker needs to be told about the Spotify source.

1. Go to the **Devices** tab (tab 2).
2. Find your speaker and click **Prime Spotify**.
3. The speaker will now show Spotify as an available source.

Repeat step 4 for each speaker.

---

## Amazon Music

> **Current status: account linking works, streaming does not.**
>
> The OAuth flow and token storage are fully functional. However, the speaker's `AmazonClient` contacts `music-api.amazon.com` directly with the access token and receives a 401. Amazon Music's streaming API requires scopes that are only available to registered Amazon Music partners — a standard Login with Amazon app does not qualify. The infrastructure is in place and will work if those scopes ever become available, but following these steps will not result in working Amazon Music playback today.

### Step 1: Register an Amazon developer app (LWA)

1. Go to [developer.amazon.com/loginwithamazon/console/site/lwa/overview.html](https://developer.amazon.com/loginwithamazon/console/site/lwa/overview.html) and log in with your Amazon account.
2. Click **Create a New Security Profile**.
3. Give it any name and description (e.g., "My AfterTouch").
4. In the security profile's **Web Settings**, add under **Allowed Return URLs**:
   ```
   http://<your-aftertouch-ip>:8000/mgmt/amazon/callback
   ```
   Replace `<your-aftertouch-ip>:8000` with the address of your AfterTouch server.
5. Save and note down the **Client ID** and **Client Secret**.

### Step 2: Enter credentials in AfterTouch

1. Open the AfterTouch web interface and go to the **Settings** tab.
2. Scroll to **Amazon Music Integration**.
3. Enter your **Client ID**, **Client Secret**, and the **Redirect URI** you registered above.
4. Click **Save Settings**.

The status should change to **Active**.

### Step 3: Authorize your Amazon account

1. Go to the **Local Account** tab (tab 7).
2. Click **Connect Amazon Music to this Account**.
3. An Amazon login window opens. Log in and grant permission.
4. When the window closes, your account is linked.

### Step 4: Prime your speaker

1. Go to the **Devices** tab (tab 2).
2. Find your speaker and click **Prime Amazon**.
3. The speaker will now show Amazon Music as an available source.

Repeat step 4 for each speaker.

---

## Troubleshooting

**"Failed to initialize" when clicking Connect:**
The app credentials in Settings are missing or incorrect. Double-check the Client ID, Client Secret, and Redirect URI. The Redirect URI in AfterTouch must exactly match the one registered in the developer portal.

**The login window opens but redirects to an error page:**
The Redirect URI registered with Spotify/Amazon does not match what AfterTouch is sending. Make sure the address (including the port) is identical in both places.

**The speaker doesn't show the new source after priming:**
Try rebooting the speaker. It may take a minute to update its source list after priming.

**The login window doesn't open (popup blocked):**
Allow popups from the AfterTouch address in your browser settings, then try again. Alternatively, the status message will show a direct link you can click.