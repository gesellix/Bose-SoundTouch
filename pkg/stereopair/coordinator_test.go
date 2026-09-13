package stereopair

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

const (
	leftIP  = "192.0.2.10"
	rightIP = "192.0.2.11"
	thirdIP = "192.0.2.12"
	leftID  = "LEFT-ID"
	rightID = "RIGHT-ID"
)

type fakeClient struct {
	mu sync.Mutex

	info         *models.DeviceInfo
	capabilities *models.Capabilities
	zone         *models.ZoneInfo
	group        *models.Group

	infoErr       error
	capabilityErr error
	zoneErr       error
	getGroupErr   error
	addErr        error
	updateErr     error
	removeErr     error

	addRequest    *models.Group
	updateRequest *models.Group
	removeCalls   int
	zoneCalls     int
	getGroupCalls int
	addGroup      func(*models.Group) *models.Group
	addResponse   func(*models.Group) *models.Group
	getZone       func(int, *models.ZoneInfo) (*models.ZoneInfo, error)
	getGroup      func(int, *models.Group) *models.Group
}

type fakeTimeoutError struct{}

func (fakeTimeoutError) Error() string { return "timeout" }

func (fakeTimeoutError) Timeout() bool { return true }

func (f *fakeClient) GetDeviceInfo() (*models.DeviceInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	result := *f.info
	return &result, nil
}

func (f *fakeClient) GetCapabilities() (*models.Capabilities, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.capabilityErr != nil {
		return nil, f.capabilityErr
	}
	result := *f.capabilities
	return &result, nil
}

func (f *fakeClient) GetZone() (*models.ZoneInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.zoneCalls++
	if f.getZone != nil {
		f.zone, f.zoneErr = f.getZone(f.zoneCalls, f.zone)
	}
	if f.zoneErr != nil {
		return nil, f.zoneErr
	}
	if f.zone == nil {
		return nil, errors.New("fake zone response is nil")
	}
	result := *f.zone
	result.Members = append([]models.Member(nil), f.zone.Members...)
	return &result, nil
}

func (f *fakeClient) GetGroup() (*models.Group, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getGroupErr != nil {
		return nil, f.getGroupErr
	}
	f.getGroupCalls++
	if f.getGroup != nil {
		f.group = f.getGroup(f.getGroupCalls, f.group)
	}

	return cloneGroup(f.group), nil
}

func (f *fakeClient) AddGroup(group *models.Group) (*models.Group, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addRequest = cloneGroup(group)
	if f.addErr != nil {
		if f.addGroup != nil {
			f.group = f.addGroup(group)
		}

		return nil, f.addErr
	}
	if f.addGroup != nil {
		f.group = f.addGroup(group)
	} else {
		f.group = configuredGroup(group.Name)
	}
	if f.addResponse != nil {
		return cloneGroup(f.addResponse(group)), nil
	}
	return cloneGroup(f.group), nil
}

func (f *fakeClient) UpdateGroup(group *models.Group) (*models.Group, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateRequest = cloneGroup(group)
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.group = cloneGroup(group)
	f.group.Status = "GROUP_OK"
	return cloneGroup(f.group), nil
}

func (f *fakeClient) RemoveGroup() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeCalls++
	if f.removeErr != nil {
		return f.removeErr
	}
	f.group = &models.Group{}
	return nil
}

func TestCreateSuccessPreservesAsymmetricPayloadAndVerifies(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Living Room Pair",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if result.Status != StatusSucceeded || len(result.Members) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if left.addRequest.SenderIPAddress != "" {
		t.Errorf("LEFT senderIPAddress = %q, want empty", left.addRequest.SenderIPAddress)
	}
	if right.addRequest.SenderIPAddress != leftIP {
		t.Errorf("RIGHT senderIPAddress = %q, want %q", right.addRequest.SenderIPAddress, leftIP)
	}
	if right.addRequest.ID != "PAIR-ID" {
		t.Errorf("RIGHT group ID = %q, want master-assigned PAIR-ID", right.addRequest.ID)
	}
	for _, request := range []*models.Group{left.addRequest, right.addRequest} {
		if request.MasterDeviceID != leftID || len(request.Roles.Roles) != 2 {
			t.Errorf("incomplete addGroup request: %+v", request)
		}
	}
	for _, member := range result.Members {
		if !member.Verified || member.Group == nil || member.Group.ID != "PAIR-ID" {
			t.Errorf("member not verified from fresh group state: %+v", member)
		}
	}
	if left.zoneCalls != 2 || right.zoneCalls != 2 {
		t.Fatalf("fresh zone reads LEFT=%d RIGHT=%d, want preflight and post-mutation reads", left.zoneCalls, right.zoneCalls)
	}
}

func TestCreateAcceptsEmptyStandaloneZonePreflight(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	left.zone = &models.ZoneInfo{}
	right.zone = &models.ZoneInfo{}
	left.getZone = transitionToStandalone(leftID)
	right.getZone = transitionToStandalone(rightID)

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.addRequest == nil || right.addRequest == nil {
		t.Fatal("empty standalone zone responses blocked pair creation")
	}
}

func TestCreateAcceptsEmptyPostMutationZoneVerification(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	left.getZone = transitionToEmptyZone()
	right.getZone = transitionToEmptyZone()

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	for _, member := range result.Members {
		if !member.Verified || member.VerificationError != nil {
			t.Fatalf("empty post-create zone was not accepted: %+v", member)
		}
	}
}

func TestCreateTransitionsConsistentTemporaryZoneToStereoPair(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	left.zone = temporaryZone(leftID, rightID)
	right.zone = temporaryZone(leftID, rightID)
	left.getZone = transitionToStandalone(leftID)
	right.getZone = transitionToStandalone(rightID)

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Living Room Pair",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.addRequest == nil || right.addRequest == nil {
		t.Fatal("same-zone candidates were not mutated")
	}
	if left.zoneCalls != 2 || right.zoneCalls != 2 {
		t.Fatalf("fresh zone reads LEFT=%d RIGHT=%d, want 2/2", left.zoneCalls, right.zoneCalls)
	}
}

func TestCreateTransitionsCandidatesFromLargerZoneWithThirdMaster(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	left.zone = temporaryZone("THIRD-ID", leftID, rightID)
	right.zone = temporaryZone("THIRD-ID", leftID, rightID)
	left.getZone = transitionToStandalone(leftID)
	right.getZone = transitionToStandalone(rightID)

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Living Room Pair",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.addRequest == nil || right.addRequest == nil {
		t.Fatal("same larger-zone candidates were not mutated")
	}
	if left.zoneCalls != 2 || right.zoneCalls != 2 {
		t.Fatalf("fresh zone reads LEFT=%d RIGHT=%d, want 2/2", left.zoneCalls, right.zoneCalls)
	}
}

func TestCreateRejectsOneSidedTemporaryZoneWithoutMutation(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	left.zone = temporaryZone(leftID, rightID)

	result, err := coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.addRequest != nil || right.addRequest != nil {
		t.Fatal("addGroup called for one-sided temporary-zone state")
	}
}

func TestCreateRejectsDifferentTemporaryZonesWithoutMutation(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	left.zone = temporaryZone(leftID, rightID)
	right.zone = temporaryZone(rightID, leftID)

	result, err := coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.addRequest != nil || right.addRequest != nil {
		t.Fatal("addGroup called for candidates reporting different temporary zones")
	}
}

func TestCreateRejectsInconsistentOrMalformedZoneViewsWithoutMutation(t *testing.T) {
	tests := []struct {
		name      string
		leftZone  *models.ZoneInfo
		rightZone *models.ZoneInfo
	}{
		{
			name:      "inconsistent membership",
			leftZone:  temporaryZone(leftID, rightID),
			rightZone: temporaryZone(leftID, rightID, "OTHER-ID"),
		},
		{
			name:      "empty master",
			leftZone:  &models.ZoneInfo{Members: []models.Member{{DeviceID: leftID}, {DeviceID: rightID}}},
			rightZone: temporaryZone(leftID, rightID),
		},
		{
			name:      "whitespace master",
			leftZone:  &models.ZoneInfo{Master: "  "},
			rightZone: &models.ZoneInfo{Master: rightID},
		},
		{
			name:      "standalone master is another device",
			leftZone:  &models.ZoneInfo{Master: "OTHER-ID"},
			rightZone: &models.ZoneInfo{Master: rightID},
		},
		{
			name: "duplicate member",
			leftZone: &models.ZoneInfo{Master: leftID, Members: []models.Member{
				{DeviceID: leftID}, {DeviceID: rightID}, {DeviceID: rightID},
			}},
			rightZone: temporaryZone(leftID, rightID),
		},
		{
			name:      "candidate absent from matching zone",
			leftZone:  temporaryZone(leftID, "OTHER-ID"),
			rightZone: temporaryZone(leftID, "OTHER-ID"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left, right, coordinator := newCreateCoordinator()
			left.zone = test.leftZone
			right.zone = test.rightZone

			result, err := coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP})
			if err == nil || result.Status != StatusFailed {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			if left.addRequest != nil || right.addRequest != nil {
				t.Fatal("addGroup called after invalid zone preflight")
			}
		})
	}
}

func TestCreateRejectsInvalidPreflightWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(left, right *fakeClient)
	}{
		{
			name: "not stereo capable",
			mutate: func(_ *fakeClient, right *fakeClient) {
				right.capabilities.LRStereo = false
			},
		},
		{
			name: "capabilities belong to another speaker",
			mutate: func(_ *fakeClient, right *fakeClient) {
				right.capabilities.DeviceID = "OTHER-ID"
			},
		},
		{
			name: "different Marge backends",
			mutate: func(_ *fakeClient, right *fakeClient) {
				right.info.MargeURL = "http://other-aftertouch.example"
			},
		},
		{
			name: "in temporary zone",
			mutate: func(_ *fakeClient, right *fakeClient) {
				right.zone.Members = []models.Member{{DeviceID: leftID, IP: leftIP}}
			},
		},
		{
			name: "already grouped",
			mutate: func(_ *fakeClient, right *fakeClient) {
				right.group = configuredGroup("Existing Pair")
			},
		},
		{
			name: "unreachable",
			mutate: func(_ *fakeClient, right *fakeClient) {
				right.infoErr = errors.New("offline")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left, right, coordinator := newCreateCoordinator()
			test.mutate(left, right)
			result, err := coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP})
			if err == nil || result.Status != StatusFailed {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			if left.addRequest != nil || right.addRequest != nil {
				t.Fatal("addGroup called after failed preflight")
			}
		})
	}
}

// TestCreateAllowsDifferingMargeAccounts guards against reintroducing an
// unjustified same-Marge-account requirement: a real, working cross-account
// stereo pair (created pre-lifecycle-API via the CLI's direct /addGroup
// calls, which never checked Marge accounts) must remain creatable through
// this API too. See i655 code-review finding #0.
func TestCreateAllowsDifferingMargeAccounts(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	right.info.MargeAccountUUID = "ACCOUNT2"

	result, err := coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair"})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v; want succeeded despite differing Marge accounts", result, err)
	}
	if left.addRequest == nil || right.addRequest == nil {
		t.Fatal("addGroup not called for a valid cross-account pair")
	}
}

func TestCreatePartialFailureIsCompensatedAndReported(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	right.addErr = errors.New("right add failed")

	result, err := coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair"})
	if err == nil {
		t.Fatal("Create unexpectedly succeeded")
	}
	if result.Status != StatusFailed || !result.CompensationAttempted || !result.CompensationComplete {
		t.Fatalf("compensation result = %+v", result)
	}
	if left.removeCalls != 1 || right.removeCalls != 0 {
		t.Fatalf("remove calls LEFT=%d RIGHT=%d, want 1/0", left.removeCalls, right.removeCalls)
	}
	if !result.Members[0].CompensationVerified || !result.Members[1].CompensationVerified {
		t.Fatalf("cleanup was not verified empty: %+v", result.Members)
	}
	if left.zoneCalls != 2 || right.zoneCalls != 2 {
		t.Fatalf("fresh compensation zone reads LEFT=%d RIGHT=%d, want 2/2", left.zoneCalls, right.zoneCalls)
	}
}

func TestCreateSameZonePartialFailureRequiresZoneTopologyRestoration(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	left.zone = temporaryZone(leftID, leftID, rightID)
	right.zone = temporaryZone(leftID, leftID, rightID)
	right.addErr = errors.New("right add failed")
	coordinator.uncertainOutcomeDelays = nil
	left.getZone = func(call int, current *models.ZoneInfo) (*models.ZoneInfo, error) {
		if call == 2 {
			return &models.ZoneInfo{Master: leftID}, nil
		}

		return current, nil
	}

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err == nil || result.Status != StatusDegraded || result.CompensationComplete {
		t.Fatalf("result = %+v, err = %v; want degraded rollback proof", result, err)
	}
	if left.removeCalls != 1 || right.removeCalls != 0 {
		t.Fatalf("remove calls LEFT=%d RIGHT=%d, want 1/0", left.removeCalls, right.removeCalls)
	}
	if result.Members[0].CompensationVerified || result.Members[0].CompensationError == nil {
		t.Fatalf("changed LEFT zone topology was accepted: %+v", result.Members[0])
	}
	if !result.Members[1].CompensationVerified {
		t.Fatalf("unchanged RIGHT zone topology was not verified: %+v", result.Members[1])
	}
	if left.zoneCalls != 2 || right.zoneCalls != 2 {
		t.Fatalf("fresh compensation zone reads LEFT=%d RIGHT=%d, want 2/2", left.zoneCalls, right.zoneCalls)
	}
}

func TestCreateCompensationRejectsUnprovenZonePostcondition(t *testing.T) {
	tests := []struct {
		name    string
		getZone func(int, *models.ZoneInfo) (*models.ZoneInfo, error)
	}{
		{
			name: "read failure",
			getZone: func(call int, current *models.ZoneInfo) (*models.ZoneInfo, error) {
				if call == 2 {
					return nil, errors.New("zone unavailable")
				}

				return current, nil
			},
		},
		{
			name: "malformed response",
			getZone: func(call int, current *models.ZoneInfo) (*models.ZoneInfo, error) {
				if call == 2 {
					return &models.ZoneInfo{Master: "  "}, nil
				}

				return current, nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left, right, coordinator := newCreateCoordinator()
			right.addErr = errors.New("right add failed")
			coordinator.uncertainOutcomeDelays = nil
			left.getZone = test.getZone

			result, err := coordinator.Create(CreateRequest{
				LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
			})
			if err == nil || result.Status != StatusDegraded || result.CompensationComplete {
				t.Fatalf("result = %+v, err = %v; want degraded rollback proof", result, err)
			}
			if result.Members[0].CompensationVerified || result.Members[0].CompensationError == nil {
				t.Fatalf("unproven LEFT zone postcondition was accepted: %+v", result.Members[0])
			}
			if !result.Members[1].CompensationVerified {
				t.Fatalf("RIGHT compensation was not verified: %+v", result.Members[1])
			}
			if left.zoneCalls != 2 || right.zoneCalls != 2 {
				t.Fatalf("fresh compensation zone reads LEFT=%d RIGHT=%d, want 2/2", left.zoneCalls, right.zoneCalls)
			}
		})
	}
}

func TestVerifyCompensationZoneAcceptsEquivalentStandaloneRepresentations(t *testing.T) {
	empty := &models.ZoneInfo{}
	explicit := &models.ZoneInfo{Master: leftID}

	if err := verifyCompensationZone(empty, explicit, leftID); err != nil {
		t.Fatalf("empty to explicit standalone rejected: %v", err)
	}
	if err := verifyCompensationZone(explicit, empty, leftID); err != nil {
		t.Fatalf("explicit to empty standalone rejected: %v", err)
	}
}

func TestCreateReverifiesUncertainTimeout(t *testing.T) {
	_, right, coordinator := newCreateCoordinator()
	right.addErr = fakeTimeoutError{}
	right.addGroup = func(group *models.Group) *models.Group {
		return configuredGroup(group.Name)
	}
	right.getGroup = func(call int, current *models.Group) *models.Group {
		if call <= 2 {
			return &models.Group{}
		}

		return configuredGroup("Pair")
	}
	coordinator.uncertainOutcomeDelays = []time.Duration{0}

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if right.getGroupCalls != 3 {
		t.Fatalf("RIGHT getGroup calls = %d, want 3", right.getGroupCalls)
	}
	if result.Members[1].Group == nil || result.Members[1].Group.ID != "PAIR-ID" {
		t.Fatalf("late physical generation was not verified: %+v", result.Members[1])
	}
}

func TestCreateReverifiesSuccessfulAddGroupResponseWithoutID(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	right.addResponse = func(*models.Group) *models.Group {
		return &models.Group{}
	}
	right.getGroup = func(call int, current *models.Group) *models.Group {
		if call <= 2 {
			return &models.Group{}
		}

		return configuredGroup("Pair")
	}
	coordinator.uncertainOutcomeDelays = []time.Duration{0}

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.getGroupCalls != 3 || right.getGroupCalls != 3 {
		t.Fatalf("getGroup calls LEFT=%d RIGHT=%d, want 3/3",
			left.getGroupCalls, right.getGroupCalls)
	}
	if right.addRequest == nil || right.addRequest.ID != "PAIR-ID" {
		t.Fatalf("RIGHT addGroup request = %+v, want master-assigned PAIR-ID", right.addRequest)
	}
	if left.removeCalls != 0 || right.removeCalls != 0 {
		t.Fatalf("remove calls LEFT=%d RIGHT=%d, want 0/0", left.removeCalls, right.removeCalls)
	}
	for _, member := range result.Members {
		if !member.Verified || member.Group == nil || member.Group.ID != "PAIR-ID" {
			t.Fatalf("late physical generation was not verified: %+v", member)
		}
	}
}

func TestCreateResolvesMasterResponseWithoutIDBeforeMutatingSlave(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	left.addResponse = func(*models.Group) *models.Group {
		return &models.Group{}
	}
	left.getGroup = func(call int, current *models.Group) *models.Group {
		if call <= 2 {
			return &models.Group{}
		}

		return configuredGroup("Pair")
	}
	coordinator.uncertainOutcomeDelays = []time.Duration{0}

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.getGroupCalls != 4 || right.getGroupCalls != 2 {
		t.Fatalf("getGroup calls LEFT=%d RIGHT=%d, want 4/2",
			left.getGroupCalls, right.getGroupCalls)
	}
	if right.addRequest == nil || right.addRequest.ID != "PAIR-ID" {
		t.Fatalf("RIGHT addGroup request = %+v, want resolved master PAIR-ID", right.addRequest)
	}
	if left.removeCalls != 0 || right.removeCalls != 0 {
		t.Fatalf("remove calls LEFT=%d RIGHT=%d, want 0/0", left.removeCalls, right.removeCalls)
	}
}

func TestCreateCompensatesGenerationObservedAfterTimeout(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	right.addErr = fakeTimeoutError{}
	right.addGroup = func(*models.Group) *models.Group {
		return &models.Group{}
	}
	coordinator.uncertainOutcomeDelays = nil

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if !result.CompensationAttempted || !result.CompensationComplete || left.removeCalls != 1 {
		t.Fatalf("verified timed-out generation was not compensated: result=%+v LEFT removes=%d",
			result, left.removeCalls)
	}
	if right.removeCalls != 0 {
		t.Fatalf("RIGHT remove calls = %d, want 0", right.removeCalls)
	}
}

func TestCreateCompensationReverifiesLateRemovalWithoutReplay(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	right.addErr = errors.New("right add failed")
	left.removeErr = fakeTimeoutError{}
	left.getGroup = func(call int, current *models.Group) *models.Group {
		if call >= 5 {
			return &models.Group{}
		}

		return current
	}
	coordinator.uncertainOutcomeDelays = []time.Duration{0}

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err == nil || result.Status != StatusFailed || !result.CompensationComplete {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.removeCalls != 1 {
		t.Fatalf("LEFT remove calls = %d, want exactly 1", left.removeCalls)
	}
	if !result.Members[0].CompensationVerified {
		t.Fatalf("late compensation was not verified: %+v", result.Members[0])
	}
}

func TestCreateCompensationRetiresExactPersistedGeneration(t *testing.T) {
	left, right, _ := newCreateCoordinator()
	right.addErr = errors.New("right add failed")

	var cleaned GenerationRef
	coordinator := NewWithGenerationCleanup(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		func(ref GenerationRef) error {
			cleaned = ref

			return nil
		})

	result, err := coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair"})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if !result.PersistenceAttempted || !result.PersistenceComplete || result.PersistenceError != nil {
		t.Fatalf("persistence result = %+v", result)
	}
	if cleaned.GroupID != "PAIR-ID" || cleaned.DeviceID != leftID || cleaned.AccountID != "ACCOUNT1" {
		t.Fatalf("cleaned generation = %+v", cleaned)
	}
}

func TestCreateMasterFailureDoesNotMutateSlave(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	left.addErr = errors.New("left add failed")

	result, err := coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair"})
	if err == nil || result.Status != StatusDegraded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if right.addRequest != nil || result.Members[1].MutationAttempted {
		t.Fatalf("slave was mutated after master failure: request=%+v member=%+v",
			right.addRequest, result.Members[1])
	}
	if left.removeCalls != 0 || right.removeCalls != 0 || result.CompensationAttempted {
		t.Fatalf("unexpected compensation after unobserved master failure: result=%+v", result)
	}
}

func TestCreateFailsClosedWhenGenerationPreflightFindsPersistedState(t *testing.T) {
	group := configuredGroup("Old Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)

	preflightCalls := 0
	coordinator := NewWithGenerationPersistence(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		func(GenerationRef) error { return errors.New("datastore unavailable") },
		func([]GenerationRef) error {
			preflightCalls++

			return errors.New("stale generation still active")
		},
	)

	dissolved, err := coordinator.Dissolve(DissolveRequest{
		MemberIPAddress: leftIP,
		ExpectedGroupID: "PAIR-ID",
	})
	if err == nil || dissolved.Status != StatusDegraded {
		t.Fatalf("dissolve result = %+v, err = %v", dissolved, err)
	}

	created, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "New Pair",
	})
	if err == nil || created.Status != StatusFailed || preflightCalls != 1 {
		t.Fatalf("create result = %+v, preflight calls = %d, err = %v", created, preflightCalls, err)
	}
	if !errors.Is(created.PersistenceError, ErrUnavailable) {
		t.Fatalf("persistence error = %v, want ErrUnavailable", created.PersistenceError)
	}
	if left.addRequest != nil || right.addRequest != nil {
		t.Fatal("addGroup called while an earlier persisted generation remained active")
	}
}

func TestCreateRevalidatesPhysicalStateAfterGenerationPreflight(t *testing.T) {
	left, right, _ := newCreateCoordinator()
	coordinator := NewWithGenerationPersistence(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		nil,
		func([]GenerationRef) error {
			right.mu.Lock()
			right.group = configuredGroup("Concurrent Pair")
			right.mu.Unlock()

			return nil
		},
	)

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.addRequest != nil || right.addRequest != nil {
		t.Fatal("addGroup called after physical state changed during persistence preflight")
	}
}

func TestCreateRevalidatesZoneTopologyAfterGenerationPreflight(t *testing.T) {
	left, right, _ := newCreateCoordinator()
	left.zone = temporaryZone(leftID, rightID)
	right.zone = temporaryZone(leftID, rightID)
	coordinator := NewWithGenerationPersistence(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		nil,
		func([]GenerationRef) error {
			right.mu.Lock()
			right.zone = &models.ZoneInfo{Master: rightID}
			right.mu.Unlock()

			return nil
		},
	)

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.zoneCalls != 2 || right.zoneCalls != 2 {
		t.Fatalf("fresh zone reads LEFT=%d RIGHT=%d, want initial and post-barrier reads", left.zoneCalls, right.zoneCalls)
	}
	if left.addRequest != nil || right.addRequest != nil {
		t.Fatal("addGroup called after zone topology changed during persistence preflight")
	}
}

func TestCreatePreservesVerifiedPairWhenPostMutationZoneReadbackDiffers(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	right.getZone = func(call int, current *models.ZoneInfo) (*models.ZoneInfo, error) {
		if call == 2 {
			return temporaryZone(leftID, rightID), nil
		}

		return current, nil
	}

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err == nil || result.Status != StatusDegraded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if result.Group == nil || result.Group.ID != "PAIR-ID" {
		t.Fatalf("verified pair was not preserved: %+v", result.Group)
	}
	if !result.Members[0].Verified || !result.Members[1].Verified ||
		result.Members[1].VerificationError == nil {
		t.Fatalf("zone proof failure not reported alongside verified group: %+v", result.Members)
	}
	if result.CompensationAttempted || left.removeCalls != 0 || right.removeCalls != 0 {
		t.Fatalf("verified pair was compensated after zone-only mismatch: %+v", result)
	}
}

func TestCreatePostconditionMismatchIsDegraded(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	right.addGroup = func(group *models.Group) *models.Group {
		mismatch := configuredGroup(group.Name)
		mismatch.Name = "Unrelated Pair"
		return mismatch
	}

	result, err := coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair"})
	if err == nil {
		t.Fatal("Create unexpectedly succeeded")
	}
	if result.Status != StatusDegraded || !result.CompensationAttempted || result.CompensationComplete {
		t.Fatalf("result = %+v", result)
	}
	if left.removeCalls != 1 || right.removeCalls != 0 {
		t.Fatalf("remove calls LEFT=%d RIGHT=%d, want only exact requested state removed", left.removeCalls, right.removeCalls)
	}
}

func TestCreateCompensationDoesNotRemoveDifferentGeneration(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	right.addErr = errors.New("right add failed")
	left.getGroup = func(call int, current *models.Group) *models.Group {
		if call != 2 || current == nil || current.IsEmpty() {
			return current
		}

		replacement := cloneGroup(current)
		replacement.ID = "NEWER-PAIR-ID"

		return replacement
	}

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err == nil || result.Status != StatusDegraded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if result.CompensationAttempted || left.removeCalls != 0 {
		t.Fatalf("different generation was selected for compensation: result=%+v removeCalls=%d",
			result, left.removeCalls)
	}
}

func TestCreateCompensationRechecksGenerationImmediatelyBeforeRemoval(t *testing.T) {
	left, right, coordinator := newCreateCoordinator()
	right.addErr = errors.New("right add failed")
	left.getGroup = func(call int, current *models.Group) *models.Group {
		if call != 3 || current == nil || current.IsEmpty() {
			return current
		}

		replacement := cloneGroup(current)
		replacement.ID = "NEWER-PAIR-ID"

		return replacement
	}

	result, err := coordinator.Create(CreateRequest{
		LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "Pair",
	})
	if err == nil || result.Status != StatusDegraded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if result.CompensationAttempted || left.removeCalls != 0 {
		t.Fatalf("replacement generation was removed: result=%+v removeCalls=%d", result, left.removeCalls)
	}
	if !errors.Is(result.Members[0].CompensationError, ErrConflict) {
		t.Fatalf("compensation error = %v, want ErrConflict", result.Members[0].CompensationError)
	}
}

func TestInspectReturnsCanonicalFreshPair(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Inspect(leftIP)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.Status != StatusSucceeded || result.Group == nil || result.Group.ID != "PAIR-ID" {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Members) != 2 || !result.Members[0].Verified || !result.Members[1].Verified {
		t.Fatalf("member reads were not verified: %+v", result.Members)
	}
}

func TestInspectAcceptsRightMemberAsGroupMaster(t *testing.T) {
	group := configuredGroup("Pair")
	group.MasterDeviceID = rightID
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Inspect(rightIP)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if result.Status != StatusSucceeded || result.Group == nil || result.Group.MasterDeviceID != rightID {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Members) != 2 || !result.Members[0].Verified || !result.Members[1].Verified {
		t.Fatalf("member reads were not verified: %+v", result.Members)
	}
}

func TestInspectRejectsMalformedOrNonMemberGroupMaster(t *testing.T) {
	tests := []struct {
		name     string
		masterID string
	}{
		{name: "empty", masterID: ""},
		{name: "non-member", masterID: "OTHER-ID"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			group := configuredGroup("Pair")
			group.MasterDeviceID = test.masterID
			left := readyClient(leftID, "Left")
			right := readyClient(rightID, "Right")
			left.group = cloneGroup(group)
			right.group = cloneGroup(group)
			coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

			result, err := coordinator.Inspect(leftIP)
			if err == nil || result.Status != StatusFailed {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			if len(result.Members) != 1 || !errors.Is(result.Members[0].PreflightError, ErrConflict) {
				t.Fatalf("invalid master was not rejected as a conflict: %+v", result.Members)
			}
		})
	}
}

func TestInspectPreservesGenerationForPartialDissolveRecovery(t *testing.T) {
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = configuredGroup("Pair")
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Inspect(leftIP)
	if err == nil || result.Status != StatusDegraded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if result.Group == nil || result.Group.ID != "PAIR-ID" {
		t.Fatalf("degraded inspection lost recovery generation: %+v", result)
	}
}

func TestRenameUpdatesFullGroupOnBothAndVerifiesAgreement(t *testing.T) {
	group := configuredGroup("Old Name")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP,
		ExpectedGroupID: "PAIR-ID",
		Name:            "New Name",
	})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if result.Status != StatusSucceeded {
		t.Fatalf("result = %+v", result)
	}
	for _, client := range []*fakeClient{left, right} {
		if client.updateRequest == nil || client.updateRequest.ID != "PAIR-ID" || client.updateRequest.Name != "New Name" || len(client.updateRequest.Roles.Roles) != 2 {
			t.Errorf("full group was not updated: %+v", client.updateRequest)
		}
		if client.updateRequest.Status != "" {
			t.Errorf("read-only status sent in update: %q", client.updateRequest.Status)
		}
	}
}

func TestRenamePersistsVerifiedGeneration(t *testing.T) {
	group := configuredGroup("Old Name")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	var persistedRef GenerationRef
	var persistedName string
	coordinator := NewWithGenerationLifecyclePersistence(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		nil,
		nil,
		func(ref GenerationRef, name string) error {
			persistedRef = ref
			persistedName = name
			return nil
		},
	)

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID", Name: "New Name",
	})
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if result.Status != StatusSucceeded || !result.PersistenceAttempted || !result.PersistenceComplete {
		t.Fatalf("result = %+v", result)
	}
	if persistedRef.GroupID != "PAIR-ID" || persistedRef.DeviceID != leftID ||
		persistedRef.ExpectedGroup == nil || persistedRef.ExpectedGroup.Name != "Old Name" || persistedName != "New Name" {
		t.Fatalf("persisted ref = %+v, name = %q", persistedRef, persistedName)
	}
}

func TestRenameRejectsSplitMargeBackendWithoutMutation(t *testing.T) {
	group := configuredGroup("Old Name")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	right.info.MargeURL = "http://other-aftertouch.example"
	persistenceCalls := 0
	coordinator := NewWithGenerationLifecyclePersistence(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		nil,
		nil,
		func(_ GenerationRef, _ string) error {
			persistenceCalls++
			return nil
		},
	)

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID", Name: "New Name",
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v; want failed ownership preflight", result, err)
	}
	if left.updateRequest != nil || right.updateRequest != nil || persistenceCalls != 0 {
		t.Fatalf("ownership mismatch mutated pair: left=%+v right=%+v persistence=%d",
			left.updateRequest, right.updateRequest, persistenceCalls)
	}
	if len(result.Members) != 2 || !errors.Is(result.Members[1].PreflightError, ErrConflict) {
		t.Fatalf("members = %+v; want peer ownership conflict", result.Members)
	}
}

// TestRenameAllowsDifferingMargeAccounts guards against reintroducing an
// unjustified same-Marge-account requirement into the shared loadPair
// preflight used by Rename/Dissolve. See i655 code-review finding #0.
func TestRenameAllowsDifferingMargeAccounts(t *testing.T) {
	group := configuredGroup("Old Name")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	right.info.MargeAccountUUID = "OTHER-ACCOUNT"
	coordinator := NewWithGenerationLifecyclePersistence(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		nil,
		nil,
		func(_ GenerationRef, _ string) error { return nil },
	)

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID", Name: "New Name",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v; want succeeded despite differing Marge accounts", result, err)
	}
	if left.updateRequest == nil || right.updateRequest == nil {
		t.Fatal("updateGroup not called for a valid cross-account pair")
	}
}

func TestRenamePersistenceFailureIsDegradedAndRetryable(t *testing.T) {
	group := configuredGroup("Old Name")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	persistenceCalls := 0
	coordinator := NewWithGenerationLifecyclePersistence(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		nil,
		nil,
		func(_ GenerationRef, _ string) error {
			persistenceCalls++
			if persistenceCalls == 1 {
				return errors.New("backend unavailable")
			}
			return nil
		},
	)

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID", Name: "New Name",
	})
	if err == nil || result.Status != StatusDegraded || !result.PersistenceAttempted ||
		result.PersistenceComplete || result.PersistenceError == nil {
		t.Fatalf("first result = %+v, err = %v", result, err)
	}

	result, err = coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID", Name: "New Name",
	})
	if err != nil || result.Status != StatusSucceeded || !result.PersistenceComplete || persistenceCalls != 2 {
		t.Fatalf("retry result = %+v, err = %v, calls = %d", result, err, persistenceCalls)
	}
}

func TestRenameDetectsPhysicalChangeDuringPersistence(t *testing.T) {
	group := configuredGroup("Old Name")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := NewWithGenerationLifecyclePersistence(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		nil,
		nil,
		func(_ GenerationRef, _ string) error {
			left.group = configuredGroup("Concurrent Name")
			right.group = configuredGroup("Concurrent Name")
			return nil
		},
	)

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID", Name: "New Name",
	})
	if err == nil || result.Status != StatusDegraded || !result.PersistenceComplete || result.Group != nil {
		t.Fatalf("result = %+v, err = %v; want degraded physical divergence", result, err)
	}
	for _, member := range result.Members {
		if member.Verified || member.VerificationError == nil {
			t.Fatalf("member = %+v; want fresh verification failure", member)
		}
	}
}

func TestRenamePartialFailureIsDegradedAndRetryable(t *testing.T) {
	group := configuredGroup("Old Name")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	right.updateErr = errors.New("update failed")
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: rightIP,
		ExpectedGroupID: "PAIR-ID",
		Name:            "New Name",
	})
	if err == nil || result.Status != StatusDegraded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if result.Members[1].Verified || result.Members[1].VerificationError == nil {
		t.Fatalf("RIGHT mismatch not reported: %+v", result.Members[1])
	}

	right.updateErr = nil
	result, err = coordinator.Rename(RenameRequest{
		MemberIPAddress: rightIP,
		ExpectedGroupID: "PAIR-ID",
		Name:            "New Name",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("retry result = %+v, err = %v", result, err)
	}
	if left.group.Name != "New Name" || right.group.Name != "New Name" {
		t.Fatalf("retry did not converge names: LEFT=%q RIGHT=%q", left.group.Name, right.group.Name)
	}
}

func TestRenameRejectsActiveZoneOnEitherMember(t *testing.T) {
	group := configuredGroup("Old Name")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	right.zone.Members = []models.Member{{DeviceID: leftID, IP: leftIP}}
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP,
		ExpectedGroupID: "PAIR-ID",
		Name:            "New Name",
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.zoneCalls != 1 || right.zoneCalls != 1 {
		t.Fatalf("fresh zone reads LEFT=%d RIGHT=%d, want 1/1", left.zoneCalls, right.zoneCalls)
	}
	if left.updateRequest != nil || right.updateRequest != nil {
		t.Fatal("updateGroup called despite active-zone preflight rejection")
	}
}

func TestRenameRejectsStaleGroupIDWithoutMutation(t *testing.T) {
	group := configuredGroup("Current Name")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP,
		ExpectedGroupID: "OLDER-PAIR-ID",
		Name:            "Stale Rename",
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.updateRequest != nil || right.updateRequest != nil {
		t.Fatal("updateGroup called for a stale group generation")
	}
}

func TestRenameRejectsMismatchedPhysicalDeviceIdentity(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient("DIFFERENT-ID", "Replacement")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: leftIP,
		ExpectedGroupID: "PAIR-ID",
		Name:            "New Name",
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.updateRequest != nil || right.updateRequest != nil {
		t.Fatal("updateGroup called after physical identity mismatch")
	}
}

func TestRenameRejectsNonMemberInitiatingEndpoint(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	third := readyClient("THIRD-ID", "Unrelated")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	third.group = cloneGroup(group)
	coordinator := New(factoryFor(map[string]*fakeClient{
		leftIP: left, rightIP: right, thirdIP: third,
	}))

	result, err := coordinator.Rename(RenameRequest{
		MemberIPAddress: thirdIP,
		ExpectedGroupID: "PAIR-ID",
		Name:            "Unauthorized Rename",
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.updateRequest != nil || right.updateRequest != nil || third.updateRequest != nil {
		t.Fatal("updateGroup called for a non-member initiating endpoint")
	}
}

func TestDissolveRemovesAndVerifiesEveryMember(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Dissolve(DissolveRequest{MemberIPAddress: rightIP, ExpectedGroupID: "PAIR-ID"})
	if err != nil {
		t.Fatalf("Dissolve: %v", err)
	}
	if result.Status != StatusSucceeded || left.removeCalls != 1 || right.removeCalls != 1 {
		t.Fatalf("result = %+v; remove calls LEFT=%d RIGHT=%d", result, left.removeCalls, right.removeCalls)
	}
	for _, member := range result.Members {
		if !member.Verified || member.Group == nil || !member.Group.IsEmpty() {
			t.Errorf("member not verified empty: %+v", member)
		}
	}
}

func TestDissolveRecoversOneSidedRenameDrift(t *testing.T) {
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = configuredGroup("Renamed pair")
	right.group = configuredGroup("Original pair")
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Dissolve(DissolveRequest{
		MemberIPAddress: leftIP,
		ExpectedGroupID: "PAIR-ID",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.removeCalls != 1 || right.removeCalls != 1 {
		t.Fatalf("remove calls LEFT=%d RIGHT=%d, want 1/1", left.removeCalls, right.removeCalls)
	}
}

func TestSameGroupTopologyIgnoresNameButRejectsIdentityDrift(t *testing.T) {
	original := configuredGroup("Original pair")
	renamed := cloneGroup(original)
	renamed.Name = "Renamed pair"
	if !sameGroupTopology(original, renamed) {
		t.Fatal("name-only drift changed group topology")
	}

	tests := map[string]func(*models.Group){
		"group ID": func(group *models.Group) { group.ID = "OTHER-ID" },
		"master":   func(group *models.Group) { group.MasterDeviceID = rightID },
		"role":     func(group *models.Group) { group.Roles.Roles[1].Role = "LEFT" },
		"device":   func(group *models.Group) { group.Roles.Roles[1].DeviceID = "OTHER-ID" },
		"IP":       func(group *models.Group) { group.Roles.Roles[1].IPAddress = "198.51.100.11" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := cloneGroup(original)
			mutate(changed)
			if sameGroupTopology(original, changed) {
				t.Fatalf("%s drift was accepted as the same topology", name)
			}
		})
	}
}

func TestDissolveReverifiesLateRemovalWithoutReplay(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	left.removeErr = fakeTimeoutError{}
	left.getGroup = func(call int, current *models.Group) *models.Group {
		if call >= 5 {
			return &models.Group{}
		}

		return current
	}
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))
	coordinator.uncertainOutcomeDelays = []time.Duration{0}

	result, err := coordinator.Dissolve(DissolveRequest{
		MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.removeCalls != 1 {
		t.Fatalf("LEFT remove calls = %d, want exactly 1", left.removeCalls)
	}
	if !result.Members[0].Verified {
		t.Fatalf("late removal was not verified: %+v", result.Members[0])
	}
}

// TestDissolveCompensatesMemberThatNeverConverges covers a partial failure
// that plain reverification (as in TestDissolveReverifiesLateRemovalWithoutReplay)
// cannot resolve on its own: LEFT's group genuinely never empties after its
// first RemoveGroup call, so Dissolve must retry the removal (compensateDissolve)
// rather than settling for StatusDegraded. See i655 code-review finding #2.
func TestDissolveCompensatesMemberThatNeverConverges(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	left.getGroup = func(_ int, _ *models.Group) *models.Group {
		if left.removeCalls >= 2 {
			return &models.Group{}
		}

		return cloneGroup(group)
	}
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))
	coordinator.uncertainOutcomeDelays = []time.Duration{0}

	result, err := coordinator.Dissolve(DissolveRequest{
		MemberIPAddress: rightIP, ExpectedGroupID: "PAIR-ID",
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.removeCalls != 2 {
		t.Fatalf("LEFT remove calls = %d, want exactly 2 (initial + compensation retry)", left.removeCalls)
	}
	if right.removeCalls != 1 {
		t.Fatalf("RIGHT remove calls = %d, want exactly 1", right.removeCalls)
	}
}

func TestDissolveSkipsRemoveOnMemberAlreadyReportingEmpty(t *testing.T) {
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = configuredGroup("Pair")
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Dissolve(DissolveRequest{MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID"})
	if err != nil {
		t.Fatalf("Dissolve: %v", err)
	}
	if result.Status != StatusSucceeded || left.removeCalls != 1 || right.removeCalls != 0 {
		t.Fatalf("result = %+v; remove calls LEFT=%d RIGHT=%d, want 1/0", result, left.removeCalls, right.removeCalls)
	}
}

func TestDissolveRejectsRetryThroughAlreadyEmptyAddressedMember(t *testing.T) {
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	right.group = configuredGroup("Pair")
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Dissolve(DissolveRequest{MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID"})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.removeCalls != 0 || right.removeCalls != 0 {
		t.Fatalf("removeGroup called through an empty addressed member: LEFT=%d RIGHT=%d",
			left.removeCalls, right.removeCalls)
	}
}

func TestDissolveRecoversExactSnapshotThroughAlreadyEmptyMember(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	right.group = cloneGroup(group)

	var cleaned GenerationRef
	coordinator := NewWithGenerationCleanup(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		func(ref GenerationRef) error {
			cleaned = ref
			return nil
		},
	)

	result, err := coordinator.Dissolve(DissolveRequest{
		MemberIPAddress: leftIP,
		ExpectedGroupID: group.ID,
		ExpectedGroup:   group,
	})
	if err != nil || result.Status != StatusSucceeded {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.removeCalls != 0 || right.removeCalls != 1 {
		t.Fatalf("remove calls LEFT=%d RIGHT=%d, want 0/1", left.removeCalls, right.removeCalls)
	}
	if cleaned.GroupID != group.ID || cleaned.DeviceID != leftID || cleaned.AccountID != "ACCOUNT1" {
		t.Fatalf("cleanup ref = %+v", cleaned)
	}
}

func TestDissolveRetiresSnapshotAfterBothMembersAreFreshlyVerifiedEmpty(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")

	cleanupCalls := 0
	coordinator := NewWithGenerationCleanup(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		func(ref GenerationRef) error {
			cleanupCalls++
			if ref.GroupID != group.ID || ref.DeviceID != leftID {
				t.Fatalf("cleanup ref = %+v", ref)
			}
			return nil
		},
	)

	result, err := coordinator.Dissolve(DissolveRequest{
		MemberIPAddress: rightIP,
		ExpectedGroupID: group.ID,
		ExpectedGroup:   group,
	})
	if err != nil || result.Status != StatusSucceeded || !result.PersistenceComplete {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.removeCalls != 0 || right.removeCalls != 0 || cleanupCalls != 1 {
		t.Fatalf("remove calls LEFT=%d RIGHT=%d; cleanup=%d, want 0/0/1",
			left.removeCalls, right.removeCalls, cleanupCalls)
	}
}

func TestDissolveRejectsSnapshotFromNonMemberEndpoint(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	third := readyClient("THIRD-ID", "Unrelated")
	cleanupCalls := 0
	coordinator := NewWithGenerationCleanup(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right, thirdIP: third}),
		func(GenerationRef) error {
			cleanupCalls++
			return nil
		},
	)

	result, err := coordinator.Dissolve(DissolveRequest{
		MemberIPAddress: thirdIP,
		ExpectedGroupID: group.ID,
		ExpectedGroup:   group,
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.removeCalls != 0 || right.removeCalls != 0 || third.removeCalls != 0 || cleanupCalls != 0 {
		t.Fatalf("unsafe mutation LEFT=%d RIGHT=%d THIRD=%d cleanup=%d",
			left.removeCalls, right.removeCalls, third.removeCalls, cleanupCalls)
	}
}

func TestDissolveSubstitutedSnapshotCannotRetireRealPair(t *testing.T) {
	persisted := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	third := readyClient("THIRD-ID", "Substitute")
	right.group = cloneGroup(persisted)

	submitted := cloneGroup(persisted)
	submitted.Roles.Roles[1] = models.GroupRole{
		DeviceID: "THIRD-ID", Role: "RIGHT", IPAddress: thirdIP,
	}
	retireCalls := 0
	coordinator := NewWithGenerationCleanup(
		factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right, thirdIP: third}),
		func(ref GenerationRef) error {
			if !sameGroupConfiguration(ref.ExpectedGroup, persisted) {
				return errors.New("stored topology does not match submitted snapshot")
			}

			retireCalls++
			return nil
		},
	)

	result, err := coordinator.Dissolve(DissolveRequest{
		MemberIPAddress: leftIP,
		ExpectedGroupID: submitted.ID,
		ExpectedGroup:   submitted,
	})
	if err == nil || result.Status != StatusDegraded || result.PersistenceComplete {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if retireCalls != 0 || right.removeCalls != 0 || right.getGroupCalls != 0 {
		t.Fatalf("real pair was touched: retire=%d right removes=%d right reads=%d",
			retireCalls, right.removeCalls, right.getGroupCalls)
	}
}

func TestDissolveRetiresExactGenerationInsideMutationBoundary(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)

	cleanupStarted := make(chan GenerationRef, 1)
	releaseCleanup := make(chan struct{})
	coordinator := NewWithGenerationCleanup(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		func(ref GenerationRef) error {
			cleanupStarted <- ref
			<-releaseCleanup

			return nil
		})

	dissolveDone := make(chan Result, 1)
	go func() {
		result, _ := coordinator.Dissolve(DissolveRequest{MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID"})
		dissolveDone <- result
	}()

	cleaned := <-cleanupStarted
	if cleaned.GroupID != "PAIR-ID" || cleaned.DeviceID != leftID || cleaned.AccountID != "ACCOUNT1" {
		t.Fatalf("cleanup ref = %+v", cleaned)
	}

	createDone := make(chan struct{})
	go func() {
		_, _ = coordinator.Create(CreateRequest{LeftIPAddress: leftIP, RightIPAddress: rightIP, Name: "New Pair"})
		close(createDone)
	}()

	select {
	case <-createDone:
		t.Fatal("concurrent create passed the coordinator lock before persistence cleanup completed")
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseCleanup)
	result := <-dissolveDone
	if result.Status != StatusSucceeded || !result.PersistenceAttempted || !result.PersistenceComplete {
		t.Fatalf("dissolve result = %+v", result)
	}
	<-createDone
}

func TestDissolveReportsPersistenceFailureAsDegraded(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := NewWithGenerationCleanup(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		func(GenerationRef) error { return errors.New("datastore unavailable") })

	result, err := coordinator.Dissolve(DissolveRequest{MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID"})
	if err == nil || result.Status != StatusDegraded || !result.PersistenceAttempted || result.PersistenceComplete {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if !errors.Is(result.PersistenceError, ErrUnavailable) {
		t.Fatalf("persistence error = %v, want ErrUnavailable", result.PersistenceError)
	}
}

func TestDissolvePreservesPersistenceConflict(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := NewWithGenerationCleanup(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}),
		func(GenerationRef) error { return fmt.Errorf("stale persistence: %w", ErrConflict) })

	result, err := coordinator.Dissolve(DissolveRequest{MemberIPAddress: leftIP, ExpectedGroupID: "PAIR-ID"})
	if err == nil || result.Status != StatusDegraded || !result.PersistenceAttempted || result.PersistenceComplete {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if !errors.Is(result.PersistenceError, ErrConflict) || errors.Is(result.PersistenceError, ErrUnavailable) {
		t.Fatalf("persistence error = %v, want ErrConflict only", result.PersistenceError)
	}
	if len(result.Members) != 2 || !result.Members[0].Verified || !result.Members[1].Verified {
		t.Fatalf("members = %+v, want both physically verified standalone", result.Members)
	}
}

func TestDissolveRejectsActiveZoneOnEitherMember(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	left.zone.Members = []models.Member{{DeviceID: rightID, IP: rightIP}}
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Dissolve(DissolveRequest{MemberIPAddress: rightIP, ExpectedGroupID: "PAIR-ID"})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.zoneCalls != 1 || right.zoneCalls != 1 {
		t.Fatalf("fresh zone reads LEFT=%d RIGHT=%d, want 1/1", left.zoneCalls, right.zoneCalls)
	}
	if left.removeCalls != 0 || right.removeCalls != 0 {
		t.Fatalf("removeGroup called despite active zone: LEFT=%d RIGHT=%d", left.removeCalls, right.removeCalls)
	}
}

func TestDissolveRejectsStaleGroupIDWithoutMutation(t *testing.T) {
	group := configuredGroup("Pair")
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	left.group = cloneGroup(group)
	right.group = cloneGroup(group)
	coordinator := New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))

	result, err := coordinator.Dissolve(DissolveRequest{
		MemberIPAddress: leftIP,
		ExpectedGroupID: "OLDER-PAIR-ID",
	})
	if err == nil || result.Status != StatusFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if left.removeCalls != 0 || right.removeCalls != 0 {
		t.Fatalf("removeGroup called for stale generation: LEFT=%d RIGHT=%d",
			left.removeCalls, right.removeCalls)
	}
}

func newCreateCoordinator() (*fakeClient, *fakeClient, *Coordinator) {
	left := readyClient(leftID, "Left")
	right := readyClient(rightID, "Right")
	return left, right, New(factoryFor(map[string]*fakeClient{leftIP: left, rightIP: right}))
}

func readyClient(deviceID, name string) *fakeClient {
	return &fakeClient{
		info: &models.DeviceInfo{
			DeviceID: deviceID, Name: name, Type: "SoundTouch 10",
			MargeAccountUUID: "ACCOUNT1", MargeURL: "http://aftertouch.example",
		},
		capabilities: &models.Capabilities{DeviceID: deviceID, LRStereo: true},
		zone:         &models.ZoneInfo{Master: deviceID},
		group:        &models.Group{},
	}
}

func temporaryZone(master string, members ...string) *models.ZoneInfo {
	zone := &models.ZoneInfo{Master: master}
	for _, deviceID := range members {
		zone.Members = append(zone.Members, models.Member{DeviceID: deviceID})
	}

	return zone
}

func transitionToStandalone(deviceID string) func(int, *models.ZoneInfo) (*models.ZoneInfo, error) {
	return func(call int, current *models.ZoneInfo) (*models.ZoneInfo, error) {
		if call == 2 {
			return &models.ZoneInfo{Master: deviceID}, nil
		}

		return current, nil
	}
}

func transitionToEmptyZone() func(int, *models.ZoneInfo) (*models.ZoneInfo, error) {
	return func(call int, current *models.ZoneInfo) (*models.ZoneInfo, error) {
		if call == 2 {
			return &models.ZoneInfo{}, nil
		}

		return current, nil
	}
}

func configuredGroup(name string) *models.Group {
	return &models.Group{
		ID:             "PAIR-ID",
		Name:           name,
		MasterDeviceID: leftID,
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: leftID, Role: "LEFT", IPAddress: leftIP},
			{DeviceID: rightID, Role: "RIGHT", IPAddress: rightIP},
		}},
		SenderIPAddress: leftIP,
		Status:          "GROUP_OK",
	}
}

func factoryFor(clients map[string]*fakeClient) ClientFactory {
	return func(ipAddress string) (Client, error) {
		client, ok := clients[ipAddress]
		if !ok {
			return nil, errors.New("unknown speaker")
		}
		return client, nil
	}
}
