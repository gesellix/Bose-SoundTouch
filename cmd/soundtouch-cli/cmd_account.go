package main

import (
	"fmt"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/urfave/cli/v2"
)

// createCredentialsForSource creates credentials for the specified source type
func createCredentialsForSource(source, user, password, displayName string) *models.MusicServiceCredentials {
	switch source {
	case "SPOTIFY":
		return models.NewSpotifyCredentials(user, password)
	case "PANDORA":
		return models.NewPandoraCredentials(user, password)
	case "AMAZON":
		return models.NewAmazonMusicCredentials(user, password)
	case "DEEZER":
		return models.NewDeezerCredentials(user, password)
	case "IHEART":
		return models.NewIHeartRadioCredentials(user, password)
	case "STORED_MUSIC":
		if displayName == "" {
			displayName = "Network Music Library"
		}

		return models.NewStoredMusicCredentials(user, displayName)
	default:
		// Generic credentials for other services
		if displayName == "" {
			displayName = source
		}

		return models.NewMusicServiceCredentials(source, displayName, user, password)
	}
}

// validateAccountInput validates the input parameters for account management
func validateAccountInput(source, user, password string) error {
	if source == "" {
		return fmt.Errorf("source is required (use --source)")
	}

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	// STORED_MUSIC doesn't require a password
	if source != "STORED_MUSIC" && password == "" {
		return fmt.Errorf("password is required for %s (use --password)", source)
	}

	return nil
}

// addMusicServiceAccount handles adding a music service account
func addMusicServiceAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	source := strings.ToUpper(c.String("source"))
	user := c.String("user")
	password := c.String("password")
	displayName := c.String("name")

	if validationErr := validateAccountInput(source, user, password); validationErr != nil {
		return validationErr
	}

	PrintDeviceHeader(fmt.Sprintf("Adding %s account", source), clientConfig.Host, clientConfig.Port)

	credentials := createCredentialsForSource(source, user, password, displayName)

	// Override display name if provided
	if c.IsSet("name") {
		credentials.DisplayName = displayName
	}

	fmt.Printf("  Service: %s\n", credentials.GetDescription())
	fmt.Printf("  User: %s\n", user)

	if source == "STORED_MUSIC" {
		fmt.Printf("  Type: Network Music Library\n")
	} else {
		fmt.Printf("  Type: Streaming Service\n")
	}

	err = client.SetMusicServiceAccount(credentials)
	if err != nil {
		return fmt.Errorf("failed to add music service account: %w", err)
	}

	PrintSuccess(fmt.Sprintf("%s account added successfully", source))

	// Show next steps
	fmt.Printf("\n💡 Next Steps:\n")
	fmt.Printf("   • Check available sources: soundtouch-cli --host %s source list\n", clientConfig.Host)
	fmt.Printf("   • Select this source: soundtouch-cli --host %s source select --source %s --account %s\n", clientConfig.Host, source, user)

	return nil
}

// removeMusicServiceAccount handles removing a music service account
func removeMusicServiceAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	source := strings.ToUpper(c.String("source"))
	user := c.String("user")
	displayName := c.String("name")

	if source == "" {
		return fmt.Errorf("source is required (use --source)")
	}

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	PrintDeviceHeader(fmt.Sprintf("Removing %s account", source), clientConfig.Host, clientConfig.Port)

	var credentials *models.MusicServiceCredentials

	// Create credentials for removal (empty password)
	switch source {
	case "SPOTIFY":
		credentials = models.NewSpotifyCredentials(user, "")
	case "PANDORA":
		credentials = models.NewPandoraCredentials(user, "")
	case "AMAZON":
		credentials = models.NewAmazonMusicCredentials(user, "")
	case "DEEZER":
		credentials = models.NewDeezerCredentials(user, "")
	case "IHEART":
		credentials = models.NewIHeartRadioCredentials(user, "")
	case "STORED_MUSIC":
		if displayName == "" {
			displayName = "Network Music Library"
		}

		credentials = models.NewStoredMusicCredentials(user, displayName)
	default:
		// Generic credentials for other services
		if displayName == "" {
			displayName = source
		}

		credentials = models.NewMusicServiceCredentials(source, displayName, user, "")
	}

	// Override display name if provided
	if c.IsSet("name") {
		credentials.DisplayName = displayName
	}

	fmt.Printf("  Service: %s\n", credentials.GetDescription())
	fmt.Printf("  User: %s\n", user)

	err = client.RemoveMusicServiceAccount(credentials)
	if err != nil {
		return fmt.Errorf("failed to remove music service account: %w", err)
	}

	PrintSuccess(fmt.Sprintf("%s account removed successfully", source))

	return nil
}

// addSpotifyAccount is a convenience command for adding Spotify accounts
func addSpotifyAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")
	password := c.String("password")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	if password == "" {
		return fmt.Errorf("password is required (use --password)")
	}

	PrintDeviceHeader("Adding Spotify Premium account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)
	fmt.Printf("  Service: Spotify Premium\n")

	err = client.AddSpotifyAccount(user, password)
	if err != nil {
		return fmt.Errorf("failed to add Spotify account: %w", err)
	}

	PrintSuccess("Spotify account added successfully")

	// Show next steps
	fmt.Printf("\n💡 Next Steps:\n")
	fmt.Printf("   • Check available sources: soundtouch-cli --host %s source list\n", clientConfig.Host)
	fmt.Printf("   • Select Spotify: soundtouch-cli --host %s source spotify\n", clientConfig.Host)

	return nil
}

// removeSpotifyAccount is a convenience command for removing Spotify accounts
func removeSpotifyAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	PrintDeviceHeader("Removing Spotify account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)

	err = client.RemoveSpotifyAccount(user)
	if err != nil {
		return fmt.Errorf("failed to remove Spotify account: %w", err)
	}

	PrintSuccess("Spotify account removed successfully")

	return nil
}

// addPandoraAccount is a convenience command for adding Pandora accounts
func addPandoraAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")
	password := c.String("password")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	if password == "" {
		return fmt.Errorf("password is required (use --password)")
	}

	PrintDeviceHeader("Adding Pandora account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)
	fmt.Printf("  Service: Pandora Music Service\n")

	err = client.AddPandoraAccount(user, password)
	if err != nil {
		return fmt.Errorf("failed to add Pandora account: %w", err)
	}

	PrintSuccess("Pandora account added successfully")

	// Show next steps
	fmt.Printf("\n💡 Next Steps:\n")
	fmt.Printf("   • Check available sources: soundtouch-cli --host %s source list\n", clientConfig.Host)
	fmt.Printf("   • Select Pandora: soundtouch-cli --host %s source select --source PANDORA --account %s\n", clientConfig.Host, user)

	return nil
}

// removePandoraAccount is a convenience command for removing Pandora accounts
func removePandoraAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	PrintDeviceHeader("Removing Pandora account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)

	err = client.RemovePandoraAccount(user)
	if err != nil {
		return fmt.Errorf("failed to remove Pandora account: %w", err)
	}

	PrintSuccess("Pandora account removed successfully")

	return nil
}

// addStoredMusicAccount is a convenience command for adding STORED_MUSIC accounts
func addStoredMusicAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	stClient, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")
	displayName := c.String("name")

	if user == "" {
		return fmt.Errorf("user is required (use --user) - this should be the UPnP server GUID with /0 suffix")
	}

	if displayName == "" {
		displayName = "Network Music Library"
	}

	PrintDeviceHeader("Adding network music library", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  Server ID: %s\n", user)
	fmt.Printf("  Display Name: %s\n", displayName)
	fmt.Printf("  Type: UPnP/DLNA Media Server\n")

	err = stClient.AddStoredMusicAccount(user, displayName)
	if err != nil {
		return fmt.Errorf("failed to add network music library: %w", err)
	}

	PrintSuccess("Network music library added successfully")

	// Send a sourcesUpdated nudge so the speaker re-fetches its account list and
	// registers the new source without requiring a power-cycle. This is
	// best-effort: a failure here does not abort the command.
	if info, infoErr := stClient.GetDeviceInfo(); infoErr == nil && info != nil && info.DeviceID != "" {
		if nudgeErr := stClient.NotifySourcesUpdated(info.DeviceID); nudgeErr == nil {
			fmt.Println("  Sent a sources refresh to the speaker (no reboot needed).")
		} else {
			fmt.Println("  Warning: could not send sources refresh; you may need to power-cycle the speaker for the new source to register.")
		}
	} else {
		fmt.Println("  Warning: could not retrieve device ID; you may need to power-cycle the speaker for the new source to register.")
	}

	// Show next steps
	fmt.Printf("\n💡 Next Steps:\n")
	fmt.Printf("   • Check available sources: soundtouch-cli --host %s source list\n", clientConfig.Host)
	fmt.Printf("   • Browse library: soundtouch-cli --host %s browse stored-music --account %s\n", clientConfig.Host, user)

	return nil
}

// addAmazonMusicAccount is a convenience command for adding Amazon Music accounts
func addAmazonMusicAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")
	password := c.String("password")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	if password == "" {
		return fmt.Errorf("password is required (use --password)")
	}

	PrintDeviceHeader("Adding Amazon Music account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)
	fmt.Printf("  Service: Amazon Music\n")

	err = client.AddAmazonMusicAccount(user, password)
	if err != nil {
		return fmt.Errorf("failed to add Amazon Music account: %w", err)
	}

	PrintSuccess("Amazon Music account added successfully")

	// Show next steps
	fmt.Printf("\n💡 Next Steps:\n")
	fmt.Printf("   • Check available sources: soundtouch-cli --host %s source list\n", clientConfig.Host)
	fmt.Printf("   • Select Amazon Music: soundtouch-cli --host %s source select --source AMAZON --account %s\n", clientConfig.Host, user)

	return nil
}

// removeAmazonMusicAccount is a convenience command for removing Amazon Music accounts
func removeAmazonMusicAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	PrintDeviceHeader("Removing Amazon Music account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)

	err = client.RemoveAmazonMusicAccount(user)
	if err != nil {
		return fmt.Errorf("failed to remove Amazon Music account: %w", err)
	}

	PrintSuccess("Amazon Music account removed successfully")

	return nil
}

// addDeezerAccount is a convenience command for adding Deezer accounts
func addDeezerAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")
	password := c.String("password")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	if password == "" {
		return fmt.Errorf("password is required (use --password)")
	}

	PrintDeviceHeader("Adding Deezer Premium account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)
	fmt.Printf("  Service: Deezer Premium\n")

	err = client.AddDeezerAccount(user, password)
	if err != nil {
		return fmt.Errorf("failed to add Deezer account: %w", err)
	}

	PrintSuccess("Deezer account added successfully")

	// Show next steps
	fmt.Printf("\n💡 Next Steps:\n")
	fmt.Printf("   • Check available sources: soundtouch-cli --host %s source list\n", clientConfig.Host)
	fmt.Printf("   • Select Deezer: soundtouch-cli --host %s source select --source DEEZER --account %s\n", clientConfig.Host, user)

	return nil
}

// removeDeezerAccount is a convenience command for removing Deezer accounts
func removeDeezerAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	PrintDeviceHeader("Removing Deezer account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)

	err = client.RemoveDeezerAccount(user)
	if err != nil {
		return fmt.Errorf("failed to remove Deezer account: %w", err)
	}

	PrintSuccess("Deezer account removed successfully")

	return nil
}

// addIHeartRadioAccount is a convenience command for adding iHeartRadio accounts
func addIHeartRadioAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")
	password := c.String("password")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	if password == "" {
		return fmt.Errorf("password is required (use --password)")
	}

	PrintDeviceHeader("Adding iHeartRadio account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)
	fmt.Printf("  Service: iHeartRadio\n")

	err = client.AddIHeartRadioAccount(user, password)
	if err != nil {
		return fmt.Errorf("failed to add iHeartRadio account: %w", err)
	}

	PrintSuccess("iHeartRadio account added successfully")

	// Show next steps
	fmt.Printf("\n💡 Next Steps:\n")
	fmt.Printf("   • Check available sources: soundtouch-cli --host %s source list\n", clientConfig.Host)
	fmt.Printf("   • Select iHeartRadio: soundtouch-cli --host %s source select --source IHEART --account %s\n", clientConfig.Host, user)

	return nil
}

// removeIHeartRadioAccount is a convenience command for removing iHeartRadio accounts
func removeIHeartRadioAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	PrintDeviceHeader("Removing iHeartRadio account", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  User: %s\n", user)

	err = client.RemoveIHeartRadioAccount(user)
	if err != nil {
		return fmt.Errorf("failed to remove iHeartRadio account: %w", err)
	}

	PrintSuccess("iHeartRadio account removed successfully")

	return nil
}

// removeStoredMusicAccount is a convenience command for removing STORED_MUSIC accounts
func removeStoredMusicAccount(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	user := c.String("user")
	displayName := c.String("name")

	if user == "" {
		return fmt.Errorf("user is required (use --user)")
	}

	if displayName == "" {
		displayName = "Network Music Library"
	}

	PrintDeviceHeader("Removing network music library", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  Server ID: %s\n", user)
	fmt.Printf("  Display Name: %s\n", displayName)

	err = client.RemoveStoredMusicAccount(user, displayName)
	if err != nil {
		return fmt.Errorf("failed to remove network music library: %w", err)
	}

	PrintSuccess("Network music library removed successfully")

	return nil
}

// listMusicServiceAccounts shows configured music service accounts from sources
func listMusicServiceAccounts(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	PrintDeviceHeader("Music service accounts", clientConfig.Host, clientConfig.Port)

	sources, err := client.GetSources()
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}

	// Filter for streaming/music service sources
	musicSources := []string{"SPOTIFY", "PANDORA", "AMAZON", "DEEZER", "IHEART", "STORED_MUSIC", "LOCAL_MUSIC"}

	found := false

	for _, musicSource := range musicSources {
		sourcesOfType := sources.GetSourcesByType(musicSource)
		if len(sourcesOfType) > 0 {
			found = true

			fmt.Printf("\n📱 %s:\n", getServiceDisplayName(musicSource))

			for _, source := range sourcesOfType {
				status := "🔴 Unavailable"
				if source.Status == models.SourceStatusReady {
					status = "🟢 Ready"
				}

				accountInfo := ""
				if source.SourceAccount != "" && source.SourceAccount != source.Source {
					accountInfo = fmt.Sprintf(" (%s)", source.SourceAccount)
				}

				fmt.Printf("    %s %s%s\n", status, source.GetDisplayName(), accountInfo)
			}
		}
	}

	if !found {
		fmt.Printf("  📭 No music service accounts configured\n")
		fmt.Printf("\n💡 Add accounts with:\n")
		fmt.Printf("   • soundtouch-cli --host %s account add-spotify --user <email> --password <pass>\n", clientConfig.Host)
		fmt.Printf("   • soundtouch-cli --host %s account add-pandora --user <user> --password <pass>\n", clientConfig.Host)
		fmt.Printf("   • soundtouch-cli --host %s account add --source AMAZON --user <user> --password <pass>\n", clientConfig.Host)
	}

	return nil
}

// pairDevice triggers the Stockholm registration flow via WebSocket
func pairDevice(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	accountID := c.String("id")
	token := c.String("token")

	PrintDeviceHeader("Pairing device with Marge account", clientConfig.Host, clientConfig.Port)
	fmt.Printf("  Account ID: %s\n", accountID)

	// We need a WebSocket client for this
	ws := client.NewWebSocketClient(nil)

	err = ws.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to device WebSocket: %w", err)
	}

	defer func() { _ = ws.Disconnect() }()

	err = ws.PairWithAccount(accountID, token)
	if err != nil {
		return fmt.Errorf("failed to send pairing request: %w", err)
	}

	PrintSuccess("Pairing request sent successfully")
	fmt.Println("💡 The device will now register itself with the cloud service.")

	return nil
}

// unpairDevice triggers the Stockholm unregistration flow via WebSocket
func unpairDevice(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	PrintDeviceHeader("Unpairing device from Marge account", clientConfig.Host, clientConfig.Port)

	// We need a WebSocket client for this
	ws := client.NewWebSocketClient(nil)

	err = ws.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to device WebSocket: %w", err)
	}

	defer func() { _ = ws.Disconnect() }()

	err = ws.UnPairFromAccount()
	if err != nil {
		return fmt.Errorf("failed to send unpairing request: %w", err)
	}

	PrintSuccess("Unpairing request sent successfully")

	return nil
}

// getServiceDisplayName returns a user-friendly display name for a service
func getServiceDisplayName(source string) string {
	switch source {
	case "SPOTIFY":
		return "Spotify"
	case "PANDORA":
		return "Pandora"
	case "AMAZON":
		return "Amazon Music"
	case "DEEZER":
		return "Deezer"
	case "IHEART":
		return "iHeartRadio"
	case "STORED_MUSIC":
		return "Network Libraries"
	case "LOCAL_MUSIC":
		return "Local Music Servers"
	default:
		return source
	}
}
