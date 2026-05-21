package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/export"
	"github.com/gesellix/bose-soundtouch/pkg/service/health"
	speakerssh "github.com/gesellix/bose-soundtouch/pkg/ssh"
)

// diagnosticReport is the structured summary included as diagnostic.json
// inside the encrypted archive. Raw datastore XML files are added verbatim
// alongside it so the maintainer can compare on-disk state with what the
// service serves.
type diagnosticReport struct {
	GeneratedAt    string               `json:"generated_at"`
	ServiceVersion map[string]string    `json:"service_version"`
	HealthChecks   []health.CheckResult `json:"health_checks"`
	Devices        []deviceDiagnostic   `json:"devices"`
}

type deviceDiagnostic struct {
	AccountID       string             `json:"account_id"`
	DeviceID        string             `json:"device_id"`
	ProductCode     string             `json:"product_code,omitempty"`
	FirmwareVersion string             `json:"firmware_version,omitempty"`
	Name            string             `json:"name,omitempty"`
	IPAddress       string             `json:"ip_address,omitempty"`
	Sources         []sourceDiagnostic `json:"sources,omitempty"`
	Presets         []presetDiagnostic `json:"presets,omitempty"`
}

type sourceDiagnostic struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	SourceKeyType string `json:"source_key_type,omitempty"`
	ProviderID    string `json:"provider_id,omitempty"`
	Status        string `json:"status,omitempty"`
}

type presetDiagnostic struct {
	Slot     string `json:"slot"`
	Name     string `json:"name,omitempty"`
	Source   string `json:"source,omitempty"`
	SourceID string `json:"source_id,omitempty"`
	Location string `json:"location,omitempty"`
}

// HandleExportDiagnostic builds a tar.gz archive containing a structured JSON
// summary plus the raw datastore XML files verbatim, encrypts the archive with
// the maintainer's embedded SSH public key (age/agessh), and returns it as a
// downloadable .age file. Credentials and authentication tokens are
// intentionally excluded from the JSON summary; XML files are included as-is.
func (s *Server) HandleExportDiagnostic(w http.ResponseWriter, _ *http.Request) {
	archive, err := s.buildDiagnosticArchive()
	if err != nil {
		log.Printf("[Export] build archive: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to build diagnostic archive")

		return
	}

	encrypted, err := export.EncryptDiagnostic(archive)
	if err != nil {
		log.Printf("[Export] encrypt: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to encrypt diagnostic archive")

		return
	}

	ts := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	filename := fmt.Sprintf("aftertouch-diagnostic-%s.age", ts)

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if _, err := w.Write(encrypted); err != nil {
		log.Printf("[Export] write response: %v", err)
	}
}

// buildDiagnosticArchive returns a gzipped tar archive containing:
//   - diagnostic.json  — structured health/device summary
//   - datastore/accounts/{account}/devices/{device}/*.xml  — raw XML verbatim
//   - http/service/...  — HTTP responses from the local service endpoints
//   - http/speaker/...  — HTTP responses from each speaker's local API (port 8090)
//   - system/ca.pem     — service CA certificate (if configured)
//   - system/resolv.conf — host DNS resolver configuration
//   - settings.json     — service settings with secrets redacted
//   - env.txt           — filtered process environment
func (s *Server) buildDiagnosticArchive() ([]byte, error) {
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	report := s.buildDiagnosticReport()

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal summary: %w", err)
	}

	if addErr := addTarBytes(tw, "diagnostic.json", jsonBytes); addErr != nil {
		return nil, fmt.Errorf("add diagnostic.json: %w", addErr)
	}

	devices, err := s.ds.ListAllDevices()
	if err != nil {
		log.Printf("[Export] list devices: %v", err)
	}

	for i := range devices {
		dev := &devices[i]
		dir := s.ds.AccountDeviceDir(dev.AccountID, dev.DeviceID)

		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Printf("[Export] read dir %s: %v", dir, err)

			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".xml") {
				continue
			}

			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				log.Printf("[Export] read %s: %v", entry.Name(), err)

				continue
			}

			archivePath := "datastore/accounts/" + dev.AccountID + "/devices/" + dev.DeviceID + "/" + entry.Name()
			if err := addTarBytes(tw, archivePath, data); err != nil {
				log.Printf("[Export] add %s: %v", archivePath, err)
			}
		}
	}

	client := diagHTTPClient()
	s.addServiceHTTP(tw, client, devices)
	s.addSpeakerHTTP(tw, client, devices)
	addSpeakerSSH(tw, devices)
	s.addSystemFiles(tw)
	s.addServiceLog(tw)
	s.addSettingsJSON(tw)
	addEnvVars(tw)

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}

	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}

	return buf.Bytes(), nil
}

// diagHTTPClient returns an HTTP client suited for internal diagnostic fetches:
// short timeout and TLS verification skipped so it can call the service's own
// HTTPS endpoint without the CA being trusted by the host OS.
func diagHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
}

func diagFetch(client *http.Client, rawURL string) ([]byte, error) {
	resp, err := client.Get(rawURL) //nolint:noctx
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	return io.ReadAll(resp.Body)
}

// addServiceHTTP appends HTTP responses from the local service into the archive
// under http/service/. Each fetch is best-effort; errors are logged and skipped.
func (s *Server) addServiceHTTP(tw *tar.Writer, client *http.Client, devices []models.ServiceDeviceInfo) {
	base := strings.TrimRight(s.serverURL, "/")

	tryAdd := func(archivePath, url string) {
		data, err := diagFetch(client, url)
		if err != nil {
			log.Printf("[Export] fetch %s: %v", url, err)

			return
		}

		if err := addTarBytes(tw, archivePath, data); err != nil {
			log.Printf("[Export] add %s: %v", archivePath, err)
		}
	}

	tryAdd("http/service/sourceproviders.xml", base+"/streaming/sourceproviders")

	seenAccounts := map[string]bool{}

	for i := range devices {
		dev := &devices[i]

		if !seenAccounts[dev.AccountID] {
			seenAccounts[dev.AccountID] = true
			pfx := "http/service/account-" + dev.AccountID
			acct := base + "/streaming/account/" + dev.AccountID
			tryAdd(pfx+"/full.xml", acct+"/full")
			tryAdd(pfx+"/sources.xml", acct+"/sources")
			tryAdd(pfx+"/presets.xml", acct+"/presets")
		}

		if dev.DeviceID == "" {
			continue
		}

		dpfx := "http/service/account-" + dev.AccountID + "/device-" + dev.DeviceID
		dpath := base + "/streaming/account/" + dev.AccountID + "/device/" + dev.DeviceID
		tryAdd(dpfx+"/presets.xml", dpath+"/presets")
		tryAdd(dpfx+"/recents.xml", dpath+"/recents")
	}
}

// addSpeakerHTTP appends HTTP responses fetched directly from each speaker's
// local API (port 8090) into the archive under http/speaker/{deviceID}/.
// Speakers that are unreachable are silently skipped.
func (s *Server) addSpeakerHTTP(tw *tar.Writer, client *http.Client, devices []models.ServiceDeviceInfo) {
	endpoints := []string{"sources", "presets", "now_playing", "info", "recents"}

	for i := range devices {
		dev := &devices[i]

		if dev.IPAddress == "" {
			continue
		}

		speakerBase := "http://" + dev.IPAddress + ":8090"
		id := dev.DeviceID

		if id == "" {
			id = dev.IPAddress
		}

		for _, ep := range endpoints {
			data, err := diagFetch(client, speakerBase+"/"+ep)
			if err != nil {
				log.Printf("[Export] speaker %s /%s: %v", id, ep, err)

				continue
			}

			archivePath := "http/speaker/" + id + "/" + ep + ".xml"
			if err := addTarBytes(tw, archivePath, data); err != nil {
				log.Printf("[Export] add %s: %v", archivePath, err)
			}
		}
	}
}

// speakerSSHPaths lists file paths to retrieve from each speaker via SSH.
var speakerSSHPaths = []string{
	"/etc/pki/tls/certs/ca-bundle.crt",
	"/etc/ssl/certs/ca-certificates.crt",
}

// speakerLogWindow is the default look-back period for speaker syslog entries.
const speakerLogWindow = 20 * time.Minute

// speakerLogFormats are the timestamp layouts attempted when parsing a busybox
// syslog line. Busybox logread produces "Mon Jan _2 15:04:05 2006" (with year)
// on newer firmware; older builds omit the year.
var speakerLogFormats = []string{
	"Mon Jan _2 15:04:05 2006", // newer busybox: "Wed Jun  4 12:34:56 2025"
	"Mon Jan 02 15:04:05 2006", // zero-padded day variant
}

// parseSpeakerLogTime extracts the timestamp from the leading field of a busybox
// syslog line. currentYear is used as a fallback when the log line has no year
// field. Returns the zero Time and false when no format matches.
func parseSpeakerLogTime(line string, currentYear int) (time.Time, bool) {
	for _, layout := range speakerLogFormats {
		if len(line) < len(layout) {
			continue
		}

		t, err := time.Parse(layout, line[:len(layout)])
		if err == nil {
			return t.UTC(), true
		}
	}

	// Try without year: assume current year.
	noYearFmt := "Mon Jan _2 15:04:05"
	noYearFmt02 := "Mon Jan 02 15:04:05"

	for _, layout := range []string{noYearFmt, noYearFmt02} {
		if len(line) < len(layout) {
			continue
		}

		withYear := line[:len(layout)] + strings.Repeat(" ", 1) + fmt.Sprintf("%d", currentYear)
		fullLayout := layout + " 2006"

		t, err := time.Parse(fullLayout, withYear)
		if err == nil {
			return t.UTC(), true
		}
	}

	return time.Time{}, false
}

// filterSpeakerLog returns only the lines from rawLog whose timestamp falls
// within the given window before now. Lines whose timestamp cannot be parsed
// are kept (fail-open) so that unparseable headers or continuation lines are
// not silently dropped.
func filterSpeakerLog(rawLog string, window time.Duration) string {
	cutoff := time.Now().UTC().Add(-window)
	currentYear := time.Now().Year()

	var out strings.Builder

	for _, line := range strings.SplitAfter(rawLog, "\n") {
		t, ok := parseSpeakerLogTime(line, currentYear)
		if !ok || !t.Before(cutoff) {
			out.WriteString(line)
		}
	}

	return out.String()
}

// addSpeakerSSH connects to each speaker via SSH and copies the speaker-side
// CA certificate files and log output into the archive under ssh/speaker/{deviceID}/.
// Speakers that are unreachable or have SSH disabled are silently skipped.
func addSpeakerSSH(tw *tar.Writer, devices []models.ServiceDeviceInfo) {
	for i := range devices {
		dev := &devices[i]

		if dev.IPAddress == "" {
			continue
		}

		id := dev.DeviceID
		if id == "" {
			id = dev.IPAddress
		}

		sc := speakerssh.NewClient(dev.IPAddress)

		for _, remotePath := range speakerSSHPaths {
			data, err := sc.ReadFile(remotePath)
			if err != nil {
				log.Printf("[Export] SSH %s %s: %v", id, remotePath, err)

				continue
			}

			archivePath := "ssh/speaker/" + id + remotePath
			if err := addTarBytes(tw, archivePath, data); err != nil {
				log.Printf("[Export] add %s: %v", archivePath, err)
			}
		}

		// dmesg and any other plain commands.
		for filename, cmd := range map[string]string{"dmesg.txt": "dmesg"} {
			out, err := sc.Run(cmd)
			if err != nil && strings.TrimSpace(out) == "" {
				log.Printf("[Export] SSH %s %q: %v", id, cmd, err)

				continue
			}

			archivePath := "ssh/speaker/" + id + "/" + filename
			if err := addTarBytes(tw, archivePath, []byte(out)); err != nil {
				log.Printf("[Export] add %s: %v", archivePath, err)
			}
		}

		// Syslog: fetch raw, strip 127.0.0.1 noise, then keep only the last speakerLogWindow.
		rawLog, logErr := sc.Run("logread 2>/dev/null | grep -v '127.0.0.1'")
		if logErr != nil && strings.TrimSpace(rawLog) == "" {
			log.Printf("[Export] SSH %s logread: %v", id, logErr)
		} else {
			filtered := filterSpeakerLog(rawLog, speakerLogWindow)
			archivePath := "ssh/speaker/" + id + "/logread.txt"

			if addErr := addTarBytes(tw, archivePath, []byte(filtered)); addErr != nil {
				log.Printf("[Export] add %s: %v", archivePath, addErr)
			}
		}
	}
}

// addServiceLog appends the in-memory service log buffer as logs/service.txt.
// Each entry is formatted as "2006-01-02T15:04:05Z <message>".
func (s *Server) addServiceLog(tw *tar.Writer) {
	if s.logBuf == nil {
		return
	}

	entries := s.logBuf.Snapshot()
	if len(entries) == 0 {
		return
	}

	var sb strings.Builder

	for _, e := range entries {
		sb.WriteString(e.Time.UTC().Format(time.RFC3339))
		sb.WriteByte(' ')
		sb.WriteString(e.Message)
		sb.WriteByte('\n')
	}

	if err := addTarBytes(tw, "logs/service.txt", []byte(sb.String())); err != nil {
		log.Printf("[Export] add logs/service.txt: %v", err)
	}
}

// addSystemFiles appends the service CA cert (if configured) and the host
// resolver configuration into the archive under system/.
func (s *Server) addSystemFiles(tw *tar.Writer) {
	if path := s.ownCACertPath(); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[Export] read CA cert %s: %v", path, err)
		} else if err := addTarBytes(tw, "system/ca.pem", data); err != nil {
			log.Printf("[Export] add system/ca.pem: %v", err)
		}
	}

	if data, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		if err := addTarBytes(tw, "system/resolv.conf", data); err != nil {
			log.Printf("[Export] add system/resolv.conf: %v", err)
		}
	}
}

// diagSettings is a copy of datastore.Settings with secrets zeroed out so the
// struct can be marshalled into the archive without exposing credentials.
type diagSettings struct {
	ServerURL             string         `json:"server_url"`
	HTTPSServerURL        string         `json:"https_server_url,omitempty"`
	RedactLogs            bool           `json:"redact_logs"`
	LogBodies             bool           `json:"log_bodies"`
	RecordInteractions    bool           `json:"record_interactions"`
	DiscoveryInterval     string         `json:"discovery_interval,omitempty"`
	DiscoveryEnabled      bool           `json:"discovery_enabled"`
	DNSEnabled            bool           `json:"dns_enabled"`
	DNSUpstream           []string       `json:"dns_upstream,omitempty"`
	DNSBindAddr           string         `json:"dns_bind_addr,omitempty"`
	InternalPaths         []string       `json:"internal_paths,omitempty"`
	Shortcuts             map[string]int `json:"shortcuts,omitempty"`
	SpotifyClientID       string         `json:"spotify_client_id,omitempty"`
	SpotifyClientSecret   string         `json:"spotify_client_secret,omitempty"`
	SpotifyRedirectURI    string         `json:"spotify_redirect_uri,omitempty"`
	AmazonClientID        string         `json:"amazon_client_id,omitempty"`
	AmazonClientSecret    string         `json:"amazon_client_secret,omitempty"`
	AmazonRedirectURI     string         `json:"amazon_redirect_uri,omitempty"`
	TrustForwardedHeaders bool           `json:"trust_forwarded_headers,omitempty"`
	TrustedProxyCIDRs     []string       `json:"trusted_proxy_cidrs,omitempty"`
	TuneInStreamFormats   string         `json:"tunein_stream_formats,omitempty"`
}

// addSettingsJSON serialises the service settings into the archive as
// settings.json. OAuth client secrets are replaced with "[REDACTED]" so the
// file is safe to share.
func (s *Server) addSettingsJSON(tw *tar.Writer) {
	st, err := s.ds.GetSettings()
	if err != nil {
		log.Printf("[Export] get settings: %v", err)

		return
	}

	redact := func(v string) string {
		if v != "" {
			return "[REDACTED]"
		}

		return ""
	}

	ds := diagSettings{
		ServerURL:             st.ServerURL,
		HTTPSServerURL:        st.HTTPServerURL,
		RedactLogs:            st.RedactLogs,
		LogBodies:             st.LogBodies,
		RecordInteractions:    st.RecordInteractions,
		DiscoveryInterval:     st.DiscoveryInterval,
		DiscoveryEnabled:      st.DiscoveryEnabled,
		DNSEnabled:            st.DNSEnabled,
		DNSUpstream:           st.DNSUpstream,
		DNSBindAddr:           st.DNSBindAddr,
		InternalPaths:         st.InternalPaths,
		Shortcuts:             st.Shortcuts,
		SpotifyClientID:       st.SpotifyClientID,
		SpotifyClientSecret:   redact(st.SpotifyClientSecret),
		SpotifyRedirectURI:    st.SpotifyRedirectURI,
		AmazonClientID:        st.AmazonClientID,
		AmazonClientSecret:    redact(st.AmazonClientSecret),
		AmazonRedirectURI:     st.AmazonRedirectURI,
		TrustForwardedHeaders: st.TrustForwardedHeaders,
		TrustedProxyCIDRs:     st.TrustedProxyCIDRs,
		TuneInStreamFormats:   st.TuneInStreamFormats,
	}

	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		log.Printf("[Export] marshal settings: %v", err)

		return
	}

	if err := addTarBytes(tw, "settings.json", data); err != nil {
		log.Printf("[Export] add settings.json: %v", err)
	}
}

// secretEnvKeywords lists substrings that, if present in an env-var name,
// cause the variable to be omitted from the diagnostic export.
var secretEnvKeywords = []string{
	"secret", "password", "passwd", "token", "apikey", "api_key",
	"credential", "auth", "private", "passphrase",
}

// addEnvVars appends a filtered list of environment variables to the archive
// as env.txt. Variables whose names suggest credentials are omitted.
func addEnvVars(tw *tar.Writer) {
	raw := os.Environ()
	sort.Strings(raw)

	var lines []string

	for _, kv := range raw {
		name := strings.ToLower(strings.SplitN(kv, "=", 2)[0])
		skip := false

		for _, kw := range secretEnvKeywords {
			if strings.Contains(name, kw) {
				skip = true

				break
			}
		}

		if !skip {
			lines = append(lines, kv)
		}
	}

	data := []byte(strings.Join(lines, "\n") + "\n")
	if err := addTarBytes(tw, "env.txt", data); err != nil {
		log.Printf("[Export] add env.txt: %v", err)
	}
}

func addTarBytes(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Now().UTC(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	_, err := tw.Write(data)

	return err
}

func (s *Server) buildDiagnosticReport() diagnosticReport {
	report := diagnosticReport{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		ServiceVersion: buildVersionInfo(),
	}

	if s.healthRegistry != nil {
		report.HealthChecks = s.healthRegistry.RunAll()
	}

	devices, err := s.ds.ListAllDevices()
	if err != nil {
		log.Printf("[Export] list devices: %v", err)

		return report
	}

	for i := range devices {
		dev := &devices[i]
		dd := deviceDiagnostic{
			AccountID:       dev.AccountID,
			DeviceID:        dev.DeviceID,
			ProductCode:     dev.ProductCode,
			FirmwareVersion: dev.FirmwareVersion,
			Name:            dev.Name,
			IPAddress:       dev.IPAddress,
		}

		if sources, err := s.ds.GetConfiguredSources(dev.AccountID, dev.DeviceID); err == nil {
			for i := range sources {
				src := &sources[i]
				dd.Sources = append(dd.Sources, sourceDiagnostic{
					ID:            src.ID,
					Name:          src.Name,
					SourceKeyType: src.SourceKeyType,
					ProviderID:    src.SourceProviderID,
					Status:        src.Status,
				})
			}
		}

		if presets, err := s.ds.GetPresets(dev.AccountID, dev.DeviceID); err == nil {
			for i := range presets {
				p := &presets[i]
				dd.Presets = append(dd.Presets, presetDiagnostic{
					Slot:     p.ButtonNumber,
					Name:     p.Name,
					Source:   p.Source,
					SourceID: p.SourceID,
					Location: p.Location,
				})
			}
		}

		report.Devices = append(report.Devices, dd)
	}

	return report
}
