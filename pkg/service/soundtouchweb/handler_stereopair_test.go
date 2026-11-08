package soundtouchweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/stereopair"
)

type fakeStereoPairLifecycle struct {
	inspectCalls   int
	inspectResult  stereopair.Result
	inspectErr     error
	createResult   stereopair.Result
	createErr      error
	renameResult   stereopair.Result
	renameErr      error
	dissolveResult stereopair.Result
	dissolveErr    error

	createRequest   stereopair.CreateRequest
	renameRequest   stereopair.RenameRequest
	dissolveRequest stereopair.DissolveRequest
}

func (f *fakeStereoPairLifecycle) Inspect(string) (stereopair.Result, error) {
	f.inspectCalls++
	return f.inspectResult, f.inspectErr
}

func (f *fakeStereoPairLifecycle) Create(req stereopair.CreateRequest) (stereopair.Result, error) {
	f.createRequest = req
	return f.createResult, f.createErr
}

func (f *fakeStereoPairLifecycle) Rename(req stereopair.RenameRequest) (stereopair.Result, error) {
	f.renameRequest = req
	return f.renameResult, f.renameErr
}

func (f *fakeStereoPairLifecycle) Dissolve(req stereopair.DissolveRequest) (stereopair.Result, error) {
	f.dissolveRequest = req

	return f.dissolveResult, f.dissolveErr
}

func stereoPairTestApp(lifecycle StereoPairLifecycle) *WebApp {
	app := NewWebApp()
	app.StereoPairs = lifecycle
	for _, speaker := range []struct {
		host, id, name string
	}{
		{"192.0.2.10", "left-id", "Left"},
		{"192.0.2.11", "right-id", "Right"},
	} {
		conn := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{
			DeviceID:  speaker.id,
			Name:      speaker.name,
			Type:      "SoundTouch 10",
			IPAddress: speaker.host,
		})
		app.AddDevice(speaker.host, conn)
	}

	return app
}

func decodeStereoPairAPIResponse(t *testing.T, response *httptest.ResponseRecorder) struct {
	Success bool               `json:"success"`
	Data    stereoPairResponse `json:"data"`
	Error   string             `json:"error"`
} {
	t.Helper()

	var payload struct {
		Success bool               `json:"success"`
		Data    stereoPairResponse `json:"data"`
		Error   string             `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	return payload
}

func TestHandleGetStereoPairReportsStandaloneCapableSpeaker(t *testing.T) {
	fake := &fakeStereoPairLifecycle{inspectResult: stereopair.Result{
		Operation: stereopair.OperationInspect,
		Status:    stereopair.StatusSucceeded,
		Group:     &models.Group{},
	}}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodGet, "/api/control/devices/192.0.2.10/stereo-pair", nil), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleGetStereoPair(response, request)

	payload := decodeStereoPairAPIResponse(t, response)
	if response.Code != http.StatusOK || !payload.Success || !payload.Data.Capable || payload.Data.Paired {
		t.Fatalf("unexpected standalone response: status=%d payload=%+v", response.Code, payload)
	}
}

func TestHandleGetStereoPairSkipsUnsupportedModel(t *testing.T) {
	fake := &fakeStereoPairLifecycle{}
	app := NewWebApp()
	app.StereoPairs = fake
	app.AddDevice("192.0.2.30", webtypes.NewDeviceConnection(nil, &models.DeviceInfo{
		DeviceID:  "st30-id",
		Name:      "Living Room",
		Type:      "SoundTouch 30",
		IPAddress: "192.0.2.30",
	}))

	request := withChiParams(httptest.NewRequest(http.MethodGet,
		"/api/control/devices/192.0.2.30/stereo-pair", nil), map[string]string{"id": "192.0.2.30"})
	response := httptest.NewRecorder()
	app.HandleGetStereoPair(response, request)

	payload := decodeStereoPairAPIResponse(t, response)
	if response.Code != http.StatusOK || !payload.Success || payload.Data.Capable || payload.Data.Paired {
		t.Fatalf("unexpected unsupported-model response: status=%d payload=%+v", response.Code, payload)
	}
	if fake.inspectCalls != 0 {
		t.Fatalf("stereo lifecycle Inspect calls = %d, want 0", fake.inspectCalls)
	}
}

func TestHandleGetStereoPairMapsUnavailableSpeakerToBadGateway(t *testing.T) {
	fake := &fakeStereoPairLifecycle{
		inspectResult: stereopair.Result{
			Operation: stereopair.OperationInspect,
			Status:    stereopair.StatusFailed,
			Members: []stereopair.MemberResult{{
				IPAddress:      "192.0.2.10",
				PreflightError: fmt.Errorf("%w: timeout", stereopair.ErrUnavailable),
			}},
		},
		inspectErr: &stereopair.Error{Operation: stereopair.OperationInspect, Status: stereopair.StatusFailed},
	}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodGet,
		"/api/control/devices/192.0.2.10/stereo-pair", nil), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleGetStereoPair(response, request)

	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", response.Code, response.Body.String())
	}
}

func TestHandleGetStereoPairPreservesDegradedRecoveryGeneration(t *testing.T) {
	group := &models.Group{ID: "PAIR-ID", Name: "Living Room", MasterDeviceID: "left-id"}
	fake := &fakeStereoPairLifecycle{
		inspectResult: stereopair.Result{
			Operation: stereopair.OperationInspect,
			Status:    stereopair.StatusDegraded,
			Group:     group,
			Members: []stereopair.MemberResult{
				{IPAddress: "192.0.2.10", DeviceID: "left-id", Group: group, Verified: true},
				{IPAddress: "192.0.2.11", DeviceID: "right-id", VerificationError: errors.New("group is empty")},
			},
		},
		inspectErr: &stereopair.Error{Operation: stereopair.OperationInspect, Status: stereopair.StatusDegraded},
	}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodGet,
		"/api/control/devices/192.0.2.10/stereo-pair", nil), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleGetStereoPair(response, request)

	payload := decodeStereoPairAPIResponse(t, response)
	if response.Code != http.StatusOK || payload.Success || !payload.Data.Paired ||
		payload.Data.Group == nil || payload.Data.Group.ID != "PAIR-ID" {
		t.Fatalf("degraded recovery data was lost: status=%d payload=%+v", response.Code, payload)
	}
}

func TestHandleCreateStereoPairPassesPhysicalHostsAndName(t *testing.T) {
	group := &models.Group{ID: "1234567", Name: "Office", MasterDeviceID: "left-id"}
	fake := &fakeStereoPairLifecycle{createResult: stereopair.Result{
		Operation: stereopair.OperationCreate,
		Status:    stereopair.StatusSucceeded,
		Group:     group,
	}}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodPost,
		"/api/control/devices/192.0.2.10/stereo-pair",
		strings.NewReader(`{"rightId":"192.0.2.11","name":" Office "}`)), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleCreateStereoPair(response, request)

	if fake.createRequest.LeftIPAddress != "192.0.2.10" || fake.createRequest.RightIPAddress != "192.0.2.11" || fake.createRequest.Name != "Office" {
		t.Fatalf("unexpected coordinator request: %+v", fake.createRequest)
	}
	payload := decodeStereoPairAPIResponse(t, response)
	if response.Code != http.StatusOK || !payload.Success || !payload.Data.Paired || payload.Data.Group.ID != "1234567" {
		t.Fatalf("unexpected create response: status=%d payload=%+v", response.Code, payload)
	}
}

func TestHandleCreateStereoPairRespondsBeforeStatusRefresh(t *testing.T) {
	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/now_playing" {
			close(refreshStarted)
			<-releaseRefresh
		}

		http.Error(w, "status unavailable", http.StatusServiceUnavailable)
	}))
	defer speaker.Close()
	defer close(releaseRefresh)

	group := &models.Group{ID: "1234567", Name: "Office", MasterDeviceID: "left-id"}
	fake := &fakeStereoPairLifecycle{createResult: stereopair.Result{
		Operation: stereopair.OperationCreate,
		Status:    stereopair.StatusSucceeded,
		Group:     group,
		Members: []stereopair.MemberResult{{
			IPAddress: "192.0.2.10",
			DeviceID:  "left-id",
			Verified:  true,
			Group:     group,
		}},
	}}
	app := stereoPairTestApp(fake)
	left, _ := app.GetDevice("192.0.2.10")
	left.Client = client.NewClientFromHost(speaker.URL)

	request := withChiParams(httptest.NewRequest(http.MethodPost,
		"/api/control/devices/192.0.2.10/stereo-pair",
		strings.NewReader(`{"rightId":"192.0.2.11","name":"Office"}`)), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	handlerDone := make(chan struct{})
	go func() {
		app.HandleCreateStereoPair(response, request)
		close(handlerDone)
	}()

	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("follow-up status refresh did not start")
	}

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("lifecycle response waited for the blocked status refresh")
	}

	payload := decodeStereoPairAPIResponse(t, response)
	if response.Code != http.StatusOK || !payload.Success || payload.Data.Group == nil || payload.Data.Group.ID != group.ID {
		t.Fatalf("unexpected create response: status=%d payload=%+v", response.Code, payload)
	}
	if projected := left.Status().Group; projected == nil || projected.ID != group.ID {
		t.Fatalf("coordinator projection was not applied before refresh: %+v", projected)
	}
}

func TestApplyStereoPairProjectionSupersedesInFlightGroupRefresh(t *testing.T) {
	app := stereoPairTestApp(&fakeStereoPairLifecycle{})
	left, _ := app.GetDevice("192.0.2.10")
	staleGeneration := left.BeginGroupRefresh()
	newGroup := &models.Group{ID: "pair-new", MasterDeviceID: "left-id"}

	app.applyStereoPairProjection(stereopair.Result{Members: []stereopair.MemberResult{{
		IPAddress: "192.0.2.10",
		Group:     newGroup,
	}}})

	if left.ApplyPolledGroup(staleGeneration, &models.Group{ID: "pair-old"}) {
		t.Fatal("older status refresh replaced the lifecycle projection")
	}
	if projected := left.Status().Group; projected == nil || projected.ID != newGroup.ID {
		t.Fatalf("projected group = %+v, want %q", projected, newGroup.ID)
	}
}

func TestStereoPairResponseWaitsForPreMutationWebSocketWriter(t *testing.T) {
	app := stereoPairTestApp(&fakeStereoPairLifecycle{})
	app.webSocketWriteTimeout = 50 * time.Millisecond
	blockedWriter := &deadlineBlockingWebSocketWriter{started: make(chan struct{})}
	app.registerDeviceWebSocketClient(blockedWriter)
	defer app.removeDeviceWebSocketClient(blockedWriter)

	writerDone := make(chan struct{})
	go func() {
		_ = app.withDeviceConnWrite(blockedWriter, func(batch webSocketWriteBatch) error {
			return batch.writeJSON(blockedWriter, struct{}{})
		})
		close(writerDone)
	}()
	<-blockedWriter.started

	response := httptest.NewRecorder()
	responseDone := make(chan struct{})
	group := &models.Group{ID: "PAIR-ID", MasterDeviceID: "left-id"}
	left, _ := app.GetDevice("192.0.2.10")
	go func() {
		app.completeStereoPairMutation(response, left.DeviceInfo, stereopair.Result{
			Operation: stereopair.OperationCreate,
			Status:    stereopair.StatusSucceeded,
			Group:     group,
			Members: []stereopair.MemberResult{{
				IPAddress: "192.0.2.10",
				DeviceID:  "left-id",
				Group:     group,
			}},
		}, nil)
		close(responseDone)
	}()

	select {
	case <-responseDone:
		t.Fatal("lifecycle response overtook a pre-mutation WebSocket writer")
	case <-time.After(10 * time.Millisecond):
	}

	select {
	case <-writerDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("stalled pre-mutation WebSocket writer ignored its batch deadline")
	}
	select {
	case <-responseDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("lifecycle response did not pass the WebSocket ordering barrier")
	}
}

func TestHandleCreateStereoPairUsesDeviceInfoIPsForHostnameRegistryKeys(t *testing.T) {
	fake := &fakeStereoPairLifecycle{createResult: stereopair.Result{
		Operation: stereopair.OperationCreate,
		Status:    stereopair.StatusSucceeded,
		Group:     &models.Group{ID: "1234567"},
	}}
	app := NewWebApp()
	app.StereoPairs = fake
	for _, speaker := range []struct {
		key, ip, id string
	}{
		{"left.example.test", "192.0.2.10", "left-id"},
		{"right.example.test", "192.0.2.11", "right-id"},
	} {
		app.AddDevice(speaker.key, webtypes.NewDeviceConnection(nil, &models.DeviceInfo{
			DeviceID: speaker.id, Type: "ST10", IPAddress: speaker.ip,
		}))
	}

	request := withChiParams(httptest.NewRequest(http.MethodPost,
		"/api/control/devices/left.example.test/stereo-pair",
		strings.NewReader(`{"rightId":"right.example.test","name":"Office"}`)), map[string]string{"id": "left.example.test"})
	response := httptest.NewRecorder()
	app.HandleCreateStereoPair(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	payload := decodeStereoPairAPIResponse(t, response)
	if !payload.Data.Capable {
		t.Fatal("ST10 model spelling was not reported as stereo capable")
	}
	if fake.createRequest.LeftIPAddress != "192.0.2.10" || fake.createRequest.RightIPAddress != "192.0.2.11" {
		t.Fatalf("coordinator addresses = %+v", fake.createRequest)
	}
}

func TestHandleRenameStereoPairPassesExpectedGeneration(t *testing.T) {
	fake := &fakeStereoPairLifecycle{renameResult: stereopair.Result{
		Operation: stereopair.OperationRename,
		Status:    stereopair.StatusSucceeded,
		Group:     &models.Group{ID: "PAIR-ID", Name: "Office"},
	}}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodPatch,
		"/api/control/devices/192.0.2.10/stereo-pair",
		strings.NewReader(`{"groupId":" PAIR-ID ","name":" Office "}`)), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleRenameStereoPair(response, request)

	if fake.renameRequest.ExpectedGroupID != "PAIR-ID" || fake.renameRequest.Name != "Office" {
		t.Fatalf("rename request = %+v", fake.renameRequest)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
}

func TestHandleRenameStereoPairRequiresExpectedGeneration(t *testing.T) {
	fake := &fakeStereoPairLifecycle{}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodPatch,
		"/api/control/devices/192.0.2.10/stereo-pair",
		strings.NewReader(`{"name":"Office"}`)), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleRenameStereoPair(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
	}
}

func TestHandleDissolveStereoPairPreservesDegradedDetails(t *testing.T) {
	verifyErr := errors.New("right speaker still reports the pair")
	fake := &fakeStereoPairLifecycle{dissolveResult: stereopair.Result{
		Operation: stereopair.OperationDissolve,
		Status:    stereopair.StatusDegraded,
		Members: []stereopair.MemberResult{{
			IPAddress:         "192.0.2.11",
			DeviceID:          "right-id",
			VerificationError: verifyErr,
		}},
	}, dissolveErr: &stereopair.Error{Operation: stereopair.OperationDissolve, Status: stereopair.StatusDegraded}}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodDelete,
		"/api/control/devices/192.0.2.10/stereo-pair",
		strings.NewReader(`{"groupId":"PAIR-ID"}`)), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleDissolveStereoPair(response, request)

	if fake.dissolveRequest.ExpectedGroupID != "PAIR-ID" {
		t.Fatalf("dissolve request = %+v", fake.dissolveRequest)
	}

	payload := decodeStereoPairAPIResponse(t, response)
	if response.Code != http.StatusBadGateway || payload.Success || len(payload.Data.Members) != 1 ||
		payload.Data.Members[0].VerificationError != verifyErr.Error() {
		t.Fatalf("unexpected degraded response: status=%d payload=%+v", response.Code, payload)
	}
}

func TestHandleDissolveStereoPairPassesExactRecoverySnapshot(t *testing.T) {
	fake := &fakeStereoPairLifecycle{dissolveResult: stereopair.Result{
		Operation: stereopair.OperationDissolve,
		Status:    stereopair.StatusSucceeded,
		Group:     &models.Group{},
	}}
	app := stereoPairTestApp(fake)
	body := `{"groupId":"PAIR-ID","group":{"ID":"PAIR-ID","Name":"Office","MasterDeviceID":"left-id","Roles":{"Roles":[{"DeviceID":"left-id","Role":"LEFT","IPAddress":"192.0.2.10"},{"DeviceID":"right-id","Role":"RIGHT","IPAddress":"192.0.2.11"}]}}}`

	request := withChiParams(httptest.NewRequest(http.MethodDelete,
		"/api/control/devices/192.0.2.10/stereo-pair", strings.NewReader(body)),
		map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleDissolveStereoPair(response, request)

	group := fake.dissolveRequest.ExpectedGroup
	if response.Code != http.StatusOK || group == nil || group.ID != "PAIR-ID" ||
		group.MasterDeviceID != "left-id" || len(group.Roles.Roles) != 2 {
		t.Fatalf("status=%d dissolve request=%+v", response.Code, fake.dissolveRequest)
	}
}

func TestHandleDissolveStereoPairRejectsUnregisteredSnapshotMember(t *testing.T) {
	fake := &fakeStereoPairLifecycle{}
	app := stereoPairTestApp(fake)
	body := `{"groupId":"PAIR-ID","group":{"ID":"PAIR-ID","MasterDeviceID":"left-id","Roles":{"Roles":[{"DeviceID":"left-id","Role":"LEFT","IPAddress":"192.0.2.10"},{"DeviceID":"substitute-id","Role":"RIGHT","IPAddress":"192.0.2.99"}]}}}`

	request := withChiParams(httptest.NewRequest(http.MethodDelete,
		"/api/control/devices/192.0.2.10/stereo-pair", strings.NewReader(body)),
		map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleDissolveStereoPair(response, request)

	if response.Code != http.StatusConflict || fake.dissolveRequest.ExpectedGroup != nil {
		t.Fatalf("status=%d coordinator request=%+v", response.Code, fake.dissolveRequest)
	}
}

func TestHandleDissolveStereoPairReportsCompletedPersistence(t *testing.T) {
	fake := &fakeStereoPairLifecycle{dissolveResult: stereopair.Result{
		Operation:            stereopair.OperationDissolve,
		Status:               stereopair.StatusSucceeded,
		Group:                &models.Group{},
		PersistenceAttempted: true,
		PersistenceComplete:  true,
	}}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodDelete,
		"/api/control/devices/192.0.2.10/stereo-pair",
		strings.NewReader(`{"groupId":"PAIR-ID"}`)), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleDissolveStereoPair(response, request)

	payload := decodeStereoPairAPIResponse(t, response)
	if response.Code != http.StatusOK || !payload.Success ||
		!payload.Data.PersistenceAttempted || !payload.Data.PersistenceComplete {
		t.Fatalf("status=%d payload=%+v", response.Code, payload)
	}
}

func TestHandleDissolveStereoPairReportsPersistenceFailure(t *testing.T) {
	fake := &fakeStereoPairLifecycle{dissolveResult: stereopair.Result{
		Operation:            stereopair.OperationDissolve,
		Status:               stereopair.StatusDegraded,
		Group:                &models.Group{},
		PersistenceAttempted: true,
		PersistenceError:     errors.New("datastore unavailable"),
	}, dissolveErr: &stereopair.Error{Operation: stereopair.OperationDissolve, Status: stereopair.StatusDegraded}}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodDelete,
		"/api/control/devices/192.0.2.10/stereo-pair",
		strings.NewReader(`{"groupId":"PAIR-ID"}`)), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleDissolveStereoPair(response, request)

	payload := decodeStereoPairAPIResponse(t, response)
	if response.Code != http.StatusBadGateway || payload.Success ||
		payload.Data.Status != string(stereopair.StatusDegraded) ||
		payload.Data.PersistenceError != "datastore unavailable" {
		t.Fatalf("unexpected persistence response: status=%d payload=%+v", response.Code, payload)
	}
}

func TestHandleDissolveStereoPairMapsPersistenceConflictToConflict(t *testing.T) {
	persistenceErr := fmt.Errorf("generation remains active: %w", stereopair.ErrConflict)
	fake := &fakeStereoPairLifecycle{dissolveResult: stereopair.Result{
		Operation:            stereopair.OperationDissolve,
		Status:               stereopair.StatusDegraded,
		Group:                &models.Group{},
		PersistenceAttempted: true,
		PersistenceError:     persistenceErr,
		Members: []stereopair.MemberResult{
			{IPAddress: "192.0.2.10", Verified: true, Group: &models.Group{}},
			{IPAddress: "192.0.2.11", Verified: true, Group: &models.Group{}},
		},
	}, dissolveErr: &stereopair.Error{Operation: stereopair.OperationDissolve, Status: stereopair.StatusDegraded}}
	app := stereoPairTestApp(fake)

	request := withChiParams(httptest.NewRequest(http.MethodDelete,
		"/api/control/devices/192.0.2.10/stereo-pair",
		strings.NewReader(`{"groupId":"PAIR-ID"}`)), map[string]string{"id": "192.0.2.10"})
	response := httptest.NewRecorder()
	app.HandleDissolveStereoPair(response, request)

	payload := decodeStereoPairAPIResponse(t, response)
	if response.Code != http.StatusConflict || payload.Success ||
		payload.Data.Status != string(stereopair.StatusDegraded) || payload.Data.Paired ||
		payload.Data.PersistenceError != persistenceErr.Error() {
		t.Fatalf("unexpected conflict response: status=%d payload=%+v", response.Code, payload)
	}
}
