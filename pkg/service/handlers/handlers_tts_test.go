package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/tts"
	"github.com/go-chi/chi/v5"
)

// ttsTestRouter wires just the TTS routes against a fresh server.
func ttsTestRouter(t *testing.T, baseURL string) (*chi.Mux, *Server) {
	t.Helper()

	ds := datastore.NewDataStore(t.TempDir())
	server := NewServer(ds, nil, baseURL, false, false, false)

	r := chi.NewRouter()
	r.Get("/media/tts/{id}", server.HandleTTSMedia)
	r.Post("/mgmt/tts/speak", server.HandleMgmtTTSSpeak)
	r.Get("/mgmt/tts/config", server.HandleMgmtTTSConfig)

	return r, server
}

// mockCloudTTS returns an httptest server that emits base64-encoded audio.
func mockCloudTTS(t *testing.T, audio string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"audioContent": base64.StdEncoding.EncodeToString([]byte(audio)),
		})
	}))
}

func TestHandleMgmtTTSConfigNotConfigured(t *testing.T) {
	r, _ := ttsTestRouter(t, "http://localhost:8001")

	req := httptest.NewRequest(http.MethodGet, "/mgmt/tts/config", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body["configured"] != false {
		t.Fatalf("configured = %v, want false", body["configured"])
	}
}

func TestHandleMgmtTTSConfigConfigured(t *testing.T) {
	r, server := ttsTestRouter(t, "http://localhost:8001")
	server.SetTTSService(tts.NewService(tts.NewTranslateProvider(), tts.Config{AppKey: "k", DefaultLanguage: "EN"}))

	req := httptest.NewRequest(http.MethodGet, "/mgmt/tts/config", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var body map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)

	if body["configured"] != true {
		t.Fatalf("configured = %v, want true", body["configured"])
	}

	if body["provider"] != tts.ProviderTranslate {
		t.Fatalf("provider = %v, want %s", body["provider"], tts.ProviderTranslate)
	}

	if body["appKeyConfigured"] != true {
		t.Fatalf("appKeyConfigured = %v, want true", body["appKeyConfigured"])
	}
}

func TestHandleTTSMediaServesCachedClip(t *testing.T) {
	const audio = "synth-bytes"

	mock := mockCloudTTS(t, audio)
	defer mock.Close()

	r, server := ttsTestRouter(t, "http://localhost:8001")

	provider := tts.NewCloudProvider("k")
	provider.SetEndpoint(mock.URL)
	svc := tts.NewService(provider, tts.Config{BaseURL: "http://localhost:8001", AppKey: "k"})
	server.SetTTSService(svc)

	// Populate the cache and learn the media id.
	playURL, err := svc.Prepare(context.Background(), tts.Request{Text: "hello", Language: "en-US"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	id := strings.TrimPrefix(playURL, "http://localhost:8001/media/tts/")
	if id == playURL {
		t.Fatalf("unexpected play URL: %s", playURL)
	}

	req := httptest.NewRequest(http.MethodGet, "/media/tts/"+id, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if rec.Body.String() != audio {
		t.Fatalf("body = %q, want %q", rec.Body.String(), audio)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Fatalf("content-type = %q, want audio/mpeg", ct)
	}
}

func TestHandleTTSMediaMissingClip(t *testing.T) {
	r, server := ttsTestRouter(t, "http://localhost:8001")
	server.SetTTSService(tts.NewService(tts.NewTranslateProvider(), tts.Config{}))

	req := httptest.NewRequest(http.MethodGet, "/media/tts/does-not-exist.mp3", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleMgmtTTSSpeakNotConfigured(t *testing.T) {
	r, _ := ttsTestRouter(t, "http://localhost:8001")

	req := httptest.NewRequest(http.MethodPost, "/mgmt/tts/speak", strings.NewReader(`{"host":"192.0.2.10","text":"hi"}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestHandleMgmtTTSSpeakValidation(t *testing.T) {
	r, server := ttsTestRouter(t, "http://localhost:8001")
	server.SetTTSService(tts.NewService(tts.NewTranslateProvider(), tts.Config{AppKey: "k"}))

	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty text", `{"host":"192.0.2.10","text":"  "}`, http.StatusBadRequest},
		{"no target", `{"text":"hello"}`, http.StatusBadRequest},
		{"bad json", `{not json}`, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mgmt/tts/speak", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
