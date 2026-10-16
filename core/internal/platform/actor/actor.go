package actor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Role identifies one of the four business actors or the internal SOPORTE actor.
type Role string

const (
	RoleAdmin    Role = "ADMIN"
	RoleSoporte  Role = "SOPORTE"
	RoleOwner    Role = "OWNER"
	RoleEmployee Role = "EMPLOYEE"
	RolePlayer   Role = "PLAYER"
)

// AccountKind distinguishes work accounts from customer accounts.
type AccountKind string

const (
	KindWork     AccountKind = "WORK"
	KindCustomer AccountKind = "CUSTOMER"
)

// ScopeContext holds the active business and branch selection inside an access token.
type ScopeContext struct {
	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id,omitempty"`
}

// Principal is the resolved, verified actor attached to a request context by the
// authn guard middleware. It is a portable value type so modules never import
// identity's core.
type Principal struct {
	AccountID string
	Email     string
	Kind      AccountKind
	Actor     Role
	Context   *ScopeContext
}

type contextKey struct{}

// WithPrincipal stores the verified principal in ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}

// FromContext extracts the verified principal from ctx.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}

// NewUUIDv7 generates a time-ordered RFC 9562 UUIDv7 string using crypto/rand.
func NewUUIDv7() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random bytes for uuidv7: %w", err)
	}
	ms := uint64(time.Now().UTC().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)

	// version 7 (0111)
	b[6] = (b[6] & 0x0f) | 0x70
	// variant RFC 4122 / 9562 (10xx)
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// IsValidUUID checks canonical 36-char UUID syntax (8-4-4-4-12 hex digits) and
// returns the lowercase canonical form.
func NormalizeUUID(raw string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if len(s) != 36 {
		return "", false
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return "", false
	}
	hexPart := s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	if _, err := hex.DecodeString(hexPart); err != nil {
		return "", false
	}
	return s, true
}
