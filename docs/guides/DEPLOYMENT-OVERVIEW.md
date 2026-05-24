# AfterTouch Deployment Overview

AfterTouch replaces the Bose SoundTouch cloud, which shut down on 2026-05-06. There are two
ways to run it — pick the one that fits your situation.

---

## Which deployment is right for me?

|  | External host | On-device |
|--|---------------|-----------|
| **What it means** | AfterTouch runs on a separate computer (Raspberry Pi, NAS, PC) on your LAN. Speakers are pointed at that host. | AfterTouch runs directly on the SoundTouch speaker itself. No extra hardware. |
| **Extra hardware needed** | Yes — a Raspberry Pi or any always-on machine | No |
| **Multiple speakers** | Easy — one instance serves all speakers on the LAN | Each speaker needs its own install |
| **Invasiveness** | Low — only the speaker's server-URL config changes | Slightly higher — writes to the speaker's persistent storage |
| **Updates** | Update the host; speakers pick it up automatically | SSH into each speaker to update |
| **Good for** | Households with several speakers; users who want a central dashboard | Single-speaker households; users who don't have an always-on computer |

---

## Option A — External host (Raspberry Pi, NAS, PC)

The speaker stays unmodified. You run AfterTouch on a machine you already have
on your home network, then tell the speaker to use it instead of the Bose cloud.

| | Link |
|--|------|
| **User-friendly walkthrough** | [External Host Walkthrough](EXTERNAL-HOST-WALKTHROUGH.md) — step-by-step from install through preset setup |
| **Raspberry Pi quick-install** | [Raspberry Pi Guide](RASPBERRY-PI.md) — one-command installer, systemd integration |
| **Technical reference** | [Deployment Guide](DEPLOYMENT.md) — Docker, Kubernetes, systemd unit, configuration |

---

## Option B — On-device (AfterTouch on the speaker)

AfterTouch runs on the SoundTouch speaker itself. Requires one SSH session to
install; after that, the speaker self-hosts its own AfterTouch.

| | Link |
|--|------|
| **User-friendly walkthrough** | [On-Device Install Walkthrough](ON-DEVICE-INSTALL-WALKTHROUGH.md) — SSH connection through verified radio preset playback |
| **Installer reference** | [On-Device Installer README](../../scripts/on-device-install/README.md) — flags, paths, VERSION override, update/rollback |

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

For troubleshooting either deployment see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

---

## Architecture and planning documents

If you're a contributor or interested in the technical design decisions
(install patterns, user journeys, Gio/Wails tradeoffs, mini-build discussion),
see [docs/architecture/DEVICE-LOCAL-INSTALL.md](../architecture/DEVICE-LOCAL-INSTALL.md).
