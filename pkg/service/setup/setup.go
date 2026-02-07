package setup

import (
	"encoding/xml"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/ssh"
)

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
		return nil, fmt.Errorf("failed to fetch info from %s: %v", infoURL, err)
	}
	defer resp.Body.Close()

	var infoXML DeviceInfoXML
	if err := xml.NewDecoder(resp.Body).Decode(&infoXML); err != nil {
		return nil, fmt.Errorf("failed to decode info XML from %s: %v", infoURL, err)
	}

	for _, comp := range infoXML.Components {
		if comp.Category == "SCM" {
			infoXML.SoftwareVer = comp.SoftwareVersion
			if infoXML.SerialNumber == "" {
				infoXML.SerialNumber = comp.SerialNumber
			}
		} else if comp.Category == "PackagedProduct" {
			if infoXML.SerialNumber == "" {
				infoXML.SerialNumber = comp.SerialNumber
			}
		}
	}

	return &infoXML, nil
}

// GetMigrationSummary returns a summary of the current and planned state of the speaker.
func (m *Manager) GetMigrationSummary(deviceIP string, targetURL string, proxyURL string, options map[string]string) (*MigrationSummary, error) {
	if targetURL == "" {
		targetURL = m.ServerURL
	}
	client := ssh.NewClient(deviceIP)

	summary := &MigrationSummary{
		SSHSuccess: false,
	}

	// 0. Populate from datastore if available
	if m.DataStore != nil {
		devices, err := m.DataStore.ListAllDevices()
		if err == nil {
			for _, d := range devices {
				if d.IPAddress == deviceIP {
					summary.DeviceName = d.Name
					summary.DeviceModel = d.ProductCode
					summary.DeviceSerial = d.DeviceSerialNumber
					summary.FirmwareVersion = d.FirmwareVersion
					break
				}
			}
		} else {
			log.Printf("Warning: failed to list devices from datastore: %v", err)
		}
	}

	// 0a. Supplement with live info from :8090/info
	infoXML, err := m.GetLiveDeviceInfo(deviceIP)
	if err == nil {
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
	} else {
		log.Printf("Warning: %v", err)
	}

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
	var currentConfig string
	path := SoundTouchSdkPrivateCfgPath
	client = ssh.NewClient(deviceIP)

	// Check if .original exists
	if _, err := client.Run(fmt.Sprintf("[ -f %s.original ]", path)); err == nil {
		originalConfig, _ := client.Run(fmt.Sprintf("cat %s.original", path))
		if originalConfig != "" {
			summary.OriginalConfig = originalConfig
		}
	}

	// Check file details
	fileInfo, _ := client.Run(fmt.Sprintf("ls -l %s", path))
	if fileInfo != "" {
		fmt.Printf("File info for %s: %s\n", path, fileInfo)
	}

	// Try cat
	config, err := client.Run(fmt.Sprintf("cat %s", path))
	if err == nil && config != "" {
		currentConfig = config
		summary.SSHSuccess = true
		summary.CurrentConfig = currentConfig
		fmt.Printf("Current config from %s at %s (length: %d):\n%q\n", deviceIP, path, len(currentConfig), currentConfig)

		// Parse current config
		var currentCfg PrivateCfg
		if xml.Unmarshal([]byte(currentConfig), &currentCfg) == nil {
			summary.ParsedCurrentConfig = &currentCfg

			if proxyURL == "" {
				proxyURL = targetURL
			}

			// Apply options if provided
			if options != nil {
				// Marge
				if options["marge"] == "original" {
					plannedCfg.MargeServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.MargeServerUrl)
				}
				// Stats
				if options["stats"] == "original" {
					plannedCfg.StatsServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.StatsServerUrl)
				}
				// SwUpdate
				if options["sw_update"] == "original" {
					plannedCfg.SwUpdateUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.SwUpdateUrl)
				}
				// BMX
				if options["bmx"] == "original" {
					plannedCfg.BmxRegistryUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.BmxRegistryUrl)
				}
			} else if proxyURL != "" {
				// Default to proxy everything if proxyURL is explicitly provided but no options
				// (Maintain backward compatibility for now if needed, but we'll probably always pass options from UI)
				// Actually, if proxyURL is set but no options, let's keep the previous behavior of proxying all.
				plannedCfg.MargeServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.MargeServerUrl)
				plannedCfg.StatsServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.StatsServerUrl)
				plannedCfg.SwUpdateUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.SwUpdateUrl)
				plannedCfg.BmxRegistryUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.BmxRegistryUrl)
			}
		}
	} else {
		// Fallback: try base64 if cat returned empty string but file has size > 0
		if config == "" && fileInfo != "" {
			fmt.Printf("Cat returned empty for %s, trying base64\n", path)
			b64Config, err := client.Run(fmt.Sprintf("base64 %s", path))
			if err == nil && b64Config != "" {
				fmt.Printf("Base64 output for %s (length %d)\n", path, len(b64Config))
			}
		}

		// If SSH failed or file couldn't be read
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
	}

	xmlContent, err := xml.MarshalIndent(plannedCfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal planned XML: %v", err)
	}
	summary.PlannedConfig = "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n" + string(xmlContent)

	// 3. Check for remote services files
	locations := []string{
		"/etc/remote_services",
		"/mnt/nv/remote_services",
		"/tmp/remote_services",
	}

	for _, loc := range locations {
		_, err := client.Run(fmt.Sprintf("[ -e %s ]", loc))
		if err == nil {
			summary.RemoteServicesFound = append(summary.RemoteServicesFound, loc)
			summary.RemoteServicesEnabled = true
			if loc != "/tmp/remote_services" {
				summary.RemoteServicesPersistent = true
			}
		}
	}

	return summary, nil
}

// MigrateSpeaker configures the speaker at the given IP to use this soundcork service.
func (m *Manager) MigrateSpeaker(deviceIP string, targetURL string, proxyURL string, options map[string]string) error {
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
				if options["marge"] == "original" {
					cfg.MargeServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.MargeServerUrl)
				}
				if options["stats"] == "original" {
					cfg.StatsServerUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.StatsServerUrl)
				}
				if options["sw_update"] == "original" {
					cfg.SwUpdateUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.SwUpdateUrl)
				}
				if options["bmx"] == "original" {
					cfg.BmxRegistryUrl = fmt.Sprintf("%s/proxy/%s", proxyURL, currentCfg.BmxRegistryUrl)
				}
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
		return fmt.Errorf("failed to marshal XML: %v", err)
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
		return fmt.Errorf("failed to upload config: %v", err)
	}

	// 2. Reboot the speaker (requires 'rw' command first to make filesystem writable)
	if _, err := client.Run(fmt.Sprintf("%s && reboot", rwCmd)); err != nil {
		return fmt.Errorf("failed to reboot speaker: %v", err)
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
	if output, err := client.Run(fmt.Sprintf("%s && cp %s %s.original", rwCmd, remotePath, remotePath)); err == nil {
		return nil
	} else {
		fmt.Printf("Direct cp failed: %v (output: %s), falling back to cat+upload\n", err, output)
	}

	// Fallback to cat + upload
	config, err := client.Run(fmt.Sprintf("cat %s", remotePath))
	if err != nil || config == "" {
		return fmt.Errorf("failed to read current config: %v", err)
	}

	// Ensure rw before upload fallback
	_, _ = client.Run(rwCmd)
	if err := client.UploadContent([]byte(config), remotePath+".original"); err != nil {
		return fmt.Errorf("failed to upload backup config: %v", err)
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
