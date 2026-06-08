package bmx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

var radioBrowserBaseURL = "https://all.api.radio-browser.info"

const radioBrowserPageSize = 20

// radioBrowserCursor is the opaque pagination cursor for RadioBrowser search results.
type radioBrowserCursor struct {
	Query      string `json:"q"`
	NextOffset int    `json:"o"`
}

// RadioBrowserSearch searches for radio stations using the RadioBrowser API (first page).
func RadioBrowserSearch(query string) (*models.BmxNavResponse, error) {
	return RadioBrowserSearchPage(query, 0)
}

// RadioBrowserSearchPage searches for radio stations at a specific offset.
func RadioBrowserSearchPage(query string, offset int) (*models.BmxNavResponse, error) {
	searchURL := fmt.Sprintf("%s/json/stations/search?name=%s&limit=%d&offset=%d&hidebroken=true&order=clickcount&reverse=true",
		radioBrowserBaseURL, url.QueryEscape(query), radioBrowserPageSize, offset)

	resp, err := http.Get(searchURL) //nolint:noctx
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("radio-browser search failed with status %d", resp.StatusCode)
	}

	var stations []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stations); err != nil {
		return nil, err
	}

	section := models.BmxNavSection{
		Name:  "Stations",
		Items: make([]models.BmxNavItem, 0, len(stations)),
	}

	for _, station := range stations {
		name, _ := station["name"].(string)
		uuid, _ := station["stationuuid"].(string)
		favicon, _ := station["favicon"].(string)
		country, _ := station["country"].(string)
		tags, _ := station["tags"].(string)

		subtitle := country
		if tags != "" {
			if subtitle != "" {
				subtitle += " · "
			}

			subtitle += tags
		}

		// Relative SoundTouch playback location for RadioBrowser. The speaker
		// prepends the BMX-registry base URL (radioBrowserBaseURL + "/soundtouch")
		// when it follows a RADIO_BROWSER source, so the href must stay relative.
		location := fmt.Sprintf("/stations/byuuid/%s", uuid)

		item := models.BmxNavItem{
			Name:     name,
			ImageUrl: favicon,
			Subtitle: subtitle,
			Links: &models.Links{
				BmxPlayback: &models.Link{
					Href: location,
					Type: "stationurl",
				},
			},
		}
		section.Items = append(section.Items, item)
	}

	// Attach a BmxNext link only when the page is full (more results likely exist).
	if len(stations) == radioBrowserPageSize {
		cursorData := radioBrowserCursor{Query: query, NextOffset: offset + radioBrowserPageSize}

		cursorJSON, err := json.Marshal(cursorData)
		if err == nil {
			encoded := base64.RawURLEncoding.EncodeToString(cursorJSON)

			section.Links = &models.Links{
				BmxNext: &models.Link{Href: "/v1/radiobrowser/search/next?cursor=" + encoded},
			}
		}
	}

	navResp := &models.BmxNavResponse{
		BmxSections: []models.BmxNavSection{section},
	}

	return navResp, nil
}

// RadioBrowserSearchNext fetches the next page of RadioBrowser search results using the
// opaque cursor produced by RadioBrowserSearchPage.
func RadioBrowserSearchNext(encodedCursor string) (*models.BmxNavResponse, error) {
	cursorBytes, err := base64.RawURLEncoding.DecodeString(encodedCursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	var cursor radioBrowserCursor
	if err := json.Unmarshal(cursorBytes, &cursor); err != nil {
		return nil, fmt.Errorf("invalid cursor: %w", err)
	}

	if cursor.Query == "" {
		return nil, fmt.Errorf("invalid cursor: missing query")
	}

	if cursor.NextOffset < 0 {
		return nil, fmt.Errorf("invalid cursor: negative offset")
	}

	return RadioBrowserSearchPage(cursor.Query, cursor.NextOffset)
}
