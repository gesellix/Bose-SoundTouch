//go:build !(linux && arm)

package netcompat

import (
	"log"
	"net"
	"sync"
)

// Listen is net.Listen. This build has no accept4 problem to work around, so
// there is deliberately nothing between the caller and the standard library.
func Listen(network, address string) (net.Listener, error) {
	if requestedFallback() == fallbackForceOn {
		noFallbackWarning.Do(func() {
			log.Printf("[netcompat] %s=1 has no effect on this build; the accept4 fallback is compiled in for linux/arm only", FallbackEnv)
		})
	}

	return net.Listen(network, address)
}

var noFallbackWarning sync.Once
