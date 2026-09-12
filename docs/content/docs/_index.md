---
title: Introduction
sidebar:
  open: true
---

# Bose SoundTouch Toolkit Documentation

Bose shut down the SoundTouch cloud on 6 May 2026. This toolkit, **AfterTouch**,
runs the missing service on a machine in your own home, so your speakers keep
their presets, music browsing and stereo pairing.

## 🚀 Start here

### I want my speakers working again
- **[Getting Started](guides/GETTING-STARTED.md)** - the short route: run the service, migrate a speaker, get the preset buttons working
- **[Migration Guide](guides/MIGRATION-GUIDE.md)** - the full process, one step at a time
- **[What the shutdown broke](guides/SURVIVAL-GUIDE.md)** - what stopped working, what still works without AfterTouch, and what AfterTouch restores
- **[Get your preset buttons working again](guides/PRESETS.md)** - save stations, library folders and playlists to the six slots
- **[Troubleshooting](guides/TROUBLESHOOTING.md)** - when a speaker will not appear, or a source is missing

### I want to build on this
- **[Quick start: the Go client library](guides/GO-CLIENT-QUICKSTART.md)** - control a speaker from your own Go program
- **[API Endpoints](reference/API-ENDPOINTS.md)** - the service and speaker HTTP surface
- **[CLI Reference](guides/CLI-REFERENCE.md)** - every `soundtouch-cli` command

### Running it day to day
- **[SoundTouch Service Guide](guides/SOUNDTOUCH-SERVICE.md)** - service operation and configuration
- **[Backup Tool](https://github.com/gesellix/Bose-SoundTouch/blob/main/cmd/soundtouch-backup/README.md)** - back up speaker data and, if you still have one, a cloud account export

## 📋 Essential Documentation

The documentation is organized into three main categories:

### 1. **User Guides** - For everyday users migrating and managing devices
### 2. **Technical Reference** - For developers and advanced configuration
### 3. **Concept Documentation** - For contributors and system architects

## 🗂 Documentation Structure

## 🗂 User Guides

### Migration & Setup
- **[Complete Migration Guide](guides/MIGRATION-GUIDE.md)** - 📖 **Main guide** for migrating from Bose Cloud
- [Cloud Shutdown Survival Guide](guides/SURVIVAL-GUIDE.md) - What the shutdown broke and what AfterTouch restores
- [Migration & Safety Guide](guides/MIGRATION-SAFETY.md) - Advanced migration strategies
- [Initial Device Setup](guides/DEVICE-INITIAL-SETUP.md) - First-time device configuration
- [Raspberry Pi Setup](guides/RASPBERRY-PI.md) - Installing on Raspberry Pi

### Daily Management
- [SoundTouch Service Guide](guides/SOUNDTOUCH-SERVICE.md) - Service operation and maintenance
- [Troubleshooting](guides/TROUBLESHOOTING.md) - Common issues and solutions
- [HTTPS Setup](guides/HTTPS-SETUP.md) - Secure connections
- [Deployment Guide](guides/DEPLOYMENT.md) - Production deployments

### Advanced Features
- [MAC Address Mapping](guides/MAC-ADDRESS-MAPPING.md) - Device identification
- [CLI Reference](guides/CLI-REFERENCE.md) - Command-line tools
- [Backup Tool](https://github.com/gesellix/Bose-SoundTouch/blob/main/cmd/soundtouch-backup/README.md) - Cloud account and speaker data backup
- [IoT Implementation Guide](guides/IOT-IMPLEMENTATION-GUIDE.md) - IoT integrations
- [MQTT Integration Design](guides/MQTT-INTEGRATION-DESIGN.md) - MQTT setup

## 📚 Technical Reference

### API Documentation
- [API Endpoints](reference/API-ENDPOINTS.md) - REST API reference
- [Spotify Account Addition](reference/spotify-account-addition.md) - Technical requests for Spotify
- [WebSocket Events](reference/WEBSOCKET-EVENTS.md) - Real-time events
- [Zone Management](reference/ZONE-MANAGEMENT.md) - Multi-room control
- [Preset Management](reference/PRESET-MANAGEMENT.md) - Preset operations (the user guide is [Get your preset buttons working again](guides/PRESETS.md))

### Analysis & Research
- [Upstream URLs](analysis/UPSTREAM-URLS.md) - Bose service endpoints
- [Device Redirect Methods](analysis/DEVICE-REDIRECT-METHODS.md) - Migration techniques
- [IoT Configuration Analysis](analysis/IOT-CONFIGURATION-ANALYSIS.md) - Device configurations
- [IoT Config Summary](analysis/IOT-CONFIG-SUMMARY.md) - Configuration summaries

### Device Lifecycle & Network Independence
- **[Device Lifecycle and /power_on Enhancement](appendix/device-lifecycle-and-power-on-enhancement.md)** - Complete analysis of device registration and network independence improvements
- [/power_on Implementation Guide](appendix/power-on-implementation-guide.md) - Technical implementation details for enhanced device management

## 🏗 Concept Documentation

- [Spotify Overview](concepts/spotify-overview.md) — mental model, Spotify Connect vs OAuth-intercept, DNS rewrite gotcha
- [Spotify OAuth](concepts/spotify-oauth.md) — flows and management endpoints
- [Amazon Music OAuth](concepts/amazon-music-oauth.md) — companion to Spotify OAuth; same protocol shape, different scopes
- [Encrypted Export](concepts/ENCRYPTED-EXPORT.md) — `.age`-encrypted diagnostic bundles
- [Request Recording](appendix/REQUEST_RECORDING_CONCEPT.md) — how the proxy captures live device traffic for parity testing

Older planning artefacts ("Enhanced State Management System", "Upstream Service Simulation") live under [docs/archive/](../../archive/) — kept for the record, no longer current.

## 💡 Quick Reference

### Common Tasks
- **Migrate first device**: Follow [Migration Guide Step 5](guides/MIGRATION-GUIDE.md#step-5-migrate-individual-devices)
- **Check device health**: Dashboard → Devices → [Device Name] → Health Status
- **Backup configuration**: Dashboard → Settings → Backup → Create Backup
- **Add new device**: Dashboard → Devices → Discover Devices → Register

### Getting Help
- **Issues & Bugs**: [GitHub Issues](https://github.com/gesellix/Bose-SoundTouch/issues)
- **Questions & Discussion**: [GitHub Discussions](https://github.com/gesellix/Bose-SoundTouch/discussions)
- **Documentation**: Check troubleshooting guides first
- **Community**: Share experiences and help others
- **Direct chat (last resort)**: There's a small Discord for the rare case where an email exchange or an issue/discussion thread needs real-time back-and-forth. It's not a primary support channel: please start with Issues or Discussions. If a conversation genuinely needs it, ask in your thread and I'll share an invite.

For a complete list of all documents, browse the sections in the sidebar.
