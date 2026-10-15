---
name: database-postgres-patterns
description: "Trigger: database, postgres, postgresql, migrations, goose, schema, sql, exclusion constraint, double booking, btree_gist. Enforce PostgreSQL per-context schema isolation, Goose migrations, and MVP concurrency constraints."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when creating or modifying PostgreSQL database schemas, writing Goose SQL migrations, configuring database connections, or designing persistence models and concurrency guards for CanchaLibre.

## Hard Rules

- **Schema-Per-Bounded-Context Isolation**:
  - Every bounded context module owns an independent PostgreSQL schema (`schema_{context}`). Do not assume a pre-fixed list of schemas or create a schema for nonexistent payment gateways; CanchaLibre records manual ARS payment accumulations on reservations without a payment movement ledger.
  - Cross-schema foreign keys (`REFERENCES schema_other.table(id)`) and direct cross-schema SQL `JOIN` statements are strictly forbidden. Store inter-context references as logical UUIDs validated through application ports.
- **Goose Migration Standards**:
  - Write migrations in pure SQL using `pressly/goose` directives (`-- +goose Up` and `-- +goose Down`).
  - Place migration files strictly inside their owning bounded context:
    `internal/modules/{context}/adapters/outbound/postgres/migrations/`
  - Every migration must be reversible with an explicit, working `-- +goose Down` block.
- **Mathematical Concurrency & Anti-Overlap Guards**:
  - Never rely solely on application-level read-then-write checks for capacity or overlap invariants. Combine transactional locking (`SELECT ... FOR UPDATE`) with database constraints using `btree_gist` and `tstzrange` where applicable.
  - **Preserve MVP Request vs. Confirmation Semantics**:
    - **Physical court offered slots**: Offered slots (`turnos`) on the same physical court must not overlap across any sport.
    - **Pending requests are non-exclusive**: Multiple customers may hold pending requests for the same offered slot simultaneously. **Never** apply an exclusion predicate (such as `WHERE estado != 'CANCELADA'`) to a table/state that includes pending requests, as that would illegally block second pending requests (violating MVP AC-04).
    - **Confirmed court allocation**: At most one active confirmed reservation may occupy a physical court/slot at a time. Scope reservation exclusion or uniqueness predicates strictly to active confirmed allocations.
    - **Customer cross-branch overlap**: A customer cannot hold two confirmed reservations with overlapping time ranges, even across different branches.
- **Resource & Indexing Discipline (2 vCPU / 8 GB VPS)**:
  - Index all intra-schema foreign keys, lookup predicates, and temporal range columns (`B-Tree` or `GiST`).
  - Execute all queries as parameterized statements via `pgx/v5` (`MaxConns: 15–20`, `MinConns: 2`, `MaxConnLifetime: 1h`, PostgreSQL `max_connections = 50`).
  - Avoid unindexed table scans that cause excessive disk I/O on the 100 GB NVMe.

## Decision Gates

| Need | Required Pattern | Forbidden Pattern |
| --- | --- | --- |
| Creating tables for a bounded context | `CREATE TABLE schema_{context}.table_name (...)` | Placing domain tables in `public` or sharing tables across contexts |
| Referencing entity from another context | Store logical UUID (`branch_id uuid NOT NULL`) | Cross-schema `FOREIGN KEY` or cross-schema SQL `JOIN` |
| Writing a schema migration | Reversible pure SQL (`-- +goose Up` / `-- +goose Down`) per context | Non-reversible migrations or auto-migration ORMs |
| Guarding court offers & confirmations | `btree_gist` / `tstzrange` exclusion or unique guards scoped to offers and confirmed allocations | Applying exclusion constraints to pending requests or relying on unlocked `SELECT` before `INSERT` |
| Preventing customer overlapping bookings | Database constraint / transactional lock preventing overlapping confirmed ranges per customer | Allowing concurrent confirmations to create overlapping bookings for one customer |

## Execution Steps

1. Create a timestamped Goose migration file (`goose create <name> sql`) in `internal/modules/{context}/adapters/outbound/postgres/migrations/`.
2. In `-- +goose Up`, ensure the context schema exists (`CREATE SCHEMA IF NOT EXISTS schema_{context};`) and enable `btree_gist` when temporal range exclusion constraints are used.
3. Define tables with explicit column types, `UUIDv7` or `UUIDv4` primary keys, intra-schema foreign keys, and targeted `B-Tree`/`GiST` indexes.
4. Scope exclusion and uniqueness constraints carefully so court offers do not overlap, confirmed reservations are mutually exclusive per court and per customer time range, and pending requests remain non-exclusive.
5. In `-- +goose Down`, provide exact inverse DDL statements to drop constraints, indexes, and tables cleanly.
6. Verify migrations against a real PostgreSQL 16+ instance using `goose up` and `goose down`.

## Output Contract

Return:
- Path of created or updated migration files under `internal/modules/{context}/adapters/outbound/postgres/migrations/`.
- Verification that DDL uses `schema_{context}` isolation with zero cross-schema joins or foreign keys.
- Verification that concurrency constraints prevent court offer overlaps, court double-confirmations, and customer cross-branch confirmed overlaps while permitting multiple pending requests.
- Confirmation that `-- +goose Down` cleanly reverses `-- +goose Up`.

## References

- Migration tool: `pressly/goose`.
- Target Engine: PostgreSQL 16+, `pgx/v5`, `btree_gist`.
- Architecture: `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
- Backend Go Patterns: `.agents/skills/backend-go-patterns/SKILL.md`.
- Server Limits: `.agents/skills/server-infrastructure/SKILL.md`.
