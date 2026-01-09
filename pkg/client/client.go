package client

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// Client represents a SoundTouch API client
type Client struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
	userAgent  string
}

// ClientConfig holds configuration for the SoundTouch client
type ClientConfig struct {
	Host      string
	Port      int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns a default client configuration
func DefaultConfig() ClientConfig {
	return ClientConfig{
		Host:      "localhost",
		Port:      8090,
		Timeout:   10 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Client/1.0",
	}
}

// NewClient creates a new SoundTouch API client
func NewClient(config ClientConfig) *Client {
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

// GetPresets retrieves configured presets from the /presets endpoint
func (c *Client) GetPresets() (*models.Presets, error) {
	var presets models.Presets
	err := c.get("/presets", &presets)
	if err != nil {
		return nil, fmt.Errorf("failed to get presets: %w", err)
	}
	return &presets, nil
}

// SendKey sends a key press command to the device (press followed by release)
func (c *Client) SendKey(keyValue string) error {
	if !models.IsValidKey(keyValue) {
		return fmt.Errorf("invalid key value: %s", keyValue)
	}

	// Send press state
	keyPress := models.NewKey(keyValue)
	err := c.post("/key", keyPress, nil)
	if err != nil {
		return fmt.Errorf("failed to send key press: %w", err)
	}

	// Send release state
	keyRelease := models.NewKeyRelease(keyValue)
	err = c.post("/key", keyRelease, nil)
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
	return c.post("/key", key, nil)
}

// SendKeyRelease sends a key release command
func (c *Client) SendKeyRelease(keyValue string) error {
	if !models.IsValidKey(keyValue) {
		return fmt.Errorf("invalid key value: %s", keyValue)
	}

	key := models.NewKeyRelease(keyValue)
	return c.post("/key", key, nil)
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
	return c.post("/volume", volumeReq, nil)
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

	return c.post("/bass", bassReq, nil)
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

	return c.post("/balance", balanceReq, nil)
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
func (c *Client) SelectSource(source string, sourceAccount string) error {
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

	return c.post("/select", contentItem, nil)
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
	defer resp.Body.Close()

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
func (c *Client) post(endpoint string, payload interface{}, result interface{}) error {
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
	defer resp.Body.Close()

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
