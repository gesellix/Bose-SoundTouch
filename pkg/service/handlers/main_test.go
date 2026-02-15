package handlers

import (
	"net/http"
	"net/url"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/go-chi/chi/v5"
)

func setupRouter(targetURL string, ds *datastore.DataStore) (*chi.Mux, *Server) {
	target, _ := url.Parse(targetURL)
	proxy := &reverseProxy{target: target}
	server := &Server{ds: ds}

	r := chi.NewRouter()
	r.Get("/", server.HandleRoot)

	// Setup media and web directories for tests
	r.Get("/media/*", server.HandleMedia())
	r.Get("/web/*", server.HandleWeb())

	// Setup BMX for tests
	r.Route("/bmx", func(r chi.Router) {
		r.Get("/registry/v1/services", server.HandleBMXRegistry)
		r.Get("/tunein/v1/playback/station/{stationID}", server.HandleTuneInPlayback)
		r.Get("/tunein/v1/playback/episodes/{podcastID}", server.HandleTuneInPodcastInfo)
		r.Get("/tunein/v1/playback/episode/{podcastID}", server.HandleTuneInPlaybackPodcast)
		r.Post("/orion/v1/playback/station/{data}", server.HandleOrionPlayback)
	})

	// Setup Marge for tests
	r.Route("/marge", func(r chi.Router) {
		r.Get("/streaming/sourceproviders", server.HandleMargeSourceProviders)
		r.Get("/accounts/{account}/full", server.HandleMargeAccountFull)
		r.Post("/streaming/support/power_on", server.HandleMargePowerOn)
		r.Get("/updates/soundtouch", server.HandleMargeSoftwareUpdate)
		r.Get("/accounts/{account}/devices/{device}/presets", server.HandleMargePresets)
		r.Post("/accounts/{account}/devices/{device}/presets/{presetNumber}", server.HandleMargeUpdatePreset)
		r.Post("/accounts/{account}/devices/{device}/recents", server.HandleMargeAddRecent)
		r.Post("/accounts/{account}/devices", server.HandleMargeAddDevice)
		r.Delete("/accounts/{account}/devices/{device}", server.HandleMargeRemoveDevice)
		r.Get("/streaming/account/{account}/provider_settings", server.HandleMargeProviderSettings)
		r.Get("/streaming/device/{device}/streaming_token", server.HandleMargeStreamingToken)
		r.Post("/streaming/support/customersupport", server.HandleMargeCustomerSupport)
		r.Get("/streaming/device_setting/account/{account}/device/{device}/device_settings", server.HandleMargeGetDeviceSettings)
		r.Post("/streaming/device_setting/account/{account}/device/{device}/device_settings", server.HandleMargeUpdateDeviceSettings)
		r.Get("/streaming/account/{account}/emailaddress", server.HandleMargeGetEmailAddress)
	})

	// Setup Customer for tests
	r.Route("/customer", func(r chi.Router) {
		r.Get("/account/{account}", server.HandleMargeAccountProfile)
		r.Post("/account/{account}", server.HandleMargeUpdateAccountProfile)
		r.Post("/account/{account}/password", server.HandleMargeChangePassword)
	})

	// Setup Setup for tests
	r.Route("/setup", func(r chi.Router) {
		r.Get("/devices", server.HandleListDiscoveredDevices)
		r.Delete("/devices/{deviceId}", server.HandleRemoveDevice)
		r.Get("/settings", server.HandleGetSettings)
		r.Post("/settings", server.HandleUpdateSettings)
		r.Get("/proxy-settings", server.HandleGetProxySettings)
		r.Post("/proxy-settings", server.HandleUpdateProxySettings)
		r.Post("/ensure-remote-services/{deviceIP}", server.HandleEnsureRemoteServices)
		r.Post("/remove-remote-services/{deviceIP}", server.HandleRemoveRemoteServices)
		r.Post("/migrate/{deviceIP}", server.HandleMigrateDevice)
		r.Post("/revert/{deviceIP}", server.HandleRevertMigration)
		r.Post("/reboot/{deviceIP}", server.HandleRebootDevice)
		r.Post("/trust-ca/{deviceIP}", server.HandleTrustCACert)
		r.Post("/test-connection/{deviceIP}", server.HandleTestConnection)
		r.Post("/test-hosts/{deviceIP}", server.HandleTestHostsRedirection)
		r.Get("/ca.crt", server.HandleGetCACert)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})

	return r, server
}

type reverseProxy struct {
	target *url.URL
}

func (p *reverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Simplified proxy for testing
	w.WriteHeader(http.StatusAccepted) // Custom status to identify proxy hit in tests
	_, _ = w.Write([]byte("Proxied to " + p.target.String()))
}
