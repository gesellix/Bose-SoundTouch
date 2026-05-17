// Package main demonstrates content selection functionality for Bose SoundTouch devices.
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

	fmt.Printf("🎵 SoundTouch Content Selection Example\n")
	fmt.Printf("📱 Device: %s:%d\n\n", config.Host, config.Port)

	// Demonstrate various content selection methods
	if err := demonstrateContentSelection(c); err != nil {
		log.Fatalf("Demo failed: %v", err)
	}

	fmt.Println("\n✅ Content selection demo completed!")
}

func demonstrateContentSelection(c *client.Client) error {
	// 1. Demonstrate LOCAL_INTERNET_RADIO with streamUrl format
	fmt.Println("📻 Step 1: Demonstrating LOCAL_INTERNET_RADIO with streamUrl format...")

	if err := demoLocalInternetRadioStreamUrl(c); err != nil {
		return fmt.Errorf("failed LOCAL_INTERNET_RADIO demo: %w", err)
	}

	// Wait and show what's playing
	time.Sleep(3 * time.Second)

	if err := showNowPlaying(c); err != nil {
		fmt.Printf("⚠️  Could not get now playing: %v\n", err)
	}

	// 2. Demonstrate LOCAL_INTERNET_RADIO with direct stream
	fmt.Println("\n📻 Step 2: Demonstrating LOCAL_INTERNET_RADIO with direct stream...")

	if err := demoLocalInternetRadioDirect(c); err != nil {
		return fmt.Errorf("failed direct stream demo: %w", err)
	}

	// Wait and show what's playing
	time.Sleep(3 * time.Second)

	if err := showNowPlaying(c); err != nil {
		fmt.Printf("⚠️  Could not get now playing: %v\n", err)
	}

	// 3. Demonstrate LOCAL_MUSIC selection
	fmt.Println("\n💿 Step 3: Demonstrating LOCAL_MUSIC selection...")

	if err := demoLocalMusic(c); err != nil {
		fmt.Printf("⚠️  LOCAL_MUSIC demo failed (this requires SoundTouch App Media Server): %v\n", err)
	} else {
		// Wait and show what's playing
		time.Sleep(3 * time.Second)

		if err := showNowPlaying(c); err != nil {
			fmt.Printf("⚠️  Could not get now playing: %v\n", err)
		}
	}

	// 4. Demonstrate STORED_MUSIC selection
	fmt.Println("\n💾 Step 4: Demonstrating STORED_MUSIC selection...")

	if err := demoStoredMusic(c); err != nil {
		fmt.Printf("⚠️  STORED_MUSIC demo failed (this requires UPnP/DLNA media server): %v\n", err)
	} else {
		// Wait and show what's playing
		time.Sleep(3 * time.Second)

		if err := showNowPlaying(c); err != nil {
			fmt.Printf("⚠️  Could not get now playing: %v\n", err)
		}
	}

	// 5. Demonstrate generic ContentItem selection
	fmt.Println("\n🎯 Step 5: Demonstrating generic ContentItem selection...")

	if err := demoGenericContentItem(c); err != nil {
		return fmt.Errorf("failed generic ContentItem demo: %w", err)
	}

	// Wait and show what's playing
	time.Sleep(3 * time.Second)

	if err := showNowPlaying(c); err != nil {
		fmt.Printf("⚠️  Could not get now playing: %v\n", err)
	}

	return nil
}

func demoLocalInternetRadioStreamUrl(c *client.Client) error {
	fmt.Printf("  📡 Using streamUrl format with proxy server...\n")

	// Example using the streamUrl format from the wiki
	// This uses a proxy server that accepts the actual stream URL as a parameter
	location := "http://contentapi.gmuth.de/station.php?name=Antenne%20Chillout&streamUrl=https://stream.antenne.de/chillout/stream/aacp"
	itemName := "Antenne Chillout"
	containerArt := "https://www.radio.net/300/antennechillout.png?version=7fddbc7d3f37557ad3291d66fff40f323e1779d6"

	fmt.Printf("      Station: %s\n", itemName)
	fmt.Printf("      Proxy URL: %s\n", location)

	err := c.SelectLocalInternetRadio(location, "", itemName, containerArt)
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ Successfully selected internet radio with streamUrl format\n")

	return nil
}

func demoLocalInternetRadioDirect(c *client.Client) error {
	fmt.Printf("  📡 Using direct stream URL...\n")

	// Example using a direct stream URL
	location := "https://freetestdata.com/wp-content/uploads/2021/09/Free_Test_Data_1MB_MP3.mp3"
	itemName := "Test Audio Stream"

	fmt.Printf("      Stream: %s\n", itemName)
	fmt.Printf("      URL: %s\n", location)

	err := c.SelectLocalInternetRadio(location, "", itemName, "")
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ Successfully selected direct internet radio stream\n")

	return nil
}

func demoLocalMusic(c *client.Client) error {
	fmt.Printf("  💿 Selecting LOCAL_MUSIC content...\n")

	// Example LOCAL_MUSIC selection (requires SoundTouch App Media Server)
	// These are example values - in practice, you'd get these from navigation
	location := "album:983"
	sourceAccount := "3f205110-4a57-4e91-810a-123456789012" // Example GUID
	itemName := "Welcome to the New"
	containerArt := "http://192.0.2.14:8085/v1/albums/983/image"

	fmt.Printf("      Album: %s\n", itemName)
	fmt.Printf("      Location: %s\n", location)
	fmt.Printf("      Account: %s\n", sourceAccount)

	err := c.SelectLocalMusic(location, sourceAccount, itemName, containerArt)
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ Successfully selected local music content\n")

	return nil
}

func demoStoredMusic(c *client.Client) error {
	fmt.Printf("  💾 Selecting STORED_MUSIC content...\n")

	// Example STORED_MUSIC selection (requires UPnP/DLNA media server)
	// These are example values - in practice, you'd get these from navigation
	location := "6_a2874b5d_4f83d999"
	sourceAccount := "d09708a1-5953-44bc-a413-123456789012/0" // Example UPnP server GUID
	itemName := "Christmas Album"

	fmt.Printf("      Album: %s\n", itemName)
	fmt.Printf("      Location: %s\n", location)
	fmt.Printf("      Account: %s\n", sourceAccount)

	err := c.SelectStoredMusic(location, sourceAccount, itemName, "")
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ Successfully selected stored music content\n")

	return nil
}

func demoGenericContentItem(c *client.Client) error {
	fmt.Printf("  🎯 Using generic ContentItem selection...\n")

	// Example using SelectContentItem directly for maximum flexibility
	contentItem := &models.ContentItem{
		Source:        "TUNEIN",
		Type:          "stationurl",
		Location:      "/v1/playback/station/s33828", // K-LOVE Radio
		SourceAccount: "",
		IsPresetable:  true,
		ItemName:      "K-LOVE Radio",
		ContainerArt:  "http://cdn-profiles.tunein.com/s33828/images/logog.png",
	}

	fmt.Printf("      Content: %s\n", contentItem.ItemName)
	fmt.Printf("      Source: %s\n", contentItem.Source)
	fmt.Printf("      Location: %s\n", contentItem.Location)

	err := c.SelectContentItem(contentItem)
	if err != nil {
		return err
	}

	fmt.Printf("  ✅ Successfully selected content using ContentItem\n")

	return nil
}

func showNowPlaying(c *client.Client) error {
	nowPlaying, err := c.GetNowPlaying()
	if err != nil {
		return err
	}

	if nowPlaying.IsEmpty() {
		fmt.Printf("  ⏸️  No content currently playing\n")
		return nil
	}

	fmt.Printf("  🎵 Now Playing:\n")
	fmt.Printf("      Title: %s\n", nowPlaying.GetDisplayTitle())

	if nowPlaying.GetDisplayArtist() != "" {
		fmt.Printf("      Artist: %s\n", nowPlaying.GetDisplayArtist())
	}

	if nowPlaying.Album != "" {
		fmt.Printf("      Album: %s\n", nowPlaying.Album)
	}

	fmt.Printf("      Source: %s\n", nowPlaying.Source)
	fmt.Printf("      Status: %s\n", nowPlaying.PlayStatus.String())

	if nowPlaying.ContentItem != nil && nowPlaying.ContentItem.Location != "" {
		fmt.Printf("      Location: %s\n", nowPlaying.ContentItem.Location)
	}

	return nil
}

func printUsage() {
	fmt.Println("🎵 SoundTouch Content Selection Example")
	fmt.Println()
	fmt.Println("This example demonstrates the new content selection features:")
	fmt.Println("• LOCAL_INTERNET_RADIO with streamUrl format")
	fmt.Println("• LOCAL_INTERNET_RADIO with direct stream URLs")
	fmt.Println("• LOCAL_MUSIC content selection")
	fmt.Println("• STORED_MUSIC content selection")
	fmt.Println("• Generic ContentItem selection")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s <device_ip>\n", os.Args[0])
	fmt.Println()
	fmt.Println("Example:")
	fmt.Printf("  %s 192.0.2.100\n", os.Args[0])
	fmt.Println()
	fmt.Println("Prerequisites:")
	fmt.Println("• SoundTouch device on your network")
	fmt.Println("• Device IP address")
	fmt.Println("• Device powered on and connected")
	fmt.Println()
	fmt.Println("Note:")
	fmt.Println("• LOCAL_MUSIC examples require SoundTouch App Media Server")
	fmt.Println("• STORED_MUSIC examples require UPnP/DLNA media server")
	fmt.Println("• Some streams may not work depending on your network/location")
}
