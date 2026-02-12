package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/urfave/cli/v2"
)

// playTTS plays a Text-To-Speech message on the speaker
func playTTS(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	text := c.String("text")
	appKey := c.String("app-key")
	volume := c.Int("volume")
	language := c.String("language")

	if text == "" {
		PrintError("Text message is required")
		return fmt.Errorf("text message cannot be empty")
	}

	if appKey == "" {
		PrintError("App key is required")
		return fmt.Errorf("app key cannot be empty")
	}

	PrintDeviceHeader(fmt.Sprintf("Playing TTS message: \"%s\"", text), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// URL encode the text for Google TTS
	encodedText := url.QueryEscape(text)

	// Build TTS URL with language support
	ttsURL := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&tl=%s&client=tw-ob&q=%s", language, encodedText)

	// Create PlayInfo for TTS
	playInfo := &models.PlayInfo{
		URL:     ttsURL,
		AppKey:  appKey,
		Service: "TTS Notification",
		Message: "Google TTS",
		Reason:  text,
	}

	if volume > 0 {
		playInfo.SetVolume(volume)
	}

	err = client.PlayCustom(playInfo)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to play TTS message: %v", err))
		return err
	}

	fmt.Printf("✅ TTS message sent successfully\n")

	if volume > 0 {
		fmt.Printf("   Volume: %d\n", volume)
	} else {
		fmt.Printf("   Volume: current level\n")
	}

	fmt.Printf("   Language: %s\n", strings.ToUpper(language))
	fmt.Printf("   Message: \"%s\"\n", text)

	return nil
}

// playURL plays audio content from a URL on the speaker
func playURL(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	urlStr := c.String("url")
	appKey := c.String("app-key")
	service := c.String("service")
	message := c.String("message")
	reason := c.String("reason")
	volume := c.Int("volume")

	if urlStr == "" {
		PrintError("URL is required")
		return fmt.Errorf("URL cannot be empty")
	}

	if appKey == "" {
		PrintError("App key is required")
		return fmt.Errorf("app key cannot be empty")
	}

	// Set defaults if not provided
	if service == "" {
		service = "URL Playback"
	}

	if message == "" {
		message = "Audio Content"
	}

	if reason == "" {
		// Extract filename or use URL as reason
		if idx := strings.LastIndex(urlStr, "/"); idx != -1 && idx < len(urlStr)-1 {
			reason = urlStr[idx+1:]
		} else {
			reason = urlStr
		}
	}

	PrintDeviceHeader(fmt.Sprintf("Playing URL: %s", urlStr), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Create PlayInfo for URL content
	playInfo := models.NewURLPlayInfo(urlStr, appKey, service, message, reason)

	if volume > 0 {
		playInfo.SetVolume(volume)
	}

	err = client.PlayCustom(playInfo)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to play URL content: %v", err))
		return err
	}

	fmt.Printf("✅ URL playback started successfully\n")
	fmt.Printf("   URL: %s\n", urlStr)
	fmt.Printf("   Service: %s\n", service)
	fmt.Printf("   Message: %s\n", message)

	if volume > 0 {
		fmt.Printf("   Volume: %d\n", volume)
	} else {
		fmt.Printf("   Volume: current level\n")
	}

	return nil
}

// playNotification plays a notification sound or a local file on the speaker
func playNotification(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	path := c.String("path")

	if path != "" {
		PrintDeviceHeader(fmt.Sprintf("Playing notification file: %s", path), clientConfig.Host, clientConfig.Port)
	} else {
		PrintDeviceHeader("Playing notification beep", clientConfig.Host, clientConfig.Port)
	}

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	err = client.PlayNotification(path)
	if err != nil {
		if path != "" {
			PrintError(fmt.Sprintf("Failed to play notification file: %v", err))
		} else {
			PrintError(fmt.Sprintf("Failed to play notification beep: %v", err))
		}

		return err
	}

	if path != "" {
		fmt.Printf("✅ Notification file sent successfully: %s\n", path)
	} else {
		fmt.Printf("✅ Notification beep played successfully\n")
	}

	return nil
}

// playNotificationBeep plays a notification beep on the speaker (uses existing endpoint)
func playNotificationBeep(c *cli.Context) error {
	return playNotification(c)
}

// showSpeakerHelp displays help information about speaker functionality
func showSpeakerHelp(_ *cli.Context) error {
	fmt.Println("SoundTouch Speaker Playback Commands")
	fmt.Println("=====================================")
	fmt.Println()
	fmt.Println("The /speaker endpoint supports playing notifications and URL content:")
	fmt.Println()
	fmt.Println("• Text-to-Speech (TTS) Messages:")
	fmt.Println("  Play spoken messages using Google TTS")
	fmt.Println("  Example: soundtouch-cli speaker tts --text \"Hello World\" --app-key YOUR_KEY")
	fmt.Println()
	fmt.Println("• URL Content Playback:")
	fmt.Println("  Play audio files from HTTP/HTTPS URLs")
	fmt.Println("  Example: soundtouch-cli speaker url --url \"https://example.com/audio.mp3\" --app-key YOUR_KEY")
	fmt.Println()
	fmt.Println("• Notification Beep:")
	fmt.Println("  Play a simple notification sound")
	fmt.Println("  Example: soundtouch-cli speaker beep")
	fmt.Println()
	fmt.Println("• Custom Notification:")
	fmt.Println("  Play a device-local PCM file as notification")
	fmt.Println("  Example: soundtouch-cli speaker notify --path \"/opt/Bose/chimes/grouped.pcm\"")
	fmt.Println()
	fmt.Println("Notes:")
	fmt.Println("• Only ST-10 (Series III) speakers support the /speaker endpoint")
	fmt.Println("• ST-300 and other models may not support this functionality")
	fmt.Println("• You need to provide your own app_key for TTS and URL playback")
	fmt.Println("• Currently playing content is paused during playback and resumed after")
	fmt.Println("• If device is a zone master, content plays on all zone members")
	fmt.Println("• Volume is automatically restored after playback completes")
	fmt.Println()
	fmt.Println("Supported Languages for TTS:")
	fmt.Println("EN (English), DE (German), ES (Spanish), FR (French), IT (Italian),")
	fmt.Println("NL (Dutch), PT (Portuguese), RU (Russian), ZH (Chinese), JA (Japanese)")

	return nil
}
