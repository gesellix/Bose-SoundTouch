package webtypes

import (
	"fmt"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// BalanceTarget is an admitted physical master and pair-certainty generation.
// Its unexported token prevents callers from constructing a valid target.
type BalanceTarget struct {
	HardwareID string
	GroupID    string
	Group      *models.Group
	Balance    *models.Balance
	token      uint64
}

// WithBassOperation serializes bass mutations for one physical speaker.
func (c *DeviceConnection) WithBassOperation(operation func()) {
	c.bassOperationMu.Lock()
	defer c.bassOperationMu.Unlock()

	operation()
}

// WithBalanceOperation serializes balance mutations for one stereo pair
// master connection.
func (c *DeviceConnection) WithBalanceOperation(operation func()) {
	c.balanceOperationMu.Lock()
	defer c.balanceOperationMu.Unlock()

	operation()
}

// BeginBassRefresh coalesces authoritative bass reads into one active and at
// most one trailing read.
func (c *DeviceConnection) BeginBassRefresh() bool {
	return beginCoalescedRefresh(&c.bassRefresh)
}

// EndBassRefresh closes an authoritative bass read and reports whether one
// trailing read was requested while it ran.
func (c *DeviceConnection) EndBassRefresh() bool {
	return endCoalescedRefresh(&c.bassRefresh)
}

func beginCoalescedRefresh(state interface {
	Load() int32
	CompareAndSwap(int32, int32) bool
}) bool {
	for {
		switch current := state.Load(); current {
		case 0:
			if state.CompareAndSwap(0, 1) {
				return true
			}
		default:
			if state.CompareAndSwap(current, 2) {
				return false
			}
		}
	}
}

func endCoalescedRefresh(state interface {
	Load() int32
	CompareAndSwap(int32, int32) bool
}) bool {
	for {
		switch current := state.Load(); current {
		case 2:
			if state.CompareAndSwap(2, 1) {
				return true
			}
		default:
			if state.CompareAndSwap(current, 0) {
				return false
			}
		}
	}
}

// BeginGroupRefreshIfBaseline reserves a completion-ordered group refresh only
// if baseline is still applied, and immediately disables balance writes.
func (c *DeviceConnection) BeginGroupRefreshIfBaseline(baseline uint64) (uint64, bool) {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	if c.groupAppliedGeneration != baseline {
		return 0, false
	}

	c.groupGeneration++
	c.acousticGroupToken++
	c.acousticGroupCertified = 0

	return c.groupGeneration, true
}

// ApplyPolledGroupForBaseline completes a refresh reserved by
// BeginGroupRefreshIfBaseline while preserving completion-based ordering.
func (c *DeviceConnection) ApplyPolledGroupForBaseline(
	baseline, generation uint64,
	group *models.Group,
) (bool, bool) {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	if c.groupAppliedGeneration != baseline || generation <= c.groupAppliedGeneration {
		return false, false
	}

	c.groupAppliedGeneration = generation

	return true, c.applyConfirmedGroupLocked(generation, normalizeGroup(group), time.Time{})
}

// ConfirmedBalanceTarget captures the currently certified pair master.
func (c *DeviceConnection) ConfirmedBalanceTarget() (BalanceTarget, bool) {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	deviceID := ""
	if c.DeviceInfo != nil {
		deviceID = strings.TrimSpace(c.DeviceInfo.DeviceID)
	}

	status := c.Status()
	if status == nil || c.acousticGroupCertified == 0 ||
		c.acousticGroupCertified != c.acousticGroupToken ||
		!confirmedPairMaster(deviceID, status.Group) {
		return BalanceTarget{}, false
	}

	return BalanceTarget{
		HardwareID: deviceID,
		GroupID:    strings.TrimSpace(status.Group.ID),
		Group:      status.Group,
		Balance:    status.Balance,
		token:      c.acousticGroupToken,
	}, true
}

// WithCurrentWebSocket holds the connection's WebSocket identity while fn
// runs, rejecting replacement, removal, or close.
func (c *DeviceConnection) WithCurrentWebSocket(
	expected *client.WebSocketClient,
	fn func() error,
) error {
	c.webSocketMu.RLock()
	defer c.webSocketMu.RUnlock()

	if expected == nil || c.WebSocket != expected {
		return fmt.Errorf("speaker WebSocket changed")
	}

	select {
	case <-c.done:
		return fmt.Errorf("device connection closed")
	default:
	}

	return fn()
}

// WithConfirmedBalanceTarget holds group certainty while fn runs.
func (c *DeviceConnection) WithConfirmedBalanceTarget(target BalanceTarget, fn func() error) error {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	status := c.Status()
	if status == nil || c.DeviceInfo == nil || target.token == 0 || target.token != c.acousticGroupToken ||
		c.acousticGroupCertified != c.acousticGroupToken ||
		target.HardwareID != strings.TrimSpace(c.DeviceInfo.DeviceID) ||
		target.GroupID != strings.TrimSpace(status.Group.ID) ||
		!models.SameGroup(target.Group, status.Group) ||
		!confirmedPairMaster(target.HardwareID, status.Group) {
		return fmt.Errorf("stereo pair topology changed")
	}

	return fn()
}

// BalanceTargetCurrent checks a captured target without invoking a callback.
func (c *DeviceConnection) BalanceTargetCurrent(target BalanceTarget) bool {
	return c.WithConfirmedBalanceTarget(target, func() error { return nil }) == nil
}

// ReserveBalanceField reserves a balance generation for an admitted pair.
func (c *DeviceConnection) ReserveBalanceField(target BalanceTarget) (uint64, bool) {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	status := c.Status()
	if status == nil || target.token == 0 || target.token != c.acousticGroupToken ||
		c.acousticGroupCertified != c.acousticGroupToken ||
		!models.SameGroup(target.Group, status.Group) ||
		!confirmedPairMaster(target.HardwareID, status.Group) {
		return 0, false
	}

	return c.BeginFieldPoll(FieldBalance), true
}

// BeginBalanceRead captures pair certainty and reserves the balance field
// generation before asynchronous I/O starts.
func (c *DeviceConnection) BeginBalanceRead() (BalanceTarget, uint64, bool) {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	deviceID := ""
	if c.DeviceInfo != nil {
		deviceID = strings.TrimSpace(c.DeviceInfo.DeviceID)
	}

	status := c.Status()
	if status == nil || c.acousticGroupCertified == 0 ||
		c.acousticGroupCertified != c.acousticGroupToken ||
		!confirmedPairMaster(deviceID, status.Group) {
		c.BeginFieldPoll(FieldBalance)
		return BalanceTarget{}, 0, false
	}

	generation := c.BeginFieldPoll(FieldBalance)

	return BalanceTarget{
		HardwareID: deviceID,
		GroupID:    strings.TrimSpace(status.Group.ID),
		Group:      status.Group,
		Balance:    status.Balance,
		token:      c.acousticGroupToken,
	}, generation, true
}

// ApplyBalanceRead applies an authoritative result only while both its field
// generation and pair-certainty token remain current.
func (c *DeviceConnection) ApplyBalanceRead(
	target BalanceTarget,
	generation uint64,
	balance *models.Balance,
) (uint64, bool) {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	if balance == nil || target.token == 0 || target.token != c.acousticGroupToken ||
		c.acousticGroupCertified != c.acousticGroupToken ||
		!confirmedPairMaster(target.HardwareID, c.Status().Group) ||
		!models.SameGroup(target.Group, c.Status().Group) {
		return 0, false
	}

	if !c.CompleteFieldPoll(FieldBalance, generation, func(status *DeviceStatus) {
		status.Balance = balance
	}) {
		return 0, false
	}

	return c.Status().BalanceRevision, true
}

func confirmedPairMaster(deviceID string, group *models.Group) bool {
	return validStereoPair(group) && strings.TrimSpace(group.MasterDeviceID) == strings.TrimSpace(deviceID)
}

// ReserveBassWrite checks capability authority at mutation admission and
// invalidates older reads without holding the field lock over network I/O.
func (c *DeviceConnection) ReserveBassWrite(capabilities *models.BassCapabilities) bool {
	c.fieldGenMu.Lock()
	defer c.fieldGenMu.Unlock()

	if capabilities == nil || !capabilities.BassAvailable || c.Status().BassCapabilities != capabilities {
		return false
	}

	c.fieldGen[FieldBass].issued++
	c.fieldGen[FieldBass].applied = c.fieldGen[FieldBass].issued

	return true
}

// ReserveBassRead binds a post-write reading to the capability authority the
// operation admitted. A newer capability result is never recaptured silently.
func (c *DeviceConnection) ReserveBassRead(capabilities *models.BassCapabilities) (uint64, bool) {
	c.fieldGenMu.Lock()
	defer c.fieldGenMu.Unlock()

	if capabilities == nil || c.Status().BassCapabilities != capabilities {
		return 0, false
	}

	c.fieldGen[FieldBass].issued++

	return c.fieldGen[FieldBass].issued, true
}

// CompleteBassRead merges only the bass reading, never its cached capability
// snapshot, while the admitted capability authority and generation still hold.
func (c *DeviceConnection) CompleteBassRead(generation uint64, capabilities *models.BassCapabilities, bass *models.Bass) bool {
	c.fieldGenMu.Lock()
	defer c.fieldGenMu.Unlock()

	if c.Status().BassCapabilities != capabilities || generation <= c.fieldGen[FieldBass].applied {
		return false
	}

	c.updateStatusLocked(func(status *DeviceStatus) { status.Bass = bass; recordFieldRevision(status, FieldBass, generation) })
	c.fieldGen[FieldBass].applied = generation

	return true
}
