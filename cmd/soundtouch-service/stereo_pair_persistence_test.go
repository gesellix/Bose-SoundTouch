package main

import (
	"encoding/xml"
	"errors"
	"fmt"
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

type rejectingRoundTripper struct{}

func (rejectingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
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
		&http.Client{Transport: rejectingRoundTripper{}},
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
		&http.Client{Transport: rejectingRoundTripper{}},
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

func TestEmbeddedStereoPairPersistenceUsesExternalMargeBackend(t *testing.T) {
	active := true
	deleteCalls := 0
	postCalls := 0
	expected := persistenceTestGroup("7654321")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/device/LEFT-ID/group"):
			if !active {
				_, _ = w.Write([]byte(`<group/>`))
				return
			}
			_, _ = fmt.Fprintf(w, `<group id="%s"><name>%s</name><masterDeviceId>%s</masterDeviceId><roles><groupRole><deviceId>LEFT-ID</deviceId><role>LEFT</role><ipAddress>192.0.2.10</ipAddress></groupRole><groupRole><deviceId>RIGHT-ID</deviceId><role>RIGHT</role><ipAddress>192.0.2.11</ipAddress></groupRole></roles></group>`, expected.ID, expected.Name, expected.MasterDeviceID)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/group/"+expected.ID):
			deleteCalls++
			active = false
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/group/"+expected.ID):
			postCalls++
			var update models.Group
			if err := xml.NewDecoder(r.Body).Decode(&update); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			expected = &update
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup, _, rename := embeddedStereoPairGenerationPersistence(
		datastore.NewDataStore(t.TempDir()),
		func() []string { return []string{"http://aftertouch.invalid:18000"} },
		neverDNSHijacked,
		server.Client(),
	)
	if err := rename(stereopair.GenerationRef{
		DeviceID: "LEFT-ID", AccountID: "ACCOUNT1", MargeURL: server.URL + "/marge",
		GroupID: expected.ID, ExpectedGroup: persistenceTestGroup(expected.ID),
	}, "Renamed living room"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if postCalls != 1 || expected.Name != "Renamed living room" {
		t.Fatalf("external POST calls = %d, name = %q; want 1, Renamed living room", postCalls, expected.Name)
	}

	if err := cleanup(stereopair.GenerationRef{
		DeviceID: "LEFT-ID", AccountID: "ACCOUNT1", MargeURL: server.URL + "/marge",
		GroupID: expected.ID, ExpectedGroup: expected,
	}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if deleteCalls != 1 || active {
		t.Fatalf("external DELETE calls = %d, active = %v; want 1, false", deleteCalls, active)
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
	_, preflight, _ := embeddedStereoPairGenerationPersistence(
		ds,
		func() []string {
			providerCalls++
			return []string{currentURL}
		},
		neverDNSHijacked,
		&http.Client{Transport: rejectingRoundTripper{}},
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

	err = preflight([]stereopair.GenerationRef{{
		DeviceID: "LEFT-ID", AccountID: "OLD-ACCOUNT", MargeURL: "http://old.invalid:18000",
	}})
	if err == nil || !strings.Contains(err.Error(), "unexpected HTTP persistence request") {
		t.Fatalf("old URL preflight error = %v, want external HTTP dispatch", err)
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
		&http.Client{Transport: rejectingRoundTripper{}},
	)

	err = preflight([]stereopair.GenerationRef{{
		DeviceID: "LEFT-ID", AccountID: "NEW-ACCOUNT", MargeURL: "https://streaming.bose.com",
	}})
	if err == nil || !strings.Contains(err.Error(), groupID) {
		t.Fatalf("preflight error = %v, want local generation %s found via datastore, not an external HTTP call", err, groupID)
	}
}
