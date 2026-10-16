package authn_test

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/authn"
	"canchalibre/core/internal/platform/security"
)

type stubVerifier struct {
	accounts map[string]authn.AccountState
	scopes   map[string]actor.Role
}

func (s *stubVerifier) GetAccountState(_ context.Context, accountID string) (authn.AccountState, error) {
	st, ok := s.accounts[accountID]
	if !ok {
		return authn.AccountState{}, errors.New("not found")
	}
	return st, nil
}

func (s *stubVerifier) VerifyScope(_ context.Context, accountID, businessID, branchID string) (actor.Role, error) {
	key := accountID + "|" + businessID + "|" + branchID
	r, ok := s.scopes[key]
	if !ok {
		return "", errors.New("not authorized")
	}
	return r, nil
}

func (s *stubVerifier) HasAnyActiveMembership(_ context.Context, accountID string) (actor.Role, bool, error) {
	for k, r := range s.scopes {
		if strings.HasPrefix(k, accountID+"|") {
			return r, true, nil
		}
	}
	return "", false, nil
}

func TestGuardRevocationCutsNextRequestImmediately(t *testing.T) {
	t.Parallel()

	secret := []byte("0123456789abcdef0123456789abcdef")
	accID := "019294f0-0000-7000-8000-000000000002"
	busID := "019294f0-0000-7000-8000-000000000101"
	brID := "019294f0-0000-7000-8000-000000000201"

	now := time.Now().UTC()
	tok, err := security.SignAccessToken(secret, security.AccessTokenClaims{
		Subject:   accID,
		Actor:     actor.RoleOwner,
		Context:   &actor.ScopeContext{BusinessID: busID, BranchID: brID},
		IssuedAt:  now,
		ExpiresAt: now.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	sv := &stubVerifier{
		accounts: map[string]authn.AccountState{
			accID: {AccountID: accID, Email: "owner@canchalibre.local", Kind: actor.KindWork, Status: "ACTIVE"},
		},
		scopes: map[string]actor.Role{
			accID + "|" + busID + "|" + brID: actor.RoleOwner,
		},
	}

	guard := authn.NewGuard(secret, sv, sv)

	p, status, _, err := guard.AuthenticateRequest(context.Background(), "Bearer "+tok)
	if err != nil || status != 0 || p.Actor != actor.RoleOwner {
		t.Fatalf("expected active membership to pass, got status=%d err=%v", status, err)
	}

	// Revoke the membership in the backing store while the token is still cryptographically valid
	delete(sv.scopes, accID+"|"+busID+"|"+brID)

	_, status, probType, err := guard.AuthenticateRequest(context.Background(), "Bearer "+tok)
	if err == nil || status != http.StatusForbidden {
		t.Fatalf("expected revoked membership to fail with 403 on next request, got status=%d typ=%s err=%v", status, probType, err)
	}
}

func TestNoCrossContextCoreImports(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}} -> {{join .Imports \",\"}}", "./internal/modules/...")
	cmd.Dir = "../../.."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, string(out))
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, " -> ")
		if len(parts) != 2 {
			continue
		}
		pkg, imports := parts[0], parts[1]
		if strings.Contains(pkg, "/modules/identity/") && strings.Contains(imports, "canchalibre/core/internal/modules/organization/core") {
			t.Fatalf("forbidden cross-context core import in %s: %s", pkg, imports)
		}
		if strings.Contains(pkg, "/modules/organization/") && strings.Contains(imports, "canchalibre/core/internal/modules/identity/core") {
			t.Fatalf("forbidden cross-context core import in %s: %s", pkg, imports)
		}
	}
}
