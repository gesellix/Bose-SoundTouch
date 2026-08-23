---
title: "SoundTouch CLI Reference"
---
**Complete command reference for the soundtouch-cli tool**

This document provides comprehensive documentation for all available commands and options in the `soundtouch-cli` tool.

## Overview

The SoundTouch CLI uses a hierarchical command structure with subcommands for different operations:

```bash
soundtouch-cli [global-flags] <command> [command-flags] [subcommand] [subcommand-flags]
```

## Global Flags

These flags can be used with any command:

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--host` | `-h` | Device IP address or hostname | Required for most commands |
| `--port` | `-p` | Device port number | `8090` |
| `--timeout` | `-t` | Request timeout duration | `10s` |
| `--help` | | Show command help | |
| `--version` | `-v` | Show CLI version | |

## Commands

### Discovery

Discover SoundTouch devices on the network.

#### `discover devices`

Discover and list all SoundTouch devices.

```bash
soundtouch-cli discover devices [flags]
```

**Flags:**
- `--all`, `-a`: Show detailed information for all devices
- `--timeout`: Discovery timeout (default: 10s)

**Examples:**
```bash
# Basic discovery
soundtouch-cli discover devices

# Show detailed info for all discovered devices
soundtouch-cli discover devices --all

# Discovery with custom timeout
soundtouch-cli discover devices --timeout 15s
```

### Device Information

Get information about your SoundTouch device.

#### `info`

Get basic device information.

```bash
soundtouch-cli --host <device> info
```

**Example:**
```bash
soundtouch-cli --host 192.0.2.10 info
```

#### `name get|set`

Get or set the device name.

```bash
# Get current name
soundtouch-cli --host <device> name get

# Set new name
soundtouch-cli --host <device> name set --value "My SoundTouch"
```

#### `capabilities`

Get device capabilities and features.

```bash
soundtouch-cli --host <device> capabilities
```

### Preset Management

Manage device presets (favorite content shortcuts).

#### `preset <subcommand>`

Preset management commands.

```bash
# List all presets
soundtouch-cli --host <device> preset list

# Store currently playing content as preset
soundtouch-cli --host <device> preset store-current --slot <1-6>

# Store specific content as preset
soundtouch-cli --host <device> preset store --slot <1-6> --source <SOURCE> --location <LOCATION> [options]

# Select and play a preset
soundtouch-cli --host <device> preset select --slot <1-6>

# Remove a preset
soundtouch-cli --host <device> preset remove --slot <1-6>
```

**Store Current Content Examples:**
```bash
# Store what's currently playing as preset 1
soundtouch-cli --host 192.0.2.10 preset store-current --slot 1

# Store current Spotify track as preset 3
soundtouch-cli --host 192.0.2.10 preset store-current --slot 3
```

**Store Specific Content Examples:**
```bash
# Store Spotify playlist
soundtouch-cli --host 192.0.2.10 preset store \
  --slot 1 \
  --source SPOTIFY \
  --location "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M" \
  --source-account "your_username" \
  --name "Today's Top Hits"

# Store radio station
soundtouch-cli --host 192.0.2.10 preset store \
  --slot 2 \
  --source TUNEIN \
  --location "/v1/playback/station/s33828" \
  --name "K-LOVE Radio"

# Store internet radio
soundtouch-cli --host 192.0.2.10 preset store \
  --slot 3 \
  --source LOCAL_INTERNET_RADIO \
  --location "https://stream.example.com/jazz" \
  --name "Jazz Radio Stream"
```

**Selection and Management Examples:**
```bash
# List all presets
soundtouch-cli --host 192.0.2.10 preset list

# Select preset 1
soundtouch-cli --host 192.0.2.10 preset select --slot 1

# Remove preset 6
soundtouch-cli --host 192.0.2.10 preset remove --slot 6
```

**Getting Content Locations:**

To find content locations for the `--location` parameter:

```bash
# Show current content details (includes location for all sources)
soundtouch-cli --host 192.0.2.10 play now

# Show detailed content information
soundtouch-cli --host 192.0.2.10 play now --verbose
```

### Recent Content

Recently played content management.

#### `recents <subcommand>`

Recently played content commands.

```bash
# List recently played items
soundtouch-cli --host <device> recents list [--limit <number>] [--detailed]

# Filter recent items by source or type
soundtouch-cli --host <device> recents filter --source <SOURCE> [--type <TYPE>] [--limit <number>]

# Show only the most recent item
soundtouch-cli --host <device> recents latest

# Show statistics about recent content
soundtouch-cli --host <device> recents stats
```

**Basic Usage Examples:**
```bash
# List last 10 recent items (default)
soundtouch-cli --host 192.0.2.10 recents list

# Show all recent items with detailed information
soundtouch-cli --host 192.0.2.10 recents list --limit 0 --detailed

# Show only the most recent item
soundtouch-cli --host 192.0.2.10 recents latest
```

**Filtering Examples:**
```bash
# Show only Spotify items
soundtouch-cli --host 192.0.2.10 recents filter --source SPOTIFY

# Show only tracks (no stations or playlists)
soundtouch-cli --host 192.0.2.10 recents filter --type track

# Show only presetable items
soundtouch-cli --host 192.0.2.10 recents filter --type presetable

# Show last 5 local music items
soundtouch-cli --host 192.0.2.10 recents filter --source LOCAL_MUSIC --limit 5
```

**Available Sources:**
- `SPOTIFY` - Spotify streaming
- `LOCAL_MUSIC` - Local music files
- `STORED_MUSIC` - Stored music library
- `TUNEIN` - TuneIn radio stations
- `PANDORA` - Pandora music
- `AMAZON` - Amazon Music
- `DEEZER` - Deezer streaming

**Available Types:**
- `track` - Individual songs
- `station` - Radio stations
- `playlist` - Music playlists
- `album` - Music albums
- `presetable` - Items that can be saved as presets

**Statistics Example:**
```bash
# Get detailed statistics about recent content
soundtouch-cli --host 192.0.2.10 recents stats
```

#### `presets` (Legacy)

Get configured presets (legacy command for backward compatibility).

```bash
soundtouch-cli --host <device> presets
```

### Playback Control

Control music playback on your device.

#### `play <subcommand>`

Playback control commands.

```bash
# Get current playback status
soundtouch-cli --host <device> play now

# Start playback
soundtouch-cli --host <device> play start

# Pause playback
soundtouch-cli --host <device> play pause

# Stop playback
soundtouch-cli --host <device> play stop

# Next track
soundtouch-cli --host <device> play next

# Previous track
soundtouch-cli --host <device> play prev
```

#### `preset`

Select a preset by number.

```bash
soundtouch-cli --host <device> preset --preset <1-6>
```

**Examples:**
```bash
# Select preset 1
soundtouch-cli --host 192.0.2.10 preset --preset 1

# Select preset 6
soundtouch-cli --host 192.0.2.10 preset --preset 6
```

#### `track`

Get current track information.

```bash
soundtouch-cli --host <device> track
```

### Key Commands

Send key commands to the device (simulates remote control).

#### `key <subcommand>`

Send various key commands.

```bash
# Send generic key command
soundtouch-cli --host <device> key send --key <KEY_NAME>

# Specific key commands
soundtouch-cli --host <device> key power
soundtouch-cli --host <device> key mute
soundtouch-cli --host <device> key thumbs-up
soundtouch-cli --host <device> key thumbs-down
soundtouch-cli --host <device> key volume-up
soundtouch-cli --host <device> key volume-down
```

**Available Key Names:**
- `PLAY`, `PAUSE`, `STOP`
- `POWER`, `MUTE`
- `VOLUME_UP`, `VOLUME_DOWN`
- `PRESET_1` through `PRESET_6`
- `NEXT_TRACK`, `PREV_TRACK`
- `THUMBS_UP`, `THUMBS_DOWN`
- `SHUFFLE_ON`, `SHUFFLE_OFF`
- `REPEAT_ON`, `REPEAT_OFF`

### Volume Control

Manage device volume.

#### `volume <subcommand>`

Volume control commands.

```bash
# Get current volume
soundtouch-cli --host <device> volume get

# Set specific volume level (0-100)
soundtouch-cli --host <device> volume set --level <0-100>

# Increase volume
soundtouch-cli --host <device> volume up [--amount <1-10>]

# Decrease volume
soundtouch-cli --host <device> volume down [--amount <1-10>]
```

**Examples:**
```bash
# Get volume
soundtouch-cli --host 192.0.2.10 volume get

# Set volume to 50
soundtouch-cli --host 192.0.2.10 volume set --level 50

# Increase volume by 5
soundtouch-cli --host 192.0.2.10 volume up --amount 5

# Decrease volume by 3 (default amount is 2)
soundtouch-cli --host 192.0.2.10 volume down --amount 3
```

### Audio Sources

Manage audio input sources.

#### `source <subcommand>`

Audio source commands.

```bash
# List available sources
soundtouch-cli --host <device> source list

# Select specific source
soundtouch-cli --host <device> source select --source <SOURCE> [--account <ACCOUNT>]

# Quick source selection
soundtouch-cli --host <device> source spotify
soundtouch-cli --host <device> source bluetooth
soundtouch-cli --host <device> source aux

# Custom radio selection (via soundtouch-service)
soundtouch-cli --host <device> source custom-radio --url <STREAM_URL> [--name <NAME>] [--artwork <ARTWORK>] [--service-url <SERVICE_URL>]

# Advanced content selection
soundtouch-cli --host <device> source internet-radio --location <URL> [--name <NAME>]
soundtouch-cli --host <device> source local-music --location <LOCATION> --account <ACCOUNT>
soundtouch-cli --host <device> source stored-music --location <LOCATION> --account <ACCOUNT>
soundtouch-cli --host <device> source content --source <SOURCE> --location <LOCATION>
```

**Source Names:**
- `SPOTIFY` - Spotify streaming
- `BLUETOOTH` - Bluetooth input
- `AUX` - AUX input
- `AIRPLAY` - AirPlay
- `LOCAL_MUSIC` - SoundTouch App Media Server content
- `LOCAL_INTERNET_RADIO` - Internet radio streams
- `STORED_MUSIC` - UPnP/DLNA media server content
- `TUNEIN` - TuneIn radio stations
- `PANDORA` - Pandora music service
- `PRODUCT` - Product-specific sources (TV, HDMI)

**Examples:**
```bash
# List all sources
soundtouch-cli --host 192.0.2.10 source list

# Select Spotify
soundtouch-cli --host 192.0.2.10 source spotify

# Select Spotify with specific account
soundtouch-cli --host 192.0.2.10 source select --source SPOTIFY --account user@example.com

# Select Bluetooth
soundtouch-cli --host 192.0.2.10 source bluetooth

# Select internet radio with streamUrl format
soundtouch-cli --host 192.0.2.10 source internet-radio \
  --location "http://contentapi.gmuth.de/station.php?name=MyStation&streamUrl=https://stream.example.com/radio" \
  --name "My Radio Station" \
  --artwork "https://example.com/art.png"

# Select internet radio with direct stream URL
soundtouch-cli --host 192.0.2.10 source internet-radio \
  --location "https://stream.example.com/radio" \
  --name "My Stream"

# Select local music content (requires SoundTouch App Media Server)
soundtouch-cli --host 192.0.2.10 source local-music \
  --location "album:983" \
  --account "3f205110-4a57-4e91-810a-123456789012" \
  --name "Welcome to the New"

# Select stored music content (requires UPnP/DLNA media server)
soundtouch-cli --host 192.0.2.10 source stored-music \
  --location "6_a2874b5d_4f83d999" \
  --account "d09708a1-5953-44bc-a413-123456789012/0" \
  --name "Christmas Album"

# Advanced content selection with all options
soundtouch-cli --host 192.0.2.10 source content \
  --source LOCAL_INTERNET_RADIO \
  --location "https://stream.example.com/radio" \
  --name "My Stream" \
  --type stationurl \
  --presetable

# Get introspect data for Spotify
soundtouch-cli --host 192.0.2.10 source introspect --source SPOTIFY

# Get introspect data with account
soundtouch-cli --host 192.0.2.10 source introspect --source SPOTIFY --account user@spotify.com

# Spotify introspect (convenience command)
soundtouch-cli --host 192.0.2.10 source introspect-spotify

# Get introspect data for all available services
soundtouch-cli --host 192.0.2.10 source introspect-all

# Check service availability
soundtouch-cli --host 192.0.2.10 source availability

# Compare sources and availability
soundtouch-cli --host 192.0.2.10 source compare
```

**Content Selection Commands:**

| Command | Description | Requirements |
|---------|-------------|--------------|
| `internet-radio` | Select internet radio stream (LOCAL_INTERNET_RADIO) | Stream URL |
| `custom-radio` | Select custom radio stream via soundtouch-service | Stream URL and service URL |
| `local-music` | Select local music content (LOCAL_MUSIC) | SoundTouch App Media Server |
| `stored-music` | Select stored music content (STORED_MUSIC) | UPnP/DLNA media server |
| `content` | Generic content selection (advanced) | Source and location |

**streamUrl Format Support:**

The `internet-radio` command supports the streamUrl proxy format from the [SoundTouch WebServices API Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API#select-local_internet_radio---streamurl-format):

```bash
# Using contentapi.gmuth.de proxy for complex streams
soundtouch-cli --host 192.0.2.10 source internet-radio \
  --location "http://contentapi.gmuth.de/station.php?name=Antenne%20Chillout&streamUrl=https://stream.antenne.de/chillout/stream/aacp" \
  --name "Antenne Chillout"

# Using local soundtouch-service for custom streams
soundtouch-cli --host 192.0.2.10 source custom-radio \
  --url "https://stream.antenne.de/chillout/stream/aacp" \
  --name "Antenne Chillout" \
  --service-url "http://localhost:8080"
```

#### Service Introspection

Get detailed information about music service states, user accounts, capabilities, and authentication status.

**Introspect Commands:**

```bash
# Get introspect data for specific service
soundtouch-cli --host <device> source introspect --source <SERVICE> [--account <ACCOUNT>]

# Spotify introspect (convenience)
soundtouch-cli --host <device> source introspect-spotify [--account <ACCOUNT>]

# Get introspect data for all services
soundtouch-cli --host <device> source introspect-all
```

**Supported Services for Introspect:**
- `SPOTIFY` - Spotify streaming service
- `PANDORA` - Pandora music service
- `TUNEIN` - TuneIn radio service
- `AMAZON` - Amazon Music service
- `DEEZER` - Deezer streaming service

**Introspect Information Includes:**
- Service state (Active, Inactive, InactiveUnselected)
- User account information
- Current playback status and content URI
- Service capabilities (skip, seek, resume support)
- Authentication token status
- Subscription type and content history limits
- Shuffle mode and data collection settings

**Examples:**
```bash
# Get Spotify service status
soundtouch-cli --host 192.0.2.10 source introspect --source SPOTIFY

# Get Spotify status with specific account
soundtouch-cli --host 192.0.2.10 source introspect --source SPOTIFY --account my_spotify_user

# Use Spotify convenience command
soundtouch-cli --host 192.0.2.10 source introspect-spotify

# Get status for all available streaming services
soundtouch-cli --host 192.0.2.10 source introspect-all

# Check which services are available before introspecting
soundtouch-cli --host 192.0.2.10 source availability
```

### Music Service Account Management

Manage music streaming service accounts and network music library connections.

#### `account <subcommand>`

Music service account management commands.

```bash
# List configured accounts
soundtouch-cli --host <device> account list

# Add music service account (generic)
soundtouch-cli --host <device> account add --source <SOURCE> --user <USER> --password <PASS> [--name <NAME>]

# Remove music service account (generic)
soundtouch-cli --host <device> account remove --source <SOURCE> --user <USER> [--name <NAME>]

# Service-specific convenience commands
soundtouch-cli --host <device> account add-spotify --user <EMAIL> --password <PASS>
soundtouch-cli --host <device> account add-pandora --user <USER> --password <PASS>
soundtouch-cli --host <device> account add-amazon --user <USER> --password <PASS>
soundtouch-cli --host <device> account add-deezer --user <USER> --password <PASS>
soundtouch-cli --host <device> account add-iheart --user <USER> --password <PASS>
soundtouch-cli --host <device> account add-nas --user <GUID/0> [--name <NAME>]

# Remove accounts
soundtouch-cli --host <device> account remove-spotify --user <EMAIL>
soundtouch-cli --host <device> account remove-pandora --user <USER>
soundtouch-cli --host <device> account remove-amazon --user <USER>
soundtouch-cli --host <device> account remove-deezer --user <USER>
soundtouch-cli --host <device> account remove-iheart --user <USER>
soundtouch-cli --host <device> account remove-nas --user <GUID/0> [--name <NAME>]

# Unpair the device from its Marge cloud account entirely
soundtouch-cli --host <device> account unpair
```

**Supported Services:**
- **SPOTIFY**: Spotify Premium accounts
- **PANDORA**: Pandora Music Service accounts
- **AMAZON**: Amazon Music accounts
- **DEEZER**: Deezer Premium accounts
- **IHEART**: iHeartRadio accounts
- **STORED_MUSIC**: Network music libraries (NAS/UPnP/DLNA servers)

**Examples:**
```bash
# List all configured music service accounts
soundtouch-cli --host 192.0.2.10 account list

# Add a Spotify Premium account
soundtouch-cli --host 192.0.2.10 account add-spotify \
  --user "user@spotify.com" \
  --password "mypassword"

# Add a Pandora account
soundtouch-cli --host 192.0.2.10 account add-pandora \
  --user "pandora_username" \
  --password "pandora_password"

# Add an Amazon Music account
soundtouch-cli --host 192.0.2.10 account add-amazon \
  --user "amazon_user" \
  --password "amazon_password"

# Add a network music library (NAS/UPnP)
soundtouch-cli --host 192.0.2.10 account add-nas \
  --user "d09708a1-5953-44bc-a413-123456789012/0" \
  --name "My Music Server"

# Remove a Spotify account
soundtouch-cli --host 192.0.2.10 account remove-spotify \
  --user "user@spotify.com"

# Generic account management
soundtouch-cli --host 192.0.2.10 account add \
  --source DEEZER \
  --user "deezer_user" \
  --password "deezer_pass" \
  --name "Deezer Premium"

soundtouch-cli --host 192.0.2.10 account remove \
  --source DEEZER \
  --user "deezer_user"
```

**Notes:**
- Music service accounts must be configured before you can browse or play content from those services
- Network music libraries (STORED_MUSIC) don't require passwords, only the UPnP server GUID
- After adding an account, use `source list` to verify it appears as available
- Some services may require additional authentication steps through their mobile apps
- `account unpair` is different from the above: it sends `UnPairDeviceWithAccount`
  over the speaker's own local WebSocket to remove its **Marge cloud account**
  pairing entirely (`margeAccountUUID`), not a single streaming-service login.
  See `setup revert` for the related "undo a migration" operation, which
  deliberately does *not* call this — the two are separate steps.

### Bass Control

Adjust bass levels (equalizer).

#### `bass <subcommand>`

Bass control commands.

```bash
# Get current bass level
soundtouch-cli --host <device> bass get

# Set bass level (-9 to 9)
soundtouch-cli --host <device> bass set --level <-9 to 9>

# Increase bass
soundtouch-cli --host <device> bass up [--amount <1-5>]

# Decrease bass
soundtouch-cli --host <device> bass down [--amount <1-5>]

# Get bass capabilities
soundtouch-cli --host <device> bass capabilities
```

**Examples:**
```bash
# Get current bass
soundtouch-cli --host 192.0.2.10 bass get

# Set bass to +3
soundtouch-cli --host 192.0.2.10 bass set --level 3

# Increase bass by 2
soundtouch-cli --host 192.0.2.10 bass up --amount 2

# Decrease bass by 1 (default)
soundtouch-cli --host 192.0.2.10 bass down
```

### Balance Control

Adjust left/right balance.

#### `balance <subcommand>`

Balance control commands.

```bash
# Get current balance
soundtouch-cli --host <device> balance get

# Set balance (-50 to 50, negative=left, positive=right)
soundtouch-cli --host <device> balance set --level <-50 to 50>

# Shift balance left
soundtouch-cli --host <device> balance left [--amount <1-10>]

# Shift balance right
soundtouch-cli --host <device> balance right [--amount <1-10>]

# Center balance
soundtouch-cli --host <device> balance center
```

**Examples:**
```bash
# Get balance
soundtouch-cli --host 192.0.2.10 balance get

# Set balance 10 units to the right
soundtouch-cli --host 192.0.2.10 balance set --level 10

# Shift left by 5 units (default)
soundtouch-cli --host 192.0.2.10 balance left

# Center the balance
soundtouch-cli --host 192.0.2.10 balance center
```

### Clock and Time

Manage device clock settings.

#### `clock <subcommand>`

Clock control commands.

```bash
# Get current time
soundtouch-cli --host <device> clock get

# Set time manually (HH:MM format)
soundtouch-cli --host <device> clock set --time "14:30"

# Set to current system time
soundtouch-cli --host <device> clock now

# Display settings
soundtouch-cli --host <device> clock display get
soundtouch-cli --host <device> clock display enable
soundtouch-cli --host <device> clock display disable
soundtouch-cli --host <device> clock display brightness --brightness <low|medium|high|off>
soundtouch-cli --host <device> clock display format --format <12|24>
```

**Examples:**
```bash
# Get current time
soundtouch-cli --host 192.0.2.10 clock get

# Set time to 2:30 PM
soundtouch-cli --host 192.0.2.10 clock set --time "14:30"

# Sync with system time
soundtouch-cli --host 192.0.2.10 clock now

# Enable clock display
soundtouch-cli --host 192.0.2.10 clock display enable

# Set 24-hour format
soundtouch-cli --host 192.0.2.10 clock display format --format 24

# Set high brightness
soundtouch-cli --host 192.0.2.10 clock display brightness --brightness high
```

### Network Information

Get network and connectivity information.

#### `network <subcommand>`

Network information commands.

```bash
# Get network information
soundtouch-cli --host <device> network info

# Ping the device
soundtouch-cli --host <device> network ping

# Get device base URL
soundtouch-cli --host <device> network url
```

### Zone Management

Manage multi-room zones (multiple speakers playing together).

#### `zone <subcommand>`

Zone management commands.

```bash
# Get current zone configuration
soundtouch-cli --host <device> zone get

# Get zone status
soundtouch-cli --host <device> zone status

# List zone members
soundtouch-cli --host <device> zone members

# Create new zone
soundtouch-cli --host <device> zone create --members <ip1,ip2,ip3>

# Add device to zone
soundtouch-cli --host <device> zone add --member <ip>

# Remove device from zone
soundtouch-cli --host <device> zone remove --member <ip>

# Dissolve current zone
soundtouch-cli --host <device> zone dissolve

# Set zone configuration
soundtouch-cli --host <device> zone set --master <ip> --members <ip1,ip2>
```

**Examples:**
```bash
# Get current zone info
soundtouch-cli --host 192.0.2.10 zone get

# Create zone with three speakers
soundtouch-cli --host 192.0.2.10 zone create --members 192.0.2.11,192.0.2.12

# Add speaker to existing zone
soundtouch-cli --host 192.0.2.10 zone add --member 192.0.2.13

# Remove speaker from zone
soundtouch-cli --host 192.0.2.10 zone remove --member 192.0.2.12

# Dissolve the zone (make all speakers independent)
soundtouch-cli --host 192.0.2.10 zone dissolve
```

### Stereo Pair Management

Create and manage a persistent LEFT/RIGHT pair of two SoundTouch 10 speakers.
This is distinct from a temporary multi-room zone. Both speakers must be
online, stereo-capable, standalone, and outside any zone before a lifecycle
operation. Pair creation also requires both speakers to use the same Marge
account and backend. Run lifecycle commands from the site containing both
speakers; site-relative Marge names such as `unifi` do not identify a remote
site when resolved by the CLI host.

```bash
# Inspect a standalone speaker or either member of a pair
soundtouch-cli --host 192.0.2.10 group status

# Create a pair; the LEFT speaker becomes the master
soundtouch-cli group create \
  --left 192.0.2.10 \
  --right 192.0.2.11 \
  --name "Living Room"

# Rename through either member
soundtouch-cli --host 192.0.2.10 group rename --name "Living Room Pair"

# Dissolve the pair without removing either speaker from AfterTouch
soundtouch-cli --host 192.0.2.10 group remove
```

Create, rename, and remove verify fresh state on both speakers. Rename and
remove first inspect the current group and carry its ID as a generation guard;
if the pair changes before mutation, the operation fails without touching the
newer pair. A partial transition is reported as degraded with per-speaker
details rather than as a successful operation. A remove attempt carries the
last exact L/R topology, freshly verifies both speakers, and retires
persistence only if the stored generation still matches it. Before create,
the CLI verifies
both speakers as standalone, queries their current Marge backend for stale
group records, and refuses to mutate either speaker while any record remains.
After verified physical cleanup, the CLI removes the exact group ID through the
Marge URL and account freshly read from the speaker. A backend cleanup failure
is therefore visible as a degraded result instead of leaving an apparently
successful stale generation.

### Browse and Navigation

Browse and navigate content sources on your device.

#### `browse <subcommand>`

Browse content from different sources.

```bash
# Browse TuneIn stations
soundtouch-cli --host <device> browse tunein

# Browse Pandora stations (requires account)
soundtouch-cli --host <device> browse pandora --source-account <pandora_account>

# Browse stored music library (requires device ID)
soundtouch-cli --host <device> browse stored-music --source-account <device_id>

# Browse any content source with pagination
soundtouch-cli --host <device> browse content --source <SOURCE> [--start <num>] [--limit <num>]

# Browse with menu navigation (for sources that support it)
soundtouch-cli --host <device> browse menu --source <SOURCE> --menu <MENU_TYPE> [--sort <SORT_ORDER>]

# Browse into a container/directory
soundtouch-cli --host <device> browse container --source <SOURCE> --location <LOCATION> [--type <TYPE>]
```

**Examples:**
```bash
# Browse TuneIn stations
soundtouch-cli --host 192.0.2.10 browse tunein

# Browse first 50 TuneIn stations
soundtouch-cli --host 192.0.2.10 browse tunein --limit 50

# Browse Pandora radio stations
soundtouch-cli --host 192.0.2.10 browse pandora --source-account myuser123

# Browse Pandora with menu navigation
soundtouch-cli --host 192.0.2.10 browse menu --source PANDORA --source-account myuser123 --menu radioStations --sort dateCreated

# Browse stored music library
soundtouch-cli --host 192.0.2.10 browse stored-music --source-account device_12345

# Browse into a music album container
soundtouch-cli --host 192.0.2.10 browse container --source STORED_MUSIC --location "album:983" --type dir
```

### Station Search and Management

Search for and manage radio stations and streaming content.

#### `station <subcommand>`

Search and manage stations.

##### Built-in search: the `find` family (recommended)

The `find` commands run the search **inside the CLI itself**, querying the
radio provider's public API directly. They need **neither the speaker's
cloud nor a running `soundtouch-service`**, and they don't require a
reachable speaker (`--host`) to search — so they keep working even after
the speaker's original cloud is gone. This is the recommended way to
search.

- `station find --provider tunein|radiobrowser --query <term>` — unified
  built-in search. `--provider` defaults to `tunein`.
- `station find-tunein --query <term>` — TuneIn sibling
  (= `find --provider tunein`).
- `station find-radiobrowser --query <term>` — Radio Browser sibling
  (= `find --provider radiobrowser`).
- `… --more` — follow up to three additional result pages when available
  (both TuneIn and Radio Browser paginate).

Results include each station's playback `Location`, which you can feed to
`source tunein` (TuneIn) or a preset/play flow.

```bash
# Unified built-in search (no speaker required)
soundtouch-cli station find --provider tunein --query "jazz"

# TuneIn sibling
soundtouch-cli station find-tunein --query "jazz"

# Radio Browser, walking extra result pages
soundtouch-cli station find-radiobrowser --query "jazz" --more
```

##### Deprecated: speaker-based search

These commands ask the **speaker** to search, which only works while the
speaker's cloud source is reachable. They are **deprecated** — each prints
a deprecation notice — and will be removed in a future release. Prefer the
`find` family above. There is no built-in equivalent for Pandora or
Spotify *yet*; those still require the speaker and your account.

```bash
# [DEPRECATED] Search across any source via the speaker → use `station find`
soundtouch-cli --host <device> station search --source <SOURCE> --query <SEARCH_TERM>

# [DEPRECATED] Search TuneIn via the speaker → use `station find-tunein`
soundtouch-cli --host <device> station search-tunein --query <SEARCH_TERM>

# [DEPRECATED] Search Pandora via the speaker (no built-in equivalent yet)
soundtouch-cli --host <device> station search-pandora --source-account <ACCOUNT> --query <SEARCH_TERM>

# [DEPRECATED] Search Spotify via the speaker (no built-in equivalent yet)
soundtouch-cli --host <device> station search-spotify --source-account <ACCOUNT> --query <SEARCH_TERM>
```

##### Manage stations

```bash
# Add station and play immediately
soundtouch-cli --host <device> station add --source <SOURCE> --token <TOKEN> --name <NAME>

# Remove station from collection
soundtouch-cli --host <device> station remove --source <SOURCE> --location <LOCATION>

# List saved stations for a source
soundtouch-cli --host <device> station list --source <SOURCE> [--source-account <ACCOUNT>]
```

**Station Management Examples:**
```bash
# Add a station found from search results (use token from search output)
soundtouch-cli --host 192.0.2.10 station add \
  --source TUNEIN \
  --token "c121508" \
  --name "Classic Rock Radio"

# Add Pandora station with account
soundtouch-cli --host 192.0.2.10 station add \
  --source PANDORA \
  --source-account myuser123 \
  --token "TR:12345" \
  --name "My Custom Station"

# Remove a station (use location from browse/search results)
soundtouch-cli --host 192.0.2.10 station remove \
  --source TUNEIN \
  --location "/v1/playback/station/s33828"
```

**Workflow Example - Discover and Play New Content:**
```bash
# 1. Search for content (built-in, no speaker needed)
soundtouch-cli station find-tunein --query "smooth jazz"

# 2. Add interesting station from results (copy token from output)
soundtouch-cli --host 192.0.2.10 station add \
  --source TUNEIN \
  --token "c456789" \
  --name "Smooth Jazz 24/7"

# 3. Station is automatically playing! Or browse for more options:
soundtouch-cli --host 192.0.2.10 browse tunein --limit 10
```

### Speaker Notifications and Content

Play notifications, TTS messages, and audio content (ST-10 Series only).

#### `speaker <subcommand>`

Speaker notification and content playback commands.

```bash
# Play Text-to-Speech message
soundtouch-cli --host <device> speaker tts --text <MESSAGE> --app-key <KEY> [--volume <LEVEL>] [--language <CODE>]

# Play audio content from URL
soundtouch-cli --host <device> speaker url --url <URL> --app-key <KEY> [--volume <LEVEL>] [--service <NAME>] [--message <MSG>] [--reason <REASON>]

# Play notification beep
soundtouch-cli --host <device> speaker beep

# Get detailed help about speaker functionality
soundtouch-cli speaker help
```

**TTS Examples:**
```bash
# Basic TTS in English
soundtouch-cli --host 192.0.2.10 speaker tts \
  --text "Hello, welcome home" \
  --app-key "your-app-key"

# TTS with volume and language
soundtouch-cli --host 192.0.2.10 speaker tts \
  --text "Bonjour le monde" \
  --app-key "your-app-key" \
  --volume 70 \
  --language FR

# TTS for home automation alert
soundtouch-cli --host 192.0.2.10 speaker tts \
  --text "Motion detected at front door" \
  --app-key "security-system-key" \
  --volume 80
```

**URL Content Examples:**
```bash
# Play audio file from URL
soundtouch-cli --host 192.0.2.10 speaker url \
  --url "https://example.com/doorbell.mp3" \
  --app-key "your-app-key" \
  --volume 75

# Play with custom metadata
soundtouch-cli --host 192.0.2.10 speaker url \
  --url "https://example.com/song.mp3" \
  --app-key "your-app-key" \
  --service "Music Service" \
  --message "Beautiful Song" \
  --reason "Artist Name" \
  --volume 60

# Emergency alert
soundtouch-cli --host 192.0.2.10 speaker url \
  --url "https://alerts.example.com/fire-alarm.wav" \
  --app-key "emergency-system" \
  --service "Emergency System" \
  --message "Fire Alert" \
  --volume 100
```

**Simple Notifications:**
```bash
# Quick beep notification
soundtouch-cli --host 192.0.2.10 speaker beep

# Test device connectivity with beep
soundtouch-cli --host 192.0.2.10 speaker beep
```

**Supported Languages for TTS:**
- `EN` - English (default)
- `DE` - German
- `ES` - Spanish
- `FR` - French
- `IT` - Italian
- `NL` - Dutch
- `PT` - Portuguese
- `RU` - Russian
- `ZH` - Chinese
- `JA` - Japanese

**Important Notes:**
- Only works with ST-10 (Series III) speakers
- ST-300 and other models may not support speaker notifications
- App key is required for TTS and URL playback (user-provided)
- Volume is automatically restored after notification completes
- Currently playing content is paused during notification and resumed after
- If device is zone master, notification plays on all zone members

### WebSocket Events

#### `events <subcommand>`

Real-time device event monitoring via WebSocket connection.

##### `events subscribe`

Subscribe to real-time device events and display them in the terminal.

**Usage:**
```bash
soundtouch-cli --host <device> events subscribe [flags]
```

**Flags:**
- `--filter, -f <types>` - Filter events by type (comma-separated)
- `--duration, -d <duration>` - How long to listen (0 = infinite)
- `--no-reconnect` - Disable automatic reconnection
- `--verbose, -v` - Enable verbose logging

**Event Types:**
- `nowPlaying` - Track changes, playback status
- `volume` - Volume and mute changes
- `connection` - Network connectivity status
- `preset` - Preset configuration changes
- `zone` - Multiroom zone changes
- `bass` - Bass level changes
- `sdkInfo` - SDK version information
- `userActivity` - User interaction notifications

**Examples:**
```bash
# Monitor all events
soundtouch-cli --host 192.0.2.10 events subscribe

# Monitor only volume and now playing events
soundtouch-cli --host 192.0.2.10 events subscribe --filter volume,nowPlaying

# Monitor for 5 minutes with verbose output
soundtouch-cli --host 192.0.2.10 events subscribe --duration 5m --verbose

# Monitor zone events without automatic reconnection
soundtouch-cli --host 192.0.2.10 events subscribe --filter zone --no-reconnect
```

**Notes:**
- WebSocket connection automatically reconnects on connection loss (unless disabled)
- Press Ctrl+C to stop monitoring
- Events are displayed in real-time with emoji indicators
- Verbose mode shows additional technical details

### Update Check

#### `update-check`

Check GitHub Releases for a newer `soundtouch-cli` version. Unlike
`soundtouch-service`'s periodic background check, this doesn't need a
`--host` or any device on the network: it's a single, on-demand GitHub API
request. Running the command is itself the opt-in, so there's no config
flag or persisted state.

**Usage:**
```bash
soundtouch-cli update-check
```

**Example output:**
```
A newer version is available: v1.3.0 (you're on v1.2.0)
https://github.com/gesellix/Bose-SoundTouch/releases/tag/v1.3.0
```

**Notes:**
- `soundtouch-backup` has the same `update-check` command.
- If the running binary isn't a released version (e.g. a dev build),
  the command reports that and skips the comparison.

### Setup & Migration

The `setup <subcommand>` group provisions a speaker end-to-end: enabling
SSH, factory-reset + Wi-Fi re-provisioning, pointing it at AfterTouch, CA
trust, account pairing, reverting, and one-shot data sync. Each subcommand
wraps an existing `pkg/service/setup` helper directly — there's no separate
business logic in the CLI layer. Manual provisioning-loop background:
[docs/analysis/SETUP-WEBSOCKET-EXPERIMENT.md](../analysis/SETUP-WEBSOCKET-EXPERIMENT.md)
and [Device Initial Setup](DEVICE-INITIAL-SETUP.md).

#### `setup inspect`

Non-destructive snapshot of the speaker: identity, pairing state, Wi-Fi,
sources, presets, and (with `--telnet`) the runtime URL configuration via
`getpdo`. Good first command to run against an unfamiliar speaker.

```bash
soundtouch-cli --host <device> setup inspect
soundtouch-cli --host <device> setup inspect --telnet   # also reads runtime URLs (slower)
```

#### `setup ssh-check`

Probes whether port 22 is reachable. On failure, prints the `enable-ssh`
suggestion and the USB-stick fallback procedure.

```bash
soundtouch-cli --host <device> setup ssh-check [--timeout 3s]
```

#### `setup enable-ssh`

Bootstraps SSH on a speaker with no prior access, via the port-17000
`envswitch` trick (#471) — no USB stick needed. Auto-pairs an unpaired
(factory-reset) device first by default (the injection needs something to
poll), waits for `:22`, and persists the `remote_services` marker so SSH
survives a reboot.

```bash
soundtouch-cli --host <device> setup enable-ssh
soundtouch-cli --host <device> setup enable-ssh --service-url https://192.0.2.10:8443
```

Flags:
- `--service-url` — optional; only the vehicle for the injection, no live
  server required. Set the real URL later via `setup migrate`.
- `--wait` (default `90s`) — how long to wait for `:22` after injection.
- `--full-config` — for stubborn devices (ST Portable, CineMate 520) where
  the default injection is accepted but `sshd` never starts: writes all
  four config URLs (the #515 sequence) and reboots.
- `--command-delay` — only affects `--full-config`; pause between its 6
  steps.
- `--no-auto-pair` / `--account` — skip or control the automatic pairing
  check.
- `--no-reset-urls` — skip restoring clean `boseurls` after SSH is up.
- `--no-persist` — skip persisting `remote_services` (SSH won't survive a
  reboot).
- `--authorized-key` — opt-in hardening: install an SSH public key instead
  of relying on the empty-password login.
- `--close-17000` — opt-in hardening: firewall off port 17000 from the LAN
  (loopback access kept).

#### `setup remote-services`

Enables (default) or removes the `remote_services` SSH-enablement marker.

```bash
soundtouch-cli --host <device> setup remote-services            # ensure it's present
soundtouch-cli --host <device> setup remote-services --remove   # disable SSH after next reboot
```

#### `setup factory-reset`

Issues `sys factorydefault` over telnet — wipes account, presets, and
Wi-Fi, and reboots the speaker into its own setup-mode AP. Prints the next
steps (`wait-ap`, then `wifi-push`).

```bash
soundtouch-cli --host <device> setup factory-reset
```

> **Heads-up:** just before resetting, the speaker sends
> `DELETE /streaming/account/{id}/device/{id}` to whatever `margeURL` is
> *currently* configured. If that still points at `streaming.bose.com`
> (not AfterTouch), AfterTouch keeps a stale datastore entry — migrate
> first if you want a clean record.

#### `setup wait-ap`

Polls the speaker's setup-mode AP (default `192.0.2.1`) until `/info`
responds, after a factory reset.

```bash
soundtouch-cli setup wait-ap [--ap-host 192.0.2.1] [--interval 2s] [--timeout 5m]
```

#### `setup wifi-push`

POSTs `AddWirelessProfile` to the speaker's setup-mode endpoint — pushes
your home Wi-Fi credentials while connected to the speaker's AP.

```bash
soundtouch-cli setup wifi-push --ssid="YourHomeSSID" --pass='your-password'
```

Flags: `--security` (default `wpa_or_wpa2`), `--ap-host` (default
`192.0.2.1`), `--request-timeout` (default `30s` — the speaker can be slow
to ACK before tearing down AP mode; 10s often races).

#### `setup wait-online`

Polls mDNS until a speaker matching `--match` comes online on the home
network — run this after switching back from the speaker's AP.

```bash
soundtouch-cli setup wait-online --match=<last-6-hex-of-deviceID>
```

`--match` is empty by default (first speaker seen); `--interval` (`3s`) and
`--timeout` (`5m`) control the poll.

#### `setup install-ca`

Fetches AfterTouch's CA cert from `/api/setup/ca.crt` and injects it into
the speaker's trust store via SSH.

```bash
soundtouch-cli --host <device> setup install-ca --service-url https://192.0.2.10:8443
```

`--auth` (`user:pass`) supplies basic-auth credentials up front; omit it to
be prompted interactively if the endpoint returns 401.

#### `setup migrate`

Applies a migration method to point the speaker at AfterTouch — the CLI
equivalent of the web UI's Migrate tab.

```bash
soundtouch-cli --host <device> setup migrate --service-url http://192.0.2.10:8000 --method telnet
```

`--method` is one of `telnet` (default) | `hosts` | `resolv` | `xml`.
`--proxy-url` sets an optional upstream proxy (only used by `--method=xml`).
`--skip-preflight` skips AfterTouch's settings preflight check (useful when
that endpoint is unreachable).

`--marge-url`/`--stats-url`/`--sw-update-url`/`--bmx-url` override the
corresponding field instead of deriving it from `--service-url` (applies to
both `--method=telnet` and `--method=xml`). Useful beyond soundcork-style
setups: e.g. pointing a speaker back at the **original Bose cloud URLs**
without a full `setup revert` — telnet writes both the runtime and
persisted layers in a single connection, no SSH or `.original` backup
needed:

```bash
soundtouch-cli --host <device> setup migrate --method telnet \
  --service-url https://streaming.bose.com \
  --marge-url https://streaming.bose.com \
  --stats-url https://events.api.bosecm.com \
  --sw-update-url https://worldwide.bose.com/updates/soundtouch \
  --bmx-url https://content.api.bose.io/bmx/registry/v1/services
```

#### `setup revert`

Undoes a migration. The default `--method ssh` is the CLI equivalent of the
web UI's **Revert to Defaults** button: it restores
`SoundTouchSdkPrivateCfg.xml`, `/etc/hosts`, and `/etc/resolv.conf` from their
`.original` backups, removes the AfterTouch DNS-hook artifacts, and strips
just the AfterTouch-labeled certificate out of the trust bundle.

```bash
soundtouch-cli --host <device> setup revert
```

For a telnet-only migration, `--method telnet` restores the four canonical
Bose service URLs without requiring SSH or an XML backup:

```bash
soundtouch-cli --host <device> setup revert --method telnet
```

This only changes `margeServerUrl`, `statsServerUrl`, `swUpdateUrl`, and
`bmxRegistryUrl`. It does not restore filesystem, DNS, CA, SSH, or account
state. Reboot the speaker afterwards and verify all four persisted values.
The `--marge-url`, `--stats-url`, `--sw-update-url`, and `--bmx-url` flags can
override the canonical defaults for firmware- or region-specific values.
These flags require `--method telnet`; using them with the default SSH method
is an error. Each value must be an absolute HTTP or HTTPS service URL without
userinfo, query parameters, fragments, whitespace, control characters, or
shell metacharacters.
Telnet writes are sequential rather than transactional. If the command reports
an error, read back and reconcile all four fields before retrying or rebooting;
the error distinguishes a partial runtime update from an uncertain persistence
outcome after `envswitch`.

**Out of scope for this command** (matches the web UI button): SSH /
`remote_services` persistence (use `setup remote-services --remove`) and
account pairing (use `account unpair`) are untouched — revert them
separately if you want a fully clean speaker.

#### `setup reboot`

Reboots the speaker — useful to force the envswitch parallel-persistence
layer to apply after a migration.

```bash
soundtouch-cli --host <device> setup reboot [--method telnet|ssh]
```

`--method` defaults to `telnet`, which works without SSH on modern
firmware.

#### `setup verify`

Read-only status probe across every migration axis (transports, URL
configuration, DNS interception, CA/TLS, pairing) — doubles as a preflight
check before applying changes and a verification step afterward. Exits
non-zero if nothing reports migrated, so it's usable as a CI gate.

```bash
soundtouch-cli --host <device> setup verify --service-url http://192.0.2.10:8000
```

#### `setup plan`

Recommends the next setup/migration steps based on `inspect` + `verify`
state — prints a ready-to-run command for each recommended step.

```bash
soundtouch-cli --host <device> setup plan --service-url http://192.0.2.10:8000
soundtouch-cli --host <device> setup plan --service-url http://192.0.2.10:8000 --reset   # plan a full factory-reset → Wi-Fi → migrate → pair flow
```

`--wifi-ssid` overrides the SSID used for the `wifi-push` step in a reset
plan (default: reuse the SSID `inspect` found). `--include-pair` (default
`true`) can be disabled if you'll pair manually.

#### `setup pair`

Pairs the speaker with an account via the WebSocket `SETUP` state machine
(`--mode=full`, matching the Bose app's own flow) or a minimal
`setMargeAccount`-only call (`--mode=bare`, the same underlying call the
Health tab's "empty margeAccountUUID" QuickFix uses).

```bash
soundtouch-cli --host <device> setup pair --mode=full --account=1111111 --service-url http://192.0.2.10:8000
soundtouch-cli --host <device> setup pair --mode=bare --account=1111111 --service-url http://192.0.2.10:8000
```

`--account` empty generates a fresh 7-digit ID. `--name` sets the speaker
name during pairing (empty keeps current). `--language` defaults to `3`
(English). `--token` defaults to a built-in placeholder matching the Bose
app's token shape.

`--mode=full` first reads `/supportedURLs` and `/soundTouchConfigurationStatus`
and only runs the state machine when the device reports
`SOUNDTOUCH_NOT_CONFIGURED` (see [#615](https://github.com/gesellix/Bose-SoundTouch/issues/615):
a speaker can be reachable, named, and already account-paired yet still
report `SOUNDTOUCH_NOT_CONFIGURED`, leaving the "install the Bose app"
prompt on screen — only a full pass through the state machine clears it).
An already-configured device is a no-op; an unsupported route or an
unrecognised status value fails the command instead of guessing.

#### `setup sync`

Pulls presets, recents, and sources from the speaker into AfterTouch's
datastore — the CLI equivalent of the web UI's Devices → Sync Data button.
Read-only towards the speaker: it never writes anything back.

```bash
soundtouch-cli --host <device> setup sync --service-url http://192.0.2.10:8000
```

`--auth` (`user:pass`) supplies basic-auth credentials up front; omit it to
be prompted interactively if the endpoint returns 401.

## Common Usage Patterns

### Quick Device Setup

```bash
# Discover devices
soundtouch-cli discover devices

# Get device info
soundtouch-cli --host 192.0.2.10 info

# Set comfortable volume and start playing
soundtouch-cli --host 192.0.2.10 volume set --level 30
soundtouch-cli --host 192.0.2.10 source spotify
soundtouch-cli --host 192.0.2.10 play start
```

### Daily Usage

```bash
# Morning routine
soundtouch-cli --host 192.0.2.10 preset --preset 1  # Morning playlist
soundtouch-cli --host 192.0.2.10 volume set --level 25

# Pause for a call
soundtouch-cli --host 192.0.2.10 play pause

# Resume
soundtouch-cli --host 192.0.2.10 play start

# Evening routine
soundtouch-cli --host 192.0.2.10 preset --preset 3  # Evening playlist
soundtouch-cli --host 192.0.2.10 volume set --level 15
```

### Multi-room Setup

```bash
# Create a zone with living room as master
soundtouch-cli --host 192.0.2.10 zone create --members 192.0.2.11,192.0.2.12

# Control the whole zone from master
soundtouch-cli --host 192.0.2.10 volume set --level 40
soundtouch-cli --host 192.0.2.10 source spotify
soundtouch-cli --host 192.0.2.10 preset --preset 2

# Later, dissolve the zone
soundtouch-cli --host 192.0.2.10 zone dissolve
```

### Audio Tuning

```bash
# Get current audio settings
soundtouch-cli --host 192.0.2.10 volume get
soundtouch-cli --host 192.0.2.10 bass get
soundtouch-cli --host 192.0.2.10 balance get

# Adjust for better sound
soundtouch-cli --host 192.0.2.10 bass set --level 2      # Slight bass boost
soundtouch-cli --host 192.0.2.10 balance set --level -5  # Slightly left
soundtouch-cli --host 192.0.2.10 volume set --level 35   # Good listening level
```

## Error Handling

The CLI provides clear error messages for common issues:

### Device Not Found
```
Error: Failed to connect to device: connection refused
```
**Solutions:**
- Check IP address is correct
- Ensure device is powered on
- Verify network connectivity with `soundtouch-cli --host <device> network ping`

### Invalid Commands
```
Error: unknown command "volumee" for "soundtouch-cli"
```
**Solution:** Check command spelling and structure using `--help`

### Missing Required Flags
```
Error: required flag "host" not set
```
**Solution:** Provide required flags: `--host <device>`

## Getting Help

```bash
# General help
soundtouch-cli --help

# Command-specific help
soundtouch-cli volume --help
soundtouch-cli zone --help

# Subcommand help
soundtouch-cli volume set --help
soundtouch-cli zone create --help
```

## Configuration

### Environment Variables

You can set default values using environment variables:

```bash
export SOUNDTOUCH_HOST=192.0.2.10
export SOUNDTOUCH_PORT=8090
export SOUNDTOUCH_TIMEOUT=15s

# Now you can omit these flags
soundtouch-cli info
soundtouch-cli volume get
```

### Configuration File

Create `~/.soundtouch.env`:

```
SOUNDTOUCH_HOST=192.0.2.10
SOUNDTOUCH_PORT=8090
SOUNDTOUCH_TIMEOUT=15s
SOUNDTOUCH_DISCOVERY_TIMEOUT=10s
```

## See Also

- [Getting Started Guide](GETTING-STARTED.md) - Basic setup and usage
- [WebSocket Events](../reference/WEBSOCKET-EVENTS.md) - Real-time monitoring
- [Zone Management](../reference/ZONE-MANAGEMENT.md) - Multi-room setup
- [API Endpoints](../reference/API-ENDPOINTS.md) - Complete API reference
