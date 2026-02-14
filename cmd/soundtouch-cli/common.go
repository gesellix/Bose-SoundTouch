package main

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/urfave/cli/v2"
)

// CommonFlags defines flags that are shared across multiple commands
var CommonFlags = []cli.Flag{
	&cli.StringFlag{
		Name:    "host",
		Usage:   "SoundTouch device host/IP address (can include port like host:8090)",
		EnvVars: []string{"SOUNDTOUCH_HOST"},
	},
	&cli.IntFlag{
		Name:    "port",
		Aliases: []string{"p"},
		Usage:   "SoundTouch device port",
		Value:   8090,
		EnvVars: []string{"SOUNDTOUCH_PORT"},
	},
	&cli.DurationFlag{
		Name:    "timeout",
		Aliases: []string{"t"},
		Usage:   "Request timeout",
		Value:   10 * time.Second,
	},
}

// ClientConfig holds configuration for creating a SoundTouch client
type ClientConfig struct {
	Host    string
	Port    int
	Timeout time.Duration
}

// GetClientConfig extracts client configuration from CLI context
func GetClientConfig(c *cli.Context) *ClientConfig {
	host := c.String("host")
	port := c.Int("port")
	timeout := c.Duration("timeout")

	// Parse host:port if host contains a port
	if host != "" {
		if finalHost, finalPort := parseHostPort(host, port); finalHost != "" {
			host = finalHost
			port = finalPort
		}
	}

	return &ClientConfig{
		Host:    host,
		Port:    port,
		Timeout: timeout,
	}
}

// RequireHost validates that a host is provided for commands that need it
func RequireHost(c *cli.Context) error {
	if c.String("host") == "" {
		return fmt.Errorf("host is required. Use --host flag or set SOUNDTOUCH_HOST environment variable")
	}

	return nil
}

// CreateSoundTouchClient creates a configured SoundTouch client
func CreateSoundTouchClient(config *ClientConfig) (*client.Client, error) {
	cfg, err := loadConfig(config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	clientConfig := &client.Config{
		Host:      config.Host,
		Port:      config.Port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	return client.NewClient(clientConfig), nil
}

// loadConfig loads the application configuration with optional timeout override
func loadConfig(timeout time.Duration) (*config.Config, error) {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return nil, err
	}

	// Override timeout if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	return cfg, nil
}

// parseHostPort splits a host:port string into separate host and port components
// If no port is specified, returns the original host and the provided default port
func parseHostPort(hostPort string, defaultPort int) (string, int) {
	// Check if host contains a port (has a colon)
	if strings.Contains(hostPort, ":") {
		host, portStr, err := net.SplitHostPort(hostPort)
		if err != nil {
			// If parsing fails, return original host and default port
			return hostPort, defaultPort
		}

		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			// If port parsing fails or is invalid, return host and default port
			return host, defaultPort
		}

		return host, port
	}

	// No port specified, return original host and default port
	return hostPort, defaultPort
}

// PrintDeviceHeader prints a standard header for device commands
func PrintDeviceHeader(operation, host string, port int) {
	fmt.Printf("%s from %s:%d...\n", operation, host, port)
}

// resolveLocation converts potential URLs to SoundTouch locations
func resolveLocation(source, location string) (string, string) {
	// If it's not a URL, return as is
	if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") {
		return source, location
	}

	// TuneIn URL conversion
	// Example: https://tunein.com/radio/WDR-2-Rheinland-1004-s213886/
	if strings.Contains(location, "tunein.com/radio/") {
		trimmed := strings.TrimSuffix(location, "/")

		parts := strings.Split(trimmed, "-")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			if strings.HasPrefix(lastPart, "s") {
				return "TUNEIN", "/v1/playback/station/" + lastPart
			}
		}
		// Fallback for URLs like https://tunein.com/radio/s213886/
		parts = strings.Split(trimmed, "/")

		lastPart := parts[len(parts)-1]
		if strings.HasPrefix(lastPart, "s") {
			return "TUNEIN", "/v1/playback/station/" + lastPart
		}
	}

	// Spotify URL conversion
	// Example: https://open.spotify.com/album/7F50uh7oGitmAEScRKV6pD?si=YhDPWL9LRGO5whz1wLsteA
	if strings.Contains(location, "open.spotify.com/") {
		re := regexp.MustCompile(`https://open\.spotify\.com/([^/]+)/([^?]+)`)

		matches := re.FindStringSubmatch(location)
		if len(matches) >= 3 {
			contentType := matches[1]
			contentID := matches[2]
			uri := fmt.Sprintf("spotify:%s:%s", contentType, contentID)
			encodedURI := base64.StdEncoding.EncodeToString([]byte(uri))

			return "SPOTIFY", "/playback/container/" + encodedURI
		}
	}

	return source, location
}

type Metadata struct {
	Name    string
	Artwork string
}

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
}

func fetchTuneInMetadata(url string) (*Metadata, error) {
	if !strings.Contains(url, "tunein.com/radio/") {
		return nil, fmt.Errorf("url is not a TuneIn radio URL")
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*100)) // Limit to 100KB
	if err != nil {
		return nil, err
	}

	rawHTML := string(body)
	metadata := &Metadata{}

	// Simple extraction of og:title and og:image
	// Example: <meta data-react-helmet="true" property="og:title" content="WDR 2 Rheinland, 100.4 FM, Köln | Free Internet Radio | TuneIn"/>
	// Example: <meta data-react-helmet="true" property="og:image" content="https://cdn-radiotime-logos.tunein.com/s213886g.png"/>

	titlePrefix := `property="og:title" content="`
	if idx := strings.Index(rawHTML, titlePrefix); idx != -1 {
		start := idx + len(titlePrefix)

		end := strings.Index(rawHTML[start:], `"`)
		if end != -1 {
			title := html.UnescapeString(rawHTML[start : start+end])
			// Clean up title (remove ", 100.4 FM, Köln | Free Internet Radio | TuneIn")
			if pipeIdx := strings.Index(title, " | "); pipeIdx != -1 {
				title = title[:pipeIdx]
			}

			if commaIdx := strings.Index(title, ", "); commaIdx != -1 {
				title = title[:commaIdx]
			}

			metadata.Name = title
		}
	}

	imagePrefix := `property="og:image" content="`
	if idx := strings.Index(rawHTML, imagePrefix); idx != -1 {
		start := idx + len(imagePrefix)

		end := strings.Index(rawHTML[start:], `"`)
		if end != -1 {
			metadata.Artwork = rawHTML[start : start+end]
		}
	}

	return metadata, nil
}

func fetchSpotifyMetadata(url string) (*Metadata, error) {
	if !strings.Contains(url, "open.spotify.com/") {
		return nil, fmt.Errorf("url is not a Spotify URL")
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*200)) // Spotify pages can be larger
	if err != nil {
		return nil, err
	}

	rawHTML := string(body)
	metadata := &Metadata{}

	// Simple extraction of og:title and og:image
	// Example: <meta property="og:title" content="Terminal Caribe - Album by Santi &amp; Tuğçe | Spotify"

	titlePrefix := `property="og:title" content="`
	if idx := strings.Index(rawHTML, titlePrefix); idx != -1 {
		start := idx + len(titlePrefix)

		end := strings.Index(rawHTML[start:], `"`)
		if end != -1 {
			title := html.UnescapeString(rawHTML[start : start+end])
			// Clean up title (remove " | Spotify")
			if pipeIdx := strings.Index(title, " | "); pipeIdx != -1 {
				title = title[:pipeIdx]
			}

			// Spotify often has "- Album by ..." or "- Playlist by ..."
			// We might want to keep it or clean it up.
			// User's TuneIn example cleaned it up.
			// For now let's just keep what Spotify provides as title minus the " | Spotify" part.

			metadata.Name = title
		}
	}

	imagePrefix := `property="og:image" content="`
	if idx := strings.Index(rawHTML, imagePrefix); idx != -1 {
		start := idx + len(imagePrefix)

		end := strings.Index(rawHTML[start:], `"`)
		if end != -1 {
			metadata.Artwork = rawHTML[start : start+end]
		}
	}

	return metadata, nil
}

// PrintSuccess prints a standard success message
func PrintSuccess(message string) {
	fmt.Printf("✓ %s\n", message)
}

// PrintError prints a standard error message
func PrintError(message string) {
	fmt.Printf("✗ %s\n", message)
}

// PrintWarning prints a standard warning message
func PrintWarning(message string) {
	fmt.Printf("⚠️  %s\n", message)
}

// showVersionInfo displays detailed version information including build details
func showVersionInfo(_ *cli.Context) error {
	fmt.Printf("%s version %s\n", os.Args[0], version)
	fmt.Printf("Build commit: %s\n", commit)
	fmt.Printf("Build date: %s\n", date)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	return nil
}
