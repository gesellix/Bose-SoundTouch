package health

// CheckIDPublicNetworkPlayer is the registry id of the check for an open
// embedded player on a public deployment. The player and its /api/control API
// have no login of their own (issue 663), which is fine on a home network and
// not on the internet.
//
// This is a visibility-only nudge, like CheckIDMgmtDefaultCredentials. Login
// for the player is planned for v1.0; until then the operator can turn the
// player off or protect it at a reverse proxy.
const CheckIDPublicNetworkPlayer = "public_network_player_open"

// RegisterPublicNetworkPlayerCheck registers the check. isOpen reports whether
// this is a --deployment-mode=public-network install with the player enabled.
func RegisterPublicNetworkPlayerCheck(r *Registry, isOpen func() bool) {
	r.Register(Check{
		ID:    CheckIDPublicNetworkPlayer,
		Title: "Embedded player is not exposed without login on a public deployment",
		Run: func() []Finding {
			return runPublicNetworkPlayerCheck(isOpen())
		},
	})
}

func runPublicNetworkPlayerCheck(open bool) []Finding {
	if !open {
		return nil
	}

	return []Finding{{
		Severity: SeverityWarning,
		Message: "This install is set to public-network, and the embedded player at /app and its " +
			"/api/control API are enabled. They have no login, so anyone who can reach this service can use them.",
		Details: "Turn the player off in Settings, or require a login for /app and /api/control at your " +
			"reverse proxy. The speaker-facing routes must stay reachable without a login, so don't protect " +
			"the whole hostname.",
	}}
}
