package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
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
