package models

import (
	"encoding/xml"
	"fmt"
	"time"
)

// ClockTime represents the device's system time
type ClockTime struct {
	XMLName     xml.Name   `xml:"clockTime"`
	UTCTime     int64      `xml:"utcTime,attr,omitempty"`
	CueMusic    int        `xml:"cueMusic,attr,omitempty"`
	TimeFormat  string     `xml:"timeFormat,attr,omitempty"`
	Brightness  int        `xml:"brightness,attr,omitempty"`
	ClockError  int        `xml:"clockError,attr,omitempty"`
	UTCSyncTime int64      `xml:"utcSyncTime,attr,omitempty"`
	LocalTime   *LocalTime `xml:"localTime,omitempty"`
	Zone        string     `xml:"zone,attr,omitempty"`
	UTC         int64      `xml:"utc,attr,omitempty"`
	Value       string     `xml:",chardata"`
}

// LocalTime represents the local time component of ClockTime
type LocalTime struct {
	XMLName    xml.Name `xml:"localTime"`
	Year       int      `xml:"year,attr"`
	Month      int      `xml:"month,attr"`
	DayOfMonth int      `xml:"dayOfMonth,attr"`
	DayOfWeek  int      `xml:"dayOfWeek,attr"`
	Hour       int      `xml:"hour,attr"`
	Minute     int      `xml:"minute,attr"`
	Second     int      `xml:"second,attr"`
}

// GetTime returns the clock time as a time.Time object
// Priority: LocalTime > UTCTime > UTC > Value
func (c *ClockTime) GetTime() (time.Time, error) {
	// Try LocalTime first (most accurate)
	if c.LocalTime != nil {
		// Note: Device returns month as 0-11, but Go expects 1-12
		month := c.LocalTime.Month + 1
		if month > 12 {
			month = 12
		}

		if month < 1 {
			month = 1
		}

		return time.Date(
			c.LocalTime.Year,
			time.Month(month),
			c.LocalTime.DayOfMonth,
			c.LocalTime.Hour,
			c.LocalTime.Minute,
			c.LocalTime.Second,
			0,
			time.Local,
		), nil
	}

	// Try UTCTime attribute
	if c.UTCTime > 0 {
		return time.Unix(c.UTCTime, 0), nil
	}

	// Try legacy UTC attribute
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
	if c.UTCTime > 0 {
		return c.UTCTime
	}

	return c.UTC
}

// GetZone returns the timezone if available
func (c *ClockTime) GetZone() string {
	return c.Zone
}

// GetTimeFormat returns the time format setting
func (c *ClockTime) GetTimeFormat() string {
	return c.TimeFormat
}

// GetBrightness returns the clock brightness setting
func (c *ClockTime) GetBrightness() int {
	return c.Brightness
}

// GetClockError returns the clock error status
func (c *ClockTime) GetClockError() int {
	return c.ClockError
}

// GetUTCSyncTime returns the UTC sync time
func (c *ClockTime) GetUTCSyncTime() int64 {
	return c.UTCSyncTime
}

// GetLocalTime returns the local time component
func (c *ClockTime) GetLocalTime() *LocalTime {
	return c.LocalTime
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
	return c.UTCTime == 0 && c.UTC == 0 && c.Value == "" && c.LocalTime == nil
}

// SetTime sets the clock time from a time.Time object
func (c *ClockTime) SetTime(t time.Time) {
	c.UTCTime = t.Unix()
	c.UTC = t.Unix() // Keep for backward compatibility
	c.Value = t.UTC().Format("2006-01-02 15:04:05")
	c.Zone = t.Location().String()

	// Set LocalTime component
	c.LocalTime = &LocalTime{
		Year:       t.Year(),
		Month:      int(t.Month()) - 1, // Device expects 0-11
		DayOfMonth: t.Day(),
		DayOfWeek:  int(t.Weekday()),
		Hour:       t.Hour(),
		Minute:     t.Minute(),
		Second:     t.Second(),
	}
}

// SetUTC sets the clock time from a UTC timestamp
func (c *ClockTime) SetUTC(utc int64) {
	c.UTCTime = utc
	c.UTC = utc // Keep for backward compatibility
	t := time.Unix(utc, 0).UTC()
	c.Value = t.Format("2006-01-02 15:04:05")

	// Set LocalTime component
	c.LocalTime = &LocalTime{
		Year:       t.Year(),
		Month:      int(t.Month()) - 1, // Device expects 0-11
		DayOfMonth: t.Day(),
		DayOfWeek:  int(t.Weekday()),
		Hour:       t.Hour(),
		Minute:     t.Minute(),
		Second:     t.Second(),
	}
}

// ClockTimeRequest represents a request to set the device time.
//
// The POST body mirrors the device's GET /clockTime response shape —
// firmware 27 expects `utcTime` as the attribute name, not `utc`, and
// rejects any chardata or zone attribute with "Error parsing request"
// (confirmed against ST10/ST20/ST30 in live testing 2026-05-12).
//
// We deliberately do NOT send TimeFormat / Brightness in the request:
// those belong to /clockDisplay and including them here either gets
// ignored or rejected depending on firmware revision.
type ClockTimeRequest struct {
	XMLName xml.Name `xml:"clockTime"`
	UTCTime int64    `xml:"utcTime,attr"`
}

// NewClockTimeRequest creates a new clock time request from a time.Time.
// The input may be in any zone — we always send Unix-seconds, which the
// device interprets as UTC and renders according to its own clockDisplay
// configuration.
func NewClockTimeRequest(t time.Time) *ClockTimeRequest {
	return &ClockTimeRequest{UTCTime: t.Unix()}
}

// NewClockTimeRequestUTC creates a new clock time request from a Unix
// timestamp in seconds.
func NewClockTimeRequestUTC(utc int64) *ClockTimeRequest {
	return &ClockTimeRequest{UTCTime: utc}
}

// Validate checks if the clock time request is valid.
func (r *ClockTimeRequest) Validate() error {
	if r.UTCTime <= 0 {
		return fmt.Errorf("UTC timestamp must be provided")
	}

	// Plausibility window: after year 2000, before year 2100.
	if r.UTCTime < 946684800 || r.UTCTime > 4102444800 {
		return fmt.Errorf("UTC timestamp %d is outside reasonable range", r.UTCTime)
	}

	return nil
}
