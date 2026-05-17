package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Get device IP from command line
	deviceIP := os.Args[1]

	// Create client
	config := &client.Config{
		Host:    deviceIP,
		Port:    8090,
		Timeout: 10 * time.Second,
	}
	c := client.NewClient(config)

	fmt.Printf("🎵 SoundTouch Preset Management Example\n")
	fmt.Printf("📱 Device: %s:%d\n\n", config.Host, config.Port)

	// Demonstrate all preset management features
	if err := demonstratePresetManagement(c); err != nil {
		log.Fatalf("Demo failed: %v", err)
	}

	fmt.Println("\n✅ Preset management demo completed!")
}

func demonstratePresetManagement(c *client.Client) error {
	// 1. Get current presets
	fmt.Println("📋 Step 1: Getting current presets...")
	if err := showCurrentPresets(c); err != nil {
		return fmt.Errorf("failed to get presets: %w", err)
	}

	// 2. Check if current content is presetable
	fmt.Println("\n🔍 Step 2: Checking current content...")
	if err := checkCurrentContent(c); err != nil {
		return fmt.Errorf("failed to check current content: %w", err)
	}

	// 3. Store current content as preset (if possible)
	fmt.Println("\n💾 Step 3: Storing current content as preset...")
	if err := storeCurrentAsPreset(c); err != nil {
		fmt.Printf("⚠️  Cannot store current content: %v\n", err)

		// 4. Store a Spotify playlist as alternative example
		fmt.Println("\n💿 Step 4: Storing Spotify playlist as preset...")
		if err := storeSpotifyPlaylist(c); err != nil {
			return fmt.Errorf("failed to store Spotify playlist: %w", err)
		}
	}

	// 5. Store a radio station
	fmt.Println("\n📻 Step 5: Storing radio station as preset...")
	if err := storeRadioStation(c); err != nil {
		return fmt.Errorf("failed to store radio station: %w", err)
	}

	// 6. Show updated presets
	fmt.Println("\n📋 Step 6: Showing updated presets...")
	if err := showCurrentPresets(c); err != nil {
		return fmt.Errorf("failed to get updated presets: %w", err)
	}

	// 7. Select a preset
	fmt.Println("\n🎯 Step 7: Selecting preset 1...")
	if err := selectPreset(c, 1); err != nil {
		return fmt.Errorf("failed to select preset: %w", err)
	}

	// 8. Demonstrate WebSocket events
	fmt.Println("\n📡 Step 8: Demonstrating preset events...")
	if err := demonstrateWebSocketEvents(c); err != nil {
		return fmt.Errorf("failed to demonstrate WebSocket events: %w", err)
	}

	return nil
}

func showCurrentPresets(c *client.Client) error {
	presets, err := c.GetPresets()
	if err != nil {
		return err
	}

	// Filter out placeholder presets (issue #308): self-closing
	// <preset/> entries from a factory-reset device and
	// INVALID_SOURCE placeholders from healthy devices both panic if
	// their fields are dereferenced directly.
	configured := make([]models.Preset, 0, len(presets.Preset))

	for _, p := range presets.Preset {
		if !p.IsEmpty() {
			configured = append(configured, p)
		}
	}

	if len(configured) == 0 {
		fmt.Println("  📭 No presets configured")
		return nil
	}

	fmt.Printf("  📻 Found %d configured presets:\n", len(configured))

	for _, preset := range configured {
		fmt.Printf("    %d. %s\n", preset.ID, preset.GetDisplayName())
		fmt.Printf("       Source: %s\n", preset.GetSource())

		if location := preset.GetLocation(); location != "" {
			fmt.Printf("       Location: %s\n", location)
		}

		if preset.CreatedOn != nil && *preset.CreatedOn != 0 {
			createdTime := time.Unix(*preset.CreatedOn, 0)
			fmt.Printf("       Created: %s\n", createdTime.Format("2006-01-02 15:04:05"))
		}

		fmt.Println()
	}

	// Show available slots
	emptySlots := presets.GetEmptyPresetSlots()
	if len(emptySlots) > 0 {
		fmt.Printf("  🆓 Available slots: %v\n", emptySlots)
	} else {
		fmt.Println("  🈵 All preset slots are occupied")
	}

	return nil
}

func checkCurrentContent(c *client.Client) error {
	nowPlaying, err := c.GetNowPlaying()
	if err != nil {
		return err
	}

	if nowPlaying.IsEmpty() {
		fmt.Println("  ⏸️  No content currently playing")
		return nil
	}

	fmt.Printf("  🎵 Now Playing: %s\n", nowPlaying.Track)
	if nowPlaying.Artist != "" {
		fmt.Printf("      Artist: %s\n", nowPlaying.Artist)
	}
	fmt.Printf("      Source: %s\n", nowPlaying.Source)

	if nowPlaying.ContentItem == nil {
		fmt.Println("      ❌ No content item available")
		return nil
	}

	fmt.Printf("      Presetable: %t\n", nowPlaying.ContentItem.IsPresetable)
	if nowPlaying.ContentItem.Location != "" {
		fmt.Printf("      Location: %s\n", nowPlaying.ContentItem.Location)
	}

	return nil
}

func storeCurrentAsPreset(c *client.Client) error {
	// Check if current content is presetable
	presetable, err := c.IsCurrentContentPresetable()
	if err != nil {
		return err
	}

	if !presetable {
		return fmt.Errorf("current content is not presetable")
	}

	// Find an available slot
	nextSlot, err := c.GetNextAvailablePresetSlot()
	if err != nil {
		return err
	}

	fmt.Printf("  💾 Storing current content as preset %d...\n", nextSlot)

	err = c.StoreCurrentAsPreset(nextSlot)
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ Successfully stored as preset %d\n", nextSlot)
	return nil
}

func storeSpotifyPlaylist(c *client.Client) error {
	// Find an available slot
	nextSlot, err := c.GetNextAvailablePresetSlot()
	if err != nil {
		return err
	}

	// Example Spotify playlist
	spotifyContent := &models.ContentItem{
		Source:        "SPOTIFY",
		Type:          "uri",
		Location:      "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M", // Today's Top Hits
		SourceAccount: "spotify_user",
		IsPresetable:  true,
		ItemName:      "Today's Top Hits",
		ContainerArt:  "https://i.scdn.co/image/ab67706f00000003c13b4f1084cea7bededbcadc",
	}

	fmt.Printf("  💿 Storing Spotify playlist as preset %d...\n", nextSlot)
	fmt.Printf("      Playlist: %s\n", spotifyContent.ItemName)
	fmt.Printf("      URI: %s\n", spotifyContent.Location)

	err = c.StorePreset(nextSlot, spotifyContent)
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ Successfully stored Spotify playlist as preset %d\n", nextSlot)
	return nil
}

func storeRadioStation(c *client.Client) error {
	// Find an available slot
	nextSlot, err := c.GetNextAvailablePresetSlot()
	if err != nil {
		return err
	}

	// Example radio station
	radioContent := &models.ContentItem{
		Source:        "TUNEIN",
		Type:          "stationurl",
		Location:      "/v1/playback/station/s33828", // K-LOVE
		SourceAccount: "",
		IsPresetable:  true,
		ItemName:      "K-LOVE Radio",
		ContainerArt:  "http://cdn-profiles.tunein.com/s33828/images/logog.png",
	}

	fmt.Printf("  📻 Storing radio station as preset %d...\n", nextSlot)
	fmt.Printf("      Station: %s\n", radioContent.ItemName)
	fmt.Printf("      Location: %s\n", radioContent.Location)

	err = c.StorePreset(nextSlot, radioContent)
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ Successfully stored radio station as preset %d\n", nextSlot)
	return nil
}

func selectPreset(c *client.Client, presetNumber int) error {
	// First check if the preset exists
	presets, err := c.GetPresets()
	if err != nil {
		return err
	}

	preset := presets.GetPresetByID(presetNumber)
	if preset == nil || preset.IsEmpty() {
		return fmt.Errorf("preset %d is empty", presetNumber)
	}

	fmt.Printf("  🎯 Selecting preset %d: %s\n", presetNumber, preset.GetDisplayName())

	err = c.SelectPreset(presetNumber)
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ Successfully selected preset %d\n", presetNumber)

	// Wait a moment and show what's now playing
	time.Sleep(2 * time.Second)
	fmt.Println("  🎵 Checking what's now playing...")

	nowPlaying, err := c.GetNowPlaying()
	if err != nil {
		fmt.Printf("  ⚠️  Could not get now playing: %v\n", err)
		return nil
	}

	if !nowPlaying.IsEmpty() {
		fmt.Printf("      Now Playing: %s\n", nowPlaying.Track)
		if nowPlaying.Artist != "" {
			fmt.Printf("      Artist: %s\n", nowPlaying.Artist)
		}
		fmt.Printf("      Source: %s\n", nowPlaying.Source)
	}

	return nil
}

func demonstrateWebSocketEvents(c *client.Client) error {
	// Create WebSocket client
	wsClient := c.NewWebSocketClient(nil)

	// Set up preset event handler
	wsClient.OnPresetUpdated(func(event *models.PresetUpdatedEvent) {
		fmt.Printf("  📡 Preset Update Event Received!\n")
		fmt.Printf("      Device: %s\n", event.DeviceID)
		fmt.Printf("      Presets count: %d\n", len(event.Presets.Preset))

		for _, preset := range event.Presets.Preset {
			if !preset.IsEmpty() {
				fmt.Printf("      - Preset %d: %s (%s)\n",
					preset.ID, preset.GetDisplayName(), preset.GetSource())
			}
		}
	})

	// Connect to WebSocket
	fmt.Printf("  📡 Connecting to WebSocket for real-time events...\n")
	err := wsClient.Connect()
	if err != nil {
		return err
	}
	defer wsClient.Disconnect()

	fmt.Printf("  ✅ WebSocket connected, listening for preset events...\n")
	fmt.Printf("  🔄 Making a preset change to trigger an event...\n")

	// Find an available slot and store something to trigger an event
	nextSlot, err := c.GetNextAvailablePresetSlot()
	if err != nil {
		// If no slots available, remove the last preset we created
		nextSlot = 6
		fmt.Printf("  🗑️  Removing preset %d to trigger event...\n", nextSlot)
		c.RemovePreset(nextSlot)
	} else {
		// Store a simple test preset
		testContent := &models.ContentItem{
			Source:        "TUNEIN",
			Type:          "stationurl",
			Location:      "/v1/playback/station/s25111", // BBC Radio 1
			SourceAccount: "",
			IsPresetable:  true,
			ItemName:      "BBC Radio 1",
		}
		fmt.Printf("  💾 Storing test preset %d to trigger event...\n", nextSlot)
		c.StorePreset(nextSlot, testContent)
	}

	// Wait for event
	fmt.Println("  ⏳ Waiting 3 seconds for WebSocket event...")
	time.Sleep(3 * time.Second)

	fmt.Println("  📡 WebSocket events demonstration complete")
	return nil
}

func printUsage() {
	fmt.Println("🎵 SoundTouch Preset Management Example")
	fmt.Println()
	fmt.Println("This example demonstrates all preset management features:")
	fmt.Println("• List current presets")
	fmt.Println("• Check if content is presetable")
	fmt.Println("• Store current content as preset")
	fmt.Println("• Store Spotify playlists as presets")
	fmt.Println("• Store radio stations as presets")
	fmt.Println("• Select presets")
	fmt.Println("• Handle preset WebSocket events")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s <device_ip>\n", os.Args[0])
	fmt.Println()
	fmt.Println("Example:")
	fmt.Printf("  %s 192.168.1.100\n", os.Args[0])
	fmt.Println()
	fmt.Println("Prerequisites:")
	fmt.Println("• SoundTouch device on your network")
	fmt.Println("• Device IP address")
	fmt.Println("• Device powered on and connected")
}
