// Package main provides the soundtouch-cli clock control commands.
package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/urfave/cli/v2"
)

// getClockTime retrieves the current clock time from the device
func getClockTime(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader("Getting clock time", clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	clockTime, err := client.GetClockTime()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to get clock time: %v", err))
		return err
	}

	fmt.Println("Clock Time Information:")

	if timeObj, err := clockTime.GetTime(); err == nil {
		fmt.Printf("  Current time: %s\n", timeObj.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Local time: %02d:%02d:%02d\n", timeObj.Hour(), timeObj.Minute(), timeObj.Second())
	} else {
		fmt.Printf("  Parse error: %v\n", err)

		if clockTime.Value != "" {
			fmt.Printf("  Raw value: %s\n", clockTime.Value)
		}
	}

	if clockTime.GetLocalTime() != nil {
		lt := clockTime.GetLocalTime()

		fmt.Printf("  Local time details:\n")
		fmt.Printf("    Date: %04d-%02d-%02d (day %d)\n", lt.Year, lt.Month+1, lt.DayOfMonth, lt.DayOfWeek)
		fmt.Printf("    Time: %02d:%02d:%02d\n", lt.Hour, lt.Minute, lt.Second)
	}

	if clockTime.GetUTC() > 0 {
		utcTime := time.Unix(clockTime.GetUTC(), 0)
		fmt.Printf("  UTC timestamp: %d (%s)\n", clockTime.GetUTC(), utcTime.Format("2006-01-02 15:04:05 MST"))
	}

	if clockTime.GetTimeFormat() != "" {
		fmt.Printf("  Time format: %s\n", clockTime.GetTimeFormat())
	}

	if clockTime.GetBrightness() > 0 {
		fmt.Printf("  Brightness: %d\n", clockTime.GetBrightness())
	}

	if clockTime.GetUTCSyncTime() > 0 {
		syncTime := time.Unix(clockTime.GetUTCSyncTime(), 0)
		fmt.Printf("  Last sync: %s\n", syncTime.Format("2006-01-02 15:04:05 MST"))
	}

	if clockTime.GetClockError() != 0 {
		fmt.Printf("  Clock error: %d\n", clockTime.GetClockError())
	}

	if clockTime.GetZone() != "" {
		fmt.Printf("  Time zone: %s\n", clockTime.GetZone())
	}

	return nil
}

// setClockTime sets the clock time on the device
func setClockTime(c *cli.Context) error {
	timeStr := c.String("time")
	clientConfig := GetClientConfig(c)

	// Parse time string (HH:MM format)
	var hour, minute int

	var err error

	if timeStr == "now" {
		now := time.Now()
		hour = now.Hour()
		minute = now.Minute()
		PrintDeviceHeader(fmt.Sprintf("Setting clock time to current time (%02d:%02d)", hour, minute), clientConfig.Host, clientConfig.Port)
	} else {
		// Try to parse as Unix timestamp first
		if timestamp, parseErr := strconv.ParseInt(timeStr, 10, 64); parseErr == nil {
			targetTime := time.Unix(timestamp, 0)
			hour = targetTime.Hour()
			minute = targetTime.Minute()
			PrintDeviceHeader(fmt.Sprintf("Setting clock time from Unix timestamp %d (%02d:%02d)", timestamp, hour, minute), clientConfig.Host, clientConfig.Port)
		} else {
			// Parse as HH:MM format
			hour, minute, err = parseTimeString(timeStr)
			if err != nil {
				PrintError(fmt.Sprintf("Invalid time format. Use HH:MM, Unix timestamp, or 'now': %v", err))
				return err
			}

			PrintDeviceHeader(fmt.Sprintf("Setting clock time to %02d:%02d", hour, minute), clientConfig.Host, clientConfig.Port)
		}
	}

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Create a time object for today with the specified hour and minute
	now := time.Now()
	targetTime := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

	clockTimeRequest := models.NewClockTimeRequest(targetTime)

	err = client.SetClockTime(clockTimeRequest)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to set clock time: %v", err))
		return err
	}

	PrintSuccess(fmt.Sprintf("Clock time set to %02d:%02d", hour, minute))

	return nil
}

// setClockTimeNow sets the clock time to the current system time
func setClockTimeNow(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader("Setting clock time to current system time", clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	err = client.SetClockTimeNow()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to set clock time: %v", err))
		return err
	}

	now := time.Now()
	PrintSuccess(fmt.Sprintf("Clock time set to current time (%02d:%02d)", now.Hour(), now.Minute()))

	return nil
}

// setClockDisplayTimezone POSTs only the timezoneInfo attribute,
// leaving format/brightness untouched. Useful after a clock now to
// make the speaker's logs and front-panel display tick in local time
// instead of UTC.
func setClockDisplayTimezone(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	tz := c.String("tz")

	PrintDeviceHeader(fmt.Sprintf("Setting clock timezone to %s", tz), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	request := models.NewClockDisplayRequest().SetTimeZone(tz)
	if err := client.SetClockDisplay(request); err != nil {
		PrintError(fmt.Sprintf("Failed to set timezone: %v", err))
		return err
	}

	PrintSuccess(fmt.Sprintf("Timezone set to %s", tz))

	return nil
}

// getClockDisplay retrieves the current clock display settings
func getClockDisplay(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader("Getting clock display settings", clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	clockDisplay, err := client.GetClockDisplay()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to get clock display: %v", err))
		return err
	}

	fmt.Println("Clock Display Settings:")
	fmt.Printf("  Enabled: %t\n", clockDisplay.IsEnabled())
	fmt.Printf("  Brightness: %d (%s)\n", clockDisplay.GetBrightness(), clockDisplay.GetBrightnessLevel())
	fmt.Printf("  Format: %s (%s)\n", clockDisplay.GetFormat(), clockDisplay.GetFormatDescription())

	if clockDisplay.IsAutoDimEnabled() {
		fmt.Printf("  Auto-dim: enabled\n")
	}

	if clockDisplay.GetTimeZone() != "" {
		fmt.Printf("  Time zone: %s\n", clockDisplay.GetTimeZone())
	}

	return nil
}

// enableClockDisplay enables the clock display
func enableClockDisplay(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader("Enabling clock display", clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	err = client.EnableClockDisplay()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to enable clock display: %v", err))
		return err
	}

	PrintSuccess("Clock display enabled")

	return nil
}

// disableClockDisplay disables the clock display
func disableClockDisplay(c *cli.Context) error {
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader("Disabling clock display", clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	err = client.DisableClockDisplay()
	if err != nil {
		PrintError(fmt.Sprintf("Failed to disable clock display: %v", err))
		return err
	}

	PrintSuccess("Clock display disabled")

	return nil
}

// setClockDisplayBrightness sets the clock display brightness
func setClockDisplayBrightness(c *cli.Context) error {
	brightness := c.String("brightness")
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Setting clock display brightness to %s", brightness), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Convert brightness string to numeric value
	var brightnessLevel int

	switch brightness {
	case "low", "LOW":
		brightnessLevel = 25
	case "medium", "MEDIUM", "med":
		brightnessLevel = 50
	case "high", "HIGH":
		brightnessLevel = 100
	case "off", "OFF":
		brightnessLevel = 0
	default:
		PrintError("Invalid brightness. Use: low, medium, high, or off")
		return fmt.Errorf("invalid brightness value")
	}

	err = client.SetClockDisplayBrightness(brightnessLevel)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to set clock display brightness: %v", err))
		return err
	}

	PrintSuccess(fmt.Sprintf("Clock display brightness set to %s", brightness))

	return nil
}

// setClockDisplayFormat sets the clock display format
func setClockDisplayFormat(c *cli.Context) error {
	format := c.String("format")
	clientConfig := GetClientConfig(c)
	PrintDeviceHeader(fmt.Sprintf("Setting clock display format to %s", format), clientConfig.Host, clientConfig.Port)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to create client: %v", err))
		return err
	}

	// Validate and normalize format value
	var formatSetting models.ClockFormat

	switch format {
	case "12", "12h", "12hour":
		formatSetting = models.ClockFormat12Hour
	case "24", "24h", "24hour":
		formatSetting = models.ClockFormat24Hour
	case "auto":
		formatSetting = models.ClockFormatAuto
	default:
		PrintError("Invalid format. Use: 12 (12-hour), 24 (24-hour), or auto")
		return fmt.Errorf("invalid format value")
	}

	err = client.SetClockDisplayFormat(formatSetting)
	if err != nil {
		PrintError(fmt.Sprintf("Failed to set clock display format: %v", err))
		return err
	}

	PrintSuccess(fmt.Sprintf("Clock display format set to %s", format))

	return nil
}

// parseTimeString parses a time string in HH:MM format
func parseTimeString(timeStr string) (int, int, error) {
	if len(timeStr) != 5 || timeStr[2] != ':' {
		return 0, 0, fmt.Errorf("time must be in HH:MM format")
	}

	hourStr := timeStr[0:2]
	minuteStr := timeStr[3:5]

	hour, err := strconv.Atoi(hourStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid hour: %s", hourStr)
	}

	minute, err := strconv.Atoi(minuteStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minute: %s", minuteStr)
	}

	if hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("hour must be between 0 and 23")
	}

	if minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("minute must be between 0 and 59")
	}

	return hour, minute, nil
}
