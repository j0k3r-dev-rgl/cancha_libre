package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/modules/identity/core/ports"
	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/security"
)

// Service implements the identity bounded-context use cases.
type Service struct {
	repo              ports.Repository
	scopes            ports.ScopePort
	emailSender       ports.EmailSender
	jwtSecret         []byte
	invitationURLBase string
	now               func() time.Time
}

// Config configures the identity use-case service.
type Config struct {
	JWTSecret         []byte
	InvitationURLBase string
}

// NewService constructs the identity Service.
func NewService(repo ports.Repository, scopes ports.ScopePort, emailSender ports.EmailSender, cfg Config) *Service {
	base := strings.TrimRight(strings.TrimSpace(cfg.InvitationURLBase), "/")
	if base == "" {
		base = "http://localhost:3000/auth/activate"
	}
	return &Service{
		repo:              repo,
		scopes:            scopes,
		emailSender:       emailSender,
		jwtSecret:         cfg.JWTSecret,
		invitationURLBase: base,
		now:               func() time.Time { return time.Now().UTC() },
	}
}

// LoginInput holds the credentials presented to POST /api/v1/auth/login.
type LoginInput struct {
	Email       string
	Identifier  string
	Password    string
	AccountKind string
}

// LoginOutput holds the issued session tokens and the available work contexts.
type LoginOutput struct {
	AccessToken      string
	ExpiresIn        int
	RefreshToken     string
	RefreshExpiresAt time.Time
	Context          []domain.LoginContextItem
}

func hasControlChars(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// Login authenticates an account and issues an access token plus a rotating refresh session.
func (s *Service) Login(ctx context.Context, in LoginInput) (LoginOutput, error) {
	rawEmail := strings.TrimSpace(in.Email)
	if rawEmail == "" {
		rawEmail = strings.TrimSpace(in.Identifier)
	}
	if in.Email != "" && in.Identifier != "" && !strings.EqualFold(strings.TrimSpace(in.Email), strings.TrimSpace(in.Identifier)) {
		s.auditRefused(ctx, "login", nil, nil, "conflicting email and identifier")
		return LoginOutput{}, domain.ErrValidation
	}
	normEmail, err := security.NormalizeEmail(rawEmail)
	if err != nil || in.Password == "" || len(in.Password) > security.MaxPasswordBytes || hasControlChars(in.Password) {
		s.auditRefused(ctx, "login", nil, nil, "invalid login input")
		return LoginOutput{}, domain.ErrValidation
	}

	kindStr := strings.ToUpper(strings.TrimSpace(in.AccountKind))
	if kindStr != "" && kindStr != string(actor.KindWork) && kindStr != string(actor.KindCustomer) {
		s.auditRefused(ctx, "login", nil, nil, "invalid accountKind")
		return LoginOutput{}, domain.ErrValidation
	}

	var matchedAcc domain.Account
	var pwHash string

	if kindStr != "" {
		acc, hash, err := s.repo.FindAccountByEmailAndKind(ctx, normEmail, actor.AccountKind(kindStr))
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				s.auditRefused(ctx, "login", nil, nil, "account not found")
				return LoginOutput{}, domain.ErrUnauthenticated
			}
			return LoginOutput{}, err
		}
		matchedAcc = acc
		pwHash = hash
		if pwHash == "" || !security.VerifyPassword(in.Password, pwHash) {
			s.auditRefused(ctx, "login", nil, &matchedAcc.ID, "invalid credentials")
			return LoginOutput{}, domain.ErrUnauthenticated
		}
	} else {
		accounts, hashes, err := s.repo.FindAccountByEmailAnyKind(ctx, normEmail)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				s.auditRefused(ctx, "login", nil, nil, "account not found")
				return LoginOutput{}, domain.ErrUnauthenticated
			}
			return LoginOutput{}, err
		}
		found := false
		for _, candidate := range accounts {
			if h := hashes[candidate.ID]; h != "" && security.VerifyPassword(in.Password, h) {
				matchedAcc = candidate
				pwHash = h
				found = true
				break
			}
		}
		if !found {
			subjID := accounts[0].ID
			s.auditRefused(ctx, "login", nil, &subjID, "invalid credentials")
			return LoginOutput{}, domain.ErrUnauthenticated
		}
	}

	if matchedAcc.Status != domain.StatusActive {
		s.auditRefused(ctx, "login", nil, &matchedAcc.ID, "account status is "+matchedAcc.Status)
		return LoginOutput{}, domain.ErrForbidden
	}

	role, ctxClaim, loginContexts, err := s.resolveRoleAndContexts(ctx, matchedAcc)
	if err != nil {
		s.auditRefused(ctx, "login", nil, &matchedAcc.ID, err.Error())
		return LoginOutput{}, err
	}

	now := s.now()
	accessTok, err := security.SignAccessToken(s.jwtSecret, security.AccessTokenClaims{
		Subject:   matchedAcc.ID,
		Actor:     role,
		Context:   ctxClaim,
		IssuedAt:  now,
		ExpiresAt: now.Add(security.AccessTokenTTL),
	})
	if err != nil {
		return LoginOutput{}, err
	}

	refreshRaw, refreshHash, err := security.NewOpaqueToken()
	if err != nil {
		return LoginOutput{}, err
	}
	refreshExp := now.Add(security.RefreshTokenTTL)
	if _, err := s.repo.CreateRefreshSession(ctx, matchedAcc.ID, refreshHash, refreshExp); err != nil {
		return LoginOutput{}, err
	}

	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "login",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &matchedAcc.ID,
		SubjectAccountID: &matchedAcc.ID,
		Reason:           "login succeeded",
	})

	return LoginOutput{
		AccessToken:      accessTok,
		ExpiresIn:        int(security.AccessTokenTTL.Seconds()),
		RefreshToken:     refreshRaw,
		RefreshExpiresAt: refreshExp,
		Context:          loginContexts,
	}, nil
}

func (s *Service) resolveRoleAndContexts(ctx context.Context, acc domain.Account) (actor.Role, *actor.ScopeContext, []domain.LoginContextItem, error) {
	if acc.Kind == actor.KindCustomer {
		return actor.RolePlayer, nil, nil, nil
	}
	if acc.PlatformRole != nil {
		return *acc.PlatformRole, nil, nil, nil
	}
	if s.scopes == nil {
		return "", nil, nil, domain.ErrForbidden
	}
	ctxs, err := s.scopes.ListAccountContexts(ctx, acc.ID)
	if err != nil {
		return "", nil, nil, err
	}
	if len(ctxs) == 0 {
		return "", nil, nil, domain.ErrForbidden
	}
	items := make([]domain.LoginContextItem, 0, len(ctxs))
	for _, c := range ctxs {
		items = append(items, domain.LoginContextItem{
			BusinessID: c.BusinessID,
			BranchID:   c.BranchID,
			Role:       c.Role,
		})
	}
	primary := items[0]
	return primary.Role, &actor.ScopeContext{
		BusinessID: primary.BusinessID,
		BranchID:   primary.BranchID,
	}, items, nil
}

// RefreshOutput holds the rotated session credentials.
type RefreshOutput struct {
	AccessToken      string
	ExpiresIn        int
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// Refresh exchanges a refresh token for a new access token and rotated refresh cookie.
func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (RefreshOutput, error) {
	tok := strings.TrimSpace(rawRefreshToken)
	if tok == "" || len(tok) > 512 || hasControlChars(tok) {
		s.auditRefused(ctx, "refresh", nil, nil, "missing or malformed refresh token")
		return RefreshOutput{}, domain.ErrUnauthenticated
	}

	nextRaw, nextHash, err := security.NewOpaqueToken()
	if err != nil {
		return RefreshOutput{}, err
	}
	now := s.now()
	nextExp := now.Add(security.RefreshTokenTTL)

	sess, err := s.repo.RotateRefreshSession(ctx, security.HashOpaqueToken(tok), nextHash, nextExp)
	if err != nil {
		if errors.Is(err, domain.ErrRefreshReuse) {
			s.auditRefused(ctx, "refresh", nil, nil, "refresh token reuse detected; family revoked")
			return RefreshOutput{}, domain.ErrRefreshReuse
		}
		s.auditRefused(ctx, "refresh", nil, nil, "invalid or expired refresh token")
		return RefreshOutput{}, domain.ErrUnauthenticated
	}

	acc, err := s.repo.FindAccountByID(ctx, sess.AccountID)
	if err != nil || acc.Status != domain.StatusActive {
		s.auditRefused(ctx, "refresh", &sess.AccountID, &sess.AccountID, "account not active")
		return RefreshOutput{}, domain.ErrUnauthenticated
	}

	role, ctxClaim, _, err := s.resolveRoleAndContexts(ctx, acc)
	if err != nil {
		s.auditRefused(ctx, "refresh", &acc.ID, &acc.ID, "no active role or context")
		return RefreshOutput{}, domain.ErrUnauthenticated
	}

	accessTok, err := security.SignAccessToken(s.jwtSecret, security.AccessTokenClaims{
		Subject:   acc.ID,
		Actor:     role,
		Context:   ctxClaim,
		IssuedAt:  now,
		ExpiresAt: now.Add(security.AccessTokenTTL),
	})
	if err != nil {
		return RefreshOutput{}, err
	}

	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "refresh",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &acc.ID,
		SubjectAccountID: &acc.ID,
		Reason:           "refresh succeeded",
	})

	return RefreshOutput{
		AccessToken:      accessTok,
		ExpiresIn:        int(security.AccessTokenTTL.Seconds()),
		RefreshToken:     nextRaw,
		RefreshExpiresAt: nextExp,
	}, nil
}

// ActivateInvitation consumes a single-use work invitation and sets the account password.
func (s *Service) ActivateInvitation(ctx context.Context, rawToken, rawPassword string) (domain.Account, error) {
	tok := strings.TrimSpace(rawToken)
	if tok == "" || len(tok) > 512 || hasControlChars(tok) {
		s.auditRefused(ctx, "activate_invitation", nil, nil, "invalid invitation token format")
		return domain.Account{}, domain.ErrValidation
	}
	if err := security.ValidatePassword(rawPassword); err != nil {
		s.auditRefused(ctx, "activate_invitation", nil, nil, "password policy violation")
		return domain.Account{}, domain.ErrValidation
	}

	inv, err := s.repo.FindInvitationByHash(ctx, security.HashOpaqueToken(tok))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.auditRefused(ctx, "activate_invitation", nil, nil, "unknown invitation token")
			return domain.Account{}, domain.ErrValidation
		}
		return domain.Account{}, err
	}

	now := s.now()
	if inv.ConsumedAt != nil {
		s.auditRefused(ctx, "activate_invitation", nil, inv.ConsumedBy, "invitation already consumed")
		return domain.Account{}, domain.ErrInvitationConsumed
	}
	if inv.RetiredAt != nil {
		s.auditRefused(ctx, "activate_invitation", nil, nil, "invitation retired")
		return domain.Account{}, domain.ErrInvitationExpired
	}
	if !inv.ExpiresAt.After(now) {
		s.auditRefused(ctx, "activate_invitation", nil, nil, "invitation expired")
		return domain.Account{}, domain.ErrInvitationExpired
	}

	pwHash, err := security.HashPassword(rawPassword)
	if err != nil {
		s.auditRefused(ctx, "activate_invitation", nil, nil, "password hash validation failed")
		return domain.Account{}, domain.ErrValidation
	}

	acc, activatedInv, err := s.repo.ActivateInvitation(ctx, inv.ID, inv.Email, inv.IntendedRole, pwHash, now)
	if err != nil {
		s.auditRefused(ctx, "activate_invitation", nil, nil, err.Error())
		return domain.Account{}, err
	}

	if (activatedInv.IntendedRole == actor.RoleOwner || activatedInv.IntendedRole == actor.RoleEmployee) && activatedInv.BusinessID != nil && s.scopes != nil {
		if err := s.scopes.EnsureMembershipOnActivation(ctx, acc.ID, *activatedInv.BusinessID, activatedInv.BranchID, activatedInv.IntendedRole); err != nil {
			return domain.Account{}, err
		}
	}

	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "activate_invitation",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &acc.ID,
		SubjectAccountID: &acc.ID,
		Reason:           "invitation activated",
		Detail:           map[string]any{"invitation_id": activatedInv.ID, "role": string(activatedInv.IntendedRole)},
	})
	return acc, nil
}

// RequestPasswordRecovery issues a recovery token when an active account exists,
// returning the same outcome whether or not the email exists.
func (s *Service) RequestPasswordRecovery(ctx context.Context, rawEmail, rawAccountKind string) error {
	normEmail, err := security.NormalizeEmail(rawEmail)
	if err != nil {
		s.auditRefused(ctx, "password_recovery", nil, nil, "invalid email")
		return domain.ErrValidation
	}
	kindStr := strings.ToUpper(strings.TrimSpace(rawAccountKind))
	if kindStr != string(actor.KindWork) && kindStr != string(actor.KindCustomer) {
		s.auditRefused(ctx, "password_recovery", nil, nil, "invalid accountKind")
		return domain.ErrValidation
	}

	acc, _, err := s.repo.FindAccountByEmailAndKind(ctx, normEmail, actor.AccountKind(kindStr))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Deliberately indistinguishable from an existing account (202)
			return nil
		}
		return err
	}
	if acc.Status != domain.StatusActive {
		return nil
	}

	rawTok, tokHash, err := security.NewOpaqueToken()
	if err != nil {
		return err
	}
	exp := s.now().Add(security.PasswordRecoveryTTL)
	if err := s.repo.CreatePasswordRecoveryToken(ctx, acc.ID, tokHash, exp); err != nil {
		return err
	}
	if s.emailSender != nil {
		_ = s.emailSender.SendPasswordRecovery(ctx, acc.Email, rawTok)
	}
	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "password_recovery",
		Outcome:          domain.AuditOutcomeSuccess,
		SubjectAccountID: &acc.ID,
		Reason:           "password recovery token issued",
	})
	return nil
}

// ResetPassword consumes a single-use password recovery token and updates the password.
func (s *Service) ResetPassword(ctx context.Context, rawToken, rawPassword string) error {
	tok := strings.TrimSpace(rawToken)
	if tok == "" || len(tok) > 512 || hasControlChars(tok) {
		s.auditRefused(ctx, "reset_password", nil, nil, "invalid token format")
		return domain.ErrValidation
	}
	if err := security.ValidatePassword(rawPassword); err != nil {
		s.auditRefused(ctx, "reset_password", nil, nil, "password policy violation")
		return domain.ErrValidation
	}
	pwHash, err := security.HashPassword(rawPassword)
	if err != nil {
		s.auditRefused(ctx, "reset_password", nil, nil, "password hash error")
		return domain.ErrValidation
	}

	accID, err := s.repo.ConsumePasswordRecoveryToken(ctx, security.HashOpaqueToken(tok), pwHash, s.now())
	if err != nil {
		if errors.Is(err, domain.ErrUnauthenticated) {
			s.auditRefused(ctx, "reset_password", nil, nil, "invalid or expired recovery token")
			return domain.ErrUnauthenticated
		}
		return err
	}

	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "reset_password",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &accID,
		SubjectAccountID: &accID,
		Reason:           "password reset completed",
	})
	return nil
}

// Logout revokes the caller's refresh session (idempotent).
func (s *Service) Logout(ctx context.Context, p actor.Principal, rawRefreshToken string) error {
	tok := strings.TrimSpace(rawRefreshToken)
	if tok != "" && len(tok) <= 512 && !hasControlChars(tok) {
		if err := s.repo.RevokeRefreshSessionByHash(ctx, security.HashOpaqueToken(tok)); err != nil {
			return err
		}
	} else {
		if err := s.repo.RevokeRefreshSessionsByAccount(ctx, p.AccountID); err != nil {
			return err
		}
	}
	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "logout",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &p.AccountID,
		SubjectAccountID: &p.AccountID,
		Reason:           "session logged out",
	})
	return nil
}

// GetOwnAccount returns the authenticated actor's account profile.
func (s *Service) GetOwnAccount(ctx context.Context, p actor.Principal) (domain.Account, error) {
	acc, err := s.repo.FindAccountByID(ctx, p.AccountID)
	if err != nil {
		return domain.Account{}, err
	}
	if acc.Status == domain.StatusClosed {
		return domain.Account{}, domain.ErrNotFound
	}
	return acc, nil
}

// ListOwnContexts returns the active business+branch contexts for the authenticated actor.
func (s *Service) ListOwnContexts(ctx context.Context, p actor.Principal) ([]domain.ContextOption, error) {
	if p.Kind != actor.KindWork || p.Actor == actor.RoleAdmin || p.Actor == actor.RoleSoporte || s.scopes == nil {
		return []domain.ContextOption{}, nil
	}
	return s.scopes.ListAccountContexts(ctx, p.AccountID)
}

// SelectContext validates an active membership for (businessID, branchID) and
// issues a new access token carrying that ctx claim.
func (s *Service) SelectContext(ctx context.Context, p actor.Principal, businessID, branchID string) (string, int, error) {
	normBusID, ok1 := actor.NormalizeUUID(businessID)
	normBrID, ok2 := actor.NormalizeUUID(branchID)
	if !ok1 || !ok2 {
		s.auditRefused(ctx, "select_context", &p.AccountID, &p.AccountID, "invalid business_id or branch_id UUID")
		return "", 0, domain.ErrValidation
	}

	if p.Kind != actor.KindWork || p.Actor == actor.RoleAdmin || p.Actor == actor.RoleSoporte || s.scopes == nil {
		s.auditRefused(ctx, "select_context", &p.AccountID, &p.AccountID, "actor cannot select a branch context")
		return "", 0, domain.ErrContextNotAuthorized
	}

	role, err := s.scopes.VerifyScope(ctx, p.AccountID, normBusID, normBrID)
	if err != nil {
		s.auditRefused(ctx, "select_context", &p.AccountID, &p.AccountID, "context not authorized")
		return "", 0, domain.ErrContextNotAuthorized
	}

	now := s.now()
	accessTok, err := security.SignAccessToken(s.jwtSecret, security.AccessTokenClaims{
		Subject:   p.AccountID,
		Actor:     role,
		Context:   &actor.ScopeContext{BusinessID: normBusID, BranchID: normBrID},
		IssuedAt:  now,
		ExpiresAt: now.Add(security.AccessTokenTTL),
	})
	if err != nil {
		return "", 0, err
	}

	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "select_context",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &p.AccountID,
		SubjectAccountID: &p.AccountID,
		Reason:           "active context selected",
		Detail:           map[string]any{"business_id": normBusID, "branch_id": normBrID, "role": string(role)},
	})
	return accessTok, int(security.AccessTokenTTL.Seconds()), nil
}

// InviteSupportOperator allows ADMIN to issue a single-use SOPORTE invitation.
func (s *Service) InviteSupportOperator(ctx context.Context, p actor.Principal, rawEmail string) (domain.Invitation, string, error) {
	if p.Actor != actor.RoleAdmin {
		s.auditRefused(ctx, "invite_support_operator", &p.AccountID, nil, "actor is not ADMIN")
		return domain.Invitation{}, "", domain.ErrForbidden
	}
	normEmail, err := security.NormalizeEmail(rawEmail)
	if err != nil {
		s.auditRefused(ctx, "invite_support_operator", &p.AccountID, nil, "invalid email")
		return domain.Invitation{}, "", domain.ErrValidation
	}

	existing, _, err := s.repo.FindAccountByEmailAndKind(ctx, normEmail, actor.KindWork)
	if err == nil {
		if existing.Status == domain.StatusActive {
			s.auditRefused(ctx, "invite_support_operator", &p.AccountID, &existing.ID, "work account is already active")
			return domain.Invitation{}, "", domain.ErrConflict
		}
		if existing.PlatformRole != nil && *existing.PlatformRole != actor.RoleSoporte {
			s.auditRefused(ctx, "invite_support_operator", &p.AccountID, &existing.ID, "conflicting platform role")
			return domain.Invitation{}, "", domain.ErrConflict
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Invitation{}, "", err
	}

	hasUnexpired, err := s.repo.HasUnexpiredInvitation(ctx, normEmail, actor.RoleSoporte, nil, nil)
	if err != nil {
		return domain.Invitation{}, "", err
	}
	if hasUnexpired {
		s.auditRefused(ctx, "invite_support_operator", &p.AccountID, nil, "unexpired SOPORTE invitation already exists")
		return domain.Invitation{}, "", domain.ErrConflict
	}

	role := actor.RoleSoporte
	acc, err := s.repo.EnsurePendingWorkAccount(ctx, normEmail, &role)
	if err != nil {
		return domain.Invitation{}, "", err
	}

	rawTok, tokHash, err := security.NewOpaqueToken()
	if err != nil {
		return domain.Invitation{}, "", err
	}
	inv, err := s.repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    tokHash,
		Email:        normEmail,
		IntendedRole: actor.RoleSoporte,
		IssuedBy:     &p.AccountID,
		ExpiresAt:    s.now().Add(security.DefaultInvitationTTL),
	})
	if err != nil {
		return domain.Invitation{}, "", err
	}
	if s.emailSender != nil {
		_ = s.emailSender.SendInvitation(ctx, normEmail, string(actor.RoleSoporte), rawTok)
	}
	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "invite_support_operator",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &p.AccountID,
		SubjectAccountID: &acc.ID,
		Reason:           "SOPORTE invitation issued",
		Detail:           map[string]any{"invitation_id": inv.ID, "email": normEmail},
	})
	return inv, rawTok, nil
}

// RevokeSupportOperator allows ADMIN to close a SOPORTE operator account and revoke its sessions.
func (s *Service) RevokeSupportOperator(ctx context.Context, p actor.Principal, targetAccountID string) error {
	if p.Actor != actor.RoleAdmin {
		s.auditRefused(ctx, "revoke_support_operator", &p.AccountID, nil, "actor is not ADMIN")
		return domain.ErrForbidden
	}
	normTargetID, ok := actor.NormalizeUUID(targetAccountID)
	if !ok {
		s.auditRefused(ctx, "revoke_support_operator", &p.AccountID, nil, "invalid accountId UUID")
		return domain.ErrValidation
	}
	if err := s.repo.RevokeSupportOperator(ctx, normTargetID); err != nil {
		s.auditRefused(ctx, "revoke_support_operator", &p.AccountID, &normTargetID, err.Error())
		return err
	}
	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "revoke_support_operator",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &p.AccountID,
		SubjectAccountID: &normTargetID,
		Reason:           "SOPORTE operator revoked",
	})
	return nil
}

// ProvisionAdminInput configures the private first-ADMIN command.
type ProvisionAdminInput struct {
	Email     string
	TTL       time.Duration
	Supersede bool
	DryRun    bool
}

// ProvisionAdminOutput holds the result of the private first-ADMIN command.
type ProvisionAdminOutput struct {
	InvitationID  string
	ActivationURL string
	ExpiresAt     time.Time
	DryRun        bool
}

// ProvisionAdmin implements the private core/cmd/provision-admin contract (§4).
func (s *Service) ProvisionAdmin(ctx context.Context, in ProvisionAdminInput) (ProvisionAdminOutput, error) {
	normEmail, err := security.NormalizeEmail(in.Email)
	if err != nil {
		return ProvisionAdminOutput{}, fmt.Errorf("%w: invalid email address", domain.ErrValidation)
	}
	ttl := in.TTL
	if ttl == 0 {
		ttl = security.DefaultInvitationTTL
	}
	if ttl < 0 {
		return ProvisionAdminOutput{}, fmt.Errorf("%w: ttl must be positive", domain.ErrValidation)
	}

	existing, _, err := s.repo.FindAccountByEmailAndKind(ctx, normEmail, actor.KindWork)
	if err == nil {
		if existing.Status == domain.StatusActive && existing.PlatformRole != nil && *existing.PlatformRole == actor.RoleAdmin {
			if !in.DryRun {
				s.auditRefused(ctx, "provision_admin", nil, &existing.ID, "email is already an active ADMIN")
			}
			return ProvisionAdminOutput{}, fmt.Errorf("%w: email is already an active ADMIN", domain.ErrConflict)
		}
		if existing.Status == domain.StatusActive {
			if !in.DryRun {
				s.auditRefused(ctx, "provision_admin", nil, &existing.ID, "email is already an active work account")
			}
			return ProvisionAdminOutput{}, fmt.Errorf("%w: email is already an active work account", domain.ErrConflict)
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return ProvisionAdminOutput{}, err
	}

	hasUnexpired, err := s.repo.HasUnexpiredInvitation(ctx, normEmail, actor.RoleAdmin, nil, nil)
	if err != nil {
		return ProvisionAdminOutput{}, err
	}
	if hasUnexpired && !in.Supersede {
		if !in.DryRun {
			s.auditRefused(ctx, "provision_admin", nil, nil, "unexpired ADMIN invitation exists without --supersede")
		}
		return ProvisionAdminOutput{}, fmt.Errorf("%w: unexpired ADMIN invitation already exists (pass --supersede to replace)", domain.ErrConflict)
	}

	if in.DryRun {
		return ProvisionAdminOutput{
			DryRun:    true,
			ExpiresAt: s.now().Add(ttl),
		}, nil
	}

	if hasUnexpired && in.Supersede {
		if err := s.repo.SupersedeInvitations(ctx, normEmail, actor.RoleAdmin); err != nil {
			return ProvisionAdminOutput{}, err
		}
	}

	role := actor.RoleAdmin
	acc, err := s.repo.EnsurePendingWorkAccount(ctx, normEmail, &role)
	if err != nil {
		return ProvisionAdminOutput{}, err
	}

	rawTok, tokHash, err := security.NewOpaqueToken()
	if err != nil {
		return ProvisionAdminOutput{}, err
	}
	exp := s.now().Add(ttl)
	inv, err := s.repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    tokHash,
		Email:        normEmail,
		IntendedRole: actor.RoleAdmin,
		ExpiresAt:    exp,
	})
	if err != nil {
		return ProvisionAdminOutput{}, err
	}

	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "provision_admin",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   nil,
		SubjectAccountID: &acc.ID,
		Reason:           "provision-admin command issued ADMIN invitation",
		Detail: map[string]any{
			"binary":        "provision-admin",
			"email":         normEmail,
			"invitation_id": inv.ID,
			"superseded":    in.Supersede,
		},
	})

	return ProvisionAdminOutput{
		InvitationID:  inv.ID,
		ActivationURL: fmt.Sprintf("%s?token=%s", s.invitationURLBase, rawTok),
		ExpiresAt:     inv.ExpiresAt,
	}, nil
}

// --- Methods implementing organization/core/ports.IdentityPort ---

func (s *Service) VerifyWorkAccountReadyForCoOwner(ctx context.Context, accountID string) error {
	acc, err := s.repo.FindAccountByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrValidation
		}
		return err
	}
	if acc.Kind != actor.KindWork || acc.Status == domain.StatusClosed {
		return domain.ErrValidation
	}
	if acc.PlatformRole != nil {
		return domain.ErrConflict
	}
	return nil
}

func (s *Service) EnsureCoOwnerAccountByEmail(ctx context.Context, issuerAccountID, email, businessID string) (string, error) {
	normEmail, err := security.NormalizeEmail(email)
	if err != nil {
		return "", domain.ErrValidation
	}
	acc, _, err := s.repo.FindAccountByEmailAndKind(ctx, normEmail, actor.KindWork)
	if err == nil {
		if acc.PlatformRole != nil {
			return "", domain.ErrConflict
		}
		return acc.ID, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return "", err
	}

	created, err := s.repo.EnsurePendingWorkAccount(ctx, normEmail, nil)
	if err != nil {
		return "", err
	}
	rawTok, tokHash, err := security.NewOpaqueToken()
	if err != nil {
		return "", err
	}
	_, err = s.repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    tokHash,
		Email:        normEmail,
		IntendedRole: actor.RoleOwner,
		BusinessID:   &businessID,
		IssuedBy:     &issuerAccountID,
		ExpiresAt:    s.now().Add(security.DefaultInvitationTTL),
	})
	if err != nil {
		return "", err
	}
	if s.emailSender != nil {
		_ = s.emailSender.SendInvitation(ctx, normEmail, string(actor.RoleOwner), rawTok)
	}
	return created.ID, nil
}

func (s *Service) LookupAccountKindAndRoleByEmail(ctx context.Context, email string) (bool, string, *actor.Role, error) {
	normEmail, err := security.NormalizeEmail(email)
	if err != nil {
		return false, "", nil, domain.ErrValidation
	}
	acc, _, err := s.repo.FindAccountByEmailAndKind(ctx, normEmail, actor.KindWork)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, "", nil, nil
		}
		return false, "", nil, err
	}
	return true, acc.ID, acc.PlatformRole, nil
}

func (s *Service) CreateWorkInvitation(ctx context.Context, issuerAccountID, email string, intendedRole actor.Role, businessID string, branchID *string) (string, time.Time, error) {
	normEmail, err := security.NormalizeEmail(email)
	if err != nil {
		return "", time.Time{}, domain.ErrValidation
	}
	hasUnexpired, err := s.repo.HasUnexpiredInvitation(ctx, normEmail, intendedRole, &businessID, branchID)
	if err != nil {
		return "", time.Time{}, err
	}
	if hasUnexpired {
		return "", time.Time{}, domain.ErrConflict
	}
	rawTok, tokHash, err := security.NewOpaqueToken()
	if err != nil {
		return "", time.Time{}, err
	}
	inv, err := s.repo.CreateInvitation(ctx, domain.Invitation{
		TokenHash:    tokHash,
		Email:        normEmail,
		IntendedRole: intendedRole,
		BusinessID:   &businessID,
		BranchID:     branchID,
		IssuedBy:     &issuerAccountID,
		ExpiresAt:    s.now().Add(security.DefaultInvitationTTL),
	})
	if err != nil {
		return "", time.Time{}, err
	}
	if s.emailSender != nil {
		_ = s.emailSender.SendInvitation(ctx, normEmail, string(intendedRole), rawTok)
	}
	return inv.ID, inv.ExpiresAt, nil
}

func (s *Service) RevokeWorkInvitation(ctx context.Context, _ string, businessID, invitationID string) error {
	return s.repo.DeleteUnconsumedInvitation(ctx, invitationID, businessID)
}

func (s *Service) RecordAudit(ctx context.Context, event, outcome string, actorAccountID, subjectAccountID *string, reason string, detail map[string]any) error {
	return s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            event,
		Outcome:          outcome,
		ActorAccountID:   actorAccountID,
		SubjectAccountID: subjectAccountID,
		Reason:           reason,
		Detail:           detail,
	})
}

func (s *Service) auditRefused(ctx context.Context, event string, actorID, subjectID *string, reason string) {
	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            event,
		Outcome:          domain.AuditOutcomeRefused,
		ActorAccountID:   actorID,
		SubjectAccountID: subjectID,
		Reason:           reason,
	})
}
