package security_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/security"
)

func TestPasswordPolicyAndArgon2id(t *testing.T) {
	t.Parallel()

	// Fewer than 12 characters must fail
	if err := security.ValidatePassword("short123456"); err == nil {
		t.Fatal("expected 11-character password to fail validation")
	}

	// Control character inside password must fail
	if err := security.ValidatePassword("validlength\x00123"); err == nil {
		t.Fatal("expected password with NUL byte to fail validation")
	}

	// Over 128 bytes must fail
	if err := security.ValidatePassword(strings.Repeat("a", 129)); err == nil {
		t.Fatal("expected 129-byte password to fail validation")
	}

	// Unicode NFC equivalence: "e\u0301" (decomposed) and "\u00e9" (composed)
	decomposed := "contrase\u0301na-segura-12"
	composed := "contrase\u00f1a-segura-12"
	_ = decomposed
	_ = composed
	decomposedE := "clavesegurae\u0301" // 12 runes once NFC-normalized ("claveseguraé")
	composedE := "clavesegura\u00e9"

	hash, err := security.HashPassword(decomposedE)
	if err != nil {
		t.Fatalf("expected valid 12-rune NFC password to hash, got: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=1,p=2$") {
		t.Fatalf("expected PHC argon2id prefix, got %q", hash)
	}
	if !security.VerifyPassword(composedE, hash) {
		t.Fatal("expected NFC-equivalent password to verify against hash")
	}
	if security.VerifyPassword("claveseguraX", hash) {
		t.Fatal("expected wrong password not to verify")
	}
}

func TestAccessTokenHS256AndClaims(t *testing.T) {
	t.Parallel()

	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	// Staff token with context
	tok, err := security.SignAccessToken(secret, security.AccessTokenClaims{
		Subject: "019294f0-0000-7000-8000-000000000001",
		Actor:   actor.RoleOwner,
		Context: &actor.ScopeContext{
			BusinessID: "019294f0-0000-7000-8000-000000000010",
			BranchID:   "019294f0-0000-7000-8000-000000000020",
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(security.AccessTokenTTL),
	})
	if err != nil {
		t.Fatalf("SignAccessToken failed: %v", err)
	}

	claims, err := security.VerifyAccessToken(secret, tok, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("VerifyAccessToken failed: %v", err)
	}
	if claims.Subject != "019294f0-0000-7000-8000-000000000001" || claims.Actor != actor.RoleOwner {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.Context == nil || claims.Context.BusinessID != "019294f0-0000-7000-8000-000000000010" {
		t.Fatalf("expected context claim, got %+v", claims.Context)
	}

	// Expired token must fail
	if _, err := security.VerifyAccessToken(secret, tok, now.Add(31*time.Minute)); err == nil {
		t.Fatal("expected expired token to be rejected")
	}

	// Wrong secret must fail
	if _, err := security.VerifyAccessToken([]byte("wrong-secret-0123456789abcdef0123"), tok, now); err == nil {
		t.Fatal("expected wrong signature to be rejected")
	}
}

func TestOpaqueTokenSHA256(t *testing.T) {
	t.Parallel()

	raw, hash, err := security.NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken failed: %v", err)
	}
	if len(raw) < 40 {
		t.Fatalf("expected base64url token, got %q", raw)
	}
	if len(hash) != 32 {
		t.Fatalf("expected 32-byte SHA-256 hash, got %d bytes", len(hash))
	}
	if !bytes.Equal(hash, security.HashOpaqueToken(raw)) {
		t.Fatal("expected HashOpaqueToken(raw) to match returned hash")
	}
}
