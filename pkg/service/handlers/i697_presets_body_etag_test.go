package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/marge"
)

// TestPresetsETagFollowsWhatIsServed is a regression test for issue 697.
// The presets ETag was the modification time of Presets.xml. When only what
// /presets serves changed (here: the Spotify source each preset pulls its
// source block from; on hardware: a fixed build rendering the account), a
// speaker that cached the old list got 304 and kept it, accounts missing.
// The ETag now hashes the body, so the speaker gets the new list; and an
// unchanged list still answers 304, for /recents too.
func TestPresetsETagFollowsWhatIsServed(t *testing.T) {
	const (
		account     = "0000000"
		device      = "DEVICEID01"
		spotifyUser = "spotify-user"
	)

	tempDir := t.TempDir()
	ds := datastore.NewDataStore(tempDir)

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(deviceDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{DeviceID: device, AccountID: account, Name: device}); err != nil {
		t.Fatalf("save device info: %v", err)
	}

	preset := models.ServicePreset{ButtonNumber: "2", ID: "2"}
	preset.Source = constants.ProviderSpotify
	preset.SourceAccount = spotifyUser
	preset.ContentItemType = "tracklisturl"
	preset.Location = "/playback/container/c3BvdGlmeTphbGJ1bTpURVNU"
	preset.Name = "Album"

	if err := ds.SavePresets(account, device, []models.ServicePreset{preset}); err != nil {
		t.Fatalf("save presets: %v", err)
	}

	r, _ := setupRouter("http://localhost:8001", ds)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	base := ts.URL + "/streaming/account/" + account + "/device/" + device

	get := func(path, ifNoneMatch string) (int, string) {
		t.Helper()

		req, _ := http.NewRequest(http.MethodGet, base+path, nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		_ = res.Body.Close()

		return res.StatusCode, res.Header.Get("ETag")
	}

	status, etag := get("/presets", "")
	if status != http.StatusOK || etag == "" {
		t.Fatalf("first /presets: status %d, ETag %q", status, etag)
	}

	if status, _ = get("/presets", etag); status != http.StatusNotModified {
		t.Fatalf("unchanged /presets with its own ETag: status %d, want 304", status)
	}

	presetsFile := filepath.Join(deviceDir, constants.PresetsFile)

	before, err := os.Stat(presetsFile)
	if err != nil {
		t.Fatalf("stat Presets.xml: %v", err)
	}

	// Only Sources.xml changes: the Spotify source with the account appears.
	if _, err = marge.AddSource(ds, account, spotifyUser, strconv.Itoa(constants.SpotifyProviderID), "bs-handle", constants.CredentialTypeTokenV3, ""); err != nil {
		t.Fatalf("add source: %v", err)
	}

	after, err := os.Stat(presetsFile)
	if err != nil {
		t.Fatalf("stat Presets.xml: %v", err)
	}

	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("test premise broken: AddSource rewrote Presets.xml")
	}

	if status, _ = get("/presets", etag); status != http.StatusOK {
		t.Fatalf("/presets after the served source changed: status %d, want 200 (speaker would keep the old list)", status)
	}

	status, recentsETag := get("/recent", "")
	if status != http.StatusOK || recentsETag == "" {
		t.Fatalf("first /recent: status %d, ETag %q", status, recentsETag)
	}

	if status, _ = get("/recent", recentsETag); status != http.StatusNotModified {
		t.Fatalf("unchanged /recent with its own ETag: status %d, want 304", status)
	}
}
