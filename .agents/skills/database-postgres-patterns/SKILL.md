---
name: database-postgres-patterns
description: "Trigger: database, postgres, postgresql, schema, sql, reset, seed, exclusion constraint, double booking, btree_gist. Enforce PostgreSQL per-context schema isolation, reset-on-start schema files, and MVP concurrency constraints."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when creating or modifying PostgreSQL database schemas, writing `schema/*.sql` reset DDL, configuring database connections, or designing persistence models and concurrency guards for CanchaLibre.

## Hard Rules

- **Schema-Per-Bounded-Context Isolation**:
  - Every bounded context module owns an independent PostgreSQL schema (`schema_{context}`). Do not assume a pre-fixed list of schemas or create a schema for nonexistent payment gateways; CanchaLibre records manual ARS payment accumulations on reservations without a payment movement ledger.
  - Cross-schema foreign keys (`REFERENCES schema_other.table(id)`) and direct cross-schema SQL `JOIN` statements are strictly forbidden. Store inter-context references as logical UUIDs validated through application ports.
- **Reset & Schema Standards**:
  - Write schema as plain, non-versioned SQL applied on every local reset — no migration tool and no versioned migrations.
  - Place schema files strictly inside their owning bounded context:
    `internal/modules/{context}/adapters/outbound/postgres/schema/`
  - Name files `NNNN_<name>.sql` and apply them in lexical order.
  - Every local start of the API binary drops and recreates the database, applies every context's `schema/*.sql`, and loads the seed. There is no `Up`/`Down` pairing and no reversibility requirement: the reset recreates the schema from scratch and leaves no residue of the previous state.
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
| Writing schema DDL | Plain `schema/*.sql` per context, applied on local reset in lexical order | Versioned migration files, migration tooling, or auto-migration ORMs |
| Guarding court offers & confirmations | `btree_gist` / `tstzrange` exclusion or unique guards scoped to offers and confirmed allocations | Applying exclusion constraints to pending requests or relying on unlocked `SELECT` before `INSERT` |
| Preventing customer overlapping bookings | Database constraint / transactional lock preventing overlapping confirmed ranges per customer | Allowing concurrent confirmations to create overlapping bookings for one customer |

## Execution Steps

1. Create a schema file `NNNN_<name>.sql` in `internal/modules/{context}/adapters/outbound/postgres/schema/`.
2. At the top of the file, ensure the context schema exists (`CREATE SCHEMA IF NOT EXISTS schema_{context};`) and enable `btree_gist` when temporal range exclusion constraints are used.
3. Define tables with explicit column types, `UUIDv7` or `UUIDv4` primary keys, intra-schema foreign keys, and targeted `B-Tree`/`GiST` indexes.
4. Scope exclusion and uniqueness constraints carefully so court offers do not overlap, confirmed reservations are mutually exclusive per court and per customer time range, and pending requests remain non-exclusive.
5. Return to a clean state through the local reset routine, which drops and recreates the database, applies every context `schema/*.sql` in lexical order, and loads the seed — do not write `Down` statements.
6. Verify the DDL against a real PostgreSQL 16+ instance by running the local reset cycle.

## Output Contract

Return:
- Path of created or updated schema files under `internal/modules/{context}/adapters/outbound/postgres/schema/`.
- Verification that DDL uses `schema_{context}` isolation with zero cross-schema joins or foreign keys.
- Verification that concurrency constraints prevent court offer overlaps, court double-confirmations, and customer cross-branch confirmed overlaps while permitting multiple pending requests.
- Confirmation that the local reset recreates the schema from scratch and leaves no residue of the previous state.

## References

- Schema application: plain `schema/*.sql` per context, applied in lexical order by the local reset routine.
- Target Engine: PostgreSQL 16+, `pgx/v5`, `btree_gist`.
- Architecture: `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
- Backend Go Patterns: `.agents/skills/backend-go-patterns/SKILL.md`.
- Server Limits: `.agents/skills/server-infrastructure/SKILL.md`.
