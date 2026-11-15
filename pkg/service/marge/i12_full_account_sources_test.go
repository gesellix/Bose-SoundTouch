package marge

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestFullListsTheSourcesOfEveryDevice: /full took the
// account's <sources> from the alphabetically last device only, so a device
// with fewer sources sorting last shrank what every speaker of the account
// was told. The list now holds every source any device has, once, without
// account-less placeholders.
func TestFullListsTheSourcesOfEveryDevice(t *testing.T) {
	const account = "0000000"

	ds := datastore.NewDataStore(t.TempDir())

	write := func(device, sources string) {
		t.Helper()

		dir := ds.AccountDeviceDir(account, device)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		info := `<info deviceID="` + device + `"><name>` + device + `</name><type>SoundTouch 10</type></info>`
		if err := os.WriteFile(filepath.Join(dir, "DeviceInfo.xml"), []byte(info), 0o644); err != nil {
			t.Fatalf("write DeviceInfo.xml: %v", err)
		}

		if err := os.WriteFile(filepath.Join(dir, "Sources.xml"), []byte("<sources>"+sources+"</sources>"), 0o644); err != nil {
			t.Fatalf("write Sources.xml: %v", err)
		}
	}

	const (
		mediaServer = `<source id="SRC_media" type="Audio" displayName="Library" secret="" secretType="token" sourceproviderid="7"><sourceKey type="STORED_MUSIC" account="00000000-0000-0000-0000-0000000000aa/0"/></source>`
		spotify     = `<source id="SRC_spotify" type="Audio" secret="bs-handle" secretType="token_version_3" sourceproviderid="15"><sourceKey type="SPOTIFY" account="spotify-user"/></source>`
		tunein      = `<source id="10004" type="Audio" secret="eyJzZXJpYWwiOiJ0dW5laW4ifQ==" secretType="token" sourceproviderid="25"><sourceKey type="TUNEIN" account=""/></source>`
		amazonEmpty = `<source id="10006" type="Audio" displayName="Amazon Music" secret="" secretType="token" sourceproviderid="20"><sourceKey type="AMAZON" account=""/></source>`
	)

	// Sorts first: the media server, Spotify, and an account-less Amazon entry.
	write("DEVICEID01", mediaServer+spotify+amazonEmpty+tunein)
	// Sorts last: radio only, plus the same Spotify account again.
	write("DEVICEID02", tunein+spotify)

	full, err := AccountFullToXMLReadOnly(ds, account)
	if err != nil {
		t.Fatalf("/full: %v", err)
	}

	top := regexp.MustCompile(`(?s)</devices>.*?<sources>(.*)</sources>`).FindSubmatch(full)
	if top == nil {
		t.Fatalf("no account-level <sources> in /full:\n%s", full)
	}

	count := func(providerID, username string) int {
		n := 0

		for _, block := range strings.Split(string(top[1]), "<source ")[1:] {
			if strings.Contains(block, "<sourceproviderid>"+providerID+"</sourceproviderid>") &&
				strings.Contains(block, "<username>"+username+"</username>") {
				n++
			}
		}

		return n
	}

	if n := count("7", "00000000-0000-0000-0000-0000000000aa/0"); n != 1 {
		t.Errorf("media server listed %d times, want 1:\n%s", n, top[1])
	}

	if n := count("15", "spotify-user"); n != 1 {
		t.Errorf("Spotify account listed %d times, want 1 (it is on both devices):\n%s", n, top[1])
	}

	if regexp.MustCompile(`<sourceproviderid>20</sourceproviderid>`).Match(top[1]) {
		t.Errorf("account-less Amazon placeholder listed:\n%s", top[1])
	}
}
