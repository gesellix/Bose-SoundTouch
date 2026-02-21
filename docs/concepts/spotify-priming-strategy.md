# Spotify Priming Strategy

This document outlines the strategy for ensuring Bose SoundTouch devices are correctly "primed" for Spotify Connect integration within the AfterTouch ecosystem.

## Overview

To enable Spotify Connect for SoundTouch devices, especially for remote availability outside the local network, the speaker must be associated with a Spotify account via a process called "priming." This involves sending an `addUser` command to the speaker's ZeroConf API (port 8200) containing a valid Spotify username and OAuth access token.

AfterTouch adopts a **Server-Centric Hybrid Model** that prioritizes device cleanliness and user intent while providing automated self-healing.

## Core Principles

### 1. User Intent (Opt-in)
AfterTouch replicates the native Bose "Add Source" experience. No Spotify priming occurs until a user explicitly links their Spotify account through the AfterTouch Management Dashboard. This ensures privacy and respects users who do not wish to use Spotify.

### 2. Device Cleanliness (Minimalist Footprint)
We avoid invasive modifications to the speaker's filesystem. 
- **No On-Device Scripts:** We deprecate the use of internal boot-primer scripts.
- **Native Communication:** We rely on the speaker's native ability to talk to Bose services, which are intercepted via DNS to point to the AfterTouch server.

### 3. Triggers for Priming
Priming is triggered when the speaker signals it is active and ready, specifically:

- **Power On:** When the speaker calls the `/marge/streaming/support/power_on` endpoint, AfterTouch ensures the device's ZeroConf state is correctly primed. This is the primary trigger.
- **Manual Override:** Users can manually trigger a "Prime Spotify" from the device list in the UI if needed.

During any of these events, the server:
1. Checks if a Spotify account is linked in AfterTouch.
2. Checks the device's current priming status (via ZeroConf).
3. If unprimed and an account is linked, it pushes the priming command.

### 4. Automated Recovery
AfterTouch ensures that if a speaker loses its session (due to a crash or power loss), it is re-primed when it next powers on and reaches out to the service.

### 5. Decoupling
The logic for account management and device interaction remains decoupled:
- **Spotify Service:** Manages OAuth tokens and account state.
- **Discovery Service:** Finds devices and tracks their network presence.
- **Orchestrator:** Connects the two, deciding when to push tokens to discovered devices based on the current link status.

## Workflow

### Initial Setup (The "Add Source" UX)
1. User opens the AfterTouch Dashboard.
2. User selects "Link Spotify Account."
3. OAuth flow completes; AfterTouch stores the token.
4. AfterTouch immediately triggers a discovery run to find and prime all compatible speakers.

### Maintenance (The "Watchdog" UX)
1. A speaker reboots or loses its token.
2. A discovery event occurs (periodic or triggered by UI).
3. AfterTouch detects the "Empty" user state on the speaker.
4. AfterTouch pushes a fresh token from the Spotify Service.
5. UI reflects that the device is "Managed by AfterTouch" and healthy.

### Manual Override
Users can manually trigger a "Re-prime" or "Refresh Link" from the device list in the UI if they suspect the automated self-healing is delayed or if they want to force a specific account onto a device.

## Network Topology & Deployment Scenarios

The strategy adapts based on where the AfterTouch server is deployed:

### Local Deployment (Home Server / Docker)
- **Mechanism:** Both "Pull" (Marge) and "Push" (ZeroConf side-channel) are used.
- **Advantage:** The server can proactively fix the speaker's state via port 8200 as soon as it sees a "Liveness Signal."

### External Deployment (Cloud VPS)
- **Mechanism:** Primarily relies on "Pull" (Marge).
- **Constraint:** The server cannot reach port 8200 on the speaker due to NAT/Firewall.
- **Strategy:** In this scenario, AfterTouch acts as a passive token provider. The speaker must initiate the connection to our intercepted Bose endpoints to receive its Spotify configuration. If the speaker completely loses its user state and stops "pulling," a manual re-prime from a local machine or a temporary local discovery run might be required.

## Transition & Cleanup

As AfterTouch moves to the Server-Centric model, we will:
1.  **Revert On-Device Migration:** Update the Setup Manager to remove legacy `spotify-boot-primer` scripts and `rc.local` hooks from the speakers.
2.  **Consolidated Directory:** We maintain the `/mnt/nv/soundtouch-service/` base directory for other configuration needs (e.g., `aftertouch.resolv.conf`), but it will no longer contain Spotify-specific credentials or scripts.
3.  **No On-Device Credentials:** The `/mnt/nv/soundtouch-service/spotify-primer.conf` will be removed, ensuring that no sensitive AfterTouch login details are stored on the speaker in plain text.

## Implementation Roadmap (Conceptual)

1. **Revert On-Device Migration:** Update the Setup Manager to remove legacy scripts and `rc.local` hooks.
2. **Server-Side Priming Logic:** Implement a `PrimeDevice(ip)` method in the server that fetches a fresh token and calls the ZeroConf API.
3. **Discovery Hook:** Integrate `PrimeDevice` into the discovery handler (`handleDiscoveredDevice`) with a check for unprimed state.
4. **UI Enhancements:** Update the Speaker List to show "Spotify Linked" status and provide manual refresh buttons.
