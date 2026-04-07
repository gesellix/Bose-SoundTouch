package marge

import (
	"encoding/xml"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestLastPlayedAtParity(t *testing.T) {
	now := time.Now().Unix()
	utcTimeStr := strconv.FormatInt(now, 10)
	expectedLastPlayedAt := time.Unix(now, 0).UTC().Format("2006-01-02T15:04:05.000+00:00")

	recents := []models.ServiceRecent{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:   "1",
				Name: "Recent 1",
			},
			UtcTime:      utcTimeStr,
			LastPlayedAt: "", // Empty in datastore
		},
	}

	sources := []models.ConfiguredSource{}

	fullRecents := mapRecentsToFullResponse(recents, sources)

	if len(fullRecents) != 1 {
		t.Fatalf("Expected 1 recent, got %d", len(fullRecents))
	}

	if fullRecents[0].LastPlayedAt != expectedLastPlayedAt {
		t.Errorf("Expected LastPlayedAt %s, got %s", expectedLastPlayedAt, fullRecents[0].LastPlayedAt)
	}

	// Verify XML marshaling
	data, err := xml.Marshal(fullRecents[0])
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	xmlStr := string(data)
	if !strings.Contains(xmlStr, "<lastplayedat>"+expectedLastPlayedAt+"</lastplayedat>") {
		t.Errorf("XML missing expected lastplayedat tag: %s", xmlStr)
	}
}
