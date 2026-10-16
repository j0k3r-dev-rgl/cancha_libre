---
name: api-contracts-openapi
description: "Trigger: api, contract, openapi, swagger, swaggo, type sync, dtos, client generation. Enforce OpenAPI 3.1 documentation in Go and automatic TypeScript type generation for Next.js."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.1"
---

## Activation Contract

Use this skill when exposing or modifying Go HTTP handlers, documenting API contracts, generating OpenAPI specifications, or synchronizing backend response/request types with Next.js in CanchaLibre.

## Hard Rules

- **OpenAPI 3.1 Target & Generator Validation Prerequisite**:
  - The authoritative API specification target is OpenAPI 3.1.
  - **Toolchain (validated in T0)**: the generator is `github.com/danielgtaylor/huma/v2` pinned to `v2.39.1` with the `humachi` adapter over `github.com/go-chi/chi/v5`. It emits OpenAPI 3.1 from Go types and serves it at `/openapi.json`, with the schema documents at `/schemas` and the UI at `/docs`. `oapi-codegen` remains the only viable spec-first alternative; `swaggo/swag` stays rejected because it emits Swagger 2.0 only.
  - **Toolchain Prerequisite**: `swaggo/swag` natively targets Swagger 2.0. Do not silently relax the specification target to 2.0, do not silently switch to an unapproved third-party generator, and do not assert that current `swag` produces OpenAPI 3.1 without verified version and output evidence.
  - Mark unvalidated generator tooling as blocking automated contract generation until toolchain compatibility is established. Existing `swag init` commands are retained only as a conditional legacy workflow after verifying tool version and output schema.
- **Explicit DTOs and RFC 7807 Problem Details**:
  - Request bodies and responses in bounded context HTTP adapters (`internal/modules/{context}/adapters/inbound/http`) must use explicit Go structs with `json` tags.
  - Prohibited: `map[string]interface{}`, `any`, or unexported anonymous structs in API boundaries.
  - All error responses must adhere strictly to the RFC 7807 Problem Details schema (`type`, `title`, `status`, `detail`, `instance`).
- **Response `$schema` Link Convention (huma toolchain)**:
  - huma's default `SchemaLinkTransformer` (registered by `huma.DefaultConfig` as a create hook) is **enabled by decision**, not a defect. Do not disable it and do not hand-declare `$schema` in DTOs.
  - Every struct response body therefore carries a `$schema` string field pointing at `/schemas/{Type}.json`, plus a `Link: </schemas/{Type}.json>; rel="describedBy"` header.
  - Never report the extra field as contract drift: the same transformer declares it in `components.schemas.{Type}.properties.$schema` as `readOnly: true`, so the served response validates against the published contract.
  - Failure to touch every observable effect is a defect: a response DTO is not documented until a contract test asserts the `$schema` field is present, points at the right schema document, and is declared read-only in the contract.
- **Automated Type Generation & Repository Path Reality**:
  - Frontend TypeScript types must be generated automatically from the validated OpenAPI specification. Manual authoring or editing of API request/response types in TypeScript is strictly forbidden.
  - Repository layout uses `core/` (Go backend) and `front/` (Next.js frontend). Never invent or reference non-existent paths like `apps/api` or `apps/web`.
  - Always discover existing package scripts in `front/package.json` before recommending type generation; do not invent unverified script commands.
- **Scope Discipline and Drift Prevention**:
  - Backend-only work does not authorize implementing unrelated frontend features, but type/contract synchronization remains mandatory whenever API endpoints change and generation tooling is enabled.
  - Endpoint modifications and corresponding generated contract types must be verified together to prevent contract drift.
- **Bounded Context Architecture**:
  - Bounded contexts follow canonical layout: `internal/modules/{context}/core/{domain,ports,usecase}` and `adapters/{inbound,outbound}` without assuming a fixed context map.

## Decision Gates

| Task / Situation | Required Pattern | Forbidden Pattern |
| --- | --- | --- |
| Documenting API target | Maintain OpenAPI 3.1 target; validate toolchain output | Silently downgrading to Swagger 2.0 or claiming `swag` outputs 3.1 without proof |
| Generator unvalidated | Treat automated generation as blocked on toolchain selection | Silently switching to arbitrary unvetted tools or faking 3.1 compliance |
| Specifying payload schemas | Explicit Go DTO structs with `json` field tags | Using `map[string]any`, untyped JSON, or inline anonymous structs |
| Documenting a huma response DTO | Keep the default schema link transformer; assert `$schema` and `Link` in the contract test | Disabling the transformer, hand-declaring `$schema`, or reporting it as contract drift |
| Error response contract | RFC 7807 Problem Details DTO | Returning ad-hoc JSON errors, raw strings, or leaking database internals |
| Frontend API types | Generate types from validated spec into `front/` | Writing handwritten TypeScript interfaces for backend responses |
| Executing type generation | Inspect `front/package.json` for real scripts | Inventing non-existent scripts or assuming `apps/web` monorepo paths |
| Modifying backend endpoint | Synchronize contracts atomically when tooling is active | Leaving contract unsynchronized or branching into unrelated frontend work |

## Execution Steps

1. In the Go bounded context (`internal/modules/{context}/adapters/inbound/http`), declare explicit request and response DTO structs with `json` tags and RFC 7807 error structures.
2. Register the operation with `huma.Register`, declaring the operation id, method, path, summary, tags and the error status codes it may return. The OpenAPI 3.1 document is generated from those declarations; never hand-author it. A new response DTO is not documented until its contract test asserts the `$schema` field and the contract declaration described above.
3. Validate OpenAPI generator compatibility: if using `swag` as a conditional legacy workflow, verify generator version and output specification before proceeding. If OpenAPI 3.1 validation fails, flag the toolchain prerequisite as blocking automated generation.
4. Discover type-generation scripts in `front/package.json` (do not assume pre-configured scripts or `apps/web` paths). Run the validated generation workflow targeting `front/`.
5. Ensure Server Actions and frontend features consume the generated types without manual type shadowing.
6. Verify contract synchronization without introducing unrelated frontend feature changes.

## Output Contract

Return:
- Explicit Go DTOs and RFC 7807 problem details defined for modified endpoints.
- Handlers documented within bounded context adapters (`internal/modules/{context}/adapters/inbound/http`).
- Toolchain validation status reported (OpenAPI 3.1 compatibility verified or blocked on tooling evidence).
- Contract synchronization status confirmed between `core/` and `front/` without manual DTO authoring.

## References

- Architecture: `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
- Backend Go Patterns: `.agents/skills/backend-go-patterns/SKILL.md`.
- Next.js Patterns: `.agents/skills/frontend-nextjs-patterns/SKILL.md`.
- Server Infrastructure: `.agents/skills/server-infrastructure/SKILL.md`.
