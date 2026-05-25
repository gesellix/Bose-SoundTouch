---
title: "AfterTouch Deployment Overview"
---
AfterTouch replaces the Bose SoundTouch cloud, which shut down on 2026-05-06. There are
three ways to run it — pick the one that fits your situation.

---

## Which deployment is right for me?

|                       | Local network host                                              | Cloud / VPS                                                                     | On-device                                                                                                                         |
|-----------------------|-----------------------------------------------------------------|---------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| **What it means**     | AfterTouch runs on a Raspberry Pi, NAS, or PC on your home LAN. | AfterTouch runs on a remote server you own (Hetzner, DigitalOcean, Coolify, …). | AfterTouch runs directly on the SoundTouch speaker itself.                                                                        |
| **Extra hardware**    | Yes — an always-on machine at home                              | No — uses a server you already have                                             | No                                                                                                                                |
| **Multiple speakers** | Easy — one instance for all LAN speakers                        | Yes — one instance, managed remotely                                            | One install may serve multiple speakers if port 8000 is LAN-accessible (firmware-dependent; older devices may bind loopback only) |
| **Speaker migration** | Via the AfterTouch web UI                                       | Via `soundtouch-cli` on your local machine                                      | Via SSH into the speaker                                                                                                          |
| **HTTPS needed**      | No — HTTP on the LAN is fine                                    | **Yes** — speakers require a valid certificate                                  | No                                                                                                                                |
| **Updates**           | Update the host once                                            | Update the server once                                                          | SSH into each speaker                                                                                                             |
| **Good for**          | Most households; want a central dashboard                       | No always-on home machine; already have a VPS                                   | Single-speaker; no extra hardware at all                                                                                          |

---

## Option A — Local network host (Raspberry Pi, NAS, PC)

Run AfterTouch on a machine already on your home network. The speaker is pointed at it
via a simple URL change — nothing else on the speaker is modified.

|                                | Link                                                                                               |
|--------------------------------|----------------------------------------------------------------------------------------------------|
| **User-friendly walkthrough**  | [External Host Walkthrough](EXTERNAL-HOST-WALKTHROUGH.md) — install → discover → migrate → presets |
| **Raspberry Pi quick-install** | [Raspberry Pi Guide](RASPBERRY-PI.md) — one-command installer, systemd integration                 |
| **Technical reference**        | [Deployment Guide](DEPLOYMENT.md) — Docker, Kubernetes, systemd unit, configuration                |

---

## Option B — Cloud / VPS

Run AfterTouch on a public server. Because the server can't reach your LAN via mDNS,
speaker migration is done with `soundtouch-cli` from your local machine, and HTTPS with
a real certificate is required. Read the security notes in the walkthrough before
exposing AfterTouch to the internet.

|                               | Link                                                                                                                    |
|-------------------------------|-------------------------------------------------------------------------------------------------------------------------|
| **User-friendly walkthrough** | [Cloud Deploy Walkthrough](CLOUD-DEPLOY-WALKTHROUGH.md) — VPS setup, CLI migration, TuneIn gotcha                       |
| **Technical reference**       | [Deployment Guide](DEPLOYMENT.md) — Docker Compose, environment variables, reverse proxy                                |
| **Community field report**    | [discussion #295](https://github.com/gesellix/Bose-SoundTouch/discussions/295) — Hetzner + Coolify setup by wimdeblauwe |

---

## Option C — On-device (AfterTouch on the speaker)

AfterTouch runs on the SoundTouch speaker itself. Requires one SSH session to install;
after that, the speaker self-hosts its own AfterTouch. Delivers the **complete AfterTouch
feature set** without any extra hardware.

|                               | Link                                                                                                                      |
|-------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| **User-friendly walkthrough** | [On-Device Install Walkthrough](ON-DEVICE-INSTALL-WALKTHROUGH.md) — SSH connection through verified radio preset playback |
| **Installer reference**       | [On-Device Installer README](https://github.com/gesellix/Bose-SoundTouch/blob/main/scripts/on-device-install/README.md) — flags, paths, VERSION override, update/rollback |

---

## After choosing a deployment path

Once AfterTouch is running and your speaker is migrated, the next steps are the
same regardless of which deployment you chose:

- **Health tab** — open the AfterTouch UI → Health, and run any QuickFixes shown
  (especially *"empty margeAccountUUID"* if present).
- **Music sources** — the Health tab also shows whether Internet Radio, TuneIn,
  and Radio Browser are active.
- **Presets** — use the AfterTouch web UI or `soundtouch-cli preset store-current`
  to program the physical preset buttons.

For troubleshooting any deployment see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

---

## Architecture and planning documents

If you're a contributor or interested in the technical design decisions
(install patterns, user journeys, Gio/Wails tradeoffs, mini-build discussion),
see [docs/architecture/DEVICE-LOCAL-INSTALL.md](../architecture/DEVICE-LOCAL-INSTALL.md).
