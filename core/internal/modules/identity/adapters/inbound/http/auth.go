package http

import (
	"context"
	nethttp "net/http"

	"github.com/danielgtaylor/huma/v2"

	"canchalibre/core/internal/modules/identity/core/domain"
	"canchalibre/core/internal/modules/identity/core/usecase"
	platformhttp "canchalibre/core/internal/platform/http"
)

// emptyOutput is the output of an operation that returns no response body.
type emptyOutput struct{}

func registerLogin(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "login",
		RequestBody:   &huma.RequestBody{Description: "Authenticated credentials, the account kind and the requested session."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/auth/login",
		Summary:       "Authenticate an account",
		Description:   "Verifies the credentials of a WORK or CUSTOMER account, then issues an HS256 access token and a rotating refresh session cookie.",
		Tags:          []string{platformhttp.TagAuth},
		DefaultStatus: nethttp.StatusOK,
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err403Forbidden,
			err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *loginRequest) (*loginResponse, error) {
		out, err := deps.Service.Login(ctx, usecase.LoginInput{
			Email:       in.Body.Email,
			Password:    in.Body.Password,
			AccountKind: in.Body.AccountKind,
		})
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &loginResponse{}
		response.SetCookie = newRefreshCookie(out.RefreshToken, out.RefreshExpiresAt)
		response.Body.AccessToken = out.AccessToken
		response.Body.ExpiresIn = out.ExpiresIn
		response.Body.Context = loginContextItems(out.Context)
		return response, nil
	})
}

type loginRequestBody struct {
	Email       string `json:"email" example:"owner@canchalibre.local" doc:"Account email address."`
	Password    string `json:"password" example:"correct horse battery" doc:"Account password, 12 to 128 characters after NFC normalization."`
	AccountKind string `json:"accountKind,omitempty" enum:"WORK,CUSTOMER" example:"WORK" doc:"Account kind; omit to try both kinds of the same email."`
}

type loginRequest struct {
	Body loginRequestBody
}

type loginContextItem struct {
	BusinessID string `json:"business_id" example:"019294f0-0000-7000-8000-000000000101" doc:"Business UUID."`
	BranchID   string `json:"branch_id" example:"019294f0-0000-7000-8000-000000000201" doc:"Branch UUID."`
	Role       string `json:"role" example:"OWNER" doc:"Membership role in this context."`
}

type loginResponseBody struct {
	AccessToken string             `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIwMTkyOTRmMC0wMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDIifQ.signature" doc:"HS256 access token to send as a Bearer credential."`
	ExpiresIn   int                `json:"expires_in" example:"1800" doc:"Access token lifetime in seconds."`
	Context     []loginContextItem `json:"context" example:"[{\"business_id\":\"019294f0-0000-7000-8000-000000000101\",\"branch_id\":\"019294f0-0000-7000-8000-000000000201\",\"role\":\"OWNER\"}]" doc:"Available work contexts; null for platform and customer actors."`
}

type loginResponse struct {
	SetCookie nethttp.Cookie `header:"Set-Cookie"`
	Body      loginResponseBody
}

// loginContextItems converts the use-case context list into the documented body
// shape, preserving null for actors with no branch context.
func loginContextItems(items []domain.LoginContextItem) []loginContextItem {
	if items == nil {
		return nil
	}
	out := make([]loginContextItem, 0, len(items))
	for _, item := range items {
		out = append(out, loginContextItem{
			BusinessID: item.BusinessID,
			BranchID:   item.BranchID,
			Role:       string(item.Role),
		})
	}
	return out
}

func registerRefresh(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "refresh",
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/auth/refresh",
		Summary:       "Rotate a refresh session",
		Description:   "Exchanges the HttpOnly refresh cookie for a new access token and a rotated refresh cookie. Presenting an already rotated token revokes the whole session family.",
		Tags:          []string{platformhttp.TagAuth},
		DefaultStatus: nethttp.StatusOK,
		Responses: errorResponses(api,
			err401Unauthenticated, err401RefreshReuse, err500Internal,
		),
	}, func(ctx context.Context, in *refreshRequest) (*refreshResponse, error) {
		out, err := deps.Service.Refresh(ctx, in.Cookie)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &refreshResponse{}
		response.SetCookie = newRefreshCookie(out.RefreshToken, out.RefreshExpiresAt)
		response.Body.AccessToken = out.AccessToken
		response.Body.ExpiresIn = out.ExpiresIn
		return response, nil
	})
}

type refreshRequest struct {
	Cookie string `cookie:"cl_refresh_token" doc:"Rotated HttpOnly session cookie."`
}

type refreshResponseBody struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIwMTkyOTRmMC0wMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDIifQ.signature" doc:"Rotated HS256 access token."`
	ExpiresIn   int    `json:"expires_in" example:"1800" doc:"Access token lifetime in seconds."`
}

type refreshResponse struct {
	SetCookie nethttp.Cookie `header:"Set-Cookie"`
	Body      refreshResponseBody
}

func registerActivateInvitation(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "activateInvitation",
		RequestBody:   &huma.RequestBody{Description: "Single-use invitation token and the new account password."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/auth/invitations/activate",
		Summary:       "Activate a single-use invitation",
		Description:   "Consumes an ADMIN, OWNER, EMPLOYEE or SOPORTE invitation, sets the account password and activates the account.",
		Tags:          []string{platformhttp.TagAuth},
		DefaultStatus: nethttp.StatusOK,
		Responses: errorResponses(api,
			err400Validation, err403Forbidden, err409InvitationConsumed, err409InvitationExpired,
			err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *activateInvitationRequest) (*activateInvitationResponse, error) {
		account, err := deps.Service.ActivateInvitation(ctx, in.Body.Token, in.Body.Password)
		if err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &activateInvitationResponse{}
		response.Body.AccountID = account.ID
		response.Body.Status = account.Status
		return response, nil
	})
}

type activateInvitationRequestBody struct {
	Token    string `json:"token" example:"J3fQ1n0p8sV5cW2xZ7mL4tR9yB6kA1dH" doc:"Single-use invitation token."`
	Password string `json:"password" example:"correct horse battery" doc:"New password, 12 to 128 characters after NFC normalization."`
}

type activateInvitationRequest struct {
	Body activateInvitationRequestBody
}

type activateInvitationResponseBody struct {
	AccountID string `json:"account_id" example:"019294f0-0000-7000-8000-000000000002" doc:"Activated account UUID."`
	Status    string `json:"status" example:"ACTIVE" doc:"Account status after activation."`
}

type activateInvitationResponse struct {
	Body activateInvitationResponseBody
}

func registerRequestPasswordRecovery(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "requestPasswordRecovery",
		RequestBody:   &huma.RequestBody{Description: "Email and account kind that own the password to recover."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/auth/password/recovery",
		Summary:       "Request a password recovery token",
		Description:   "Returns the same 202 answer whether or not the account exists, so the response never discloses account existence.",
		Tags:          []string{platformhttp.TagAuth},
		DefaultStatus: nethttp.StatusAccepted,
		Responses: errorResponses(api,
			err400Validation, err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *requestPasswordRecoveryRequest) (*emptyOutput, error) {
		if err := deps.Service.RequestPasswordRecovery(ctx, in.Body.Email, in.Body.AccountKind); err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &emptyOutput{}, nil
	})
}

type requestPasswordRecoveryRequestBody struct {
	Email       string `json:"email" example:"owner@canchalibre.local" doc:"Account email address."`
	AccountKind string `json:"accountKind" enum:"WORK,CUSTOMER" example:"WORK" doc:"Account kind that owns the email."`
}

type requestPasswordRecoveryRequest struct {
	Body requestPasswordRecoveryRequestBody
}

func registerResetPassword(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "resetPassword",
		RequestBody:   &huma.RequestBody{Description: "Single-use recovery token and the replacement password."},
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/auth/password/reset",
		Summary:       "Reset a password with a single-use token",
		Description:   "Consumes a single-use password recovery token and replaces the account credential. An invalid or expired token answers 401 with a single type, so the reason is not disclosed.",
		Tags:          []string{platformhttp.TagAuth},
		DefaultStatus: nethttp.StatusOK,
		Responses: errorResponses(api,
			err400Validation, err401Unauthenticated, err415UnsupportedMediaType, err422Validation, err500Internal,
		),
	}, func(ctx context.Context, in *resetPasswordRequest) (*emptyOutput, error) {
		if err := deps.Service.ResetPassword(ctx, in.Body.Token, in.Body.Password); err != nil {
			return nil, deps.fail(ctx, err)
		}
		return &emptyOutput{}, nil
	})
}

type resetPasswordRequestBody struct {
	Token    string `json:"token" example:"J3fQ1n0p8sV5cW2xZ7mL4tR9yB6kA1dH" doc:"Single-use password recovery token."`
	Password string `json:"password" example:"correct horse battery" doc:"New password, 12 to 128 characters after NFC normalization."`
}

type resetPasswordRequest struct {
	Body resetPasswordRequestBody
}

func registerLogout(api huma.API, deps Deps) {
	huma.Register(api, huma.Operation{
		OperationID:   "logout",
		Method:        nethttp.MethodPost,
		Path:          "/api/v1/auth/logout",
		Summary:       "Revoke the current session",
		Description:   "Revokes the caller's refresh session and clears the HttpOnly session cookie. Revoking an already revoked session still succeeds.",
		Tags:          []string{platformhttp.TagAuth},
		DefaultStatus: nethttp.StatusNoContent,
		Middlewares:   deps.guarded(api),
		Responses: errorResponses(api,
			err401Unauthenticated, err500Internal,
		),
	}, func(ctx context.Context, in *logoutRequest) (*logoutResponse, error) {
		principal, err := deps.principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := deps.Service.Logout(ctx, principal, in.Cookie); err != nil {
			return nil, deps.fail(ctx, err)
		}

		response := &logoutResponse{}
		response.SetCookie = clearRefreshCookie()
		return response, nil
	})
}

type logoutRequest struct {
	Cookie string `cookie:"cl_refresh_token" doc:"Rotated HttpOnly session cookie."`
}

type logoutResponse struct {
	SetCookie nethttp.Cookie `header:"Set-Cookie"`
}
