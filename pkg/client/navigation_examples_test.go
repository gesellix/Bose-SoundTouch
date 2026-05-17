package client

import (
	"fmt"
	"log"
)

// ExampleClient_Navigate demonstrates basic navigation of content sources
func ExampleClient_Navigate() {
	config := &Config{Host: "192.0.2.100", Port: 8090}
	client := NewClient(config)

	// Navigate TuneIn content
	response, err := client.Navigate("TUNEIN", "", 1, 10)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d items in TuneIn\n", response.TotalItems)

	for _, item := range response.Items {
		fmt.Printf("- %s (%s)\n", item.GetDisplayName(), item.Type)
	}
}

// ExampleClient_SearchStation demonstrates searching for radio stations
func ExampleClient_SearchStation() {
	config := &Config{Host: "192.0.2.100", Port: 8090}
	client := NewClient(config)

	// Search for jazz stations on TuneIn
	results, err := client.SearchTuneInStations("jazz")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d search results\n", results.GetResultCount())

	// Show stations found
	stations := results.GetStations()
	for _, station := range stations {
		fmt.Printf("Station: %s\n", station.GetDisplayName())

		if station.Description != "" {
			fmt.Printf("  Description: %s\n", station.Description)
		}
	}
}

// ExampleClient_AddStation demonstrates adding a station and playing it
func ExampleClient_AddStation() {
	config := &Config{Host: "192.0.2.100", Port: 8090}
	client := NewClient(config)

	// First, search for content to get a token
	results, err := client.SearchPandoraStations("user123", "classic rock")
	if err != nil {
		log.Fatal(err)
	}

	// Find an artist to create a station from
	artists := results.GetArtists()
	if len(artists) == 0 {
		fmt.Println("No artists found")
		return
	}

	artist := artists[0]
	stationName := artist.Name + " Radio"

	// Add the station (this immediately starts playing it)
	err = client.AddStation("PANDORA", "user123", artist.Token, stationName)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Added and started playing: %s\n", stationName)
}

// Example_navigationWorkflow demonstrates a complete workflow
func Example_navigationWorkflow() {
	config := &Config{Host: "192.0.2.100", Port: 8090}
	client := NewClient(config)

	// 1. Search for content
	fmt.Println("Searching for Taylor Swift...")

	searchResults, err := client.SearchPandoraStations("user123", "Taylor Swift")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d total results\n", searchResults.GetResultCount())

	// 2. Show different types of results
	songs := searchResults.GetSongs()
	artists := searchResults.GetArtists()
	stations := searchResults.GetStations()

	fmt.Printf("Songs: %d, Artists: %d, Stations: %d\n",
		len(songs), len(artists), len(stations))

	// 3. Find an artist to create a station from
	if len(artists) > 0 {
		artist := artists[0]
		fmt.Printf("Creating station from artist: %s (Token: %s)\n",
			artist.Name, artist.Token)

		// Note: In a real scenario, you'd call AddStation here
		// This would immediately start playing the new station
		fmt.Printf("Would add station: %s Radio\n", artist.Name)
	}

	// 4. Browse existing Pandora stations
	fmt.Println("\nBrowsing existing Pandora stations...")

	pandoraStations, err := client.GetPandoraStations("user123")
	if err != nil {
		fmt.Printf("Could not get Pandora stations: %v\n", err)
		return
	}

	fmt.Printf("Found %d existing stations\n", len(pandoraStations.Items))

	// 5. Show how to remove a station (if any exist)
	if len(pandoraStations.Items) > 0 {
		station := pandoraStations.Items[0]
		if station.ContentItem != nil {
			fmt.Printf("Could remove station: %s\n", station.GetDisplayName())
			// err := client.RemoveStation(station.ContentItem)
		}
	}
}

// ExampleClient_NavigateContainer demonstrates browsing into directories
func ExampleClient_NavigateContainer() {
	config := &Config{Host: "192.0.2.100", Port: 8090}
	client := NewClient(config)

	// First, get the stored music library root
	musicLibrary, err := client.GetStoredMusicLibrary("device123/0")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Music library has %d items\n", musicLibrary.TotalItems)

	// Find a directory to browse into
	directories := musicLibrary.GetDirectories()
	if len(directories) == 0 {
		fmt.Println("No directories found")
		return
	}

	// Browse into the first directory
	directory := directories[0]
	fmt.Printf("Browsing into: %s\n", directory.GetDisplayName())

	contents, err := client.NavigateContainer(
		"STORED_MUSIC",
		"device123/0",
		1, 100,
		directory.ContentItem,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Show what's in the directory
	tracks := contents.GetTracks()
	subdirs := contents.GetDirectories()

	fmt.Printf("Found %d tracks and %d subdirectories\n",
		len(tracks), len(subdirs))

	// Show first few tracks
	for i, track := range tracks[:minInt(3, len(tracks))] {
		fmt.Printf("%d. %s", i+1, track.GetDisplayName())

		if track.ArtistName != "" {
			fmt.Printf(" - %s", track.ArtistName)
		}

		fmt.Println()
	}
}

// Example_searchAndPlayWorkflow demonstrates search -> add -> play workflow
func Example_searchAndPlayWorkflow() {
	config := &Config{Host: "192.0.2.100", Port: 8090}
	client := NewClient(config)

	searchTerm := "classic rock"
	fmt.Printf("Searching for '%s'...\n", searchTerm)

	// 1. Search for content
	results, err := client.SearchTuneInStations(searchTerm)
	if err != nil {
		log.Fatal(err)
	}

	stations := results.GetStations()
	if len(stations) == 0 {
		fmt.Println("No stations found")
		return
	}

	// 2. Show available stations
	fmt.Printf("Found %d stations\n", len(stations))

	for i, station := range stations[:minInt(5, len(stations))] {
		fmt.Printf("%d. %s", i+1, station.GetDisplayName())

		if station.Description != "" {
			fmt.Printf(" - %s", station.Description)
		}

		fmt.Println()
	}

	// 3. In a real app, user would select one
	selectedStation := stations[0]
	fmt.Printf("\nSelected: %s\n", selectedStation.GetDisplayName())

	// 4. For TuneIn, you might need to add it as a station first
	// (depending on the service and how the API works)
	if selectedStation.Token != "" {
		fmt.Printf("Would add station with token: %s\n", selectedStation.Token)
		// err := client.AddStation("TUNEIN", "", selectedStation.Token, selectedStation.Name)
	}

	fmt.Println("Station would now be playing!")
}

// Helper function for min calculation
func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
