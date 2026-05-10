package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the cost parameter used for new admin password hashes.
// Kept at the library default (10) to balance CPU cost against login latency.
const bcryptCost = bcrypt.DefaultCost

const (
	bcryptHashPrefix = "bcrypt:"
	sha256HashPrefix = "sha256:"
)

var warnOnce sync.Once

type AdminConfigReader interface {
	AdminPasswordHash() string
	AdminJWTExpireHours() int
	AdminJWTValidAfterUnix() int64
}

func AdminKey() string {
	return effectiveAdminKey(nil)
}

func effectiveAdminKey(store AdminConfigReader) string {
	if store != nil {
		if hash := strings.TrimSpace(store.AdminPasswordHash()); hash != "" {
			return ""
		}
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_ADMIN_KEY")); v != "" {
		return v
	}
	warnOnce.Do(func() {
		slog.Error("SECURITY: DS2API_ADMIN_KEY is not set. Using insecure default \"admin\". Set a strong key before use.")
	})
	return "admin"
}

func jwtSecret(store AdminConfigReader) string {
	if v := strings.TrimSpace(os.Getenv("DS2API_JWT_SECRET")); v != "" {
		return v
	}
	if store != nil {
		if hash := strings.TrimSpace(store.AdminPasswordHash()); hash != "" {
			return hash
		}
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_ADMIN_KEY")); v != "" {
		return v
	}
	return "admin"
}

func jwtExpireHours(store AdminConfigReader) int {
	if store != nil {
		if n := store.AdminJWTExpireHours(); n > 0 {
			return n
		}
	}
	if v := strings.TrimSpace(os.Getenv("DS2API_JWT_EXPIRE_HOURS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 24
}

func CreateJWT(expireHours int) (string, error) {
	return CreateJWTWithStore(expireHours, nil)
}

func CreateJWTWithStore(expireHours int, store AdminConfigReader) (string, error) {
	if expireHours <= 0 {
		expireHours = jwtExpireHours(store)
	}
	issuedAt := time.Now().Unix()
	// If sessions were invalidated in this same second, move iat forward by
	// one second so newly minted tokens remain valid with strict cutoff checks.
	if store != nil {
		if validAfter := store.AdminJWTValidAfterUnix(); validAfter >= issuedAt {
			issuedAt = validAfter + 1
		}
	}
	expireAt := time.Unix(issuedAt, 0).Add(time.Duration(expireHours) * time.Hour).Unix()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	payload := map[string]any{"iat": issuedAt, "exp": expireAt, "role": "admin"}
	h, _ := json.Marshal(header)
	p, _ := json.Marshal(payload)
	headerB64 := rawB64Encode(h)
	payloadB64 := rawB64Encode(p)
	msg := headerB64 + "." + payloadB64
	sig := signHS256(msg, store)
	return msg + "." + rawB64Encode(sig), nil
}

func VerifyJWT(token string) (map[string]any, error) {
	return VerifyJWTWithStore(token, nil)
}

func VerifyJWTWithStore(token string, store AdminConfigReader) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}
	msg := parts[0] + "." + parts[1]
	expected := signHS256(msg, store)
	actual, err := rawB64Decode(parts[2])
	if err != nil {
		return nil, errors.New("invalid signature")
	}
	if !hmac.Equal(expected, actual) {
		return nil, errors.New("invalid signature")
	}
	payloadBytes, err := rawB64Decode(parts[1])
	if err != nil {
		return nil, errors.New("invalid payload")
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, errors.New("invalid payload")
	}
	exp, _ := payload["exp"].(float64)
	if int64(exp) < time.Now().Unix() {
		return nil, errors.New("token expired")
	}
	if store != nil {
		validAfter := store.AdminJWTValidAfterUnix()
		if validAfter > 0 {
			iat, _ := payload["iat"].(float64)
			if int64(iat) <= validAfter {
				return nil, errors.New("token expired")
			}
		}
	}
	return payload, nil
}

func VerifyAdminRequest(r *http.Request) error {
	return VerifyAdminRequestWithStore(r, nil)
}

func VerifyAdminRequestWithStore(r *http.Request, store AdminConfigReader) error {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return errors.New("authentication required")
	}
	token := strings.TrimSpace(authHeader[7:])
	if token == "" {
		return errors.New("authentication required")
	}
	if VerifyAdminCredential(token, store) {
		return nil
	}
	if _, err := VerifyJWTWithStore(token, store); err == nil {
		return nil
	}
	return errors.New("invalid credentials")
}

func VerifyAdminCredential(candidate string, store AdminConfigReader) bool {
	ok, _ := VerifyAdminCredentialWithUpgrade(candidate, store)
	return ok
}

// VerifyAdminCredentialWithUpgrade verifies the admin candidate against the
// configured password hash (or fallback admin key). When the stored hash uses
// a legacy algorithm (currently sha256) and verification succeeds, the
// returned upgradedHash is a freshly generated bcrypt hash that the caller
// should persist transparently. upgradedHash is empty when no rewrite is
// needed (already bcrypt, fallback admin-key path, or verification failed).
func VerifyAdminCredentialWithUpgrade(candidate string, store AdminConfigReader) (ok bool, upgradedHash string) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false, ""
	}
	if store != nil {
		hash := strings.TrimSpace(store.AdminPasswordHash())
		if hash != "" {
			if !verifyAdminPasswordHash(candidate, hash) {
				return false, ""
			}
			if isLegacyHash(hash) {
				if up := HashAdminPassword(candidate); up != "" {
					return true, up
				}
			}
			return true, ""
		}
	}
	key := effectiveAdminKey(store)
	if key == "" {
		return false, ""
	}
	if subtle.ConstantTimeCompare([]byte(candidate), []byte(key)) != 1 {
		return false, ""
	}
	return true, ""
}

func UsingDefaultAdminKey(store AdminConfigReader) bool {
	if store != nil && strings.TrimSpace(store.AdminPasswordHash()) != "" {
		return false
	}
	return strings.TrimSpace(os.Getenv("DS2API_ADMIN_KEY")) == ""
}

// HashAdminPassword produces a bcrypt-prefixed hash for the supplied raw
// password. Empty input returns an empty string. Legacy sha256-prefixed
// hashes are still accepted by verifyAdminPasswordHash for backward
// compatibility but never produced here.
//
// The raw password is pre-hashed with SHA-256 before being fed to bcrypt so
// that passwords longer than bcrypt's 72-byte input limit are handled
// correctly rather than silently falling back to the legacy sha256 algorithm.
// Both HashAdminPassword and verifyAdminPasswordHash apply the same
// normalization step, keeping the stored format opaque to callers.
func HashAdminPassword(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	normalized := prehashForBcrypt(raw)
	digest, err := bcrypt.GenerateFromPassword([]byte(normalized), bcryptCost)
	if err != nil {
		// Should not happen with a 64-char hex input, but guard anyway.
		slog.Error("bcrypt hash generation failed; falling back to legacy sha256 hash", "err", err)
		sum := sha256.Sum256([]byte(raw))
		return sha256HashPrefix + hex.EncodeToString(sum[:])
	}
	return bcryptHashPrefix + string(digest)
}

// prehashForBcrypt converts the raw password to a 64-character hex-encoded
// SHA-256 digest so that bcrypt always receives input well within its 72-byte
// limit, regardless of how long the raw password is.
func prehashForBcrypt(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// isLegacyHash reports whether the encoded hash uses an algorithm that should
// be transparently upgraded on next successful verification. Only the known
// legacy sha256 prefix is treated as upgradeable; unrecognised or future
// prefixes are left untouched to avoid silently overwriting them.
func isLegacyHash(encoded string) bool {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return false
	}
	return strings.HasPrefix(strings.ToLower(encoded), sha256HashPrefix)
}

func verifyAdminPasswordHash(candidate, encoded string) bool {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return false
	}
	// Only lowercase the prefix marker; the body of bcrypt hashes is
	// case-sensitive (modular crypt format / Base64) and must be preserved.
	lower := strings.ToLower(encoded)
	switch {
	case strings.HasPrefix(lower, bcryptHashPrefix):
		hash := encoded[len(bcryptHashPrefix):]
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(prehashForBcrypt(candidate))) == nil
	case strings.HasPrefix(lower, sha256HashPrefix):
		want := lower[len(sha256HashPrefix):]
		sum := sha256.Sum256([]byte(candidate))
		got := hex.EncodeToString(sum[:])
		return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
	default:
		// Legacy plaintext (no recognised prefix): constant-time compare
		// against the lowercased value to preserve historical behaviour.
		return subtle.ConstantTimeCompare([]byte(candidate), []byte(lower)) == 1
	}
}

func signHS256(msg string, store AdminConfigReader) []byte {
	h := hmac.New(sha256.New, []byte(jwtSecret(store)))
	_, _ = h.Write([]byte(msg))
	return h.Sum(nil)
}

func rawB64Encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func rawB64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
