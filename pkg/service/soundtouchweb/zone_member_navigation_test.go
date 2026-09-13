package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestHandleAPIDeviceKeepsCollapsedZoneMemberAddressable(t *testing.T) {
	app := NewWebApp()
	zone := &models.ZoneInfo{
		Master: "master-id",
		Members: []models.Member{
			{DeviceID: "master-id", IP: "192.0.2.10"},
			{DeviceID: "member-id", IP: "192.0.2.11"},
		},
	}
	for _, entry := range []DeviceEntry{
		projectionDeviceWithZone("192.0.2.10", "master-id", "Atrium", true, nil, zone),
		projectionDeviceWithZone("192.0.2.11", "member-id", "Breakfast Room", true, nil, nil),
	} {
		app.AddDevice(entry.ID, entry.Device)
	}

	if _, visible := app.deviceViewSnapshot()["192.0.2.11"]; visible {
		t.Fatal("zone member remained visible as a separate inventory card")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/control/devices/192.0.2.11", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.11"})
	response := httptest.NewRecorder()
	app.HandleAPIDevice(response, req)

	var payload struct {
		Success bool       `json:"success"`
		Data    deviceView `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode member detail: %v", err)
	}
	if response.Code != http.StatusOK || !payload.Success {
		t.Fatalf("member detail status=%d payload=%+v", response.Code, payload)
	}
	if payload.Data.Info == nil || payload.Data.Info.Name != "Breakfast Room" {
		t.Fatalf("member detail = %+v, want the logical zone member", payload.Data)
	}
}
