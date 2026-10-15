---
name: backend-go-testing
description: "Trigger: test, testing, go test, backend test, unit test, integration test, race detector, mock, table-driven. Enforce Go testing pyramid per bounded context, concurrency safety with race detector, pure domain unit tests, and Postgres integration tests."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when authoring, updating, or reviewing tests for the Go backend of CanchaLibre, including domain unit tests, use case orchestration tests, concurrency stress tests, and PostgreSQL integration tests.

## Hard Rules

- **Per-Context Hexagonal Testing Pyramid**:
  - `internal/modules/{context}/core/domain`: 100% pure unit tests (`_test.go`). Zero mocks, zero SQL, zero HTTP imports. Validate entity/aggregate invariants, state transitions, lead-time cutoffs, DNI normalization, accumulated payment status calculation, and sentinel domain errors.
  - `internal/modules/{context}/core/usecase`: Unit tests using mock or fake implementations of `core/ports` interfaces. Verify workflow orchestration, actor/membership/scope authorization, and error propagation without mutated state on rejection.
  - `internal/modules/{context}/adapters/outbound/postgres`: Integration tests against a real PostgreSQL 16+ instance. Test actual SQL queries, transaction rollbacks/commits, row locks (`SELECT ... FOR UPDATE`), and GiST exclusion/uniqueness constraints.
  - `internal/modules/{context}/adapters/inbound/http`: Handler tests using standard `net/http/httptest`. Validate DTO decoding, input validation, HTTP status mapping, and RFC 7807 Problem Details responses.
  - Existing tests in the bootstrap skeleton (`core/internal/adapter/http/router_test.go`) must remain passing while bounded contexts are introduced.
- **Mandatory Race Detector**:
  - Run all local and CI test suites with the race detector enabled: `go test -race ./...`. Any data race is a blocking failure.
- **Idiomatic Table-Driven Structure**:
  - Multi-case tests must use table-driven structs (`tests := []struct{ name string, ... }`) executed via `t.Run(tt.name, func(t *testing.T) { ... })` with descriptive boundary and error scenario names.
- **Concurrency & Parallelism Discipline**:
  - Enable `t.Parallel()` on pure domain and in-memory use case unit tests.
  - Do not run database integration tests in parallel against shared tables unless isolated per-test schemas or transaction rollbacks are explicitly configured.
- **MVP Concurrency Stress Verification**:
  - Reservation, offer, and limit workflows must include concurrent integration tests using `sync.WaitGroup` and `-race`:
    - **Court offer collisions**: Concurrent overlapping slot or batch creations on the same physical court (including across different sports) must reject conflicts atomically without partial batch creation.
    - **Non-exclusive pending requests**: Multiple eligible customers requesting the same available slot concurrently must all succeed as pending while within branch/global limits (1 per branch, 5 global).
    - **Competing confirmations**: Multiple staff members confirming different pending requests for the same slot concurrently must confirm exactly one reservation and close competing requests as unavailable.
    - **Customer cross-branch overlap**: Concurrent confirmations for the same customer on overlapping time ranges across branches must confirm at most one and reject the overlap.
- **Hermetic State & Deterministic Time**:
  - Database tests must clean up via `t.Cleanup()` or isolated transactions so no dirty fixtures leak across tests.
  - Inject controllable clock/time functions for cutoff, expiration, and retry window tests; never use `time.Sleep` to test business time boundaries.

## Decision Gates

| Target Under Test | Test Strategy | Forbidden Pattern |
| --- | --- | --- |
| Domain entities / invariants | Pure unit test in `core/domain` (`t.Parallel()`) | Mocking domain structs or importing SQL/HTTP packages |
| Use Case orchestration | Unit test in `core/usecase` with port fakes/mocks | Connecting to real PostgreSQL or external APIs in unit tests |
| Concurrency & capacity guards | Postgres integration test with goroutines (`sync.WaitGroup`) + `-race` | Testing race conditions with sequential calls or in-memory mocks alone |
| Database repositories / SQL | Integration test in `adapters/outbound/postgres` against real Postgres | Mocking `pgxpool` driver internals |
| HTTP handlers | `httptest.NewRecorder()` + Chi router in `adapters/inbound/http` | Starting a real TCP network listener (`http.ListenAndServe`) |

## Execution Steps

1. Identify the bounded context and target layer (`core/domain`, `core/usecase`, `adapters/outbound/postgres`, or `adapters/inbound/http`).
2. Write table-driven tests covering happy paths, exact boundary equality (e.g., inclusive cutoffs vs. slot start), unauthorized actor/scope refusals, and unchanged state on error.
3. For stateful booking, offer batching, and limit workflows, write concurrent integration tests using `sync.WaitGroup` against PostgreSQL.
4. Execute `go test -race -v ./...` from `core/`.
5. Verify zero data races, zero goroutine leaks, and hermetic database teardown.

## Output Contract

Return:
- Colocated `*_test.go` files adhering to per-context hexagonal layer isolation.
- Output of `go test -race ./...` confirming zero races and passing assertions.
- Concurrency integration test coverage verified for court offers, pending limits, competing confirmations, and cross-branch customer overlaps.

## References

- Language: Go 1.24+ standard library `testing`, `net/http/httptest`.
- Modular Hexagonal Architecture: `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
- Backend Go Patterns: `.agents/skills/backend-go-patterns/SKILL.md`.
- Database Patterns: `.agents/skills/database-postgres-patterns/SKILL.md`.
