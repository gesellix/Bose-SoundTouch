package handlers

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// SCMUDCRequest represents the structure of incoming SCMUDC telemetry data
type SCMUDCRequest struct {
	Envelope struct {
		MonoTime               int64  `json:"monoTime"`
		PayloadProtocolVersion string `json:"payloadProtocolVersion"`
		PayloadType            string `json:"payloadType"`
		ProtocolVersion        string `json:"protocolVersion"`
		Time                   string `json:"time"`
		UniqueID               string `json:"uniqueId"`
	} `json:"envelope"`
	Payload struct {
		DeviceInfo struct {
			BoseID             string `json:"boseID"`
			DeviceID           string `json:"deviceID"`
			DeviceType         string `json:"deviceType"`
			SerialNumber       string `json:"serialNumber"`
			SoftwareVersion    string `json:"softwareVersion"`
			SystemSerialNumber string `json:"systemSerialNumber"`
		} `json:"deviceInfo"`
		Events []SCMUDCEvent `json:"events"`
	} `json:"payload"`
}

// SCMUDCEvent represents individual events within SCMUDC payload
type SCMUDCEvent struct {
	Data     SCMUDCEventData `json:"data"`
	MonoTime int64           `json:"monoTime"`
	Time     string          `json:"time"`
	Type     string          `json:"type"`
}

// SCMUDCEventData contains the event-specific data
type SCMUDCEventData struct {
	ButtonID    string `json:"buttonId,omitempty"`
	ContentItem string `json:"contentItem,omitempty"`
	Origin      string `json:"origin"`
	Preset      string `json:"preset,omitempty"`
}

// EnrichedSCMUDCEvent contains processed and human-readable event information
type EnrichedSCMUDCEvent struct {
	Origin      string          `json:"origin"`
	Action      string          `json:"action"`
	Command     string          `json:"command"`
	Summary     string          `json:"summary"`
	DecodedData *DecodedContent `json:"decoded_data,omitempty"`
	Timestamp   time.Time       `json:"timestamp"`
}

// DecodedContent represents decoded Base64 content from device events
type DecodedContent struct {
	ContentType   string `json:"content_type"`
	ItemName      string `json:"item_name"`
	SourceAccount string `json:"source_account"`
	Location      string `json:"location"`
	ArtworkURL    string `json:"artwork_url,omitempty"`
	IsPresetable  bool   `json:"is_presetable"`
	XMLContent    string `json:"xml_content"`
}

// ContentItemXML represents the XML structure found in Base64-encoded content
type ContentItemXML struct {
	XMLName       xml.Name `xml:"ContentItem"`
	Source        string   `xml:"source,attr"`
	Type          string   `xml:"type,attr"`
	Location      string   `xml:"location,attr"`
	SourceAccount string   `xml:"sourceAccount,attr"`
	IsPresetable  string   `xml:"isPresetable,attr"`
	ItemName      string   `xml:"itemName"`
	ContainerArt  string   `xml:"containerArt"`
}

// SCMUDCEnricher provides functionality to enrich SCMUDC event data
type SCMUDCEnricher struct{}

// NewSCMUDCEnricher creates a new SCMUDC enricher instance
func NewSCMUDCEnricher() *SCMUDCEnricher {
	return &SCMUDCEnricher{}
}

// EnrichSCMUDCRequest processes a raw SCMUDC request and returns enriched event data
func (e *SCMUDCEnricher) EnrichSCMUDCRequest(body []byte) (*EnrichedSCMUDCEvent, error) {
	var scmudcReq SCMUDCRequest
	if err := json.Unmarshal(body, &scmudcReq); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SCMUDC request: %w", err)
	}

	// Process the first event (most requests contain single events)
	if len(scmudcReq.Payload.Events) == 0 {
		return nil, fmt.Errorf("no events found in SCMUDC request")
	}

	event := scmudcReq.Payload.Events[0]

	// Parse timestamp
	timestamp, _ := time.Parse(time.RFC3339, event.Time)

	enriched := &EnrichedSCMUDCEvent{
		Origin:    event.Data.Origin,
		Action:    event.Type,
		Timestamp: timestamp,
	}

	switch event.Data.Origin {
	case "gabbo":
		e.enrichAppEvent(enriched, &event)
	case "console":
		e.enrichConsoleEvent(enriched, &event)
	case "device":
		e.enrichDeviceEvent(enriched, &event)
	default:
		enriched.Command = "Unknown"
		enriched.Summary = fmt.Sprintf("Unknown origin: %s", event.Data.Origin)
	}

	return enriched, nil
}

// enrichAppEvent processes events from the SoundTouch app
func (e *SCMUDCEnricher) enrichAppEvent(enriched *EnrichedSCMUDCEvent, event *SCMUDCEvent) {
	enriched.Command = event.Data.ButtonID
	enriched.Summary = fmt.Sprintf("App: %s", e.formatButton(event.Data.ButtonID))
}

// enrichConsoleEvent processes events from physical device controls
func (e *SCMUDCEnricher) enrichConsoleEvent(enriched *EnrichedSCMUDCEvent, event *SCMUDCEvent) {
	enriched.Command = event.Data.ButtonID
	enriched.Summary = fmt.Sprintf("Device: %s", e.formatButton(event.Data.ButtonID))
}

// enrichDeviceEvent processes internal device events with content data
func (e *SCMUDCEnricher) enrichDeviceEvent(enriched *EnrichedSCMUDCEvent, event *SCMUDCEvent) {
	if event.Data.ContentItem != "" {
		if decoded := e.decodeContentItem(event.Data.ContentItem); decoded != nil {
			enriched.Command = decoded.ItemName
			enriched.Summary = fmt.Sprintf("Device: %s", e.summarizeContent(decoded))
			enriched.DecodedData = decoded

			return
		}
	}

	// Fallback for device events without content
	enriched.Command = "System Action"
	enriched.Summary = fmt.Sprintf("Device: %s", enriched.Action)
}

// decodeContentItem decodes Base64-encoded XML content from device events
func (e *SCMUDCEnricher) decodeContentItem(base64Content string) *DecodedContent {
	data, err := base64.StdEncoding.DecodeString(base64Content)
	if err != nil {
		return nil
	}

	var contentItem ContentItemXML
	if err := xml.Unmarshal(data, &contentItem); err != nil {
		return nil
	}

	return &DecodedContent{
		ContentType:   contentItem.Source,
		ItemName:      contentItem.ItemName,
		SourceAccount: contentItem.SourceAccount,
		Location:      contentItem.Location,
		ArtworkURL:    contentItem.ContainerArt,
		IsPresetable:  strings.EqualFold(contentItem.IsPresetable, "true"),
		XMLContent:    string(data),
	}
}

// formatButton converts button IDs to human-readable names
func (e *SCMUDCEnricher) formatButton(buttonID string) string {
	buttonNames := map[string]string{
		"POWER":       "Power",
		"PLAY":        "Play",
		"PAUSE":       "Pause",
		"STOP":        "Stop",
		"NEXT_TRACK":  "Skip Forward",
		"PREV_TRACK":  "Skip Backward",
		"VOLUME_UP":   "Volume Up",
		"VOLUME_DOWN": "Volume Down",
		"MUTE":        "Mute",
		"PRESET_1":    "Preset 1",
		"PRESET_2":    "Preset 2",
		"PRESET_3":    "Preset 3",
		"PRESET_4":    "Preset 4",
		"PRESET_5":    "Preset 5",
		"PRESET_6":    "Preset 6",
	}

	if name, exists := buttonNames[buttonID]; exists {
		return name
	}

	return buttonID
}

// summarizeContent creates a short summary of content for UI display
func (e *SCMUDCEnricher) summarizeContent(content *DecodedContent) string {
	if content.ItemName != "" {
		return fmt.Sprintf("Playing %s", e.truncateString(content.ItemName, 30))
	}

	if content.ContentType != "" {
		return fmt.Sprintf("Playing %s content", content.ContentType)
	}

	return "Playing content"
}

// truncateString truncates a string to maxLength with ellipsis
func (e *SCMUDCEnricher) truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}

	return s[:maxLength-3] + "..."
}

// GetOriginDescription returns a human-readable description of the event origin
func (e *SCMUDCEnricher) GetOriginDescription(origin string) string {
	descriptions := map[string]string{
		"gabbo":   "SoundTouch App",
		"console": "Device Hardware",
		"device":  "Device Internal",
	}

	if desc, exists := descriptions[origin]; exists {
		return desc
	}

	return "Unknown Origin"
}

// GenerateEnrichedComments creates comment lines for .http files
func (e *SCMUDCEnricher) GenerateEnrichedComments(enriched *EnrichedSCMUDCEvent) []string {
	comments := []string{
		fmt.Sprintf("// Origin: %s (%s)", e.GetOriginDescription(enriched.Origin), enriched.Origin),
		fmt.Sprintf("// Action: %s", enriched.Action),
		fmt.Sprintf("// Command: %s", enriched.Command),
		fmt.Sprintf("// Summary: %s", enriched.Summary),
	}

	if enriched.DecodedData != nil {
		comments = append(comments,
			"// Decoded Content:",
			fmt.Sprintf("//   Source: %s", enriched.DecodedData.ContentType),
			fmt.Sprintf("//   Track: %s", enriched.DecodedData.ItemName),
			fmt.Sprintf("//   Account: %s", enriched.DecodedData.SourceAccount),
		)

		if enriched.DecodedData.ArtworkURL != "" {
			comments = append(comments, fmt.Sprintf("//   Artwork: %s", enriched.DecodedData.ArtworkURL))
		}

		comments = append(comments,
			"//",
			"// Full XML:",
		)

		// Add XML content as comments, line by line
		xmlLines := strings.Split(enriched.DecodedData.XMLContent, "\n")
		for _, line := range xmlLines {
			if strings.TrimSpace(line) != "" {
				comments = append(comments, fmt.Sprintf("// %s", strings.TrimSpace(line)))
			}
		}
	}

	return comments
}

// IsSCMUDCRequest checks if a request path is a SCMUDC endpoint
func IsSCMUDCRequest(path string) bool {
	return strings.Contains(path, "/v1/scmudc/")
}

// GetActionIcon returns an emoji icon for the given action type
func GetActionIcon(action string) string {
	icons := map[string]string{
		"play-pressed":          "▶️",
		"pause-pressed":         "⏸️",
		"power-pressed":         "⚡",
		"stop-pressed":          "⏹️",
		"skip-forward-pressed":  "⏭️",
		"skip-backward-pressed": "⏪",
		"preset-pressed":        "⭐",
		"play-item":             "🎵",
		"preset-assigned":       "🔖",
	}

	if icon, exists := icons[action]; exists {
		return icon
	}

	return "🔘"
}

// GetOriginIcon returns an emoji icon for the given origin
func GetOriginIcon(origin string) string {
	icons := map[string]string{
		"gabbo":   "📱",  // SoundTouch App
		"console": "🎛️", // Device Console
		"device":  "🔄",  // Internal System
	}

	if icon, exists := icons[origin]; exists {
		return icon
	}

	return "❓"
}
