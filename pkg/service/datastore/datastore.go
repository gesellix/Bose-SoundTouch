// Package datastore provides a simple XML-based datastore for SoundTouch devices.
package datastore

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
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

// ErrGroupNotFound is returned when no group is found for a given device.
var ErrGroupNotFound = errors.New("group not found")

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isSafeIdentifier returns true if the given identifier is safe to use
// as a single path component (for account IDs, device IDs, etc.).
// It rejects empty strings, path separators, and parent directory references.
func isSafeIdentifier(id string) bool {
	if id == "" {
		return false
	}

	// Disallow obvious path traversal / multi-component paths.
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return false
	}

	// Allow a conservative set of characters commonly found in IDs:
	// letters, digits, underscore, dash, dot, and colon (for MAC-like IDs).
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' || c == ':' {
			continue
		}

		return false
	}

	return true
}

// DataStore represents the device and configuration storage.
type DataStore struct {
	// DataDir is the (possibly relative) base directory for all datastore files.
	DataDir string
	// baseDir is the absolute, normalized base directory used for path safety checks.
	baseDir string

	// rootMu guards lazy initialisation of root.
	rootMu sync.Mutex
	// root is an os.Root anchored at baseDir. All filesystem operations within
	// the datastore go through it, so ".." or absolute paths in
	// caller-supplied components cannot escape the root — the Go runtime
	// enforces containment regardless of what safeJoin's output looks like.
	// Lazily opened so NewDataStore stays a pure constructor.
	root *os.Root

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
// that the resulting absolute path stays within baseDir. If any element would
// escape baseDir (absolute path, "..", or — on Windows — a drive/colon), the
// function falls back to baseDir to prevent directory traversal.
//
// The validation up-front uses filepath.IsLocal, which CodeQL recognises as a
// path-traversal sanitiser, so taint analysis at call sites that subsequently
// hand the result to os.ReadFile / os.Open / os.Remove etc. propagates safely.
// The post-join prefix check below stays as belt-and-suspenders for any
// unusual platform behaviour IsLocal does not cover.
func (ds *DataStore) safeJoin(elem ...string) string {
	for _, e := range elem {
		if e == "" {
			// filepath.Join silently skips empty elements, but IsLocal
			// returns false for "" — treat empties as a no-op.
			continue
		}

		if !filepath.IsLocal(e) {
			// Element is absolute, contains ".." or a reserved Windows
			// component. Refuse to join.
			return ds.baseDir
		}
	}

	// Join the base directory with the (now sanitised) elements.
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

	// Belt-and-suspenders: ensure the resolved path is within the base
	// directory even if filepath.IsLocal somehow misjudged a component.
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

// getRoot returns the lazily-opened *os.Root anchored at baseDir. The root is
// created on first call after MkdirAll-ing baseDir; subsequent calls return
// the cached handle. Filesystem operations performed via the returned root
// cannot escape baseDir even if the relative path passed to them is malicious.
func (ds *DataStore) getRoot() (*os.Root, error) {
	ds.rootMu.Lock()
	defer ds.rootMu.Unlock()

	if ds.root != nil {
		return ds.root, nil
	}

	if ds.baseDir == "" {
		return nil, fmt.Errorf("datastore: baseDir not configured")
	}

	if err := os.MkdirAll(ds.baseDir, 0755); err != nil {
		return nil, fmt.Errorf("datastore: ensure baseDir %s: %w", ds.baseDir, err)
	}

	r, err := os.OpenRoot(ds.baseDir)
	if err != nil {
		return nil, fmt.Errorf("datastore: open root at %s: %w", ds.baseDir, err)
	}

	ds.root = r

	return r, nil
}

// Close releases any open filesystem handles held by the datastore. Safe to
// call on a never-used DataStore.
func (ds *DataStore) Close() error {
	ds.rootMu.Lock()
	defer ds.rootMu.Unlock()

	if ds.root == nil {
		return nil
	}

	err := ds.root.Close()
	ds.root = nil

	return err
}

// rootRel converts a path produced by safeJoin (or by filepath.Join over
// ds.DataDir) into the form expected by *os.Root methods — relative to
// baseDir, no leading separator. Tolerates both absolute paths and paths
// whose root is the relative ds.DataDir.
//
// Returns "." for baseDir itself.
func (ds *DataStore) rootRel(absPath string) (string, error) {
	// If the input is relative, absolutise so the comparison with baseDir
	// works regardless of how DataDir was originally configured.
	if !filepath.IsAbs(absPath) {
		a, err := filepath.Abs(absPath)
		if err != nil {
			return "", fmt.Errorf("datastore: absolutise %s: %w", absPath, err)
		}

		absPath = a
	}

	if absPath == ds.baseDir {
		return ".", nil
	}

	rel, err := filepath.Rel(ds.baseDir, absPath)
	if err != nil {
		return "", fmt.Errorf("datastore: %s is outside baseDir: %w", absPath, err)
	}

	if rel == "." || rel == "" {
		return ".", nil
	}

	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("datastore: %s is outside baseDir", absPath)
	}

	return rel, nil
}

// rootStat is the os.Stat equivalent for a path under baseDir.
func (ds *DataStore) rootStat(absPath string) (os.FileInfo, error) {
	r, err := ds.getRoot()
	if err != nil {
		return nil, err
	}

	rel, err := ds.rootRel(absPath)
	if err != nil {
		return nil, err
	}

	return r.Stat(rel)
}

// rootReadFile is the os.ReadFile equivalent.
func (ds *DataStore) rootReadFile(absPath string) ([]byte, error) {
	r, err := ds.getRoot()
	if err != nil {
		return nil, err
	}

	rel, err := ds.rootRel(absPath)
	if err != nil {
		return nil, err
	}

	return r.ReadFile(rel)
}

// rootWriteFile is the os.WriteFile equivalent.
func (ds *DataStore) rootWriteFile(absPath string, data []byte, perm os.FileMode) error {
	r, err := ds.getRoot()
	if err != nil {
		return err
	}

	rel, err := ds.rootRel(absPath)
	if err != nil {
		return err
	}

	return r.WriteFile(rel, data, perm)
}

// rootMkdirAll is the os.MkdirAll equivalent.
func (ds *DataStore) rootMkdirAll(absPath string, perm os.FileMode) error {
	r, err := ds.getRoot()
	if err != nil {
		return err
	}

	rel, err := ds.rootRel(absPath)
	if err != nil {
		return err
	}

	if rel == "." {
		return nil
	}

	return r.MkdirAll(rel, perm)
}

// rootRemove is the os.Remove equivalent.
func (ds *DataStore) rootRemove(absPath string) error {
	r, err := ds.getRoot()
	if err != nil {
		return err
	}

	rel, err := ds.rootRel(absPath)
	if err != nil {
		return err
	}

	return r.Remove(rel)
}

// rootRemoveAll is the os.RemoveAll equivalent.
func (ds *DataStore) rootRemoveAll(absPath string) error {
	r, err := ds.getRoot()
	if err != nil {
		return err
	}

	rel, err := ds.rootRel(absPath)
	if err != nil {
		return err
	}

	return r.RemoveAll(rel)
}

// rootRename is the os.Rename equivalent. Both paths must be under baseDir.
func (ds *DataStore) rootRename(oldAbs, newAbs string) error {
	r, err := ds.getRoot()
	if err != nil {
		return err
	}

	oldRel, err := ds.rootRel(oldAbs)
	if err != nil {
		return err
	}

	newRel, err := ds.rootRel(newAbs)
	if err != nil {
		return err
	}

	return r.Rename(oldRel, newRel)
}

// rootReadDir lists the entries in absPath. Equivalent to os.ReadDir,
// including the same alphabetical-by-name sort order — *os.File.ReadDir(-1)
// returns entries in directory order, but callers (and existing tests)
// depend on the sorted contract that os.ReadDir documents.
func (ds *DataStore) rootReadDir(absPath string) ([]os.DirEntry, error) {
	r, err := ds.getRoot()
	if err != nil {
		return nil, err
	}

	rel, err := ds.rootRel(absPath)
	if err != nil {
		return nil, err
	}

	f, err := r.Open(rel)
	if err != nil {
		return nil, err
	}

	defer func() { _ = f.Close() }()

	entries, err := f.ReadDir(-1)
	if err != nil {
		return entries, err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	return entries, nil
}

// rootExists is true when absPath exists under baseDir.
func (ds *DataStore) rootExists(absPath string) bool {
	_, err := ds.rootStat(absPath)
	return err == nil
}

// ReadDirUnderBase lists the entries in absPath, which must resolve to a
// directory under the datastore baseDir. Cross-package callers (marge,
// handlers, …) use this instead of os.ReadDir so that the underlying
// *os.Root sanitises the path against traversal.
func (ds *DataStore) ReadDirUnderBase(absPath string) ([]os.DirEntry, error) {
	return ds.rootReadDir(absPath)
}

// MkdirAllUnderBase creates a directory tree under baseDir.
func (ds *DataStore) MkdirAllUnderBase(absPath string, perm os.FileMode) error {
	return ds.rootMkdirAll(absPath, perm)
}

// WriteFileUnderBase atomically writes data to absPath, which must be under
// baseDir.
func (ds *DataStore) WriteFileUnderBase(absPath string, data []byte, perm os.FileMode) error {
	return ds.rootWriteFile(absPath, data, perm)
}

// rootOpen is the os.Open equivalent for a path under baseDir. The caller
// owns the returned *os.File and must Close it.
func (ds *DataStore) rootOpen(absPath string) (*os.File, error) {
	r, err := ds.getRoot()
	if err != nil {
		return nil, err
	}

	rel, err := ds.rootRel(absPath)
	if err != nil {
		return nil, err
	}

	return r.Open(rel)
}

// ListAccounts returns a list of all account IDs (directories in the data root).
func (ds *DataStore) ListAccounts() ([]string, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	// Account data is stored in 'accounts' subdirectory within the data root.
	accountsDir := filepath.Join(ds.baseDir, "accounts")
	if !ds.rootExists(accountsDir) {
		return []string{"default"}, nil
	}

	entries, err := ds.rootReadDir(accountsDir)
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
	if _, err := ds.rootStat(directPath); err == nil {
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
		if _, err := ds.rootStat(mappedPath); err == nil {
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

	data, err := ds.rootReadFile(deviceInfoPath)
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
		CreatedOn       string `xml:"createdOn,omitempty"`
		UpdatedOn       string `xml:"updatedOn,omitempty"`
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
		CreatedOn:       info.CreatedOn,
		UpdatedOn:       info.UpdatedOn,
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

	type seenEntry struct {
		index   int
		account string
	}

	seenIDs := make(map[string]seenEntry)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		// Sort "default" to the back so a real-account entry always
		// wins the first-seen race. "default" exists as a pre-pair
		// placeholder; once the speaker pairs with a real account it
		// becomes orphan state. We don't try to pick a winner among
		// multiple real accounts via mtime or other proxies — the
		// authoritative signal is the URL of the speaker's incoming
		// PUT, which only the live handler sees. Stale real-account
		// entries are flagged as findings by the consistency check
		// instead.
		accounts := make([]os.DirEntry, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				accounts = append(accounts, e)
			}
		}

		sort.SliceStable(accounts, func(i, j int) bool {
			ai, aj := accounts[i].Name(), accounts[j].Name()
			if ai == accountIDDefault && aj != accountIDDefault {
				return false
			}

			if aj == accountIDDefault && ai != accountIDDefault {
				return true
			}

			return ai < aj
		})

		for _, acc := range accounts {
			accDevices := ds.listDevicesInAccount(dir, acc.Name())
			for i := range accDevices {
				info := accDevices[i]
				info.AccountID = acc.Name()

				key := info.DeviceID
				if key == "" {
					key = info.IPAddress
				}

				entry, alreadySeen := seenIDs[key]
				if !alreadySeen {
					devices = append(devices, info)
					seenIDs[key] = seenEntry{index: len(devices) - 1, account: info.AccountID}

					continue
				}

				// "default" never replaces a real-account entry.
				// But when two "default" entries collide across data dirs,
				// prefer the one with a non-empty name (more information).
				if info.AccountID == accountIDDefault {
					if entry.account == accountIDDefault && devices[entry.index].Name == "" && info.Name != "" {
						devices[entry.index] = info
						seenIDs[key] = seenEntry{index: entry.index, account: info.AccountID}
					}

					continue
				}

				// First real-account encountered wins (sort is
				// stable + alphabetical so the result is
				// deterministic across runs). Don't pick a different
				// winner here — the consistency check enumerates all
				// the duplicate account dirs for the operator to
				// clean up.
				if entry.account == accountIDDefault {
					devices[entry.index] = info
					seenIDs[key] = seenEntry{index: entry.index, account: info.AccountID}
				}
			}
		}
	}

	return devices, nil
}

// AllAccountsForDevice returns every account directory that contains a
// device with the given deviceID, in their on-disk order. Used by the
// consistency check to enumerate stale account entries left behind
// when the speaker was re-paired to a different account.
func (ds *DataStore) AllAccountsForDevice(deviceID string) []string {
	if deviceID == "" {
		return nil
	}

	var hits []string

	seen := map[string]bool{}

	for _, dir := range ds.getPossibleDataDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, acc := range entries {
			if !acc.IsDir() {
				continue
			}

			devicePath := filepath.Join(dir, acc.Name(), "devices", deviceID)
			if _, err := os.Stat(devicePath); err != nil {
				continue
			}

			if !seen[acc.Name()] {
				seen[acc.Name()] = true
				hits = append(hits, acc.Name())
			}
		}
	}

	return hits
}

// accountIDDefault is the placeholder account id assigned to records
// that exist before a speaker has been paired with a real Marge
// account. Treated as fallback in ListAllDevices and skipped in
// consistency checks when a real-account entry exists for the same
// device.
const accountIDDefault = "default"

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
		CreatedOn       string `xml:"createdOn,omitempty"`
		UpdatedOn       string `xml:"updatedOn,omitempty"`
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
	presets, needsRewrite, err := ds.readPresetsLocked(account, device)
	if err != nil {
		return nil, err
	}

	if needsRewrite {
		log.Printf("[Datastore] Presets.xml for device %s used legacy <ContentItem> format; rewriting in canonical form", device)

		if werr := ds.SavePresets(account, device, presets); werr != nil {
			log.Printf("[Datastore] failed to rewrite normalised Presets.xml for device %s: %v", device, werr)
		}
	}

	return presets, nil
}

// readPresetsLocked is the locked read half of GetPresets. It returns the
// parsed presets and a flag indicating whether the on-disk file used the
// legacy <ContentItem> (capital C) format that needs rewriting.
func (ds *DataStore) readPresetsLocked(account, device string) ([]models.ServicePreset, bool, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.PresetsFile)

	data, err := ds.rootReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.ServicePreset{}, false, nil
		}

		return nil, false, err
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
			} `xml:"contentItem"`
			SourceID string `xml:"sourceid"`
		} `xml:"preset"`
	}

	// encoding/xml is case-sensitive. Older AfterTouch versions (and raw
	// speaker XML) used <ContentItem> (capital C); normalise to lowercase
	// before unmarshaling so legacy files are parsed correctly.
	normalized := bytes.ReplaceAll(data, []byte("<ContentItem"), []byte("<contentItem"))
	normalized = bytes.ReplaceAll(normalized, []byte("</ContentItem>"), []byte("</contentItem>"))

	needsRewrite := !bytes.Equal(normalized, data)

	if err := xml.Unmarshal(normalized, &presetsWrap); err != nil {
		return nil, false, fmt.Errorf("malformed presets XML at %s: %w", path, err)
	}

	presets := []models.ServicePreset{}

	for i := range presetsWrap.Presets {
		p := &presetsWrap.Presets[i]

		presets = append(presets, models.ServicePreset{
			ServiceContentItem: models.ServiceContentItem{
				Name:            p.ContentItem.ItemName,
				Source:          repairLeakedSource(account, device, "preset "+p.ID, p.ContentItem.Source, p.SourceID, ds),
				Type:            p.ContentItem.Type,
				ContentItemType: p.ContentItem.Type,
				Location:        p.ContentItem.Location,
				SourceAccount:   p.ContentItem.SourceAccount,
				IsPresetable:    p.ContentItem.IsPresetable,
				SourceID:        p.SourceID,
			},
			ID:           p.ID,
			ButtonNumber: p.ID,
			ContainerArt: p.ContentItem.ContainerArt,
			CreatedOn:    p.CreatedOn,
			UpdatedOn:    p.UpdatedOn,
		})
	}

	return presets, needsRewrite, nil
}

// repairLeakedSource quietly substitutes the speaker-perspective
// SourceKeyType for a persisted Source that has the protocol-level
// "Audio" leak signature from the legacy syncPresets/syncRecents path.
// Critically it only repairs the *leak* — when persisted Source carries
// a non-leak symbolic value, even one that disagrees with the current
// Sources.xml mapping for the same SourceID, we leave it alone. That's
// what protects the GH-343 case: a TUNEIN preset whose SourceID was
// re-classified to RADIOPLAYER in Sources.xml must stay TUNEIN here,
// otherwise the speaker's previously-stored intent gets silently
// overwritten by the stale source-list entry.
//
// Speaker is the source of truth: this only un-rots data that was
// never the speaker's perspective to begin with.
func repairLeakedSource(account, device, label, persistedSource, sourceID string, ds *DataStore) string {
	if !isLeakedSourceValue(persistedSource) {
		return persistedSource
	}

	if sourceID == "" {
		return persistedSource
	}

	sources, err := ds.getConfiguredSourcesLocked(account, device)
	if err != nil {
		return persistedSource
	}

	for i := range sources {
		if sources[i].ID == sourceID && sources[i].SourceKeyType != "" {
			log.Printf("[Datastore] %s: repaired leaked source %q -> %q via sourceid=%s (account=%s device=%s) — likely written by the pre-fix marge.syncPresets/syncRecents path; speaker's perspective is now restored on read",
				label, persistedSource, sources[i].SourceKeyType, sourceID, account, device)

			return sources[i].SourceKeyType
		}
	}

	return persistedSource
}

// isLeakedSourceValue identifies the known protocol-level leak signature
// (the upstream <source type="Audio"> attribute that the old
// marge.syncPresets persisted into ServicePreset.Source). Empty Source
// is also treated as a leak so the resolve fires for legacy entries
// missing the attribute entirely.
func isLeakedSourceValue(s string) bool {
	return s == "" || s == "Audio"
}

// getConfiguredSourcesLocked is GetConfiguredSources without the
// fileMutex.RLock() — callers must already hold it. Used by
// repairLeakedSource from within GetPresets/GetRecents which already
// hold the lock.
func (ds *DataStore) getConfiguredSourcesLocked(account, device string) ([]models.ConfiguredSource, error) {
	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.SourcesFile)

	data, err := ds.rootReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ds.getDefaultSources(), nil
		}

		return nil, err
	}

	type persistentSource struct {
		ID            string `xml:"id,attr,omitempty"`
		Type          string `xml:"type,attr,omitempty"`
		SourceKeyType string `xml:"-"`
		SourceKey     struct {
			Type    string `xml:"type,attr"`
			Account string `xml:"account,attr"`
		} `xml:"sourceKey"`
	}

	var sourcesWrap struct {
		Sources []persistentSource `xml:"source"`
	}

	if err := xml.Unmarshal(data, &sourcesWrap); err != nil {
		return nil, err
	}

	out := make([]models.ConfiguredSource, 0, len(sourcesWrap.Sources))
	for i := range sourcesWrap.Sources {
		ps := sourcesWrap.Sources[i]
		out = append(out, models.ConfiguredSource{
			ID:            ps.ID,
			Type:          ps.Type,
			SourceKeyType: ps.SourceKey.Type,
		})
	}

	return out, nil
}

// SavePresets saves the preset list for the specified account and device.
func (ds *DataStore) SavePresets(account, device string, presets []models.ServicePreset) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.PresetsFile)
	if err := ds.rootMkdirAll(filepath.Dir(path), 0755); err != nil {
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
		} `xml:"contentItem"`
		SourceID string `xml:"sourceid,omitempty"`
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

		// Preserve the speaker's IsPresetable verdict instead of forcing
		// "true". The speaker firmware sets isPresetable="false" for
		// content it can't independently recall later (e.g. Spotify Connect
		// pushes from a phone — see GH-235). Hard-coding "true" makes the
		// on-disk XML look valid while the speaker still refuses to play
		// the preset, which leaves users debugging a phantom "stored but
		// won't play" state. Default to "true" only when the caller
		// supplied nothing.
		pxml.ContentItem.IsPresetable = p.IsPresetable
		if pxml.ContentItem.IsPresetable == "" {
			pxml.ContentItem.IsPresetable = "true"
		}

		if pxml.ContentItem.IsPresetable == "false" {
			log.Printf("[Datastore] SavePresets: storing preset %s as isPresetable=false (account=%s device=%s source=%s) — speaker firmware marked this content non-recallable; preset will appear on the speaker but pressing it will not play",
				pxml.ID, account, device, p.Source)
		}

		pxml.ContentItem.ItemName = p.Name
		pxml.ContentItem.ContainerArt = p.ContainerArt
		pxml.SourceID = p.SourceID
		px.Presets = append(px.Presets, pxml)
	}

	data, err := xml.MarshalIndent(px, "", "    ")
	if err != nil {
		return err
	}

	header := []byte(xml.Header)

	return ds.atomicWriteFile(path, append(header, data...))
}

func (ds *DataStore) atomicWriteFile(filename string, data []byte) error {
	perm := os.FileMode(0644)

	tempFile := filename + ".tmp"
	if err := ds.rootWriteFile(tempFile, data, perm); err != nil {
		return err
	}

	return ds.rootRename(tempFile, filename)
}

// GetRecents returns the list of recently played items for the specified account and device.
func (ds *DataStore) GetRecents(account, device string) ([]models.ServiceRecent, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.RecentsFile)

	data, err := ds.rootReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.ServiceRecent{}, nil
		}

		return nil, err
	}

	type RecentXML struct {
		DeviceID    string `xml:"deviceID,attr,omitempty"`
		UtcTime     string `xml:"utcTime,attr,omitempty"`
		ID          string `xml:"id,attr"`
		ContentItem struct {
			Source        string `xml:"source,attr"`
			Type          string `xml:"type,attr"`
			Location      string `xml:"location,attr"`
			SourceAccount string `xml:"sourceAccount,attr"`
			IsPresetable  string `xml:"isPresetable,attr"`
			ItemName      string `xml:"itemName"`
			ContainerArt  string `xml:"containerArt,omitempty"`
		} `xml:"contentItem"`
		CreatedOn    string `xml:"createdOn,omitempty"`
		UpdatedOn    string `xml:"updatedOn,omitempty"`
		LastPlayedAt string `xml:"lastplayedat,omitempty"`
		SourceID     string `xml:"sourceid,omitempty"`
		Username     string `xml:"username,omitempty"`
	}

	type RecentsXML struct {
		XMLName xml.Name    `xml:"recents"`
		Recents []RecentXML `xml:"recent"`
	}

	var wrap RecentsXML
	if err := xml.Unmarshal(data, &wrap); err != nil {
		return nil, fmt.Errorf("malformed recents XML at %s: %w", path, err)
	}

	recents := make([]models.ServiceRecent, 0, len(wrap.Recents))
	maxID := 0

	for i := range wrap.Recents {
		rx := &wrap.Recents[i]
		r := models.ServiceRecent{
			DeviceID: rx.DeviceID,
			UtcTime:  rx.UtcTime,
			ServiceContentItem: models.ServiceContentItem{
				ID:            rx.ID,
				Name:          rx.ContentItem.ItemName,
				Source:        rx.ContentItem.Source,
				Type:          rx.ContentItem.Type,
				Location:      rx.ContentItem.Location,
				SourceAccount: rx.ContentItem.SourceAccount,
				IsPresetable:  rx.ContentItem.IsPresetable,
				ContainerArt:  rx.ContentItem.ContainerArt,
			},
			CreatedOn:    rx.CreatedOn,
			UpdatedOn:    rx.UpdatedOn,
			LastPlayedAt: rx.LastPlayedAt,
		}
		r.SourceID = rx.SourceID

		if id, err := strconv.Atoi(r.ID); err == nil {
			if id > maxID {
				maxID = id
			}
		}

		recents = append(recents, r)
	}

	for i := range recents {
		r := &recents[i]
		if r.ContentItemType == "" {
			if r.Type != "" {
				r.ContentItemType = r.Type
			}
		}

		if _, err := strconv.Atoi(recents[i].ID); err != nil || recents[i].ID == "" {
			maxID++
			recents[i].ID = strconv.Itoa(maxID)
		}

		r.Source = repairLeakedSource(account, device, "recent "+r.ID, r.Source, r.SourceID, ds)
	}

	return recents, nil
}

// SaveRecents saves the recent items list for the specified account and device.
func (ds *DataStore) SaveRecents(account, device string, recents []models.ServiceRecent) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	dir := ds.AccountDeviceDir(account, device)
	if err := ds.rootMkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, constants.RecentsFile)

	type RecentXML struct {
		DeviceID    string `xml:"deviceID,attr,omitempty"`
		UtcTime     string `xml:"utcTime,attr,omitempty"`
		ID          string `xml:"id,attr"`
		ContentItem struct {
			Source        string `xml:"source,attr"`
			Type          string `xml:"type,attr"`
			Location      string `xml:"location,attr"`
			SourceAccount string `xml:"sourceAccount,attr"`
			IsPresetable  string `xml:"isPresetable,attr"`
			ItemName      string `xml:"itemName"`
			ContainerArt  string `xml:"containerArt,omitempty"`
		} `xml:"contentItem"`
		CreatedOn    string `xml:"createdOn,omitempty"`
		UpdatedOn    string `xml:"updatedOn,omitempty"`
		LastPlayedAt string `xml:"lastplayedat,omitempty"`
		SourceID     string `xml:"sourceid,omitempty"`
		Username     string `xml:"username,omitempty"`
	}

	type RecentsXML struct {
		XMLName xml.Name    `xml:"recents"`
		Recents []RecentXML `xml:"recent"`
	}

	wrap := RecentsXML{
		Recents: make([]RecentXML, 0, len(recents)),
	}

	for i := range recents {
		r := &recents[i]
		rx := RecentXML{
			DeviceID:     r.DeviceID,
			UtcTime:      r.UtcTime,
			ID:           r.ID,
			CreatedOn:    r.CreatedOn,
			UpdatedOn:    r.UpdatedOn,
			LastPlayedAt: r.LastPlayedAt,
			SourceID:     r.SourceID,
			Username:     r.Username,
		}
		rx.ContentItem.Source = r.Source
		rx.ContentItem.Type = r.Type
		rx.ContentItem.Location = r.Location
		rx.ContentItem.SourceAccount = r.SourceAccount

		rx.ContentItem.IsPresetable = r.IsPresetable
		if rx.ContentItem.IsPresetable == "" {
			rx.ContentItem.IsPresetable = "true"
		}

		rx.ContentItem.ItemName = r.Name
		rx.ContentItem.ContainerArt = r.ContainerArt
		rx.SourceID = r.SourceID

		wrap.Recents = append(wrap.Recents, rx)
	}

	data, err := xml.MarshalIndent(wrap, "", "    ")
	if err != nil {
		return err
	}

	header := []byte(xml.Header)

	return ds.atomicWriteFile(path, append(header, data...))
}

// SaveDeviceInfo saves device information for the specified account and device.
func (ds *DataStore) SaveDeviceInfo(account, device string, info *models.ServiceDeviceInfo) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	if device == "" {
		return fmt.Errorf("device ID/name cannot be empty")
	}

	if !isSafeIdentifier(device) {
		return fmt.Errorf("invalid device ID")
	}

	if account == "" {
		return fmt.Errorf("account ID cannot be empty")
	}

	if !isSafeIdentifier(account) {
		return fmt.Errorf("invalid account ID")
	}

	// Try to load existing device info to avoid overwriting existing details with empty values.
	ds.mergeWithExistingDeviceInfo(account, device, info)

	dir := ds.AccountDeviceDir(account, device)
	if err := ds.rootMkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, constants.DeviceInfoFile)

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
		Components      []componentXML   `xml:"components>component"`
		NetworkInfo     []NetworkInfoXML `xml:"networkInfo"`
		DiscoveryMethod string           `xml:"discoveryMethod,omitempty"`
		CreatedOn       string           `xml:"createdOn,omitempty"`
		UpdatedOn       string           `xml:"updatedOn,omitempty"`
	}

	// Parsing product code back to type and moduleType (best effort)
	devType, moduleType := ds.parseProductCode(info.ProductCode)

	ix := InfoXML{
		DeviceID:        info.DeviceID,
		Name:            info.Name,
		Type:            devType,
		ModuleType:      moduleType,
		DiscoveryMethod: info.DiscoveryMethod,
		CreatedOn:       info.CreatedOn,
		UpdatedOn:       info.UpdatedOn,
	}

	if ix.DiscoveryMethod == "" {
		ix.DiscoveryMethod = "sync_full"
	}

	ix.Components = ds.buildComponentsXML(info)

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

	return ds.atomicWriteFile(path, append(header, data...))
}

func (ds *DataStore) mergeWithExistingDeviceInfo(account, device string, info *models.ServiceDeviceInfo) {
	existing, _ := ds.getDeviceInfoNoLock(account, device)
	if existing == nil {
		return
	}

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

	// CreatedOn is set once at first persistence and never re-derived
	// from inbound data — preserve unconditionally so the
	// "first-paired" timestamp survives renames, IP refreshes, etc.
	// UpdatedOn is the opposite: every write that reaches here is by
	// definition an update, so callers that want it refreshed must
	// set it explicitly. If they didn't, fall back to the existing
	// value (better than a regression to empty).
	if existing.CreatedOn != "" {
		info.CreatedOn = existing.CreatedOn
	}

	if info.UpdatedOn == "" {
		info.UpdatedOn = existing.UpdatedOn
	}
}

func (ds *DataStore) parseProductCode(productCode string) (string, string) {
	devType := productCode
	moduleType := ""

	for i := 0; i < len(productCode); i++ {
		if productCode[i] == ' ' {
			devType = productCode[:i]
			moduleType = productCode[i+1:]

			break
		}
	}

	return devType, moduleType
}

type componentXML struct {
	ComponentCategory string `xml:"componentCategory"`
	SoftwareVersion   string `xml:"softwareVersion,omitempty"`
	SerialNumber      string `xml:"serialNumber,omitempty"`
}

func (ds *DataStore) buildComponentsXML(info *models.ServiceDeviceInfo) []componentXML {
	var components []componentXML
	for _, comp := range info.Components {
		components = append(components, componentXML{
			ComponentCategory: comp.Category,
			SoftwareVersion:   comp.SoftwareVersion,
			SerialNumber:      comp.SerialNumber,
		})
	}

	if len(components) == 0 && (info.FirmwareVersion != "" || info.DeviceSerialNumber != "" || info.ProductSerialNumber != "") {
		components = []componentXML{
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
	} else if len(components) > 0 {
		if info.FirmwareVersion != "" {
			for i := range components {
				if components[i].ComponentCategory == "SCM" {
					components[i].SoftwareVersion = info.FirmwareVersion
					break
				}
			}
		}
	}

	return components
}

// SaveAccountInfo stores account-level metadata in the datastore.
func (ds *DataStore) SaveAccountInfo(accountID string, info *models.ServiceAccountInfo) error {
	if ds == nil || ds.DataDir == "" || accountID == "" {
		return nil
	}

	dir := ds.AccountDir(accountID)
	if err := ds.rootMkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, "account.json")

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	return ds.atomicWriteFile(path, data)
}

// GetAccountInfo retrieves account-level metadata from the datastore.
func (ds *DataStore) GetAccountInfo(accountID string) (*models.ServiceAccountInfo, error) {
	if ds == nil || ds.DataDir == "" || accountID == "" {
		return &models.ServiceAccountInfo{AccountID: accountID}, nil
	}

	// Try account root (canonical location)
	path := filepath.Join(ds.AccountDir(accountID), "account.json")
	if !ds.rootExists(path) {
		return &models.ServiceAccountInfo{AccountID: accountID, IsPlaceholder: true}, nil
	}

	data, err := ds.rootReadFile(path)
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

	return ds.rootRemoveAll(dir)
}

// RemoveDeviceDir is an alias for RemoveDevice for backwards compatibility.
func (ds *DataStore) RemoveDeviceDir(account, device string) error {
	return ds.RemoveDevice(account, device)
}

// DeduceSourceIDs updates the source IDs in the given slice by deducing them from recents and presets.
func (ds *DataStore) DeduceSourceIDs(account, device string, sources []models.ConfiguredSource) {
	// Deduce source IDs from recents and presets
	deducedIDs := ds.collectDeducedIDs(account, device)

	for i := range sources {
		if id, ok := deducedIDs[sources[i].SourceProviderID]; ok {
			sources[i].ID = id
		} else if sources[i].SourceKeyType == "AUX" {
			auxID := strconv.Itoa(constants.AuxProviderID)
			if id, ok := deducedIDs[auxID]; ok {
				sources[i].ID = id
				sources[i].SourceProviderID = auxID
			}
		}
	}
}

func (ds *DataStore) collectDeducedIDs(account, device string) map[string]string {
	deducedIDs := make(map[string]string)

	// Check recents and presets to find source IDs for provider IDs 2, 9, 11, 25
	for _, filename := range []string{constants.RecentsFile, constants.PresetsFile} {
		fileContent, err := ds.rootReadFile(filepath.Join(ds.AccountDeviceDir(account, device), filename))
		if err != nil {
			continue
		}

		ds.parseIDsFromFile(fileContent, deducedIDs)
	}

	return deducedIDs
}

func (ds *DataStore) parseIDsFromFile(fileContent []byte, deducedIDs map[string]string) {
	decoder := xml.NewDecoder(bytes.NewReader(fileContent))
	for {
		token, _ := decoder.Token()
		if token == nil {
			break
		}

		if se, ok := token.(xml.StartElement); ok {
			switch se.Name.Local {
			case "source":
				ds.parseSourceElement(decoder, &se, deducedIDs)
			case "recent", "preset":
				ds.parseRecentPresetElement(decoder, &se, deducedIDs)
			}
		}
	}
}

func (ds *DataStore) parseSourceElement(decoder *xml.Decoder, se *xml.StartElement, deducedIDs map[string]string) {
	var s struct {
		ID               string `xml:"id,attr"`
		SourceProviderID string `xml:"sourceproviderid"`
		// Also check for sourceproviderid as attribute just in case
		SourceProviderIDAttr string `xml:"sourceproviderid,attr"`
	}
	if err := decoder.DecodeElement(&s, se); err == nil {
		pid := s.SourceProviderID
		if pid == "" {
			pid = s.SourceProviderIDAttr
		}

		ds.extractIDs(pid, s.ID, deducedIDs)
	}
}

func (ds *DataStore) parseRecentPresetElement(decoder *xml.Decoder, se *xml.StartElement, deducedIDs map[string]string) {
	var s struct {
		SourceID         string `xml:"sourceid"`
		SourceProviderID string `xml:"sourceproviderid"`
		ContentItem      struct {
			Source string `xml:"source,attr"`
			Type   string `xml:"type,attr"`
		} `xml:"contentItem"`
		Source struct {
			SourceProviderID string `xml:"sourceproviderid"`
		} `xml:"source"`
	}
	if err := decoder.DecodeElement(&s, se); err == nil {
		pid := s.SourceProviderID
		if pid == "" {
			pid = s.Source.SourceProviderID
		}

		if pid == "" {
			// For AUX, we often don't have provider ID 9 but we know its name/source
			switch s.ContentItem.Source {
			case constants.ProviderAux:
				pid = strconv.Itoa(constants.AuxProviderID)
			case constants.ProviderInternetRadio:
				pid = strconv.Itoa(constants.InternetRadioProviderID)
			case constants.ProviderLocalInternetRadio:
				pid = strconv.Itoa(constants.LocalInternetRadioProviderID)
			case constants.ProviderTunein:
				pid = strconv.Itoa(constants.TuneinProviderID)
			}
		}

		ds.extractIDs(pid, s.SourceID, deducedIDs)
	}
}

func (ds *DataStore) extractIDs(providerID, sourceID string, deducedIDs map[string]string) {
	if sourceID == "" || providerID == "" {
		return
	}
	// Stick to the provider ids mentioned: 2, 9, 11, 25
	switch providerID {
	case strconv.Itoa(constants.InternetRadioProviderID),
		strconv.Itoa(constants.AuxProviderID),
		strconv.Itoa(constants.LocalInternetRadioProviderID),
		strconv.Itoa(constants.TuneinProviderID):
		if _, exists := deducedIDs[providerID]; !exists {
			deducedIDs[providerID] = sourceID
		}
	}
}

// GetConfiguredSources retrieves all configured sources for the specified account and device.
func (ds *DataStore) GetConfiguredSources(account, device string) ([]models.ConfiguredSource, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.SourcesFile)

	data, err := ds.rootReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			sources := ds.getDefaultSources()
			ds.DeduceSourceIDs(account, device, sources)

			return sources, nil
		}

		return nil, err
	}

	type persistentSource struct {
		DisplayName      string `xml:"displayName,attr,omitempty"`
		ID               string `xml:"id,attr,omitempty"`
		Secret           string `xml:"secret,attr"`
		SecretType       string `xml:"secretType,attr"`
		Type             string `xml:"type,attr,omitempty"`
		CreatedOn        string `xml:"createdOn,attr,omitempty"`
		UpdatedOn        string `xml:"updatedOn,attr,omitempty"`
		SourceProviderID string `xml:"sourceproviderid,attr,omitempty"`
		Credential       struct {
			Type  string `xml:"type,attr"`
			Value string `xml:",chardata"`
		} `xml:"credential,omitempty"`
		SourceKey struct {
			Type    string `xml:"type,attr"`
			Account string `xml:"account,attr"`
		} `xml:"sourceKey"`
	}

	var sourcesWrap struct {
		Sources []persistentSource `xml:"source"`
	}

	if err := xml.Unmarshal(data, &sourcesWrap); err != nil {
		return nil, fmt.Errorf("malformed sources XML at %s: %w", path, err)
	}

	sources := make([]models.ConfiguredSource, len(sourcesWrap.Sources))
	defaults := ds.getDefaultSources()

	// Pre-claim IDs already explicitly set in the file so the canonical fill
	// below doesn't reuse them when multiple entries share a SourceKey.Type.
	claimedIDs := make(map[string]bool, len(sourcesWrap.Sources))
	for i := range sourcesWrap.Sources {
		if id := sourcesWrap.Sources[i].ID; id != "" {
			claimedIDs[id] = true
		}
	}

	for i := range sourcesWrap.Sources {
		ps := &sourcesWrap.Sources[i]
		s := &sources[i]

		s.DisplayName = ps.DisplayName
		s.ID = ps.ID
		s.Secret = ps.Secret
		s.SecretType = ps.SecretType
		s.Type = ps.Type
		s.CreatedOn = ps.CreatedOn
		s.UpdatedOn = ps.UpdatedOn
		s.SourceProviderID = ps.SourceProviderID
		s.SourceKey.Type = ps.SourceKey.Type
		s.SourceKey.Account = ps.SourceKey.Account

		// Prioritize Credential element if present, otherwise use secret/secretType attributes
		if ps.Credential.Value != "" {
			s.Secret = ps.Credential.Value
			s.SecretType = ps.Credential.Type
		}

		// Ensure Secret/SecretType values are prioritized from legacy fields if still missing
		if s.Secret == "" && s.Credential.Value != "" {
			s.Secret = s.Credential.Value
		}

		if s.SecretType == "" && s.Credential.Type != "" {
			s.SecretType = s.Credential.Type
		}

		// Ensure SourceKey values are prioritized for legacy fields
		if s.SourceKey.Type != "" {
			s.SourceKeyType = s.SourceKey.Type
		}

		if s.SourceKey.Account != "" {
			s.SourceKeyAccount = s.SourceKey.Account
		}

		applyCanonicalDefaults(s, defaults, claimedIDs)

		// Last-resort ID for unknown providers.
		if s.ID == "" {
			s.ID = strconv.Itoa(2000001 + i)
		}
	}

	return sources, nil
}

// applyCanonicalDefaults fills missing canonical ID/Type/SourceProviderID for
// known providers and repairs Type that was previously synthesized from
// SourceKey.Type (e.g. "AUX") rather than the canonical value (e.g. "Audio").
// Without this, the on-device Sources.xml — which carries only displayName +
// sourceKey — would round-trip as id="2000001+i" type="<sourceKey.Type>" and
// be rejected by the speaker as INVALID_SOURCE after migration.
//
// claimedIDs tracks which canonical IDs are already in use so that multiple
// entries with the same SourceKey.Type don't collide on the same ID.
func applyCanonicalDefaults(s *models.ConfiguredSource, defaults []models.ConfiguredSource, claimedIDs map[string]bool) {
	def := findCanonicalSource(defaults, s.SourceKey.Type)
	if def == nil {
		return
	}

	if s.ID == "" && !claimedIDs[def.ID] {
		s.ID = def.ID
		claimedIDs[def.ID] = true
	}

	if s.Type == "" || s.Type == s.SourceKey.Type {
		s.Type = def.Type
	}

	if s.SourceProviderID == "" {
		s.SourceProviderID = def.SourceProviderID
	}
}

// findCanonicalSource returns the default source matching the given
// SourceKey.Type, or nil if it's not one of our known providers.
func findCanonicalSource(defaults []models.ConfiguredSource, sourceKeyType string) *models.ConfiguredSource {
	if sourceKeyType == "" {
		return nil
	}

	for i := range defaults {
		if defaults[i].SourceKey.Type == sourceKeyType {
			return &defaults[i]
		}
	}

	return nil
}

// SaveConfiguredSources saves the configured sources list for the specified account and device.
func (ds *DataStore) SaveConfiguredSources(account, device string, sources []models.ConfiguredSource) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.SourcesFile)
	if err := ds.rootMkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	type persistentSource struct {
		DisplayName      string `xml:"displayName,attr,omitempty"`
		ID               string `xml:"id,attr,omitempty"`
		Secret           string `xml:"secret,attr"`
		SecretType       string `xml:"secretType,attr"`
		Type             string `xml:"type,attr,omitempty"`
		CreatedOn        string `xml:"createdOn,attr,omitempty"`
		UpdatedOn        string `xml:"updatedOn,attr,omitempty"`
		SourceProviderID string `xml:"sourceproviderid,attr,omitempty"`
		Credential       struct {
			Type  string `xml:"type,attr"`
			Value string `xml:",chardata"`
		} `xml:"credential,omitempty"`
		SourceKey struct {
			Type    string `xml:"type,attr"`
			Account string `xml:"account,attr"`
		} `xml:"sourceKey"`
	}

	// Deduplicate by ID before saving; first occurrence wins to preserve established data
	seen := make(map[string]bool)

	deduped := make([]models.ConfiguredSource, 0, len(sources))
	for i := range sources {
		s := &sources[i]
		if s.ID != "" {
			if seen[s.ID] {
				continue
			}

			seen[s.ID] = true
		}

		deduped = append(deduped, *s)
	}

	sources = deduped

	// Ensure SourceKey is populated from legacy fields if necessary before saving
	// and map to persistentSource to avoid custom MarshalXML for disk storage
	persistSources := make([]persistentSource, len(sources))
	for i := range sources {
		s := sources[i]
		if s.SourceKey.Type == "" && s.SourceKeyType != "" {
			s.SourceKey.Type = s.SourceKeyType
		}

		if s.SourceKey.Account == "" && s.SourceKeyAccount != "" {
			s.SourceKey.Account = s.SourceKeyAccount
		}

		persistSources[i] = persistentSource{
			DisplayName:      s.DisplayName,
			ID:               s.ID,
			Secret:           s.Secret,
			SecretType:       s.SecretType,
			Type:             s.Type,
			CreatedOn:        s.CreatedOn,
			UpdatedOn:        s.UpdatedOn,
			SourceProviderID: s.SourceProviderID,
		}

		// Save to Credential element as well for parity with official Bose format
		if s.Secret != "" {
			persistSources[i].Credential.Value = s.Secret
			persistSources[i].Credential.Type = s.SecretType
		} else if s.Credential.Value != "" {
			persistSources[i].Credential.Value = s.Credential.Value
			persistSources[i].Credential.Type = s.Credential.Type
		}

		if persistSources[i].Secret == "" && s.Credential.Value != "" {
			persistSources[i].Secret = s.Credential.Value
		}

		if persistSources[i].SecretType == "" && s.Credential.Type != "" {
			persistSources[i].SecretType = s.Credential.Type
		}

		persistSources[i].SourceKey.Type = s.SourceKey.Type
		persistSources[i].SourceKey.Account = s.SourceKey.Account
	}

	wrap := struct {
		XMLName xml.Name           `xml:"sources"`
		Sources []persistentSource `xml:"source"`
	}{
		Sources: persistSources,
	}

	data, err := xml.MarshalIndent(wrap, "", "    ")
	if err != nil {
		return err
	}

	header := []byte(xml.Header)

	return ds.atomicWriteFile(path, append(header, data...))
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

// GenerateSerialSecret generates a base64 encoded JSON object with the specified serial.
func GenerateSerialSecret(serial string) string {
	m := map[string]string{"serial": serial}

	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}

	return base64.StdEncoding.EncodeToString(b)
}

// GetDefaultSources returns the list of default sources.
func (ds *DataStore) GetDefaultSources() []models.ConfiguredSource {
	return ds.getDefaultSources()
}

// CanonicalSourceByID returns the canonical default ConfiguredSource for one
// of the well-known built-in source IDs (10001..10005). The returned source
// has its SourceKey.Type / SourceKey.Account mirrored from
// SourceKeyType / SourceKeyAccount so it round-trips through SaveConfiguredSources
// without losing the embedded struct fields. Returns (zero, false) when the
// ID isn't one we can synthesise.
//
// Callers that auto-add missing sources (e.g. UpdatePreset when a speaker
// PUTs a preset referencing a canonical source AfterTouch hasn't been told
// about yet) should use this to keep parity with the post-pair defaults.
func (ds *DataStore) CanonicalSourceByID(id string) (models.ConfiguredSource, bool) {
	defaults := ds.getDefaultSources()
	for i := range defaults {
		if defaults[i].ID == id {
			return defaults[i], true
		}
	}

	return models.ConfiguredSource{}, false
}

func (ds *DataStore) getDefaultSources() []models.ConfiguredSource {
	sources := []models.ConfiguredSource{
		{
			ID:               "10001",
			DisplayName:      "AUX IN",
			SourceKeyType:    constants.ProviderAux,
			SourceKeyAccount: constants.ProviderAux,
			Type:             "Audio",
			Status:           "READY",
			CreatedOn:        "2015-03-11T19:12:38.000+00:00",
			UpdatedOn:        "2015-03-11T19:12:38.000+00:00",
		},
		{
			ID:               "10002",
			DisplayName:      "",
			SourceKeyType:    constants.ProviderInternetRadio,
			SourceKeyAccount: "",
			SourceProviderID: strconv.Itoa(constants.InternetRadioProviderID),
			Type:             "Audio",
			SecretType:       "token",
			Status:           "READY",
			CreatedOn:        "2015-03-11T19:12:38.000+00:00",
			UpdatedOn:        "2015-03-11T19:12:38.000+00:00",
		},
		{
			ID:               "10003",
			DisplayName:      "",
			SourceKeyType:    constants.ProviderLocalInternetRadio,
			SourceKeyAccount: "",
			SourceProviderID: strconv.Itoa(constants.LocalInternetRadioProviderID),
			Type:             "Audio",
			Secret:           GenerateSerialSecret("local-internet-radio"),
			SecretType:       "token",
			Status:           "READY",
			CreatedOn:        "2019-01-24T08:18:37.000+00:00",
			UpdatedOn:        "2019-02-03T18:35:45.000+00:00",
		},
		{
			ID:               "10004",
			DisplayName:      "",
			SourceKeyType:    constants.ProviderTunein,
			SourceKeyAccount: "",
			SourceProviderID: strconv.Itoa(constants.TuneinProviderID),
			Type:             "Audio",
			Secret:           GenerateSerialSecret("tunein"),
			SecretType:       "token",
			Status:           "READY",
			CreatedOn:        "2017-07-20T16:43:48.000+00:00",
			UpdatedOn:        "2017-07-20T16:43:48.000+00:00",
		},
		{
			ID:               "10005",
			DisplayName:      "",
			SourceKeyType:    constants.ProviderRadioBrowser,
			SourceKeyAccount: "",
			SourceProviderID: strconv.Itoa(constants.RadioBrowserProviderID),
			Type:             "Audio",
			SecretType:       "token",
			Status:           "READY",
			CreatedOn:        "2026-02-16T01:01:01.000+00:00",
			UpdatedOn:        "2026-02-16T01:01:01.000+00:00",
		},
	}

	for i := range sources {
		sources[i].SourceKey.Type = sources[i].SourceKeyType
		sources[i].SourceKey.Account = sources[i].SourceKeyAccount
	}

	return sources
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

	info, err := ds.rootStat(path)
	if err != nil {
		return 0
	}

	return info.ModTime().UnixNano() / int64(time.Millisecond)
}

// HasConfiguredSources reports whether a Sources.xml file exists for the given account and device.
func (ds *DataStore) HasConfiguredSources(account, device string) bool {
	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.SourcesFile)
	_, err := ds.rootStat(path)

	return err == nil
}

// GetETagForSources returns the ETag (modification time) for the sources file for a specific device.
func (ds *DataStore) GetETagForSources(account, device string) int64 {
	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.SourcesFile)

	info, err := ds.rootStat(path)
	if err != nil {
		return 0
	}

	return info.ModTime().UnixNano() / int64(time.Millisecond)
}

// GetETagForRecents returns the ETag (modification time) for the recents file for a specific device.
func (ds *DataStore) GetETagForRecents(account, device string) int64 {
	path := filepath.Join(ds.AccountDeviceDir(account, device), constants.RecentsFile)

	info, err := ds.rootStat(path)
	if err != nil {
		return 0
	}

	return info.ModTime().UnixNano() / int64(time.Millisecond)
}

// GetETagForAccount returns a content hash (SHA-256) over presets, sources, and recents for the account and device.
// If device is empty, it hashes across all devices in the account.
// The default sources fingerprint is always included so that newly added defaults (e.g. Amazon)
// invalidate cached responses even when the stored Sources.xml has not changed.
func (ds *DataStore) GetETagForAccount(account, device string) string {
	h := sha256.New()

	// Include the default sources fingerprint so mergeDefaultSources changes are visible.
	defaults := ds.GetDefaultSources()
	for i := range defaults {
		_, _ = io.WriteString(h, defaults[i].ID+defaults[i].SourceKeyType+defaults[i].DisplayName)
	}

	if device != "" {
		deviceDir := ds.AccountDeviceDir(account, device)
		for _, name := range []string{constants.PresetsFile, constants.SourcesFile, constants.RecentsFile} {
			f, err := ds.rootOpen(filepath.Join(deviceDir, name))
			if err != nil {
				continue
			}

			_, _ = io.Copy(h, f)
			_ = f.Close()
		}

		return hex.EncodeToString(h.Sum(nil))
	}

	devicesDir := ds.AccountDevicesDir(account)

	// Ignore error: missing directory is treated as no devices, producing a
	// stable non-empty hash rather than "" which would false-match an absent
	// If-None-Match header and return 304 on the first request.
	entries, _ := ds.rootReadDir(devicesDir)

	for _, entry := range entries {
		if entry.IsDir() {
			deviceDir := ds.AccountDeviceDir(account, entry.Name())
			for _, name := range []string{constants.PresetsFile, constants.SourcesFile, constants.RecentsFile} {
				f, err := ds.rootOpen(filepath.Join(deviceDir, name))
				if err != nil {
					continue
				}

				_, _ = io.Copy(h, f)
				_ = f.Close()
			}
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// Settings represents the global service settings.
type Settings struct {
	ServerURL           string         `json:"server_url"`
	HTTPServerURL       string         `json:"https_server_url,omitempty"`
	RedactLogs          bool           `json:"redact_logs"`
	LogBodies           bool           `json:"log_bodies"`
	RecordInteractions  bool           `json:"record_interactions"`
	DiscoveryInterval   string         `json:"discovery_interval,omitempty"`
	DiscoveryEnabled    bool           `json:"discovery_enabled"`
	DNSEnabled          bool           `json:"dns_enabled"`
	DNSUpstream         []string       `json:"dns_upstream,omitempty"`
	DNSBindAddr         string         `json:"dns_bind_addr,omitempty"`
	InternalPaths       []string       `json:"internal_paths,omitempty"`
	Shortcuts           map[string]int `json:"shortcuts,omitempty"`
	SpotifyClientID     string         `json:"spotify_client_id,omitempty"`
	SpotifyClientSecret string         `json:"spotify_client_secret,omitempty"`
	SpotifyRedirectURI  string         `json:"spotify_redirect_uri,omitempty"`
	AmazonClientID      string         `json:"amazon_client_id,omitempty"`
	AmazonClientSecret  string         `json:"amazon_client_secret,omitempty"`
	AmazonRedirectURI   string         `json:"amazon_redirect_uri,omitempty"`

	// TrustForwardedHeaders enables proxy-aware client IP resolution: when the
	// immediate TCP peer is one of the TrustedProxyCIDRs, the X-Real-IP /
	// X-Forwarded-For / True-Client-IP headers are honoured and replace
	// r.RemoteAddr. Required when the service is fronted by nginx, Caddy, or
	// any other reverse proxy. Default false — direct LAN deployments must
	// not enable this, otherwise a malicious LAN-resident client could spoof
	// its source IP via these headers.
	TrustForwardedHeaders bool `json:"trust_forwarded_headers,omitempty"`

	// TrustedProxyCIDRs is the list of CIDR blocks whose immediate TCP peers
	// are allowed to set X-Forwarded-* headers when TrustForwardedHeaders is
	// true. Defaults to loopback (127.0.0.0/8 and ::1/128) — i.e. only a
	// reverse proxy on the same host. Override only if the proxy lives on a
	// different host within a known-good private subnet.
	TrustedProxyCIDRs []string `json:"trusted_proxy_cidrs,omitempty"`

	// TLSExtraHosts is the persisted list of additional DNS names or IPs
	// to include in the TLS certificate SAN list. Merged with the
	// CLI/env --tls-extra-host values at startup (CLI/env wins; persisted
	// values are additive and deduplicated). Applying a change requires a
	// service restart so the TLS cert can be regenerated. Used by the
	// `speaker_marge_url` health check's QuickFix and the Settings tab UI.
	TLSExtraHosts []string `json:"tls_extra_hosts,omitempty"`

	// TuneInStreamFormats overrides the comma-separated format list
	// AfterTouch sends to TuneIn's Tune.ashx (formats=…). Empty value
	// uses bmx.DefaultTuneInStreamFormats ("mp3,aac,ogg"), which
	// matches AfterTouch's pre-2026-05-10 behaviour and plays on
	// every SoundTouch model verified so far. PR #249 had added
	// "hls" unconditionally; that regressed playback on the
	// SoundTouch line (#292 — speaker can't parse the .m3u8 playlist
	// and blinks amber). Operators with HLS-compatible speakers can
	// set this to e.g. "mp3,aac,ogg,hls" via settings.json. The value
	// is passed through verbatim; AfterTouch does not validate the
	// individual format tokens.
	TuneInStreamFormats string `json:"tunein_stream_formats,omitempty"`
}

// GetSettings retrieves the global service settings.
func (ds *DataStore) GetSettings() (Settings, error) {
	if ds == nil || ds.DataDir == "" {
		return Settings{}, nil
	}

	path := filepath.Join(ds.DataDir, "settings.json")
	if !ds.rootExists(path) {
		return Settings{}, nil
	}

	data, err := ds.rootReadFile(path)
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

	if err := ds.rootMkdirAll(ds.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	path := filepath.Join(ds.DataDir, "settings.json")

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return ds.atomicWriteFile(path, data)
}

// SaveUsageStats saves usage statistics to the datastore.
func (ds *DataStore) SaveUsageStats(stats models.UsageStats) error {
	dir := filepath.Join(ds.DataDir, "stats", "usage")
	if err := ds.rootMkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%d_%s.json", time.Now().UnixNano(), stats.DeviceID)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	return ds.atomicWriteFile(path, data)
}

// SaveErrorStats saves error statistics to the datastore.
func (ds *DataStore) SaveErrorStats(stats models.ErrorStats) error {
	dir := filepath.Join(ds.DataDir, "stats", "error")
	if err := ds.rootMkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%d_%s.json", time.Now().UnixNano(), stats.DeviceID)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	return ds.atomicWriteFile(path, data)
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
	if err := ds.rootMkdirAll(dir, 0755); err != nil {
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

	return ds.atomicWriteFile(path, data)
}

// LoadDNSDiscoveries loads DNS discoveries from the datastore.
func (ds *DataStore) LoadDNSDiscoveries() ([]DNSDiscoveryEntry, error) {
	if ds == nil || ds.DataDir == "" {
		return []DNSDiscoveryEntry{}, nil
	}

	path := filepath.Join(ds.DataDir, "dns", "discoveries.json")
	if !ds.rootExists(path) {
		return []DNSDiscoveryEntry{}, nil
	}

	data, err := ds.rootReadFile(path)
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
	if !ds.rootExists(path) {
		return nil
	}

	return ds.rootRemove(path)
}

// groupFilePath returns the on-disk path for a group file.
func (ds *DataStore) groupFilePath(account, groupID string) string {
	return filepath.Join(ds.AccountDevicesDir(account), "Group_"+groupID+".xml")
}

// generateGroupID returns a unique 7-digit group ID that has no existing file.
func (ds *DataStore) generateGroupID(account string) string {
	for {
		id := fmt.Sprintf("%07d", rand.Int63n(10_000_000)) //nolint:gosec
		if !ds.rootExists(ds.groupFilePath(account, id)) {
			return id
		}
	}
}

// GetGroupForDevice returns the group containing the given device, or nil if ungrouped.
func (ds *DataStore) GetGroupForDevice(account, deviceID string) (*models.Group, error) {
	ds.fileMutex.RLock()
	defer ds.fileMutex.RUnlock()

	dir := ds.AccountDevicesDir(account)

	entries, err := ds.rootReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrGroupNotFound
		}

		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "Group_") || !strings.HasSuffix(e.Name(), ".xml") {
			continue
		}

		data, readErr := ds.rootReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			continue
		}

		var g models.Group
		if unmarshalErr := xml.Unmarshal(data, &g); unmarshalErr != nil {
			continue
		}

		for _, role := range g.Roles.Roles {
			if role.DeviceID == deviceID {
				return &g, nil
			}
		}
	}

	return nil, ErrGroupNotFound
}

// AddGroup saves a new group to disk and returns its generated ID.
func (ds *DataStore) AddGroup(account string, group *models.Group) (string, error) {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	dir := ds.AccountDevicesDir(account)
	if err := ds.rootMkdirAll(dir, 0755); err != nil {
		return "", err
	}

	id := ds.generateGroupID(account)
	group.ID = id

	data, err := xml.MarshalIndent(group, "", "    ")
	if err != nil {
		return "", err
	}

	return id, ds.atomicWriteFile(ds.groupFilePath(account, id), append([]byte(xml.Header), data...))
}

// ModifyGroup updates the name of an existing group and returns the updated group.
func (ds *DataStore) ModifyGroup(account, groupID, newName string) (*models.Group, error) {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	path := ds.groupFilePath(account, groupID)

	data, err := ds.rootReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("group %s not found", groupID)
		}

		return nil, err
	}

	var g models.Group
	if xmlErr := xml.Unmarshal(data, &g); xmlErr != nil {
		return nil, xmlErr
	}

	g.Name = newName

	updated, err := xml.MarshalIndent(&g, "", "    ")
	if err != nil {
		return nil, err
	}

	if err := ds.atomicWriteFile(path, append([]byte(xml.Header), updated...)); err != nil {
		return nil, err
	}

	return &g, nil
}

// DeleteGroup removes a group from disk.
func (ds *DataStore) DeleteGroup(account, groupID string) error {
	ds.fileMutex.Lock()
	defer ds.fileMutex.Unlock()

	err := ds.rootRemove(ds.groupFilePath(account, groupID))
	if os.IsNotExist(err) {
		return fmt.Errorf("group %s not found", groupID)
	}

	return err
}

// SaveTuneInFavorite records a TuneIn station as favorited by creating a marker file.
// File presence indicates the station is a favorite; no content is stored.
func (ds *DataStore) SaveTuneInFavorite(stationID string) error {
	if ds == nil || ds.DataDir == "" || stationID == "" {
		return nil
	}

	dir := ds.safeJoin("tunein", "favorites")
	if err := ds.rootMkdirAll(dir, 0755); err != nil {
		return err
	}

	return ds.rootWriteFile(ds.safeJoin("tunein", "favorites", stationID), nil, 0644)
}

// DeleteTuneInFavorite removes a previously saved TuneIn favorite marker file.
// Returns nil if the station was not favorited.
func (ds *DataStore) DeleteTuneInFavorite(stationID string) error {
	if ds == nil || ds.DataDir == "" || stationID == "" {
		return nil
	}

	err := ds.rootRemove(ds.safeJoin("tunein", "favorites", stationID))
	if os.IsNotExist(err) {
		return nil
	}

	return err
}
