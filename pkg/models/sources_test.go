package models

import (
	"encoding/xml"
	"testing"
)

func TestSourceStatus_UnmarshalXML(t *testing.T) {
	tests := []struct {
		name     string
		xmlInput string
		expected SourceStatus
	}{
		{
			name:     "ready status",
			xmlInput: `<status>READY</status>`,
			expected: SourceStatusReady,
		},
		{
			name:     "unavailable status",
			xmlInput: `<status>UNAVAILABLE</status>`,
			expected: SourceStatusUnavailable,
		},
		{
			name:     "error status",
			xmlInput: `<status>ERROR</status>`,
			expected: SourceStatusError,
		},
		{
			name:     "unknown status defaults to unavailable",
			xmlInput: `<status>UNKNOWN_STATUS</status>`,
			expected: SourceStatusUnavailable,
		},
		{
			name:     "empty status defaults to unavailable",
			xmlInput: `<status></status>`,
			expected: SourceStatusUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status SourceStatus

			err := xml.Unmarshal([]byte(tt.xmlInput), &status)
			if err != nil {
				t.Fatalf("Failed to unmarshal XML: %v", err)
			}

			if status != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, status)
			}
		})
	}
}

func TestSourceStatus_Methods(t *testing.T) {
	tests := []struct {
		status        SourceStatus
		isReady       bool
		isUnavailable bool
		toString      string
	}{
		{SourceStatusReady, true, false, "Ready"},
		{SourceStatusUnavailable, false, true, "Unavailable"},
		{SourceStatusError, false, false, "Error"},
		{SourceStatus("UNKNOWN"), false, false, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.toString, func(t *testing.T) {
			if tt.status.IsReady() != tt.isReady {
				t.Errorf("IsReady() = %v, want %v", tt.status.IsReady(), tt.isReady)
			}
			if tt.status.IsUnavailable() != tt.isUnavailable {
				t.Errorf("IsUnavailable() = %v, want %v", tt.status.IsUnavailable(), tt.isUnavailable)
			}
			if tt.status.String() != tt.toString {
				t.Errorf("String() = %v, want %v", tt.status.String(), tt.toString)
			}
		})
	}
}

func TestSourceItem_Methods(t *testing.T) {
	tests := []struct {
		name                string
		sourceItem          SourceItem
		expectedDisplayName string
		isSpotify           bool
		isBluetooth         bool
		isAux               bool
		isStreaming         bool
		isLocal             bool
		supportsMultiroom   bool
	}{
		{
			name: "spotify source with display name",
			sourceItem: SourceItem{
				Source:           "SPOTIFY",
				SourceAccount:    "user@example.com",
				Status:           SourceStatusReady,
				IsLocal:          false,
				MultiroomAllowed: true,
				DisplayName:      "user+spotify@example.com",
			},
			expectedDisplayName: "user+spotify@example.com",
			isSpotify:           true,
			isBluetooth:         false,
			isAux:               false,
			isStreaming:         true,
			isLocal:             false,
			supportsMultiroom:   true,
		},
		{
			name: "aux source",
			sourceItem: SourceItem{
				Source:           "AUX",
				SourceAccount:    "AUX",
				Status:           SourceStatusReady,
				IsLocal:          true,
				MultiroomAllowed: true,
				DisplayName:      "AUX IN",
			},
			expectedDisplayName: "AUX IN",
			isSpotify:           false,
			isBluetooth:         false,
			isAux:               true,
			isStreaming:         false,
			isLocal:             true,
			supportsMultiroom:   true,
		},
		{
			name: "bluetooth source without display name",
			sourceItem: SourceItem{
				Source:           "BLUETOOTH",
				Status:           SourceStatusUnavailable,
				IsLocal:          true,
				MultiroomAllowed: true,
			},
			expectedDisplayName: "Bluetooth",
			isSpotify:           false,
			isBluetooth:         true,
			isAux:               false,
			isStreaming:         false,
			isLocal:             true,
			supportsMultiroom:   true,
		},
		{
			name: "tunein streaming service",
			sourceItem: SourceItem{
				Source:           "TUNEIN",
				Status:           SourceStatusReady,
				IsLocal:          false,
				MultiroomAllowed: true,
			},
			expectedDisplayName: "Tunein",
			isSpotify:           false,
			isBluetooth:         false,
			isAux:               false,
			isStreaming:         true,
			isLocal:             false,
			supportsMultiroom:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sourceItem.GetDisplayName() != tt.expectedDisplayName {
				t.Errorf("GetDisplayName() = %v, want %v", tt.sourceItem.GetDisplayName(), tt.expectedDisplayName)
			}
			if tt.sourceItem.IsSpotify() != tt.isSpotify {
				t.Errorf("IsSpotify() = %v, want %v", tt.sourceItem.IsSpotify(), tt.isSpotify)
			}
			if tt.sourceItem.IsBluetoothSource() != tt.isBluetooth {
				t.Errorf("IsBluetoothSource() = %v, want %v", tt.sourceItem.IsBluetoothSource(), tt.isBluetooth)
			}
			if tt.sourceItem.IsAuxSource() != tt.isAux {
				t.Errorf("IsAuxSource() = %v, want %v", tt.sourceItem.IsAuxSource(), tt.isAux)
			}
			if tt.sourceItem.IsStreamingService() != tt.isStreaming {
				t.Errorf("IsStreamingService() = %v, want %v", tt.sourceItem.IsStreamingService(), tt.isStreaming)
			}
			if tt.sourceItem.IsLocalSource() != tt.isLocal {
				t.Errorf("IsLocalSource() = %v, want %v", tt.sourceItem.IsLocalSource(), tt.isLocal)
			}
			if tt.sourceItem.SupportsMultiroom() != tt.supportsMultiroom {
				t.Errorf("SupportsMultiroom() = %v, want %v", tt.sourceItem.SupportsMultiroom(), tt.supportsMultiroom)
			}
		})
	}
}

func TestSources_UnmarshalXML(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<sources deviceID="A81B6A536A98">
    <sourceItem source="AUX" sourceAccount="AUX" status="READY" isLocal="true" multiroomallowed="true">AUX IN</sourceItem>
    <sourceItem source="SPOTIFY" sourceAccount="user@example.com" status="READY" isLocal="false" multiroomallowed="true">user+spotify@example.com</sourceItem>
    <sourceItem source="BLUETOOTH" status="UNAVAILABLE" isLocal="true" multiroomallowed="true" />
    <sourceItem source="TUNEIN" status="READY" isLocal="false" multiroomallowed="true" />
</sources>`

	var sources Sources
	err := xml.Unmarshal([]byte(xmlData), &sources)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	// Test basic fields
	if sources.DeviceID != "A81B6A536A98" {
		t.Errorf("Expected DeviceID 'A81B6A536A98', got '%s'", sources.DeviceID)
	}

	if len(sources.SourceItem) != 4 {
		t.Errorf("Expected 4 source items, got %d", len(sources.SourceItem))
	}

	// Test first source item (AUX)
	auxSource := sources.SourceItem[0]
	if auxSource.Source != "AUX" {
		t.Errorf("Expected first source 'AUX', got '%s'", auxSource.Source)
	}
	if auxSource.Status != SourceStatusReady {
		t.Errorf("Expected first source status Ready, got %v", auxSource.Status)
	}
	if !auxSource.IsLocal {
		t.Error("Expected first source to be local")
	}
	if auxSource.DisplayName != "AUX IN" {
		t.Errorf("Expected first source display name 'AUX IN', got '%s'", auxSource.DisplayName)
	}

	// Test Spotify source
	spotifySource := sources.SourceItem[1]
	if spotifySource.Source != "SPOTIFY" {
		t.Errorf("Expected second source 'SPOTIFY', got '%s'", spotifySource.Source)
	}
	if spotifySource.SourceAccount != "user@example.com" {
		t.Errorf("Expected Spotify source account 'user@example.com', got '%s'", spotifySource.SourceAccount)
	}
}

func TestSources_FilterMethods(t *testing.T) {
	sources := Sources{
		DeviceID: "TEST123",
		SourceItem: []SourceItem{
			{Source: "AUX", Status: SourceStatusReady, IsLocal: true, MultiroomAllowed: true},
			{Source: "SPOTIFY", SourceAccount: "user1", Status: SourceStatusReady, IsLocal: false, MultiroomAllowed: true},
			{Source: "SPOTIFY", SourceAccount: "user2", Status: SourceStatusUnavailable, IsLocal: false, MultiroomAllowed: true},
			{Source: "BLUETOOTH", Status: SourceStatusUnavailable, IsLocal: true, MultiroomAllowed: true},
			{Source: "TUNEIN", Status: SourceStatusReady, IsLocal: false, MultiroomAllowed: true},
		},
	}

	// Test GetAvailableSources
	available := sources.GetAvailableSources()
	if len(available) != 3 {
		t.Errorf("Expected 3 available sources, got %d", len(available))
	}

	// Test GetSpotifySources
	spotifySources := sources.GetSpotifySources()
	if len(spotifySources) != 2 {
		t.Errorf("Expected 2 Spotify sources, got %d", len(spotifySources))
	}

	// Test GetReadySpotifySources
	readySpotify := sources.GetReadySpotifySources()
	if len(readySpotify) != 1 {
		t.Errorf("Expected 1 ready Spotify source, got %d", len(readySpotify))
	}

	// Test GetStreamingSources
	streaming := sources.GetStreamingSources()
	if len(streaming) != 3 { // SPOTIFY (2) + TUNEIN (1)
		t.Errorf("Expected 3 streaming sources, got %d", len(streaming))
	}

	// Test GetLocalSources
	local := sources.GetLocalSources()
	if len(local) != 2 { // AUX + BLUETOOTH
		t.Errorf("Expected 2 local sources, got %d", len(local))
	}

	// Test GetMultiroomSources
	multiroom := sources.GetMultiroomSources()
	if len(multiroom) != 5 { // All sources support multiroom in this test
		t.Errorf("Expected 5 multiroom sources, got %d", len(multiroom))
	}

	// Test HasSource methods
	if !sources.HasSpotify() {
		t.Error("Expected HasSpotify() to return true")
	}

	if sources.HasBluetooth() {
		t.Error("Expected HasBluetooth() to return false (unavailable)")
	}

	if !sources.HasAux() {
		t.Error("Expected HasAux() to return true")
	}

	// Test count methods
	if sources.GetSourceCount() != 5 {
		t.Errorf("Expected total source count 5, got %d", sources.GetSourceCount())
	}

	if sources.GetReadySourceCount() != 3 {
		t.Errorf("Expected ready source count 3, got %d", sources.GetReadySourceCount())
	}
}

func TestSources_EmptyResponse(t *testing.T) {
	sources := Sources{
		DeviceID:   "EMPTY123",
		SourceItem: []SourceItem{},
	}

	// Test empty sources
	if len(sources.GetAvailableSources()) != 0 {
		t.Error("Expected no available sources for empty response")
	}

	if sources.HasSpotify() {
		t.Error("Expected HasSpotify() to return false for empty response")
	}

	if sources.GetSourceCount() != 0 {
		t.Errorf("Expected source count 0 for empty response, got %d", sources.GetSourceCount())
	}
}

func TestSourceItem_GetDisplayName_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		sourceItem   SourceItem
		expectedName string
	}{
		{
			name: "with display name",
			sourceItem: SourceItem{
				Source:      "SPOTIFY",
				DisplayName: "My Spotify",
			},
			expectedName: "My Spotify",
		},
		{
			name: "with source account different from source",
			sourceItem: SourceItem{
				Source:        "SPOTIFY",
				SourceAccount: "user@example.com",
			},
			expectedName: "user@example.com",
		},
		{
			name: "with source account same as source",
			sourceItem: SourceItem{
				Source:        "AUX",
				SourceAccount: "AUX",
			},
			expectedName: "Aux",
		},
		{
			name: "no display name or account",
			sourceItem: SourceItem{
				Source: "BLUETOOTH",
			},
			expectedName: "Bluetooth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sourceItem.GetDisplayName() != tt.expectedName {
				t.Errorf("GetDisplayName() = %v, want %v", tt.sourceItem.GetDisplayName(), tt.expectedName)
			}
		})
	}
}
