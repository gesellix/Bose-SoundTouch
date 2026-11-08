package soundtouchweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

func TestBassControlsUseSpeakerRangeAndCacheCapabilities(t *testing.T) {
	var reads, writes atomic.Int32
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bassCapabilities":
			reads.Add(1)
			_, _ = w.Write([]byte(`<bassCapabilities><bassAvailable>true</bassAvailable><bassMin>-9</bassMin><bassMax>0</bassMax><bassDefault>0</bassDefault></bassCapabilities>`))
		case "/bass":
			if r.Method == http.MethodPost {
				writes.Add(1)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer speaker.Close()
	app := NewWebApp()
	device := webtypes.NewDeviceConnection(client.NewClient(&client.Config{Host: speaker.URL}), &models.DeviceInfo{DeviceID: "SPEAKER"})
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"level":1}`, http.StatusBadRequest},
		{`{"level":-9}`, http.StatusOK},
		{`{"level":0}`, http.StatusOK},
		{`{"level":-10}`, http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		app.handleBassControl(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body)), device)
		if response.Code != tc.code {
			t.Fatalf("%s: status=%d body=%s", tc.body, response.Code, response.Body.String())
		}
	}
	if reads.Load() != 1 || writes.Load() != 2 {
		t.Fatalf("capability reads=%d bass writes=%d", reads.Load(), writes.Load())
	}
}

func TestBassCapabilityFailureCanRecover(t *testing.T) {
	var reads, writes atomic.Int32
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bassCapabilities" {
			if reads.Add(1) == 1 {
				http.Error(w, "temporary failure", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`<bassCapabilities><bassAvailable>true</bassAvailable><bassMin>-9</bassMin><bassMax>0</bassMax></bassCapabilities>`))
			return
		}
		writes.Add(1)
	}))
	defer speaker.Close()
	app := NewWebApp()
	device := webtypes.NewDeviceConnection(client.NewClient(&client.Config{Host: speaker.URL}), &models.DeviceInfo{DeviceID: "SPEAKER"})
	// +4 is inside the fixed -9..9 range but above this speaker's bassMax 0.
	// The first request can't read the capabilities and writes, as main did;
	// the second reads them and refuses.
	for _, code := range []int{http.StatusOK, http.StatusBadRequest} {
		response := httptest.NewRecorder()
		app.handleBassControl(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"level":4}`)), device)
		if response.Code != code {
			t.Fatalf("status=%d want=%d body=%s", response.Code, code, response.Body.String())
		}
	}
	if writes.Load() != 1 {
		t.Fatalf("bass writes=%d", writes.Load())
	}
}

func TestBassControlsRejectUnsupportedSpeaker(t *testing.T) {
	var writes atomic.Int32
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writes.Add(1)
		}
		_, _ = w.Write([]byte(`<bassCapabilities><bassAvailable>false</bassAvailable></bassCapabilities>`))
	}))
	defer speaker.Close()
	device := webtypes.NewDeviceConnection(client.NewClient(&client.Config{Host: speaker.URL}), &models.DeviceInfo{DeviceID: "SPEAKER"})
	response := httptest.NewRecorder()
	NewWebApp().handleBassControl(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"level":0}`)), device)
	if response.Code != http.StatusConflict || writes.Load() != 0 {
		t.Fatalf("status=%d writes=%d body=%s", response.Code, writes.Load(), response.Body.String())
	}
}

func TestBassControlsRefuseAnAddressNowAnsweredByAnotherSpeaker(t *testing.T) {
	var writes atomic.Int32
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writes.Add(1)
		}
		_, _ = w.Write([]byte(`<bassCapabilities deviceID="REPLACEMENT"><bassAvailable>true</bassAvailable><bassMin>-9</bassMin><bassMax>0</bassMax><bassDefault>0</bassDefault></bassCapabilities>`))
	}))
	defer speaker.Close()
	device := webtypes.NewDeviceConnection(client.NewClient(&client.Config{Host: speaker.URL}), &models.DeviceInfo{DeviceID: "SPEAKER"})
	response := httptest.NewRecorder()
	NewWebApp().handleBassControl(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"level":-1}`)), device)
	if response.Code != http.StatusConflict || writes.Load() != 0 {
		t.Fatalf("status=%d writes=%d body=%s", response.Code, writes.Load(), response.Body.String())
	}
}
