// Package main runs a LAN-visible DLNA / UPnP MediaServer backed by the
// dlnatest in-memory content tree.
//
// Usage:
//
//	example-dlna-server [--port 8200] [--name "My Library"]
//
// The server:
//   - Binds an HTTP server on 0.0.0.0:<port> (default 8200).
//   - Detects the host's primary LAN IPv4 to build the SSDP LOCATION header
//     and the absolute <res> URLs inside DIDL-Lite Browse responses.
//   - Joins the SSDP multicast group 239.255.255.250:1900 and answers
//     M-SEARCH requests whose ST matches upnp:rootdevice, ssdp:all, or
//     urn:schemas-upnp-org:device:MediaServer:1.
//   - Periodically sends ssdp:alive NOTIFY announcements.
//   - Sends ssdp:byebye on graceful shutdown (SIGINT / SIGTERM).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/dlna/dlnatest"
)

const (
	ssdpMulticastAddr = "239.255.255.250:1900"
	ssdpMulticastIP   = "239.255.255.250"
	ssdpPort          = 1900

	mediaServerURN = "urn:schemas-upnp-org:device:MediaServer:1"
	contentDirURN  = "urn:schemas-upnp-org:service:ContentDirectory:1"
	notifyInterval = 30 * time.Second
	ssdpMaxAge     = 1800
	serverVersion  = "AfterTouch/1.0 UPnP/1.0 AfterTouchDLNA/1.0"
)

func main() {
	port := flag.Int("port", 8200, "HTTP port to bind")
	name := flag.String("name", "AfterTouch Test Library", "UPnP friendlyName advertised over SSDP")

	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	lanIP, err := primaryLANIP()
	if err != nil {
		logger.Warn("could not detect LAN IP, falling back to loopback", "err", err)

		lanIP = "127.0.0.1"
	}

	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	location := fmt.Sprintf("http://%s:%d/rootDesc.xml", lanIP, *port)

	srv := dlnatest.NewServer(dlnatest.WithFriendlyName(*name))

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.HTTPHandler(),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start HTTP server.
	go func() {
		logger.Info("HTTP server starting", "addr", addr, "lanIP", lanIP, "location", location)

		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "err", err)
		}
	}()

	// Give the HTTP listener a moment to bind before we advertise it.
	time.Sleep(50 * time.Millisecond)

	udn := srv.UDN

	// Start SSDP listener + responder.
	go runSSDPListener(ctx, logger, udn, location)

	// Start periodic ssdp:alive announcements.
	go runSSDPAlive(ctx, logger, udn, location)

	logger.Info("DLNA MediaServer ready", "location", location, "name", *name)

	// Wait for shutdown signal.
	<-ctx.Done()

	logger.Info("shutting down...")

	// Send byebye before exiting.
	sendByebye(logger, udn)

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutCtx); err != nil {
		logger.Error("HTTP shutdown error", "err", err)
	}

	logger.Info("stopped")
}

// ----------------------------------------------------------------------------
// SSDP listener: answers M-SEARCH requests
// ----------------------------------------------------------------------------

func runSSDPListener(ctx context.Context, logger *slog.Logger, udn, location string) {
	group := &net.UDPAddr{IP: net.ParseIP(ssdpMulticastIP), Port: ssdpPort}

	// ListenMulticastUDP joins the multicast group on a system-chosen interface.
	// We iterate over all UP multicast-capable interfaces and listen on each.
	ifaces, err := multicastInterfaces()
	if err != nil {
		logger.Warn("SSDP: cannot list interfaces, using system default", "err", err)

		ifaces = []*net.Interface{nil} // nil = system default
	}

	if len(ifaces) == 0 {
		ifaces = []*net.Interface{nil}
	}

	// We only need one listening socket; use the first usable interface.
	// net.ListenMulticastUDP binds to 0.0.0.0:1900 internally, so a single
	// call is sufficient to receive multicast traffic on all interfaces on
	// most platforms.
	conn, err := net.ListenMulticastUDP("udp4", ifaces[0], group)
	if err != nil {
		logger.Warn("SSDP: ListenMulticastUDP failed (try running as root or check firewall)", "err", err)

		return
	}

	defer conn.Close()

	logger.Info("SSDP: listening for M-SEARCH on multicast", "group", group.String())

	buf := make([]byte, 2048)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Deadline timeout is expected; just continue.
			continue
		}

		msg := string(buf[:n])
		if !strings.HasPrefix(msg, "M-SEARCH") {
			continue
		}

		st := extractHeader(msg, "ST")
		logger.Debug("SSDP: M-SEARCH received", "from", src, "ST", st)

		if !stMatches(st) {
			continue
		}

		logger.Info("SSDP: answering M-SEARCH", "from", src, "ST", st)

		reply := buildMSearchReply(udn, location, st)
		_, _ = conn.WriteToUDP([]byte(reply), src)
	}
}

// multicastInterfaces returns all UP interfaces that support multicast.
func multicastInterfaces() ([]*net.Interface, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []*net.Interface

	for i := range all {
		iface := &all[i]
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}

		result = append(result, iface)
	}

	return result, nil
}

// stMatches returns true when the ST header should receive an M-SEARCH reply.
func stMatches(st string) bool {
	switch st {
	case "ssdp:all", "upnp:rootdevice", mediaServerURN:
		return true
	}

	return false
}

// buildMSearchReply builds an HTTP/1.1 200 OK SSDP response.
func buildMSearchReply(udn, location, st string) string {
	usn := usnForST(udn, st)

	return fmt.Sprintf(
		"HTTP/1.1 200 OK\r\n"+
			"CACHE-CONTROL: max-age=%d\r\n"+
			"DATE: %s\r\n"+
			"EXT:\r\n"+
			"LOCATION: %s\r\n"+
			"SERVER: %s\r\n"+
			"ST: %s\r\n"+
			"USN: %s\r\n"+
			"\r\n",
		ssdpMaxAge,
		time.Now().UTC().Format(http.TimeFormat),
		location,
		serverVersion,
		st,
		usn,
	)
}

// usnForST builds the USN header value for a given ST.
func usnForST(udn, st string) string {
	if st == "upnp:rootdevice" || st == "ssdp:all" {
		return udn + "::upnp:rootdevice"
	}

	return udn + "::" + st
}

// ----------------------------------------------------------------------------
// SSDP alive announcements
// ----------------------------------------------------------------------------

func runSSDPAlive(ctx context.Context, logger *slog.Logger, udn, location string) {
	// Send an initial batch immediately, then repeat on the interval.
	sendAlive(logger, udn, location)

	ticker := time.NewTicker(notifyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sendAlive(logger, udn, location)
		}
	}
}

func sendAlive(logger *slog.Logger, udn, location string) {
	nts := []struct{ nt, usn string }{
		{"upnp:rootdevice", udn + "::upnp:rootdevice"},
		{udn, udn},
		{mediaServerURN, udn + "::" + mediaServerURN},
		{contentDirURN, udn + "::" + contentDirURN},
	}

	conn, err := net.Dial("udp4", ssdpMulticastAddr)
	if err != nil {
		logger.Warn("SSDP: cannot send alive notification", "err", err)

		return
	}

	defer conn.Close()

	for _, n := range nts {
		msg := fmt.Sprintf(
			"NOTIFY * HTTP/1.1\r\n"+
				"HOST: %s\r\n"+
				"CACHE-CONTROL: max-age=%d\r\n"+
				"LOCATION: %s\r\n"+
				"NT: %s\r\n"+
				"NTS: ssdp:alive\r\n"+
				"SERVER: %s\r\n"+
				"USN: %s\r\n"+
				"\r\n",
			ssdpMulticastAddr, ssdpMaxAge, location,
			n.nt, serverVersion, n.usn,
		)
		_, _ = conn.Write([]byte(msg))
	}

	logger.Debug("SSDP: alive announcements sent")
}

// ----------------------------------------------------------------------------
// SSDP byebye on shutdown
// ----------------------------------------------------------------------------

func sendByebye(logger *slog.Logger, udn string) {
	conn, err := net.Dial("udp4", ssdpMulticastAddr)
	if err != nil {
		logger.Warn("SSDP: cannot send byebye", "err", err)

		return
	}

	defer conn.Close()

	nts := []struct{ nt, usn string }{
		{"upnp:rootdevice", udn + "::upnp:rootdevice"},
		{udn, udn},
		{mediaServerURN, udn + "::" + mediaServerURN},
		{contentDirURN, udn + "::" + contentDirURN},
	}

	for _, n := range nts {
		msg := fmt.Sprintf(
			"NOTIFY * HTTP/1.1\r\n"+
				"HOST: %s\r\n"+
				"NT: %s\r\n"+
				"NTS: ssdp:byebye\r\n"+
				"USN: %s\r\n"+
				"\r\n",
			ssdpMulticastAddr, n.nt, n.usn,
		)
		_, _ = conn.Write([]byte(msg))
	}

	logger.Info("SSDP: byebye announcements sent")
}

// ----------------------------------------------------------------------------
// Network helpers
// ----------------------------------------------------------------------------

// primaryLANIP returns the first non-loopback, non-link-local IPv4 address
// found on any UP interface.
func primaryLANIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			v4 := ipNet.IP.To4()
			if v4 == nil {
				continue
			}

			if v4.IsLoopback() || v4.IsLinkLocalUnicast() {
				continue
			}

			return v4.String(), nil
		}
	}

	return "", fmt.Errorf("no usable LAN IPv4 address found")
}

// extractHeader extracts a header value from a raw HTTP-style SSDP message.
// Key comparison is case-insensitive.
func extractHeader(msg, key string) string {
	lower := strings.ToLower(key) + ":"

	for _, line := range strings.Split(msg, "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.ToLower(trimmed), lower) {
			return strings.TrimSpace(trimmed[len(lower):])
		}
	}

	return ""
}
