package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"canchalibre/core/internal/modules/organization/core/ports"
	"canchalibre/core/internal/modules/organization/core/usecase"
	platformhttp "canchalibre/core/internal/platform/http"
)

func registerAdminBusinesses(api huma.API, deps Deps) {
	// 1. createBusiness
	huma.Register(api, huma.Operation{
		OperationID:   "createBusiness",
		RequestBody:   &huma.RequestBody{Description: "Business creation details and initial owners."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/businesses",
		Summary:       "Create a business",
		Description:   "ADMIN-only. Creates a new commercial business entity and optionally provisions initial owners.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusCreated,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err409Conflict,
			err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *createBusinessRequest) (*createBusinessResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		detail, err := deps.Service.CreateBusiness(ctx, p, usecase.CreateBusinessInput{
			Name:        in.Body.Name,
			LegalName:   in.Body.LegalName,
			TaxID:       in.Body.TaxID,
			OwnerEmails: in.Body.OwnerEmails,
		})
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		owners := make([]businessOwnerViewItem, 0, len(detail.Owners))
		for _, o := range detail.Owners {
			owners = append(owners, businessOwnerViewItem{
				AccountID: o.AccountID,
				Email:     o.Email,
				Status:    o.Status,
			})
		}
		return &createBusinessResponse{
			Body: createBusinessResponseBody{
				ID:        detail.ID,
				Name:      detail.Name,
				LegalName: detail.LegalName,
				TaxID:     detail.TaxID,
				Enabled:   detail.Enabled,
				CreatedAt: detail.CreatedAt,
				Owners:    owners,
			},
		}, nil
	})

	// 2. listBusinesses
	huma.Register(api, huma.Operation{
		OperationID:   "listBusinesses",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/admin/businesses",
		Summary:       "List businesses",
		Description:   "ADMIN-only. Returns paginated business summaries with branch and owner counts.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err500Internal,
		),
	}, func(ctx context.Context, in *listBusinessesRequest) (*listBusinessesResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		items, total, err := deps.Service.ListBusinesses(ctx, p, ports.BusinessFilter{
			Status: in.Status,
			Search: in.Search,
			Limit:  in.Limit,
			Offset: in.Offset,
		})
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		viewItems := make([]businessSummaryItem, 0, len(items))
		for _, it := range items {
			viewItems = append(viewItems, businessSummaryItem{
				ID:                it.ID,
				Name:              it.Name,
				LegalName:         it.LegalName,
				TaxID:             it.TaxID,
				Enabled:           it.Enabled,
				SuspendedAt:       it.SuspendedAt,
				ArchivedAt:        it.ArchivedAt,
				CreatedAt:         it.CreatedAt,
				BranchesCount:     it.BranchesCount,
				ActiveOwnersCount: it.ActiveOwnersCount,
			})
		}
		return &listBusinessesResponse{
			Body: listBusinessesResponseBody{
				Items:  viewItems,
				Total:  total,
				Limit:  in.Limit,
				Offset: in.Offset,
			},
		}, nil
	})

	// 3. getBusiness
	huma.Register(api, huma.Operation{
		OperationID:   "getBusiness",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/admin/businesses/{businessId}",
		Summary:       "Get business detail",
		Description:   "ADMIN-only. Returns full administrative details of a business including active owners and branches.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err500Internal,
		),
	}, func(ctx context.Context, in *getBusinessRequest) (*getBusinessResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		detail, err := deps.Service.GetBusiness(ctx, p, in.BusinessID)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		owners := make([]businessOwnerViewItem, 0, len(detail.Owners))
		for _, o := range detail.Owners {
			owners = append(owners, businessOwnerViewItem{
				AccountID:   o.AccountID,
				Email:       o.Email,
				DisplayName: o.DisplayName,
				Status:      o.Status,
			})
		}
		branches := make([]businessBranchViewItem, 0, len(detail.Branches))
		for _, br := range detail.Branches {
			branches = append(branches, businessBranchViewItem{
				ID:                   br.ID,
				Name:                 br.Name,
				Slug:                 br.Slug,
				ApprovalStatus:       string(br.ApprovalStatus),
				CoordinatesConfirmed: br.CoordinatesConfirmed,
				Enabled:              br.Enabled,
				IsPublished:          br.IsPublished,
				SuspendedAt:          br.SuspendedAt,
				ArchivedAt:           br.ArchivedAt,
			})
		}

		return &getBusinessResponse{
			Body: getBusinessResponseBody{
				ID:          detail.ID,
				Name:        detail.Name,
				LegalName:   detail.LegalName,
				TaxID:       detail.TaxID,
				Enabled:     detail.Enabled,
				SuspendedAt: detail.SuspendedAt,
				ArchivedAt:  detail.ArchivedAt,
				CreatedAt:   detail.CreatedAt,
				Owners:      owners,
				Branches:    branches,
			},
		}, nil
	})

	// 4. updateBusiness
	huma.Register(api, huma.Operation{
		OperationID:   "updateBusiness",
		RequestBody:   &huma.RequestBody{Description: "Fields to update on the business."},
		Method:        nethttp.MethodPatch,
		Path:          "/api/v1/admin/businesses/{businessId}",
		Summary:       "Update business",
		Description:   "ADMIN-only. Updates commercial or fiscal attributes of a business.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err404NotFound,
			err409Conflict, err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *updateBusinessRequest) (*updateBusinessResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		b, err := deps.Service.UpdateBusiness(ctx, p, in.BusinessID, usecase.UpdateBusinessInput{
			Name:      in.Body.Name,
			LegalName: in.Body.LegalName,
			TaxID:     in.Body.TaxID,
		})
		if err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &updateBusinessResponse{
			Body: updateBusinessResponseBody{
				ID:          b.ID,
				Name:        b.Name,
				LegalName:   b.LegalName,
				TaxID:       b.TaxID,
				Enabled:     b.Enabled,
				SuspendedAt: b.SuspendedAt,
				ArchivedAt:  b.ArchivedAt,
			},
		}, nil
	})

	// 5. suspendBusiness
	huma.Register(api, huma.Operation{
		OperationID:   "suspendBusiness",
		RequestBody:   &huma.RequestBody{Description: "Suspension reason."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/businesses/{businessId}/suspend",
		Summary:       "Suspend business",
		Description:   "ADMIN-only. Administratively suspends a business and blocks access across all its branches.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err404NotFound,
			err409Conflict, err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *suspendBusinessRequest) (*suspendBusinessResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		b, err := deps.Service.SuspendBusiness(ctx, p, in.BusinessID, in.Body.Reason)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &suspendBusinessResponse{
			Body: suspendBusinessResponseBody{
				ID:          b.ID,
				SuspendedAt: b.SuspendedAt,
				Enabled:     b.Enabled,
			},
		}, nil
	})

	// 6. reactivateBusiness
	huma.Register(api, huma.Operation{
		OperationID:   "reactivateBusiness",
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/businesses/{businessId}/reactivate",
		Summary:       "Reactivate business",
		Description:   "ADMIN-only. Lifts administrative suspension of a business.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err409Conflict, err500Internal,
		),
	}, func(ctx context.Context, in *reactivateBusinessRequest) (*reactivateBusinessResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		b, err := deps.Service.ReactivateBusiness(ctx, p, in.BusinessID)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &reactivateBusinessResponse{
			Body: reactivateBusinessResponseBody{
				ID:          b.ID,
				SuspendedAt: nil,
				Enabled:     b.Enabled,
			},
		}, nil
	})

	// 7. archiveBusiness
	huma.Register(api, huma.Operation{
		OperationID:   "archiveBusiness",
		Method:        nethttp.MethodDelete,
		Path:          "/api/v1/admin/businesses/{businessId}",
		Summary:       "Archive business",
		Description:   "ADMIN-only. Logically archives a business if no active reservations or pending requests exist.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusNoContent,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err409Conflict, err500Internal,
		),
	}, func(ctx context.Context, in *archiveBusinessRequest) (*emptyOutput, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := deps.Service.ArchiveBusiness(ctx, p, in.BusinessID); err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &emptyOutput{}, nil
	})

	// 8. unarchiveBusiness
	huma.Register(api, huma.Operation{
		OperationID:   "unarchiveBusiness",
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/businesses/{businessId}/unarchive",
		Summary:       "Unarchive business",
		Description:   "ADMIN-only. Restores an archived business preserving previous state.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err409Conflict, err500Internal,
		),
	}, func(ctx context.Context, in *unarchiveBusinessRequest) (*unarchiveBusinessResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		b, err := deps.Service.UnarchiveBusiness(ctx, p, in.BusinessID)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &unarchiveBusinessResponse{
			Body: unarchiveBusinessResponseBody{
				ID:         b.ID,
				ArchivedAt: nil,
			},
		}, nil
	})
}

// Request and response DTOs

type businessOwnerViewItem struct {
	AccountID   string  `json:"account_id" example:"019294f0-0000-7000-8000-000000000002" doc:"Account UUID."`
	Email       string  `json:"email" example:"owner@canchalibre.local" doc:"Email address."`
	DisplayName *string `json:"display_name,omitempty" example:"Juan Pérez" doc:"User display name."`
	Status      string  `json:"status" example:"ACTIVE" doc:"Account status."`
}

type businessBranchViewItem struct {
	ID                   string     `json:"id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	Name                 string     `json:"name" example:"Sede Belgrano" doc:"Branch name."`
	Slug                 string     `json:"slug" example:"sede-belgrano" doc:"Branch slug."`
	ApprovalStatus       string     `json:"approval_status" example:"APPROVED" doc:"Approval status."`
	CoordinatesConfirmed bool       `json:"coordinates_confirmed" example:"true" doc:"Whether coordinates are confirmed."`
	Enabled              bool       `json:"enabled" example:"true" doc:"Owner switch."`
	IsPublished          bool       `json:"is_published" example:"true" doc:"Whether the branch is published."`
	SuspendedAt          *time.Time `json:"suspended_at,omitempty" doc:"Suspension timestamp in UTC."`
	ArchivedAt           *time.Time `json:"archived_at,omitempty" doc:"Archival timestamp in UTC."`
}

type businessSummaryItem struct {
	ID                string     `json:"id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	Name              string     `json:"name" example:"Club San Martín" doc:"Business name."`
	LegalName         *string    `json:"legal_name,omitempty" example:"San Martín Deportes SRL" doc:"Legal company name."`
	TaxID             *string    `json:"tax_id,omitempty" example:"30-11223344-5" doc:"CUIT or tax identifier."`
	Enabled           bool       `json:"enabled" example:"true" doc:"Platform master switch."`
	SuspendedAt       *time.Time `json:"suspended_at,omitempty" doc:"Suspension timestamp in UTC."`
	ArchivedAt        *time.Time `json:"archived_at,omitempty" doc:"Archival timestamp in UTC."`
	CreatedAt         time.Time  `json:"created_at" example:"2026-10-20T10:00:00Z" doc:"Creation timestamp in UTC."`
	BranchesCount     int        `json:"branches_count" example:"2" doc:"Number of active branches."`
	ActiveOwnersCount int        `json:"active_owners_count" example:"1" doc:"Number of active co-owners."`
}

type createBusinessRequestBody struct {
	Name        string   `json:"name" example:"Club San Martín" doc:"Commercial name."`
	LegalName   *string  `json:"legal_name,omitempty" example:"San Martín Deportes SRL" doc:"Legal company name."`
	TaxID       *string  `json:"tax_id,omitempty" example:"30-11223344-5" doc:"Tax ID."`
	OwnerEmails []string `json:"owner_emails,omitempty" example:"[\"owner@canchalibre.local\"]" doc:"Initial owner email addresses."`
}

type createBusinessRequest struct {
	Body createBusinessRequestBody
}

type createBusinessResponseBody struct {
	ID        string                  `json:"id" example:"019294f0-0000-7000-8000-000000000101" doc:"Created business UUID."`
	Name      string                  `json:"name" example:"Club San Martín" doc:"Business commercial name."`
	LegalName *string                 `json:"legal_name,omitempty" example:"San Martín Deportes SRL" doc:"Legal name."`
	TaxID     *string                 `json:"tax_id,omitempty" example:"30-11223344-5" doc:"Tax ID."`
	Enabled   bool                    `json:"enabled" example:"true" doc:"Master platform enabled switch."`
	CreatedAt time.Time               `json:"created_at" example:"2026-10-20T10:00:00Z" doc:"Creation timestamp in UTC."`
	Owners    []businessOwnerViewItem `json:"owners" example:"[{\"account_id\":\"019294f0-0000-7000-8000-000000000002\",\"email\":\"owner@canchalibre.local\",\"display_name\":\"Juan Pérez\",\"status\":\"ACTIVE\"}]" doc:"Initial co-owners."`
}

type createBusinessResponse struct {
	Body createBusinessResponseBody
}

type listBusinessesRequest struct {
	Status string `query:"status" example:"active" doc:"Status filter: active, suspended, archived, all."`
	Search string `query:"search" example:"San Martin" doc:"Search pattern by name or tax ID."`
	Limit  int    `query:"limit" example:"20" doc:"Page limit."`
	Offset int    `query:"offset" example:"0" doc:"Page offset."`
}

type listBusinessesResponseBody struct {
	Items  []businessSummaryItem `json:"items" example:"[{\"id\":\"019294f0-0000-7000-8000-000000000101\",\"name\":\"Club San Martín\",\"legal_name\":\"San Martín Deportes SRL\",\"tax_id\":\"30-11223344-5\",\"enabled\":true,\"created_at\":\"2026-10-20T10:00:00Z\",\"branches_count\":2,\"active_owners_count\":1}]" doc:"List of business summaries."`
	Total  int                   `json:"total" example:"1" doc:"Total count."`
	Limit  int                   `json:"limit" example:"20" doc:"Page limit."`
	Offset int                   `json:"offset" example:"0" doc:"Page offset."`
}

type listBusinessesResponse struct {
	Body listBusinessesResponseBody
}

type getBusinessRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
}

type getBusinessResponseBody struct {
	ID          string                   `json:"id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	Name        string                   `json:"name" example:"Club San Martín" doc:"Business commercial name."`
	LegalName   *string                  `json:"legal_name,omitempty" example:"San Martín Deportes SRL" doc:"Legal company name."`
	TaxID       *string                  `json:"tax_id,omitempty" example:"30-11223344-5" doc:"Tax ID."`
	Enabled     bool                     `json:"enabled" example:"true" doc:"Enabled switch."`
	SuspendedAt *time.Time               `json:"suspended_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Suspension timestamp."`
	ArchivedAt  *time.Time               `json:"archived_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Archival timestamp."`
	CreatedAt   time.Time                `json:"created_at" example:"2026-10-20T10:00:00Z" doc:"Creation timestamp in UTC."`
	Owners      []businessOwnerViewItem  `json:"owners" example:"[{\"account_id\":\"019294f0-0000-7000-8000-000000000002\",\"email\":\"owner@canchalibre.local\",\"display_name\":\"Juan Pérez\",\"status\":\"ACTIVE\"}]" doc:"Associated owners."`
	Branches    []businessBranchViewItem `json:"branches" example:"[{\"id\":\"019294f0-0000-7000-8000-000000000201\",\"name\":\"Sede Belgrano\",\"slug\":\"sede-belgrano\",\"approval_status\":\"APPROVED\",\"coordinates_confirmed\":true,\"enabled\":true,\"is_published\":true}]" doc:"Associated branches."`
}

type getBusinessResponse struct {
	Body getBusinessResponseBody
}

type updateBusinessRequestBody struct {
	Name      *string `json:"name,omitempty" example:"Club San Martín Renombrado" doc:"Updated commercial name."`
	LegalName *string `json:"legal_name,omitempty" example:"San Martín Nueva SRL" doc:"Updated legal name."`
	TaxID     *string `json:"tax_id,omitempty" example:"30-99887766-5" doc:"Updated tax ID."`
}

type updateBusinessRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	Body       updateBusinessRequestBody
}

type updateBusinessResponseBody struct {
	ID          string     `json:"id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	Name        string     `json:"name" example:"Club San Martín Renombrado" doc:"Updated name."`
	LegalName   *string    `json:"legal_name,omitempty" example:"San Martín Nueva SRL" doc:"Updated legal name."`
	TaxID       *string    `json:"tax_id,omitempty" example:"30-99887766-5" doc:"Updated tax ID."`
	Enabled     bool       `json:"enabled" example:"true" doc:"Enabled switch."`
	SuspendedAt *time.Time `json:"suspended_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Suspension timestamp."`
	ArchivedAt  *time.Time `json:"archived_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Archival timestamp."`
}

type updateBusinessResponse struct {
	Body updateBusinessResponseBody
}

type suspendBusinessRequestBody struct {
	Reason string `json:"reason" example:"Violation of service terms." doc:"Suspension reason."`
}

type suspendBusinessRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	Body       suspendBusinessRequestBody
}

type suspendBusinessResponseBody struct {
	ID          string     `json:"id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	SuspendedAt *time.Time `json:"suspended_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Suspension timestamp in UTC."`
	Enabled     bool       `json:"enabled" example:"false" doc:"Enabled status (false)."`
}

type suspendBusinessResponse struct {
	Body suspendBusinessResponseBody
}

type reactivateBusinessRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
}

type reactivateBusinessResponseBody struct {
	ID          string     `json:"id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	SuspendedAt *time.Time `json:"suspended_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Suspension timestamp (null)."`
	Enabled     bool       `json:"enabled" example:"true" doc:"Enabled status (true)."`
}

type reactivateBusinessResponse struct {
	Body reactivateBusinessResponseBody
}

type archiveBusinessRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
}

type unarchiveBusinessRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
}

type unarchiveBusinessResponseBody struct {
	ID         string     `json:"id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	ArchivedAt *time.Time `json:"archived_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Archival timestamp (null)."`
}

type unarchiveBusinessResponse struct {
	Body unarchiveBusinessResponseBody
}
