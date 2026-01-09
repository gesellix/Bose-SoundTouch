package models

import (
	"encoding/xml"
	"testing"
)

func TestNetworkInformation_UnmarshalXML(t *testing.T) {
	tests := []struct {
		name     string
		xmlData  string
		expected NetworkInformation
	}{
		{
			name: "WiFi device with connected and disconnected interfaces",
			xmlData: `<networkInfo wifiProfileCount="2">
<interfaces>
<interface type="WIFI_INTERFACE" name="wlan0" macAddress="AA:BB:CC:DD:EE:FF" ipAddress="192.168.1.10" ssid="MyHomeNetwork" frequencyKHz="5500000" state="NETWORK_WIFI_CONNECTED" signal="EXCELLENT_SIGNAL" mode="STATION"/>
<interface type="WIFI_INTERFACE" name="wlan1" macAddress="AA:BB:CC:DD:EE:01" state="NETWORK_WIFI_DISCONNECTED"/>
</interfaces>
</networkInfo>`,
			expected: NetworkInformation{
				WifiProfileCount: 2,
				Interfaces: NetworkInterfaces{
					Interfaces: []NetworkInterface{
						{
							Type:         "WIFI_INTERFACE",
							Name:         "wlan0",
							MacAddress:   "AA:BB:CC:DD:EE:FF",
							IPAddress:    "192.168.1.10",
							SSID:         "MyHomeNetwork",
							FrequencyKHz: 5500000,
							State:        "NETWORK_WIFI_CONNECTED",
							Signal:       "EXCELLENT_SIGNAL",
							Mode:         "STATION",
						},
						{
							Type:       "WIFI_INTERFACE",
							Name:       "wlan1",
							MacAddress: "AA:BB:CC:DD:EE:01",
							State:      "NETWORK_WIFI_DISCONNECTED",
						},
					},
				},
			},
		},
		{
			name: "Ethernet device",
			xmlData: `<networkInfo wifiProfileCount="3">
<interfaces>
<interface type="ETHERNET_INTERFACE" name="eth0" macAddress="AA:BB:CC:DD:EE:FF" ipAddress="192.168.1.10" state="NETWORK_ETHERNET_CONNECTED"/>
</interfaces>
</networkInfo>`,
			expected: NetworkInformation{
				WifiProfileCount: 3,
				Interfaces: NetworkInterfaces{
					Interfaces: []NetworkInterface{
						{
							Type:       "ETHERNET_INTERFACE",
							Name:       "eth0",
							MacAddress: "AA:BB:CC:DD:EE:FF",
							IPAddress:  "192.168.1.10",
							State:      "NETWORK_ETHERNET_CONNECTED",
						},
					},
				},
			},
		},
		{
			name: "Empty interfaces",
			xmlData: `<networkInfo wifiProfileCount="0">
<interfaces>
</interfaces>
</networkInfo>`,
			expected: NetworkInformation{
				WifiProfileCount: 0,
				Interfaces: NetworkInterfaces{
					Interfaces: []NetworkInterface{},
				},
			},
		},
		{
			name: "Multiple WiFi interfaces with different signals",
			xmlData: `<networkInfo wifiProfileCount="1">
<interfaces>
<interface type="WIFI_INTERFACE" name="wlan0" macAddress="AA:BB:CC:DD:EE:FF" ipAddress="192.168.1.10" ssid="StrongNetwork" frequencyKHz="2400000" state="NETWORK_WIFI_CONNECTED" signal="GOOD_SIGNAL" mode="STATION"/>
<interface type="WIFI_INTERFACE" name="wlan1" macAddress="AA:BB:CC:DD:EE:01" ssid="WeakNetwork" frequencyKHz="5200000" state="NETWORK_WIFI_CONNECTED" signal="POOR_SIGNAL" mode="STATION"/>
</interfaces>
</networkInfo>`,
			expected: NetworkInformation{
				WifiProfileCount: 1,
				Interfaces: NetworkInterfaces{
					Interfaces: []NetworkInterface{
						{
							Type:         "WIFI_INTERFACE",
							Name:         "wlan0",
							MacAddress:   "AA:BB:CC:DD:EE:FF",
							IPAddress:    "192.168.1.10",
							SSID:         "StrongNetwork",
							FrequencyKHz: 2400000,
							State:        "NETWORK_WIFI_CONNECTED",
							Signal:       "GOOD_SIGNAL",
							Mode:         "STATION",
						},
						{
							Type:         "WIFI_INTERFACE",
							Name:         "wlan1",
							MacAddress:   "AA:BB:CC:DD:EE:01",
							SSID:         "WeakNetwork",
							FrequencyKHz: 5200000,
							State:        "NETWORK_WIFI_CONNECTED",
							Signal:       "POOR_SIGNAL",
							Mode:         "STATION",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var networkInfo NetworkInformation
			err := xml.Unmarshal([]byte(tt.xmlData), &networkInfo)
			if err != nil {
				t.Fatalf("Failed to unmarshal XML: %v", err)
			}

			if networkInfo.WifiProfileCount != tt.expected.WifiProfileCount {
				t.Errorf("Expected WifiProfileCount %d, got %d", tt.expected.WifiProfileCount, networkInfo.WifiProfileCount)
			}

			interfaces := networkInfo.GetInterfaces()
			expectedInterfaces := tt.expected.Interfaces.Interfaces

			if len(interfaces) != len(expectedInterfaces) {
				t.Errorf("Expected %d interfaces, got %d", len(expectedInterfaces), len(interfaces))
				return
			}

			for i, iface := range interfaces {
				if i >= len(expectedInterfaces) {
					break
				}
				expectedIface := expectedInterfaces[i]

				if iface.Type != expectedIface.Type {
					t.Errorf("Interface %d: Expected Type %q, got %q", i, expectedIface.Type, iface.Type)
				}
				if iface.Name != expectedIface.Name {
					t.Errorf("Interface %d: Expected Name %q, got %q", i, expectedIface.Name, iface.Name)
				}
				if iface.MacAddress != expectedIface.MacAddress {
					t.Errorf("Interface %d: Expected MacAddress %q, got %q", i, expectedIface.MacAddress, iface.MacAddress)
				}
				if iface.IPAddress != expectedIface.IPAddress {
					t.Errorf("Interface %d: Expected IPAddress %q, got %q", i, expectedIface.IPAddress, iface.IPAddress)
				}
				if iface.SSID != expectedIface.SSID {
					t.Errorf("Interface %d: Expected SSID %q, got %q", i, expectedIface.SSID, iface.SSID)
				}
				if iface.State != expectedIface.State {
					t.Errorf("Interface %d: Expected State %q, got %q", i, expectedIface.State, iface.State)
				}
			}
		})
	}
}

func TestNetworkInformation_GetInterfaceByType(t *testing.T) {
	networkInfo := NetworkInformation{
		Interfaces: NetworkInterfaces{
			Interfaces: []NetworkInterface{
				{Type: InterfaceTypeWiFi, MacAddress: "AA:BB:CC:DD:EE:FF", State: StateWiFiConnected},
				{Type: InterfaceTypeEthernet, MacAddress: "11:22:33:44:55:66", State: StateEthernetConnected},
			},
		},
	}

	tests := []struct {
		name          string
		interfaceType string
		expectFound   bool
		expectedMAC   string
	}{
		{
			name:          "Find WiFi interface",
			interfaceType: InterfaceTypeWiFi,
			expectFound:   true,
			expectedMAC:   "AA:BB:CC:DD:EE:FF",
		},
		{
			name:          "Find Ethernet interface",
			interfaceType: InterfaceTypeEthernet,
			expectFound:   true,
			expectedMAC:   "11:22:33:44:55:66",
		},
		{
			name:          "Case insensitive search",
			interfaceType: "wifi_interface",
			expectFound:   true,
			expectedMAC:   "AA:BB:CC:DD:EE:FF",
		},
		{
			name:          "Interface not found",
			interfaceType: "BLUETOOTH_INTERFACE",
			expectFound:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iface := networkInfo.GetInterfaceByType(tt.interfaceType)

			if tt.expectFound {
				if iface == nil {
					t.Error("Expected to find interface, got nil")
					return
				}
				if iface.MacAddress != tt.expectedMAC {
					t.Errorf("Expected MAC %q, got %q", tt.expectedMAC, iface.MacAddress)
				}
			} else {
				if iface != nil {
					t.Error("Expected not to find interface, got non-nil")
				}
			}
		})
	}
}

func TestNetworkInformation_GetActiveInterfaces(t *testing.T) {
	networkInfo := NetworkInformation{
		Interfaces: NetworkInterfaces{
			Interfaces: []NetworkInterface{
				{Type: InterfaceTypeWiFi, State: StateWiFiConnected, SSID: "ConnectedWiFi"},
				{Type: InterfaceTypeWiFi, State: StateWiFiDisconnected, SSID: "DisconnectedWiFi"},
				{Type: InterfaceTypeEthernet, State: StateEthernetConnected},
				{Type: InterfaceTypeEthernet, State: StateEthernetDisconnected},
			},
		},
	}

	activeInterfaces := networkInfo.GetActiveInterfaces()

	expectedActiveCount := 2 // One WiFi connected, one Ethernet connected
	if len(activeInterfaces) != expectedActiveCount {
		t.Errorf("Expected %d active interfaces, got %d", expectedActiveCount, len(activeInterfaces))
	}

	// Check that the correct interfaces are marked as active
	activeStates := make(map[string]bool)
	for _, iface := range activeInterfaces {
		activeStates[iface.State] = true
	}

	expectedActiveStates := []string{StateWiFiConnected, StateEthernetConnected}
	for _, expectedState := range expectedActiveStates {
		if !activeStates[expectedState] {
			t.Errorf("Expected %s interface to be active", expectedState)
		}
	}
}

func TestNetworkInformation_HasConnectivity(t *testing.T) {
	tests := []struct {
		name        string
		networkInfo NetworkInformation
		hasWiFi     bool
		hasEthernet bool
	}{
		{
			name: "Has both WiFi and Ethernet",
			networkInfo: NetworkInformation{
				Interfaces: NetworkInterfaces{
					Interfaces: []NetworkInterface{
						{Type: InterfaceTypeWiFi},
						{Type: InterfaceTypeEthernet},
					},
				},
			},
			hasWiFi:     true,
			hasEthernet: true,
		},
		{
			name: "Has only WiFi",
			networkInfo: NetworkInformation{
				Interfaces: NetworkInterfaces{
					Interfaces: []NetworkInterface{
						{Type: InterfaceTypeWiFi},
					},
				},
			},
			hasWiFi:     true,
			hasEthernet: false,
		},
		{
			name: "Has only Ethernet",
			networkInfo: NetworkInformation{
				Interfaces: NetworkInterfaces{
					Interfaces: []NetworkInterface{
						{Type: InterfaceTypeEthernet},
					},
				},
			},
			hasWiFi:     false,
			hasEthernet: true,
		},
		{
			name:        "No interfaces",
			networkInfo: NetworkInformation{},
			hasWiFi:     false,
			hasEthernet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.networkInfo.HasWiFi(); got != tt.hasWiFi {
				t.Errorf("Expected HasWiFi() %v, got %v", tt.hasWiFi, got)
			}
			if got := tt.networkInfo.HasEthernet(); got != tt.hasEthernet {
				t.Errorf("Expected HasEthernet() %v, got %v", tt.hasEthernet, got)
			}
		})
	}
}

func TestNetworkInterface_IsConnected(t *testing.T) {
	tests := []struct {
		name        string
		iface       NetworkInterface
		isConnected bool
	}{
		{
			name:        "WiFi connected",
			iface:       NetworkInterface{State: StateWiFiConnected},
			isConnected: true,
		},
		{
			name:        "WiFi disconnected",
			iface:       NetworkInterface{State: StateWiFiDisconnected},
			isConnected: false,
		},
		{
			name:        "Ethernet connected",
			iface:       NetworkInterface{State: StateEthernetConnected},
			isConnected: true,
		},
		{
			name:        "Ethernet disconnected",
			iface:       NetworkInterface{State: StateEthernetDisconnected},
			isConnected: false,
		},
		{
			name:        "Unknown state",
			iface:       NetworkInterface{State: "UNKNOWN_STATE"},
			isConnected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.iface.IsConnected(); got != tt.isConnected {
				t.Errorf("Expected IsConnected() %v, got %v", tt.isConnected, got)
			}
		})
	}
}

func TestNetworkInterface_TypeChecks(t *testing.T) {
	tests := []struct {
		name       string
		iface      NetworkInterface
		isWiFi     bool
		isEthernet bool
	}{
		{
			name:       "WiFi interface",
			iface:      NetworkInterface{Type: InterfaceTypeWiFi},
			isWiFi:     true,
			isEthernet: false,
		},
		{
			name:       "Ethernet interface",
			iface:      NetworkInterface{Type: InterfaceTypeEthernet},
			isWiFi:     false,
			isEthernet: true,
		},
		{
			name:       "Unknown interface",
			iface:      NetworkInterface{Type: "UNKNOWN_INTERFACE"},
			isWiFi:     false,
			isEthernet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.iface.IsWiFi(); got != tt.isWiFi {
				t.Errorf("Expected IsWiFi() %v, got %v", tt.isWiFi, got)
			}
			if got := tt.iface.IsEthernet(); got != tt.isEthernet {
				t.Errorf("Expected IsEthernet() %v, got %v", tt.isEthernet, got)
			}
		})
	}
}

func TestNetworkInterface_WiFiFrequency(t *testing.T) {
	tests := []struct {
		name         string
		frequencyKHz int
		expectedGHz  float64
		expectedBand string
		expectedFmt  string
	}{
		{
			name:         "5GHz WiFi",
			frequencyKHz: 5500000,
			expectedGHz:  5.5,
			expectedBand: "5GHz",
			expectedFmt:  "5.5 GHz",
		},
		{
			name:         "2.4GHz WiFi",
			frequencyKHz: 2400000,
			expectedGHz:  2.4,
			expectedBand: "2.4GHz",
			expectedFmt:  "2.4 GHz",
		},
		{
			name:         "No frequency",
			frequencyKHz: 0,
			expectedGHz:  0,
			expectedBand: "",
			expectedFmt:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iface := NetworkInterface{FrequencyKHz: tt.frequencyKHz}

			if got := iface.GetFrequencyGHz(); got != tt.expectedGHz {
				t.Errorf("Expected GetFrequencyGHz() %v, got %v", tt.expectedGHz, got)
			}
			if got := iface.GetFrequencyBand(); got != tt.expectedBand {
				t.Errorf("Expected GetFrequencyBand() %q, got %q", tt.expectedBand, got)
			}
			if got := iface.FormatFrequency(); got != tt.expectedFmt {
				t.Errorf("Expected FormatFrequency() %q, got %q", tt.expectedFmt, got)
			}
		})
	}
}

func TestNetworkInterface_SignalQuality(t *testing.T) {
	tests := []struct {
		name            string
		signal          string
		expectedQuality int
		expectedDesc    string
	}{
		{
			name:            "Excellent signal",
			signal:          SignalExcellent,
			expectedQuality: 90,
			expectedDesc:    "Excellent",
		},
		{
			name:            "Good signal",
			signal:          SignalGood,
			expectedQuality: 70,
			expectedDesc:    "Good",
		},
		{
			name:            "Fair signal",
			signal:          SignalFair,
			expectedQuality: 50,
			expectedDesc:    "Fair",
		},
		{
			name:            "Poor signal",
			signal:          SignalPoor,
			expectedQuality: 30,
			expectedDesc:    "Poor",
		},
		{
			name:            "Unknown signal",
			signal:          "UNKNOWN_SIGNAL",
			expectedQuality: 0,
			expectedDesc:    "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iface := NetworkInterface{Signal: tt.signal}

			if got := iface.GetSignalQuality(); got != tt.expectedQuality {
				t.Errorf("Expected GetSignalQuality() %d, got %d", tt.expectedQuality, got)
			}
			if got := iface.GetSignalDescription(); got != tt.expectedDesc {
				t.Errorf("Expected GetSignalDescription() %q, got %q", tt.expectedDesc, got)
			}
		})
	}
}

func TestNetworkInterface_GetNetworkSummary(t *testing.T) {
	tests := []struct {
		name     string
		iface    NetworkInterface
		expected string
	}{
		{
			name: "Connected WiFi with SSID and signal",
			iface: NetworkInterface{
				Type:         InterfaceTypeWiFi,
				State:        StateWiFiConnected,
				SSID:         "MyNetwork",
				Signal:       SignalExcellent,
				FrequencyKHz: 5500000,
			},
			expected: "MyNetwork (Excellent, 5GHz)",
		},
		{
			name: "Connected WiFi without frequency",
			iface: NetworkInterface{
				Type:   InterfaceTypeWiFi,
				State:  StateWiFiConnected,
				SSID:   "TestNetwork",
				Signal: SignalGood,
			},
			expected: "TestNetwork (Good)",
		},
		{
			name: "Connected WiFi without SSID",
			iface: NetworkInterface{
				Type:   InterfaceTypeWiFi,
				State:  StateWiFiConnected,
				Signal: SignalFair,
			},
			expected: "Hidden Network (Fair)",
		},
		{
			name: "Connected Ethernet",
			iface: NetworkInterface{
				Type:  InterfaceTypeEthernet,
				State: StateEthernetConnected,
			},
			expected: "Wired Connection",
		},
		{
			name: "Disconnected WiFi",
			iface: NetworkInterface{
				Type:  InterfaceTypeWiFi,
				State: StateWiFiDisconnected,
			},
			expected: "Disconnected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.iface.GetNetworkSummary(); got != tt.expected {
				t.Errorf("Expected GetNetworkSummary() %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestNetworkInterface_ValidateAddresses(t *testing.T) {
	tests := []struct {
		name     string
		iface    NetworkInterface
		validIP  bool
		validMAC bool
	}{
		{
			name: "Valid IPv4 and MAC",
			iface: NetworkInterface{
				IPAddress:  "192.168.1.10",
				MacAddress: "AA:BB:CC:DD:EE:FF",
			},
			validIP:  true,
			validMAC: true,
		},
		{
			name: "Valid IPv6 and MAC with dashes",
			iface: NetworkInterface{
				IPAddress:  "2001:db8::1",
				MacAddress: "AA-BB-CC-DD-EE-FF",
			},
			validIP:  true,
			validMAC: true,
		},
		{
			name: "Invalid IP and MAC",
			iface: NetworkInterface{
				IPAddress:  "invalid.ip",
				MacAddress: "invalid:mac",
			},
			validIP:  false,
			validMAC: false,
		},
		{
			name: "Empty addresses",
			iface: NetworkInterface{
				IPAddress:  "",
				MacAddress: "",
			},
			validIP:  false,
			validMAC: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.iface.ValidateIP(); got != tt.validIP {
				t.Errorf("Expected ValidateIP() %v, got %v", tt.validIP, got)
			}
			if got := tt.iface.ValidateMAC(); got != tt.validMAC {
				t.Errorf("Expected ValidateMAC() %v, got %v", tt.validMAC, got)
			}
		})
	}
}

func TestNetworkInformation_GetConnectedInterfaces(t *testing.T) {
	networkInfo := NetworkInformation{
		Interfaces: NetworkInterfaces{
			Interfaces: []NetworkInterface{
				{
					Type:       InterfaceTypeWiFi,
					State:      StateWiFiConnected,
					SSID:       "WiFiNetwork",
					IPAddress:  "192.168.1.10",
					MacAddress: "AA:BB:CC:DD:EE:FF",
				},
				{
					Type:  InterfaceTypeWiFi,
					State: StateWiFiDisconnected,
					SSID:  "DisconnectedWiFi",
				},
				{
					Type:       InterfaceTypeEthernet,
					State:      StateEthernetConnected,
					IPAddress:  "192.168.1.11",
					MacAddress: "11:22:33:44:55:66",
				},
			},
		},
	}

	// Test connected WiFi interface
	wifiIface := networkInfo.GetConnectedWiFiInterface()
	if wifiIface == nil {
		t.Error("Expected to find connected WiFi interface")
	} else if wifiIface.SSID != "WiFiNetwork" {
		t.Errorf("Expected WiFi SSID 'WiFiNetwork', got %q", wifiIface.SSID)
	}

	// Test connected Ethernet interface
	ethIface := networkInfo.GetConnectedEthernetInterface()
	if ethIface == nil {
		t.Error("Expected to find connected Ethernet interface")
	} else if ethIface.IPAddress != "192.168.1.11" {
		t.Errorf("Expected Ethernet IP '192.168.1.11', got %q", ethIface.IPAddress)
	}

	// Test with no connected interfaces
	emptyInfo := NetworkInformation{}
	if emptyInfo.GetConnectedWiFiInterface() != nil {
		t.Error("Expected no connected WiFi interface in empty network info")
	}
	if emptyInfo.GetConnectedEthernetInterface() != nil {
		t.Error("Expected no connected Ethernet interface in empty network info")
	}
}
