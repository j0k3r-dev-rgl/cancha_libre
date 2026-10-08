---
name: backend-go-testing
description: "Trigger: test, testing, go test, backend test, unit test, integration test, race detector, mock, table-driven. Enforce Go testing pyramid, concurrency safety with race detector, pure domain unit tests, and Postgres integration tests."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when authoring, updating, or reviewing tests for the Go backend of CanchaLibre, including unit tests, use case mocking, concurrency stress tests, and database integration tests.

## Hard Rules

- **Hexagonal Testing Pyramid**:
  - `internal/domain`: 100% pure unit tests (`_test.go`). Zero mocks, zero SQL, zero HTTP imports. Validate invariants, state transitions, slot duration calculations, and domain sentinel errors.
  - `internal/usecase`: Unit tests using mock or fake implementations of domain repository interfaces. Test workflow orchestration, authorization, and error propagation.
  - `internal/adapter/postgres`: Integration tests against real PostgreSQL. Test actual SQL queries, transaction boundaries, and constraint violations (GiST exclusion and `SELECT ... FOR UPDATE`).
  - `internal/adapter/http`: Handler tests using standard `net/http/httptest`. Validate request decoding, input validation errors, HTTP status codes, and RFC 7807 Problem Details responses.
- **Mandatory Race Detector**:
  - All test executions in local development and CI must run with the race detector enabled: `go test -race ./...`. Tests with data races are strictly rejected.
- **Idiomatic Table-Driven Structure**:
  - Multi-case tests must use standard table-driven test structs (`tests := []struct{ name string, ... }`) with `t.Run(tt.name, func(t *testing.T) { ... })`.
  - Provide meaningful scenario names describing the boundary or failure condition.
- **Concurrency & Parallelism Discipline**:
  - Enable `t.Parallel()` on pure domain and unit tests.
  - Prohibit `t.Parallel()` on database integration tests sharing tables or sequences unless isolated schemas or transaction rollbacks per test are explicitly configured.
- **Double-Booking Concurrency Verification**:
  - Booking and reservation flows must include explicit concurrency tests: launch multiple concurrent goroutines (`sync.WaitGroup`) attempting to book the exact same court and time slot.
  - Exactly one must succeed; all concurrent collisions must fail safely (e.g. 409 Conflict / domain slot occupied error) without deadlocks or double allocations.
- **Hermetic State & Teardown**:
  - Database tests must clean up their state via `t.Cleanup()` or run inside an uncommitted transaction rollback. Never leave dirty fixtures that pollute subsequent tests.

## Decision Gates

| Target Under Test | Test Strategy | Forbidden Pattern |
| --- | --- | --- |
| Domain entity / logic | Pure unit test (`t.Parallel()`) | Mocking domain entities or importing DB packages |
| Use Case orchestration | Unit test with interface mock/fake | Connecting to real database in usecase tests |
| Booking concurrency / locks | Integration test with goroutines + `-race` | Testing booking race conditions with sequential calls |
| Database repository / SQL | Integration test against Postgres instance | Mocking `pgxpool` SQL driver internals |
| HTTP endpoints | `httptest.NewRecorder()` + router | Starting actual network listener (`http.ListenAndServe`) |

## Execution Steps

1. Identify target layer (Domain, UseCase, Postgres Adapter, or HTTP Handler).
2. Write table-driven test covering happy path, validation boundaries, and known domain errors.
3. For reservation use cases, write concurrent test using `sync.WaitGroup` to stress race conditions.
4. Execute `go test -race -v ./path/to/package/...`.
5. Ensure zero data races and complete test cleanup.

## Output Contract

Return:
- Colocated `*_test.go` files adhering to layer isolation.
- Evidence of race detector validation (`go test -race`).
- Concurrency test verification for reservation/locking mechanics.

## References

- Language: Go 1.24+ standard library `testing`, `httptest`.
- Backend Architecture: `.agents/skills/backend-go-patterns/SKILL.md`.
- Database Patterns: `.agents/skills/database-postgres-patterns/SKILL.md`.
