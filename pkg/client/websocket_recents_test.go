package client

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestWebSocketClient_RecentsUpdated(t *testing.T) {
	tests := []struct {
		name        string
		frame       string
		setHandlers bool
		wantItems   int
	}{
		{
			name:  "notification only",
			frame: `<updates deviceID="DEVICEID01"><recentsUpdated/></updates>`,
		},
		{
			name: "with payload via SetHandlers",
			frame: `<updates deviceID="DEVICEID01"><recentsUpdated><recents>
				<recent deviceID="DEVICEID01" createdOn="123" id="recent-1">
					<ContentItem source="INTERNET_RADIO" type="stationurl" location="https://example.com/radio" sourceAccount="account-1" isPresetable="true">
						<itemName>Example Radio</itemName>
					</ContentItem>
				</recent>
			</recents></recentsUpdated></updates>`,
			setHandlers: true,
			wantItems:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsClient := NewClientFromHost("192.0.2.10").NewWebSocketClient(nil)
			var got *models.RecentsUpdatedEvent
			calls := 0
			handler := func(event *models.RecentsUpdatedEvent) {
				got = event
				calls++
			}
			if tt.setHandlers {
				wsClient.SetHandlers(&models.WebSocketEventHandlers{OnRecentsUpdated: handler})
			} else {
				wsClient.OnRecentsUpdated(handler)
			}
			wsClient.OnUnknownEvent(func(*models.WebSocketEvent) {
				t.Error("recentsUpdated reached the unknown event handler")
			})

			wsClient.handleMessage([]byte(tt.frame))

			if calls != 1 || got == nil {
				t.Fatalf("OnRecentsUpdated calls = %d, event = %+v; want one non-nil event", calls, got)
			}
			if got.DeviceID != "DEVICEID01" {
				t.Errorf("DeviceID = %q, want DEVICEID01", got.DeviceID)
			}
			if len(got.Recents.Items) != tt.wantItems {
				t.Fatalf("received %d recent items, want %d", len(got.Recents.Items), tt.wantItems)
			}
			if tt.wantItems == 0 {
				return
			}
			item := got.Recents.Items[0]
			if item.ID != "recent-1" || item.CreatedOn != 123 || item.DeviceID != "DEVICEID01" {
				t.Errorf("recent metadata = %+v", item)
			}
			wantContent := models.ContentItem{
				Source: "INTERNET_RADIO", Type: "stationurl", Location: "https://example.com/radio",
				SourceAccount: "account-1", IsPresetable: true, ItemName: "Example Radio",
			}
			if item.ContentItem != wantContent {
				t.Errorf("ContentItem = %+v, want %+v", item.ContentItem, wantContent)
			}
		})
	}
}

func TestWebSocketClient_RecentsUpdatedWithoutHandler(t *testing.T) {
	wsClient := NewClientFromHost("192.0.2.10").NewWebSocketClient(nil)
	unknownCalls := 0
	wsClient.OnUnknownEvent(func(*models.WebSocketEvent) { unknownCalls++ })

	wsClient.handleMessage([]byte(`<updates deviceID="DEVICEID01"><recentsUpdated/></updates>`))
	wsClient.OnRecentsUpdated(func(*models.RecentsUpdatedEvent) { t.Error("cleared handler was called") })
	wsClient.OnRecentsUpdated(nil)
	wsClient.handleMessage([]byte(`<updates deviceID="DEVICEID01"><recentsUpdated/></updates>`))
	if unknownCalls != 0 {
		t.Fatalf("recentsUpdated called unknown handler %d times without a typed callback", unknownCalls)
	}

	wsClient.handleMessage([]byte(`<updates deviceID="DEVICEID01"><futureEvent/></updates>`))
	if unknownCalls != 1 {
		t.Errorf("unknown event handler calls = %d, want 1", unknownCalls)
	}
}
