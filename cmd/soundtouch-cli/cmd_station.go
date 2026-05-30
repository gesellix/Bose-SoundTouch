package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/stations"
	"github.com/urfave/cli/v2"
)

// searchStations handles searching for stations across different sources
func searchStations(c *cli.Context) error {
	PrintDeprecation(
		"station search",
		"It asks the speaker to search, which fails when the speaker's cloud is gone.",
		`soundtouch-cli station find --provider tunein --query "<your search>"`,
	)

	source := c.String("source")
	sourceAccount := c.String("source-account")
	searchTerm := c.String("query")

	if searchTerm == "" {
		PrintError("Search query is required")
		return fmt.Errorf("search query cannot be empty")
	}

	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Searching %s for: %s", source, searchTerm), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Check service availability for the source
	checker := NewServiceAvailabilityChecker(client)

	actionDescription := fmt.Sprintf("search %s stations", source)
	if !checker.CheckSourceAvailable(source, actionDescription) {
		return fmt.Errorf("source '%s' is not available for station search", source)
	}

	response, err := client.SearchStation(source, sourceAccount, searchTerm)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to search stations: %v", err))
		return err
	}

	printSearchResults(response, searchTerm)

	return nil
}

// searchTuneIn handles searching TuneIn specifically
func searchTuneIn(c *cli.Context) error {
	PrintDeprecation(
		"station search-tunein",
		"It asks the speaker to search, which fails when the speaker's cloud is gone.",
		`soundtouch-cli station find-tunein --query "<your search>"`,
	)

	searchTerm := c.String("query")

	if searchTerm == "" {
		PrintError("Search query is required")
		return fmt.Errorf("search query cannot be empty")
	}

	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Searching TuneIn for: %s", searchTerm), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Check TuneIn availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.ValidateTuneInAvailable("search TuneIn stations") {
		return fmt.Errorf("TuneIn is not available on this device")
	}

	response, err := client.SearchTuneInStations(searchTerm)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to search TuneIn: %v", err))
		return err
	}

	printSearchResults(response, searchTerm)

	return nil
}

// searchPandora handles searching Pandora specifically
func searchPandora(c *cli.Context) error {
	PrintDeprecation(
		"station search-pandora",
		"There is no built-in Pandora search yet (it requires the speaker and your account).",
		"",
	)

	sourceAccount := c.String("source-account")
	searchTerm := c.String("query")

	if sourceAccount == "" {
		PrintError("Pandora source account is required")
		return fmt.Errorf("source account required for Pandora")
	}

	if searchTerm == "" {
		PrintError("Search query is required")
		return fmt.Errorf("search query cannot be empty")
	}

	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Searching Pandora for: %s", searchTerm), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Check Pandora availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.ValidatePandoraAvailable("search Pandora stations") {
		return fmt.Errorf("pandora is not available on this device")
	}

	response, err := client.SearchPandoraStations(sourceAccount, searchTerm)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to search Pandora: %v", err))
		return err
	}

	printSearchResults(response, searchTerm)

	return nil
}

// searchSpotify handles searching Spotify specifically
func searchSpotify(c *cli.Context) error {
	PrintDeprecation(
		"station search-spotify",
		"There is no built-in Spotify search yet (it requires the speaker and your account).",
		"",
	)

	sourceAccount := c.String("source-account")
	searchTerm := c.String("query")

	if sourceAccount == "" {
		PrintError("Spotify source account is required")
		return fmt.Errorf("source account required for Spotify")
	}

	if searchTerm == "" {
		PrintError("Search query is required")
		return fmt.Errorf("search query cannot be empty")
	}

	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Searching Spotify for: %s", searchTerm), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Check Spotify availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.ValidateSpotifyAvailable("search Spotify content") {
		return fmt.Errorf("spotify is not available on this device")
	}

	response, err := client.SearchSpotifyContent(sourceAccount, searchTerm)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to search Spotify: %v", err))
		return err
	}

	printSearchResults(response, searchTerm)

	return nil
}

// addStation handles adding a station and playing it immediately
func addStation(c *cli.Context) error {
	source := c.String("source")
	sourceAccount := c.String("source-account")
	token := c.String("token")
	name := c.String("name")

	if source == "" {
		PrintError("Source is required")
		return fmt.Errorf("source cannot be empty")
	}

	if token == "" {
		PrintError("Station token is required")
		return fmt.Errorf("token cannot be empty")
	}

	if name == "" {
		PrintError("Station name is required")
		return fmt.Errorf("name cannot be empty")
	}

	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Adding %s station: %s", source, name), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Check service availability for the source
	checker := NewServiceAvailabilityChecker(client)

	actionDescription := fmt.Sprintf("add %s station", source)
	if !checker.CheckSourceAvailable(source, actionDescription) {
		return fmt.Errorf("source '%s' is not available for adding stations", source)
	}

	err = client.AddStation(source, sourceAccount, token, name)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to add station: %v", err))
		return err
	}

	PrintSuccess(fmt.Sprintf("Added and started playing station: %s", name))

	return nil
}

// removeStation handles removing a station from collections
func removeStation(c *cli.Context) error {
	source := c.String("source")
	location := c.String("location")
	itemType := c.String("type")
	sourceAccount := c.String("source-account")

	if source == "" {
		PrintError("Source is required")
		return fmt.Errorf("source cannot be empty")
	}

	if location == "" {
		PrintError("Station location is required")
		return fmt.Errorf("location cannot be empty")
	}

	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Removing %s station", source), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Create content item for the station to remove
	contentItem := &models.ContentItem{
		Source:        source,
		Location:      location,
		Type:          itemType,
		SourceAccount: sourceAccount,
	}

	err = client.RemoveStation(contentItem)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to remove station: %v", err))
		return err
	}

	PrintSuccess("Station removed successfully")

	return nil
}

// printSearchResults formats and displays search results
func printSearchResults(response *models.SearchStationResponse, searchTerm string) {
	fmt.Printf("Search Results for '%s':\n", searchTerm)

	if response.IsEmpty() {
		fmt.Printf("  No results found\n")
		return
	}

	fmt.Printf("  Total results: %d\n", response.GetResultCount())

	// Group results by type for better display
	songs := response.GetSongs()
	artists := response.GetArtists()
	stations := response.GetStations()

	printSongs(songs)
	printArtists(artists)
	printStations(stations)
	printSearchHints(response, songs, artists, stations)
}

// printSongs prints song search results
func printSongs(songs []models.SearchResult) {
	if len(songs) == 0 {
		return
	}

	fmt.Printf("\n  🎵 Songs (%d):\n", len(songs))

	for i := range songs {
		song := &songs[i]
		fmt.Printf("    %d. %s\n", i+1, song.GetDisplayName())

		if song.Artist != "" {
			fmt.Printf("       Artist: %s\n", song.Artist)
		}

		if song.Album != "" {
			fmt.Printf("       Album: %s\n", song.Album)
		}

		if song.SourceAccount != "" {
			fmt.Printf("       Account: %s\n", song.SourceAccount)
		}

		fmt.Printf("       Token: %s\n", song.Token)
		fmt.Println()
	}
}

// printArtists prints artist search results
func printArtists(artists []models.SearchResult) {
	if len(artists) == 0 {
		return
	}

	fmt.Printf("  🎤 Artists (%d):\n", len(artists))

	for i := range artists {
		artist := &artists[i]
		fmt.Printf("    %d. %s\n", i+1, artist.GetDisplayName())

		if artist.SourceAccount != "" {
			fmt.Printf("       Account: %s\n", artist.SourceAccount)
		}

		fmt.Printf("       Token: %s\n", artist.Token)
		fmt.Println()
	}
}

// printStations prints station search results
func printStations(stations []models.SearchResult) {
	if len(stations) == 0 {
		return
	}

	fmt.Printf("  📻 Stations (%d):\n", len(stations))

	for i := range stations {
		station := &stations[i]
		fmt.Printf("    %d. %s\n", i+1, station.GetDisplayName())

		if station.SourceAccount != "" {
			fmt.Printf("       Account: %s\n", station.SourceAccount)
		}

		fmt.Printf("       Token: %s\n", station.Token)

		if station.Description != "" {
			fmt.Printf("       Description: %s\n", station.Description)
		}

		fmt.Println()
	}
}

// printSearchHints prints usage hints for search results
func printSearchHints(response *models.SearchStationResponse, songs, artists, stations []models.SearchResult) {
	fmt.Printf("💡 Usage hints:\n")
	fmt.Printf("   • To add a station and play it: station add --source %s --token <token> --name <name>\n", response.Source)

	if hasAccountResults(response) {
		fmt.Printf("   • Include --source-account <account> when adding stations that require it\n")
	}

	if len(songs) > 0 || len(artists) > 0 || len(stations) > 0 {
		fmt.Printf("   • Copy the token from results above to use with 'station add'\n")
	}
}

// hasAccountResults checks if any results have source accounts
func hasAccountResults(response *models.SearchStationResponse) bool {
	allResults := response.GetAllResults()
	for i := range allResults {
		if allResults[i].SourceAccount != "" {
			return true
		}
	}

	return false
}

// listStations handles listing saved stations
func listStations(c *cli.Context) error {
	source := c.String("source")
	sourceAccount := c.String("source-account")

	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Getting %s stations", source), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Check service availability for the source
	checker := NewServiceAvailabilityChecker(client)

	actionDescription := fmt.Sprintf("list %s stations", source)
	if !checker.CheckSourceAvailable(source, actionDescription) {
		return fmt.Errorf("source '%s' is not available for listing stations", source)
	}

	var response *models.NavigateResponse

	switch strings.ToUpper(source) {
	case "TUNEIN":
		response, err = client.GetTuneInStations(sourceAccount)
	case "PANDORA":
		if sourceAccount == "" {
			PrintError("Pandora source account is required")
			return fmt.Errorf("source account required for Pandora")
		}

		response, err = client.GetPandoraStations(sourceAccount)
	default:
		return fmt.Errorf("listing stations is not supported for source: %s", source)
	}

	if err != nil {
		PrintError(fmt.Sprintf("Failed to get stations: %v", err))
		return err
	}

	printStationList(response, source)

	return nil
}

// printStationList formats and displays saved station results
func printStationList(response *models.NavigateResponse, source string) {
	fmt.Printf("Saved %s Stations:\n", source)

	if response.TotalItems == 0 {
		fmt.Printf("  No stations found\n")
		return
	}

	stations := response.GetStations()
	fmt.Printf("  Total stations: %d\n", response.TotalItems)
	fmt.Printf("  Showing: %d\n\n", len(stations))

	for i, station := range stations {
		fmt.Printf("  %d. %s\n", i+1, station.Name)

		if station.ContentItem != nil {
			if station.ContentItem.Location != "" {
				fmt.Printf("     Location: %s\n", station.ContentItem.Location)
			}

			if station.ContentItem.SourceAccount != "" {
				fmt.Printf("     Account: %s\n", station.ContentItem.SourceAccount)
			}

			if station.ContentItem.IsPresetable {
				fmt.Printf("     Can be saved as preset: Yes\n")
			}
		}

		if station.Type != "" {
			fmt.Printf("     Type: %s\n", station.Type)
		}

		fmt.Println()
	}

	// Show usage hints
	fmt.Printf("💡 Usage hints:\n")
	fmt.Printf("   • To play a station: Use the location value with 'play content' command\n")
	fmt.Printf("   • To save as preset: Use 'preset set' command with the location\n")
}

// printBmxNavResults renders a *models.BmxNavResponse to stdout.
// For each section it prints the section name as a header, then each item's
// name, subtitle, and playback location so the user can act on it.
func printBmxNavResults(resp *models.BmxNavResponse) {
	if len(resp.BmxSections) == 0 {
		fmt.Println("  No results found")
		return
	}

	for _, section := range resp.BmxSections {
		if section.Name != "" {
			fmt.Printf("\n  [%s]\n", section.Name)
		}

		if len(section.Items) == 0 {
			fmt.Println("    (empty)")
			continue
		}

		for i, item := range section.Items {
			fmt.Printf("  %3d. %s\n", i+1, item.Name)

			if item.Subtitle != "" {
				fmt.Printf("       %s\n", item.Subtitle)
			}

			if item.Links != nil && item.Links.BmxPlayback != nil {
				fmt.Printf("       Location: %s\n", item.Links.BmxPlayback.Href)
			}
		}
	}
}

// bmxNavCursor extracts the opaque cursor value from a section's BmxNext link.
// The Href looks like "...?cursor=<value>"; this returns the cursor query param.
// Returns "" when no next link is present.
func bmxNavCursor(section *models.BmxNavSection) string {
	if section == nil || section.Links == nil || section.Links.BmxNext == nil {
		return ""
	}

	href := section.Links.BmxNext.Href
	if href == "" {
		return ""
	}

	// The cursor is the query parameter named "cursor".
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}

	return parsed.Query().Get("cursor")
}

// runFind performs a built-in station search for the given provider and
// prints the results. The search runs inside the CLI itself, querying the
// radio provider's public API directly — it needs neither the speaker's
// cloud nor a running soundtouch-service. When more is true it follows up
// to three additional result pages while a next cursor is available.
func runFind(provider stations.Provider, label, query string, more bool) error {
	if query == "" {
		PrintError("Search query is required")
		return fmt.Errorf("search query cannot be empty")
	}

	fmt.Printf("Searching %s for: %s\n", label, query)

	resp, err := stations.Search(provider, query)
	if err != nil {
		PrintError(fmt.Sprintf("Search failed: %v", err))
		return err
	}

	printBmxNavResults(resp)

	if !more {
		return nil
	}

	const maxExtraPages = 3
	for page := 0; page < maxExtraPages; page++ {
		// Find a cursor from any section that has one.
		cursor := ""
		for i := range resp.BmxSections {
			cursor = bmxNavCursor(&resp.BmxSections[i])
			if cursor != "" {
				break
			}
		}

		if cursor == "" {
			break
		}

		fmt.Printf("\n  -- page %d --\n", page+2)

		resp, err = stations.SearchNext(provider, cursor)
		if err != nil {
			PrintError(fmt.Sprintf("Failed to fetch next page: %v", err))
			return err
		}

		printBmxNavResults(resp)
	}

	return nil
}

// findStations is the action for the unified `station find` with
// --provider / --query / --more.
func findStations(c *cli.Context) error {
	providerStr := c.String("provider")

	var (
		provider stations.Provider
		label    string
	)

	switch strings.ToLower(providerStr) {
	case "tunein":
		provider, label = stations.ProviderTuneIn, "TuneIn"
	case "radiobrowser":
		provider, label = stations.ProviderRadioBrowser, "Radio Browser"
	default:
		PrintError(fmt.Sprintf("Unknown provider %q: must be 'tunein' or 'radiobrowser'", providerStr))
		return fmt.Errorf("unknown provider: %s", providerStr)
	}

	return runFind(provider, label, c.String("query"), c.Bool("more"))
}

// findTuneIn is the action for `station find-tunein` (built-in TuneIn search).
func findTuneIn(c *cli.Context) error {
	return runFind(stations.ProviderTuneIn, "TuneIn", c.String("query"), c.Bool("more"))
}

// findRadioBrowser is the action for `station find-radiobrowser`
// (built-in Radio Browser search).
func findRadioBrowser(c *cli.Context) error {
	return runFind(stations.ProviderRadioBrowser, "Radio Browser", c.String("query"), c.Bool("more"))
}
