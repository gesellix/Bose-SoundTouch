// Package setup contains speaker migration and configuration helpers.
package setup

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"

	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/ssh"
	"github.com/gesellix/bose-soundtouch/pkg/telnet"
)

// MigrationMethod represents the method used to migrate a speaker.
type MigrationMethod string

const (
	// MigrationMethodXML redirects services by modifying SoundTouchSdkPrivateCfg.xml.
	MigrationMethodXML MigrationMethod = "xml"
	// MigrationMethodHosts redirects services by modifying /etc/hosts and updating the CA trust store.
	MigrationMethodHosts MigrationMethod = "hosts"
	// MigrationMethodResolvConf redirects services by injecting a priority DNS hook into the DHCP logic and updating the CA trust store.
	MigrationMethodResolvConf MigrationMethod = "resolv"
	// MigrationMethodTelnet redirects services by driving the device's diagnostic
	// shell on TCP port 17000. Requires no SSH access on the device.
	MigrationMethodTelnet MigrationMethod = "telnet"
)

// SoundTouchSdkPrivateCfgPath is the path to the speaker's private configuration file on device.
const SoundTouchSdkPrivateCfgPath = "/opt/Bose/etc/SoundTouchSdkPrivateCfg.xml"

// PrivateCfg represents the SoundTouchSdkPrivateCfg XML structure.
type PrivateCfg struct {
	XMLName                    xml.Name `xml:"SoundTouchSdkPrivateCfg" json:"-"`
	MargeServerUrl             string   `xml:"margeServerUrl" json:"margeServerUrl"`
	StatsServerUrl             string   `xml:"statsServerUrl" json:"statsServerUrl"`
	SwUpdateUrl                string   `xml:"swUpdateUrl" json:"swUpdateUrl"`
	UsePandoraProductionServer bool     `xml:"usePandoraProductionServer" json:"usePandoraProductionServer"`
	IsZeroconfEnabled          bool     `xml:"isZeroconfEnabled" json:"isZeroconfEnabled"`
	SaveMargeCustomerReport    bool     `xml:"saveMargeCustomerReport" json:"saveMargeCustomerReport"`
	BmxRegistryUrl             string   `xml:"bmxRegistryUrl" json:"bmxRegistryUrl"`
}

// MigrationSummary provides details about the state of a speaker before migration.
type MigrationSummary struct {
	SSHSuccess               bool        `json:"ssh_success"`
	CurrentConfig            string      `json:"current_config"`
	PlannedConfig            string      `json:"planned_config"`
	OriginalConfig           string      `json:"original_config,omitempty"`
	ParsedCurrentConfig      *PrivateCfg `json:"parsed_current_config,omitempty"`
	PlannedHosts             string      `json:"planned_hosts,omitempty"`
	RemoteServicesEnabled    bool        `json:"remote_services_enabled"`
	RemoteServicesPersistent bool        `json:"remote_services_persistent"`
	RemoteServicesFound      []string    `json:"remote_services_found"`
	RemoteServicesCheckErr   string      `json:"remote_services_check_err,omitempty"`
	DeviceName               string      `json:"device_name,omitempty"`
	DeviceModel              string      `json:"device_model,omitempty"`
	DeviceSerial             string      `json:"device_serial,omitempty"`
	DeviceID                 string      `json:"device_id,omitempty"`
	AccountID                string      `json:"account_id,omitempty"`
	FirmwareVersion          string      `json:"firmware_version,omitempty"`
	CACertTrusted            bool        `json:"ca_cert_trusted"`
	ServerHTTPSURL           string      `json:"server_https_url,omitempty"`
	CurrentResolvConf        string      `json:"current_resolv_conf,omitempty"`
	PlannedResolv            string      `json:"planned_resolv,omitempty"`
	IsMigrated               bool        `json:"is_migrated"`
	ResolveIPError           string      `json:"resolve_ip_error,omitempty"`
	MirrorEnabled            bool        `json:"mirror_enabled"`
	MirrorEndpoints          []string    `json:"mirror_endpoints,omitempty"`
	SkipMirrorEndpoints      []string    `json:"skip_mirror_endpoints,omitempty"`
	PreferredSource          string      `json:"preferred_source,omitempty"`

	// Telnet (port 17000) preflight state — populated when the user is about to
	// or has just used MigrationMethodTelnet.
	TelnetReachable      bool   `json:"telnet_reachable"`
	TelnetBanner         string `json:"telnet_banner,omitempty"`
	TelnetVerifiedConfig string `json:"telnet_verified_config,omitempty"`
	TelnetProbeError     string `json:"telnet_probe_error,omitempty"`

	// KnownAccountIDs are accountIDs already present in the local datastore;
	// the UI offers them as choices when pairing a fresh device.
	KnownAccountIDs []string `json:"known_account_ids,omitempty"`
}

// SSHClient defines the interface for SSH operations.
type SSHClient interface {
	Run(command string) (string, error)
	UploadContent(content []byte, remotePath string) error
}

// TelnetClient defines the interface for the device's port-17000 diagnostic
// shell. The concrete implementation lives in github.com/gesellix/bose-soundtouch/pkg/telnet;
// the interface exists so tests can substitute a mock.
type TelnetClient interface {
	Dial() error
	Probe() (string, error)
	SendCommand(cmd string) (string, error)
	Close() error
}

// Manager handles the migration of speakers to the service.
type Manager struct {
	ServerURL string
	DataStore *datastore.DataStore
	Crypto    *certmanager.CertificateManager
	NewSSH    func(host string) SSHClient
	NewTelnet func(host string) TelnetClient

	// GetDNSRunning is an optional callback to check the actual state of the DNS server.
	GetDNSRunning func() (bool, string)

	// HTTPGet is an optional override for http.Get (primarily for testing).
	HTTPGet func(url string) (*http.Response, error)

	// Spotify management credentials for the boot primer
	MgmtUsername string
	MgmtPassword string
}

// NewManager creates a new Manager with the given base server URL.
func NewManager(serverURL string, ds *datastore.DataStore, cm *certmanager.CertificateManager) *Manager {
	return &Manager{
		ServerURL: serverURL,
		DataStore: ds,
		Crypto:    cm,
		NewSSH: func(host string) SSHClient {
			return ssh.NewClient(host)
		},
		NewTelnet: func(host string) TelnetClient {
			return telnet.NewClient(host)
		},
		HTTPGet:      http.Get,
		MgmtUsername: "admin",
		MgmtPassword: "change_me!",
	}
}

// DeviceInfoXML represents the XML structure from :8090/info
type DeviceInfoXML struct {
	XMLName          xml.Name `xml:"info" json:"-"`
	DeviceID         string   `xml:"deviceID,attr" json:"deviceID"`
	Name             string   `xml:"name" json:"name"`
	Type             string   `xml:"type" json:"type"`
	ModuleType       string   `xml:"moduleType" json:"moduleType"`
	MargeAccountUUID string   `xml:"margeAccountUUID" json:"margeAccountUUID"`
	MargeURL         string   `xml:"margeURL" json:"margeURL"`
	CountryCode      string   `xml:"countryCode" json:"countryCode"`
	RegionCode       string   `xml:"regionCode" json:"regionCode"`
	Variant          string   `xml:"variant" json:"variant"`
	VariantMode      string   `xml:"variantMode" json:"variantMode"`
	Components       []struct {
		Category        string `xml:"componentCategory"`
		SoftwareVersion string `xml:"softwareVersion"`
		SerialNumber    string `xml:"serialNumber"`
	} `xml:"components>component" json:"-"`
	NetworkInfo []struct {
		Type       string `xml:"type,attr"`
		MacAddress string `xml:"macAddress"`
		IPAddress  string `xml:"ipAddress"`
	} `xml:"networkInfo" json:"networkInfo"`
	SoftwareVer  string `xml:"-" json:"softwareVersion"`
	SerialNumber string `xml:"-" json:"serialNumber"`
}

// GetLiveDeviceInfo fetches live information from the speaker's :8090/info endpoint.
func (m *Manager) GetLiveDeviceInfo(deviceIP string) (*DeviceInfoXML, error) {
	infoURL := fmt.Sprintf("http://%s:8090/info", deviceIP)
	// For testing, if the IP already contains a port, don't append :8090
	if host, _, err := net.SplitHostPort(deviceIP); err == nil {
		infoURL = fmt.Sprintf("http://%s/info", deviceIP)
		_ = host
	}

	resp, err := m.HTTPGet(infoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch info from %s: %w", infoURL, err)
	}

	defer func() { _ = resp.Body.Close() }()

	var infoXML DeviceInfoXML
	if err := m.parseDeviceInfoXML(resp.Body, &infoXML); err != nil {
		return nil, fmt.Errorf("failed to decode info XML from %s: %w", infoURL, err)
	}

	return &infoXML, nil
}

// parseDeviceInfoXML is a helper method for parsing device info XML from a reader
func (m *Manager) parseDeviceInfoXML(reader io.Reader, infoXML *DeviceInfoXML) error {
	if err := xml.NewDecoder(reader).Decode(infoXML); err != nil {
		return err
	}

	// Extract data from components
	for _, comp := range infoXML.Components {
		switch comp.Category {
		case "SCM":
			infoXML.SoftwareVer = comp.SoftwareVersion
			if infoXML.SerialNumber == "" {
				infoXML.SerialNumber = comp.SerialNumber
			}
		case "PackagedProduct":
			if infoXML.SerialNumber == "" {
				infoXML.SerialNumber = comp.SerialNumber
			}
		}
	}

	return nil
}

// GetPrimaryMacAddress returns the primary MAC address from the SCM network interface.
func (d *DeviceInfoXML) GetPrimaryMacAddress() string {
	for _, net := range d.NetworkInfo {
		if net.Type == "SCM" && net.MacAddress != "" {
			return net.MacAddress
		}
	}

	return ""
}

// GetMigrationSummary returns a summary of the current and planned state of the speaker.
func (m *Manager) GetMigrationSummary(deviceIP, targetURL, proxyURL string, options map[string]string) (*MigrationSummary, error) {
	if targetURL == "" {
		targetURL = m.ServerURL
	}

	summary := &MigrationSummary{
		SSHSuccess: false,
	}

	// Populate device info from datastore and live info
	m.populateDeviceInfo(summary, deviceIP)

	// 1. Initial planned config
	plannedCfg := PrivateCfg{
		MargeServerUrl:             targetURL,
		StatsServerUrl:             targetURL,
		SwUpdateUrl:                fmt.Sprintf("%s/updates/soundtouch", targetURL),
		UsePandoraProductionServer: true,
		IsZeroconfEnabled:          true,
		SaveMargeCustomerReport:    false,
		BmxRegistryUrl:             fmt.Sprintf("%s/bmx/registry/v1/services", targetURL),
	}

	// 2. Check SSH and read current config
	currentConfig, err := m.checkCurrentConfig(summary, deviceIP)
	if err == nil && currentConfig != "" {
		summary.CurrentConfig = currentConfig
		fmt.Printf("Current config from %s (length: %d):\n%q\n", deviceIP, len(currentConfig), currentConfig)

		// Parse current config
		var currentCfg PrivateCfg
		if xml.Unmarshal([]byte(currentConfig), &currentCfg) == nil {
			summary.ParsedCurrentConfig = &currentCfg

			if proxyURL == "" {
				proxyURL = targetURL
			}

			// Apply options if provided
			if options != nil {
				m.applyProxyOptions(&plannedCfg, proxyURL, options, &currentCfg)
			}
		}
	}
	// Note: CurrentConfig is set by checkCurrentConfig in all cases (success or failure)

	xmlContent, err := xml.MarshalIndent(plannedCfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal planned XML: %w", err)
	}

	summary.PlannedConfig = "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" + string(xmlContent)

	// 2b. Planned network config (hosts entries, resolv.conf preview, resolve error)
	m.populatePlannedNetworkConfig(summary, deviceIP, targetURL)

	// 3. Check for remote services files
	m.checkRemoteServices(summary, deviceIP)

	// 4. Check if CA certificate is trusted
	m.checkCACertTrusted(summary, deviceIP)

	// 4b. Check current /etc/resolv.conf
	if summary.SSHSuccess {
		client := m.NewSSH(deviceIP)
		if resolvConf, err := client.Run("cat /etc/resolv.conf"); err == nil {
			summary.CurrentResolvConf = resolvConf
		}
	}

	// 5. Provide HTTPS URL for testing
	summary.ServerHTTPSURL = m.buildServerHTTPSURL(targetURL)

	// 6. Check if migrated
	m.checkIsMigrated(summary, deviceIP)

	// 7. Mirroring settings
	if m.DataStore != nil {
		settings, err := m.DataStore.GetSettings()
		if err == nil {
			summary.MirrorEnabled = settings.MirrorEnabled
			summary.MirrorEndpoints = settings.MirrorEndpoints
			summary.SkipMirrorEndpoints = settings.SkipMirrorEndpoints
			summary.PreferredSource = settings.PreferredSource
		}
	}

	return summary, nil
}

func (m *Manager) populatePlannedNetworkConfig(summary *MigrationSummary, deviceIP, targetURL string) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return
	}

	hostName := parsedURL.Hostname()
	if hostName == "" || hostName == "localhost" {
		return
	}

	client := m.NewSSH(deviceIP)

	hostIP, resolveErr := m.resolveIP(hostName, client)
	if resolveErr != nil {
		summary.ResolveIPError = resolveErr.Error()
	}

	if hostIP == "" {
		hostIP = hostName
	}

	summary.PlannedResolv = fmt.Sprintf("# Created by Aftertouch/SoundTouch-Service\n# Priority nameserver for Bose service redirection\nnameserver %s\n", hostIP)

	domains := []string{
		"streaming.bose.com",
		"updates.bose.com",
		"stats.bose.com",
		"bmx.bose.com",
		"content.api.bose.io",
		"events.api.bosecm.com",
		"bose-prod.apigee.net",
		"worldwide.bose.com",
		"music.api.bose.com",
		"media.bose.io",
		"downloads.bose.com",
		"voice.api.bose.io",
	}

	hostsLines := make([]string, len(domains))
	for i, domain := range domains {
		hostsLines[i] = fmt.Sprintf("%s\t%s", hostIP, domain)
	}

	summary.PlannedHosts = strings.Join(hostsLines, "\n")
}

func (m *Manager) buildServerHTTPSURL(targetURL string) string {
	parsedURL, err := url.Parse(targetURL)
	if err != nil || parsedURL.Hostname() == "" {
		return ""
	}

	httpsPort := os.Getenv("HTTPS_PORT")
	if httpsPort == "" {
		httpsPort = "8443"
	}

	return fmt.Sprintf("https://%s:%s/health", parsedURL.Hostname(), httpsPort)
}

// checkIsMigrated determines if the device is already migrated to AfterTouch.
func (m *Manager) checkIsMigrated(summary *MigrationSummary, deviceIP string) {
	if !summary.SSHSuccess {
		return
	}

	client := m.NewSSH(deviceIP)

	if m.isXMLMigrated(summary) || m.isHostsMigrated(client, summary) || m.isResolvConfMigrated(client, summary) {
		summary.IsMigrated = true
	}
}

// isXMLMigrated checks whether current XML config already points to our server.
func (m *Manager) isXMLMigrated(summary *MigrationSummary) bool {
	if summary.ParsedCurrentConfig == nil {
		return false
	}

	parsedTarget, err := url.Parse(m.ServerURL)
	if err != nil {
		return false
	}

	targetHost := parsedTarget.Hostname()

	return strings.Contains(summary.ParsedCurrentConfig.MargeServerUrl, targetHost) ||
		strings.Contains(summary.ParsedCurrentConfig.StatsServerUrl, targetHost) ||
		strings.Contains(summary.ParsedCurrentConfig.SwUpdateUrl, targetHost) ||
		strings.Contains(summary.ParsedCurrentConfig.BmxRegistryUrl, targetHost)
}

// isHostsMigrated checks if /etc/hosts contains Bose domain redirections and CA is trusted.
func (m *Manager) isHostsMigrated(client SSHClient, summary *MigrationSummary) bool {
	hostsContent, err := client.Run("cat /etc/hosts")
	if err != nil {
		return false
	}

	boseDomains := []string{
		"streaming.bose.com",
		"updates.bose.com",
		"stats.bose.com",
		"bmx.bose.com",
	}
	for _, domain := range boseDomains {
		if strings.Contains(hostsContent, domain) && summary.CACertTrusted {
			return true
		}
	}

	return false
}

// isResolvConfMigrated checks for Aftertouch DNS migration signals and CA trust.
func (m *Manager) isResolvConfMigrated(client SSHClient, summary *MigrationSummary) bool {
	// Hook file present
	if _, err := client.Run("[ -f /mnt/nv/aftertouch.resolv.conf ]"); err == nil {
		return summary.CACertTrusted
	}

	if summary.CurrentResolvConf == "" {
		return false
	}

	// Marker comment present
	if strings.Contains(summary.CurrentResolvConf, "# Priority nameserver for Bose service redirection") && summary.CACertTrusted {
		return true
	}

	// Match hostname or resolved IP
	parsedTarget, err := url.Parse(m.ServerURL)
	if err != nil {
		return false
	}

	targetHost := parsedTarget.Hostname()
	if strings.Contains(summary.CurrentResolvConf, targetHost) && summary.CACertTrusted {
		return true
	}

	resolvedIP, _ := m.resolveIP(targetHost, client)
	if resolvedIP != "" && strings.Contains(summary.CurrentResolvConf, resolvedIP) && summary.CACertTrusted {
		return true
	}

	return false
}

// populateDeviceInfo fills in device information from datastore and live info
func (m *Manager) populateDeviceInfo(summary *MigrationSummary, deviceIP string) {
	// Populate from datastore if available
	if m.DataStore != nil {
		devices, err := m.DataStore.ListAllDevices()
		if err == nil {
			for i := range devices {
				d := devices[i]
				if d.IPAddress != deviceIP {
					continue
				}

				summary.DeviceName = d.Name
				summary.DeviceModel = d.ProductCode
				summary.DeviceSerial = d.DeviceSerialNumber
				summary.DeviceID = d.DeviceID
				summary.AccountID = d.AccountID
				summary.FirmwareVersion = d.FirmwareVersion

				break
			}
		}
	}

	// Supplement with live info from :8090/info
	if infoXML, err := m.GetLiveDeviceInfo(deviceIP); err == nil {
		if infoXML.Name != "" {
			summary.DeviceName = infoXML.Name
		}

		if infoXML.Type != "" {
			summary.DeviceModel = infoXML.Type
		}

		if infoXML.SerialNumber != "" {
			summary.DeviceSerial = infoXML.SerialNumber
		}

		if infoXML.SoftwareVer != "" {
			summary.FirmwareVersion = infoXML.SoftwareVer
		}

		if infoXML.DeviceID != "" {
			summary.DeviceID = infoXML.DeviceID
		}

		if infoXML.MargeAccountUUID != "" {
			summary.AccountID = infoXML.MargeAccountUUID
		}
	}
}

// checkCurrentConfig reads and validates the current speaker configuration
func (m *Manager) checkCurrentConfig(summary *MigrationSummary, deviceIP string) (string, error) {
	path := SoundTouchSdkPrivateCfgPath
	client := m.NewSSH(deviceIP)

	// Check if .original exists
	if _, checkErr := client.Run(fmt.Sprintf("[ -f %s.original ]", path)); checkErr == nil {
		if originalConfig, _ := client.Run(fmt.Sprintf("cat %s.original", path)); originalConfig != "" {
			summary.OriginalConfig = originalConfig
		}
	}

	// Try to read current config
	config, err := client.Run(fmt.Sprintf("cat %s", path))
	if err == nil && config != "" {
		summary.SSHSuccess = true
		return config, nil
	}

	// Fallback: try base64 if cat returned empty string but file has size > 0
	if config == "" {
		if fileInfo, _ := client.Run(fmt.Sprintf("ls -l %s", path)); fileInfo != "" {
			if b64Config, configErr := client.Run(fmt.Sprintf("base64 %s", path)); configErr == nil && b64Config != "" {
				// File exists but couldn't read content properly
				summary.SSHSuccess = true
				summary.CurrentConfig = fmt.Sprintf("Error reading config: %v", err)

				return "", fmt.Errorf("config file exists but couldn't read content")
			}
		}
	}

	// If SSH failed or file couldn't be read, check if SSH connection works at all
	if _, sshErr := client.Run("ls /"); sshErr == nil {
		summary.SSHSuccess = true
		if err != nil {
			summary.CurrentConfig = fmt.Sprintf("Error reading config: %v", err)
		} else {
			summary.CurrentConfig = config // Might be empty
		}
	} else {
		summary.SSHSuccess = false
		summary.CurrentConfig = fmt.Sprintf("SSH connection failed: %v", sshErr)
	}

	return "", err
}

// applyProxyOptions modifies planned config based on proxy options.
// Each field accepts: "proxied" (route through proxyURL), "original" (keep current value), or unset (use targetURL via plannedCfg default).
func (m *Manager) applyProxyOptions(plannedCfg *PrivateCfg, proxyURL string, options map[string]string, currentCfg *PrivateCfg) {
	if currentCfg == nil {
		return
	}

	if options["marge"] == "proxied" && currentCfg.MargeServerUrl != "" {
		plannedCfg.MargeServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.MargeServerUrl)
	} else if options["marge"] == "original" && currentCfg.MargeServerUrl != "" {
		plannedCfg.MargeServerUrl = currentCfg.MargeServerUrl
	}

	if options["stats"] == "proxied" && currentCfg.StatsServerUrl != "" {
		plannedCfg.StatsServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.StatsServerUrl)
	} else if options["stats"] == "original" && currentCfg.StatsServerUrl != "" {
		plannedCfg.StatsServerUrl = currentCfg.StatsServerUrl
	}

	if options["sw_update"] == "proxied" && currentCfg.SwUpdateUrl != "" {
		plannedCfg.SwUpdateUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.SwUpdateUrl)
	} else if options["sw_update"] == "original" && currentCfg.SwUpdateUrl != "" {
		plannedCfg.SwUpdateUrl = currentCfg.SwUpdateUrl
	}

	if options["bmx"] == "proxied" && currentCfg.BmxRegistryUrl != "" {
		plannedCfg.BmxRegistryUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.BmxRegistryUrl)
	} else if options["bmx"] == "original" && currentCfg.BmxRegistryUrl != "" {
		plannedCfg.BmxRegistryUrl = currentCfg.BmxRegistryUrl
	}
}

// checkRemoteServices checks for remote services files on the device
func (m *Manager) checkRemoteServices(summary *MigrationSummary, deviceIP string) {
	client := m.NewSSH(deviceIP)
	locations := []string{
		"/etc/remote_services",
		"/mnt/nv/remote_services",
		"/tmp/remote_services",
	}

	for _, loc := range locations {
		if _, err := client.Run(fmt.Sprintf("[ -e %s ]", loc)); err == nil {
			summary.RemoteServicesFound = append(summary.RemoteServicesFound, loc)

			summary.RemoteServicesEnabled = true
			if loc != "/tmp/remote_services" {
				summary.RemoteServicesPersistent = true
			}
		}
	}
}

// checkCACertTrusted checks if the local CA certificate is already in the device's trust store.
func (m *Manager) checkCACertTrusted(summary *MigrationSummary, deviceIP string) {
	if m.Crypto == nil {
		return
	}

	client := m.NewSSH(deviceIP)
	bundlePath := "/etc/pki/tls/certs/ca-bundle.crt"

	// First, check for the label
	output, err := client.Run(fmt.Sprintf("grep -F %q %s", CALabel, bundlePath))
	if err == nil && strings.Contains(output, CALabel) {
		summary.CACertTrusted = true
		return
	}

	caCertPEM, err := os.ReadFile(m.Crypto.GetCACertPath())
	if err != nil {
		return
	}

	// We look for the first part of the certificate (e.g. the first 64 chars of the base64 data)
	// to see if it's already in the bundle.
	lines := strings.Split(string(caCertPEM), "\n")

	var certData string

	for _, line := range lines {
		if !strings.Contains(line, "BEGIN CERTIFICATE") && !strings.Contains(line, "END CERTIFICATE") && line != "" {
			certData = line
			break
		}
	}

	if certData == "" {
		return
	}

	// Use grep to check for the certificate data in the bundle
	_, err = client.Run(fmt.Sprintf("grep -F %q %s", certData, bundlePath))
	if err == nil {
		summary.CACertTrusted = true
	}
}

// MigrateSpeaker configures the speaker at the given IP to use this service.
func (m *Manager) MigrateSpeaker(deviceIP, targetURL, proxyURL string, options map[string]string, method MigrationMethod) (string, error) {
	if targetURL == "" {
		targetURL = m.ServerURL
	}

	if method == "" {
		method = MigrationMethodXML
	}

	// Telnet is SSH-free by design — skip the SSH-based off-device backup and
	// rw pre-flight, both of which would fail on devices that haven't been
	// rooted via remote_services.
	if method == MigrationMethodTelnet {
		return m.migrateViaTelnet(deviceIP, targetURL)
	}

	var logs string

	// 0. Off-device backup for safety
	if backupErr := m.BackupConfigOffDevice(deviceIP); backupErr != nil {
		logs += fmt.Sprintf("Warning: Failed to create off-device backup: %v\n", backupErr)
		// We continue, but this is a warning
	} else {
		logs += "Successfully created off-device backup of current configuration.\n"
	}

	// 0b. Pre-flight check for SSH /rw permissions
	client := m.NewSSH(deviceIP)

	rwCmd := "(rw || mount -o remount,rw /)"
	if rwTest, rwErr := client.Run(rwCmd); rwErr != nil {
		return logs, fmt.Errorf("pre-flight check failed: cannot gain write access (cmd: %s, output: %s): %w", rwCmd, rwTest, rwErr)
	}

	logs += "Pre-flight: Write access verified.\n"

	switch method {
	case MigrationMethodHosts:
		out, err := m.migrateViaHosts(deviceIP, targetURL)
		return logs + out, err

	case MigrationMethodResolvConf:
		if err := m.checkDNSPreFlight(); err != nil {
			return logs, err
		}

		out, err := m.migrateViaResolvConf(deviceIP, targetURL)

		return logs + out, err

	case MigrationMethodXML:
		out, err := m.migrateViaXML(deviceIP, targetURL, proxyURL, options, client, rwCmd)
		return logs + out, err

	default:
		return logs, fmt.Errorf("unsupported migration method: %s", method)
	}
}

func (m *Manager) checkDNSPreFlight() error {
	// Pre-flight check: DNS server must be enabled and bound to port 53
	settings, err := m.DataStore.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to retrieve settings: %w", err)
	}

	if !settings.DNSEnabled {
		return fmt.Errorf("DNS discovery server is not enabled. Please enable it in Settings before using /etc/resolv.conf migration")
	}

	if !strings.HasSuffix(settings.DNSBindAddr, ":53") && settings.DNSBindAddr != "53" {
		return fmt.Errorf("DNS discovery server is bound to %s, but port 53 is required for /etc/resolv.conf migration", settings.DNSBindAddr)
	}

	// Also check the actual running state if callback is available
	if m.GetDNSRunning != nil {
		isRunning, bindAddr := m.GetDNSRunning()
		if !isRunning {
			return fmt.Errorf("DNS discovery server is configured but not actually running on %s. Please check logs for binding errors", bindAddr)
		}

		if !strings.HasSuffix(bindAddr, ":53") && bindAddr != "53" {
			// This shouldn't happen based on previous check, but for completeness
			return fmt.Errorf("DNS discovery server is running on %s, but port 53 is required", bindAddr)
		}
	}

	return nil
}

func (m *Manager) migrateViaXML(deviceIP, targetURL, proxyURL string, options map[string]string, client SSHClient, rwCmd string) (string, error) {
	var logs string

	out, err := m.EnsureRemoteServices(deviceIP)
	logs += "Ensuring remote services:\n" + out + "\n"

	if err != nil {
		// Log but continue migration? Or fail? The requirement is "to ensure stable 'remote_services'"
		// Let's log it.
		fmt.Printf("Warning: failed to ensure remote services: %v\n", err)
	}

	cfg := PrivateCfg{
		MargeServerUrl:             targetURL,
		StatsServerUrl:             targetURL,
		SwUpdateUrl:                fmt.Sprintf("%s/updates/soundtouch", targetURL),
		UsePandoraProductionServer: true,
		IsZeroconfEnabled:          true,
		SaveMargeCustomerReport:    false,
		BmxRegistryUrl:             fmt.Sprintf("%s/bmx/registry/v1/services", targetURL),
	}

	// If we can read current config, apply per-field options
	if curCfg, curCfgErr := client.Run(fmt.Sprintf("cat %s", SoundTouchSdkPrivateCfgPath)); curCfgErr == nil && curCfg != "" {
		logs += "Read current configuration\n"

		var currentCfg PrivateCfg
		if xml.Unmarshal([]byte(curCfg), &currentCfg) == nil {
			if proxyURL == "" {
				proxyURL = targetURL
			}

			if options != nil {
				m.applyProxyOptions(&cfg, proxyURL, options, &currentCfg)
			} else if proxyURL != "" {
				cfg.MargeServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.MargeServerUrl)
				cfg.StatsServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.StatsServerUrl)
				cfg.SwUpdateUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.SwUpdateUrl)
				cfg.BmxRegistryUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.BmxRegistryUrl)
			}
		}
	}

	xmlContent, err := xml.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return logs, fmt.Errorf("failed to marshal XML: %w", err)
	}

	// Add XML header
	xmlContent = append([]byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"), xmlContent...)

	// 0. Backup original config if it doesn't exist
	remotePath := SoundTouchSdkPrivateCfgPath

	if backupOut, err := client.Run(fmt.Sprintf("[ -f %s.original ]", remotePath)); err != nil {
		logs += fmt.Sprintf("Backing up original config to %s.original (check: %s)\n", remotePath, backupOut)
		fmt.Printf("Backing up original config to %s.original\n", remotePath)

		if output, err := client.Run(fmt.Sprintf("%s && cp %s %s.original", rwCmd, remotePath, remotePath)); err != nil {
			logs += fmt.Sprintf("cp backup failed: %v (output: %s)\n", err, output)
			fmt.Printf("cp backup failed: %v (output: %s)\n", err, output)

			if config, err := client.Run(fmt.Sprintf("cat %s", remotePath)); err == nil && config != "" {
				if err := client.UploadContent([]byte(config), remotePath+".original"); err != nil {
					logs += "failed to upload backup config: " + err.Error() + "\n"
					return logs, fmt.Errorf("cannot create backup of %s before migration: %w", remotePath, err)
				}

				logs += "Uploaded backup config via fallback\n"
			} else {
				return logs, fmt.Errorf("cannot create backup of %s before migration: failed to read original config", remotePath)
			}
		} else {
			logs += "Copied backup config to .original\n"
		}
	} else {
		logs += "Backup .original already exists\n"
	}

	// 1. Upload the configuration
	out, _ = client.Run(rwCmd)

	logs += rwCmd + ": " + out + "\n"
	if err := client.UploadContent(xmlContent, remotePath); err != nil {
		return logs, fmt.Errorf("failed to upload config: %w", err)
	}

	logs += "Uploaded new configuration to " + remotePath + "\n"

	// 2. Verify the configuration on device
	if verification, err := client.Run(fmt.Sprintf("cat %s", remotePath)); err == nil {
		if !strings.Contains(verification, cfg.MargeServerUrl) {
			return logs, fmt.Errorf("verification failed: uploaded config on %s does not contain expected margeServerUrl", deviceIP)
		}

		logs += "Verified configuration on device\n"
	} else {
		logs += fmt.Sprintf("Warning: could not verify configuration on device: %v\n", err)
	}

	// 3. Inject CA Certificate (optional but recommended)
	summary := &MigrationSummary{}
	m.checkCACertTrusted(summary, deviceIP)

	if !summary.CACertTrusted {
		out, err := m.TrustCACert(deviceIP)

		logs += "Trusting CA:\n" + out + "\n"

		if err != nil {
			fmt.Printf("Warning: failed to trust CA: %v\n", err)
		}
	}

	return logs, nil
}

// BackupConfigOffDevice creates a local backup of the speaker's configuration files in the DataStore.
func (m *Manager) BackupConfigOffDevice(deviceIP string) error {
	if m.DataStore == nil {
		return fmt.Errorf("datastore not configured")
	}

	client := m.NewSSH(deviceIP)

	// We need the serial number and account identifier to find the right directory in DataStore
	info, err := m.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	accountID := info.MargeAccountUUID
	deviceID := info.SerialNumber

	if deviceID == "" {
		deviceID = info.DeviceID
	}

	if deviceID == "" {
		deviceID = deviceIP
	}

	if accountID == "" {
		// Try to find account ID from existing device entries if info didn't have it
		devices, _ := m.DataStore.ListAllDevices()
		for i := range devices {
			if devices[i].DeviceSerialNumber == info.SerialNumber || (info.DeviceID != "" && devices[i].DeviceID == info.DeviceID) {
				accountID = devices[i].AccountID
				break
			}
		}
	}

	if accountID == "" {
		accountID = "default"
	}

	deviceDir := m.DataStore.AccountDeviceDir(accountID, deviceID)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		return fmt.Errorf("failed to create device directory: %w", err)
	}

	// 1. Backup SoundTouchSdkPrivateCfg.xml
	if config, err := client.Run(fmt.Sprintf("cat %s", SoundTouchSdkPrivateCfgPath)); err == nil && config != "" {
		backupPath := filepath.Join(deviceDir, "SoundTouchSdkPrivateCfg.xml.bak")
		if err := os.WriteFile(backupPath, []byte(config), 0644); err != nil {
			return fmt.Errorf("failed to write config backup: %w", err)
		}
	}

	// 2. Backup /etc/hosts
	if hosts, err := client.Run("cat /etc/hosts"); err == nil && hosts != "" {
		backupPath := filepath.Join(deviceDir, "hosts.bak")
		if err := os.WriteFile(backupPath, []byte(hosts), 0644); err != nil {
			return fmt.Errorf("failed to write hosts backup: %w", err)
		}
	}

	return nil
}

// BackupConfig creates a backup of the current configuration on the speaker.
func (m *Manager) BackupConfig(deviceIP string) (string, error) {
	client := m.NewSSH(deviceIP)
	remotePath := SoundTouchSdkPrivateCfgPath
	rwCmd := "(rw || mount -o remount,rw /)"

	// Check if .original already exists
	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", remotePath)); err == nil {
		return "", fmt.Errorf("backup already exists at %s.original", remotePath)
	}

	// Try to copy on the device first (more reliable), ensuring filesystem is writable
	output, cpErr := client.Run(fmt.Sprintf("%s && cp %s %s.original", rwCmd, remotePath, remotePath))
	if cpErr == nil {
		return output, nil
	}

	logs := output + "\n"
	fmt.Printf("Direct cp failed: %v (output: %s), falling back to cat+upload\n", cpErr, output)

	// Fallback to cat + upload
	config, err := client.Run(fmt.Sprintf("cat %s", remotePath))

	logs += "cat " + remotePath + ": " + config + "\n"
	if err != nil || config == "" {
		return logs, fmt.Errorf("failed to read current config: %w", err)
	}

	// Ensure rw before upload fallback
	out, _ := client.Run(rwCmd)

	logs += rwCmd + ": " + out + "\n"
	if err := client.UploadContent([]byte(config), remotePath+".original"); err != nil {
		return logs, fmt.Errorf("failed to upload backup config: %w", err)
	}

	logs += "Uploaded backup to " + remotePath + ".original\n"

	return logs, nil
}

// EnsureRemoteServices ensures that remote services are enabled on the device.
// It tries to create an empty file in one of the known valid locations.
func (m *Manager) EnsureRemoteServices(deviceIP string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	// Try locations in order of preference
	locations := []string{
		"/etc/remote_services",
		"/mnt/nv/remote_services",
		"/tmp/remote_services",
	}

	var logs string

	for _, loc := range locations {
		// Try to make filesystem writable for each location that might need it
		// Combining rw && touch ensures it's attempted in the same sequence
		out, err := client.Run(fmt.Sprintf("%s && touch %s", rwCmd, loc))

		logs += fmt.Sprintf("touch %s (with rw): %s\n", loc, out)
		if err == nil {
			return logs, nil
		}
		// If rw && touch failed, try just touch (e.g. for /tmp which doesn't need rw)
		out, err = client.Run(fmt.Sprintf("touch %s", loc))

		logs += fmt.Sprintf("touch %s: %s\n", loc, out)
		if err == nil {
			return logs, nil
		}
	}

	return logs, fmt.Errorf("failed to enable remote services in any of the locations: %v", locations)
}

// TrustCACert injects the local CA certificate into the device's shared trust store.
func (m *Manager) TrustCACert(deviceIP string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	var logs string

	caCertPEM, err := os.ReadFile(m.Crypto.GetCACertPath())
	if err != nil {
		return "", fmt.Errorf("failed to read CA certificate: %w", err)
	}

	bundlePath := "/etc/pki/tls/certs/ca-bundle.crt"
	out, _ := client.Run(rwCmd)
	logs += rwCmd + ": " + out + "\n"

	// Backup bundle if it doesn't exist
	if _, backupErr := client.Run(fmt.Sprintf("[ -f %s.original ]", bundlePath)); backupErr != nil {
		out, _ := client.Run(fmt.Sprintf("cp %s %s.original", bundlePath, bundlePath))
		logs += fmt.Sprintf("cp %s %s.original: %s\n", bundlePath, bundlePath, out)
	}

	// Check if the label already exists in the bundle
	bundleContent, err := client.Run(fmt.Sprintf("cat %s", bundlePath))

	logs += "cat " + bundlePath + " (check existing)\n"
	if err != nil {
		return logs, fmt.Errorf("failed to read bundle: %w", err)
	}

	if strings.Contains(bundleContent, CALabel) {
		// Label found, let's replace the whole block between labels if we used them,
		// or just remove the lines containing the label and re-append.
		// For simplicity, let's remove everything between CALabel tags if we had them,
		// but since we only had one line before, let's just remove lines containing CALabel
		// and the cert data if possible.
		// A better way is to rebuild the bundle without our CA.
		lines := strings.Split(bundleContent, "\n")

		var newLines []string

		inOurCA := false

		for _, line := range lines {
			if strings.Contains(line, CALabel) {
				inOurCA = !inOurCA
				continue
			}

			if !inOurCA {
				newLines = append(newLines, line)
			}
		}

		bundleContent = strings.Join(newLines, "\n")
		if bundleContent != "" && !strings.HasSuffix(bundleContent, "\n") {
			bundleContent += "\n"
		}
	} else if bundleContent != "" && !strings.HasSuffix(bundleContent, "\n") {
		bundleContent += "\n"
	}

	// Append with labels
	labeledCert := fmt.Sprintf("\n%s\n%s%s\n", CALabel, string(caCertPEM), CALabel)
	newBundleContent := bundleContent + labeledCert

	if err := client.UploadContent([]byte(newBundleContent), bundlePath); err != nil {
		return logs, fmt.Errorf("failed to update bundle: %w", err)
	}

	logs += "Uploaded updated bundle to " + bundlePath + "\n"

	return logs, nil
}

func (m *Manager) migrateViaHosts(deviceIP, targetURL string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	var logs string

	// 1. Parse targetURL to get IP for /etc/hosts
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse target URL: %w", err)
	}

	hostName := parsedURL.Hostname()
	if hostName == "" || hostName == "localhost" {
		// Use a better guess if needed, but for now expect valid IP/hostname
		return "", fmt.Errorf("target URL must contain a valid IP or hostname (got %s)", hostName)
	}

	hostIP, err := m.resolveIP(hostName, client)
	if err != nil {
		return logs, fmt.Errorf("cannot resolve target hostname for migration: %w", err)
	}

	logs += fmt.Sprintf("Resolved %s to %s\n", hostName, hostIP)

	// 2. Prepare /etc/hosts entries
	domains := []string{
		"streaming.bose.com",
		"updates.bose.com",
		"stats.bose.com",
		"bmx.bose.com",
		"content.api.bose.io",
		"events.api.bosecm.com",
		"bose-prod.apigee.net",
		"worldwide.bose.com",
		"media.bose.io",
		"downloads.bose.com",
		"voice.api.bose.io",
	}

	hostsContent, err := client.Run("cat /etc/hosts")

	logs += "cat /etc/hosts: " + hostsContent + "\n"
	if err != nil {
		return logs, fmt.Errorf("failed to read /etc/hosts: %w", err)
	}

	hostsContent = m.generateHostsContent(hostsContent, domains, hostIP)

	// 3. Upload new /etc/hosts
	out, _ := client.Run(rwCmd)
	logs += rwCmd + ": " + out + "\n"
	// Backup /etc/hosts if it doesn't exist
	if _, err := client.Run("[ -f /etc/hosts.original ]"); err != nil {
		out, _ := client.Run("cp /etc/hosts /etc/hosts.original")
		logs += "cp /etc/hosts /etc/hosts.original: " + out + "\n"
	}

	if err := client.UploadContent([]byte(hostsContent), "/etc/hosts"); err != nil {
		return logs, fmt.Errorf("failed to update /etc/hosts: %w", err)
	}

	logs += "Uploaded updated /etc/hosts\n"

	// 4. Verify /etc/hosts on device
	if err := m.verifyHosts(client, domains, hostIP, deviceIP); err != nil {
		return logs, err
	}

	logs += "Verified /etc/hosts on device\n"

	fmt.Printf("Updated /etc/hosts on %s:\n%s\n", deviceIP, hostsContent)

	// 5. Inject CA Certificate
	summary := &MigrationSummary{}
	m.checkCACertTrusted(summary, deviceIP)

	if !summary.CACertTrusted {
		out, err := m.TrustCACert(deviceIP)

		logs += "Trusting CA:\n" + out + "\n"
		if err != nil {
			return logs, err
		}
	} else {
		logs += "CA certificate already trusted, skipping injection\n"

		fmt.Printf("CA certificate already trusted on %s, skipping injection\n", deviceIP)
	}

	return logs, nil
}

func (m *Manager) generateHostsContent(currentContent string, domains []string, hostIP string) string {
	lines := strings.Split(currentContent, "\n")

	var newLines []string

	domainFound := make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			newLines = append(newLines, line)
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) >= 2 {
			domain := fields[1]
			isBoseDomain := false

			for _, d := range domains {
				if d == domain {
					isBoseDomain = true
					break
				}
			}

			if isBoseDomain {
				// Update existing entry with new IP
				newLines = append(newLines, fmt.Sprintf("%s\t%s", hostIP, domain))
				domainFound[domain] = true

				continue
			}
		}

		newLines = append(newLines, line)
	}

	// Add missing domains
	for _, domain := range domains {
		if !domainFound[domain] {
			newLines = append(newLines, fmt.Sprintf("%s\t%s", hostIP, domain))
		}
	}

	hostsContent := strings.Join(newLines, "\n")
	if !strings.HasSuffix(hostsContent, "\n") {
		hostsContent += "\n"
	}

	return hostsContent
}

func (m *Manager) verifyHosts(client SSHClient, domains []string, hostIP, deviceIP string) error {
	verification, err := client.Run("cat /etc/hosts")
	if err != nil {
		return fmt.Errorf("could not verify /etc/hosts on device: %w", err)
	}

	for _, domain := range domains {
		if !strings.Contains(verification, domain) || !strings.Contains(verification, hostIP) {
			return fmt.Errorf("verification failed: /etc/hosts on %s does not contain expected redirection for %s", deviceIP, domain)
		}
	}

	return nil
}

func (m *Manager) migrateViaResolvConf(deviceIP, targetURL string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	var logs string

	// 1. Resolve target hostname to IP
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse target URL: %w", err)
	}

	hostName := parsedURL.Hostname()
	if hostName == "" || hostName == "localhost" {
		return "", fmt.Errorf("target URL must contain a valid IP or hostname (got %s)", hostName)
	}

	hostIP, err := m.resolveIP(hostName, client)
	if err != nil {
		return logs, fmt.Errorf("cannot resolve target hostname for migration: %w", err)
	}

	logs += fmt.Sprintf("Resolved %s to %s\n", hostName, hostIP)

	// 2. Prepare /mnt/nv/soundtouch-service/aftertouch.resolv.conf content
	resolvContent := fmt.Sprintf("# Created by Aftertouch/SoundTouch-Service\n# Priority nameserver for Bose service redirection\nnameserver %s\n", hostIP)

	// 3. Upload /mnt/nv/soundtouch-service/aftertouch.resolv.conf
	// Ensure /mnt/nv/soundtouch-service exists
	_, _ = client.Run("mkdir -p /mnt/nv/soundtouch-service")

	if uploadErr := client.UploadContent([]byte(resolvContent), "/mnt/nv/soundtouch-service/aftertouch.resolv.conf"); uploadErr != nil {
		return logs, fmt.Errorf("failed to upload /mnt/nv/soundtouch-service/aftertouch.resolv.conf: %w", uploadErr)
	}

	logs += "Uploaded /mnt/nv/soundtouch-service/aftertouch.resolv.conf\n"

	// 4. Update /mnt/nv/rc.local with idempotent patch
	patchOut, err := m.updateRcLocalWithDNSHook(client)
	logs += patchOut

	if err != nil {
		return logs, err
	}

	// 5. Cleanup legacy file
	_, _ = client.Run("rm -f /mnt/nv/aftertouch.resolv.conf")

	// 6. Apply patch immediately to /etc/udhcpc.d/50default
	rwOut, _ := client.Run(rwCmd)
	logs += rwCmd + ": " + rwOut + "\n"

	hookMarker := "/mnt/nv/soundtouch-service/aftertouch.resolv.conf"
	targetDHCPFile := "/etc/udhcpc.d/50default"
	dhcpPatchOut, err := m.patchDHCPFile(client, targetDHCPFile, hookMarker)
	logs += dhcpPatchOut

	if err != nil {
		logs += fmt.Sprintf("Warning: could not apply/verify patch on %s: %v\n", targetDHCPFile, err)
	}

	// Apply patch immediately to /opt/Bose/udhcpc.script if it exists
	targetScript := "/opt/Bose/udhcpc.script"
	if _, err := client.Run(fmt.Sprintf("[ -f %s ]", targetScript)); err == nil {
		scriptPatchOut, err := m.patchUdhcpcScript(client, targetScript, hookMarker)
		logs += scriptPatchOut

		if err != nil {
			logs += fmt.Sprintf("Warning: could not apply/verify patch on %s: %v\n", targetScript, err)
		}
	}

	// 6. Inject CA Certificate
	summary := &MigrationSummary{}
	m.checkCACertTrusted(summary, deviceIP)

	if !summary.CACertTrusted {
		out, err := m.TrustCACert(deviceIP)

		logs += "Trusting CA:\n" + out + "\n"
		if err != nil {
			return logs, err
		}
	} else {
		logs += "CA certificate already trusted, skipping injection\n"
	}

	return logs, nil
}

func (m *Manager) updateRcLocalWithDNSHook(client SSHClient) (string, error) {
	var logs string

	rcLocalPath := "/mnt/nv/rc.local"
	targetDHCPFile := "/etc/udhcpc.d/50default"
	hookMarker := "/mnt/nv/soundtouch-service/aftertouch.resolv.conf"

	// Check if rc.local exists and read it
	currentRcLocal, rcErr := client.Run(fmt.Sprintf("cat %s", rcLocalPath))
	if rcErr != nil {
		currentRcLocal = ""
	}

	if strings.Contains(currentRcLocal, hookMarker) {
		return fmt.Sprintf("%s already contains Aftertouch hook logic\n", rcLocalPath), nil
	}

	patchStartMarker := "# --- Aftertouch DNS hook START ---"
	patchEndMarker := "# --- Aftertouch DNS hook END ---"

	patchLogic := fmt.Sprintf(`
%s
# prioritizes our custom nameserver if it exists
if [ -f "%s" ]; then
    if [ -f "%s" ] && ! grep -q "%s" "%s"; then
        logger -t "aftertouch" "Patching %s with Aftertouch DNS hook"
        sed -i '/echo "search \$domain"/a \        [ -f '"%s"' ] && cat '"%s"' && dns=""' "%s"
    fi
    targetScript="/opt/Bose/udhcpc.script"
    if [ -f "$targetScript" ] && ! grep -q "%s" "$targetScript"; then
        logger -t "aftertouch" "Patching $targetScript with Aftertouch DNS hook"
        sed -i '/echo "search \$search_list # \$interface" >> \$RESOLV_CONF/a \                [ -f '"%s"' ] && cat '"%s"' >> '"\$RESOLV_CONF"' && dns=""' "$targetScript"
    fi
fi
%s
`, patchStartMarker, hookMarker, targetDHCPFile, hookMarker, targetDHCPFile, targetDHCPFile, hookMarker, hookMarker, targetDHCPFile, hookMarker, hookMarker, hookMarker, patchEndMarker)

	newRcLocal := currentRcLocal
	// Remove old-style DNS hook if it exists
	if strings.Contains(newRcLocal, "# Aftertouch DNS hook") && !strings.Contains(newRcLocal, patchStartMarker) {
		// Old removal: filter out lines between the marker and the first 'fi'
		lines := strings.Split(newRcLocal, "\n")

		var filteredLines []string

		skip := false

		for _, line := range lines {
			if strings.Contains(line, "# Aftertouch DNS hook") {
				skip = true
				continue
			}

			if skip && strings.TrimSpace(line) == "fi" {
				skip = false
				continue
			}

			if !skip {
				filteredLines = append(filteredLines, line)
			}
		}

		newRcLocal = strings.Join(filteredLines, "\n")
	}

	// Remove existing marker-based hook if it exists (for update)
	if strings.Contains(newRcLocal, patchStartMarker) {
		startIdx := strings.Index(newRcLocal, patchStartMarker)

		endIdx := strings.Index(newRcLocal, patchEndMarker)
		if startIdx != -1 && endIdx != -1 {
			newRcLocal = newRcLocal[:startIdx] + newRcLocal[endIdx+len(patchEndMarker):]
		}
	}
	// Remove "cat: can't open..." error message if it was accidentally saved in the file
	if strings.Contains(newRcLocal, "cat: can't open") {
		newRcLocal = ""
	}

	if !strings.HasPrefix(newRcLocal, "#!/bin/sh") {
		newRcLocal = "#!/bin/sh\n" + strings.TrimPrefix(newRcLocal, "#!/bin/sh")
	}

	if !strings.HasSuffix(newRcLocal, "\n") {
		newRcLocal += "\n"
	}

	newRcLocal += patchLogic

	if err := client.UploadContent([]byte(newRcLocal), rcLocalPath); err != nil {
		return logs, fmt.Errorf("failed to update %s: %w", rcLocalPath, err)
	}

	logs += fmt.Sprintf("Updated %s with DNS hook logic\n", rcLocalPath)

	// Make it executable
	_, _ = client.Run(fmt.Sprintf("chmod +x %s", rcLocalPath))

	return logs, nil
}

func (m *Manager) patchDHCPFile(client SSHClient, targetDHCPFile, hookMarker string) (string, error) {
	var logs string

	// Backup if it doesn't exist
	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", targetDHCPFile)); err != nil {
		out, _ := client.Run(fmt.Sprintf("cp %s %s.original", targetDHCPFile, targetDHCPFile))
		logs += fmt.Sprintf("cp %s %s.original: %s\n", targetDHCPFile, targetDHCPFile, out)
	} else {
		// If backup exists, revert to it first to ensure we start from a clean state
		_, _ = client.Run(fmt.Sprintf("cp %s.original %s", targetDHCPFile, targetDHCPFile))
	}

	// Run the patch logic via SSH to apply it now
	patchCmd := fmt.Sprintf("sed -i '/echo \"search \\$domain\"/a \\        [ -f '\"%s\"' ] && cat '\"%s\"' && dns=\"\"' %s", hookMarker, hookMarker, targetDHCPFile)
	if _, err := client.Run(patchCmd); err != nil {
		return logs, fmt.Errorf("failed to apply patch immediately to %s: %w", targetDHCPFile, err)
	}

	logs += fmt.Sprintf("Applied patch to %s\n", targetDHCPFile)

	// Verify patch on 50default
	if verification, err := client.Run(fmt.Sprintf("grep -q \"%s\" %s && echo \"OK\"", hookMarker, targetDHCPFile)); err == nil && strings.TrimSpace(verification) == "OK" {
		logs += fmt.Sprintf("Verified patch on %s\n", targetDHCPFile)
	} else {
		return logs, fmt.Errorf("could not verify patch on %s: %w", targetDHCPFile, err)
	}

	return logs, nil
}

func (m *Manager) patchUdhcpcScript(client SSHClient, targetScript, hookMarker string) (string, error) {
	var logs string

	// Backup if it doesn't exist
	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", targetScript)); err != nil {
		out, _ := client.Run(fmt.Sprintf("cp %s %s.original", targetScript, targetScript))
		logs += fmt.Sprintf("cp %s %s.original: %s\n", targetScript, targetScript, out)
	} else {
		// If backup exists, revert to it first to ensure we start from a clean state
		_, _ = client.Run(fmt.Sprintf("cp %s.original %s", targetScript, targetScript))
	}

	patchCmdScript := fmt.Sprintf("sed -i '/echo \"search \\$search_list # \\$interface\" >> \\$RESOLV_CONF/a \\                [ -f '\"%s\"' ] && cat '\"%s\"' >> '\"\\$RESOLV_CONF\"' && dns=\"\"' %s", hookMarker, hookMarker, targetScript)
	if _, err := client.Run(patchCmdScript); err != nil {
		return logs, fmt.Errorf("failed to apply patch immediately to %s: %w", targetScript, err)
	}

	logs += fmt.Sprintf("Applied patch to %s\n", targetScript)

	// Verify patch on udhcpc.script
	if verification, err := client.Run(fmt.Sprintf("grep -q \"%s\" %s && echo \"OK\"", hookMarker, targetScript)); err == nil && strings.TrimSpace(verification) == "OK" {
		logs += fmt.Sprintf("Verified patch on %s\n", targetScript)
	} else {
		return logs, fmt.Errorf("could not verify patch on %s: %w", targetScript, err)
	}

	return logs, nil
}

// RevertMigration reverts the speaker to its original Bose cloud configuration.
func (m *Manager) RevertMigration(deviceIP string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	var logs string

	// 1. Revert SoundTouchSdkPrivateCfg.xml
	out, err := m.revertXMLConfig(client, rwCmd)

	logs += out
	if err != nil {
		return logs, err
	}

	// 2. Revert /etc/hosts
	logs += m.revertHosts(client, rwCmd)

	// 2b. Revert /etc/resolv.conf
	logs += m.revertResolvConf(client, rwCmd)

	// 2c. Revert Aftertouch DNS Hook
	logs += m.revertAftertouchHook(client, rwCmd)

	// 3. Remove CA certificate from trust store if it exists
	logs += m.revertCACert(client, rwCmd)

	return logs, nil
}

func (m *Manager) revertXMLConfig(client SSHClient, rwCmd string) (string, error) {
	var logs string

	remotePath := SoundTouchSdkPrivateCfgPath
	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", remotePath)); err == nil {
		logs += fmt.Sprintf("Reverting %s from backup\n", remotePath)
		fmt.Printf("Reverting %s from backup\n", remotePath)
		out, err := client.Run(fmt.Sprintf("%s && cp %s.original %s", rwCmd, remotePath, remotePath))

		logs += fmt.Sprintf("cp %s.original %s: %s\n", remotePath, remotePath, out)
		if err != nil {
			return logs, fmt.Errorf("failed to revert %s: %w", remotePath, err)
		}
	} else {
		return logs, fmt.Errorf("backup %s.original not found, cannot revert", remotePath)
	}

	return logs, nil
}

func (m *Manager) revertHosts(client SSHClient, rwCmd string) string {
	var logs string

	hostsPath := "/etc/hosts"
	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", hostsPath)); err == nil {
		logs += fmt.Sprintf("Reverting %s from backup\n", hostsPath)
		fmt.Printf("Reverting %s from backup\n", hostsPath)
		out, err := client.Run(fmt.Sprintf("%s && cp %s.original %s", rwCmd, hostsPath, hostsPath))

		logs += fmt.Sprintf("cp %s.original %s: %s\n", hostsPath, hostsPath, out)
		if err != nil {
			fmt.Printf("Warning: failed to revert %s: %v\n", hostsPath, err)
		}
	}

	return logs
}

func (m *Manager) revertResolvConf(client SSHClient, rwCmd string) string {
	var logs string

	resolvPath := "/etc/resolv.conf"
	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", resolvPath)); err == nil {
		logs += fmt.Sprintf("Reverting %s from backup\n", resolvPath)
		fmt.Printf("Reverting %s from backup\n", resolvPath)

		// Try to remove immutable flag if it was set
		_, _ = client.Run(fmt.Sprintf("chattr -i %s", resolvPath))

		out, err := client.Run(fmt.Sprintf("%s && cp %s.original %s", rwCmd, resolvPath, resolvPath))

		logs += fmt.Sprintf("cp %s.original %s: %s\n", resolvPath, resolvPath, out)
		if err != nil {
			fmt.Printf("Warning: failed to revert %s: %v\n", resolvPath, err)
		}
	}

	return logs
}

func (m *Manager) revertAftertouchHook(client SSHClient, rwCmd string) string {
	var logs string

	aftertouchConfPath := "/mnt/nv/soundtouch-service/aftertouch.resolv.conf"
	legacyConfPath := "/mnt/nv/aftertouch.resolv.conf"
	rcLocalPath := "/mnt/nv/rc.local"
	targetDHCPFile := "/etc/udhcpc.d/50default"

	for _, p := range []string{aftertouchConfPath, legacyConfPath} {
		if _, err := client.Run(fmt.Sprintf("[ -f %s ]", p)); err == nil {
			logs += fmt.Sprintf("Removing %s\n", p)
			fmt.Printf("Removing %s\n", p)
			_, _ = client.Run(fmt.Sprintf("rm %s", p))
		}
	}

	logs += m.removeRcLocalHooks(client, rcLocalPath)

	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", targetDHCPFile)); err == nil {
		logs += fmt.Sprintf("Reverting %s from backup\n", targetDHCPFile)
		fmt.Printf("Reverting %s from backup\n", targetDHCPFile)
		out, err := client.Run(fmt.Sprintf("%s && cp %s.original %s", rwCmd, targetDHCPFile, targetDHCPFile))

		logs += fmt.Sprintf("cp %s.original %s: %s\n", targetDHCPFile, targetDHCPFile, out)
		if err != nil {
			fmt.Printf("Warning: failed to revert %s: %v\n", targetDHCPFile, err)
		}
	}

	targetScript := "/opt/Bose/udhcpc.script"
	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", targetScript)); err == nil {
		logs += fmt.Sprintf("Reverting %s from backup\n", targetScript)
		fmt.Printf("Reverting %s from backup\n", targetScript)
		out, err := client.Run(fmt.Sprintf("%s && cp %s.original %s", rwCmd, targetScript, targetScript))

		logs += fmt.Sprintf("cp %s.original %s: %s\n", targetScript, targetScript, out)
		if err != nil {
			fmt.Printf("Warning: failed to revert %s: %v\n", targetScript, err)
		}
	}

	return logs
}

func (m *Manager) removeRcLocalHooks(client SSHClient, rcLocalPath string) string {
	var logs string

	patchStartMarker := "# --- Aftertouch DNS hook START ---"
	patchEndMarker := "# --- Aftertouch DNS hook END ---"
	spotifyPatchStartMarker := "# --- Aftertouch Spotify hook START ---"
	spotifyPatchEndMarker := "# --- Aftertouch Spotify hook END ---"
	aftertouchConfPath := "/mnt/nv/soundtouch-service/aftertouch.resolv.conf"
	legacyAftertouchConfPath := "/mnt/nv/aftertouch.resolv.conf"

	currentRcLocal, err := client.Run(fmt.Sprintf("cat %s", rcLocalPath))
	if err != nil {
		return ""
	}

	// Remove "cat: can't open..." error message if it was accidentally saved in the file
	if strings.Contains(currentRcLocal, "cat: can't open") {
		logs += fmt.Sprintf("Removing corrupted %s\n", rcLocalPath)
		_, _ = client.Run(fmt.Sprintf("rm %s", rcLocalPath))

		return logs
	}

	modified := false

	if strings.Contains(currentRcLocal, patchStartMarker) {
		logs += fmt.Sprintf("Removing Aftertouch hook logic from %s\n", rcLocalPath)
		fmt.Printf("Removing Aftertouch hook logic from %s\n", rcLocalPath)

		startIdx := strings.Index(currentRcLocal, patchStartMarker)
		endIdx := strings.Index(currentRcLocal, patchEndMarker)

		if startIdx != -1 && endIdx != -1 {
			currentRcLocal = currentRcLocal[:startIdx] + currentRcLocal[endIdx+len(patchEndMarker):]
			modified = true
		}
	} else if strings.Contains(currentRcLocal, aftertouchConfPath) || strings.Contains(currentRcLocal, legacyAftertouchConfPath) || strings.Contains(currentRcLocal, "# Aftertouch DNS hook") {
		logs += fmt.Sprintf("Removing legacy Aftertouch hook logic from %s\n", rcLocalPath)
		fmt.Printf("Removing legacy Aftertouch hook logic from %s\n", rcLocalPath)

		lines := strings.Split(currentRcLocal, "\n")

		var newLines []string

		skip := false

		for _, line := range lines {
			if strings.Contains(line, "# Aftertouch DNS hook") {
				skip = true
				continue
			}

			if skip && strings.TrimSpace(line) == "fi" {
				skip = false
				continue
			}

			if !skip {
				newLines = append(newLines, line)
			}
		}

		currentRcLocal = strings.Join(newLines, "\n")
		modified = true
	}

	if strings.Contains(currentRcLocal, spotifyPatchStartMarker) {
		logs += fmt.Sprintf("Removing Spotify hook logic from %s\n", rcLocalPath)
		fmt.Printf("Removing Spotify hook logic from %s\n", rcLocalPath)

		startIdx := strings.Index(currentRcLocal, spotifyPatchStartMarker)
		endIdx := strings.Index(currentRcLocal, spotifyPatchEndMarker)

		if startIdx != -1 && endIdx != -1 {
			currentRcLocal = currentRcLocal[:startIdx] + currentRcLocal[endIdx+len(spotifyPatchEndMarker):]
			modified = true
		}
	}

	if modified {
		if err := client.UploadContent([]byte(currentRcLocal), rcLocalPath); err != nil {
			fmt.Printf("Warning: failed to update %s: %v\n", rcLocalPath, err)
		}
	}

	return logs
}

func (m *Manager) revertCACert(client SSHClient, rwCmd string) string {
	var logs string

	bundlePath := "/etc/pki/tls/certs/ca-bundle.crt"
	if bundleContent, err := client.Run(fmt.Sprintf("cat %s", bundlePath)); err == nil && strings.Contains(bundleContent, CALabel) {
		logs += fmt.Sprintf("Removing local CA certificate from %s\n", bundlePath)
		fmt.Printf("Removing local CA certificate from %s\n", bundlePath)

		lines := strings.Split(bundleContent, "\n")

		var newLines []string

		inOurCA := false

		for _, line := range lines {
			if strings.Contains(line, CALabel) {
				inOurCA = !inOurCA
				continue
			}

			if !inOurCA {
				newLines = append(newLines, line)
			}
		}

		bundleContent = strings.Join(newLines, "\n")
		if bundleContent != "" && !strings.HasSuffix(bundleContent, "\n") {
			bundleContent += "\n"
		}

		out, _ := client.Run(rwCmd)

		logs += rwCmd + ": " + out + "\n"

		if err := client.UploadContent([]byte(bundleContent), bundlePath); err != nil {
			logs += "Warning: failed to remove CA from " + bundlePath + ": " + err.Error() + "\n"
			fmt.Printf("Warning: failed to remove CA from %s: %v\n", bundlePath, err)
		} else {
			logs += "Uploaded updated bundle (CA removed)\n"
		}
	}

	return logs
}

// RemoveRemoteServices removes remote services from the device by deleting the known remote_services files.
func (m *Manager) RemoveRemoteServices(deviceIP string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	locations := []string{
		"/etc/remote_services",
		"/mnt/nv/remote_services",
		"/tmp/remote_services",
	}

	var (
		logs   string
		errors []error
	)

	for _, loc := range locations {
		// Try to make filesystem writable and remove the file
		out, err := client.Run(fmt.Sprintf("%s && rm -v %s", rwCmd, loc))

		logs += fmt.Sprintf("Removing %s: %s\n", loc, out)
		if err != nil {
			// If rw && rm failed, try just rm (e.g. for /tmp)
			out, err = client.Run(fmt.Sprintf("rm -v %s", loc))

			logs += fmt.Sprintf("Fallback removing %s: %s\n", loc, out)
			if err != nil {
				errors = append(errors, fmt.Errorf("failed to remove %s: %w", loc, err))
			}
		}
	}

	if len(errors) == len(locations) {
		return logs, fmt.Errorf("failed to remove remote services from any location: %v", errors)
	}

	return logs, nil
}

// RebootMethod selects the transport used to reboot a speaker.
type RebootMethod string

const (
	// RebootMethodSSH reboots via SSH `reboot` (the original behavior). Requires
	// a rooted device (remote_services unlocked).
	RebootMethodSSH RebootMethod = "ssh"
	// RebootMethodTelnet reboots via the device's port-17000 diagnostic shell
	// using `sys reboot`. Requires no SSH access.
	RebootMethodTelnet RebootMethod = "telnet"
)

// Reboot reboots the speaker at the given IP using the requested transport.
// An empty method defaults to RebootMethodSSH, preserving prior behavior.
func (m *Manager) Reboot(deviceIP string, method RebootMethod) (string, error) {
	if method == "" {
		method = RebootMethodSSH
	}

	switch method {
	case RebootMethodSSH:
		return m.rebootViaSSH(deviceIP)
	case RebootMethodTelnet:
		return m.rebootViaTelnet(deviceIP)
	default:
		return "", fmt.Errorf("unsupported reboot method: %s", method)
	}
}

func (m *Manager) rebootViaSSH(deviceIP string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	fmt.Printf("Rebooting speaker at %s via SSH\n", deviceIP)

	out, err := client.Run(fmt.Sprintf("%s && reboot", rwCmd))
	if err != nil {
		return out, fmt.Errorf("failed to reboot speaker: %w", err)
	}

	return out, nil
}

func (m *Manager) rebootViaTelnet(deviceIP string) (string, error) {
	if m.NewTelnet == nil {
		return "", errors.New("telnet reboot not configured: Manager.NewTelnet is nil")
	}

	fmt.Printf("Rebooting speaker at %s via telnet\n", deviceIP)

	t := m.NewTelnet(deviceIP)
	if err := t.Dial(); err != nil {
		return "", fmt.Errorf("telnet dial %s:17000 failed: %w", deviceIP, err)
	}

	defer func() { _ = t.Close() }()

	// We deliberately don't wait for a response — the device closes the socket
	// as part of rebooting, and SendCommand would surface that as an error
	// even though the reboot itself succeeded. Treat any short read or close
	// as "command was accepted".
	resp, err := t.SendCommand("sys reboot")
	if err != nil {
		// A read error after the write is the expected case (socket dies on
		// reboot). Only surface real transport failures; treat the rest as
		// success and let the caller verify by polling :8090/info.
		if isLikelyRebootCloseError(err) {
			return resp + "\n[connection closed by reboot]", nil
		}

		return resp, fmt.Errorf("failed to send sys reboot: %w", err)
	}

	return resp, nil
}

// isLikelyRebootCloseError returns true if err looks like the socket closed
// because the device started rebooting, rather than a real connectivity
// problem. We are intentionally generous here: the user already opted into
// rebooting, so a closed socket is expected.
func isLikelyRebootCloseError(err error) bool {
	msg := err.Error()
	for _, marker := range []string{"EOF", "closed", "connection reset", "broken pipe", "timed out"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}

	return false
}

// TestDomain is the fake domain used for preliminary redirection tests.
const TestDomain = "custom-test-api.bose.fake"

// CALabel is the label used to identify the local CA certificate in the trust store.
const CALabel = "# AfterTouch"

// TestHostsRedirection performs a preliminary check to see if /etc/hosts redirection works.
func (m *Manager) TestHostsRedirection(deviceIP, targetURL string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	hostIP, parsedURL, err := m.parseTargetURLAndResolveIP(targetURL, client)
	if err != nil {
		return "", err
	}

	testDomain := TestDomain
	testEntry := fmt.Sprintf("%s\t%s", hostIP, testDomain)

	if addErr := m.addTemporaryHostEntry(client, deviceIP, testDomain, testEntry, rwCmd); addErr != nil {
		return "", addErr
	}

	defer m.cleanupTemporaryHostEntry(client, testDomain, rwCmd)

	output, err := m.runHTTPRedirectionTest(client, parsedURL, testDomain)
	if err != nil {
		return output, err
	}

	httpsOutput, httpsErr := m.runHTTPSRedirectionTest(client, testDomain)

	combinedOutput := output + "\n---\n" + httpsOutput
	if httpsErr != nil {
		return combinedOutput, fmt.Errorf("hosts redirection HTTPS test failed: %w", httpsErr)
	}

	return combinedOutput, nil
}

// TestDNSRedirection performs a check from the device to see if DNS queries are intercepted by the AfterTouch service.
func (m *Manager) TestDNSRedirection(deviceIP, targetURL string) (string, error) {
	client := m.NewSSH(deviceIP)

	hostIP, _, err := m.parseTargetURLAndResolveIP(targetURL, client)
	if err != nil {
		return "", err
	}

	// Use a raw DNS query via nc (netcat) to test DNS resolution from the device,
	// because BusyBox nslookup might not support custom ports.
	testDomain := "aftertouch.test"

	// Fetch configured DNS port if available
	dnsPort := "53"

	if m.DataStore != nil {
		if dsSettings, getSettingsErr := m.DataStore.GetSettings(); getSettingsErr == nil && dsSettings.DNSBindAddr != "" {
			if lastColon := strings.LastIndex(dsSettings.DNSBindAddr, ":"); lastColon != -1 {
				port := dsSettings.DNSBindAddr[lastColon+1:]
				if _, atoiErr := strconv.Atoi(port); atoiErr == nil {
					dnsPort = port
				}
			}
		}
	}

	// Raw DNS query for aftertouch.test (Type A, Class IN)
	// Transaction ID: 0xAAAA, Flags: 0x0100 (Standard query), Questions: 1, Answer RRs: 0, Authority RRs: 0, Additional RRs: 0
	// Query: aftertouch.test, Type: A, Class: IN
	// For TCP, we need a 2-byte length prefix: 0x0021 (33 bytes)
	dnsQueryHex := "\\x00\\x21\\xaa\\xaa\\x01\\x00\\x00\\x01\\x00\\x00\\x00\\x00\\x00\\x00\\x0aaftertouch\\x04test\\x00\\x00\\x01\\x00\\x01"
	// We use TCP (default for nc) because BusyBox nc might not support -u,
	// and our DNS server listens on both TCP and UDP.
	// DNS over TCP response also has a 2-byte length prefix, but tail -c 4 will still get the IP from the end.
	ncCmd := fmt.Sprintf("echo -ne '%s' | nc -w 5 %s %s | tail -c 4 | od -An -tu1", dnsQueryHex, hostIP, dnsPort)

	output, err := client.Run(ncCmd)
	if err == nil {
		// Parse the IP from od output: " 192 168 178 122"
		fields := strings.Fields(output)
		if len(fields) == 4 {
			resolvedIP := fmt.Sprintf("%s.%s.%s.%s", fields[0], fields[1], fields[2], fields[3])
			if resolvedIP == hostIP {
				return fmt.Sprintf("Success: Raw DNS query for %s returned %s via nc to %s:%s", testDomain, resolvedIP, hostIP, dnsPort), nil
			}

			return output, fmt.Errorf("DNS redirection test failed: nc returned %s, expected %s", resolvedIP, hostIP)
		}
	}

	// Fallback to nslookup if nc fails (maybe nc is missing or it's standard port 53)
	serverAddr := hostIP
	if dnsPort != "53" {
		serverAddr = fmt.Sprintf("%s:%s", hostIP, dnsPort)
	}

	nslookupCmd := fmt.Sprintf("nslookup %s %s", testDomain, serverAddr)
	nslookupOutput, nslookupErr := client.Run(nslookupCmd)

	if nslookupErr == nil && strings.Contains(nslookupOutput, hostIP) {
		return nslookupOutput, nil
	}

	return fmt.Sprintf("nc Output: %s (err: %v)\nnslookup Output: %s (err: %v)", output, err, nslookupOutput, nslookupErr),
		fmt.Errorf("DNS redirection test failed: both nc and nslookup failed to resolve %s", testDomain)
}

func (m *Manager) parseTargetURLAndResolveIP(targetURL string, client SSHClient) (string, *url.URL, error) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse target URL: %w", err)
	}

	hostName := parsedURL.Hostname()
	if hostName == "" || hostName == "localhost" {
		return "", nil, fmt.Errorf("target URL must contain a valid IP or hostname (got %s)", hostName)
	}

	hostIP, err := m.resolveIP(hostName, client)
	if err != nil {
		return "", nil, fmt.Errorf("cannot resolve target hostname: %w", err)
	}

	return hostIP, parsedURL, nil
}

func (m *Manager) addTemporaryHostEntry(client SSHClient, deviceIP, testDomain, testEntry, rwCmd string) error {
	hostsContent, err := client.Run("cat /etc/hosts")
	if err != nil {
		return fmt.Errorf("failed to read /etc/hosts: %w", err)
	}

	if strings.Contains(hostsContent, testDomain) {
		lines := strings.Split(hostsContent, "\n")

		var newLines []string

		for _, line := range lines {
			if line != "" && !strings.Contains(line, testDomain) {
				newLines = append(newLines, line)
			}
		}

		hostsContent = strings.Join(newLines, "\n")
		if len(newLines) > 0 {
			hostsContent += "\n"
		}
	}

	_, _ = client.Run(rwCmd)

	if hostsContent != "" && !strings.HasSuffix(hostsContent, "\n") {
		hostsContent += "\n"
	}

	newHostsContent := hostsContent + testEntry + "\n"
	if uploadErr := client.UploadContent([]byte(newHostsContent), "/etc/hosts"); uploadErr != nil {
		return fmt.Errorf("failed to add test entry to /etc/hosts: %w", uploadErr)
	}

	fmt.Printf("Updated /etc/hosts on %s with test entry:\n%s\n", deviceIP, newHostsContent)

	return nil
}

func (m *Manager) cleanupTemporaryHostEntry(client SSHClient, testDomain, rwCmd string) {
	currentContent, _ := client.Run("cat /etc/hosts")
	lines := strings.Split(currentContent, "\n")

	var newLines []string

	for _, line := range lines {
		if line != "" && !strings.Contains(line, testDomain) {
			newLines = append(newLines, line)
		}
	}

	finalContent := strings.Join(newLines, "\n")
	if len(newLines) > 0 {
		finalContent += "\n"
	}

	_, _ = client.Run(rwCmd)
	_ = client.UploadContent([]byte(finalContent), "/etc/hosts")
}

func (m *Manager) runHTTPRedirectionTest(client SSHClient, parsedURL *url.URL, testDomain string) (string, error) {
	httpTestURL := fmt.Sprintf("http://%s:%s/health", testDomain, parsedURL.Port())
	if parsedURL.Port() == "" || parsedURL.Port() == "80" {
		httpTestURL = fmt.Sprintf("http://%s/health", testDomain)
	}

	cmd := fmt.Sprintf("curl --max-time 15 --connect-timeout 10 -v -s -L %s", httpTestURL)

	output, err := client.Run(cmd)
	if err != nil {
		return output, fmt.Errorf("hosts redirection HTTP test failed: %w", err)
	}

	return output, nil
}

func (m *Manager) runHTTPSRedirectionTest(client SSHClient, testDomain string) (string, error) {
	httpsPort := os.Getenv("HTTPS_PORT")
	if httpsPort == "" {
		httpsPort = "8443"
	}

	httpsTestURL := fmt.Sprintf("https://%s:%s/health", testDomain, httpsPort)
	if httpsPort == "443" {
		httpsTestURL = fmt.Sprintf("https://%s/health", testDomain)
	}

	caPEM, err := os.ReadFile(m.Crypto.GetCACertPath())
	if err != nil {
		return "", fmt.Errorf("failed to read CA cert for HTTPS test: %w", err)
	}

	caPath := "/tmp/soundtouch-test-ca.crt"
	if err := client.UploadContent(caPEM, caPath); err != nil {
		return "", fmt.Errorf("failed to upload temporary CA for HTTPS test: %w", err)
	}

	defer func() {
		_, _ = client.Run("rm " + caPath)
	}()

	httpsCmd := fmt.Sprintf("curl --max-time 15 --connect-timeout 10 -v -s -L --cacert %s %s", caPath, httpsTestURL)

	return client.Run(httpsCmd)
}

// TestConnection performs a connection check from the device to the server.
func (m *Manager) TestConnection(deviceIP, targetURL string, useExplicitCA bool) (string, error) {
	client := m.NewSSH(deviceIP)

	caPath := ""

	if useExplicitCA {
		// Temporary upload CA to device
		caPEM, err := os.ReadFile(m.Crypto.GetCACertPath())
		if err != nil {
			return "", fmt.Errorf("failed to read CA cert: %w", err)
		}

		caPath = "/tmp/soundtouch-test-ca.crt"
		if err := client.UploadContent(caPEM, caPath); err != nil {
			return "", fmt.Errorf("failed to upload temporary CA: %w", err)
		}

		defer func() {
			_, _ = client.Run("rm " + caPath)
		}()
	}

	cmd := fmt.Sprintf("curl --max-time 15 --connect-timeout 10 -v -s -L %s", targetURL)
	if useExplicitCA {
		cmd += " --cacert " + caPath
	}

	output, err := client.Run(cmd)
	if err != nil {
		return output, fmt.Errorf("connection test failed: %w", err)
	}

	return output, nil
}

// GetResolvedIP returns the resolved IP for a hostname, attempting to resolve it from any connected device first.
func (m *Manager) GetResolvedIP(host string) string {
	ip, _ := m.resolveIP(host, nil)
	return ip
}

// resolveIP resolves a hostname to an IP address.
// It first tries to resolve from the device via SSH ping (authoritative for migration).
// If that fails, it falls back to resolving from the service itself.
// An error is returned whenever the SSH ping did not produce the IP, so callers that
// write config to the device can abort rather than risk writing an unresolvable hostname.
func (m *Manager) resolveIP(host string, client SSHClient) (string, error) {
	if net.ParseIP(host) != nil {
		return host, nil
	}

	// 1. Try resolving FROM the device via SSH (authoritative: gives the IP the device will actually use)
	if client != nil {
		// Use ping to resolve hostname on the device.
		// Busybox ping output usually looks like: PING host (1.2.3.4): 56 data bytes
		output, err := client.Run(fmt.Sprintf("ping -c 1 %s", host))
		if err == nil {
			// Extract IP from parentheses: (1.2.3.4)
			start := strings.Index(output, "(")

			end := strings.Index(output, ")")
			if start != -1 && end > start {
				ip := output[start+1 : end]
				if net.ParseIP(ip) != nil {
					fmt.Printf("Resolved %s to %s from device\n", host, ip)
					return ip, nil
				}
			}
		}
	}

	// 2. Fallback: resolve FROM the service itself (unreliable for migration — NAT/split-DNS may differ)
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("cannot resolve %q: SSH ping from device failed and service-side DNS lookup also failed", host)
	}

	// Prefer IPv4
	var resolved string

	for _, ip := range ips {
		if ip.To4() != nil {
			resolved = ip.String()
			break
		}
	}

	if resolved == "" {
		resolved = ips[0].String()
	}

	return resolved, fmt.Errorf("resolved %q to %s from service, not from device — result may be wrong if NAT or split-DNS is in use", host, resolved)
}

// SyncDeviceData fetches presets, recents and sources from the device and saves them to the datastore.
func (m *Manager) SyncDeviceData(deviceIP string) error {
	// 1. Fetch info to get Serial Number (account identifier)
	info, err := m.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	log.Printf("Starting sync for device at %s: Name='%s', DeviceID='%s', SerialNumber='%s'",
		deviceIP, info.Name, info.DeviceID, info.SerialNumber)

	accountID := ""

	// Use deviceID from /info as canonical identifier (MAC address)
	deviceID := info.DeviceID
	if deviceID == "" {
		log.Printf("No deviceID found in /info response for device '%s' at %s", info.Name, deviceIP)
		return fmt.Errorf("no deviceID found in /info response for device at %s - cannot sync without canonical device identifier", deviceIP)
	}

	log.Printf("Using deviceID '%s' for sync operations (MAC address from /info)", deviceID)

	if info.MargeAccountUUID != "" {
		accountID = info.MargeAccountUUID
	}

	if accountID == "" {
		// Try to find account ID from existing device entries if info didn't have it
		devices, _ := m.DataStore.ListAllDevices()
		for i := range devices {
			if devices[i].DeviceSerialNumber == info.SerialNumber || devices[i].DeviceID == info.DeviceID {
				accountID = devices[i].AccountID
				break
			}
		}
	}

	if accountID == "" {
		accountID = "default"
	}

	// 2. Fetch Presets from :8090
	m.syncPresets(deviceIP, accountID, deviceID)

	// 3. Fetch Recents from :8090
	m.syncRecents(deviceIP, accountID, deviceID)

	// 4. Fetch Sources
	m.syncSources(deviceIP, accountID, deviceID)

	// 5. Create off-device backup of system configuration
	_ = m.BackupConfigOffDevice(deviceIP)

	return nil
}

func (m *Manager) syncPresets(deviceIP, accountID, deviceID string) {
	presetsURL := fmt.Sprintf("http://%s:8090/presets", deviceIP)
	if _, _, splitErr := net.SplitHostPort(deviceIP); splitErr == nil {
		presetsURL = fmt.Sprintf("http://%s/presets", deviceIP)
	}

	log.Printf("[SYNC] Syncing presets for %s", deviceIP)

	resp, err := m.HTTPGet(presetsURL)
	if err != nil {
		log.Printf("[SYNC_ERR] Failed to fetch presets for %s: %v", deviceIP, err)
		return
	}

	defer func() { _ = resp.Body.Close() }()

	var ps models.Presets
	if decodeErr := xml.NewDecoder(resp.Body).Decode(&ps); decodeErr != nil {
		return
	}

	var servicePresets []models.ServicePreset

	for _, p := range ps.Preset {
		if p.ContentItem == nil {
			continue
		}

		createdOn := ""
		if p.CreatedOn != nil {
			createdOn = strconv.FormatInt(*p.CreatedOn, 10)
		}

		updatedOn := ""
		if p.UpdatedOn != nil {
			updatedOn = strconv.FormatInt(*p.UpdatedOn, 10)
		}

		servicePresets = append(servicePresets, models.ServicePreset{
			ServiceContentItem: models.ServiceContentItem{
				ID:            strconv.Itoa(p.ID),
				Name:          p.ContentItem.ItemName,
				Source:        p.ContentItem.Source,
				Type:          p.ContentItem.Type,
				Location:      p.ContentItem.Location,
				SourceAccount: p.ContentItem.SourceAccount,
				SourceID:      "", // Preset doesn't have SourceID in ContentItem usually
				IsPresetable:  strconv.FormatBool(p.ContentItem.IsPresetable),
			},
			ID:           strconv.Itoa(p.ID),
			ButtonNumber: strconv.Itoa(p.ID),
			ContainerArt: p.ContentItem.ContainerArt,
			CreatedOn:    createdOn,
			UpdatedOn:    updatedOn,
		})
	}

	_ = m.DataStore.SavePresets(accountID, deviceID, servicePresets)
}

func (m *Manager) syncRecents(deviceIP, accountID, deviceID string) {
	recentsURL := fmt.Sprintf("http://%s:8090/recents", deviceIP)
	if _, _, splitErr := net.SplitHostPort(deviceIP); splitErr == nil {
		recentsURL = fmt.Sprintf("http://%s/recents", deviceIP)
	}

	resp, err := m.HTTPGet(recentsURL)
	if err != nil {
		return
	}

	defer func() { _ = resp.Body.Close() }()

	var rr models.RecentsResponse
	if decodeErr := xml.NewDecoder(resp.Body).Decode(&rr); decodeErr != nil {
		return
	}

	var serviceRecents []models.ServiceRecent

	for _, r := range rr.Items {
		if r.ContentItem == nil {
			continue
		}

		serviceRecents = append(serviceRecents, models.ServiceRecent{
			ServiceContentItem: models.ServiceContentItem{
				ID:            r.ID,
				Name:          r.ContentItem.ItemName,
				Source:        r.ContentItem.Source,
				Type:          r.ContentItem.Type,
				Location:      r.ContentItem.Location,
				SourceAccount: r.ContentItem.SourceAccount,
				SourceID:      "", // RecentsResponseItem doesn't have SourceID usually
				IsPresetable:  strconv.FormatBool(r.ContentItem.IsPresetable),
				ContainerArt:  r.ContentItem.ContainerArt,
			},
			DeviceID: r.DeviceID,
			UtcTime:  strconv.FormatInt(r.UTCTime, 10),
		})
	}

	_ = m.DataStore.SaveRecents(accountID, deviceID, serviceRecents)
}

func (m *Manager) syncSources(deviceIP, accountID, deviceID string) {
	client := m.NewSSH(deviceIP)

	sourcesXML, err := client.Run("cat /mnt/nv/BoseApp-Persistence/1/Sources.xml")
	if err == nil && sourcesXML != "" {
		var srs struct {
			Sources []models.ConfiguredSource `xml:"source"`
		}
		if xmlErr := xml.Unmarshal([]byte(sourcesXML), &srs); xmlErr == nil {
			// After unmarshaling from SSH, ensure legacy fields are synced for internal use
			for i := range srs.Sources {
				s := &srs.Sources[i]
				s.SourceKeyType = s.SourceKey.Type
				s.SourceKeyAccount = s.SourceKey.Account
			}

			_ = m.DataStore.SaveConfiguredSources(accountID, deviceID, srs.Sources)

			return
		}
	}

	// Fallback to :8090/sources
	sourcesURL := fmt.Sprintf("http://%s:8090/sources", deviceIP)
	if _, _, splitErr := net.SplitHostPort(deviceIP); splitErr == nil {
		sourcesURL = fmt.Sprintf("http://%s/sources", deviceIP)
	}

	resp, err := m.HTTPGet(sourcesURL)
	if err != nil {
		return
	}

	defer func() { _ = resp.Body.Close() }()

	var srs models.Sources
	if decodeErr := xml.NewDecoder(resp.Body).Decode(&srs); decodeErr == nil {
		var configuredSources []models.ConfiguredSource

		for _, s := range srs.SourceItem {
			cs := models.ConfiguredSource{
				DisplayName: s.DisplayName,
				Secret:      "",
				SecretType:  "",
			}
			if s.Status == "READY" {
				cs.SecretType = "token"
			}

			if s.Source == constants.ProviderSpotify {
				cs.SecretType = "token_version_3"
			}

			cs.SourceKey.Type = s.Source
			cs.SourceKey.Account = s.SourceAccount
			// Also set legacy fields for now
			cs.SourceKeyType = s.Source
			cs.SourceKeyAccount = s.SourceAccount

			configuredSources = append(configuredSources, cs)
		}

		_ = m.DataStore.SaveConfiguredSources(accountID, deviceID, configuredSources)
	}
}
