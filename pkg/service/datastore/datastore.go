// Package datastore provides a simple XML-based datastore for SoundTouch devices.
package datastore

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DataStore represents the device and configuration storage.
type DataStore struct {
	// DataDir is the (possibly relative) base directory for all datastore files.
	DataDir string
	// baseDir is the absolute, normalized base directory used for path safety checks.
	baseDir string

	eventMutex     sync.RWMutex
	deviceEvents   map[string][]models.DeviceEvent
	idMutex        sync.RWMutex
	deviceMappings map[string]string
	fileMutex      sync.RWMutex
}

// normalizeMAC normalizes a MAC address to a consistent format
func normalizeMAC(mac string) string {
	if mac == "" {
		return ""
	}
	// Remove spaces and common separators, then convert to uppercase
	mac = strings.TrimSpace(mac)
	mac = strings.ReplaceAll(mac, " ", "")
	mac = strings.ReplaceAll(mac, ":", "")
	mac = strings.ReplaceAll(mac, "-", "")
	mac = strings.ToUpper(mac)

	return mac
}

// NewDataStore creates a new DataStore.
// NewDataStore creates a new DataStore instance with the specified data directory.
func NewDataStore(dataDir string) *DataStore {
	if dataDir == "" {
		dataDir = "data"
	}

	absBase, err := filepath.Abs(dataDir)
	if err != nil {
		// Fallback to the provided dataDir if Abs fails; this preserves existing behavior.
		absBase = dataDir
	}

	return &DataStore{
		DataDir:        dataDir,
		baseDir:        absBase,
		deviceEvents:   make(map[string][]models.DeviceEvent),
		deviceMappings: make(map[string]string),
	}
}

// safeJoin joins the given path elements to the datastore baseDir and ensures
// that the resulting absolute path stays within baseDir. If the check fails,
// baseDir is returned to prevent directory traversal.
func (ds *DataStore) safeJoin(elem ...string) string {
	// Join the base directory with the provided elements.
	path := filepath.Join(append([]string{ds.baseDir}, elem...)...)

	absPath, err := filepath.Abs(path)
	if err != nil {
		// On error, fall back to baseDir to avoid using an unexpected path.
		return ds.baseDir
	}

	base := ds.baseDir
	if base == "" {
		// If baseDir is not set for some reason, fall back to original path.
		return absPath
	}

	// Ensure the resolved path is within the base directory.
	baseWithSep := base
	if !strings.HasSuffix(baseWithSep, string(os.PathSeparator)) {
		baseWithSep += string(os.PathSeparator)
	}

	if absPath == base || strings.HasPrefix(absPath, baseWithSep) {
		return absPath
	}

	// If the path would escape the base directory, return baseDir as a safe default.
	return base
}

// SafeJoin returns a safe joined path relative to the datastore base directory.
func (ds *DataStore) SafeJoin(elem ...string) string {
	return ds.safeJoin(elem...)
}

// ListAccounts returns a list of all account IDs (directories in the data root).
func (ds *DataStore) ListAccounts() ([]string, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	// Account data is stored in 'accounts' subdirectory within the data root.
	accountsDir := filepath.Join(ds.baseDir, "accounts")
	if !exists(accountsDir) {
		return []string{"default"}, nil
	}

	entries, err := os.ReadDir(accountsDir)
	if err != nil {
		return nil, err
	}

	accounts := make([]string, 0)

	for _, entry := range entries {
		if entry.IsDir() {
			// Basic filter to ignore common hidden/system dirs
			if entry.Name() != ".git" && entry.Name() != "logs" {
				accounts = append(accounts, entry.Name())
			}
		}
	}

	if len(accounts) == 0 {
		accounts = append(accounts, "default")
	}

	return accounts, nil
}

// AccountDir returns the directory path for a specific account.
func (ds *DataStore) AccountDir(account string) string {
	return ds.safeJoin("accounts", account)
}

// AccountDevicesDir returns the devices directory path for a specific account.
func (ds *DataStore) AccountDevicesDir(account string) string {
	return ds.safeJoin("accounts", account, constants.DevicesDir)
}

// AccountDeviceDir returns the directory path for a specific device within an account.
func (ds *DataStore) AccountDeviceDir(account, device string) string {
	// First, check if the device directory exists directly with the given deviceID
	// This prioritizes MAC-based deviceIDs over legacy mappings
	directPath := ds.safeJoin("accounts", account, constants.DevicesDir, device)
	if _, err := os.Stat(directPath); err == nil {
		// Directory exists, use the direct deviceID (preferred for MAC-based IDs)
		return directPath
	}

	// If direct path doesn't exist, check device mappings for backward compatibility
	ds.idMutex.RLock()

	mappedDevice, ok := ds.deviceMappings[device]
	if !ok {
		// Try with normalized MAC address
		normalizedDevice := normalizeMAC(device)
		mappedDevice, ok = ds.deviceMappings[normalizedDevice]
	}

	ds.idMutex.RUnlock()

	if ok {
		// Use the mapped device only if it exists and the direct path doesn't
		mappedPath := ds.safeJoin("accounts", account, constants.DevicesDir, mappedDevice)
		if _, err := os.Stat(mappedPath); err == nil {
			return mappedPath
		}
	}

	// If neither direct path nor mapping work, return the direct path
	// (this allows new devices to be created with MAC-based IDs)
	return directPath
}

// GetDeviceInfo retrieves device information for the specified account and device.
func (ds *DataStore) GetDeviceInfo(account, device string) (*models.ServiceDeviceInfo, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	return ds.getDeviceInfoNoLock(account, device)
}

func (ds *DataStore) getDeviceInfoNoLock(account, device string) (*models.ServiceDeviceInfo, error) {
	path := ds.AccountDeviceDir(account, device)
	deviceInfoPath := filepath.Join(path, constants.DeviceInfoFile)

	data, err := os.ReadFile(deviceInfoPath)
	if err != nil {
		return nil, err
	}

	var info struct {
		XMLName    xml.Name `xml:"info"`
		DeviceID   string   `xml:"deviceID,attr"`
		Name       string   `xml:"name"`
		Type       string   `xml:"type"`
		ModuleType string   `xml:"moduleType"`
		Components []struct {
			Category        string `xml:"componentCategory"`
			SoftwareVersion string `xml:"softwareVersion"`
			SerialNumber    string `xml:"serialNumber"`
		} `xml:"components>component"`
		NetworkInfo []struct {
			Type       string `xml:"type,attr"`
			IPAddress  string `xml:"ipAddress"`
			MacAddress string `xml:"macAddress"`
		} `xml:"networkInfo"`
		DiscoveryMethod string `xml:"discoveryMethod"`
	}

	if err := xml.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	deviceInfo := &models.ServiceDeviceInfo{
		DeviceID:        info.DeviceID,
		AccountID:       account, // Set AccountID from parameter
		ProductCode:     fmt.Sprintf("%s %s", info.Type, info.ModuleType),
		Name:            info.Name,
		DiscoveryMethod: info.DiscoveryMethod,
	}

	for _, comp := range info.Components {
		deviceInfo.Components = append(deviceInfo.Components, models.ServiceComponent{
			Category:        comp.Category,
			SoftwareVersion: comp.SoftwareVersion,
			SerialNumber:    comp.SerialNumber,
		})

		switch comp.Category {
		case "SCM":
			deviceInfo.FirmwareVersion = comp.SoftwareVersion
			deviceInfo.DeviceSerialNumber = comp.SerialNumber
		case "PackagedProduct":
			deviceInfo.ProductSerialNumber = comp.SerialNumber
		}
	}

	for _, net := range info.NetworkInfo {
		if net.Type == "SCM" {
			deviceInfo.IPAddress = net.IPAddress
			deviceInfo.MacAddress = net.MacAddress
		}
	}

	return deviceInfo, nil
}

// ListAllDevices returns a list of all devices in all accounts.
func (ds *DataStore) ListAllDevices() ([]models.ServiceDeviceInfo, error) {
	dirs := ds.getPossibleDataDirs()
	if len(dirs) == 0 {
		return []models.ServiceDeviceInfo{}, nil
	}

	devices := []models.ServiceDeviceInfo{}
	seenIDs := make(map[string]bool)

	for _, dir := range dirs {
		accounts, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, acc := range accounts {
			if !acc.IsDir() {
				continue
			}

			accDevices := ds.listDevicesInAccount(dir, acc.Name())
			for i := range accDevices {
				info := accDevices[i]
				info.AccountID = acc.Name()

				key := info.DeviceID
				if key == "" {
					key = info.IPAddress
				}

				if !seenIDs[key] || info.Name != "" {
					if seenIDs[key] && info.Name != "" {
						// Replace previous empty-named entry with one that has a name
						for j := range devices {
							existing := &devices[j]
							if (existing.DeviceID != "" && existing.DeviceID == info.DeviceID) ||
								(existing.IPAddress != "" && existing.IPAddress == info.IPAddress) {
								devices[j] = info
								break
							}
						}
					} else if !seenIDs[key] {
						devices = append(devices, info)
						seenIDs[key] = true
					}
				}
			}
		}
	}

	return devices, nil
}

func (ds *DataStore) getPossibleDataDirs() []string {
	dirs := []string{}
	// Check primary data directory
	if ds.DataDir != "" {
		if exists(filepath.Join(ds.DataDir, "accounts")) {
			dirs = append(dirs, filepath.Join(ds.DataDir, "accounts"))
		}
		// Also check the DataDir itself as a base for account directories
		if exists(ds.DataDir) && ds.DataDir != "." {
			dirs = append(dirs, ds.DataDir)
		}
	}

	// Also check st-go/data/accounts if it's different and exists
	altDir := "st-go/data/accounts"
	if exists(altDir) {
		dirs = append(dirs, altDir)
	}
	// And st-go/data/accounts/default
	altDir2 := "st-go/data"
	if exists(altDir2) {
		dirs = append(dirs, altDir2)
	}
	// And repro_data
	altDir3 := "repro_data"
	if exists(altDir3) {
		dirs = append(dirs, altDir3)
	}

	// Add special handling for test environments where we might have account directories
	// directly in the current working directory or a temp dir.
	// Walk up from DataDir to find any 'accounts' directory.
	curr := ds.DataDir
	for i := 0; i < 3; i++ {
		absCurr, _ := filepath.Abs(curr)
		if exists(filepath.Join(absCurr, "accounts")) {
			dirs = append(dirs, filepath.Join(absCurr, "accounts"))
		}

		if exists(absCurr) {
			dirs = append(dirs, absCurr)
		}

		if curr == "." || curr == "/" || curr == "" {
			break
		}

		curr = filepath.Dir(curr)
	}

	// Remove duplicates and ensure unique directories
	uniqueDirs := make(map[string]bool)
	result := []string{}

	for _, dir := range dirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			absDir = dir
		}

		if !uniqueDirs[absDir] {
			uniqueDirs[absDir] = true

			result = append(result, dir)
		}
	}

	return result
}

func (ds *DataStore) listDevicesInAccount(baseDir, accountName string) []models.ServiceDeviceInfo {
	devices := []models.ServiceDeviceInfo{}
	devicesDir := filepath.Join(baseDir, accountName, constants.DevicesDir)

	deviceEntries, err := os.ReadDir(devicesDir)
	if err != nil {
		return devices
	}

	for _, dev := range deviceEntries {
		var (
			info *models.ServiceDeviceInfo
			err  error
		)

		if !dev.IsDir() {
			if dev.Name() == constants.DeviceInfoFile {
				// Special case for DeviceInfo.xml directly in devicesDir
				path := filepath.Join(devicesDir, constants.DeviceInfoFile)
				info, err = ds.parseDeviceInfoFile(path)
			}
		} else {
			path := filepath.Join(devicesDir, dev.Name(), constants.DeviceInfoFile)
			info, err = ds.parseDeviceInfoFile(path)
		}

		if err == nil && info != nil {
			// Update bidirectional device mappings for resolution
			ds.updateDeviceMappings(*info)

			devices = append(devices, *info)
		}
	}

	return devices
}

func (ds *DataStore) parseDeviceInfoFile(path string) (*models.ServiceDeviceInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var info struct {
		XMLName    xml.Name `xml:"info"`
		DeviceID   string   `xml:"deviceID,attr"`
		Name       string   `xml:"name"`
		Type       string   `xml:"type"`
		ModuleType string   `xml:"moduleType"`
		Components []struct {
			Category        string `xml:"componentCategory"`
			SoftwareVersion string `xml:"softwareVersion"`
			SerialNumber    string `xml:"serialNumber"`
		} `xml:"components>component"`
		NetworkInfo []struct {
			Type       string `xml:"type,attr"`
			IPAddress  string `xml:"ipAddress"`
			MacAddress string `xml:"macAddress"`
		} `xml:"networkInfo"`
		DiscoveryMethod string `xml:"discoveryMethod"`
	}

	if err := xml.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	deviceInfo := &models.ServiceDeviceInfo{
		DeviceID:        info.DeviceID,
		ProductCode:     info.Type,
		Name:            info.Name,
		DiscoveryMethod: info.DiscoveryMethod,
	}

	for _, comp := range info.Components {
		deviceInfo.Components = append(deviceInfo.Components, models.ServiceComponent{
			Category:        comp.Category,
			SoftwareVersion: comp.SoftwareVersion,
			SerialNumber:    comp.SerialNumber,
		})

		switch comp.Category {
		case "SCM":
			deviceInfo.FirmwareVersion = comp.SoftwareVersion
			deviceInfo.DeviceSerialNumber = comp.SerialNumber
		case "PackagedProduct":
			deviceInfo.ProductSerialNumber = comp.SerialNumber
		}
	}

	for _, net := range info.NetworkInfo {
		if net.Type == "SCM" {
			deviceInfo.IPAddress = net.IPAddress
			deviceInfo.MacAddress = net.MacAddress
		}
	}

	return deviceInfo, nil
}

// GetPresets retrieves all presets for the specified account and device.
func (ds *DataStore) GetPresets(account, device string) ([]models.ServicePreset, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.PresetsFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var presetsWrap struct {
		Presets []struct {
			ID          string `xml:"id,attr"`
			CreatedOn   string `xml:"createdOn,attr"`
			UpdatedOn   string `xml:"updatedOn,attr"`
			ContentItem struct {
				Source        string `xml:"source,attr"`
				Type          string `xml:"type,attr"`
				Location      string `xml:"location,attr"`
				SourceAccount string `xml:"sourceAccount,attr"`
				IsPresetable  string `xml:"isPresetable,attr"`
				ItemName      string `xml:"itemName"`
				ContainerArt  string `xml:"containerArt"`
			} `xml:"ContentItem"`
			Source *models.ConfiguredSource `xml:"source"`
		} `xml:"preset"`
	}

	if err := xml.Unmarshal(data, &presetsWrap); err != nil {
		return nil, fmt.Errorf("malformed presets XML at %s: %w", path, err)
	}

	presets := []models.ServicePreset{}

	for i := range presetsWrap.Presets {
		p := &presetsWrap.Presets[i]

		cit := p.ContentItem.Type

		presets = append(presets, models.ServicePreset{
			ServiceContentItem: models.ServiceContentItem{
				Name:            p.ContentItem.ItemName,
				Source:          p.ContentItem.Source,
				Type:            p.ContentItem.Type,
				Location:        p.ContentItem.Location,
				SourceAccount:   p.ContentItem.SourceAccount,
				IsPresetable:    p.ContentItem.IsPresetable,
				ContentItemType: cit,
			},
			ID:           p.ID,
			ButtonNumber: p.ID,
			ContainerArt: p.ContentItem.ContainerArt,
			CreatedOn:    p.CreatedOn,
			UpdatedOn:    p.UpdatedOn,
			SourceConfig: p.Source,
		})
	}

	return presets, nil
}

// SavePresets saves the preset list for the specified account and device.
func (ds *DataStore) SavePresets(account, device string, presets []models.ServicePreset) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.PresetsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	type PresetXML struct {
		ID          string `xml:"id,attr"`
		CreatedOn   string `xml:"createdOn,attr"`
		UpdatedOn   string `xml:"updatedOn,attr"`
		ContentItem struct {
			Source        string `xml:"source,attr,omitempty"`
			Type          string `xml:"type,attr"`
			Location      string `xml:"location,attr"`
			SourceAccount string `xml:"sourceAccount,attr"`
			IsPresetable  string `xml:"isPresetable,attr"`
			ItemName      string `xml:"itemName"`
			ContainerArt  string `xml:"containerArt"`
		} `xml:"ContentItem"`
		Source *models.ConfiguredSource `xml:"source,omitempty"`
	}

	type PresetsXML struct {
		XMLName xml.Name    `xml:"presets"`
		Presets []PresetXML `xml:"preset"`
	}

	var px PresetsXML

	for i := range presets {
		p := &presets[i]

		var pxml PresetXML

		pxml.ID = p.ButtonNumber
		if pxml.ID == "" {
			pxml.ID = p.ID
		}

		pxml.CreatedOn = p.CreatedOn
		pxml.UpdatedOn = p.UpdatedOn
		pxml.ContentItem.Source = p.Source
		pxml.ContentItem.Type = p.Type
		pxml.ContentItem.Location = p.Location
		pxml.ContentItem.SourceAccount = p.SourceAccount
		pxml.ContentItem.IsPresetable = "true"
		pxml.ContentItem.ItemName = p.Name
		pxml.ContentItem.ContainerArt = p.ContainerArt
		pxml.Source = p.SourceConfig
		px.Presets = append(px.Presets, pxml)
	}

	data, err := xml.MarshalIndent(px, "", "    ")
	if err != nil {
		return err
	}

	header := []byte(xml.Header)

	return os.WriteFile(path, append(header, data...), 0644)
}

// GetRecents retrieves all recent items for the specified account and device.
func (ds *DataStore) GetRecents(account, device string) ([]models.ServiceRecent, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.RecentsFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	type RecentsXML struct {
		XMLName xml.Name               `xml:"recents"`
		Recents []models.ServiceRecent `xml:"recent"`
	}

	var recentsWrap RecentsXML

	if err := xml.Unmarshal(data, &recentsWrap); err != nil {
		return nil, fmt.Errorf("malformed recents XML at %s: %w", path, err)
	}

	recents := recentsWrap.Recents
	maxID := 0

	for i := range recents {
		r := &recents[i]

		if id, err := strconv.Atoi(r.ID); err == nil {
			if id > maxID {
				maxID = id
			}
		}
	}

	for i := range recents {
		r := &recents[i]
		if r.ContentItemType == "" {
			r.ContentItemType = r.Type
		}

		if _, err := strconv.Atoi(recents[i].ID); err != nil || recents[i].ID == "" {
			maxID++
			recents[i].ID = strconv.Itoa(maxID)
		}
	}

	return recents, nil
}

// SaveRecents saves the recent items list for the specified account and device.
func (ds *DataStore) SaveRecents(account, device string, recents []models.ServiceRecent) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	dir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, constants.RecentsFile)

	type RecentsXML struct {
		XMLName xml.Name               `xml:"recents"`
		Recents []models.ServiceRecent `xml:"recent"`
	}

	wrap := RecentsXML{
		Recents: recents,
	}

	data, err := xml.MarshalIndent(wrap, "", "    ")
	if err != nil {
		return err
	}

	header := []byte(xml.Header)

	return os.WriteFile(path, append(header, data...), 0644)
}

// SaveDeviceInfo saves device information for the specified account and device.
func (ds *DataStore) SaveDeviceInfo(account, device string, info *models.ServiceDeviceInfo) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	if device == "" {
		return fmt.Errorf("device ID/name cannot be empty")
	}

	// Try to load existing device info to avoid overwriting existing details with empty values.
	existing, _ := ds.getDeviceInfoNoLock(account, device)
	if existing != nil {
		if info.Name == "" {
			info.Name = existing.Name
		}

		if info.ProductCode == "" {
			info.ProductCode = existing.ProductCode
		}

		if info.DeviceSerialNumber == "" {
			info.DeviceSerialNumber = existing.DeviceSerialNumber
		}

		if info.ProductSerialNumber == "" {
			info.ProductSerialNumber = existing.ProductSerialNumber
		}

		if info.FirmwareVersion == "" {
			info.FirmwareVersion = existing.FirmwareVersion
		}

		if info.IPAddress == "" {
			info.IPAddress = existing.IPAddress
		}

		if info.MacAddress == "" {
			info.MacAddress = existing.MacAddress
		}

		if info.DiscoveryMethod == "" {
			info.DiscoveryMethod = existing.DiscoveryMethod
		}
	}

	dir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, constants.DeviceInfoFile)

	type ComponentXML struct {
		ComponentCategory string `xml:"componentCategory"`
		SoftwareVersion   string `xml:"softwareVersion,omitempty"`
		SerialNumber      string `xml:"serialNumber,omitempty"`
	}

	type NetworkInfoXML struct {
		Type       string `xml:"type,attr"`
		IPAddress  string `xml:"ipAddress"`
		MacAddress string `xml:"macAddress"`
	}

	type InfoXML struct {
		XMLName         xml.Name         `xml:"info"`
		DeviceID        string           `xml:"deviceID,attr"`
		Name            string           `xml:"name"`
		Type            string           `xml:"type"`
		ModuleType      string           `xml:"moduleType"`
		Components      []ComponentXML   `xml:"components>component"`
		NetworkInfo     []NetworkInfoXML `xml:"networkInfo"`
		DiscoveryMethod string           `xml:"discoveryMethod,omitempty"`
	}

	// Parsing product code back to type and moduleType (best effort)
	// Python: f"{type} {module_type}"
	devType := info.ProductCode
	moduleType := ""

	for i := 0; i < len(info.ProductCode); i++ {
		if info.ProductCode[i] == ' ' {
			devType = info.ProductCode[:i]
			moduleType = info.ProductCode[i+1:]

			break
		}
	}

	ix := InfoXML{
		DeviceID:        info.DeviceID,
		Name:            info.Name,
		Type:            devType,
		ModuleType:      moduleType,
		DiscoveryMethod: info.DiscoveryMethod,
	}

	if ix.DiscoveryMethod == "" {
		ix.DiscoveryMethod = "sync_full"
	}

	for _, comp := range info.Components {
		ix.Components = append(ix.Components, ComponentXML{
			ComponentCategory: comp.Category,
			SoftwareVersion:   comp.SoftwareVersion,
			SerialNumber:      comp.SerialNumber,
		})
	}

	if len(ix.Components) == 0 {
		ix.Components = []ComponentXML{
			{
				ComponentCategory: "SCM",
				SoftwareVersion:   info.FirmwareVersion,
				SerialNumber:      info.DeviceSerialNumber,
			},
			{
				ComponentCategory: "PackagedProduct",
				SerialNumber:      info.ProductSerialNumber,
			},
		}
	}

	ix.NetworkInfo = []NetworkInfoXML{
		{
			Type:       "SCM",
			IPAddress:  info.IPAddress,
			MacAddress: info.MacAddress,
		},
	}

	data, err := xml.MarshalIndent(ix, "", "    ")
	if err != nil {
		return err
	}

	header := []byte(xml.Header)

	return os.WriteFile(path, append(header, data...), 0644)
}

// SaveAccountInfo saves account-level metadata to the datastore.
func (ds *DataStore) SaveAccountInfo(accountID string, info *models.ServiceAccountInfo) error {
	if ds == nil || ds.DataDir == "" || accountID == "" {
		return nil
	}

	dir := ds.AccountDir(accountID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, "account.json")

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GetAccountInfo retrieves account-level metadata from the datastore.
func (ds *DataStore) GetAccountInfo(accountID string) (*models.ServiceAccountInfo, error) {
	if ds == nil || ds.DataDir == "" || accountID == "" {
		return &models.ServiceAccountInfo{AccountID: accountID}, nil
	}

	path := filepath.Join(ds.AccountDir(accountID), "account.json")
	if !exists(path) {
		return &models.ServiceAccountInfo{AccountID: accountID}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var info models.ServiceAccountInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// RemoveDevice removes a device and all its data from the specified account.
func (ds *DataStore) RemoveDevice(account, device string) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	dir := ds.AccountDeviceDir(account, device)

	return os.RemoveAll(dir)
}

// RemoveDeviceDir is an alias for RemoveDevice for backwards compatibility.
func (ds *DataStore) RemoveDeviceDir(account, device string) error {
	return ds.RemoveDevice(account, device)
}

// GetConfiguredSources retrieves all configured sources for the specified account and device.
func (ds *DataStore) GetConfiguredSources(account, device string) ([]models.ConfiguredSource, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.SourcesFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var sourcesWrap struct {
		Sources []models.ConfiguredSource `xml:"source"`
	}

	if err := xml.Unmarshal(data, &sourcesWrap); err != nil {
		return nil, fmt.Errorf("malformed sources XML at %s: %w", path, err)
	}

	for i := range sourcesWrap.Sources {
		s := &sourcesWrap.Sources[i]

		// Ensure SourceKey values are prioritized for legacy fields
		if s.SourceKey.Type != "" {
			s.SourceKeyType = s.SourceKey.Type
		}

		if s.SourceKey.Account != "" {
			s.SourceKeyAccount = s.SourceKey.Account
		}

		// Ensure Type is populated from SourceKey if missing
		if s.Type == "" && s.SourceKey.Type != "" {
			s.Type = s.SourceKey.Type
		}

		if s.ID == "" {
			s.ID = strconv.Itoa(100001 + i)
		}
	}

	return sourcesWrap.Sources, nil
}

// SaveConfiguredSources saves the configured sources list for the specified account and device.
func (ds *DataStore) SaveConfiguredSources(account, device string, sources []models.ConfiguredSource) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.SourcesFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	type sourcesWrap struct {
		XMLName xml.Name                  `xml:"sources"`
		Sources []models.ConfiguredSource `xml:"source"`
	}

	// Ensure SourceKey is populated from legacy fields if necessary before saving
	for i := range sources {
		s := &sources[i]
		if s.SourceKey.Type == "" && s.SourceKeyType != "" {
			s.SourceKey.Type = s.SourceKeyType
		}

		if s.SourceKey.Account == "" && s.SourceKeyAccount != "" {
			s.SourceKey.Account = s.SourceKeyAccount
		}
	}

	wrap := sourcesWrap{
		Sources: sources,
	}

	data, err := xml.MarshalIndent(wrap, "", "    ")
	if err != nil {
		return err
	}

	header := []byte(xml.Header)

	return os.WriteFile(path, append(header, data...), 0644)
}

// updateDeviceMappings creates bidirectional mappings for device resolution
func (ds *DataStore) updateDeviceMappings(info models.ServiceDeviceInfo) {
	ds.idMutex.Lock()
	defer ds.idMutex.Unlock()

	deviceID := info.DeviceID
	macAddress := info.MacAddress
	deviceSerial := info.DeviceSerialNumber

	// If device is stored with MAC as deviceID and has a serial, create backward mapping
	if isMACAddressFormat(deviceID) && deviceSerial != "" && deviceSerial != deviceID {
		ds.deviceMappings[deviceSerial] = deviceID
	}

	// If device is stored with serial as deviceID and has a MAC, create forward mapping
	if !isMACAddressFormat(deviceID) && macAddress != "" {
		ds.deviceMappings[macAddress] = deviceID
		// Also store normalized MAC version
		normalizedMAC := normalizeMAC(macAddress)
		if normalizedMAC != macAddress {
			ds.deviceMappings[normalizedMAC] = deviceID
		}
	}
}

// UpdateMapping maintains backward compatibility for external callers
func (ds *DataStore) UpdateMapping(mac, serial string) {
	if mac == "" || serial == "" {
		return
	}

	ds.idMutex.Lock()
	defer ds.idMutex.Unlock()

	// In the new system, MAC addresses are preferred as deviceIDs
	// So map the serial TO the MAC (reverse of old system)
	ds.deviceMappings[serial] = mac

	// Also map MAC to serial for any remaining legacy code
	ds.deviceMappings[mac] = serial

	normalizedMAC := normalizeMAC(mac)
	if normalizedMAC != mac {
		ds.deviceMappings[normalizedMAC] = serial
	}
}

// isMACAddressFormat checks if a string looks like a MAC address
func isMACAddressFormat(s string) bool {
	// AABBCCDDEEFF format
	if len(s) == 12 {
		return isHexOnly(s)
	}

	// AA:BB:CC:DD:EE:FF or AA-BB-CC-DD-EE-FF format
	if len(s) == 17 && (strings.Contains(s, ":") || strings.Contains(s, "-")) {
		s = strings.ReplaceAll(s, "-", ":")

		parts := strings.Split(s, ":")
		if len(parts) != 6 {
			return false
		}

		for _, part := range parts {
			if len(part) != 2 || !isHexOnly(part) {
				return false
			}
		}

		return true
	}

	return false
}

func isHexOnly(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'A' || r > 'F') && (r < 'a' || r > 'f') {
			return false
		}
	}

	return true
}

// Initialize creates the necessary directory structure for the datastore and populates ID mappings.
func (ds *DataStore) Initialize() error {
	// Ensure base data directory exists
	if err := os.MkdirAll(ds.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Scan for devices to populate MAC to Serial mapping
	_, err := ds.ListAllDevices()

	return err
}

// GetETagForPresets returns the ETag (modification time) for the presets file for a specific device.
func (ds *DataStore) GetETagForPresets(account, device string) int64 {
	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.PresetsFile)

	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	return info.ModTime().UnixNano() / int64(time.Millisecond)
}

// GetETagForSources returns the ETag (modification time) for the sources file for a specific device.
func (ds *DataStore) GetETagForSources(account, device string) int64 {
	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.SourcesFile)

	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	return info.ModTime().UnixNano() / int64(time.Millisecond)
}

// GetETagForRecents returns the ETag (modification time) for the recents file for a specific device.
func (ds *DataStore) GetETagForRecents(account, device string) int64 {
	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.RecentsFile)

	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	return info.ModTime().UnixNano() / int64(time.Millisecond)
}

// GetETagForAccount returns the highest ETag among presets, sources, and recents for the account and device.
func (ds *DataStore) GetETagForAccount(account, device string) int64 {
	e1 := ds.GetETagForPresets(account, device)
	e2 := ds.GetETagForSources(account, device)
	e3 := ds.GetETagForRecents(account, device)

	maxETag := e1
	if e2 > maxETag {
		maxETag = e2
	}

	if e3 > maxETag {
		maxETag = e3
	}

	return maxETag
}

// Settings represents the global service settings.
type Settings struct {
	ServerURL          string         `json:"server_url"`
	HTTPServerURL      string         `json:"https_server_url,omitempty"`
	RedactLogs         bool           `json:"redact_logs"`
	LogBodies          bool           `json:"log_bodies"`
	RecordInteractions bool           `json:"record_interactions"`
	DiscoveryInterval  string         `json:"discovery_interval,omitempty"`
	DiscoveryEnabled   bool           `json:"discovery_enabled"`
	DNSEnabled         bool           `json:"dns_enabled"`
	DNSUpstream        []string       `json:"dns_upstream,omitempty"`
	DNSBindAddr        string         `json:"dns_bind_addr,omitempty"`
	MirrorEnabled      bool           `json:"mirror_enabled"`
	MirrorEndpoints    []string       `json:"mirror_endpoints,omitempty"`
	PreferredSource    string         `json:"preferred_source,omitempty"`
	InternalPaths      []string       `json:"internal_paths,omitempty"`
	Shortcuts          map[string]int `json:"shortcuts,omitempty"`
}

// GetSettings retrieves the global service settings.
func (ds *DataStore) GetSettings() (Settings, error) {
	if ds == nil || ds.DataDir == "" {
		return Settings{}, nil
	}

	path := filepath.Join(ds.DataDir, "settings.json")
	if !exists(path) {
		return Settings{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, err
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, err
	}

	return settings, nil
}

// SaveSettings saves the global service settings.
func (ds *DataStore) SaveSettings(settings Settings) error {
	if ds == nil || ds.DataDir == "" {
		return nil
	}

	if err := os.MkdirAll(ds.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	path := filepath.Join(ds.DataDir, "settings.json")

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// SaveUsageStats saves usage statistics to the datastore.
func (ds *DataStore) SaveUsageStats(stats models.UsageStats) error {
	dir := filepath.Join(ds.DataDir, "stats", "usage")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%d_%s.json", time.Now().UnixNano(), stats.DeviceID)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// SaveErrorStats saves error statistics to the datastore.
func (ds *DataStore) SaveErrorStats(stats models.ErrorStats) error {
	dir := filepath.Join(ds.DataDir, "stats", "error")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%d_%s.json", time.Now().UnixNano(), stats.DeviceID)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// AddDeviceEvent adds a device event to the in-memory event store.
func (ds *DataStore) AddDeviceEvent(deviceID string, event models.DeviceEvent) {
	ds.eventMutex.Lock()
	defer ds.eventMutex.Unlock()

	events := ds.deviceEvents[deviceID]
	events = append(events, event)

	// Keep only last 100 events
	if len(events) > 100 {
		events = events[len(events)-100:]
	}

	ds.deviceEvents[deviceID] = events
}

// GetDeviceEvents retrieves all events for the specified device.
func (ds *DataStore) GetDeviceEvents(deviceID string) []models.DeviceEvent {
	ds.eventMutex.RLock()
	defer ds.eventMutex.RUnlock()

	events, ok := ds.deviceEvents[deviceID]
	if !ok {
		return []models.DeviceEvent{}
	}

	// Return a copy to avoid race conditions if the caller modifies it
	copiedEvents := make([]models.DeviceEvent, len(events))
	copy(copiedEvents, events)

	return copiedEvents
}

// DNSDiscoveryEntry represents a persisted DNS discovery.
type DNSDiscoveryEntry struct {
	Hostname      string    `json:"hostname"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	QueryCount    int       `json:"query_count"`
	IsBoseService bool      `json:"is_bose_service"`
	IsIntercepted bool      `json:"is_intercepted"`
	RemoteAddr    string    `json:"remote_addr,omitempty"`
}

// SaveDNSDiscoveries saves DNS discoveries to the datastore.
func (ds *DataStore) SaveDNSDiscoveries(discoveries []DNSDiscoveryEntry) error {
	if ds == nil || ds.DataDir == "" {
		return nil
	}

	dir := filepath.Join(ds.DataDir, "dns")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create dns directory: %w", err)
	}

	path := filepath.Join(dir, "discoveries.json")

	// Sort by last seen descending
	sort.Slice(discoveries, func(i, j int) bool {
		return discoveries[i].LastSeen.After(discoveries[j].LastSeen)
	})

	data, err := json.MarshalIndent(discoveries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// LoadDNSDiscoveries loads DNS discoveries from the datastore.
func (ds *DataStore) LoadDNSDiscoveries() ([]DNSDiscoveryEntry, error) {
	if ds == nil || ds.DataDir == "" {
		return []DNSDiscoveryEntry{}, nil
	}

	path := filepath.Join(ds.DataDir, "dns", "discoveries.json")
	if !exists(path) {
		return []DNSDiscoveryEntry{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var discoveries []DNSDiscoveryEntry
	if err := json.Unmarshal(data, &discoveries); err != nil {
		return nil, err
	}

	return discoveries, nil
}

// ClearDNSDiscoveries removes all DNS discoveries from the datastore.
func (ds *DataStore) ClearDNSDiscoveries() error {
	if ds == nil || ds.DataDir == "" {
		return nil
	}

	path := filepath.Join(ds.DataDir, "dns", "discoveries.json")
	if !exists(path) {
		return nil
	}

	return os.Remove(path)
}
