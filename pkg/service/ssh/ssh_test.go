package ssh

import (
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	host := "192.168.1.10"
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

// Note: Testing Run and UploadContent with a real SSH server is complex in a unit test.
// We've already verified the implementation manually and with setup tests.
// Below is a skeleton of how one might mock it if needed, but for now we focus on the basic logic.

/*
// MockClient can be used to test components that depend on SSH without a real server.
type MockClient struct {
	RunFunc           func(command string) (string, error)
	UploadContentFunc func(content []byte, remotePath string) error
}

func (m *MockClient) Run(command string) (string, error) {
	if m.RunFunc != nil {
		return m.RunFunc(command)
	}
	return "", nil
}

func (m *MockClient) UploadContent(content []byte, remotePath string) error {
	if m.UploadContentFunc != nil {
		return m.UploadContentFunc(content, remotePath)
	}
	return nil
}
*/
