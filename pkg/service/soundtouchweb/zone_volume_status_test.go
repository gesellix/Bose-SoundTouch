package soundtouchweb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
)

func TestZoneVolumeResponseStatuses(t *testing.T) {
	for _, memberOnly := range []bool{false, true} {
		for _, test := range []struct {
			name        string
			reject      bool
			unavailable bool
			mismatch    bool
			stale       bool
			uncertain   bool
			unknownHTTP bool
			readFailure bool
			wantCode    int
			wantStatus  volumeResultStatus
			wantGets    int
		}{
			{name: "confirmed", wantCode: 200, wantStatus: volumeConfirmed, wantGets: 1},
			{name: "mismatch", mismatch: true, wantCode: 200, wantStatus: volumeUnverified, wantGets: 3},
			{name: "newer event", stale: true, wantCode: 200, wantStatus: volumeUnverified, wantGets: 3},
			{name: "uncertain transport", uncertain: true, mismatch: true, wantCode: 200, wantStatus: volumeUnverified, wantGets: 3},
			{name: "unknown HTTP outcome", unknownHTTP: true, wantCode: 200, wantStatus: volumeUnverified, wantGets: 3},
			{name: "read failed", readFailure: true, wantCode: 200, wantStatus: volumeUnverified, wantGets: 1},
			{name: "definitive rejection", reject: true, wantCode: 502, wantStatus: volumeFailed},
			{name: "prewrite unavailable", unavailable: true, wantCode: 502, wantStatus: volumeFailed},
		} {
			t.Run(fmt.Sprintf("member=%v/%s", memberOnly, test.name), func(t *testing.T) {
				zone, zoneXML := testZone()
				master := newVolumeTestSpeaker(t, 20, zoneXML)
				member := newVolumeTestSpeaker(t, 20, "")
				app := NewWebApp()
				app.volumeReadbackRetryWait = func(time.Duration) {}
				masterConn := addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Kitchen", "SoundTouch 30", master, 20, nil, zone)
				memberConn := addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Dining", "SoundTouch 20", member, 20, nil, nil)
				member.ignoreWrites = test.mismatch
				member.uncertainWrite = test.uncertain
				if test.readFailure {
					member.onVolumePost = func() {
						member.mu.Lock()
						member.volumeResponse = "<volume"
						member.mu.Unlock()
					}
				}
				if test.reject {
					member.writeStatus = http.StatusBadRequest
				}
				if test.unknownHTTP {
					member.writeStatus = http.StatusInternalServerError
					member.writeBody = "unknown outcome"
				}
				if test.unavailable {
					app.RemoveDevice("192.0.2.20")
				}
				if test.stale {
					member.onVolumeGet = func(int) { app.applyVolumeEvent(memberConn, masterConn.Status().Volume) }
				}
				if !memberOnly {
					master.ignoreWrites = test.mismatch
					master.uncertainWrite = test.uncertain
					if test.readFailure {
						master.onVolumePost = func() {
							master.mu.Lock()
							master.volumeResponse = "<volume"
							master.mu.Unlock()
						}
					}
					if test.reject {
						master.writeStatus = http.StatusBadRequest
					}
					if test.unknownHTTP {
						master.writeStatus = http.StatusInternalServerError
						master.writeBody = "unknown outcome"
					}
					if test.unavailable {
						master.volumeResponse = "<volume"
					}
					if test.stale {
						master.onVolumeGet = func(int) { app.applyVolumeEvent(masterConn, memberConn.Status().Volume) }
					}
				}
				response := httptest.NewRecorder()
				if memberOnly {
					app.HandleZoneMemberVolume(response, zoneMemberVolumeTestRequest("192.0.2.10", "MEMBER", 40))
				} else {
					request := withChiParams(httptest.NewRequest(http.MethodPost, "/zone/volume/40", nil),
						map[string]string{"id": "192.0.2.10", "volume": "40"})
					app.HandleZoneVolume(response, request)
				}
				var payload struct {
					Success bool `json:"success"`
					Data    struct {
						Partial bool                     `json:"partial"`
						Members []zoneVolumeMemberResult `json:"members"`
					} `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if response.Code != test.wantCode || payload.Success != (test.wantCode == 200) ||
					payload.Data.Partial != (test.wantStatus != volumeConfirmed) || len(payload.Data.Members) == 0 {
					t.Fatalf("response = %d %s", response.Code, response.Body.String())
				}
				for _, result := range payload.Data.Members {
					if result.Status != test.wantStatus {
						t.Fatalf("result = %+v", result)
					}
				}
				wantGets := test.wantGets
				if !memberOnly && !test.unavailable {
					wantGets++ // fresh batch baseline precedes write reconciliation
				}
				if _, posts, gets := member.operations(); gets != wantGets ||
					(!test.unavailable && fmt.Sprint(posts) != "[40]") ||
					(test.unavailable && len(posts) != 0) {
					t.Fatalf("member operations = posts %v gets %d", posts, gets)
				}
			})
		}
	}
}

func TestVolumeTopologyConflictIsTypedAndPrewrite(t *testing.T) {
	speaker := newVolumeTestSpeaker(t, 20, "")
	app := NewWebApp()
	conn := addVolumeTestDevice(app, "speaker", "SPEAKER", "Office", "SoundTouch 20", speaker, 20, nil, nil)
	topology, _ := conn.SnapshotGroupTopology()
	conn.ApplyGroupEvent(nil, time.Now())
	member := zoneVolumeMemberResult{}
	app.applyVolumeTarget(&member, "speaker", conn, topology, nil, 40)
	member.Error = "message without the original keyword"
	code, success := volumeResponseStatus([]zoneVolumeMemberResult{member})
	if code != http.StatusConflict || success || member.Status != volumeFailed {
		t.Fatalf("status = %d success=%v member=%+v", code, success, member)
	}
	if _, posts, gets := speaker.operations(); len(posts) != 0 || gets != 0 {
		t.Fatalf("prewrite conflict performed I/O: posts %v gets %d", posts, gets)
	}
}

func TestVolumeIOLeavesTopologyEventsUnblocked(t *testing.T) {
	for _, duringRead := range []bool{false, true} {
		t.Run(fmt.Sprintf("read=%v", duringRead), func(t *testing.T) {
			zone, zoneXML := testZone()
			master := newVolumeTestSpeaker(t, 20, zoneXML)
			member := newVolumeTestSpeaker(t, 20, "")
			app := NewWebApp()
			masterConn := addVolumeTestDevice(app, "192.0.2.10", "MASTER", "Kitchen", "SoundTouch 30", master, 20, nil, zone)
			memberConn := addVolumeTestDevice(app, "192.0.2.20", "MEMBER", "Dining", "SoundTouch 20", member, 20, nil, nil)
			advanceTopology := func() {
				masterConn.BeginZoneEventRefresh()
				memberConn.ApplyGroupEvent(nil, time.Now())
			}
			if duringRead {
				member.onVolumeGet = func(int) { advanceTopology() }
			} else {
				member.onVolumePost = advanceTopology
			}
			response := httptest.NewRecorder()
			app.HandleZoneMemberVolume(response, zoneMemberVolumeTestRequest("192.0.2.10", "MEMBER", 40))
			var payload struct {
				Success bool                   `json:"success"`
				Data    zoneMemberVolumeResult `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if response.Code != 200 || !payload.Success || !payload.Data.Partial ||
				payload.Data.Members[0].Status != volumeUnverified {
				t.Fatalf("topology event during I/O = %d %s", response.Code, response.Body.String())
			}
			if _, posts, gets := member.operations(); fmt.Sprint(posts) != "[40]" || gets != 1 {
				t.Fatalf("operations = posts %v gets %d", posts, gets)
			}
			if memberConn.Status().Volume.ActualVolume != 20 {
				t.Fatalf("obsolete confirmation merged: %+v", memberConn.Status().Volume)
			}
		})
	}
}

// TestRefusedVolumeWriteIsDefinitive: a write whose connection was refused
// never reached the speaker. Counting it as unverified made a zone with every
// member offline answer 200 success:true.
func TestRefusedVolumeWriteIsDefinitive(t *testing.T) {
	speaker := httptest.NewServer(http.NotFoundHandler())
	host := speaker.URL
	speaker.Close()

	err := client.NewClient(&client.Config{Host: host, Timeout: time.Second}).SetVolume(20)
	if err == nil {
		t.Fatal("SetVolume against a closed port succeeded")
	}

	if uncertainVolumeWrite(err) {
		t.Fatalf("refused connection counted as uncertain: %v", err)
	}
}
