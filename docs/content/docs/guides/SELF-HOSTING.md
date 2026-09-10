---
title: "Self-Hosting AfterTouch"
---
This guide walks you through running AfterTouch on your own computer or server. No programming knowledge required.

---

## What is self-hosting?

AfterTouch is software that runs on a computer in your home and takes over the role of Bose's cloud servers. Your speakers talk to it instead of Bose.

For this to work, the computer running AfterTouch must be:

- **Always on** (or at least on whenever you want to use your speakers)
- **On the same local network** as your speakers
- **Reachable by a stable IP address** (see [Stable IP Address](#stable-ip-address) below)

Good choices: a Raspberry Pi, a NAS (like Synology or QNAP), an always-on PC or Mac, or a small server. A laptop that you close and put away is not ideal.

---

## Step 1: Get the software

See the **[Downloads page](../downloads/_index.md)** for the full list of builds and how to pick the right one for your system. You want the `soundtouch-service` tool; download the build whose suffix matches your computer:

| Your system                           | File to download                              |
|---------------------------------------|-----------------------------------------------|
| Raspberry Pi (64-bit)                 | `soundtouch-service-vX.Y.Z-linux-arm64`       |
| Raspberry Pi (32-bit)                 | `soundtouch-service-vX.Y.Z-linux-armv7`       |
| Older 32-bit ARM (NAS, Pi 1, Pi Zero) | `soundtouch-service-vX.Y.Z-linux-armv5`       |
| Linux (64-bit PC)                     | `soundtouch-service-vX.Y.Z-linux-amd64`       |
| macOS (Apple Silicon)                 | `soundtouch-service-vX.Y.Z-darwin-arm64`      |
| macOS (Intel)                         | `soundtouch-service-vX.Y.Z-darwin-amd64`      |
| Windows                               | `soundtouch-service-vX.Y.Z-windows-amd64.exe` |

(`X.Y.Z` is the current release version.) The download is a single ready-to-run executable called `soundtouch-service` (or `soundtouch-service.exe` on Windows) — no archive to extract.

### Alternative: Docker

If you already use Docker, you can run AfterTouch as a container instead. See the [Deployment Guide](DEPLOYMENT.md) for Docker instructions.

---

## Step 2: Run it

Open a terminal (or Command Prompt on Windows), navigate to the folder where you extracted the file, and run:

```
./soundtouch-service
```

On Windows:
```
soundtouch-service.exe
```

You should see log output like:
```
Starting AfterTouch service on :8000
```

AfterTouch is now running on port 8000.

---

## Step 3: Open the web interface

In a web browser on any device on your network, go to:

```
http://<your-server-ip>:8000
```

Replace `<your-server-ip>` with the actual IP address of the computer running AfterTouch. For example: `http://192.0.2.100:8000`.

If you are on the same computer that is running AfterTouch, you can use `http://localhost:8000`.

You should see the AfterTouch web interface with tabs: Overview, Settings, Devices, and so on.

---

## Step 4: Configure the server URL

This is the most important setting. Go to the **Settings** tab and set the **Target Domain** to the full address of your AfterTouch server — the same address you used to open the web interface:

```
http://192.0.2.100:8000
```

Use the IP address of your server, **not** `localhost`. Your speakers need to reach this address over the network, and they cannot resolve `localhost`.

Click **Save Settings**.

---

## Step 5: Proceed with migration

You are now ready to migrate your speakers. Follow the main [Migration Guide](MIGRATION-GUIDE.md) for the remaining steps (discovering devices, syncing data, and redirecting your speakers to AfterTouch).

---

## Keeping AfterTouch running

By default, AfterTouch stops when you close the terminal. To keep it running permanently:

**Raspberry Pi / Linux:** See the [Raspberry Pi Guide](RASPBERRY-PI.md) for instructions on running AfterTouch as a background service using `systemd`.

**NAS devices:** Most NAS systems support Docker. Use the Docker instructions in the [Deployment Guide](DEPLOYMENT.md).

**macOS:** You can use `launchd` to run AfterTouch at login. Creating a `launchd` plist is beyond this guide, but the [Deployment Guide](DEPLOYMENT.md) has a systemd example you can adapt.

**Windows:** You can use Task Scheduler to run AfterTouch at startup.

---

## Stable IP address

AfterTouch must always be reachable at the same address, because your speakers will be configured to point to it. If the IP changes, your speakers will stop working until you reconfigure them.

The easiest solution is to assign a **static (fixed) IP address** to the computer running AfterTouch in your router's settings. Look for "DHCP reservation" or "static IP" in your router's administration interface, and bind the server's MAC address to a fixed IP.

---

## Very old NAS boxes and kernels

A NAS you already own is often the most convenient place to run AfterTouch,
but some of them are genuinely old, and old shows up in two ways. Both look
alarming and both have a fix.

**"Illegal instruction" with no other output.** The `linux-armv7` build
contains floating-point instructions that ARMv5 and ARMv6 CPUs do not have,
so it dies the moment it starts. Use the `linux-armv5` build instead: it runs
on ARMv5, ARMv6 and ARMv7 alike. This is also the right download for a
Raspberry Pi 1 or Pi Zero. The install scripts pick it for you.

**`accept4: function not implemented`.** The service starts, says it is
listening, and then dies as soon as anything connects:

```
accept tcp [::]:8000: accept4: function not implemented
```

That is a kernel older than 2.6.36 on 32-bit ARM, which predates the
`accept4()` system call. AfterTouch detects this at startup and falls back to
the older `accept()` call automatically, so you should not have to do
anything; you will see a line about it in the log. If you need to switch the
fallback on or off by hand, set `AFTERTOUCH_ACCEPT_FALLBACK` to `1`, `0` or
`auto`.

**What is not promised.** Go itself supports Linux 3.2 and newer. Kernels
below that are outside that window and outside our CI, which has no hardware
that old to test on, so `accept4()` may not be the last missing piece.
AfterTouch is known to get as far as accepting connections there; if you hit
something further along, please open an issue with the exact error, your
`uname -a`, and what you were doing.

---

## Security note

The main web interface has no login by default — on a typical home network this is fine, since only devices on your local network can reach it.

The Management API (Spotify/Amazon account linking, the Local Accounts page) is a separate area that's *always* protected by HTTP Basic Auth, but ships with a published default (`admin` / `change_me!`) — anyone who has read the docs can use it. If you want real protection — for example, on a shared network — set your own:

```
./soundtouch-service --mgmt-username admin --mgmt-password yourpassword
```

See [Configuration Options](SOUNDTOUCH-SERVICE.md#configuration-options) for the full list of settings and env-var equivalents. Note that this does *not* cover the Settings tab, where your Spotify/Amazon Client ID and Secret are stored — that tab has no separate protection today.
