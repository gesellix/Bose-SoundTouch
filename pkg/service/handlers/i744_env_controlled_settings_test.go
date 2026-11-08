package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestHandleGetSettingsListsEnvControlledFields_GH744(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	_, server := setupRouter("http://127.0.0.1:8000", ds)
	server.SetEnvControlledSettings(map[string]string{"spotify_redirect_uri": "SPOTIFY_REDIRECT_URI"})

	rec := httptest.NewRecorder()
	server.HandleGetSettings(rec, httptest.NewRequest(http.MethodGet, "/setup/settings", nil))

	var body struct {
		EnvControlled map[string]string `json:"env_controlled"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.EnvControlled["spotify_redirect_uri"] != "SPOTIFY_REDIRECT_URI" {
		t.Errorf("env_controlled = %v", body.EnvControlled)
	}
}
