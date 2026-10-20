package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"canchalibre/core/internal/modules/identity/core/ports"
	platformhttp "canchalibre/core/internal/platform/http"
)

func registerAdminUsers(api huma.API, deps Deps) {
	// 16. listUsers
	huma.Register(api, huma.Operation{
		OperationID:   "listUsers",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/admin/users",
		Summary:       "List platform users",
		Description:   "ADMIN-only. Returns paginated list of accounts across all roles with status and membership counts.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *listUsersRequest) (*listUsersResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		offset, err := platformhttp.PageOffset(in.Page, in.Limit)
		if err != nil {
			return nil, err
		}
		items, total, err := deps.Service.ListUsers(ctx, p, ports.UserFilter{
			Kind:   in.Kind,
			Role:   in.Role,
			Status: in.Status,
			Search: in.Search,
			Limit:  in.Limit,
			Offset: offset,
		})
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		viewItems := make([]userSummaryItemResponse, 0, len(items))
		for _, u := range items {
			var roleStr *string
			if u.PlatformRole != nil {
				r := string(*u.PlatformRole)
				roleStr = &r
			}
			viewItems = append(viewItems, userSummaryItemResponse{
				ID:               u.ID,
				Email:            u.Email,
				DisplayName:      u.DisplayName,
				Kind:             string(u.Kind),
				PlatformRole:     roleStr,
				Status:           u.Status,
				CreatedAt:        u.CreatedAt,
				MembershipsCount: u.MembershipsCount,
			})
		}
		return &listUsersResponse{
			Body: listUsersResponseBody{
				Items: viewItems,
				Total: total,
				Limit: in.Limit,
				Page:  in.Page,
			},
		}, nil
	})

	// 17. getUser
	huma.Register(api, huma.Operation{
		OperationID:   "getUser",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/admin/users/{userId}",
		Summary:       "Get user detail",
		Description:   "ADMIN-only. Returns detailed profile and active memberships of an account.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err500Internal,
		),
	}, func(ctx context.Context, in *getUserRequest) (*getUserResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		detail, err := deps.Service.GetUser(ctx, p, in.UserID)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		var roleStr *string
		if detail.PlatformRole != nil {
			r := string(*detail.PlatformRole)
			roleStr = &r
		}

		memberships := make([]userMembershipItemResponse, 0, len(detail.Memberships))
		for _, m := range detail.Memberships {
			memberships = append(memberships, userMembershipItemResponse{
				ID:           m.ID,
				BusinessID:   m.BusinessID,
				BusinessName: m.BusinessName,
				BranchID:     m.BranchID,
				BranchName:   m.BranchName,
				Role:         string(m.Role),
				CreatedAt:    m.CreatedAt,
			})
		}

		return &getUserResponse{
			Body: getUserResponseBody{
				ID:           detail.ID,
				Email:        detail.Email,
				DisplayName:  detail.DisplayName,
				Kind:         string(detail.Kind),
				PlatformRole: roleStr,
				Status:       detail.Status,
				CreatedAt:    detail.CreatedAt,
				Memberships:  memberships,
			},
		}, nil
	})

	// 18. suspendUser
	huma.Register(api, huma.Operation{
		OperationID:   "suspendUser",
		RequestBody:   &huma.RequestBody{Description: "Suspension reason."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/users/{userId}/suspend",
		Summary:       "Suspend user account",
		Description:   "ADMIN-only. Administratively suspends an account and revokes active sessions.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err404NotFound,
			err409Conflict, err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *suspendUserRequest) (*suspendUserResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := deps.Service.SuspendUser(ctx, p, in.UserID, in.Body.Reason); err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &suspendUserResponse{
			Body: suspendUserResponseBody{
				ID:     in.UserID,
				Status: "SUSPENDED",
			},
		}, nil
	})

	// 19. reactivateUser
	huma.Register(api, huma.Operation{
		OperationID:   "reactivateUser",
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/users/{userId}/reactivate",
		Summary:       "Reactivate user account",
		Description:   "ADMIN-only. Lifts administrative suspension of an account.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err409Conflict, err500Internal,
		),
	}, func(ctx context.Context, in *reactivateUserRequest) (*reactivateUserResponse, error) {
		p, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := deps.Service.ReactivateUser(ctx, p, in.UserID); err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &reactivateUserResponse{
			Body: reactivateUserResponseBody{
				ID:     in.UserID,
				Status: "ACTIVE",
			},
		}, nil
	})
}

// Request and Response DTOs

type userSummaryItemResponse struct {
	ID               string    `json:"id" example:"019294f0-0000-7000-8000-000000000002" doc:"Account UUID."`
	Email            string    `json:"email" example:"owner@canchalibre.local" doc:"Email address."`
	DisplayName      *string   `json:"display_name,omitempty" example:"Juan Pérez" doc:"Display name."`
	Kind             string    `json:"kind" example:"WORK" doc:"Account kind: WORK or CUSTOMER."`
	PlatformRole     *string   `json:"platform_role,omitempty" example:"ADMIN" doc:"Platform role if ADMIN or SOPORTE."`
	Status           string    `json:"status" example:"ACTIVE" doc:"Account status: ACTIVE, SUSPENDED, CLOSED."`
	CreatedAt        time.Time `json:"created_at" example:"2026-10-20T10:00:00Z" doc:"Creation timestamp in UTC."`
	MembershipsCount int       `json:"memberships_count" example:"1" doc:"Number of active memberships."`
}

type listUsersRequest struct {
	Kind   string `query:"kind" example:"WORK" doc:"Filter by account kind: WORK or CUSTOMER."`
	Role   string `query:"role" example:"ADMIN" doc:"Filter by role."`
	Status string `query:"status" example:"ACTIVE" doc:"Filter by account status."`
	Search string `query:"search" example:"owner" doc:"Search pattern by email or display name."`
	Limit  int    `query:"limit" default:"20" example:"20" doc:"Page size, from 1 to 100."`
	Page   int    `query:"page" default:"1" example:"1" doc:"One-based page number."`
}

type listUsersResponseBody struct {
	Items []userSummaryItemResponse `json:"items" example:"[{\"id\":\"019294f0-0000-7000-8000-000000000002\",\"email\":\"owner@canchalibre.local\",\"display_name\":\"Juan Pérez\",\"kind\":\"WORK\",\"platform_role\":\"ADMIN\",\"status\":\"ACTIVE\",\"created_at\":\"2026-10-20T10:00:00Z\",\"memberships_count\":1}]" doc:"List of user summaries."`
	Total int                       `json:"total" example:"5" doc:"Total count."`
	Limit int                       `json:"limit" example:"20" doc:"Page limit."`
	Page  int                       `json:"page" example:"1" doc:"One-based page number."`
}

type listUsersResponse struct {
	Body listUsersResponseBody
}

type getUserRequest struct {
	UserID string `path:"userId" example:"019294f0-0000-7000-8000-000000000002" doc:"User account UUID."`
}

type userMembershipItemResponse struct {
	ID           string    `json:"id" example:"019294f0-0000-7000-8000-000000000301" doc:"Membership UUID."`
	BusinessID   string    `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	BusinessName string    `json:"business_name" example:"Club San Martín" doc:"Business commercial name."`
	BranchID     *string   `json:"branch_id,omitempty" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID if scoped."`
	BranchName   *string   `json:"branch_name,omitempty" example:"Sede Belgrano" doc:"Branch name if scoped."`
	Role         string    `json:"role" example:"OWNER" doc:"Membership role."`
	CreatedAt    time.Time `json:"created_at" example:"2026-10-20T10:00:00Z" doc:"Creation timestamp in UTC."`
}

type getUserResponseBody struct {
	ID           string                       `json:"id" example:"019294f0-0000-7000-8000-000000000002" doc:"Account UUID."`
	Email        string                       `json:"email" example:"owner@canchalibre.local" doc:"Email address."`
	DisplayName  *string                      `json:"display_name,omitempty" example:"Juan Pérez" doc:"Display name."`
	Kind         string                       `json:"kind" example:"WORK" doc:"Account kind."`
	PlatformRole *string                      `json:"platform_role,omitempty" example:"ADMIN" doc:"Platform role."`
	Status       string                       `json:"status" example:"ACTIVE" doc:"Account status."`
	CreatedAt    time.Time                    `json:"created_at" example:"2026-10-20T10:00:00Z" doc:"Creation timestamp in UTC."`
	Memberships  []userMembershipItemResponse `json:"memberships" example:"[{\"id\":\"019294f0-0000-7000-8000-000000000301\",\"business_id\":\"019294f0-0000-7000-8000-000000000101\",\"business_name\":\"Club San Martín\",\"role\":\"OWNER\",\"created_at\":\"2026-10-20T10:00:00Z\"}]" doc:"User memberships."`
}

type getUserResponse struct {
	Body getUserResponseBody
}

type suspendUserRequestBody struct {
	Reason string `json:"reason" example:"Infracción reiterada de políticas." doc:"Suspension reason."`
}

type suspendUserRequest struct {
	UserID string `path:"userId" example:"019294f0-0000-7000-8000-000000000002" doc:"Account UUID to suspend."`
	Body   suspendUserRequestBody
}

type suspendUserResponseBody struct {
	ID     string `json:"id" example:"019294f0-0000-7000-8000-000000000002" doc:"Account UUID."`
	Status string `json:"status" example:"SUSPENDED" doc:"Updated status."`
}

type suspendUserResponse struct {
	Body suspendUserResponseBody
}

type reactivateUserRequest struct {
	UserID string `path:"userId" example:"019294f0-0000-7000-8000-000000000002" doc:"Account UUID to reactivate."`
}

type reactivateUserResponseBody struct {
	ID     string `json:"id" example:"019294f0-0000-7000-8000-000000000002" doc:"Account UUID."`
	Status string `json:"status" example:"ACTIVE" doc:"Updated status."`
}

type reactivateUserResponse struct {
	Body reactivateUserResponseBody
}
