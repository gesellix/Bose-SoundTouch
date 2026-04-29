package amazon

import "github.com/gesellix/bose-soundtouch/pkg/service/zeroconf"

// PushAmazonCredentials pushes Amazon Music credentials to a speaker using the
// ZeroConf DH key exchange protocol. Falls back to simplified token push if
// the speaker does not support DH (older firmware).
// zcBaseURL is the base URL of the ZeroConf endpoint, e.g. "http://192.168.1.10:8200/zc".
func PushAmazonCredentials(zcBaseURL, username, accessToken string) error {
	return zeroconf.PushCredentials(zcBaseURL, username, accessToken)
}
