package discovery

import (
	"testing"
)

// TestMediaServerFromDescription_WithCDS verifies that a description that
// includes a ContentDirectory service produces a valid MediaServer (ok=true)
// with all fields populated.
func TestMediaServerFromDescription_WithCDS(t *testing.T) {
	// Use the canned XML defined in ssdp_test.go (same package).
	location := "http://192.0.2.1:49000/rootDesc.xml"

	desc, err := parseDescription([]byte(cannedDescriptionXML), location)
	if err != nil {
		t.Fatalf("parseDescription: %v", err)
	}

	srv, ok := mediaServerFromDescription(desc)
	if !ok {
		t.Fatal("mediaServerFromDescription: ok=false, want true")
	}

	if srv.UDN == "" {
		t.Error("UDN is empty")
	}

	if srv.FriendlyName == "" {
		t.Error("FriendlyName is empty")
	}

	if srv.CDSControlURL == "" {
		t.Error("CDSControlURL is empty")
	}

	// Control URL must be absolute.
	if !isAbsoluteURL(srv.CDSControlURL) {
		t.Errorf("CDSControlURL %q is not absolute", srv.CDSControlURL)
	}

	// Icon must be resolved.
	if srv.IconURL == "" {
		t.Error("IconURL is empty")
	}

	if !isAbsoluteURL(srv.IconURL) {
		t.Errorf("IconURL %q is not absolute", srv.IconURL)
	}

	t.Logf("MediaServer: UDN=%q FriendlyName=%q CDSControlURL=%q IconURL=%q",
		srv.UDN, srv.FriendlyName, srv.CDSControlURL, srv.IconURL)
}

// TestMediaServerFromDescription_WithoutCDS verifies that a description
// without a ContentDirectory service returns ok=false.
func TestMediaServerFromDescription_WithoutCDS(t *testing.T) {
	const xmlNoCDS = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
    <friendlyName>SoundTouch 20</friendlyName>
    <manufacturer>Bose</manufacturer>
    <modelName>SoundTouch 20</modelName>
    <UDN>uuid:bose-st20-0001</UDN>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
        <controlURL>/ctl/AVTransport</controlURL>
      </service>
    </serviceList>
  </device>
</root>`

	desc, err := parseDescription([]byte(xmlNoCDS), "http://192.0.2.10:8200/desc.xml")
	if err != nil {
		t.Fatalf("parseDescription: %v", err)
	}

	_, ok := mediaServerFromDescription(desc)
	if ok {
		t.Error("mediaServerFromDescription: ok=true, want false (no ContentDirectory)")
	}
}

// TestMediaServerFromDescription_Nil ensures a nil Description returns ok=false
// without panicking.
func TestMediaServerFromDescription_Nil(t *testing.T) {
	_, ok := mediaServerFromDescription(nil)
	if ok {
		t.Error("mediaServerFromDescription(nil): ok=true, want false")
	}
}

// TestMediaServerFromDescription_FlatServer verifies a flat description (no
// sub-devices, CDS in root) maps correctly.
func TestMediaServerFromDescription_FlatServer(t *testing.T) {
	const xmlFlat = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <URLBase>http://198.51.100.20:8200</URLBase>
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
    <friendlyName>MiniDLNA</friendlyName>
    <manufacturer>Justin Maggard</manufacturer>
    <modelName>MiniDLNA</modelName>
    <UDN>uuid:minidlna-0001</UDN>
    <iconList>
      <icon>
        <mimetype>image/png</mimetype>
        <width>48</width>
        <height>48</height>
        <url>/icons/sm.png</url>
      </icon>
    </iconList>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ContentDirectory:1</serviceType>
        <controlURL>/ctl/ContentDir</controlURL>
      </service>
    </serviceList>
  </device>
</root>`

	desc, err := parseDescription([]byte(xmlFlat), "http://198.51.100.20:8200/rootDesc.xml")
	if err != nil {
		t.Fatalf("parseDescription: %v", err)
	}

	srv, ok := mediaServerFromDescription(desc)
	if !ok {
		t.Fatal("mediaServerFromDescription: ok=false, want true")
	}

	if srv.FriendlyName != "MiniDLNA" {
		t.Errorf("FriendlyName = %q, want %q", srv.FriendlyName, "MiniDLNA")
	}

	if srv.UDN != "uuid:minidlna-0001" {
		t.Errorf("UDN = %q, want %q", srv.UDN, "uuid:minidlna-0001")
	}

	wantCDS := "http://198.51.100.20:8200/ctl/ContentDir"
	if srv.CDSControlURL != wantCDS {
		t.Errorf("CDSControlURL = %q, want %q", srv.CDSControlURL, wantCDS)
	}

	wantIcon := "http://198.51.100.20:8200/icons/sm.png"
	if srv.IconURL != wantIcon {
		t.Errorf("IconURL = %q, want %q", srv.IconURL, wantIcon)
	}
}

// isAbsoluteURL returns true when s starts with "http://" or "https://".
func isAbsoluteURL(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || (len(s) > 8 && s[:8] == "https://"))
}
