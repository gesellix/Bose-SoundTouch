package models

import (
	"encoding/xml"
	"fmt"
)

// Volume represents the response from GET /volume endpoint
type Volume struct {
	XMLName      xml.Name `xml:"volume"`
	DeviceID     string   `xml:"deviceID,attr"`
	TargetVolume int      `xml:"targetvolume"`
	ActualVolume int      `xml:"actualvolume"`
	MuteEnabled  bool     `xml:"muteenabled"`
}

// VolumeRequest represents the request for POST /volume endpoint
type VolumeRequest struct {
	XMLName xml.Name `xml:"volume"`
	Value   int      `xml:",chardata"`
}

// NewVolumeRequest creates a new volume set request
func NewVolumeRequest(volume int) *VolumeRequest {
	return &VolumeRequest{
		Value: volume,
	}
}

// GetLevel returns the current actual volume level
func (v *Volume) GetLevel() int {
	return v.ActualVolume
}

// GetTargetLevel returns the target volume level
func (v *Volume) GetTargetLevel() int {
	return v.TargetVolume
}

// IsMuted returns whether the device is muted
func (v *Volume) IsMuted() bool {
	return v.MuteEnabled
}

// IsVolumeSync returns true if target and actual volumes match
func (v *Volume) IsVolumeSync() bool {
	return v.TargetVolume == v.ActualVolume
}

// GetVolumeString returns a formatted string representation
func (v *Volume) GetVolumeString() string {
	if v.IsMuted() {
		return "Muted"
	}
	return fmt.Sprintf("%d", v.ActualVolume)
}

// ValidateVolumeLevel checks if a volume level is valid (0-100)
func ValidateVolumeLevel(level int) bool {
	return level >= 0 && level <= 100
}

// ClampVolumeLevel ensures a volume level is within valid range
func ClampVolumeLevel(level int) int {
	if level < 0 {
		return 0
	}
	if level > 100 {
		return 100
	}
	return level
}

// Volume level constants
const (
	VolumeMin    = 0
	VolumeMax    = 100
	VolumeMute   = 0
	VolumeQuiet  = 10
	VolumeLow    = 25
	VolumeMedium = 50
	VolumeHigh   = 75
	VolumeLoud   = 100
)

// GetVolumeLevelName returns a descriptive name for volume levels
func GetVolumeLevelName(level int) string {
	switch {
	case level == 0:
		return "Mute"
	case level <= 10:
		return "Very Quiet"
	case level <= 25:
		return "Quiet"
	case level <= 50:
		return "Medium"
	case level <= 75:
		return "High"
	default:
		return "Loud"
	}
}
