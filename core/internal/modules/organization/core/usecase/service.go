package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/modules/organization/core/ports"
	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/security"
)

// Service implements the organization bounded-context use cases for S1 (the
// business/branch/membership spine).
type Service struct {
	repo     ports.Repository
	identity ports.IdentityPort
}

// NewService constructs the organization Service.
func NewService(repo ports.Repository, identity ports.IdentityPort) *Service {
	return &Service{
		repo:     repo,
		identity: identity,
	}
}

// AssignCoOwnerInput holds the request payload for POST /api/v1/admin/businesses/{businessId}/co-owners.
type AssignCoOwnerInput struct {
	AccountID    string
	Email        string
	IntendedRole string
}

// AssignCoOwner assigns a business co-owner (actor ADMIN).
func (s *Service) AssignCoOwner(ctx context.Context, p actor.Principal, businessID string, in AssignCoOwnerInput) (domain.Membership, error) {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.Membership{}, domain.ErrForbidden
	}
	normBusID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, nil, "invalid businessId UUID", nil)
		return domain.Membership{}, domain.ErrValidation
	}

	rawAccID := strings.TrimSpace(in.AccountID)
	rawEmail := strings.TrimSpace(in.Email)
	rawRole := strings.ToUpper(strings.TrimSpace(in.IntendedRole))

	// Must supply either {account_id} OR {email, intended_role: OWNER}, never both or neither
	if (rawAccID == "" && rawEmail == "") || (rawAccID != "" && rawEmail != "") {
		s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, nil, "must provide exactly one of account_id or email", nil)
		return domain.Membership{}, domain.ErrValidation
	}
	if rawEmail != "" && rawRole != string(actor.RoleOwner) {
		s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, nil, "email assignment requires intended_role=OWNER", nil)
		return domain.Membership{}, domain.ErrValidation
	}
	if rawAccID != "" && rawRole != "" && rawRole != string(actor.RoleOwner) {
		s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, nil, "invalid intended_role", nil)
		return domain.Membership{}, domain.ErrValidation
	}

	var targetAccountID string
	if rawAccID != "" {
		normAccID, ok := actor.NormalizeUUID(rawAccID)
		if !ok {
			s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, nil, "invalid account_id UUID", nil)
			return domain.Membership{}, domain.ErrValidation
		}
		targetAccountID = normAccID
	} else {
		if _, err := security.NormalizeEmail(rawEmail); err != nil {
			s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, nil, "invalid email", nil)
			return domain.Membership{}, domain.ErrValidation
		}
	}

	busExists, err := s.repo.BusinessExists(ctx, normBusID)
	if err != nil {
		return domain.Membership{}, err
	}
	if !busExists {
		s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, nil, "unknown business", nil)
		return domain.Membership{}, domain.ErrNotFound
	}

	if rawAccID != "" {
		if err := s.identity.VerifyWorkAccountReadyForCoOwner(ctx, targetAccountID); err != nil {
			s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, &targetAccountID, err.Error(), nil)
			if errors.Is(err, domain.ErrConflict) || strings.Contains(err.Error(), "conflict") {
				return domain.Membership{}, domain.ErrConflict
			}
			return domain.Membership{}, domain.ErrValidation
		}
	} else {
		accID, err := s.identity.EnsureCoOwnerAccountByEmail(ctx, p.AccountID, rawEmail, normBusID)
		if err != nil {
			s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, nil, err.Error(), nil)
			if strings.Contains(err.Error(), "conflict") {
				return domain.Membership{}, domain.ErrConflict
			}
			return domain.Membership{}, domain.ErrValidation
		}
		targetAccountID = accID
	}

	m, err := s.repo.CreateMembership(ctx, domain.Membership{
		AccountID:  targetAccountID,
		BusinessID: normBusID,
		Role:       actor.RoleOwner,
	})
	if err != nil {
		s.audit(ctx, "assign_co_owner", "REFUSED", &p.AccountID, &targetAccountID, err.Error(), nil)
		return domain.Membership{}, err
	}

	s.audit(ctx, "assign_co_owner", "SUCCESS", &p.AccountID, &targetAccountID, "co-owner assigned", map[string]any{
		"membership_id": m.ID,
		"business_id":   normBusID,
	})
	return m, nil
}

// RevokeCoOwner revokes a business co-owner (actor ADMIN), refusing last-co-owner revocation.
func (s *Service) RevokeCoOwner(ctx context.Context, p actor.Principal, businessID, accountID string) error {
	if p.Actor != actor.RoleAdmin {
		s.audit(ctx, "revoke_co_owner", "REFUSED", &p.AccountID, nil, "actor is not ADMIN", nil)
		return domain.ErrForbidden
	}
	normBusID, ok1 := actor.NormalizeUUID(businessID)
	normAccID, ok2 := actor.NormalizeUUID(accountID)
	if !ok1 || !ok2 {
		s.audit(ctx, "revoke_co_owner", "REFUSED", &p.AccountID, nil, "invalid UUID parameter", nil)
		return domain.ErrNotFound
	}

	if err := s.repo.RevokeCoOwner(ctx, normBusID, normAccID); err != nil {
		s.audit(ctx, "revoke_co_owner", "REFUSED", &p.AccountID, &normAccID, err.Error(), nil)
		return err
	}

	s.audit(ctx, "revoke_co_owner", "SUCCESS", &p.AccountID, &normAccID, "co-owner revoked", map[string]any{
		"business_id": normBusID,
	})
	return nil
}

// ListMemberships lists active memberships of a business (actor OWNER).
func (s *Service) ListMemberships(ctx context.Context, p actor.Principal, businessID string) ([]domain.Membership, error) {
	normBusID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		s.audit(ctx, "list_memberships", "REFUSED", &p.AccountID, nil, "invalid businessId UUID", nil)
		return nil, domain.ErrNotFound
	}

	if p.Actor != actor.RoleOwner {
		s.audit(ctx, "list_memberships", "REFUSED", &p.AccountID, nil, "actor is not OWNER", nil)
		return nil, domain.ErrForbidden
	}

	busExists, err := s.repo.BusinessExists(ctx, normBusID)
	if err != nil {
		return nil, err
	}
	if !busExists {
		s.audit(ctx, "list_memberships", "REFUSED", &p.AccountID, nil, "unknown business", nil)
		return nil, domain.ErrNotFound
	}

	isOwner, err := s.repo.IsActiveCoOwner(ctx, p.AccountID, normBusID)
	if err != nil {
		return nil, err
	}
	if !isOwner {
		s.audit(ctx, "list_memberships", "REFUSED", &p.AccountID, nil, "no active co-owner membership in business", nil)
		return nil, domain.ErrForbidden
	}
	if p.Context != nil && p.Context.BusinessID != "" && p.Context.BusinessID != normBusID {
		s.audit(ctx, "list_memberships", "REFUSED", &p.AccountID, nil, "business outside active token context", nil)
		return nil, domain.ErrNotFound
	}

	return s.repo.ListMemberships(ctx, normBusID)
}

// InviteMemberInput holds the payload for POST /api/v1/businesses/{businessId}/invitations.
type InviteMemberInput struct {
	Email        string
	IntendedRole string
	BranchID     *string
}

// InviteMemberOutput holds the created invitation metadata.
type InviteMemberOutput struct {
	InvitationID string
	ExpiresAt    time.Time
}

// InviteMember issues a work invitation for OWNER or EMPLOYEE (actor OWNER).
func (s *Service) InviteMember(ctx context.Context, p actor.Principal, businessID string, in InviteMemberInput) (InviteMemberOutput, error) {
	if p.Actor != actor.RoleOwner {
		s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "actor is not OWNER", nil)
		return InviteMemberOutput{}, domain.ErrForbidden
	}
	normBusID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "invalid businessId UUID", nil)
		return InviteMemberOutput{}, domain.ErrValidation
	}

	normEmail, err := security.NormalizeEmail(in.Email)
	if err != nil {
		s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "invalid email", nil)
		return InviteMemberOutput{}, domain.ErrValidation
	}

	roleStr := strings.ToUpper(strings.TrimSpace(in.IntendedRole))
	if roleStr != string(actor.RoleOwner) && roleStr != string(actor.RoleEmployee) {
		s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "intended_role must be OWNER or EMPLOYEE", nil)
		return InviteMemberOutput{}, domain.ErrValidation
	}
	intendedRole := actor.Role(roleStr)

	var normBrID *string
	if in.BranchID != nil && strings.TrimSpace(*in.BranchID) != "" {
		b, ok := actor.NormalizeUUID(*in.BranchID)
		if !ok {
			s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "invalid branch_id UUID", nil)
			return InviteMemberOutput{}, domain.ErrValidation
		}
		normBrID = &b
	}

	// Owning shape: OWNER takes no branch_id; EMPLOYEE requires one
	if intendedRole == actor.RoleOwner && normBrID != nil {
		s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "OWNER invitation takes no branch_id", nil)
		return InviteMemberOutput{}, domain.ErrValidation
	}
	if intendedRole == actor.RoleEmployee && normBrID == nil {
		s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "EMPLOYEE invitation requires branch_id", nil)
		return InviteMemberOutput{}, domain.ErrValidation
	}

	busExists, err := s.repo.BusinessExists(ctx, normBusID)
	if err != nil {
		return InviteMemberOutput{}, err
	}
	if !busExists {
		s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "unknown business", nil)
		return InviteMemberOutput{}, domain.ErrNotFound
	}

	isOwner, err := s.repo.IsActiveCoOwner(ctx, p.AccountID, normBusID)
	if err != nil {
		return InviteMemberOutput{}, err
	}
	if !isOwner {
		s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "actor is not co-owner of this business", nil)
		return InviteMemberOutput{}, domain.ErrForbidden
	}

	if normBrID != nil {
		belongs, err := s.repo.BranchBelongsToBusiness(ctx, normBusID, *normBrID)
		if err != nil {
			return InviteMemberOutput{}, err
		}
		if !belongs {
			s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, "branch outside business scope", nil)
			return InviteMemberOutput{}, domain.ErrNotFound
		}
	}

	accExists, existingAccID, platformRole, err := s.identity.LookupAccountKindAndRoleByEmail(ctx, normEmail)
	if err != nil {
		return InviteMemberOutput{}, err
	}
	if accExists {
		if platformRole != nil {
			s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, &existingAccID, "target email belongs to platform actor", nil)
			return InviteMemberOutput{}, domain.ErrConflict
		}
		if intendedRole == actor.RoleOwner {
			alreadyOwner, err := s.repo.IsActiveCoOwner(ctx, existingAccID, normBusID)
			if err != nil {
				return InviteMemberOutput{}, err
			}
			if alreadyOwner {
				s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, &existingAccID, "already an active co-owner", nil)
				return InviteMemberOutput{}, domain.ErrConflict
			}
		} else if normBrID != nil {
			roleInBr, err := s.repo.VerifyScope(ctx, existingAccID, normBusID, *normBrID)
			if err == nil && (roleInBr == actor.RoleEmployee || roleInBr == actor.RoleOwner) {
				s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, &existingAccID, "already has active membership in branch", nil)
				return InviteMemberOutput{}, domain.ErrConflict
			}
		}
	}

	invID, exp, err := s.identity.CreateWorkInvitation(ctx, p.AccountID, normEmail, intendedRole, normBusID, normBrID)
	if err != nil {
		s.audit(ctx, "invite_member", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		if strings.Contains(err.Error(), "conflict") {
			return InviteMemberOutput{}, domain.ErrConflict
		}
		return InviteMemberOutput{}, err
	}

	s.audit(ctx, "invite_member", "SUCCESS", &p.AccountID, nil, "member invitation issued", map[string]any{
		"invitation_id": invID,
		"business_id":   normBusID,
		"intended_role": string(intendedRole),
	})
	return InviteMemberOutput{
		InvitationID: invID,
		ExpiresAt:    exp,
	}, nil
}

// RevokeMembership revokes a membership in a business (actor OWNER, idempotent).
func (s *Service) RevokeMembership(ctx context.Context, p actor.Principal, businessID, membershipID string) error {
	if p.Actor != actor.RoleOwner {
		s.audit(ctx, "revoke_membership", "REFUSED", &p.AccountID, nil, "actor is not OWNER", nil)
		return domain.ErrForbidden
	}
	normBusID, ok1 := actor.NormalizeUUID(businessID)
	normMemID, ok2 := actor.NormalizeUUID(membershipID)
	if !ok1 || !ok2 {
		s.audit(ctx, "revoke_membership", "REFUSED", &p.AccountID, nil, "invalid UUID parameter", nil)
		return domain.ErrNotFound
	}

	busExists, err := s.repo.BusinessExists(ctx, normBusID)
	if err != nil {
		return err
	}
	if !busExists {
		s.audit(ctx, "revoke_membership", "REFUSED", &p.AccountID, nil, "unknown business", nil)
		return domain.ErrNotFound
	}

	isOwner, err := s.repo.IsActiveCoOwner(ctx, p.AccountID, normBusID)
	if err != nil {
		return err
	}
	if !isOwner {
		s.audit(ctx, "revoke_membership", "REFUSED", &p.AccountID, nil, "actor is not co-owner of this business", nil)
		return domain.ErrForbidden
	}

	if err := s.repo.RevokeMembershipByID(ctx, normBusID, normMemID); err != nil {
		s.audit(ctx, "revoke_membership", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return err
	}

	s.audit(ctx, "revoke_membership", "SUCCESS", &p.AccountID, nil, "membership revoked", map[string]any{
		"business_id":   normBusID,
		"membership_id": normMemID,
	})
	return nil
}

// RevokeInvitation revokes an invitation in a business (actor OWNER, idempotent).
func (s *Service) RevokeInvitation(ctx context.Context, p actor.Principal, businessID, invitationID string) error {
	if p.Actor != actor.RoleOwner {
		s.audit(ctx, "revoke_invitation", "REFUSED", &p.AccountID, nil, "actor is not OWNER", nil)
		return domain.ErrForbidden
	}
	normBusID, ok1 := actor.NormalizeUUID(businessID)
	normInvID, ok2 := actor.NormalizeUUID(invitationID)
	if !ok1 || !ok2 {
		s.audit(ctx, "revoke_invitation", "REFUSED", &p.AccountID, nil, "invalid UUID parameter", nil)
		return domain.ErrNotFound
	}

	busExists, err := s.repo.BusinessExists(ctx, normBusID)
	if err != nil {
		return err
	}
	if !busExists {
		s.audit(ctx, "revoke_invitation", "REFUSED", &p.AccountID, nil, "unknown business", nil)
		return domain.ErrNotFound
	}

	isOwner, err := s.repo.IsActiveCoOwner(ctx, p.AccountID, normBusID)
	if err != nil {
		return err
	}
	if !isOwner {
		s.audit(ctx, "revoke_invitation", "REFUSED", &p.AccountID, nil, "actor is not co-owner of this business", nil)
		return domain.ErrForbidden
	}

	if err := s.identity.RevokeWorkInvitation(ctx, p.AccountID, normBusID, normInvID); err != nil {
		s.audit(ctx, "revoke_invitation", "REFUSED", &p.AccountID, nil, err.Error(), nil)
		return domain.ErrNotFound
	}

	s.audit(ctx, "revoke_invitation", "SUCCESS", &p.AccountID, nil, "invitation revoked", map[string]any{
		"business_id":   normBusID,
		"invitation_id": normInvID,
	})
	return nil
}

func (s *Service) audit(ctx context.Context, event, outcome string, actorID, subjectID *string, reason string, detail map[string]any) {
	if s.identity != nil {
		_ = s.identity.RecordAudit(ctx, event, outcome, actorID, subjectID, reason, detail)
	}
}
