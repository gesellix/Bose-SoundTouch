package bmx

import (
	"testing"
)

func TestPlayCustomStream(t *testing.T) {
	// Test Standard Base64
	dataStd := "eyJzdHJlYW1VcmwiOiJodHRwOi8vZXhhbXBsZS5jb20vc3RyZWFtLm1wMyIsImltYWdlVXJsIjoiaW1hZ2UucG5nIiwibmFtZSI6IlN0cmVhbSBOYW1lIn0="

	resp, err := PlayCustomStream(dataStd)
	if err != nil {
		t.Fatalf("PlayCustomStream with standard base64 failed: %v", err)
	}

	if resp.Name != "Stream Name" {
		t.Errorf("Expected name Stream Name, got %s", resp.Name)
	}

	// Test URL-safe Base64
	dataURL := "eyJzdHJlYW1VcmwiOiJodHRwOi8vZXhhbXBsZS5jb20vc3RyZWFtLm1wMyIsImltYWdlVXJsIjoiaW1hZ2UucG5nIiwibmFtZSI6IlN0cmVhbSBOYW1lIn0="

	resp, err = PlayCustomStream(dataURL)
	if err != nil {
		t.Fatalf("PlayCustomStream with URL-safe base64 failed: %v", err)
	}

	if resp.Name != "Stream Name" {
		t.Errorf("Expected name Stream Name, got %s", resp.Name)
	}
}
