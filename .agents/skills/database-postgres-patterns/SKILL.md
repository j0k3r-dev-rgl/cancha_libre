---
name: database-postgres-patterns
description: "Trigger: database, postgres, postgresql, migrations, goose, schema, sql, exclusion constraint, double booking, btree_gist. Enforce PostgreSQL modular schema isolation, Goose migrations per module, and anti-double booking constraints."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when creating or modifying PostgreSQL database schemas, writing Goose SQL migrations, configuring database connections, or designing data models for CanchaLibre.

## Hard Rules

- **Schema-Per-Module Isolation**:
  - Every module owns an independent PostgreSQL schema (e.g., `schema_canchas`, `schema_reservas`, `schema_usuarios`, `schema_pagos`).
  - Cross-schema foreign keys and direct cross-schema SQL `JOIN` statements are strictly forbidden. Inter-module references must be stored as raw UUID/logical IDs.
- **Goose Migration Standards**:
  - Migrations use pure SQL with `pressly/goose` directives (`-- +goose Up` and `-- +goose Down`).
  - Migration files are located strictly inside each module:
    `internal/modules/{module-name}/adapters/outbound/postgres/migrations/`
  - Every migration must be reversible with an explicit, working `-- +goose Down` block.
- **Mathematical Anti-Double Booking Guard**:
  - Any reservation/turn scheduling table must use the `btree_gist` extension and an explicit PostgreSQL Exclusion Constraint over the court/slot ID and the time range (`tstzrange`).
  - Pattern: `EXCLUDE USING gist (cancha_id WITH =, rango_horario WITH &&) WHERE (estado != 'CANCELADA')`.
  - Database rejects overlapping bookings natively; the application must never rely on software-only read-then-write checks.
- **Resource Constraints (2 vCPU / 8 GB VPS)**:
  - All foreign keys (within the same schema) and timestamp range columns must be indexed (B-Tree or GiST).
  - Queries must use parameterized statements via `pgx/v5`.
  - Avoid large unindexed table scans that trigger high disk I/O on the 100 GB NVMe.

## Decision Gates

| Need | Required Pattern | Forbidden Pattern |
| --- | --- | --- |
| Creating tables for a module | `CREATE TABLE schema_{module}.table_name (...)` | Placing tables inside default `public` schema |
| Referencing entity from another module | Store logical UUID (`cancha_id uuid NOT NULL`) | Creating physical `REFERENCES schema_canchas.canchas(id)` |
| Writing a migration | Two-phase pure SQL (`-- +goose Up` / `-- +goose Down`) | Non-reversible migrations or raw DDL in application code |
| Preventing overlapping reservations | PostgreSQL `tstzrange` + GiST Exclusion Constraint | Relying solely on `SELECT` check before `INSERT` |

## Execution Steps

1. Create a new Goose migration file using timestamp prefix: `goose create <name> sql` in the module's migration directory.
2. In `-- +goose Up`, ensure the schema exists: `CREATE SCHEMA IF NOT EXISTS schema_{module};`.
3. Enable `btree_gist` extension if implementing temporal exclusion constraints.
4. Define tables with explicit types, primary keys (UUIDv7 or UUIDv4), and indexes on query predicates.
5. In `-- +goose Down`, provide exact inverse DDL statements (drop tables, constraints, indexes).
6. Verify migration locally with `goose up` and `goose down`.

## Output Contract

Return:
- Path of newly created migration files per module.
- Verification that DDL uses `schema_{module}` namespaces.
- Verification that exclusion constraints protect against double bookings.
- Verification that reverse `-- +goose Down` statements are present and valid.

## References

- Migration tool: `pressly/goose`.
- Target Engine: PostgreSQL 16+.
- Server Limits: `.agents/skills/server-infrastructure/SKILL.md`.
- Architecture: `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
