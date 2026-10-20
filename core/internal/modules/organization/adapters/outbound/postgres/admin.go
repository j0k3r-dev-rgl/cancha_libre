package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"canchalibre/core/internal/modules/organization/core/domain"
	"canchalibre/core/internal/modules/organization/core/ports"
	"canchalibre/core/internal/platform/actor"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CreateBusiness inserts a new business into schema_organization.business.
func (r *Repository) CreateBusiness(ctx context.Context, b domain.Business) (domain.Business, error) {
	busID, ok := actor.NormalizeUUID(b.ID)
	if !ok {
		return domain.Business{}, domain.ErrValidation
	}

	var created domain.Business
	err := r.pool.QueryRow(ctx, `
		INSERT INTO schema_organization.business (id, name, legal_name, tax_id, enabled)
		VALUES ($1::uuid, $2, $3, $4, $5)
		RETURNING id, name, legal_name, tax_id, enabled, created_at, suspended_at, archived_at
	`, busID, b.Name, b.LegalName, b.TaxID, b.Enabled).Scan(
		&created.ID,
		&created.Name,
		&created.LegalName,
		&created.TaxID,
		&created.Enabled,
		&created.CreatedAt,
		&created.SuspendedAt,
		&created.ArchivedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Business{}, domain.ErrConflict
		}
		return domain.Business{}, fmt.Errorf("create business: %w", err)
	}
	return created, nil
}

// ListBusinesses queries businesses with filters and pagination.
func (r *Repository) ListBusinesses(ctx context.Context, filter ports.BusinessFilter) ([]domain.BusinessSummary, int, error) {
	var conditions []string
	var args []any
	argIdx := 1

	switch filter.Status {
	case "active":
		conditions = append(conditions, "b.enabled = true AND b.suspended_at IS NULL AND b.archived_at IS NULL")
	case "suspended":
		conditions = append(conditions, "b.suspended_at IS NOT NULL AND b.archived_at IS NULL")
	case "archived":
		conditions = append(conditions, "b.archived_at IS NOT NULL")
	case "all":
		// no filter
	default:
		// Default to non-archived businesses
		conditions = append(conditions, "b.archived_at IS NULL")
	}

	if strings.TrimSpace(filter.Search) != "" {
		searchPattern := "%" + strings.ToLower(strings.TrimSpace(filter.Search)) + "%"
		conditions = append(conditions, fmt.Sprintf("(lower(b.name) LIKE $%d OR lower(coalesce(b.legal_name, '')) LIKE $%d OR lower(coalesce(b.tax_id, '')) LIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, searchPattern)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT count(*) FROM schema_organization.business b %s", whereClause)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count businesses: %w", err)
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
		SELECT b.id, b.name, b.legal_name, b.tax_id, b.enabled, b.suspended_at, b.archived_at, b.created_at,
		       (SELECT count(*) FROM schema_organization.branch br WHERE br.business_id = b.id AND br.archived_at IS NULL) as branches_count,
		       (SELECT count(*) FROM schema_organization.membership m WHERE m.business_id = b.id AND m.role = 'OWNER' AND m.revoked_at IS NULL) as active_owners_count
		FROM schema_organization.business b
		%s
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	queryArgs := append(args, limit, offset)
	rows, err := r.pool.Query(ctx, dataQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list businesses: %w", err)
	}
	defer rows.Close()

	var items []domain.BusinessSummary
	for rows.Next() {
		var s domain.BusinessSummary
		if err := rows.Scan(
			&s.ID,
			&s.Name,
			&s.LegalName,
			&s.TaxID,
			&s.Enabled,
			&s.SuspendedAt,
			&s.ArchivedAt,
			&s.CreatedAt,
			&s.BranchesCount,
			&s.ActiveOwnersCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan business summary: %w", err)
		}
		items = append(items, s)
	}
	if items == nil {
		items = []domain.BusinessSummary{}
	}
	return items, total, nil
}

// GetBusinessByID retrieves full business details, its owner account IDs, and its branches.
func (r *Repository) GetBusinessByID(ctx context.Context, businessID string) (domain.Business, []string, []domain.BusinessBranchView, error) {
	busID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		return domain.Business{}, nil, nil, domain.ErrNotFound
	}

	var b domain.Business
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, legal_name, tax_id, enabled, created_at, suspended_at, archived_at
		FROM schema_organization.business
		WHERE id = $1::uuid
	`, busID).Scan(
		&b.ID,
		&b.Name,
		&b.LegalName,
		&b.TaxID,
		&b.Enabled,
		&b.CreatedAt,
		&b.SuspendedAt,
		&b.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Business{}, nil, nil, domain.ErrNotFound
		}
		return domain.Business{}, nil, nil, fmt.Errorf("get business: %w", err)
	}

	ownerRows, err := r.pool.Query(ctx, `
		SELECT account_id::text
		FROM schema_organization.membership
		WHERE business_id = $1::uuid AND role = 'OWNER' AND revoked_at IS NULL
		ORDER BY created_at ASC
	`, busID)
	if err != nil {
		return domain.Business{}, nil, nil, fmt.Errorf("get business owners: %w", err)
	}
	defer ownerRows.Close()

	var ownerIDs []string
	for ownerRows.Next() {
		var id string
		if err := ownerRows.Scan(&id); err == nil {
			ownerIDs = append(ownerIDs, id)
		}
	}

	branchRows, err := r.pool.Query(ctx, `
		SELECT id, name, slug, approval_status, coordinates_confirmed, enabled, suspended_at, archived_at
		FROM schema_organization.branch
		WHERE business_id = $1::uuid
		ORDER BY name ASC
	`, busID)
	if err != nil {
		return domain.Business{}, nil, nil, fmt.Errorf("get business branches: %w", err)
	}
	defer branchRows.Close()

	var branches []domain.BusinessBranchView
	for branchRows.Next() {
		var br domain.BusinessBranchView
		var statusStr string
		if err := branchRows.Scan(
			&br.ID,
			&br.Name,
			&br.Slug,
			&statusStr,
			&br.CoordinatesConfirmed,
			&br.Enabled,
			&br.SuspendedAt,
			&br.ArchivedAt,
		); err != nil {
			return domain.Business{}, nil, nil, fmt.Errorf("scan business branch: %w", err)
		}
		br.ApprovalStatus = domain.ApprovalStatus(statusStr)
		br.IsPublished = br.ApprovalStatus == domain.ApprovalStatusApproved &&
			br.CoordinatesConfirmed &&
			br.Enabled &&
			br.SuspendedAt == nil &&
			br.ArchivedAt == nil &&
			b.Enabled &&
			b.SuspendedAt == nil &&
			b.ArchivedAt == nil
		branches = append(branches, br)
	}
	if branches == nil {
		branches = []domain.BusinessBranchView{}
	}

	return b, ownerIDs, branches, nil
}

// UpdateBusiness updates mutable business fields.
func (r *Repository) UpdateBusiness(ctx context.Context, businessID string, name, legalName, taxID *string) (domain.Business, error) {
	busID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		return domain.Business{}, domain.ErrNotFound
	}

	var b domain.Business
	err := r.pool.QueryRow(ctx, `
		UPDATE schema_organization.business
		SET name = COALESCE($2, name),
		    legal_name = COALESCE($3, legal_name),
		    tax_id = COALESCE($4, tax_id)
		WHERE id = $1::uuid AND archived_at IS NULL
		RETURNING id, name, legal_name, tax_id, enabled, created_at, suspended_at, archived_at
	`, busID, name, legalName, taxID).Scan(
		&b.ID,
		&b.Name,
		&b.LegalName,
		&b.TaxID,
		&b.Enabled,
		&b.CreatedAt,
		&b.SuspendedAt,
		&b.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Business{}, domain.ErrNotFound
		}
		if isUniqueViolation(err) {
			return domain.Business{}, domain.ErrConflict
		}
		return domain.Business{}, fmt.Errorf("update business: %w", err)
	}
	return b, nil
}

// SuspendBusiness administratively suspends a business.
func (r *Repository) SuspendBusiness(ctx context.Context, businessID string) (domain.Business, error) {
	busID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		return domain.Business{}, domain.ErrNotFound
	}

	var b domain.Business
	err := r.pool.QueryRow(ctx, `
		UPDATE schema_organization.business
		SET suspended_at = $2, enabled = false
		WHERE id = $1::uuid AND suspended_at IS NULL AND archived_at IS NULL
		RETURNING id, name, legal_name, tax_id, enabled, created_at, suspended_at, archived_at
	`, busID, time.Now().UTC()).Scan(
		&b.ID,
		&b.Name,
		&b.LegalName,
		&b.TaxID,
		&b.Enabled,
		&b.CreatedAt,
		&b.SuspendedAt,
		&b.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			var suspendedAt *time.Time
			checkErr := r.pool.QueryRow(ctx, "SELECT true, suspended_at FROM schema_organization.business WHERE id = $1::uuid", busID).Scan(&exists, &suspendedAt)
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return domain.Business{}, domain.ErrNotFound
			}
			if suspendedAt != nil {
				return domain.Business{}, domain.ErrAlreadySuspended
			}
			return domain.Business{}, domain.ErrConflict
		}
		return domain.Business{}, fmt.Errorf("suspend business: %w", err)
	}
	return b, nil
}

// ReactivateBusiness lifts administrative suspension.
func (r *Repository) ReactivateBusiness(ctx context.Context, businessID string) (domain.Business, error) {
	busID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		return domain.Business{}, domain.ErrNotFound
	}

	var b domain.Business
	err := r.pool.QueryRow(ctx, `
		UPDATE schema_organization.business
		SET suspended_at = NULL, enabled = true
		WHERE id = $1::uuid AND suspended_at IS NOT NULL AND archived_at IS NULL
		RETURNING id, name, legal_name, tax_id, enabled, created_at, suspended_at, archived_at
	`, busID).Scan(
		&b.ID,
		&b.Name,
		&b.LegalName,
		&b.TaxID,
		&b.Enabled,
		&b.CreatedAt,
		&b.SuspendedAt,
		&b.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			var suspendedAt *time.Time
			checkErr := r.pool.QueryRow(ctx, "SELECT true, suspended_at FROM schema_organization.business WHERE id = $1::uuid", busID).Scan(&exists, &suspendedAt)
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return domain.Business{}, domain.ErrNotFound
			}
			if suspendedAt == nil {
				return domain.Business{}, domain.ErrNotSuspended
			}
			return domain.Business{}, domain.ErrConflict
		}
		return domain.Business{}, fmt.Errorf("reactivate business: %w", err)
	}
	return b, nil
}

// ArchiveBusiness logically archives a business and its branches (BE-ORG-06).
func (r *Repository) ArchiveBusiness(ctx context.Context, businessID string) error {
	busID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		return domain.ErrNotFound
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin archive tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var busExists bool
	var archivedAt *time.Time
	err = tx.QueryRow(ctx, "SELECT true, archived_at FROM schema_organization.business WHERE id = $1::uuid", busID).Scan(&busExists, &archivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if archivedAt != nil {
		return domain.ErrAlreadyArchived
	}

	// Check if any branch of this business has pending approval requests
	var hasPendingRequests bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM schema_organization.branch_approval_history h
			JOIN schema_organization.branch br ON br.id = h.branch_id
			WHERE br.business_id = $1::uuid AND h.status = 'PENDING'
		)
	`, busID).Scan(&hasPendingRequests)
	if err != nil {
		return err
	}
	if hasPendingRequests {
		return domain.ErrActiveReservations
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, "UPDATE schema_organization.business SET archived_at = $2 WHERE id = $1::uuid", busID, now)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE schema_organization.branch SET archived_at = $2 WHERE business_id = $1::uuid AND archived_at IS NULL", busID, now)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UnarchiveBusiness restores an archived business (BE-ORG-07).
func (r *Repository) UnarchiveBusiness(ctx context.Context, businessID string) (domain.Business, error) {
	busID, ok := actor.NormalizeUUID(businessID)
	if !ok {
		return domain.Business{}, domain.ErrNotFound
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Business{}, fmt.Errorf("begin unarchive tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Restore branches that were archived when the business was archived
	_, err = tx.Exec(ctx, `
		UPDATE schema_organization.branch
		SET archived_at = NULL
		WHERE business_id = $1::uuid
		  AND archived_at IS NOT NULL
		  AND archived_at = (SELECT archived_at FROM schema_organization.business WHERE id = $1::uuid)
	`, busID)
	if err != nil {
		return domain.Business{}, fmt.Errorf("unarchive business branches: %w", err)
	}

	var b domain.Business
	err = tx.QueryRow(ctx, `
		UPDATE schema_organization.business
		SET archived_at = NULL
		WHERE id = $1::uuid AND archived_at IS NOT NULL
		RETURNING id, name, legal_name, tax_id, enabled, created_at, suspended_at, archived_at
	`, busID).Scan(
		&b.ID,
		&b.Name,
		&b.LegalName,
		&b.TaxID,
		&b.Enabled,
		&b.CreatedAt,
		&b.SuspendedAt,
		&b.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			var archivedAt *time.Time
			checkErr := tx.QueryRow(ctx, "SELECT true, archived_at FROM schema_organization.business WHERE id = $1::uuid", busID).Scan(&exists, &archivedAt)
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return domain.Business{}, domain.ErrNotFound
			}
			if archivedAt == nil {
				return domain.Business{}, domain.ErrNotArchived
			}
			return domain.Business{}, domain.ErrConflict
		}
		return domain.Business{}, fmt.Errorf("unarchive business: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Business{}, fmt.Errorf("commit unarchive tx: %w", err)
	}

	return b, nil
}

// ListBranchRequests queries branch requests inbox.
func (r *Repository) ListBranchRequests(ctx context.Context, filter ports.BranchRequestFilter) ([]domain.BranchRequestItem, int, error) {
	var conditions []string
	var args []any
	argIdx := 1

	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("h.status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf(`
		SELECT count(*)
		FROM schema_organization.branch_approval_history h
		JOIN schema_organization.branch br ON br.id = h.branch_id
		JOIN schema_organization.business b ON b.id = br.business_id
		%s
	`, whereClause)

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count branch requests: %w", err)
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
		SELECT h.id, h.branch_id, br.business_id, b.name, br.name, h.submitted_by, h.submitted_at,
		       h.reviewed_by, h.reviewed_at, h.status, h.rejection_reason
		FROM schema_organization.branch_approval_history h
		JOIN schema_organization.branch br ON br.id = h.branch_id
		JOIN schema_organization.business b ON b.id = br.business_id
		%s
		ORDER BY h.submitted_at DESC, h.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	queryArgs := append(args, limit, offset)
	rows, err := r.pool.Query(ctx, dataQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list branch requests: %w", err)
	}
	defer rows.Close()

	var items []domain.BranchRequestItem
	for rows.Next() {
		var item domain.BranchRequestItem
		var statusStr string
		if err := rows.Scan(
			&item.ID,
			&item.BranchID,
			&item.BusinessID,
			&item.BusinessName,
			&item.BranchName,
			&item.SubmittedBy,
			&item.SubmittedAt,
			&item.ReviewedBy,
			&item.ReviewedAt,
			&statusStr,
			&item.RejectionReason,
		); err != nil {
			return nil, 0, fmt.Errorf("scan branch request item: %w", err)
		}
		item.Status = domain.ApprovalStatus(statusStr)
		items = append(items, item)
	}
	if items == nil {
		items = []domain.BranchRequestItem{}
	}
	return items, total, nil
}

// GetBranchRequestDetail retrieves full branch request details and previous history.
func (r *Repository) GetBranchRequestDetail(ctx context.Context, requestID string) (domain.BranchRequestDetail, error) {
	reqID, ok := actor.NormalizeUUID(requestID)
	if !ok {
		return domain.BranchRequestDetail{}, domain.ErrNotFound
	}

	var d domain.BranchRequestDetail
	var statusStr, brStatusStr string
	err := r.pool.QueryRow(ctx, `
		SELECT h.id, h.branch_id, br.business_id, b.name, br.name, h.submitted_by, h.submitted_at,
		       h.reviewed_by, h.reviewed_at, h.status, h.rejection_reason,
		       br.id, br.business_id, br.name, br.slug, br.description, br.phone, br.whatsapp, br.email,
		       br.legal_name, br.tax_id, br.street_address, br.floor_apt, br.city, br.state_province,
		       br.postal_code, br.country, br.latitude, br.longitude, br.google_place_id, br.coordinates_confirmed,
		       br.min_booking_lead_minutes, br.min_cancellation_lead_minutes, br.approval_status, br.enabled,
		       br.suspended_at, br.created_at, br.archived_at
		FROM schema_organization.branch_approval_history h
		JOIN schema_organization.branch br ON br.id = h.branch_id
		JOIN schema_organization.business b ON b.id = br.business_id
		WHERE h.id = $1::uuid
	`, reqID).Scan(
		&d.ID,
		&d.BranchID,
		&d.BusinessID,
		&d.BusinessName,
		&d.BranchName,
		&d.SubmittedBy,
		&d.SubmittedAt,
		&d.ReviewedBy,
		&d.ReviewedAt,
		&statusStr,
		&d.RejectionReason,
		&d.Branch.ID,
		&d.Branch.BusinessID,
		&d.Branch.Name,
		&d.Branch.Slug,
		&d.Branch.Description,
		&d.Branch.Phone,
		&d.Branch.Whatsapp,
		&d.Branch.Email,
		&d.Branch.LegalName,
		&d.Branch.TaxID,
		&d.Branch.StreetAddress,
		&d.Branch.FloorApt,
		&d.Branch.City,
		&d.Branch.StateProvince,
		&d.Branch.PostalCode,
		&d.Branch.Country,
		&d.Branch.Latitude,
		&d.Branch.Longitude,
		&d.Branch.GooglePlaceID,
		&d.Branch.CoordinatesConfirmed,
		&d.Branch.MinBookingLeadMins,
		&d.Branch.MinCancelLeadMins,
		&brStatusStr,
		&d.Branch.Enabled,
		&d.Branch.SuspendedAt,
		&d.Branch.CreatedAt,
		&d.Branch.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.BranchRequestDetail{}, domain.ErrNotFound
		}
		return domain.BranchRequestDetail{}, fmt.Errorf("get branch request: %w", err)
	}
	d.Status = domain.ApprovalStatus(statusStr)
	d.Branch.ApprovalStatus = domain.ApprovalStatus(brStatusStr)

	// Fetch full history for this branch
	histRows, err := r.pool.Query(ctx, `
		SELECT id, branch_id, submitted_by, submitted_at, reviewed_by, reviewed_at, status, rejection_reason
		FROM schema_organization.branch_approval_history
		WHERE branch_id = $1::uuid
		ORDER BY submitted_at ASC
	`, d.BranchID)
	if err != nil {
		return domain.BranchRequestDetail{}, fmt.Errorf("get branch history: %w", err)
	}
	defer histRows.Close()

	var history []domain.BranchApprovalHistory
	for histRows.Next() {
		var h domain.BranchApprovalHistory
		var hStatus string
		if err := histRows.Scan(
			&h.ID,
			&h.BranchID,
			&h.SubmittedBy,
			&h.SubmittedAt,
			&h.ReviewedBy,
			&h.ReviewedAt,
			&hStatus,
			&h.RejectionReason,
		); err != nil {
			return domain.BranchRequestDetail{}, fmt.Errorf("scan branch history item: %w", err)
		}
		h.Status = domain.ApprovalStatus(hStatus)
		history = append(history, h)
	}
	d.History = history
	return d, nil
}

// ApproveBranchRequest approves a pending branch request.
func (r *Repository) ApproveBranchRequest(ctx context.Context, requestID, reviewedBy string) (domain.BranchApprovalHistory, error) {
	reqID, ok := actor.NormalizeUUID(requestID)
	if !ok {
		return domain.BranchApprovalHistory{}, domain.ErrNotFound
	}
	reviewerID, ok := actor.NormalizeUUID(reviewedBy)
	if !ok {
		return domain.BranchApprovalHistory{}, domain.ErrValidation
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.BranchApprovalHistory{}, fmt.Errorf("begin approve tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var hist domain.BranchApprovalHistory
	var currentStatus string
	now := time.Now().UTC()
	err = tx.QueryRow(ctx, `
		UPDATE schema_organization.branch_approval_history
		SET status = 'APPROVED', reviewed_by = $2::uuid, reviewed_at = $3
		WHERE id = $1::uuid AND status = 'PENDING'
		RETURNING id, branch_id, submitted_by, submitted_at, reviewed_by, reviewed_at, status
	`, reqID, reviewerID, now).Scan(
		&hist.ID,
		&hist.BranchID,
		&hist.SubmittedBy,
		&hist.SubmittedAt,
		&hist.ReviewedBy,
		&hist.ReviewedAt,
		&currentStatus,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			checkErr := tx.QueryRow(ctx, "SELECT true, status FROM schema_organization.branch_approval_history WHERE id = $1::uuid", reqID).Scan(&exists, &currentStatus)
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return domain.BranchApprovalHistory{}, domain.ErrNotFound
			}
			if currentStatus != string(domain.ApprovalStatusPending) {
				return domain.BranchApprovalHistory{}, domain.ErrAlreadyResolved
			}
			return domain.BranchApprovalHistory{}, domain.ErrConflict
		}
		return domain.BranchApprovalHistory{}, fmt.Errorf("approve branch request: %w", err)
	}
	hist.Status = domain.ApprovalStatus(currentStatus)

	// Update branch approval status to APPROVED
	_, err = tx.Exec(ctx, `
		UPDATE schema_organization.branch
		SET approval_status = 'APPROVED'
		WHERE id = $1::uuid
	`, hist.BranchID)
	if err != nil {
		return domain.BranchApprovalHistory{}, fmt.Errorf("update branch approval status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.BranchApprovalHistory{}, err
	}
	return hist, nil
}

// RejectBranchRequest rejects a pending branch request with a reason.
func (r *Repository) RejectBranchRequest(ctx context.Context, requestID, reviewedBy, reason string) (domain.BranchApprovalHistory, error) {
	reqID, ok := actor.NormalizeUUID(requestID)
	if !ok {
		return domain.BranchApprovalHistory{}, domain.ErrNotFound
	}
	reviewerID, ok := actor.NormalizeUUID(reviewedBy)
	if !ok {
		return domain.BranchApprovalHistory{}, domain.ErrValidation
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.BranchApprovalHistory{}, fmt.Errorf("begin reject tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var hist domain.BranchApprovalHistory
	var currentStatus string
	now := time.Now().UTC()
	err = tx.QueryRow(ctx, `
		UPDATE schema_organization.branch_approval_history
		SET status = 'REJECTED', reviewed_by = $2::uuid, reviewed_at = $3, rejection_reason = $4
		WHERE id = $1::uuid AND status = 'PENDING'
		RETURNING id, branch_id, submitted_by, submitted_at, reviewed_by, reviewed_at, status, rejection_reason
	`, reqID, reviewerID, now, reason).Scan(
		&hist.ID,
		&hist.BranchID,
		&hist.SubmittedBy,
		&hist.SubmittedAt,
		&hist.ReviewedBy,
		&hist.ReviewedAt,
		&currentStatus,
		&hist.RejectionReason,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			checkErr := tx.QueryRow(ctx, "SELECT true, status FROM schema_organization.branch_approval_history WHERE id = $1::uuid", reqID).Scan(&exists, &currentStatus)
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return domain.BranchApprovalHistory{}, domain.ErrNotFound
			}
			if currentStatus != string(domain.ApprovalStatusPending) {
				return domain.BranchApprovalHistory{}, domain.ErrAlreadyResolved
			}
			return domain.BranchApprovalHistory{}, domain.ErrConflict
		}
		return domain.BranchApprovalHistory{}, fmt.Errorf("reject branch request: %w", err)
	}
	hist.Status = domain.ApprovalStatus(currentStatus)

	// Update branch approval status to REJECTED
	_, err = tx.Exec(ctx, `
		UPDATE schema_organization.branch
		SET approval_status = 'REJECTED'
		WHERE id = $1::uuid
	`, hist.BranchID)
	if err != nil {
		return domain.BranchApprovalHistory{}, fmt.Errorf("update branch rejection status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.BranchApprovalHistory{}, err
	}
	return hist, nil
}

// ListBranches queries global branches across the platform.
func (r *Repository) ListBranches(ctx context.Context, filter ports.BranchFilter) ([]domain.BranchView, int, error) {
	var conditions []string
	var args []any
	argIdx := 1

	if filter.BusinessID != "" {
		conditions = append(conditions, fmt.Sprintf("br.business_id = $%d::uuid", argIdx))
		args = append(args, filter.BusinessID)
		argIdx++
	}

	if filter.ApprovalStatus != "" {
		conditions = append(conditions, fmt.Sprintf("br.approval_status = $%d", argIdx))
		args = append(args, filter.ApprovalStatus)
		argIdx++
	}

	if filter.City != "" {
		conditions = append(conditions, fmt.Sprintf("lower(br.city) = lower($%d)", argIdx))
		args = append(args, filter.City)
		argIdx++
	}

	switch filter.Status {
	case "active":
		conditions = append(conditions, "br.enabled = true AND br.suspended_at IS NULL AND br.archived_at IS NULL")
	case "suspended":
		conditions = append(conditions, "br.suspended_at IS NOT NULL AND br.archived_at IS NULL")
	case "archived":
		conditions = append(conditions, "br.archived_at IS NOT NULL")
	case "all":
		// no filter
	default:
		// Default to non-archived
		conditions = append(conditions, "br.archived_at IS NULL")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf(`
		SELECT count(*)
		FROM schema_organization.branch br
		JOIN schema_organization.business b ON b.id = br.business_id
		%s
	`, whereClause)

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count branches: %w", err)
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
		SELECT br.id, br.business_id, b.name, br.name, br.slug, br.city, br.state_province,
		       br.approval_status, br.coordinates_confirmed, br.enabled, br.suspended_at,
		       br.archived_at, br.created_at, b.enabled, b.suspended_at, b.archived_at
		FROM schema_organization.branch br
		JOIN schema_organization.business b ON b.id = br.business_id
		%s
		ORDER BY br.created_at DESC, br.id DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	queryArgs := append(args, limit, offset)
	rows, err := r.pool.Query(ctx, dataQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list branches: %w", err)
	}
	defer rows.Close()

	var items []domain.BranchView
	for rows.Next() {
		var v domain.BranchView
		var statusStr string
		var busEnabled bool
		var busSuspendedAt, busArchivedAt *time.Time
		if err := rows.Scan(
			&v.ID,
			&v.BusinessID,
			&v.BusinessName,
			&v.Name,
			&v.Slug,
			&v.City,
			&v.StateProvince,
			&statusStr,
			&v.CoordinatesConfirmed,
			&v.Enabled,
			&v.SuspendedAt,
			&v.ArchivedAt,
			&v.CreatedAt,
			&busEnabled,
			&busSuspendedAt,
			&busArchivedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan branch view: %w", err)
		}
		v.ApprovalStatus = domain.ApprovalStatus(statusStr)
		v.IsPublished = v.ApprovalStatus == domain.ApprovalStatusApproved &&
			v.CoordinatesConfirmed &&
			v.Enabled &&
			v.SuspendedAt == nil &&
			v.ArchivedAt == nil &&
			busEnabled &&
			busSuspendedAt == nil &&
			busArchivedAt == nil
		items = append(items, v)
	}
	if items == nil {
		items = []domain.BranchView{}
	}
	return items, total, nil
}

// SuspendBranch administratively suspends an individual branch.
func (r *Repository) SuspendBranch(ctx context.Context, branchID string) (domain.Branch, error) {
	brID, ok := actor.NormalizeUUID(branchID)
	if !ok {
		return domain.Branch{}, domain.ErrNotFound
	}

	var br domain.Branch
	var statusStr string
	err := r.pool.QueryRow(ctx, `
		UPDATE schema_organization.branch
		SET suspended_at = $2
		WHERE id = $1::uuid AND suspended_at IS NULL AND archived_at IS NULL
		RETURNING id, business_id, name, slug, description, phone, whatsapp, email,
		          street_address, floor_apt, city, state_province, postal_code, country,
		          latitude, longitude, google_place_id, coordinates_confirmed,
		          min_booking_lead_minutes, min_cancellation_lead_minutes,
		          approval_status, enabled, suspended_at, created_at, archived_at
	`, brID, time.Now().UTC()).Scan(
		&br.ID,
		&br.BusinessID,
		&br.Name,
		&br.Slug,
		&br.Description,
		&br.Phone,
		&br.Whatsapp,
		&br.Email,
		&br.StreetAddress,
		&br.FloorApt,
		&br.City,
		&br.StateProvince,
		&br.PostalCode,
		&br.Country,
		&br.Latitude,
		&br.Longitude,
		&br.GooglePlaceID,
		&br.CoordinatesConfirmed,
		&br.MinBookingLeadMins,
		&br.MinCancelLeadMins,
		&statusStr,
		&br.Enabled,
		&br.SuspendedAt,
		&br.CreatedAt,
		&br.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			var suspendedAt *time.Time
			checkErr := r.pool.QueryRow(ctx, "SELECT true, suspended_at FROM schema_organization.branch WHERE id = $1::uuid", brID).Scan(&exists, &suspendedAt)
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return domain.Branch{}, domain.ErrNotFound
			}
			if suspendedAt != nil {
				return domain.Branch{}, domain.ErrAlreadySuspended
			}
			return domain.Branch{}, domain.ErrConflict
		}
		return domain.Branch{}, fmt.Errorf("suspend branch: %w", err)
	}
	br.ApprovalStatus = domain.ApprovalStatus(statusStr)
	return br, nil
}

// ReactivateBranch lifts administrative suspension on a branch.
func (r *Repository) ReactivateBranch(ctx context.Context, branchID string) (domain.Branch, error) {
	brID, ok := actor.NormalizeUUID(branchID)
	if !ok {
		return domain.Branch{}, domain.ErrNotFound
	}

	var br domain.Branch
	var statusStr string
	err := r.pool.QueryRow(ctx, `
		UPDATE schema_organization.branch
		SET suspended_at = NULL
		WHERE id = $1::uuid AND suspended_at IS NOT NULL AND archived_at IS NULL
		RETURNING id, business_id, name, slug, description, phone, whatsapp, email,
		          street_address, floor_apt, city, state_province, postal_code, country,
		          latitude, longitude, google_place_id, coordinates_confirmed,
		          min_booking_lead_minutes, min_cancellation_lead_minutes,
		          approval_status, enabled, suspended_at, created_at, archived_at
	`, brID).Scan(
		&br.ID,
		&br.BusinessID,
		&br.Name,
		&br.Slug,
		&br.Description,
		&br.Phone,
		&br.Whatsapp,
		&br.Email,
		&br.StreetAddress,
		&br.FloorApt,
		&br.City,
		&br.StateProvince,
		&br.PostalCode,
		&br.Country,
		&br.Latitude,
		&br.Longitude,
		&br.GooglePlaceID,
		&br.CoordinatesConfirmed,
		&br.MinBookingLeadMins,
		&br.MinCancelLeadMins,
		&statusStr,
		&br.Enabled,
		&br.SuspendedAt,
		&br.CreatedAt,
		&br.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			var suspendedAt *time.Time
			checkErr := r.pool.QueryRow(ctx, "SELECT true, suspended_at FROM schema_organization.branch WHERE id = $1::uuid", brID).Scan(&exists, &suspendedAt)
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return domain.Branch{}, domain.ErrNotFound
			}
			if suspendedAt == nil {
				return domain.Branch{}, domain.ErrNotSuspended
			}
			return domain.Branch{}, domain.ErrConflict
		}
		return domain.Branch{}, fmt.Errorf("reactivate branch: %w", err)
	}
	br.ApprovalStatus = domain.ApprovalStatus(statusStr)
	return br, nil
}

// ListUserMemberships lists active memberships for an account ID.
func (r *Repository) ListUserMemberships(ctx context.Context, accountID string) ([]domain.UserMembershipView, error) {
	accID, ok := actor.NormalizeUUID(accountID)
	if !ok {
		return []domain.UserMembershipView{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.business_id, b.name, m.branch_id, s.name, m.role, m.created_at
		FROM schema_organization.membership m
		JOIN schema_organization.business b ON b.id = m.business_id
		LEFT JOIN schema_organization.branch s ON s.id = m.branch_id
		WHERE m.account_id = $1::uuid AND m.revoked_at IS NULL
		ORDER BY m.created_at DESC
	`, accID)
	if err != nil {
		return nil, fmt.Errorf("list user memberships: %w", err)
	}
	defer rows.Close()

	var items []domain.UserMembershipView
	for rows.Next() {
		var v domain.UserMembershipView
		var roleStr string
		if err := rows.Scan(
			&v.ID,
			&v.BusinessID,
			&v.BusinessName,
			&v.BranchID,
			&v.BranchName,
			&roleStr,
			&v.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user membership: %w", err)
		}
		v.Role = actor.Role(roleStr)
		items = append(items, v)
	}
	if items == nil {
		items = []domain.UserMembershipView{}
	}
	return items, nil
}
