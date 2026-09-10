// Package netcompat provides net.Listen with a fallback for Linux kernels that
// predate the accept4() syscall.
//
// accept4() reached Linux in 2.6.28, but 32-bit ARM did not get it until
// 2.6.36. Go carried an accept4-to-accept fallback, dropped it, restored it for
// linux/arm (golang/go#57333), and dropped it again once the minimum supported
// kernel became 3.2 (golang/go#67001). As of Go 1.27 internal/poll calls
// accept4() unconditionally on every linux build, so on such a kernel a
// listener binds successfully and then fails on its first connection with:
//
//	accept tcp [::]:8001: accept4: function not implemented
//
// Those kernels are outside Go's supported range, so everything here is
// explicitly best-effort. The fallback exists only for linux/arm; every other
// platform gets net.Listen unchanged, byte for byte. See
// https://github.com/gesellix/Bose-SoundTouch/issues/698.
package netcompat

import (
	"log"
	"os"
	"strings"
	"sync"
)

// FallbackEnv names the environment variable that overrides accept4 detection.
//
// "1" forces the raw-accept fallback on, "0" forces it off, and "auto" (or an
// empty value) probes the kernel. The override makes both paths reachable on
// one machine without a rebuild, which matters because the only hardware that
// needs the fallback is hardware we cannot put in CI.
const FallbackEnv = "AFTERTOUCH_ACCEPT_FALLBACK"

// fallbackMode is what FallbackEnv asked for.
type fallbackMode int

const (
	// fallbackAuto probes the kernel and decides. This is the default.
	fallbackAuto fallbackMode = iota
	// fallbackForceOn skips the probe and always uses the raw accept path.
	fallbackForceOn
	// fallbackForceOff skips the probe and never uses the raw accept path.
	fallbackForceOff
)

// requestedFallback reads FallbackEnv. An unrecognised value is reported once
// and treated as "auto" rather than refused: a typo in an env var should not
// stop the service from starting.
func requestedFallback() fallbackMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(FallbackEnv))) {
	case "1", "true", "on", "yes":
		return fallbackForceOn
	case "0", "false", "off", "no":
		return fallbackForceOff
	case "", "auto":
		return fallbackAuto
	default:
		// Once, not per listener: the value cannot change between calls.
		badValueWarning.Do(func() {
			log.Printf("[netcompat] Ignoring unrecognised %s value; using auto detection", FallbackEnv)
		})

		return fallbackAuto
	}
}

var badValueWarning sync.Once
