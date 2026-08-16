package ssh

import (
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	host := "192.0.2.10"

	client := NewClient(host)
	if client.Host != host {
		t.Errorf("Expected host %s, got %s", host, client.Host)
	}

	if client.User != "root" {
		t.Errorf("Expected user root, got %s", client.User)
	}
}

func TestGetConfig(t *testing.T) {
	client := NewClient("localhost")

	config := client.getConfig()
	if config.User != "root" {
		t.Errorf("Expected config user root, got %s", config.User)
	}

	if len(config.Auth) == 0 {
		t.Error("Expected at least one auth method")
	}
}

func TestRun_DialFailure(t *testing.T) {
	// Use an invalid port/host to trigger dial failure
	client := NewClient("127.0.0.1:0")

	_, err := client.Run("ls")
	if err == nil {
		t.Error("Expected dial failure, got nil")
	}

	if !strings.Contains(err.Error(), "failed to dial") {
		t.Errorf("Expected 'failed to dial' error, got: %v", err)
	}
}

func TestClose_NoOpWithoutConnect(t *testing.T) {
	client := NewClient("127.0.0.1")

	if err := client.Close(); err != nil {
		t.Errorf("Close on a never-connected client should be a no-op, got: %v", err)
	}
}

func TestConnect_DialFailureLeavesConnNil(t *testing.T) {
	client := NewClient("127.0.0.1:0")

	err := client.Connect()
	if err == nil {
		t.Fatal("Expected dial failure, got nil")
	}

	if !strings.Contains(err.Error(), "failed to dial") {
		t.Errorf("Expected 'failed to dial' error, got: %v", err)
	}

	if client.conn != nil {
		t.Error("Connect should leave conn nil after a dial failure, so Run/UploadContent still fall back to their own one-off dial")
	}

	// Close after a failed Connect should still be a harmless no-op.
	if err := client.Close(); err != nil {
		t.Errorf("Close after a failed Connect should be a no-op, got: %v", err)
	}
}
