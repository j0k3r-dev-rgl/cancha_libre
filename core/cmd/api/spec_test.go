package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"canchalibre/core/internal/platform/config"
	platformhttp "canchalibre/core/internal/platform/http"
)

// This file is the contract rule for the frozen S1 endpoint inventory
// (docs/endpoint-acceptance-criteria.md section 5.2, docs/design/platform-error-model.md
// section 1, docs/design/identity-endpoints.md section 3): it builds the real router with the real composition root,
// serves it over httptest and asserts against the fetched OpenAPI 3.1 document.
//
// The rules asserted here are the ones the design froze:
//   - exactly the 17 documented S1 operations, at the documented path and method;
//   - every documented error status is an explicit Responses entry whose
//     application/problem+json example carries the documented
//     urn:canchalibre:error:<slug> from the closed catalog;
//   - every body-accepting operation declares 415 unsupported-media-type;
//   - no type outside the catalog and no about:blank anywhere in the document.

type operationSpec struct {
	method string
	path   string
}

// documentedOperations is the frozen S1 and S2 inventory. It must match
// docs/endpoint-acceptance-criteria.md section 5.2 row by row.
var documentedOperations = map[string]operationSpec{
	// S1 Identity & Core Spine (17 operations)
	"login":                   {http.MethodPost, "/api/v1/auth/login"},
	"refresh":                 {http.MethodPost, "/api/v1/auth/refresh"},
	"activateInvitation":      {http.MethodPost, "/api/v1/auth/invitations/activate"},
	"requestPasswordRecovery": {http.MethodPost, "/api/v1/auth/password/recovery"},
	"resetPassword":           {http.MethodPost, "/api/v1/auth/password/reset"},
	"logout":                  {http.MethodPost, "/api/v1/auth/logout"},
	"getOwnAccount":           {http.MethodGet, "/api/v1/me"},
	"listOwnContexts":         {http.MethodGet, "/api/v1/me/contexts"},
	"selectContext":           {http.MethodPost, "/api/v1/me/context"},
	"assignCoOwner":           {http.MethodPost, "/api/v1/admin/businesses/{businessId}/co-owners"},
	"revokeCoOwner":           {http.MethodDelete, "/api/v1/admin/businesses/{businessId}/co-owners/{accountId}"},
	"inviteSupportOperator":   {http.MethodPost, "/api/v1/admin/support-operators"},
	"revokeSupportOperator":   {http.MethodDelete, "/api/v1/admin/support-operators/{accountId}"},
	"listMemberships":         {http.MethodGet, "/api/v1/businesses/{businessId}/memberships"},
	"inviteMember":            {http.MethodPost, "/api/v1/businesses/{businessId}/invitations"},
	"revokeMembership":        {http.MethodDelete, "/api/v1/businesses/{businessId}/memberships/{membershipId}"},
	"revokeInvitation":        {http.MethodDelete, "/api/v1/businesses/{businessId}/invitations/{invitationId}"},

	// S2 Platform Administration (19 operations)
	"createBusiness":       {http.MethodPost, "/api/v1/admin/businesses"},
	"listBusinesses":       {http.MethodGet, "/api/v1/admin/businesses"},
	"getBusiness":          {http.MethodGet, "/api/v1/admin/businesses/{businessId}"},
	"updateBusiness":       {http.MethodPatch, "/api/v1/admin/businesses/{businessId}"},
	"suspendBusiness":      {http.MethodPost, "/api/v1/admin/businesses/{businessId}/suspend"},
	"reactivateBusiness":   {http.MethodPost, "/api/v1/admin/businesses/{businessId}/reactivate"},
	"archiveBusiness":      {http.MethodDelete, "/api/v1/admin/businesses/{businessId}"},
	"unarchiveBusiness":    {http.MethodPost, "/api/v1/admin/businesses/{businessId}/unarchive"},
	"listBranchRequests":   {http.MethodGet, "/api/v1/admin/branch-requests"},
	"getBranchRequest":     {http.MethodGet, "/api/v1/admin/branch-requests/{requestId}"},
	"approveBranchRequest": {http.MethodPost, "/api/v1/admin/branch-requests/{requestId}/approve"},
	"rejectBranchRequest":  {http.MethodPost, "/api/v1/admin/branch-requests/{requestId}/reject"},
	"listAdminBranches":    {http.MethodGet, "/api/v1/admin/branches"},
	"suspendBranch":        {http.MethodPost, "/api/v1/admin/branches/{branchId}/suspend"},
	"reactivateBranch":     {http.MethodPost, "/api/v1/admin/branches/{branchId}/reactivate"},
	"listUsers":            {http.MethodGet, "/api/v1/admin/users"},
	"getUser":              {http.MethodGet, "/api/v1/admin/users/{userId}"},
	"suspendUser":          {http.MethodPost, "/api/v1/admin/users/{userId}/suspend"},
	"reactivateUser":       {http.MethodPost, "/api/v1/admin/users/{userId}/reactivate"},
}

// documentedErrors lists, per operation, every documented error status and the
// catalog URNs that status may carry, exactly as frozen in section 5.2.
var documentedErrors = map[string]map[int][]string{
	"login": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"refresh": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated, platformhttp.TypeRefreshReuse},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"activateInvitation": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusConflict:            {platformhttp.TypeInvitationConsumed, platformhttp.TypeInvitationExpired},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"requestPasswordRecovery": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"resetPassword": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"logout": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"getOwnAccount": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"listOwnContexts": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"selectContext": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeContextNotAuthorized},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"assignCoOwner": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"revokeCoOwner": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeLastCoOwner},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"inviteSupportOperator": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"revokeSupportOperator": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"listMemberships": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"inviteMember": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"revokeMembership": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeLastCoOwner},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"revokeInvitation": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},

	// S2 Platform Administration (19 operations)
	"createBusiness": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"listBusinesses": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"getBusiness": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"updateBusiness": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"suspendBusiness": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"reactivateBusiness": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"archiveBusiness": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"unarchiveBusiness": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"listBranchRequests": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"getBranchRequest": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"approveBranchRequest": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"rejectBranchRequest": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"listAdminBranches": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"suspendBranch": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"reactivateBranch": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"listUsers": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"getUser": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"suspendUser": {
		http.StatusBadRequest:          {platformhttp.TypeValidation},
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
	"reactivateUser": {
		http.StatusUnauthorized:        {platformhttp.TypeUnauthenticated},
		http.StatusForbidden:           {platformhttp.TypeForbidden},
		http.StatusNotFound:            {platformhttp.TypeNotFound},
		http.StatusConflict:            {platformhttp.TypeConflict},
		http.StatusInternalServerError: {platformhttp.TypeInternal},
	},
}

// catalogTypes is the closed catalog of docs/design/platform-error-model.md section 1.1.
var catalogTypes = map[string]bool{
	platformhttp.TypeValidation:           true,
	platformhttp.TypeUnauthenticated:      true,
	platformhttp.TypeRefreshReuse:         true,
	platformhttp.TypeForbidden:            true,
	platformhttp.TypeNotFound:             true,
	platformhttp.TypeConflict:             true,
	platformhttp.TypeLastCoOwner:          true,
	platformhttp.TypeInvitationConsumed:   true,
	platformhttp.TypeInvitationExpired:    true,
	platformhttp.TypeContextNotAuthorized: true,
	platformhttp.TypeMethodNotAllowed:     true,
	platformhttp.TypeUnsupportedMediaType: true,
	platformhttp.TypeInternal:             true,
}

// systemOperations are the platform probes that are deliberately not part of the
// identity/organization inventory.
var systemOperations = map[string]bool{"health": true, "ping": true}

type openAPIMediaType struct {
	Example  json.RawMessage            `json:"example"`
	Examples map[string]json.RawMessage `json:"examples"`
}

type openAPIRequestBody struct {
	Content map[string]openAPIMediaType `json:"content"`
}

type openAPIResponse struct {
	Content map[string]openAPIMediaType `json:"content"`
}

type openAPIOperation struct {
	OperationID string                     `json:"operationId"`
	RequestBody *openAPIRequestBody        `json:"requestBody"`
	Responses   map[string]openAPIResponse `json:"responses"`
}

type openAPIDocument struct {
	OpenAPI string                                 `json:"openapi"`
	Paths   map[string]map[string]openAPIOperation `json:"paths"`
}

// documentedOperationByID resolves an operation id to its documented method+path.
func documentedOperationByID(t *testing.T, id string) operationSpec {
	t.Helper()
	spec, ok := documentedOperations[id]
	if !ok {
		t.Fatalf("test table is missing the documented operation %q", id)
	}
	return spec
}

// buildRouterForTest builds the real router through the composition root with a
// lazy pool that never connects: the contract and refusal tests only exercise
// schema generation and refusals that are decided before any query runs.
func buildRouterForTest(t *testing.T) http.Handler {
	t.Helper()

	pool, err := pgxpool.New(context.Background(),
		"postgres://postgres:postgres@127.0.0.1:1/canchalibre_spec?sslmode=disable")
	if err != nil {
		t.Fatalf("construct spec pool: %v", err)
	}
	t.Cleanup(pool.Close)

	router, _ := newRouter(pool, config.Config{
		JWTSecret:         "0123456789abcdef0123456789abcdef",
		InvitationURLBase: "http://localhost:3000/auth/activate",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return router
}

// fetchSpec serves the real router over httptest and returns the decoded OpenAPI
// document plus the raw JSON.
func fetchSpec(t *testing.T) (openAPIDocument, []byte) {
	t.Helper()

	server := httptest.NewServer(buildRouterForTest(t))
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("fetch /openapi.json: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /openapi.json, got %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /openapi.json: %v", err)
	}

	var doc openAPIDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode /openapi.json: %v", err)
	}
	return doc, raw
}

// problemTypesAt returns the catalog URNs declared for a response status through
// the application/problem+json content, reading both the singular example and the
// named examples map.
func problemTypesAt(op openAPIOperation, status int) ([]string, bool) {
	resp, ok := op.Responses[strconv.Itoa(status)]
	if !ok {
		return nil, false
	}
	media, ok := resp.Content["application/problem+json"]
	if !ok {
		return nil, false
	}
	var types []string
	if len(media.Example) > 0 {
		var example struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(media.Example, &example); err == nil && example.Type != "" {
			types = append(types, example.Type)
		}
	}
	names := make([]string, 0, len(media.Examples))
	for name := range media.Examples {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		var wrapper struct {
			Value struct {
				Type string `json:"type"`
			} `json:"value"`
		}
		if err := json.Unmarshal(media.Examples[name], &wrapper); err == nil && wrapper.Value.Type != "" {
			types = append(types, wrapper.Value.Type)
		}
	}
	return types, true
}

func TestOpenAPISpecDeclaresFrozenInventory(t *testing.T) {
	doc, _ := fetchSpec(t)

	if !strings.HasPrefix(doc.OpenAPI, "3.1") {
		t.Fatalf("expected OpenAPI 3.1, got %q", doc.OpenAPI)
	}

	found := map[string]operationSpec{}
	for path, methods := range doc.Paths {
		for method, op := range methods {
			if op.OperationID == "" {
				continue
			}
			found[op.OperationID] = operationSpec{method: strings.ToUpper(method), path: path}
		}
	}

	for id, want := range documentedOperations {
		got, ok := found[id]
		if !ok {
			t.Fatalf("operation %q is missing from the served contract; the frozen inventory requires %s %s",
				id, want.method, want.path)
		}
		if got.method != want.method || got.path != want.path {
			t.Fatalf("operation %q is documented at %s %s, the frozen inventory requires %s %s",
				id, got.method, got.path, want.method, want.path)
		}
	}

	for id := range found {
		if documentedOperations[id].path != "" || systemOperations[id] {
			continue
		}
		t.Fatalf("unexpected operation %q in the contract; the frozen inventory has exactly %d identity/organization operations", id, len(documentedOperations))
	}

	inventoryCount := 0
	for _, methods := range doc.Paths {
		for _, op := range methods {
			if op.OperationID == "" || systemOperations[op.OperationID] {
				continue
			}
			inventoryCount++
		}
	}
	if inventoryCount != len(documentedOperations) {
		t.Fatalf("the contract serves %d identity/organization operations, the frozen inventory has exactly %d",
			inventoryCount, len(documentedOperations))
	}
}

func TestOpenAPISpecDeclaresEveryDocumentedProblemType(t *testing.T) {
	doc, _ := fetchSpec(t)

	for id := range documentedOperations {
		spec := documentedOperationByID(t, id)
		op, ok := doc.Paths[spec.path][strings.ToLower(spec.method)]
		if !ok {
			t.Fatalf("operation %q is not served at %s %s", id, spec.method, spec.path)
		}
		for status, wantTypes := range documentedErrors[id] {
			declared, ok := problemTypesAt(op, status)
			if !ok {
				t.Fatalf("operation %q does not declare status %d with an application/problem+json response", id, status)
			}
			for _, want := range wantTypes {
				found := false
				for _, declaredType := range declared {
					if declaredType == want {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("operation %q status %d declares problem types %v, missing documented %q",
						id, status, declared, want)
				}
			}
		}
	}
}

func TestOpenAPISpecDeclares415ForEveryBodyAcceptingOperation(t *testing.T) {
	doc, _ := fetchSpec(t)

	for path, methods := range doc.Paths {
		for method, op := range methods {
			if op.OperationID == "" {
				continue
			}
			types, declared := problemTypesAt(op, http.StatusUnsupportedMediaType)
			if op.RequestBody == nil {
				if declared {
					t.Fatalf("operation %q has no request body but declares 415 (%v)", op.OperationID, types)
				}
				continue
			}
			if len(op.RequestBody.Content) == 0 {
				continue
			}
			if !declared {
				t.Fatalf("body-accepting operation %q (%s %s) does not declare 415 with application/problem+json",
					op.OperationID, strings.ToUpper(method), path)
			}
			if len(types) != 1 || types[0] != platformhttp.TypeUnsupportedMediaType {
				t.Fatalf("operation %q declares 415 types %v, expected exactly [%q]",
					op.OperationID, types, platformhttp.TypeUnsupportedMediaType)
			}
		}
	}
}

func TestOpenAPISpecUsesOnlyTheClosedCatalogAndNeverAboutBlank(t *testing.T) {
	doc, raw := fetchSpec(t)

	if strings.Contains(string(raw), "about:blank") {
		t.Fatalf("the served contract contains about:blank, which the catalog forbids")
	}

	for path, methods := range doc.Paths {
		for method, op := range methods {
			if op.OperationID == "" {
				continue
			}
			for statusStr := range op.Responses {
				status, err := strconv.Atoi(statusStr)
				if err != nil {
					continue
				}
				types, _ := problemTypesAt(op, status)
				for _, problemType := range types {
					if !catalogTypes[problemType] {
						t.Fatalf("%s %s status %d documents type %q, which is not in the frozen catalog",
							strings.ToUpper(method), path, status, problemType)
					}
				}
			}
		}
	}
}

// TestOpenAPISpecNeverPublishesThePrivateMetricsPath keeps the scrape endpoint out
// of the contract every client reads. It is a raw router route precisely so that
// it cannot appear here, and this test is what keeps it that way if someone later
// registers it with huma.
func TestOpenAPISpecNeverPublishesThePrivateMetricsPath(t *testing.T) {
	doc, raw := fetchSpec(t)

	if _, ok := doc.Paths[platformhttp.MetricsPath]; ok {
		t.Fatalf("the private scrape path %q is published in the OpenAPI document", platformhttp.MetricsPath)
	}
	for path, methods := range doc.Paths {
		for method, op := range methods {
			if op.OperationID == "metrics" {
				t.Fatalf("operation %q is published at %s %s", op.OperationID, method, path)
			}
		}
	}
	if strings.Contains(string(raw), `"/metrics"`) {
		t.Fatal("the served OpenAPI document mentions the private scrape path")
	}
}
