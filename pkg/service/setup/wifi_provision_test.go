package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestPushWiFiCredentials_BuildsCanonicalRequest(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotCT     string
		gotBody   string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")

		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)

		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" ?><AddWirelessProfileResponse />`))
	}))
	defer srv.Close()

	apHost := strings.TrimPrefix(srv.URL, "http://")

	err := PushWiFiCredentials(context.Background(), PushWiFiCredentialsParams{
		APHost:   apHost,
		SSID:     "MyHomeNetwork",
		Password: "s3cret",
		// Security and HTTPClient default
	})
	if err != nil {
		t.Fatalf("PushWiFiCredentials: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}

	if gotPath != "/addWirelessProfile" {
		t.Errorf("path = %s, want /addWirelessProfile", gotPath)
	}

	if gotCT != "text/xml" {
		t.Errorf("content-type = %s, want text/xml", gotCT)
	}

	if !strings.Contains(gotBody, `ssid="MyHomeNetwork"`) {
		t.Errorf("body missing ssid: %s", gotBody)
	}

	if !strings.Contains(gotBody, `password="s3cret"`) {
		t.Errorf("body missing password: %s", gotBody)
	}

	if !strings.Contains(gotBody, `securityType="wpa_or_wpa2"`) {
		t.Errorf("body should default to wpa_or_wpa2 security, got: %s", gotBody)
	}
}

func TestPushWiFiCredentials_EscapesQuotesInCredentials(t *testing.T) {
	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := PushWiFiCredentials(context.Background(), PushWiFiCredentialsParams{
		APHost:   strings.TrimPrefix(srv.URL, "http://"),
		SSID:     `net "with quote`,
		Password: `pa<ss>`,
	})
	if err != nil {
		t.Fatalf("PushWiFiCredentials: %v", err)
	}

	// Quotes must be escaped so they don't break the attribute context.
	if strings.Contains(gotBody, `ssid="net "with quote"`) {
		t.Errorf("quotes in SSID must be escaped, got: %s", gotBody)
	}

	if !strings.Contains(gotBody, "&quot;") {
		t.Errorf("expected &quot; escape in body: %s", gotBody)
	}
}

func TestPushWiFiCredentials_RequiresSSID(t *testing.T) {
	err := PushWiFiCredentials(context.Background(), PushWiFiCredentialsParams{Password: "x"})
	if err == nil || !strings.Contains(err.Error(), "SSID") {
		t.Errorf("err = %v, want SSID-required error", err)
	}
}

func TestPushWiFiCredentials_SurfacesHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	err := PushWiFiCredentials(context.Background(), PushWiFiCredentialsParams{
		APHost: strings.TrimPrefix(srv.URL, "http://"),
		SSID:   "X",
	})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("err = %v, want to surface HTTP 403", err)
	}
}

func TestWaitForAP_ReturnsOnceInfoSucceeds(t *testing.T) {
	var calls atomic.Int32

	httpGet := func(_ string) (*http.Response, error) {
		c := calls.Add(1)
		if c < 3 {
			return nil, errors.New("no route to host")
		}

		body := `<info deviceID="AABBCCDDEEFF"><name>Bose SoundTouch DE4803</name><margeAccountUUID></margeAccountUUID></info>`

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{},
		}, nil
	}

	cfg := PollConfig{Interval: 5 * time.Millisecond, Timeout: 500 * time.Millisecond}

	info, err := WaitForAP(context.Background(), "", cfg, httpGet)
	if err != nil {
		t.Fatalf("WaitForAP: %v", err)
	}

	if info.DeviceID != "AABBCCDDEEFF" {
		t.Errorf("DeviceID = %q, want AABBCCDDEEFF", info.DeviceID)
	}

	if calls.Load() < 3 {
		t.Errorf("expected at least 3 polls, got %d", calls.Load())
	}
}

func TestWaitForAP_TimesOut(t *testing.T) {
	httpGet := func(_ string) (*http.Response, error) {
		return nil, errors.New("network unreachable")
	}

	cfg := PollConfig{Interval: 5 * time.Millisecond, Timeout: 30 * time.Millisecond}

	_, err := WaitForAP(context.Background(), "", cfg, httpGet)
	if err == nil || !strings.Contains(err.Error(), "did not respond") {
		t.Errorf("err = %v, want timeout error", err)
	}
}

func TestWaitForAP_RespectsContextCancellation(t *testing.T) {
	httpGet := func(_ string) (*http.Response, error) {
		return nil, errors.New("network unreachable")
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(15 * time.Millisecond)
		cancel()
	}()

	cfg := PollConfig{Interval: 5 * time.Millisecond, Timeout: 5 * time.Second}

	_, err := WaitForAP(ctx, "", cfg, httpGet)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %v, want context-cancellation error", err)
	}
}

// stubMDNS is a controllable MDNSDiscoverer.
type stubMDNS struct {
	results [][]*models.DiscoveredDevice
	call    atomic.Int32
}

func (s *stubMDNS) DiscoverDevices(_ context.Context) ([]*models.DiscoveredDevice, error) {
	i := s.call.Add(1) - 1
	if int(i) >= len(s.results) {
		return nil, fmt.Errorf("exhausted")
	}

	return s.results[i], nil
}

func TestWaitForOnline_MatchesSubstringInNameOrSerial(t *testing.T) {
	stub := &stubMDNS{
		results: [][]*models.DiscoveredDevice{
			nil, // first poll: nothing yet
			{
				{Name: "Other Bose Speaker", SerialNo: "AAAAAAAAAAAA", Host: "192.168.1.50"},
				{Name: "Bose SoundTouch DE4803", SerialNo: "506583DE4803", Host: "192.168.1.42"},
			},
		},
	}

	cfg := PollConfig{Interval: 5 * time.Millisecond, Timeout: 500 * time.Millisecond}

	d, err := WaitForOnline(context.Background(), "DE4803", cfg, stub)
	if err != nil {
		t.Fatalf("WaitForOnline: %v", err)
	}

	if d.Host != "192.168.1.42" {
		t.Errorf("Host = %q, want 192.168.1.42", d.Host)
	}
}

func TestWaitForOnline_EmptyMatcherReturnsFirst(t *testing.T) {
	stub := &stubMDNS{
		results: [][]*models.DiscoveredDevice{
			{
				{Name: "Bose SoundTouch DE4803", Host: "192.168.1.42"},
			},
		},
	}

	cfg := PollConfig{Interval: 5 * time.Millisecond, Timeout: 500 * time.Millisecond}

	d, err := WaitForOnline(context.Background(), "", cfg, stub)
	if err != nil {
		t.Fatalf("WaitForOnline: %v", err)
	}

	if d.Host != "192.168.1.42" {
		t.Errorf("Host = %q, want 192.168.1.42", d.Host)
	}
}

func TestWaitForOnline_TimesOutWhenNoMatch(t *testing.T) {
	stub := &stubMDNS{
		results: [][]*models.DiscoveredDevice{
			{{Name: "Wrong One", SerialNo: "X", Host: "192.168.1.99"}},
			{{Name: "Wrong One", SerialNo: "X", Host: "192.168.1.99"}},
			{{Name: "Wrong One", SerialNo: "X", Host: "192.168.1.99"}},
		},
	}

	cfg := PollConfig{Interval: 5 * time.Millisecond, Timeout: 25 * time.Millisecond}

	_, err := WaitForOnline(context.Background(), "DE4803", cfg, stub)
	if err == nil || !strings.Contains(err.Error(), "no speaker matching") {
		t.Errorf("err = %v, want no-match timeout error", err)
	}
}
