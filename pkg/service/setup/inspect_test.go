package setup

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// inspectFakes wires canned XML bodies into Manager.HTTPGet keyed by URL
// path. A missing path returns 404; an empty-string body returns the
// supplied err.
type inspectFakes struct {
	responses map[string]string
	errs      map[string]error
}

func (f *inspectFakes) get(url string) (*http.Response, error) {
	for path, body := range f.responses {
		if strings.HasSuffix(url, path) {
			if e := f.errs[path]; e != nil {
				return nil, e
			}

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{},
			}, nil
		}
	}

	return &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(strings.NewReader("not found")),
		Header:     http.Header{},
	}, nil
}

func TestInspect_HappyPath(t *testing.T) {
	f := &inspectFakes{
		responses: map[string]string{
			"/info": `<info deviceID="506583DE4803">
  <name>Bose SoundTouch DE4803</name>
  <type>SoundTouch 10</type>
  <margeAccountUUID>1234567</margeAccountUUID>
  <margeURL>http://aftertouch.local:8000</margeURL>
  <components>
    <component>
      <componentCategory>SCM</componentCategory>
      <softwareVersion>27.0.6</softwareVersion>
      <serialNumber>F23456789012</serialNumber>
    </component>
  </components>
</info>`,
			"/networkInfo": `<networkInfo wifiProfileCount="1">
  <interfaces>
    <interface type="WIFI_INTERFACE" name="wlan0" macAddress="aa:bb:cc:dd:ee:ff" ipAddress="192.168.1.42" ssid="MyHomeNetwork" frequencyKHz="2452000" state="NETWORK_WIFI_CONNECTED" signal="GOOD_SIGNAL" mode="STATION"/>
  </interfaces>
</networkInfo>`,
			"/sources": `<sources><sourceItem source="TUNEIN" status="READY"/><sourceItem source="SPOTIFY" sourceAccount="user@example.com" status="READY"/></sources>`,
			"/presets": `<presets><preset id="1"><ContentItem source="TUNEIN" type="stationurl"><itemName>1LIVE</itemName></ContentItem></preset></presets>`,
		},
	}

	m := &Manager{HTTPGet: f.get}

	r := m.Inspect("192.168.1.42", InspectOptions{})

	if r.InfoErr != nil {
		t.Errorf("InfoErr = %v, want nil", r.InfoErr)
	}

	if r.Info == nil || r.Info.DeviceID != "506583DE4803" {
		t.Errorf("Info.DeviceID = %v, want 506583DE4803", r.Info)
	}

	if r.Info.MargeAccountUUID != "1234567" {
		t.Errorf("MargeAccountUUID = %q, want 1234567", r.Info.MargeAccountUUID)
	}

	if r.Network == nil || len(r.Network.Interfaces.Interfaces) == 0 {
		t.Fatalf("Network parse failed: %v / %v", r.Network, r.NetworkErr)
	}

	wifi := r.Network.Interfaces.Interfaces[0]
	if wifi.SSID != "MyHomeNetwork" {
		t.Errorf("SSID = %q, want MyHomeNetwork", wifi.SSID)
	}

	if r.Sources == nil || len(r.Sources.SourceItem) != 2 {
		t.Errorf("Sources = %v, want 2 entries", r.Sources)
	}

	if r.Presets == nil || len(r.Presets.Presets) != 1 {
		t.Errorf("Presets = %v, want 1 preset", r.Presets)
	}

	if r.Presets.Presets[0].ContentItem.ItemName != "1LIVE" {
		t.Errorf("preset name = %q, want 1LIVE", r.Presets.Presets[0].ContentItem.ItemName)
	}
}

func TestInspect_PartialFailureRecordsPerSectionErrors(t *testing.T) {
	// /info ok, /presets returns network error — the rest of the report
	// must still populate.
	f := &inspectFakes{
		responses: map[string]string{
			"/info":        `<info deviceID="X"><name>n</name></info>`,
			"/networkInfo": `<networkInfo><interfaces></interfaces></networkInfo>`,
			"/sources":     `<sources/>`,
			"/presets":     "", // body unused — errs map below triggers error
		},
		errs: map[string]error{
			"/presets": errors.New("connection reset"),
		},
	}

	m := &Manager{HTTPGet: f.get}

	r := m.Inspect("192.168.1.42", InspectOptions{})

	if r.InfoErr != nil {
		t.Errorf("InfoErr = %v, want nil", r.InfoErr)
	}

	if r.PresetsErr == nil {
		t.Error("expected PresetsErr to be populated")
	}

	if r.Sources == nil {
		t.Error("Sources should still populate despite PresetsErr")
	}
}

func TestInspect_TelnetRuntimeURLs(t *testing.T) {
	f := &inspectFakes{
		responses: map[string]string{
			"/info": `<info deviceID="X"><name>n</name></info>`,
		},
	}

	tn := &fakeTelnet{
		responses: map[string]string{
			"getpdo CurrentSystemConfiguration": "margeServerUrl: http://aftertouch.local:8000\nstatsServerUrl: http://aftertouch.local:8000\n",
		},
	}

	m := &Manager{
		HTTPGet:   f.get,
		NewTelnet: func(string) TelnetClient { return tn },
	}

	r := m.Inspect("192.168.1.42", InspectOptions{IncludeTelnet: true})

	if r.RuntimeErr != nil {
		t.Errorf("RuntimeErr = %v, want nil", r.RuntimeErr)
	}

	if !strings.Contains(r.RuntimeURLs, "margeServerUrl") {
		t.Errorf("RuntimeURLs missing expected content: %q", r.RuntimeURLs)
	}
}

func TestInspect_TelnetSkippedWhenOptionDisabled(t *testing.T) {
	f := &inspectFakes{
		responses: map[string]string{
			"/info": `<info deviceID="X"><name>n</name></info>`,
		},
	}

	m := &Manager{HTTPGet: f.get}

	r := m.Inspect("192.168.1.42", InspectOptions{IncludeTelnet: false})

	if r.RuntimeURLs != "" || r.RuntimeErr != nil {
		t.Errorf("telnet runtime fields should be zero when IncludeTelnet=false, got %q / %v",
			r.RuntimeURLs, r.RuntimeErr)
	}
}
