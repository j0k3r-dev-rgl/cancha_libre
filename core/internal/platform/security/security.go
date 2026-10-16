package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"

	"canchalibre/core/internal/platform/actor"
)

const (
	// MinPasswordRunes is the minimum password length in Unicode runes after NFC normalization.
	MinPasswordRunes = 12
	// MaxPasswordBytes caps password byte length to prevent hashing DoS.
	MaxPasswordBytes = 128

	argonTime    uint32 = 1
	argonMemory  uint32 = 19456 // 19 MiB (OWASP minimum, safe for 2 vCPU / 8 GB VPS)
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16

	// AccessTokenTTL is D4 from docs/design/identity-auth.md (30 minutes).
	AccessTokenTTL = 30 * time.Minute
	// RefreshTokenTTL is D5 from docs/design/identity-auth.md (7 days).
	RefreshTokenTTL = 7 * 24 * time.Hour
	// DefaultInvitationTTL is the default invitation lifetime (24 hours).
	DefaultInvitationTTL = 24 * time.Hour
	// PasswordRecoveryTTL is the recovery token lifetime (30 minutes).
	PasswordRecoveryTTL = 30 * time.Minute
)

var (
	ErrInvalidPassword = errors.New("password must be between 12 characters and 128 bytes without control characters")
	ErrInvalidEmail    = errors.New("invalid email address")
	ErrInvalidToken    = errors.New("invalid or expired token")
)

// NormalizeEmail trims surrounding whitespace, normalizes Unicode to NFC,
// lowercases the address, and validates basic structure without control characters.
func NormalizeEmail(raw string) (string, error) {
	s := norm.NFC.String(strings.TrimSpace(raw))
	s = strings.ToLower(s)
	if s == "" || len(s) > 254 || !utf8.ValidString(s) {
		return "", ErrInvalidEmail
	}
	for _, r := range s {
		if r <= 0x20 || r == 0x7f {
			return "", ErrInvalidEmail
		}
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at != strings.LastIndexByte(s, '@') || at == len(s)-1 {
		return "", ErrInvalidEmail
	}
	local := s[:at]
	domain := s[at+1:]
	if len(local) > 64 || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return "", ErrInvalidEmail
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", ErrInvalidEmail
	}
	return s, nil
}

// ValidatePassword normalizes raw to Unicode NFC and enforces the 12-rune minimum,
// 128-byte ceiling, and absence of control characters.
func ValidatePassword(raw string) error {
	_, err := normalizeAndValidatePassword(raw)
	return err
}

func normalizeAndValidatePassword(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", ErrInvalidPassword
	}
	nfc := norm.NFC.String(raw)
	if len(nfc) > MaxPasswordBytes {
		return "", ErrInvalidPassword
	}
	if utf8.RuneCountInString(nfc) < MinPasswordRunes {
		return "", ErrInvalidPassword
	}
	for _, r := range nfc {
		if r < 0x20 || r == 0x7f {
			return "", ErrInvalidPassword
		}
	}
	return nfc, nil
}

// HashPassword validates raw and hashes its NFC-normalized form with Argon2id.
func HashPassword(raw string) (string, error) {
	nfc, err := normalizeAndValidatePassword(raw)
	if err != nil {
		return "", err
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("read salt: %w", err)
	}
	hash := argon2.IDKey([]byte(nfc), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword checks raw against a PHC-formatted Argon2id hash in constant time.
func VerifyPassword(raw, encoded string) bool {
	if !utf8.ValidString(raw) {
		return false
	}
	nfc := norm.NFC.String(raw)
	if len(nfc) == 0 || len(nfc) > MaxPasswordBytes {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var memory, timeCost uint32
	var threads uint8
	for _, kv := range strings.Split(parts[3], ",") {
		switch {
		case strings.HasPrefix(kv, "m="):
			v, err := strconv.ParseUint(strings.TrimPrefix(kv, "m="), 10, 32)
			if err != nil {
				return false
			}
			memory = uint32(v)
		case strings.HasPrefix(kv, "t="):
			v, err := strconv.ParseUint(strings.TrimPrefix(kv, "t="), 10, 32)
			if err != nil {
				return false
			}
			timeCost = uint32(v)
		case strings.HasPrefix(kv, "p="):
			v, err := strconv.ParseUint(strings.TrimPrefix(kv, "p="), 10, 8)
			if err != nil {
				return false
			}
			threads = uint8(v)
		}
	}
	if memory == 0 || timeCost == 0 || threads == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false
	}
	wantHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(wantHash) == 0 {
		return false
	}
	gotHash := argon2.IDKey([]byte(nfc), salt, timeCost, memory, threads, uint32(len(wantHash)))
	return subtle.ConstantTimeCompare(gotHash, wantHash) == 1
}

// AccessTokenClaims matches D3 of docs/design/identity-auth.md.
type AccessTokenClaims struct {
	Subject   string              `json:"sub"`
	Actor     actor.Role          `json:"actor"`
	Context   *actor.ScopeContext `json:"ctx,omitempty"`
	IssuedAt  time.Time           `json:"-"`
	ExpiresAt time.Time           `json:"-"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtPayload struct {
	Sub   string              `json:"sub"`
	Actor actor.Role          `json:"actor"`
	Ctx   *actor.ScopeContext `json:"ctx,omitempty"`
	Iat   int64               `json:"iat"`
	Exp   int64               `json:"exp"`
}

const hs256HeaderB64 = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"

// SignAccessToken signs an HS256 JWT with the given secret.
func SignAccessToken(secret []byte, claims AccessTokenClaims) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("jwt secret must be at least 32 bytes")
	}
	if claims.Subject == "" || claims.Actor == "" {
		return "", errors.New("sub and actor claims are required")
	}
	payload := jwtPayload{
		Sub:   claims.Subject,
		Actor: claims.Actor,
		Ctx:   claims.Context,
		Iat:   claims.IssuedAt.UTC().Unix(),
		Exp:   claims.ExpiresAt.UTC().Unix(),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal jwt payload: %w", err)
	}
	unsigned := hs256HeaderB64 + "." + base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig, nil
}

// VerifyAccessToken verifies the HS256 signature, header algorithm, and expiry.
func VerifyAccessToken(secret []byte, token string, now time.Time) (AccessTokenClaims, error) {
	if len(secret) < 32 || len(token) == 0 || len(token) > 4096 {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	var hdr jwtHeader
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil || hdr.Alg != "HS256" || hdr.Typ != "JWT" {
		return AccessTokenClaims{}, ErrInvalidToken
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	expectedSig := mac.Sum(nil)
	if subtle.ConstantTimeCompare(sigBytes, expectedSig) != 1 {
		return AccessTokenClaims{}, ErrInvalidToken
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	var payload jwtPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	if payload.Sub == "" || payload.Actor == "" || payload.Exp <= now.UTC().Unix() {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	if _, ok := actor.NormalizeUUID(payload.Sub); !ok {
		return AccessTokenClaims{}, ErrInvalidToken
	}
	if payload.Ctx != nil {
		if _, ok := actor.NormalizeUUID(payload.Ctx.BusinessID); !ok {
			return AccessTokenClaims{}, ErrInvalidToken
		}
		if payload.Ctx.BranchID != "" {
			if _, ok := actor.NormalizeUUID(payload.Ctx.BranchID); !ok {
				return AccessTokenClaims{}, ErrInvalidToken
			}
		}
	}
	return AccessTokenClaims{
		Subject:   payload.Sub,
		Actor:     payload.Actor,
		Context:   payload.Ctx,
		IssuedAt:  time.Unix(payload.Iat, 0).UTC(),
		ExpiresAt: time.Unix(payload.Exp, 0).UTC(),
	}, nil
}

// NewOpaqueToken generates a 32-byte cryptographically random token encoded as
// base64url, alongside its 32-byte SHA-256 digest for database storage.
func NewOpaqueToken() (string, []byte, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", nil, fmt.Errorf("read random token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(b[:])
	return raw, HashOpaqueToken(raw), nil
}

// HashOpaqueToken computes the 32-byte SHA-256 digest of raw.
func HashOpaqueToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
