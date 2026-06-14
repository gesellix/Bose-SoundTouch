package marge

import (
	"os"
	"strconv"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestAddSource_MultipleStoredMusicServersCoexist is a regression test for the
// media-server eviction bug: AddSource deduped STORED_MUSIC by provider ID, so
// registering a second DLNA media server overwrote the first. The first then
// vanished from /full + /sources and the speaker dropped it, meaning only one
// media server could ever stay registered.
//
// Each media server is a separate account (username = "<UDN>/0"), so two
// distinct servers must coexist, while re-adding the same server (same account)
// updates in place.
func TestAddSource_MultipleStoredMusicServersCoexist(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "addsource-storedmusic-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	account := "6919733"
	device := "A81B6A536A98"

	if mkErr := os.MkdirAll(ds.AccountDeviceDir(account, device), 0o755); mkErr != nil {
		t.Fatalf("mkdir device dir: %v", mkErr)
	}

	sm := strconv.Itoa(constants.StoredMusicProviderID)

	const (
		fritzAcct = "fa095ecc-e13e-40e7-8e6c-e0286d5bc000/0"
		testAcct  = "4d696e69-444c-164e-9d41-72ecda78e4c1/0"
	)

	// Register two different media servers.
	if _, err := AddSource(ds, account, fritzAcct, sm, "", "", "fritz"); err != nil {
		t.Fatalf("add server 1: %v", err)
	}

	if _, err := AddSource(ds, account, testAcct, sm, "", "", "AfterTouch Test Library"); err != nil {
		t.Fatalf("add server 2: %v", err)
	}

	// storedMusicAccounts returns the set of STORED_MUSIC source accounts the
	// datastore would serve via /full + /sources. SourceKey.Account is the
	// persisted identity (the display name lives on the speaker, set via
	// setMusicServiceAccount, and does not round-trip here).
	storedMusicAccounts := func() map[string]bool {
		sources, gerr := ds.GetConfiguredSources(account, device)
		if gerr != nil {
			t.Fatalf("get sources: %v", gerr)
		}

		out := map[string]bool{}

		for _, s := range sources {
			if s.SourceProviderID == sm {
				out[s.SourceKey.Account] = true
			}
		}

		return out
	}

	got := storedMusicAccounts()
	if len(got) != 2 {
		t.Fatalf("expected 2 STORED_MUSIC sources, got %d: %+v", len(got), got)
	}

	if !got[fritzAcct] {
		t.Errorf("first media server was evicted (account %q missing)", fritzAcct)
	}

	if !got[testAcct] {
		t.Errorf("second media server not registered (account %q missing)", testAcct)
	}

	// Re-adding the SAME server (same account) updates in place; it must not
	// create a duplicate or drop the other server.
	if _, err := AddSource(ds, account, fritzAcct, sm, "", "", "fritz (renamed)"); err != nil {
		t.Fatalf("re-add server 1: %v", err)
	}

	got = storedMusicAccounts()
	if len(got) != 2 || !got[fritzAcct] || !got[testAcct] {
		t.Fatalf("re-adding the same server should keep exactly both accounts; got %+v", got)
	}
}
