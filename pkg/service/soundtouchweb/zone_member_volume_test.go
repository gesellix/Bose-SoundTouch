package soundtouchweb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func zoneMemberVolumeTestRequest(masterID, memberID string, volume int) *http.Request {
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf(
		"/devices/%s/zone/member/%s/volume/%d",
		masterID,
		memberID,
		volume,
	), nil)

	return withChiParams(request, map[string]string{
		"id":       masterID,
		"memberId": memberID,
		"volume":   fmt.Sprint(volume),
	})
}

func TestHandleZoneMemberVolumeTargetsStandaloneLogicalMember(t *testing.T) {
	zone, zoneXML := testZone()
	masterSpeaker := newVolumeTestSpeaker(t, 20, zoneXML)
	memberSpeaker := newVolumeTestSpeaker(t, 31, "")

	app := NewWebApp()
	addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Kitchen", "SoundTouch 30", masterSpeaker, 20, nil, zone)
	memberConn := addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Dining", "SoundTouch 20", memberSpeaker, 31, nil, nil)

	response := httptest.NewRecorder()
	app.HandleZoneMemberVolume(response, zoneMemberVolumeTestRequest("192.0.2.10", "192.0.2.20", 44))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Data zoneMemberVolumeResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.Partial || payload.Data.ControlID != "192.0.2.20" ||
		len(payload.Data.Members) != 1 || payload.Data.Members[0].Actual == nil ||
		*payload.Data.Members[0].Actual != 44 {
		t.Fatalf("result = %+v", payload.Data)
	}
	if _, posts, gets := memberSpeaker.operations(); fmt.Sprint(posts) != "[44]" || gets != 1 {
		t.Fatalf("member operations = posts %v gets %d", posts, gets)
	}
	if _, posts, gets := masterSpeaker.operations(); len(posts) != 0 || gets != 0 {
		t.Fatalf("zone master received member volume I/O: posts %v gets %d", posts, gets)
	}
	if memberConn.Status().Volume.ActualVolume != 44 {
		t.Fatalf("cached member volume = %+v", memberConn.Status().Volume)
	}
}

func TestHandleZoneMemberVolumeUsesConfirmedStereoMasterOnce(t *testing.T) {
	zone := &models.ZoneInfo{Master: "MASTER", Members: []models.Member{
		{DeviceID: "MASTER", IP: "192.0.2.10"},
		{DeviceID: "RIGHT", IP: "192.0.2.21"},
	}}
	zoneXML := `<zone master="MASTER"><member ipaddress="192.0.2.10">MASTER</member><member ipaddress="192.0.2.21">RIGHT</member></zone>`
	group := &models.Group{
		ID: "pair", Name: "Living Room", MasterDeviceID: "RIGHT", Status: "GROUP_OK",
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: "LEFT", Role: "LEFT", IPAddress: "192.0.2.20"},
			{DeviceID: "RIGHT", Role: "RIGHT", IPAddress: "192.0.2.21"},
		}},
	}

	masterSpeaker := newVolumeTestSpeaker(t, 20, zoneXML)
	leftSpeaker := newVolumeTestSpeaker(t, 30, "")
	rightSpeaker := newVolumeTestSpeaker(t, 30, "")
	app := NewWebApp()
	addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Kitchen", "SoundTouch 30", masterSpeaker, 20, nil, zone)
	addVolumeTestDevice(app, "192.0.2.20", "LEFT", "Living Room", "SoundTouch 10", leftSpeaker, 30, group, nil)
	rightConn := addVolumeTestDevice(app, "192.0.2.21", "RIGHT", "Living Room", "SoundTouch 10", rightSpeaker, 30, group, nil)

	view := app.deviceViewSnapshot()["192.0.2.10"]
	if view.Zone == nil || len(view.Zone.Members) != 2 || view.Zone.Members[1].Kind != "stereoPair" ||
		view.Zone.Members[1].ControlID != "192.0.2.21" {
		t.Fatalf("logical stereo projection = %+v", view.Zone)
	}

	response := httptest.NewRecorder()
	app.HandleZoneMemberVolume(response, zoneMemberVolumeTestRequest("192.0.2.10", "RIGHT", 46))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	if _, posts, gets := rightSpeaker.operations(); fmt.Sprint(posts) != "[46]" || gets != 1 {
		t.Fatalf("pair master operations = posts %v gets %d", posts, gets)
	}
	if _, posts, gets := leftSpeaker.operations(); len(posts) != 0 || gets != 0 {
		t.Fatalf("pair slave received volume I/O: posts %v gets %d", posts, gets)
	}
	if rightConn.Status().Volume.ActualVolume != 46 {
		t.Fatalf("pair master cache = %+v", rightConn.Status().Volume)
	}
}
