// Package ssh provides simple SSH operations for SoundTouch speakers.
package ssh

import (
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client wraps an SSH client to perform operations on SoundTouch speakers.
type Client struct {
	Host string
	User string
}

// NewClient creates a new SSH client for the given host. The default user is "root".
func NewClient(host string) *Client {
	return &Client{
		Host: host,
		User: "root",
	}
}

// getConfig returns the SSH client configuration with the legacy cipher/kex suites
// required by older SoundTouch device firmware.
func (c *Client) getConfig() *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(""),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
		Config: ssh.Config{
			KeyExchanges: []string{
				"diffie-hellman-group1-sha1",
				"diffie-hellman-group14-sha1",
				"ecdh-sha2-nistp256",
				"ecdh-sha2-nistp384",
				"ecdh-sha2-nistp521",
				"curve25519-sha256@libssh.org",
			},
			Ciphers: []string{
				"aes128-ctr",
				"aes192-ctr",
				"aes256-ctr",
				"aes128-cbc",
				"3des-cbc",
				"aes128-gcm@openssh.com",
				"arcfour256",
				"arcfour128",
			},
		},
		HostKeyAlgorithms: []string{
			ssh.KeyAlgoRSASHA256,
			ssh.KeyAlgoRSASHA512,
			ssh.KeyAlgoRSA,
			ssh.KeyAlgoECDSA256,
			ssh.KeyAlgoECDSA384,
			ssh.KeyAlgoECDSA521,
			ssh.KeyAlgoED25519,
		},
	}
}

// Run executes a command on the remote host and returns the combined stdout and stderr.
func (c *Client) Run(command string) (string, error) {
	config := c.getConfig()

	client, err := ssh.Dial("tcp", c.Host+":22", config)
	if err != nil {
		return "", fmt.Errorf("failed to dial: %w", err)
	}

	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	defer func() { _ = session.Close() }()

	output, err := session.CombinedOutput(command)

	return string(output), err
}

// ReadFile downloads the content of a file on the remote host.
// An empty file that causes cat to exit non-zero (a firmware quirk on some devices)
// is returned as empty bytes rather than an error.
func (c *Client) ReadFile(remotePath string) ([]byte, error) {
	output, err := c.Run(fmt.Sprintf("cat %s", remotePath))
	if err != nil && strings.TrimSpace(output) != "" {
		return nil, err
	}

	return []byte(output), nil
}

// ReadDir downloads all regular files under remotePath, returning a map of
// absolute remote path → file content. Missing or unreadable files are skipped.
func (c *Client) ReadDir(remotePath string) (map[string][]byte, error) {
	listing, err := c.Run(fmt.Sprintf("find %s -type f 2>/dev/null", remotePath))
	if err != nil || strings.TrimSpace(listing) == "" {
		return nil, fmt.Errorf("cannot list %s: %w", remotePath, err)
	}

	result := make(map[string][]byte)

	for _, path := range strings.Split(strings.TrimSpace(listing), "\n") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}

		data, readErr := c.ReadFile(path)
		if readErr != nil {
			continue
		}

		result[path] = data
	}

	return result, nil
}

// UploadContent uploads the given content to a file on the remote host using stdin piping.
func (c *Client) UploadContent(content []byte, remotePath string) error {
	config := c.getConfig()

	client, err := ssh.Dial("tcp", c.Host+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	defer func() { _ = session.Close() }()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if startErr := session.Start(fmt.Sprintf("cat > %s", remotePath)); startErr != nil {
		return fmt.Errorf("failed to start upload command: %w", startErr)
	}

	_, err = stdin.Write(content)
	_ = stdin.Close()

	if err != nil {
		return fmt.Errorf("failed to write content to stdin: %w", err)
	}

	stderrBuf := new(strings.Builder)

	go func() { _, _ = io.Copy(stderrBuf, stderr) }()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("failed to finish upload: %w (stderr: %s)", err, stderrBuf.String())
	}

	return nil
}
