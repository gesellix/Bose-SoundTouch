// Package setup contains speaker migration and configuration helpers.
package setup

import (
	"encoding/xml"
	"fmt"
	"net"
	"net/http"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/ssh"
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
	RemoteServicesEnabled    bool        `json:"remote_services_enabled"`
	RemoteServicesPersistent bool        `json:"remote_services_persistent"`
	RemoteServicesFound      []string    `json:"remote_services_found"`
	RemoteServicesCheckErr   string      `json:"remote_services_check_err,omitempty"`
	DeviceName               string      `json:"device_name,omitempty"`
	DeviceModel              string      `json:"device_model,omitempty"`
	DeviceSerial             string      `json:"device_serial,omitempty"`
	FirmwareVersion          string      `json:"firmware_version,omitempty"`
}

// Manager handles the migration of speakers to the soundcork service.
type Manager struct {
	ServerURL string
	DataStore *datastore.DataStore
}

// NewManager creates a new Manager with the given base server URL.
func NewManager(serverURL string, ds *datastore.DataStore) *Manager {
	return &Manager{ServerURL: serverURL, DataStore: ds}
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

	// 3. Check for remote services files
	m.checkRemoteServices(summary, deviceIP)

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
	client := ssh.NewClient(deviceIP)

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
	client := ssh.NewClient(deviceIP)
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

// MigrateSpeaker configures the speaker at the given IP to use this soundcork service.
func (m *Manager) MigrateSpeaker(deviceIP, targetURL, proxyURL string, options map[string]string) error {
	if targetURL == "" {
		targetURL = m.ServerURL
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
	client := ssh.NewClient(deviceIP)
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
	client := ssh.NewClient(deviceIP)
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
	client := ssh.NewClient(deviceIP)
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
