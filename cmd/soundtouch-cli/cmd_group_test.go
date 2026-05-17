package main

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// happyAddGroupServer fakes a speaker's /addGroup that echoes the request
// with an assigned ID and GROUP_OK status, matching real hardware behaviour.
func happyAddGroupServer(t *testing.T, assignedID string) (*httptest.Server, *[]string) {
	t.Helper()

	bodies := make([]string, 0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/addGroup" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)

			return
		}

		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))

		var got models.Group
		if err := xml.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		got.ID = assignedID
		got.Status = "GROUP_OK"

		w.Header().Set("Content-Type", "application/xml")

		enc, _ := xml.Marshal(&got)
		_, _ = w.Write(enc)
	}))

	return srv, &bodies
}

func newTestGroupClient(serverURL string) *client.Client {
	return client.NewClientFromHost(serverURL)
}

func sampleGroupRequest(leftIP, rightIP string) *models.Group {
	return &models.Group{
		Name:           "Living Room",
		MasterDeviceID: "9070658C9D4A",
		Roles: models.GroupRoles{
			Roles: []models.GroupRole{
				{DeviceID: "9070658C9D4A", Role: "LEFT", IPAddress: leftIP},
				{DeviceID: "F45EAB3115DA", Role: "RIGHT", IPAddress: rightIP},
			},
		},
		// senderIPAddress is intentionally not set here; propagateAddGroup
		// adds it to the slave's copy only.
	}
}

func TestPropagateAddGroup_BothSucceed(t *testing.T) {
	leftSrv, leftBodies := happyAddGroupServer(t, "9999999")
	defer leftSrv.Close()

	rightSrv, rightBodies := happyAddGroupServer(t, "9999999")
	defer rightSrv.Close()

	leftClient := newTestGroupClient(leftSrv.URL)
	rightClient := newTestGroupClient(rightSrv.URL)

	req := sampleGroupRequest("192.0.2.131", "192.0.2.134")

	leftOut, rightOut := propagateAddGroup(leftClient, rightClient, "192.0.2.131", "192.0.2.134", req)

	if leftOut.err != nil {
		t.Errorf("LEFT err = %v, want nil", leftOut.err)
	}

	if rightOut.err != nil {
		t.Errorf("RIGHT err = %v, want nil", rightOut.err)
	}

	if leftOut.group == nil || leftOut.group.ID != "9999999" || leftOut.group.Status != "GROUP_OK" {
		t.Errorf("LEFT group = %+v, want id=9999999 status=GROUP_OK", leftOut.group)
	}

	if rightOut.group == nil || rightOut.group.Status != "GROUP_OK" {
		t.Errorf("RIGHT group = %+v, want status=GROUP_OK", rightOut.group)
	}

	// Both speakers must have received the roles, but only the slave's payload
	// carries senderIPAddress — see propagateAddGroup for the why.
	for label, bodies := range map[string]*[]string{"LEFT": leftBodies, "RIGHT": rightBodies} {
		if len(*bodies) != 1 {
			t.Fatalf("%s: expected exactly one POST, got %d", label, len(*bodies))
		}

		body := (*bodies)[0]
		for _, want := range []string{"<role>LEFT</role>", "<role>RIGHT</role>"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s body missing %q\nbody:\n%s", label, want, body)
			}
		}
	}

	leftBody := (*leftBodies)[0]
	if strings.Contains(leftBody, "<senderIPAddress>") {
		t.Errorf("LEFT (master) body must NOT carry <senderIPAddress>, otherwise the master flips into slave mode (issue #252)\nbody:\n%s", leftBody)
	}

	rightBody := (*rightBodies)[0]
	if !strings.Contains(rightBody, "<senderIPAddress>192.0.2.131</senderIPAddress>") {
		t.Errorf("RIGHT (slave) body must carry <senderIPAddress>192.0.2.131</senderIPAddress>\nbody:\n%s", rightBody)
	}
}

func TestPropagateAddGroup_RightFails(t *testing.T) {
	leftSrv, _ := happyAddGroupServer(t, "9999999")
	defer leftSrv.Close()

	rightSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer rightSrv.Close()

	leftClient := newTestGroupClient(leftSrv.URL)
	rightClient := newTestGroupClient(rightSrv.URL)

	req := sampleGroupRequest("192.0.2.131", "192.0.2.134")

	leftOut, rightOut := propagateAddGroup(leftClient, rightClient, "192.0.2.131", "192.0.2.134", req)

	if leftOut.err != nil {
		t.Errorf("LEFT err = %v, want nil", leftOut.err)
	}

	if rightOut.err == nil {
		t.Error("RIGHT err = nil, want non-nil")
	}
}

func TestPostAddGroup_StatusOtherThanGroupOKIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<group><status>GROUP_NOT_READY</status></group>`))
	}))
	defer srv.Close()

	out := postAddGroup(newTestGroupClient(srv.URL), "test", sampleGroupRequest("1.1.1.1", "2.2.2.2"))

	if out.err == nil {
		t.Fatal("expected error for non-GROUP_OK status")
	}

	if !strings.Contains(out.err.Error(), "GROUP_NOT_READY") {
		t.Errorf("error %q does not mention returned status", out.err)
	}
}

func TestPostAddGroup_EmptyStatusIsAccepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<group id="42"><name>n</name></group>`))
	}))
	defer srv.Close()

	out := postAddGroup(newTestGroupClient(srv.URL), "test", sampleGroupRequest("1.1.1.1", "2.2.2.2"))

	if out.err != nil {
		t.Errorf("err = %v, want nil for empty status (some firmware omits it)", out.err)
	}

	if out.group == nil || out.group.ID != "42" {
		t.Errorf("group = %+v, want id=42", out.group)
	}
}
