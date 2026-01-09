package models

import (
	"encoding/xml"
	"fmt"
	"time"
)

// ClockTime represents the device's system time
type ClockTime struct {
	XMLName xml.Name `xml:"clockTime"`
	Zone    string   `xml:"zone,attr,omitempty"`
	UTC     int64    `xml:"utc,attr,omitempty"`
	Value   string   `xml:",chardata"`
}

// GetTime returns the clock time as a time.Time object
// If UTC is provided, it uses that; otherwise tries to parse the Value
func (c *ClockTime) GetTime() (time.Time, error) {
	if c.UTC > 0 {
		return time.Unix(c.UTC, 0), nil
	}

	if c.Value != "" {
		// Try parsing common time formats
		formats := []string{
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"15:04:05",
		}

		for _, format := range formats {
			if t, err := time.Parse(format, c.Value); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("unable to parse time value: %s", c.Value)
	}

	return time.Time{}, fmt.Errorf("no time data available")
}

// GetUTC returns the UTC timestamp if available
func (c *ClockTime) GetUTC() int64 {
	return c.UTC
}

// GetZone returns the timezone if available
func (c *ClockTime) GetZone() string {
	return c.Zone
}

// GetTimeString returns a formatted time string
func (c *ClockTime) GetTimeString() string {
	if t, err := c.GetTime(); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	return c.Value
}

// IsEmpty returns true if the clock time has no data
func (c *ClockTime) IsEmpty() bool {
	return c.UTC == 0 && c.Value == ""
}

// SetTime sets the clock time from a time.Time object
func (c *ClockTime) SetTime(t time.Time) {
	c.UTC = t.Unix()
	c.Value = t.UTC().Format("2006-01-02 15:04:05")
	c.Zone = t.Location().String()
}

// SetUTC sets the clock time from a UTC timestamp
func (c *ClockTime) SetUTC(utc int64) {
	c.UTC = utc
	t := time.Unix(utc, 0).UTC()
	c.Value = t.Format("2006-01-02 15:04:05")
}

// ClockTimeRequest represents a request to set the device time
type ClockTimeRequest struct {
	XMLName xml.Name `xml:"clockTime"`
	Zone    string   `xml:"zone,attr,omitempty"`
	UTC     int64    `xml:"utc,attr,omitempty"`
	Value   string   `xml:",chardata"`
}

// NewClockTimeRequest creates a new clock time request from a time.Time
func NewClockTimeRequest(t time.Time) *ClockTimeRequest {
	return &ClockTimeRequest{
		Zone:  t.Location().String(),
		UTC:   t.Unix(),
		Value: t.UTC().Format("2006-01-02 15:04:05"),
	}
}

// NewClockTimeRequestUTC creates a new clock time request from UTC timestamp
func NewClockTimeRequestUTC(utc int64) *ClockTimeRequest {
	t := time.Unix(utc, 0).UTC()
	return &ClockTimeRequest{
		UTC:   utc,
		Value: t.Format("2006-01-02 15:04:05"),
	}
}

// Validate checks if the clock time request is valid
func (r *ClockTimeRequest) Validate() error {
	if r.UTC <= 0 && r.Value == "" {
		return fmt.Errorf("either UTC timestamp or time value must be provided")
	}

	if r.UTC > 0 {
		// Validate UTC timestamp is reasonable (after year 2000, before year 2100)
		if r.UTC < 946684800 || r.UTC > 4102444800 {
			return fmt.Errorf("UTC timestamp %d is outside reasonable range", r.UTC)
		}
	}

	return nil
}
