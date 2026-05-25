package spotify

import "github.com/gesellix/bose-soundtouch/pkg/service/zeroconf"

// ErrAddUserNoOp re-exports zeroconf.ErrAddUserNoOp so callers in the spotify
// package don't need a direct dependency on the zeroconf package to recognise
// the benign-no-op sentinel.
var ErrAddUserNoOp = zeroconf.ErrAddUserNoOp

// ZeroConfGetInfo fetches the speaker's DH public key via GET ?action=getInfo.
// host must be a literal private-network IP address.
// port is the ZeroConf port (typically "8200"); pass "" to omit it from the URL.
func ZeroConfGetInfo(host, port string) ([]byte, error) {
	return zeroconf.GetInfo(host, port)
}

// PushSpotifyCredentials pushes Spotify credentials to a speaker using the full
// ZeroConf DH key exchange protocol. Falls back to simplified token push if
// the speaker does not support DH (older firmware).
// host must be a literal private-network IP address.
// port is the ZeroConf port (typically "8200"); pass "" to omit it from the URL.
func PushSpotifyCredentials(host, port, username, accessToken string) error {
	return zeroconf.PushCredentials(host, port, username, accessToken)
}
