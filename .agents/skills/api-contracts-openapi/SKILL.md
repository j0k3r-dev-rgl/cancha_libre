---
name: api-contracts-openapi
description: "Trigger: api, contract, openapi, swagger, swaggo, type sync, dtos, client generation. Enforce OpenAPI 3.1 documentation in Go and automatic TypeScript type generation for Next.js."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when exposing or modifying Go HTTP handlers, documenting API contracts, generating OpenAPI/Swagger specifications, or synchronizing backend response/request types with the Next.js frontend.

## Hard Rules

- **Standardized Swag Annotations in Go**:
  - Every HTTP handler in `adapters/inbound/http` must contain complete `swaggo/swag` declarative comments (`@Summary`, `@Description`, `@Tags`, `@Accept`, `@Produce`, `@Param`, `@Success`, `@Failure`, `@Router`).
- **Explicit DTOs Only**:
  - Request bodies and responses must use explicit Go structs with `json` tags and Swagger documentation types.
  - Prohibited: using `map[string]interface{}`, `any`, or unexported anonymous structs in API boundaries.
  - Error responses must follow the RFC 7807 Problem Details schema across all endpoints.
- **Automated Type Generation**:
  - The OpenAPI spec is generated in the Go backend via `swag init`.
  - The Next.js frontend must generate TypeScript types from the generated spec via `openapi-typescript` into `@repo/types` or `apps/web/src/types/api.generated.ts`.
  - Manual authoring of API response or request interfaces in TypeScript is strictly forbidden.
- **Contract Drift Prevention**:
  - Any backend endpoint modification must run `swag init` and re-run frontend type generation as a single atomic change before committing.

## Decision Gates

| Task | Required Pattern | Forbidden Pattern |
| --- | --- | --- |
| Adding an API endpoint | Add Swag doc tags to handler + run `swag init` | Returning undocumented endpoints or raw JSON maps |
| Consuming API in Next.js | Import types from `api.generated.ts` | Writing manual TypeScript `interface ApiResponse` |
| Error handling contract | Return RFC 7807 Problem Details DTO | Returning random plain text or inconsistent error shapes |
| Type synchronization | Automated script (`pnpm generate:types`) | Copy-pasting Go structs into TypeScript files manually |

## Execution Steps

1. In the Go HTTP handler, declare input/output DTOs with `json` and `binding` tags.
2. Annotate the handler function with standard Swag tags (including 200/201 success and 400/404/409/500 failure codes).
3. Run `swag init -g cmd/api/main.go -o docs/` to regenerate `swagger.json` and `swagger.yaml`.
4. In the Next.js package, execute `pnpm generate:types` (running `openapi-typescript ../../apps/api/docs/swagger.json -o src/types/api.generated.ts`).
5. Ensure Server Actions and frontend components consume the newly generated types.

## Output Contract

Return:
- Updated Swag annotations in Go handlers.
- Regenerated OpenAPI specification file.
- Regenerated TypeScript API contract file.
- Verification that no handwritten API types exist in Next.js.

## References

- Go Swagger tool: `github.com/swaggo/swag`.
- TS Generator: `openapi-typescript`.
- Architecture: `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
