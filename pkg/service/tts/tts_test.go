package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTranslateProviderReturnsDirectURL(t *testing.T) {
	p := NewTranslateProvider()

	res, err := p.Synthesize(context.Background(), Request{Text: "Hello world", Language: "EN"})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if res.DirectURL == "" {
		t.Fatal("expected a DirectURL")
	}

	if len(res.Audio) != 0 {
		t.Fatalf("translate provider should not return audio bytes, got %d", len(res.Audio))
	}

	if !strings.Contains(res.DirectURL, "translate_tts") || !strings.Contains(res.DirectURL, "tl=EN") {
		t.Fatalf("unexpected DirectURL: %s", res.DirectURL)
	}
}

func TestCloudProviderSynthesize(t *testing.T) {
	const want = "fake-mp3-bytes"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Errorf("expected api key in query, got %q", got)
		}

		body, _ := io.ReadAll(r.Body)

		var req cloudSynthesizeRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode request: %v", err)
		}

		if req.Input.Text != "Hi there" {
			t.Errorf("unexpected text: %q", req.Input.Text)
		}

		if req.AudioConfig.AudioEncoding != "MP3" {
			t.Errorf("unexpected encoding: %q", req.AudioConfig.AudioEncoding)
		}

		resp := cloudSynthesizeResponse{AudioContent: base64.StdEncoding.EncodeToString([]byte(want))}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewCloudProvider("test-key")
	p.SetEndpoint(srv.URL)

	res, err := p.Synthesize(context.Background(), Request{Text: "Hi there", Language: "en-US", Format: FormatMP3})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if string(res.Audio) != want {
		t.Fatalf("audio = %q, want %q", res.Audio, want)
	}

	if res.ContentType != "audio/mpeg" {
		t.Fatalf("content type = %q, want audio/mpeg", res.ContentType)
	}

	if res.DirectURL != "" {
		t.Fatalf("cloud provider should not set DirectURL, got %q", res.DirectURL)
	}
}

func TestCloudProviderHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"denied"}`))
	}))
	defer srv.Close()

	p := NewCloudProvider("test-key")
	p.SetEndpoint(srv.URL)

	if _, err := p.Synthesize(context.Background(), Request{Text: "x"}); err == nil {
		t.Fatal("expected error on non-200 response")
	}
}

func TestCloudProviderNoAPIKey(t *testing.T) {
	p := NewCloudProvider("")

	if _, err := p.Synthesize(context.Background(), Request{Text: "x"}); err == nil {
		t.Fatal("expected error when no API key configured")
	}
}

func TestClipCacheTTLEviction(t *testing.T) {
	c := newClipCache(time.Minute, 10)

	base := time.Now()
	c.now = func() time.Time { return base }

	c.put("a.mp3", []byte("audio"), "audio/mpeg")

	if _, _, ok := c.get("a.mp3"); !ok {
		t.Fatal("entry should be present immediately")
	}

	// Advance past the TTL.
	c.now = func() time.Time { return base.Add(2 * time.Minute) }

	if _, _, ok := c.get("a.mp3"); ok {
		t.Fatal("entry should have expired")
	}
}

func TestClipCacheCapacityEviction(t *testing.T) {
	c := newClipCache(time.Hour, 2)

	base := time.Now()
	// Each put advances the clock so "oldest" is well-defined.
	c.now = func() time.Time { return base }
	c.put("a.mp3", []byte("a"), "audio/mpeg")

	c.now = func() time.Time { return base.Add(time.Second) }
	c.put("b.mp3", []byte("b"), "audio/mpeg")

	c.now = func() time.Time { return base.Add(2 * time.Second) }
	c.put("c.mp3", []byte("c"), "audio/mpeg")

	if _, _, ok := c.get("a.mp3"); ok {
		t.Fatal("oldest entry 'a' should have been evicted")
	}

	if _, _, ok := c.get("b.mp3"); !ok {
		t.Fatal("entry 'b' should still be present")
	}

	if _, _, ok := c.get("c.mp3"); !ok {
		t.Fatal("entry 'c' should still be present")
	}
}

// stubProvider lets Service tests control the Result without real HTTP.
type stubProvider struct {
	name  string
	res   Result
	calls int
}

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) Synthesize(_ context.Context, _ Request) (Result, error) {
	p.calls++
	return p.res, nil
}

func TestServicePrepareDirectURL(t *testing.T) {
	p := &stubProvider{name: ProviderTranslate, res: Result{DirectURL: "https://example.invalid/say.mp3"}}
	svc := NewService(p, Config{})

	url, err := svc.Prepare(context.Background(), Request{Text: "hello"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if url != "https://example.invalid/say.mp3" {
		t.Fatalf("url = %q", url)
	}
}

func TestServicePrepareCachesAndReuses(t *testing.T) {
	p := &stubProvider{name: ProviderGoogleCloud, res: Result{Audio: []byte("bytes"), ContentType: "audio/mpeg"}}
	svc := NewService(p, Config{BaseURL: "https://soundtouch.local/"})

	url1, err := svc.Prepare(context.Background(), Request{Text: "hello", Language: "en-US"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	if !strings.HasPrefix(url1, "https://soundtouch.local/media/tts/") {
		t.Fatalf("unexpected media url: %s", url1)
	}

	// Identical request should hit the cache, not call the provider again.
	url2, err := svc.Prepare(context.Background(), Request{Text: "hello", Language: "en-US"})
	if err != nil {
		t.Fatalf("Prepare (cached): %v", err)
	}

	if url1 != url2 {
		t.Fatalf("expected stable url, got %q and %q", url1, url2)
	}

	if p.calls != 1 {
		t.Fatalf("provider called %d times, want 1 (second should be cached)", p.calls)
	}

	// The clip should be retrievable for the media handler.
	id := strings.TrimPrefix(url1, "https://soundtouch.local/media/tts/")

	audio, ct, ok := svc.Clip(id)
	if !ok {
		t.Fatal("clip should be cached")
	}

	if string(audio) != "bytes" || ct != "audio/mpeg" {
		t.Fatalf("clip = %q (%s)", audio, ct)
	}
}

func TestServicePrepareAudioWithoutBaseURL(t *testing.T) {
	p := &stubProvider{name: ProviderGoogleCloud, res: Result{Audio: []byte("bytes"), ContentType: "audio/mpeg"}}
	svc := NewService(p, Config{}) // no BaseURL

	if _, err := svc.Prepare(context.Background(), Request{Text: "hello"}); err == nil {
		t.Fatal("expected error when hosting audio without a base URL")
	}
}

func TestServicePrepareEmptyText(t *testing.T) {
	p := &stubProvider{name: ProviderTranslate, res: Result{DirectURL: "x"}}
	svc := NewService(p, Config{})

	if _, err := svc.Prepare(context.Background(), Request{Text: "   "}); err == nil {
		t.Fatal("expected error on empty text")
	}
}
