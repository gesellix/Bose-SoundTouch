package health

// CheckIDAdminAreaAuth is the registry id of the admin-area-gate
// availability check. It fires when AdminAreaAuth is unset — the tri-state
// setting that lets an operator opt in now to gating the entire admin area
// (not just /api/mgmt/*) behind Basic Auth, ahead of a future release
// flipping the default. See #419 and
// _/i419/design-admin-area-auth-gate.md.
//
// This is a visibility-only nudge, same spirit as
// CheckIDMgmtDefaultCredentials: it does not gate anything and does not
// change default behavior. It exists so operators who dismissed the
// in-app announcement banner (or never saw it, on an older release) can
// still discover the option via the Health tab.
const CheckIDAdminAreaAuth = "admin_area_auth_available"

// RegisterAdminAreaAuthCheck registers the check. getAdminAreaAuthMode
// returns the live AdminAreaAuth mode (typically Server.AdminAreaAuthMode),
// passed as a callback rather than importing the handlers package directly
// to avoid a circular import (handlers already imports health).
func RegisterAdminAreaAuthCheck(r *Registry, getAdminAreaAuthMode func() string) {
	r.Register(Check{
		ID:    CheckIDAdminAreaAuth,
		Title: "Admin area can require login for the entire admin console, not just Spotify/Amazon linking",
		Run: func() []Finding {
			return runAdminAreaAuthCheck(getAdminAreaAuthMode)
		},
	})
}

func runAdminAreaAuthCheck(getAdminAreaAuthMode func() string) []Finding {
	if getAdminAreaAuthMode() != "" {
		// Already decided (enabled or explicitly disabled) — nothing to nudge.
		return nil
	}

	return []Finding{{
		Severity: SeverityInfo,
		Message: "Only Spotify/Amazon account linking and the Local Account tab currently require login. " +
			"The rest of the admin area (Devices, Settings, Migration, Health, Logs, ...) is open to anyone " +
			"on the network.",
		Details: "Set admin_area_auth to \"enabled\" in Settings to require the Management API login " +
			"(same credentials as MGMT_USERNAME/MGMT_PASSWORD) for the entire admin area. A future release " +
			"is expected to make this the default; you can opt in now, or set it to \"disabled\" to keep " +
			"today's behavior once that happens. See issue #419.",
	}}
}
