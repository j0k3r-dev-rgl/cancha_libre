---
name: backend-go-patterns
description: "Trigger: backend, go, golang, api, usecase, domain, repository, reservation logic, concurrency. Enforce per-context hexagonal boundaries, transactional safety, and concurrency patterns in the Go backend."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when developing, refactoring, or reviewing Go backend code for CanchaLibre, including domain models, use cases, HTTP handlers, concurrency guards, and database interactions.

## Hard Rules

- **Per-Context Hexagonal Boundaries**:
  - Organize backend business code by bounded context under `internal/modules/{context}`:
    - `core/domain`: Pure entities, aggregates, value objects, and sentinel domain errors. Zero external dependencies.
    - `core/ports`: Inbound and outbound interfaces and contract DTOs.
    - `core/usecase`: Business workflows orchestrating domain models and outbound ports. Never import HTTP or SQL driver packages here.
    - `adapters/outbound/postgres`: Repository implementations using `github.com/jackc/pgx/v5/pgxpool`.
    - `adapters/inbound/http`: HTTP handlers (Chi router), request DTO parsing/validation, and RFC 7807 Problem Details responses.
  - Note: Existing flat directories in `core/internal/{domain,usecase,adapter}` are initial bootstrap scaffolding; new domain features must follow the bounded-context layout in `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
- **HTTP Routing with Chi (`github.com/go-chi/chi/v5`)**:
  - All HTTP routing uses Chi (`go-chi/chi/v5`) with standard `http.HandlerFunc(w http.ResponseWriter, r *http.Request)`.
  - Use standard Chi middlewares (`chimiddleware.RequestID`, `chimiddleware.RealIP`, `chimiddleware.Logger`, `chimiddleware.Recoverer`, `chimiddleware.Timeout`).
  - Extract route parameters via `chi.URLParam(r, "param")`.
  - Prohibited: Heavy frameworks with proprietary context abstractions (`gin`, `echo`, `fiber`) or `fasthttp` engines.
- **Concurrency & Anti-Double Booking Guards**:
  - Never rely on an unguarded `SELECT` followed by `INSERT`/`UPDATE` for state-changing booking, offer, or limit operations.
  - Execute competing mutations inside explicit database transactions (`tx.BeginTx`) using row-level locking (`SELECT ... FOR UPDATE`) and PostgreSQL temporal exclusion constraints (`tstzrange` with GiST) where applicable.
  - Preserve MVP booking semantics: multiple pending requests may coexist for the same offered slot without blocking capacity; confirming a request must atomically occupy the slot, verify the customer has no overlapping confirmed reservation across branches, and close competing pending requests for that slot as unavailable.
- **Mandatory Context Propagation**:
  - Every handler, use case, port, and repository method must accept `ctx context.Context` as its first parameter and propagate it to all downstream I/O calls.
- **Resource Discipline (2 vCPU / 8 GB VPS)**:
  - Configure `pgxpool.Config`: `MaxConns` between 15 and 20, `MinConns: 2`, `MaxConnLifetime: 1h` (server `max_connections = 50`).
  - Never spawn unbounded goroutines on HTTP requests. Background processing must use bounded worker pools with explicit context cancellation; durable timers/retries must persist state in PostgreSQL.
- **Error Contract**:
  - Define sentinel domain errors in `core/domain` (e.g., `ErrSlotUnavailable`, `ErrCourtNotFound`, `ErrCustomerOverlap`, `ErrPendingLimitExceeded`).
  - Map domain errors in HTTP adapters to standard status codes (400 Bad Request, 403 Forbidden, 404 Not Found, 409 Conflict, 422 Unprocessable Entity) using RFC 7807 Problem Details. Never expose raw SQL or provider internals.

## Decision Gates

| Task | Pattern to Apply | Forbidden Pattern |
| --- | --- | --- |
| Placing new domain logic | Bounded context (`internal/modules/{context}/core/...`) | Dumping all domains into a single flat global package |
| Confirming a reservation | Explicit transaction + row lock / exclusion guard + close competing requests | Unguarded read-then-write or blocking multiple pending requests on a slot |
| Exposing new API route | Explicit DTO -> validate -> call inbound usecase port -> map error to RFC 7807 | Executing SQL queries or business rules directly inside the HTTP handler |
| HTTP routing and middleware | Chi (`go-chi/chi/v5`) with standard `http.HandlerFunc` | Frameworks with custom contexts (`Gin`, `Echo`, `Fiber`) |
| Database queries | Parameterized SQL with `pgx/v5` | Heavy ORMs that obscure query plans, locks, and connection lifecycle |
| Background tasks | Bounded worker pool with context + DB state for durable retries | Bare `go func()` without lifecycle bounds or volatile-only critical retries |

## Execution Steps

1. Define or update domain entities, invariants, and sentinel errors in `internal/modules/{context}/core/domain`.
2. Declare inbound and outbound port interfaces in `internal/modules/{context}/core/ports`.
3. Implement business workflows and transaction boundaries in `internal/modules/{context}/core/usecase`.
4. Implement parameterized PostgreSQL queries in `internal/modules/{context}/adapters/outbound/postgres` using `pgxpool`.
5. Wire HTTP handlers in `internal/modules/{context}/adapters/inbound/http` with DTO validation and RFC 7807 error mapping.
6. Register routes and dependencies in the composition root (`cmd/api`) and verify with unit and concurrency integration tests.

## Output Contract

Return:
- Files created or modified adhering to per-context hexagonal package boundaries.
- Transaction and concurrency protection verified for offer, request, and confirmation flows.
- Context propagation, bounded worker discipline, and `pgxpool` connection settings verified.
- Domain errors mapped cleanly to RFC 7807 Problem Details responses.

## References

- Target: Go 1.24+, `pgx/v5`, Chi router (`github.com/go-chi/chi/v5`).
- Modular Hexagonal Architecture: `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
- Database Patterns: `.agents/skills/database-postgres-patterns/SKILL.md`.
- Server Limits: `.agents/skills/server-infrastructure/SKILL.md`.
