package stockholm

import (
	"crypto/rand"
	"encoding/hex"
)

// kiloDefaultValue is the published default for the Stockholm "kilo"
// constant, carried over from the upstream krahl/soundcork-stockholm-app
// project (BackendApplication.java). Not a secret — this is the exact
// value the Stockholm JS expects to read via getConstant("kilo") when
// nothing else has stored a different one. Seeded into NativeState on
// first run; also returned by the bridge as a fallback if the state
// entry is missing.
const kiloDefaultValue = "a7928d7b43dcd49f0af31e5aeed26458"

func randomHexUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}
