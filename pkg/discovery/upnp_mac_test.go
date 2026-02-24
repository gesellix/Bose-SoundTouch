package discovery

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestUPnP_EnrichDeviceInfo_RealDeviceXML(t *testing.T) {
	// This tests the exact UPnP XML format provided by the user
	realDeviceXML := `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
    <specVersion>
        <major>1</major>
        <minor>0</minor>
    </specVersion>
    <device>
        <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
        <friendlyName>Sound Machinechen</friendlyName>
        <qq:X_QPlay_SoftwareCapability xmlns:qq="http://www.tencent.com">QPlay:2</qq:X_QPlay_SoftwareCapability>
        <manufacturer>Bose Corporation</manufacturer>
        <manufacturerURL>http://www.bose.com</manufacturerURL>
        <modelName>SoundTouch 10</modelName>
        <modelNumber></modelNumber>
        <modelDescription>Bose SoundTouch Wireless Streaming Audio Device</modelDescription>
        <modelURL>http://www.bose.com</modelURL>
        <serialNumber>A81B6A536A98</serialNumber>
        <UDN>uuid:BO5EBO5E-F00D-F00D-FEED-A81B6A536A98</UDN>
        <serviceList>
            <service>
                <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
                <serviceId>urn:upnp-org:serviceId:AVTransport</serviceId>
                <SCPDURL>/Xml/AVTransport3.xml</SCPDURL>
                <controlURL>/AVTransport/Control</controlURL>
                <eventSubURL>/AVTransport/Event</eventSubURL>
            </service>
            <service>
                <serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
                <serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId>
                <SCPDURL>/Xml/ConnectionManager3.xml</SCPDURL>
                <controlURL>/ConnectionManager/Control</controlURL>
                <eventSubURL>/ConnectionManager/Event</eventSubURL>
            </service>
            <service>
                <serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
                <serviceId>urn:upnp-org:serviceId:RenderingControl</serviceId>
                <SCPDURL>/Xml/RenderingControl3.xml</SCPDURL>
                <controlURL>/RenderingControl/Control</controlURL>
                <eventSubURL>/RenderingControl/Event</eventSubURL>
            </service>
            <service>
                <serviceType>urn:schemas-tencent-com:service:QPlay:2</serviceType>
                <serviceId>urn:tencent-com:serviceId:QPlay</serviceId>
                <controlURL>/QPlay/Control</controlURL>
                <eventSubURL>/QPlay/Event</eventSubURL>
                <SCPDURL>/Xml/QPlay.xml</SCPDURL>
            </service>
        </serviceList>
    </device>
</root>`

	// Create a test server that serves the UPnP XML
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		fmt.Fprint(w, realDeviceXML)
	}))
	defer server.Close()

	// Create a discovered device to enrich
	device := &models.DiscoveredDevice{
		Host: "192.168.178.35",
		Port: 8091,
		Name: "Initial Device Name",
	}

	// Create discovery service and enrich the device
	service := NewService(5 * time.Second)
	err := service.EnrichDeviceInfo(device, server.URL)

	if err != nil {
		t.Fatalf("enrichDeviceInfo failed: %v", err)
	}

	// Verify that the MAC address was extracted correctly from serialNumber
	expectedMAC := "A81B6A536A98"
	if device.UPnPSerial != expectedMAC {
		t.Errorf("Expected UPnPSerial '%s', got '%s'", expectedMAC, device.UPnPSerial)
	}

	// Verify other enriched fields
	expectedName := "Sound Machinechen"
	if device.Name != expectedName {
		t.Errorf("Expected Name '%s', got '%s'", expectedName, device.Name)
	}

	expectedModel := "SoundTouch 10"
	if device.ModelID != expectedModel {
		t.Errorf("Expected ModelID '%s', got '%s'", expectedModel, device.ModelID)
	}

	t.Logf("✓ Successfully extracted MAC address '%s' from UPnP serialNumber field", device.UPnPSerial)
	t.Logf("✓ Device name: '%s'", device.Name)
	t.Logf("✓ Device model: '%s'", device.ModelID)
}

func TestUPnP_MACAddressDiscovery_Integration(t *testing.T) {
	// Test various MAC address formats that might appear in serialNumber
	testCases := []struct {
		name               string
		serialNumberInXML  string
		expectedUPnPSerial string
		description        string
	}{
		{
			name:               "StandardMAC",
			serialNumberInXML:  "A81B6A536A98",
			expectedUPnPSerial: "A81B6A536A98",
			description:        "Standard MAC address format without separators",
		},
		{
			name:               "MACWithColons",
			serialNumberInXML:  "A8:1B:6A:53:6A:98",
			expectedUPnPSerial: "A8:1B:6A:53:6A:98",
			description:        "MAC address with colon separators",
		},
		{
			name:               "MACWithDashes",
			serialNumberInXML:  "A8-1B-6A-53-6A-98",
			expectedUPnPSerial: "A8-1B-6A-53-6A-98",
			description:        "MAC address with dash separators",
		},
		{
			name:               "LowercaseMAC",
			serialNumberInXML:  "a81b6a536a98",
			expectedUPnPSerial: "a81b6a536a98",
			description:        "Lowercase MAC address",
		},
		{
			name:               "MixedCaseMAC",
			serialNumberInXML:  "a81B6a536A98",
			expectedUPnPSerial: "a81B6a536A98",
			description:        "Mixed case MAC address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create UPnP XML with the specific serialNumber format
			xmlTemplate := `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
    <device>
        <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
        <friendlyName>Test Device</friendlyName>
        <manufacturer>Bose Corporation</manufacturer>
        <modelName>SoundTouch 10</modelName>
        <serialNumber>%s</serialNumber>
        <UDN>uuid:BO5EBO5E-F00D-F00D-FEED-TEST</UDN>
    </device>
</root>`

			deviceXML := fmt.Sprintf(xmlTemplate, tc.serialNumberInXML)

			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/xml")
				fmt.Fprint(w, deviceXML)
			}))
			defer server.Close()

			// Create and enrich device
			device := &models.DiscoveredDevice{
				Host: "192.168.1.100",
				Port: 8090,
			}

			service := NewService(5 * time.Second)
			err := service.EnrichDeviceInfo(device, server.URL)

			if err != nil {
				t.Errorf("%s: enrichDeviceInfo failed: %v", tc.description, err)
				return
			}

			if device.UPnPSerial != tc.expectedUPnPSerial {
				t.Errorf("%s: Expected UPnPSerial '%s', got '%s'",
					tc.description, tc.expectedUPnPSerial, device.UPnPSerial)
			} else {
				t.Logf("✓ %s: Successfully extracted '%s'", tc.description, device.UPnPSerial)
			}
		})
	}
}

func TestUPnP_URLPattern_Realistic(t *testing.T) {
	// Test the exact URL pattern mentioned:
	// http://192.168.178.35:8091/XD/BO5EBO5E-F00D-F00D-FEED-A81B6A536A98.xml

	realDeviceXML := `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
    <device>
        <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
        <friendlyName>Sound Machinechen</friendlyName>
        <manufacturer>Bose Corporation</manufacturer>
        <modelName>SoundTouch 10</modelName>
        <serialNumber>A81B6A536A98</serialNumber>
        <UDN>uuid:BO5EBO5E-F00D-F00D-FEED-A81B6A536A98</UDN>
    </device>
</root>`

	// Create server that responds to the specific path
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/XD/BO5EBO5E-F00D-F00D-FEED-A81B6A536A98.xml" {
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprint(w, realDeviceXML)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Test enrichment using the realistic URL path
	device := &models.DiscoveredDevice{
		Host: "192.168.178.35",
		Port: 8091,
		Name: "Initial Name",
	}

	service := NewService(5 * time.Second)
	locationURL := server.URL + "/XD/BO5EBO5E-F00D-F00D-FEED-A81B6A536A98.xml"
	err := service.EnrichDeviceInfo(device, locationURL)

	if err != nil {
		t.Fatalf("enrichDeviceInfo failed for realistic URL: %v", err)
	}

	// Verify MAC address extraction
	expectedMAC := "A81B6A536A98"
	if device.UPnPSerial != expectedMAC {
		t.Errorf("Expected MAC '%s', got '%s'", expectedMAC, device.UPnPSerial)
	}

	// Note: The MAC address in the URL and in the XML serialNumber should match
	if device.UPnPSerial == expectedMAC {
		t.Logf("✓ MAC address '%s' extracted from UPnP XML matches expected value", device.UPnPSerial)
		t.Logf("✓ This MAC can now be used for datastore mapping")
		t.Logf("✓ Request URL pattern: GET /streaming/account/{account}/device/%s/presets", device.UPnPSerial)
	}
}

func TestUPnP_ErrorHandling(t *testing.T) {
	service := NewService(5 * time.Second)
	device := &models.DiscoveredDevice{
		Host: "192.168.1.100",
		Name: "Test Device",
	}

	t.Run("InvalidXML", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprint(w, "invalid xml content")
		}))
		defer server.Close()

		err := service.EnrichDeviceInfo(device, server.URL)
		if err == nil {
			t.Error("Expected error for invalid XML, got nil")
		} else {
			t.Logf("✓ Correctly handled invalid XML: %v", err)
		}
	})

	t.Run("HTTPError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		err := service.EnrichDeviceInfo(device, server.URL)
		if err == nil {
			t.Error("Expected error for HTTP 500, got nil")
		} else {
			t.Logf("✓ Correctly handled HTTP error: %v", err)
		}
	})

	t.Run("MissingSerialNumber", func(t *testing.T) {
		xmlWithoutSerial := `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
    <device>
        <friendlyName>Test Device</friendlyName>
        <modelName>Test Model</modelName>
    </device>
</root>`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprint(w, xmlWithoutSerial)
		}))
		defer server.Close()

		deviceCopy := *device // Make a copy to avoid modifying the original
		err := service.EnrichDeviceInfo(&deviceCopy, server.URL)

		// Should not error, but UPnPSerial should be empty
		if err != nil {
			t.Errorf("Unexpected error for missing serialNumber: %v", err)
		}

		if deviceCopy.UPnPSerial != "" {
			t.Errorf("Expected empty UPnPSerial, got '%s'", deviceCopy.UPnPSerial)
		} else {
			t.Logf("✓ Correctly handled missing serialNumber (empty UPnPSerial)")
		}
	})
}
