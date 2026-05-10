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
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	if store != nil {
		hash := strings.TrimSpace(store.AdminPasswordHash())
		if hash != "" {
			return verifyAdminPasswordHash(candidate, hash)
		}
	}
	key := effectiveAdminKey(store)
	if key == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(key)) == 1
}

func UsingDefaultAdminKey(store AdminConfigReader) bool {
	if store != nil && strings.TrimSpace(store.AdminPasswordHash()) != "" {
		return false
	}
	return strings.TrimSpace(os.Getenv("DS2API_ADMIN_KEY")) == ""
}

func HashAdminPassword(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func verifyAdminPasswordHash(candidate, encoded string) bool {
	encoded = strings.TrimSpace(strings.ToLower(encoded))
	if encoded == "" {
		return false
	}
	if strings.HasPrefix(encoded, "sha256:") {
		want := strings.TrimPrefix(encoded, "sha256:")
		sum := sha256.Sum256([]byte(candidate))
		got := hex.EncodeToString(sum[:])
		return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(encoded)) == 1
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
