package fakespeaker

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestFakeSpeakerServesFixtures(t *testing.T) {
	s, err := Start(Config{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = s.Stop(ctx)
	})

	cases := []struct {
		path string
		root string
	}{
		{"/info", "info"},
		{"/presets", "presets"},
		{"/recents", "recents"},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := http.Get("http://" + s.HTTPAddr() + tc.path) //nolint:noctx
			if err != nil {
				t.Fatalf("get %s: %v", tc.path, err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			var root struct {
				XMLName xml.Name
			}

			if err := xml.Unmarshal(body, &root); err != nil {
				t.Fatalf("parse XML: %v\n%s", err, body)
			}

			if root.XMLName.Local != tc.root {
				t.Fatalf("root element = %q, want %q", root.XMLName.Local, tc.root)
			}
		})
	}
}
