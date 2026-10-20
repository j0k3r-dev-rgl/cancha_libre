# Organization Endpoints: Slice S2 (Platform Administration & Core Organization)

This design specifies the HTTP API operations for **Slice S2: Platform Administration & Core Organization**. Per architecture decision, Slice S2 is deliberately bounded to **Part 1: Platform Administration (`ADMIN` and `SOPORTE`)**, keeping the slice cohesive, atomic, and 100% empirically and adversarially verifiable against the contract harness (`docs/endpoint-acceptance-criteria.md` §5.4).

Owner branch configuration, operating hours, photo media upload, court catalog, and the public player catalog are assigned to subsequent slices (`S2b — Owner Operations & Courts`, `S3 — Inventory`, `S5 — Discovery`).

It pairs with [`docs/design/organization-ddl.md`](organization-ddl.md), which defines the underlying PostgreSQL tables and derived predicates.

---

## 1. Architectural Decisions

| # | Topic | Decision |
| --- | --- | --- |
| D1 | Slice S2 Scope | S2 delivers the platform-level administration surface: business lifecycle (onboarding, edits, suspensions, archival), branch approval inbox (`ADMIN` & `SOPORTE`), global branch inspection, and global user governance. Owner-level branch asset management is deferred to S2b. |
| D2 | Route Prefixes & Namespace | All endpoints in this slice are served under the administrative namespace `/api/v1/admin/`. |
| D3 | RBAC & Actor Guard | Enforces strict role checks in `internal/platform/authn`: <br>- Business lifecycle & user governance: strictly **`ADMIN`**.<br>- Branch request inbox & approvals: **`ADMIN`** and **`SOPORTE`**.<br>- Attempts by `OWNER`, `EMPLOYEE`, or `PLAYER` receive a standard `403 forbidden` refusal. |
| D4 | Error Model & RFC 7807 | Every error response returns `application/problem+json` with a stable URN from [`docs/design/platform-error-model.md`](platform-error-model.md). Conflicting states return `409 conflict`. Non-existent records or records outside administrative scope return `404 not-found`. |
| D5 | Preserved Review Rounds (`BE-ORG-02`) | Approving or rejecting a branch request writes an audit record to `branch_approval_history` and updates `branch.approval_status`. Rejection **strictly requires** a non-empty `rejection_reason`. |
| D6 | Administrative Suspension | Multi-level suspension: suspending a business (`business.suspended_at`) cascades to block all its branches; suspending a branch (`branch.suspended_at`) isolates that specific facility; suspending a user (`account.status = 'SUSPENDED'`) immediately invalidates their access in the guard. |
| D7 | Business Archival Prerequisite (`BE-ORG-06`) | Archiving a business sets `archived_at` and is refused with `409 conflict` if unresolved pending requests or active reservations exist. S2 enforces this check against the database tables. |

---

## 2. Endpoint Inventory: Slice S2 (Platform Administration)

Frozen before implementation. All routes are prefixed with `/api/v1`. Path parameters are UUIDs.

| Operation ID | Method and Path | Actor | Requirements | AC | Idempotency |
| --- | --- | --- | --- | --- | --- |
| `createBusiness` | `POST /admin/businesses` | `ADMIN` | BE-ORG-01, BE-ACC-05 | AC-26, AC-43 | Non-idempotent: creates a new business entity |
| `listBusinesses` | `GET /admin/businesses` | `ADMIN` | BE-ORG-01 | AC-26 | Idempotent |
| `getBusiness` | `GET /admin/businesses/{businessId}` | `ADMIN` | BE-ORG-01, BE-ACC-05 | AC-26 | Idempotent |
| `updateBusiness` | `PATCH /admin/businesses/{businessId}` | `ADMIN` | BE-ORG-01 | AC-26 | Idempotent |
| `suspendBusiness` | `POST /admin/businesses/{businessId}/suspend` | `ADMIN` | BE-ORG-01, BE-ACC-07 | AC-19 | Idempotent: repeated suspension preserves state |
| `reactivateBusiness` | `POST /admin/businesses/{businessId}/reactivate` | `ADMIN` | BE-ORG-01, BE-ACC-07 | AC-19 | Idempotent: repeated reactivation preserves state |
| `archiveBusiness` | `DELETE /admin/businesses/{businessId}` | `ADMIN` | BE-ORG-06 | AC-40 | Idempotent |
| `unarchiveBusiness` | `POST /admin/businesses/{businessId}/unarchive` | `ADMIN` | BE-ORG-07 | AC-40 | Idempotent |
| `listBranchRequests` | `GET /admin/branch-requests` | `ADMIN`, `SOPORTE` | BE-ORG-02 | AC-25, AC-41 | Idempotent |
| `getBranchRequest` | `GET /admin/branch-requests/{requestId}` | `ADMIN`, `SOPORTE` | BE-ORG-02 | AC-25, AC-41 | Idempotent |
| `approveBranchRequest` | `POST /admin/branch-requests/{requestId}/approve` | `ADMIN`, `SOPORTE` | BE-ORG-02 | AC-25 | Non-idempotent: state transition to APPROVED |
| `rejectBranchRequest` | `POST /admin/branch-requests/{requestId}/reject` | `ADMIN`, `SOPORTE` | BE-ORG-02 | AC-25 | Non-idempotent: state transition to REJECTED |
| `listAdminBranches` | `GET /admin/branches` | `ADMIN`, `SOPORTE` | BE-ORG-01, BE-ORG-02 | AC-26 | Idempotent |
| `suspendBranch` | `POST /admin/branches/{branchId}/suspend` | `ADMIN` | BE-ORG-01, BE-ACC-07 | AC-19 | Idempotent |
| `reactivateBranch` | `POST /admin/branches/{branchId}/reactivate` | `ADMIN` | BE-ORG-01, BE-ACC-07 | AC-19 | Idempotent |
| `listUsers` | `GET /admin/users` | `ADMIN` | BE-ACC-07 | AC-42 | Idempotent |
| `getUser` | `GET /admin/users/{userId}` | `ADMIN` | BE-ACC-07 | AC-42 | Idempotent |
| `suspendUser` | `POST /admin/users/{userId}/suspend` | `ADMIN` | BE-ACC-07 | AC-19 | Idempotent |
| `reactivateUser` | `POST /admin/users/{userId}/reactivate` | `ADMIN` | BE-ACC-07 | AC-19 | Idempotent |

---

## 3. Request Shapes, Success Responses and Documented Errors

In the error column, status codes map to catalog slugs in [`docs/design/platform-error-model.md`](platform-error-model.md) §1.1 (`status → slug`).

| Operation ID | Request Shape | Success Response | Documented Errors (`status → slug`) |
| --- | --- | --- | --- |
| `createBusiness` | `{ name: string, legal_name?: string, tax_id?: string, owner_emails?: string[] }` | `201` `{ id, name, legal_name, tax_id, enabled, created_at, owners: [{ account_id, email, status }] }` | `400` validation; `401` unauthenticated; `403` forbidden; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `listBusinesses` | Query: `status?`, `search?`, `page?`, `limit?` | `200` `{ items: [{ id, name, legal_name, tax_id, enabled, suspended_at, archived_at, created_at, active_owners_count, branches_count }], total, page, limit }` | `400` validation; `401` unauthenticated; `403` forbidden; `422` validation; `500` internal |
| `getBusiness` | none | `200` `{ id, name, legal_name, tax_id, enabled, suspended_at, archived_at, created_at, owners: [{ account_id, email, display_name }], branches: [...] }` | `401` unauthenticated; `403` forbidden; `404` not-found; `500` internal |
| `updateBusiness` | `{ name?: string, legal_name?: string, tax_id?: string }` | `200` `{ id, name, legal_name, tax_id, enabled, suspended_at, archived_at }` | `400` validation; `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `suspendBusiness` | `{ reason: string }` | `200` `{ id, suspended_at, enabled }` | `400` validation; `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `reactivateBusiness` | empty body | `200` `{ id, suspended_at: null, enabled }` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |
| `archiveBusiness` | none | `204` empty body | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict (active reservations/requests exist); `500` internal |
| `unarchiveBusiness` | empty body | `200` `{ id, archived_at: null }` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |
| `listBranchRequests` | Query: `status?`, `page?`, `limit?` | `200` `{ items: [{ id, branch_id, business_id, business_name, branch_name, submitted_by, submitted_at, status, reviewed_by, reviewed_at, rejection_reason }], total, page, limit }` | `400` validation; `401` unauthenticated; `403` forbidden; `422` validation; `500` internal |
| `getBranchRequest` | none | `200` `{ id, branch_id, business_id, business_name, branch: { name, slug, street_address, city, state_province, postal_code, latitude, longitude, coordinates_confirmed }, status, submitted_by, submitted_at, reviewed_by, reviewed_at, rejection_reason, history: [...] }` | `401` unauthenticated; `403` forbidden; `404` not-found; `500` internal |
| `approveBranchRequest` | empty body | `200` `{ id, branch_id, status: "APPROVED", reviewed_by, reviewed_at }` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict (already resolved); `500` internal |
| `rejectBranchRequest` | `{ rejection_reason: string }` | `200` `{ id, branch_id, status: "REJECTED", rejection_reason, reviewed_by, reviewed_at }` | `400` validation (empty or unsupported reason); `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict (already resolved); `415` unsupported-media-type; `422` validation; `500` internal |
| `listAdminBranches` | Query: `business_id?`, `approval_status?`, `status?`, `city?`, `page?`, `limit?` | `200` `{ items: [{ id, business_id, business_name, name, slug, city, state_province, approval_status, enabled, suspended_at, archived_at, created_at }], total, page, limit }` | `400` validation; `401` unauthenticated; `403` forbidden; `422` validation; `500` internal |
| `suspendBranch` | `{ reason: string }` | `200` `{ id, suspended_at }` | `400` validation; `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `reactivateBranch` | empty body | `200` `{ id, suspended_at: null }` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |
| `listUsers` | Query: `kind?`, `role?`, `status?`, `search?`, `page?`, `limit?` | `200` `{ items: [{ id, email, display_name, kind, platform_role, status, created_at, memberships_count }], total, page, limit }` | `400` validation; `401` unauthenticated; `403` forbidden; `422` validation; `500` internal |
| `getUser` | none | `200` `{ id, email, display_name, kind, platform_role, status, created_at, memberships: [{ id, business_id, business_name, branch_id, branch_name, role, created_at }] }` | `401` unauthenticated; `403` forbidden; `404` not-found; `500` internal |
| `suspendUser` | `{ reason: string }` | `200` `{ id, status: "SUSPENDED" }` | `400` validation; `401` unauthenticated; `403` forbidden (cannot suspend self); `404` not-found; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `reactivateUser` | empty body | `200` `{ id, status: "ACTIVE" }` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |

---

## 4. Verification and Contract Obligations

The four administrative lists use one-based `page` (default `1`) and `limit` (default `20`, range `1..100`), returning `{items,total,page,limit}`. `offset` is internal, not a public pagination parameter. Non-positive pages, out-of-range limits and integer-overflowing offsets return `400 validation`; non-numeric query values return `422 validation`. Results use descending creation/submission time with descending UUID as the stable tie-breaker. Beyond-last pages return an empty `items` array with the requested page and unchanged total.

Body operations distinguish usecase validation (`400`, such as a present but blank rejection reason) from schema validation (`422`, such as an absent required property or a wrong JSON type). Both use `urn:canchalibre:error:validation`; neither permits partial domain writes. The six body-operation inventory rows and four numeric-query list rows include both statuses.

Business and branch reactivation always include `suspended_at: null`; business unarchive always includes `archived_at: null`. These fields are required and nullable in the generated OpenAPI 3.1 response schemas, not optional omitted properties.

S2 business text fields, free-text query filters, and administrative suspension/rejection reasons reject embedded NUL (`\u0000`) with `400 urn:canchalibre:error:validation` before domain writes. PostgreSQL text cannot represent NUL; otherwise supported Unicode and whitespace remain accepted. Refusal auditing uses a fixed explanation rather than persisting the invalid value. This rule does not change authentication, password or email normalization policies.

Development branch-review inputs are documented in [S2 verification fixtures](s2-verification-fixtures.md). They supply independent pending requests for `ADMIN` and `SOPORTE`, including a prior resolved round and a competing-review case, without introducing owner-side endpoints.

1. Every documented error declares an explicit huma `Responses` entry with a typed example using the RFC 7807 catalog.
2. Refusals mutate no application state.
3. Every endpoint is added to the empirical contract-verification matrix (§5.4) and must pass at 100% prior to slice closure.
