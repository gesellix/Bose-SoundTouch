// Package dlna is a minimal DLNA / UPnP ContentDirectory browse client.
// It talks to a MediaServer's ContentDirectory:1 service via SOAP Browse
// actions, and parses the DIDL-Lite responses into structured Go types.
//
// Device discovery lives in pkg/discovery; this package is only the browse half.
package dlna

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
)

// BrowseResult holds one page of a ContentDirectory Browse response.
type BrowseResult struct {
	Containers   []Container
	Items        []Item
	TotalMatches int
	Returned     int
}

// Container is a folder / album / playlist node in the DLNA content tree.
type Container struct {
	ID         string
	ParentID   string
	Title      string
	ChildCount int
}

// Item is a single playable object (track, photo, video). Use IsAudioItem to
// check whether a SoundTouch renderer can play it.
type Item struct {
	ID          string
	ParentID    string
	Title       string
	Artist      string
	Album       string
	Class       string
	MimeType    string
	StreamURL   string
	AlbumArtURL string
	DurationSec int
}

// IsAudioItem reports whether the item is an audio track. Photos, videos, and
// unrecognised items return false.
func (it Item) IsAudioItem() bool {
	if strings.HasPrefix(strings.ToLower(it.MimeType), "audio/") {
		return true
	}

	c := strings.ToLower(it.Class)

	return strings.Contains(c, "audioitem") || strings.Contains(c, "musictrack")
}

// Browse calls ContentDirectory:Browse on srv and returns one page of results.
// objectID "0" is the server root. start is the page offset, count the page
// size (0 defaults to 50 on the caller side so the request is always bounded).
func Browse(ctx context.Context, srv discovery.MediaServer, objectID string, start, count int) (BrowseResult, error) {
	if srv.CDSControlURL == "" {
		return BrowseResult{}, fmt.Errorf("dlna: server %q has no ContentDirectory control URL", srv.FriendlyName)
	}

	if objectID == "" {
		objectID = "0"
	}

	if count <= 0 {
		count = 50
	}

	body := fmt.Sprintf(
		`<?xml version="1.0" encoding="utf-8"?>`+
			`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" `+
			`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">`+
			`<s:Body>`+
			`<u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">`+
			`<ObjectID>%s</ObjectID>`+
			`<BrowseFlag>BrowseDirectChildren</BrowseFlag>`+
			`<Filter>*</Filter>`+
			`<StartingIndex>%d</StartingIndex>`+
			`<RequestedCount>%d</RequestedCount>`+
			`<SortCriteria></SortCriteria>`+
			`</u:Browse>`+
			`</s:Body>`+
			`</s:Envelope>`,
		xmlEscape(objectID), start, count,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.CDSControlURL, strings.NewReader(body))
	if err != nil {
		return BrowseResult{}, fmt.Errorf("dlna: build Browse request: %w", err)
	}

	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return BrowseResult{}, fmt.Errorf("dlna: Browse request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return BrowseResult{}, fmt.Errorf("dlna: read Browse response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return BrowseResult{}, fmt.Errorf("dlna: Browse status %d: %s", resp.StatusCode, truncate(string(raw), 240))
	}

	return parseBrowseResponse(raw)
}

// soapBrowseEnvelope is the relevant subset of the Browse SOAP response.
type soapBrowseEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		BrowseResponse struct {
			Result         string `xml:"Result"`
			NumberReturned int    `xml:"NumberReturned"`
			TotalMatches   int    `xml:"TotalMatches"`
		} `xml:"BrowseResponse"`
	} `xml:"Body"`
}

// didlLite mirrors the embedded DIDL-Lite XML returned in the <Result> element.
type didlLite struct {
	XMLName    xml.Name        `xml:"DIDL-Lite"`
	Containers []didlContainer `xml:"container"`
	Items      []didlItem      `xml:"item"`
}

type didlContainer struct {
	ID         string `xml:"id,attr"`
	ParentID   string `xml:"parentID,attr"`
	ChildCount int    `xml:"childCount,attr"`
	Title      string `xml:"title"`
}

type didlItem struct {
	ID       string  `xml:"id,attr"`
	ParentID string  `xml:"parentID,attr"`
	Title    string  `xml:"title"`
	Class    string  `xml:"class"`
	Artist   string  `xml:"artist"`
	Album    string  `xml:"album"`
	AlbumArt string  `xml:"albumArtURI"`
	Res      []didlR `xml:"res"`
}

type didlR struct {
	ProtocolInfo string `xml:"protocolInfo,attr"`
	Duration     string `xml:"duration,attr"`
	Value        string `xml:",chardata"`
}

// parseBrowseResponse is a pure function: it parses raw SOAP Browse response
// bytes (including the nested DIDL-Lite inside <Result>) into a BrowseResult.
// Testable without HTTP.
func parseBrowseResponse(raw []byte) (BrowseResult, error) {
	var env soapBrowseEnvelope

	if err := xml.Unmarshal(raw, &env); err != nil {
		return BrowseResult{}, fmt.Errorf("dlna: parse SOAP envelope: %w", err)
	}

	resultXML := env.Body.BrowseResponse.Result

	if resultXML == "" {
		return BrowseResult{
			TotalMatches: env.Body.BrowseResponse.TotalMatches,
			Returned:     env.Body.BrowseResponse.NumberReturned,
		}, nil
	}

	var didl didlLite

	if err := xml.Unmarshal([]byte(resultXML), &didl); err != nil {
		return BrowseResult{}, fmt.Errorf("dlna: parse DIDL-Lite: %w", err)
	}

	out := BrowseResult{
		TotalMatches: env.Body.BrowseResponse.TotalMatches,
		Returned:     env.Body.BrowseResponse.NumberReturned,
	}

	for _, c := range didl.Containers {
		out.Containers = append(out.Containers, Container{
			ID:         c.ID,
			ParentID:   c.ParentID,
			Title:      c.Title,
			ChildCount: c.ChildCount,
		})
	}

	for i := range didl.Items {
		it := &didl.Items[i]
		stream := ""
		mime := ""
		duration := 0

		if len(it.Res) > 0 {
			stream = strings.TrimSpace(it.Res[0].Value)
			mime = MimeFromProtocolInfo(it.Res[0].ProtocolInfo)
			duration = ParseHMS(it.Res[0].Duration)
		}

		out.Items = append(out.Items, Item{
			ID:          it.ID,
			ParentID:    it.ParentID,
			Title:       it.Title,
			Class:       it.Class,
			Artist:      it.Artist,
			Album:       it.Album,
			AlbumArtURL: it.AlbumArt,
			StreamURL:   stream,
			MimeType:    mime,
			DurationSec: duration,
		})
	}

	return out, nil
}

// MimeFromProtocolInfo extracts the MIME type from a DLNA protocolInfo string.
// Format is "protocol:network:contentType:additionalInfo", e.g.
// "http-get:*:audio/mpeg:*". Returns the third colon-separated field.
func MimeFromProtocolInfo(pi string) string {
	parts := strings.Split(pi, ":")
	if len(parts) < 3 {
		return ""
	}

	return parts[2]
}

// ParseHMS converts a DIDL-Lite duration string in "H:MM:SS[.mmm]" format
// to a total number of seconds.
func ParseHMS(d string) int {
	if d == "" {
		return 0
	}

	// Strip optional fractional seconds ("0:03:42.000" -> "0:03:42").
	if idx := strings.Index(d, "."); idx >= 0 {
		d = d[:idx]
	}

	parts := strings.Split(d, ":")
	if len(parts) != 3 {
		return 0
	}

	h, m, s := 0, 0, 0
	_, _ = fmt.Sscanf(parts[0], "%d", &h)
	_, _ = fmt.Sscanf(parts[1], "%d", &m)
	_, _ = fmt.Sscanf(parts[2], "%d", &s)

	return h*3600 + m*60 + s
}

// xmlEscape returns s as XML-safe text (escapes &, <, >, ", ').
func xmlEscape(s string) string {
	var b strings.Builder

	xml.EscapeText(&b, []byte(s)) //nolint:errcheck // strings.Builder never errors

	return b.String()
}

// truncate returns the first n bytes of s followed by "..." when len(s) > n.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}
