package client

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// balanceReadback retains element presence while decoding a balance response.
// models.Balance deliberately remains a permissive wire model, so client
// readbacks validate the complete response before exposing it to callers.
type balanceReadback struct {
	XMLName   xml.Name `xml:"balance"`
	DeviceID  string   `xml:"deviceID,attr"`
	Available *string  `xml:"balanceAvailable"`
	Min       *string  `xml:"balanceMin"`
	Max       *string  `xml:"balanceMax"`
	Default   *string  `xml:"balanceDefault"`
	Target    *string  `xml:"targetBalance"`
	Actual    *string  `xml:"actualBalance"`
}

func (readback *balanceReadback) balance() (*models.Balance, error) {
	available, err := parseBalanceReadbackAvailability(readback.Available)
	if err != nil {
		return nil, err
	}

	balance := &models.Balance{
		XMLName:   readback.XMLName,
		DeviceID:  readback.DeviceID,
		Available: available,
	}

	if !available {
		for _, field := range []struct {
			text *string
			name string
			dest *int
		}{
			{text: readback.Min, name: "balanceMin", dest: &balance.Min},
			{text: readback.Max, name: "balanceMax", dest: &balance.Max},
			{text: readback.Default, name: "balanceDefault", dest: &balance.Default},
			{text: readback.Target, name: "targetBalance", dest: &balance.Target},
			{text: readback.Actual, name: "actualBalance", dest: &balance.Actual},
		} {
			if field.text == nil {
				continue
			}

			value, err := parseBalanceReadbackInteger(field.text, field.name)
			if err != nil {
				return nil, err
			}

			*field.dest = value
		}

		return balance, nil
	}

	for _, field := range []struct {
		text *string
		name string
		dest *int
	}{
		{text: readback.Min, name: "balanceMin", dest: &balance.Min},
		{text: readback.Max, name: "balanceMax", dest: &balance.Max},
		{text: readback.Default, name: "balanceDefault", dest: &balance.Default},
		{text: readback.Target, name: "targetBalance", dest: &balance.Target},
		{text: readback.Actual, name: "actualBalance", dest: &balance.Actual},
	} {
		if field.text == nil {
			return nil, fmt.Errorf("incomplete balance readback: missing %s", field.name)
		}

		value, err := parseBalanceReadbackInteger(field.text, field.name)
		if err != nil {
			return nil, err
		}

		*field.dest = value
	}

	if balance.Min > balance.Max {
		return nil, fmt.Errorf(
			"invalid balance readback: balanceMin %d exceeds balanceMax %d",
			balance.Min,
			balance.Max,
		)
	}

	if balance.Default < balance.Min || balance.Default > balance.Max {
		return nil, fmt.Errorf(
			"invalid balance readback: balanceDefault %d is outside the advertised range %d to %d",
			balance.Default,
			balance.Min,
			balance.Max,
		)
	}

	return balance, nil
}

func parseBalanceReadbackAvailability(text *string) (bool, error) {
	if text == nil {
		return false, fmt.Errorf("incomplete balance readback: missing balanceAvailable")
	}

	raw := strings.TrimSpace(*text)
	if raw == "" {
		return false, fmt.Errorf("invalid balance readback balanceAvailable: %q", *text)
	}

	available, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid balance readback balanceAvailable: %q", *text)
	}

	return available, nil
}

func parseBalanceReadbackInteger(text *string, field string) (int, error) {
	raw := strings.TrimSpace(*text)
	if raw == "" {
		return 0, fmt.Errorf("invalid balance readback %s: %q", field, *text)
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid balance readback %s: %q", field, *text)
	}

	return value, nil
}
