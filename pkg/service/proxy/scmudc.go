package proxy

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
)

// SCMUDCRequest represents the structure of SCMUDC telemetry requests
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

// SCMUDCEvent represents individual events within SCMUDC requests
type SCMUDCEvent struct {
	Type string `json:"type"`
	Data struct {
		ButtonID    string `json:"buttonId,omitempty"`
		Origin      string `json:"origin"`
		ContentItem string `json:"contentItem,omitempty"`
		Preset      string `json:"preset,omitempty"`
	} `json:"data"`
}

// EnrichedSCMUDCEvent contains human-readable analysis of SCMUDC events
type EnrichedSCMUDCEvent struct {
	Origin      string          `json:"origin"`
	Action      string          `json:"action"`
	Command     string          `json:"command"`
	Summary     string          `json:"summary"`
	DecodedData *DecodedContent `json:"decoded_data,omitempty"`
}

// DecodedContent represents decoded ContentItem XML data
type DecodedContent struct {
	ContentType   string `json:"content_type"`
	ItemName      string `json:"item_name"`
	SourceAccount string `json:"source_account,omitempty"`
	Location      string `json:"location,omitempty"`
	ArtworkURL    string `json:"artwork_url,omitempty"`
	IsPresetable  bool   `json:"is_presetable,omitempty"`
	XMLContent    string `json:"xml_content,omitempty"`
}

// ContentItem represents the XML structure within Base64-encoded content
type ContentItem struct {
	XMLName       xml.Name `xml:"ContentItem"`
	Source        string   `xml:"source,attr"`
	Type          string   `xml:"type,attr"`
	Location      string   `xml:"location,attr"`
	SourceAccount string   `xml:"sourceAccount,attr"`
	IsPresetable  string   `xml:"isPresetable,attr"`
	ItemName      string   `xml:"itemName"`
	ContainerArt  string   `xml:"containerArt"`
}

// enrichSCMUDCRequest analyzes and enriches SCMUDC request data
func enrichSCMUDCRequest(body []byte) *EnrichedSCMUDCEvent {
	var scmudcReq SCMUDCRequest
	if err := json.Unmarshal(body, &scmudcReq); err != nil {
		return nil
	}

	// Process the first event (most requests contain single events)
	if len(scmudcReq.Payload.Events) == 0 {
		return nil
	}

	event := scmudcReq.Payload.Events[0]
	enriched := &EnrichedSCMUDCEvent{
		Origin: event.Data.Origin,
		Action: event.Type,
	}

	switch event.Data.Origin {
	case "gabbo":
		enriched.Command = event.Data.ButtonID
		enriched.Summary = fmt.Sprintf("App: %s", formatButton(event.Data.ButtonID))

	case "console":
		enriched.Command = event.Data.ButtonID
		enriched.Summary = fmt.Sprintf("Device: %s", formatButton(event.Data.ButtonID))

	case "device":
		if event.Data.ContentItem != "" {
			if decoded := decodeContentItem(event.Data.ContentItem); decoded != nil {
				enriched.Command = decoded.ItemName
				enriched.Summary = fmt.Sprintf("Device: %s", summarizeContent(decoded))
				enriched.DecodedData = decoded
			} else {
				enriched.Command = "Content Item"
				enriched.Summary = "Device: Content Action"
			}
		} else {
			enriched.Command = "System Action"
			enriched.Summary = "Device: Internal Action"
		}
	}

	return enriched
}

// decodeContentItem decodes Base64-encoded XML content
func decodeContentItem(base64Content string) *DecodedContent {
	data, err := base64.StdEncoding.DecodeString(base64Content)
	if err != nil {
		return nil
	}

	var contentItem ContentItem
	if err := xml.Unmarshal(data, &contentItem); err != nil {
		return nil
	}

	decoded := &DecodedContent{
		ContentType:   contentItem.Source,
		ItemName:      contentItem.ItemName,
		SourceAccount: contentItem.SourceAccount,
		Location:      contentItem.Location,
		ArtworkURL:    contentItem.ContainerArt,
		XMLContent:    string(data),
	}

	if contentItem.IsPresetable == "true" {
		decoded.IsPresetable = true
	}

	return decoded
}

// formatButton converts button IDs to human-readable names
func formatButton(buttonID string) string {
	switch buttonID {
	case "POWER":
		return "Power Button"
	case "PLAY":
		return "Play Button"
	case "PAUSE":
		return "Pause Button"
	case "STOP":
		return "Stop Button"
	case "NEXT_TRACK":
		return "Next Track"
	case "PREV_TRACK":
		return "Previous Track"
	case "PRESET_1", "PRESET_2", "PRESET_3", "PRESET_4", "PRESET_5", "PRESET_6":
		return fmt.Sprintf("Preset %s", strings.TrimPrefix(buttonID, "PRESET_"))
	default:
		return buttonID
	}
}

// summarizeContent creates a brief summary of content items
func summarizeContent(decoded *DecodedContent) string {
	switch decoded.ContentType {
	case "SPOTIFY":
		return fmt.Sprintf("Spotify: %s", decoded.ItemName)
	case "PANDORA":
		return fmt.Sprintf("Pandora: %s", decoded.ItemName)
	case "INTERNET_RADIO":
		return fmt.Sprintf("Radio: %s", decoded.ItemName)
	case "STORED_MUSIC":
		return fmt.Sprintf("Library: %s", decoded.ItemName)
	default:
		if decoded.ItemName != "" {
			return fmt.Sprintf("%s: %s", decoded.ContentType, decoded.ItemName)
		}

		return fmt.Sprintf("%s Content", decoded.ContentType)
	}
}

// getOriginDescription returns human-readable origin descriptions
func getOriginDescription(origin string) string {
	switch origin {
	case "gabbo":
		return "SoundTouch App"
	case "console":
		return "Device Hardware"
	case "device":
		return "Internal System"
	default:
		return origin
	}
}

// generateSCMUDCComments creates enriched comments for .http files
func generateSCMUDCComments(enriched *EnrichedSCMUDCEvent) []string {
	if enriched == nil {
		return nil
	}

	comments := []string{
		fmt.Sprintf("// Origin: %s (%s)", getOriginDescription(enriched.Origin), enriched.Origin),
		fmt.Sprintf("// Action: %s", enriched.Action),
		fmt.Sprintf("// Command: %s", enriched.Command),
		fmt.Sprintf("// Summary: %s", enriched.Summary),
	}

	if enriched.DecodedData != nil {
		comments = append(comments,
			"//",
			"// Decoded Content:",
			fmt.Sprintf("// - Source: %s", enriched.DecodedData.ContentType),
			fmt.Sprintf("// - Item: %s", enriched.DecodedData.ItemName),
		)

		if enriched.DecodedData.SourceAccount != "" {
			comments = append(comments, fmt.Sprintf("// - Account: %s", enriched.DecodedData.SourceAccount))
		}

		if enriched.DecodedData.ArtworkURL != "" {
			comments = append(comments, fmt.Sprintf("// - Artwork: %s", enriched.DecodedData.ArtworkURL))
		}

		if enriched.DecodedData.IsPresetable {
			comments = append(comments, "// - Presetable: Yes")
		}

		if enriched.DecodedData.XMLContent != "" {
			comments = append(comments,
				"//",
				"// Full XML Content:",
			)

			// Add XML content as comments, line by line
			lines := strings.Split(enriched.DecodedData.XMLContent, "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					comments = append(comments, fmt.Sprintf("// %s", strings.TrimSpace(line)))
				}
			}
		}
	}

	return comments
}
