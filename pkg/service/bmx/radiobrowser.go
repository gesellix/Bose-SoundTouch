package bmx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

var radioBrowserBaseURL = "https://all.api.radio-browser.info"

// RadioBrowserSearch searches for radio stations using the RadioBrowser API.
func RadioBrowserSearch(query string) (*models.BmxNavResponse, error) {
	searchURL := fmt.Sprintf("%s/json/stations/search?name=%s&limit=20&order=clickcount&reverse=true",
		radioBrowserBaseURL, url.QueryEscape(query))

	resp, err := http.Get(searchURL)
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

	navResp := &models.BmxNavResponse{
		BmxSections: []models.BmxNavSection{
			{
				Name:  "Stations",
				Items: make([]models.BmxNavItem, 0, len(stations)),
			},
		},
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

		// SoundTouch format location for RadioBrowser
		location := fmt.Sprintf("%s/soundtouch/stations/byuuid/%s", radioBrowserBaseURL, uuid)

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
		navResp.BmxSections[0].Items = append(navResp.BmxSections[0].Items, item)
	}

	return navResp, nil
}
