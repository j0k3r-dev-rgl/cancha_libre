package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"canchalibre/core/internal/modules/organization/core/usecase"
	platformhttp "canchalibre/core/internal/platform/http"
)

func registerAssignCoOwner(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "assignCoOwner",
		RequestBody:   &huma.RequestBody{Description: "Existing account_id, or email plus intended_role OWNER."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/businesses/{businessId}/co-owners",
		Summary:       "Assign a business co-owner",
		Description:   "ADMIN-only. Accepts either an existing work account_id or an email plus intended_role OWNER, never both.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusCreated,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err404NotFound,
			err409Conflict, err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *assignCoOwnerRequest) (*assignCoOwnerResponse, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		membership, err := deps.Service.AssignCoOwner(ctx, principal, in.BusinessID, usecase.AssignCoOwnerInput{
			AccountID:    in.Body.AccountID,
			Email:        in.Body.Email,
			IntendedRole: in.Body.IntendedRole,
		})
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &assignCoOwnerResponse{}
		response.Body.MembershipID = membership.ID
		response.Body.Role = string(membership.Role)
		response.Body.BusinessID = membership.BusinessID
		return response, nil
	})
}

type assignCoOwnerRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID that receives the co-owner."`
	Body       assignCoOwnerRequestBody
}

type assignCoOwnerRequestBody struct {
	AccountID    string `json:"account_id,omitempty" example:"019294f0-0000-7000-8000-000000000002" doc:"Existing work account UUID; mutually exclusive with email."`
	Email        string `json:"email,omitempty" example:"owner2@canchalibre.local" doc:"Email of the co-owner to invite; requires intended_role OWNER."`
	IntendedRole string `json:"intended_role,omitempty" enum:"OWNER" example:"OWNER" doc:"Role of the email path; must be OWNER."`
}

type assignCoOwnerResponseBody struct {
	MembershipID string `json:"membership_id" example:"019294f0-0000-7000-8000-000000000301" doc:"Created membership UUID."`
	Role         string `json:"role" example:"OWNER" doc:"Role granted by the membership."`
	BusinessID   string `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID the membership belongs to."`
}

type assignCoOwnerResponse struct {
	Body assignCoOwnerResponseBody
}

func registerRevokeCoOwner(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "revokeCoOwner",
		Method:        nethttp.MethodDelete,
		Path:          "/api/v1/admin/businesses/{businessId}/co-owners/{accountId}",
		Summary:       "Revoke a business co-owner",
		Description:   "ADMIN-only and idempotent. Refuses with 409 last-co-owner when the revocation would leave the business without an active co-owner.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusNoContent,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err409LastCoOwner, err500Internal,
		),
	}, func(ctx context.Context, in *revokeCoOwnerRequest) (*emptyOutput, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := deps.Service.RevokeCoOwner(ctx, principal, in.BusinessID, in.AccountID); err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &emptyOutput{}, nil
	})
}

type revokeCoOwnerRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	AccountID  string `path:"accountId" example:"019294f0-0000-7000-8000-000000000002" doc:"UUID of the co-owner account to revoke."`
}

func registerListMemberships(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "listMemberships",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/businesses/{businessId}/memberships",
		Summary:       "List a business's active memberships",
		Description:   "OWNER-only. An unknown business and a business outside the actor's scope both answer 404, indistinguishable on purpose.",
		Tags:          []string{platformhttp.TagOrganization},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err500Internal,
		),
	}, func(ctx context.Context, in *listMembershipsRequest) (*listMembershipsResponse, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		memberships, err := deps.Service.ListMemberships(ctx, principal, in.BusinessID)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &listMembershipsResponse{}
		response.Body = make([]membershipItem, 0, len(memberships))
		for _, membership := range memberships {
			response.Body = append(response.Body, membershipItem{
				MembershipID: membership.ID,
				AccountID:    membership.AccountID,
				Role:         string(membership.Role),
				BranchID:     membership.BranchID,
				CreatedAt:    membership.CreatedAt,
			})
		}
		return response, nil
	})
}

type listMembershipsRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID whose memberships are listed."`
}

type membershipItem struct {
	MembershipID string    `json:"membership_id" example:"019294f0-0000-7000-8000-000000000301" doc:"Membership UUID."`
	AccountID    string    `json:"account_id" example:"019294f0-0000-7000-8000-000000000002" doc:"Account UUID of the member."`
	Role         string    `json:"role" example:"EMPLOYEE" doc:"Membership role: OWNER or EMPLOYEE."`
	BranchID     *string   `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID for an EMPLOYEE membership, null for a business-wide OWNER."`
	CreatedAt    time.Time `json:"created_at" example:"2026-10-16T13:00:00Z" doc:"Membership creation time in UTC."`
}

type listMembershipsResponse struct {
	Body []membershipItem
}
