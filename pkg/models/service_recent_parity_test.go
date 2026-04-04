package models

import (
	"encoding/xml"
	"testing"
)

func TestServiceRecent_Parity(t *testing.T) {
	t.Run("Unmarshal local response (nested contentItem)", func(t *testing.T) {
		localXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<recent deviceID="" utcTime="1774176828" id="2568595253">
  <contentItem source="Audio" type="" location="/playback/container/c3BvdGlmeTphbGJ1bTo2clQ4eWVyODR4b2gwdDE3cG9Mc21u" sourceAccount="user-name" isPresetable="true">
    <itemName>Coco, Pt. 1</itemName>
  </contentItem>
  <createdOn>2026-03-14T22:39:17.000+00:00</createdOn>
  <updatedOn>2026-03-14T22:39:17.000+00:00</updatedOn>
  <lastplayedat>2026-03-22T10:53:48.000+00:00</lastplayedat>
  <sourceid>10863533</sourceid>
  <source displayName="user-name" secret="TOKEN" secretType="token_version_3" id="10863533" type="Audio" createdOn="2016-01-06T08:52:04.000+00:00" updatedOn="2020-04-25T20:29:11.000+00:00" sourceproviderid="15">
    <sourceKey type="Audio" account="user-name"></sourceKey>
  </source>
</recent>`
		var recent ServiceRecent
		err := xml.Unmarshal([]byte(localXML), &recent)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if recent.ID != "2568595253" {
			t.Errorf("Expected ID 2568595253, got %s", recent.ID)
		}
		if recent.Name != "Coco, Pt. 1" {
			t.Errorf("Expected Name 'Coco, Pt. 1', got %s", recent.Name)
		}
		if recent.SourceID != "10863533" {
			t.Errorf("Expected SourceID 10863533, got %s", recent.SourceID)
		}
	})

	t.Run("Unmarshal upstream response (flat contentItem)", func(t *testing.T) {
		upstreamXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<recent id="2569047180">
  <contentItemType>tracklisturl</contentItemType>
  <createdOn>2026-03-22T10:00:04.000+00:00</createdOn>
  <lastplayedat>2026-03-22T10:53:48.000+00:00</lastplayedat>
  <location>/playback/container/c3BvdGlmeTphbGJ1bTowMUpRS3RjQ1hIZGppVHpHRFk3NXhP</location>
  <name>Dopamine</name>
  <source id="10863533" type="Audio">
    <createdOn>2016-01-06T08:52:04.000+00:00</createdOn>
    <credential type="token_version_3">TOKEN</credential>
    <name>user-name</name>
    <sourceproviderid>15</sourceproviderid>
    <sourcename>user-name@mail.internal</sourcename>
    <sourceSettings/>
    <updatedOn>2020-04-25T20:29:11.000+00:00</updatedOn>
    <username>user-name</username>
  </source>
  <sourceid>10863533</sourceid>
  <updatedOn>2026-03-22T10:53:50.719+00:00</updatedOn>
</recent>`
		var recent ServiceRecent
		err := xml.Unmarshal([]byte(upstreamXML), &recent)
		if err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if recent.ID != "2569047180" {
			t.Errorf("Expected ID 2569047180, got %s", recent.ID)
		}
		if recent.Name != "Dopamine" {
			t.Errorf("Expected Name 'Dopamine', got %s", recent.Name)
		}
		if recent.ContentItemType != "tracklisturl" {
			t.Errorf("Expected ContentItemType 'tracklisturl', got %s", recent.ContentItemType)
		}
		if recent.Location != "/playback/container/c3BvdGlmeTphbGJ1bTowMUpRS3RjQ1hIZGppVHpHRFk3NXhP" {
			t.Errorf("Expected Location '/playback/container/c3BvdGlmeTphbGJ1bTowMUpRS3RjQ1hIZGppVHpHRFk3NXhP', got %s", recent.Location)
		}
		if recent.SourceID != "10863533" {
			t.Errorf("Expected SourceID 10863533, got %s", recent.SourceID)
		}
	})

	t.Run("Marshal ServiceRecent should follow local style (nested)", func(t *testing.T) {
		recent := ServiceRecent{
			ServiceContentItem: ServiceContentItem{
				ID:              "2569047180",
				Name:            "Dopamine",
				ContentItemType: "tracklisturl",
				Location:        "/playback/container/c3BvdGlmeTphbGJ1bTowMUpRS3RjQ1hIZGppVHpHRFk3NXhP",
				SourceID:        "10863533",
				Source:          "SPOTIFY",
				Type:            "tracklisturl",
				SourceAccount:   "user-name",
				IsPresetable:    "true",
			},
			CreatedOn:    "2026-03-22T10:00:04.000+00:00",
			UpdatedOn:    "2026-03-22T10:53:50.719+00:00",
			LastPlayedAt: "2026-03-22T10:53:48.000+00:00",
		}

		data, err := xml.MarshalIndent(recent, "", "  ")
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		xmlStr := string(data)
		if !contains_substr(xmlStr, "<contentItem ") || !contains_substr(xmlStr, "<itemName>Dopamine</itemName>") {
			t.Errorf("Marshaled ServiceRecent missing nested <contentItem> element\nGot: %s", xmlStr)
		}
	})

	t.Run("Marshal RecentItemParity should follow upstream style (flat)", func(t *testing.T) {
		recent := RecentItemParity{
			ID:              "2569047180",
			Name:            "Dopamine",
			ContentItemType: "tracklisturl",
			Location:        "/playback/container/c3BvdGlmeTphbGJ1bTowMUpRS3RjQ1hIZGppVHpHRFk3NXhP",
			SourceID:        "10863533",
			CreatedOn:       "2026-03-22T10:00:04.000+00:00",
			UpdatedOn:       "2026-03-22T10:53:50.719+00:00",
			LastPlayedAt:    "2026-03-22T10:53:48.000+00:00",
			Source: &RecentItemParitySource{
				ID:   "10863533",
				Type: "Audio",
				Credential: &RecentItemParityCredential{
					Type:  "token",
					Value: "",
				},
			},
		}

		data, err := xml.MarshalIndent(recent, "", "  ")
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		xmlStr := string(data)
		expectedElements := []string{
			`<recent id="2569047180">`,
			`<contentItemType>tracklisturl</contentItemType>`,
			`<createdOn>2026-03-22T10:00:04.000+00:00</createdOn>`,
			`<lastplayedat>2026-03-22T10:53:48.000+00:00</lastplayedat>`,
			`<location>/playback/container/c3BvdGlmeTphbGJ1bTowMUpRS3RjQ1hIZGppVHpHRFk3NXhP</location>`,
			`<name>Dopamine</name>`,
			`<sourceid>10863533</sourceid>`,
			`<updatedOn>2026-03-22T10:53:50.719+00:00</updatedOn>`,
			`<credential type="token"></credential>`,
		}

		for _, expected := range expectedElements {
			if !contains_substr(xmlStr, expected) {
				t.Errorf("Marshaled XML missing expected element: %s\nGot: %s", expected, xmlStr)
			}
		}

		// It should NOT have nested contentItem
		if contains_substr(xmlStr, "<contentItem ") || contains_substr(xmlStr, "<contentItem>") {
			t.Errorf("Marshaled RecentItemParity should not have nested <contentItem> element\nGot: %s", xmlStr)
		}
	})

	t.Run("Round-trip: Nested XML -> ServiceRecent -> Unmarshal -> Marshal -> Nested XML", func(t *testing.T) {
		nestedXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<recent deviceID="DEVICE_ID" utcTime="1774176828" id="2568595253">
  <contentItem source="Audio" type="TRACK" location="/playback/container/c3BvdGlmeTphbGJ1bTo2clQ4eWVyODR4b2gwdDE3cG9Mc21u" sourceAccount="user-name" isPresetable="true">
    <itemName>Coco, Pt. 1</itemName>
  </contentItem>
</recent>`
		var recent1 ServiceRecent
		if err := xml.Unmarshal([]byte(nestedXML), &recent1); err != nil {
			t.Fatalf("Unmarshal nested failed: %v", err)
		}

		// Marshal it (should produce nested XML again)
		nestedData, err := xml.MarshalIndent(recent1, "", "  ")
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		xmlStr := string(nestedData)
		if !contains_substr(xmlStr, "<contentItem ") || !contains_substr(xmlStr, "<itemName>Coco, Pt. 1</itemName>") {
			t.Errorf("Round-trip failed to maintain nested structure\nGot: %s", xmlStr)
		}
	})
}

func contains_substr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) > 0 && (s[:len(substr)] == substr || contains_substr(s[1:], substr))))
}
