// Package client provides a comprehensive HTTP client for controlling Bose SoundTouch devices.
//
// This package implements the complete Bose SoundTouch Web API, enabling full programmatic
// control of SoundTouch speakers including playback control, volume management, source
// selection, multiroom zone management, and real-time event monitoring.
//
// # Basic Usage
//
// Create a client and control your SoundTouch device:
//
//	config := &client.Config{
//		Host: "192.168.1.100",
//		Port: 8090,
//		Timeout: 10 * time.Second,
//	}
//	client := client.NewClient(config)
//
//	// Get device information
//	info, err := client.GetInfo()
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Device: %s (Type: %s)\n", info.Name, info.Type)
//
//	// Control playback
//	err = client.Play()
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Adjust volume
//	err = client.SetVolume(50)
//	if err != nil {
//		log.Fatal(err)
//	}
//
// # Advanced Features
//
// The client supports all SoundTouch API endpoints:
//
//	// Get current playback status
//	nowPlaying, err := client.GetNowPlaying()
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Now Playing: %s by %s\n", nowPlaying.Track, nowPlaying.Artist)
//
//	// Select audio source
//	err = client.SelectSource("SPOTIFY", "")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Control bass and balance
//	err = client.SetBass(3)  // Range: -9 to +9
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	err = client.SetBalance(-10)  // Range: -50 (left) to +50 (right)
//	if err != nil {
//		log.Fatal(err)
//	}
//
// # Multiroom Zone Management
//
// Create and manage multiroom zones:
//
//	// Get current zone configuration
//	zone, err := client.GetZone()
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Create a new zone with multiple speakers
//	newZone := &models.ZoneRequest{
//		Master: "192.168.1.100",
//		Members: []models.MemberEntry{
//			{IP: "192.168.1.101"},
//			{IP: "192.168.1.102"},
//		},
//	}
//	err = client.SetZone(newZone)
//	if err != nil {
//		log.Fatal(err)
//	}
//
// # Real-time Events
//
// Monitor device state changes using WebSocket connections:
//
//	ctx := context.Background()
//	events, err := client.SubscribeToEvents(ctx)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	for event := range events {
//		switch e := event.(type) {
//		case *models.NowPlayingUpdated:
//			fmt.Printf("Track changed: %s\n", e.Track)
//		case *models.VolumeUpdated:
//			fmt.Printf("Volume: %d\n", e.ActualVolume)
//		case *models.ConnectionStateUpdated:
//			fmt.Printf("Connection: %s\n", e.State)
//		}
//	}
//
// # Error Handling
//
// The client provides detailed error information:
//
//	err := client.SetVolume(150)  // Invalid volume
//	if err != nil {
//		fmt.Printf("Error: %v\n", err)  // Will indicate volume out of range
//	}
//
// # Configuration
//
// The Config struct supports various options:
//
//	config := &client.Config{
//		Host:      "192.168.1.100",
//		Port:      8090,
//		Timeout:   15 * time.Second,
//		UserAgent: "MyApp/1.0",
//	}
//
// # Supported Operations
//
//   - Device Information & Capabilities
//   - Playback Control (Play/Pause/Stop/Next/Previous/Key commands)
//   - Volume Control (Get/Set/Increment/Decrement)
//   - Bass Control (-9 to +9 range)
//   - Balance Control (-50 to +50 range)
//   - Source Selection (Spotify, Bluetooth, AUX, Radio, etc.)
//   - Preset Management (Get configured presets)
//   - Clock/Time Management
//   - Network Information
//   - Multiroom Zone Management
//   - Real-time WebSocket Event Monitoring
package client

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// defaultSoundTouchPort is the standard port for SoundTouch devices
const defaultSoundTouchPort = 8090

// Client represents a SoundTouch API client
type Client struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
	userAgent  string
}

// Config holds configuration for the SoundTouch client
type Config struct {
	Host      string
	Port      int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns a default client configuration
func DefaultConfig() *Config {
	return &Config{
		Host:      "localhost",
		Port:      8090,
		Timeout:   30 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Client/1.0",
	}
}

// NewClient creates a new SoundTouch API client
func NewClient(config *Config) *Client {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	if config.UserAgent == "" {
		config.UserAgent = "Bose-SoundTouch-Go-Client/1.0"
	}

	if config.Port == 0 {
		config.Port = 8090
	}

	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", config.Host, config.Port),
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		timeout:   config.Timeout,
		userAgent: config.UserAgent,
	}
}

// NewClientFromHost creates a new client with just a host address
func NewClientFromHost(host string) *Client {
	config := DefaultConfig()
	config.Host = host

	return NewClient(config)
}

// GetDeviceInfo retrieves device information from the /info endpoint
func (c *Client) GetDeviceInfo() (*models.DeviceInfo, error) {
	var deviceInfo models.DeviceInfo

	err := c.get("/info", &deviceInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to get device info: %w", err)
	}

	return &deviceInfo, nil
}

// GetNowPlaying retrieves current playback information from the /now_playing endpoint
func (c *Client) GetNowPlaying() (*models.NowPlaying, error) {
	var nowPlaying models.NowPlaying

	err := c.get("/now_playing", &nowPlaying)
	if err != nil {
		return nil, fmt.Errorf("failed to get now playing: %w", err)
	}

	return &nowPlaying, nil
}

// GetSources retrieves available audio sources from the /sources endpoint
func (c *Client) GetSources() (*models.Sources, error) {
	var sources models.Sources

	err := c.get("/sources", &sources)
	if err != nil {
		return nil, fmt.Errorf("failed to get sources: %w", err)
	}

	return &sources, nil
}

// GetServiceAvailability retrieves service availability status from the /serviceAvailability endpoint
func (c *Client) GetServiceAvailability() (*models.ServiceAvailability, error) {
	var serviceAvailability models.ServiceAvailability

	err := c.get("/serviceAvailability", &serviceAvailability)
	if err != nil {
		return nil, fmt.Errorf("failed to get service availability: %w", err)
	}

	return &serviceAvailability, nil
}

// GetName retrieves the device name from the /name endpoint
func (c *Client) GetName() (*models.Name, error) {
	var name models.Name

	err := c.get("/name", &name)
	if err != nil {
		return nil, fmt.Errorf("failed to get device name: %w", err)
	}

	return &name, nil
}

// GetCapabilities retrieves device capabilities from the /capabilities endpoint
func (c *Client) GetCapabilities() (*models.Capabilities, error) {
	var capabilities models.Capabilities

	err := c.get("/capabilities", &capabilities)
	if err != nil {
		return nil, fmt.Errorf("failed to get device capabilities: %w", err)
	}

	return &capabilities, nil
}

// GetSupportedURLs retrieves all supported endpoints from the /supportedURLs endpoint
func (c *Client) GetSupportedURLs() (*models.SupportedURLsResponse, error) {
	var supportedURLs models.SupportedURLsResponse

	err := c.get("/supportedURLs", &supportedURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to get supported URLs: %w", err)
	}

	return &supportedURLs, nil
}

// GetPresets retrieves configured presets from the /presets endpoint
func (c *Client) GetPresets() (*models.Presets, error) {
	var presets models.Presets

	err := c.get("/presets", &presets)
	if err != nil {
		return nil, fmt.Errorf("failed to get presets: %w", err)
	}

	return &presets, nil
}

// GetNextAvailablePresetSlot returns the next available preset slot (1-6), or error if all are used
func (c *Client) GetNextAvailablePresetSlot() (int, error) {
	presets, err := c.GetPresets()
	if err != nil {
		return 0, fmt.Errorf("failed to get presets: %w", err)
	}

	emptySlots := presets.GetEmptyPresetSlots()
	if len(emptySlots) == 0 {
		return 0, fmt.Errorf("all preset slots are occupied")
	}

	// Return the first available slot
	return emptySlots[0], nil
}

// IsCurrentContentPresetable checks if the currently playing content can be saved as a preset
func (c *Client) IsCurrentContentPresetable() (bool, error) {
	nowPlaying, err := c.GetNowPlaying()
	if err != nil {
		return false, fmt.Errorf("failed to get now playing: %w", err)
	}

	if nowPlaying.IsEmpty() || nowPlaying.ContentItem == nil {
		return false, nil
	}

	return nowPlaying.ContentItem.IsPresetable, nil
}

// StorePreset saves content as a preset on the SoundTouch device
func (c *Client) StorePreset(id int, contentItem *models.ContentItem) error {
	if id < 1 || id > 6 {
		return fmt.Errorf("preset ID must be between 1 and 6, got %d", id)
	}

	if contentItem == nil {
		return fmt.Errorf("content item cannot be nil")
	}

	now := time.Now().Unix()
	preset := &models.Preset{
		ID:          id,
		CreatedOn:   &now,
		UpdatedOn:   &now,
		ContentItem: contentItem,
	}

	err := c.post("/storePreset", preset)
	if err != nil {
		return fmt.Errorf("failed to store preset %d: %w", id, err)
	}

	return nil
}

// StoreCurrentAsPreset saves currently playing content as preset
func (c *Client) StoreCurrentAsPreset(id int) error {
	if id < 1 || id > 6 {
		return fmt.Errorf("preset ID must be between 1 and 6, got %d", id)
	}

	nowPlaying, err := c.GetNowPlaying()
	if err != nil {
		return fmt.Errorf("failed to get current content: %w", err)
	}

	if nowPlaying.IsEmpty() || nowPlaying.ContentItem == nil {
		return fmt.Errorf("no content currently playing")
	}

	if !nowPlaying.ContentItem.IsPresetable {
		return fmt.Errorf("current content cannot be saved as preset")
	}

	return c.StorePreset(id, nowPlaying.ContentItem)
}

// RemovePreset deletes a preset from the SoundTouch device
func (c *Client) RemovePreset(id int) error {
	if id < 1 || id > 6 {
		return fmt.Errorf("preset ID must be between 1 and 6, got %d", id)
	}

	preset := &models.Preset{ID: id}

	err := c.post("/removePreset", preset)
	if err != nil {
		return fmt.Errorf("failed to remove preset %d: %w", id, err)
	}

	return nil
}

// SendKey sends a key press command to the device (press followed by release)
func (c *Client) SendKey(keyValue string) error {
	if !models.IsValidKey(keyValue) {
		return fmt.Errorf("invalid key value: %s", keyValue)
	}

	// Send press state
	keyPress := models.NewKey(keyValue)

	err := c.post("/key", keyPress)
	if err != nil {
		return fmt.Errorf("failed to send key press: %w", err)
	}

	// Send release state
	keyRelease := models.NewKeyRelease(keyValue)

	err = c.post("/key", keyRelease)
	if err != nil {
		return fmt.Errorf("failed to send key release: %w", err)
	}

	return nil
}

// SendKeyPress sends a key press command (alias for SendKey - sends press+release)
func (c *Client) SendKeyPress(keyValue string) error {
	return c.SendKey(keyValue)
}

// SendKeyPressOnly sends only the key press state (without release)
func (c *Client) SendKeyPressOnly(keyValue string) error {
	if !models.IsValidKey(keyValue) {
		return fmt.Errorf("invalid key value: %s", keyValue)
	}

	key := models.NewKey(keyValue)

	return c.post("/key", key)
}

// SendKeyRelease sends a key release command
func (c *Client) SendKeyRelease(keyValue string) error {
	if !models.IsValidKey(keyValue) {
		return fmt.Errorf("invalid key value: %s", keyValue)
	}

	key := models.NewKeyRelease(keyValue)

	return c.post("/key", key)
}

// SendKeyReleaseOnly sends only the key release state (alias for SendKeyRelease)
func (c *Client) SendKeyReleaseOnly(keyValue string) error {
	return c.SendKeyRelease(keyValue)
}

// Play sends a PLAY key command
func (c *Client) Play() error {
	return c.SendKey(models.KeyPlay)
}

// Pause sends a PAUSE key command
func (c *Client) Pause() error {
	return c.SendKey(models.KeyPause)
}

// Stop sends a STOP key command
func (c *Client) Stop() error {
	return c.SendKey(models.KeyStop)
}

// NextTrack sends a NEXT_TRACK key command
func (c *Client) NextTrack() error {
	return c.SendKey(models.KeyNextTrack)
}

// PrevTrack sends a PREV_TRACK key command
func (c *Client) PrevTrack() error {
	return c.SendKey(models.KeyPrevTrack)
}

// VolumeUp sends a VOLUME_UP key command
func (c *Client) VolumeUp() error {
	return c.SendKey(models.KeyVolumeUp)
}

// VolumeDown sends a VOLUME_DOWN key command
func (c *Client) VolumeDown() error {
	return c.SendKey(models.KeyVolumeDown)
}

// SelectPreset sends a preset key command (1-6)
func (c *Client) SelectPreset(presetNumber int) error {
	var keyValue string

	switch presetNumber {
	case 1:
		keyValue = models.KeyPreset1
	case 2:
		keyValue = models.KeyPreset2
	case 3:
		keyValue = models.KeyPreset3
	case 4:
		keyValue = models.KeyPreset4
	case 5:
		keyValue = models.KeyPreset5
	case 6:
		keyValue = models.KeyPreset6
	default:
		return fmt.Errorf("invalid preset number: %d (must be 1-6)", presetNumber)
	}

	return c.SendKey(keyValue)
}

// GetVolume retrieves the current volume level from the /volume endpoint
func (c *Client) GetVolume() (*models.Volume, error) {
	var volume models.Volume

	err := c.get("/volume", &volume)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume: %w", err)
	}

	return &volume, nil
}

// SetVolume sets the volume level using the /volume endpoint
func (c *Client) SetVolume(level int) error {
	if !models.ValidateVolumeLevel(level) {
		return fmt.Errorf("invalid volume level: %d (must be 0-100)", level)
	}

	volumeReq := models.NewVolumeRequest(level)

	return c.post("/volume", volumeReq)
}

// SetVolumeSafe sets volume with validation and clamping
func (c *Client) SetVolumeSafe(level int) error {
	clampedLevel := models.ClampVolumeLevel(level)
	return c.SetVolume(clampedLevel)
}

// IncreaseVolume increases volume by the specified amount (with safety limits)
func (c *Client) IncreaseVolume(amount int) (*models.Volume, error) {
	currentVolume, err := c.GetVolume()
	if err != nil {
		return nil, fmt.Errorf("failed to get current volume: %w", err)
	}

	newLevel := models.ClampVolumeLevel(currentVolume.GetLevel() + amount)

	err = c.SetVolume(newLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to set volume: %w", err)
	}

	// Return updated volume
	return c.GetVolume()
}

// DecreaseVolume decreases volume by the specified amount (with safety limits)
func (c *Client) DecreaseVolume(amount int) (*models.Volume, error) {
	currentVolume, err := c.GetVolume()
	if err != nil {
		return nil, fmt.Errorf("failed to get current volume: %w", err)
	}

	newLevel := models.ClampVolumeLevel(currentVolume.GetLevel() - amount)

	err = c.SetVolume(newLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to set volume: %w", err)
	}

	// Return updated volume
	return c.GetVolume()
}

// GetBass retrieves the current bass level from the /bass endpoint
func (c *Client) GetBass() (*models.Bass, error) {
	var bass models.Bass

	err := c.get("/bass", &bass)
	if err != nil {
		return nil, fmt.Errorf("failed to get bass: %w", err)
	}

	return &bass, nil
}

// SetBass sets the bass level using the /bass endpoint
func (c *Client) SetBass(level int) error {
	if !models.ValidateBassLevel(level) {
		return fmt.Errorf("invalid bass level: %d (must be between %d and %d)", level, models.BassLevelMin, models.BassLevelMax)
	}

	bassReq, err := models.NewBassRequest(level)
	if err != nil {
		return fmt.Errorf("failed to create bass request: %w", err)
	}

	return c.post("/bass", bassReq)
}

// SetBassSafe sets bass with validation and clamping
func (c *Client) SetBassSafe(level int) error {
	clampedLevel := models.ClampBassLevel(level)
	return c.SetBass(clampedLevel)
}

// IncreaseBass increases bass by the specified amount (with safety limits)
func (c *Client) IncreaseBass(amount int) (*models.Bass, error) {
	currentBass, err := c.GetBass()
	if err != nil {
		return nil, fmt.Errorf("failed to get current bass: %w", err)
	}

	newLevel := models.ClampBassLevel(currentBass.GetLevel() + amount)

	err = c.SetBass(newLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to set bass: %w", err)
	}

	// Return updated bass
	return c.GetBass()
}

// DecreaseBass decreases bass by the specified amount (with safety limits)
func (c *Client) DecreaseBass(amount int) (*models.Bass, error) {
	currentBass, err := c.GetBass()
	if err != nil {
		return nil, fmt.Errorf("failed to get current bass: %w", err)
	}

	newLevel := models.ClampBassLevel(currentBass.GetLevel() - amount)

	err = c.SetBass(newLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to set bass: %w", err)
	}

	// Return updated bass
	return c.GetBass()
}

// GetBalance retrieves the current balance level from the /balance endpoint
func (c *Client) GetBalance() (*models.Balance, error) {
	var balance models.Balance

	err := c.get("/balance", &balance)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	return &balance, nil
}

// SetBalance sets the balance level using the /balance endpoint
func (c *Client) SetBalance(level int) error {
	if !models.ValidateBalanceLevel(level) {
		return fmt.Errorf("invalid balance level: %d (must be between %d and %d)", level, models.BalanceLevelMin, models.BalanceLevelMax)
	}

	balanceReq, err := models.NewBalanceRequest(level)
	if err != nil {
		return fmt.Errorf("failed to create balance request: %w", err)
	}

	return c.post("/balance", balanceReq)
}

// SetBalanceSafe sets balance with validation and clamping
func (c *Client) SetBalanceSafe(level int) error {
	clampedLevel := models.ClampBalanceLevel(level)
	return c.SetBalance(clampedLevel)
}

// IncreaseBalance increases balance by the specified amount (with safety limits)
func (c *Client) IncreaseBalance(amount int) (*models.Balance, error) {
	currentBalance, err := c.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get current balance: %w", err)
	}

	newLevel := models.ClampBalanceLevel(currentBalance.GetLevel() + amount)

	err = c.SetBalance(newLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to set balance: %w", err)
	}

	// Return updated balance
	return c.GetBalance()
}

// DecreaseBalance decreases balance by the specified amount (with safety limits)
func (c *Client) DecreaseBalance(amount int) (*models.Balance, error) {
	currentBalance, err := c.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get current balance: %w", err)
	}

	newLevel := models.ClampBalanceLevel(currentBalance.GetLevel() - amount)

	err = c.SetBalance(newLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to set balance: %w", err)
	}

	// Return updated balance
	return c.GetBalance()
}

// SelectSource selects an audio source using the /select endpoint
func (c *Client) SelectSource(source, sourceAccount string) error {
	// Validate source parameter
	if source == "" {
		return fmt.Errorf("source cannot be empty")
	}

	// Create ContentItem for source selection
	contentItem := &models.ContentItem{
		Source:        source,
		SourceAccount: sourceAccount,
		ItemName:      source, // Use source as default item name
	}

	// For certain sources, we might want to customize the item name
	switch source {
	case "SPOTIFY":
		contentItem.ItemName = "Spotify"
	case "BLUETOOTH":
		contentItem.ItemName = "Bluetooth"
	case "AUX":
		contentItem.ItemName = "AUX Input"
	case "TUNEIN":
		contentItem.ItemName = "TuneIn"
	case "PANDORA":
		contentItem.ItemName = "Pandora"
	case "AMAZON":
		contentItem.ItemName = "Amazon Music"
	case "IHEARTRADIO":
		contentItem.ItemName = "iHeartRadio"
	case "STORED_MUSIC":
		contentItem.ItemName = "Stored Music"
	}

	return c.post("/select", contentItem)
}

// SelectSourceFromItem selects an audio source using a SourceItem
func (c *Client) SelectSourceFromItem(sourceItem *models.SourceItem) error {
	if sourceItem == nil {
		return fmt.Errorf("sourceItem cannot be nil")
	}

	return c.SelectSource(sourceItem.Source, sourceItem.SourceAccount)
}

// SelectSpotify is a convenience method to select Spotify source
func (c *Client) SelectSpotify(sourceAccount string) error {
	return c.SelectSource("SPOTIFY", sourceAccount)
}

// SelectBluetooth is a convenience method to select Bluetooth source
func (c *Client) SelectBluetooth() error {
	return c.SelectSource("BLUETOOTH", "")
}

// SelectAux is a convenience method to select AUX input
func (c *Client) SelectAux() error {
	return c.SelectSource("AUX", "")
}

// SelectTuneIn is a convenience method to select TuneIn source
func (c *Client) SelectTuneIn(sourceAccount string) error {
	return c.SelectSource("TUNEIN", sourceAccount)
}

// SelectPandora is a convenience method to select Pandora source
func (c *Client) SelectPandora(sourceAccount string) error {
	return c.SelectSource("PANDORA", sourceAccount)
}

// SelectContentItem selects content using a ContentItem directly.
// This method allows full control over all ContentItem properties including
// complex location parameters for LOCAL_INTERNET_RADIO streamUrl format.
//
// Example usage for LOCAL_INTERNET_RADIO with streamUrl:
//
//	contentItem := &models.ContentItem{
//	  Source:       "LOCAL_INTERNET_RADIO",
//	  Type:         "stationurl",
//	  Location:     "http://contentapi.gmuth.de/station.php?name=MyStation&streamUrl=https://stream.example.com/radio",
//	  IsPresetable: true,
//	  ItemName:     "My Radio Station",
//	  ContainerArt: "https://example.com/art.png",
//	}
//	err := client.SelectContentItem(contentItem)
func (c *Client) SelectContentItem(contentItem *models.ContentItem) error {
	if contentItem == nil {
		return fmt.Errorf("contentItem cannot be nil")
	}

	if contentItem.Source == "" {
		return fmt.Errorf("contentItem source cannot be empty")
	}

	return c.post("/select", contentItem)
}

// SelectLocalInternetRadio is a convenience method to select LOCAL_INTERNET_RADIO content.
// For simple direct stream URLs, use streamURL parameter.
// For complex streamUrl format (with proxy), use the location parameter with full URL.
//
// Example 1 - Direct stream:
//
//	err := client.SelectLocalInternetRadio("https://stream.example.com/radio", "", "My Radio", "")
//
// Example 2 - StreamUrl format with proxy:
//
//	location := "http://contentapi.gmuth.de/station.php?name=MyStation&streamUrl=https://stream.example.com/radio"
//	err := client.SelectLocalInternetRadio(location, "", "My Radio", "https://example.com/art.png")
func (c *Client) SelectLocalInternetRadio(location, sourceAccount, itemName, containerArt string) error {
	if location == "" {
		return fmt.Errorf("location cannot be empty")
	}

	contentItem := &models.ContentItem{
		Source:        "LOCAL_INTERNET_RADIO",
		Type:          "stationurl",
		Location:      location,
		SourceAccount: sourceAccount,
		IsPresetable:  true,
		ItemName:      itemName,
		ContainerArt:  containerArt,
	}

	if itemName == "" {
		contentItem.ItemName = "Internet Radio"
	}

	return c.SelectContentItem(contentItem)
}

// SelectLocalMusic is a convenience method to select LOCAL_MUSIC content.
// This is used for SoundTouch App Media Server content on local computers.
//
// Example:
//
//	err := client.SelectLocalMusic("album:983", "3f205110-4a57-4e91-810a-123456789012", "Welcome to the New", "http://192.168.1.14:8085/v1/albums/983/image")
func (c *Client) SelectLocalMusic(location, sourceAccount, itemName, containerArt string) error {
	if location == "" {
		return fmt.Errorf("location cannot be empty")
	}

	if sourceAccount == "" {
		return fmt.Errorf("sourceAccount cannot be empty for LOCAL_MUSIC")
	}

	contentItem := &models.ContentItem{
		Source:        "LOCAL_MUSIC",
		Type:          "album", // Default type, could be "track", "artist", etc.
		Location:      location,
		SourceAccount: sourceAccount,
		IsPresetable:  true,
		ItemName:      itemName,
		ContainerArt:  containerArt,
	}

	if itemName == "" {
		contentItem.ItemName = "Local Music"
	}

	return c.SelectContentItem(contentItem)
}

// SelectStoredMusic is a convenience method to select STORED_MUSIC content.
// This is used for UPnP/DLNA media servers and NAS libraries.
//
// Example:
//
//	err := client.SelectStoredMusic("6_a2874b5d_4f83d999", "d09708a1-5953-44bc-a413-123456789012/0", "Christmas Album", "")
func (c *Client) SelectStoredMusic(location, sourceAccount, itemName, containerArt string) error {
	if location == "" {
		return fmt.Errorf("location cannot be empty")
	}

	if sourceAccount == "" {
		return fmt.Errorf("sourceAccount cannot be empty for STORED_MUSIC")
	}

	contentItem := &models.ContentItem{
		Source:        "STORED_MUSIC",
		Location:      location,
		SourceAccount: sourceAccount,
		IsPresetable:  true,
		ItemName:      itemName,
		ContainerArt:  containerArt,
	}

	if itemName == "" {
		contentItem.ItemName = "Stored Music"
	}

	return c.SelectContentItem(contentItem)
}

// GetClockTime retrieves the device's current time from the /clockTime endpoint
func (c *Client) GetClockTime() (*models.ClockTime, error) {
	var clockTime models.ClockTime

	err := c.get("/clockTime", &clockTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get clock time: %w", err)
	}

	return &clockTime, nil
}

// SetClockTime sets the device's time via the /clockTime endpoint
func (c *Client) SetClockTime(request *models.ClockTimeRequest) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid clock time request: %w", err)
	}

	err := c.post("/clockTime", request)
	if err != nil {
		return fmt.Errorf("failed to set clock time: %w", err)
	}

	return nil
}

// SetClockTimeNow sets the device's time to the current system time
func (c *Client) SetClockTimeNow() error {
	request := models.NewClockTimeRequest(time.Now())
	return c.SetClockTime(request)
}

// GetClockDisplay retrieves clock display settings from the /clockDisplay endpoint
func (c *Client) GetClockDisplay() (*models.ClockDisplay, error) {
	var clockDisplay models.ClockDisplay

	err := c.get("/clockDisplay", &clockDisplay)
	if err != nil {
		return nil, fmt.Errorf("failed to get clock display settings: %w", err)
	}

	return &clockDisplay, nil
}

// SetClockDisplay configures clock display settings via the /clockDisplay endpoint
func (c *Client) SetClockDisplay(request *models.ClockDisplayRequest) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid clock display request: %w", err)
	}

	if !request.HasChanges() {
		return fmt.Errorf("no changes specified in clock display request")
	}

	err := c.post("/clockDisplay", request)
	if err != nil {
		return fmt.Errorf("failed to set clock display: %w", err)
	}

	return nil
}

// EnableClockDisplay enables the clock display with default settings
func (c *Client) EnableClockDisplay() error {
	request := models.NewClockDisplayRequest().SetEnabled(true)
	return c.SetClockDisplay(request)
}

// DisableClockDisplay disables the clock display
func (c *Client) DisableClockDisplay() error {
	request := models.NewClockDisplayRequest().SetEnabled(false)
	return c.SetClockDisplay(request)
}

// SetClockDisplayBrightness sets the clock display brightness (0-100)
func (c *Client) SetClockDisplayBrightness(brightness int) error {
	request := models.NewClockDisplayRequest().SetBrightness(brightness)
	return c.SetClockDisplay(request)
}

// SetClockDisplayFormat sets the clock display format (12/24 hour)
func (c *Client) SetClockDisplayFormat(format models.ClockFormat) error {
	request := models.NewClockDisplayRequest().SetFormat(format)
	return c.SetClockDisplay(request)
}

// GetNetworkInfo retrieves network information from the /networkInfo endpoint
func (c *Client) GetNetworkInfo() (*models.NetworkInformation, error) {
	var networkInfo models.NetworkInformation

	err := c.get("/networkInfo", &networkInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to get network info: %w", err)
	}

	return &networkInfo, nil
}

// Ping checks if the device is reachable by calling /info
func (c *Client) Ping() error {
	_, err := c.GetDeviceInfo()
	return err
}

// BaseURL returns the base URL for this client
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Host returns the host for this client
func (c *Client) Host() string {
	return c.baseURL
}

// get performs a GET request and unmarshals the XML response
func (c *Client) get(endpoint string, result interface{}) error {
	url := c.baseURL + endpoint

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log the error but don't override the main error
			_ = closeErr // Explicitly ignore the error
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the actual response first
	if err := xml.Unmarshal(body, result); err != nil {
		// Check if it might be an API error response instead
		var apiError models.APIError
		if xmlErr := xml.Unmarshal(body, &apiError); xmlErr == nil && apiError.Message != "" {
			return &apiError
		}

		return fmt.Errorf("failed to unmarshal XML response: %w", err)
	}

	return nil
}

// post performs a POST request with XML body
func (c *Client) post(endpoint string, payload interface{}) error {
	url := c.baseURL + endpoint

	var body io.Reader

	if payload != nil {
		xmlData, err := xml.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal XML request: %w", err)
		}

		body = bytes.NewReader(xmlData)
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log the error but don't override the main error
			_ = closeErr // Explicitly ignore the error
		}
	}()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	return nil
}

// postWithResponse performs a POST request with XML body and parses the response
func (c *Client) postWithResponse(endpoint string, payload, result interface{}) error {
	url := c.baseURL + endpoint

	var body io.Reader

	if payload != nil {
		xmlData, err := xml.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal XML request: %w", err)
		}

		body = bytes.NewReader(xmlData)
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log the error but don't override the main error
			_ = closeErr // Explicitly ignore the error
		}
	}()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	if result != nil {
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		// Parse the actual response first
		if err := xml.Unmarshal(responseBody, result); err != nil {
			// Check if it might be an API error response instead
			var apiError models.APIError
			if xmlErr := xml.Unmarshal(responseBody, &apiError); xmlErr == nil && apiError.Message != "" {
				return &apiError
			}

			return fmt.Errorf("failed to unmarshal XML response: %w", err)
		}
	}

	return nil
}

// GetZone gets the current multiroom zone configuration
func (c *Client) GetZone() (*models.ZoneInfo, error) {
	var zone models.ZoneInfo

	err := c.get("/getZone", &zone)

	return &zone, err
}

// SetZone configures multiroom zone settings
func (c *Client) SetZone(zoneRequest *models.ZoneRequest) error {
	if err := zoneRequest.Validate(); err != nil {
		return fmt.Errorf("invalid zone request: %w", err)
	}

	return c.post("/setZone", zoneRequest)
}

// CreateZone creates a new multiroom zone with the specified master and members
func (c *Client) CreateZone(masterDeviceID string, memberDeviceIDs []string) error {
	zoneRequest := models.NewZoneRequest(masterDeviceID)

	for _, deviceID := range memberDeviceIDs {
		zoneRequest.AddMemberByDeviceID(deviceID)
	}

	return c.SetZone(zoneRequest)
}

// CreateZoneWithIPs creates a new multiroom zone with device IDs and IP addresses
func (c *Client) CreateZoneWithIPs(masterDeviceID string, members map[string]string) error {
	zoneRequest := models.NewZoneRequest(masterDeviceID)

	for deviceID, ipAddress := range members {
		zoneRequest.AddMember(deviceID, ipAddress)
	}

	return c.SetZone(zoneRequest)
}

// AddToZone adds a device to an existing zone
func (c *Client) AddToZone(deviceID, ipAddress string) error {
	// Get current zone configuration
	currentZone, err := c.GetZone()
	if err != nil {
		return fmt.Errorf("failed to get current zone: %w", err)
	}

	// Convert to zone request and add member
	zoneRequest := currentZone.ToZoneRequest()
	zoneRequest.AddMember(deviceID, ipAddress)

	return c.SetZone(zoneRequest)
}

// RemoveFromZone removes a device from the current zone
func (c *Client) RemoveFromZone(deviceID string) error {
	// Get current zone configuration
	currentZone, err := c.GetZone()
	if err != nil {
		return fmt.Errorf("failed to get current zone: %w", err)
	}

	// Convert to zone request and remove member
	zoneRequest := currentZone.ToZoneRequest()
	zoneRequest.RemoveMember(deviceID)

	return c.SetZone(zoneRequest)
}

// DissolveZone dissolves the current zone, making all devices standalone
func (c *Client) DissolveZone() error {
	// Get current zone configuration
	currentZone, err := c.GetZone()
	if err != nil {
		return fmt.Errorf("failed to get current zone: %w", err)
	}

	// Create standalone configuration (master only, no members)
	zoneRequest := models.NewZoneRequest(currentZone.Master)

	return c.SetZone(zoneRequest)
}

// IsInZone checks if this device is part of a multiroom zone
func (c *Client) IsInZone() (bool, error) {
	zone, err := c.GetZone()
	if err != nil {
		return false, err
	}

	return !zone.IsStandalone(), nil
}

// GetZoneStatus returns the zone status for this device
func (c *Client) GetZoneStatus() (models.ZoneStatus, error) {
	zone, err := c.GetZone()
	if err != nil {
		return models.ZoneStatusStandalone, err
	}

	// Get device info to determine our device ID
	deviceInfo, err := c.GetDeviceInfo()
	if err != nil {
		return models.ZoneStatusStandalone, fmt.Errorf("failed to get device info: %w", err)
	}

	return zone.GetZoneStatus(deviceInfo.DeviceID), nil
}

// GetZoneMembers returns all devices in the current zone
func (c *Client) GetZoneMembers() ([]string, error) {
	zone, err := c.GetZone()
	if err != nil {
		return nil, err
	}

	return zone.GetAllDeviceIDs(), nil
}

// SetName sets the device name
func (c *Client) SetName(name string) error {
	nameRequest := models.Name{
		XMLName: xml.Name{Local: "name"},
		Value:   name,
	}

	return c.post("/name", nameRequest)
}

// GetBassCapabilities retrieves the bass capabilities for the device
func (c *Client) GetBassCapabilities() (*models.BassCapabilities, error) {
	var bassCapabilities models.BassCapabilities

	err := c.get("/bassCapabilities", &bassCapabilities)

	return &bassCapabilities, err
}

// GetTrackInfo retrieves track information (duplicate of GetNowPlaying per official API)
// WARNING: This endpoint times out on real devices despite being documented in the official API.
// Use GetNowPlaying() instead for reliable track information.
func (c *Client) GetTrackInfo() (*models.NowPlaying, error) {
	var nowPlaying models.NowPlaying

	err := c.get("/trackInfo", &nowPlaying)

	return &nowPlaying, err
}

// GetAudioDSPControls retrieves the current DSP audio controls
// Only available if audiodspcontrols is listed in the reply to GET /capabilities
func (c *Client) GetAudioDSPControls() (*models.AudioDSPControls, error) {
	// Check if DSP controls are supported by checking capabilities
	capabilities, err := c.GetCapabilities()
	if err != nil {
		return nil, fmt.Errorf("failed to check device capabilities: %w", err)
	}

	// Check if audiodspcontrols capability exists
	if !c.hasCapability(capabilities, "audiodspcontrols") {
		return nil, fmt.Errorf("audiodspcontrols not supported by this device")
	}

	var dspControls models.AudioDSPControls

	err = c.get("/audiodspcontrols", &dspControls)

	return &dspControls, err
}

// SetAudioDSPControls sets the DSP audio controls
// Only available if audiodspcontrols is listed in the reply to GET /capabilities
func (c *Client) SetAudioDSPControls(audioMode string, videoSyncDelay int) error {
	request := &models.AudioDSPControlsRequest{
		AudioMode:           audioMode,
		VideoSyncAudioDelay: videoSyncDelay,
	}

	// Validate against current capabilities
	capabilities, err := c.GetAudioDSPControls()
	if err != nil {
		return fmt.Errorf("DSP controls not supported or available: %w", err)
	}

	if validationErr := request.Validate(capabilities); validationErr != nil {
		return fmt.Errorf("invalid DSP controls request: %w", validationErr)
	}

	return c.post("/audiodspcontrols", request)
}

// SetAudioMode sets only the audio mode (leaving video sync delay unchanged)
func (c *Client) SetAudioMode(mode string) error {
	request := &models.AudioDSPControlsRequest{
		AudioMode: mode,
	}

	// Validate against current capabilities if possible
	capabilities, err := c.GetAudioDSPControls()
	if err == nil {
		if validationErr := request.Validate(capabilities); validationErr != nil {
			return fmt.Errorf("invalid audio mode: %w", validationErr)
		}
	}

	return c.post("/audiodspcontrols", request)
}

// SetVideoSyncAudioDelay sets only the video sync audio delay (leaving audio mode unchanged)
func (c *Client) SetVideoSyncAudioDelay(delay int) error {
	request := &models.AudioDSPControlsRequest{
		VideoSyncAudioDelay: delay,
	}

	if err := request.Validate(nil); err != nil {
		return fmt.Errorf("invalid video sync delay: %w", err)
	}

	return c.post("/audiodspcontrols", request)
}

// GetAudioProductToneControls retrieves the current advanced tone controls (bass/treble)
// Only available if audioproducttonecontrols is listed in the reply to GET /capabilities
func (c *Client) GetAudioProductToneControls() (*models.AudioProductToneControls, error) {
	// Check if tone controls are supported by checking capabilities
	capabilities, err := c.GetCapabilities()
	if err != nil {
		return nil, fmt.Errorf("failed to check device capabilities: %w", err)
	}

	// Check if audioproducttonecontrols capability exists
	if !c.hasCapability(capabilities, "audioproducttonecontrols") {
		return nil, fmt.Errorf("audioproducttonecontrols not supported by this device")
	}

	var toneControls models.AudioProductToneControls

	err = c.get("/audioproducttonecontrols", &toneControls)

	return &toneControls, err
}

// SetAudioProductToneControls sets the advanced tone controls (bass and/or treble)
func (c *Client) SetAudioProductToneControls(bass, treble *int) error {
	request := &models.AudioProductToneControlsRequest{}

	if bass != nil {
		request.Bass = models.NewBassControlValue(*bass)
	}

	if treble != nil {
		request.Treble = models.NewTrebleControlValue(*treble)
	}

	// Validate against current capabilities if possible
	capabilities, err := c.GetAudioProductToneControls()
	if err == nil {
		if validationErr := request.Validate(capabilities); validationErr != nil {
			return fmt.Errorf("invalid tone controls request: %w", validationErr)
		}
	}

	return c.post("/audioproducttonecontrols", request)
}

// SetAdvancedBass sets only the advanced bass control
func (c *Client) SetAdvancedBass(level int) error {
	return c.SetAudioProductToneControls(&level, nil)
}

// SetAdvancedTreble sets only the advanced treble control
func (c *Client) SetAdvancedTreble(level int) error {
	return c.SetAudioProductToneControls(nil, &level)
}

// GetAudioProductLevelControls retrieves the current speaker level controls
// Only available if audioproductlevelcontrols is listed in the reply to GET /capabilities
func (c *Client) GetAudioProductLevelControls() (*models.AudioProductLevelControls, error) {
	// Check if level controls are supported by checking capabilities
	capabilities, err := c.GetCapabilities()
	if err != nil {
		return nil, fmt.Errorf("failed to check device capabilities: %w", err)
	}

	// Check if audioproductlevelcontrols capability exists
	if !c.hasCapability(capabilities, "audioproductlevelcontrols") {
		return nil, fmt.Errorf("audioproductlevelcontrols not supported by this device")
	}

	var levelControls models.AudioProductLevelControls

	err = c.get("/audioproductlevelcontrols", &levelControls)

	return &levelControls, err
}

// SetAudioProductLevelControls sets the speaker level controls
func (c *Client) SetAudioProductLevelControls(frontCenter, rearSurround *int) error {
	request := &models.AudioProductLevelControlsRequest{}

	if frontCenter != nil {
		request.FrontCenterSpeakerLevel = models.NewFrontCenterLevelValue(*frontCenter)
	}

	if rearSurround != nil {
		request.RearSurroundSpeakersLevel = models.NewRearSurroundLevelValue(*rearSurround)
	}

	// Validate against current capabilities if possible
	capabilities, err := c.GetAudioProductLevelControls()
	if err == nil {
		if validationErr := request.Validate(capabilities); validationErr != nil {
			return fmt.Errorf("invalid level controls request: %w", validationErr)
		}
	}

	return c.post("/audioproductlevelcontrols", request)
}

// SetFrontCenterSpeakerLevel sets only the front-center speaker level
func (c *Client) SetFrontCenterSpeakerLevel(level int) error {
	return c.SetAudioProductLevelControls(&level, nil)
}

// SetRearSurroundSpeakersLevel sets only the rear-surround speakers level
func (c *Client) SetRearSurroundSpeakersLevel(level int) error {
	return c.SetAudioProductLevelControls(nil, &level)
}

// AddZoneSlave adds a single device to an existing zone using the official /addZoneSlave endpoint
func (c *Client) AddZoneSlave(masterDeviceID, slaveDeviceID, slaveIP string) error {
	request := models.NewZoneSlaveRequest(masterDeviceID)
	request.AddSlave(slaveDeviceID, slaveIP)

	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid zone slave request: %w", err)
	}

	return c.post("/addZoneSlave", request)
}

// AddZoneSlaveByDeviceID adds a single device to an existing zone by device ID only
func (c *Client) AddZoneSlaveByDeviceID(masterDeviceID, slaveDeviceID string) error {
	return c.AddZoneSlave(masterDeviceID, slaveDeviceID, "")
}

// RemoveZoneSlave removes a single device from an existing zone using the official /removeZoneSlave endpoint
func (c *Client) RemoveZoneSlave(masterDeviceID, slaveDeviceID, slaveIP string) error {
	request := models.NewZoneSlaveRequest(masterDeviceID)
	request.AddSlave(slaveDeviceID, slaveIP)

	if err := request.Validate(); err != nil {
		return fmt.Errorf("invalid zone slave request: %w", err)
	}

	return c.post("/removeZoneSlave", request)
}

// RemoveZoneSlaveByDeviceID removes a single device from an existing zone by device ID only
func (c *Client) RemoveZoneSlaveByDeviceID(masterDeviceID, slaveDeviceID string) error {
	return c.RemoveZoneSlave(masterDeviceID, slaveDeviceID, "")
}

// RequestToken generates a new bearer token from the device
func (c *Client) RequestToken() (*models.BearerToken, error) {
	var token models.BearerToken

	err := c.get("/requestToken", &token)
	if err != nil {
		return nil, fmt.Errorf("failed to request token: %w", err)
	}

	return &token, nil
}

// Navigate browses content within a source (e.g., browse music libraries, stations)
func (c *Client) Navigate(source, sourceAccount string, startItem, numItems int) (*models.NavigateResponse, error) {
	if source == "" {
		return nil, fmt.Errorf("source cannot be empty")
	}

	if startItem < 1 {
		return nil, fmt.Errorf("startItem must be >= 1, got %d", startItem)
	}

	if numItems < 1 {
		return nil, fmt.Errorf("numItems must be >= 1, got %d", numItems)
	}

	request := models.NewNavigateRequest(source, sourceAccount, startItem, numItems)

	var response models.NavigateResponse

	err := c.postWithResponse("/navigate", request, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to navigate %s: %w", source, err)
	}

	return &response, nil
}

// NavigateWithMenu browses content with menu and sort parameters (e.g., Pandora stations)
func (c *Client) NavigateWithMenu(source, sourceAccount, menu, sort string, startItem, numItems int) (*models.NavigateResponse, error) {
	if source == "" {
		return nil, fmt.Errorf("source cannot be empty")
	}

	if startItem < 1 {
		return nil, fmt.Errorf("startItem must be >= 1, got %d", startItem)
	}

	if numItems < 1 {
		return nil, fmt.Errorf("numItems must be >= 1, got %d", numItems)
	}

	request := models.NewNavigateRequestWithMenu(source, sourceAccount, menu, sort, startItem, numItems)

	var response models.NavigateResponse

	err := c.postWithResponse("/navigate", request, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to navigate %s with menu %s: %w", source, menu, err)
	}

	return &response, nil
}

// NavigateContainer browses a specific container/directory within a source
func (c *Client) NavigateContainer(source, sourceAccount string, startItem, numItems int, containerItem *models.ContentItem) (*models.NavigateResponse, error) {
	if source == "" {
		return nil, fmt.Errorf("source cannot be empty")
	}

	if containerItem == nil {
		return nil, fmt.Errorf("container item cannot be nil")
	}

	if startItem < 1 {
		return nil, fmt.Errorf("startItem must be >= 1, got %d", startItem)
	}

	if numItems < 1 {
		return nil, fmt.Errorf("numItems must be >= 1, got %d", numItems)
	}

	request := models.NewNavigateRequestWithItem(source, sourceAccount, startItem, numItems, containerItem)

	var response models.NavigateResponse

	err := c.postWithResponse("/navigate", request, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to navigate container in %s: %w", source, err)
	}

	return &response, nil
}

// AddStation adds a station to a music service collection and immediately starts playing it
func (c *Client) AddStation(source, sourceAccount, token, name string) error {
	if source == "" {
		return fmt.Errorf("source cannot be empty")
	}

	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	if name == "" {
		return fmt.Errorf("station name cannot be empty")
	}

	request := models.NewAddStationRequest(source, sourceAccount, token, name)

	var response models.StationResponse

	err := c.postWithResponse("/addStation", request, &response)
	if err != nil {
		return fmt.Errorf("failed to add station '%s' to %s: %w", name, source, err)
	}

	return nil
}

// RemoveStation removes a station from a music service collection
func (c *Client) RemoveStation(contentItem *models.ContentItem) error {
	if contentItem == nil {
		return fmt.Errorf("content item cannot be nil")
	}

	if contentItem.Source == "" {
		return fmt.Errorf("content item source cannot be empty")
	}

	if contentItem.Location == "" {
		return fmt.Errorf("content item location cannot be empty")
	}

	var response models.StationResponse

	err := c.postWithResponse("/removeStation", contentItem, &response)
	if err != nil {
		return fmt.Errorf("failed to remove station from %s: %w", contentItem.Source, err)
	}

	return nil
}

// GetPandoraStations gets all Pandora radio stations for an account
func (c *Client) GetPandoraStations(sourceAccount string) (*models.NavigateResponse, error) {
	if sourceAccount == "" {
		return nil, fmt.Errorf("pandora source account cannot be empty")
	}

	return c.NavigateWithMenu("PANDORA", sourceAccount, "radioStations", "dateCreated", 1, 100)
}

// GetTuneInStations browses TuneIn stations/content
func (c *Client) GetTuneInStations(sourceAccount string) (*models.NavigateResponse, error) {
	return c.Navigate("TUNEIN", sourceAccount, 1, 100)
}

// GetStoredMusicLibrary browses stored music library
func (c *Client) GetStoredMusicLibrary(sourceAccount string) (*models.NavigateResponse, error) {
	if sourceAccount == "" {
		return nil, fmt.Errorf("stored music source account cannot be empty")
	}

	return c.Navigate("STORED_MUSIC", sourceAccount, 1, 1000)
}

// SearchStation searches for stations/content within a music service
func (c *Client) SearchStation(source, sourceAccount, searchTerm string) (*models.SearchStationResponse, error) {
	if source == "" {
		return nil, fmt.Errorf("source cannot be empty")
	}

	if searchTerm == "" {
		return nil, fmt.Errorf("search term cannot be empty")
	}

	request := models.NewSearchStationRequest(source, sourceAccount, searchTerm)

	var response models.SearchStationResponse

	err := c.postWithResponse("/searchStation", request, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to search stations in %s: %w", source, err)
	}

	return &response, nil
}

// SearchPandoraStations searches for Pandora stations by artist/song name
func (c *Client) SearchPandoraStations(sourceAccount, searchTerm string) (*models.SearchStationResponse, error) {
	if sourceAccount == "" {
		return nil, fmt.Errorf("pandora source account cannot be empty")
	}

	return c.SearchStation("PANDORA", sourceAccount, searchTerm)
}

// SearchTuneInStations searches for TuneIn stations/content
func (c *Client) SearchTuneInStations(searchTerm string) (*models.SearchStationResponse, error) {
	return c.SearchStation("TUNEIN", "", searchTerm)
}

// SearchSpotifyContent searches for Spotify content (playlists, tracks, etc.)
func (c *Client) SearchSpotifyContent(sourceAccount, searchTerm string) (*models.SearchStationResponse, error) {
	if sourceAccount == "" {
		return nil, fmt.Errorf("spotify source account cannot be empty")
	}

	return c.SearchStation("SPOTIFY", sourceAccount, searchTerm)
}

// hasCapability checks if a capability is present in the device capabilities
func (c *Client) hasCapability(capabilities *models.Capabilities, capability string) bool {
	// Convert capabilities to string and check if it contains the capability
	// This is a simplified check - in practice, you'd parse the actual capabilities XML structure
	capStr := fmt.Sprintf("%+v", capabilities)
	return strings.Contains(capStr, capability)
}

// PlayTTS plays a Text-To-Speech message using Google TTS on the speaker
func (c *Client) PlayTTS(text, appKey, language string, volume ...int) error {
	playInfo := models.NewTTSPlayInfo(text, appKey, language, volume...)

	if err := playInfo.Validate(); err != nil {
		return fmt.Errorf("invalid TTS request: %w", err)
	}

	return c.postPlayInfo(playInfo)
}

// PlayURL plays audio content from a URL on the speaker
func (c *Client) PlayURL(url, appKey, service, message, reason string, volume ...int) error {
	playInfo := models.NewURLPlayInfo(url, appKey, service, message, reason, volume...)

	if err := playInfo.Validate(); err != nil {
		return fmt.Errorf("invalid URL play request: %w", err)
	}

	return c.postPlayInfo(playInfo)
}

// PlayCustom plays custom content using a PlayInfo configuration
func (c *Client) PlayCustom(playInfo *models.PlayInfo) error {
	if err := playInfo.Validate(); err != nil {
		return fmt.Errorf("invalid play request: %w", err)
	}

	return c.postPlayInfo(playInfo)
}

// PlayNotificationBeep plays a notification beep on the device
func (c *Client) PlayNotificationBeep() error {
	return c.PlayNotification("")
}

// PlayNotification plays a notification. If a non-empty local path is provided,
// it will be sent as XML body to play that specific device-local PCM file.
// When path is empty, the device's default beep is triggered.
func (c *Client) PlayNotification(path string) error {
	// Empty path -> trigger default beep via GET
	if strings.TrimSpace(path) == "" {
		var status models.StationResponse
		return c.get("/playNotification", &status)
	}

	// Non-empty path -> POST minimal XML payload as required by the device
	payload := struct {
		XMLName    xml.Name `xml:"audioSource"`
		PathToFile string   `xml:"pathToFile,attr"`
	}{
		XMLName:    xml.Name{Local: "audioSource"},
		PathToFile: path,
	}

	return c.post("/playNotification", payload)
}

// Introspect retrieves introspect data for a specified music service
func (c *Client) Introspect(source, sourceAccount string) (*models.IntrospectResponse, error) {
	if source == "" {
		return nil, fmt.Errorf("source cannot be empty")
	}

	request := models.NewIntrospectRequest(source, sourceAccount)

	var response models.IntrospectResponse

	err := c.postWithResponse("/introspect", request, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get introspect data for %s: %w", source, err)
	}

	return &response, nil
}

// IntrospectSpotify is a convenience method to get introspect data for Spotify
func (c *Client) IntrospectSpotify(sourceAccount string) (*models.IntrospectResponse, error) {
	return c.Introspect("SPOTIFY", sourceAccount)
}

// GetRecents retrieves recently played content from the device
func (c *Client) GetRecents() (*models.RecentsResponse, error) {
	var response models.RecentsResponse

	err := c.get("/recents", &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent items: %w", err)
	}

	return &response, nil
}

// postPlayInfo sends a PlayInfo request to the /speaker endpoint
func (c *Client) postPlayInfo(playInfo *models.PlayInfo) error {
	return c.post("/speaker", playInfo)
}

// SetMusicServiceAccount adds or updates a music service account
func (c *Client) SetMusicServiceAccount(credentials *models.MusicServiceCredentials) error {
	if credentials == nil {
		return fmt.Errorf("credentials cannot be nil")
	}

	if err := credentials.Validate(); err != nil {
		return fmt.Errorf("invalid credentials: %w", err)
	}

	var response models.MusicServiceAccountResponse

	err := c.postWithResponse("/setMusicServiceAccount", credentials, &response)
	if err != nil {
		return fmt.Errorf("failed to set music service account for %s: %w", credentials.Source, err)
	}

	if !response.IsSuccess() {
		return fmt.Errorf("music service account operation failed: unexpected response %s", response.Status)
	}

	return nil
}

// RemoveMusicServiceAccount removes an existing music service account
func (c *Client) RemoveMusicServiceAccount(credentials *models.MusicServiceCredentials) error {
	if credentials == nil {
		return fmt.Errorf("credentials cannot be nil")
	}

	if credentials.Source == "" {
		return fmt.Errorf("source cannot be empty")
	}

	if credentials.User == "" {
		return fmt.Errorf("user cannot be empty")
	}

	// For removal, ensure password is empty
	removalCredentials := &models.MusicServiceCredentials{
		Source:      credentials.Source,
		DisplayName: credentials.DisplayName,
		User:        credentials.User,
		Pass:        "", // Empty password indicates removal
	}

	var response models.MusicServiceAccountResponse

	err := c.postWithResponse("/removeMusicServiceAccount", removalCredentials, &response)
	if err != nil {
		return fmt.Errorf("failed to remove music service account for %s: %w", credentials.Source, err)
	}

	if !response.IsSuccess() {
		return fmt.Errorf("music service account removal failed: unexpected response %s", response.Status)
	}

	return nil
}

// AddSpotifyAccount adds a Spotify Premium account
func (c *Client) AddSpotifyAccount(user, password string) error {
	credentials := models.NewSpotifyCredentials(user, password)
	return c.SetMusicServiceAccount(credentials)
}

// RemoveSpotifyAccount removes a Spotify account
func (c *Client) RemoveSpotifyAccount(user string) error {
	credentials := models.NewSpotifyCredentials(user, "")
	return c.RemoveMusicServiceAccount(credentials)
}

// AddPandoraAccount adds a Pandora account
func (c *Client) AddPandoraAccount(user, password string) error {
	credentials := models.NewPandoraCredentials(user, password)
	return c.SetMusicServiceAccount(credentials)
}

// RemovePandoraAccount removes a Pandora account
func (c *Client) RemovePandoraAccount(user string) error {
	credentials := models.NewPandoraCredentials(user, "")
	return c.RemoveMusicServiceAccount(credentials)
}

// AddStoredMusicAccount adds a STORED_MUSIC (NAS/UPnP) account
func (c *Client) AddStoredMusicAccount(user, displayName string) error {
	credentials := models.NewStoredMusicCredentials(user, displayName)
	return c.SetMusicServiceAccount(credentials)
}

// RemoveStoredMusicAccount removes a STORED_MUSIC account
func (c *Client) RemoveStoredMusicAccount(user, displayName string) error {
	credentials := models.NewStoredMusicCredentials(user, displayName)
	return c.RemoveMusicServiceAccount(credentials)
}

// AddAmazonMusicAccount adds an Amazon Music account
func (c *Client) AddAmazonMusicAccount(user, password string) error {
	credentials := models.NewAmazonMusicCredentials(user, password)
	return c.SetMusicServiceAccount(credentials)
}

// RemoveAmazonMusicAccount removes an Amazon Music account
func (c *Client) RemoveAmazonMusicAccount(user string) error {
	credentials := models.NewAmazonMusicCredentials(user, "")
	return c.RemoveMusicServiceAccount(credentials)
}

// AddDeezerAccount adds a Deezer Premium account
func (c *Client) AddDeezerAccount(user, password string) error {
	credentials := models.NewDeezerCredentials(user, password)
	return c.SetMusicServiceAccount(credentials)
}

// RemoveDeezerAccount removes a Deezer account
func (c *Client) RemoveDeezerAccount(user string) error {
	credentials := models.NewDeezerCredentials(user, "")
	return c.RemoveMusicServiceAccount(credentials)
}

// AddIHeartRadioAccount adds an iHeartRadio account
func (c *Client) AddIHeartRadioAccount(user, password string) error {
	credentials := models.NewIHeartRadioCredentials(user, password)
	return c.SetMusicServiceAccount(credentials)
}

// RemoveIHeartRadioAccount removes an iHeartRadio account
func (c *Client) RemoveIHeartRadioAccount(user string) error {
	credentials := models.NewIHeartRadioCredentials(user, "")
	return c.RemoveMusicServiceAccount(credentials)
}
