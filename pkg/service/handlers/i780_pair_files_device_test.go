package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/marge"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/go-chi/chi/v5"
)

// Issue 780: a Wireless Link Adapter on firmware 20 uses the account it was
// paired with, but its /info keeps reporting an empty margeAccountUUID, so
// discovery alone left it filed under "default".

const (
	i780Device  = "AABBCCDDEE0A"
	i780Account = "1234567"
	i780Presets = `<presets><preset id="2"><ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="/station?data=abc" isPresetable="true"><itemName>Live Station</itemName></ContentItem></preset></presets>`
)

// i780Speaker is a speaker whose /info never reports an account.
type i780Speaker struct {
	setMargeSupported bool
	presets           string
	presetsStatus     int
}

func (sp *i780Speaker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/supportedURLs":
		if sp.setMargeSupported {
			fmt.Fprint(w, `<supportedURLs><URL location="/setMargeAccount"/></supportedURLs>`)
			return
		}

		fmt.Fprint(w, `<supportedURLs><URL location="/info"/></supportedURLs>`)
	case "/setMargeAccount":
		w.WriteHeader(http.StatusOK)
	case "/info":
		fmt.Fprintf(w, `<info deviceID="%s"><name>Adapter</name><type>SoundTouch Wireless Link Adapter</type><margeAccountUUID></margeAccountUUID><moduleType>sm2</moduleType></info>`, i780Device)
	case "/presets":
		if sp.presetsStatus != 0 {
			w.WriteHeader(sp.presetsStatus)
			return
		}

		fmt.Fprint(w, sp.presets)
	case "/recents":
		fmt.Fprint(w, `<recents/>`)
	default:
		http.NotFound(w, r)
	}
}

type i780Telnet struct{ reply string }

func (t *i780Telnet) Dial() error                        { return nil }
func (t *i780Telnet) Probe() (string, error)             { return "", nil }
func (t *i780Telnet) SendCommand(string) (string, error) { return t.reply, nil }
func (t *i780Telnet) Close() error                       { return nil }

type i780Env struct {
	ds     *datastore.DataStore
	srv    *Server
	router chi.Router
	addr   string
}

// newI780Env files the device under "default", as discovery does for a speaker
// that reports no account.
func newI780Env(t *testing.T, speaker *i780Speaker) *i780Env {
	t.Helper()

	ds := datastore.NewDataStore(t.TempDir())

	ts := httptest.NewServer(speaker)
	t.Cleanup(ts.Close)

	addr := strings.TrimPrefix(ts.URL, "http://")

	info := &models.ServiceDeviceInfo{DeviceID: i780Device, AccountID: "default", Name: "Adapter", IPAddress: addr}
	if err := ds.SaveDeviceInfo("default", i780Device, info); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	sm := setup.NewManager(ts.URL, ds, nil)
	srv := NewServer(ds, sm, "http://localhost", false, false, false)

	r := chi.NewRouter()
	r.Post("/setup/pair-account/{deviceId}", srv.HandlePairAccount)

	return &i780Env{ds: ds, srv: srv, router: r, addr: addr}
}

func (e *i780Env) pair(t *testing.T) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/setup/pair-account/"+i780Device+"?account_id="+i780Account, nil)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("pair-account: status %d: %s", rec.Code, rec.Body.String())
	}

	return rec.Body.String()
}

func (e *i780Env) filedUnder(t *testing.T) string {
	t.Helper()

	devices, err := e.ds.ListAllDevices()
	if err != nil {
		t.Fatalf("ListAllDevices: %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("want one device, got %+v", devices)
	}

	return devices[0].AccountID
}

func TestPairAccountFilesTheDeviceUnderTheAccount_GH780(t *testing.T) {
	env := newI780Env(t, &i780Speaker{setMargeSupported: true, presets: i780Presets})

	env.pair(t)

	if got := env.filedUnder(t); got != i780Account {
		t.Fatalf("filed under %q, want %q", got, i780Account)
	}

	info, err := env.ds.GetExactDeviceInfo(i780Account, i780Device)
	if err != nil {
		t.Fatalf("GetExactDeviceInfo: %v", err)
	}

	if info.AccountID != i780Account {
		t.Errorf("stored AccountID = %q, want %q", info.AccountID, i780Account)
	}

	// An empty /info must not send it back on the next discovery.
	env.srv.handleDiscoveredDevice(models.DiscoveredDevice{Host: env.addr, Name: "Adapter", DiscoveryMethod: "mDNS"})

	if got := env.filedUnder(t); got != i780Account {
		t.Errorf("after discovery filed under %q, want %q", got, i780Account)
	}
}

// The device joins the account's /full by moving. It never reported presets, so
// without this copy /full would list it with none.
func TestPairAccountKeepsTheSpeakersPresetsInFull_GH780(t *testing.T) {
	env := newI780Env(t, &i780Speaker{setMargeSupported: true, presets: i780Presets})

	env.pair(t)

	presets, err := env.ds.GetPresets(i780Account, i780Device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if len(presets) != 1 || presets[0].Name != "Live Station" {
		t.Fatalf("presets = %+v, want the speaker's one", presets)
	}

	full, err := marge.AccountFullToXML(env.ds, i780Account)
	if err != nil {
		t.Fatalf("AccountFullToXML: %v", err)
	}

	if !strings.Contains(string(full), "Live Station") {
		t.Errorf("/full does not carry the speaker's preset: %s", full)
	}
}

func TestPairAccountStaysUnderDefaultWhenPresetsCannotBeRead_GH780(t *testing.T) {
	env := newI780Env(t, &i780Speaker{setMargeSupported: true, presetsStatus: http.StatusInternalServerError})

	body := env.pair(t)

	if got := env.filedUnder(t); got != "default" {
		t.Errorf("filed under %q, want it left under default", got)
	}

	if !strings.Contains(body, "default") {
		t.Errorf("response does not say the device stayed filed under default: %s", body)
	}
}

func TestPairAccountDoesNotReplaceStoredPresets_GH780(t *testing.T) {
	env := newI780Env(t, &i780Speaker{setMargeSupported: true, presets: i780Presets})

	stored := []models.ServicePreset{{
		ServiceContentItem: models.ServiceContentItem{ID: "1", Name: "Stored Station", Source: "TUNEIN", Location: "/v1/playback/station/s1"},
	}}
	if err := env.ds.SavePresets("default", i780Device, stored); err != nil {
		t.Fatalf("SavePresets: %v", err)
	}

	env.pair(t)

	presets, err := env.ds.GetPresets(i780Account, i780Device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if len(presets) != 1 || presets[0].Name != "Stored Station" {
		t.Errorf("presets = %+v, want the stored one", presets)
	}
}

func TestPairAccountOverTelnetFilesOnlyOnAConfirmedReply_GH780(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  string
	}{
		{name: "OK", reply: "envswitch accountid set " + i780Account + "\r\nOK\r\n", want: i780Account},
		{name: "rejected option", reply: "Invalid Command Option\r\n", want: "default"},
		{name: "no reply", reply: "", want: "default"},
		{name: "OK only inside a word", reply: "BOOKED\r\n", want: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newI780Env(t, &i780Speaker{presets: i780Presets})
			env.srv.sm.NewTelnet = func(string) setup.TelnetClient { return &i780Telnet{reply: tt.reply} }

			env.pair(t)

			if got := env.filedUnder(t); got != tt.want {
				t.Errorf("filed under %q, want %q", got, tt.want)
			}
		})
	}
}
