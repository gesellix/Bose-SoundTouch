package models

import (
	"encoding/xml"
	"testing"
	"time"
)

func TestName_UnmarshalXML(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?><name>Sound Machinechen</name>`

	var name Name
	err := xml.Unmarshal([]byte(xmlData), &name)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	if name.Value != "Sound Machinechen" {
		t.Errorf("Expected name 'Sound Machinechen', got '%s'", name.Value)
	}

	if name.GetName() != "Sound Machinechen" {
		t.Errorf("Expected GetName() 'Sound Machinechen', got '%s'", name.GetName())
	}

	if name.String() != "Sound Machinechen" {
		t.Errorf("Expected String() 'Sound Machinechen', got '%s'", name.String())
	}

	if name.IsEmpty() {
		t.Error("Expected IsEmpty() to return false for non-empty name")
	}
}

func TestName_EmptyName(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?><name></name>`

	var name Name
	err := xml.Unmarshal([]byte(xmlData), &name)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	if !name.IsEmpty() {
		t.Error("Expected IsEmpty() to return true for empty name")
	}

	if name.GetName() != "" {
		t.Errorf("Expected GetName() to return empty string, got '%s'", name.GetName())
	}
}

func TestCapabilities_UnmarshalXML(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<capabilities deviceID="A81B6A536A98">
    <networkConfig>
        <dualMode>true</dualMode>
        <wsapiproxy>true</wsapiproxy>
        <allInterfacesSupported />
        <wlanInterfaces />
        <security />
    </networkConfig>
    <dspCapabilities>
        <dspMonoStereo available="false" />
    </dspCapabilities>
    <lightswitch>false</lightswitch>
    <clockDisplay>false</clockDisplay>
    <capability name="systemtimeout" url="/systemtimeout" info="" />
    <capability name="rebroadcastlatencymode" url="/rebroadcastlatencymode" info="" />
    <lrStereoCapable>true</lrStereoCapable>
    <bcoresetCapable>false</bcoresetCapable>
    <disablePowerSaving>true</disablePowerSaving>
</capabilities>`

	var capabilities Capabilities
	err := xml.Unmarshal([]byte(xmlData), &capabilities)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	// Test basic fields
	if capabilities.DeviceID != "A81B6A536A98" {
		t.Errorf("Expected DeviceID 'A81B6A536A98', got '%s'", capabilities.DeviceID)
	}

	// Test boolean capabilities
	if capabilities.HasLightswitch() {
		t.Error("Expected HasLightswitch() to return false")
	}

	if capabilities.HasClockDisplay() {
		t.Error("Expected HasClockDisplay() to return false")
	}

	if !capabilities.HasLRStereoCapability() {
		t.Error("Expected HasLRStereoCapability() to return true")
	}

	if capabilities.HasBCOResetCapability() {
		t.Error("Expected HasBCOResetCapability() to return false")
	}

	if !capabilities.HasPowerSavingDisabled() {
		t.Error("Expected HasPowerSavingDisabled() to return true")
	}

	// Test network capabilities
	if !capabilities.HasDualModeNetwork() {
		t.Error("Expected HasDualModeNetwork() to return true")
	}

	if !capabilities.HasWSAPIProxy() {
		t.Error("Expected HasWSAPIProxy() to return true")
	}

	// Test DSP capabilities
	if capabilities.HasDSPMonoStereo() {
		t.Error("Expected HasDSPMonoStereo() to return false")
	}

	// Test capability by name
	if !capabilities.HasCapability("systemtimeout") {
		t.Error("Expected to have systemtimeout capability")
	}

	if capabilities.HasCapability("nonexistent") {
		t.Error("Expected to not have nonexistent capability")
	}

	// Test capability names
	capNames := capabilities.GetCapabilityNames()
	if len(capNames) != 2 {
		t.Errorf("Expected 2 capability names, got %d", len(capNames))
	}

	// Test summaries
	networkCaps := capabilities.GetNetworkCapabilities()
	expectedNetworkCaps := []string{"Dual Mode", "WSAPI Proxy"}
	if len(networkCaps) != len(expectedNetworkCaps) {
		t.Errorf("Expected %d network capabilities, got %d", len(expectedNetworkCaps), len(networkCaps))
	}

	audioCaps := capabilities.GetAudioCapabilities()
	expectedAudioCaps := []string{"L/R Stereo"}
	if len(audioCaps) != len(expectedAudioCaps) {
		t.Errorf("Expected %d audio capabilities, got %d", len(expectedAudioCaps), len(audioCaps))
	}

	systemCaps := capabilities.GetSystemCapabilities()
	expectedSystemCaps := []string{"Power Saving Disabled"}
	if len(systemCaps) != len(expectedSystemCaps) {
		t.Errorf("Expected %d system capabilities, got %d", len(expectedSystemCaps), len(systemCaps))
	}
}

func TestCapabilities_HostedWifiConfig(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<capabilities deviceID="1234567890AB">
    <networkConfig>
        <hostedWifiConfigWebPage hostedBy="BCO" generation="1" port="80">true</hostedWifiConfigWebPage>
        <wsapiproxy>false</wsapiproxy>
        <allInterfacesSupported />
        <wlanInterfaces />
        <security />
    </networkConfig>
    <lightswitch>true</lightswitch>
    <clockDisplay>true</clockDisplay>
</capabilities>`

	var capabilities Capabilities
	err := xml.Unmarshal([]byte(xmlData), &capabilities)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	if !capabilities.HasHostedWifiConfig() {
		t.Error("Expected HasHostedWifiConfig() to return true")
	}

	if capabilities.GetHostedWifiPort() != "80" {
		t.Errorf("Expected hosted wifi port '80', got '%s'", capabilities.GetHostedWifiPort())
	}

	if capabilities.GetHostedWifiHostedBy() != "BCO" {
		t.Errorf("Expected hosted wifi hosted by 'BCO', got '%s'", capabilities.GetHostedWifiHostedBy())
	}

	if !capabilities.HasLightswitch() {
		t.Error("Expected HasLightswitch() to return true")
	}

	if !capabilities.HasClockDisplay() {
		t.Error("Expected HasClockDisplay() to return true")
	}
}

func TestPresets_UnmarshalXML(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<presets>
    <preset id="1" createdOn="1719128436" updatedOn="1728740382">
        <ContentItem source="SPOTIFY" type="tracklisturl" location="/playback/container/abc123" sourceAccount="user@example.com" isPresetable="true">
            <itemName>My Playlist</itemName>
            <containerArt>https://i.scdn.co/image/ab67616d00001e025ff75c5d082fc50a3a74ad7b</containerArt>
        </ContentItem>
    </preset>
    <preset id="2" createdOn="1703353552" updatedOn="1743615710">
        <ContentItem source="SPOTIFY" type="tracklisturl" location="/playback/container/def456" sourceAccount="user@example.com" isPresetable="true">
            <itemName>Chill Music Collection</itemName>
            <containerArt>https://i.scdn.co/image/ab67616d0000b273e07c8adc6fb49168dc8b7a2f</containerArt>
        </ContentItem>
    </preset>
    <preset id="3">
        <ContentItem source="TUNEIN" type="stationurl" location="http://stream.example.com" isPresetable="true">
            <itemName>Radio Station</itemName>
        </ContentItem>
    </preset>
</presets>`

	var presets Presets
	err := xml.Unmarshal([]byte(xmlData), &presets)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	// Test basic structure
	if presets.GetPresetCount() != 3 {
		t.Errorf("Expected 3 presets, got %d", presets.GetPresetCount())
	}

	// Test first preset
	preset1 := presets.GetPresetByID(1)
	if preset1 == nil {
		t.Fatal("Expected to find preset with ID 1")
	}

	if preset1.GetDisplayName() != "My Playlist" {
		t.Errorf("Expected display name 'My Playlist', got '%s'", preset1.GetDisplayName())
	}

	if !preset1.IsSpotifyPreset() {
		t.Error("Expected preset 1 to be a Spotify preset")
	}

	if preset1.GetSource() != "SPOTIFY" {
		t.Errorf("Expected source 'SPOTIFY', got '%s'", preset1.GetSource())
	}

	if preset1.GetSourceAccount() != "user@example.com" {
		t.Errorf("Expected source account 'user@example.com', got '%s'", preset1.GetSourceAccount())
	}

	if !preset1.HasTimestamps() {
		t.Error("Expected preset 1 to have timestamps")
	}

	// Test timestamps
	expectedCreated := time.Unix(1719128436, 0)
	if !preset1.GetCreatedTime().Equal(expectedCreated) {
		t.Errorf("Expected created time %v, got %v", expectedCreated, preset1.GetCreatedTime())
	}

	expectedUpdated := time.Unix(1728740382, 0)
	if !preset1.GetUpdatedTime().Equal(expectedUpdated) {
		t.Errorf("Expected updated time %v, got %v", expectedUpdated, preset1.GetUpdatedTime())
	}

	// Test third preset (TuneIn radio)
	preset3 := presets.GetPresetByID(3)
	if preset3 == nil {
		t.Fatal("Expected to find preset with ID 3")
	}

	if preset3.IsSpotifyPreset() {
		t.Error("Expected preset 3 to not be a Spotify preset")
	}

	if preset3.GetSource() != "TUNEIN" {
		t.Errorf("Expected source 'TUNEIN', got '%s'", preset3.GetSource())
	}

	if preset3.HasTimestamps() {
		t.Error("Expected preset 3 to not have timestamps")
	}

	// Test filtering methods
	spotifyPresets := presets.GetSpotifyPresets()
	if len(spotifyPresets) != 2 {
		t.Errorf("Expected 2 Spotify presets, got %d", len(spotifyPresets))
	}

	tuneinPresets := presets.GetPresetsBySource("TUNEIN")
	if len(tuneinPresets) != 1 {
		t.Errorf("Expected 1 TuneIn preset, got %d", len(tuneinPresets))
	}

	// Test empty slots
	emptySlots := presets.GetEmptyPresetSlots()
	expectedEmpty := []int{4, 5, 6}
	if len(emptySlots) != len(expectedEmpty) {
		t.Errorf("Expected %d empty slots, got %d", len(expectedEmpty), len(emptySlots))
	}

	// Test used slots
	usedSlots := presets.GetUsedPresetSlots()
	expectedUsed := []int{1, 2, 3}
	if len(usedSlots) != len(expectedUsed) {
		t.Errorf("Expected %d used slots, got %d", len(expectedUsed), len(usedSlots))
	}

	// Test summary
	summary := presets.GetPresetsSummary()
	if summary["total"] != 3 {
		t.Errorf("Expected total 3, got %d", summary["total"])
	}
	if summary["used"] != 3 {
		t.Errorf("Expected used 3, got %d", summary["used"])
	}
	if summary["spotify"] != 2 {
		t.Errorf("Expected spotify 2, got %d", summary["spotify"])
	}
	if summary["SPOTIFY"] != 2 {
		t.Errorf("Expected SPOTIFY 2, got %d", summary["SPOTIFY"])
	}
	if summary["TUNEIN"] != 1 {
		t.Errorf("Expected TUNEIN 1, got %d", summary["TUNEIN"])
	}

	// Test most recent preset
	mostRecent := presets.GetMostRecentPreset()
	if mostRecent == nil {
		t.Fatal("Expected to find most recent preset")
	}
	if mostRecent.ID != 2 {
		t.Errorf("Expected most recent preset ID 2, got %d", mostRecent.ID)
	}

	// Test oldest preset
	oldest := presets.GetOldestPreset()
	if oldest == nil {
		t.Fatal("Expected to find oldest preset")
	}
	if oldest.ID != 2 {
		t.Errorf("Expected oldest preset ID 2, got %d", oldest.ID)
	}
}

func TestPresets_EmptyPresets(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?><presets></presets>`

	var presets Presets
	err := xml.Unmarshal([]byte(xmlData), &presets)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	if presets.HasPresets() {
		t.Error("Expected HasPresets() to return false for empty presets")
	}

	if presets.GetPresetCount() != 0 {
		t.Errorf("Expected preset count 0, got %d", presets.GetPresetCount())
	}

	emptySlots := presets.GetEmptyPresetSlots()
	expectedSlots := []int{1, 2, 3, 4, 5, 6}
	if len(emptySlots) != len(expectedSlots) {
		t.Errorf("Expected %d empty slots, got %d", len(expectedSlots), len(emptySlots))
	}

	if presets.GetMostRecentPreset() != nil {
		t.Error("Expected GetMostRecentPreset() to return nil for empty presets")
	}

	if presets.GetOldestPreset() != nil {
		t.Error("Expected GetOldestPreset() to return nil for empty presets")
	}
}

func TestPreset_EdgeCases(t *testing.T) {
	// Test preset without ContentItem
	emptyPreset := Preset{ID: 1}

	if !emptyPreset.IsEmpty() {
		t.Error("Expected IsEmpty() to return true for preset without ContentItem")
	}

	if emptyPreset.GetDisplayName() != "Preset 1" {
		t.Errorf("Expected display name 'Preset 1', got '%s'", emptyPreset.GetDisplayName())
	}

	if emptyPreset.GetSource() != "" {
		t.Errorf("Expected empty source, got '%s'", emptyPreset.GetSource())
	}

	if emptyPreset.GetArtworkURL() != "" {
		t.Errorf("Expected empty artwork URL, got '%s'", emptyPreset.GetArtworkURL())
	}

	if emptyPreset.IsSpotifyPreset() {
		t.Error("Expected IsSpotifyPreset() to return false for empty preset")
	}

	// Test preset with ContentItem but no artwork
	presetNoArt := Preset{
		ID: 2,
		ContentItem: &ContentItem{
			Source:   "TUNEIN",
			ItemName: "Test Station",
		},
	}

	if presetNoArt.GetArtworkURL() != "" {
		t.Errorf("Expected empty artwork URL, got '%s'", presetNoArt.GetArtworkURL())
	}

	if presetNoArt.GetDisplayName() != "Test Station" {
		t.Errorf("Expected display name 'Test Station', got '%s'", presetNoArt.GetDisplayName())
	}
}
