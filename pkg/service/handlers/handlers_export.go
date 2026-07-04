package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/export"
	"github.com/gesellix/bose-soundtouch/pkg/service/health"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/gesellix/bose-soundtouch/pkg/speaker"
	speakerssh "github.com/gesellix/bose-soundtouch/pkg/ssh"
	"github.com/gesellix/bose-soundtouch/pkg/telnet"
)

// diagnosticReport is the structured summary included as diagnostic.json
// inside the encrypted archive. Raw datastore XML files are added verbatim
// alongside it so the maintainer can compare on-disk state with what the
// service serves.
type diagnosticReport struct {
	GeneratedAt         string               `json:"generated_at"`
	ServiceVersion      map[string]string    `json:"service_version"`
	HealthChecks        []health.CheckResult `json:"health_checks"`
	Devices             []deviceDiagnostic   `json:"devices"`
	DeprecatedRouteHits map[string]int64     `json:"deprecated_route_hits,omitempty"`
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
	RedirectConfig  *redirectConfig    `json:"redirect_config,omitempty"`
}

// redirectConfig captures where a speaker is configured to send its
// marge/BMX/streaming traffic, and how AfterTouch obtained that config. The
// URLs come from the on-device SoundTouchSdkPrivateCfg.xml (via SSH) or, when
// SSH is unavailable, from `getpdo CurrentSystemConfiguration` over telnet.
// A speaker reachable only via telnet was, by elimination, migrated with the
// telnet method — the xml/hosts/resolv methods all require SSH.
type redirectConfig struct {
	Source                  string `json:"source"` // "ssh" | "telnet" | "none"
	SSHReachable            bool   `json:"ssh_reachable"`
	InferredMigrationMethod string `json:"inferred_migration_method,omitempty"`
	MargeServerURL          string `json:"marge_server_url,omitempty"`
	StatsServerURL          string `json:"stats_server_url,omitempty"`
	SwUpdateURL             string `json:"sw_update_url,omitempty"`
	BmxRegistryURL          string `json:"bmx_registry_url,omitempty"`
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

	// Collect speaker redirect config first (SSH-preferred, telnet fallback);
	// it writes raw artifacts into the archive and feeds parsed URLs +
	// provenance into the JSON report below.
	redirectCfgs := s.collectSpeakerRedirectConfig(tw)

	report := s.buildDiagnosticReport(redirectCfgs)

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
			log.Printf("[Export] read dir %s: %v", sanitizeLog(dir), err)

			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".xml") {
				continue
			}

			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				log.Printf("[Export] read %s: %v", sanitizeLog(entry.Name()), err)

				continue
			}

			archivePath := "datastore/accounts/" + dev.AccountID + "/devices/" + dev.DeviceID + "/" + entry.Name()
			if err := addTarBytes(tw, archivePath, data); err != nil {
				log.Printf("[Export] add %s: %v", sanitizeLog(archivePath), err)
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
			log.Printf("[Export] fetch %s: %v", sanitizeLog(url), err)

			return
		}

		if err := addTarBytes(tw, archivePath, data); err != nil {
			log.Printf("[Export] add %s: %v", sanitizeLog(archivePath), err)
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
				log.Printf("[Export] speaker %s /%s: %v", sanitizeLog(id), ep, err)

				continue
			}

			archivePath := "http/speaker/" + id + "/" + ep + ".xml"
			if err := addTarBytes(tw, archivePath, data); err != nil {
				log.Printf("[Export] add %s: %v", sanitizeLog(archivePath), err)
			}
		}
	}
}

// speakerSSHPaths lists file paths to retrieve from each speaker via SSH.
// Each is best-effort: paths that don't exist on a given firmware/migration
// method are logged and skipped.
var speakerSSHPaths = []string{
	"/etc/pki/tls/certs/ca-bundle.crt",
	"/etc/pki/tls/certs/ca-bundle.crt.original", // pre-migration CA bundle backup
	"/etc/ssl/certs/ca-certificates.crt",
	// The speaker's own persisted source list. The firmware registers source
	// types from this file at boot; comparing it against the service's
	// Sources.xml (and the runtime /sources) is the key evidence when radio
	// source types won't activate after an in-place migration.
	speaker.SourcesFileLocation,
	// Redirection-relevant state: where the speaker resolves Bose hostnames
	// and what it had before migration. (The live marge/BMX URL config in
	// SoundTouchSdkPrivateCfg.xml is handled by collectSpeakerRedirectConfig,
	// which also parses it and has a telnet fallback; its .original backup is
	// a plain raw pull here, showing the pre-migration Bose URLs.)
	"/opt/Bose/etc/SoundTouchSdkPrivateCfg.xml.original", // pre-migration URL config backup
	"/etc/hosts",                // deprecated hosts-migration redirect or manual edits
	"/etc/hosts.original",       // factory backup, if the hosts method was ever used
	"/etc/resolv.conf",          // which DNS the speaker actually queries
	"/etc/resolv.conf.original", // pre-migration resolv backup (resolv method)
	"/mnt/nv/soundtouch-service/aftertouch.resolv.conf", // resolv-method nameserver hook
	"/mnt/nv/aftertouch.resolv.conf",                    // legacy resolv-hook path
	// resolv method's live redirection mechanism: a boot hook plus DHCP-renew
	// hooks that prepend our nameserver. Whether these are intact decides if
	// the speaker resolves Bose hosts to AfterTouch or escapes to the real
	// cloud — written under a root remount, so persistent on the rootfs.
	"/mnt/nv/rc.local",        // boot hook installing the resolv override
	"/etc/udhcpc.d/50default", // DHCP-renew hook (primary)
	"/opt/Bose/udhcpc.script", // DHCP-renew hook (alternate, if present)
	"/etc/remote_services",    // SSH-enablement marker (preferred location)
	"/mnt/nv/remote_services", // SSH-enablement marker (NV fallback)
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
				log.Printf("[Export] SSH %s %s: %v", sanitizeLog(id), remotePath, err)

				continue
			}

			archivePath := "ssh/speaker/" + id + remotePath
			if err := addTarBytes(tw, archivePath, data); err != nil {
				log.Printf("[Export] add %s: %v", sanitizeLog(archivePath), err)
			}
		}

		// dmesg and any other plain commands.
		// firewall.txt: iptables-save yields the active filter on FW 27.0.6;
		// ip6tables-save typically yields nothing (no error) but is harmless
		// and future-proofs IPv6-capable builds. Catches a self-inflicted
		// DROP of speaker↔service or speaker↔Bose-host traffic.
		for filename, cmd := range map[string]string{
			"dmesg.txt":    "dmesg",
			"firewall.txt": "iptables-save 2>/dev/null; ip6tables-save 2>/dev/null",
		} {
			out, err := sc.Run(cmd)
			if err != nil && strings.TrimSpace(out) == "" {
				log.Printf("[Export] SSH %s %q: %v", sanitizeLog(id), cmd, err)

				continue
			}

			archivePath := "ssh/speaker/" + id + "/" + filename
			if err := addTarBytes(tw, archivePath, []byte(out)); err != nil {
				log.Printf("[Export] add %s: %v", sanitizeLog(archivePath), err)
			}
		}

		// Syslog: fetch raw, strip 127.0.0.1 noise, then keep only the last speakerLogWindow.
		rawLog, logErr := sc.Run("logread 2>/dev/null | grep -v '127.0.0.1'")
		if logErr != nil && strings.TrimSpace(rawLog) == "" {
			log.Printf("[Export] SSH %s logread: %v", sanitizeLog(id), logErr)
		} else {
			filtered := filterSpeakerLog(rawLog, speakerLogWindow)
			archivePath := "ssh/speaker/" + id + "/logread.txt"

			if addErr := addTarBytes(tw, archivePath, []byte(filtered)); addErr != nil {
				log.Printf("[Export] add %s: %v", sanitizeLog(archivePath), addErr)
			}
		}
	}
}

// sshReachable reports whether the speaker accepts a TCP connection on the SSH
// port. Used only to classify provenance: a speaker that answers telnet but
// not SSH was, by elimination, migrated via the telnet method.
func sshReachable(ip string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "22"), time.Second)
	if err != nil {
		return false
	}

	_ = conn.Close()

	return true
}

// readTelnetSystemConfig reads the live URL set from the speaker's diagnostic
// shell (port 17000) via `getpdo CurrentSystemConfiguration`. Returns the raw
// reply and true on success.
func readTelnetSystemConfig(ip string) (string, bool) {
	t := telnet.NewClient(ip)
	if err := t.Dial(); err != nil {
		return "", false
	}

	defer func() { _ = t.Close() }()

	_, _ = t.Probe()

	resp, err := t.SendCommand("getpdo CurrentSystemConfiguration")
	if err != nil || strings.TrimSpace(resp) == "" {
		return "", false
	}

	return resp, true
}

// collectSpeakerRedirectConfig determines, per speaker, where it is configured
// to send marge/BMX/streaming traffic — the data that decides whether a
// TuneIn/RadioBrowser select reaches AfterTouch or escapes to the dead Bose
// cloud. It prefers the persisted SoundTouchSdkPrivateCfg.xml over SSH; when
// SSH is unavailable it falls back to `getpdo CurrentSystemConfiguration` over
// telnet (the same channel the telnet migration uses). Raw artifacts are
// written into the archive; parsed URLs + provenance are returned keyed by
// device ID for inclusion in diagnostic.json.
func (s *Server) collectSpeakerRedirectConfig(tw *tar.Writer) map[string]*redirectConfig {
	out := map[string]*redirectConfig{}

	devices, err := s.ds.ListAllDevices()
	if err != nil {
		log.Printf("[Export] redirect config: list devices: %v", err)

		return out
	}

	for i := range devices {
		dev := &devices[i]
		if dev.IPAddress == "" {
			continue
		}

		id := dev.DeviceID
		if id == "" {
			id = dev.IPAddress
		}

		rc := &redirectConfig{Source: "none", SSHReachable: sshReachable(dev.IPAddress)}

		// 1. Prefer the persisted XML over SSH.
		sc := speakerssh.NewClient(dev.IPAddress)
		if data, rerr := sc.ReadFile(setup.SoundTouchSdkPrivateCfgPath); rerr == nil {
			archivePath := "ssh/speaker/" + id + setup.SoundTouchSdkPrivateCfgPath
			if aerr := addTarBytes(tw, archivePath, data); aerr != nil {
				log.Printf("[Export] add %s: %v", sanitizeLog(archivePath), aerr)
			}

			var cfg setup.PrivateCfg
			if xerr := xml.Unmarshal(data, &cfg); xerr == nil {
				rc.Source = "ssh"
				rc.MargeServerURL = cfg.MargeServerUrl
				rc.StatsServerURL = cfg.StatsServerUrl
				rc.SwUpdateURL = cfg.SwUpdateUrl
				rc.BmxRegistryURL = cfg.BmxRegistryUrl
			} else {
				log.Printf("[Export] parse %s for %s: %v", setup.SoundTouchSdkPrivateCfgPath, sanitizeLog(id), xerr)
			}
		}

		// 2. Fall back to telnet getpdo when SSH gave us nothing.
		if rc.Source == "none" {
			if raw, ok := readTelnetSystemConfig(dev.IPAddress); ok {
				archivePath := "telnet/speaker/" + id + "/system-configuration.txt"
				if aerr := addTarBytes(tw, archivePath, []byte(raw)); aerr != nil {
					log.Printf("[Export] add %s: %v", sanitizeLog(archivePath), aerr)
				}

				if fields := setup.ParseGetpdoConfig(raw); len(fields) > 0 {
					rc.Source = "telnet"
					rc.MargeServerURL = fields["margeServerUrl"]
					rc.StatsServerURL = fields["statsServerUrl"]
					rc.SwUpdateURL = fields["swUpdateUrl"]
					rc.BmxRegistryURL = fields["bmxRegistryUrl"]

					// SSH-free reachability implies the telnet migration method
					// (xml/hosts/resolv all require SSH).
					if !rc.SSHReachable {
						rc.InferredMigrationMethod = "telnet"
					}
				}
			}
		}

		out[dev.DeviceID] = rc
	}

	return out
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
	ServerURL              string         `json:"server_url"`
	HTTPSServerURL         string         `json:"https_server_url,omitempty"`
	HTTPSServerURLOverride string         `json:"https_server_url_override,omitempty"`
	RedactLogs             bool           `json:"redact_logs"`
	LogBodies              bool           `json:"log_bodies"`
	RecordInteractions     bool           `json:"record_interactions"`
	DiscoveryInterval      string         `json:"discovery_interval,omitempty"`
	DiscoveryEnabled       bool           `json:"discovery_enabled"`
	DNSEnabled             bool           `json:"dns_enabled"`
	DNSUpstream            []string       `json:"dns_upstream,omitempty"`
	DNSBindAddr            string         `json:"dns_bind_addr,omitempty"`
	InternalPaths          []string       `json:"internal_paths,omitempty"`
	Shortcuts              map[string]int `json:"shortcuts,omitempty"`
	SpotifyClientID        string         `json:"spotify_client_id,omitempty"`
	SpotifyClientSecret    string         `json:"spotify_client_secret,omitempty"`
	SpotifyRedirectURI     string         `json:"spotify_redirect_uri,omitempty"`
	AmazonClientID         string         `json:"amazon_client_id,omitempty"`
	AmazonClientSecret     string         `json:"amazon_client_secret,omitempty"`
	AmazonRedirectURI      string         `json:"amazon_redirect_uri,omitempty"`
	TrustForwardedHeaders  bool           `json:"trust_forwarded_headers,omitempty"`
	TrustedProxyCIDRs      []string       `json:"trusted_proxy_cidrs,omitempty"`
	TuneInStreamFormats    string         `json:"tunein_stream_formats,omitempty"`
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

	// st.HTTPServerURL is the HTTPS override (empty = derive). Report the
	// effective URL actually in use plus the override, so a diagnostic
	// makes an advertised-URL/listener mismatch (issue #355) legible
	// instead of showing an empty field when the URL is derived.
	_, effectiveHTTPSURL := s.GetSettings()

	ds := diagSettings{
		ServerURL:              st.ServerURL,
		HTTPSServerURL:         effectiveHTTPSURL,
		HTTPSServerURLOverride: st.HTTPServerURL,
		RedactLogs:             st.RedactLogs,
		LogBodies:              st.LogBodies,
		RecordInteractions:     st.RecordInteractions,
		DiscoveryInterval:      st.DiscoveryInterval,
		DiscoveryEnabled:       st.DiscoveryEnabled,
		DNSEnabled:             st.DNSEnabled,
		DNSUpstream:            st.DNSUpstream,
		DNSBindAddr:            st.DNSBindAddr,
		InternalPaths:          st.InternalPaths,
		Shortcuts:              st.Shortcuts,
		SpotifyClientID:        st.SpotifyClientID,
		SpotifyClientSecret:    redact(st.SpotifyClientSecret),
		SpotifyRedirectURI:     st.SpotifyRedirectURI,
		AmazonClientID:         st.AmazonClientID,
		AmazonClientSecret:     redact(st.AmazonClientSecret),
		AmazonRedirectURI:      st.AmazonRedirectURI,
		TrustForwardedHeaders:  st.TrustForwardedHeaders,
		TrustedProxyCIDRs:      st.TrustedProxyCIDRs,
		TuneInStreamFormats:    st.TuneInStreamFormats,
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

func (s *Server) buildDiagnosticReport(redirectCfgs map[string]*redirectConfig) diagnosticReport {
	report := diagnosticReport{
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339),
		ServiceVersion:      buildVersionInfo(),
		DeprecatedRouteHits: s.DeprecatedRouteHits(),
	}

	if s.healthRegistry != nil {
		report.HealthChecks = s.runHealthChecks()
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

		dd.RedirectConfig = redirectCfgs[dev.DeviceID]

		report.Devices = append(report.Devices, dd)
	}

	return report
}
