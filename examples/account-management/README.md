# Music Service Account Management Example

This example demonstrates how to manage music streaming service accounts and network music library connections on Bose SoundTouch devices.

## Overview

The SoundTouch device can store credentials for various music streaming services and network music libraries. This allows you to:

- Add streaming service accounts (Spotify, Pandora, Amazon Music, Deezer, iHeartRadio)
- Configure network music libraries (NAS/UPnP/DLNA servers)
- Remove accounts when no longer needed
- List currently configured accounts

## Running the Example

1. Update the device IP address in `main.go`:
   ```go
   config := &client.Config{
       Host:    "192.168.1.100", // Replace with your device IP
       Port:    8090,
       Timeout: 10 * time.Second,
   }
   ```

2. Run the example:
   ```bash
   go run main.go
   ```

## Supported Music Services

### Streaming Services (require username/password)
- **Spotify Premium**: Personal Spotify accounts
- **Pandora**: Pandora Music Service accounts  
- **Amazon Music**: Amazon Music accounts
- **Deezer Premium**: Deezer subscription accounts
- **iHeartRadio**: iHeartRadio accounts

### Network Music Libraries (no password required)
- **STORED_MUSIC**: NAS, UPnP, and DLNA media servers
- **LOCAL_MUSIC**: Local music servers

## Key Features Demonstrated

### 1. Adding Accounts

```go
// Convenience methods for popular services
err := client.AddSpotifyAccount("user@spotify.com", "password")
err := client.AddPandoraAccount("username", "password")
err := client.AddAmazonMusicAccount("username", "password")

// Generic method for any service
credentials := models.NewMusicServiceCredentials("TIDAL", "Tidal HiFi", "user", "pass")
err := client.SetMusicServiceAccount(credentials)

// Network music library (no password needed)
err := client.AddStoredMusicAccount("server-guid/0", "My Music Server")
```

### 2. Removing Accounts

```go
// Convenience methods
err := client.RemoveSpotifyAccount("user@spotify.com")
err := client.RemovePandoraAccount("username")

// Generic removal method
credentials := models.NewSpotifyCredentials("user@spotify.com", "") // Empty password = removal
err := client.RemoveMusicServiceAccount(credentials)
```

### 3. Validating Credentials

```go
credentials := models.NewSpotifyCredentials("user", "pass")
if err := credentials.Validate(); err != nil {
    log.Fatal("Invalid credentials:", err)
}
```

### 4. Checking Account Status

```go
sources, err := client.GetSources()
if err != nil {
    log.Fatal(err)
}

// Look for sources with accounts configured
for _, source := range sources.Sources {
    if source.SourceAccount != "" {
        fmt.Printf("Service: %s, Account: %s, Status: %s\n", 
            source.Source, source.SourceAccount, source.Status)
    }
}
```

## CLI Usage Examples

After setting up accounts programmatically, you can also manage them via the CLI:

```bash
# List configured accounts
soundtouch-cli --host 192.168.1.10 account list

# Add accounts via CLI
soundtouch-cli --host 192.168.1.10 account add-spotify --user user@spotify.com --password mypass
soundtouch-cli --host 192.168.1.10 account add-pandora --user pandora_user --password pandora_pass
soundtouch-cli --host 192.168.1.10 account add-nas --user "guid/0" --name "My NAS"

# Remove accounts
soundtouch-cli --host 192.168.1.10 account remove-spotify --user user@spotify.com
```

## Network Music Libraries

For STORED_MUSIC (NAS/UPnP) services:

1. The `user` field should contain the UPnP server GUID followed by `/0`
2. You can find the GUID by discovering UPnP devices on your network
3. No password is required
4. You can specify a custom display name for the library

Example GUID format: `d09708a1-5953-44bc-a413-123456789012/0`

## Error Handling

The example includes comprehensive error handling for common scenarios:

- Network connectivity issues
- Invalid credentials
- Missing required fields
- Service-specific authentication failures

## Security Notes

- Credentials are sent securely to the SoundTouch device over your local network
- The device stores encrypted credentials internally
- Passwords are only required during the initial setup
- Use the removal methods to completely delete stored credentials

## Next Steps

After configuring accounts:

1. Use `source list` to verify services are available
2. Use `source select` to choose a music service
3. Use `browse` commands to explore content
4. Use `play` commands to start playback

See the [CLI Reference](../../docs/guides/CLI-REFERENCE.md) for complete documentation.
