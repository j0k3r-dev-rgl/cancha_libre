# CanchaLibre documentation

Each document here has one job. This page says which one, in what order to read them, and where a document moved in the 2026-10-16 restructure.

## Reading order

1. [`mvp.md`](mvp.md) — what the product does and the business rules that constrain it: scope, flows, and the `AC-*` acceptance criteria. **Product behaviour is defined here.**
2. [`prd-backend.md`](prd-backend.md) — what the backend must deliver: the `BE-*` requirements derived from the MVP, and the checks that close them.
3. [`roadmap-backend.md`](roadmap-backend.md) — how the work is ordered: the blocking toolchain blocks, the eight capability slices, the coverage table and the open items.
4. [`endpoint-acceptance-criteria.md`](endpoint-acceptance-criteria.md) — how a slice closes: the three acceptance levels (slice, endpoint, task), the endpoint case matrix, and the empirical contract-verification rule.
5. [`design/`](design/) — the frozen designs a slice implements. Read the ones the slice names; they decide interfaces, never requirements.

## Documents

| Document | Job |
| --- | --- |
| [`mvp.md`](mvp.md) | Product scope and business rules; the `AC-*` acceptance criteria the backend slices demonstrate. |
| [`prd-backend.md`](prd-backend.md) | Backend requirements (`BE-*`) and the per-requirement evidence that closes them. |
| [`roadmap-backend.md`](roadmap-backend.md) | Slice map, blocking blocks T0–T3, requirement coverage, operation prerequisites, open items. |
| [`endpoint-acceptance-criteria.md`](endpoint-acceptance-criteria.md) | Roadmap section 5: slice-level, endpoint-level and task-level acceptance, the `§5.2` inventory rows and the `§5.4` verification harness. |

## Designs

| Document | Decides |
| --- | --- |
| [`design/identity-auth.md`](design/identity-auth.md) | Token and active-context transport: JWT claims, TTLs, rotating refresh, and the guard invariant that makes revocation immediate. |
| [`design/identity-ddl.md`](design/identity-ddl.md) | Apply-on-reset SQL for `schema_identity`, the business/branch/membership spine, and the guard's indexed read. |
| [`design/identity-endpoints.md`](design/identity-endpoints.md) | The decision index (D1–D10), the inventory facts that are decisions, deliberate boundaries and the review checklist. |
| [`design/platform-error-model.md`](design/platform-error-model.md) | The closed RFC 7807 `type` catalog, the huma emission mechanism, the unrouted-path fallback, and the verification run behind them. |
| [`design/platform-module-layout.md`](design/platform-module-layout.md) | Bounded-context layout, what replaces the flat bootstrap, and the no-cross-context-import rule. |
| [`design/provision-admin.md`](design/provision-admin.md) | The private first-`ADMIN` command: input, refusals, exit codes, no password, audit row. |
| [`design/metrics-endpoint.md`](design/metrics-endpoint.md) | The private Prometheus endpoint: surface, instruments, `METRICS_TOKEN` gate and operator boundaries. |
| [`design/organization-ddl.md`](design/organization-ddl.md) | Complete SQL DDL for `schema_organization` (S2): businesses, branches, approval history, operating hours, courts, and photos without denormalized state. |
| [`design/organization-endpoints.md`](design/organization-endpoints.md) | Endpoint inventory and contracts for Slice S2 (Platform Administration & Core Organization): business lifecycle, branch requests review, and global governance. |

## Rename map — restructure of 2026-10-16

The restructure changed which file owns a block of text and nothing else: section numbers, headings, tables and decisions are the ones that were already there. So a citation by number (`§5.2`, `§1.1`, `D5`) still resolves, and only the file in front of it changes.

| Before | After |
| --- | --- |
| `docs/roadmap-backend.md` §5 | [`endpoint-acceptance-criteria.md`](endpoint-acceptance-criteria.md) — §5 whole, headings and numbering unchanged |
| `docs/design-identity-endpoints.md` §1 + §5 | [`design/platform-error-model.md`](design/platform-error-model.md) |
| `docs/design-identity-endpoints.md` §2 | [`design/platform-module-layout.md`](design/platform-module-layout.md) |
| `docs/design-identity-endpoints.md` §4 | [`design/provision-admin.md`](design/provision-admin.md) |
| `docs/design-identity-endpoints.md` (header, `## Decisions`, §3, §6, §7) | [`design/identity-endpoints.md`](design/identity-endpoints.md) |
| `docs/design-identity-auth.md` | [`design/identity-auth.md`](design/identity-auth.md) — move and rename only |
| `docs/design-identity-ddl.md` | [`design/identity-ddl.md`](design/identity-ddl.md) — move and rename only |
| `docs/design-metrics-endpoint.md` | [`design/metrics-endpoint.md`](design/metrics-endpoint.md) — move and rename only |

Historic `## Log` blocks under `odd/tasks/` still cite the old paths on purpose: they record what was observed then, and rewriting them would falsify the record. This table is the bridge for a reader who lands on one of those old paths.
