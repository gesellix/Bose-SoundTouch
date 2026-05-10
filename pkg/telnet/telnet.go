// Package telnet provides a minimal line-oriented client for the SoundTouch
// device's diagnostic shell on TCP port 17000.
//
// The protocol observed in the wild is a plain TCP stream with no Telnet
// option negotiation (no IAC sequences), so the client uses the standard
// library's net package directly. All I/O is deadline-driven so a wedged
// device can never stall the caller indefinitely.
package telnet

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

// Default values for a fresh Client.
//
// The dial and read budgets were originally tighter (2s / 5s); both were
// relaxed after observing transient i/o-timeout failures on healthy
// speakers that reliably resolved on a second attempt. The diagnostic
// shell on FW 27.0.6 occasionally takes >2s to accept a fresh TCP
// connection — likely while the device is servicing other work — so a
// short dial budget produces flaky preflight results without indicating
// a real reachability problem.
const (
	DefaultPort         = 17000
	DefaultDialTimeout  = 4 * time.Second
	DefaultReadTimeout  = 7 * time.Second
	DefaultWriteTimeout = 3 * time.Second
	// idleWindow is how long we wait for further bytes after the first
	// byte of a response before treating the response as complete.
	idleWindow = 600 * time.Millisecond
)

// Client is a connected (or about-to-be-connected) session to a SoundTouch
// diagnostic shell. A Client is not safe for concurrent use; create one per
// device interaction.
type Client struct {
	Host         string
	Port         int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	conn net.Conn
}

// NewClient returns a Client targeting host:17000 with the default timeouts.
func NewClient(host string) *Client {
	return &Client{
		Host:         host,
		Port:         DefaultPort,
		DialTimeout:  DefaultDialTimeout,
		ReadTimeout:  DefaultReadTimeout,
		WriteTimeout: DefaultWriteTimeout,
	}
}

// Dial establishes the TCP connection. Subsequent calls are a no-op as long
// as the existing connection is still open.
func (c *Client) Dial() error {
	if c.conn != nil {
		return nil
	}

	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))

	conn, err := net.DialTimeout("tcp", addr, c.DialTimeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	c.conn = conn

	return nil
}

// Close terminates the TCP connection. Calling Close on a closed Client is a
// no-op.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	c.conn = nil

	return err
}

// Probe reads any banner the device emits immediately after connect. It
// returns whatever bytes arrive within a short window; an empty banner is
// not treated as an error because some firmware revisions stay silent until
// the first command.
func (c *Client) Probe() (string, error) {
	if c.conn == nil {
		return "", errors.New("telnet: not connected")
	}

	if err := c.conn.SetReadDeadline(time.Now().Add(idleWindow * 2)); err != nil {
		return "", fmt.Errorf("set read deadline: %w", err)
	}

	buf := make([]byte, 1024)

	n, err := c.conn.Read(buf)
	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		return "", fmt.Errorf("read banner: %w", err)
	}

	return string(buf[:n]), nil
}

// SendCommand writes cmd followed by CRLF and reads the device's response.
// The read terminates when the connection has been idle for idleWindow after
// the first byte arrived, or when the overall ReadTimeout is reached.
//
// Returns the raw response text (callers decide what counts as success — the
// device's textual conventions vary by firmware: some commands return "OK",
// others echo state, others return nothing).
func (c *Client) SendCommand(cmd string) (string, error) {
	if c.conn == nil {
		return "", errors.New("telnet: not connected")
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout)); err != nil {
		return "", fmt.Errorf("set write deadline: %w", err)
	}

	if _, err := c.conn.Write([]byte(cmd + "\r\n")); err != nil {
		return "", fmt.Errorf("write %q: %w", cmd, err)
	}

	overall := time.Now().Add(c.ReadTimeout)

	var buf bytes.Buffer

	chunk := make([]byte, 1024)
	haveBytes := false

	for {
		deadline := overall

		if haveBytes {
			d := time.Now().Add(idleWindow)
			if d.Before(overall) {
				deadline = d
			}
		}

		if err := c.conn.SetReadDeadline(deadline); err != nil {
			return buf.String(), fmt.Errorf("set read deadline: %w", err)
		}

		n, err := c.conn.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])

			haveBytes = true
		}

		if err == nil {
			continue
		}

		if errors.Is(err, os.ErrDeadlineExceeded) {
			if haveBytes {
				return buf.String(), nil
			}

			return buf.String(), fmt.Errorf("timed out waiting for response to %q", cmd)
		}

		return buf.String(), fmt.Errorf("read after %q: %w", cmd, err)
	}
}
