package models

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// ClockDisplay represents the device's clock display settings
type ClockDisplay struct {
	XMLName    xml.Name `xml:"clockDisplay"`
	DeviceID   string   `xml:"deviceID,attr,omitempty"`
	Enabled    bool     `xml:"enabled,attr,omitempty"`
	Format     string   `xml:"format,attr,omitempty"`
	Brightness int      `xml:"brightness,attr,omitempty"`
	AutoDim    bool     `xml:"autoDim,attr,omitempty"`
	TimeZone   string   `xml:"timeZone,attr,omitempty"`
	Value      string   `xml:",chardata"`
}

// ClockFormat represents supported clock display formats
type ClockFormat string

const (
	ClockFormat12Hour ClockFormat = "12"
	ClockFormat24Hour ClockFormat = "24"
	ClockFormatAuto   ClockFormat = "auto"
)

// IsEnabled returns true if the clock display is enabled
func (c *ClockDisplay) IsEnabled() bool {
	return c.Enabled
}

// GetFormat returns the clock display format (12/24 hour)
func (c *ClockDisplay) GetFormat() string {
	if c.Format == "" {
		return "12" // Default to 12-hour format
	}
	return c.Format
}

// GetFormatDescription returns a human-readable format description
func (c *ClockDisplay) GetFormatDescription() string {
	switch strings.ToLower(c.Format) {
	case "12":
		return "12-hour format (AM/PM)"
	case "24":
		return "24-hour format"
	case "auto":
		return "Auto format (system default)"
	default:
		return "12-hour format (AM/PM)" // Default
	}
}

// GetBrightness returns the display brightness level (0-100)
func (c *ClockDisplay) GetBrightness() int {
	if c.Brightness < 0 {
		return 0
	}
	if c.Brightness > 100 {
		return 100
	}
	return c.Brightness
}

// GetBrightnessLevel returns a descriptive brightness level
func (c *ClockDisplay) GetBrightnessLevel() string {
	brightness := c.GetBrightness()
	switch {
	case brightness == 0:
		return "Off"
	case brightness <= 25:
		return "Low"
	case brightness <= 50:
		return "Medium"
	case brightness <= 75:
		return "High"
	default:
		return "Maximum"
	}
}

// IsAutoDimEnabled returns true if auto-dim is enabled
func (c *ClockDisplay) IsAutoDimEnabled() bool {
	return c.AutoDim
}

// GetTimeZone returns the timezone setting
func (c *ClockDisplay) GetTimeZone() string {
	return c.TimeZone
}

// GetDeviceID returns the device ID
func (c *ClockDisplay) GetDeviceID() string {
	return c.DeviceID
}

// IsEmpty returns true if the clock display has no configuration
func (c *ClockDisplay) IsEmpty() bool {
	return !c.Enabled && c.Format == "" && c.Brightness == 0 && c.TimeZone == ""
}

// ClockDisplayRequest represents a request to configure clock display settings
type ClockDisplayRequest struct {
	XMLName    xml.Name `xml:"clockDisplay"`
	Enabled    *bool    `xml:"enabled,attr,omitempty"`
	Format     string   `xml:"format,attr,omitempty"`
	Brightness *int     `xml:"brightness,attr,omitempty"`
	AutoDim    *bool    `xml:"autoDim,attr,omitempty"`
	TimeZone   string   `xml:"timeZone,attr,omitempty"`
}

// NewClockDisplayRequest creates a new clock display configuration request
func NewClockDisplayRequest() *ClockDisplayRequest {
	return &ClockDisplayRequest{}
}

// SetEnabled sets whether the clock display is enabled
func (r *ClockDisplayRequest) SetEnabled(enabled bool) *ClockDisplayRequest {
	r.Enabled = &enabled
	return r
}

// SetFormat sets the clock display format (12/24 hour)
func (r *ClockDisplayRequest) SetFormat(format ClockFormat) *ClockDisplayRequest {
	r.Format = string(format)
	return r
}

// SetBrightness sets the display brightness (0-100)
func (r *ClockDisplayRequest) SetBrightness(brightness int) *ClockDisplayRequest {
	if brightness < 0 {
		brightness = 0
	}
	if brightness > 100 {
		brightness = 100
	}
	r.Brightness = &brightness
	return r
}

// SetAutoDim sets whether auto-dim is enabled
func (r *ClockDisplayRequest) SetAutoDim(autoDim bool) *ClockDisplayRequest {
	r.AutoDim = &autoDim
	return r
}

// SetTimeZone sets the timezone
func (r *ClockDisplayRequest) SetTimeZone(timeZone string) *ClockDisplayRequest {
	r.TimeZone = timeZone
	return r
}

// Validate checks if the clock display request is valid
func (r *ClockDisplayRequest) Validate() error {
	if r.Format != "" {
		format := strings.ToLower(r.Format)
		if format != "12" && format != "24" && format != "auto" {
			return fmt.Errorf("invalid format '%s': must be '12', '24', or 'auto'", r.Format)
		}
	}

	if r.Brightness != nil {
		if *r.Brightness < 0 || *r.Brightness > 100 {
			return fmt.Errorf("brightness must be between 0 and 100, got %d", *r.Brightness)
		}
	}

	return nil
}

// HasChanges returns true if the request has any configuration changes
func (r *ClockDisplayRequest) HasChanges() bool {
	return r.Enabled != nil || r.Format != "" || r.Brightness != nil || r.AutoDim != nil || r.TimeZone != ""
}
