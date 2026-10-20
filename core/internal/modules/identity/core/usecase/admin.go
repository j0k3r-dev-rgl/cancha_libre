package usecase

import (
	"context"
	"errors"
	"strings"

	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/modules/identity/core/ports"
	"canchalibre/core/internal/platform/actor"
)

// ListUsers returns a paginated list of platform users (ADMIN only).
func (s *Service) ListUsers(ctx context.Context, p actor.Principal, filter ports.UserFilter) ([]domain.UserSummary, int, error) {
	if p.Actor != actor.RoleAdmin {
		s.auditRefused(ctx, "list_users", &p.AccountID, nil, "actor is not ADMIN")
		return nil, 0, domain.ErrForbidden
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.Kind = strings.ToUpper(strings.TrimSpace(filter.Kind))
	if filter.Kind != "" && filter.Kind != string(actor.KindWork) && filter.Kind != string(actor.KindCustomer) {
		s.auditRefused(ctx, "list_users", &p.AccountID, nil, "invalid kind filter")
		return nil, 0, domain.ErrValidation
	}
	filter.Status = strings.ToUpper(strings.TrimSpace(filter.Status))
	if filter.Status != "" && filter.Status != domain.StatusPending && filter.Status != domain.StatusActive && filter.Status != domain.StatusSuspended && filter.Status != domain.StatusClosed {
		s.auditRefused(ctx, "list_users", &p.AccountID, nil, "invalid status filter")
		return nil, 0, domain.ErrValidation
	}

	filter.Role = strings.ToUpper(strings.TrimSpace(filter.Role))
	if filter.Role != "" &&
		filter.Role != string(actor.RoleAdmin) &&
		filter.Role != string(actor.RoleSoporte) &&
		filter.Role != string(actor.RoleOwner) &&
		filter.Role != string(actor.RoleEmployee) &&
		filter.Role != string(actor.RolePlayer) {
		s.auditRefused(ctx, "list_users", &p.AccountID, nil, "invalid role filter")
		return nil, 0, domain.ErrValidation
	}

	if strings.ContainsRune(filter.Search, 0) || strings.ContainsRune(filter.Role, 0) {
		s.auditRefused(ctx, "list_users", &p.AccountID, nil, "filter contains an unsupported NUL character")
		return nil, 0, domain.ErrValidation
	}

	return s.repo.ListUsers(ctx, filter)
}

// GetUser returns full profile and memberships of a user (ADMIN only).
func (s *Service) GetUser(ctx context.Context, p actor.Principal, userID string) (domain.UserDetail, error) {
	if p.Actor != actor.RoleAdmin {
		s.auditRefused(ctx, "get_user", &p.AccountID, nil, "actor is not ADMIN")
		return domain.UserDetail{}, domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(userID)
	if !ok {
		s.auditRefused(ctx, "get_user", &p.AccountID, nil, "invalid userId UUID")
		return domain.UserDetail{}, domain.ErrNotFound
	}

	acc, err := s.repo.GetAccountDetail(ctx, normID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.auditRefused(ctx, "get_user", &p.AccountID, nil, "user not found")
			return domain.UserDetail{}, domain.ErrNotFound
		}
		return domain.UserDetail{}, err
	}

	var memberships []domain.UserMembershipView
	if s.scopes != nil {
		views, err := s.scopes.ListUserMemberships(ctx, normID)
		if err == nil {
			memberships = views
		}
	}
	if memberships == nil {
		memberships = []domain.UserMembershipView{}
	}

	return domain.UserDetail{
		ID:           acc.ID,
		Email:        acc.Email,
		DisplayName:  acc.DisplayName,
		Kind:         acc.Kind,
		PlatformRole: acc.PlatformRole,
		Status:       acc.Status,
		CreatedAt:    acc.CreatedAt,
		Memberships:  memberships,
	}, nil
}

// SuspendUser administratively suspends an account and revokes active sessions (ADMIN only).
func (s *Service) SuspendUser(ctx context.Context, p actor.Principal, targetUserID, reason string) error {
	if p.Actor != actor.RoleAdmin {
		s.auditRefused(ctx, "suspend_user", &p.AccountID, nil, "actor is not ADMIN")
		return domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(targetUserID)
	if !ok {
		s.auditRefused(ctx, "suspend_user", &p.AccountID, nil, "invalid userId UUID")
		return domain.ErrNotFound
	}
	if normID == p.AccountID {
		s.auditRefused(ctx, "suspend_user", &p.AccountID, &normID, "admin cannot suspend their own account")
		return domain.ErrForbidden
	}
	if strings.TrimSpace(reason) == "" || strings.ContainsRune(reason, 0) {
		s.auditRefused(ctx, "suspend_user", &p.AccountID, &normID, "reason is required")
		return domain.ErrValidation
	}

	acc, err := s.repo.FindAccountByID(ctx, normID)
	if err != nil {
		s.auditRefused(ctx, "suspend_user", &p.AccountID, &normID, "account not found")
		return domain.ErrNotFound
	}
	if acc.Status == domain.StatusSuspended {
		s.auditRefused(ctx, "suspend_user", &p.AccountID, &normID, "account already suspended")
		return domain.ErrConflict
	}
	if acc.Status == domain.StatusClosed {
		s.auditRefused(ctx, "suspend_user", &p.AccountID, &normID, "account already closed")
		return domain.ErrConflict
	}

	if err := s.repo.SuspendAccount(ctx, normID); err != nil {
		s.auditRefused(ctx, "suspend_user", &p.AccountID, &normID, err.Error())
		return err
	}

	// Revoke all active refresh sessions immediately so the user cannot refresh their tokens
	_ = s.repo.RevokeRefreshSessionsByAccount(ctx, normID)

	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "suspend_user",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &p.AccountID,
		SubjectAccountID: &normID,
		Reason:           reason,
	})
	return nil
}

// ReactivateUser restores an account from SUSPENDED back to ACTIVE (ADMIN only).
func (s *Service) ReactivateUser(ctx context.Context, p actor.Principal, targetUserID string) error {
	if p.Actor != actor.RoleAdmin {
		s.auditRefused(ctx, "reactivate_user", &p.AccountID, nil, "actor is not ADMIN")
		return domain.ErrForbidden
	}
	normID, ok := actor.NormalizeUUID(targetUserID)
	if !ok {
		s.auditRefused(ctx, "reactivate_user", &p.AccountID, nil, "invalid userId UUID")
		return domain.ErrNotFound
	}

	acc, err := s.repo.FindAccountByID(ctx, normID)
	if err != nil {
		s.auditRefused(ctx, "reactivate_user", &p.AccountID, &normID, "account not found")
		return domain.ErrNotFound
	}
	if acc.Status != domain.StatusSuspended {
		s.auditRefused(ctx, "reactivate_user", &p.AccountID, &normID, "account is not suspended")
		return domain.ErrConflict
	}

	if err := s.repo.ReactivateAccount(ctx, normID); err != nil {
		s.auditRefused(ctx, "reactivate_user", &p.AccountID, &normID, err.Error())
		return err
	}

	_ = s.repo.AppendAudit(ctx, domain.AuditEntry{
		Event:            "reactivate_user",
		Outcome:          domain.AuditOutcomeSuccess,
		ActorAccountID:   &p.AccountID,
		SubjectAccountID: &normID,
		Reason:           "reactivated by admin",
	})
	return nil
}

// GetAccountViewsByIDs returns user summary information for a list of account IDs.
func (s *Service) GetAccountViewsByIDs(ctx context.Context, accountIDs []string) ([]domain.UserSummary, error) {
	views := make([]domain.UserSummary, 0, len(accountIDs))
	for _, id := range accountIDs {
		acc, err := s.repo.FindAccountByID(ctx, id)
		if err == nil {
			views = append(views, domain.UserSummary{
				ID:           acc.ID,
				Email:        acc.Email,
				DisplayName:  acc.DisplayName,
				Kind:         acc.Kind,
				PlatformRole: acc.PlatformRole,
				Status:       acc.Status,
				CreatedAt:    acc.CreatedAt,
			})
		}
	}
	return views, nil
}
