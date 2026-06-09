package models

import (
	"encoding/xml"
	"testing"
)

func TestListMediaServersResponse_Populated(t *testing.T) {
	raw := `<ListMediaServersResponse>` +
		`<media_server id="uuid:1234-5678" mac="AA:BB:CC:DD:EE:FF" ip="192.0.2.5"` +
		` manufacturer="ExampleCorp" model_name="NAS-3000" friendly_name="My NAS"` +
		` model_description="Home NAS" location="http://192.0.2.5:8200/rootDesc.xml" />` +
		`<media_server id="uuid:AAAA-BBBB" mac="11:22:33:44:55:66" ip="192.0.2.6"` +
		` manufacturer="OtherCorp" model_name="Media-1" friendly_name="Living Room NAS"` +
		` location="http://192.0.2.6:8200/rootDesc.xml" />` +
		`</ListMediaServersResponse>`

	var resp ListMediaServersResponse

	if err := xml.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(resp.MediaServers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(resp.MediaServers))
	}

	first := resp.MediaServers[0]

	if first.ID != "uuid:1234-5678" {
		t.Errorf("server[0].ID = %q; want %q", first.ID, "uuid:1234-5678")
	}

	if first.MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("server[0].MAC = %q; want %q", first.MAC, "AA:BB:CC:DD:EE:FF")
	}

	if first.IP != "192.0.2.5" {
		t.Errorf("server[0].IP = %q; want %q", first.IP, "192.0.2.5")
	}

	if first.FriendlyName != "My NAS" {
		t.Errorf("server[0].FriendlyName = %q; want %q", first.FriendlyName, "My NAS")
	}

	if first.Manufacturer != "ExampleCorp" {
		t.Errorf("server[0].Manufacturer = %q; want %q", first.Manufacturer, "ExampleCorp")
	}

	if first.ModelName != "NAS-3000" {
		t.Errorf("server[0].ModelName = %q; want %q", first.ModelName, "NAS-3000")
	}

	if first.ModelDescription != "Home NAS" {
		t.Errorf("server[0].ModelDescription = %q; want %q", first.ModelDescription, "Home NAS")
	}

	if first.Location != "http://192.0.2.5:8200/rootDesc.xml" {
		t.Errorf("server[0].Location = %q; want %q", first.Location, "http://192.0.2.5:8200/rootDesc.xml")
	}

	second := resp.MediaServers[1]

	if second.ID != "uuid:AAAA-BBBB" {
		t.Errorf("server[1].ID = %q; want %q", second.ID, "uuid:AAAA-BBBB")
	}

	if second.ModelDescription != "" {
		t.Errorf("server[1].ModelDescription should be empty for omitted attr, got %q", second.ModelDescription)
	}
}

func TestListMediaServersResponse_Empty(t *testing.T) {
	// Speakers can return a self-closing element when no servers are visible.
	for _, raw := range []string{
		`<ListMediaServersResponse />`,
		`<ListMediaServersResponse></ListMediaServersResponse>`,
	} {
		var resp ListMediaServersResponse

		if err := xml.Unmarshal([]byte(raw), &resp); err != nil {
			t.Fatalf("unmarshal %q failed: %v", raw, err)
		}

		if len(resp.MediaServers) != 0 {
			t.Errorf("expected 0 servers for %q, got %d", raw, len(resp.MediaServers))
		}
	}
}
