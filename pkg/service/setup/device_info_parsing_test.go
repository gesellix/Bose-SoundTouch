package setup

import (
	"strings"
	"testing"
)

func TestDeviceInfoXML_RealWorldParsing(t *testing.T) {
	// Real XML response from a SoundTouch device's /info endpoint
	xmlData := `<info deviceID="AABBCCDDEEFF">
<name>Living Room SoundTouch</name>
<type>SoundTouch 10</type>
<margeAccountUUID>1000001</margeAccountUUID>
<components>
<component>
<componentCategory>SCM</componentCategory>
<softwareVersion>27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29</softwareVersion>
<serialNumber>I6332527703739342000020</serialNumber>
</component>
<component>
<componentCategory>PackagedProduct</componentCategory>
<softwareVersion>27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29</softwareVersion>
<serialNumber>069231P63364828AE</serialNumber>
</component>
</components>
<margeURL>https://streaming.bose.com</margeURL>
<networkInfo type="SCM">
<macAddress>AABBCCDDEEFF</macAddress>
<ipAddress>192.0.2.100</ipAddress>
</networkInfo>
<networkInfo type="SMSC">
<macAddress>AABBCCDDEE01</macAddress>
<ipAddress>192.0.2.100</ipAddress>
</networkInfo>
<moduleType>sm2</moduleType>
<variant>rhino</variant>
<variantMode>normal</variantMode>
<countryCode>GB</countryCode>
<regionCode>GB</regionCode>
</info>`

	manager := NewManager("http://localhost:8000", nil, nil)

	// Parse the XML directly (simulating what GetLiveDeviceInfo does)
	var infoXML DeviceInfoXML
	if err := manager.parseDeviceInfoXML(strings.NewReader(xmlData), &infoXML); err != nil {
		t.Fatalf("Failed to parse XML: %v", err)
	}

	// Verify basic fields
	if infoXML.DeviceID != "AABBCCDDEEFF" {
		t.Errorf("Expected deviceID 'AABBCCDDEEFF', got '%s'", infoXML.DeviceID)
	}

	if infoXML.Name != "Living Room SoundTouch" {
		t.Errorf("Expected name 'Living Room SoundTouch', got '%s'", infoXML.Name)
	}

	if infoXML.Type != "SoundTouch 10" {
		t.Errorf("Expected type 'SoundTouch 10', got '%s'", infoXML.Type)
	}

	if infoXML.ModuleType != "sm2" {
		t.Errorf("Expected moduleType 'sm2', got '%s'", infoXML.ModuleType)
	}

	if infoXML.MargeAccountUUID != "1000001" {
		t.Errorf("Expected margeAccountUUID '1000001', got '%s'", infoXML.MargeAccountUUID)
	}

	if infoXML.MargeURL != "https://streaming.bose.com" {
		t.Errorf("Expected margeURL 'https://streaming.bose.com', got '%s'", infoXML.MargeURL)
	}

	if infoXML.CountryCode != "GB" {
		t.Errorf("Expected countryCode 'GB', got '%s'", infoXML.CountryCode)
	}

	if infoXML.RegionCode != "GB" {
		t.Errorf("Expected regionCode 'GB', got '%s'", infoXML.RegionCode)
	}

	if infoXML.Variant != "rhino" {
		t.Errorf("Expected variant 'rhino', got '%s'", infoXML.Variant)
	}

	if infoXML.VariantMode != "normal" {
		t.Errorf("Expected variantMode 'normal', got '%s'", infoXML.VariantMode)
	}

	// Verify components
	if len(infoXML.Components) != 2 {
		t.Fatalf("Expected 2 components, got %d", len(infoXML.Components))
	}

	scmFound := false
	packagedProductFound := false
	for _, comp := range infoXML.Components {
		switch comp.Category {
		case "SCM":
			scmFound = true
			expectedSoftware := "27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29"
			if comp.SoftwareVersion != expectedSoftware {
				t.Errorf("Expected SCM software version '%s', got '%s'", expectedSoftware, comp.SoftwareVersion)
			}
			if comp.SerialNumber != "I6332527703739342000020" {
				t.Errorf("Expected SCM serial 'I6332527703739342000020', got '%s'", comp.SerialNumber)
			}
		case "PackagedProduct":
			packagedProductFound = true
			if comp.SerialNumber != "069231P63364828AE" {
				t.Errorf("Expected PackagedProduct serial '069231P63364828AE', got '%s'", comp.SerialNumber)
			}
		}
	}

	if !scmFound {
		t.Error("SCM component not found")
	}
	if !packagedProductFound {
		t.Error("PackagedProduct component not found")
	}

	// Verify network info
	if len(infoXML.NetworkInfo) != 2 {
		t.Fatalf("Expected 2 networkInfo entries, got %d", len(infoXML.NetworkInfo))
	}

	scmNetworkFound := false
	smscNetworkFound := false
	for _, net := range infoXML.NetworkInfo {
		switch net.Type {
		case "SCM":
			scmNetworkFound = true
			if net.MacAddress != "AABBCCDDEEFF" {
				t.Errorf("Expected SCM MAC 'AABBCCDDEEFF', got '%s'", net.MacAddress)
			}
			if net.IPAddress != "192.0.2.100" {
				t.Errorf("Expected SCM IP '192.0.2.100', got '%s'", net.IPAddress)
			}
		case "SMSC":
			smscNetworkFound = true
			if net.MacAddress != "AABBCCDDEE01" {
				t.Errorf("Expected SMSC MAC 'AABBCCDDEE01', got '%s'", net.MacAddress)
			}
			if net.IPAddress != "192.0.2.100" {
				t.Errorf("Expected SMSC IP '192.0.2.100', got '%s'", net.IPAddress)
			}
		}
	}

	if !scmNetworkFound {
		t.Error("SCM networkInfo not found")
	}
	if !smscNetworkFound {
		t.Error("SMSC networkInfo not found")
	}

	// Test the GetPrimaryMacAddress method
	primaryMAC := infoXML.GetPrimaryMacAddress()
	if primaryMAC != "AABBCCDDEEFF" {
		t.Errorf("Expected primary MAC 'AABBCCDDEEFF', got '%s'", primaryMAC)
	}

	t.Logf("✅ Successfully parsed real device info XML")
	t.Logf("   Device ID (MAC): %s", infoXML.DeviceID)
	t.Logf("   Device Name: %s", infoXML.Name)
	t.Logf("   Product: %s %s", infoXML.Type, infoXML.ModuleType)
	t.Logf("   Account: %s", infoXML.MargeAccountUUID)
	t.Logf("   Primary MAC: %s", primaryMAC)
	t.Logf("   Component Serial: %s", infoXML.SerialNumber)
	t.Logf("   Software Version: %s", infoXML.SoftwareVer)
}

func TestDeviceInfoXML_GetPrimaryMacAddress_EdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		networkInfo []struct {
			Type       string
			MacAddress string
			IPAddress  string
		}
		expected string
	}{
		{
			name:        "no_network_info",
			networkInfo: nil,
			expected:    "",
		},
		{
			name: "scm_first",
			networkInfo: []struct {
				Type       string
				MacAddress string
				IPAddress  string
			}{
				{"SCM", "AABBCCDDEEFF", "192.0.2.1"},
				{"SMSC", "AABBCCDDEE01", "192.0.2.1"},
			},
			expected: "AABBCCDDEEFF",
		},
		{
			name: "scm_second",
			networkInfo: []struct {
				Type       string
				MacAddress string
				IPAddress  string
			}{
				{"SMSC", "AABBCCDDEE01", "192.0.2.1"},
				{"SCM", "AABBCCDDEEFF", "192.0.2.1"},
			},
			expected: "AABBCCDDEEFF",
		},
		{
			name: "no_scm",
			networkInfo: []struct {
				Type       string
				MacAddress string
				IPAddress  string
			}{
				{"SMSC", "AABBCCDDEE01", "192.0.2.1"},
				{"OTHER", "AABBCCDDEE03", "192.0.2.1"},
			},
			expected: "",
		},
		{
			name: "scm_empty_mac",
			networkInfo: []struct {
				Type       string
				MacAddress string
				IPAddress  string
			}{
				{"SCM", "", "192.0.2.1"},
				{"SMSC", "AABBCCDDEE01", "192.0.2.1"},
			},
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info := DeviceInfoXML{}
			for _, net := range tc.networkInfo {
				info.NetworkInfo = append(info.NetworkInfo, struct {
					Type       string `xml:"type,attr"`
					MacAddress string `xml:"macAddress"`
					IPAddress  string `xml:"ipAddress"`
				}{
					Type:       net.Type,
					MacAddress: net.MacAddress,
					IPAddress:  net.IPAddress,
				})
			}

			result := info.GetPrimaryMacAddress()
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestDeviceInfoXML_ComponentParsing(t *testing.T) {
	xmlData := `<info deviceID="AABBCCDDEEFF">
<name>Test Device</name>
<type>SoundTouch 10</type>
<components>
<component>
<componentCategory>SCM</componentCategory>
<softwareVersion>27.0.6.46330.5043500</softwareVersion>
<serialNumber>I6332527703739342000020</serialNumber>
</component>
<component>
<componentCategory>PackagedProduct</componentCategory>
<serialNumber>069231P63364828AE</serialNumber>
</component>
</components>
</info>`

	manager := NewManager("http://localhost:8000", nil, nil)

	var infoXML DeviceInfoXML
	if err := manager.parseDeviceInfoXML(strings.NewReader(xmlData), &infoXML); err != nil {
		t.Fatalf("Failed to parse XML: %v", err)
	}

	// Verify that SerialNumber and SoftwareVer are populated from components
	if infoXML.SerialNumber != "I6332527703739342000020" {
		t.Errorf("Expected SerialNumber 'I6332527703739342000020', got '%s'", infoXML.SerialNumber)
	}

	if infoXML.SoftwareVer != "27.0.6.46330.5043500" {
		t.Errorf("Expected SoftwareVer '27.0.6.46330.5043500', got '%s'", infoXML.SoftwareVer)
	}
}
