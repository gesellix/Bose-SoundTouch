package stockholm

import (
	"crypto/rand"
	"encoding/hex"
)

func randomHexUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}
