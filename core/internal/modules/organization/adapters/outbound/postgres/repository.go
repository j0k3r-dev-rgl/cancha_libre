package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/platform/actor"
)

// Repository implements organization/core/ports.Repository and authn.ScopeVerifier.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a PostgreSQL repository for schema_organization.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// VerifyScope executes the guard's single indexed lookup (docs/design-identity-ddl.md).
func (r *Repository) VerifyScope(ctx context.Context, accountID, businessID, branchID string) (actor.Role, error) {
	accID, ok1 := actor.NormalizeUUID(accountID)
	busID, ok2 := actor.NormalizeUUID(businessID)
	if !ok1 || !ok2 {
		return "", domain.ErrContextNotAuthorized
	}

	if branchID == "" {
		// Business-level OWNER check (when no branch is specified)
		var roleStr string
		err := r.pool.QueryRow(ctx, `
			SELECT m.role
			FROM schema_organization.membership m
			JOIN schema_organization.business b ON b.id = m.business_id
			WHERE m.account_id = $1::uuid
			  AND m.business_id = $2::uuid
			  AND m.branch_id IS NULL
			  AND m.revoked_at IS NULL
			  AND b.enabled AND b.archived_at IS NULL
			LIMIT 1
		`, accID, busID).Scan(&roleStr)
		if err != nil {
			return "", domain.ErrContextNotAuthorized
		}
		return actor.Role(roleStr), nil
	}

	brID, ok3 := actor.NormalizeUUID(branchID)
	if !ok3 {
		return "", domain.ErrContextNotAuthorized
	}

	var roleStr string
	var matchedBranch *string
	err := r.pool.QueryRow(ctx, `
		SELECT m.role, m.branch_id::text
		FROM schema_organization.membership m
		JOIN schema_organization.business b ON b.id = m.business_id
		JOIN schema_organization.branch   s ON s.id = COALESCE(m.branch_id, $3::uuid) AND s.business_id = b.id
		WHERE m.account_id  = $1::uuid
		  AND m.business_id = $2::uuid
		  AND (m.branch_id IS NULL OR m.branch_id = $3::uuid)
		  AND m.revoked_at IS NULL
		  AND b.enabled AND b.archived_at IS NULL
		  AND s.enabled AND s.archived_at IS NULL
		ORDER BY CASE WHEN m.role = 'OWNER' THEN 0 ELSE 1 END
		LIMIT 1
	`, accID, busID, brID).Scan(&roleStr, &matchedBranch)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrContextNotAuthorized
		}
		return "", err
	}
	return actor.Role(roleStr), nil
}

// HasAnyActiveMembership reports whether the account has at least one active
// membership in an enabled business/branch.
func (r *Repository) HasAnyActiveMembership(ctx context.Context, accountID string) (actor.Role, bool, error) {
	accID, ok := actor.NormalizeUUID(accountID)
	if !ok {
		return "", false, nil
	}
	var roleStr string
	err := r.pool.QueryRow(ctx, `
		SELECT m.role
		FROM schema_organization.membership m
		JOIN schema_organization.business b ON b.id = m.business_id
		LEFT JOIN schema_organization.branch s ON s.id = m.branch_id AND s.business_id = b.id
		WHERE m.account_id = $1::uuid
		  AND m.revoked_at IS NULL
		  AND b.enabled AND b.archived_at IS NULL
		  AND (m.branch_id IS NULL OR (s.enabled AND s.archived_at IS NULL))
		ORDER BY CASE WHEN m.role = 'OWNER' THEN 0 ELSE 1 END
		LIMIT 1
	`, accID).Scan(&roleStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return actor.Role(roleStr), true, nil
}

// ListContextsForAccount returns every enabled (business, branch, role) pair
// covered by the account's active memberships. An OWNER membership expands to
// every enabled branch of that business.
func (r *Repository) ListContextsForAccount(ctx context.Context, accountID string) ([]domain.ContextView, error) {
	accID, ok := actor.NormalizeUUID(accountID)
	if !ok {
		return []domain.ContextView{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (b.id, s.id)
		       b.id::text, b.name, s.id::text, s.name, m.role
		FROM schema_organization.membership m
		JOIN schema_organization.business b ON b.id = m.business_id
		JOIN schema_organization.branch   s ON s.business_id = b.id
		                                   AND (m.branch_id IS NULL OR s.id = m.branch_id)
		WHERE m.account_id = $1::uuid
		  AND m.revoked_at IS NULL
		  AND b.enabled AND b.archived_at IS NULL
		  AND s.enabled AND s.archived_at IS NULL
		ORDER BY b.id, s.id, CASE WHEN m.role = 'OWNER' THEN 0 ELSE 1 END
	`, accID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.ContextView{}
	for rows.Next() {
		var cv domain.ContextView
		var roleStr string
		if err := rows.Scan(&cv.BusinessID, &cv.BusinessName, &cv.BranchID, &cv.BranchName, &roleStr); err != nil {
			return nil, err
		}
		cv.Role = actor.Role(roleStr)
		out = append(out, cv)
	}
	return out, rows.Err()
}

func (r *Repository) BusinessExists(ctx context.Context, businessID string) (bool, error) {
	busID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		return false, nil
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM schema_organization.business
			WHERE id = $1::uuid AND archived_at IS NULL
		)
	`, busID).Scan(&exists)
	return exists, err
}

func (r *Repository) BranchBelongsToBusiness(ctx context.Context, businessID, branchID string) (bool, error) {
	busID, ok1 := actor.NormalizeUUID(businessID)
	brID, ok2 := actor.NormalizeUUID(branchID)
	if !ok1 || !ok2 {
		return false, nil
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM schema_organization.branch
			WHERE id = $1::uuid AND business_id = $2::uuid AND archived_at IS NULL
		)
	`, brID, busID).Scan(&exists)
	return exists, err
}

func (r *Repository) IsActiveCoOwner(ctx context.Context, accountID, businessID string) (bool, error) {
	accID, ok1 := actor.NormalizeUUID(accountID)
	busID, ok2 := actor.NormalizeUUID(businessID)
	if !ok1 || !ok2 {
		return false, nil
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM schema_organization.membership m
			JOIN schema_organization.business b ON b.id = m.business_id
			WHERE m.account_id = $1::uuid
			  AND m.business_id = $2::uuid
			  AND m.role = 'OWNER'
			  AND m.revoked_at IS NULL
			  AND b.enabled AND b.archived_at IS NULL
		)
	`, accID, busID).Scan(&exists)
	return exists, err
}

func (r *Repository) HasActiveMembershipInBusiness(ctx context.Context, accountID, businessID string) (bool, error) {
	accID, ok1 := actor.NormalizeUUID(accountID)
	busID, ok2 := actor.NormalizeUUID(businessID)
	if !ok1 || !ok2 {
		return false, nil
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM schema_organization.membership m
			JOIN schema_organization.business b ON b.id = m.business_id
			WHERE m.account_id = $1::uuid
			  AND m.business_id = $2::uuid
			  AND m.revoked_at IS NULL
			  AND b.enabled AND b.archived_at IS NULL
		)
	`, accID, busID).Scan(&exists)
	return exists, err
}

func (r *Repository) CreateMembership(ctx context.Context, m domain.Membership) (domain.Membership, error) {
	accID, ok1 := actor.NormalizeUUID(m.AccountID)
	busID, ok2 := actor.NormalizeUUID(m.BusinessID)
	if !ok1 || !ok2 {
		return domain.Membership{}, domain.ErrValidation
	}
	var brID *string
	if m.BranchID != nil && *m.BranchID != "" {
		normBr, ok := actor.NormalizeUUID(*m.BranchID)
		if !ok {
			return domain.Membership{}, domain.ErrValidation
		}
		brID = &normBr
	}
	id := m.ID
	if id == "" {
		var err error
		id, err = actor.NewUUIDv7()
		if err != nil {
			return domain.Membership{}, err
		}
	}

	var out domain.Membership
	var roleStr string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO schema_organization.membership (id, account_id, business_id, branch_id, role)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5)
		RETURNING id::text, account_id::text, business_id::text, branch_id::text, role, created_at, revoked_at
	`, id, accID, busID, brID, string(m.Role)).Scan(
		&out.ID, &out.AccountID, &out.BusinessID, &out.BranchID, &roleStr, &out.CreatedAt, &out.RevokedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return domain.Membership{}, domain.ErrConflict
			case "23503":
				return domain.Membership{}, domain.ErrNotFound
			case "23514":
				return domain.Membership{}, domain.ErrValidation
			}
		}
		return domain.Membership{}, err
	}
	out.Role = actor.Role(roleStr)
	return out, nil
}

// RevokeCoOwner revokes an OWNER membership for accountID in businessID,
// enforcing BE-ACC-05 last-co-owner protection under concurrency by locking the
// parent business row before counting active owners.
func (r *Repository) RevokeCoOwner(ctx context.Context, businessID, accountID string) error {
	busID, ok1 := actor.NormalizeUUID(businessID)
	accID, ok2 := actor.NormalizeUUID(accountID)
	if !ok1 || !ok2 {
		return domain.ErrNotFound
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize co-owner mutations on this business
	var lockedBusID string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM schema_organization.business
		WHERE id = $1::uuid AND archived_at IS NULL
		FOR UPDATE
	`, busID).Scan(&lockedBusID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}

	// Check if an active OWNER row exists for this account
	var activeMemID string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM schema_organization.membership
		WHERE business_id = $1::uuid AND account_id = $2::uuid AND role = 'OWNER' AND revoked_at IS NULL
		FOR UPDATE
	`, busID, accID).Scan(&activeMemID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Check whether an already-revoked OWNER row exists (idempotent repeat)
		var hadRevoked bool
		if errRev := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM schema_organization.membership
				WHERE business_id = $1::uuid AND account_id = $2::uuid AND role = 'OWNER' AND revoked_at IS NOT NULL
			)
		`, busID, accID).Scan(&hadRevoked); errRev == nil && hadRevoked {
			return nil
		}
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}

	var activeOwners int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM schema_organization.membership
		WHERE business_id = $1::uuid AND role = 'OWNER' AND revoked_at IS NULL
	`, busID).Scan(&activeOwners); err != nil {
		return err
	}
	if activeOwners <= 1 {
		return domain.ErrLastCoOwner
	}

	if _, err := tx.Exec(ctx, `
		UPDATE schema_organization.membership
		   SET revoked_at = now()
		 WHERE id = $1::uuid
	`, activeMemID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) ListMemberships(ctx context.Context, businessID string) ([]domain.Membership, error) {
	busID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		return nil, domain.ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, account_id::text, business_id::text, branch_id::text, role, created_at
		FROM schema_organization.membership
		WHERE business_id = $1::uuid AND revoked_at IS NULL
		ORDER BY created_at ASC, id ASC
	`, busID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Membership{}
	for rows.Next() {
		var m domain.Membership
		var roleStr string
		if err := rows.Scan(&m.ID, &m.AccountID, &m.BusinessID, &m.BranchID, &roleStr, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Role = actor.Role(roleStr)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *Repository) RevokeMembershipByID(ctx context.Context, businessID, membershipID string) error {
	busID, ok1 := actor.NormalizeUUID(businessID)
	memID, ok2 := actor.NormalizeUUID(membershipID)
	if !ok1 || !ok2 {
		return domain.ErrNotFound
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock business row to serialize last-co-owner checks
	var lockedBusID string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM schema_organization.business
		WHERE id = $1::uuid AND archived_at IS NULL
		FOR UPDATE
	`, busID).Scan(&lockedBusID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}

	var roleStr string
	var revokedAt *any
	var isRevoked bool
	err = tx.QueryRow(ctx, `
		SELECT role, (revoked_at IS NOT NULL)
		FROM schema_organization.membership
		WHERE id = $1::uuid AND business_id = $2::uuid
		FOR UPDATE
	`, memID, busID).Scan(&roleStr, &isRevoked)
	_ = revokedAt
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if isRevoked {
		// Idempotent: revoking an already-revoked membership succeeds (204)
		return nil
	}

	if actor.Role(roleStr) == actor.RoleOwner {
		var activeOwners int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM schema_organization.membership
			WHERE business_id = $1::uuid AND role = 'OWNER' AND revoked_at IS NULL
		`, busID).Scan(&activeOwners); err != nil {
			return err
		}
		if activeOwners <= 1 {
			return domain.ErrLastCoOwner
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE schema_organization.membership
		   SET revoked_at = now()
		 WHERE id = $1::uuid
	`, memID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
