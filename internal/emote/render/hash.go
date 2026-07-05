package render

import (
	"crypto/sha256"
	"encoding/hex"
)

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
