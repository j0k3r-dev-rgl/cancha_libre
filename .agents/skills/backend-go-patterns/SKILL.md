---
name: backend-go-patterns
description: "Trigger: backend, go, golang, api, usecase, domain, repository, reservation logic, concurrency. Enforce Clean Architecture, transactional safety, and concurrency patterns in the Go backend."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when developing, refactoring, or reviewing Go backend code for CanchaLibre, including domain entities, use cases, HTTP handlers, and database interactions.

## Hard Rules

- **Clean Layered Boundaries**:
  - `internal/domain`: Pure business entities (Cancha, Turno, Reserva, Deporte), value objects, repository interfaces, and sentinel domain errors. Zero external dependencies.
  - `internal/usecase`: Business workflows orchestrating domain models and ports. Never import HTTP or SQL libraries here.
  - `internal/adapter/postgres`: Repository implementations using `github.com/jackc/pgx/v5/pgxpool`.
  - `internal/adapter/http`: HTTP handlers (Chi router), input DTO parsing/validation, and RFC 7807 Problem Details responses.
- **HTTP Routing with Chi (`github.com/go-chi/chi/v5`)**:
  - All HTTP routing must use Chi router (`go-chi/chi/v5`), maintaining 100% compatibility with standard `http.HandlerFunc(w http.ResponseWriter, r *http.Request)`.
  - Use standard Chi middlewares (`chimiddleware.RequestID`, `chimiddleware.RealIP`, `chimiddleware.Logger`, `chimiddleware.Recoverer`, `chimiddleware.Timeout`).
  - Route URL parameters must be extracted via `chi.URLParam(r, "param")`.
  - Prohibited: Heavy frameworks with proprietary context abstractions (`gin.Context`, `echo.Context`, `fiber.Ctx`) or `fasthttp`-based engines (Fiber) that break standard `net/http` compatibility or risk memory buffer corruption.
- **Double-Booking Guard (Zero Race Conditions)**:
  - Never trust a simple `SELECT` followed by `INSERT` for reservations.
  - All booking operations must run inside an explicit transaction (`tx.BeginTx`) using row-level locking (`SELECT ... FOR UPDATE`) or PostgreSQL temporal range exclusion constraints (`tstzrange` with GiST).
- **Mandatory Context Propagation**:
  - Every handler, use case, and repository method must accept `ctx context.Context` as its first parameter and pass it down to all I/O calls.
- **Resource Discipline (2 vCPU / 8 GB VPS)**:
  - Configure `pgxpool.Config`: `MaxConns` between 15 and 20, `MinConns: 2`, `MaxConnLifetime: 1h`.
  - No unbounded goroutine spawning on HTTP requests. Background jobs must use bounded worker pools.
- **Error Contract**:
  - Domain defines sentinel errors (e.g., `ErrSlotUnavailable`, `ErrCourtNotFound`).
  - Handlers map domain errors to standard HTTP status codes (409 Conflict, 404 Not Found, 400 Bad Request). Never expose raw SQL errors to clients.

## Decision Gates

| Task | Pattern to Apply | Forbidden Pattern |
| --- | --- | --- |
| Creating a reservation | Transaction + pessimistic lock / exclusion constraint | Simple read-then-write without row locks |
| Exposing new API route | DTO struct -> validate -> call Usecase -> map error | Handling DB queries directly inside the HTTP handler |
| HTTP routing and middleware | Chi (`go-chi/chi/v5`) with standard `http.HandlerFunc` | Heavy frameworks (`Gin`, `Echo`, `Fiber`) or proprietary contexts |
| Database queries | Raw SQL with `pgx` or `sqlc` | Heavy ORMs that hide query plans and connection lifecycle |
| Background tasks | Bounded worker pool with channels and context | Bare `go func()` without cancellation or concurrency limits |

## Execution Steps

1. Define or update domain models and repository interfaces in `internal/domain`.
2. Implement business rules and transaction boundaries in `internal/usecase`.
3. Implement database queries in `internal/adapter/postgres` using `pgxpool` with parameterized queries.
4. Wire endpoints in `internal/adapter/http` with strict request validation and Problem Details error responses.
5. Add unit tests for use cases with mock/fake repositories and integration tests for concurrent booking scenarios.

## Output Contract

Return:
- Files modified adhering to package boundaries.
- Concurrency protection mechanism verified for booking flows.
- Context propagation and connection pool settings verified.

## References

- Target: Go 1.24+, `pgx/v5`, Chi router (`github.com/go-chi/chi/v5`).
- Server Limits: `.agents/skills/server-infrastructure/SKILL.md`.
