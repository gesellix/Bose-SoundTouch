package client

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/speaker"
)

// avTransportControlPath is the UPnP AVTransport control endpoint on the
// speaker's MediaRenderer (served on speaker.UPnPPort, not HTTPPort).
const avTransportControlPath = "/AVTransport/Control"

// avTransportServiceType is the UPnP service type used in the SOAPAction header
// and the action element namespace.
const avTransportServiceType = "urn:schemas-upnp-org:service:AVTransport:1"

// soapEnvelope wraps a SOAP action body in the standard envelope.
const soapEnvelope = `<?xml version="1.0" encoding="utf-8"?>` +
	`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"` +
	` s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
	`<s:Body>%s</s:Body></s:Envelope>`

// PlayURLViaUPnP plays an audio URL on the speaker through its UPnP AVTransport
// service: SetAVTransportURI followed by Play.
//
// Unlike the /speaker play_info path (PlayURL/PlayCustom), this needs no app_key
// and no DNS interception, so it works on a plain LAN. The trade-offs: it
// switches the speaker to the UPNP source and replaces the current playback
// (it does not duck and resume like a notification), and the speaker itself must
// be able to reach mediaURL. The speaker auto-plays on SetAVTransportURI on
// current firmware; the explicit Play afterwards makes it robust regardless of
// the speaker's prior transport state.
func (c *Client) PlayURLViaUPnP(mediaURL string) error {
	mediaURL = strings.TrimSpace(mediaURL)
	if mediaURL == "" {
		return fmt.Errorf("media URL cannot be empty")
	}

	// The speaker's AVTransport rejects https:// outright ("URI must start with
	// http://, qplay:// or Stored Music XML") and then reports a misleading
	// "No URI supplied" 402. Fail fast with an actionable message instead.
	if strings.HasPrefix(strings.ToLower(mediaURL), "https://") {
		return fmt.Errorf("the speaker's UPnP AVTransport only accepts plain http:// URLs, not https:// — host the clip over HTTP, or use a method that proxies it (e.g. the service TTS/radio path): %s", mediaURL)
	}

	if err := c.SetAVTransportURI(mediaURL); err != nil {
		return err
	}

	return c.AVTransportPlay()
}

// SetAVTransportURI points the speaker's AVTransport at mediaURL (UPnP
// SetAVTransportURI action). Metadata is sent empty, which the speaker accepts.
// Note the speaker only accepts http:// (and qplay:// / Stored Music) URIs, not
// https://; PlayURLViaUPnP guards against that.
func (c *Client) SetAVTransportURI(mediaURL string) error {
	body := `<u:SetAVTransportURI xmlns:u="` + avTransportServiceType + `">` +
		`<InstanceID>0</InstanceID>` +
		`<CurrentURI>` + escapeXMLText(mediaURL) + `</CurrentURI>` +
		`<CurrentURIMetaData></CurrentURIMetaData>` +
		`</u:SetAVTransportURI>`

	return c.soapAVTransport("SetAVTransportURI", body)
}

// AVTransportPlay starts playback (UPnP Play action, Speed 1).
func (c *Client) AVTransportPlay() error {
	body := `<u:Play xmlns:u="` + avTransportServiceType + `">` +
		`<InstanceID>0</InstanceID><Speed>1</Speed>` +
		`</u:Play>`

	return c.soapAVTransport("Play", body)
}

// soapAVTransport POSTs a SOAP action to the speaker's AVTransport control URL.
func (c *Client) soapAVTransport(action, innerBody string) error {
	controlURL, err := c.avTransportControlURL()
	if err != nil {
		return err
	}

	payload := fmt.Sprintf(soapEnvelope, innerBody)

	req, err := http.NewRequest(http.MethodPost, controlURL, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create %s request: %w", action, err)
	}

	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("User-Agent", c.userAgent)
	// UPnP control points send a SOAPAction header. Write it through the map
	// directly to preserve the exact casing the UPnP convention uses (Set would
	// canonicalise it to "Soapaction"), mirroring how this codebase preserves
	// the speaker-facing ETag header casing.
	req.Header["SOAPAction"] = []string{`"` + avTransportServiceType + "#" + action + `"`}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute %s: %w", action, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return fmt.Errorf("UPnP %s failed with status %d: %s", action, resp.StatusCode, strings.TrimSpace(string(b)))
	}

	return nil
}

// avTransportControlURL derives the UPnP AVTransport control URL
// (http://<host>:<UPnPPort>/AVTransport/Control) from the client's base URL,
// which targets the :8090 local API. UPnP control lives on a different port.
func (c *Client) avTransportControlURL() (string, error) {
	if c.avTransportURLOverride != "" {
		return c.avTransportURLOverride, nil
	}

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL %q: %w", c.baseURL, err)
	}

	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("no host in base URL %q", c.baseURL)
	}

	hostPort := net.JoinHostPort(host, strconv.Itoa(speaker.UPnPPort))

	return "http://" + hostPort + avTransportControlPath, nil
}

// escapeXMLText XML-escapes a string for safe inclusion as element character
// data (e.g. the media URL inside <CurrentURI>).
func escapeXMLText(s string) string {
	var b bytes.Buffer

	_ = xml.EscapeText(&b, []byte(s))

	return b.String()
}
