package spotify

import "github.com/gesellix/bose-soundtouch/pkg/service/zeroconf"

// ErrAddUserNoOp re-exports zeroconf.ErrAddUserNoOp so callers in the spotify
// package don't need a direct dependency on the zeroconf package to recognise
// the benign-no-op sentinel.
var ErrAddUserNoOp = zeroconf.ErrAddUserNoOp

// ZeroConfGetInfo fetches the speaker's DH public key via GET ?action=getInfo.
func ZeroConfGetInfo(zcBaseURL string) ([]byte, error) {
	return zeroconf.GetInfo(zcBaseURL)
}

// PushSpotifyCredentials pushes Spotify credentials to a speaker using the full
// ZeroConf DH key exchange protocol. Falls back to simplified token push if
// the speaker does not support DH (older firmware).
// zcBaseURL is the base URL of the ZeroConf endpoint, e.g. "http://192.168.1.10:8200/zc".
func PushSpotifyCredentials(zcBaseURL, username, accessToken string) error {
	return zeroconf.PushCredentials(zcBaseURL, username, accessToken)
}
