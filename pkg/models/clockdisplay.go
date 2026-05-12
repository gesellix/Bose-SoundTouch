package models

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ClockDisplay represents the device's clock display settings.
//
// Wire format (confirmed against ST10/ST20 firmware 27.0.6 — flat
// attributes on the outer <clockDisplay> are rejected with
// "Error parsing request"):
//
//	<clockDisplay deviceID="…">
//	  <clockConfig timezoneInfo="Europe/Berlin"
//	               userEnable="true"
//	               timeFormat="TIME_FORMAT_24HOUR_ID"
//	               userOffsetMinute="0"
//	               brightnessLevel="70"
//	               userUtcTime="0"/>
//	</clockDisplay>
//
// The struct keeps its historical flat-field public API so the CLI and
// other callers don't have to be rewritten; custom MarshalXML /
// UnmarshalXML methods bridge to the nested format on the wire.
type ClockDisplay struct {
	XMLName    xml.Name `xml:"-"`
	DeviceID   string
	Enabled    bool
	Format     string // public-facing values: "12", "24", "auto"
	Brightness int
	AutoDim    bool // not on the device's wire format; preserved for API compat
	TimeZone   string
	Value      string // kept for API compat — older fixtures stored chardata here
}

// Wire constants for clockConfig/@timeFormat.
const (
	wireTimeFormat12Hour = "TIME_FORMAT_12HOUR_ID"
	wireTimeFormat24Hour = "TIME_FORMAT_24HOUR_ID"
	wireTimeFormatAuto   = "TIME_FORMAT_AUTO_ID"
)

func mapToWireFormat(f string) string {
	switch strings.ToLower(f) {
	case "12":
		return wireTimeFormat12Hour
	case "24":
		return wireTimeFormat24Hour
	case "auto":
		return wireTimeFormatAuto
	default:
		return ""
	}
}

func mapFromWireFormat(wire string) string {
	switch wire {
	case wireTimeFormat12Hour:
		return "12"
	case wireTimeFormat24Hour:
		return "24"
	case wireTimeFormatAuto:
		return "auto"
	default:
		return ""
	}
}

// UnmarshalXML decodes the nested <clockDisplay><clockConfig …/></clockDisplay>
// into ClockDisplay's flat fields. Tolerates the older flat shape too —
// either because it appears in legacy captures or for forward-compat with
// firmwares that may revert.
func (c *ClockDisplay) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "deviceID":
			c.DeviceID = attr.Value
		case "enabled":
			c.Enabled = attr.Value == "true"
		case "format":
			c.Format = attr.Value
		case "brightness":
			c.Brightness, _ = strconv.Atoi(attr.Value)
		case "autoDim":
			c.AutoDim = attr.Value == "true"
		case "timeZone":
			c.TimeZone = attr.Value
		}
	}

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "clockConfig" {
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "timezoneInfo":
						c.TimeZone = attr.Value
					case "userEnable":
						c.Enabled = attr.Value == "true"
					case "timeFormat":
						if mapped := mapFromWireFormat(attr.Value); mapped != "" {
							c.Format = mapped
						}
					case "brightnessLevel":
						c.Brightness, _ = strconv.Atoi(attr.Value)
					}
				}
			}

			if err := d.Skip(); err != nil {
				return err
			}

		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text != "" {
				c.Value = text
			}

		case xml.EndElement:
			return nil
		}
	}

	return nil
}

// ClockFormat represents supported clock display formats
type ClockFormat string

const (
	// ClockFormat12Hour represents 12-hour clock format
	ClockFormat12Hour ClockFormat = "12"
	// ClockFormat24Hour represents 24-hour clock format
	ClockFormat24Hour ClockFormat = "24"
	// ClockFormatAuto represents automatic clock format selection
	ClockFormatAuto ClockFormat = "auto"
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

// ClockDisplayRequest represents a request to configure clock display
// settings. Fields use the same public names as the response struct;
// MarshalXML produces the nested wire format the device requires.
type ClockDisplayRequest struct {
	XMLName    xml.Name `xml:"-"`
	Enabled    *bool
	Format     string
	Brightness *int
	AutoDim    *bool
	TimeZone   string
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

// MarshalXML emits the nested <clockDisplay><clockConfig …/></clockDisplay>
// envelope the device accepts. Empty fields are omitted so partial updates
// (e.g. "set only the timezone") don't accidentally clear other settings.
//
// AutoDim has no counterpart in the captured wire format; we still accept
// it in the public API for backward-compat but it is not emitted.
func (r ClockDisplayRequest) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	display := xml.StartElement{Name: xml.Name{Local: "clockDisplay"}}
	if err := e.EncodeToken(display); err != nil {
		return err
	}

	cfg := xml.StartElement{Name: xml.Name{Local: "clockConfig"}}

	if r.TimeZone != "" {
		cfg.Attr = append(cfg.Attr, xml.Attr{
			Name:  xml.Name{Local: "timezoneInfo"},
			Value: r.TimeZone,
		})
	}

	if r.Enabled != nil {
		cfg.Attr = append(cfg.Attr, xml.Attr{
			Name:  xml.Name{Local: "userEnable"},
			Value: strconv.FormatBool(*r.Enabled),
		})
	}

	if r.Format != "" {
		if wire := mapToWireFormat(r.Format); wire != "" {
			cfg.Attr = append(cfg.Attr, xml.Attr{
				Name:  xml.Name{Local: "timeFormat"},
				Value: wire,
			})
		}
	}

	if r.Brightness != nil {
		cfg.Attr = append(cfg.Attr, xml.Attr{
			Name:  xml.Name{Local: "brightnessLevel"},
			Value: strconv.Itoa(*r.Brightness),
		})
	}

	if err := e.EncodeToken(cfg); err != nil {
		return err
	}

	if err := e.EncodeToken(xml.EndElement{Name: cfg.Name}); err != nil {
		return err
	}

	if err := e.EncodeToken(xml.EndElement{Name: display.Name}); err != nil {
		return err
	}

	return e.Flush()
}
