package main

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/urfave/cli/v2"
)

func printSource(source models.SourceItem) {
	fmt.Printf("    • %s", source.GetDisplayName())

	if source.SourceAccount != "" && source.SourceAccount != source.Source {
		fmt.Printf(" (%s)", source.SourceAccount)
	}

	var attributes []string
	if source.IsLocalSource() {
		attributes = append(attributes, "Local")
		attributes = append(attributes, "Available")
	}

	if len(attributes) > 0 {
		fmt.Printf(" [%s]", strings.Join(attributes, ", "))
	}

	fmt.Println()
}

// listSources handles listing available audio sources
func listSources(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	PrintDeviceHeader("Getting available sources", clientConfig.Host, clientConfig.Port)

	sources, err := client.GetSources()
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}

	fmt.Printf("Available Audio Sources:\n")
	fmt.Printf("  Device ID: %s\n", sources.DeviceID)

	// Show ready sources first
	availableSources := sources.GetAvailableSources()
	if len(availableSources) > 0 {
		fmt.Printf("  Ready Sources:\n")

		for _, source := range availableSources {
			printSource(source)
		}
	}

	// Show all configured sources
	fmt.Printf("  All Sources:\n")

	for _, source := range sources.SourceItem {
		status := "Available"
		if !source.IsLocalSource() {
			status = "Remote"
		}

		fmt.Printf("    • %s (%s)\n", source.GetDisplayName(), status)

		if source.SourceAccount != "" && source.SourceAccount != source.Source {
			fmt.Printf("      Account: %s\n", source.SourceAccount)
		}
	}

	// Show streaming sources
	streamingSources := sources.GetStreamingSources()
	if len(streamingSources) > 0 {
		fmt.Printf("  Streaming Services:\n")

		for _, source := range streamingSources {
			fmt.Printf("    • %s", source.GetDisplayName())

			if source.SourceAccount != "" {
				fmt.Printf(" (%s)", source.SourceAccount)
			}

			fmt.Println()
		}
	}

	// Show service availability summary
	fmt.Println()

	checker := NewServiceAvailabilityChecker(client)
	checker.PrintServiceAvailabilitySummary()

	return nil
}

// selectSource handles selecting an audio source
func selectSource(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	sourceName := strings.ToUpper(c.String("source"))
	sourceAccount := c.String("account")

	// Check service availability
	checker := NewServiceAvailabilityChecker(client)

	actionDescription := fmt.Sprintf("select %s source", strings.ToLower(sourceName))
	if !checker.CheckSourceAvailable(sourceName, actionDescription) {
		return fmt.Errorf("source '%s' is not available", sourceName)
	}

	PrintDeviceHeader(fmt.Sprintf("Selecting source '%s'", sourceName), clientConfig.Host, clientConfig.Port)

	err = client.SelectSource(sourceName, sourceAccount)
	if err != nil {
		return fmt.Errorf("failed to select source: %w", err)
	}

	if sourceAccount != "" {
		PrintSuccess(fmt.Sprintf("Source '%s' with account '%s' selected", sourceName, sourceAccount))
	} else {
		PrintSuccess(fmt.Sprintf("Source '%s' selected", sourceName))
	}

	return nil
}

// selectSpotify handles selecting Spotify source
func selectSpotify(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	// Check Spotify availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.ValidateSpotifyAvailable("select Spotify source") {
		return fmt.Errorf("spotify is not available on this device")
	}

	PrintDeviceHeader("Selecting Spotify source", clientConfig.Host, clientConfig.Port)

	err = client.SelectSpotify("")
	if err != nil {
		return fmt.Errorf("failed to select Spotify: %w", err)
	}

	PrintSuccess("Spotify source selected")

	return nil
}

// selectBluetooth handles selecting Bluetooth source
func selectBluetooth(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	// Check Bluetooth availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.ValidateBluetoothAvailable("select Bluetooth source") {
		return fmt.Errorf("bluetooth is not available on this device")
	}

	PrintDeviceHeader("Selecting Bluetooth source", clientConfig.Host, clientConfig.Port)

	err = client.SelectBluetooth()
	if err != nil {
		return fmt.Errorf("failed to select Bluetooth: %w", err)
	}

	PrintSuccess("Bluetooth source selected")

	return nil
}

// selectAux handles selecting AUX input source
func selectAux(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	PrintDeviceHeader("Selecting AUX input source", clientConfig.Host, clientConfig.Port)

	err = client.SelectAux()
	if err != nil {
		return fmt.Errorf("failed to select AUX: %w", err)
	}

	PrintSuccess("AUX input source selected")

	return nil
}

// selectLocalInternetRadio handles selecting LOCAL_INTERNET_RADIO source
func selectLocalInternetRadio(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	location := c.String("location")
	if location == "" {
		return fmt.Errorf("location is required (use --location)")
	}

	sourceAccount := c.String("account")
	itemName := c.String("name")
	containerArt := c.String("artwork")

	// Check LOCAL_INTERNET_RADIO availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.CheckSourceAvailable("LOCAL_INTERNET_RADIO", "select internet radio") {
		return fmt.Errorf("LOCAL_INTERNET_RADIO is not available")
	}

	PrintDeviceHeader("Selecting internet radio stream", clientConfig.Host, clientConfig.Port)

	if itemName != "" {
		fmt.Printf("  Station: %s\n", itemName)
	}

	fmt.Printf("  Location: %s\n", location)

	err = client.SelectLocalInternetRadio(location, sourceAccount, itemName, containerArt)
	if err != nil {
		return fmt.Errorf("failed to select internet radio: %w", err)
	}

	PrintSuccess("Internet radio stream selected")

	return nil
}

// selectCustomRadio handles selecting custom radio stream via soundtouch-service
func selectCustomRadio(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	streamURL := c.String("url")
	itemName := c.String("name")
	containerArt := c.String("artwork")
	serviceURL := c.String("service-url")

	encodedURL := base64.URLEncoding.EncodeToString([]byte(streamURL))
	location := fmt.Sprintf("%s/custom/v1/playback/%s", serviceURL, encodedURL)

	params := url.Values{}
	if itemName != "" {
		params.Add("name", itemName)
	}

	if containerArt != "" {
		params.Add("imageUrl", containerArt)
	}

	if len(params) > 0 {
		location += "?" + params.Encode()
	}

	// Check LOCAL_INTERNET_RADIO availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.CheckSourceAvailable("LOCAL_INTERNET_RADIO", "select custom radio") {
		return fmt.Errorf("LOCAL_INTERNET_RADIO is not available")
	}

	PrintDeviceHeader("Selecting custom radio stream", clientConfig.Host, clientConfig.Port)

	if itemName != "" {
		fmt.Printf("  Station: %s\n", itemName)
	}

	fmt.Printf("  URL: %s\n", streamURL)
	fmt.Printf("  Proxy: %s\n", location)

	err = client.SelectLocalInternetRadio(location, "", itemName, containerArt)
	if err != nil {
		return fmt.Errorf("failed to select custom radio: %w", err)
	}

	PrintSuccess("Custom radio stream selected")

	return nil
}

// selectLocalMusic handles selecting LOCAL_MUSIC source
func selectLocalMusic(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	location := c.String("location")
	if location == "" {
		return fmt.Errorf("location is required (use --location)")
	}

	sourceAccount := c.String("account")
	if sourceAccount == "" {
		return fmt.Errorf("account is required for LOCAL_MUSIC (use --account)")
	}

	itemName := c.String("name")
	containerArt := c.String("artwork")

	// Check LOCAL_MUSIC availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.CheckSourceAvailable("LOCAL_MUSIC", "select local music") {
		return fmt.Errorf("LOCAL_MUSIC is not available")
	}

	PrintDeviceHeader("Selecting local music content", clientConfig.Host, clientConfig.Port)

	if itemName != "" {
		fmt.Printf("  Content: %s\n", itemName)
	}

	fmt.Printf("  Location: %s\n", location)
	fmt.Printf("  Account: %s\n", sourceAccount)

	err = client.SelectLocalMusic(location, sourceAccount, itemName, containerArt)
	if err != nil {
		return fmt.Errorf("failed to select local music: %w", err)
	}

	PrintSuccess("Local music content selected")

	return nil
}

// selectStoredMusic handles selecting STORED_MUSIC source
func selectStoredMusic(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	location := c.String("location")
	if location == "" {
		return fmt.Errorf("location is required (use --location)")
	}

	sourceAccount := c.String("account")
	if sourceAccount == "" {
		return fmt.Errorf("account is required for STORED_MUSIC (use --account)")
	}

	itemName := c.String("name")
	containerArt := c.String("artwork")

	// Check STORED_MUSIC availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.CheckSourceAvailable("STORED_MUSIC", "select stored music") {
		return fmt.Errorf("STORED_MUSIC is not available")
	}

	PrintDeviceHeader("Selecting stored music content", clientConfig.Host, clientConfig.Port)

	if itemName != "" {
		fmt.Printf("  Content: %s\n", itemName)
	}

	fmt.Printf("  Location: %s\n", location)
	fmt.Printf("  Account: %s\n", sourceAccount)

	err = client.SelectStoredMusic(location, sourceAccount, itemName, containerArt)
	if err != nil {
		return fmt.Errorf("failed to select stored music: %w", err)
	}

	PrintSuccess("Stored music content selected")

	return nil
}

// selectContent handles selecting content using a ContentItem directly
func selectContent(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	// Required parameters
	source := strings.ToUpper(c.String("source"))
	if source == "" {
		return fmt.Errorf("source is required (use --source)")
	}

	location := c.String("location")
	if location == "" {
		return fmt.Errorf("location is required (use --location)")
	}

	// Optional parameters
	sourceAccount := c.String("account")
	itemName := c.String("name")
	containerArt := c.String("artwork")
	itemType := c.String("type")
	isPresetable := c.Bool("presetable")

	// Create ContentItem
	contentItem := &models.ContentItem{
		Source:        source,
		Type:          itemType,
		Location:      location,
		SourceAccount: sourceAccount,
		IsPresetable:  isPresetable,
		ItemName:      itemName,
		ContainerArt:  containerArt,
	}

	// Set default type if not specified
	if itemType == "" {
		switch source {
		case "SPOTIFY":
			contentItem.Type = "uri"
		case "TUNEIN", "LOCAL_INTERNET_RADIO":
			contentItem.Type = "stationurl"
		case "LOCAL_MUSIC":
			contentItem.Type = "album" // default, could be track, artist, etc.
		}
	}

	// Set default item name if not specified
	if itemName == "" {
		contentItem.ItemName = source
	}

	PrintDeviceHeader("Selecting content", clientConfig.Host, clientConfig.Port)

	fmt.Printf("  Source: %s\n", source)
	fmt.Printf("  Location: %s\n", location)

	if sourceAccount != "" {
		fmt.Printf("  Account: %s\n", sourceAccount)
	}

	if itemName != "" {
		fmt.Printf("  Name: %s\n", itemName)
	}

	if itemType != "" {
		fmt.Printf("  Type: %s\n", itemType)
	}

	err = client.SelectContentItem(contentItem)
	if err != nil {
		return fmt.Errorf("failed to select content: %w", err)
	}

	PrintSuccess("Content selected")

	return nil
}

// getServiceAvailability handles displaying service availability information
func getServiceAvailability(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	PrintDeviceHeader("Getting service availability", clientConfig.Host, clientConfig.Port)

	serviceAvailability, err := client.GetServiceAvailability()
	if err != nil {
		return fmt.Errorf("failed to get service availability: %w", err)
	}

	fmt.Printf("Service Availability Report:\n")
	fmt.Printf("  Total Services: %d\n", serviceAvailability.GetServiceCount())
	fmt.Printf("  Available Services: %d\n", serviceAvailability.GetAvailableServiceCount())
	fmt.Printf("  Unavailable Services: %d\n", serviceAvailability.GetUnavailableServiceCount())

	// Show available services
	fmt.Printf("\n✅ Available Services:\n")

	availableServices := serviceAvailability.GetAvailableServices()
	if len(availableServices) == 0 {
		fmt.Printf("    None\n")
	} else {
		for _, service := range availableServices {
			fmt.Printf("    • %s\n", formatServiceTypeForDisplay(models.ServiceType(service.Type)))
		}
	}

	// Show unavailable services with reasons
	fmt.Printf("\n❌ Unavailable Services:\n")

	unavailableServices := serviceAvailability.GetUnavailableServices()
	if len(unavailableServices) == 0 {
		fmt.Printf("    None\n")
	} else {
		for _, service := range unavailableServices {
			reason := ""
			if service.Reason != "" {
				reason = fmt.Sprintf(" (%s)", service.Reason)
			}

			fmt.Printf("    • %s%s\n", formatServiceTypeForDisplay(models.ServiceType(service.Type)), reason)
		}
	}

	// Show service categories
	fmt.Printf("\n🎵 Streaming Services:\n")

	streamingServices := serviceAvailability.GetStreamingServices()
	availableCount := 0

	for _, service := range streamingServices {
		status := "❌"
		if service.IsAvailable {
			status = "✅"
			availableCount++
		}

		fmt.Printf("    %s %s\n", status, formatServiceTypeForDisplay(models.ServiceType(service.Type)))
	}

	fmt.Printf("    Summary: %d/%d streaming services available\n", availableCount, len(streamingServices))

	fmt.Printf("\n🔗 Local Input Services:\n")

	localServices := serviceAvailability.GetLocalServices()
	localAvailableCount := 0

	for _, service := range localServices {
		status := "❌"
		if service.IsAvailable {
			status = "✅"
			localAvailableCount++
		}

		fmt.Printf("    %s %s\n", status, formatServiceTypeForDisplay(models.ServiceType(service.Type)))
	}

	fmt.Printf("    Summary: %d/%d local services available\n", localAvailableCount, len(localServices))

	return nil
}

// compareSourcesAndAvailability compares configured sources with service availability
func compareSourcesAndAvailability(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	PrintDeviceHeader("Comparing sources and service availability", clientConfig.Host, clientConfig.Port)

	// Get both sources and service availability
	sources, err := client.GetSources()
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}

	serviceAvailability, err := client.GetServiceAvailability()
	if err != nil {
		return fmt.Errorf("failed to get service availability: %w", err)
	}

	fmt.Printf("Source vs Availability Comparison:\n\n")

	performSourceComparisons(sources, serviceAvailability)
	printSourceSummary(sources, serviceAvailability)

	return nil
}

// performSourceComparisons compares configured sources with availability
func performSourceComparisons(sources *models.Sources, serviceAvailability *models.ServiceAvailability) {
	// Check key services
	comparisons := []struct {
		name                 string
		configuredCheck      func() bool
		availableCheck       func() bool
		getConfiguredSources func() []models.SourceItem
	}{
		{
			"Spotify",
			sources.HasSpotify,
			serviceAvailability.HasSpotify,
			sources.GetSpotifySources,
		},
		{
			"Bluetooth",
			sources.HasBluetooth,
			serviceAvailability.HasBluetooth,
			func() []models.SourceItem { return sources.GetSourcesByType("BLUETOOTH") },
		},
	}

	for _, comp := range comparisons {
		compareServiceStatus(comp.name, comp.configuredCheck(), comp.availableCheck(), serviceAvailability)
	}
}

// compareServiceStatus compares a single service's configuration vs availability
func compareServiceStatus(serviceName string, configured, available bool, serviceAvailability *models.ServiceAvailability) {
	fmt.Printf("🔍 %s:\n", serviceName)
	fmt.Printf("    Configured: %s\n", boolToStatus(configured))
	fmt.Printf("    Available: %s\n", boolToStatus(available))

	switch {
	case available && !configured:
		fmt.Printf("    💡 %s is available but not configured - consider setting it up\n", serviceName)
	case configured && !available:
		fmt.Printf("    ⚠️  %s is configured but not available - check device status\n", serviceName)
		printServiceUnavailableReason(serviceName, serviceAvailability)
	case configured && available:
		fmt.Printf("    ✅ %s is properly configured and available\n", serviceName)
	default:
		fmt.Printf("    ➖ %s is neither configured nor available\n", serviceName)
	}

	fmt.Println()
}

// printServiceUnavailableReason prints the reason why a service is unavailable
func printServiceUnavailableReason(serviceName string, serviceAvailability *models.ServiceAvailability) {
	var service *models.Service

	switch serviceName {
	case "Spotify":
		service = serviceAvailability.GetServiceByType(models.ServiceTypeSpotify)
	case "Bluetooth":
		service = serviceAvailability.GetServiceByType(models.ServiceTypeBluetooth)
	}

	if service != nil && service.Reason != "" {
		fmt.Printf("    📝 Reason: %s\n", service.Reason)
	}
}

// printSourceSummary prints a summary of sources and services
func printSourceSummary(sources *models.Sources, serviceAvailability *models.ServiceAvailability) {
	// Summary
	fmt.Printf("📊 Summary:\n")
	fmt.Printf("    Total configured sources: %d\n", sources.GetSourceCount())
	fmt.Printf("    Ready configured sources: %d\n", sources.GetReadySourceCount())
	fmt.Printf("    Total available services: %d\n", serviceAvailability.GetAvailableServiceCount())
	fmt.Printf("    Total possible services: %d\n", serviceAvailability.GetServiceCount())
}

// boolToStatus converts boolean to user-friendly status
func boolToStatus(b bool) string {
	if b {
		return "✅ Yes"
	}

	return "❌ No"
}
