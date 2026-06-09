// Package dlnatest provides an in-process DLNA / UPnP MediaServer for use in
// unit tests (via httptest.Server) and as a real LAN-visible server.
//
// The server handles:
//   - GET /rootDesc.xml    device description (UPnP root device)
//   - POST /ctl/ContentDir ContentDirectory Browse SOAP action
//   - GET /MediaItems/*.wav synthesised audio bytes (1 s silent WAV)
//   - GET /icons/sm.png    minimal 1x1 PNG so icon fetches do not 404
//
// All absolute URLs in DIDL-Lite <res> elements are built from the
// incoming request's Host header, so the same handler works unchanged
// behind httptest.Server and a real net.Listener.
package dlnatest

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
)

// ----------------------------------------------------------------------------
// Content tree model
// ----------------------------------------------------------------------------

// Container represents a DLNA object.container node.
type Container struct {
	ID       string
	ParentID string
	Title    string
	Class    string // upnp:class value, e.g. "object.container.storageFolder"
	Children []*Item
}

// Item represents a DLNA object.item.audioItem node.
type Item struct {
	ID       string
	ParentID string
	Title    string
	Class    string // upnp:class value, e.g. "object.item.audioItem.musicTrack"
	Artist   string
	Album    string
	MimeType string
	DurSec   float64 // duration in seconds
	Payload  []byte  // raw audio bytes served at /MediaItems/<ID>.<ext>
}

// mediaExt returns the file extension for this item's MIME type.
func (it *Item) mediaExt() string {
	switch it.MimeType {
	case "audio/x-wav", "audio/wav":
		return "wav"
	default:
		return "bin"
	}
}

// Tree is the in-memory content tree. Root containers are stored by ID.
type Tree struct {
	Containers []*Container // ordered; first container is the default music folder
}

// DefaultTree returns a minimal two-track music library that matches the
// structure used in the spec/capture comments.
func DefaultTree() *Tree {
	track01 := silentWAV(1, 8000, 1)
	track02 := silentWAV(1, 8000, 1)

	music := &Container{
		ID:       "1",
		ParentID: "0",
		Title:    "Music",
		Class:    "object.container.storageFolder",
		Children: []*Item{
			{
				ID:       "1$4$0",
				ParentID: "1$4",
				Title:    "track01",
				Class:    "object.item.audioItem.musicTrack",
				MimeType: "audio/x-wav",
				DurSec:   1.0,
				Payload:  track01,
			},
			{
				ID:       "1$4$1",
				ParentID: "1$4",
				Title:    "track02",
				Class:    "object.item.audioItem.musicTrack",
				MimeType: "audio/x-wav",
				DurSec:   1.0,
				Payload:  track02,
			},
		},
	}

	return &Tree{Containers: []*Container{music}}
}

// containerByID returns the container with the given ID, or nil.
func (t *Tree) containerByID(id string) *Container {
	for _, c := range t.Containers {
		if c.ID == id {
			return c
		}
	}

	return nil
}

// itemByID returns the first item in any container whose ID matches.
func (t *Tree) itemByID(id string) *Item {
	for _, c := range t.Containers {
		for _, it := range c.Children {
			if it.ID == id {
				return it
			}
		}
	}

	return nil
}

// ----------------------------------------------------------------------------
// Server
// ----------------------------------------------------------------------------

// Option is a functional option for NewServer.
type Option func(*Server)

// WithFriendlyName overrides the UPnP friendlyName.
func WithFriendlyName(name string) Option {
	return func(s *Server) { s.FriendlyName = name }
}

// WithUDN overrides the UPnP Unique Device Name (UUID).
func WithUDN(udn string) Option {
	return func(s *Server) { s.UDN = udn }
}

// WithTree replaces the entire content tree.
func WithTree(tree *Tree) Option {
	return func(s *Server) { s.tree = tree }
}

// Server is the DLNA / UPnP MediaServer implementation.
type Server struct {
	FriendlyName string
	UDN          string
	tree         *Tree
}

// NewServer creates a Server with the supplied options applied.
func NewServer(opts ...Option) *Server {
	s := &Server{
		FriendlyName: "AfterTouch Test Library",
		UDN:          "uuid:4d696e69-444c-164e-9d41-72ecda78e4c1",
		tree:         DefaultTree(),
	}

	for _, o := range opts {
		o(s)
	}

	return s
}

// NewHTTPTest starts an httptest.Server backed by s and returns both.
// Call ts.Close() when the test is done.
func NewHTTPTest(opts ...Option) (*httptest.Server, *Server) {
	s := NewServer(opts...)
	ts := httptest.NewServer(s.HTTPHandler())

	return ts, s
}

// HTTPHandler returns an http.Handler that serves all DLNA endpoints.
func (s *Server) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/rootDesc.xml", s.serveRootDesc)
	mux.HandleFunc("/ctl/ContentDir", s.serveContentDir)
	mux.HandleFunc("/icons/sm.png", s.serveIcon)
	mux.HandleFunc("/MediaItems/", s.serveMediaItem)

	return mux
}

// ----------------------------------------------------------------------------
// /rootDesc.xml
// ----------------------------------------------------------------------------

func (s *Server) serveRootDesc(w http.ResponseWriter, _ *http.Request) {
	type specVersion struct {
		Major int `xml:"major"`
		Minor int `xml:"minor"`
	}

	type icon struct {
		MimeType string `xml:"mimetype"`
		Width    int    `xml:"width"`
		Height   int    `xml:"height"`
		Depth    int    `xml:"depth"`
		URL      string `xml:"url"`
	}

	type service struct {
		ServiceType string `xml:"serviceType"`
		ServiceID   string `xml:"serviceId"`
		ControlURL  string `xml:"controlURL"`
		EventSubURL string `xml:"eventSubURL,omitempty"`
		SCPDURL     string `xml:"SCPDURL,omitempty"`
	}

	type device struct {
		DeviceType   string    `xml:"deviceType"`
		FriendlyName string    `xml:"friendlyName"`
		Manufacturer string    `xml:"manufacturer"`
		ModelName    string    `xml:"modelName"`
		ModelNumber  string    `xml:"modelNumber"`
		SerialNumber string    `xml:"serialNumber"`
		UDN          string    `xml:"UDN"`
		IconList     []icon    `xml:"iconList>icon"`
		ServiceList  []service `xml:"serviceList>service"`
	}

	type rootDesc struct {
		XMLName     xml.Name    `xml:"urn:schemas-upnp-org:device-1-0 root"`
		SpecVersion specVersion `xml:"specVersion"`
		Device      device      `xml:"device"`
	}

	desc := rootDesc{
		SpecVersion: specVersion{Major: 1, Minor: 0},
		Device: device{
			DeviceType:   "urn:schemas-upnp-org:device:MediaServer:1",
			FriendlyName: s.FriendlyName,
			Manufacturer: "AfterTouch",
			ModelName:    "AfterTouch Test MediaServer",
			ModelNumber:  "1",
			SerialNumber: "00000000",
			UDN:          s.UDN,
			IconList: []icon{
				{MimeType: "image/png", Width: 48, Height: 48, Depth: 24, URL: "/icons/sm.png"},
			},
			ServiceList: []service{
				{
					ServiceType: "urn:schemas-upnp-org:service:ContentDirectory:1",
					ServiceID:   "urn:upnp-org:serviceId:ContentDirectory",
					ControlURL:  "/ctl/ContentDir",
					EventSubURL: "/evt/ContentDir",
					SCPDURL:     "/ContentDir.xml",
				},
				{
					ServiceType: "urn:schemas-upnp-org:service:ConnectionManager:1",
					ServiceID:   "urn:upnp-org:serviceId:ConnectionManager",
					ControlURL:  "/ctl/ConnectionMgr",
				},
			},
		},
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")

	if _, err := fmt.Fprint(w, xml.Header); err != nil {
		http.Error(w, "write error", http.StatusInternalServerError)

		return
	}

	enc := xml.NewEncoder(w)
	enc.Indent("", "")

	if err := enc.Encode(desc); err != nil {
		// Headers already sent; best effort.
		return
	}
}

// ----------------------------------------------------------------------------
// /ctl/ContentDir (ContentDirectory Browse SOAP action)
// ----------------------------------------------------------------------------

// soapBrowseRequest is the envelope we parse from the incoming POST.
type soapBrowseRequest struct {
	Body struct {
		Browse struct {
			ObjectID       string `xml:"ObjectID"`
			BrowseFlag     string `xml:"BrowseFlag"`
			StartingIndex  int    `xml:"StartingIndex"`
			RequestedCount int    `xml:"RequestedCount"`
		} `xml:"Browse"`
	} `xml:"Body"`
}

func (s *Server) serveContentDir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	// Parse the SOAP envelope (lenient: ignore namespace prefixes via xml.Unmarshal).
	var req soapBrowseRequest
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad soap envelope", http.StatusBadRequest)

		return
	}

	objectID := req.Body.Browse.ObjectID
	startIndex := req.Body.Browse.StartingIndex
	reqCount := req.Body.Browse.RequestedCount

	base := baseURL(r)

	var didl string

	var total int

	switch objectID {
	case "0":
		// Root: return containers.
		didl, total = s.browseRoot(startIndex, reqCount)
	default:
		// Try as a container ID.
		if c := s.tree.containerByID(objectID); c != nil {
			didl, total = s.browseContainer(c, startIndex, reqCount, base)
		} else {
			// Unknown object: return empty result.
			didl = emptyDIDL()
			total = 0
		}
	}

	// NumberReturned is the count of items in this page.
	var returned int
	if reqCount <= 0 || reqCount > total-startIndex {
		returned = total - startIndex
	} else {
		returned = reqCount
	}

	if returned < 0 {
		returned = 0
	}

	writeSOAPBrowseResponse(w, didl, returned, total)
}

// baseURL builds an absolute http://host:port prefix from the request.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	return scheme + "://" + r.Host
}

// browseRoot returns DIDL-Lite for the root container (ObjectID "0").
func (s *Server) browseRoot(start, count int) (string, int) {
	containers := s.tree.Containers
	total := len(containers)
	page := page(containers, start, count)

	var b strings.Builder

	b.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" `)
	b.WriteString(`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" `)
	b.WriteString(`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" `)
	b.WriteString(`xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">`)

	for _, c := range page {
		childCount := len(c.Children)
		_, _ = fmt.Fprintf(&b,
			`<container id=%s parentID=%s restricted="1" childCount="%d">`,
			xmlAttr(c.ID), xmlAttr(c.ParentID), childCount,
		)
		b.WriteString(`<dc:title>` + xmlEsc(c.Title) + `</dc:title>`)
		b.WriteString(`<upnp:class>` + xmlEsc(c.Class) + `</upnp:class>`)
		b.WriteString(`</container>`)
	}

	b.WriteString(`</DIDL-Lite>`)

	return b.String(), total
}

// browseContainer returns DIDL-Lite for the items inside a container.
func (s *Server) browseContainer(c *Container, start, count int, base string) (string, int) {
	items := c.Children
	total := len(items)
	pageItems := pageItems(items, start, count)

	var b strings.Builder

	b.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" `)
	b.WriteString(`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" `)
	b.WriteString(`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" `)
	b.WriteString(`xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">`)

	for _, it := range pageItems {
		size := len(it.Payload)
		dur := formatDuration(it.DurSec)
		resURL := fmt.Sprintf("%s/MediaItems/%s.%s", base, urlPathEsc(it.ID), it.mediaExt())

		_, _ = fmt.Fprintf(&b,
			`<item id=%s parentID=%s restricted="1">`,
			xmlAttr(it.ID), xmlAttr(it.ParentID),
		)
		b.WriteString(`<dc:title>` + xmlEsc(it.Title) + `</dc:title>`)

		if it.Artist != "" {
			b.WriteString(`<dc:creator>` + xmlEsc(it.Artist) + `</dc:creator>`)
		}

		b.WriteString(`<upnp:class>` + xmlEsc(it.Class) + `</upnp:class>`)
		_, _ = fmt.Fprintf(&b,
			`<res size="%d" duration="%s" bitrate="128000" sampleFrequency="8000" nrAudioChannels="1" protocolInfo="http-get:*:%s:*">%s</res>`,
			size, dur, xmlEsc(it.MimeType), xmlEsc(resURL),
		)
		b.WriteString(`</item>`)
	}

	b.WriteString(`</DIDL-Lite>`)

	return b.String(), total
}

func emptyDIDL() string {
	return `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" ` +
		`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" ` +
		`xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/"></DIDL-Lite>`
}

// writeSOAPBrowseResponse writes the full SOAP envelope around the DIDL-Lite result.
func writeSOAPBrowseResponse(w http.ResponseWriter, didl string, returned, total int) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")

	// The DIDL-Lite result must appear as XML-escaped text inside the <Result> element.
	escaped := xmlEsc(didl)

	body := `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body>` +
		`<u:BrowseResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">` +
		`<Result>` + escaped + `</Result>` +
		`<NumberReturned>` + strconv.Itoa(returned) + `</NumberReturned>` +
		`<TotalMatches>` + strconv.Itoa(total) + `</TotalMatches>` +
		`<UpdateID>0</UpdateID>` +
		`</u:BrowseResponse>` +
		`</s:Body>` +
		`</s:Envelope>`

	_, _ = fmt.Fprint(w, body)
}

// ----------------------------------------------------------------------------
// /MediaItems/<id>.<ext>
// ----------------------------------------------------------------------------

func (s *Server) serveMediaItem(w http.ResponseWriter, r *http.Request) {
	// Path: /MediaItems/<id>.<ext>
	rel := strings.TrimPrefix(r.URL.Path, "/MediaItems/")
	// Strip extension.
	dot := strings.LastIndexByte(rel, '.')
	id := rel

	if dot >= 0 {
		id = rel[:dot]
	}

	// The ID may contain '$' which is percent-encoded in URLs.
	// url.PathUnescape would normally handle this, but the mux already decoded it.
	item := s.tree.itemByID(id)
	if item == nil {
		http.NotFound(w, r)

		return
	}

	w.Header().Set("Content-Type", item.MimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(item.Payload)))
	_, _ = w.Write(item.Payload)
}

// ----------------------------------------------------------------------------
// /icons/sm.png
// ----------------------------------------------------------------------------

// tinyPNG is a 1x1 white pixel PNG (67 bytes, entirely static).
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR length + type
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // width=1, height=1
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, // bit depth=8, color=RGB, ...
	0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, // IHDR CRC; IDAT length + type
	0x54, 0x08, 0xd7, 0x63, 0xf8, 0xff, 0xff, 0x3f, // IDAT data (deflate)
	0x00, 0x05, 0xfe, 0x02, 0xfe, 0xdc, 0xcc, 0x59, // IDAT continued
	0xe7, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, // IDAT CRC; IEND length + type
	0x44, 0xae, 0x42, 0x60, 0x82, // IEND CRC
}

func (s *Server) serveIcon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(tinyPNG)))
	_, _ = w.Write(tinyPNG)
}

// ----------------------------------------------------------------------------
// WAV synthesis
// ----------------------------------------------------------------------------

// silentWAV generates a minimal PCM WAV file: mono, 16-bit, given sample rate
// and duration in seconds. All samples are zero (silence).
func silentWAV(durationSec float64, sampleRate, channels int) []byte {
	numSamples := int(float64(sampleRate) * durationSec)
	bitsPerSample := 16
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	dataSize := numSamples * blockAlign
	fileSize := 36 + dataSize

	buf := make([]byte, 44+dataSize)

	// RIFF header
	copy(buf[0:], "RIFF")
	le32(buf[4:], uint32(fileSize))
	copy(buf[8:], "WAVE")

	// fmt chunk
	copy(buf[12:], "fmt ")
	le32(buf[16:], 16) // chunk size
	le16(buf[20:], 1)  // PCM
	le16(buf[22:], uint16(channels))
	le32(buf[24:], uint32(sampleRate))
	le32(buf[28:], uint32(byteRate))
	le16(buf[32:], uint16(blockAlign))
	le16(buf[34:], uint16(bitsPerSample))

	// data chunk
	copy(buf[36:], "data")
	le32(buf[40:], uint32(dataSize))
	// samples are already zero

	return buf
}

func le16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func le32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

// ----------------------------------------------------------------------------
// Utility helpers
// ----------------------------------------------------------------------------

// xmlEsc escapes s for use as XML text content.
func xmlEsc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s)) //nolint:errcheck // strings.Builder never errors

	return b.String()
}

// xmlAttr returns s as a double-quoted XML attribute value with proper escaping.
func xmlAttr(s string) string {
	return `"` + xmlEsc(s) + `"`
}

// urlPathEsc percent-encodes characters that are not safe in a URL path
// segment. We only need to encode '$' (which appears in item IDs).
func urlPathEsc(s string) string {
	return strings.ReplaceAll(s, "$", "%24")
}

// formatDuration converts seconds to "h:mm:ss.mmm" as used in DIDL-Lite.
func formatDuration(sec float64) string {
	ms := int(sec * 1000)
	h := ms / 3600000
	ms -= h * 3600000
	m := ms / 60000
	ms -= m * 60000
	s := ms / 1000
	ms -= s * 1000

	return fmt.Sprintf("%d:%02d:%02d.%03d", h, m, s, ms)
}

// page returns a slice of containers for the requested page.
func page(containers []*Container, start, count int) []*Container {
	if start >= len(containers) {
		return nil
	}

	end := len(containers)
	if count > 0 && start+count < end {
		end = start + count
	}

	return containers[start:end]
}

// pageItems returns a slice of items for the requested page.
func pageItems(items []*Item, start, count int) []*Item {
	if start >= len(items) {
		return nil
	}

	end := len(items)
	if count > 0 && start+count < end {
		end = start + count
	}

	return items[start:end]
}
