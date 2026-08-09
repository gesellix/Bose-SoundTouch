package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/updatecheck"
	"github.com/go-chi/chi/v5"
)

func newAnnouncementsTestServer(t *testing.T) *Server {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "announcements-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	return NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)
}

func listAnnouncements(t *testing.T, s *Server, target string) (int, []announcementDTO) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/announcements?target="+target, nil)
	rr := httptest.NewRecorder()

	s.HandleListAnnouncements(rr, req)

	if rr.Code != http.StatusOK {
		return rr.Code, nil
	}

	var body struct {
		Announcements []announcementDTO `json:"announcements"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	return rr.Code, body.Announcements
}

func TestHandleListAnnouncements_InvalidTarget(t *testing.T) {
	s := newAnnouncementsTestServer(t)

	for _, target := range []string{"", "chooser", "bogus"} {
		req := httptest.NewRequest(http.MethodGet, "/api/announcements?target="+target, nil)
		rr := httptest.NewRecorder()

		s.HandleListAnnouncements(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("target=%q: expected 400 (chooser is reserved, not wired yet), got %d", target, rr.Code)
		}
	}
}

// TestHandleListAnnouncements_AdminGateNotice is a regression test for the
// #419 admin-gate announcement's ShowWhile/Targets/dismissal behavior end to
// end: visible for "admin" while AdminAreaAuth is unset, invisible for
// "app", invisible once the mode is set, and invisible once dismissed.
func TestHandleListAnnouncements_AdminGateNotice(t *testing.T) {
	const noticeID = "admin-area-auth-419"

	t.Run("visible for admin target while unset", func(t *testing.T) {
		s := newAnnouncementsTestServer(t)

		status, active := listAnnouncements(t, s, announcementTargetAdmin)
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		if !containsAnnouncementID(active, noticeID) {
			t.Errorf("expected %q to be active for target=admin while unset, got %+v", noticeID, active)
		}
	})

	t.Run("not visible for app target", func(t *testing.T) {
		s := newAnnouncementsTestServer(t)

		_, active := listAnnouncements(t, s, announcementTargetApp)
		if containsAnnouncementID(active, noticeID) {
			t.Errorf("expected %q to NOT be active for target=app (Targets is admin-only), got %+v", noticeID, active)
		}
	})

	t.Run("not visible once AdminAreaAuth is decided", func(t *testing.T) {
		s := newAnnouncementsTestServer(t)
		s.SetAdminAreaAuth("enabled")

		_, active := listAnnouncements(t, s, announcementTargetAdmin)
		if containsAnnouncementID(active, noticeID) {
			t.Errorf("expected %q to disappear once the mode is decided, got %+v", noticeID, active)
		}
	})

	t.Run("not visible once dismissed", func(t *testing.T) {
		s := newAnnouncementsTestServer(t)

		if err := s.RecordDismissal(noticeID); err != nil {
			t.Fatalf("RecordDismissal failed: %v", err)
		}

		_, active := listAnnouncements(t, s, announcementTargetAdmin)
		if containsAnnouncementID(active, noticeID) {
			t.Errorf("expected %q to disappear once dismissed, got %+v", noticeID, active)
		}
	})
}

func containsAnnouncementID(active []announcementDTO, id string) bool {
	for _, a := range active {
		if a.ID == id {
			return true
		}
	}

	return false
}

func TestHandleDismissAnnouncement_UnknownID(t *testing.T) {
	s := newAnnouncementsTestServer(t)

	r := chi.NewRouter()
	r.Post("/api/announcements/{id}/dismiss", s.HandleDismissAnnouncement)

	req := httptest.NewRequest(http.MethodPost, "/api/announcements/not-a-real-id/dismiss", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for an unknown announcement id, got %d", rr.Code)
	}
}

// TestHandleDismissAnnouncement_Success verifies dismissing a known
// announcement both succeeds and is reflected by a subsequent list call —
// end-to-end through the HTTP handlers, not just the underlying
// Server.RecordDismissal/IsAnnouncementDismissed pair.
func TestHandleDismissAnnouncement_Success(t *testing.T) {
	s := newAnnouncementsTestServer(t)

	r := chi.NewRouter()
	r.Post("/api/announcements/{id}/dismiss", s.HandleDismissAnnouncement)

	req := httptest.NewRequest(http.MethodPost, "/api/announcements/admin-area-auth-419/dismiss", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	_, active := listAnnouncements(t, s, announcementTargetAdmin)
	if containsAnnouncementID(active, "admin-area-auth-419") {
		t.Errorf("expected the notice to be gone from the list after dismissal, got %+v", active)
	}
}

// newServerWithUpdateAvailable builds a Server whose registered
// updatecheck.Checker reports a newer version than currentVersion, via the
// same persisted-state-seeding path a real restart would use (not a mock —
// exercises the real NewChecker/UpdateCheckResult round trip).
func newServerWithUpdateAvailable(t *testing.T, currentVersion, latestVersion string) *Server {
	t.Helper()

	s := newAnnouncementsTestServer(t)

	ds := datastore.NewDataStore(t.TempDir())
	if err := ds.SaveUpdateCheckState(datastore.UpdateCheckState{
		LastCheckedAt:   "2026-08-09T00:00:00Z",
		LastSeenVersion: latestVersion,
		LastReleaseURL:  "https://example.invalid/releases/" + latestVersion,
	}); err != nil {
		t.Fatalf("Failed to seed update-check state: %v", err)
	}

	s.SetUpdateChecker(updatecheck.NewChecker(ds, "owner/repo", currentVersion))

	return s
}

// TestHandleListAnnouncements_UpdateAvailable is the regression test for
// #591's reuse of the #419 announcements mechanism: the update-available
// entry's dynamic message/target/dismissal behavior end to end.
func TestHandleListAnnouncements_UpdateAvailable(t *testing.T) {
	t.Run("visible for both admin and app targets when available", func(t *testing.T) {
		s := newServerWithUpdateAvailable(t, "v1.0.0", "v1.2.0")

		for _, target := range []string{announcementTargetAdmin, announcementTargetApp} {
			_, active := listAnnouncements(t, s, target)

			var found *announcementDTO
			for i := range active {
				if active[i].ID == "update-available-v1.2.0" {
					found = &active[i]
				}
			}

			if found == nil {
				t.Fatalf("target=%s: expected an update-available-v1.2.0 entry, got %+v", target, active)
			}
			if found.Message == "" {
				t.Errorf("target=%s: expected a non-empty dynamic message", target)
			}
			// The release URL belongs in the structured link field, not
			// embedded as text in the message — the message must stay
			// generic across other future announcements too.
			if strings.Contains(found.Message, "http") {
				t.Errorf("target=%s: expected the URL out of Message, got %q", target, found.Message)
			}
			if found.LinkURL == "" {
				t.Errorf("target=%s: expected a non-empty LinkURL", target)
			}
			if found.LinkText == "" {
				t.Errorf("target=%s: expected a non-empty LinkText", target)
			}
		}
	})

	t.Run("not visible when already up to date", func(t *testing.T) {
		s := newServerWithUpdateAvailable(t, "v1.2.0", "v1.2.0")

		_, active := listAnnouncements(t, s, announcementTargetAdmin)
		if containsAnnouncementID(active, "update-available-v1.2.0") {
			t.Errorf("expected no update-available entry when up to date, got %+v", active)
		}
	})

	t.Run("dismissing one version does not suppress a later version", func(t *testing.T) {
		s := newServerWithUpdateAvailable(t, "v1.0.0", "v1.2.0")

		if err := s.RecordDismissal("update-available-v1.2.0"); err != nil {
			t.Fatalf("RecordDismissal failed: %v", err)
		}

		_, active := listAnnouncements(t, s, announcementTargetAdmin)
		if containsAnnouncementID(active, "update-available-v1.2.0") {
			t.Error("expected the v1.2.0 notice to be dismissed")
		}

		// A later check finds a newer version still: must reappear under a
		// DIFFERENT dismissal key, not stay suppressed.
		newDS := datastore.NewDataStore(t.TempDir())
		if err := newDS.SaveUpdateCheckState(datastore.UpdateCheckState{
			LastCheckedAt:   "2026-08-10T00:00:00Z",
			LastSeenVersion: "v1.3.0",
		}); err != nil {
			t.Fatalf("Failed to seed newer state: %v", err)
		}
		s.SetUpdateChecker(updatecheck.NewChecker(newDS, "owner/repo", "v1.0.0"))

		_, active = listAnnouncements(t, s, announcementTargetAdmin)
		if !containsAnnouncementID(active, "update-available-v1.3.0") {
			t.Errorf("expected the v1.3.0 notice to appear despite v1.2.0 being dismissed, got %+v", active)
		}
	})
}

func TestHandleDismissAnnouncement_UpdateAvailable(t *testing.T) {
	s := newServerWithUpdateAvailable(t, "v1.0.0", "v1.2.0")

	r := chi.NewRouter()
	r.Post("/api/announcements/{id}/dismiss", s.HandleDismissAnnouncement)

	req := httptest.NewRequest(http.MethodPost, "/api/announcements/update-available-v1.2.0/dismiss", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	_, active := listAnnouncements(t, s, announcementTargetAdmin)
	if containsAnnouncementID(active, "update-available-v1.2.0") {
		t.Errorf("expected the notice to be gone after dismissal, got %+v", active)
	}
}
