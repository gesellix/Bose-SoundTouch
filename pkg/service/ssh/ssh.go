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

// NewClient creates a new SSH client for the given host.
func NewClient(host string) *Client {
	return &Client{
		Host: host,
		User: "root",
	}
}

// getConfig returns the SSH client configuration.
func (c *Client) getConfig() *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: c.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(""), // Default password for SoundTouch root is often empty or not used with these settings
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
			ssh.KeyAlgoDSA,
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
		return "", fmt.Errorf("failed to dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	return string(output), err
}

// UploadContent uploads the given content to a file on the remote host.
// It uses a simple approach: echoing the content into a file.
// For larger files, a proper SCP or SFTP implementation would be better.
func (c *Client) UploadContent(content []byte, remotePath string) error {
	config := c.getConfig()
	client, err := ssh.Dial("tcp", c.Host+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	// Use a pipe to write content to the remote command's stdin
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %v", err)
	}

	// Capture stderr to get better error messages
	stderr, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %v", err)
	}

	// Read content from stdin and write to the remote file
	cmd := fmt.Sprintf("cat > %s", remotePath)

	// Start the command
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("failed to start upload command: %v", err)
	}

	// Write content and close stdin
	_, err = stdin.Write(content)
	stdin.Close()
	if err != nil {
		return fmt.Errorf("failed to write content to stdin: %v", err)
	}

	// Read stderr in case of failure
	stderrBuf := new(strings.Builder)
	go io.Copy(stderrBuf, stderr)

	// Wait for the command to finish
	if err := session.Wait(); err != nil {
		return fmt.Errorf("failed to finish upload: %v (stderr: %s)", err, stderrBuf.String())
	}

	return nil
}
