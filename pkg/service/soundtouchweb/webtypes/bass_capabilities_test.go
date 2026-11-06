package webtypes

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestBassCapabilitiesRejectMismatchedIdentityWithoutCaching(t *testing.T) {
	for _, reportedID := range []string{"SPEAKER", ""} {
		t.Run("recovery identity="+reportedID, func(t *testing.T) {
			var reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id := reportedID
				if reads.Add(1) == 1 {
					id = "OTHER"
				}
				_, _ = fmt.Fprintf(w, `<bassCapabilities deviceID="%s"><bassAvailable>true</bassAvailable><bassMin>-9</bassMin><bassMax>0</bassMax></bassCapabilities>`, id)
			}))
			defer server.Close()
			conn := NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{DeviceID: "SPEAKER"})
			if _, err := conn.LoadBassCapabilities(); !errors.Is(err, ErrBassCapabilitiesIdentityChanged) {
				t.Fatalf("mismatched identity error = %v", err)
			}
			caps, err := conn.LoadBassCapabilities()
			if err != nil || caps.DeviceID != reportedID || caps.BassMax != 0 {
				t.Fatalf("retry capabilities = %+v, error = %v", caps, err)
			}
			caps.BassMax = 99
			cached, err := conn.LoadBassCapabilities()
			if err != nil || cached.BassMax != 0 || reads.Load() != 2 {
				t.Fatalf("cache mutated or reloaded: %+v, error = %v, reads = %d", cached, err, reads.Load())
			}
		})
	}
}

func TestBassCapabilitiesWarmupMergesCurrentStatusAndCoalesces(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reads.Add(1) == 1 {
			close(started)
		}
		<-release
		_, _ = fmt.Fprint(w, `<bassCapabilities deviceID="SPEAKER"><bassAvailable>true</bassAvailable><bassMin>-9</bassMin><bassMax>0</bassMax></bassCapabilities>`)
	}))
	defer server.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	conn := NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{DeviceID: "SPEAKER"})
	before := conn.Status()
	published := make(chan struct{}, 1)
	conn.WarmBassCapabilities(func() { published <- struct{}{} })
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("capability request did not begin")
	}
	for i := 0; i < 25; i++ {
		conn.WarmBassCapabilities(func() { published <- struct{}{} })
	}
	conn.ApplyFieldEvent(FieldBass, func(status *DeviceStatus) {
		status.Bass = &models.Bass{ActualBass: -5}
	})
	conn.ApplyFieldEvent(FieldVolume, func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 42}
	})
	current := conn.Status()
	close(release)
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("capabilities were not published")
	}
	status := conn.Status()
	if reads.Load() != 1 || status.BassCapabilities == nil || status.Bass.ActualBass != -5 || status.Volume.ActualVolume != 42 {
		t.Fatalf("warm-up lost current status or did not coalesce: reads=%d status=%+v", reads.Load(), status)
	}
	if before.BassCapabilities != nil || current.BassCapabilities != nil || status.Revision != current.Revision+1 {
		t.Fatal("warm-up mutated an existing status snapshot")
	}
	if current.BassRevision == 0 || status.BassRevision != current.BassRevision {
		t.Fatal("capability warm-up advanced bass read evidence")
	}
	if status.IsConnected || status.HTTPReachable || status.Connectivity != before.Connectivity {
		t.Fatal("capability metadata changed connectivity")
	}
	caps, err := conn.LoadBassCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	caps.BassMin = -99
	if status.BassCapabilities.BassMin != -9 {
		t.Fatal("returned capability copy aliases published metadata")
	}
}

func TestBassCapabilitiesWarmupSkipsClosedConnection(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()
	conn := NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{DeviceID: "SPEAKER"})
	conn.Close()
	conn.WarmBassCapabilities(func() { t.Error("closed connection published capabilities") })
	if _, err := conn.LoadBassCapabilities(); err == nil || reads.Load() != 0 || conn.bassCapabilitiesRefreshing.Load() {
		t.Fatalf("closed connection loaded capabilities: error=%v reads=%d", err, reads.Load())
	}
}

func TestBassCapabilitiesWarmupFailureRetries(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reads.Add(1) == 1 {
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `<bassCapabilities deviceID="SPEAKER"><bassAvailable>true</bassAvailable><bassMin>-9</bassMin><bassMax>0</bassMax></bassCapabilities>`)
	}))
	defer server.Close()
	conn := NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{DeviceID: "SPEAKER"})
	conn.WarmBassCapabilities(nil)
	deadline := time.After(2 * time.Second)
	for conn.bassCapabilitiesRefreshing.Load() {
		select {
		case <-deadline:
			t.Fatal("failed warm-up did not finish")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if conn.Status().BassCapabilities != nil {
		t.Fatal("failed warm-up published fallback capabilities")
	}
	published := make(chan struct{}, 1)
	conn.WarmBassCapabilities(func() { published <- struct{}{} })
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("warm-up did not retry after a temporary failure")
	}
	if reads.Load() != 2 || conn.Status().BassCapabilities == nil {
		t.Fatalf("retry reads=%d capabilities=%+v", reads.Load(), conn.Status().BassCapabilities)
	}
}

func TestBassCapabilitiesWarmupDropsResponseAfterClose(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = fmt.Fprint(w, `<bassCapabilities deviceID="SPEAKER"><bassAvailable>true</bassAvailable></bassCapabilities>`)
	}))
	defer server.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	conn := NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{DeviceID: "SPEAKER"})
	var published atomic.Bool
	conn.WarmBassCapabilities(func() { published.Store(true) })
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("capability request did not begin")
	}
	conn.Close()
	close(release)
	deadline := time.After(2 * time.Second)
	for conn.bassCapabilitiesRefreshing.Load() {
		select {
		case <-deadline:
			t.Fatal("closed connection warm-up did not finish")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if conn.Status().BassCapabilities != nil || conn.bassCapabilities != nil || published.Load() {
		t.Fatal("closed connection retained or published in-flight metadata")
	}
}
