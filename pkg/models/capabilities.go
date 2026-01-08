package models

import "encoding/xml"

// Capabilities represents the response from /capabilities endpoint
type Capabilities struct {
	XMLName       xml.Name       `xml:"capabilities"`
	DeviceID      string         `xml:"deviceID,attr"`
	NetworkConfig *NetworkConfig `xml:"networkConfig,omitempty"`
	DSPConfig     *DSPConfig     `xml:"dspCapabilities,omitempty"`
	Lightswitch   bool           `xml:"lightswitch,omitempty"`
	ClockDisplay  bool           `xml:"clockDisplay,omitempty"`
	Capability    []Capability   `xml:"capability,omitempty"`
	LRStereo      bool           `xml:"lrStereoCapable,omitempty"`
	BCOReset      bool           `xml:"bcoresetCapable,omitempty"`
	PowerSaving   bool           `xml:"disablePowerSaving,omitempty"`
}

// NetworkConfig represents network configuration capabilities
type NetworkConfig struct {
	HostedWifiConfig    *HostedWifiConfig `xml:"hostedWifiConfigWebPage,omitempty"`
	DualMode            bool              `xml:"dualMode,omitempty"`
	WSAPIProxy          bool              `xml:"wsapiproxy,omitempty"`
	AllInterfaceSupport *AllInterfaces    `xml:"allInterfacesSupported,omitempty"`
	WLANInterfaces      *WLANInterfaces   `xml:"wlanInterfaces,omitempty"`
	Security            *Security         `xml:"security,omitempty"`
}

// HostedWifiConfig represents hosted wifi configuration settings
type HostedWifiConfig struct {
	HostedBy   string `xml:"hostedBy,attr,omitempty"`
	Generation string `xml:"generation,attr,omitempty"`
	Port       string `xml:"port,attr,omitempty"`
	Enabled    bool   `xml:",chardata"`
}

// AllInterfaces represents all network interfaces support
type AllInterfaces struct{}

// WLANInterfaces represents WLAN interfaces configuration
type WLANInterfaces struct{}

// Security represents security configuration
type Security struct{}

// DSPConfig represents DSP capabilities
type DSPConfig struct {
	DSPMonoStereo *DSPMonoStereo `xml:"dspMonoStereo,omitempty"`
}

// DSPMonoStereo represents mono/stereo DSP capability
type DSPMonoStereo struct {
	Available bool `xml:"available,attr"`
}

// Capability represents an individual device capability
type Capability struct {
	Name string `xml:"name,attr"`
	URL  string `xml:"url,attr"`
	Info string `xml:"info,attr"`
}

// HasLightswitch returns true if the device has a light switch
func (c *Capabilities) HasLightswitch() bool {
	return c.Lightswitch
}

// HasClockDisplay returns true if the device has a clock display
func (c *Capabilities) HasClockDisplay() bool {
	return c.ClockDisplay
}

// HasLRStereoCapability returns true if the device supports left/right stereo
func (c *Capabilities) HasLRStereoCapability() bool {
	return c.LRStereo
}

// HasBCOResetCapability returns true if the device supports BCO reset
func (c *Capabilities) HasBCOResetCapability() bool {
	return c.BCOReset
}

// HasPowerSavingDisabled returns true if power saving is disabled
func (c *Capabilities) HasPowerSavingDisabled() bool {
	return c.PowerSaving
}

// HasDualModeNetwork returns true if the device supports dual mode networking
func (c *Capabilities) HasDualModeNetwork() bool {
	return c.NetworkConfig != nil && c.NetworkConfig.DualMode
}

// HasWSAPIProxy returns true if the device supports WSAPI proxy
func (c *Capabilities) HasWSAPIProxy() bool {
	return c.NetworkConfig != nil && c.NetworkConfig.WSAPIProxy
}

// HasHostedWifiConfig returns true if the device supports hosted wifi configuration
func (c *Capabilities) HasHostedWifiConfig() bool {
	return c.NetworkConfig != nil &&
		c.NetworkConfig.HostedWifiConfig != nil &&
		c.NetworkConfig.HostedWifiConfig.Enabled
}

// GetHostedWifiPort returns the hosted wifi configuration port
func (c *Capabilities) GetHostedWifiPort() string {
	if c.HasHostedWifiConfig() {
		return c.NetworkConfig.HostedWifiConfig.Port
	}
	return ""
}

// GetHostedWifiHostedBy returns who hosts the wifi configuration
func (c *Capabilities) GetHostedWifiHostedBy() string {
	if c.HasHostedWifiConfig() {
		return c.NetworkConfig.HostedWifiConfig.HostedBy
	}
	return ""
}

// HasDSPMonoStereo returns true if DSP mono/stereo is available
func (c *Capabilities) HasDSPMonoStereo() bool {
	return c.DSPConfig != nil &&
		c.DSPConfig.DSPMonoStereo != nil &&
		c.DSPConfig.DSPMonoStereo.Available
}

// GetCapabilityByName returns a capability by name
func (c *Capabilities) GetCapabilityByName(name string) *Capability {
	for _, cap := range c.Capability {
		if cap.Name == name {
			return &cap
		}
	}
	return nil
}

// HasCapability returns true if the device has the specified capability
func (c *Capabilities) HasCapability(name string) bool {
	return c.GetCapabilityByName(name) != nil
}

// GetCapabilityNames returns a list of all capability names
func (c *Capabilities) GetCapabilityNames() []string {
	names := make([]string, len(c.Capability))
	for i, cap := range c.Capability {
		names[i] = cap.Name
	}
	return names
}

// GetNetworkCapabilities returns a summary of network capabilities
func (c *Capabilities) GetNetworkCapabilities() []string {
	var capabilities []string

	if c.HasDualModeNetwork() {
		capabilities = append(capabilities, "Dual Mode")
	}
	if c.HasWSAPIProxy() {
		capabilities = append(capabilities, "WSAPI Proxy")
	}
	if c.HasHostedWifiConfig() {
		capabilities = append(capabilities, "Hosted WiFi Config")
	}

	return capabilities
}

// GetAudioCapabilities returns a summary of audio capabilities
func (c *Capabilities) GetAudioCapabilities() []string {
	var capabilities []string

	if c.HasLRStereoCapability() {
		capabilities = append(capabilities, "L/R Stereo")
	}
	if c.HasDSPMonoStereo() {
		capabilities = append(capabilities, "DSP Mono/Stereo")
	}

	return capabilities
}

// GetSystemCapabilities returns a summary of system capabilities
func (c *Capabilities) GetSystemCapabilities() []string {
	var capabilities []string

	if c.HasLightswitch() {
		capabilities = append(capabilities, "Light Switch")
	}
	if c.HasClockDisplay() {
		capabilities = append(capabilities, "Clock Display")
	}
	if c.HasBCOResetCapability() {
		capabilities = append(capabilities, "BCO Reset")
	}
	if c.HasPowerSavingDisabled() {
		capabilities = append(capabilities, "Power Saving Disabled")
	}

	return capabilities
}
