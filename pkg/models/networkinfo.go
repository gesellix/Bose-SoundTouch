package models

import (
	"encoding/xml"
	"fmt"
	"net"
	"strings"
)

// NetworkInformation represents network information from the /networkInfo endpoint
type NetworkInformation struct {
	XMLName          xml.Name          `xml:"networkInfo"`
	WifiProfileCount int               `xml:"wifiProfileCount,attr,omitempty"`
	Interfaces       NetworkInterfaces `xml:"interfaces"`
}

// NetworkInterfaces represents the interfaces container
type NetworkInterfaces struct {
	XMLName    xml.Name           `xml:"interfaces"`
	Interfaces []NetworkInterface `xml:"interface"`
}

// NetworkInterface represents a single network interface
type NetworkInterface struct {
	XMLName      xml.Name `xml:"interface"`
	Type         string   `xml:"type,attr"`
	Name         string   `xml:"name,attr,omitempty"`
	MacAddress   string   `xml:"macAddress,attr,omitempty"`
	IPAddress    string   `xml:"ipAddress,attr,omitempty"`
	SSID         string   `xml:"ssid,attr,omitempty"`
	FrequencyKHz int      `xml:"frequencyKHz,attr,omitempty"`
	State        string   `xml:"state,attr,omitempty"`
	Signal       string   `xml:"signal,attr,omitempty"`
	Mode         string   `xml:"mode,attr,omitempty"`
}

// Interface types
const (
	InterfaceTypeWiFi     = "WIFI_INTERFACE"
	InterfaceTypeEthernet = "ETHERNET_INTERFACE"
)

// Interface states
const (
	StateWiFiConnected        = "NETWORK_WIFI_CONNECTED"
	StateWiFiDisconnected     = "NETWORK_WIFI_DISCONNECTED"
	StateEthernetConnected    = "NETWORK_ETHERNET_CONNECTED"
	StateEthernetDisconnected = "NETWORK_ETHERNET_DISCONNECTED"
)

// Signal strength levels
const (
	SignalExcellent = "EXCELLENT_SIGNAL"
	SignalGood      = "GOOD_SIGNAL"
	SignalFair      = "FAIR_SIGNAL"
	SignalPoor      = "POOR_SIGNAL"
)

// WiFi modes
const (
	ModeStation     = "STATION"
	ModeAccessPoint = "ACCESS_POINT"
)

// GetWifiProfileCount returns the number of WiFi profiles
func (n *NetworkInformation) GetWifiProfileCount() int {
	return n.WifiProfileCount
}

// GetInterfaces returns all network interfaces
func (n *NetworkInformation) GetInterfaces() []NetworkInterface {
	return n.Interfaces.Interfaces
}

// GetInterfaceByType returns the first interface of the specified type
func (n *NetworkInformation) GetInterfaceByType(interfaceType string) *NetworkInterface {
	for _, iface := range n.GetInterfaces() {
		if strings.EqualFold(iface.Type, interfaceType) {
			return &iface
		}
	}
	return nil
}

// GetActiveInterfaces returns only active/connected interfaces
func (n *NetworkInformation) GetActiveInterfaces() []NetworkInterface {
	var active []NetworkInterface
	for _, iface := range n.GetInterfaces() {
		if iface.IsConnected() {
			active = append(active, iface)
		}
	}
	return active
}

// HasWiFi returns true if the device has WiFi connectivity
func (n *NetworkInformation) HasWiFi() bool {
	return n.GetInterfaceByType(InterfaceTypeWiFi) != nil
}

// HasEthernet returns true if the device has Ethernet connectivity
func (n *NetworkInformation) HasEthernet() bool {
	return n.GetInterfaceByType(InterfaceTypeEthernet) != nil
}

// GetConnectedWiFiInterface returns the connected WiFi interface if available
func (n *NetworkInformation) GetConnectedWiFiInterface() *NetworkInterface {
	for _, iface := range n.GetInterfaces() {
		if iface.IsWiFi() && iface.IsConnected() {
			return &iface
		}
	}
	return nil
}

// GetConnectedEthernetInterface returns the connected Ethernet interface if available
func (n *NetworkInformation) GetConnectedEthernetInterface() *NetworkInterface {
	for _, iface := range n.GetInterfaces() {
		if iface.IsEthernet() && iface.IsConnected() {
			return &iface
		}
	}
	return nil
}

// IsEmpty returns true if there's no network information
func (n *NetworkInformation) IsEmpty() bool {
	return len(n.GetInterfaces()) == 0
}

// Interface methods

// GetType returns the interface type
func (ni *NetworkInterface) GetType() string {
	return ni.Type
}

// GetName returns the interface name
func (ni *NetworkInterface) GetName() string {
	return ni.Name
}

// GetMacAddress returns the MAC address
func (ni *NetworkInterface) GetMacAddress() string {
	return ni.MacAddress
}

// GetIPAddress returns the IP address
func (ni *NetworkInterface) GetIPAddress() string {
	return ni.IPAddress
}

// GetSSID returns the WiFi SSID (only for WiFi interfaces)
func (ni *NetworkInterface) GetSSID() string {
	return ni.SSID
}

// GetFrequencyKHz returns the WiFi frequency in kHz
func (ni *NetworkInterface) GetFrequencyKHz() int {
	return ni.FrequencyKHz
}

// GetFrequencyGHz returns the WiFi frequency in GHz
func (ni *NetworkInterface) GetFrequencyGHz() float64 {
	if ni.FrequencyKHz == 0 {
		return 0
	}
	return float64(ni.FrequencyKHz) / 1000000.0
}

// GetFrequencyBand returns the WiFi frequency band (2.4GHz or 5GHz)
func (ni *NetworkInterface) GetFrequencyBand() string {
	ghz := ni.GetFrequencyGHz()
	if ghz == 0 {
		return ""
	}
	if ghz < 3.0 {
		return "2.4GHz"
	}
	return "5GHz"
}

// GetState returns the interface state
func (ni *NetworkInterface) GetState() string {
	return ni.State
}

// GetSignal returns the signal strength
func (ni *NetworkInterface) GetSignal() string {
	return ni.Signal
}

// GetMode returns the WiFi mode
func (ni *NetworkInterface) GetMode() string {
	return ni.Mode
}

// IsConnected returns true if the interface is connected
func (ni *NetworkInterface) IsConnected() bool {
	state := ni.GetState()
	return state == StateWiFiConnected || state == StateEthernetConnected
}

// IsDisconnected returns true if the interface is disconnected
func (ni *NetworkInterface) IsDisconnected() bool {
	state := ni.GetState()
	return state == StateWiFiDisconnected || state == StateEthernetDisconnected
}

// IsWiFi returns true if this is a WiFi interface
func (ni *NetworkInterface) IsWiFi() bool {
	return ni.Type == InterfaceTypeWiFi
}

// IsEthernet returns true if this is an Ethernet interface
func (ni *NetworkInterface) IsEthernet() bool {
	return ni.Type == InterfaceTypeEthernet
}

// ValidateIP returns true if the IP address is valid
func (ni *NetworkInterface) ValidateIP() bool {
	if ni.IPAddress == "" {
		return false
	}
	return net.ParseIP(ni.IPAddress) != nil
}

// ValidateMAC returns true if the MAC address is valid
func (ni *NetworkInterface) ValidateMAC() bool {
	if ni.MacAddress == "" {
		return false
	}
	_, err := net.ParseMAC(ni.MacAddress)
	return err == nil
}

// HasWiFiInfo returns true if the interface has WiFi-specific information
func (ni *NetworkInterface) HasWiFiInfo() bool {
	return ni.SSID != "" || ni.FrequencyKHz > 0 || ni.Signal != ""
}

// GetSignalQuality returns a normalized signal quality percentage (0-100)
func (ni *NetworkInterface) GetSignalQuality() int {
	switch ni.Signal {
	case SignalExcellent:
		return 90
	case SignalGood:
		return 70
	case SignalFair:
		return 50
	case SignalPoor:
		return 30
	default:
		return 0
	}
}

// GetSignalDescription returns a user-friendly signal description
func (ni *NetworkInterface) GetSignalDescription() string {
	switch ni.Signal {
	case SignalExcellent:
		return "Excellent"
	case SignalGood:
		return "Good"
	case SignalFair:
		return "Fair"
	case SignalPoor:
		return "Poor"
	default:
		return "Unknown"
	}
}

// GetStateDescription returns a user-friendly state description
func (ni *NetworkInterface) GetStateDescription() string {
	switch ni.State {
	case StateWiFiConnected:
		return "WiFi Connected"
	case StateWiFiDisconnected:
		return "WiFi Disconnected"
	case StateEthernetConnected:
		return "Ethernet Connected"
	case StateEthernetDisconnected:
		return "Ethernet Disconnected"
	default:
		return "Unknown State"
	}
}

// GetModeDescription returns a user-friendly mode description
func (ni *NetworkInterface) GetModeDescription() string {
	switch ni.Mode {
	case ModeStation:
		return "Station (Client)"
	case ModeAccessPoint:
		return "Access Point"
	default:
		return "Unknown Mode"
	}
}

// FormatFrequency returns a formatted frequency string
func (ni *NetworkInterface) FormatFrequency() string {
	if ni.FrequencyKHz == 0 {
		return ""
	}

	ghz := ni.GetFrequencyGHz()
	if ghz >= 1.0 {
		return fmt.Sprintf("%.1f GHz", ghz)
	}

	mhz := float64(ni.FrequencyKHz) / 1000.0
	return fmt.Sprintf("%.0f MHz", mhz)
}

// GetNetworkSummary returns a summary string of the network interface
func (ni *NetworkInterface) GetNetworkSummary() string {
	if ni.IsWiFi() && ni.IsConnected() {
		ssid := ni.GetSSID()
		if ssid == "" {
			ssid = "Hidden Network"
		}
		signal := ni.GetSignalDescription()
		band := ni.GetFrequencyBand()
		if band != "" {
			return fmt.Sprintf("%s (%s, %s)", ssid, signal, band)
		}
		return fmt.Sprintf("%s (%s)", ssid, signal)
	} else if ni.IsEthernet() && ni.IsConnected() {
		return "Wired Connection"
	} else if ni.IsConnected() {
		return "Connected"
	}
	return "Disconnected"
}

// String returns a string representation of the network interface
func (ni *NetworkInterface) String() string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Type: %s", ni.GetType()))

	if ni.GetName() != "" {
		parts = append(parts, fmt.Sprintf("Name: %s", ni.GetName()))
	}

	if ni.GetIPAddress() != "" {
		parts = append(parts, fmt.Sprintf("IP: %s", ni.GetIPAddress()))
	}

	if ni.GetMacAddress() != "" {
		parts = append(parts, fmt.Sprintf("MAC: %s", ni.GetMacAddress()))
	}

	if ni.IsWiFi() && ni.GetSSID() != "" {
		parts = append(parts, fmt.Sprintf("SSID: %s", ni.GetSSID()))
	}

	if ni.GetState() != "" {
		parts = append(parts, fmt.Sprintf("State: %s", ni.GetStateDescription()))
	}

	return strings.Join(parts, ", ")
}
