package client

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// HTTP readbacks must explicitly contain both levels. Keep this separate from
// models.Volume because firmware WebSocket events may contain only one field.
type volumeReadback struct {
	XMLName      xml.Name `xml:"volume"`
	DeviceID     string   `xml:"deviceID,attr"`
	TargetVolume *string  `xml:"targetvolume"`
	ActualVolume *string  `xml:"actualvolume"`
	MuteEnabled  bool     `xml:"muteenabled"`
}

func (readback *volumeReadback) volume() (*models.Volume, error) {
	target, err := parseVolumeReadbackLevel(readback.TargetVolume, "targetvolume")
	if err != nil {
		return nil, err
	}

	actual, err := parseVolumeReadbackLevel(readback.ActualVolume, "actualvolume")
	if err != nil {
		return nil, err
	}

	return &models.Volume{
		XMLName:      readback.XMLName,
		DeviceID:     readback.DeviceID,
		TargetVolume: target,
		ActualVolume: actual,
		MuteEnabled:  readback.MuteEnabled,
	}, nil
}

func parseVolumeReadbackLevel(text *string, field string) (int, error) {
	if text == nil || strings.TrimSpace(*text) == "" {
		return 0, fmt.Errorf("incomplete volume readback: missing %s", field)
	}

	level, err := strconv.Atoi(strings.TrimSpace(*text))
	if err != nil || !models.ValidateVolumeLevel(level) {
		return 0, fmt.Errorf("invalid volume readback %s: %q", field, *text)
	}

	return level, nil
}
