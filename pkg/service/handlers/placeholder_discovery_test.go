package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/marge"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

// TestHandleDiscoveredDevice_SeedsPlaceholdersWithoutSpotify guards the
// "Spotify-agnostic" property of placeholder seeding: a brand-new device
// being discovered must get its SPOTIFY/SpotifyConnectUserName placeholder
// even when no Spotify service is configured and no OAuth account is linked.
// That's the use case where the operator hasn't set anything up yet but the
// user pushes Spotify Connect from their phone and wants to preset it.
func TestHandleDiscoveredDevice_SeedsPlaceholdersWithoutSpotify(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "placeholder-discovery-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	const (
		deviceID  = "AABBCCDDEEFF"
		accountID = "7654321"
	)

	deviceInfoXML := `<info deviceID="` + deviceID + `">
<name>Bare Speaker</name>
<type>SoundTouch 20</type>
<margeAccountUUID>` + accountID + `</margeAccountUUID>
<components>
<component>
<componentCategory>SCM</componentCategory>
<softwareVersion>27.0.6</softwareVersion>
<serialNumber>SN-PLACEHOLDER-TEST</serialNumber>
</component>
</components>
<networkInfo type="SCM">
<macAddress>` + deviceID + `</macAddress>
<ipAddress>127.0.0.1</ipAddress>
</networkInfo>
</info>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, deviceInfoXML)

			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	deviceIP := server.URL[len("http://"):]

	ds := datastore.NewDataStore(tempDir)
	sm := setup.NewManager(server.URL, ds, nil)
	srv := NewServer(ds, sm, "http://localhost", false, false, false)

	// Deliberately NOT calling srv.SetSpotifyService — the whole point is
	// that placeholder seeding must work without any music-service config.
	if srv.spotifyService != nil {
		t.Fatalf("test precondition: spotifyService should be nil for this scenario")
	}

	srv.handleDiscoveredDevice(models.DiscoveredDevice{
		Host:            deviceIP,
		Name:            "Bare Speaker",
		DiscoveryMethod: "UPnP",
	})

	sources, err := ds.GetConfiguredSources(accountID, deviceID)
	if err != nil {
		t.Fatalf("GetConfiguredSources: %v", err)
	}

	found := false
	for i := range sources {
		if sources[i].SourceKey.Type == "SPOTIFY" && sources[i].SourceKey.Account == marge.PlaceholderSpotifyConnectAccount {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected SPOTIFY/%s placeholder after discovery (without Spotify config), got %d sources",
			marge.PlaceholderSpotifyConnectAccount, len(sources))
		for i := range sources {
			t.Logf("  source[%d]: %s/%s (id=%s)", i, sources[i].SourceKey.Type, sources[i].SourceKey.Account, sources[i].ID)
		}
	}
}
