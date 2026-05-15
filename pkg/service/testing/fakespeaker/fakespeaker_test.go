package fakespeaker

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
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
		{"/networkInfo", "networkInfo"},
		{"/sources", "sources"},
		{"/supportedURLs", "supportedURLs"},
		{"/getGroup", "group"},
		{"/removeGroup", "group"},
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

func TestFakeSpeakerAddGroupEchoesWithGroupOK(t *testing.T) {
	s, err := Start(Config{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = s.Stop(ctx)
	})

	posted := `<?xml version="1.0" encoding="UTF-8"?>
<group>
    <name>TEST</name>
    <masterDeviceId>DEADBEEFCAFE</masterDeviceId>
    <roles>
      <groupRole><deviceId>DEADBEEFCAFE</deviceId><role>LEFT</role><ipAddress>127.0.0.1</ipAddress></groupRole>
      <groupRole><deviceId>0000DEADBEEF</deviceId><role>RIGHT</role><ipAddress>127.0.0.2</ipAddress></groupRole>
    </roles>
</group>`

	resp, err := http.Post("http://"+s.HTTPAddr()+"/addGroup", "application/xml", strings.NewReader(posted)) //nolint:noctx
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// Echo: the posted name + roles survive in the response.
	if !bytes.Contains(body, []byte("<name>TEST</name>")) {
		t.Errorf("response missing posted <name>; body:\n%s", body)
	}

	if !bytes.Contains(body, []byte("<masterDeviceId>DEADBEEFCAFE</masterDeviceId>")) {
		t.Errorf("response missing posted <masterDeviceId>; body:\n%s", body)
	}

	// Success marker: <status>GROUP_OK</status> appears before </group>.
	statusIdx := bytes.Index(body, []byte("<status>GROUP_OK</status>"))
	if statusIdx < 0 {
		t.Fatalf("response missing <status>GROUP_OK</status>; body:\n%s", body)
	}

	closeIdx := bytes.LastIndex(body, []byte("</group>"))
	if closeIdx < 0 || statusIdx >= closeIdx {
		t.Errorf("<status> not nested inside <group>...</group>; body:\n%s", body)
	}
}

func TestFakeSpeakerUpdateGroupEchoesWithGroupOK(t *testing.T) {
	s, err := Start(Config{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = s.Stop(ctx)
	})

	posted := `<?xml version="1.0" encoding="UTF-8"?>
<group>
    <name>RENAMED</name>
    <masterDeviceId>DEADBEEFCAFE</masterDeviceId>
</group>`

	resp, err := http.Post("http://"+s.HTTPAddr()+"/updateGroup", "application/xml", strings.NewReader(posted)) //nolint:noctx
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if !bytes.Contains(body, []byte("<name>RENAMED</name>")) {
		t.Errorf("response missing posted <name>; body:\n%s", body)
	}

	if !bytes.Contains(body, []byte("<status>GROUP_OK</status>")) {
		t.Errorf("response missing <status>GROUP_OK</status>; body:\n%s", body)
	}
}

func TestFakeSpeakerRemoveGroupRejectsNonGET(t *testing.T) {
	s, err := Start(Config{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = s.Stop(ctx)
	})

	resp, err := http.Post("http://"+s.HTTPAddr()+"/removeGroup", "application/xml", strings.NewReader("")) //nolint:noctx
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}

	if got := resp.Header.Get("Allow"); got != "GET" {
		t.Errorf("Allow header = %q, want %q", got, "GET")
	}
}
