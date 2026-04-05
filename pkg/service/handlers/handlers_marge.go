package handlers

import (
	"crypto/rand"
	"encoding/xml"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/marge"
	"github.com/go-chi/chi/v5"
)

// HandleMargeCreateAccount creates a new account from Stockholm (XML).
func (s *Server) HandleMargeCreateAccount(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req models.MargeAccountCreateRequest

	err = xml.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, "Invalid XML body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Use provided ID or generate new 7-digit ID
	var id string

	if req.ID != "" {
		id = req.ID
	} else {
		for {
			n, _ := rand.Int(rand.Reader, big.NewInt(9000000))
			id = strconv.FormatInt(n.Int64()+1000000, 10)

			existing, _ := s.ds.GetAccountInfo(id)
			if existing == nil || existing.IsPlaceholder {
				break
			}
		}
	}

	info := &models.ServiceAccountInfo{
		AccountID:         id,
		PreferredLanguage: req.PreferredLanguage,
	}
	if info.PreferredLanguage == "" {
		info.PreferredLanguage = "en"
	}

	err = s.ds.SaveAccountInfo(id, info)
	if err != nil {
		http.Error(w, "Failed to save account", http.StatusInternalServerError)
		return
	}

	// Stockholm expects the account XML in response
	data, err := marge.AccountFullToXML(s.ds, id)
	if err != nil {
		http.Error(w, "Failed to generate account XML", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(data)
}

// HandleMargeLogin handles account login from Stockholm.
func (s *Server) HandleMargeLogin(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req models.MargeLoginRequest
	if err = xml.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid XML body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Simple mock: find account by email or just return a default one if none exists
	// For now, let's just return a fixed one for testing if nothing else matches
	accounts, err := s.ds.ListAccounts()

	accountID := ""

	if err == nil {
		for _, id := range accounts {
			if id == "default" {
				continue
			}
			// In a real system we'd check email/password
			// Here we just pick the first one or use fallback
			accountID = id

			break
		}
	}

	if accountID == "" {
		http.Error(w, "No accounts found", http.StatusUnauthorized)
		return
	}

	data, err := marge.AccountFullToXML(s.ds, accountID)
	if err != nil {
		http.Error(w, "Failed to generate account XML", http.StatusInternalServerError)
		return
	}

	// Bose returns a token in the Credentials header
	w.Header().Set("Credentials", "mock-token-"+accountID)
	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	_, _ = w.Write(data)
}

// HandleMargeSourceProviders returns the Marge source providers.
func (s *Server) HandleMargeSourceProviders(w http.ResponseWriter, r *http.Request) {
	etag := strconv.FormatInt(time.Now().UnixMilli(), 10)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, err := marge.SourceProvidersToXML()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.Header()["ETag"] = []string{etag}
	_, _ = w.Write(data)
}

// HandleMargeAccountFull returns the full Marge account information.
func (s *Server) HandleMargeAccountFull(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	device := r.URL.Query().Get("device")

	etag := strconv.FormatInt(s.ds.GetETagForAccount(account, device), 10)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, err := marge.AccountFullToXML(s.ds, account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.Header()["ETag"] = []string{etag}
	_, _ = w.Write(data)
}

// HandleMargeAccountSources returns the Marge account sources.
func (s *Server) HandleMargeAccountSources(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	device := r.URL.Query().Get("device")

	etag := strconv.FormatInt(s.ds.GetETagForAccount(account, device), 10)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, err := marge.AccountSourcesToXML(s.ds, account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.1+xml")
	w.Header()["ETag"] = []string{etag}
	_, _ = w.Write(data)
}

// HandleMargeAccountDevices returns the Marge account devices.
func (s *Server) HandleMargeAccountDevices(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	device := r.URL.Query().Get("device")

	etag := strconv.FormatInt(s.ds.GetETagForAccount(account, device), 10)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, err := marge.AccountDevicesToXML(s.ds, account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.1+xml")
	w.Header()["ETag"] = []string{etag}
	_, _ = w.Write(data)
}

// HandleMargePowerOn handles the Marge power on request.
func (s *Server) HandleMargePowerOn(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[Marge] Failed to read power_on body: %v", err)
		w.WriteHeader(http.StatusOK)

		return
	}

	var req models.CustomerSupportRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		log.Printf("[Marge] Failed to parse power_on body: %v", err)

		// Fallback to remote address if body parsing fails
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			go s.PrimeDeviceWithSpotify(host)
		}

		w.WriteHeader(http.StatusOK)

		return
	}

	deviceID := req.Device.ID
	deviceIP := req.DiagnosticData.DeviceLandscape.IPAddress

	log.Printf("[Marge] Device %s powered on (IP: %s)", deviceID, deviceIP)

	// Persist device details provided in the power_on request
	if deviceID != "" && s.ds != nil {
		// Use "default" account if not found or if the device is not yet mapped to an account.
		// In a real scenario, this might be resolved differently if we already have the account info.
		accountID := "default"
		if existing := s.findExistingDeviceInfoByDeviceID(deviceID); existing != nil && existing.AccountID != "" {
			accountID = existing.AccountID
		}

		macAddress := ""
		if len(req.DiagnosticData.DeviceLandscape.MacAddresses) > 0 {
			macAddress = req.DiagnosticData.DeviceLandscape.MacAddresses[0]
		}

		info := &models.ServiceDeviceInfo{
			DeviceID:            deviceID,
			AccountID:           accountID,
			ProductCode:         req.Device.Product.ProductCode,
			DeviceSerialNumber:  req.Device.SerialNumber,
			ProductSerialNumber: req.Device.Product.SerialNumber,
			FirmwareVersion:     req.Device.FirmwareVersion,
			IPAddress:           deviceIP,
			MacAddress:          macAddress,
			DiscoveryMethod:     "power_on",
		}

		if err := s.ds.SaveDeviceInfo(accountID, deviceID, info); err != nil {
			log.Printf("[Marge] Failed to save device info for %s: %v", deviceID, err)
		}
	}

	if deviceIP != "" {
		go s.PrimeDeviceWithSpotify(deviceIP)
	} else {
		// Fallback to remote address if IP is missing from XML
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			go s.PrimeDeviceWithSpotify(host)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// HandleMargeAccountProfile returns the account profile.
func (s *Server) HandleMargeAccountProfile(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "account")

	// Mock profile data
	profile := models.AccountProfileResponse{
		AccountID:    accountID,
		Email:        "user@example.com",
		FirstName:    "SoundTouch",
		LastName:     "User",
		CountryCode:  "US",
		LanguageCode: "en",
	}

	data, err := xml.MarshalIndent(profile, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(data)
}

// HandleMargeUpdateAccountProfile updates the account profile.
func (s *Server) HandleMargeUpdateAccountProfile(w http.ResponseWriter, _ *http.Request) {
	// Stub implementation
	w.WriteHeader(http.StatusOK)
}

// HandleMargeChangePassword changes the account password.
func (s *Server) HandleMargeChangePassword(w http.ResponseWriter, _ *http.Request) {
	// Stub implementation
	w.WriteHeader(http.StatusOK)
}

// HandleMargeGetEmailAddress returns the account email address.
func (s *Server) HandleMargeGetEmailAddress(w http.ResponseWriter, _ *http.Request) {
	resp := models.EmailAddressResponse{
		Email: "user@example.com",
	}

	data, err := xml.MarshalIndent(resp, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(data)
}

// HandleMargeGetDeviceSettings returns device settings.
func (s *Server) HandleMargeGetDeviceSettings(w http.ResponseWriter, _ *http.Request) {
	resp := models.DeviceSettingsResponse{
		Settings: []models.DeviceSetting{
			{Name: "CLOCK_FORMAT", Value: "24HR"},
		},
	}

	data, err := xml.MarshalIndent(resp, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(data)
}

// HandleMargeUpdateDeviceSettings updates device settings.
func (s *Server) HandleMargeUpdateDeviceSettings(w http.ResponseWriter, _ *http.Request) {
	// Stub implementation
	w.WriteHeader(http.StatusOK)
}

// HandleMargeSoftwareUpdate returns the Marge software update information.
func (s *Server) HandleMargeSoftwareUpdate(w http.ResponseWriter, r *http.Request) {
	etag := "default-embedded"
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.Header()["ETag"] = []string{etag}

	// For the account-specific firmware route, always return the software_update tag.
	// This route is specifically used by firmware like Bose_Lisa/27.0.6.
	if chi.URLParam(r, "account") != "" {
		w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")

		xmlData := marge.SoftwareUpdateToXML()
		w.Header().Set("Content-Length", strconv.Itoa(len(xmlData)))
		_, _ = w.Write([]byte(xmlData))

		return
	}

	if len(swUpdateXML) > 0 {
		w.Header().Set("Content-Length", strconv.Itoa(len(swUpdateXML)))
		_, _ = w.Write(swUpdateXML)
	} else {
		xmlData := marge.SoftwareUpdateToXML()
		w.Header().Set("Content-Length", strconv.Itoa(len(xmlData)))
		_, _ = w.Write([]byte(xmlData))
	}
}

// HandleMargePresets returns the Marge presets for a device.
func (s *Server) HandleMargePresets(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	device := chi.URLParam(r, "device")
	if !validatePathID(account) || !validatePathID(device) {
		http.Error(w, "Invalid account or device ID", http.StatusBadRequest)
		return
	}

	etag := strconv.FormatInt(s.ds.GetETagForPresets(account, device), 10)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, err := marge.PresetsToXML(s.ds, account, device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.Header()["ETag"] = []string{etag}
	_, _ = w.Write(data)
}

// HandleMargeUpdatePreset updates a Marge preset.
func (s *Server) HandleMargeUpdatePreset(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	device := chi.URLParam(r, "device")
	if !validatePathID(account) || !validatePathID(device) {
		http.Error(w, "Invalid account or device ID", http.StatusBadRequest)
		return
	}

	etag := strconv.FormatInt(s.ds.GetETagForPresets(account, device), 10)
	w.Header()["ETag"] = []string{etag}

	presetNumberStr := chi.URLParam(r, "presetNumber")

	presetNumber, err := strconv.Atoi(presetNumberStr)
	if err != nil {
		http.Error(w, "Invalid preset number", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}

	data, err := marge.UpdatePreset(s.ds, account, device, presetNumber, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	_, _ = w.Write(data)
}

// HandleMargeRecents returns the Marge recents for a device.
func (s *Server) HandleMargeRecents(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	device := chi.URLParam(r, "device")
	if !validatePathID(account) || !validatePathID(device) {
		http.Error(w, "Invalid account or device ID", http.StatusBadRequest)
		return
	}

	etag := strconv.FormatInt(s.ds.GetETagForRecents(account, device), 10)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	data, err := marge.RecentsToXML(s.ds, account, device)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.Header()["ETag"] = []string{etag}
	_, _ = w.Write(data)
}

// HandleMargeAddRecent adds a recent item to Marge.
func (s *Server) HandleMargeAddRecent(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	device := chi.URLParam(r, "device")
	if !validatePathID(account) || !validatePathID(device) {
		http.Error(w, "Invalid account or device ID", http.StatusBadRequest)
		return
	}

	etag := strconv.FormatInt(s.ds.GetETagForRecents(account, device), 10)
	w.Header()["ETag"] = []string{etag}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}

	data, err := marge.AddRecent(s.ds, account, device, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(data)
}

// HandleMargeAddDevice adds a device to a Marge account.
func (s *Server) HandleMargeAddDevice(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")
	if !validatePathID(account) {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}

	deviceID, data, err := marge.AddDeviceToAccount(s.ds, account, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.Header().Set("Location", s.serverURL+"/account/"+account+"/device/"+deviceID)
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(data)
}

// HandleMargeRemovePreset removes a preset for the specified account and device.
func (s *Server) HandleMargeRemovePreset(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")
	if !validatePathID(account) {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	device := chi.URLParam(r, "device")
	if !validatePathID(device) {
		http.Error(w, "Invalid device ID", http.StatusBadRequest)
		return
	}

	presetStr := chi.URLParam(r, "presetNumber")

	presetNumber, err := strconv.Atoi(presetStr)
	if err != nil || presetNumber < 1 || presetNumber > 6 {
		http.Error(w, "Invalid preset number", http.StatusBadRequest)
		return
	}

	if err := marge.RemovePreset(s.ds, account, device, presetNumber); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleMargeRemoveDevice removes a device from a Marge account.
func (s *Server) HandleMargeRemoveDevice(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")
	if !validatePathID(account) {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	device := chi.URLParam(r, "device")
	if !validatePathID(device) {
		http.Error(w, "Invalid device ID", http.StatusBadRequest)
		return
	}

	if err := marge.RemoveDeviceFromAccount(s.ds, account, device); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok": true}`))
}

// HandleMargeAddSource handles adding a new music source to the account.
// POST /streaming/account/{account}/source
func (s *Server) HandleMargeAddSource(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")
	if !validatePathID(account) {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[Marge] Failed to read body: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)

		return
	}

	resp, err := marge.AddSourceToAccount(s.ds, account, body)
	if err != nil {
		log.Printf("[Marge] Failed to add source: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(resp)
}

// HandleMargeProviderSettings returns Marge provider settings.
func (s *Server) HandleMargeProviderSettings(w http.ResponseWriter, r *http.Request) {
	account := chi.URLParam(r, "account")

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	_, _ = w.Write([]byte(marge.ProviderSettingsToXML(account)))
}

// HandleMargeStreamingToken returns a streaming token for the device.
func (s *Server) HandleMargeStreamingToken(w http.ResponseWriter, _ *http.Request) {
	// Simple mock token for offline use.
	// In a real production environment, this would be a JWT or similar signed token.
	// Some speakers might expect a specific format; we use a distinctive prefix
	// to indicate it's a locally generated token.
	tokenValue := "st-local-token-" + strconv.FormatInt(time.Now().Unix(), 10)
	bearerToken := models.NewBearerToken(tokenValue)

	data, err := xml.Marshal(bearerToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.Header().Set("Authorization", bearerToken.GetAuthHeader())
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(constants.XMLHeader))
	_, _ = w.Write(data)
}

// HandleMargeDeviceGroup returns grouping information for a device (empty group by default).
func (s *Server) HandleMargeDeviceGroup(w http.ResponseWriter, _ *http.Request) {
	// Native firmware expects vnd.bose.streaming content type
	w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(constants.XMLHeader + `<group/>`))
}

// HandleMargeDeviceGroupServer returns grouping server information (404 by default if not a server).
func (s *Server) HandleMargeDeviceGroupServer(w http.ResponseWriter, r *http.Request) {
	// Not in a group as server
	http.NotFound(w, r)
}

// HandleMargeDeviceGroupMember returns grouping member information (404 by default if not a member).
func (s *Server) HandleMargeDeviceGroupMember(w http.ResponseWriter, r *http.Request) {
	// Not in a group as member
	http.NotFound(w, r)
}

// HandleMargeCustomerSupport handles Marge customer support uploads.
func (s *Server) HandleMargeCustomerSupport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var req models.CustomerSupportRequest
	if err = xml.Unmarshal(body, &req); err != nil {
		// Log error but might still return 200 as Bose expects
		log.Printf("Failed to unmarshal CustomerSupportRequest: %v", err)
	}

	// Create a DeviceEvent for support data
	event := models.DeviceEvent{
		Type:     "customer-support-upload",
		Time:     time.Now().Format(time.RFC3339),
		MonoTime: time.Now().UnixNano() / int64(time.Millisecond),
		Data: map[string]interface{}{
			"firmware": req.Device.FirmwareVersion,
			"product":  req.Device.Product.ProductCode,
			"ip":       req.DiagnosticData.DeviceLandscape.IPAddress,
			"rssi":     req.DiagnosticData.DeviceLandscape.RSSI,
		},
	}
	s.ds.AddDeviceEvent(req.Device.ID, event)

	// Update DeviceInfo if possible
	devices, err := s.ds.ListAllDevices()
	if err == nil {
		var account string

		for i := range devices {
			dev := &devices[i]
			if dev.DeviceID == req.Device.ID {
				account = dev.AccountID
				break
			}
		}

		if account != "" {
			info, err := s.ds.GetDeviceInfo(account, req.Device.ID)
			if err == nil && info != nil {
				info.IPAddress = req.DiagnosticData.DeviceLandscape.IPAddress

				info.FirmwareVersion = req.Device.FirmwareVersion
				if len(req.DiagnosticData.DeviceLandscape.MacAddresses) > 0 {
					info.MacAddress = req.DiagnosticData.DeviceLandscape.MacAddresses[0]
				}

				_ = s.ds.SaveDeviceInfo(account, req.Device.ID, info)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}
