package discovery

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestEnrichDeviceInfo(t *testing.T) {
	// Mock UPnP device description XML
	xmlData := `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
    <device>
        <friendlyName>Sound Machinery</friendlyName>
        <modelName>SoundTouch 10</modelName>
        <serialNumber>A81B6A536A09</serialNumber>
    </device>
</root>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, xmlData)
	}))
	defer server.Close()

	device := &models.DiscoveredDevice{
		Host: "127.0.0.1",
		Name: "Initial Name",
	}

	service := NewService(1 * time.Second)
	service.httpClient = server.Client()
	err := service.enrichDeviceInfo(device, server.URL)

	if err != nil {
		t.Fatalf("enrichDeviceInfo failed: %v", err)
	}

	if device.Name != "Sound Machinery" {
		t.Errorf("expected Name 'Sound Machinery', got '%s'", device.Name)
	}

	if device.ModelID != "SoundTouch 10" {
		t.Errorf("expected ModelID 'SoundTouch 10', got '%s'", device.ModelID)
	}

	if device.UPnPSerial != "A81B6A536A09" {
		t.Errorf("expected UPnPSerial 'A81B6A536A09', got '%s'", device.UPnPSerial)
	}
}

func TestUPnP_Unmarshal(t *testing.T) {
	data := `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
    <device>
        <friendlyName>Sound Machinery</friendlyName>
        <modelName>SoundTouch 10</modelName>
        <serialNumber>A81B6A536A09</serialNumber>
    </device>
</root>`

	var upnpRoot struct {
		XMLName xml.Name `xml:"root"`
		Device  struct {
			FriendlyName string `xml:"friendlyName"`
			ModelName    string `xml:"modelName"`
			SerialNumber string `xml:"serialNumber"`
		} `xml:"device"`
	}

	err := xml.Unmarshal([]byte(data), &upnpRoot)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if upnpRoot.Device.FriendlyName != "Sound Machinery" {
		t.Errorf("expected FriendlyName 'Sound Machinery', got '%s'", upnpRoot.Device.FriendlyName)
	}
	if upnpRoot.Device.ModelName != "SoundTouch 10" {
		t.Errorf("expected ModelName 'SoundTouch 10', got '%s'", upnpRoot.Device.ModelName)
	}
	if upnpRoot.Device.SerialNumber != "A81B6A536A09" {
		t.Errorf("expected SerialNumber 'A81B6A536A09', got '%s'", upnpRoot.Device.SerialNumber)
	}
}
