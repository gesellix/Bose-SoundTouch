package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Announcement target values. Mirrors the vocabulary Settings.DefaultLanding
// already uses (see defaultLanding() in handlers_media.go) rather than
// inventing new names — "chooser" is the neutral welcome page, "app" is the
// embedded player, "admin" is this admin console.
const (
	announcementTargetChooser = "chooser"
	announcementTargetApp     = "app"
	announcementTargetAdmin   = "admin"
)

// Announcement is one entry in the small, in-code (not admin-authored)
// announcement list — see #419 design doc,
// _/i419/design-admin-area-auth-gate.md. ShowWhile lets an entry key off
// live server state (e.g. "only while the admin-area gate hasn't been
// decided yet"); nil means always show (until dismissed).
type Announcement struct {
	ID        string
	Message   string
	Level     string
	Targets   []string
	ShowWhile func(*Server) bool
}

// announcements is the full, in-code list. announcementTargetChooser is
// prepared as a valid target value (see the constant above) but no entry
// here uses it yet: the chooser landing page (handlers_media.go, landingHTML)
// is currently fully static with no JS at all, unlike /admin and /app, so it
// can't render or dismiss a banner yet. Wire a chooser-targeted entry only
// once that client-side logic exists.
var announcements = []Announcement{
	{
		ID:      "admin-area-auth-419",
		Level:   "info",
		Targets: []string{announcementTargetAdmin},
		Message: "A future release will require login for this entire admin area by default (today, only " +
			"Spotify/Amazon linking and the Local Account tab do). You can opt in now in Settings, or " +
			"dismiss this once you've decided. See issue #419 for details.",
		ShowWhile: func(s *Server) bool {
			return s.AdminAreaAuthMode() == ""
		},
	},
}

// announcementDTO is the JSON shape returned by HandleListAnnouncements —
// deliberately smaller than Announcement (no ShowWhile func, no Targets;
// the caller already asked for a specific target).
type announcementDTO struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

func containsString(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}

	return false
}

// HandleListAnnouncements returns the announcements currently active for
// the requested target (query param, one of "app" or "admin" — "chooser" is
// a reserved value, not yet wired to any handler), filtered by ShowWhile and
// excluding anything already dismissed. Deliberately NOT behind
// BasicAuthAdmin: the admin-area-gate notice specifically needs to reach
// operators who haven't set up credentials yet, the exact audience an
// admin-only endpoint would exclude.
func (s *Server) HandleListAnnouncements(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")

	switch target {
	case announcementTargetApp, announcementTargetAdmin:
	default:
		http.Error(w, "target must be app or admin", http.StatusBadRequest)
		return
	}

	active := make([]announcementDTO, 0, len(announcements))

	for _, a := range announcements {
		if !containsString(a.Targets, target) {
			continue
		}

		if a.ShowWhile != nil && !a.ShowWhile(s) {
			continue
		}

		if s.IsAnnouncementDismissed(a.ID) {
			continue
		}

		active = append(active, announcementDTO{ID: a.ID, Message: a.Message, Level: a.Level})
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{"announcements": active}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleDismissAnnouncement records a dismissal for the given announcement
// id (see Server.RecordDismissal). The id is validated against the known
// announcements list rather than accepted as arbitrary input — it ends up
// as part of a filename in the local activity log (datastore.RecordActivity),
// and this is the one call site where the id comes from an HTTP request
// rather than a compile-time constant. Also not behind BasicAuthAdmin, for
// the same reason as HandleListAnnouncements.
func (s *Server) HandleDismissAnnouncement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	found := false

	for _, a := range announcements {
		if a.ID == id {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Unknown announcement id", http.StatusNotFound)
		return
	}

	if err := s.RecordDismissal(id); err != nil {
		http.Error(w, "Failed to record dismissal: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
