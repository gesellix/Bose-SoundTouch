package webtypes

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// ErrBassCapabilitiesIdentityChanged identifies a response from another speaker.
var ErrBassCapabilitiesIdentityChanged = errors.New("bass capabilities speaker identity changed")

// LoadBassCapabilities loads capabilities once per connection and returns a copy.
func (c *DeviceConnection) LoadBassCapabilities() (*models.BassCapabilities, error) {
	c.bassCapabilitiesMu.Lock()
	defer c.bassCapabilitiesMu.Unlock()

	select {
	case <-c.Done():
		return nil, fmt.Errorf("speaker connection closed")
	default:
	}

	if c.bassCapabilities == nil {
		if c.Client == nil {
			return nil, fmt.Errorf("speaker client unavailable")
		}

		capabilities, err := c.Client.GetBassCapabilities()
		if err != nil {
			return nil, err
		}

		if capabilities == nil {
			return nil, fmt.Errorf("speaker returned no bass capabilities")
		}

		if reportedID := strings.TrimSpace(capabilities.DeviceID); reportedID != "" &&
			(c.DeviceInfo == nil || reportedID != strings.TrimSpace(c.DeviceInfo.DeviceID)) {
			return nil, ErrBassCapabilitiesIdentityChanged
		}

		select {
		case <-c.Done():
			return nil, fmt.Errorf("speaker connection closed")
		default:
		}

		cached := *capabilities
		c.bassCapabilities = &cached
	}

	result := *c.bassCapabilities

	return &result, nil
}

// WarmBassCapabilities publishes stable metadata without delaying a status round.
// At most one warm-up runs per connection; failures can retry on a later round.
// Neither the cached value nor the returned status snapshot is mutated in place.
func (c *DeviceConnection) WarmBassCapabilities(published func()) {
	select {
	case <-c.Done():
		return
	default:
	}

	if c.Client == nil || c.Status().BassCapabilities != nil ||
		!c.bassCapabilitiesRefreshing.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer c.bassCapabilitiesRefreshing.Store(false)

		capabilities, err := c.LoadBassCapabilities()
		if err != nil {
			return
		}

		select {
		case <-c.Done():
			return
		default:
		}

		c.UpdateStatus(func(status *DeviceStatus) {
			status.BassCapabilities = capabilities
		})

		if published != nil {
			published()
		}
	}()
}
