# Bose SoundTouch Toolkit Documentation

Welcome to the documentation for the Bose SoundTouch Toolkit. This comprehensive toolkit helps you keep your Bose SoundTouch speakers functional even after the Bose Cloud shutdown in May 2026, with enhanced local management and monitoring capabilities.

## 🚀 Start Here

### For New Users
- **[Complete Migration Guide](guides/MIGRATION-GUIDE.md)** - Step-by-step guide from Bose Cloud to local control
- **[Getting Started](guides/GETTING-STARTED.md)** - Quick introduction to the toolkit

### For Existing Users
- **[Cloud Shutdown Survival Guide](guides/SURVIVAL-GUIDE.md)** - Prepare for the May 2026 shutdown
- **[Backup Tool](../cmd/soundtouch-backup/README.md)** - Back up your cloud account and speaker data before shutdown
- **[SoundTouch Service Guide](guides/SOUNDTOUCH-SERVICE.md)** - Advanced service configuration

## 📋 Essential Documentation

The documentation is organized into three main categories:

### 1. **User Guides** - For everyday users migrating and managing devices
### 2. **Technical Reference** - For developers and advanced configuration
### 3. **Concept Documentation** - For contributors and system architects

## 🗂 Documentation Structure

## 🗂 User Guides

### Migration & Setup
- **[Complete Migration Guide](guides/MIGRATION-GUIDE.md)** - 📖 **Main guide** for migrating from Bose Cloud
- [Cloud Shutdown Survival Guide](guides/SURVIVAL-GUIDE.md) - Prepare for service shutdown
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
- [Backup Tool](../cmd/soundtouch-backup/README.md) - Cloud account and speaker data backup
- [IoT Implementation Guide](guides/IOT-IMPLEMENTATION-GUIDE.md) - IoT integrations
- [MQTT Integration Design](guides/MQTT-INTEGRATION-DESIGN.md) - MQTT setup

## 📚 Technical Reference

### API Documentation
- [API Endpoints](reference/API-ENDPOINTS.md) - REST API reference
- [Spotify Account Addition](reference/spotify-account-addition.md) - Technical requests for Spotify
- [WebSocket Events](reference/WEBSOCKET-EVENTS.md) - Real-time events
- [Zone Management](reference/ZONE-MANAGEMENT.md) - Multi-room control
- [Preset Management](reference/PRESET-MANAGEMENT.md) - Preset operations

### Analysis & Research
- [Upstream URLs](analysis/UPSTREAM-URLS.md) - Bose service endpoints
- [Device Redirect Methods](analysis/DEVICE-REDIRECT-METHODS.md) - Migration techniques
- [IoT Configuration Analysis](analysis/IOT-CONFIGURATION-ANALYSIS.md) - Device configurations
- [IoT Config Summary](analysis/IOT-CONFIG-SUMMARY.md) - Configuration summaries

### Device Lifecycle & Network Independence
- **[Device Lifecycle and /power_on Enhancement](device-lifecycle-and-power-on-enhancement.md)** - Complete analysis of device registration and network independence improvements
- [/power_on Implementation Guide](power-on-implementation-guide.md) - Technical implementation details for enhanced device management

## 🏗 Concept Documentation

Current concept docs are listed under the **Concepts** section of [SUMMARY.md](SUMMARY.md#concepts). Highlights:

- [Spotify Overview](concepts/spotify-overview.md) — mental model, Spotify Connect vs OAuth-intercept, DNS rewrite gotcha
- [Spotify OAuth](concepts/spotify-oauth.md) — flows and management endpoints
- [Amazon Music OAuth](concepts/amazon-music-oauth.md) — companion to Spotify OAuth; same protocol shape, different scopes
- [Encrypted Export](concepts/ENCRYPTED-EXPORT.md) — `.age`-encrypted diagnostic bundles
- [Request Recording](REQUEST_RECORDING_CONCEPT.md) — how the proxy captures live device traffic for parity testing

Older planning artefacts ("Enhanced State Management System", "Upstream Service Simulation") live under [docs/archive/](archive/) — kept for the record, no longer current.

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

For a complete list of all documents, see the [Summary](SUMMARY.md).
