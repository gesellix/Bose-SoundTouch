# SoundTouch CLI Reference

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
soundtouch-cli --host 192.168.1.10 info
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
soundtouch-cli --host 192.168.1.10 preset store-current --slot 1

# Store current Spotify track as preset 3
soundtouch-cli --host 192.168.1.10 preset store-current --slot 3
```

**Store Specific Content Examples:**
```bash
# Store Spotify playlist
soundtouch-cli --host 192.168.1.10 preset store \
  --slot 1 \
  --source SPOTIFY \
  --location "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M" \
  --source-account "your_username" \
  --name "Today's Top Hits"

# Store radio station
soundtouch-cli --host 192.168.1.10 preset store \
  --slot 2 \
  --source TUNEIN \
  --location "/v1/playbook/station/s33828" \
  --name "K-LOVE Radio"

# Store internet radio
soundtouch-cli --host 192.168.1.10 preset store \
  --slot 3 \
  --source LOCAL_INTERNET_RADIO \
  --location "https://stream.example.com/jazz" \
  --name "Jazz Radio Stream"
```

**Selection and Management Examples:**
```bash
# List all presets
soundtouch-cli --host 192.168.1.10 preset list

# Select preset 1
soundtouch-cli --host 192.168.1.10 preset select --slot 1

# Remove preset 6
soundtouch-cli --host 192.168.1.10 preset remove --slot 6
```

**Getting Content Locations:**

To find content locations for the `--location` parameter:

```bash
# Show current content details (includes location for all sources)
soundtouch-cli --host 192.168.1.10 play now

# Show detailed content information
soundtouch-cli --host 192.168.1.10 play now --verbose
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
soundtouch-cli --host 192.168.1.10 recents list

# Show all recent items with detailed information
soundtouch-cli --host 192.168.1.10 recents list --limit 0 --detailed

# Show only the most recent item
soundtouch-cli --host 192.168.1.10 recents latest
```

**Filtering Examples:**
```bash
# Show only Spotify items
soundtouch-cli --host 192.168.1.10 recents filter --source SPOTIFY

# Show only tracks (no stations or playlists)
soundtouch-cli --host 192.168.1.10 recents filter --type track

# Show only presetable items
soundtouch-cli --host 192.168.1.10 recents filter --type presetable

# Show last 5 local music items
soundtouch-cli --host 192.168.1.10 recents filter --source LOCAL_MUSIC --limit 5
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
soundtouch-cli --host 192.168.1.10 recents stats
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
soundtouch-cli --host 192.168.1.10 preset --preset 1

# Select preset 6
soundtouch-cli --host 192.168.1.10 preset --preset 6
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
soundtouch-cli --host 192.168.1.10 volume get

# Set volume to 50
soundtouch-cli --host 192.168.1.10 volume set --level 50

# Increase volume by 5
soundtouch-cli --host 192.168.1.10 volume up --amount 5

# Decrease volume by 3 (default amount is 2)
soundtouch-cli --host 192.168.1.10 volume down --amount 3
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
soundtouch-cli --host 192.168.1.10 source list

# Select Spotify
soundtouch-cli --host 192.168.1.10 source spotify

# Select Spotify with specific account
soundtouch-cli --host 192.168.1.10 source select --source SPOTIFY --account user@example.com

# Select Bluetooth
soundtouch-cli --host 192.168.1.10 source bluetooth

# Select internet radio with streamUrl format
soundtouch-cli --host 192.168.1.10 source internet-radio \
  --location "http://contentapi.gmuth.de/station.php?name=MyStation&streamUrl=https://stream.example.com/radio" \
  --name "My Radio Station" \
  --artwork "https://example.com/art.png"

# Select internet radio with direct stream URL
soundtouch-cli --host 192.168.1.10 source internet-radio \
  --location "https://stream.example.com/radio" \
  --name "My Stream"

# Select local music content (requires SoundTouch App Media Server)
soundtouch-cli --host 192.168.1.10 source local-music \
  --location "album:983" \
  --account "3f205110-4a57-4e91-810a-123456789012" \
  --name "Welcome to the New"

# Select stored music content (requires UPnP/DLNA media server)
soundtouch-cli --host 192.168.1.10 source stored-music \
  --location "6_a2874b5d_4f83d999" \
  --account "d09708a1-5953-44bc-a413-123456789012/0" \
  --name "Christmas Album"

# Advanced content selection with all options
soundtouch-cli --host 192.168.1.10 source content \
  --source LOCAL_INTERNET_RADIO \
  --location "https://stream.example.com/radio" \
  --name "My Stream" \
  --type stationurl \
  --presetable

# Get introspect data for Spotify
soundtouch-cli --host 192.168.1.10 source introspect --source SPOTIFY

# Get introspect data with account
soundtouch-cli --host 192.168.1.10 source introspect --source SPOTIFY --account user@spotify.com

# Spotify introspect (convenience command)
soundtouch-cli --host 192.168.1.10 source introspect-spotify

# Get introspect data for all available services
soundtouch-cli --host 192.168.1.10 source introspect-all

# Check service availability
soundtouch-cli --host 192.168.1.10 source availability

# Compare sources and availability
soundtouch-cli --host 192.168.1.10 source compare
```

**Content Selection Commands:**

| Command | Description | Requirements |
|---------|-------------|--------------|
| `internet-radio` | Select internet radio stream (LOCAL_INTERNET_RADIO) | Stream URL |
| `local-music` | Select local music content (LOCAL_MUSIC) | SoundTouch App Media Server |
| `stored-music` | Select stored music content (STORED_MUSIC) | UPnP/DLNA media server |
| `content` | Generic content selection (advanced) | Source and location |

**streamUrl Format Support:**

The `internet-radio` command supports the streamUrl proxy format from the [SoundTouch WebServices API Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API#select-local_internet_radio---streamurl-format):

```bash
# Using contentapi.gmuth.de proxy for complex streams
soundtouch-cli --host 192.168.1.10 source internet-radio \
  --location "http://contentapi.gmuth.de/station.php?name=Antenne%20Chillout&streamUrl=https://stream.antenne.de/chillout/stream/aacp" \
  --name "Antenne Chillout"
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
soundtouch-cli --host 192.168.1.10 source introspect --source SPOTIFY

# Get Spotify status with specific account
soundtouch-cli --host 192.168.1.10 source introspect --source SPOTIFY --account my_spotify_user

# Use Spotify convenience command
soundtouch-cli --host 192.168.1.10 source introspect-spotify

# Get status for all available streaming services
soundtouch-cli --host 192.168.1.10 source introspect-all

# Check which services are available before introspecting
soundtouch-cli --host 192.168.1.10 source availability
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
soundtouch-cli --host 192.168.1.10 account list

# Add a Spotify Premium account
soundtouch-cli --host 192.168.1.10 account add-spotify \
  --user "user@spotify.com" \
  --password "mypassword"

# Add a Pandora account
soundtouch-cli --host 192.168.1.10 account add-pandora \
  --user "pandora_username" \
  --password "pandora_password"

# Add an Amazon Music account
soundtouch-cli --host 192.168.1.10 account add-amazon \
  --user "amazon_user" \
  --password "amazon_password"

# Add a network music library (NAS/UPnP)
soundtouch-cli --host 192.168.1.10 account add-nas \
  --user "d09708a1-5953-44bc-a413-123456789012/0" \
  --name "My Music Server"

# Remove a Spotify account
soundtouch-cli --host 192.168.1.10 account remove-spotify \
  --user "user@spotify.com"

# Generic account management
soundtouch-cli --host 192.168.1.10 account add \
  --source DEEZER \
  --user "deezer_user" \
  --password "deezer_pass" \
  --name "Deezer Premium"

soundtouch-cli --host 192.168.1.10 account remove \
  --source DEEZER \
  --user "deezer_user"
```

**Notes:**
- Music service accounts must be configured before you can browse or play content from those services
- Network music libraries (STORED_MUSIC) don't require passwords, only the UPnP server GUID
- After adding an account, use `source list` to verify it appears as available
- Some services may require additional authentication steps through their mobile apps

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
soundtouch-cli --host 192.168.1.10 bass get

# Set bass to +3
soundtouch-cli --host 192.168.1.10 bass set --level 3

# Increase bass by 2
soundtouch-cli --host 192.168.1.10 bass up --amount 2

# Decrease bass by 1 (default)
soundtouch-cli --host 192.168.1.10 bass down
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
soundtouch-cli --host 192.168.1.10 balance get

# Set balance 10 units to the right
soundtouch-cli --host 192.168.1.10 balance set --level 10

# Shift left by 5 units (default)
soundtouch-cli --host 192.168.1.10 balance left

# Center the balance
soundtouch-cli --host 192.168.1.10 balance center
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
soundtouch-cli --host 192.168.1.10 clock get

# Set time to 2:30 PM
soundtouch-cli --host 192.168.1.10 clock set --time "14:30"

# Sync with system time
soundtouch-cli --host 192.168.1.10 clock now

# Enable clock display
soundtouch-cli --host 192.168.1.10 clock display enable

# Set 24-hour format
soundtouch-cli --host 192.168.1.10 clock display format --format 24

# Set high brightness
soundtouch-cli --host 192.168.1.10 clock display brightness --brightness high
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
soundtouch-cli --host 192.168.1.10 zone get

# Create zone with three speakers
soundtouch-cli --host 192.168.1.10 zone create --members 192.168.1.11,192.168.1.12

# Add speaker to existing zone
soundtouch-cli --host 192.168.1.10 zone add --member 192.168.1.13

# Remove speaker from zone
soundtouch-cli --host 192.168.1.10 zone remove --member 192.168.1.12

# Dissolve the zone (make all speakers independent)
soundtouch-cli --host 192.168.1.10 zone dissolve
```

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
soundtouch-cli --host 192.168.1.10 browse tunein

# Browse first 50 TuneIn stations
soundtouch-cli --host 192.168.1.10 browse tunein --limit 50

# Browse Pandora radio stations
soundtouch-cli --host 192.168.1.10 browse pandora --source-account myuser123

# Browse Pandora with menu navigation
soundtouch-cli --host 192.168.1.10 browse menu --source PANDORA --source-account myuser123 --menu radioStations --sort dateCreated

# Browse stored music library
soundtouch-cli --host 192.168.1.10 browse stored-music --source-account device_12345

# Browse into a music album container
soundtouch-cli --host 192.168.1.10 browse container --source STORED_MUSIC --location "album:983" --type dir
```

### Station Search and Management

Search for and manage radio stations and streaming content.

#### `station <subcommand>`

Search and manage stations.

```bash
# Search across any source
soundtouch-cli --host <device> station search --source <SOURCE> --query <SEARCH_TERM>

# Search TuneIn specifically
soundtouch-cli --host <device> station search-tunein --query <SEARCH_TERM>

# Search Pandora specifically (requires account)
soundtouch-cli --host <device> station search-pandora --source-account <ACCOUNT> --query <SEARCH_TERM>

# Search Spotify specifically (requires account)
soundtouch-cli --host <device> station search-spotify --source-account <ACCOUNT> --query <SEARCH_TERM>

# Add station and play immediately
soundtouch-cli --host <device> station add --source <SOURCE> --token <TOKEN> --name <NAME>

# Remove station from collection
soundtouch-cli --host <device> station remove --source <SOURCE> --location <LOCATION>
```

**Search Examples:**
```bash
# Search TuneIn for jazz stations
soundtouch-cli --host 192.168.1.10 station search-tunein --query "jazz"

# Search Pandora for Taylor Swift
soundtouch-cli --host 192.168.1.10 station search-pandora --source-account myuser123 --query "Taylor Swift"

# Search Spotify for workout playlists
soundtouch-cli --host 192.168.1.10 station search-spotify --source-account spotify_user --query "workout playlist"

# General search across any source
soundtouch-cli --host 192.168.1.10 station search --source TUNEIN --query "classic rock"
```

**Station Management Examples:**
```bash
# Add a station found from search results (use token from search output)
soundtouch-cli --host 192.168.1.10 station add \
  --source TUNEIN \
  --token "c121508" \
  --name "Classic Rock Radio"

# Add Pandora station with account
soundtouch-cli --host 192.168.1.10 station add \
  --source PANDORA \
  --source-account myuser123 \
  --token "TR:12345" \
  --name "My Custom Station"

# Remove a station (use location from browse/search results)
soundtouch-cli --host 192.168.1.10 station remove \
  --source TUNEIN \
  --location "/v1/playbook/station/s33828"
```

**Workflow Example - Discover and Play New Content:**
```bash
# 1. Search for content
soundtouch-cli --host 192.168.1.10 station search-tunein --query "smooth jazz"

# 2. Add interesting station from results (copy token from output)
soundtouch-cli --host 192.168.1.10 station add \
  --source TUNEIN \
  --token "c456789" \
  --name "Smooth Jazz 24/7"

# 3. Station is automatically playing! Or browse for more options:
soundtouch-cli --host 192.168.1.10 browse tunein --limit 10
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
soundtouch-cli --host 192.168.1.10 speaker tts \
  --text "Hello, welcome home" \
  --app-key "your-app-key"

# TTS with volume and language
soundtouch-cli --host 192.168.1.10 speaker tts \
  --text "Bonjour le monde" \
  --app-key "your-app-key" \
  --volume 70 \
  --language FR

# TTS for home automation alert
soundtouch-cli --host 192.168.1.10 speaker tts \
  --text "Motion detected at front door" \
  --app-key "security-system-key" \
  --volume 80
```

**URL Content Examples:**
```bash
# Play audio file from URL
soundtouch-cli --host 192.168.1.10 speaker url \
  --url "https://example.com/doorbell.mp3" \
  --app-key "your-app-key" \
  --volume 75

# Play with custom metadata
soundtouch-cli --host 192.168.1.10 speaker url \
  --url "https://example.com/song.mp3" \
  --app-key "your-app-key" \
  --service "Music Service" \
  --message "Beautiful Song" \
  --reason "Artist Name" \
  --volume 60

# Emergency alert
soundtouch-cli --host 192.168.1.10 speaker url \
  --url "https://alerts.example.com/fire-alarm.wav" \
  --app-key "emergency-system" \
  --service "Emergency System" \
  --message "Fire Alert" \
  --volume 100
```

**Simple Notifications:**
```bash
# Quick beep notification
soundtouch-cli --host 192.168.1.10 speaker beep

# Test device connectivity with beep
soundtouch-cli --host 192.168.1.10 speaker beep
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
soundtouch-cli --host 192.168.1.10 events subscribe

# Monitor only volume and now playing events
soundtouch-cli --host 192.168.1.10 events subscribe --filter volume,nowPlaying

# Monitor for 5 minutes with verbose output
soundtouch-cli --host 192.168.1.10 events subscribe --duration 5m --verbose

# Monitor zone events without automatic reconnection
soundtouch-cli --host 192.168.1.10 events subscribe --filter zone --no-reconnect
```

**Notes:**
- WebSocket connection automatically reconnects on connection loss (unless disabled)
- Press Ctrl+C to stop monitoring
- Events are displayed in real-time with emoji indicators
- Verbose mode shows additional technical details

## Common Usage Patterns

### Quick Device Setup

```bash
# Discover devices
soundtouch-cli discover devices

# Get device info
soundtouch-cli --host 192.168.1.10 info

# Set comfortable volume and start playing
soundtouch-cli --host 192.168.1.10 volume set --level 30
soundtouch-cli --host 192.168.1.10 source spotify
soundtouch-cli --host 192.168.1.10 play start
```

### Daily Usage

```bash
# Morning routine
soundtouch-cli --host 192.168.1.10 preset --preset 1  # Morning playlist
soundtouch-cli --host 192.168.1.10 volume set --level 25

# Pause for a call
soundtouch-cli --host 192.168.1.10 play pause

# Resume
soundtouch-cli --host 192.168.1.10 play start

# Evening routine
soundtouch-cli --host 192.168.1.10 preset --preset 3  # Evening playlist
soundtouch-cli --host 192.168.1.10 volume set --level 15
```

### Multi-room Setup

```bash
# Create a zone with living room as master
soundtouch-cli --host 192.168.1.10 zone create --members 192.168.1.11,192.168.1.12

# Control the whole zone from master
soundtouch-cli --host 192.168.1.10 volume set --level 40
soundtouch-cli --host 192.168.1.10 source spotify
soundtouch-cli --host 192.168.1.10 preset --preset 2

# Later, dissolve the zone
soundtouch-cli --host 192.168.1.10 zone dissolve
```

### Audio Tuning

```bash
# Get current audio settings
soundtouch-cli --host 192.168.1.10 volume get
soundtouch-cli --host 192.168.1.10 bass get
soundtouch-cli --host 192.168.1.10 balance get

# Adjust for better sound
soundtouch-cli --host 192.168.1.10 bass set --level 2      # Slight bass boost
soundtouch-cli --host 192.168.1.10 balance set --level -5  # Slightly left
soundtouch-cli --host 192.168.1.10 volume set --level 35   # Good listening level
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
export SOUNDTOUCH_HOST=192.168.1.10
export SOUNDTOUCH_PORT=8090
export SOUNDTOUCH_TIMEOUT=15s

# Now you can omit these flags
soundtouch-cli info
soundtouch-cli volume get
```

### Configuration File

Create `~/.soundtouch.env`:

```
SOUNDTOUCH_HOST=192.168.1.10
SOUNDTOUCH_PORT=8090
SOUNDTOUCH_TIMEOUT=15s
SOUNDTOUCH_DISCOVERY_TIMEOUT=10s
```

## See Also

- [Getting Started Guide](GETTING-STARTED.md) - Basic setup and usage
- [WebSocket Events](../reference/WEBSOCKET-EVENTS.md) - Real-time monitoring
- [Zone Management](../reference/ZONE-MANAGEMENT.md) - Multi-room setup
- [API Endpoints](../reference/API-ENDPOINTS.md) - Complete API reference
