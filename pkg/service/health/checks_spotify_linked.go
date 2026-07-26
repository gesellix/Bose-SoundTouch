package health

// CheckIDSpotifyAccountLinked is the registry id of the Spotify
// account-link check. It fires when a Spotify developer app is
// configured (client_id/secret set) but no account has completed the
// OAuth link — the state in which every speaker's Spotify token
// request 503s ("No Spotify accounts linked"), and Spotify presets
// only appear to work right after a manual Spotify Connect session
// (which bypasses AfterTouch's token proxy entirely and doesn't
// survive a reboot).
//
// See issue #269: this state is otherwise invisible outside the
// service log / a diagnostic export, so operators mistake it for a
// per-speaker Spotify limitation instead of a link they never
// finished.
const CheckIDSpotifyAccountLinked = "spotify_account_linked"

// RegisterSpotifyAccountLinkedCheck registers the check.
// getSpotifyConfigured reports whether a Spotify client_id/secret is
// set (typically Server's spotifyService != nil); getLinkedAccountCount
// returns how many Spotify accounts have completed the OAuth link
// (typically len(spotifyService.GetAccounts())).
func RegisterSpotifyAccountLinkedCheck(r *Registry, getSpotifyConfigured func() bool, getLinkedAccountCount func() int) {
	r.Register(Check{
		ID:    CheckIDSpotifyAccountLinked,
		Title: "Spotify account is linked",
		Run: func() []Finding {
			return runSpotifyAccountLinkedCheck(getSpotifyConfigured(), getLinkedAccountCount())
		},
	})
}

func runSpotifyAccountLinkedCheck(configured bool, linkedCount int) []Finding {
	if !configured || linkedCount > 0 {
		return nil
	}

	return []Finding{{
		Severity: SeverityWarning,
		Message: "Spotify is configured (client ID/secret set) but no account is linked yet. " +
			"Every speaker's Spotify token request will fail (503 \"No Spotify accounts linked\"), " +
			"so stored Spotify presets stay dead until you manually Spotify Connect from a phone — " +
			"and that only primes the one speaker you connected to, only until its next reboot.",
		Details: "Finish the link on the Admin page: Settings → Spotify → \"Link Spotify Account\", " +
			"then complete the consent screen. Once linked, AfterTouch re-primes each speaker " +
			"automatically on power-on.",
	}}
}
