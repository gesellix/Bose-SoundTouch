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
