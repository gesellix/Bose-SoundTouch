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
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
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
	mediaDir := flag.String("media-dir", "", "serve real audio files + artwork from this directory "+
		"(searched recursively, so an artist/album tree works) instead of the built-in silent test "+
		"tracks. Audio: .mp3/.wav/.flac/.m4a/.ogg. Art per track: a sibling <name>.jpg/.png, else a "+
		"cover.jpg/cover.png/folder.jpg in the same album folder. Files are loaded into memory, so "+
		"point it at an album or a modest folder, not your whole library")

	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	lanIP, err := primaryLANIP()
	if err != nil {
		logger.Warn("could not detect LAN IP, falling back to loopback", "err", err)

		lanIP = "127.0.0.1"
	}

	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	location := fmt.Sprintf("http://%s:%d/rootDesc.xml", lanIP, *port)

	// Join the SSDP multicast group on the interface that owns the LAN IP. On
	// macOS net.Interfaces() lists lo0 (UP+MULTICAST) first, so picking the
	// "first" multicast interface would join on loopback and never receive the
	// LAN M-SEARCH from clients like AfterTouch.
	lanIface := interfaceForIP(lanIP)
	if lanIface != nil {
		logger.Info("SSDP: will join multicast on LAN interface", "iface", lanIface.Name, "ip", lanIP)
	}

	opts := []dlnatest.Option{dlnatest.WithFriendlyName(*name)}

	if *mediaDir != "" {
		tree, n, err := loadTreeFromDir(*mediaDir, *name)
		if err != nil {
			logger.Error("failed to load --media-dir", "dir", *mediaDir, "err", err)
			os.Exit(1)
		}

		opts = append(opts, dlnatest.WithTree(tree))

		logger.Info("serving real media from directory", "dir", *mediaDir, "tracks", n)
	}

	srv := dlnatest.NewServer(opts...)

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: withAccessLog(logger, srv.HTTPHandler()),
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
	go runSSDPListener(ctx, logger, udn, location, lanIface)

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

func runSSDPListener(ctx context.Context, logger *slog.Logger, udn, location string, ifi *net.Interface) {
	group := &net.UDPAddr{IP: net.ParseIP(ssdpMulticastIP), Port: ssdpPort}

	// Join the group on the LAN interface. If we could not resolve it, fall back
	// to the first non-loopback multicast interface (never loopback, which would
	// only ever receive same-host loopback traffic).
	if ifi == nil {
		if cands, err := multicastInterfaces(); err == nil {
			for _, c := range cands {
				if c != nil && c.Flags&net.FlagLoopback == 0 {
					ifi = c

					break
				}
			}
		}
	}

	conn, err := net.ListenMulticastUDP("udp4", ifi, group)
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

// ----------------------------------------------------------------------------
// --media-dir loader
// ----------------------------------------------------------------------------

// loadTreeFromDir walks dir recursively and builds a single flat content folder
// from every audio file found, so an artist/album tree works. Album art for a
// track is, in order of preference: a sibling <basename>.<img>, then a
// cover.jpg/cover.png/folder.jpg in the track's own directory. Returns the tree
// and track count.
func loadTreeFromDir(dir, fallbackName string) (*dlnatest.Tree, int, error) {
	rootClean := filepath.Clean(dir)

	// Cache the resolved cover per directory so we read each album's folder.jpg
	// once rather than for every track in it.
	type cover struct {
		data []byte
		mime string
	}

	coverCache := map[string]cover{}

	dirCover := func(d string) ([]byte, string) {
		if c, ok := coverCache[d]; ok {
			return c.data, c.mime
		}

		var c cover

		for _, n := range []string{"cover.jpg", "cover.jpeg", "cover.png", "folder.jpg", "folder.png", "albumart.jpg", "albumart.png"} {
			if b, err := os.ReadFile(filepath.Join(d, n)); err == nil {
				c = cover{data: b, mime: imageMimeForExt(filepath.Ext(n))}

				break
			}
		}

		coverCache[d] = c

		return c.data, c.mime
	}

	// Group tracks by their containing directory (preserving first-seen order),
	// so each real album folder becomes its own browsable + playable container
	// named after the directory, rather than one flat list named after --name.
	type group struct {
		dir   string
		items []*dlnatest.Item
	}

	groups := map[string]*group{}

	var order []string

	total := 0

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // skip unreadable entries and directories
		}

		mime := audioMimeForExt(filepath.Ext(path))
		if mime == "" {
			return nil // not an audio file we recognise
		}

		payload, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // skip unreadable file, keep walking
		}

		trackDir := filepath.Dir(path)
		base := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))

		// Prefer a per-track image sibling; fall back to the album-folder cover.
		art, artMime := dirCover(trackDir)

		for _, ae := range []string{".jpg", ".jpeg", ".png", ".webp"} {
			if b, aerr := os.ReadFile(filepath.Join(trackDir, base+ae)); aerr == nil {
				art = b
				artMime = imageMimeForExt(ae)

				break
			}
		}

		g := groups[trackDir]
		if g == nil {
			g = &group{dir: trackDir}
			groups[trackDir] = g
			order = append(order, trackDir)
		}

		g.items = append(g.items, &dlnatest.Item{
			Title:      base,
			Class:      "object.item.audioItem.musicTrack",
			Artist:     artistForDir(trackDir, rootClean),
			Album:      albumTitle(trackDir, rootClean, fallbackName),
			MimeType:   mime,
			Payload:    payload,
			ArtPayload: art,
			ArtMime:    artMime,
		})
		total++

		return nil
	})
	if walkErr != nil {
		return nil, 0, walkErr
	}

	if total == 0 {
		return nil, 0, fmt.Errorf("no audio files (.mp3/.wav/.flac/.m4a/.ogg) found under %s", dir)
	}

	containers := make([]*dlnatest.Container, 0, len(order))

	for ci, d := range order {
		cid := strconv.Itoa(ci + 1)
		g := groups[d]

		for ti, it := range g.items {
			it.ID = fmt.Sprintf("%s$%d", cid, ti)
			it.ParentID = cid
		}

		containers = append(containers, &dlnatest.Container{
			ID:       cid,
			ParentID: "0",
			Title:    albumTitle(d, rootClean, fallbackName),
			Class:    "object.container.storageFolder",
			Children: g.items,
		})
	}

	return &dlnatest.Tree{Containers: containers}, total, nil
}

// albumTitle returns the display name for a track directory: the directory's own
// name, or the fallback (the --name) when the tracks sit directly in the root.
func albumTitle(trackDir, root, fallback string) string {
	if filepath.Clean(trackDir) == root {
		return fallback
	}

	return filepath.Base(trackDir)
}

// artistForDir derives the artist from the directory above the album folder
// (e.g. <root>/<artist>/<album>/track.mp3 → "<artist>"). Falls back to
// "Unknown Artist" when there is no artist level (album directly under root, or
// tracks directly in root).
func artistForDir(trackDir, root string) string {
	clean := filepath.Clean(trackDir)
	if clean == root {
		return "Unknown Artist"
	}

	parent := filepath.Dir(clean)
	if parent == root {
		return "Unknown Artist"
	}

	return filepath.Base(parent)
}

// audioMimeForExt maps an audio file extension to a MIME type, or "" if the
// extension is not a recognised audio format.
func audioMimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/x-wav"
	case ".flac":
		return "audio/flac"
	case ".m4a", ".mp4":
		return "audio/mp4"
	case ".ogg":
		return "audio/ogg"
	}

	return ""
}

// imageMimeForExt maps an image file extension to a MIME type.
func imageMimeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	}

	return "application/octet-stream"
}

// ----------------------------------------------------------------------------
// HTTP access logging (debugging aid)
// ----------------------------------------------------------------------------

// statusRecorder captures the status code and byte count of a response.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n

	return n, err
}

// withAccessLog logs every HTTP request the server handles. For ContentDirectory
// Browse POSTs it also surfaces the ObjectID and BrowseFlag so the speaker's
// browse sequence (and whether it ever resolves a track's metadata) is visible.
func withAccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var browseAttrs []any

		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "ContentDir") {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))

			browseAttrs = []any{
				"objectID", between(string(body), "<ObjectID>", "</ObjectID>"),
				"browseFlag", between(string(body), "<BrowseFlag>", "</BrowseFlag>"),
			}
		}

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"from", r.RemoteAddr,
			"dur", time.Since(start).String(),
		}
		attrs = append(attrs, browseAttrs...)

		logger.Info("HTTP", attrs...)
	})
}

// between returns the text between the first occurrence of openTag and the next
// closeTag, or "" if not found. Used for lightweight SOAP field extraction in logs.
func between(s, openTag, closeTag string) string {
	i := strings.Index(s, openTag)
	if i < 0 {
		return ""
	}

	i += len(openTag)

	j := strings.Index(s[i:], closeTag)
	if j < 0 {
		return ""
	}

	return s[i : i+j]
}

// interfaceForIP returns the UP, multicast-capable interface that owns the given
// IPv4 address, or nil if none is found.
func interfaceForIP(ip string) *net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for i := range ifaces {
		iface := &ifaces[i]
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}

		addrs, aerr := iface.Addrs()
		if aerr != nil {
			continue
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if v4 := ipNet.IP.To4(); v4 != nil && v4.String() == ip {
					return iface
				}
			}
		}
	}

	return nil
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
