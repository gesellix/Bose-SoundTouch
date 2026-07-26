package health

// CheckIDMgmtDefaultCredentials is the registry id of the
// management-credentials check. It fires when the Management API
// (Basic Auth in front of /api/mgmt/*, guarding Spotify/Amazon
// account linking and Local Accounts) is still using the published
// default username/password. Those defaults are documented publicly,
// so anyone who has read the docs can use them.
//
// This is a visibility-only nudge (see issue #419): it does not gate
// anything and does not change default behavior. A future release is
// expected to require an explicit choice (set a password, or opt out)
// before /admin is reachable; until then this just surfaces the gap.
const CheckIDMgmtDefaultCredentials = "mgmt_default_credentials"

// DefaultMgmtUsername and DefaultMgmtPassword are the published
// defaults (cmd/soundtouch-service flags --mgmt-username/--mgmt-password).
// Duplicated here (rather than imported) to keep this package's
// dependency surface small; keep in sync if the CLI defaults change.
const (
	DefaultMgmtUsername = "admin"
	DefaultMgmtPassword = "change_me!"
)

// RegisterMgmtDefaultCredentialsCheck registers the check.
// getMgmtCredentials returns the currently-configured management
// username and password (typically Server's mgmtUsername/mgmtPassword).
func RegisterMgmtDefaultCredentialsCheck(r *Registry, getMgmtCredentials func() (username, password string)) {
	r.Register(Check{
		ID:    CheckIDMgmtDefaultCredentials,
		Title: "Management API credentials have been changed from the published default",
		Run: func() []Finding {
			username, password := getMgmtCredentials()
			return runMgmtDefaultCredentialsCheck(username, password)
		},
	})
}

func runMgmtDefaultCredentialsCheck(username, password string) []Finding {
	if username != DefaultMgmtUsername || password != DefaultMgmtPassword {
		return nil
	}

	return []Finding{{
		Severity: SeverityInfo,
		Message: "The Management API (Spotify/Amazon account linking, Local Accounts) is still using " +
			"the published default credentials (admin / change_me!). Anyone who has read the docs can use them.",
		Details: "Set MGMT_USERNAME and MGMT_PASSWORD (env vars or the matching CLI flags) to your own " +
			"values and restart the service.",
	}}
}
