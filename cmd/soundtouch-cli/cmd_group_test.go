package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/stereopair"
)

func TestGroupClientFactoryUsesMemberHostAndConfiguredPort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getGroup" {
			t.Errorf("path = %q, want /getGroup", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<group id="pair-id"><name>Pair</name></group>`))
	}))
	defer srv.Close()

	host, port := testServerHostPort(t, srv.URL)
	factory := groupClientFactory(&ClientConfig{
		Host:    "192.0.2.200",
		Port:    port,
		Timeout: time.Second,
	})

	memberClient, err := factory(host)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}

	group, err := memberClient.GetGroup()
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}

	if group.ID != "pair-id" || group.Name != "Pair" {
		t.Fatalf("group = %+v, want test server response", group)
	}
}

func TestGroupClientFactoryUsesConfiguredTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`<group/>`))
	}))
	defer srv.Close()

	host, port := testServerHostPort(t, srv.URL)
	factory := groupClientFactory(&ClientConfig{Port: port, Timeout: 5 * time.Millisecond})
	memberClient, err := factory(host)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}

	if _, err := memberClient.GetGroup(); err == nil {
		t.Fatal("GetGroup succeeded, want configured timeout")
	}
}

func TestMargeGroupGenerationURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{base: "http://aftertouch.example:8000", want: "http://aftertouch.example:8000/streaming/account/ACCOUNT1/group/PAIR1"},
		{base: "http://unifi:8001/marge", want: "http://unifi:8001/marge/streaming/account/ACCOUNT1/group/PAIR1"},
		{base: "https://proxy.example/prefix/streaming/", want: "https://proxy.example/prefix/streaming/account/ACCOUNT1/group/PAIR1"},
	}

	for _, test := range tests {
		got, err := stereopair.MargeGroupGenerationURL(stereopair.GenerationRef{
			MargeURL: test.base, AccountID: "ACCOUNT1", GroupID: "PAIR1",
		})
		if err != nil {
			t.Fatalf("margeGroupGenerationURL(%q): %v", test.base, err)
		}
		if got != test.want {
			t.Errorf("margeGroupGenerationURL(%q) = %q, want %q", test.base, got, test.want)
		}
	}
}

func TestDeleteMargeGroupGenerationUsesExactEndpoint(t *testing.T) {
	deleteSeen := false
	getSeen := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			deleteSeen = true
			if r.URL.Path != "/streaming/account/ACCOUNT1/group/PAIR1" {
				t.Errorf("DELETE path = %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			getSeen = true
			if r.URL.Path != "/streaming/account/ACCOUNT1/device/LEFT-ID/group" {
				t.Errorf("GET path = %s", r.URL.Path)
			}
			if deleteSeen {
				_, _ = w.Write([]byte(`<group/>`))
			} else {
				_, _ = w.Write([]byte(`<group id="PAIR1"><masterDeviceId>LEFT-ID</masterDeviceId><roles><groupRole><deviceId>LEFT-ID</deviceId><role>LEFT</role><ipAddress>192.0.2.10</ipAddress></groupRole><groupRole><deviceId>RIGHT-ID</deviceId><role>RIGHT</role><ipAddress>192.0.2.11</ipAddress></groupRole></roles></group>`))
			}
		}
	}))
	defer server.Close()

	err := stereopair.DeleteMargeGroupGeneration(server.Client(), stereopair.GenerationRef{
		MargeURL: server.URL, AccountID: "ACCOUNT1", GroupID: "PAIR1", DeviceID: "LEFT-ID",
		ExpectedGroup: &models.Group{
			ID:             "PAIR1",
			MasterDeviceID: "LEFT-ID",
			Roles: models.GroupRoles{Roles: []models.GroupRole{
				{DeviceID: "LEFT-ID", Role: "LEFT", IPAddress: "192.0.2.10"},
				{DeviceID: "RIGHT-ID", Role: "RIGHT", IPAddress: "192.0.2.11"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("deleteMargeGroupGeneration: %v", err)
	}
	if !deleteSeen || !getSeen {
		t.Fatalf("Marge cleanup requests DELETE=%t GET=%t, want both", deleteSeen, getSeen)
	}
}

func TestPrintGroupResultDetailsReportsMemberFailuresAndCleanup(t *testing.T) {
	result := stereopair.Result{
		Operation:             stereopair.OperationCreate,
		Status:                stereopair.StatusDegraded,
		CompensationAttempted: true,
		PersistenceError:      errors.New("datastore unavailable"),
		Members: []stereopair.MemberResult{
			{
				IPAddress:      "192.0.2.10",
				DeviceID:       "LEFT-ID",
				PreflightError: errors.New("offline"),
			},
			{
				IPAddress:             "192.0.2.11",
				MutationError:         errors.New("add failed"),
				VerificationError:     errors.New("unexpected group"),
				CompensationAttempted: true,
				CompensationError:     errors.New("remove failed"),
			},
		},
	}

	output := captureStdout(t, func() {
		printGroupResultDetails(result)
	})

	for _, expected := range []string{
		"Stereo-pair create result is degraded",
		"192.0.2.10 (LEFT-ID) preflight failed: offline",
		"192.0.2.11 mutation failed: add failed",
		"192.0.2.11 verification failed: unexpected group",
		"192.0.2.11 cleanup failed: remove failed",
		"Partial stereo-pair state cleanup is incomplete",
		"Persistent group generation update failed: datastore unavailable",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("output missing %q:\n%s", expected, output)
		}
	}
}

func TestDissolveRecoveryHostSelectsStillGroupedMember(t *testing.T) {
	result := stereopair.Result{
		Group: &models.Group{ID: "PAIR-ID"},
		Members: []stereopair.MemberResult{
			{IPAddress: "192.0.2.10", Group: &models.Group{}},
			{IPAddress: "192.0.2.11", Group: &models.Group{ID: "PAIR-ID"}},
		},
	}

	if got := dissolveRecoveryHost(result, "192.0.2.10"); got != "192.0.2.11" {
		t.Fatalf("recovery host = %q, want surviving member", got)
	}
}

func testServerHostPort(t *testing.T, serverURL string) (string, int) {
	t.Helper()

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split server host: %v", err)
	}

	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}

	return host, port
}

// TestCLIStereoPairGenerationPersistenceSkipsWritesButAttemptsRead covers
// the CLI's generation-lifecycle wiring: cleanup and rename must never push
// to a Marge backend -- the speaker itself self-reports its own group
// teardown/rename to whatever backend it's configured with (see
// HandleMargeDeleteGroup/HandleMargeModifyGroup, only ever called by
// speakers). Preflight still attempts its read-only dangling-generation
// check, but a failure there (network error, wrong credentials, real Bose
// cloud rejecting us, ...) must not block Create.
func TestCLIStereoPairGenerationPersistenceSkipsWritesButAttemptsRead(t *testing.T) {
	getCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/device/LEFT-ID/group") {
			getCalls++
			http.Error(w, "unauthorized", http.StatusUnauthorized)

			return
		}

		t.Fatalf("unexpected external %s %s: cleanup/rename must not write to a backend the CLI doesn't own", r.Method, r.URL.Path)
	}))
	defer server.Close()

	cleanup, preflight, rename := cliStereoPairGenerationPersistence(server.Client())

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
