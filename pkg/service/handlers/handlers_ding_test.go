package handlers

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newDingTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	_, server := setupRouter("http://localhost:8001", nil)

	r := chi.NewRouter()
	r.Get("/media/aftertouch-ding.wav", server.HandleDing)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	return ts
}

func TestHandleDing_DefaultIsValidWAV(t *testing.T) {
	ts := newDingTestServer(t)

	res, err := http.Get(ts.URL + "/media/aftertouch-ding.wav")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	if ct := res.Header.Get("Content-Type"); ct != "audio/wav" {
		t.Errorf("expected audio/wav, got %q", ct)
	}

	body := readAll(t, res)
	if !bytes.HasPrefix(body, []byte("RIFF")) {
		t.Errorf("expected RIFF prefix")
	}

	if !bytes.Equal(body[8:12], []byte("WAVE")) {
		t.Errorf("expected WAVE format marker")
	}

	if sr := binary.LittleEndian.Uint32(body[24:28]); sr != 22050 {
		t.Errorf("expected default sample rate 22050, got %d", sr)
	}
}

func TestHandleDing_OverrideSampleRate(t *testing.T) {
	ts := newDingTestServer(t)

	res, err := http.Get(ts.URL + "/media/aftertouch-ding.wav?sample-rate=44100")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	body := readAll(t, res)
	if sr := binary.LittleEndian.Uint32(body[24:28]); sr != 44100 {
		t.Errorf("expected sample rate 44100, got %d", sr)
	}
}

func TestHandleDing_InvalidParamFallsBackToDefault(t *testing.T) {
	ts := newDingTestServer(t)

	res, err := http.Get(ts.URL + "/media/aftertouch-ding.wav?sample-rate=notanumber&pitch-high=-50")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		t.Errorf("expected 200 even with bad params, got %d", res.StatusCode)
	}

	body := readAll(t, res)
	if sr := binary.LittleEndian.Uint32(body[24:28]); sr != 22050 {
		t.Errorf("invalid sample-rate should fall back to default, got %d", sr)
	}
}

func TestHandleDing_OutOfRangeSampleRateFallsBackToDefault(t *testing.T) {
	// CodeQL flagged the int→uint32 truncation; reject values
	// outside the sane audio range at the parser, so neither
	// the WAV header nor the int→uint32 cast can be tricked.
	cases := []string{
		"1",          // below dingMinSampleRate
		"7999",       // just under the floor
		"500000",     // above dingMaxSampleRate
		"4294967300", // > uint32 — the truncation source
	}

	ts := newDingTestServer(t)

	for _, sr := range cases {
		t.Run(sr, func(t *testing.T) {
			res, err := http.Get(ts.URL + "/media/aftertouch-ding.wav?sample-rate=" + sr)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer res.Body.Close()

			body := readAll(t, res)
			if got := binary.LittleEndian.Uint32(body[24:28]); got != 22050 {
				t.Errorf("sample-rate=%s should clamp to default 22050, got %d", sr, got)
			}
		})
	}
}

func TestHandleDing_DefaultIsCached(t *testing.T) {
	// Reset the cache by simulating a fresh process.
	dingDefaultCache.once = sync.Once{}
	dingDefaultCache.data = nil

	ts := newDingTestServer(t)

	resA, err := http.Get(ts.URL + "/media/aftertouch-ding.wav")
	if err != nil {
		t.Fatalf("first GET: %v", err)
	}
	a := readAll(t, resA)

	resB, err := http.Get(ts.URL + "/media/aftertouch-ding.wav")
	if err != nil {
		t.Fatalf("second GET: %v", err)
	}
	b := readAll(t, resB)

	if !bytes.Equal(a, b) {
		t.Errorf("expected cached default to be byte-identical across requests")
	}
}

func readAll(t *testing.T, res *http.Response) []byte {
	t.Helper()

	defer res.Body.Close()

	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(res.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}

	return body.Bytes()
}
