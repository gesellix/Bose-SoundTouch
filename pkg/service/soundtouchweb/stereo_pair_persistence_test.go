package soundtouchweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/stereopair"
)

// TestPlayerStereoPairGenerationPersistenceSkipsWritesButAttemptsRead covers
// the standalone player's (and embedded player's default) generation
// lifecycle wiring: cleanup and rename must never push to a Marge backend --
// the speaker itself self-reports its own group teardown/rename to whatever
// backend it's configured with (see HandleMargeDeleteGroup/HandleMargeModifyGroup,
// only ever called by speakers). Preflight still attempts its read-only
// dangling-generation check, but a failure there (network error, wrong
// credentials, real Bose cloud rejecting us, ...) must not block Create.
func TestPlayerStereoPairGenerationPersistenceSkipsWritesButAttemptsRead(t *testing.T) {
	getCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/device/LEFT-ID/group") {
			getCalls++
			http.Error(w, "unauthorized", http.StatusUnauthorized)

			return
		}

		t.Fatalf("unexpected external %s %s: cleanup/rename must not write to a backend the player doesn't own", r.Method, r.URL.Path)
	}))
	defer server.Close()

	cleanup, preflight, rename := playerStereoPairGenerationPersistence(func() *http.Client {
		return server.Client()
	})

	ref := stereopair.GenerationRef{
		DeviceID: "LEFT-ID", AccountID: "ACCOUNT1", MargeURL: server.URL + "/marge",
		GroupID: "7654321",
	}

	if err := rename(ref, "Renamed living room"); err != nil {
		t.Fatalf("rename = %v, want nil (speaker self-reports its own rename)", err)
	}
	if err := cleanup(ref); err != nil {
		t.Fatalf("cleanup = %v, want nil (speaker self-reports its own teardown)", err)
	}
	if err := preflight([]stereopair.GenerationRef{ref}); err != nil {
		t.Fatalf("preflight = %v, want nil: an unauthenticated external check must not block Create", err)
	}
	if getCalls != 1 {
		t.Fatalf("external GET calls = %d, want exactly 1 (preflight must still attempt the read)", getCalls)
	}
}
