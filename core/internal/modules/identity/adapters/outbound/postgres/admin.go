package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/modules/identity/core/ports"
	"canchalibre/core/internal/platform/actor"
)

// ListUsers queries schema_identity.account with filters, search, and pagination.
func (r *Repository) ListUsers(ctx context.Context, filter ports.UserFilter) ([]domain.UserSummary, int, error) {
	var conditions []string
	var args []any
	argIdx := 1

	if filter.Kind != "" {
		conditions = append(conditions, fmt.Sprintf("a.kind = $%d", argIdx))
		args = append(args, filter.Kind)
		argIdx++
	}

	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("a.status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}

	if filter.Role != "" {
		roleUpper := strings.ToUpper(filter.Role)
		if roleUpper == string(actor.RoleAdmin) || roleUpper == string(actor.RoleSoporte) {
			conditions = append(conditions, fmt.Sprintf("a.platform_role = $%d", argIdx))
			args = append(args, roleUpper)
			argIdx++
		}
	}

	if strings.TrimSpace(filter.Search) != "" {
		searchPattern := "%" + strings.ToLower(strings.TrimSpace(filter.Search)) + "%"
		conditions = append(conditions, fmt.Sprintf("(lower(a.email) LIKE $%d OR lower(coalesce(a.display_name, '')) LIKE $%d)", argIdx, argIdx))
		args = append(args, searchPattern)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT count(*) FROM schema_identity.account a %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	dataQuery := fmt.Sprintf(`
		SELECT a.id, a.email, a.display_name, a.kind, a.platform_role, a.status, a.created_at,
		       (SELECT count(*) FROM schema_organization.membership m WHERE m.account_id = a.id AND m.revoked_at IS NULL) as memberships_count
		FROM schema_identity.account a
		%s
		ORDER BY a.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	queryArgs := append(args, limit, offset)
	rows, err := r.pool.Query(ctx, dataQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users query: %w", err)
	}
	defer rows.Close()

	var items []domain.UserSummary
	for rows.Next() {
		var u domain.UserSummary
		var kindStr string
		var platformRole *string
		if err := rows.Scan(
			&u.ID,
			&u.Email,
			&u.DisplayName,
			&kindStr,
			&platformRole,
			&u.Status,
			&u.CreatedAt,
			&u.MembershipsCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan user summary: %w", err)
		}
		u.Kind = actor.AccountKind(kindStr)
		if platformRole != nil {
			r := actor.Role(*platformRole)
			u.PlatformRole = &r
		}
		items = append(items, u)
	}
	if items == nil {
		items = []domain.UserSummary{}
	}
	return items, total, nil
}

// GetAccountDetail retrieves full account details by ID.
func (r *Repository) GetAccountDetail(ctx context.Context, accountID string) (domain.Account, error) {
	accID, ok := actor.NormalizeUUID(accountID)
	if !ok {
		return domain.Account{}, domain.ErrNotFound
	}
	row := r.pool.QueryRow(ctx, `
		SELECT id, email, kind, platform_role, status, display_name, created_at, updated_at, closed_at
		FROM schema_identity.account
		WHERE id = $1::uuid
	`, accID)
	return scanAccount(row)
}

// SuspendAccount updates account status to SUSPENDED.
func (r *Repository) SuspendAccount(ctx context.Context, accountID string) error {
	accID, ok := actor.NormalizeUUID(accountID)
	if !ok {
		return domain.ErrNotFound
	}
	cmdTag, err := r.pool.Exec(ctx, `
		UPDATE schema_identity.account
		SET status = $2, updated_at = $3
		WHERE id = $1::uuid AND status <> $2 AND status <> $4
	`, accID, domain.StatusSuspended, time.Now().UTC(), domain.StatusClosed)
	if err != nil {
		return fmt.Errorf("suspend account: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		var currentStatus string
		err := r.pool.QueryRow(ctx, "SELECT status FROM schema_identity.account WHERE id = $1::uuid", accID).Scan(&currentStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if currentStatus == domain.StatusSuspended || currentStatus == domain.StatusClosed {
			return domain.ErrConflict
		}
		return domain.ErrNotFound
	}
	return nil
}

// ReactivateAccount restores account status from SUSPENDED back to ACTIVE.
func (r *Repository) ReactivateAccount(ctx context.Context, accountID string) error {
	accID, ok := actor.NormalizeUUID(accountID)
	if !ok {
		return domain.ErrNotFound
	}
	cmdTag, err := r.pool.Exec(ctx, `
		UPDATE schema_identity.account
		SET status = $2, updated_at = $3
		WHERE id = $1::uuid AND status = $4
	`, accID, domain.StatusActive, time.Now().UTC(), domain.StatusSuspended)
	if err != nil {
		return fmt.Errorf("reactivate account: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		var currentStatus string
		err := r.pool.QueryRow(ctx, "SELECT status FROM schema_identity.account WHERE id = $1::uuid", accID).Scan(&currentStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if currentStatus != domain.StatusSuspended {
			return domain.ErrConflict
		}
		return domain.ErrNotFound
	}
	return nil
}
