package setup

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// InspectOptions controls how Manager.Inspect probes the speaker.
type InspectOptions struct {
	// IncludeTelnet runs `getpdo CurrentSystemConfiguration` over telnet to
	// capture the speaker's runtime URL configuration. Slower and not
	// always reachable on hardened firmware, hence opt-in.
	IncludeTelnet bool
}

// InspectSection is one slice of an InspectReport. Each section has an
// independent error so a partial failure (e.g. /presets refused) does not
// hide the rest of the report.
type InspectSection struct {
	Name string
	Err  error
}

// InspectReport summarises everything we can learn about a speaker
// without writing to it. Used to populate UI before factory-reset / pair
// flows and to record the deviceID-suffix for later wait-online calls.
type InspectReport struct {
	DeviceIP string

	Info        *DeviceInfoXML             `json:"info,omitempty"`
	InfoErr     error                      `json:"-"`
	Network     *models.NetworkInformation `json:"network,omitempty"`
	NetworkErr  error                      `json:"-"`
	Sources     *models.Sources            `json:"sources,omitempty"`
	SourcesErr  error                      `json:"-"`
	Presets     *PresetList                `json:"presets,omitempty"`
	PresetsErr  error                      `json:"-"`
	RuntimeURLs string                     `json:"runtime_urls,omitempty"`
	RuntimeErr  error                      `json:"-"`
}

// PresetList is a minimal preset summary — just enough to render a
// "preset N: <name>" overview. The full preset model lives in
// pkg/models, but we don't need it here.
type PresetList struct {
	XMLName xml.Name `xml:"presets"`
	Presets []struct {
		ID          string `xml:"id,attr"`
		ContentItem struct {
			Source        string `xml:"source,attr"`
			SourceAccount string `xml:"sourceAccount,attr"`
			Type          string `xml:"type,attr"`
			ItemName      string `xml:"itemName"`
		} `xml:"ContentItem"`
	} `xml:"preset"`
}

// Inspect gathers a non-destructive snapshot of the speaker at deviceIP.
// Every probe is best-effort: individual section errors are recorded on
// the report rather than aborting the whole call.
func (m *Manager) Inspect(deviceIP string, opts InspectOptions) *InspectReport {
	r := &InspectReport{DeviceIP: deviceIP}

	r.Info, r.InfoErr = m.GetLiveDeviceInfo(deviceIP)
	r.Network, r.NetworkErr = m.fetchNetworkInfo(deviceIP)
	r.Sources, r.SourcesErr = m.fetchSources(deviceIP)
	r.Presets, r.PresetsErr = m.fetchPresets(deviceIP)

	if opts.IncludeTelnet {
		r.RuntimeURLs, r.RuntimeErr = m.fetchRuntimeURLs(deviceIP)
	}

	return r
}

func (m *Manager) fetchXML(deviceIP, path string, out any) error {
	url := buildDeviceURL(deviceIP, path)

	resp, err := m.HTTPGet(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s: %w", url, err)
	}

	return xml.Unmarshal(body, out)
}

func (m *Manager) fetchNetworkInfo(deviceIP string) (*models.NetworkInformation, error) {
	var n models.NetworkInformation
	if err := m.fetchXML(deviceIP, "/networkInfo", &n); err != nil {
		return nil, err
	}

	return &n, nil
}

func (m *Manager) fetchSources(deviceIP string) (*models.Sources, error) {
	var s models.Sources
	if err := m.fetchXML(deviceIP, "/sources", &s); err != nil {
		return nil, err
	}

	return &s, nil
}

func (m *Manager) fetchPresets(deviceIP string) (*PresetList, error) {
	var p PresetList
	if err := m.fetchXML(deviceIP, "/presets", &p); err != nil {
		return nil, err
	}

	return &p, nil
}

// fetchRuntimeURLs reads the device's runtime URL configuration via the
// port-17000 diagnostic shell. The response is a multi-line text blob —
// we return it as-is so the caller can decide how to format it.
func (m *Manager) fetchRuntimeURLs(deviceIP string) (string, error) {
	if m.NewTelnet == nil {
		return "", errors.New("telnet probe disabled: Manager.NewTelnet is nil")
	}

	t := m.NewTelnet(deviceIP)
	if err := t.Dial(); err != nil {
		return "", fmt.Errorf("telnet dial %s:17000: %w", deviceIP, err)
	}

	defer func() { _ = t.Close() }()

	_, _ = t.Probe()

	resp, err := t.SendCommand("getpdo CurrentSystemConfiguration")
	if err != nil {
		return "", fmt.Errorf("getpdo: %w", err)
	}

	if isCommandNotFound(resp) {
		return "", errors.New("device rejected `getpdo` (firmware does not expose this command)")
	}

	return strings.TrimRight(resp, "\r\n"), nil
}
