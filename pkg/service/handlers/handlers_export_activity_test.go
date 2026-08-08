package handlers

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// tarEntries reads every file name + content out of a tar written by
// addActivityLog, for assertions.
func tarEntries(t *testing.T, tw *tar.Writer, buf *bytes.Buffer) map[string]string {
	t.Helper()

	if err := tw.Close(); err != nil {
		t.Fatalf("Failed to close tar writer: %v", err)
	}

	entries := make(map[string]string)
	tr := tar.NewReader(buf)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read tar entry: %v", err)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("Failed to read tar entry content: %v", err)
		}

		entries[hdr.Name] = string(data)
	}

	return entries
}

// TestAddActivityLog_EmptyByDefault verifies a fresh install (nothing
// recorded via datastore.RecordActivity yet — the common case, since
// stats/activity/ won't exist at all) doesn't error and adds nothing.
func TestAddActivityLog_EmptyByDefault(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "export-activity-empty-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	server := NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	server.addActivityLog(tw)

	entries := tarEntries(t, tw, &buf)
	if len(entries) != 0 {
		t.Errorf("Expected no tar entries for an empty activity log, got %+v", entries)
	}
}

// TestAddActivityLog_IncludesRecordedDismissal is the regression test for
// the #419 design's stated privacy guarantee: a dismissal recorded locally
// must actually show up in the diagnostic export, not just in theory. This
// closes the loop DIAGNOSTIC-EXPORT.md documents.
func TestAddActivityLog_IncludesRecordedDismissal(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "export-activity-dismissal-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	server := NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)

	if err := server.RecordDismissal("admin-area-auth-419"); err != nil {
		t.Fatalf("RecordDismissal failed: %v", err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	server.addActivityLog(tw)

	entries := tarEntries(t, tw, &buf)
	if len(entries) != 1 {
		t.Fatalf("Expected exactly 1 tar entry, got %+v", entries)
	}

	var (
		name    string
		content string
	)
	for n, c := range entries {
		name, content = n, c
	}

	if !strings.HasPrefix(name, "stats/activity/notification_dismissed/") {
		t.Errorf("Expected entry under stats/activity/notification_dismissed/, got %q", name)
	}
	if !strings.Contains(content, "admin-area-auth-419") {
		t.Errorf("Expected entry content to reference the dismissed id, got %q", content)
	}
}
