package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/stereopair"
)

// neverDNSHijacked is the default DNS-hijack predicate for tests that don't
// exercise the DNS-migrated-speaker path (see
// TestEmbeddedStereoPairPersistenceTreatsDNSHijackedBoseHostAsLocal).
func neverDNSHijacked(string) bool { return false }

// rejectingRoundTripper errors on every request and counts how many it saw.
// A preflight failure is now logged and swallowed rather than propagated
// (see TestEmbeddedStereoPairPersistenceSkipsExternalWritesButAttemptsRead),
// so tests that need to prove an external dispatch actually happened check
// calls rather than the returned error.
type rejectingRoundTripper struct {
	calls int
}

func (r *rejectingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls++

	return nil, errors.New("unexpected HTTP persistence request")
}

func persistenceTestGroup(id string) *models.Group {
	return &models.Group{
		ID:             id,
		Name:           "Living room",
		MasterDeviceID: "LEFT-ID",
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: "LEFT-ID", Role: "LEFT", IPAddress: "192.0.2.10"},
			{DeviceID: "RIGHT-ID", Role: "RIGHT", IPAddress: "192.0.2.11"},
		}},
	}
}

func TestEmbeddedStereoPairPersistenceUsesLocalDatastoreAcrossAccounts(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	group := persistenceTestGroup("")
	groupID, err := ds.AddGroup("OLD-ACCOUNT", group)
	if err != nil {
		t.Fatalf("AddGroup: %v", err)
	}

	localURL := "https://aftertouch.invalid:18443"
	cleanup, preflight, rename := embeddedStereoPairGenerationPersistence(
		ds,
		func() []string { return []string{localURL} },
		neverDNSHijacked,
		&http.Client{Transport: &rejectingRoundTripper{}},
	)

	err = preflight([]stereopair.GenerationRef{{
		DeviceID: "LEFT-ID", AccountID: "NEW-ACCOUNT", MargeURL: localURL,
	}})
	if err == nil || !strings.Contains(err.Error(), groupID) {
		t.Fatalf("preflight error = %v, want cross-account generation %s", err, groupID)
	}
	if err := rename(stereopair.GenerationRef{
		DeviceID: "LEFT-ID", AccountID: "NEW-ACCOUNT", MargeURL: localURL,
		GroupID: groupID, ExpectedGroup: group,
	}, "Renamed living room"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	group.Name = "Renamed living room"

	if err := cleanup(stereopair.GenerationRef{
		DeviceID: "LEFT-ID", AccountID: "NEW-ACCOUNT", MargeURL: localURL,
		GroupID: groupID, ExpectedGroup: group,
	}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if err := preflight([]stereopair.GenerationRef{{
		DeviceID: "LEFT-ID", AccountID: "NEW-ACCOUNT", MargeURL: localURL,
	}}); err != nil {
		t.Fatalf("preflight after exact cleanup: %v", err)
	}
}

func TestEmbeddedStereoPairCleanupMapsAmbiguousGenerationToConflict(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	group := persistenceTestGroup("")
	groupID, err := ds.AddGroup("ACCOUNT1", group)
	if err != nil {
		t.Fatalf("AddGroup: %v", err)
	}
	localURL := "https://aftertouch.invalid:18443"
	cleanup, _, _ := embeddedStereoPairGenerationPersistence(
		ds,
		func() []string { return []string{localURL} },
		neverDNSHijacked,
		&http.Client{Transport: &rejectingRoundTripper{}},
	)
	wrongTopology := persistenceTestGroup(groupID)
	wrongTopology.Roles.Roles[1].DeviceID = "SUBSTITUTE-RIGHT-ID"

	err = cleanup(stereopair.GenerationRef{
		DeviceID: "LEFT-ID", AccountID: "ACCOUNT1", MargeURL: localURL,
		GroupID: groupID, ExpectedGroup: wrongTopology,
	})
	if !errors.Is(err, stereopair.ErrConflict) || errors.Is(err, stereopair.ErrUnavailable) {
		t.Fatalf("cleanup error = %v, want ErrConflict only", err)
	}
}

// TestEmbeddedStereoPairPersistenceSkipsExternalWritesButAttemptsRead covers
// an external (non-local) MargeURL: cleanup and rename must never push to a
// backend we don't own -- the speaker itself self-reports its own group
// teardown/rename to whatever Marge backend it's configured with, which is
// the entire reason HandleMargeDeleteGroup/HandleMargeModifyGroup exist
// (they're only ever called by speakers). Preflight still attempts its
// read-only dangling-generation check, but a failure there (network error,
// wrong credentials, real Bose cloud rejecting us, ...) must not block
// Create, since it's a best-effort check on top of the coordinator's own
// physical preflight, not the primary guard.
func TestEmbeddedStereoPairPersistenceSkipsExternalWritesButAttemptsRead(t *testing.T) {
	getCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/device/LEFT-ID/group") {
			getCalls++
			http.Error(w, "unauthorized", http.StatusUnauthorized)

			return
		}

		t.Fatalf("unexpected external %s %s: cleanup/rename must not write to a backend we don't own", r.Method, r.URL.Path)
	}))
	defer server.Close()

	cleanup, preflight, rename := embeddedStereoPairGenerationPersistence(
		datastore.NewDataStore(t.TempDir()),
		func() []string { return []string{"http://aftertouch.invalid:18000"} },
		neverDNSHijacked,
		server.Client(),
	)

	ref := stereopair.GenerationRef{
		DeviceID: "LEFT-ID", AccountID: "ACCOUNT1", MargeURL: server.URL + "/marge",
		GroupID: "7654321", ExpectedGroup: persistenceTestGroup("7654321"),
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

func TestEmbeddedStereoPairPersistenceReadsOneCurrentURLSnapshot(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	group := persistenceTestGroup("")
	groupID, err := ds.AddGroup("OLD-ACCOUNT", group)
	if err != nil {
		t.Fatalf("AddGroup: %v", err)
	}

	currentURL := "http://old.invalid:18000"
	providerCalls := 0
	transport := &rejectingRoundTripper{}
	_, preflight, _ := embeddedStereoPairGenerationPersistence(
		ds,
		func() []string {
			providerCalls++
			return []string{currentURL}
		},
		neverDNSHijacked,
		&http.Client{Transport: transport},
	)

	currentURL = "http://new.invalid:18000"
	err = preflight([]stereopair.GenerationRef{
		{DeviceID: "LEFT-ID", AccountID: "NEW-ACCOUNT", MargeURL: currentURL},
		{DeviceID: "RIGHT-ID", AccountID: "NEW-ACCOUNT", MargeURL: currentURL},
	})
	if err == nil || !strings.Contains(err.Error(), groupID) {
		t.Fatalf("preflight error = %v, want current local generation %s", err, groupID)
	}
	if providerCalls != 1 {
		t.Fatalf("URL provider calls = %d, want one coherent snapshot", providerCalls)
	}

	// The old URL no longer matches localMargeURLs()'s current snapshot, so
	// this ref is external. Preflight still attempts the read (proven by the
	// transport call count) but no longer propagates its failure -- an
	// external check failing must not block Create.
	err = preflight([]stereopair.GenerationRef{{
		DeviceID: "LEFT-ID", AccountID: "OLD-ACCOUNT", MargeURL: "http://old.invalid:18000",
	}})
	if err != nil {
		t.Fatalf("old URL preflight error = %v, want nil (external check failure must not block)", err)
	}
	if transport.calls != 1 {
		t.Fatalf("external HTTP dispatch calls = %d, want exactly 1", transport.calls)
	}
}

// TestEmbeddedStereoPairPersistenceTreatsDNSHijackedBoseHostAsLocal covers a
// speaker migrated at the DNS level: its own reported MargeURL is still the
// literal Bose cloud hostname (DNS migration never changes it), but this
// service's DNS hijack redirects that hostname to itself on the network.
// Routing it through the external HTTP path instead would reach the real,
// still-live Bose cloud and 401 there, hard-blocking Create for a normal
// DNS-migrated setup. Uses rejectingRoundTripper to prove no HTTP call is
// attempted at all.
func TestEmbeddedStereoPairPersistenceTreatsDNSHijackedBoseHostAsLocal(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	group := persistenceTestGroup("")
	groupID, err := ds.AddGroup("OLD-ACCOUNT", group)
	if err != nil {
		t.Fatalf("AddGroup: %v", err)
	}

	_, preflight, _ := embeddedStereoPairGenerationPersistence(
		ds,
		func() []string { return []string{"https://aftertouch.invalid:18443"} },
		func(margeURL string) bool { return strings.Contains(margeURL, "streaming.bose.com") },
		&http.Client{Transport: &rejectingRoundTripper{}},
	)

	err = preflight([]stereopair.GenerationRef{{
		DeviceID: "LEFT-ID", AccountID: "NEW-ACCOUNT", MargeURL: "https://streaming.bose.com",
	}})
	if err == nil || !strings.Contains(err.Error(), groupID) {
		t.Fatalf("preflight error = %v, want local generation %s found via datastore, not an external HTTP call", err, groupID)
	}
}
