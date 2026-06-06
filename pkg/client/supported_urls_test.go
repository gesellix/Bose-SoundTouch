package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestClient_GetSupportedURLs(t *testing.T) {
	tests := []struct {
		name             string
		responseXML      string
		expectedError    bool
		expectedDeviceID string
		expectedURLCount int
		expectedURLs     []string
	}{
		{
			name: "successful_supported_urls_retrieval",
			responseXML: `<?xml version="1.0" encoding="UTF-8"?>
<supportedURLs deviceID="AABBCCDDEE0A">
	<URL location="/info" />
	<URL location="/capabilities" />
	<URL location="/supportedURLs" />
	<URL location="/volume" />
	<URL location="/bass" />
	<URL location="/balance" />
	<URL location="/presets" />
	<URL location="/nowPlaying" />
	<URL location="/key" />
	<URL location="/sources" />
	<URL location="/serviceAvailability" />
	<URL location="/navigate" />
	<URL location="/search" />
	<URL location="/addStation" />
	<URL location="/removeStation" />
	<URL location="/clock" />
	<URL location="/name" />
	<URL location="/networkInfo" />
	<URL location="/setZone" />
	<URL location="/addZoneSlave" />
	<URL location="/removeZoneSlave" />
	<URL location="/audiodspcontrols" />
	<URL location="/audioproducttonecontrols" />
	<URL location="/audioproductlevelcontrols" />
	<URL location="/bassCapabilities" />
</supportedURLs>`,
			expectedError:    false,
			expectedDeviceID: "AABBCCDDEE0A",
			expectedURLCount: 25,
			expectedURLs: []string{
				"/info", "/capabilities", "/supportedURLs", "/volume", "/bass",
				"/balance", "/presets", "/nowPlaying", "/key", "/sources",
				"/serviceAvailability", "/navigate", "/search", "/addStation",
				"/removeStation", "/clock", "/name", "/networkInfo", "/setZone",
				"/addZoneSlave", "/removeZoneSlave", "/audiodspcontrols",
				"/audioproducttonecontrols", "/audioproductlevelcontrols", "/bassCapabilities",
			},
		},
		{
			name: "minimal_device_supported_urls",
			responseXML: `<?xml version="1.0" encoding="UTF-8"?>
<supportedURLs deviceID="12345">
	<URL location="/info" />
	<URL location="/capabilities" />
	<URL location="/volume" />
	<URL location="/nowPlaying" />
	<URL location="/key" />
</supportedURLs>`,
			expectedError:    false,
			expectedDeviceID: "12345",
			expectedURLCount: 5,
			expectedURLs:     []string{"/info", "/capabilities", "/volume", "/nowPlaying", "/key"},
		},
		{
			name: "empty_supported_urls",
			responseXML: `<?xml version="1.0" encoding="UTF-8"?>
<supportedURLs deviceID="EMPTY123">
</supportedURLs>`,
			expectedError:    false,
			expectedDeviceID: "EMPTY123",
			expectedURLCount: 0,
			expectedURLs:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.URL.Path != "/supportedURLs" {
					t.Errorf("Expected path '/supportedURLs', got '%s'", r.URL.Path)
				}

				if r.Method != "GET" {
					t.Errorf("Expected GET method, got '%s'", r.Method)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.responseXML))
			}))
			defer server.Close()

			// Parse server URL
			serverURL, _ := url.Parse(server.URL)
			host := serverURL.Hostname()
			port, _ := strconv.Atoi(serverURL.Port())

			// Create client
			client := NewClient(&Config{
				Host: host,
				Port: port,
			})

			// Test GetSupportedURLs
			supportedURLs, err := client.GetSupportedURLs()

			// Check error expectation
			if tt.expectedError && err == nil {
				t.Errorf("Expected error, but got none")
			}

			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectedError {
				// Verify device ID
				if supportedURLs.DeviceID != tt.expectedDeviceID {
					t.Errorf("Expected device ID '%s', got '%s'", tt.expectedDeviceID, supportedURLs.DeviceID)
				}

				// Verify URL count
				if supportedURLs.GetURLCount() != tt.expectedURLCount {
					t.Errorf("Expected %d URLs, got %d", tt.expectedURLCount, supportedURLs.GetURLCount())
				}

				// Verify specific URLs
				urls := supportedURLs.GetURLs()
				if len(urls) != len(tt.expectedURLs) {
					t.Errorf("Expected %d URLs in list, got %d", len(tt.expectedURLs), len(urls))
				}

				// Check each expected URL exists
				for _, expectedURL := range tt.expectedURLs {
					if !supportedURLs.HasURL(expectedURL) {
						t.Errorf("Expected URL '%s' not found in supported URLs", expectedURL)
					}
				}
			}
		})
	}
}

func TestClient_GetSupportedURLs_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(&Config{
		Host: host,
		Port: port,
	})

	// Test GetSupportedURLs with server error
	supportedURLs, err := client.GetSupportedURLs()

	// Should return error
	if err == nil {
		t.Error("Expected error for server error response, but got none")
	}

	if supportedURLs != nil {
		t.Error("Expected nil supportedURLs on error, but got result")
	}
}

func TestClient_GetSupportedURLs_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(&Config{
		Host: host,
		Port: port,
	})

	// Test GetSupportedURLs with 404 response
	supportedURLs, err := client.GetSupportedURLs()

	// Should return error
	if err == nil {
		t.Error("Expected error for 404 response, but got none")
	}

	if supportedURLs != nil {
		t.Error("Expected nil supportedURLs on error, but got result")
	}
}

func TestClient_GetSupportedURLs_InvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid xml content"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(&Config{
		Host: host,
		Port: port,
	})

	// Test GetSupportedURLs with invalid XML
	supportedURLs, err := client.GetSupportedURLs()

	// Should return error
	if err == nil {
		t.Error("Expected error for invalid XML, but got none")
	}

	if supportedURLs != nil {
		t.Error("Expected nil supportedURLs on error, but got result")
	}
}

func TestSupportedURLsResponse_Methods(t *testing.T) {
	// Create test data
	supportedURLs := &models.SupportedURLsResponse{
		DeviceID: "TEST123",
		URLs: []models.URL{
			{Location: "/info"},
			{Location: "/capabilities"},
			{Location: "/volume"},
			{Location: "/bass"},
			{Location: "/balance"},
			{Location: "/presets"},
			{Location: "/nowPlaying"},
			{Location: "/key"},
			{Location: "/sources"},
			{Location: "/navigate"},
			{Location: "/search"},
			{Location: "/audiodspcontrols"},
			{Location: "/setZone"},
			{Location: "/networkInfo"},
		},
	}

	t.Run("GetURLs", func(t *testing.T) {
		urls := supportedURLs.GetURLs()
		if len(urls) != 14 {
			t.Errorf("Expected 14 URLs, got %d", len(urls))
		}

		if urls[0] != "/info" {
			t.Errorf("Expected first URL to be '/info', got '%s'", urls[0])
		}
	})

	t.Run("HasURL", func(t *testing.T) {
		if !supportedURLs.HasURL("/info") {
			t.Error("Expected '/info' to be found")
		}

		if !supportedURLs.HasURL("/capabilities") {
			t.Error("Expected '/capabilities' to be found")
		}

		if supportedURLs.HasURL("/nonexistent") {
			t.Error("Expected '/nonexistent' not to be found")
		}
	})

	t.Run("GetURLCount", func(t *testing.T) {
		count := supportedURLs.GetURLCount()
		if count != 14 {
			t.Errorf("Expected URL count to be 14, got %d", count)
		}
	})

	t.Run("GetCoreURLs", func(t *testing.T) {
		coreURLs := supportedURLs.GetCoreURLs()

		expectedCore := []string{"/info", "/capabilities", "/sources", "/volume", "/bass", "/balance", "/presets", "/nowPlaying", "/key"}
		if len(coreURLs) != len(expectedCore) {
			t.Errorf("Expected %d core URLs, got %d", len(expectedCore), len(coreURLs))
		}

		for _, url := range expectedCore {
			found := false

			for _, core := range coreURLs {
				if core == url {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Expected core URL '%s' not found", url)
			}
		}
	})

	t.Run("GetStreamingURLs", func(t *testing.T) {
		streamingURLs := supportedURLs.GetStreamingURLs()

		expectedStreaming := []string{"/navigate", "/search", "/sources"}
		if len(streamingURLs) != len(expectedStreaming) {
			t.Errorf("Expected %d streaming URLs, got %d", len(expectedStreaming), len(streamingURLs))
		}
	})

	t.Run("GetAdvancedURLs", func(t *testing.T) {
		advancedURLs := supportedURLs.GetAdvancedURLs()

		expectedAdvanced := []string{"/audiodspcontrols", "/setZone"}
		if len(advancedURLs) != len(expectedAdvanced) {
			t.Errorf("Expected %d advanced URLs, got %d", len(expectedAdvanced), len(advancedURLs))
		}
	})

	t.Run("GetNetworkURLs", func(t *testing.T) {
		networkURLs := supportedURLs.GetNetworkURLs()

		expectedNetwork := []string{"/networkInfo"}
		if len(networkURLs) != len(expectedNetwork) {
			t.Errorf("Expected %d network URLs, got %d", len(expectedNetwork), len(networkURLs))
		}
	})

	t.Run("HasCorePlaybackSupport", func(t *testing.T) {
		if !supportedURLs.HasCorePlaybackSupport() {
			t.Error("Expected device to have core playback support")
		}
	})

	t.Run("HasPresetSupport", func(t *testing.T) {
		if !supportedURLs.HasPresetSupport() {
			t.Error("Expected device to have preset support")
		}
	})

	t.Run("HasMultiroomSupport", func(t *testing.T) {
		if !supportedURLs.HasMultiroomSupport() {
			t.Error("Expected device to have multiroom support")
		}
	})

	t.Run("HasAdvancedAudioSupport", func(t *testing.T) {
		if !supportedURLs.HasAdvancedAudioSupport() {
			t.Error("Expected device to have advanced audio support")
		}
	})

	t.Run("HasStreamingSupport", func(t *testing.T) {
		if !supportedURLs.HasStreamingSupport() {
			t.Error("Expected device to have streaming support")
		}
	})

	t.Run("GetUnsupportedURLs", func(t *testing.T) {
		checkList := []string{"/info", "/nonexistent1", "/capabilities", "/nonexistent2"}
		unsupported := supportedURLs.GetUnsupportedURLs(checkList)

		expectedUnsupported := []string{"/nonexistent1", "/nonexistent2"}
		if len(unsupported) != len(expectedUnsupported) {
			t.Errorf("Expected %d unsupported URLs, got %d", len(expectedUnsupported), len(unsupported))
		}

		for _, url := range expectedUnsupported {
			found := false

			for _, unsup := range unsupported {
				if unsup == url {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Expected unsupported URL '%s' not found", url)
			}
		}
	})
}

func TestSupportedURLsResponse_EmptyURLs(t *testing.T) {
	// Test with empty URL list
	supportedURLs := &models.SupportedURLsResponse{
		DeviceID: "EMPTY",
		URLs:     []models.URL{},
	}

	t.Run("empty_urls_basic_checks", func(t *testing.T) {
		if supportedURLs.GetURLCount() != 0 {
			t.Errorf("Expected 0 URLs, got %d", supportedURLs.GetURLCount())
		}

		if supportedURLs.HasURL("/info") {
			t.Error("Expected '/info' not to be found in empty list")
		}

		if supportedURLs.HasCorePlaybackSupport() {
			t.Error("Expected no core playback support with empty URLs")
		}

		if supportedURLs.HasPresetSupport() {
			t.Error("Expected no preset support with empty URLs")
		}
	})
}

func TestSupportedURLsResponse_FeatureMapping(t *testing.T) {
	// Create test data with comprehensive feature set
	supportedURLs := &models.SupportedURLsResponse{
		DeviceID: "FEATURE_TEST",
		URLs: []models.URL{
			// Core features
			{Location: "/info"},
			{Location: "/capabilities"},
			{Location: "/name"},
			{Location: "/supportedURLs"},
			// Audio features
			{Location: "/volume"},
			{Location: "/bass"},
			{Location: "/bassCapabilities"},
			{Location: "/balance"},
			{Location: "/audiodspcontrols"},
			// Playback features
			{Location: "/nowPlaying"},
			{Location: "/key"},
			{Location: "/trackInfo"},
			// Source features
			{Location: "/sources"},
			{Location: "/select"},
			{Location: "/serviceAvailability"},
			// Content features
			{Location: "/navigate"},
			{Location: "/search"},
			{Location: "/addStation"},
			{Location: "/removeStation"},
			// Preset features
			{Location: "/presets"},
			// Multiroom features
			{Location: "/setZone"},
			{Location: "/getZone"},
			{Location: "/addZoneSlave"},
			// Network features
			{Location: "/networkInfo"},
			{Location: "/bluetoothInfo"},
			// System features
			{Location: "/clock"},
			{Location: "/powerManagement"},
		},
	}

	t.Run("GetSupportedFeatures", func(t *testing.T) {
		features := supportedURLs.GetSupportedFeatures()
		if len(features) == 0 {
			t.Error("Expected supported features, got none")
		}

		// Check for some expected features
		featureNames := make(map[string]bool)
		for _, feature := range features {
			featureNames[feature.Name] = true
		}

		expectedFeatures := []string{
			"Device Information",
			"Volume Control",
			"Bass Control",
			"Playback Control",
			"Audio Sources",
			"Content Navigation",
			"Station Management",
			"Preset Management",
			"Multiroom Zones",
		}

		for _, expected := range expectedFeatures {
			if !featureNames[expected] {
				t.Errorf("Expected feature '%s' not found in supported features", expected)
			}
		}
	})

	t.Run("GetUnsupportedFeatures", func(t *testing.T) {
		unsupported := supportedURLs.GetUnsupportedFeatures()
		// With our comprehensive test data, there should be few unsupported features
		if len(unsupported) > 5 {
			t.Errorf("Expected few unsupported features, got %d", len(unsupported))
		}
	})

	t.Run("GetFeaturesByCategory", func(t *testing.T) {
		featuresByCategory := supportedURLs.GetFeaturesByCategory()

		expectedCategories := []string{"Core", "Audio", "Playback", "Sources", "Content", "Presets", "Multiroom", "Network", "System"}
		for _, category := range expectedCategories {
			if features, exists := featuresByCategory[category]; !exists || len(features) == 0 {
				t.Errorf("Expected category '%s' to have features", category)
			}
		}
	})

	t.Run("GetFeatureCompleteness", func(t *testing.T) {
		completeness, supported, total := supportedURLs.GetFeatureCompleteness()

		if completeness < 0 || completeness > 100 {
			t.Errorf("Completeness should be 0-100, got %d", completeness)
		}

		if supported <= 0 {
			t.Errorf("Expected some supported features, got %d", supported)
		}

		if total <= 0 {
			t.Errorf("Expected some total features, got %d", total)
		}

		if supported > total {
			t.Errorf("Supported features (%d) cannot exceed total (%d)", supported, total)
		}

		// With our comprehensive test data, should have high completeness
		if completeness < 70 {
			t.Errorf("Expected high completeness with comprehensive data, got %d%%", completeness)
		}
	})

	t.Run("GetMissingEssentialFeatures", func(t *testing.T) {
		missing := supportedURLs.GetMissingEssentialFeatures()

		// With our comprehensive test data, should have no missing essential features
		if len(missing) > 0 {
			t.Errorf("Expected no missing essential features with comprehensive data, got %d", len(missing))

			for _, feature := range missing {
				t.Errorf("Missing essential feature: %s", feature.Name)
			}
		}
	})

	t.Run("GetPartiallyImplementedFeatures", func(t *testing.T) {
		partial := supportedURLs.GetPartiallyImplementedFeatures()

		// The result depends on our test data - some features might be partial
		// This mainly tests that the function doesn't crash
		for _, feature := range partial {
			if len(feature.Endpoints) <= 1 {
				t.Errorf("Partial feature '%s' should have multiple endpoints, got %d", feature.Name, len(feature.Endpoints))
			}
		}
	})
}

func TestSupportedURLsResponse_FeatureMappingLimitedDevice(t *testing.T) {
	// Create test data for a limited device
	limitedURLs := &models.SupportedURLsResponse{
		DeviceID: "LIMITED_TEST",
		URLs: []models.URL{
			{Location: "/info"},
			{Location: "/volume"},
			{Location: "/nowPlaying"},
			{Location: "/key"},
		},
	}

	t.Run("LimitedDevice_GetMissingEssentialFeatures", func(t *testing.T) {
		missing := limitedURLs.GetMissingEssentialFeatures()

		// Should have some missing essential features
		if len(missing) == 0 {
			t.Error("Expected some missing essential features for limited device")
		}
	})

	t.Run("LimitedDevice_GetFeatureCompleteness", func(t *testing.T) {
		completeness, supported, total := limitedURLs.GetFeatureCompleteness()

		// Should have lower completeness
		if completeness > 50 {
			t.Errorf("Expected low completeness for limited device, got %d%%", completeness)
		}

		if supported == total {
			t.Error("Limited device should not support all features")
		}
	})
}

func TestEndpointFeatureMap(t *testing.T) {
	features := models.GetEndpointFeatureMap()

	t.Run("FeatureMapStructure", func(t *testing.T) {
		if len(features) == 0 {
			t.Error("Expected feature map to contain features")
		}

		for _, feature := range features {
			if feature.Name == "" {
				t.Error("Feature should have a name")
			}

			if feature.Description == "" {
				t.Error("Feature should have a description")
			}

			if len(feature.Endpoints) == 0 {
				t.Errorf("Feature '%s' should have at least one endpoint", feature.Name)
			}

			if feature.Category == "" {
				t.Errorf("Feature '%s' should have a category", feature.Name)
			}

			if feature.CLICommand == "" {
				t.Errorf("Feature '%s' should have CLI command info", feature.Name)
			}
		}
	})

	t.Run("FeatureCategories", func(t *testing.T) {
		categories := make(map[string]bool)
		for _, feature := range features {
			categories[feature.Category] = true
		}

		expectedCategories := []string{"Core", "Audio", "Playback", "Sources", "Content", "Presets", "Multiroom", "Network", "System"}
		for _, expected := range expectedCategories {
			if !categories[expected] {
				t.Errorf("Expected category '%s' not found in feature map", expected)
			}
		}
	})

	t.Run("EssentialFeatures", func(t *testing.T) {
		essentialCount := 0

		for _, feature := range features {
			if feature.Essential {
				essentialCount++
			}
		}

		if essentialCount == 0 {
			t.Error("Expected some features to be marked as essential")
		}

		// Should have a reasonable number of essential features
		if essentialCount > len(features)/2 {
			t.Errorf("Too many features marked as essential: %d/%d", essentialCount, len(features))
		}
	})
}
