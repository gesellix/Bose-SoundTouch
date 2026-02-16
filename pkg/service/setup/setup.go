// Package setup contains speaker migration and configuration helpers.
package setup

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"

	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/ssh"
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
}

// SSHClient defines the interface for SSH operations.
type SSHClient interface {
	Run(command string) (string, error)
	UploadContent(content []byte, remotePath string) error
}

// Manager handles the migration of speakers to the service.
type Manager struct {
	ServerURL string
	DataStore *datastore.DataStore
	Crypto    *certmanager.CertificateManager
	NewSSH    func(host string) SSHClient

	// GetDNSRunning is an optional callback to check the actual state of the DNS server.
	GetDNSRunning func() (bool, string)
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
	}
}

// DeviceInfoXML represents the XML structure from :8090/info
type DeviceInfoXML struct {
	XMLName          xml.Name `xml:"info" json:"-"`
	DeviceID         string   `xml:"deviceID,attr" json:"deviceID"`
	Name             string   `xml:"name" json:"name"`
	Type             string   `xml:"type" json:"type"`
	MaccAddress      string   `xml:"maccAddress" json:"maccAddress"`
	SoftwareVer      string   `xml:"-" json:"softwareVersion"`
	SerialNumber     string   `xml:"-" json:"serialNumber"`
	MargeAccountUUID string   `xml:"margeAccountUUID" json:"margeAccountUUID"`
	Components       []struct {
		Category        string `xml:"componentCategory"`
		SoftwareVersion string `xml:"softwareVersion"`
		SerialNumber    string `xml:"serialNumber"`
	} `xml:"components>component" json:"-"`
}

// GetLiveDeviceInfo fetches live information from the speaker's :8090/info endpoint.
func (m *Manager) GetLiveDeviceInfo(deviceIP string) (*DeviceInfoXML, error) {
	infoURL := fmt.Sprintf("http://%s:8090/info", deviceIP)
	// For testing, if the IP already contains a port, don't append :8090
	if host, _, err := net.SplitHostPort(deviceIP); err == nil {
		infoURL = fmt.Sprintf("http://%s/info", deviceIP)
		_ = host
	}

	resp, err := http.Get(infoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch info from %s: %w", infoURL, err)
	}

	defer func() { _ = resp.Body.Close() }()

	var infoXML DeviceInfoXML
	if err := xml.NewDecoder(resp.Body).Decode(&infoXML); err != nil {
		return nil, fmt.Errorf("failed to decode info XML from %s: %w", infoURL, err)
	}

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

	return &infoXML, nil
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
		MargeServerUrl:             fmt.Sprintf("%s/marge", targetURL),
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

	// 2b. Initial planned hosts config
	parsedURL, err := url.Parse(targetURL)
	if err == nil {
		hostName := parsedURL.Hostname()
		if hostName != "" && hostName != "localhost" {
			client := m.NewSSH(deviceIP)
			hostIP := m.resolveIP(hostName, client)

			// Predicted aftertouch.resolv.conf
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
			}

			var hostsLines []string
			for _, domain := range domains {
				hostsLines = append(hostsLines, fmt.Sprintf("%s\t%s", hostIP, domain))
			}

			summary.PlannedHosts = strings.Join(hostsLines, "\n")
		}
	}

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
	if parsedURL, err := url.Parse(targetURL); err == nil {
		hostIP := parsedURL.Hostname()
		if hostIP != "" {
			// Find HTTPS port from environment or default
			httpsPort := os.Getenv("HTTPS_PORT")
			if httpsPort == "" {
				httpsPort = "8443"
			}

			summary.ServerHTTPSURL = fmt.Sprintf("https://%s:%s/health", hostIP, httpsPort)
		}
	}

	// 6. Check if migrated
	m.checkIsMigrated(summary, deviceIP)

	return summary, nil
}

// checkIsMigrated determines if the device is already migrated to AfterTouch.
func (m *Manager) checkIsMigrated(summary *MigrationSummary, deviceIP string) {
	if !summary.SSHSuccess {
		return
	}

	// Case 1: XML Migration
	// Check if any URL in the current config points to our server (targetURL)
	if summary.ParsedCurrentConfig != nil {
		targetURL := m.ServerURL
		// Strip protocol for comparison if needed, or just check for substring
		parsedTarget, err := url.Parse(targetURL)
		if err == nil {
			targetHost := parsedTarget.Hostname()
			if strings.Contains(summary.ParsedCurrentConfig.MargeServerUrl, targetHost) ||
				strings.Contains(summary.ParsedCurrentConfig.StatsServerUrl, targetHost) ||
				strings.Contains(summary.ParsedCurrentConfig.SwUpdateUrl, targetHost) ||
				strings.Contains(summary.ParsedCurrentConfig.BmxRegistryUrl, targetHost) {
				summary.IsMigrated = true
				return
			}
		}
	}

	// Case 2: /etc/hosts + Trust CA Migration
	// Check if /etc/hosts contains redirections for Bose domains
	client := m.NewSSH(deviceIP)

	hostsContent, err := client.Run("cat /etc/hosts")
	if err == nil {
		boseDomains := []string{
			"streaming.bose.com",
			"updates.bose.com",
			"stats.bose.com",
			"bmx.bose.com",
		}
		for _, domain := range boseDomains {
			if strings.Contains(hostsContent, domain) {
				// If CA is also trusted, it's a strong indicator of migration
				if summary.CACertTrusted {
					summary.IsMigrated = true
					return
				}
			}
		}
	}

	// Case 3: /etc/resolv.conf Migration (including Aftertouch hook)
	// Check if /etc/resolv.conf contains our target nameserver OR if hook marker exists
	if summary.SSHSuccess {
		// Check for aftertouch.resolv.conf
		if _, err := client.Run("[ -f /mnt/nv/aftertouch.resolv.conf ]"); err == nil {
			if summary.CACertTrusted {
				summary.IsMigrated = true
				return
			}
		}

		if summary.CurrentResolvConf != "" {
			targetURL := m.ServerURL

			parsedTarget, err := url.Parse(targetURL)
			if err == nil {
				targetHost := parsedTarget.Hostname()
				if strings.Contains(summary.CurrentResolvConf, targetHost) {
					if summary.CACertTrusted {
						summary.IsMigrated = true
						return
					}
				}
			}
		}
	}
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

// applyProxyOptions modifies planned config based on proxy options
func (m *Manager) applyProxyOptions(plannedCfg *PrivateCfg, proxyURL string, options map[string]string, currentCfg *PrivateCfg) {
	if proxyURL == "" || currentCfg == nil {
		return
	}

	if options["marge"] == "upstream" && currentCfg.MargeServerUrl != "" {
		plannedCfg.MargeServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.MargeServerUrl)
	}

	if options["stats"] == "upstream" && currentCfg.StatsServerUrl != "" {
		plannedCfg.StatsServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.StatsServerUrl)
	}

	if options["sw_update"] == "upstream" && currentCfg.SwUpdateUrl != "" {
		plannedCfg.SwUpdateUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.SwUpdateUrl)
	}

	if options["bmx"] == "upstream" && currentCfg.BmxRegistryUrl != "" {
		plannedCfg.BmxRegistryUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.BmxRegistryUrl)
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
		MargeServerUrl:             fmt.Sprintf("%s/marge", targetURL),
		StatsServerUrl:             targetURL,
		SwUpdateUrl:                fmt.Sprintf("%s/updates/soundtouch", targetURL),
		UsePandoraProductionServer: true,
		IsZeroconfEnabled:          true,
		SaveMargeCustomerReport:    false,
		BmxRegistryUrl:             fmt.Sprintf("%s/bmx/registry/v1/services", targetURL),
	}

	// If we have a proxyURL and can read current config, use it
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
		// Try to copy existing config to .original, ensuring filesystem is writable
		if output, err := client.Run(fmt.Sprintf("%s && cp %s %s.original", rwCmd, remotePath, remotePath)); err != nil {
			logs += fmt.Sprintf("Warning: failed to cp backup config: %v (output: %s)\n", err, output)
			fmt.Printf("Warning: failed to cp backup config: %v (output: %s)\n", err, output)
			// Fallback to manual upload if cp failed (might not have cp?)
			if config, err := client.Run(fmt.Sprintf("cat %s", remotePath)); err == nil && config != "" {
				if err := client.UploadContent([]byte(config), remotePath+".original"); err != nil {
					logs += "Warning: failed to upload backup config: " + err.Error() + "\n"
					fmt.Printf("Warning: failed to upload backup config: %v\n", err)
				} else {
					logs += "Uploaded backup config via fallback\n"
				}
			}
		} else {
			logs += "Copied backup config to .original\n"
		}
	} else {
		logs += "Backup .original already exists\n"
	}

	// 1. Upload the configuration (rw is handled by calling it before if needed, but UploadContent uses cat > which needs rw)
	// We'll wrap the upload in a way that EnsureRemoteServices and others might benefit,
	// but UploadContent is a separate method. We should probably add rw to UploadContent or call it before.
	// Actually, let's call rw before UploadContent here.
	out, _ = client.Run(rwCmd)

	logs += rwCmd + ": " + out + "\n"
	if err := client.UploadContent(xmlContent, remotePath); err != nil {
		return logs, fmt.Errorf("failed to upload config: %w", err)
	}

	logs += "Uploaded new configuration to " + remotePath + "\n"

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

	hostIP := m.resolveIP(hostName, client)
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
	}

	hostsContent, err := client.Run("cat /etc/hosts")

	logs += "cat /etc/hosts: " + hostsContent + "\n"
	if err != nil {
		return logs, fmt.Errorf("failed to read /etc/hosts: %w", err)
	}

	lines := strings.Split(hostsContent, "\n")

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

	hostsContent = strings.Join(newLines, "\n")
	if !strings.HasSuffix(hostsContent, "\n") {
		hostsContent += "\n"
	}

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

	fmt.Printf("Updated /etc/hosts on %s:\n%s\n", deviceIP, hostsContent)

	// 4. Inject CA Certificate
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

	hostIP := m.resolveIP(hostName, client)
	logs += fmt.Sprintf("Resolved %s to %s\n", hostName, hostIP)

	// 2. Prepare /mnt/nv/aftertouch.resolv.conf content
	resolvContent := fmt.Sprintf("# Created by Aftertouch/SoundTouch-Service\n# Priority nameserver for Bose service redirection\nnameserver %s\n", hostIP)

	// 3. Upload /mnt/nv/aftertouch.resolv.conf
	// Ensure /mnt/nv exists
	_, _ = client.Run("mkdir -p /mnt/nv")

	if err := client.UploadContent([]byte(resolvContent), "/mnt/nv/aftertouch.resolv.conf"); err != nil {
		return logs, fmt.Errorf("failed to upload /mnt/nv/aftertouch.resolv.conf: %w", err)
	}

	logs += "Uploaded /mnt/nv/aftertouch.resolv.conf\n"

	// 4. Update /mnt/nv/rc.local with idempotent patch
	rcLocalPath := "/mnt/nv/rc.local"
	targetDHCPFile := "/etc/udhcpc.d/50default"
	hookMarker := "/mnt/nv/aftertouch.resolv.conf"

	// Check if rc.local exists and read it
	currentRcLocal, rcErr := client.Run(fmt.Sprintf("cat %s", rcLocalPath))
	if rcErr != nil {
		currentRcLocal = ""
	}

	patchLogic := fmt.Sprintf(`
# Aftertouch DNS hook: prioritizes our custom nameserver if it exists
if [ -f "%s" ]; then
    if [ -f "%s" ] && ! grep -q "%s" "%s"; then
        logger -t "aftertouch" "Patching %s with Aftertouch DNS hook"
        sed -i '/echo "search \$domain"/a \        [ -f '"%s"' ] && cat '"%s"' && dns=""' "%s"
    fi
    targetScript="/opt/Bose/udhcpc.script"
    if [ -f "$targetScript" ] && ! grep -q "%s" "$targetScript"; then
        logger -t "aftertouch" "Patching $targetScript with Aftertouch DNS hook"
        sed -i '/echo "search \$search_list # \$interface" >> \$RESOLV_CONF/a \                [ -f '"%s"' ] && cat '"%s"' >> '"$RESOLV_CONF"' && dns=""' "$targetScript"
    fi
fi
`, hookMarker, targetDHCPFile, hookMarker, targetDHCPFile, targetDHCPFile, hookMarker, hookMarker, targetDHCPFile, hookMarker, hookMarker, hookMarker)

	if !strings.Contains(currentRcLocal, hookMarker) {
		newRcLocal := currentRcLocal
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
	} else {
		logs += fmt.Sprintf("%s already contains Aftertouch hook logic\n", rcLocalPath)
	}

	// 5. Apply patch immediately to /etc/udhcpc.d/50default
	out, _ := client.Run(rwCmd)
	logs += rwCmd + ": " + out + "\n"

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
		logs += fmt.Sprintf("Failed to apply patch immediately to %s: %v\n", targetDHCPFile, err)
	} else {
		logs += fmt.Sprintf("Applied patch to %s\n", targetDHCPFile)
	}

	// Apply patch immediately to /opt/Bose/udhcpc.script if it exists
	targetScript := "/opt/Bose/udhcpc.script"
	if _, err := client.Run(fmt.Sprintf("[ -f %s ]", targetScript)); err == nil {
		// Backup if it doesn't exist
		if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", targetScript)); err != nil {
			out, _ := client.Run(fmt.Sprintf("cp %s %s.original", targetScript, targetScript))
			logs += fmt.Sprintf("cp %s %s.original: %s\n", targetScript, targetScript, out)
		} else {
			// If backup exists, revert to it first to ensure we start from a clean state
			_, _ = client.Run(fmt.Sprintf("cp %s.original %s", targetScript, targetScript))
		}

		patchCmdScript := fmt.Sprintf("sed -i '/echo \"search \\$search_list # \\$interface\" >> \\$RESOLV_CONF/a \\                [ -f '\"%s\"' ] && cat '\"%s\"' >> '\"$RESOLV_CONF\"' && dns=\"\"' %s", hookMarker, hookMarker, targetScript)
		if _, err := client.Run(patchCmdScript); err != nil {
			logs += fmt.Sprintf("Failed to apply patch immediately to %s: %v\n", targetScript, err)
		} else {
			logs += fmt.Sprintf("Applied patch to %s\n", targetScript)
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

	aftertouchConfPath := "/mnt/nv/aftertouch.resolv.conf"
	rcLocalPath := "/mnt/nv/rc.local"
	targetDHCPFile := "/etc/udhcpc.d/50default"

	if _, err := client.Run(fmt.Sprintf("[ -f %s ]", aftertouchConfPath)); err == nil {
		logs += fmt.Sprintf("Removing %s\n", aftertouchConfPath)
		fmt.Printf("Removing %s\n", aftertouchConfPath)
		_, _ = client.Run(fmt.Sprintf("rm %s", aftertouchConfPath))
	}

	if currentRcLocal, err := client.Run(fmt.Sprintf("cat %s", rcLocalPath)); err == nil {
		// Remove "cat: can't open..." error message if it was accidentally saved in the file
		if strings.Contains(currentRcLocal, "cat: can't open") {
			logs += fmt.Sprintf("Removing corrupted %s\n", rcLocalPath)
			_, _ = client.Run(fmt.Sprintf("rm %s", rcLocalPath))

			return logs
		}

		if strings.Contains(currentRcLocal, aftertouchConfPath) || strings.Contains(currentRcLocal, "# Aftertouch DNS hook") {
			logs += fmt.Sprintf("Removing Aftertouch hook logic from %s\n", rcLocalPath)
			fmt.Printf("Removing Aftertouch hook logic from %s\n", rcLocalPath)

			// Simple removal: filter out lines between the marker and the 'fi'
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

			newRcLocal := strings.Join(newLines, "\n")
			if err := client.UploadContent([]byte(newRcLocal), rcLocalPath); err != nil {
				fmt.Printf("Warning: failed to update %s: %v\n", rcLocalPath, err)
			}
		}
	}

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

// Reboot reboots the speaker at the given IP.
func (m *Manager) Reboot(deviceIP string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	fmt.Printf("Rebooting speaker at %s\n", deviceIP)

	out, err := client.Run(fmt.Sprintf("%s && reboot", rwCmd))
	if err != nil {
		return out, fmt.Errorf("failed to reboot speaker: %w", err)
	}

	return out, nil
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

	return m.resolveIP(hostName, client), parsedURL, nil
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
	return m.resolveIP(host, nil)
}

func (m *Manager) resolveIP(host string, client SSHClient) string {
	if net.ParseIP(host) != nil {
		return host
	}

	// 1. Try resolving FROM the device via SSH (best for containers/NAT)
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
					return ip
				}
			}
		}
	}

	// 2. Fallback: resolve FROM the service itself
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return host // Fallback to host if resolution fails
	}

	// Prefer IPv4
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String()
		}
	}

	return ips[0].String()
}

// SyncDeviceData fetches presets, recents and sources from the device and saves them to the datastore.
func (m *Manager) SyncDeviceData(deviceIP string) error {
	// 1. Fetch info to get Serial Number (account identifier)
	info, err := m.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	accountID := ""

	deviceID := info.SerialNumber
	if deviceID == "" {
		deviceID = deviceIP
	}

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

	resp, err := http.Get(presetsURL)
	if err != nil {
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

	resp, err := http.Get(recentsURL)
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
			},
			DeviceID:     r.DeviceID,
			UtcTime:      strconv.FormatInt(r.UTCTime, 10),
			ContainerArt: r.ContentItem.ContainerArt,
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

	resp, err := http.Get(sourcesURL)
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
				ID:          s.Source,
				SecretType:  string(s.Status),
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
