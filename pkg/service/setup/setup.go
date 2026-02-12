// Package setup contains speaker migration and configuration helpers.
package setup

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/service/crypto"
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
	FirmwareVersion          string      `json:"firmware_version,omitempty"`
	CACertTrusted            bool        `json:"ca_cert_trusted"`
	ServerHTTPSURL           string      `json:"server_https_url,omitempty"`
}

// SSHClient defines the interface for SSH operations.
type SSHClient interface {
	Run(command string) (string, error)
	UploadContent(content []byte, remotePath string) error
}

// Manager handles the migration of speakers to the soundcork service.
type Manager struct {
	ServerURL string
	DataStore *datastore.DataStore
	Crypto    *crypto.CertificateManager
	NewSSH    func(host string) SSHClient
}

// NewManager creates a new Manager with the given base server URL.
func NewManager(serverURL string, ds *datastore.DataStore, cm *crypto.CertificateManager) *Manager {
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
	XMLName      xml.Name `xml:"info" json:"-"`
	DeviceID     string   `xml:"deviceID,attr" json:"deviceID"`
	Name         string   `xml:"name" json:"name"`
	Type         string   `xml:"type" json:"type"`
	MaccAddress  string   `xml:"maccAddress" json:"maccAddress"`
	SoftwareVer  string   `xml:"-" json:"softwareVersion"`
	SerialNumber string   `xml:"-" json:"serialNumber"`
	Components   []struct {
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
			domains := []string{
				"streaming.bose.com",
				"updates.bose.com",
				"stats.bose.com",
				"bmx.bose.com",
				"content.api.bose.io",
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

	return summary, nil
}

// populateDeviceInfo fills in device information from datastore and live info
func (m *Manager) populateDeviceInfo(summary *MigrationSummary, deviceIP string) {
	// Populate from datastore if available
	if m.DataStore != nil {
		devices, err := m.DataStore.ListAllDevices()
		if err == nil {
			for _, d := range devices {
				if d.IPAddress != deviceIP {
					continue
				}

				summary.DeviceName = d.Name
				summary.DeviceModel = d.ProductCode
				summary.DeviceSerial = d.DeviceSerialNumber
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

	if options["marge"] == "original" && currentCfg.MargeServerUrl != "" {
		plannedCfg.MargeServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.MargeServerUrl)
	}

	if options["stats"] == "original" && currentCfg.StatsServerUrl != "" {
		plannedCfg.StatsServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.StatsServerUrl)
	}

	if options["sw_update"] == "original" && currentCfg.SwUpdateUrl != "" {
		plannedCfg.SwUpdateUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.SwUpdateUrl)
	}

	if options["bmx"] == "original" && currentCfg.BmxRegistryUrl != "" {
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

	client := m.NewSSH(deviceIP)
	bundlePath := "/etc/pki/tls/certs/ca-bundle.crt"
	// Use grep to check for the certificate data in the bundle
	_, err = client.Run(fmt.Sprintf("grep -F %q %s", certData, bundlePath))
	if err == nil {
		summary.CACertTrusted = true
	}
}

// MigrateSpeaker configures the speaker at the given IP to use this soundcork service.
func (m *Manager) MigrateSpeaker(deviceIP, targetURL, proxyURL string, options map[string]string, method MigrationMethod) error {
	if targetURL == "" {
		targetURL = m.ServerURL
	}

	if method == "" {
		method = MigrationMethodXML
	}

	if method == MigrationMethodHosts {
		return m.migrateViaHosts(deviceIP, targetURL)
	}

	if err := m.EnsureRemoteServices(deviceIP); err != nil {
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
	client := m.NewSSH(deviceIP)
	if currentConfig, err := client.Run(fmt.Sprintf("cat %s", SoundTouchSdkPrivateCfgPath)); err == nil && currentConfig != "" {
		var currentCfg PrivateCfg
		if xml.Unmarshal([]byte(currentConfig), &currentCfg) == nil {
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
		return fmt.Errorf("failed to marshal XML: %w", err)
	}

	// Add XML header
	xmlContent = append([]byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"), xmlContent...)

	// 0. Backup original config if it doesn't exist
	remotePath := SoundTouchSdkPrivateCfgPath
	rwCmd := "(rw || mount -o remount,rw /)"

	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", remotePath)); err != nil {
		fmt.Printf("Backing up original config to %s.original\n", remotePath)
		// Try to copy existing config to .original, ensuring filesystem is writable
		if output, err := client.Run(fmt.Sprintf("%s && cp %s %s.original", rwCmd, remotePath, remotePath)); err != nil {
			fmt.Printf("Warning: failed to cp backup config: %v (output: %s)\n", err, output)
			// Fallback to manual upload if cp failed (might not have cp?)
			if config, err := client.Run(fmt.Sprintf("cat %s", remotePath)); err == nil && config != "" {
				if err := client.UploadContent([]byte(config), remotePath+".original"); err != nil {
					fmt.Printf("Warning: failed to upload backup config: %v\n", err)
				}
			}
		}
	}

	// 1. Upload the configuration (rw is handled by calling it before if needed, but UploadContent uses cat > which needs rw)
	// We'll wrap the upload in a way that EnsureRemoteServices and others might benefit,
	// but UploadContent is a separate method. We should probably add rw to UploadContent or call it before.
	// Actually, let's call rw before UploadContent here.
	_, _ = client.Run(rwCmd)
	if err := client.UploadContent(xmlContent, remotePath); err != nil {
		return fmt.Errorf("failed to upload config: %w", err)
	}

	// 2. Reboot the speaker (requires 'rw' command first to make filesystem writable)
	if _, err := client.Run(fmt.Sprintf("%s && reboot", rwCmd)); err != nil {
		return fmt.Errorf("failed to reboot speaker: %w", err)
	}

	return nil
}

// BackupConfig creates a backup of the current configuration on the speaker.
func (m *Manager) BackupConfig(deviceIP string) error {
	client := m.NewSSH(deviceIP)
	remotePath := SoundTouchSdkPrivateCfgPath
	rwCmd := "(rw || mount -o remount,rw /)"

	// Check if .original already exists
	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", remotePath)); err == nil {
		return fmt.Errorf("backup already exists at %s.original", remotePath)
	}

	// Try to copy on the device first (more reliable), ensuring filesystem is writable
	output, cpErr := client.Run(fmt.Sprintf("%s && cp %s %s.original", rwCmd, remotePath, remotePath))
	if cpErr == nil {
		return nil
	}

	fmt.Printf("Direct cp failed: %v (output: %s), falling back to cat+upload\n", cpErr, output)

	// Fallback to cat + upload
	config, err := client.Run(fmt.Sprintf("cat %s", remotePath))
	if err != nil || config == "" {
		return fmt.Errorf("failed to read current config: %w", err)
	}

	// Ensure rw before upload fallback
	_, _ = client.Run(rwCmd)
	if err := client.UploadContent([]byte(config), remotePath+".original"); err != nil {
		return fmt.Errorf("failed to upload backup config: %w", err)
	}

	return nil
}

// EnsureRemoteServices ensures that remote services are enabled on the device.
// It tries to create an empty file in one of the known valid locations.
func (m *Manager) EnsureRemoteServices(deviceIP string) error {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	// Try locations in order of preference
	locations := []string{
		"/etc/remote_services",
		"/mnt/nv/remote_services",
		"/tmp/remote_services",
	}

	for _, loc := range locations {
		// Try to make filesystem writable for each location that might need it
		// Combining rw && touch ensures it's attempted in the same sequence
		_, err := client.Run(fmt.Sprintf("%s && touch %s", rwCmd, loc))
		if err == nil {
			return nil
		}
		// If rw && touch failed, try just touch (e.g. for /tmp which doesn't need rw)
		_, err = client.Run(fmt.Sprintf("touch %s", loc))
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("failed to enable remote services in any of the locations: %v", locations)
}

func (m *Manager) migrateViaHosts(deviceIP, targetURL string) error {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	// 1. Parse targetURL to get IP for /etc/hosts
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("failed to parse target URL: %w", err)
	}

	hostName := parsedURL.Hostname()
	if hostName == "" || hostName == "localhost" {
		// Use a better guess if needed, but for now expect valid IP/hostname
		return fmt.Errorf("target URL must contain a valid IP or hostname (got %s)", hostName)
	}

	hostIP := m.resolveIP(hostName, client)

	// 2. Prepare /etc/hosts entries
	domains := []string{
		"streaming.bose.com",
		"updates.bose.com",
		"stats.bose.com",
		"bmx.bose.com",
		"content.api.bose.io",
	}

	hostsContent, err := client.Run("cat /etc/hosts")
	if err != nil {
		return fmt.Errorf("failed to read /etc/hosts: %w", err)
	}

	for _, domain := range domains {
		if !strings.Contains(hostsContent, domain) {
			entry := fmt.Sprintf("%s\t%s", hostIP, domain)

			if hostsContent != "" && !strings.HasSuffix(hostsContent, "\n") {
				hostsContent += "\n"
			}

			hostsContent += entry + "\n"
		}
	}

	// 3. Upload new /etc/hosts
	_, _ = client.Run(rwCmd)
	// Backup /etc/hosts if it doesn't exist
	if _, err := client.Run("[ -f /etc/hosts.original ]"); err != nil {
		_, _ = client.Run("cp /etc/hosts /etc/hosts.original")
	}

	if err := client.UploadContent([]byte(hostsContent), "/etc/hosts"); err != nil {
		return fmt.Errorf("failed to update /etc/hosts: %w", err)
	}

	fmt.Printf("Updated /etc/hosts on %s:\n%s\n", deviceIP, hostsContent)

	// 4. Inject CA Certificate
	summary := &MigrationSummary{}
	m.checkCACertTrusted(summary, deviceIP)

	if !summary.CACertTrusted {
		caCertPEM, err := os.ReadFile(m.Crypto.GetCACertPath())
		if err != nil {
			return fmt.Errorf("failed to read CA certificate: %w", err)
		}

		// Append to bundle
		bundlePath := "/etc/pki/tls/certs/ca-bundle.crt"
		_, _ = client.Run(rwCmd)

		// Backup bundle if it doesn't exist
		if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", bundlePath)); err != nil {
			_, _ = client.Run(fmt.Sprintf("cp %s %s.original", bundlePath, bundlePath))
		}

		// We use session.Run for append or similar, but client.Run uses CombinedOutput.
		// Let's use a temporary file and append it.
		tmpCertPath := "/tmp/local-ca.crt"
		if err := client.UploadContent(caCertPEM, tmpCertPath); err != nil {
			return fmt.Errorf("failed to upload CA cert to tmp: %w", err)
		}

		if _, err := client.Run(fmt.Sprintf("%s && cat %s >> %s && rm %s", rwCmd, tmpCertPath, bundlePath, tmpCertPath)); err != nil {
			return fmt.Errorf("failed to append CA cert to bundle: %w", err)
		}
	} else {
		fmt.Printf("CA certificate already trusted on %s, skipping injection\n", deviceIP)
	}

	// 5. Reboot
	if _, err := client.Run(fmt.Sprintf("%s && reboot", rwCmd)); err != nil {
		return fmt.Errorf("failed to reboot speaker: %w", err)
	}

	return nil
}

// RemoveRemoteServices removes remote services from the device by deleting the known remote_services files.
func (m *Manager) RemoveRemoteServices(deviceIP string) error {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	locations := []string{
		"/etc/remote_services",
		"/mnt/nv/remote_services",
		"/tmp/remote_services",
	}

	var errors []error

	for _, loc := range locations {
		// Try to make filesystem writable and remove the file
		_, err := client.Run(fmt.Sprintf("%s && rm -f %s", rwCmd, loc))
		if err != nil {
			// If rw && rm failed, try just rm (e.g. for /tmp)
			_, err = client.Run(fmt.Sprintf("rm -f %s", loc))
			if err != nil {
				errors = append(errors, fmt.Errorf("failed to remove %s: %w", loc, err))
			}
		}
	}

	if len(errors) == len(locations) {
		return fmt.Errorf("failed to remove remote services from any location: %v", errors)
	}

	return nil
}

// TestDomain is the fake domain used for preliminary redirection tests.
const TestDomain = "custom-test-api.bose.fake"

// TestHostsRedirection performs a preliminary check to see if /etc/hosts redirection works.
func (m *Manager) TestHostsRedirection(deviceIP, targetURL string) (string, error) {
	client := m.NewSSH(deviceIP)
	rwCmd := "(rw || mount -o remount,rw /)"

	// 1. Parse targetURL to get IP for /etc/hosts
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse target URL: %w", err)
	}

	hostName := parsedURL.Hostname()
	if hostName == "" || hostName == "localhost" {
		return "", fmt.Errorf("target URL must contain a valid IP or hostname (got %s)", hostName)
	}

	hostIP := m.resolveIP(hostName, client)

	testDomain := TestDomain
	testEntry := fmt.Sprintf("%s\t%s", hostIP, testDomain)

	// 2. Add temporary entry to /etc/hosts
	hostsContent, err := client.Run("cat /etc/hosts")
	if err != nil {
		return "", fmt.Errorf("failed to read /etc/hosts: %w", err)
	}

	if strings.Contains(hostsContent, testDomain) {
		// Even if it's there, let's make sure it's correct (pointing to the current hostIP)
		// but for now, if it's there, we just assume it's okay or from a previous failed cleanup.
		// Let's remove it and re-add to be sure.
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
	// Ensure hostsContent ends with a newline if not empty
	if hostsContent != "" && !strings.HasSuffix(hostsContent, "\n") {
		hostsContent += "\n"
	}

	newHostsContent := hostsContent + testEntry + "\n"
	if err := client.UploadContent([]byte(newHostsContent), "/etc/hosts"); err != nil {
		return "", fmt.Errorf("failed to add test entry to /etc/hosts: %w", err)
	}

	fmt.Printf("Updated /etc/hosts on %s with test entry:\n%s\n", deviceIP, newHostsContent)

	defer func() {
		// Clean up test entry
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
	}()

	// 3. Test connection to the fake domain
	// 3a. HTTP (for simplicity of redirection test)
	// We use the health check endpoint on the same port but with the fake domain
	httpTestURL := fmt.Sprintf("http://%s:%s/health", testDomain, parsedURL.Port())
	if parsedURL.Port() == "" {
		httpTestURL = fmt.Sprintf("http://%s/health", testDomain)
	} else if parsedURL.Port() == "80" {
		httpTestURL = fmt.Sprintf("http://%s/health", testDomain)
	}

	cmd := fmt.Sprintf("curl -v -s -L %s", httpTestURL)

	output, err := client.Run(cmd)
	if err != nil {
		return output, fmt.Errorf("hosts redirection HTTP test failed: %w", err)
	}

	// 3b. HTTPS (to verify TLS reachability)
	httpsPort := os.Getenv("HTTPS_PORT")
	if httpsPort == "" {
		httpsPort = "8443"
	}

	httpsTestURL := fmt.Sprintf("https://%s:%s/health", testDomain, httpsPort)
	if httpsPort == "443" {
		httpsTestURL = fmt.Sprintf("https://%s/health", testDomain)
	}

	// We now include the testDomain in our SSL certificate.
	// We use the local CA certificate to verify the connection.
	caPEM, err := os.ReadFile(m.Crypto.GetCACertPath())
	if err != nil {
		return output, fmt.Errorf("failed to read CA cert for HTTPS test: %w", err)
	}

	caPath := "/tmp/soundtouch-test-ca.crt"
	if err := client.UploadContent(caPEM, caPath); err != nil {
		return output, fmt.Errorf("failed to upload temporary CA for HTTPS test: %w", err)
	}

	defer func() {
		_, _ = client.Run("rm " + caPath)
	}()

	httpsCmd := fmt.Sprintf("curl -v -s -L --cacert %s %s", caPath, httpsTestURL)

	httpsOutput, httpsErr := client.Run(httpsCmd)
	if httpsErr != nil {
		return output + "\n---\n" + httpsOutput, fmt.Errorf("hosts redirection HTTPS test failed: %w", httpsErr)
	}

	return output + "\n---\n" + httpsOutput, nil
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

	cmd := fmt.Sprintf("curl -v -s -L %s", targetURL)
	if useExplicitCA {
		cmd += " --cacert " + caPath
	}

	output, err := client.Run(cmd)
	if err != nil {
		return output, fmt.Errorf("connection test failed: %w", err)
	}

	return output, nil
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
