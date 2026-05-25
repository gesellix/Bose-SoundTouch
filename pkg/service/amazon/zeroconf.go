package amazon

import "github.com/gesellix/bose-soundtouch/pkg/service/zeroconf"

// ErrAddUserNoOp re-exports zeroconf.ErrAddUserNoOp for callers that don't
// want a direct dependency on the zeroconf package.
var ErrAddUserNoOp = zeroconf.ErrAddUserNoOp

// PushAmazonCredentials pushes Amazon Music credentials to a speaker using the
// ZeroConf DH key exchange protocol. Falls back to simplified token push if
// the speaker does not support DH (older firmware).
// host must be a literal private-network IP address.
// port is the ZeroConf port (typically "8200"); pass "" to omit it from the URL.
func PushAmazonCredentials(host, port, username, accessToken string) error {
	return zeroconf.PushCredentials(host, port, username, accessToken)
}
