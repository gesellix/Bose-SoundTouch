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
