// Package stereopair coordinates the persistent stereo-pair lifecycle across
// both SoundTouch 10 speakers in a pair.
package stereopair

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// RequestTimeout covers the speaker's observed 15-second Marge retry cycle
// with enough room for multiple attempts before an AddGroup outcome is treated
// as uncertain.
const RequestTimeout = 45 * time.Second

// Client is the subset of the SoundTouch client needed for stereo-pair
// lifecycle operations.
type Client interface {
	GetDeviceInfo() (*models.DeviceInfo, error)
	GetCapabilities() (*models.Capabilities, error)
	GetZone() (*models.ZoneInfo, error)
	GetGroup() (*models.Group, error)
	AddGroup(*models.Group) (*models.Group, error)
	UpdateGroup(*models.Group) (*models.Group, error)
	RemoveGroup() error
}

// ClientFactory returns a client addressed directly to one speaker IP.
type ClientFactory func(ipAddress string) (Client, error)

// GenerationRef identifies one exact persisted stereo-pair generation.
// DeviceInfo values come from the freshly verified physical master, rather
// than from a player registry cache.
type GenerationRef struct {
	GroupID       string
	DeviceID      string
	AccountID     string
	MargeURL      string
	ExpectedGroup *models.Group
}

// GenerationCleanup removes one exact persisted stereo-pair generation.
// Implementations must be idempotent. It runs while the coordinator's
// mutation lock is held, after physical teardown has been verified.
type GenerationCleanup func(GenerationRef) error

// GenerationPreflight verifies that no persisted group record remains for
// speakers whose fresh physical group state is empty. It must be read-only and
// fail closed; exact generation retirement is a separate operation.
type GenerationPreflight func([]GenerationRef) error

// GenerationRename updates the name of one exact persisted generation. It
// must be idempotent so a degraded physical/backend rename can be retried.
type GenerationRename func(GenerationRef, string) error

var (
	// ErrInvalidRequest marks malformed lifecycle input.
	ErrInvalidRequest = errors.New("invalid stereo-pair request")
	// ErrConflict marks a fresh speaker state that no longer matches the request.
	ErrConflict = errors.New("stereo-pair state conflict")
	// ErrUnavailable marks a speaker or transport operation that could not complete.
	ErrUnavailable       = errors.New("stereo-pair speaker unavailable")
	errNoConfiguredPair  = errors.New("no stereo pair configured")
	errUncertainMutation = errors.New("stereo-pair mutation outcome uncertain")
)

// Status describes the verified result of a lifecycle operation.
type Status string

const (
	// StatusSucceeded means every requested postcondition was freshly verified.
	StatusSucceeded Status = "succeeded"
	// StatusFailed means no unverified partial mutation remains.
	StatusFailed Status = "failed"
	// StatusDegraded means the distributed state could not be fully verified or restored.
	StatusDegraded Status = "degraded"
)

// Operation identifies a lifecycle operation.
type Operation string

const (
	// OperationInspect reads stereo-pair state without mutation.
	OperationInspect Operation = "inspect"
	// OperationCreate forms a stereo pair.
	OperationCreate Operation = "create"
	// OperationRename renames an existing stereo pair.
	OperationRename Operation = "rename"
	// OperationDissolve removes an existing stereo pair.
	OperationDissolve Operation = "dissolve"
)

// MemberResult records what happened on one speaker. Group is always from the
// final fresh verification read, never from a mutation response.
type MemberResult struct {
	IPAddress string
	DeviceID  string
	Reachable bool

	PreflightError error

	MutationAttempted bool
	MutationError     error

	Verified          bool
	VerificationError error
	Group             *models.Group

	CompensationAttempted bool
	CompensationError     error
	CompensationVerified  bool
}

// Result describes the verified distributed outcome of an operation.
type Result struct {
	Operation Operation
	Status    Status
	Group     *models.Group
	Members   []MemberResult

	CompensationAttempted bool
	CompensationComplete  bool

	PersistenceAttempted bool
	PersistenceComplete  bool
	PersistenceError     error
}

// Error reports a lifecycle operation that did not reach its requested
// verified postcondition. Result carries the per-member details.
type Error struct {
	Operation Operation
	Status    Status
}

func (e *Error) Error() string {
	return fmt.Sprintf("stereo-pair %s %s", e.Operation, e.Status)
}

// CreateRequest describes a pair. LEFT is always the master.
type CreateRequest struct {
	LeftIPAddress  string
	RightIPAddress string
	Name           string
}

// RenameRequest identifies any current member and the new pair name.
type RenameRequest struct {
	MemberIPAddress string
	ExpectedGroupID string
	Name            string
}

// DissolveRequest identifies one current member and the expected pair
// generation. The generation guard prevents a stale UI or CLI request from
// removing a newer pair.
type DissolveRequest struct {
	MemberIPAddress string
	ExpectedGroupID string
	// ExpectedGroup is the last exact generation observed by the caller. It is
	// used only when the addressed member is already standalone, so a retry can
	// safely verify both known members and finish an interrupted teardown.
	ExpectedGroup *models.Group
}

// Coordinator serializes lifecycle mutations performed through one instance.
type Coordinator struct {
	factory                ClientFactory
	generationCleanup      GenerationCleanup
	generationPreflight    GenerationPreflight
	generationRename       GenerationRename
	uncertainOutcomeDelays []time.Duration
	mu                     sync.Mutex
}

// New creates a stereo-pair lifecycle coordinator.
func New(factory ClientFactory) *Coordinator {
	return NewWithGenerationCleanup(factory, nil)
}

// NewWithGenerationCleanup creates a coordinator that also retires the exact
// persisted generation after verified physical teardown.
func NewWithGenerationCleanup(factory ClientFactory, cleanup GenerationCleanup) *Coordinator {
	return NewWithGenerationPersistence(factory, cleanup, nil)
}

// NewWithGenerationPersistence creates a coordinator with exact post-teardown
// cleanup and a read-only pre-create generation barrier.
func NewWithGenerationPersistence(
	factory ClientFactory,
	cleanup GenerationCleanup,
	preflight GenerationPreflight,
) *Coordinator {
	return NewWithGenerationLifecyclePersistence(factory, cleanup, preflight, nil)
}

// NewWithGenerationLifecyclePersistence creates a coordinator with exact
// cleanup, pre-create checks, and persistent rename support.
func NewWithGenerationLifecyclePersistence(
	factory ClientFactory,
	cleanup GenerationCleanup,
	preflight GenerationPreflight,
	rename GenerationRename,
) *Coordinator {
	return &Coordinator{
		factory:                factory,
		generationCleanup:      cleanup,
		generationPreflight:    preflight,
		generationRename:       rename,
		uncertainOutcomeDelays: []time.Duration{time.Second, 4 * time.Second, 10 * time.Second},
	}
}

type memberState struct {
	result          MemberResult
	client          Client
	info            *models.DeviceInfo
	zone            *models.ZoneInfo
	group           *models.Group
	mutationGroupID string
	mutationGroup   *models.Group

	compensationVerificationError error
}

// Inspect reads the addressed speaker and, for a configured pair, freshly
// reads both members. Group is populated only when those member reads agree.
func (c *Coordinator) Inspect(memberIPAddress string) (Result, error) {
	result := Result{Operation: OperationInspect, Status: StatusFailed}
	if c.factory == nil {
		states := []memberState{{result: MemberResult{
			IPAddress:      memberIPAddress,
			PreflightError: fmt.Errorf("%w: client factory is nil", ErrUnavailable),
		}}}

		return finish(result, states)
	}

	entry, err := c.factory(memberIPAddress)
	if err != nil {
		states := []memberState{{result: MemberResult{
			IPAddress:      memberIPAddress,
			PreflightError: wrapUnavailable("create client", err),
		}}}

		return finish(result, states)
	}

	current, err := entry.GetGroup()
	if err != nil {
		states := []memberState{{result: MemberResult{
			IPAddress:      memberIPAddress,
			PreflightError: wrapUnavailable("get current group", err),
		}}}

		return finish(result, states)
	}

	if current == nil {
		states := []memberState{{result: MemberResult{
			IPAddress:      memberIPAddress,
			Reachable:      true,
			PreflightError: wrapUnavailable("get current group", errors.New("nil response")),
		}}}

		return finish(result, states)
	}

	if current.IsEmpty() {
		states := []memberState{{result: MemberResult{IPAddress: memberIPAddress, Reachable: true, Verified: true, Group: cloneGroup(current)}}}
		result.Status = StatusSucceeded
		result.Group = cloneGroup(current)

		return finish(result, states)
	}

	members, err := pairMembers(current)
	if err != nil {
		states := []memberState{{result: MemberResult{
			IPAddress:      memberIPAddress,
			Reachable:      true,
			PreflightError: fmt.Errorf("%w: %w", ErrConflict, err),
			Group:          cloneGroup(current),
		}}}

		return finish(result, states)
	}

	// Preserve the initiating member's exact generation even when the peer is
	// already empty or unreachable. Clients can then retry a partial dissolve
	// through the surviving grouped member instead of losing the recovery key.
	result.Group = cloneGroup(current)

	states := make([]memberState, len(members))
	for i, member := range members {
		states[i].result.IPAddress = member.IPAddress
		states[i].result.DeviceID = member.DeviceID

		states[i].client, err = c.factory(member.IPAddress)
		if err != nil {
			states[i].result.PreflightError = wrapUnavailable("create client", err)
			continue
		}

		states[i].group, err = states[i].client.GetGroup()

		states[i].result.Group = cloneGroup(states[i].group)
		if err != nil {
			states[i].result.VerificationError = wrapUnavailable("get group", err)
			continue
		}

		states[i].result.Reachable = true
		if err := verifyConfiguredGroup(states[i].group); err != nil {
			states[i].result.VerificationError = err
			continue
		}

		if !sameGroupConfiguration(states[i].group, current) {
			states[i].result.VerificationError = errors.New("members do not agree on the current group")
			continue
		}

		states[i].result.Verified = true
	}

	if pairVerified(states) {
		result.Status = StatusSucceeded
		result.Group = cloneGroup(states[0].group)
	} else {
		result.Status = StatusDegraded
	}

	return finish(result, states)
}

func (c *Coordinator) renameGeneration(result *Result, ref GenerationRef, name string) error {
	if c.generationRename == nil {
		return nil
	}

	result.PersistenceAttempted = true
	result.PersistenceComplete = false

	if ref.GroupID == "" || ref.DeviceID == "" || ref.ExpectedGroup == nil ||
		ref.ExpectedGroup.ID != ref.GroupID || ref.ExpectedGroup.MasterDeviceID != ref.DeviceID {
		result.PersistenceError = fmt.Errorf("%w: persisted generation has no exact group topology", ErrConflict)

		return result.PersistenceError
	}

	if err := c.generationRename(ref, name); err != nil {
		if errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrConflict) || errors.Is(err, ErrUnavailable) {
			result.PersistenceError = fmt.Errorf("rename persisted group generation: %w", err)
		} else {
			result.PersistenceError = wrapUnavailable("rename persisted group generation", err)
		}

		return result.PersistenceError
	}

	result.PersistenceComplete = true
	result.PersistenceError = nil

	return nil
}

// Create forms a persistent stereo pair and verifies both speakers with fresh
// GetGroup calls. If only a subset adopts the requested pair, Create removes
// only freshly observed exact matches and reports whether cleanup completed.
func (c *Coordinator) Create(req CreateRequest) (Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	states := []memberState{
		{result: MemberResult{IPAddress: req.LeftIPAddress}},
		{result: MemberResult{IPAddress: req.RightIPAddress}},
	}
	result := Result{Operation: OperationCreate, Status: StatusFailed}

	expected := c.prepareCreate(req, states, &result)
	if expected == nil {
		return finish(result, states)
	}

	c.applyCreate(states, expected, req.LeftIPAddress)

	verifyPair(states, func(group *models.Group) error {
		return verifyCreatedGroup(group, expected)
	})

	if !pairVerified(states) && hasUncertainMutationOutcome(states) {
		for _, delay := range c.uncertainOutcomeDelays {
			time.Sleep(delay)
			verifyPair(states, func(group *models.Group) error {
				return verifyCreatedGroup(group, expected)
			})

			if pairVerified(states) {
				break
			}
		}
	}

	captureVerifiedGenerationIDs(states, expected)

	if pairVerified(states) {
		result.Group = cloneGroup(states[0].group)
		if !verifyCreatedPairZones(states) {
			result.Status = StatusDegraded

			return finish(result, states)
		}

		result.Status = StatusSucceeded

		return finish(result, states)
	}

	c.compensateCreate(&result, states, expected)

	return finish(result, states)
}

func (c *Coordinator) prepareCreate(req CreateRequest, states []memberState, result *Result) *models.Group {
	if c.factory == nil {
		states[0].result.PreflightError = fmt.Errorf("%w: client factory is nil", ErrUnavailable)

		return nil
	}

	for i := range states {
		c.preflightCreate(&states[i])
	}

	validateCreateCandidates(states)
	validateCreateZones(states)

	if hasPreflightError(states) {
		return nil
	}

	if c.generationPreflight != nil {
		result.PersistenceAttempted = true
		if err := c.generationPreflight(createCandidateGenerationRefs(states)); err != nil {
			result.PersistenceError = wrapUnavailable("check persisted group generations", err)
			setPreflightError(&states[0], result.PersistenceError)

			return nil
		}

		result.PersistenceComplete = true

		c.revalidateCreateCandidates(states)
		validateCreateCandidates(states)
		validateCreateZones(states)

		if hasPreflightError(states) {
			return nil
		}
	}

	name := req.Name
	if name == "" {
		name = states[0].info.Name + " + " + states[1].info.Name
	}

	return &models.Group{
		Name:           name,
		MasterDeviceID: states[0].info.DeviceID,
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: states[0].info.DeviceID, Role: "LEFT", IPAddress: req.LeftIPAddress},
			{DeviceID: states[1].info.DeviceID, Role: "RIGHT", IPAddress: req.RightIPAddress},
		}},
	}
}

func validateCreateCandidates(states []memberState) {
	if len(states) != 2 {
		return
	}

	if sameIP(states[0].result.IPAddress, states[1].result.IPAddress) {
		setPreflightError(&states[1], fmt.Errorf("%w: LEFT and RIGHT IP addresses must be distinct", ErrInvalidRequest))
	}

	if states[0].info == nil || states[1].info == nil {
		return
	}

	if states[0].info.DeviceID == states[1].info.DeviceID {
		setPreflightError(&states[1], fmt.Errorf("%w: LEFT and RIGHT device IDs must be distinct", ErrInvalidRequest))
	}

	if !SameMargeBackend(states[0].info.MargeURL, states[1].info.MargeURL) {
		setPreflightError(&states[1], fmt.Errorf("%w: LEFT and RIGHT speakers must use the same Marge backend", ErrConflict))
	}
}

type createZoneTopology struct {
	master  string
	devices map[string]struct{}
}

func validateCreateZones(states []memberState) {
	if len(states) != 2 || states[0].info == nil || states[1].info == nil {
		return
	}

	topologies := make([]createZoneTopology, len(states))

	standalone := make([]bool, len(states))
	for i := range states {
		var err error

		topologies[i], standalone[i], err = parseCreateZone(states[i].zone, states[i].info.DeviceID)
		if err != nil {
			setPreflightError(&states[i], fmt.Errorf("%w: invalid zone view: %w", ErrConflict, err))
		}
	}

	if hasPreflightError(states) {
		return
	}

	if standalone[0] && standalone[1] {
		return
	}

	if standalone[0] != standalone[1] {
		err := fmt.Errorf("%w: LEFT and RIGHT speakers disagree about temporary zone membership", ErrConflict)
		setPreflightError(&states[0], err)
		setPreflightError(&states[1], err)

		return
	}

	if !sameCreateZone(topologies[0], topologies[1]) {
		err := fmt.Errorf("%w: LEFT and RIGHT speakers report different temporary zones", ErrConflict)
		setPreflightError(&states[0], err)
		setPreflightError(&states[1], err)

		return
	}

	for i := range states {
		if _, ok := topologies[0].devices[states[i].info.DeviceID]; !ok {
			err := fmt.Errorf("%w: temporary zone does not contain both stereo-pair candidates", ErrConflict)
			setPreflightError(&states[0], err)
			setPreflightError(&states[1], err)

			return
		}
	}
}

func parseCreateZone(zone *models.ZoneInfo, deviceID string) (createZoneTopology, bool, error) {
	topology := createZoneTopology{}
	if zone == nil {
		return topology, false, errors.New("zone response is nil")
	}

	if zone.Master == "" && len(zone.Members) == 0 {
		return topology, true, nil
	}

	topology.master = strings.TrimSpace(zone.Master)
	if topology.master == "" {
		return topology, false, errors.New("zone master is empty")
	}

	topology.devices = map[string]struct{}{topology.master: {}}

	listed := make(map[string]struct{}, len(zone.Members))
	for i := range zone.Members {
		memberID := strings.TrimSpace(zone.Members[i].DeviceID)
		if memberID == "" {
			return createZoneTopology{}, false, errors.New("zone member device ID is empty")
		}

		if _, duplicate := listed[memberID]; duplicate {
			return createZoneTopology{}, false, fmt.Errorf("zone repeats member %q", memberID)
		}

		listed[memberID] = struct{}{}
		topology.devices[memberID] = struct{}{}
	}

	if _, ok := topology.devices[deviceID]; !ok {
		return createZoneTopology{}, false, errors.New("zone view does not contain the physical speaker")
	}

	standalone := len(topology.devices) == 1 && topology.master == deviceID

	return topology, standalone, nil
}

func sameCreateZone(left, right createZoneTopology) bool {
	if left.master != right.master || len(left.devices) != len(right.devices) {
		return false
	}

	for deviceID := range left.devices {
		if _, ok := right.devices[deviceID]; !ok {
			return false
		}
	}

	return true
}

func (c *Coordinator) revalidateCreateCandidates(states []memberState) {
	for i := range states {
		previous := states[i].info
		fresh := memberState{result: MemberResult{IPAddress: states[i].result.IPAddress}}
		c.preflightCreate(&fresh)

		if fresh.result.PreflightError == nil && !sameCreateIdentity(previous, fresh.info) {
			fresh.result.PreflightError = fmt.Errorf("%w: speaker identity or Marge ownership changed during preflight", ErrConflict)
		}

		states[i] = fresh
	}
}

func sameCreateIdentity(previous, current *models.DeviceInfo) bool {
	return previous != nil && current != nil &&
		previous.DeviceID == current.DeviceID &&
		previous.MargeAccountUUID == current.MargeAccountUUID &&
		SameMargeBackend(previous.MargeURL, current.MargeURL)
}

func (c *Coordinator) applyCreate(states []memberState, expected *models.Group, masterIPAddress string) {
	if len(states) != 2 {
		return
	}

	masterRequest := cloneGroup(expected)
	applyCreateMember(&states[0], masterRequest)

	if states[0].result.MutationError != nil &&
		(!isUncertainMutationOutcome(states[0].result.MutationError) ||
			!c.resolveCreateGeneration(&states[0], expected)) {
		return
	}

	// The master persists the group through Marge and receives its generated
	// ID. Give that exact generation to the slave so it cannot race a Marge
	// lookup against the master's create request.
	slaveRequest := cloneGroup(expected)
	slaveRequest.ID = states[0].mutationGroupID
	slaveRequest.SenderIPAddress = masterIPAddress
	applyCreateMember(&states[1], slaveRequest)
}

func (c *Coordinator) resolveCreateGeneration(state *memberState, expected *models.Group) bool {
	readGeneration := func() bool {
		group, err := state.client.GetGroup()
		state.group = group
		state.result.Group = cloneGroup(group)

		if err != nil || verifyCreatedGroup(group, expected) != nil {
			return false
		}

		state.mutationGroupID = group.ID
		state.mutationGroup = cloneGroup(group)
		state.result.MutationError = nil

		return true
	}

	if readGeneration() {
		return true
	}

	for _, delay := range c.uncertainOutcomeDelays {
		time.Sleep(delay)

		if readGeneration() {
			return true
		}
	}

	return false
}

func applyCreateMember(state *memberState, request *models.Group) {
	state.result.MutationAttempted = true
	response, err := state.client.AddGroup(request)
	state.result.MutationError = validateMutationResponse("addGroup", response, err)

	if state.result.MutationError == nil {
		state.mutationGroupID = response.ID
	}
}

func (c *Coordinator) compensateCreate(result *Result, states []memberState, expected *models.Group) {
	result.Status = StatusDegraded

	for i := range states {
		if states[i].mutationGroupID == "" ||
			!matchesRequestedGeneration(states[i].group, expected, states[i].mutationGroupID) {
			continue
		}

		latest, err := states[i].client.GetGroup()
		states[i].group = latest

		states[i].result.Group = cloneGroup(latest)
		if err != nil {
			states[i].result.CompensationError = wrapUnavailable("recheck group before compensation", err)

			continue
		}

		if !matchesRequestedGeneration(latest, expected, states[i].mutationGroupID) {
			states[i].result.CompensationError = fmt.Errorf("%w: group changed before compensation", ErrConflict)

			continue
		}

		states[i].mutationGroup = cloneGroup(latest)

		result.CompensationAttempted = true
		states[i].result.CompensationAttempted = true
		states[i].result.CompensationError = wrapUnavailable("remove group during compensation", states[i].client.RemoveGroup())
	}

	if !result.CompensationAttempted {
		return
	}

	verifyCompensation(states)

	if compensationReverificationPending(states) {
		for _, delay := range c.uncertainOutcomeDelays {
			time.Sleep(delay)
			verifyCompensation(states)

			if !compensationReverificationPending(states) {
				break
			}
		}
	}

	result.CompensationComplete = compensationVerified(states)
	if !result.CompensationComplete {
		for i := range states {
			if !states[i].result.CompensationVerified && states[i].result.CompensationError == nil {
				states[i].result.CompensationError = states[i].compensationVerificationError
			}
		}

		return
	}

	for _, ref := range compensationGenerations(states) {
		if err := c.cleanupGeneration(result, ref); err != nil {
			result.CompensationComplete = false
			result.Status = StatusDegraded

			return
		}
	}

	result.Status = StatusFailed
}

// Rename updates the full current group on both members, then verifies that
// both speakers expose the same renamed pair.
func (c *Coordinator) Rename(req RenameRequest) (Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := Result{Operation: OperationRename, Status: StatusFailed}

	name := strings.TrimSpace(req.Name)
	if name == "" || strings.TrimSpace(req.ExpectedGroupID) == "" {
		states := []memberState{{result: MemberResult{
			IPAddress: req.MemberIPAddress,
			PreflightError: fmt.Errorf("%w: pair name and expected group ID are required",
				ErrInvalidRequest),
		}}}

		return finish(result, states)
	}

	states, current, err := c.loadPair(req.MemberIPAddress, req.ExpectedGroupID, false)
	if err != nil {
		if len(states) == 0 {
			states = []memberState{{result: MemberResult{IPAddress: req.MemberIPAddress, PreflightError: err}}}
		}

		return finish(result, states)
	}

	applyRename(states, current, name)

	expected := cloneGroup(current)
	expected.Name = name

	verifyRenamedPair := func(group *models.Group) error {
		if err := verifyConfiguredGroup(group); err != nil {
			return err
		}

		if !sameGroupConfiguration(group, expected) {
			return errors.New("group does not match the renamed pair")
		}

		return nil
	}
	verifyPair(states, verifyRenamedPair)

	if pairVerified(states) {
		result.Group = cloneGroup(states[0].group)

		renameErr := c.renameGeneration(&result, generationRef(states, current), name)
		if c.generationRename != nil {
			// Persistence can cross a process boundary, so verify the physical
			// postcondition once more before reporting distributed success.
			verifyPair(states, verifyRenamedPair)

			if pairVerified(states) {
				result.Group = cloneGroup(states[0].group)
			} else {
				result.Group = nil
			}
		}

		if renameErr == nil && pairVerified(states) {
			result.Status = StatusSucceeded
		} else {
			result.Status = StatusDegraded
		}
	} else {
		result.Status = StatusDegraded
	}

	return finish(result, states)
}

func applyRename(states []memberState, current *models.Group, name string) {
	var wg sync.WaitGroup

	for i := range states {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			states[index].result.MutationAttempted = true

			latest, err := states[index].client.GetGroup()
			if err != nil {
				states[index].result.MutationError = wrapUnavailable("recheck group before rename", err)

				return
			}

			if !sameGroupTopology(latest, current) {
				states[index].result.MutationError = fmt.Errorf("%w: group changed before rename", ErrConflict)

				return
			}

			update := cloneGroup(latest)
			update.Name = name
			update.Status = ""
			response, updateErr := states[index].client.UpdateGroup(update)
			states[index].result.MutationError = validateMutationResponse("updateGroup", response, updateErr)
		}(i)
	}

	wg.Wait()
}

// Dissolve removes the current pair from every member that passes a fresh
// group read, then verifies empty groups on both speakers.
func (c *Coordinator) Dissolve(req DissolveRequest) (Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := Result{Operation: OperationDissolve, Status: StatusFailed}
	if strings.TrimSpace(req.ExpectedGroupID) == "" {
		states := []memberState{{result: MemberResult{
			IPAddress:      req.MemberIPAddress,
			PreflightError: fmt.Errorf("%w: expected group ID is required", ErrInvalidRequest),
		}}}

		return finish(result, states)
	}

	states, current, err := c.loadPair(req.MemberIPAddress, req.ExpectedGroupID, true)
	if err != nil && errors.Is(err, errNoConfiguredPair) && req.ExpectedGroup != nil {
		states, current, err = c.loadPairFromSnapshot(req.MemberIPAddress, req.ExpectedGroupID, req.ExpectedGroup)
	}

	if err != nil {
		if len(states) == 0 {
			states = []memberState{{result: MemberResult{IPAddress: req.MemberIPAddress, PreflightError: err}}}
		}

		return finish(result, states)
	}

	result.Group = cloneGroup(current)

	applyDissolve(states, current)
	verifyDissolved(states)

	if removalReverificationPending(states) {
		for _, delay := range c.uncertainOutcomeDelays {
			time.Sleep(delay)
			verifyDissolved(states)

			if !removalReverificationPending(states) {
				break
			}
		}
	}

	if !allVerified(states) && compensateDissolve(states) {
		verifyDissolved(states)

		if removalReverificationPending(states) {
			for _, delay := range c.uncertainOutcomeDelays {
				time.Sleep(delay)
				verifyDissolved(states)

				if !removalReverificationPending(states) {
					break
				}
			}
		}
	}

	if allVerified(states) {
		result.Group = &models.Group{}
		if cleanupErr := c.cleanupGeneration(&result, generationRef(states, current)); cleanupErr == nil {
			result.Status = StatusSucceeded
		} else {
			result.Status = StatusDegraded
		}
	} else {
		result.Status = StatusDegraded
	}

	return finish(result, states)
}

// compensateDissolve retries RemoveGroup on any member that hasn't yet
// verified empty, giving a transient partial failure (one member's removal
// races the topology check, or a firmware retry is needed) a chance to
// converge instead of settling for StatusDegraded on the first pass.
func compensateDissolve(states []memberState) bool {
	attempted := false

	for i := range states {
		if states[i].result.Verified {
			continue
		}

		attempted = true
		states[i].result.MutationAttempted = true
		states[i].result.MutationError = wrapUnavailable("remove group during dissolve compensation", states[i].client.RemoveGroup())
	}

	return attempted
}

func applyDissolve(states []memberState, current *models.Group) {
	var wg sync.WaitGroup

	for i := range states {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			latest, err := states[index].client.GetGroup()
			if err != nil {
				states[index].result.MutationError = wrapUnavailable("recheck group before dissolve", err)

				return
			}

			if latest == nil {
				states[index].result.MutationError = fmt.Errorf("%w: group changed before dissolve", ErrConflict)

				return
			}

			if latest.IsEmpty() {
				return
			}

			if !sameGroupTopology(latest, current) {
				states[index].result.MutationError = fmt.Errorf("%w: group changed before dissolve", ErrConflict)

				return
			}

			states[index].result.MutationAttempted = true
			states[index].result.MutationError = wrapUnavailable("remove group", states[index].client.RemoveGroup())
		}(i)
	}

	wg.Wait()
}

func verifyDissolved(states []memberState) {
	for i := range states {
		states[i].result.Verified = false
		states[i].result.VerificationError = nil

		group, err := states[i].client.GetGroup()
		states[i].group = group

		states[i].result.Group = cloneGroup(group)

		switch {
		case err != nil:
			states[i].result.VerificationError = wrapUnavailable("get group after dissolve", err)
		case group == nil || !group.IsEmpty():
			states[i].result.VerificationError = errors.New("group is not empty after dissolve")
		default:
			states[i].result.Verified = true
		}
	}
}

func removalReverificationPending(states []memberState) bool {
	for i := range states {
		if states[i].result.MutationAttempted && !states[i].result.Verified {
			return true
		}
	}

	return false
}

func verifyCompensation(states []memberState) {
	for i := range states {
		states[i].result.CompensationVerified = false
		states[i].compensationVerificationError = nil

		group, groupErr := states[i].client.GetGroup()
		states[i].group = group
		states[i].result.Group = cloneGroup(group)

		var verificationErrors []error
		if groupErr != nil {
			verificationErrors = append(verificationErrors, wrapUnavailable("verify compensation group", groupErr))
		} else if group == nil || !group.IsEmpty() {
			verificationErrors = append(verificationErrors, errors.New("compensation did not leave an empty group"))
		}

		zone, zoneErr := states[i].client.GetZone()
		if zoneErr != nil {
			verificationErrors = append(verificationErrors, wrapUnavailable("verify compensation zone", zoneErr))
		} else if err := verifyCompensationZone(states[i].zone, zone, states[i].result.DeviceID); err != nil {
			verificationErrors = append(verificationErrors, err)
		}

		if len(verificationErrors) == 0 {
			states[i].result.CompensationVerified = true
		} else {
			states[i].compensationVerificationError = errors.Join(verificationErrors...)
		}
	}
}

func verifyCompensationZone(expected, actual *models.ZoneInfo, deviceID string) error {
	expectedTopology, expectedStandalone, err := parseCreateZone(expected, deviceID)
	if err != nil {
		return fmt.Errorf("invalid pre-compensation zone: %w", err)
	}

	actualTopology, actualStandalone, err := parseCreateZone(actual, deviceID)
	if err != nil {
		return fmt.Errorf("invalid zone after compensation: %w", err)
	}

	if expectedStandalone || actualStandalone {
		if expectedStandalone && actualStandalone {
			return nil
		}

		return errors.New("compensation did not restore the pre-pairing zone topology")
	}

	if !sameCreateZone(expectedTopology, actualTopology) {
		return errors.New("compensation did not restore the pre-pairing zone topology")
	}

	return nil
}

func compensationVerified(states []memberState) bool {
	if len(states) == 0 {
		return false
	}

	for i := range states {
		if !states[i].result.CompensationVerified {
			return false
		}
	}

	return true
}

func compensationReverificationPending(states []memberState) bool {
	for i := range states {
		if states[i].result.CompensationAttempted && !states[i].result.CompensationVerified {
			return true
		}
	}

	return false
}

func (c *Coordinator) cleanupGeneration(result *Result, ref GenerationRef) error {
	if c.generationCleanup == nil {
		return nil
	}

	result.PersistenceAttempted = true
	result.PersistenceComplete = false

	if ref.GroupID == "" || ref.DeviceID == "" || ref.ExpectedGroup == nil ||
		ref.ExpectedGroup.ID != ref.GroupID || ref.ExpectedGroup.MasterDeviceID != ref.DeviceID {
		result.PersistenceError = fmt.Errorf("%w: persisted generation has no exact group topology", ErrConflict)

		return result.PersistenceError
	}

	if err := c.generationCleanup(ref); err != nil {
		if errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrConflict) || errors.Is(err, ErrUnavailable) {
			result.PersistenceError = fmt.Errorf("remove persisted group generation: %w", err)
		} else {
			result.PersistenceError = wrapUnavailable("remove persisted group generation", err)
		}

		return result.PersistenceError
	}

	result.PersistenceComplete = true
	result.PersistenceError = nil

	return nil
}

func generationRef(states []memberState, group *models.Group) GenerationRef {
	if group == nil {
		return GenerationRef{}
	}

	for i := range states {
		if states[i].info == nil || states[i].info.DeviceID != group.MasterDeviceID {
			continue
		}

		ref := generationRefForState(&states[i], group.ID)
		ref.ExpectedGroup = cloneGroup(group)

		return ref
	}

	return GenerationRef{GroupID: group.ID, ExpectedGroup: cloneGroup(group)}
}

func compensationGenerations(states []memberState) []GenerationRef {
	seen := make(map[string]bool)
	refs := make([]GenerationRef, 0, len(states))

	for i := range states {
		groupID := states[i].mutationGroupID
		if groupID == "" || seen[groupID] {
			continue
		}

		seen[groupID] = true
		ref := generationRef(states, states[i].mutationGroup)
		refs = append(refs, ref)
	}

	return refs
}

func generationRefForState(state *memberState, groupID string) GenerationRef {
	ref := GenerationRef{GroupID: groupID, DeviceID: state.result.DeviceID}
	if state.info == nil {
		return ref
	}

	ref.DeviceID = state.info.DeviceID
	ref.AccountID = state.info.MargeAccountUUID
	ref.MargeURL = state.info.MargeURL

	return ref
}

func createCandidateGenerationRefs(states []memberState) []GenerationRef {
	refs := make([]GenerationRef, 0, len(states))
	for i := range states {
		refs = append(refs, generationRefForState(&states[i], ""))
	}

	return refs
}

func (c *Coordinator) preflightCreate(state *memberState) {
	if net.ParseIP(state.result.IPAddress) == nil {
		state.result.PreflightError = fmt.Errorf("%w: invalid speaker IP address", ErrInvalidRequest)
		return
	}

	client, err := c.factory(state.result.IPAddress)
	if err != nil {
		state.result.PreflightError = wrapUnavailable("create client", err)
		return
	}

	state.client = client

	state.info, err = client.GetDeviceInfo()
	if err != nil {
		state.result.PreflightError = wrapUnavailable("get device info", err)
		return
	}

	state.result.Reachable = true
	if state.info == nil {
		state.result.PreflightError = wrapUnavailable("get device info", errors.New("nil response"))
		return
	}

	state.result.DeviceID = state.info.DeviceID
	if state.info.DeviceID == "" {
		state.result.PreflightError = fmt.Errorf("%w: device info has no device ID", ErrConflict)
		return
	}

	capabilities, err := client.GetCapabilities()
	if err != nil {
		state.result.PreflightError = wrapUnavailable("get capabilities", err)
		return
	}

	if !isST10(state.info) || capabilities == nil || !capabilities.HasLRStereoCapability() {
		state.result.PreflightError = fmt.Errorf("%w: speaker is not an ST10 with L/R stereo capability", ErrConflict)
		return
	}

	if capabilities.DeviceID != "" && capabilities.DeviceID != state.info.DeviceID {
		state.result.PreflightError = fmt.Errorf("%w: capabilities do not match the physical speaker", ErrConflict)
		return
	}

	zone, err := client.GetZone()
	if err != nil {
		state.result.PreflightError = wrapUnavailable("get zone", err)
		return
	}

	if zone == nil {
		state.result.PreflightError = wrapUnavailable("get zone", errors.New("nil response"))
		return
	}

	state.zone = cloneZone(zone)

	state.group, err = client.GetGroup()
	if err != nil {
		state.result.PreflightError = wrapUnavailable("get group", err)
		return
	}

	if state.group == nil || !state.group.IsEmpty() {
		state.result.PreflightError = fmt.Errorf("%w: speaker is currently grouped", ErrConflict)
	}
}

func (c *Coordinator) preflightStandaloneZone(state *memberState) {
	zone, err := state.client.GetZone()
	if err != nil {
		setPreflightError(state, wrapUnavailable("get zone", err))
		return
	}

	state.result.Reachable = true
	if zone == nil {
		setPreflightError(state, wrapUnavailable("get zone", errors.New("nil response")))
	} else if !zone.IsStandalone() {
		setPreflightError(state, fmt.Errorf("%w: speaker is currently in a zone", ErrConflict))
	}
}

func (c *Coordinator) loadPair(memberIPAddress, expectedGroupID string, allowEmptyMembers bool) ([]memberState, *models.Group, error) {
	if c.factory == nil {
		return nil, nil, fmt.Errorf("%w: client factory is nil", ErrUnavailable)
	}

	entry, err := c.factory(memberIPAddress)
	if err != nil {
		return nil, nil, wrapUnavailable("create client", err)
	}

	current, err := entry.GetGroup()
	if err != nil {
		return nil, nil, wrapUnavailable("get current group", err)
	}

	if current == nil {
		return nil, nil, wrapUnavailable("get current group", errors.New("nil response"))
	}

	if current.IsEmpty() {
		return nil, nil, fmt.Errorf("%w: %w", ErrConflict, errNoConfiguredPair)
	}

	if current.ID != expectedGroupID {
		return nil, nil, fmt.Errorf("%w: expected group %q, found %q", ErrConflict, expectedGroupID, current.ID)
	}

	if verifyErr := verifyConfiguredGroup(current); verifyErr != nil {
		return nil, nil, fmt.Errorf("%w: invalid current group: %w", ErrConflict, verifyErr)
	}

	members, err := pairMembers(current)
	if err != nil {
		return nil, nil, err
	}

	entryInfo, err := entry.GetDeviceInfo()
	if err != nil {
		return nil, nil, wrapUnavailable("get initiating device info", err)
	}

	if entryInfo == nil || !groupContainsDevice(current, entryInfo.DeviceID) {
		return nil, nil, fmt.Errorf("%w: initiating endpoint is not a member of the expected pair", ErrConflict)
	}

	states := make([]memberState, len(members))
	for i, member := range members {
		c.preflightExistingMember(&states[i], member, current, allowEmptyMembers)
	}

	validateCreateCandidates(states)

	if hasPreflightError(states) {
		return states, nil, errors.New("pair preflight failed")
	}

	return states, current, nil
}

// loadPairFromSnapshot recovers only an exact generation supplied by a caller
// that observed it before an interrupted dissolve. Every identity, capability,
// zone, and group state is read again from both physical role addresses.
func (c *Coordinator) loadPairFromSnapshot(
	memberIPAddress string,
	expectedGroupID string,
	snapshot *models.Group,
) ([]memberState, *models.Group, error) {
	current := cloneGroup(snapshot)
	if current == nil || current.ID != expectedGroupID {
		return nil, nil, fmt.Errorf("%w: recovery snapshot does not match expected group", ErrConflict)
	}

	if err := verifyConfiguredGroup(current); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid recovery snapshot: %w", ErrConflict, err)
	}

	members, err := pairMembers(current)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: invalid recovery snapshot: %w", ErrConflict, err)
	}

	entry, err := c.factory(memberIPAddress)
	if err != nil {
		return nil, nil, wrapUnavailable("create recovery client", err)
	}

	entryInfo, err := entry.GetDeviceInfo()
	if err != nil {
		return nil, nil, wrapUnavailable("get initiating device info", err)
	}

	if entryInfo == nil || !groupContainsDevice(current, entryInfo.DeviceID) {
		return nil, nil, fmt.Errorf("%w: initiating endpoint is not a member of the recovery snapshot", ErrConflict)
	}

	states := make([]memberState, len(members))
	for i, member := range members {
		c.preflightExistingMember(&states[i], member, current, true)
	}

	validateCreateCandidates(states)

	if hasPreflightError(states) {
		return states, nil, errors.New("recovery snapshot preflight failed")
	}

	return states, current, nil
}

func (c *Coordinator) preflightExistingMember(
	state *memberState,
	member models.GroupRole,
	current *models.Group,
	allowEmpty bool,
) {
	state.result.IPAddress = member.IPAddress
	state.result.DeviceID = member.DeviceID

	client, err := c.factory(member.IPAddress)
	if err != nil {
		state.result.PreflightError = wrapUnavailable("create client", err)

		return
	}

	state.client = client

	state.info, err = client.GetDeviceInfo()
	if err != nil {
		state.result.PreflightError = wrapUnavailable("get device info", err)

		return
	}

	state.result.Reachable = true
	if state.info == nil || state.info.DeviceID != member.DeviceID {
		state.result.PreflightError = fmt.Errorf("%w: group role does not match the physical speaker", ErrConflict)

		return
	}

	capabilities, err := client.GetCapabilities()
	if err != nil {
		state.result.PreflightError = wrapUnavailable("get capabilities", err)

		return
	}

	if !isST10(state.info) || capabilities == nil || !capabilities.HasLRStereoCapability() {
		state.result.PreflightError = fmt.Errorf("%w: speaker is not an ST10 with L/R stereo capability", ErrConflict)

		return
	}

	if capabilities.DeviceID != "" && capabilities.DeviceID != state.info.DeviceID {
		state.result.PreflightError = fmt.Errorf("%w: capabilities do not match the physical speaker", ErrConflict)

		return
	}

	state.group, err = client.GetGroup()
	switch {
	case err != nil:
		state.result.PreflightError = wrapUnavailable("get current group", err)
	case state.group == nil:
		state.result.PreflightError = wrapUnavailable("get current group", errors.New("nil response"))
	case state.group.IsEmpty() && !allowEmpty:
		state.result.PreflightError = fmt.Errorf("%w: member is no longer in the expected group", ErrConflict)
	case !state.group.IsEmpty() && !sameGroupTopology(state.group, current):
		state.result.PreflightError = fmt.Errorf("%w: members do not agree on the current group", ErrConflict)
	}

	c.preflightStandaloneZone(state)
}

func verifyPair(states []memberState, verify func(*models.Group) error) {
	for i := range states {
		states[i].result.Verified = false
		states[i].result.VerificationError = nil

		group, err := states[i].client.GetGroup()
		states[i].group = group

		states[i].result.Group = cloneGroup(group)
		if err != nil {
			states[i].result.VerificationError = fmt.Errorf("get group after mutation: %w", err)
			continue
		}

		if err := verify(group); err != nil {
			states[i].result.VerificationError = err
			continue
		}

		states[i].result.Verified = true
	}

	if len(states) == 2 && states[0].result.Verified && states[1].result.Verified &&
		!sameGroupConfiguration(states[0].group, states[1].group) {
		err := errors.New("members do not agree on the resulting group")
		states[0].result.Verified = false
		states[1].result.Verified = false
		states[0].result.VerificationError = err
		states[1].result.VerificationError = err
	}
}

func verifyCreatedPairZones(states []memberState) bool {
	verified := true

	for i := range states {
		zone, err := states[i].client.GetZone()

		states[i].zone = cloneZone(zone)
		if err != nil {
			states[i].result.VerificationError = fmt.Errorf("get zone after verified group mutation: %w", err)
			verified = false

			continue
		}

		_, standalone, err := parseCreateZone(zone, states[i].result.DeviceID)
		if err != nil {
			states[i].result.VerificationError = fmt.Errorf("verify zone after group mutation: %w", err)
			verified = false

			continue
		}

		if !standalone {
			states[i].result.VerificationError = errors.New("verify zone after group mutation: speaker remains in a temporary zone")
			verified = false
		}
	}

	return verified
}

func hasUncertainMutationOutcome(states []memberState) bool {
	for i := range states {
		if isUncertainMutationOutcome(states[i].result.MutationError) {
			return true
		}
	}

	return false
}

func isUncertainMutationOutcome(err error) bool {
	if errors.Is(err, errUncertainMutation) {
		return true
	}

	var timeout interface{ Timeout() bool }

	return errors.As(err, &timeout) && timeout.Timeout()
}

func captureVerifiedGenerationIDs(states []memberState, expected *models.Group) {
	for i := range states {
		if !states[i].result.Verified || !matchesRequestedPair(states[i].group, expected) {
			continue
		}

		if states[i].mutationGroupID == "" {
			states[i].mutationGroupID = states[i].group.ID
		}

		if states[i].group.ID == states[i].mutationGroupID {
			states[i].mutationGroup = cloneGroup(states[i].group)
		}
	}
}

func verifyCreatedGroup(group, expected *models.Group) error {
	if err := verifyConfiguredGroup(group); err != nil {
		return err
	}

	if !matchesRequestedPair(group, expected) {
		return errors.New("group does not match the requested pair")
	}

	return nil
}

func validateMutationResponse(operation string, response *models.Group, err error) error {
	if err != nil {
		return wrapUnavailable(operation, err)
	}

	if response == nil {
		return wrapUnavailable(operation, errors.New("nil response"))
	}

	if response.Status != "" && response.Status != "GROUP_OK" {
		return fmt.Errorf("%w: %s returned status %q", ErrConflict, operation, response.Status)
	}

	if response.ID == "" {
		return fmt.Errorf("%w: %s response has no group ID: %w",
			ErrUnavailable, operation, errUncertainMutation)
	}

	return nil
}

func verifyConfiguredGroup(group *models.Group) error {
	if group == nil || group.IsEmpty() {
		return errors.New("group is empty")
	}

	if group.ID == "" {
		return errors.New("group has no ID")
	}

	if group.Status != "" && group.Status != "GROUP_OK" {
		return fmt.Errorf("group status is %q", group.Status)
	}

	_, err := pairMembers(group)

	return err
}

func pairMembers(group *models.Group) ([]models.GroupRole, error) {
	if group == nil || len(group.Roles.Roles) != 2 {
		return nil, errors.New("stereo pair must contain exactly two roles")
	}

	roles := make(map[string]models.GroupRole, 2)

	for _, role := range group.Roles.Roles {
		if role.DeviceID == "" || net.ParseIP(role.IPAddress) == nil {
			return nil, errors.New("stereo-pair roles require device IDs and IP addresses")
		}

		if role.Role != "LEFT" && role.Role != "RIGHT" {
			return nil, fmt.Errorf("invalid stereo-pair role %q", role.Role)
		}

		if _, exists := roles[role.Role]; exists {
			return nil, fmt.Errorf("duplicate stereo-pair role %q", role.Role)
		}

		roles[role.Role] = role
	}

	left, leftOK := roles["LEFT"]

	right, rightOK := roles["RIGHT"]

	if !leftOK || !rightOK || left.DeviceID == right.DeviceID || sameIP(left.IPAddress, right.IPAddress) {
		return nil, errors.New("stereo pair requires distinct LEFT and RIGHT members")
	}

	if group.MasterDeviceID != left.DeviceID && group.MasterDeviceID != right.DeviceID {
		return nil, errors.New("group master must be a stereo-pair member")
	}

	return []models.GroupRole{left, right}, nil
}

func matchesRequestedPair(group, expected *models.Group) bool {
	if group == nil || expected == nil || group.Name != expected.Name || group.MasterDeviceID != expected.MasterDeviceID {
		return false
	}

	return sameRoles(group.Roles.Roles, expected.Roles.Roles)
}

func matchesRequestedGeneration(group, expected *models.Group, groupID string) bool {
	return groupID != "" && group != nil && group.ID == groupID && matchesRequestedPair(group, expected)
}

func sameGroupConfiguration(a, b *models.Group) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.ID == b.ID && a.Name == b.Name && a.MasterDeviceID == b.MasterDeviceID && sameRoles(a.Roles.Roles, b.Roles.Roles)
}

func sameGroupTopology(a, b *models.Group) bool {
	return a != nil && b != nil && a.ID == b.ID && a.MasterDeviceID == b.MasterDeviceID &&
		sameRoles(a.Roles.Roles, b.Roles.Roles)
}

// sameRoles delegates to models.SameGroupRoles, the shared topology-equality
// core also used by pkg/service/datastore -- see i655 code-review finding
// #10 (three independent, subtly different implementations used to coexist).
func sameRoles(a, b []models.GroupRole) bool {
	return models.SameGroupRoles(a, b)
}

func groupContainsDevice(group *models.Group, deviceID string) bool {
	if group == nil || deviceID == "" {
		return false
	}

	for i := range group.Roles.Roles {
		if group.Roles.Roles[i].DeviceID == deviceID {
			return true
		}
	}

	return false
}

func cloneGroup(group *models.Group) *models.Group {
	if group == nil {
		return nil
	}

	clone := *group
	clone.Roles.Roles = append([]models.GroupRole(nil), group.Roles.Roles...)

	return &clone
}

func cloneZone(zone *models.ZoneInfo) *models.ZoneInfo {
	if zone == nil {
		return nil
	}

	clone := *zone
	clone.Members = append([]models.Member(nil), zone.Members...)

	return &clone
}

func isST10(info *models.DeviceInfo) bool {
	typeName := strings.TrimSpace(strings.ToLower(info.Type))
	return typeName == "st10" || typeName == "soundtouch 10"
}

func sameIP(a, b string) bool {
	left, right := net.ParseIP(a), net.ParseIP(b)
	return left != nil && right != nil && left.Equal(right)
}

// SameMargeBackend reports whether two Marge base URLs resolve to the same
// normalized streaming endpoint.
func SameMargeBackend(a, b string) bool {
	left, leftErr := margeStreamingURL(a)

	right, rightErr := margeStreamingURL(b)
	if leftErr != nil || rightErr != nil {
		return false
	}

	leftURL, leftErr := url.Parse(left)

	rightURL, rightErr := url.Parse(right)
	if leftErr != nil || rightErr != nil {
		return false
	}

	return strings.EqualFold(leftURL.Scheme, rightURL.Scheme) &&
		strings.EqualFold(leftURL.Hostname(), rightURL.Hostname()) &&
		effectiveURLPort(leftURL) == effectiveURLPort(rightURL) &&
		leftURL.EscapedPath() == rightURL.EscapedPath()
}

func effectiveURLPort(endpoint *url.URL) string {
	if port := endpoint.Port(); port != "" {
		return port
	}

	switch strings.ToLower(endpoint.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func setPreflightError(state *memberState, err error) {
	if state.result.PreflightError == nil {
		state.result.PreflightError = err
	}
}

func wrapUnavailable(operation string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%w: %s: %w", ErrUnavailable, operation, err)
}

func hasPreflightError(states []memberState) bool {
	for i := range states {
		if states[i].result.PreflightError != nil {
			return true
		}
	}

	return false
}

func allVerified(states []memberState) bool {
	if len(states) == 0 {
		return false
	}

	for i := range states {
		if !states[i].result.Verified {
			return false
		}
	}

	return true
}

func pairVerified(states []memberState) bool {
	return len(states) == 2 && allVerified(states)
}

func finish(result Result, states []memberState) (Result, error) {
	result.Members = make([]MemberResult, len(states))
	for i := range states {
		result.Members[i] = states[i].result
	}

	if result.Status == StatusSucceeded {
		return result, nil
	}

	return result, &Error{Operation: result.Operation, Status: result.Status}
}
