package soundtouchweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

const (
	bluetoothPairingPollAttempts = 5
	bluetoothPairingPollInterval = 200 * time.Millisecond
)

var errBluetoothPairingUnverified = errors.New("bluetooth pairing outcome is unverified")
var errSettingsTargetChanged = errors.New("settings target changed")

const settingsTargetHeader = "X-AfterTouch-Settings-Target"

type bluetoothPairingPollStrategy struct {
	Attempts int
	Wait     func() error
}

type bluetoothPairingPollStrategyKey struct{}

func bluetoothPairingStrategy(r *http.Request) bluetoothPairingPollStrategy {
	if strategy, ok := r.Context().Value(bluetoothPairingPollStrategyKey{}).(bluetoothPairingPollStrategy); ok {
		return strategy
	}

	return bluetoothPairingPollStrategy{
		Attempts: bluetoothPairingPollAttempts,
		Wait: func() error {
			timer := time.NewTimer(bluetoothPairingPollInterval)
			defer timer.Stop()

			select {
			case <-r.Context().Done():
				return r.Context().Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func enterBluetoothPairingAndConfirm(
	mutate func() error,
	read func() (*models.NowPlaying, error),
	strategy bluetoothPairingPollStrategy,
) (*models.NowPlaying, error) {
	mutationErr := mutate()
	if mutationErr != nil && !errors.Is(mutationErr, client.ErrMutationOutcomeUnknown) {
		return nil, fmt.Errorf("bluetooth pairing request failed: %w", mutationErr)
	}

	attempts := strategy.Attempts
	if attempts < 1 {
		attempts = 1
	}

	var terminalReadErr error

	for attempt := 0; attempt < attempts; attempt++ {
		nowPlaying, err := read()
		if err != nil {
			terminalReadErr = err
		} else {
			terminalReadErr = nil

			if nowPlaying != nil && nowPlaying.Source == "BLUETOOTH" && nowPlaying.ConnectionStatusInfo.IsDiscoverable() {
				return nowPlaying, nil
			}
		}

		if attempt+1 < attempts && strategy.Wait != nil {
			if err := strategy.Wait(); err != nil {
				return nil, fmt.Errorf("%w: confirmation wait failed: %w", errBluetoothPairingUnverified, err)
			}
		}
	}

	if mutationErr != nil {
		if terminalReadErr != nil {
			return nil, fmt.Errorf("%w: command response was indeterminate (%w) and state readback failed: %w",
				errBluetoothPairingUnverified, mutationErr, terminalReadErr)
		}

		return nil, fmt.Errorf("%w: command response was indeterminate (%w) and discoverability remains unconfirmed",
			errBluetoothPairingUnverified, mutationErr)
	}

	if terminalReadErr != nil {
		return nil, fmt.Errorf("%w: speaker accepted the command but state readback failed: %w",
			errBluetoothPairingUnverified, terminalReadErr)
	}

	return nil, fmt.Errorf("%w: speaker accepted the command but discoverability remains unconfirmed",
		errBluetoothPairingUnverified)
}

type settingsSupport struct {
	ClockDisplay   bool `json:"clockDisplay"`
	ClockTime      bool `json:"clockTime"`
	SystemTimeout  bool `json:"systemTimeout"`
	Language       bool `json:"language"`
	Bluetooth      bool `json:"bluetooth"`
	BluetoothPair  bool `json:"bluetoothPair"`
	BluetoothClear bool `json:"bluetoothClear"`
	Sync           bool `json:"sync"`
	SourceNaming   bool `json:"sourceNaming"`
	WiFiOnboarding bool `json:"wifiOnboarding"`
}

type settingsClockDisplay struct {
	Enabled    *bool  `json:"enabled,omitempty"`
	Format     string `json:"format,omitempty"`
	Brightness int    `json:"brightness,omitempty"`
	TimeZone   string `json:"timeZone,omitempty"`
}

type settingsClockTime struct {
	UTC   int64  `json:"utc,omitempty"`
	Value string `json:"value,omitempty"`
}

type settingsSystemTimeout struct {
	Enabled bool `json:"enabled"`
}

type settingsLanguageOption struct {
	Code int    `json:"code"`
	Name string `json:"name"`
}

type settingsLanguage struct {
	Code    int                      `json:"code"`
	Options []settingsLanguageOption `json:"options"`
}

type settingsSync struct {
	Mode string `json:"mode"`
}

type settingsBluetooth struct {
	MACAddress       string `json:"macAddress,omitempty"`
	ConnectionStatus string `json:"connectionStatus,omitempty"`
	DeviceName       string `json:"deviceName,omitempty"`
}

type settingsSource struct {
	Source        string `json:"source"`
	SourceAccount string `json:"sourceAccount,omitempty"`
	DisplayName   string `json:"displayName"`
}

type deviceSettingsSnapshot struct {
	TargetIdentity string                 `json:"targetIdentity"`
	Support        settingsSupport        `json:"support"`
	ClockDisplay   *settingsClockDisplay  `json:"clockDisplay,omitempty"`
	ClockTime      *settingsClockTime     `json:"clockTime,omitempty"`
	SystemTimeout  *settingsSystemTimeout `json:"systemTimeout,omitempty"`
	Language       *settingsLanguage      `json:"language,omitempty"`
	Sync           *settingsSync          `json:"sync,omitempty"`
	Bluetooth      *settingsBluetooth     `json:"bluetooth,omitempty"`
	Sources        []settingsSource       `json:"sources,omitempty"`
	OnboardingURL  string                 `json:"onboardingUrl,omitempty"`
	Errors         map[string]string      `json:"errors,omitempty"`
}

var systemLanguageCodes = []models.LanguageCode{
	models.LanguageDanish, models.LanguageGerman, models.LanguageEnglish,
	models.LanguageSpanish, models.LanguageFrench, models.LanguageItalian,
	models.LanguageDutch, models.LanguageSwedish, models.LanguageJapanese,
	models.LanguageSimplifiedChinese, models.LanguageTraditionalChinese,
	models.LanguageKorean, models.LanguageThai, models.LanguageCzech,
	models.LanguageFinnish, models.LanguageGreek, models.LanguageNorwegian,
	models.LanguagePolish, models.LanguagePortuguese, models.LanguageRomanian,
	models.LanguageRussian, models.LanguageSlovenian, models.LanguageTurkish,
	models.LanguageHungarian,
}

func supportedSystemLanguageOptions(current models.LanguageCode) []settingsLanguageOption {
	options := make([]settingsLanguageOption, 0, len(systemLanguageCodes)+1)
	currentKnown := false
	languageNames := models.SystemLanguageNames()

	for _, code := range systemLanguageCodes {
		name := strings.TrimSpace(languageNames[code])
		if name == "" {
			name = fmt.Sprintf("Language %d", code)
		}

		options = append(options, settingsLanguageOption{Code: int(code), Name: name})
		currentKnown = currentKnown || code == current
	}

	if !currentKnown {
		options = append(options, settingsLanguageOption{
			Code: int(current),
			Name: fmt.Sprintf("Unknown (%d)", current),
		})
	}

	return options
}

func (app *WebApp) settingsDevice(
	w http.ResponseWriter,
	r *http.Request,
) (*webtypes.DeviceConnection, string, func(), bool) {
	deviceID := chi.URLParam(r, "id")
	if deviceID == "" {
		app.sendError(w, "Device ID required", http.StatusBadRequest)

		return nil, "", nil, false
	}

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)

		return nil, "", nil, false
	}

	targetIdentity := settingsTargetIdentity(device)
	if targetIdentity == "" {
		app.sendError(w, "Device identity not available", http.StatusServiceUnavailable)

		return nil, "", nil, false
	}

	if r.Method != http.MethodGet {
		requestedIdentity := strings.TrimSpace(r.Header.Get(settingsTargetHeader))
		switch {
		case requestedIdentity == "":
			app.sendError(w, "Settings target identity required", http.StatusPreconditionRequired)

			return nil, "", nil, false
		case requestedIdentity != targetIdentity:
			app.sendError(w, "Settings target changed; reload device settings", http.StatusConflict)

			return nil, "", nil, false
		}
	}

	release := app.beginSettingsOperation(targetIdentity)

	current, exists := app.GetDevice(deviceID)
	if !exists {
		release()
		app.sendError(w, "Device not found", http.StatusNotFound)

		return nil, "", nil, false
	}

	if settingsTargetIdentity(current) != targetIdentity {
		release()
		app.sendError(w, "Settings target changed; reload device settings", http.StatusConflict)

		return nil, "", nil, false
	}

	if current.Client == nil {
		release()
		app.sendError(w, "Device client not available", http.StatusServiceUnavailable)

		return nil, "", nil, false
	}

	return current, targetIdentity, release, true
}

func (app *WebApp) beginSettingsOperation(targetIdentity string) func() {
	app.settingsLocksMu.Lock()
	if app.settingsLocks == nil {
		app.settingsLocks = make(map[string]*sync.Mutex)
	}

	lock := app.settingsLocks[targetIdentity]
	if lock == nil {
		lock = &sync.Mutex{}
		app.settingsLocks[targetIdentity] = lock
	}
	app.settingsLocksMu.Unlock()

	lock.Lock()

	return lock.Unlock
}

func (app *WebApp) beginDeviceSettingsOperation(device *webtypes.DeviceConnection) func() {
	targetIdentity := settingsTargetIdentity(device)
	if targetIdentity == "" {
		return func() {}
	}

	return app.beginSettingsOperation(targetIdentity)
}

func settingsTargetIdentity(device *webtypes.DeviceConnection) string {
	if device == nil || device.DeviceInfo == nil {
		return ""
	}

	return strings.TrimSpace(device.DeviceInfo.DeviceID)
}

func liveSettingsTargetError(
	device *webtypes.DeviceConnection,
	targetIdentity string,
) error {
	select {
	case <-device.Done():
		return fmt.Errorf("%w: device is no longer registered", errSettingsTargetChanged)
	default:
	}

	info, err := device.Client.GetDeviceInfo()
	if err != nil {
		return fmt.Errorf("could not verify the settings target: %w", err)
	}

	select {
	case <-device.Done():
		return fmt.Errorf("%w: device is no longer registered", errSettingsTargetChanged)
	default:
	}

	liveIdentity := strings.TrimSpace(info.DeviceID)
	if liveIdentity == "" {
		return errors.New("speaker did not report a settings target identity")
	}

	if liveIdentity != targetIdentity {
		return fmt.Errorf("%w: reload device settings", errSettingsTargetChanged)
	}

	return nil
}

func validateReportedSettingsIdentity(targetIdentity, reportedIdentity string) error {
	reportedIdentity = strings.TrimSpace(reportedIdentity)
	if reportedIdentity != "" && reportedIdentity != targetIdentity {
		return fmt.Errorf("%w: speaker response identity changed", errSettingsTargetChanged)
	}

	return nil
}

func (app *WebApp) revalidateLiveSettingsTarget(
	w http.ResponseWriter,
	device *webtypes.DeviceConnection,
	targetIdentity string,
) bool {
	if err := liveSettingsTargetError(device, targetIdentity); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errSettingsTargetChanged) {
			status = http.StatusConflict
		}

		app.sendError(w, err.Error(), status)

		return false
	}

	return true
}

func hasSource(sources *models.Sources, name string) bool {
	if sources == nil {
		return false
	}

	for _, source := range sources.SourceItem {
		if source.Source == name {
			return true
		}
	}

	return false
}

func settingsSourceList(sources *models.Sources) []settingsSource {
	if sources == nil {
		return nil
	}

	result := make([]settingsSource, 0)

	for _, source := range sources.SourceItem {
		if source.Source != "AUX" {
			continue
		}

		result = append(result, settingsSource{
			Source:        source.Source,
			SourceAccount: source.SourceAccount,
			DisplayName:   source.GetDisplayName(),
		})
	}

	return result
}

func settingsConnectionStatus(nowPlaying *models.NowPlaying) (string, string) {
	if nowPlaying == nil || nowPlaying.ConnectionStatusInfo == nil {
		return "", ""
	}

	return nowPlaying.ConnectionStatusInfo.Status, nowPlaying.ConnectionStatusInfo.DeviceName
}

func readBasicDeviceSettings(
	device *webtypes.DeviceConnection,
	targetIdentity string,
	snapshot *deviceSettingsSnapshot,
) error {
	if snapshot.Support.ClockDisplay {
		value, readErr := device.Client.GetClockDisplay()
		if readErr != nil {
			snapshot.Errors["clockDisplay"] = readErr.Error()
		} else {
			if err := validateReportedSettingsIdentity(targetIdentity, value.DeviceID); err != nil {
				return err
			}

			clockDisplay := &settingsClockDisplay{
				Format:     value.Format,
				Brightness: value.Brightness,
				TimeZone:   value.TimeZone,
			}
			if value.HasEnabled() {
				clockDisplay.Enabled = &value.Enabled
			}

			snapshot.ClockDisplay = clockDisplay
		}
	}

	if snapshot.Support.ClockTime {
		value, readErr := device.Client.GetClockTime()
		if readErr != nil {
			snapshot.Errors["clockTime"] = readErr.Error()
		} else {
			snapshot.ClockTime = &settingsClockTime{UTC: value.GetUTC(), Value: value.GetTimeString()}
		}
	}

	if snapshot.Support.SystemTimeout {
		value, readErr := device.Client.GetSystemTimeout()
		if readErr != nil {
			snapshot.Errors["systemTimeout"] = readErr.Error()
		} else {
			snapshot.SystemTimeout = &settingsSystemTimeout{Enabled: value.PowerSavingEnabled}
		}
	}

	if snapshot.Support.Language {
		value, readErr := device.Client.GetLanguage()
		if readErr != nil {
			snapshot.Errors["language"] = readErr.Error()
		} else {
			snapshot.Language = &settingsLanguage{
				Code:    int(value.Code),
				Options: supportedSystemLanguageOptions(value.Code),
			}
		}
	}

	return nil
}

func readSyncDeviceSetting(device *webtypes.DeviceConnection, snapshot *deviceSettingsSnapshot) {
	if !snapshot.Support.Sync {
		return
	}

	value, readErr := device.Client.GetRebroadcastLatencyMode()
	switch {
	case readErr != nil:
		snapshot.Errors["sync"] = readErr.Error()
	case !value.Controllable:
		snapshot.Support.Sync = false
	default:
		snapshot.Sync = &settingsSync{Mode: string(value.Mode)}
	}
}

func readBluetoothDeviceSetting(
	device *webtypes.DeviceConnection,
	targetIdentity string,
	snapshot *deviceSettingsSnapshot,
) error {
	if !snapshot.Support.Bluetooth {
		return nil
	}

	value, readErr := device.Client.GetBluetoothInfo()
	if readErr != nil {
		snapshot.Errors["bluetooth"] = readErr.Error()
	} else {
		nowPlaying, nowPlayingErr := device.Client.GetNowPlaying()
		if nowPlayingErr != nil {
			snapshot.Errors["bluetooth"] = nowPlayingErr.Error()
		} else if identityErr := validateReportedSettingsIdentity(targetIdentity, nowPlaying.DeviceID); identityErr != nil {
			return identityErr
		}

		status, name := settingsConnectionStatus(nowPlaying)
		snapshot.Bluetooth = &settingsBluetooth{
			MACAddress:       value.BluetoothMACAddress,
			ConnectionStatus: status,
			DeviceName:       name,
		}
	}

	return nil
}

func readDeviceSettingsMetadata(
	device *webtypes.DeviceConnection,
	targetIdentity string,
) (*models.Capabilities, *models.SupportedURLsResponse, error) {
	capabilities, err := device.Client.GetCapabilities()
	if err != nil {
		return nil, nil, fmt.Errorf("read device capabilities: %w", err)
	}

	if validationErr := validateReportedSettingsIdentity(targetIdentity, capabilities.DeviceID); validationErr != nil {
		return nil, nil, validationErr
	}

	supportedURLs, err := device.Client.GetSupportedURLs()
	if err != nil {
		return nil, nil, fmt.Errorf("read supported device endpoints: %w", err)
	}

	if validationErr := validateReportedSettingsIdentity(targetIdentity, supportedURLs.DeviceID); validationErr != nil {
		return nil, nil, validationErr
	}

	return capabilities, supportedURLs, nil
}

func (app *WebApp) readDeviceSettings(
	device *webtypes.DeviceConnection,
	targetIdentity string,
) (*deviceSettingsSnapshot, error) {
	if err := liveSettingsTargetError(device, targetIdentity); err != nil {
		return nil, err
	}

	capabilities, supportedURLs, err := readDeviceSettingsMetadata(device, targetIdentity)
	if err != nil {
		return nil, err
	}

	snapshot := &deviceSettingsSnapshot{
		TargetIdentity: targetIdentity,
		Errors:         make(map[string]string),
	}

	sources, sourcesErr := device.Client.GetSources()
	if sourcesErr != nil {
		snapshot.Errors["sources"] = sourcesErr.Error()
	} else if err := validateReportedSettingsIdentity(targetIdentity, sources.DeviceID); err != nil {
		return nil, err
	}

	hasBluetooth := hasSource(sources, "BLUETOOTH")
	renameableSources := settingsSourceList(sources)
	snapshot.Sources = renameableSources

	snapshot.Support = settingsSupport{
		ClockDisplay:   capabilities.HasClockDisplay() && supportedURLs.HasURL("/clockDisplay"),
		ClockTime:      capabilities.HasClockDisplay() && supportedURLs.HasURL("/clockTime"),
		SystemTimeout:  capabilities.HasCapability("systemtimeout") && supportedURLs.HasURL("/systemtimeout"),
		Language:       supportedURLs.HasURL("/language"),
		Bluetooth:      hasBluetooth && supportedURLs.HasURL("/bluetoothInfo"),
		BluetoothPair:  hasBluetooth && supportedURLs.HasURL("/enterBluetoothPairing"),
		BluetoothClear: hasBluetooth && supportedURLs.HasURL("/clearBluetoothPaired"),
		Sync:           capabilities.HasCapability("rebroadcastlatencymode") && supportedURLs.HasURL("/rebroadcastlatencymode"),
		SourceNaming:   len(renameableSources) > 0 && supportedURLs.HasURL("/nameSource"),
		WiFiOnboarding: capabilities.HasHostedWifiConfig(),
	}

	if err := readBasicDeviceSettings(device, targetIdentity, snapshot); err != nil {
		return nil, err
	}

	readSyncDeviceSetting(device, snapshot)

	if err := readBluetoothDeviceSetting(device, targetIdentity, snapshot); err != nil {
		return nil, err
	}

	if snapshot.Support.WiFiOnboarding && app.OnboardingURL != "" {
		snapshot.OnboardingURL = app.OnboardingURL
	}

	if len(snapshot.Errors) == 0 {
		snapshot.Errors = nil
	}

	if err := liveSettingsTargetError(device, targetIdentity); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func (app *WebApp) writeDeviceSettings(w http.ResponseWriter, snapshot *deviceSettingsSnapshot) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: snapshot}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func writeSettingsMutationUnverified(w http.ResponseWriter, message string, snapshot *deviceSettingsSnapshot) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	if err := json.NewEncoder(w).Encode(struct {
		Success bool                    `json:"success"`
		Outcome string                  `json:"outcome"`
		Data    *deviceSettingsSnapshot `json:"data,omitempty"`
		Error   string                  `json:"error"`
	}{
		Success: false,
		Outcome: "unverified",
		Data:    snapshot,
		Error:   message,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (app *WebApp) finishObservedSettingsMutation(
	w http.ResponseWriter,
	device *webtypes.DeviceConnection,
	targetIdentity string,
	settingName string,
	mutationErr error,
	readbackErr error,
	readbackMatches bool,
) {
	identityErr := liveSettingsTargetError(device, targetIdentity)
	if identityErr == nil && definitiveSettingsMutationError(mutationErr) {
		app.sendError(w, mutationErr.Error(), http.StatusBadGateway)

		return
	}

	if mutationErr == nil && readbackErr == nil && readbackMatches && identityErr == nil {
		app.finishVerifiedSettingsMutation(w, device, targetIdentity)

		return
	}

	message := settingName + " outcome is unverified"

	switch {
	case identityErr != nil:
		message += ": " + identityErr.Error()
	case readbackErr != nil:
		message += ": readback failed: " + readbackErr.Error()
	case !readbackMatches:
		message += ": the speaker did not report the requested value"
	case mutationErr != nil:
		message += ": request failed: " + mutationErr.Error()
	}

	var snapshot *deviceSettingsSnapshot

	if identityErr == nil {
		var refreshErr error

		snapshot, refreshErr = app.readDeviceSettings(device, targetIdentity)
		if refreshErr != nil {
			message += "; settings refresh failed: " + refreshErr.Error()
			snapshot = nil
		}
	}

	writeSettingsMutationUnverified(w, message, snapshot)
}

func definitiveSettingsMutationError(err error) bool {
	if err == nil {
		return false
	}

	var speakerErrors *models.ErrorsResponse
	if errors.As(err, &speakerErrors) {
		return true
	}

	var apiError *models.APIError

	return errors.As(err, &apiError)
}

func writeSettingsMutationConfirmed(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(struct {
		Success bool   `json:"success"`
		Outcome string `json:"outcome"`
		Warning string `json:"warning"`
	}{
		Success: true,
		Outcome: "confirmed",
		Warning: message,
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (app *WebApp) finishVerifiedSettingsMutation(
	w http.ResponseWriter,
	device *webtypes.DeviceConnection,
	targetIdentity string,
) {
	snapshot, err := app.readDeviceSettings(device, targetIdentity)
	if err != nil {
		if errors.Is(err, errSettingsTargetChanged) {
			writeSettingsMutationUnverified(w, err.Error(), nil)

			return
		}

		writeSettingsMutationConfirmed(w,
			"The speaker confirmed the change, but the complete settings view could not be refreshed: "+err.Error())

		return
	}

	app.writeDeviceSettings(w, snapshot)
}

func (app *WebApp) refreshDeviceSettings(
	w http.ResponseWriter,
	device *webtypes.DeviceConnection,
	targetIdentity string,
) {
	snapshot, err := app.readDeviceSettings(device, targetIdentity)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errSettingsTargetChanged) {
			status = http.StatusConflict
		}

		app.sendError(w, err.Error(), status)

		return
	}

	app.writeDeviceSettings(w, snapshot)
}

func settingsReadErrorStatus(err error) int {
	if errors.Is(err, errSettingsTargetChanged) {
		return http.StatusConflict
	}

	return http.StatusBadGateway
}

// HandleGetDeviceSettings returns capability-gated settings with fresh readback.
func (app *WebApp) HandleGetDeviceSettings(w http.ResponseWriter, r *http.Request) {
	device, targetIdentity, release, ok := app.settingsDevice(w, r)
	if !ok {
		return
	}
	defer release()

	app.refreshDeviceSettings(w, device, targetIdentity)
}

func decodeSettingsRequest(r *http.Request, target interface{}) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request must contain one JSON object")
		}

		return err
	}

	return nil
}

func (app *WebApp) requireSetting(
	w http.ResponseWriter,
	device *webtypes.DeviceConnection,
	targetIdentity string,
	predicate func(settingsSupport) bool,
	name string,
) (*deviceSettingsSnapshot, bool) {
	snapshot, err := app.readDeviceSettings(device, targetIdentity)
	if err != nil {
		app.sendError(w, err.Error(), settingsReadErrorStatus(err))

		return nil, false
	}

	if !predicate(snapshot.Support) {
		app.sendError(w, name+" is not supported by this device", http.StatusConflict)

		return nil, false
	}

	return snapshot, true
}

func (app *WebApp) requireSettingReadback(
	w http.ResponseWriter,
	snapshot *deviceSettingsSnapshot,
	key string,
	name string,
	available bool,
) bool {
	if available {
		return true
	}

	message := name + " readback is unavailable"
	if readErr := snapshot.Errors[key]; readErr != "" {
		message += ": " + readErr
	}

	app.sendError(w, message, http.StatusBadGateway)

	return false
}

type clockDisplaySettingsRequest struct {
	Enabled  *bool  `json:"enabled"`
	Format   string `json:"format"`
	TimeZone string `json:"timeZone"`
}

func (app *WebApp) requireClockDisplayReadback(
	w http.ResponseWriter,
	device *webtypes.DeviceConnection,
	targetIdentity string,
	request clockDisplaySettingsRequest,
) bool {
	snapshot, err := app.readDeviceSettings(device, targetIdentity)
	if err != nil {
		app.sendError(w, err.Error(), settingsReadErrorStatus(err))

		return false
	}

	if !snapshot.Support.ClockDisplay {
		app.sendError(w, "Clock display is not supported by this device", http.StatusConflict)

		return false
	}

	if snapshot.ClockDisplay == nil {
		message := "Clock display readback is unavailable"
		if readErr := snapshot.Errors["clockDisplay"]; readErr != "" {
			message += ": " + readErr
		}

		app.sendError(w, message, http.StatusBadGateway)

		return false
	}

	if request.Enabled != nil && snapshot.ClockDisplay.Enabled == nil {
		app.sendError(w, "Clock display enable state is not reported as supported by this device", http.StatusConflict)

		return false
	}

	if request.Format != "" && snapshot.ClockDisplay.Format == "" {
		app.sendError(w, "Time format is not reported as supported by this device", http.StatusConflict)

		return false
	}

	if request.TimeZone != "" && strings.TrimSpace(snapshot.ClockDisplay.TimeZone) == "" {
		app.sendError(w, "Timezone is not reported as supported by this device", http.StatusConflict)

		return false
	}

	return true
}

// HandleSetClockDisplay updates supported display fields and verifies them.
func (app *WebApp) HandleSetClockDisplay(w http.ResponseWriter, r *http.Request) {
	device, targetIdentity, release, ok := app.settingsDevice(w, r)
	if !ok {
		return
	}
	defer release()

	var body clockDisplaySettingsRequest
	if err := decodeSettingsRequest(r, &body); err != nil {
		app.sendError(w, "Invalid clock display request: "+err.Error(), http.StatusBadRequest)

		return
	}

	request := models.NewClockDisplayRequest()
	if body.Enabled != nil {
		request.SetEnabled(*body.Enabled)
	}

	if body.Format != "" {
		switch models.ClockFormat(body.Format) {
		case models.ClockFormat12Hour, models.ClockFormat24Hour, models.ClockFormatAuto:
		default:
			app.sendError(w, "Clock format must be one of 12, 24, or auto", http.StatusBadRequest)

			return
		}

		request.SetFormat(models.ClockFormat(body.Format))
	}

	if body.TimeZone != "" {
		request.SetTimeZone(strings.TrimSpace(body.TimeZone))
	}

	if !request.HasChanges() {
		app.sendError(w, "No clock display changes specified", http.StatusBadRequest)

		return
	}

	if !app.requireClockDisplayReadback(w, device, targetIdentity, body) {
		return
	}

	if !app.revalidateLiveSettingsTarget(w, device, targetIdentity) {
		return
	}

	mutationErr := device.Client.SetClockDisplay(request)

	readback, readbackErr := device.Client.GetClockDisplay()
	if readbackErr == nil && readback != nil {
		readbackErr = validateReportedSettingsIdentity(targetIdentity, readback.DeviceID)
	}

	app.finishObservedSettingsMutation(w, device, targetIdentity, "Clock display",
		mutationErr, readbackErr, clockDisplayMatches(readback, body))
}

func clockDisplayMatches(value *models.ClockDisplay, request clockDisplaySettingsRequest) bool {
	if value == nil {
		return false
	}

	if request.Enabled != nil && (!value.HasEnabled() || value.Enabled != *request.Enabled) {
		return false
	}

	if request.Format != "" && value.Format != request.Format {
		return false
	}

	return request.TimeZone == "" || value.TimeZone == strings.TrimSpace(request.TimeZone)
}

// HandleSetClockTime synchronizes the speaker clock and verifies its timestamp.
func (app *WebApp) HandleSetClockTime(w http.ResponseWriter, r *http.Request) {
	device, targetIdentity, release, ok := app.settingsDevice(w, r)
	if !ok {
		return
	}
	defer release()

	snapshot, err := app.readDeviceSettings(device, targetIdentity)
	if err != nil {
		app.sendError(w, err.Error(), settingsReadErrorStatus(err))

		return
	}

	if !snapshot.Support.ClockTime {
		app.sendError(w, "Clock synchronization is not supported by this device", http.StatusConflict)

		return
	}

	if snapshot.ClockTime == nil {
		message := "Clock time readback is unavailable"
		if readErr := snapshot.Errors["clockTime"]; readErr != "" {
			message += ": " + readErr
		}

		app.sendError(w, message, http.StatusBadGateway)

		return
	}

	if snapshot.ClockTime.UTC == 0 && strings.TrimSpace(snapshot.ClockTime.Value) == "" {
		app.sendError(w, "Current time is not reported as supported by this device", http.StatusConflict)

		return
	}

	started := time.Now()

	if !app.revalidateLiveSettingsTarget(w, device, targetIdentity) {
		return
	}

	mutationErr := device.Client.SetClockTimeNow()
	readback, readbackErr := device.Client.GetClockTime()
	readbackMatches := readbackErr == nil && readback != nil && readback.GetUTC() != 0 &&
		math.Abs(float64(readback.GetUTC()-started.Unix())) <= 60
	app.finishObservedSettingsMutation(w, device, targetIdentity, "Clock synchronization",
		mutationErr, readbackErr, readbackMatches)
}

type toggleSettingsRequest struct {
	Enabled *bool `json:"enabled"`
}

// HandleSetSystemTimeout updates automatic standby and verifies readback.
func (app *WebApp) HandleSetSystemTimeout(w http.ResponseWriter, r *http.Request) {
	device, targetIdentity, release, ok := app.settingsDevice(w, r)
	if !ok {
		return
	}
	defer release()

	var body toggleSettingsRequest
	if err := decodeSettingsRequest(r, &body); err != nil || body.Enabled == nil {
		app.sendError(w, "Automatic standby requires an enabled boolean", http.StatusBadRequest)

		return
	}

	snapshot, supported := app.requireSetting(
		w, device, targetIdentity, func(s settingsSupport) bool { return s.SystemTimeout }, "Automatic standby")
	if !supported {
		return
	}

	if !app.requireSettingReadback(w, snapshot, "systemTimeout", "Automatic standby", snapshot.SystemTimeout != nil) {
		return
	}

	if !app.revalidateLiveSettingsTarget(w, device, targetIdentity) {
		return
	}

	mutationErr := device.Client.SetSystemTimeout(&models.SystemTimeout{PowerSavingEnabled: *body.Enabled})
	readback, readbackErr := device.Client.GetSystemTimeout()
	readbackMatches := readbackErr == nil && readback != nil && readback.PowerSavingEnabled == *body.Enabled
	app.finishObservedSettingsMutation(w, device, targetIdentity, "Automatic standby",
		mutationErr, readbackErr, readbackMatches)
}

type languageSettingsRequest struct {
	Code int `json:"code"`
}

// HandleSetSystemLanguage updates a validated firmware language code.
func (app *WebApp) HandleSetSystemLanguage(w http.ResponseWriter, r *http.Request) {
	device, targetIdentity, release, ok := app.settingsDevice(w, r)
	if !ok {
		return
	}
	defer release()

	var body languageSettingsRequest
	if err := decodeSettingsRequest(r, &body); err != nil {
		app.sendError(w, "Invalid language request: "+err.Error(), http.StatusBadRequest)

		return
	}

	if err := models.LanguageCode(body.Code).Validate(); err != nil {
		app.sendError(w, err.Error(), http.StatusBadRequest)

		return
	}

	snapshot, supported := app.requireSetting(
		w, device, targetIdentity, func(s settingsSupport) bool { return s.Language }, "Language control")
	if !supported {
		return
	}

	if !app.requireSettingReadback(w, snapshot, "language", "Language control", snapshot.Language != nil) {
		return
	}

	if !app.revalidateLiveSettingsTarget(w, device, targetIdentity) {
		return
	}

	mutationErr := device.Client.SetLanguage(models.LanguageCode(body.Code))
	readback, readbackErr := device.Client.GetLanguage()
	readbackMatches := readbackErr == nil && readback != nil && int(readback.Code) == body.Code
	app.finishObservedSettingsMutation(w, device, targetIdentity, "Language change",
		mutationErr, readbackErr, readbackMatches)
}

type syncSettingsRequest struct {
	Mode string `json:"mode"`
}

// HandleSetRebroadcastLatencyMode updates and verifies the sync priority.
func (app *WebApp) HandleSetRebroadcastLatencyMode(w http.ResponseWriter, r *http.Request) {
	device, targetIdentity, release, ok := app.settingsDevice(w, r)
	if !ok {
		return
	}
	defer release()

	var body syncSettingsRequest
	if err := decodeSettingsRequest(r, &body); err != nil {
		app.sendError(w, "Invalid sync request: "+err.Error(), http.StatusBadRequest)

		return
	}

	if err := models.RebroadcastLatencyModeValue(body.Mode).Validate(); err != nil {
		app.sendError(w, err.Error(), http.StatusBadRequest)

		return
	}

	snapshot, supported := app.requireSetting(
		w, device, targetIdentity, func(s settingsSupport) bool { return s.Sync }, "Sync priority")
	if !supported {
		return
	}

	if !app.requireSettingReadback(w, snapshot, "sync", "Sync priority", snapshot.Sync != nil) {
		return
	}

	if !app.revalidateLiveSettingsTarget(w, device, targetIdentity) {
		return
	}

	mutationErr := device.Client.SetRebroadcastLatencyMode(models.RebroadcastLatencyModeValue(body.Mode))
	readback, readbackErr := device.Client.GetRebroadcastLatencyMode()
	readbackMatches := readbackErr == nil && readback != nil && string(readback.Mode) == body.Mode
	app.finishObservedSettingsMutation(w, device, targetIdentity, "Sync-priority change",
		mutationErr, readbackErr, readbackMatches)
}

// HandleEnterBluetoothPairing requests and verifies discoverable mode.
func (app *WebApp) HandleEnterBluetoothPairing(w http.ResponseWriter, r *http.Request) {
	device, targetIdentity, release, ok := app.settingsDevice(w, r)
	if !ok {
		return
	}
	defer release()

	if _, supported := app.requireSetting(
		w, device, targetIdentity, func(s settingsSupport) bool { return s.BluetoothPair }, "Bluetooth pairing"); !supported {
		return
	}

	if !app.revalidateLiveSettingsTarget(w, device, targetIdentity) {
		return
	}

	nowPlaying, err := enterBluetoothPairingAndConfirm(
		device.Client.EnterBluetoothPairing,
		device.Client.GetNowPlaying,
		bluetoothPairingStrategy(r),
	)
	if errors.Is(err, errBluetoothPairingUnverified) {
		if identityErr := liveSettingsTargetError(device, targetIdentity); identityErr != nil {
			err = fmt.Errorf("%w; post-mutation identity check failed: %w", err, identityErr)
		}

		writeSettingsMutationUnverified(w, err.Error(), nil)

		return
	}

	if err != nil {
		if identityErr := liveSettingsTargetError(device, targetIdentity); identityErr != nil {
			writeSettingsMutationUnverified(w,
				fmt.Sprintf("%v; post-mutation identity check failed: %v", err, identityErr), nil)

			return
		}

		app.sendError(w, err.Error(), http.StatusBadGateway)

		return
	}

	if nowPlaying == nil {
		writeSettingsMutationUnverified(w, "Bluetooth pairing outcome is unverified: state readback was empty", nil)

		return
	}

	if identityErr := validateReportedSettingsIdentity(targetIdentity, nowPlaying.DeviceID); identityErr != nil {
		writeSettingsMutationUnverified(w, identityErr.Error(), nil)

		return
	}

	app.finishVerifiedSettingsMutation(w, device, targetIdentity)
}

// HandleClearBluetoothPairings clears pairings without claiming unobservable success.
func (app *WebApp) HandleClearBluetoothPairings(w http.ResponseWriter, r *http.Request) {
	device, targetIdentity, release, ok := app.settingsDevice(w, r)
	if !ok {
		return
	}
	defer release()

	if r.URL.Query().Get("confirmed") != "true" {
		app.sendError(w, "Bluetooth paired-device clearing requires confirmed=true", http.StatusConflict)

		return
	}

	if _, supported := app.requireSetting(
		w, device, targetIdentity, func(s settingsSupport) bool { return s.BluetoothClear }, "Bluetooth paired-device clearing"); !supported {
		return
	}

	if !app.revalidateLiveSettingsTarget(w, device, targetIdentity) {
		return
	}

	mutationErr := device.Client.ClearBluetoothPaired()
	if identityErr := liveSettingsTargetError(device, targetIdentity); identityErr != nil {
		writeSettingsMutationUnverified(w,
			fmt.Sprintf("Bluetooth clear outcome is unverified: post-mutation identity check failed: %v", identityErr), nil)

		return
	}

	if errors.Is(mutationErr, client.ErrMutationOutcomeUnknown) {
		writeSettingsMutationUnverified(w,
			"Bluetooth clear command outcome is unknown and the speaker exposes no paired-list readback: "+
				mutationErr.Error(), nil)

		return
	} else if mutationErr != nil {
		app.sendError(w, mutationErr.Error(), http.StatusBadGateway)

		return
	}

	snapshot, err := app.readDeviceSettings(device, targetIdentity)
	if err != nil {
		writeSettingsMutationUnverified(w,
			"speaker accepted the Bluetooth clear command but settings refresh failed: "+err.Error(), nil)

		return
	}

	if snapshot.Errors == nil {
		snapshot.Errors = make(map[string]string)
	}

	snapshot.Errors["bluetoothClear"] = "The speaker accepted the clear command but exposes no paired-list readback."

	writeSettingsMutationUnverified(w, snapshot.Errors["bluetoothClear"], snapshot)
}

type sourceNameSettingsRequest struct {
	Source        string `json:"source"`
	SourceAccount string `json:"sourceAccount"`
	Name          string `json:"name"`
}

// HandleSetSourceName renames a supported source and verifies the new name.
func (app *WebApp) HandleSetSourceName(w http.ResponseWriter, r *http.Request) {
	device, targetIdentity, release, ok := app.settingsDevice(w, r)
	if !ok {
		return
	}
	defer release()

	var body sourceNameSettingsRequest
	if err := decodeSettingsRequest(r, &body); err != nil {
		app.sendError(w, "Invalid source-name request: "+err.Error(), http.StatusBadRequest)

		return
	}

	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		app.sendError(w, "Source name must not be empty", http.StatusBadRequest)

		return
	}

	if _, supported := app.requireSetting(
		w, device, targetIdentity, func(s settingsSupport) bool { return s.SourceNaming }, "Source naming"); !supported {
		return
	}

	sources, err := device.Client.GetSources()
	if err == nil && sources != nil {
		err = validateReportedSettingsIdentity(targetIdentity, sources.DeviceID)
	}

	if err != nil {
		app.sendError(w, err.Error(), settingsReadErrorStatus(err))

		return
	}

	if !sourceIdentityExists(sources, body.Source, body.SourceAccount) {
		app.sendError(w, "The requested source cannot be renamed on this device", http.StatusConflict)

		return
	}

	if !app.revalidateLiveSettingsTarget(w, device, targetIdentity) {
		return
	}

	renameErr := device.Client.RenameSource(body.Source, body.SourceAccount, body.Name)

	sources, readbackErr := device.Client.GetSources()
	if readbackErr == nil && sources != nil {
		readbackErr = validateReportedSettingsIdentity(targetIdentity, sources.DeviceID)
	}

	app.finishObservedSettingsMutation(w, device, targetIdentity, "Source-name change",
		renameErr, readbackErr, sourceNameMatches(sources, body))
}

func sourceIdentityExists(sources *models.Sources, sourceName, sourceAccount string) bool {
	if sources == nil || sourceName != "AUX" {
		return false
	}

	for _, source := range sources.SourceItem {
		if source.Source == sourceName && source.SourceAccount == sourceAccount {
			return true
		}
	}

	return false
}

func sourceNameMatches(sources *models.Sources, request sourceNameSettingsRequest) bool {
	if sources == nil {
		return false
	}

	for _, source := range sources.SourceItem {
		if source.Source == request.Source && source.SourceAccount == request.SourceAccount {
			return source.DisplayName == request.Name
		}
	}

	return false
}
