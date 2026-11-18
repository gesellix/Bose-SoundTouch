package soundtouchweb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

const (
	presetTestIdentity = "AABBCCDDEEFF"
	presetTestInfo     = `<info deviceID="` + presetTestIdentity + `"><name>Speaker</name></info>`
	presetTestBank     = `<presets><preset id="3"><ContentItem source="TUNEIN" location="station-one"/></preset></presets>`
	presetReadPath     = "/api/control/devices/lib-device/presets"
	presetWritePath    = "/api/control/devices/lib-device/preset/3"
)

func TestDevicePresetsReturnsFreshBankWithoutChangingCache(t *testing.T) {
	var reads atomic.Int32
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			_, _ = w.Write([]byte(presetTestInfo))
		case "/presets":
			reads.Add(1)
			_, _ = w.Write([]byte(presetTestBank))
		default:
			t.Errorf("unexpected speaker request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer speaker.Close()
	app := newLibraryTestApp(speaker.URL)
	conn, _ := app.GetDevice("lib-device")
	cached := &models.Presets{Preset: []models.Preset{{ID: 1}}}
	conn.UpdateStatus(func(status *webtypes.DeviceStatus) { status.Presets = cached })
	router := chi.NewRouter()
	app.Mount(router, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, checkedPresetRequest(http.MethodGet, presetReadPath, conn.Status().Epoch))
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("fresh bank: status=%d cache=%q body=%s", w.Code, w.Header().Get("Cache-Control"), w.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Info   models.DeviceInfo     `json:"info"`
			Status webtypes.DeviceStatus `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	bank := response.Data.Status.Presets
	if !response.Success || response.Data.Info.DeviceID != presetTestIdentity || response.Data.Status.Epoch != conn.Status().Epoch ||
		bank == nil || len(bank.Preset) != 1 || bank.Preset[0].ID != 3 || reads.Load() != 1 {
		t.Fatalf("wrong fresh response: %s reads=%d", w.Body.String(), reads.Load())
	}
	if conn.Status().Presets != cached {
		t.Fatal("confirmation read replaced cached status")
	}
}

func TestDevicePresetsFailedSpeakerReadDoesNotReturnCachedBank(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "speaker unavailable", status: http.StatusServiceUnavailable},
		{name: "invalid response", status: http.StatusOK, body: `<error>unavailable</error>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/info" {
					_, _ = w.Write([]byte(presetTestInfo))
					return
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer speaker.Close()
			app := newLibraryTestApp(speaker.URL)
			conn, _ := app.GetDevice("lib-device")
			cached := &models.Presets{Preset: []models.Preset{{ID: 1}}}
			conn.UpdateStatus(func(status *webtypes.DeviceStatus) { status.Presets = cached })
			w := httptest.NewRecorder()
			app.HandleDevicePresets(w, checkedPresetRequest(http.MethodGet, presetReadPath, conn.Status().Epoch))
			if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), `"presets"`) || conn.Status().Presets != cached {
				t.Fatalf("failed read confirmed or replaced cache: status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestDevicePresetsRequiresTargetAndRejectsStaleEpochBeforeSpeakerIO(t *testing.T) {
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected speaker request %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer speaker.Close()
	app := newLibraryTestApp(speaker.URL)
	conn, _ := app.GetDevice("lib-device")
	missing := checkedPresetRequest(http.MethodGet, presetReadPath, conn.Status().Epoch)
	missing.Header.Del(presetTargetHeader)
	missingEpoch := checkedPresetRequest(http.MethodGet, presetReadPath, 0)
	malformed := checkedPresetRequest(http.MethodGet, presetReadPath, 0)
	malformed.Header.Set(presetEpochHeader, "unknown")
	stale := checkedPresetRequest(http.MethodGet, presetReadPath, conn.Status().Epoch+1)
	for _, test := range []struct {
		name string
		req  *http.Request
		want int
	}{
		{name: "missing target", req: missing, want: http.StatusBadRequest},
		{name: "missing epoch", req: missingEpoch, want: http.StatusBadRequest},
		{name: "malformed epoch", req: malformed, want: http.StatusBadRequest},
		{name: "wrong epoch", req: stale, want: http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			app.HandleDevicePresets(w, test.req)
			if w.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, test.want, w.Body.String())
			}
		})
	}
}

func TestDevicePresetsRejectsTargetChangeDuringRead(t *testing.T) {
	for _, test := range []string{"physical identity", "registry connection"} {
		t.Run(test, func(t *testing.T) {
			var app *WebApp
			var changed atomic.Bool
			speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/info":
					if test == "physical identity" && changed.Load() {
						_, _ = w.Write([]byte(`<info deviceID="REPLACEMENT"/>`))
						return
					}
					_, _ = w.Write([]byte(presetTestInfo))
				case "/presets":
					if test == "registry connection" {
						conn, _ := app.GetDevice("lib-device")
						if !app.RemoveDevice("lib-device") || !app.AddDevice("lib-device", webtypes.NewDeviceConnection(conn.Client, conn.DeviceInfo)) {
							t.Error("could not replace registry connection")
						}
					}
					changed.Store(true)
					_, _ = w.Write([]byte(presetTestBank))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer speaker.Close()
			app = newLibraryTestApp(speaker.URL)
			conn, _ := app.GetDevice("lib-device")
			w := httptest.NewRecorder()
			app.HandleDevicePresets(w, checkedPresetRequest(http.MethodGet, presetReadPath, conn.Status().Epoch))
			if !changed.Load() || w.Code != http.StatusConflict || strings.Contains(w.Body.String(), `"presets"`) {
				t.Fatalf("changed target confirmed: status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestCheckedPresetWritesValidateBeforeDispatch(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			for _, test := range []struct {
				name         string
				info         string
				staleEpoch   bool
				missingEpoch bool
				want         int
				writes       int32
			}{
				{name: "physical identity changed", info: `<info deviceID="REPLACEMENT"/>`, want: http.StatusConflict},
				{name: "epoch changed", info: presetTestInfo, staleEpoch: true, want: http.StatusConflict},
				{name: "missing epoch", info: presetTestInfo, missingEpoch: true, want: http.StatusBadRequest},
				{name: "target verified", info: presetTestInfo, want: http.StatusOK, writes: 1},
			} {
				t.Run(test.name, func(t *testing.T) {
					var writes atomic.Int32
					speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						switch r.URL.Path {
						case "/info":
							_, _ = w.Write([]byte(test.info))
						case "/sources":
							_, _ = w.Write([]byte(`<sources><sourceItem source="TUNEIN" status="READY"/></sources>`))
						case "/storePreset", "/removePreset":
							writes.Add(1)
						default:
							w.WriteHeader(http.StatusNotFound)
						}
					}))
					defer speaker.Close()
					app := newLibraryTestApp(speaker.URL)
					conn, _ := app.GetDevice("lib-device")
					epoch := conn.Status().Epoch
					if test.staleEpoch {
						epoch--
					}
					if test.missingEpoch {
						epoch = 0
					}
					router := chi.NewRouter()
					app.Mount(router, nil)
					w := httptest.NewRecorder()
					router.ServeHTTP(w, checkedPresetRequest(method, presetWritePath, epoch))
					if w.Code != test.want || writes.Load() != test.writes {
						t.Fatalf("status=%d want=%d writes=%d want=%d body=%s", w.Code, test.want, writes.Load(), test.writes, w.Body.String())
					}
				})
			}
		})
	}
}

func TestCheckedPresetWritesSkipDispatchAfterCallerCancelsDuringIdentityCheck(t *testing.T) {
	req := checkedPresetRequest(http.MethodDelete, presetWritePath, 0)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			t.Errorf("unexpected speaker request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		cancel()
		_, _ = w.Write([]byte(presetTestInfo))
	}))
	defer speaker.Close()
	app := newLibraryTestApp(speaker.URL)
	conn, _ := app.GetDevice("lib-device")
	req.Header.Set(presetEpochHeader, strconv.FormatInt(conn.Status().Epoch, 10))
	app.HandleRemovePreset(httptest.NewRecorder(), req.WithContext(ctx))
	if ctx.Err() == nil {
		t.Fatal("identity check did not cancel the request")
	}
}

func checkedPresetRequest(method, path string, epoch int64) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(`{"source":"TUNEIN","location":"station-one"}`))
	req.Header.Set(presetTargetHeader, presetTestIdentity)
	if epoch != 0 {
		req.Header.Set(presetEpochHeader, strconv.FormatInt(epoch, 10))
	}
	return withChiParams(req, map[string]string{"id": "lib-device", "slot": "3"})
}
