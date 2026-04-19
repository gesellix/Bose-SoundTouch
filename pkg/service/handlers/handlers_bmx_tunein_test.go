package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleTuneInNavigate(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	t.Run("Root navigate", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/bmx/tunein/v1/navigate", nil)
		req.Header.Set("Authorization", "Bearer mock-token")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if _, ok := resp["bmx_sections"]; !ok {
			t.Error("Response missing 'bmx_sections'")
		}
	})

	t.Run("Sub navigate", func(t *testing.T) {
		// Use the top-level OPML URL as a valid encoded navigate target
		encodedURI := base64.URLEncoding.EncodeToString([]byte("http://opml.radiotime.com/?render=json"))
		req := httptest.NewRequest("GET", "/bmx/tunein/v1/navigate/"+encodedURI, nil)
		req.Header.Set("Authorization", "Bearer mock-token")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/bmx/tunein/v1/navigate", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})
}

func TestHandleTuneInSearch(t *testing.T) {
	r, _ := setupRouter("http://localhost:8001", nil)

	t.Run("Search music", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/bmx/tunein/v1/search?q=music", nil)
		req.Header.Set("Authorization", "Bearer mock-token")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if _, ok := resp["bmx_sections"]; !ok {
			t.Error("Response missing 'bmx_sections'")
		}
	})

	t.Run("Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/bmx/tunein/v1/search?q=music", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Code)
		}
	})
}
