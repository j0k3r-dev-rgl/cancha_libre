package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"canchalibre/core/internal/modules/organization/core/ports"
	platformhttp "canchalibre/core/internal/platform/http"
)

func registerAdminBranches(api huma.API, deps Deps) {
	// 9. listBranchRequests
	huma.Register(api, huma.Operation{
		OperationID:   "listBranchRequests",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/admin/branch-requests",
		Summary:       "List branch requests",
		Description:   "ADMIN and SOPORTE. Returns paginated branch requests inbox filterable by review status.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *listBranchRequestsRequest) (*listBranchRequestsResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		offset, err := platformhttp.PageOffset(in.Page, in.Limit)
		if err != nil {
			return nil, err
		}
		items, total, err := deps.Service.ListBranchRequests(ctx, p, ports.BranchRequestFilter{
			Status: in.Status,
			Limit:  in.Limit,
			Offset: offset,
		})
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		viewItems := make([]branchRequestItemResponse, 0, len(items))
		for _, it := range items {
			viewItems = append(viewItems, branchRequestItemResponse{
				ID:              it.ID,
				BranchID:        it.BranchID,
				BusinessID:      it.BusinessID,
				BusinessName:    it.BusinessName,
				BranchName:      it.BranchName,
				SubmittedBy:     it.SubmittedBy,
				SubmittedAt:     it.SubmittedAt,
				ReviewedBy:      it.ReviewedBy,
				ReviewedAt:      it.ReviewedAt,
				Status:          string(it.Status),
				RejectionReason: it.RejectionReason,
			})
		}
		return &listBranchRequestsResponse{
			Body: listBranchRequestsResponseBody{
				Items: viewItems,
				Total: total,
				Limit: in.Limit,
				Page:  in.Page,
			},
		}, nil
	})

	// 10. getBranchRequest
	huma.Register(api, huma.Operation{
		OperationID:   "getBranchRequest",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/admin/branch-requests/{requestId}",
		Summary:       "Get branch request detail",
		Description:   "ADMIN and SOPORTE. Returns branch request details with physical branch data and prior review history.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err500Internal,
		),
	}, func(ctx context.Context, in *getBranchRequestRequest) (*getBranchRequestResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		detail, err := deps.Service.GetBranchRequest(ctx, p, in.RequestID)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		historyItems := make([]branchApprovalHistoryItem, 0, len(detail.History))
		for _, h := range detail.History {
			historyItems = append(historyItems, branchApprovalHistoryItem{
				ID:              h.ID,
				BranchID:        h.BranchID,
				SubmittedBy:     h.SubmittedBy,
				SubmittedAt:     h.SubmittedAt,
				ReviewedBy:      h.ReviewedBy,
				ReviewedAt:      h.ReviewedAt,
				Status:          string(h.Status),
				RejectionReason: h.RejectionReason,
			})
		}

		return &getBranchRequestResponse{
			Body: getBranchRequestResponseBody{
				ID:              detail.ID,
				BranchID:        detail.BranchID,
				BusinessID:      detail.BusinessID,
				BusinessName:    detail.BusinessName,
				BranchName:      detail.BranchName,
				SubmittedBy:     detail.SubmittedBy,
				SubmittedAt:     detail.SubmittedAt,
				ReviewedBy:      detail.ReviewedBy,
				ReviewedAt:      detail.ReviewedAt,
				Status:          string(detail.Status),
				RejectionReason: detail.RejectionReason,
				Branch: branchDetailItem{
					ID:                   detail.Branch.ID,
					BusinessID:           detail.Branch.BusinessID,
					Name:                 detail.Branch.Name,
					Slug:                 detail.Branch.Slug,
					Description:          detail.Branch.Description,
					Phone:                detail.Branch.Phone,
					Whatsapp:             detail.Branch.Whatsapp,
					Email:                detail.Branch.Email,
					StreetAddress:        detail.Branch.StreetAddress,
					FloorApt:             detail.Branch.FloorApt,
					City:                 detail.Branch.City,
					StateProvince:        detail.Branch.StateProvince,
					PostalCode:           detail.Branch.PostalCode,
					Country:              detail.Branch.Country,
					Latitude:             detail.Branch.Latitude,
					Longitude:            detail.Branch.Longitude,
					GooglePlaceID:        detail.Branch.GooglePlaceID,
					CoordinatesConfirmed: detail.Branch.CoordinatesConfirmed,
					MinBookingLeadMins:   detail.Branch.MinBookingLeadMins,
					MinCancelLeadMins:    detail.Branch.MinCancelLeadMins,
					ApprovalStatus:       string(detail.Branch.ApprovalStatus),
					Enabled:              detail.Branch.Enabled,
					SuspendedAt:          detail.Branch.SuspendedAt,
					CreatedAt:            detail.Branch.CreatedAt,
					ArchivedAt:           detail.Branch.ArchivedAt,
				},
				History: historyItems,
			},
		}, nil
	})

	// 11. approveBranchRequest
	huma.Register(api, huma.Operation{
		OperationID:   "approveBranchRequest",
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/branch-requests/{requestId}/approve",
		Summary:       "Approve branch request",
		Description:   "ADMIN and SOPORTE. Approves a branch request and updates branch approval status to APPROVED.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err409Conflict, err500Internal,
		),
	}, func(ctx context.Context, in *approveBranchRequestRequest) (*approveBranchRequestResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		hist, err := deps.Service.ApproveBranchRequest(ctx, p, in.RequestID)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &approveBranchRequestResponse{
			Body: approveBranchRequestResponseBody{
				ID:         hist.ID,
				BranchID:   hist.BranchID,
				Status:     string(hist.Status),
				ReviewedBy: hist.ReviewedBy,
				ReviewedAt: hist.ReviewedAt,
			},
		}, nil
	})

	// 12. rejectBranchRequest
	huma.Register(api, huma.Operation{
		OperationID:   "rejectBranchRequest",
		RequestBody:   &huma.RequestBody{Description: "Rejection reason."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/branch-requests/{requestId}/reject",
		Summary:       "Reject branch request",
		Description:   "ADMIN and SOPORTE. Rejects a branch request with a mandatory non-empty rejection reason.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err404NotFound,
			err409Conflict, err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *rejectBranchRequestRequest) (*rejectBranchRequestResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		hist, err := deps.Service.RejectBranchRequest(ctx, p, in.RequestID, in.Body.RejectionReason)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &rejectBranchRequestResponse{
			Body: rejectBranchRequestResponseBody{
				ID:              hist.ID,
				BranchID:        hist.BranchID,
				Status:          string(hist.Status),
				RejectionReason: hist.RejectionReason,
				ReviewedBy:      hist.ReviewedBy,
				ReviewedAt:      hist.ReviewedAt,
			},
		}, nil
	})

	// 13. listAdminBranches
	huma.Register(api, huma.Operation{
		OperationID:   "listAdminBranches",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/admin/branches",
		Summary:       "List all branches",
		Description:   "ADMIN and SOPORTE. Global listing of all branches across the platform with approval, suspension, and publication states.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *listAdminBranchesRequest) (*listAdminBranchesResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		offset, err := platformhttp.PageOffset(in.Page, in.Limit)
		if err != nil {
			return nil, err
		}
		items, total, err := deps.Service.ListBranches(ctx, p, ports.BranchFilter{
			BusinessID:     in.BusinessID,
			ApprovalStatus: in.ApprovalStatus,
			Status:         in.Status,
			City:           in.City,
			Limit:          in.Limit,
			Offset:         offset,
		})
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		viewItems := make([]adminBranchViewItem, 0, len(items))
		for _, b := range items {
			viewItems = append(viewItems, adminBranchViewItem{
				ID:                   b.ID,
				BusinessID:           b.BusinessID,
				BusinessName:         b.BusinessName,
				Name:                 b.Name,
				Slug:                 b.Slug,
				City:                 b.City,
				StateProvince:        b.StateProvince,
				ApprovalStatus:       string(b.ApprovalStatus),
				CoordinatesConfirmed: b.CoordinatesConfirmed,
				Enabled:              b.Enabled,
				IsPublished:          b.IsPublished,
				SuspendedAt:          b.SuspendedAt,
				ArchivedAt:           b.ArchivedAt,
				CreatedAt:            b.CreatedAt,
			})
		}
		return &listAdminBranchesResponse{
			Body: listAdminBranchesResponseBody{
				Items: viewItems,
				Total: total,
				Limit: in.Limit,
				Page:  in.Page,
			},
		}, nil
	})

	// 14. suspendBranch
	huma.Register(api, huma.Operation{
		OperationID:   "suspendBranch",
		RequestBody:   &huma.RequestBody{Description: "Suspension reason."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/branches/{branchId}/suspend",
		Summary:       "Suspend branch",
		Description:   "ADMIN-only. Administratively suspends an individual branch.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err404NotFound,
			err409Conflict, err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *suspendBranchRequest) (*suspendBranchResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		b, err := deps.Service.SuspendBranch(ctx, p, in.BranchID, in.Body.Reason)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &suspendBranchResponse{
			Body: suspendBranchResponseBody{
				ID:          b.ID,
				SuspendedAt: b.SuspendedAt,
			},
		}, nil
	})

	// 15. reactivateBranch
	huma.Register(api, huma.Operation{
		OperationID:   "reactivateBranch",
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/branches/{branchId}/reactivate",
		Summary:       "Reactivate branch",
		Description:   "ADMIN-only. Lifts administrative suspension of a branch.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err409Conflict, err500Internal,
		),
	}, func(ctx context.Context, in *reactivateBranchRequest) (*reactivateBranchResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		b, err := deps.Service.ReactivateBranch(ctx, p, in.BranchID)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &reactivateBranchResponse{
			Body: reactivateBranchResponseBody{
				ID:          b.ID,
				SuspendedAt: nil,
			},
		}, nil
	})
}

// Request and Response DTOs

type branchRequestItemResponse struct {
	ID              string     `json:"id" example:"019294f0-0000-7000-8000-000000000301" doc:"Request UUID."`
	BranchID        string     `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	BusinessID      string     `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	BusinessName    string     `json:"business_name" example:"Club San Martín" doc:"Business name."`
	BranchName      string     `json:"branch_name" example:"Sede Belgrano" doc:"Branch name."`
	SubmittedBy     string     `json:"submitted_by" example:"019294f0-0000-7000-8000-000000000002" doc:"Submitter account UUID."`
	SubmittedAt     time.Time  `json:"submitted_at" example:"2026-10-20T10:00:00Z" doc:"Submission timestamp."`
	ReviewedBy      *string    `json:"reviewed_by,omitempty" doc:"Reviewer account UUID."`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty" doc:"Review timestamp."`
	Status          string     `json:"status" example:"PENDING" doc:"Review status: PENDING, APPROVED, REJECTED."`
	RejectionReason *string    `json:"rejection_reason,omitempty" doc:"Rejection reason if rejected."`
}

type listBranchRequestsRequest struct {
	Status string `query:"status" example:"PENDING" doc:"Status filter: PENDING, APPROVED, REJECTED."`
	Limit  int    `query:"limit" default:"20" example:"20" doc:"Page size, from 1 to 100."`
	Page   int    `query:"page" default:"1" example:"1" doc:"One-based page number."`
}

type listBranchRequestsResponseBody struct {
	Items []branchRequestItemResponse `json:"items" example:"[{\"id\":\"019294f0-0000-7000-8000-000000000301\",\"branch_id\":\"019294f0-0000-7000-8000-000000000201\",\"business_id\":\"019294f0-0000-7000-8000-000000000101\",\"business_name\":\"Club San Martín\",\"branch_name\":\"Sede Belgrano\",\"submitted_by\":\"019294f0-0000-7000-8000-000000000002\",\"submitted_at\":\"2026-10-20T10:00:00Z\",\"status\":\"PENDING\"}]" doc:"List of branch requests."`
	Total int                         `json:"total" example:"1" doc:"Total count."`
	Limit int                         `json:"limit" example:"20" doc:"Page limit."`
	Page  int                         `json:"page" example:"1" doc:"One-based page number."`
}

type listBranchRequestsResponse struct {
	Body listBranchRequestsResponseBody
}

type getBranchRequestRequest struct {
	RequestID string `path:"requestId" example:"019294f0-0000-7000-8000-000000000301" doc:"Branch request UUID."`
}

type branchDetailItem struct {
	ID                   string     `json:"id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	BusinessID           string     `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	Name                 string     `json:"name" example:"Sede Belgrano" doc:"Branch name."`
	Slug                 string     `json:"slug" example:"sede-belgrano" doc:"Branch slug."`
	Description          *string    `json:"description,omitempty" doc:"Description."`
	Phone                *string    `json:"phone,omitempty" doc:"Phone."`
	Whatsapp             *string    `json:"whatsapp,omitempty" doc:"WhatsApp."`
	Email                *string    `json:"email,omitempty" doc:"Email."`
	StreetAddress        string     `json:"street_address" example:"Av. Libertador 4500" doc:"Physical street address."`
	FloorApt             *string    `json:"floor_apt,omitempty" doc:"Floor/apartment."`
	City                 string     `json:"city" example:"Buenos Aires" doc:"City."`
	StateProvince        string     `json:"state_province" example:"CABA" doc:"State or province."`
	PostalCode           string     `json:"postal_code" example:"C1425" doc:"Postal code."`
	Country              string     `json:"country" example:"AR" doc:"Country."`
	Latitude             *float64   `json:"latitude,omitempty" doc:"Latitude."`
	Longitude            *float64   `json:"longitude,omitempty" doc:"Longitude."`
	GooglePlaceID        *string    `json:"google_place_id,omitempty" doc:"Google Place ID."`
	CoordinatesConfirmed bool       `json:"coordinates_confirmed" example:"true" doc:"Whether coordinates were confirmed."`
	MinBookingLeadMins   int        `json:"min_booking_lead_minutes" example:"60" doc:"Booking lead time."`
	MinCancelLeadMins    int        `json:"min_cancellation_lead_minutes" example:"240" doc:"Cancellation lead time."`
	ApprovalStatus       string     `json:"approval_status" example:"PENDING_APPROVAL" doc:"Approval status."`
	Enabled              bool       `json:"enabled" example:"false" doc:"Enabled switch."`
	SuspendedAt          *time.Time `json:"suspended_at,omitempty" doc:"Suspension timestamp."`
	CreatedAt            time.Time  `json:"created_at" example:"2026-10-20T10:00:00Z" doc:"Creation timestamp."`
	ArchivedAt           *time.Time `json:"archived_at,omitempty" doc:"Archival timestamp."`
}

type branchApprovalHistoryItem struct {
	ID              string     `json:"id" example:"019294f0-0000-7000-8000-000000000301" doc:"History UUID."`
	BranchID        string     `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	SubmittedBy     string     `json:"submitted_by" example:"019294f0-0000-7000-8000-000000000002" doc:"Submitter UUID."`
	SubmittedAt     time.Time  `json:"submitted_at" example:"2026-10-20T10:00:00Z" doc:"Submission timestamp."`
	ReviewedBy      *string    `json:"reviewed_by,omitempty" doc:"Reviewer UUID."`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty" doc:"Review timestamp."`
	Status          string     `json:"status" example:"PENDING" doc:"Status."`
	RejectionReason *string    `json:"rejection_reason,omitempty" doc:"Rejection reason if rejected."`
}

type getBranchRequestResponseBody struct {
	ID              string                      `json:"id" example:"019294f0-0000-7000-8000-000000000301" doc:"Request UUID."`
	BranchID        string                      `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	BusinessID      string                      `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	BusinessName    string                      `json:"business_name" example:"Club San Martín" doc:"Business name."`
	BranchName      string                      `json:"branch_name" example:"Sede Belgrano" doc:"Branch name."`
	SubmittedBy     string                      `json:"submitted_by" example:"019294f0-0000-7000-8000-000000000002" doc:"Submitter UUID."`
	SubmittedAt     time.Time                   `json:"submitted_at" example:"2026-10-20T10:00:00Z" doc:"Submission timestamp."`
	ReviewedBy      *string                     `json:"reviewed_by,omitempty" example:"019294f0-0000-7000-8000-000000000001" doc:"Reviewer UUID."`
	ReviewedAt      *time.Time                  `json:"reviewed_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Review timestamp."`
	Status          string                      `json:"status" example:"PENDING" doc:"Status."`
	RejectionReason *string                     `json:"rejection_reason,omitempty" example:"La dirección física no coincide con el mapa." doc:"Rejection reason."`
	Branch          branchDetailItem            `json:"branch" example:"{\"id\":\"019294f0-0000-7000-8000-000000000201\",\"business_id\":\"019294f0-0000-7000-8000-000000000101\",\"name\":\"Sede Belgrano\",\"slug\":\"sede-belgrano\",\"street_address\":\"Av. Libertador 4500\",\"city\":\"Buenos Aires\",\"state_province\":\"CABA\",\"postal_code\":\"C1425\",\"country\":\"AR\",\"coordinates_confirmed\":true,\"min_booking_lead_minutes\":60,\"min_cancellation_lead_minutes\":240,\"approval_status\":\"PENDING_APPROVAL\",\"enabled\":false,\"created_at\":\"2026-10-20T10:00:00Z\"}" doc:"Branch details."`
	History         []branchApprovalHistoryItem `json:"history" example:"[{\"id\":\"019294f0-0000-7000-8000-000000000301\",\"branch_id\":\"019294f0-0000-7000-8000-000000000201\",\"submitted_by\":\"019294f0-0000-7000-8000-000000000002\",\"submitted_at\":\"2026-10-20T10:00:00Z\",\"status\":\"PENDING\"}]" doc:"Prior review history."`
}

type getBranchRequestResponse struct {
	Body getBranchRequestResponseBody
}

type approveBranchRequestRequest struct {
	RequestID string `path:"requestId" example:"019294f0-0000-7000-8000-000000000301" doc:"Branch request UUID."`
}

type approveBranchRequestResponseBody struct {
	ID         string     `json:"id" example:"019294f0-0000-7000-8000-000000000301" doc:"Request UUID."`
	BranchID   string     `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	Status     string     `json:"status" example:"APPROVED" doc:"Updated status (APPROVED)."`
	ReviewedBy *string    `json:"reviewed_by,omitempty" example:"019294f0-0000-7000-8000-000000000001" doc:"Reviewer account UUID."`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Review timestamp in UTC."`
}

type approveBranchRequestResponse struct {
	Body approveBranchRequestResponseBody
}

type rejectBranchRequestRequestBody struct {
	RejectionReason string `json:"rejection_reason" example:"La dirección física no coincide con el mapa." doc:"Mandatory rejection reason."`
}

type rejectBranchRequestRequest struct {
	RequestID string `path:"requestId" example:"019294f0-0000-7000-8000-000000000301" doc:"Branch request UUID."`
	Body      rejectBranchRequestRequestBody
}

type rejectBranchRequestResponseBody struct {
	ID              string     `json:"id" example:"019294f0-0000-7000-8000-000000000301" doc:"Request UUID."`
	BranchID        string     `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	Status          string     `json:"status" example:"REJECTED" doc:"Updated status (REJECTED)."`
	RejectionReason *string    `json:"rejection_reason,omitempty" example:"La dirección física no coincide con el mapa." doc:"Recorded rejection reason."`
	ReviewedBy      *string    `json:"reviewed_by,omitempty" example:"019294f0-0000-7000-8000-000000000001" doc:"Reviewer account UUID."`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Review timestamp in UTC."`
}

type rejectBranchRequestResponse struct {
	Body rejectBranchRequestResponseBody
}

type listAdminBranchesRequest struct {
	BusinessID     string `query:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Filter by business UUID."`
	ApprovalStatus string `query:"approval_status" example:"APPROVED" doc:"Approval status filter: PENDING_APPROVAL, APPROVED, REJECTED."`
	Status         string `query:"status" example:"active" doc:"Operational status filter: active, suspended, archived, all."`
	City           string `query:"city" example:"Buenos Aires" doc:"Filter by city."`
	Limit          int    `query:"limit" default:"20" example:"20" doc:"Page size, from 1 to 100."`
	Page           int    `query:"page" default:"1" example:"1" doc:"One-based page number."`
}

type adminBranchViewItem struct {
	ID                   string     `json:"id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	BusinessID           string     `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	BusinessName         string     `json:"business_name,omitempty" example:"Club San Martín" doc:"Business name."`
	Name                 string     `json:"name" example:"Sede Belgrano" doc:"Branch name."`
	Slug                 string     `json:"slug" example:"sede-belgrano" doc:"Branch slug."`
	City                 string     `json:"city" example:"Buenos Aires" doc:"City."`
	StateProvince        string     `json:"state_province" example:"CABA" doc:"State or province."`
	ApprovalStatus       string     `json:"approval_status" example:"APPROVED" doc:"Approval status."`
	CoordinatesConfirmed bool       `json:"coordinates_confirmed" example:"true" doc:"Whether coordinates are confirmed."`
	Enabled              bool       `json:"enabled" example:"true" doc:"Owner switch."`
	IsPublished          bool       `json:"is_published" example:"true" doc:"Whether branch is published."`
	SuspendedAt          *time.Time `json:"suspended_at,omitempty" doc:"Suspension timestamp in UTC."`
	ArchivedAt           *time.Time `json:"archived_at,omitempty" doc:"Archival timestamp in UTC."`
	CreatedAt            time.Time  `json:"created_at" example:"2026-10-20T10:00:00Z" doc:"Creation timestamp in UTC."`
}

type listAdminBranchesResponseBody struct {
	Items []adminBranchViewItem `json:"items" example:"[{\"id\":\"019294f0-0000-7000-8000-000000000201\",\"business_id\":\"019294f0-0000-7000-8000-000000000101\",\"business_name\":\"Club San Martín\",\"name\":\"Sede Belgrano\",\"slug\":\"sede-belgrano\",\"city\":\"Buenos Aires\",\"state_province\":\"CABA\",\"approval_status\":\"APPROVED\",\"coordinates_confirmed\":true,\"enabled\":true,\"is_published\":true,\"created_at\":\"2026-10-20T10:00:00Z\"}]" doc:"List of branch views."`
	Total int                   `json:"total" example:"3" doc:"Total count."`
	Limit int                   `json:"limit" example:"20" doc:"Page limit."`
	Page  int                   `json:"page" example:"1" doc:"One-based page number."`
}

type listAdminBranchesResponse struct {
	Body listAdminBranchesResponseBody
}

type suspendBranchRequestBody struct {
	Reason string `json:"reason" example:"Mantenimiento de urgencia." doc:"Suspension reason."`
}

type suspendBranchRequest struct {
	BranchID string `path:"branchId" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	Body     suspendBranchRequestBody
}

type suspendBranchResponseBody struct {
	ID          string     `json:"id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	SuspendedAt *time.Time `json:"suspended_at,omitempty" example:"2026-10-20T10:00:00Z" doc:"Suspension timestamp in UTC."`
}

type suspendBranchResponse struct {
	Body suspendBranchResponseBody
}

type reactivateBranchRequest struct {
	BranchID string `path:"branchId" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
}

type reactivateBranchResponseBody struct {
	ID          string     `json:"id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	SuspendedAt *time.Time `json:"suspended_at" nullable:"true" example:"2026-10-20T10:00:00Z" doc:"Suspension timestamp (null)."`
}

type reactivateBranchResponse struct {
	Body reactivateBranchResponseBody
}
