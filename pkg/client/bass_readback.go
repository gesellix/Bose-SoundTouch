package client

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// bassReadback retains element presence while decoding a bass response.
// models.Bass remains the public wire model, while client readbacks require
// both levels to be explicitly present and valid.
type bassReadback struct {
	XMLName  xml.Name `xml:"bass"`
	DeviceID string   `xml:"deviceID,attr"`
	Target   *string  `xml:"targetbass"`
	Actual   *string  `xml:"actualbass"`
}

func (readback *bassReadback) bass() (*models.Bass, error) {
	target, err := parseBassReadbackInteger(readback.Target, "targetbass")
	if err != nil {
		return nil, err
	}

	if !models.ValidateBassLevel(target) {
		return nil, fmt.Errorf("invalid target bass level: %d", target)
	}

	actual, err := parseBassReadbackInteger(readback.Actual, "actualbass")
	if err != nil {
		return nil, err
	}

	if !models.ValidateBassLevel(actual) {
		return nil, fmt.Errorf("invalid actual bass level: %d", actual)
	}

	return &models.Bass{
		XMLName:    readback.XMLName,
		DeviceID:   readback.DeviceID,
		TargetBass: target,
		ActualBass: actual,
	}, nil
}

// bassCapabilitiesReadback retains presence for capability fields so an
// omitted value cannot be confused with an explicitly reported zero.
type bassCapabilitiesReadback struct {
	XMLName   xml.Name `xml:"bassCapabilities"`
	DeviceID  string   `xml:"deviceID,attr"`
	Available *string  `xml:"bassAvailable"`
	Min       *string  `xml:"bassMin"`
	Max       *string  `xml:"bassMax"`
	Default   *string  `xml:"bassDefault"`
}

func (readback *bassCapabilitiesReadback) bassCapabilities() (*models.BassCapabilities, error) {
	available, err := parseBassReadbackAvailability(readback.Available)
	if err != nil {
		return nil, err
	}

	capabilities := &models.BassCapabilities{
		XMLName:       readback.XMLName,
		DeviceID:      readback.DeviceID,
		BassAvailable: available,
	}

	for _, field := range []struct {
		text *string
		name string
		dest *int
	}{
		{text: readback.Min, name: "bassMin", dest: &capabilities.BassMin},
		{text: readback.Max, name: "bassMax", dest: &capabilities.BassMax},
		{text: readback.Default, name: "bassDefault", dest: &capabilities.BassDefault},
	} {
		if field.text == nil {
			if available {
				return nil, fmt.Errorf("incomplete bass capabilities readback: missing %s", field.name)
			}

			continue
		}

		value, err := parseBassReadbackInteger(field.text, field.name)
		if err != nil {
			return nil, err
		}

		*field.dest = value
	}

	if !available {
		return capabilities, nil
	}

	if capabilities.BassMin > capabilities.BassMax {
		return nil, fmt.Errorf(
			"invalid bass capabilities readback: bassMin %d exceeds bassMax %d",
			capabilities.BassMin,
			capabilities.BassMax,
		)
	}

	if capabilities.BassDefault < capabilities.BassMin || capabilities.BassDefault > capabilities.BassMax {
		return nil, fmt.Errorf(
			"invalid bass capabilities readback: bassDefault %d is outside the advertised range %d to %d",
			capabilities.BassDefault,
			capabilities.BassMin,
			capabilities.BassMax,
		)
	}

	return capabilities, nil
}

func parseBassReadbackAvailability(text *string) (bool, error) {
	if text == nil {
		return false, fmt.Errorf("incomplete bass capabilities readback: missing bassAvailable")
	}

	raw := strings.TrimSpace(*text)
	if raw == "" {
		return false, fmt.Errorf("invalid bass capabilities readback bassAvailable: %q", *text)
	}

	available, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid bass capabilities readback bassAvailable: %q", *text)
	}

	return available, nil
}

func parseBassReadbackInteger(text *string, field string) (int, error) {
	if text == nil {
		return 0, fmt.Errorf("incomplete bass readback: missing %s", field)
	}

	raw := strings.TrimSpace(*text)
	if raw == "" {
		return 0, fmt.Errorf("invalid bass readback %s: %q", field, *text)
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid bass readback %s: %q", field, *text)
	}

	return value, nil
}
