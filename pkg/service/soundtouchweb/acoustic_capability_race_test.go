package soundtouchweb

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

func TestBassCapabilityChangeWhileInfoBlockedPreventsWrite(t *testing.T) {
	tests := []struct {
		name         string
		capabilities *models.BassCapabilities
	}{
		{
			name: "unavailable",
			capabilities: &models.BassCapabilities{
				DeviceID: "MASTER", BassAvailable: false,
			},
		},
		{
			name: "narrower range",
			capabilities: &models.BassCapabilities{
				DeviceID: "MASTER", BassAvailable: true, BassMin: -2, BassMax: 0, BassDefault: 0,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			infoStarted := make(chan struct{})
			releaseInfo := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(releaseInfo) }) }
			defer unblock()
			var infoOnce sync.Once
			var writes atomic.Int32

			app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/info":
					infoOnce.Do(func() { close(infoStarted) })
					<-releaseInfo
					fmt.Fprint(w, `<info deviceID="MASTER"><type>SoundTouch 10</type></info>`)
				case "/bass":
					if r.Method == http.MethodPost {
						writes.Add(1)
					}
					fmt.Fprint(w, `<bass/>`)
				default:
					http.NotFound(w, r)
				}
			})
			oldCapabilities := conn.Status().BassCapabilities
			response := startAcousticBassRequest(app, -3)
			awaitAcoustic(t, infoStarted, "blocked /info admission")

			applyAcceptedBassCapabilities(t, conn, test.capabilities)
			unblock()

			got := <-response
			if got.Code != http.StatusConflict {
				t.Fatalf("code=%d body=%s, want %d", got.Code, got.Body.String(), http.StatusConflict)
			}
			if writes.Load() != 0 {
				t.Fatalf("POST count=%d, want zero after capability change", writes.Load())
			}
			if status := conn.Status(); status.BassCapabilities != test.capabilities || status.BassCapabilities == oldCapabilities {
				t.Fatalf("newer capabilities were not retained: %+v", status.BassCapabilities)
			}
		})
	}
}

func TestBassCapabilityChangeDuringPostMakesOutcomeUnverified(t *testing.T) {
	postStarted := make(chan struct{})
	releasePost := make(chan struct{})
	capabilityRead := make(chan struct{})
	bassRead := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(releasePost) }) }
	defer unblock()
	var postOnce, capabilityOnce, bassOnce sync.Once
	var writes atomic.Int32

	app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			fmt.Fprint(w, `<info deviceID="MASTER"><type>SoundTouch 10</type></info>`)
		case "/bassCapabilities":
			capabilityOnce.Do(func() { close(capabilityRead) })
			fmt.Fprint(w, `<bassCapabilities deviceID="MASTER"><bassAvailable>true</bassAvailable><bassMin>-4</bassMin><bassMax>0</bassMax><bassDefault>0</bassDefault></bassCapabilities>`)
		case "/bass":
			if r.Method == http.MethodPost {
				writes.Add(1)
				postOnce.Do(func() { close(postStarted) })
				<-releasePost
				fmt.Fprint(w, `<bass/>`)
				return
			}
			bassOnce.Do(func() { close(bassRead) })
			fmt.Fprint(w, acousticBassDocument("MASTER", -3, -3))
		default:
			http.NotFound(w, r)
		}
	})
	oldCapabilities := conn.Status().BassCapabilities
	newCapabilities := &models.BassCapabilities{
		DeviceID: "MASTER", BassAvailable: true, BassMin: -4, BassMax: 0, BassDefault: 0,
	}
	response := startAcousticBassRequest(app, -3)
	awaitAcoustic(t, postStarted, "blocked bass POST")

	applyAcceptedBassCapabilities(t, conn, newCapabilities)
	unblock()

	got := <-response
	if got.Code != http.StatusConflict {
		t.Fatalf("code=%d body=%s, want %d", got.Code, got.Body.String(), http.StatusConflict)
	}
	if writes.Load() != 1 {
		t.Fatalf("POST count=%d, want exactly one uncertain mutation", writes.Load())
	}
	if status := conn.Status(); status.BassCapabilities == nil || status.BassCapabilities == oldCapabilities ||
		status.BassCapabilities.DeviceID != newCapabilities.DeviceID ||
		status.BassCapabilities.BassAvailable != newCapabilities.BassAvailable ||
		status.BassCapabilities.BassMin != newCapabilities.BassMin ||
		status.BassCapabilities.BassMax != newCapabilities.BassMax ||
		status.BassCapabilities.BassDefault != newCapabilities.BassDefault {
		t.Fatalf("post-write path restored stale capabilities: %+v", status.BassCapabilities)
	}

	awaitAcoustic(t, capabilityRead, "background capability reconciliation")
	awaitAcoustic(t, bassRead, "background bass reconciliation")
	status := conn.Status()
	if status.BassCapabilities == oldCapabilities || status.BassCapabilities == nil ||
		!status.BassCapabilities.BassAvailable || status.BassCapabilities.BassMin != -4 ||
		status.BassCapabilities.BassMax != 0 {
		t.Fatalf("background reconciliation lost newer capabilities: %+v", status.BassCapabilities)
	}
}

func TestBassCapabilityChangeDuringReadbackRejectsOldRead(t *testing.T) {
	readStarted := make(chan struct{})
	releaseRead := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(releaseRead) }) }
	defer unblock()
	var readOnce sync.Once
	var writes atomic.Int32

	app, conn := newAcousticTestConnection(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			fmt.Fprint(w, `<info deviceID="MASTER"><type>SoundTouch 10</type></info>`)
		case "/bass":
			if r.Method == http.MethodPost {
				writes.Add(1)
				fmt.Fprint(w, `<bass/>`)
				return
			}
			readOnce.Do(func() { close(readStarted) })
			<-releaseRead
			fmt.Fprint(w, acousticBassDocument("MASTER", -3, -3))
		default:
			http.NotFound(w, r)
		}
	})
	oldCapabilities := conn.Status().BassCapabilities
	newCapabilities := &models.BassCapabilities{
		DeviceID: "MASTER", BassAvailable: true, BassMin: -4, BassMax: 0, BassDefault: 0,
	}
	oldBass := conn.Status().Bass
	response := startAcousticBassRequest(app, -3)
	awaitAcoustic(t, readStarted, "blocked bass readback")

	applyAcceptedBassCapabilities(t, conn, newCapabilities)
	unblock()

	got := <-response
	if got.Code != http.StatusConflict {
		t.Fatalf("code=%d body=%s, want %d", got.Code, got.Body.String(), http.StatusConflict)
	}
	if writes.Load() != 1 {
		t.Fatalf("POST count=%d, want exactly one", writes.Load())
	}
	status := conn.Status()
	if status.BassCapabilities != newCapabilities || status.BassCapabilities == oldCapabilities {
		t.Fatalf("old readback restored stale capabilities: %+v", status.BassCapabilities)
	}
	if status.Bass != oldBass {
		t.Fatalf("superseded readback replaced last verified bass: %+v", status.Bass)
	}
}

func startAcousticBassRequest(app *WebApp, level int) <-chan *httptest.ResponseRecorder {
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- acousticRequest(app, "bass", fmt.Sprintf(`{"level":%d}`, level), "MASTER", "")
	}()

	return response
}

func applyAcceptedBassCapabilities(
	t *testing.T,
	conn *webtypes.DeviceConnection,
	capabilities *models.BassCapabilities,
) {
	t.Helper()
	generation := conn.BeginFieldPoll(webtypes.FieldBass)
	if !conn.CompleteFieldPoll(webtypes.FieldBass, generation, func(status *webtypes.DeviceStatus) {
		status.BassCapabilities = capabilities
	}) {
		t.Fatal("new authoritative bass capabilities were not accepted")
	}
}
