package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAVTransportControlURL(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "host only", host: "192.0.2.10", want: "http://192.0.2.10:8091/AVTransport/Control"},
		{name: "host with api port", host: "http://192.0.2.10:8090", want: "http://192.0.2.10:8091/AVTransport/Control"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClientFromHost(tt.host)

			got, err := c.avTransportControlURL()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("control URL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlayURLViaUPnPRejectsHTTPS(t *testing.T) {
	called := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClientFromHost("192.0.2.10")
	c.avTransportURLOverride = server.URL

	// The speaker rejects https:// URIs, so we should fail fast without even
	// contacting it, with a message that names the constraint.
	err := c.PlayURLViaUPnP("https://example.com/clip.mp3")
	if err == nil {
		t.Fatal("expected an error for an https:// URL")
	}

	if !strings.Contains(err.Error(), "http://") {
		t.Errorf("error should explain the http:// requirement, got: %v", err)
	}

	if called {
		t.Error("no SOAP request should be sent for an https:// URL")
	}
}

func TestPlayURLViaUPnP(t *testing.T) {
	type capture struct {
		path        string
		soapAction  string
		contentType string
		body        string
	}

	var calls []capture

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, capture{
			path:        r.URL.Path,
			soapAction:  r.Header.Get("SOAPAction"),
			contentType: r.Header.Get("Content-Type"),
			body:        string(b),
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClientFromHost("192.0.2.10")
	c.avTransportURLOverride = server.URL // route SOAP at the test server

	mediaURL := "http://192.0.2.99/tts/hello.mp3?a=1&b=2"
	if err := c.PlayURLViaUPnP(mediaURL); err != nil {
		t.Fatalf("PlayURLViaUPnP: %v", err)
	}

	// Two SOAP actions in order: SetAVTransportURI then Play.
	if len(calls) != 2 {
		t.Fatalf("expected 2 SOAP calls, got %d", len(calls))
	}

	set, play := calls[0], calls[1]

	if !strings.Contains(set.soapAction, "AVTransport:1#SetAVTransportURI") {
		t.Errorf("first SOAPAction = %q, want SetAVTransportURI", set.soapAction)
	}

	if !strings.Contains(play.soapAction, "AVTransport:1#Play") {
		t.Errorf("second SOAPAction = %q, want Play", play.soapAction)
	}

	if !strings.HasPrefix(set.contentType, "text/xml") {
		t.Errorf("Content-Type = %q, want text/xml", set.contentType)
	}

	// The media URL must be XML-escaped inside <CurrentURI> (the & becomes &amp;).
	if !strings.Contains(set.body, "http://192.0.2.99/tts/hello.mp3?a=1&amp;b=2") {
		t.Errorf("SetAVTransportURI body missing escaped media URL, got: %s", set.body)
	}

	if strings.Contains(set.body, "a=1&b=2") {
		t.Errorf("media URL was not XML-escaped in the body: %s", set.body)
	}
}
