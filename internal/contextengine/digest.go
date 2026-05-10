package contextengine

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256Digest returns the lowercase hex-encoded SHA-256 digest of content.
func SHA256Digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
