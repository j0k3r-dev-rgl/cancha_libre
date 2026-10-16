# Identity DDL and the minimum business/branch/membership spine

This design closes the `docs/prd-backend.md` section 7 item *Concrete table DDL and exclusion predicates*, restricted to what S1 (identity) needs: the identity schema and the minimum spine (business, branch, membership) that memberships attach to. It pairs with [`docs/design-identity-auth.md`](design-identity-auth.md), which read from these tables. It decides no court, offer, reservation or payment table (S2, S3, S6) and no endpoint (the endpoint inventory owns routes).

The schema below is not speculative: every statement was applied to a real PostgreSQL 17 instance, falsified case by case, and the concurrent rotation path was run with two simultaneous transactions. The evidence is recorded in *Verification performed*. The DDL is applied by the reset as plain, non-versioned SQL — never by a migration tool (see *Development reset and seed*).

## Decisions

| # | Topic | Decision |
| --- | --- | --- |
| D1 | Context isolation | Two schemas: `schema_identity` (who the actor is, how they prove it, how the session survives) and `schema_organization` (the commercial scope: business, branch, membership). No third schema in this block. |
| D2 | Cross-context references | `membership.account_id` is a **logical UUID** to `schema_identity.account.id`. No cross-schema foreign key and no cross-schema join, as `database-postgres-patterns` requires. The reference is validated through an application port. |
| D3 | Keys | UUID primary keys generated in Go as **UUIDv7** (time-ordered, index-friendly). No database extension is required, so the plan adds no `pgcrypto`/`uuid-ossp` dependency to the VPS. |
| D4 | Account uniqueness | Unique on `(lower(email), kind)`, not on `lower(email)`. Two independent accounts may share an email address (`BE-ACC-02`, D9 of the auth design). |
| D5 | Actor placement | The platform actors `ADMIN` and `SOPORTE` live in `account.platform_role`, **not** in `membership`. They have no business and no branch, which is why the auth design's `ctx` claim is absent for them. |
| D6 | Membership shape | `OWNER` is business-wide (`branch_id IS NULL`, equal access to every branch); `EMPLOYEE` is branch-scoped (`branch_id IS NOT NULL`). A `CHECK` enforces the shape, a composite foreign key keeps the branch inside its business, and partial unique indexes make "one active owner per account and business" and "one active employee row per account and branch" database facts. |
| D7 | Guard lookup | One partial index `(account_id, business_id, branch_id) WHERE revoked_at IS NULL` serves the guard's single read. Verified with `EXPLAIN`: an Index Scan, no sequential scan. |
| D8 | Refresh session | One row per login session keyed by the **hash** of the refresh token; `family_id` groups a session's rotations; `replaced_by` links a rotated row to its successor. Rotation is one `UPDATE … RETURNING` feeding one `INSERT` inside a single transaction, which is what makes D6 of the auth design ("exactly one winner") true under concurrency. |
| D9 | Refusals | No table is written by a refused request. `identity_audit` is append-only and not domain state: recording a refusal does not violate "a refusal mutates nothing". |

## Placement

Apply-on-reset schema DDL, one file per context, as `database-postgres-patterns` fixes:

```text
internal/modules/identity/adapters/outbound/postgres/schema/0001_identity_schema.sql
internal/modules/organization/adapters/outbound/postgres/schema/0001_organization_spine.sql
```

Those directories do not exist yet; the module layout belongs to S1. This block freezes the files, not their creation. Each file is plain, non-versioned SQL applied in lexical order by the reset, and order does not matter here: the spine's `account_id` is logical, so either context's schema may be applied first. The directory is named `schema/` and not `migrations/` because there is no migration tool and no version to move between — the reset recreates the schema from scratch on every start, which is what *Development reset and seed* documents.

## Development reset and seed

The development database is not permanent. It is dropped, recreated, re-schemed and reseeded at **every start of the API binary**, with no environment check and no confirmation. Starting the API *is* the trigger; there is no separate command to remember and no state that survives a restart.

The routine runs in this order:

1. `DROP DATABASE IF EXISTS <target> WITH (FORCE)`. This statement runs against the maintenance database, because PostgreSQL refuses `DROP DATABASE` from inside the database being dropped; `FORCE` terminates any session still connected to the target.
2. `CREATE DATABASE <target>`.
3. Connect to the recreated database.
4. Apply every context's `schema/*.sql` in lexical order (see *Placement*).
5. Load the seed found under `core/dev/`.

The seed is the layout T1.5 froze and it lives in two places. `core/dev/seed.topology.json` is committed and holds no credential: two businesses (`B1`, `B2`), three branches, the `OWNER` and `EMPLOYEE` memberships, the two platform actors (`ADMIN`, `SOPORTE`), the `PLAYER` customer, and the refusal matrix those rows exist to make refusable. Accounts are matched to that topology by email; the five passwords live in the git-ignored `.pi/api.json.local`, reached from `.pi/api.json` through `auth.accounts_file`. The load therefore creates the accounts for the five actors first, then their platform roles and memberships — the refusal matrix is what proves the seeded shape is the one section 5.4 needs.

### Accepted risk: the reset has no guard

The missing guard is a deliberate, re-confirmed decision, not an oversight. `DROP DATABASE ... WITH (FORCE)` is compiled into the API binary and executes wherever that binary runs; merely starting the API is enough to trigger it, against whatever database the connection configuration points at. On the development machine the target database is disposable and the only data at risk is the developer's own scratch state — which is the whole reason the reset exists. Anywhere else it is an unconditional destructive statement that fires without asking.

The risk was raised explicitly and the decision was re-confirmed afterwards: *"Dejalo sin guarda, es sólo mi máquina"*. It is recorded here as an **accepted risk** with the exposure named above. Excluding the routine from a production build behind a build tag remains available — the reset can live behind a tag so a release binary cannot contain the destructive statement at all — and is deliberately **not** adopted. This must be revisited before the first deploy.

## `schema_identity`

```sql
CREATE SCHEMA IF NOT EXISTS schema_identity;

-- One row per independent account. Work and customer accounts may share an
-- email address; uniqueness is therefore per (email, kind), never per email.
CREATE TABLE schema_identity.account (
    id            uuid        PRIMARY KEY,
    email         text        NOT NULL CHECK (email <> ''),
    kind          text        NOT NULL CHECK (kind IN ('CUSTOMER', 'WORK')),
    platform_role text        NULL CHECK (platform_role IN ('ADMIN', 'SOPORTE')),
    status        text        NOT NULL DEFAULT 'PENDING'
                              CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED', 'CLOSED')),
    display_name  text        NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    closed_at     timestamptz NULL,
    CONSTRAINT account_platform_role_is_work
        CHECK (platform_role IS NULL OR kind = 'WORK')
);

CREATE UNIQUE INDEX account_email_kind_key
    ON schema_identity.account (lower(email), kind);

CREATE INDEX account_status_idx
    ON schema_identity.account (status)
    WHERE status <> 'ACTIVE';

-- Password is optional by construction: a social customer has no row, and
-- adding a password later inserts one. Algorithm parameters live in the hash.
CREATE TABLE schema_identity.credential (
    account_id    uuid        PRIMARY KEY
                              REFERENCES schema_identity.account (id) ON DELETE CASCADE,
    password_hash text        NOT NULL CHECK (password_hash <> ''),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Single-use, expiring invitation. The raw token is never stored, only its hash.
CREATE TABLE schema_identity.invitation (
    id            uuid        PRIMARY KEY,
    token_hash    bytea       NOT NULL UNIQUE,
    email         text        NOT NULL CHECK (email <> ''),
    intended_role text        NOT NULL
                              CHECK (intended_role IN ('ADMIN', 'OWNER', 'EMPLOYEE', 'SOPORTE')),
    business_id   uuid        NULL,
    branch_id     uuid        NULL,
    issued_by     uuid        NULL REFERENCES schema_identity.account (id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    consumed_at   timestamptz NULL,
    consumed_by   uuid        NULL REFERENCES schema_identity.account (id),
    CONSTRAINT invitation_scope_shape CHECK (
        (intended_role IN ('ADMIN', 'SOPORTE') AND business_id IS NULL AND branch_id IS NULL)
        OR (intended_role = 'OWNER' AND business_id IS NOT NULL AND branch_id IS NULL)
        OR (intended_role = 'EMPLOYEE' AND business_id IS NOT NULL AND branch_id IS NOT NULL)
    ),
    CONSTRAINT invitation_consumption_shape CHECK (
        (consumed_at IS NULL AND consumed_by IS NULL)
        OR (consumed_at IS NOT NULL AND consumed_by IS NOT NULL)
    )
);

CREATE INDEX invitation_email_idx ON schema_identity.invitation (lower(email));

-- One row per login session. The row is replaced on every rotation; reuse of a
-- replaced token revokes the whole family. No cross-schema reference.
CREATE TABLE schema_identity.refresh_session (
    id          uuid        PRIMARY KEY,
    account_id  uuid        NOT NULL REFERENCES schema_identity.account (id) ON DELETE CASCADE,
    family_id   uuid        NOT NULL,
    token_hash  bytea       NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    rotated_at  timestamptz NULL,
    revoked_at  timestamptz NULL,
    replaced_by uuid        NULL REFERENCES schema_identity.refresh_session (id)
);

CREATE INDEX refresh_session_family_idx ON schema_identity.refresh_session (family_id);
CREATE INDEX refresh_session_account_idx ON schema_identity.refresh_session (account_id);

-- Single-use password recovery token. The raw token is never stored, only its SHA-256 hash.
CREATE TABLE schema_identity.password_recovery_token (
    id          uuid        PRIMARY KEY,
    account_id  uuid        NOT NULL REFERENCES schema_identity.account (id) ON DELETE CASCADE,
    token_hash  bytea       NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz NULL
);

CREATE INDEX password_recovery_token_account_idx
    ON schema_identity.password_recovery_token (account_id);

-- Append-only record of identity decisions, including refusals. Audit is not
-- domain state: recording a refusal does not violate "rejection mutates nothing".
CREATE TABLE schema_identity.identity_audit (
    id                 bigserial   PRIMARY KEY,
    occurred_at        timestamptz NOT NULL DEFAULT now(),
    event              text        NOT NULL CHECK (event <> ''),
    outcome            text        NOT NULL CHECK (outcome IN ('SUCCESS', 'REFUSED')),
    actor_account_id   uuid        NULL REFERENCES schema_identity.account (id),
    subject_account_id uuid        NULL REFERENCES schema_identity.account (id),
    reason             text        NOT NULL DEFAULT '',
    detail             jsonb       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX identity_audit_subject_idx
    ON schema_identity.identity_audit (subject_account_id, occurred_at DESC);
```

`platform_role` and `membership.role` are separate on purpose. `ADMIN` is a platform actor with no branch; an `OWNER` is a business actor with a membership. Collapsing them would make "`ADMIN` is refused on reservations, customers and payments" impossible to express, because an administrator would also be a member of the business they administer.

## `schema_organization` spine

```sql
CREATE SCHEMA IF NOT EXISTS schema_organization;

CREATE TABLE schema_organization.business (
    id          uuid        PRIMARY KEY,
    name        text        NOT NULL CHECK (name <> ''),
    enabled     boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz NULL
);

CREATE TABLE schema_organization.branch (
    id          uuid        PRIMARY KEY,
    business_id uuid        NOT NULL REFERENCES schema_organization.business (id),
    name        text        NOT NULL CHECK (name <> ''),
    enabled     boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz NULL
);

CREATE INDEX branch_business_idx ON schema_organization.branch (business_id);

-- Makes (branch_id, business_id) referenceable as one pair, so a membership can
-- never point at a branch that belongs to a different business.
ALTER TABLE schema_organization.branch
    ADD CONSTRAINT branch_id_business_key UNIQUE (id, business_id);

-- OWNER is business-wide (branch_id NULL); EMPLOYEE is branch-scoped.
CREATE TABLE schema_organization.membership (
    id          uuid        PRIMARY KEY,
    account_id  uuid        NOT NULL,
    business_id uuid        NOT NULL REFERENCES schema_organization.business (id),
    branch_id   uuid        NULL,
    role        text        NOT NULL CHECK (role IN ('OWNER', 'EMPLOYEE')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz NULL,
    CONSTRAINT membership_role_scope CHECK (
        (role = 'OWNER' AND branch_id IS NULL)
        OR (role = 'EMPLOYEE' AND branch_id IS NOT NULL)
    ),
    CONSTRAINT membership_branch_in_business
        FOREIGN KEY (branch_id, business_id)
        REFERENCES schema_organization.branch (id, business_id)
);

-- The guard's single lookup: account + business + (business-wide or branch).
CREATE INDEX membership_guard_idx
    ON schema_organization.membership (account_id, business_id, branch_id)
    WHERE revoked_at IS NULL;

CREATE UNIQUE INDEX membership_one_active_owner_key
    ON schema_organization.membership (account_id, business_id)
    WHERE role = 'OWNER' AND revoked_at IS NULL;

CREATE UNIQUE INDEX membership_one_active_employee_key
    ON schema_organization.membership (account_id, branch_id)
    WHERE role = 'EMPLOYEE' AND revoked_at IS NULL;
```

`revoked_at` is the only lifecycle column: revoking a membership is a timestamp, not a delete, so the audit trail and the unique indexes both stay truthful. The partial unique indexes allow a revoked owner to be re-added with equal access (`BE-ACC-05` requires refusing the **last**-co-owner revocation, which is a use-case rule computed from these rows, not a table constraint).

## The guard lookup

The auth design's invariant is one indexed read. This is the query it names:

```sql
SELECT m.role, m.branch_id
FROM schema_organization.membership m
JOIN schema_organization.business b ON b.id = m.business_id
JOIN schema_organization.branch   s ON s.id = COALESCE(m.branch_id, $3::uuid)
WHERE m.account_id  = $1::uuid
  AND m.business_id = $2::uuid
  AND (m.branch_id IS NULL OR m.branch_id = $3::uuid)
  AND m.revoked_at IS NULL
  AND b.enabled AND b.archived_at IS NULL
  AND s.enabled AND s.archived_at IS NULL;
```

An `OWNER` row (`branch_id IS NULL`) satisfies any branch of its business, which is what "equal access to their business's branches" means. A row that returns nothing — no membership, a revoked membership, a disabled business, a disabled branch — is a refusal. The `EXPLAIN` plan uses `membership_guard_idx` as an Index Scan.

## Refresh rotation, atomically

The auth design's D6 (reusing a rotated token revokes the family, no grace window) makes racing refreshes dangerous, so the exchange is one transaction with one winner:

```sql
BEGIN;
WITH rotated AS (
    UPDATE schema_identity.refresh_session s
       SET rotated_at = now()
     WHERE s.token_hash = $1
       AND s.revoked_at IS NULL
       AND s.rotated_at IS NULL
       AND s.expires_at > now()
    RETURNING s.id, s.account_id, s.family_id
)
INSERT INTO schema_identity.refresh_session (id, account_id, family_id, token_hash, expires_at)
SELECT $2, r.account_id, r.family_id, $3, now() + interval '7 days'
FROM rotated r
RETURNING id;
COMMIT;
```

The `UPDATE` is the serialization point: the second concurrent transaction re-reads the row, sees `rotated_at` set, and rotates nothing. No row returned means either an expired session or a **reused** token; the caller distinguishes them by whether the row exists with `rotated_at IS NOT NULL`, and revokes the family in that case:

```sql
UPDATE schema_identity.refresh_session
   SET revoked_at = now()
 WHERE family_id = $1 AND revoked_at IS NULL;
```

The Server Action must still keep one refresh in flight per session; the transaction prevents a corrupt state, not a wasted round trip.

## Refusals

A refused request writes no domain row. The shape of each refusal — 401 unauthenticated, 403 authenticated without permission, all as RFC 7807 problem details — comes from the auth design D8. `identity_audit` is the exception that is not domain state: an invitation refused because it was already consumed, or a DNI conflict review by `SOPORTE` (`BE-ID-06/07`), is recorded there with `outcome = 'REFUSED'`, so "nothing was mutated" stays true of the tables a client can observe.

## Verification performed

PostgreSQL 17 (`postgres:17-alpine`), throwaway container, the schema applied, then falsified case by case.

**Version caveat.** The project's development target is PostgreSQL **16**: `docker_compose/docker-compose.yml` pins `postgres:16-alpine`, and `docs/prd-backend.md` requires 16+. This evidence was produced on **17**. The schema uses no 17-only feature — the exclusion constraints, partial indexes, composite foreign keys and `DROP DATABASE … WITH (FORCE)` all exist in 16 — but "falsified" is a claim about the version it ran on, and that is not the database the reset will actually recreate. The falsification must therefore be re-run against the type the reset targets once the reset lands in S1. Until then this evidence is strong for the DDL's logic and unproven for the exact server it will run against.

| # | Case | Expected | Observed |
| --- | --- | --- | --- |
| 1 | Both context schemas applied | apply | exit 0, exit 0 |
| 2 | Same email as `CUSTOMER` and `WORK` | allowed | inserted |
| 3 | Same `(email, kind)`, different case | refused | `account_email_kind_key` violation |
| 4 | `CUSTOMER` carrying `platform_role` | refused | `account_platform_role_is_work` violation |
| 5 | `OWNER` membership with `branch_id NULL` | allowed | inserted |
| 6 | `OWNER` membership with a branch | refused | `membership_role_scope` violation |
| 7 | `EMPLOYEE` membership with `branch_id NULL` | refused | `membership_role_scope` violation |
| 8 | Membership on a branch of another business | refused | `membership_branch_in_business` violation |
| 9 | Second active `OWNER` for the same account and business | refused | `membership_one_active_owner_key` violation |
| 10 | Revoke owner, then add a new owner | allowed | inserted (partial index) |
| 11 | `ADMIN` invitation carrying a business | refused | `invitation_scope_shape` violation |
| 12 | `EMPLOYEE` invitation with business and branch | allowed | inserted |
| 13 | Guard lookup plan | Index Scan | `Index Scan using membership_guard_idx`, no sequential scan |
| 14 | Two concurrent exchanges of the same token | exactly one winner | one row inserted, the other transaction inserted nothing |

Then: reuse of the rotated token inserted nothing, and family revocation revoked both rows of the family in one statement.

**Invalidated by the removal of migrations.** The earlier evidence that both `Down` blocks ran in reverse order and left **zero** tables in `schema_%` and **zero** schemas named `schema_%` no longer applies. Reversibility is not an acceptance criterion any more; the criterion that replaces it is that the reset recreates the schema from scratch on every local start and leaves no residue of the previous state. That is the reset's own drop-and-recreate cycle to prove, not a `Down` statement, and it is not claimed here — the routine of *Development reset and seed* is not implemented yet, so no reset evidence exists.

## What this design deliberately leaves open

| Open item | Why it is not decided here | Closed by |
| --- | --- | --- |
| Entity tables beyond the spine (courts, offers, reservations, payments) | Different contexts, different slices. | S2, S3, S6 |
| The first-`ADMIN` private command and its argument surface | Command contract, not table shape. | S1 |
| DNI fingerprint, suffix length and secret rotation | S4's subject; no identity-verification table is created before it. | S4 |
| `password_hash` algorithm and parameters | Resolved for S1: **Argon2id** (`golang.org/x/crypto/argon2`, `m=19456` KiB, `t=1`, `p=2`, 16-byte salt, 32-byte key) in PHC format `$argon2id$v=19$m=19456,t=1,p=2$<salt>$<hash>`, over Unicode NFC normalized passwords of 12 characters minimum and 128 bytes maximum. | S1 (closed) |
| Retention and deletion jobs (30-day DNI clock, `BE-ID-08`) | Requires the S4 tables. | S4 |

## Review checklist

- [ ] No cross-schema foreign key and no cross-schema join exists between `schema_identity` and `schema_organization`.
- [ ] `membership.account_id` carries no `REFERENCES` clause.
- [ ] Work and customer accounts share an email without merging.
- [ ] `ADMIN` and `SOPORTE` are platform roles with no membership row.
- [ ] The guard lookup is one indexed read and returns nothing for a revoked membership, a disabled business or a disabled branch.
- [ ] Two concurrent refresh exchanges cannot revoke a legitimate session, and reusing a rotated token revokes the family.
- [ ] Refused requests leave every domain table unchanged.
- [ ] No `migrations/` directory and no migration tooling appears anywhere in the placement.
- [ ] The reset recreates the schema from scratch on every local start and leaves no residue of the previous state.

## Next step

T1.4 builds the section 5.4 verification harness (read-only subagent plus runner), T1.5 fixes the seed layout for the five actors referenced from `.pi/api.json` and consumed by the reset, and T1.6 pins `govulncheck` and decides whether `openapi.json` is committed.
