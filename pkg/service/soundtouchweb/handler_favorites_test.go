package soundtouchweb

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/favorites"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
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

type failingFavoritesStore struct {
	err error
}

func (s failingFavoritesStore) List() ([]favorites.Entry, error) {
	return nil, s.err
}

func (s failingFavoritesStore) Add(models.ContentItem) (favorites.Entry, error) {
	return favorites.Entry{}, s.err
}

func (s failingFavoritesStore) Remove(string) error {
	return s.err
}

func TestFavoritesAPIStorageErrorsStayOnOneLogLine(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, logPrefix, message string
	}{
		{"read", http.MethodGet, "/favorites", "Favorites read failed: ", "Could not load favorites. Your saved library has not been replaced."},
		{"save", http.MethodPost, "/favorites", "Favorites save failed: ", "Could not save the favorite. Refresh the library before retrying."},
		{"remove", http.MethodDelete, "/favorites/" + strings.Repeat("a", 64), "Favorites removal failed: ", "Could not remove the favorite. Refresh the library before retrying."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := NewWebApp()
			app.Favorites = failingFavoritesStore{err: errors.New("storage diagnostic\r\nFORGED_RECORD")}
			var response *httptest.ResponseRecorder
			out := captureLog(t, func() {
				response = favoriteRequest(t, app, tc.method, tc.path, `{"Source":"TUNEIN","Location":"s1"}`)
			})
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500: %s", response.Code, response.Body.String())
			}
			var payload webtypes.APIResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Success || payload.Error != tc.message || payload.Data != nil {
				t.Fatalf("storage error did not return the generic failure: %s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), "FORGED_RECORD") || strings.Contains(response.Body.String(), "storage diagnostic") {
				t.Fatalf("storage diagnostics reflected in response: %s", response.Body.String())
			}
			if want := tc.logPrefix + `storage diagnostic\r\nFORGED_RECORD` + "\n"; out != want {
				t.Fatalf("log = %q, want one physical line preserving diagnostics %q", out, want)
			}
		})
	}
}

func TestFavoritesAPIFileStoreErrorLogging(t *testing.T) {
	for _, tc := range []struct {
		name, directory, body string
		status                int
		wantEscapedPath       bool
	}{
		{"configured_path", "blocked\r\nPATH_MARKER", `{"Source":"TUNEIN","Location":"s1"}`, http.StatusInternalServerError, true},
		{"request_fields", "blocked", `{"Source":"TUNEIN","Location":"s1\r\nREQUEST_MARKER","ItemName":"Name\r\nREQUEST_MARKER"}`, http.StatusInternalServerError, false},
		{"rejected_source", "blocked", `{"Source":"AUX\r\n","Location":"s1"}`, http.StatusBadRequest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantEscapedPath && runtime.GOOS == "windows" {
				t.Skip("Windows forbids CR/LF in path components; the fake-store log cases still run")
			}
			// A regular file used as the parent forces an error even when run as root.
			blocked := filepath.Join(t.TempDir(), tc.directory)
			if err := os.WriteFile(blocked, []byte("guard"), 0600); err != nil {
				t.Fatal(err)
			}
			app := NewWebApp()
			app.Favorites = favorites.NewFileStore(filepath.Join(blocked, "favorites.json"))
			var response *httptest.ResponseRecorder
			out := captureLog(t, func() {
				response = favoriteRequest(t, app, http.MethodPost, "/favorites", tc.body)
			})
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "PATH_MARKER") || strings.Contains(response.Body.String(), "REQUEST_MARKER") {
				t.Fatalf("storage diagnostics reflected in response: %s", response.Body.String())
			}
			if tc.status == http.StatusBadRequest {
				if out != "" {
					t.Fatalf("rejected input reached storage error logging: %q", out)
				}
				return
			}
			// Windows treats the blocked parent as a missing path, so Add can
			// reach directory creation before reporting the storage error.
			if !strings.HasPrefix(out, "Favorites save failed: open favorites: ") &&
				!strings.HasPrefix(out, "Favorites save failed: create favorites directory: ") {
				t.Fatalf("missing storage diagnostic: %q", out)
			}
			if !strings.HasSuffix(out, "\n") || strings.ContainsAny(strings.TrimSuffix(out, "\n"), "\r\n") {
				t.Fatalf("storage error escaped its physical log line: %q", out)
			}
			if strings.Contains(out, "REQUEST_MARKER") {
				t.Fatalf("FileStore reflected request content in its error: %q", out)
			}
			if tc.wantEscapedPath && !strings.Contains(out, `blocked\r\nPATH_MARKER`) {
				t.Fatalf("configured path diagnostic lost: %q", out)
			}
		})
	}
}
