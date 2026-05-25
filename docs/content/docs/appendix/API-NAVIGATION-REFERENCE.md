---
title: "Navigation API Reference"
sidebar:
  exclude: true
---
## Overview

This document provides a complete API reference for the Bose SoundTouch navigation and station management functionality. For usage examples and workflows, see [NAVIGATION-GUIDE.md](NAVIGATION-GUIDE.md).

## Table of Contents

- [Client Methods](#client-methods)
- [Models](#models)
- [HTTP Endpoints](#http-endpoints)
- [XML Schemas](#xml-schemas)
- [Error Codes](#error-codes)

## Client Methods

### Navigation Methods

#### `Navigate(source, sourceAccount string, startItem, numItems int) (*models.NavigateResponse, error)`

Browse content within a source.

**Parameters:**
- `source` (string, required): Content source identifier
  - Valid values: `"TUNEIN"`, `"PANDORA"`, `"SPOTIFY"`, `"STORED_MUSIC"`, `"BLUETOOTH"`, `"AUX"`
- `sourceAccount` (string, optional): Account identifier for authenticated sources
- `startItem` (int, required): Starting position (1-based index)
- `numItems` (int, required): Number of items to retrieve

**Returns:**
- `*models.NavigateResponse`: Navigation results with items and metadata
- `error`: Error if request fails

**Example:**
```go
response, err := client.Navigate("TUNEIN", "", 1, 25)
```

**Validation:**
- `source` cannot be empty
- `startItem` must be >= 1
- `numItems` must be >= 1

---

#### `NavigateWithMenu(source, sourceAccount, menu, sort string, startItem, numItems int) (*models.NavigateResponse, error)`

Browse content with specific menu and sorting options (primarily for Pandora).

**Parameters:**
- `source` (string, required): Content source identifier
- `sourceAccount` (string, optional): Account identifier
- `menu` (string, optional): Menu context (e.g., `"radioStations"`)
- `sort` (string, optional): Sort order (e.g., `"dateCreated"`)
- `startItem` (int, required): Starting position (1-based)
- `numItems` (int, required): Number of items to retrieve

**Returns:**
- `*models.NavigateResponse`: Navigation results
- `error`: Error if request fails

**Example:**
```go
response, err := client.NavigateWithMenu("PANDORA", "user123", "radioStations", "dateCreated", 1, 100)
```

---

#### `NavigateContainer(source, sourceAccount string, startItem, numItems int, containerItem *models.ContentItem) (*models.NavigateResponse, error)`

Browse into a specific container/directory.

**Parameters:**
- `source` (string, required): Content source identifier
- `sourceAccount` (string, optional): Account identifier
- `startItem` (int, required): Starting position (1-based)
- `numItems` (int, required): Number of items to retrieve
- `containerItem` (*models.ContentItem, required): Container to browse into

**Returns:**
- `*models.NavigateResponse`: Container contents
- `error`: Error if request fails

**Example:**
```go
response, err := client.NavigateContainer("STORED_MUSIC", "device/0", 1, 100, albumContentItem)
```

**Validation:**
- `containerItem` cannot be nil
- Container must have valid `Location` field

---

### Convenience Navigation Methods

#### `GetTuneInStations(sourceAccount string) (*models.NavigateResponse, error)`

Browse TuneIn radio stations.

**Parameters:**
- `sourceAccount` (string, optional): TuneIn account (usually empty)

**Returns:**
- `*models.NavigateResponse`: TuneIn stations and content

**Example:**
```go
stations, err := client.GetTuneInStations("")
```

---

#### `GetPandoraStations(sourceAccount string) (*models.NavigateResponse, error)`

Browse Pandora radio stations with proper sorting.

**Parameters:**
- `sourceAccount` (string, required): Pandora user account identifier

**Returns:**
- `*models.NavigateResponse`: Pandora stations sorted by creation date

**Example:**
```go
stations, err := client.GetPandoraStations("user123")
```

**Validation:**
- `sourceAccount` cannot be empty

---

#### `GetStoredMusicLibrary(sourceAccount string) (*models.NavigateResponse, error)`

Browse stored/local music library.

**Parameters:**
- `sourceAccount` (string, required): Device account identifier (format: `deviceID/index`)

**Returns:**
- `*models.NavigateResponse`: Music library root contents

**Example:**
```go
library, err := client.GetStoredMusicLibrary("AABBCCDDEEFF/0")
```

**Validation:**
- `sourceAccount` cannot be empty

---

### Search Methods

#### `SearchStation(source, sourceAccount, searchTerm string) (*models.SearchStationResponse, error)`

Search for stations and content within a music service.

**Parameters:**
- `source` (string, required): Service to search
- `sourceAccount` (string, optional): Account identifier
- `searchTerm` (string, required): Search query

**Returns:**
- `*models.SearchStationResponse`: Search results categorized by type
- `error`: Error if request fails

**Example:**
```go
results, err := client.SearchStation("PANDORA", "user123", "jazz")
```

**Validation:**
- `source` cannot be empty
- `searchTerm` cannot be empty

---

#### `SearchTuneInStations(searchTerm string) (*models.SearchStationResponse, error)`

Search TuneIn radio stations.

**Parameters:**
- `searchTerm` (string, required): Search query

**Returns:**
- `*models.SearchStationResponse`: TuneIn search results

**Example:**
```go
results, err := client.SearchTuneInStations("classical music")
```

---

#### `SearchPandoraStations(sourceAccount, searchTerm string) (*models.SearchStationResponse, error)`

Search Pandora for artists and stations.

**Parameters:**
- `sourceAccount` (string, required): Pandora account identifier
- `searchTerm` (string, required): Artist or genre to search for

**Returns:**
- `*models.SearchStationResponse`: Pandora search results with songs, artists, stations

**Example:**
```go
results, err := client.SearchPandoraStations("user123", "Taylor Swift")
```

**Validation:**
- `sourceAccount` cannot be empty

---

#### `SearchSpotifyContent(sourceAccount, searchTerm string) (*models.SearchStationResponse, error)`

Search Spotify for tracks, albums, and playlists.

**Parameters:**
- `sourceAccount` (string, required): Spotify account identifier
- `searchTerm` (string, required): Content to search for

**Returns:**
- `*models.SearchStationResponse`: Spotify search results

**Example:**
```go
results, err := client.SearchSpotifyContent("user@example.com", "Queen")
```

**Validation:**
- `sourceAccount` cannot be empty

---

### Station Management Methods

#### `AddStation(source, sourceAccount, token, name string) error`

Add a station to music service collection and immediately start playing it.

**Parameters:**
- `source` (string, required): Music service identifier
- `sourceAccount` (string, optional): Account identifier
- `token` (string, required): Station token from search results
- `name` (string, required): Display name for the station

**Returns:**
- `error`: Error if operation fails

**Example:**
```go
err := client.AddStation("PANDORA", "user123", "R4328162", "Classic Rock Radio")
```

**Behavior:**
- Station is immediately selected and starts playing
- Station is added to user's collection permanently
- Generates `presetsUpdated` WebSocket event if station is stored as preset

**Validation:**
- `source` cannot be empty
- `token` cannot be empty
- `name` cannot be empty

---

#### `RemoveStation(contentItem *models.ContentItem) error`

Remove a station from music service collection.

**Parameters:**
- `contentItem` (*models.ContentItem, required): Station content item with source and location

**Returns:**
- `error`: Error if operation fails

**Example:**
```go
err := client.RemoveStation(stationContentItem)
```

**Behavior:**
- Station is removed from user's collection
- If station is currently playing, playback stops
- Generates `nowPlayingUpdated` WebSocket event if playing station was removed

**Validation:**
- `contentItem` cannot be nil
- `contentItem.Source` cannot be empty
- `contentItem.Location` cannot be empty

---

## Models

### NavigateRequest

Request structure for `/navigate` endpoint.

```go
type NavigateRequest struct {
    Source        string        `xml:"source,attr"`
    SourceAccount string        `xml:"sourceAccount,attr,omitempty"`
    Menu          string        `xml:"menu,attr,omitempty"`
    Sort          string        `xml:"sort,attr,omitempty"`
    StartItem     int           `xml:"startItem"`
    NumItems      int           `xml:"numItems"`
    Item          *NavigateItem `xml:"item,omitempty"`
}
```

**Constructors:**
- `NewNavigateRequest(source, sourceAccount string, startItem, numItems int)`
- `NewNavigateRequestWithMenu(source, sourceAccount, menu, sort string, startItem, numItems int)`
- `NewNavigateRequestWithItem(source, sourceAccount string, startItem, numItems int, item *ContentItem)`

---

### NavigateResponse

Response structure from navigation operations.

```go
type NavigateResponse struct {
    Source        string         `xml:"source,attr"`
    SourceAccount string         `xml:"sourceAccount,attr,omitempty"`
    TotalItems    int            `xml:"totalItems"`
    Items         []NavigateItem `xml:"items>item"`
}
```

**Helper Methods:**
- `GetPlayableItems() []NavigateItem` - Filter items with `Playable="1"`
- `GetDirectories() []NavigateItem` - Filter directory items (`type="dir"`)
- `GetTracks() []NavigateItem` - Filter track items (`type="track"`)
- `GetStations() []NavigateItem` - Filter station items (`type="stationurl"`)
- `IsEmpty() bool` - Check if response has no items

---

### NavigateItem

Individual item within navigation response.

```go
type NavigateItem struct {
    Playable           int                 `xml:"Playable,attr,omitempty"`
    Name               string              `xml:"name"`
    Type               string              `xml:"type"`
    ContentItem        *ContentItem        `xml:"ContentItem,omitempty"`
    MediaItemContainer *MediaItemContainer `xml:"mediaItemContainer,omitempty"`
    ArtistName         string              `xml:"artistName,omitempty"`
    AlbumName          string              `xml:"albumName,omitempty"`
}
```

**Helper Methods:**
- `GetDisplayName() string` - Get formatted display name
- `IsPlayable() bool` - Check if `Playable="1"`
- `IsDirectory() bool` - Check if `type="dir"`
- `IsTrack() bool` - Check if `type="track"`
- `IsStation() bool` - Check if `type="stationurl"`
- `GetContentItem() *ContentItem` - Get associated content item
- `GetArtwork() string` - Get artwork URL from content item

**Common Type Values:**
- `"dir"` - Directory/container
- `"track"` - Music track
- `"stationurl"` - Radio station
- `"playlist"` - Playlist
- `"album"` - Album

---

### SearchStationRequest

Request structure for station search.

```go
type SearchStationRequest struct {
    Source        string `xml:"source,attr"`
    SourceAccount string `xml:"sourceAccount,attr,omitempty"`
    SearchTerm    string `xml:",chardata"`
}
```

**Constructor:**
- `NewSearchStationRequest(source, sourceAccount, searchTerm string)`

---

### SearchStationResponse

Response structure from search operations.

```go
type SearchStationResponse struct {
    DeviceID      string         `xml:"deviceID,attr"`
    Source        string         `xml:"source,attr"`
    SourceAccount string         `xml:"sourceAccount,attr,omitempty"`
    Songs         []SearchResult `xml:"songs>searchResult"`
    Artists       []SearchResult `xml:"artists>searchResult"`
    Stations      []SearchResult `xml:"stations>searchResult"`
}
```

**Helper Methods:**
- `GetSongs() []SearchResult` - Get song results
- `GetArtists() []SearchResult` - Get artist results
- `GetStations() []SearchResult` - Get station results
- `GetAllResults() []SearchResult` - Get all results combined
- `GetResultCount() int` - Count total results
- `HasResults() bool` - Check if any results found
- `IsEmpty() bool` - Check if no results

---

### SearchResult

Individual search result item.

```go
type SearchResult struct {
    Source        string `xml:"source,attr"`
    SourceAccount string `xml:"sourceAccount,attr,omitempty"`
    Token         string `xml:"token,attr"`
    Name          string `xml:"name"`
    Artist        string `xml:"artist,omitempty"`
    Album         string `xml:"album,omitempty"`
    Logo          string `xml:"logo,omitempty"`
    Description   string `xml:"description,omitempty"`
}
```

**Helper Methods:**
- `IsSong() bool` - Check if result is a song (has `Artist` field)
- `IsArtist() bool` - Check if result is an artist (no `Artist` or `Description`)
- `IsStation() bool` - Check if result is a station (has `Description`)
- `GetDisplayName() string` - Get formatted name
- `GetFullTitle() string` - Get name with artist for songs
- `GetArtworkURL() string` - Get logo/artwork URL

**Token Usage:**
The `Token` field is used with `AddStation()` to add the result to your collection.

---

### AddStationRequest

Request structure for adding stations.

```go
type AddStationRequest struct {
    Source        string `xml:"source,attr"`
    SourceAccount string `xml:"sourceAccount,attr,omitempty"`
    Token         string `xml:"token,attr"`
    Name          string `xml:"name"`
}
```

**Constructor:**
- `NewAddStationRequest(source, sourceAccount, token, name string)`

---

### StationResponse

Response structure from station management operations.

```go
type StationResponse struct {
    Status string `xml:",chardata"`
}
```

**Common Values:**
- `"/addStation"` - Station added successfully
- `"/removeStation"` - Station removed successfully

---

## HTTP Endpoints

### POST /navigate

Browse content within a source.

**Request Body:**
```xml
<navigate source="TUNEIN" sourceAccount="">
  <startItem>1</startItem>
  <numItems>25</numItems>
</navigate>
```

**Response Body:**
```xml
<navigateResponse source="TUNEIN">
  <totalItems>5</totalItems>
  <items>
    <item Playable="1">
      <name>Station Name</name>
      <type>stationurl</type>
      <ContentItem source="TUNEIN" location="/v1/playback/station/s12345" isPresetable="true">
        <itemName>Station Name</itemName>
      </ContentItem>
    </item>
  </items>
</navigateResponse>
```

---

### POST /searchStation

Search for stations and content.

**Request Body:**
```xml
<search source="PANDORA" sourceAccount="user123">Taylor Swift</search>
```

**Response Body:**
```xml
<results deviceID="AABBCCDDEEFF" source="PANDORA" sourceAccount="user123">
  <songs>
    <searchResult source="PANDORA" sourceAccount="user123" token="S123">
      <name>Love Story</name>
      <artist>Taylor Swift</artist>
      <logo>http://example.com/artwork.jpg</logo>
    </searchResult>
  </songs>
  <artists>
    <searchResult source="PANDORA" sourceAccount="user123" token="R456">
      <name>Taylor Swift</name>
      <logo>http://example.com/artist.jpg</logo>
    </searchResult>
  </artists>
</results>
```

---

### POST /addStation

Add a station to collection and start playing.

**Request Body:**
```xml
<addStation source="PANDORA" sourceAccount="user123" token="R456">
  <name>Taylor Swift Radio</name>
</addStation>
```

**Response Body:**
```xml
<status>/addStation</status>
```

---

### POST /removeStation

Remove a station from collection.

**Request Body:**
```xml
<ContentItem source="PANDORA" location="126740707481236361" sourceAccount="user123" isPresetable="true">
  <itemName>Taylor Swift Radio</itemName>
</ContentItem>
```

**Response Body:**
```xml
<status>/removeStation</status>
```

---

## XML Schemas

### Navigate Request Schema

```xml
<xs:element name="navigate">
  <xs:complexType>
    <xs:sequence>
      <xs:element name="startItem" type="xs:int"/>
      <xs:element name="numItems" type="xs:int"/>
      <xs:element name="item" minOccurs="0">
        <xs:complexType>
          <xs:sequence>
            <xs:element name="name" type="xs:string"/>
            <xs:element name="type" type="xs:string"/>
            <xs:element name="ContentItem" type="ContentItemType"/>
          </xs:sequence>
          <xs:attribute name="Playable" type="xs:int"/>
        </xs:complexType>
      </xs:element>
    </xs:sequence>
    <xs:attribute name="source" type="xs:string" use="required"/>
    <xs:attribute name="sourceAccount" type="xs:string"/>
    <xs:attribute name="menu" type="xs:string"/>
    <xs:attribute name="sort" type="xs:string"/>
  </xs:complexType>
</xs:element>
```

### Search Request Schema

```xml
<xs:element name="search">
  <xs:complexType>
    <xs:simpleContent>
      <xs:extension base="xs:string">
        <xs:attribute name="source" type="xs:string" use="required"/>
        <xs:attribute name="sourceAccount" type="xs:string"/>
      </xs:extension>
    </xs:simpleContent>
  </xs:complexType>
</xs:element>
```

### ContentItem Type Schema

```xml
<xs:complexType name="ContentItemType">
  <xs:sequence>
    <xs:element name="itemName" type="xs:string" minOccurs="0"/>
    <xs:element name="containerArt" type="xs:string" minOccurs="0"/>
  </xs:sequence>
  <xs:attribute name="source" type="xs:string" use="required"/>
  <xs:attribute name="type" type="xs:string"/>
  <xs:attribute name="location" type="xs:string"/>
  <xs:attribute name="sourceAccount" type="xs:string"/>
  <xs:attribute name="isPresetable" type="xs:boolean"/>
</xs:complexType>
```

---

## Error Codes

### HTTP Status Codes

| Status | Meaning | Description |
|--------|---------|-------------|
| 200 | OK | Request successful |
| 400 | Bad Request | Invalid parameters or XML |
| 404 | Not Found | Endpoint or content not found |
| 500 | Internal Server Error | Device error |

### Common Error Responses

**Invalid Source:**
```xml
<error>
  <code>INVALID_SOURCE</code>
  <message>Source 'INVALID' is not available</message>
</error>
```

**Authentication Required:**
```xml
<error>
  <code>AUTH_REQUIRED</code>
  <message>Source account required for this service</message>
</error>
```

**Service Unavailable:**
```xml
<error>
  <code>SERVICE_UNAVAILABLE</code>
  <message>PANDORA service is not configured</message>
</error>
```

### Client-Side Validation Errors

The Go client performs validation before sending requests:

| Error Message | Cause | Solution |
|---------------|-------|----------|
| `"source cannot be empty"` | Empty source parameter | Provide valid source |
| `"search term cannot be empty"` | Empty search query | Provide search term |
| `"startItem must be >= 1"` | Invalid start position | Use 1-based indexing |
| `"numItems must be >= 1"` | Invalid page size | Use positive number |
| `"content item cannot be nil"` | Nil ContentItem | Provide valid ContentItem |
| `"container item cannot be nil"` | Nil container for NavigateContainer | Provide valid container |
| `"Pandora source account cannot be empty"` | Missing Pandora account | Configure Pandora account |
| `"token cannot be empty"` | Missing station token | Use token from search results |
| `"station name cannot be empty"` | Missing station name | Provide station name |

---

## WebSocket Events

Navigation and station operations generate WebSocket events:

### presetsUpdated

Generated when stations are added/removed that affect presets.

```xml
<presetsUpdated deviceID="AABBCCDDEEFF">
  <presets>
    <!-- Updated preset list -->
  </presets>
</presetsUpdated>
```

### nowPlayingUpdated

Generated when station operations affect current playback.

```xml
<nowPlayingUpdated deviceID="AABBCCDDEEFF">
  <nowPlaying source="PANDORA">
    <ContentItem source="PANDORA" location="R456" sourceAccount="user123" isPresetable="true">
      <itemName>Taylor Swift Radio</itemName>
    </ContentItem>
    <track>Love Story</track>
    <artist>Taylor Swift</artist>
    <playStatus>PLAY_STATE</playStatus>
  </nowPlaying>
</nowPlayingUpdated>
```

---

## Best Practices

### Parameter Validation

Always validate parameters before API calls:

```go
func validateNavigateParams(source string, startItem, numItems int) error {
    if source == "" {
        return fmt.Errorf("source cannot be empty")
    }
    if startItem < 1 {
        return fmt.Errorf("startItem must be >= 1")
    }
    if numItems < 1 {
        return fmt.Errorf("numItems must be >= 1")
    }
    return nil
}
```

### Error Handling

Handle both network and API errors:

```go
response, err := client.Navigate("TUNEIN", "", 1, 25)
if err != nil {
    // Check if it's a known API error
    if strings.Contains(err.Error(), "not available") {
        log.Printf("TuneIn not configured on device")
        return
    }
    return fmt.Errorf("navigation failed: %w", err)
}
```

### Pagination

Use appropriate page sizes for different contexts:

```go
// Small pages for interactive browsing
response, err := client.Navigate("TUNEIN", "", 1, 25)

// Larger pages for bulk processing
response, err := client.Navigate("STORED_MUSIC", "device/0", 1, 100)
```

### Resource Management

Cache frequently accessed data:

```go
type CachedClient struct {
    client      *client.Client
    sources     *models.Sources
    sourcesTime time.Time
}

func (c *CachedClient) GetSources() (*models.Sources, error) {
    if c.sources == nil || time.Since(c.sourcesTime) > 5*time.Minute {
        var err error
        c.sources, err = c.client.GetSources()
        c.sourcesTime = time.Now()
        return c.sources, err
    }
    return c.sources, nil
}
```

---

*For complete usage examples and workflows, see [NAVIGATION-GUIDE.md](NAVIGATION-GUIDE.md).*