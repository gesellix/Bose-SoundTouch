package dlnatest_test

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/dlna/dlnatest"
)

// ----------------------------------------------------------------------------
// rootDesc.xml
// ----------------------------------------------------------------------------

func TestRootDesc_Parses(t *testing.T) {
	ts, _ := dlnatest.NewHTTPTest()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rootDesc.xml")
	if err != nil {
		t.Fatalf("GET /rootDesc.xml: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}

	var root struct {
		XMLName xml.Name `xml:"root"`
		Device  struct {
			DeviceType   string `xml:"deviceType"`
			FriendlyName string `xml:"friendlyName"`
			ServiceList  []struct {
				ServiceType string `xml:"serviceType"`
				ServiceID   string `xml:"serviceId"`
				ControlURL  string `xml:"controlURL"`
			} `xml:"serviceList>service"`
		} `xml:"device"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	if err := xml.Unmarshal(body, &root); err != nil {
		t.Fatalf("xml.Unmarshal: %v\nbody: %s", err, body)
	}

	wantType := "urn:schemas-upnp-org:device:MediaServer:1"
	if root.Device.DeviceType != wantType {
		t.Errorf("deviceType = %q, want %q", root.Device.DeviceType, wantType)
	}

	if root.Device.FriendlyName == "" {
		t.Error("friendlyName is empty")
	}

	// Find ContentDirectory service.
	var cdControlURL string

	for _, svc := range root.Device.ServiceList {
		if svc.ServiceType == "urn:schemas-upnp-org:service:ContentDirectory:1" {
			cdControlURL = svc.ControlURL
		}
	}

	if cdControlURL == "" {
		t.Fatal("ContentDirectory service not found in rootDesc")
	}

	if cdControlURL != "/ctl/ContentDir" {
		t.Errorf("ContentDirectory controlURL = %q, want %q", cdControlURL, "/ctl/ContentDir")
	}
}

// ----------------------------------------------------------------------------
// ContentDirectory Browse
// ----------------------------------------------------------------------------

// soapBrowse sends a SOAP Browse request for the given ObjectID and returns
// the raw <Result> string and the parsed DIDL-Lite document.
func soapBrowse(t *testing.T, baseURL, objectID string) (resultRaw string, didl didlLite) {
	t.Helper()

	body := `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">` +
		`<s:Body>` +
		`<u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">` +
		`<ObjectID>` + objectID + `</ObjectID>` +
		`<BrowseFlag>BrowseDirectChildren</BrowseFlag>` +
		`<StartingIndex>0</StartingIndex>` +
		`<RequestedCount>0</RequestedCount>` +
		`</u:Browse>` +
		`</s:Body>` +
		`</s:Envelope>`

	req, err := http.NewRequest(http.MethodPost, baseURL+"/ctl/ContentDir", strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ctl/ContentDir: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}

	// Parse the SOAP envelope.
	var envelope struct {
		Body struct {
			BrowseResponse struct {
				Result         string `xml:"Result"`
				NumberReturned int    `xml:"NumberReturned"`
				TotalMatches   int    `xml:"TotalMatches"`
			} `xml:"BrowseResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("xml.Unmarshal SOAP: %v\nraw: %s", err, raw)
	}

	resultRaw = envelope.Body.BrowseResponse.Result

	// The Result is XML-escaped DIDL-Lite; unescape + parse.
	if err := xml.Unmarshal([]byte(resultRaw), &didl); err != nil {
		t.Fatalf("xml.Unmarshal DIDL-Lite: %v\nresult: %s", err, resultRaw)
	}

	return resultRaw, didl
}

// didlLite is a minimal parse target for DIDL-Lite responses.
type didlLite struct {
	XMLName    xml.Name        `xml:"DIDL-Lite"`
	Containers []didlContainer `xml:"container"`
	Items      []didlItem      `xml:"item"`
}

type didlContainer struct {
	ID         string `xml:"id,attr"`
	ParentID   string `xml:"parentID,attr"`
	ChildCount string `xml:"childCount,attr"`
	Title      string `xml:"title"`
	Class      string `xml:"class"`
}

type didlItem struct {
	ID       string    `xml:"id,attr"`
	ParentID string    `xml:"parentID,attr"`
	Title    string    `xml:"title"`
	Class    string    `xml:"class"`
	Res      []didlRes `xml:"res"`
}

type didlRes struct {
	ProtocolInfo string `xml:"protocolInfo,attr"`
	URL          string `xml:",chardata"`
}

func TestBrowseRoot_ReturnsMusicContainer(t *testing.T) {
	ts, _ := dlnatest.NewHTTPTest()
	defer ts.Close()

	_, didl := soapBrowse(t, ts.URL, "0")

	if len(didl.Containers) == 0 {
		t.Fatal("Browse root returned no containers")
	}

	var found bool

	for _, c := range didl.Containers {
		if c.Title == "Music" {
			found = true

			if c.ID == "" {
				t.Error("Music container has empty id")
			}

			if c.ParentID != "0" {
				t.Errorf("Music container parentID = %q, want \"0\"", c.ParentID)
			}
		}
	}

	if !found {
		t.Errorf("no Music container in root browse; got containers: %v", didl.Containers)
	}
}

func TestBrowseMusicFolder_ReturnsTwoItems(t *testing.T) {
	ts, _ := dlnatest.NewHTTPTest()
	defer ts.Close()

	// First get the Music container ID from root.
	_, rootDIDL := soapBrowse(t, ts.URL, "0")

	var musicID string

	for _, c := range rootDIDL.Containers {
		if c.Title == "Music" {
			musicID = c.ID
		}
	}

	if musicID == "" {
		t.Fatal("could not find Music container in root browse")
	}

	_, didl := soapBrowse(t, ts.URL, musicID)

	if len(didl.Items) != 2 {
		t.Fatalf("expected 2 audio items in Music folder, got %d", len(didl.Items))
	}

	for _, item := range didl.Items {
		if item.Title == "" {
			t.Error("item has empty title")
		}

		if len(item.Res) == 0 {
			t.Errorf("item %q has no <res> element", item.Title)

			continue
		}

		resURL := strings.TrimSpace(item.Res[0].URL)
		if resURL == "" {
			t.Errorf("item %q has empty <res> URL", item.Title)

			continue
		}

		// Verify the resource URL is fetchable and returns audio bytes.
		t.Run("fetch_"+item.Title, func(t *testing.T) {
			resp, err := http.Get(resURL)
			if err != nil {
				t.Fatalf("GET %s: %v", resURL, err)
			}

			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status %d", resURL, resp.StatusCode)
			}

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading media body: %v", err)
			}

			if len(data) < 44 {
				t.Errorf("audio payload too small (%d bytes); expected at least a WAV header", len(data))
			}

			// Verify RIFF header.
			if string(data[0:4]) != "RIFF" {
				t.Errorf("expected RIFF header, got %q", data[0:4])
			}

			if string(data[8:12]) != "WAVE" {
				t.Errorf("expected WAVE marker, got %q", data[8:12])
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Icon
// ----------------------------------------------------------------------------

func TestIcon_ReturnsPNG(t *testing.T) {
	ts, _ := dlnatest.NewHTTPTest()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/icons/sm.png")
	if err != nil {
		t.Fatalf("GET /icons/sm.png: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "image/png") {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading icon body: %v", err)
	}

	// PNG magic bytes.
	if len(data) < 8 || string(data[0:4]) != "\x89PNG" {
		t.Errorf("response does not look like a PNG (first bytes: %x)", data[:min8(len(data))])
	}
}

func min8(n int) int {
	if n < 8 {
		return n
	}

	return 8
}

// ----------------------------------------------------------------------------
// Custom tree
// ----------------------------------------------------------------------------

func TestCustomTree(t *testing.T) {
	customTree := &dlnatest.Tree{
		Containers: []*dlnatest.Container{
			{
				ID:       "99",
				ParentID: "0",
				Title:    "CustomFolder",
				Class:    "object.container.storageFolder",
				Children: []*dlnatest.Item{
					{
						ID:       "99$0",
						ParentID: "99",
						Title:    "custom-track",
						Class:    "object.item.audioItem.musicTrack",
						MimeType: "audio/x-wav",
						DurSec:   0.5,
						Payload:  []byte("RIFF\x00\x00\x00\x00WAVEfmt "),
					},
				},
			},
		},
	}

	ts, _ := dlnatest.NewHTTPTest(dlnatest.WithTree(customTree))
	defer ts.Close()

	_, didl := soapBrowse(t, ts.URL, "0")

	if len(didl.Containers) != 1 || didl.Containers[0].Title != "CustomFolder" {
		t.Errorf("custom tree root browse: got %v", didl.Containers)
	}
}
