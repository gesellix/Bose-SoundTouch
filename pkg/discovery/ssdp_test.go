package discovery

import (
	"net/url"
	"testing"
)

// cannedDescriptionXML is a realistic UPnP device description that includes
// a root device with one icon, a ContentDirectory service, and one sub-device
// (mimicking the FRITZ!Box nesting pattern). Used to exercise parseDescription
// and FindService without any network I/O.
const cannedDescriptionXML = `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <URLBase>http://192.0.2.1:49000</URLBase>
  <device>
    <deviceType>urn:schemas-upnp-org:device:Basic:1</deviceType>
    <friendlyName>FRITZ!Box 7590</friendlyName>
    <manufacturer>AVM</manufacturer>
    <modelName>FRITZ!Box 7590</modelName>
    <serialNumber>SN-001</serialNumber>
    <UDN>uuid:root-device-0001</UDN>
    <iconList>
      <icon>
        <mimetype>image/png</mimetype>
        <width>48</width>
        <height>48</height>
        <url>/icons/root.png</url>
      </icon>
    </iconList>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:Layer3Forwarding:1</serviceType>
        <controlURL>/ctl/L3Fwd</controlURL>
        <eventSubURL>/evt/L3Fwd</eventSubURL>
        <SCPDURL>/L3Fwd.xml</SCPDURL>
      </service>
    </serviceList>
    <deviceList>
      <device>
        <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
        <friendlyName>FRITZ!Box NAS</friendlyName>
        <manufacturer>AVM</manufacturer>
        <modelName>FRITZ!NAS</modelName>
        <serialNumber>SN-002</serialNumber>
        <UDN>uuid:media-server-0001</UDN>
        <iconList>
          <icon>
            <mimetype>image/png</mimetype>
            <width>32</width>
            <height>32</height>
            <url>/icons/nas.png</url>
          </icon>
        </iconList>
        <serviceList>
          <service>
            <serviceType>urn:schemas-upnp-org:service:ContentDirectory:1</serviceType>
            <controlURL>/ctl/ContentDir</controlURL>
            <eventSubURL>/evt/ContentDir</eventSubURL>
            <SCPDURL>/ContentDir.xml</SCPDURL>
          </service>
        </serviceList>
      </device>
    </deviceList>
  </device>
</root>`

// TestParseDescription_Fields checks that parseDescription populates the
// root-device fields correctly from the canned XML.
func TestParseDescription_Fields(t *testing.T) {
	location := "http://192.0.2.1:49000/rootDesc.xml"

	desc, err := parseDescription([]byte(cannedDescriptionXML), location)
	if err != nil {
		t.Fatalf("parseDescription: %v", err)
	}

	if desc.URLBase != "http://192.0.2.1:49000" {
		t.Errorf("URLBase = %q, want %q", desc.URLBase, "http://192.0.2.1:49000")
	}

	root := desc.Root

	if root.FriendlyName != "FRITZ!Box 7590" {
		t.Errorf("FriendlyName = %q, want %q", root.FriendlyName, "FRITZ!Box 7590")
	}

	if root.UDN != "uuid:root-device-0001" {
		t.Errorf("UDN = %q, want %q", root.UDN, "uuid:root-device-0001")
	}

	if root.Manufacturer != "AVM" {
		t.Errorf("Manufacturer = %q, want %q", root.Manufacturer, "AVM")
	}

	if root.ModelName != "FRITZ!Box 7590" {
		t.Errorf("ModelName = %q, want %q", root.ModelName, "FRITZ!Box 7590")
	}

	if len(root.Devices) != 1 {
		t.Fatalf("root sub-devices = %d, want 1", len(root.Devices))
	}

	sub := root.Devices[0]

	if sub.FriendlyName != "FRITZ!Box NAS" {
		t.Errorf("sub FriendlyName = %q, want %q", sub.FriendlyName, "FRITZ!Box NAS")
	}
}

// TestParseDescription_FindService confirms that FindService recurses into
// sub-devices and resolves the controlURL to an absolute form using URLBase.
func TestParseDescription_FindService(t *testing.T) {
	location := "http://192.0.2.1:49000/rootDesc.xml"

	desc, err := parseDescription([]byte(cannedDescriptionXML), location)
	if err != nil {
		t.Fatalf("parseDescription: %v", err)
	}

	// ContentDirectory is in the sub-device, not the root.
	svc, ok := desc.FindService("urn:schemas-upnp-org:service:ContentDirectory:1")
	if !ok {
		t.Fatal("FindService(ContentDirectory:1): not found")
	}

	// URLBase is http://192.0.2.1:49000, controlURL is /ctl/ContentDir.
	wantControlURL := "http://192.0.2.1:49000/ctl/ContentDir"
	if svc.ControlURL != wantControlURL {
		t.Errorf("ControlURL = %q, want %q", svc.ControlURL, wantControlURL)
	}

	// Root-only service should also be found.
	l3, ok := desc.FindService("urn:schemas-upnp-org:service:Layer3Forwarding:1")
	if !ok {
		t.Fatal("FindService(Layer3Forwarding:1): not found")
	}

	if l3.ControlURL != "http://192.0.2.1:49000/ctl/L3Fwd" {
		t.Errorf("Layer3Forwarding ControlURL = %q", l3.ControlURL)
	}
}

// TestParseDescription_FindService_Missing ensures false is returned when the
// service does not exist in the tree.
func TestParseDescription_FindService_Missing(t *testing.T) {
	location := "http://192.0.2.1:49000/rootDesc.xml"

	desc, err := parseDescription([]byte(cannedDescriptionXML), location)
	if err != nil {
		t.Fatalf("parseDescription: %v", err)
	}

	_, ok := desc.FindService("urn:schemas-upnp-org:service:DoesNotExist:1")
	if ok {
		t.Error("FindService(DoesNotExist:1) returned ok=true, want false")
	}
}

// TestParseDescription_FirstIcon checks that icon URLs are resolved to
// absolute form and that FirstIcon returns the root-device icon.
func TestParseDescription_FirstIcon(t *testing.T) {
	location := "http://192.0.2.1:49000/rootDesc.xml"

	desc, err := parseDescription([]byte(cannedDescriptionXML), location)
	if err != nil {
		t.Fatalf("parseDescription: %v", err)
	}

	ic, ok := desc.FirstIcon()
	if !ok {
		t.Fatal("FirstIcon: not found")
	}

	wantURL := "http://192.0.2.1:49000/icons/root.png"
	if ic.URL != wantURL {
		t.Errorf("icon URL = %q, want %q", ic.URL, wantURL)
	}

	if ic.Width != 48 {
		t.Errorf("icon Width = %d, want 48", ic.Width)
	}

	if ic.MimeType != "image/png" {
		t.Errorf("icon MimeType = %q, want %q", ic.MimeType, "image/png")
	}
}

// TestParseDescription_RelativeURLResolution tests URL resolution without
// URLBase (falls back to the location URL).
func TestParseDescription_RelativeURLResolution(t *testing.T) {
	const xmlNoURLBase = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
    <friendlyName>Mini NAS</friendlyName>
    <UDN>uuid:mini-001</UDN>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ContentDirectory:1</serviceType>
        <controlURL>/ctl/CDS</controlURL>
      </service>
    </serviceList>
  </device>
</root>`

	location := "http://198.51.100.5:8200/rootDesc.xml"

	desc, err := parseDescription([]byte(xmlNoURLBase), location)
	if err != nil {
		t.Fatalf("parseDescription: %v", err)
	}

	svc, ok := desc.FindService("urn:schemas-upnp-org:service:ContentDirectory:1")
	if !ok {
		t.Fatal("FindService: not found")
	}

	// Without URLBase, base URL comes from location.
	want := "http://198.51.100.5:8200/ctl/CDS"
	if svc.ControlURL != want {
		t.Errorf("ControlURL = %q, want %q", svc.ControlURL, want)
	}
}

// TestAbsURL_Variants exercises the absURL helper with several input
// combinations.
func TestAbsURL_Variants(t *testing.T) {
	cases := []struct {
		base string
		ref  string
		want string
	}{
		// Relative path resolved against explicit-port base.
		{"http://192.0.2.1:49000/desc.xml", "/ctl/CDS", "http://192.0.2.1:49000/ctl/CDS"},
		// Already absolute: returned unchanged.
		{"http://192.0.2.1:49000/", "http://198.51.100.5:8200/ctl/CDS", "http://198.51.100.5:8200/ctl/CDS"},
		// Empty ref: returned as-is.
		{"http://192.0.2.1:49000/", "", ""},
	}

	for _, tc := range cases {
		base, err := url.Parse(tc.base)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", tc.base, err)
		}

		got := absURL(base, tc.ref)

		if got != tc.want {
			t.Errorf("absURL(%q, %q) = %q, want %q", tc.base, tc.ref, got, tc.want)
		}
	}
}
