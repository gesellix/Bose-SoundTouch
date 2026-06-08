// Package stations provides a provider-neutral surface for radio station
// search, navigation, and playback across TuneIn and Radio Browser.
package stations

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/bmx"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

// Provider identifies the radio station source backend.
type Provider string

const (
	// ProviderTuneIn selects the TuneIn radio service.
	ProviderTuneIn Provider = "tunein"
	// ProviderRadioBrowser selects the Radio Browser service.
	ProviderRadioBrowser Provider = "radiobrowser"
)

// Search returns the first page of search results for query from the given provider.
func Search(provider Provider, query string) (*models.BmxNavResponse, error) {
	switch provider {
	case ProviderTuneIn:
		return bmx.TuneInSearch(query)
	case ProviderRadioBrowser:
		return bmx.RadioBrowserSearch(query)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

// SearchNext returns the next page of search results using an opaque cursor.
// The cursor is provider-specific and must not be passed across providers.
func SearchNext(provider Provider, cursor string) (*models.BmxNavResponse, error) {
	switch provider {
	case ProviderTuneIn:
		return bmx.TuneInSearchNext(cursor)
	case ProviderRadioBrowser:
		return bmx.RadioBrowserSearchNext(cursor)
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

// Navigate returns a browse response for the given path under the provider.
// The path is the chi wildcard tail from the /navigate/* route.
// For ProviderRadioBrowser, navigation is not supported.
func Navigate(provider Provider, path string) (*models.BmxNavResponse, error) {
	switch provider {
	case ProviderTuneIn:
		return navigateTuneIn(path)
	case ProviderRadioBrowser:
		return nil, fmt.Errorf("radio browser navigation is not supported")
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

// navigateTuneIn implements the path-parsing logic that was previously inline
// in HandleTuneInNavigate, dispatching to bmx.TuneInNavigate or
// bmx.TuneInNavigateProfile depending on the path prefix.
func navigateTuneIn(wildcard string) (*models.BmxNavResponse, error) {
	if wildcard == "" {
		return bmx.TuneInNavigate("", nil)
	}

	firstSlash := strings.Index(wildcard, "/")
	if firstSlash == -1 {
		return bmx.TuneInNavigate(wildcard, nil)
	}

	pfx := wildcard[:firstSlash]
	rest := wildcard[firstSlash+1:]

	switch pfx {
	case "sub":
		secondSlash := strings.Index(rest, "/")
		if secondSlash == -1 {
			return bmx.TuneInNavigate(rest, nil)
		}

		n, parseErr := strconv.Atoi(rest[:secondSlash])
		if parseErr != nil {
			return bmx.TuneInNavigate(wildcard, nil)
		}

		return bmx.TuneInNavigate(rest[secondSlash+1:], &n)
	case "profiles":
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 3 {
			return bmx.TuneInNavigate(wildcard, nil)
		}

		return bmx.TuneInNavigateProfile(parts[2])
	default:
		return bmx.TuneInNavigate(wildcard, nil)
	}
}

// PlayItem holds all information needed to build a ContentItem and send it to a speaker.
type PlayItem struct {
	Provider Provider
	Location string
	Name     string
	// Type is the ContentItem type; when empty a provider-appropriate default is used.
	Type         string
	ContainerArt string
	// SourceAccount is an optional real credential. Leave empty for anonymous access.
	SourceAccount string
}

// ResolveContentItem builds a *models.ContentItem for the given PlayItem.
// It is pure (no network calls, no client dependency).
func ResolveContentItem(item PlayItem) *models.ContentItem {
	var ci models.ContentItem

	switch item.Provider {
	case ProviderTuneIn:
		ci.Source = "TUNEIN"
		ci.Type = item.Type

		if ci.Type == "" {
			ci.Type = "stationurl"
		}

		ci.IsPresetable = true
		ci.ItemName = item.Name
		ci.Location = item.Location
		ci.ContainerArt = item.ContainerArt
	case ProviderRadioBrowser:
		// Native RADIO_BROWSER source: the speaker prepends the BMX-registry
		// base URL (https://all.api.radio-browser.info/soundtouch) to the
		// relative location and talks to Radio Browser directly. Using
		// source="URL" here makes the speaker fetch the location as a raw
		// audio stream, but it returns station JSON, not audio -> the speaker
		// reports INVALID_SOURCE (issue #479).
		ci.Source = constants.ProviderRadioBrowser
		ci.Type = "stationurl"
		ci.IsPresetable = true
		ci.ItemName = item.Name
		ci.Location = item.Location
	default:
		// Best-effort fallback for unknown providers.
		ci.Source = string(item.Provider)
		ci.Type = item.Type

		if ci.Type == "" {
			ci.Type = "stationurl"
		}

		ci.IsPresetable = true
		ci.ItemName = item.Name
		ci.Location = item.Location
	}

	// Apply the SourceAccount placeholder guard: if SourceAccount is non-empty
	// and is not just the source name echoed back by the speaker, pass it through.
	if item.SourceAccount != "" && item.SourceAccount != ci.Source {
		ci.SourceAccount = item.SourceAccount
	}

	return &ci
}
