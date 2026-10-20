# S2 — Platform Administration & Core Organization

Objective: Implement backend Slice S2 (Platform Administration & Core Organization) — business onboarding, lifecycle and governance; branch approval inbox with preserved review history; global branch supervision; and global user administration across all platform roles (19 OpenAPI 3.1 endpoints).
Branch: `feat/s2-platform-admin` | Delivery strategy: `single-pr`
Test runner: `cd core && mise exec -- go test -race ./...` and `bash scripts/verify-contract.sh --check`

## Specs

Quoted verbatim from the user and the frozen S2 contracts (`docs/roadmap-backend.md`, `docs/endpoint-acceptance-criteria.md`, `docs/design/organization-ddl.md`, `docs/design/organization-endpoints.md`).

**S1 — Scope bounded strictly to Platform Administration (`ADMIN` and `SOPORTE`).**
> «GEnial documenta pero el slice2 solo tendra la parte 1, dle administrador de plataformas, los otros endpoints iran en otros slices, para que no sea tan grande, recuerda que tienen que ser verificables al 100%, y luego continuearemos con el resto»
- Selected scope verbatim: Part 1 Platform Administration (`ADMIN` and `SOPORTE`), 19 operations under `/api/v1/admin/`. Owner branch asset operations (courts, operating hours, photos, and public catalog) are deferred to subsequent slices.

**S2 — Business Onboarding, Governance, and Archival (`ADMIN`).**
- `ADMIN` provisions the business entity and assigns initial `OWNER`s (`BE-ORG-01`, `BE-ACC-05`).
- Operations under `/api/v1/admin/businesses`:
  - `POST /admin/businesses`: create business with optional initial owner emails. Returns `201`.
  - `GET /admin/businesses`: list businesses with status filter (`active`, `suspended`, `archived`), search, and pagination.
  - `GET /admin/businesses/{businessId}`: detailed administrative view of business, its active owners, and branches.
  - `PATCH /admin/businesses/{businessId}`: update business commercial/fiscal data.
  - `POST /admin/businesses/{businessId}/suspend`: administrative suspension with reason; blocks access to all its branches.
  - `POST /admin/businesses/{businessId}/reactivate`: lift administrative suspension.
  - `DELETE /admin/businesses/{businessId}`: archive business (`BE-ORG-06`), refused with `409 conflict` if unresolved pending requests or active reservations exist.
  - `POST /admin/businesses/{businessId}/unarchive`: unarchive business (`BE-ORG-07`), restoring previous state.

**S3 — Branch Request Approval Inbox and Global Supervision (`ADMIN` and `SOPORTE`).**
- `ADMIN` and `SOPORTE` review submitted branch creation/update requests (`BE-ORG-02`).
- Rejection strictly requires a non-empty `rejection_reason`. Full review rounds are appended to `branch_approval_history` without mutating past entries.
- Operations under `/api/v1/admin/`:
  - `GET /admin/branch-requests`: inbox of branch requests, filterable by `status` (`PENDING`, `APPROVED`, `REJECTED`).
  - `GET /admin/branch-requests/{requestId}`: detailed view of a request, branch physical/geo details, and prior history.
  - `POST /admin/branch-requests/{requestId}/approve`: approve request (`approval_status = 'APPROVED'`).
  - `POST /admin/branch-requests/{requestId}/reject`: reject request with mandatory `rejection_reason`.
  - `GET /admin/branches`: global listing of all branches across the platform.
  - `POST /admin/branches/{branchId}/suspend`: suspend an individual branch (`ADMIN` only).
  - `POST /admin/branches/{branchId}/reactivate`: reactivate an individual branch (`ADMIN` only).

**S4 — Global User Governance across All Roles (`ADMIN`).**
> «ADMIN tambien gestionara todos los establecimientos y los usuarios... todos, sin importar el rol... el admin tambien podra desactivar establecimeintos completos, sucursales individuales y usuarios de la plataforma»
- Operations under `/api/v1/admin/users`:
  - `GET /admin/users`: list all platform accounts with filters by `kind` (`WORK`/`CUSTOMER`), `role`, `status`, search, and pagination.
  - `GET /admin/users/{userId}`: detailed user view with its role, status, and memberships.
  - `POST /admin/users/{userId}/suspend`: suspend account (`status = 'SUSPENDED'`), immediately cutting all active sessions in the Guard. Self-suspension is refused.
  - `POST /admin/users/{userId}/reactivate`: reactivate account (`status = 'ACTIVE'`).

**S5 — Non-denormalized Schema & PostgreSQL Migration.**
- Apply-on-reset SQL in `core/internal/modules/organization/adapters/outbound/postgres/schema/` updating `schema_organization` with full tables (`business`, `branch`, `membership`, `branch_approval_history`, `branch_operating_hours`, `branch_photo`, `court`).
- Update `schema_identity.account` status check to include `'SUSPENDED'`.
- Derived publication predicate in domain/queries: a branch is published if and only if `approval_status = 'APPROVED'`, `coordinates_confirmed = true`, `enabled = true`, neither business nor branch is suspended, and neither is archived.

**S6 — 100% Empirical & Adversarial Contract Verification Gate.**
- Every operation declared with explicit RFC 7807 problem types from the catalog.
- `scripts/verify-contract.sh --check` and `api-contract-verifier` run against the 19 operations. 100% PASS with zero findings required to close the slice.

## Tasks

- T2.1 Update PostgreSQL schema DDL for `schema_organization` and `schema_identity.account` status constraint (`SUSPENDED`), update reset and seed routines, and verify database startup. — S5 — route: inline — status: done — commit: `d1f2555`
- T2.2 Implement domain models, ports, and core use cases for business governance, branch approval inbox, and global user management, with pure domain unit tests. — S1, S2, S3, S4, S5 — route: inline — status: done — commit: `9403d44`
- T2.3 Implement outbound PostgreSQL repositories for organizations, branch approvals, and administrative user queries, with integration tests on PostgreSQL port 5435. — S2, S3, S4, S5 — route: inline — status: done — commit: `accec99`
- T2.4 Implement inbound HTTP adapters for the 19 S2 operations, wire Guard RBAC (`ADMIN` and `SOPORTE`), connect composition root in `core/cmd/api`, and verify OpenAPI 3.1 contracts (`spec_test.go` + `vacuum lint`). — S1, S2, S3, S4 — route: inline — status: done — commit: `7a5f79e`
- T2.5 Run deterministic checks (`go test -race ./...`, `verify-contract.sh --check`), execute `api-contract-verifier` against the 19 endpoints (§5.2 matrix + adversarial tests), and close findings to achieve 100% gate pass. — S6 — route: inline — status: pending — commit: pending

## Log

L1. User prompt: «GEnial recuerda que tenemos que hacer todos los endpoints para administrar, dime cuales tienes en mente»
L2. User decision bounding S2 to Platform Administration: «GEnial documenta pero el slice2 solo tendra la parte 1, dle administrador de plataformas, los otros endpoints iran en otros slices, para que no sea tan grande, recuerda que tienen que ser verificables al 100%, y luego continuearemos con el resto»
L3. User authorized implementation: «Si vamos con la implementacion!! papurri»
