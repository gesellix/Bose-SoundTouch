package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/favorites"
	"github.com/go-chi/chi/v5"
)

func favoriteRequest(t *testing.T, app *WebApp, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	app.MountWeb(router, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(method, "/api/control"+path, strings.NewReader(body)))
	return response
}

func TestFavoritesAPIIsPersistentAndIndependentOfSpeakerAndCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	app := NewWebApp()
	app.Favorites = favorites.NewFileStore(path)
	body := `{"Source":"TUNEIN","Location":"s123","ItemName":"Station","SourceAccount":"TUNEIN"}`
	response := favoriteRequest(t, app, http.MethodPost, "/favorites", body)
	if response.Code != http.StatusOK {
		t.Fatalf("add: %d %s", response.Code, response.Body.String())
	}
	var added struct {
		Data favorites.Entry `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	if len(added.Data.ID) != 64 || added.Data.ContentItem.ItemName != "Station" {
		t.Fatalf("bad entry: %+v", added.Data)
	}
	// Another browser retries with altered display metadata after losing the reply.
	response = favoriteRequest(t, app, http.MethodPost, "/favorites", strings.ReplaceAll(body, "Station", "Other name"))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "Other name") {
		t.Fatalf("duplicate overwrote metadata: %s", response.Body.String())
	}
	restarted := NewWebApp()
	restarted.Favorites = favorites.NewFileStore(path)
	response = favoriteRequest(t, restarted, http.MethodGet, "/favorites", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Station") {
		t.Fatalf("restart read: %d %s", response.Code, response.Body.String())
	}
	if app.DeviceCount() != 0 || app.CatalogEntries != nil {
		t.Fatal("favorites required a device or catalog")
	}
	for range 2 {
		response = favoriteRequest(t, restarted, http.MethodDelete, "/favorites/"+added.Data.ID, "")
		if response.Code != http.StatusOK {
			t.Fatalf("delete retry: %d %s", response.Code, response.Body.String())
		}
	}
	entries, err := restarted.Favorites.List()
	if err != nil || len(entries) != 0 {
		t.Fatalf("not removed: %+v %v", entries, err)
	}
}

func TestFavoritesAPIRejectsMalformedInputWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	app := NewWebApp()
	app.Favorites = favorites.NewFileStore(path)
	for _, body := range []string{
		`null`, `{}`, `{"Source":"TUNEIN","Location":"s1","path":"/tmp/elsewhere"}`,
		`{"Source":"TUNEIN","Location":"s1"} {}`, `{"Source":"STANDBY","Location":"x"}`,
		`{"Source":"TUNEIN","Location":"` + strings.Repeat("x", 65536) + `"}`,
	} {
		response := favoriteRequest(t, app, http.MethodPost, "/favorites", body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("expected rejection, got%d %s", response.Code, response.Body.String())
		}
	}
	response := favoriteRequest(t, app, http.MethodDelete, "/favorites/not-an-id", "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("bad ID: %d", response.Code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid requests wrote state: %v", err)
	}
}

func TestFavoritesAPIUnavailableAndCorruptionAreNotEmptyLibrary(t *testing.T) {
	app := NewWebApp()
	response := favoriteRequest(t, app, http.MethodGet, "/favorites", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":false`) {
		t.Fatalf("unwired: %s", response.Body.String())
	}
	response = favoriteRequest(t, app, http.MethodPost, "/favorites", `{"Source":"TUNEIN","Location":"s1"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired mutation: %d", response.Code)
	}
	path := filepath.Join(t.TempDir(), "favorites.json")
	original := []byte(`{"version":999,"entries":[]}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	app.Favorites = favorites.NewFileStore(path)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response = favoriteRequest(t, app, method, "/favorites", `{"Source":"TUNEIN","Location":"s1"}`)
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"success":false`) {
			t.Fatalf("corrupt state reported as valid: %d %s", response.Code, response.Body.String())
		}
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || string(unchanged) != string(original) {
		t.Fatalf("corrupt file replaced: %s %v", unchanged, err)
	}
}
