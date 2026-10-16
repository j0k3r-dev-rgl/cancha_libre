package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/platform/actor"
	"canchalibre/core/internal/platform/authn"
	"canchalibre/core/internal/platform/security"
)

// Repository implements identity/core/ports.Repository and authn.AccountVerifier.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a PostgreSQL repository for schema_identity.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetAccountState satisfies authn.AccountVerifier.
func (r *Repository) GetAccountState(ctx context.Context, accountID string) (authn.AccountState, error) {
	acc, err := r.FindAccountByID(ctx, accountID)
	if err != nil {
		return authn.AccountState{}, err
	}
	return authn.AccountState{
		AccountID:    acc.ID,
		Email:        acc.Email,
		Kind:         acc.Kind,
		PlatformRole: acc.PlatformRole,
		Status:       acc.Status,
	}, nil
}

func scanAccount(row pgx.Row) (domain.Account, error) {
	var acc domain.Account
	var kindStr string
	var platformRole *string
	err := row.Scan(
		&acc.ID,
		&acc.Email,
		&kindStr,
		&platformRole,
		&acc.Status,
		&acc.DisplayName,
		&acc.CreatedAt,
		&acc.UpdatedAt,
		&acc.ClosedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Account{}, domain.ErrNotFound
		}
		return domain.Account{}, err
	}
	acc.Kind = actor.AccountKind(kindStr)
	if platformRole != nil {
		r := actor.Role(*platformRole)
		acc.PlatformRole = &r
	}
	return acc, nil
}

func (r *Repository) FindAccountByID(ctx context.Context, accountID string) (domain.Account, error) {
	id, ok := actor.NormalizeUUID(accountID)
	if !ok {
		return domain.Account{}, domain.ErrNotFound
	}
	row := r.pool.QueryRow(ctx, `
		SELECT id::text, email, kind, platform_role, status, display_name, created_at, updated_at, closed_at
		FROM schema_identity.account
		WHERE id = $1::uuid
	`, id)
	return scanAccount(row)
}

func (r *Repository) FindAccountByEmailAndKind(ctx context.Context, email string, kind actor.AccountKind) (domain.Account, string, error) {
	normEmail, err := security.NormalizeEmail(email)
	if err != nil {
		return domain.Account{}, "", domain.ErrNotFound
	}
	var acc domain.Account
	var kindStr string
	var platformRole *string
	var pwHash *string
	err = r.pool.QueryRow(ctx, `
		SELECT a.id::text, a.email, a.kind, a.platform_role, a.status, a.display_name,
		       a.created_at, a.updated_at, a.closed_at, c.password_hash
		FROM schema_identity.account a
		LEFT JOIN schema_identity.credential c ON c.account_id = a.id
		WHERE lower(a.email) = lower($1) AND a.kind = $2
	`, normEmail, string(kind)).Scan(
		&acc.ID, &acc.Email, &kindStr, &platformRole, &acc.Status, &acc.DisplayName,
		&acc.CreatedAt, &acc.UpdatedAt, &acc.ClosedAt, &pwHash,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Account{}, "", domain.ErrNotFound
		}
		return domain.Account{}, "", err
	}
	acc.Kind = actor.AccountKind(kindStr)
	if platformRole != nil {
		role := actor.Role(*platformRole)
		acc.PlatformRole = &role
	}
	var hash string
	if pwHash != nil {
		hash = *pwHash
	}
	return acc, hash, nil
}

func (r *Repository) FindAccountByEmailAnyKind(ctx context.Context, email string) ([]domain.Account, map[string]string, error) {
	normEmail, err := security.NormalizeEmail(email)
	if err != nil {
		return nil, nil, domain.ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT a.id::text, a.email, a.kind, a.platform_role, a.status, a.display_name,
		       a.created_at, a.updated_at, a.closed_at, c.password_hash
		FROM schema_identity.account a
		LEFT JOIN schema_identity.credential c ON c.account_id = a.id
		WHERE lower(a.email) = lower($1)
		ORDER BY CASE WHEN a.kind = 'WORK' THEN 0 ELSE 1 END
	`, normEmail)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var accounts []domain.Account
	hashes := map[string]string{}
	for rows.Next() {
		var acc domain.Account
		var kindStr string
		var platformRole *string
		var pwHash *string
		if err := rows.Scan(
			&acc.ID, &acc.Email, &kindStr, &platformRole, &acc.Status, &acc.DisplayName,
			&acc.CreatedAt, &acc.UpdatedAt, &acc.ClosedAt, &pwHash,
		); err != nil {
			return nil, nil, err
		}
		acc.Kind = actor.AccountKind(kindStr)
		if platformRole != nil {
			role := actor.Role(*platformRole)
			acc.PlatformRole = &role
		}
		if pwHash != nil {
			hashes[acc.ID] = *pwHash
		}
		accounts = append(accounts, acc)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(accounts) == 0 {
		return nil, nil, domain.ErrNotFound
	}
	return accounts, hashes, nil
}

func (r *Repository) CreateAccountWithCredential(ctx context.Context, acc domain.Account, passwordHash string) (domain.Account, error) {
	normEmail, err := security.NormalizeEmail(acc.Email)
	if err != nil {
		return domain.Account{}, domain.ErrValidation
	}
	id := acc.ID
	if id == "" {
		id, err = actor.NewUUIDv7()
		if err != nil {
			return domain.Account{}, err
		}
	}
	status := acc.Status
	if status == "" {
		status = domain.StatusActive
	}
	var platformRole *string
	if acc.PlatformRole != nil {
		s := string(*acc.PlatformRole)
		platformRole = &s
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Account{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
		INSERT INTO schema_identity.account (id, email, kind, platform_role, status, display_name)
		VALUES ($1::uuid, $2, $3, $4, $5, $6)
		RETURNING id::text, email, kind, platform_role, status, display_name, created_at, updated_at, closed_at
	`, id, normEmail, string(acc.Kind), platformRole, status, acc.DisplayName)

	created, err := scanAccount(row)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Account{}, domain.ErrConflict
		}
		return domain.Account{}, err
	}

	if passwordHash != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO schema_identity.credential (account_id, password_hash)
			VALUES ($1::uuid, $2)
		`, created.ID, passwordHash); err != nil {
			return domain.Account{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Account{}, err
	}
	return created, nil
}

func (r *Repository) EnsurePendingWorkAccount(ctx context.Context, email string, platformRole *actor.Role) (domain.Account, error) {
	normEmail, err := security.NormalizeEmail(email)
	if err != nil {
		return domain.Account{}, domain.ErrValidation
	}
	existing, _, err := r.FindAccountByEmailAndKind(ctx, normEmail, actor.KindWork)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Account{}, err
	}

	id, err := actor.NewUUIDv7()
	if err != nil {
		return domain.Account{}, err
	}
	var pr *string
	if platformRole != nil {
		s := string(*platformRole)
		pr = &s
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO schema_identity.account (id, email, kind, platform_role, status, display_name)
		VALUES ($1::uuid, $2, 'WORK', $3, 'PENDING', $2)
		RETURNING id::text, email, kind, platform_role, status, display_name, created_at, updated_at, closed_at
	`, id, normEmail, pr)
	created, err := scanAccount(row)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Account{}, domain.ErrConflict
		}
		return domain.Account{}, err
	}
	return created, nil
}

func (r *Repository) CreateRefreshSession(ctx context.Context, accountID string, tokenHash []byte, expiresAt time.Time) (domain.RefreshSession, error) {
	id, err := actor.NewUUIDv7()
	if err != nil {
		return domain.RefreshSession{}, err
	}
	familyID, err := actor.NewUUIDv7()
	if err != nil {
		return domain.RefreshSession{}, err
	}
	var s domain.RefreshSession
	err = r.pool.QueryRow(ctx, `
		INSERT INTO schema_identity.refresh_session (id, account_id, family_id, token_hash, expires_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5)
		RETURNING id::text, account_id::text, family_id::text, token_hash, created_at, expires_at
	`, id, accountID, familyID, tokenHash, expiresAt.UTC()).Scan(
		&s.ID, &s.AccountID, &s.FamilyID, &s.TokenHash, &s.CreatedAt, &s.ExpiresAt,
	)
	if err != nil {
		return domain.RefreshSession{}, err
	}
	return s, nil
}

func (r *Repository) RotateRefreshSession(ctx context.Context, currentHash, nextHash []byte, nextExpiresAt time.Time) (domain.RefreshSession, error) {
	newID, err := actor.NewUUIDv7()
	if err != nil {
		return domain.RefreshSession{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.RefreshSession{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Atomic single-winner exchange from docs/design-identity-ddl.md
	var s domain.RefreshSession
	err = tx.QueryRow(ctx, `
		WITH rotated AS (
			UPDATE schema_identity.refresh_session rs
			   SET rotated_at = now(), replaced_by = $2::uuid
			 WHERE rs.token_hash = $1
			   AND rs.revoked_at IS NULL
			   AND rs.rotated_at IS NULL
			   AND rs.expires_at > now()
			RETURNING rs.id, rs.account_id, rs.family_id
		)
		INSERT INTO schema_identity.refresh_session (id, account_id, family_id, token_hash, expires_at)
		SELECT $2::uuid, r.account_id, r.family_id, $3, $4
		FROM rotated r
		RETURNING id::text, account_id::text, family_id::text, token_hash, created_at, expires_at
	`, currentHash, newID, nextHash, nextExpiresAt.UTC()).Scan(
		&s.ID, &s.AccountID, &s.FamilyID, &s.TokenHash, &s.CreatedAt, &s.ExpiresAt,
	)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return domain.RefreshSession{}, err
		}
		return s, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.RefreshSession{}, err
	}

	// No row rotated: check whether the token exists with rotated_at IS NOT NULL (reuse!)
	var familyID string
	var rotatedAt *time.Time
	var revokedAt *time.Time
	errLookup := tx.QueryRow(ctx, `
		SELECT family_id::text, rotated_at, revoked_at
		FROM schema_identity.refresh_session
		WHERE token_hash = $1
	`, currentHash).Scan(&familyID, &rotatedAt, &revokedAt)
	if errLookup != nil {
		return domain.RefreshSession{}, domain.ErrUnauthenticated
	}
	if rotatedAt != nil {
		// D6: reuse of a rotated token revokes the entire session family immediately
		if _, err := tx.Exec(ctx, `
			UPDATE schema_identity.refresh_session
			   SET revoked_at = now()
			 WHERE family_id = $1::uuid AND revoked_at IS NULL
		`, familyID); err != nil {
			return domain.RefreshSession{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.RefreshSession{}, err
		}
		return domain.RefreshSession{}, domain.ErrRefreshReuse
	}
	return domain.RefreshSession{}, domain.ErrUnauthenticated
}

func (r *Repository) RevokeRefreshSessionByHash(ctx context.Context, tokenHash []byte) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE schema_identity.refresh_session
		   SET revoked_at = COALESCE(revoked_at, now())
		 WHERE token_hash = $1
	`, tokenHash)
	return err
}

func (r *Repository) RevokeRefreshSessionsByAccount(ctx context.Context, accountID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE schema_identity.refresh_session
		   SET revoked_at = COALESCE(revoked_at, now())
		 WHERE account_id = $1::uuid AND revoked_at IS NULL
	`, accountID)
	return err
}

func (r *Repository) CreateInvitation(ctx context.Context, inv domain.Invitation) (domain.Invitation, error) {
	normEmail, err := security.NormalizeEmail(inv.Email)
	if err != nil {
		return domain.Invitation{}, domain.ErrValidation
	}
	id := inv.ID
	if id == "" {
		id, err = actor.NewUUIDv7()
		if err != nil {
			return domain.Invitation{}, err
		}
	}
	var out domain.Invitation
	var roleStr string
	err = r.pool.QueryRow(ctx, `
		INSERT INTO schema_identity.invitation
			(id, token_hash, email, intended_role, business_id, branch_id, issued_by, expires_at)
		VALUES ($1::uuid, $2, $3, $4, $5::uuid, $6::uuid, $7::uuid, $8)
		RETURNING id::text, token_hash, email, intended_role,
		          business_id::text, branch_id::text, issued_by::text,
		          created_at, expires_at, consumed_at, consumed_by::text
	`, id, inv.TokenHash, normEmail, string(inv.IntendedRole), inv.BusinessID, inv.BranchID, inv.IssuedBy, inv.ExpiresAt.UTC()).Scan(
		&out.ID, &out.TokenHash, &out.Email, &roleStr,
		&out.BusinessID, &out.BranchID, &out.IssuedBy,
		&out.CreatedAt, &out.ExpiresAt, &out.ConsumedAt, &out.ConsumedBy,
	)
	if err != nil {
		if isUniqueViolation(err) || isCheckViolation(err) {
			return domain.Invitation{}, domain.ErrValidation
		}
		return domain.Invitation{}, err
	}
	out.IntendedRole = actor.Role(roleStr)
	return out, nil
}

func (r *Repository) FindInvitationByHash(ctx context.Context, tokenHash []byte) (domain.Invitation, error) {
	var out domain.Invitation
	var roleStr string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, token_hash, email, intended_role,
		       business_id::text, branch_id::text, issued_by::text,
		       created_at, expires_at, consumed_at, consumed_by::text
		FROM schema_identity.invitation
		WHERE token_hash = $1
	`, tokenHash).Scan(
		&out.ID, &out.TokenHash, &out.Email, &roleStr,
		&out.BusinessID, &out.BranchID, &out.IssuedBy,
		&out.CreatedAt, &out.ExpiresAt, &out.ConsumedAt, &out.ConsumedBy,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Invitation{}, domain.ErrNotFound
		}
		return domain.Invitation{}, err
	}
	out.IntendedRole = actor.Role(roleStr)
	return out, nil
}

func (r *Repository) FindInvitationByID(ctx context.Context, invitationID string) (domain.Invitation, error) {
	id, ok := actor.NormalizeUUID(invitationID)
	if !ok {
		return domain.Invitation{}, domain.ErrNotFound
	}
	var out domain.Invitation
	var roleStr string
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, token_hash, email, intended_role,
		       business_id::text, branch_id::text, issued_by::text,
		       created_at, expires_at, consumed_at, consumed_by::text
		FROM schema_identity.invitation
		WHERE id = $1::uuid
	`, id).Scan(
		&out.ID, &out.TokenHash, &out.Email, &roleStr,
		&out.BusinessID, &out.BranchID, &out.IssuedBy,
		&out.CreatedAt, &out.ExpiresAt, &out.ConsumedAt, &out.ConsumedBy,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Invitation{}, domain.ErrNotFound
		}
		return domain.Invitation{}, err
	}
	out.IntendedRole = actor.Role(roleStr)
	return out, nil
}

func (r *Repository) DeleteUnconsumedInvitation(ctx context.Context, invitationID string, businessID string) error {
	invID, ok1 := actor.NormalizeUUID(invitationID)
	busID, ok2 := actor.NormalizeUUID(businessID)
	if !ok1 || !ok2 {
		return domain.ErrNotFound
	}
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM schema_identity.invitation
			WHERE id = $1::uuid AND business_id = $2::uuid
		)
	`, invID, busID).Scan(&exists)
	if err != nil || !exists {
		return domain.ErrNotFound
	}
	// Idempotent revocation: mark expired if not yet consumed so token cannot be used
	_, err = r.pool.Exec(ctx, `
		UPDATE schema_identity.invitation
		   SET expires_at = LEAST(expires_at, now() - interval '1 second')
		 WHERE id = $1::uuid AND business_id = $2::uuid AND consumed_at IS NULL
	`, invID, busID)
	return err
}

func (r *Repository) HasUnexpiredInvitation(ctx context.Context, email string, intendedRole actor.Role, businessID, branchID *string) (bool, error) {
	normEmail, err := security.NormalizeEmail(email)
	if err != nil {
		return false, domain.ErrValidation
	}
	var exists bool
	err = r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM schema_identity.invitation
			WHERE lower(email) = lower($1)
			  AND intended_role = $2
			  AND consumed_at IS NULL
			  AND expires_at > now()
			  AND ($3::uuid IS NULL AND business_id IS NULL OR business_id = $3::uuid)
			  AND ($4::uuid IS NULL AND branch_id IS NULL OR branch_id = $4::uuid)
		)
	`, normEmail, string(intendedRole), businessID, branchID).Scan(&exists)
	return exists, err
}

func (r *Repository) SupersedeInvitations(ctx context.Context, email string, intendedRole actor.Role) error {
	normEmail, err := security.NormalizeEmail(email)
	if err != nil {
		return domain.ErrValidation
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE schema_identity.invitation
		   SET expires_at = now() - interval '1 second'
		 WHERE lower(email) = lower($1)
		   AND intended_role = $2
		   AND consumed_at IS NULL
		   AND expires_at > now()
	`, normEmail, string(intendedRole))
	return err
}

func (r *Repository) ActivateInvitation(ctx context.Context, invitationID, email string, intendedRole actor.Role, passwordHash string, now time.Time) (domain.Account, domain.Invitation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Account{}, domain.Invitation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the invitation row
	var inv domain.Invitation
	var roleStr string
	err = tx.QueryRow(ctx, `
		SELECT id::text, token_hash, email, intended_role,
		       business_id::text, branch_id::text, issued_by::text,
		       created_at, expires_at, consumed_at, consumed_by::text
		FROM schema_identity.invitation
		WHERE id = $1::uuid
		FOR UPDATE
	`, invitationID).Scan(
		&inv.ID, &inv.TokenHash, &inv.Email, &roleStr,
		&inv.BusinessID, &inv.BranchID, &inv.IssuedBy,
		&inv.CreatedAt, &inv.ExpiresAt, &inv.ConsumedAt, &inv.ConsumedBy,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Account{}, domain.Invitation{}, domain.ErrNotFound
		}
		return domain.Account{}, domain.Invitation{}, err
	}
	inv.IntendedRole = actor.Role(roleStr)

	if inv.ConsumedAt != nil {
		return domain.Account{}, domain.Invitation{}, domain.ErrInvitationConsumed
	}
	if !inv.ExpiresAt.After(now) {
		return domain.Account{}, domain.Invitation{}, domain.ErrInvitationExpired
	}

	// Find or create WORK account for inv.Email
	var acc domain.Account
	var kindStr string
	var platformRole *string
	err = tx.QueryRow(ctx, `
		SELECT id::text, email, kind, platform_role, status, display_name, created_at, updated_at, closed_at
		FROM schema_identity.account
		WHERE lower(email) = lower($1) AND kind = 'WORK'
		FOR UPDATE
	`, inv.Email).Scan(
		&acc.ID, &acc.Email, &kindStr, &platformRole, &acc.Status, &acc.DisplayName,
		&acc.CreatedAt, &acc.UpdatedAt, &acc.ClosedAt,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.Invitation{}, err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		newAccID, err := actor.NewUUIDv7()
		if err != nil {
			return domain.Account{}, domain.Invitation{}, err
		}
		var pr *string
		if inv.IntendedRole == actor.RoleAdmin || inv.IntendedRole == actor.RoleSoporte {
			s := string(inv.IntendedRole)
			pr = &s
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO schema_identity.account (id, email, kind, platform_role, status, display_name)
			VALUES ($1::uuid, $2, 'WORK', $3, 'ACTIVE', $2)
			RETURNING id::text, email, kind, platform_role, status, display_name, created_at, updated_at, closed_at
		`, newAccID, strings.ToLower(inv.Email), pr)
		acc, err = scanAccount(row)
		if err != nil {
			return domain.Account{}, domain.Invitation{}, err
		}
	} else {
		acc.Kind = actor.AccountKind(kindStr)
		if platformRole != nil {
			pr := actor.Role(*platformRole)
			acc.PlatformRole = &pr
		}
		// Inventory rule for activateInvitation: 403 forbidden if email is already an active ADMIN
		// (or active platform role conflicting with business membership)
		if acc.PlatformRole != nil && *acc.PlatformRole == actor.RoleAdmin && acc.Status == domain.StatusActive {
			return domain.Account{}, domain.Invitation{}, domain.ErrForbidden
		}
		if (inv.IntendedRole == actor.RoleOwner || inv.IntendedRole == actor.RoleEmployee) && acc.PlatformRole != nil {
			return domain.Account{}, domain.Invitation{}, domain.ErrForbidden
		}
		var newPR *string
		if inv.IntendedRole == actor.RoleAdmin || inv.IntendedRole == actor.RoleSoporte {
			s := string(inv.IntendedRole)
			newPR = &s
		} else if platformRole != nil {
			newPR = platformRole
		}
		row := tx.QueryRow(ctx, `
			UPDATE schema_identity.account
			   SET status = 'ACTIVE',
			       platform_role = $2,
			       closed_at = NULL,
			       updated_at = now()
			 WHERE id = $1::uuid
			RETURNING id::text, email, kind, platform_role, status, display_name, created_at, updated_at, closed_at
		`, acc.ID, newPR)
		acc, err = scanAccount(row)
		if err != nil {
			return domain.Account{}, domain.Invitation{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO schema_identity.credential (account_id, password_hash, updated_at)
		VALUES ($1::uuid, $2, now())
		ON CONFLICT (account_id) DO UPDATE SET password_hash = EXCLUDED.password_hash, updated_at = now()
	`, acc.ID, passwordHash); err != nil {
		return domain.Account{}, domain.Invitation{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE schema_identity.invitation
		   SET consumed_at = $2, consumed_by = $3::uuid
		 WHERE id = $1::uuid
	`, inv.ID, now.UTC(), acc.ID); err != nil {
		return domain.Account{}, domain.Invitation{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Account{}, domain.Invitation{}, err
	}
	return acc, inv, nil
}

func (r *Repository) CreatePasswordRecoveryToken(ctx context.Context, accountID string, tokenHash []byte, expiresAt time.Time) error {
	id, err := actor.NewUUIDv7()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO schema_identity.password_recovery_token (id, account_id, token_hash, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, $4)
	`, id, accountID, tokenHash, expiresAt.UTC())
	return err
}

func (r *Repository) ConsumePasswordRecoveryToken(ctx context.Context, tokenHash []byte, newPasswordHash string, now time.Time) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tokenID, accountID string
	var expiresAt time.Time
	var consumedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id::text, account_id::text, expires_at, consumed_at
		FROM schema_identity.password_recovery_token
		WHERE token_hash = $1
		FOR UPDATE
	`, tokenHash).Scan(&tokenID, &accountID, &expiresAt, &consumedAt)
	if err != nil {
		return "", domain.ErrUnauthenticated
	}
	if consumedAt != nil || !expiresAt.After(now) {
		return "", domain.ErrUnauthenticated
	}

	if _, err := tx.Exec(ctx, `
		UPDATE schema_identity.password_recovery_token
		   SET consumed_at = $2
		 WHERE id = $1::uuid
	`, tokenID, now.UTC()); err != nil {
		return "", err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO schema_identity.credential (account_id, password_hash, updated_at)
		VALUES ($1::uuid, $2, now())
		ON CONFLICT (account_id) DO UPDATE SET password_hash = EXCLUDED.password_hash, updated_at = now()
	`, accountID, newPasswordHash); err != nil {
		return "", err
	}

	// Revoke existing refresh sessions on password reset for safety
	if _, err := tx.Exec(ctx, `
		UPDATE schema_identity.refresh_session
		   SET revoked_at = COALESCE(revoked_at, now())
		 WHERE account_id = $1::uuid AND revoked_at IS NULL
	`, accountID); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return accountID, nil
}

func (r *Repository) RevokeSupportOperator(ctx context.Context, accountID string) error {
	id, ok := actor.NormalizeUUID(accountID)
	if !ok {
		return domain.ErrValidation
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var platformRole *string
	err = tx.QueryRow(ctx, `
		SELECT platform_role
		FROM schema_identity.account
		WHERE id = $1::uuid AND kind = 'WORK'
		FOR UPDATE
	`, id).Scan(&platformRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if platformRole == nil || *platformRole != string(actor.RoleSoporte) {
		return domain.ErrNotFound
	}

	if _, err := tx.Exec(ctx, `
		UPDATE schema_identity.account
		   SET status = 'CLOSED',
		       closed_at = COALESCE(closed_at, now()),
		       updated_at = now()
		 WHERE id = $1::uuid
	`, id); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE schema_identity.refresh_session
		   SET revoked_at = COALESCE(revoked_at, now())
		 WHERE account_id = $1::uuid AND revoked_at IS NULL
	`, id); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) AppendAudit(ctx context.Context, entry domain.AuditEntry) error {
	detail := entry.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal audit detail: %w", err)
	}
	var actorID, subjectID *string
	if entry.ActorAccountID != nil {
		if norm, ok := actor.NormalizeUUID(*entry.ActorAccountID); ok {
			actorID = &norm
		}
	}
	if entry.SubjectAccountID != nil {
		if norm, ok := actor.NormalizeUUID(*entry.SubjectAccountID); ok {
			subjectID = &norm
		}
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO schema_identity.identity_audit
			(event, outcome, actor_account_id, subject_account_id, reason, detail)
		VALUES ($1, $2, $3::uuid, $4::uuid, $5, $6::jsonb)
	`, entry.Event, entry.Outcome, actorID, subjectID, entry.Reason, string(detailJSON))
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "23514" || pgErr.Code == "23503")
}
