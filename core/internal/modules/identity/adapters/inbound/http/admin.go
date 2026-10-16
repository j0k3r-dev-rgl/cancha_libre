package http

import (
	"context"
	nethttp "net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	platformhttp "canchalibre/core/internal/platform/http"
)

func registerInviteSupportOperator(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "inviteSupportOperator",
		RequestBody:   &huma.RequestBody{Description: "Email address of the support operator to invite."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/admin/support-operators",
		Summary:       "Invite a support operator",
		Description:   "ADMIN-only. Issues a single-use SOPORTE invitation, activated through POST /api/v1/auth/invitations/activate.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusCreated,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err409Conflict,
			err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *inviteSupportOperatorRequest) (*inviteSupportOperatorResponse, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		invitation, _, err := deps.Service.InviteSupportOperator(ctx, principal, in.Body.Email)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &inviteSupportOperatorResponse{}
		response.Body.InvitationID = invitation.ID
		response.Body.ExpiresAt = invitation.ExpiresAt
		return response, nil
	})
}

type inviteSupportOperatorRequestBody struct {
	Email string `json:"email" example:"soporte2@canchalibre.local" doc:"Email address of the support operator to invite."`
}

type inviteSupportOperatorRequest struct {
	Body inviteSupportOperatorRequestBody
}

type inviteSupportOperatorResponseBody struct {
	InvitationID string    `json:"invitation_id" example:"019294f0-0000-7000-8000-000000000701" doc:"Invitation UUID."`
	ExpiresAt    time.Time `json:"expires_at" example:"2026-10-17T13:00:00Z" doc:"Invitation expiry in UTC."`
}

type inviteSupportOperatorResponse struct {
	Body inviteSupportOperatorResponseBody
}

func registerRevokeSupportOperator(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "revokeSupportOperator",
		Method:        nethttp.MethodDelete,
		Path:          "/api/v1/admin/support-operators/{accountId}",
		Summary:       "Revoke a support operator",
		Description:   "ADMIN-only and idempotent. Closes a SOPORTE account and revokes its active refresh sessions.",
		Tags:          []string{platformhttp.TagAdmin},
		DefaultStatus: nethttp.StatusNoContent,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden, err404NotFound, err500Internal,
		),
	}, func(ctx context.Context, in *revokeSupportOperatorRequest) (*emptyOutput, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := deps.Service.RevokeSupportOperator(ctx, principal, in.AccountID); err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &emptyOutput{}, nil
	})
}

type revokeSupportOperatorRequest struct {
	AccountID string `path:"accountId" example:"019294f0-0000-7000-8000-000000000005" doc:"UUID of the SOPORTE account to close."`
}
