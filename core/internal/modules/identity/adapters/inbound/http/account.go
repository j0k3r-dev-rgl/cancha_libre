package http

import (
	"context"
	nethttp "net/http"

	"github.com/danielgtaylor/huma/v2"

	platformhttp "canchalibre/core/internal/platform/http"
)

func registerGetOwnAccount(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "getOwnAccount",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/me",
		Summary:       "Read the authenticated account profile",
		Description:   "Returns the profile of the authenticated actor. The per-request guard is the authority on account status, so a closed or suspended account is refused with 403 before this operation runs.",
		Tags:          []string{platformhttp.TagAccount},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err500Internal,
		),
	}, func(ctx context.Context, _ *emptyInput) (*getOwnAccountResponse, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		account, err := deps.Service.GetOwnAccount(ctx, principal)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &getOwnAccountResponse{}
		response.Body.AccountID = account.ID
		response.Body.Email = account.Email
		response.Body.Kind = string(account.Kind)
		response.Body.Status = account.Status
		response.Body.DisplayName = account.DisplayName
		return response, nil
	})
}

// emptyInput is the input of an operation that takes no body and no parameters.
type emptyInput struct{}

type getOwnAccountResponseBody struct {
	AccountID   string  `json:"account_id" example:"019294f0-0000-7000-8000-000000000002" doc:"Account UUID."`
	Email       string  `json:"email" example:"owner@canchalibre.local" doc:"Account email address."`
	Kind        string  `json:"kind" example:"WORK" doc:"Account kind: WORK or CUSTOMER."`
	Status      string  `json:"status" example:"ACTIVE" doc:"Account lifecycle status."`
	DisplayName *string `json:"display_name" example:"Ana Gomez" doc:"Display name, or null when unset."`
}

type getOwnAccountResponse struct {
	Body getOwnAccountResponseBody
}

func registerListOwnContexts(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "listOwnContexts",
		Method:        nethttp.MethodGet,
		Path:          "/api/v1/me/contexts",
		Summary:       "List the authenticated actor's work contexts",
		Description:   "Returns the active business and branch contexts available to the authenticated actor. Platform and customer actors receive an empty list.",
		Tags:          []string{platformhttp.TagAccount},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err500Internal,
		),
	}, func(ctx context.Context, _ *emptyInput) (*listOwnContextsResponse, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		contexts, err := deps.Service.ListOwnContexts(ctx, principal)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &listOwnContextsResponse{}
		response.Body = make([]contextItem, 0, len(contexts))
		for _, item := range contexts {
			response.Body = append(response.Body, contextItem{
				BusinessID:   item.BusinessID,
				BusinessName: item.BusinessName,
				BranchID:     item.BranchID,
				BranchName:   item.BranchName,
				Role:         string(item.Role),
			})
		}
		return response, nil
	})
}

type contextItem struct {
	BusinessID   string `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	BusinessName string `json:"business_name" example:"Cancha Libre Dev" doc:"Business display name."`
	BranchID     string `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	BranchName   string `json:"branch_name" example:"Central" doc:"Branch display name."`
	Role         string `json:"role" example:"OWNER" doc:"Membership role in this context."`
}

type listOwnContextsResponse struct {
	Body []contextItem
}

func registerSelectContext(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "selectContext",
		RequestBody:   &huma.RequestBody{Description: "Business and branch to activate for the authenticated actor."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/me/context",
		Summary:       "Select the active business and branch context",
		Description:   "Verifies an active membership for the requested business and branch, then issues a new access token carrying that ctx claim. The claim is never trusted without this check.",
		Tags:          []string{platformhttp.TagAccount},
		DefaultStatus: nethttp.StatusOK,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403ContextNotAuthorized,
			err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *selectContextRequest) (*selectContextResponse, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		accessToken, expiresIn, err := deps.Service.SelectContext(ctx, principal, in.Body.BusinessID, in.Body.BranchID)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &selectContextResponse{}
		response.Body.AccessToken = accessToken
		response.Body.ExpiresIn = expiresIn
		return response, nil
	})
}

type selectContextRequestBody struct {
	BusinessID string `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID to activate."`
	BranchID   string `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID to activate inside that business."`
}

type selectContextRequest struct {
	Body selectContextRequestBody
}

type selectContextResponseBody struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIwMTkyOTRmMC0wMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDIifQ.signature" doc:"HS256 access token carrying the selected ctx claim."`
	ExpiresIn   int    `json:"expires_in" example:"1800" doc:"Access token lifetime in seconds."`
}

type selectContextResponse struct {
	Body selectContextResponseBody
}
