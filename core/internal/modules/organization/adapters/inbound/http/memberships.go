package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"canchalibre/core/internal/modules/organization/core/usecase"
	platformhttp "canchalibre/core/internal/platform/http"
)

func registerInviteMember(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "inviteMember",
		RequestBody:   &huma.RequestBody{Description: "Email, intended role and optional branch of the member to invite."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/businesses/{businessId}/invitations",
		Summary:       "Invite a business member",
		Description:   "OWNER-only. Issues an OWNER invitation without a branch, or an EMPLOYEE invitation scoped to one branch.",
		Tags:          []string{platformhttp.TagOrganization},
		DefaultStatus: nethttp.StatusCreated,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err404NotFound,
			err409Conflict, err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *inviteMemberRequest) (*inviteMemberResponse, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		out, err := deps.Service.InviteMember(ctx, principal, in.BusinessID, usecase.InviteMemberInput{
			Email:        in.Body.Email,
			IntendedRole: in.Body.IntendedRole,
			BranchID:     in.Body.BranchID,
		})
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &inviteMemberResponse{}
		response.Body.InvitationID = out.InvitationID
		response.Body.ExpiresAt = out.ExpiresAt
		return response, nil
	})
}

type inviteMemberRequest struct {
	BusinessID string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID that issues the invitation."`
	Body       inviteMemberRequestBody
}

type inviteMemberRequestBody struct {
	Email        string  `json:"email" example:"employee2@canchalibre.local" doc:"Email address of the member to invite."`
	IntendedRole string  `json:"intended_role" enum:"OWNER,EMPLOYEE" example:"EMPLOYEE" doc:"Role to grant: OWNER or EMPLOYEE."`
	BranchID     *string `json:"branch_id,omitempty" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID, required for EMPLOYEE and forbidden for OWNER."`
}

type inviteMemberResponseBody struct {
	InvitationID string    `json:"invitation_id" example:"019294f0-0000-7000-8000-000000000701" doc:"Invitation UUID."`
	ExpiresAt    time.Time `json:"expires_at" example:"2026-10-17T13:00:00Z" doc:"Invitation expiry in UTC."`
}

type inviteMemberResponse struct {
	Body inviteMemberResponseBody
}

func registerRevokeMembership(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "revokeMembership",
		Method:        nethttp.MethodDelete,
		Path:          "/api/v1/businesses/{businessId}/memberships/{membershipId}",
		Summary:       "Revoke a business membership",
		Description:   "OWNER-only and idempotent. Refuses with 409 last-co-owner when the revocation would leave the business without an active co-owner.",
		Tags:          []string{platformhttp.TagOrganization},
		DefaultStatus: nethttp.StatusNoContent,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err409LastCoOwner, err500Internal,
		),
	}, func(ctx context.Context, in *revokeMembershipRequest) (*emptyOutput, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := deps.Service.RevokeMembership(ctx, principal, in.BusinessID, in.MembershipID); err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &emptyOutput{}, nil
	})
}

type revokeMembershipRequest struct {
	BusinessID   string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	MembershipID string `path:"membershipId" example:"019294f0-0000-7000-8000-000000000301" doc:"Membership UUID to revoke."`
}

func registerRevokeInvitation(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "revokeInvitation",
		Method:        nethttp.MethodDelete,
		Path:          "/api/v1/businesses/{businessId}/invitations/{invitationId}",
		Summary:       "Revoke a pending work invitation",
		Description:   "OWNER-only and idempotent. An unknown invitation and an invitation outside the actor's scope both answer 404.",
		Tags:          []string{platformhttp.TagOrganization},
		DefaultStatus: nethttp.StatusNoContent,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err403Forbidden, err404NotFound, err500Internal,
		),
	}, func(ctx context.Context, in *revokeInvitationRequest) (*emptyOutput, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := deps.Service.RevokeInvitation(ctx, principal, in.BusinessID, in.InvitationID); err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &emptyOutput{}, nil
	})
}

type revokeInvitationRequest struct {
	BusinessID   string `path:"businessId" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	InvitationID string `path:"invitationId" example:"019294f0-0000-7000-8000-000000000701" doc:"Invitation UUID to revoke."`
}
