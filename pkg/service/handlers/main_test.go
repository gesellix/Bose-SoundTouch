package handlers

import (
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/go-chi/chi/v5"
)

func setupRouter(targetURL string, ds *datastore.DataStore) (*chi.Mux, *Server) {
	server := NewServer(ds, nil, targetURL, false, false, false)

	r := chi.NewRouter()
	r.Use(server.OriginMiddleware)
	r.Use(server.ShortcutMiddleware)
	r.Use(server.MirrorMiddleware)
	r.Use(server.RecordMiddleware)

	r.Get("/", server.HandleRoot)

	// Setup media and web directories for tests
	r.Get("/media/*", server.HandleMedia())
	r.Get("/web/*", server.HandleWeb())

	// Setup BMX for tests
	r.Route("/bmx", func(r chi.Router) {
		r.Get("/registry/v1/services", server.HandleBMXRegistry)
		r.Get("/registry/v1/servicesAvailability", server.HandleBMXServicesAvailability)
		r.Get("/tunein/v1/playback/station/{stationID}", server.HandleTuneInPlayback)
		r.Get("/tunein/v1/playback/episodes/{podcastID}", server.HandleTuneInPodcastInfo)
		r.Get("/tunein/v1/playback/episode/{podcastID}", server.HandleTuneInPlaybackPodcast)
		r.Post("/tunein/v1/token", server.HandleTuneInToken)
		r.Post("/tunein/v1/report", server.HandleTuneInReport)
		r.Get("/tunein/v1/navigate", server.HandleTuneInNavigate)
		r.Get("/tunein/v1/navigate/*", server.HandleTuneInNavigate)
		r.Get("/tunein/v1/search", server.HandleTuneInSearch)
		r.Post("/orion/v1/playback/station/{data}", server.HandleOrionPlayback)
	})

	r.Get("/custom/v1/playback/{encodedURL}", server.HandleCustomPlayback)

	streamingRoutes := func(r chi.Router) {
		r.Get("/sourceproviders", server.HandleMargeSourceProviders)
		r.Route("/account/{account}/device", func(r chi.Router) {
			r.Post("/", server.HandleMargeAddDevice)
			r.Post("/{device}", server.HandleMargeAddDevice)
		})
		r.Get("/account/{account}/device/{device}/recent", server.HandleMargeRecents)
		r.Post("/account/{account}/device/{device}/recent", server.HandleMargeAddRecent)
		r.Get("/account/{account}/device/{device}/presets", server.HandleMargePresets)
		r.Post("/account/{account}/device/{device}/presets/{presetNumber}", server.HandleMargeUpdatePreset)
		r.Put("/account/{account}/device/{device}/preset/{presetNumber}", server.HandleMargeUpdatePreset)
		r.Post("/support/power_on", server.HandleMargePowerOn)
		r.Get("/account/{account}/provider_settings", server.HandleMargeProviderSettings)
		r.Get("/device/{device}/streaming_token", server.HandleMargeStreamingToken)
		r.Post("/support/customersupport", server.HandleMargeCustomerSupport)
		r.Get("/device_setting/account/{account}/device/{device}/device_settings", server.HandleMargeGetDeviceSettings)
		// Native group endpoint (both with and without trailing slash)
		r.Get("/account/{account}/device/{device}/group", server.HandleMargeDeviceGroup)
		r.Get("/account/{account}/device/{device}/group/", server.HandleMargeDeviceGroup)
		r.Get("/account/{account}/device/{device}/group/server", server.HandleMargeDeviceGroupServer)
		r.Get("/account/{account}/device/{device}/group/member", server.HandleMargeDeviceGroupMember)
		r.Post("/device_setting/account/{account}/device/{device}/device_settings", server.HandleMargeUpdateDeviceSettings)
		r.Get("/account/{account}/emailaddress", server.HandleMargeGetEmailAddress)
		r.Get("/account/{account}/full", server.HandleMargeAccountFull)
		r.Get("/account/{account}/sources", server.HandleMargeAccountSources)
		r.Get("/account/{account}/devices", server.HandleMargeAccountDevices)
		r.Get("/account/{account}/presets", server.HandleMargeAccountPresets)
		r.Get("/account/{account}/presets/all", server.HandleMargeAccountPresets)
		r.Get("/software/update/account/{account}", server.HandleMargeSoftwareUpdate)
		r.Post("/account", server.HandleMargeCreateAccount)
		r.Post("/account/login", server.HandleMargeLogin)
	}

	accountsRoutes := func(r chi.Router) {
		r.Get("/{account}/full", server.HandleMargeAccountFull)
		r.Get("/{account}/sources", server.HandleMargeAccountSources)
		r.Get("/{account}/devices", server.HandleMargeAccountDevices)
		r.Get("/{account}/devices/{device}/presets", server.HandleMargePresets)
		r.Post("/{account}/devices/{device}/presets/{presetNumber}", server.HandleMargeUpdatePreset)
		r.Put("/{account}/devices/{device}/preset/{presetNumber}", server.HandleMargeUpdatePreset)
		r.Get("/{account}/devices/{device}/recents", server.HandleMargeRecents)
		r.Post("/{account}/devices/{device}/recents", server.HandleMargeAddRecent)
		r.Post("/{account}/devices", server.HandleMargeAddDevice)
		r.Delete("/{account}/devices/{device}", server.HandleMargeRemoveDevice)
		r.Get("/{account}/devices/{device}/group", server.HandleMargeDeviceGroup)
		r.Get("/{account}/devices/{device}/group/", server.HandleMargeDeviceGroup)
		r.Get("/{account}/devices/{device}/group/server", server.HandleMargeDeviceGroupServer)
		r.Get("/{account}/devices/{device}/group/member", server.HandleMargeDeviceGroupMember)
	}

	// Setup Marge for tests
	r.Route("/marge", func(r chi.Router) {
		r.Route("/streaming", streamingRoutes)
		r.Route("/accounts", accountsRoutes)
		r.Get("/updates/soundtouch", server.HandleMargeSoftwareUpdate)
	})

	// Legacy or direct domain calls without /marge prefix
	r.Route("/streaming", streamingRoutes)
	r.Route("/accounts", accountsRoutes)
	r.Get("/updates/soundtouch", server.HandleMargeSoftwareUpdate)

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
		r.Post("/ensure-remote-services/{deviceId}", server.HandleEnsureRemoteServices)
		r.Post("/remove-remote-services/{deviceId}", server.HandleRemoveRemoteServices)
		r.Post("/migrate/{deviceId}", server.HandleMigrateDevice)
		r.Post("/revert/{deviceId}", server.HandleRevertMigration)
		r.Post("/reboot/{deviceId}", server.HandleRebootDevice)
		r.Post("/trust-ca/{deviceId}", server.HandleTrustCACert)
		r.Post("/test-connection/{deviceId}", server.HandleTestConnection)
		r.Post("/test-hosts/{deviceId}", server.HandleTestHostsRedirection)
		r.Get("/ca.crt", server.HandleGetCACert)
	})

	r.NotFound(server.HandleNotFound)

	return r, server
}

func init() {
	// Silence logger for tests
	// log.SetOutput(io.Discard)
}
