# S1 — identity slice implementation

Objective: Implement backend Slice S1 (identity) — bounded-context hexagonal layout, mandatory configuration, startup PostgreSQL reset and seed, Argon2id/HS256/SHA-256 auth, per-request membership guard, 17 OpenAPI 3.1 endpoints, and `provision-admin` CLI.
Branch: `feat/s1-identity` | Delivery strategy: `ask-on-risk`
Test runner: `cd core && mise exec -- go test -race ./...` and `bash scripts/verify-contract.sh --check`

## Specs

Quoted verbatim from the user and the frozen S1 contracts (`docs/roadmap-backend.md`, `docs/design-identity-auth.md`, `docs/design-identity-ddl.md`, `docs/design-identity-endpoints.md`).

**S1 — Customer login/signup deferred; base `WORK`/`CUSTOMER` separation and `PLAYER` seed kept.**
> 1- no va a entrar en este s1, a dime que sirgnifica el s1... no entra por que eso lo vamos a agregar cuando hagamos el login del cliente, ahora nos concetraremos en la base.. no nos interesan por ahora el resto
- Selected option verbatim: `Solo base WORK/CUSTOMER + seed (Recomendado)` — `Sin registro de cliente, pero quedan kind (WORK/CUSTOMER) y el PLAYER del seed para testear rechazos 403.`
- Check: `schema_identity.account` enforces `UNIQUE (lower(email), kind)` with `kind IN ('CUSTOMER', 'WORK')`; the seed loads `player@canchalibre.local` (`CUSTOMER`), and protected staff/admin endpoints refuse a valid `PLAYER` token with `403 urn:canchalibre:error:forbidden`.

**S2 — `SOPORTE` provisioned and revoked exclusively by `ADMIN` from the platform.**
> 2- el usuarii soporte solo lo provisiona el admin desde la plataforma
- Selected option verbatim: `Invitar + revocar SOPORTE (Recomendado)` — `POST /admin/support-operators (emite invitación de 1 uso) y DELETE /admin/support-operators/{accountId} para revocarlo.`
- Operations added to the S1 inventory under `/api/v1`:
  - `inviteSupportOperator` (`POST /admin/support-operators`): actor `ADMIN`, body `{email}`, issues a single-use `SOPORTE` invitation (`intended_role = 'SOPORTE'`, `business_id IS NULL`, `branch_id IS NULL`), returns `201 {invitation_id, expires_at}`; errors: `400 validation`, `401 unauthenticated`, `403 forbidden`, `409 conflict` (email already an active `SOPORTE` or has an unexpired `SOPORTE` invitation), `415 unsupported-media-type`, `500 internal`. Activated via `POST /auth/invitations/activate`.
  - `revokeSupportOperator` (`DELETE /admin/support-operators/{accountId}`): actor `ADMIN`, idempotent revocation of a `SOPORTE` account (`status = 'CLOSED'`, `closed_at = now()`, active refresh sessions revoked), returns `204` empty body; errors: `400 validation`, `401 unauthenticated`, `403 forbidden`, `404 not-found` (unknown account or not a `SOPORTE` operator), `500 internal`.
- Check: `scripts/check-inventory-consistency.sh` passes with 17 operations; `ADMIN` can invite and revoke `SOPORTE`; non-`ADMIN` actors receive `403 urn:canchalibre:error:forbidden` with zero domain rows mutated.

**S3 — Minimum 12-character password policy and fast, secure cryptography.**
> 3- minima de 12 caracteres, luego toma las mejores desiciones de seguridad en encriptacion, que sea seguro y rapido
- Password policy: Unicode NFC normalized before validation/hashing; minimum **12 characters** (Unicode runes after NFC normalization) and maximum **128 bytes**; violations return `400 urn:canchalibre:error:validation`.
- Password hashing: **Argon2id** (`golang.org/x/crypto/argon2`, `time=1`, `memory=19456` KiB [19 MiB OWASP minimum, fast and safe inside the 2 vCPU / 8 GB VPS budget], `threads=2`, `keyLen=32`, 16-byte `crypto/rand` salt, PHC format `$argon2id$v=19$m=19456,t=1,p=2$<salt>$<hash>`, verified with `crypto/subtle.ConstantTimeCompare`).
- Access token: **HMAC-SHA256 (`HS256`)** JWT signed with `JWT_SECRET` (minimum 32 bytes), strict `alg == "HS256"` check, 30-minute TTL (`D4`), claims `sub`, `actor`, `ctx { business_id, branch_id }` (`ctx` omitted for `ADMIN`, `SOPORTE`, `PLAYER`).
- Opaque tokens (refresh, invitation, password recovery): 32 bytes from `crypto/rand` encoded as `base64.RawURLEncoding`, persisted in PostgreSQL exclusively as **SHA-256** (`bytea`), never raw; refresh cookie is `HttpOnly; Secure; SameSite=Lax` with 7-day TTL (`D5`).
- Check: unit tests assert 11-rune password rejection, 12-rune acceptance, NFC equivalence, Argon2id hash/verify, HS256 signature/expiry/alg-confusion rejection, and SHA-256-only DB storage.

**S4 — Mandatory `DATABASE_URL` with no silent fallback.**
> 4- si, sacalo y si ko esta que falle
- `internal/platform/config` requires `DATABASE_URL` (and `JWT_SECRET`, plus `INVITATION_URL_BASE` or default/required per command) with no silent default connection string; missing `DATABASE_URL` returns an error and causes `cmd/api` and `cmd/provision-admin` to fail immediately before any network or database operation.
- Check: unit test and CLI execution with unset/empty `DATABASE_URL` fail with non-zero exit code.

**S5 — Bounded-context hexagonal layout, RFC 7807 error catalog, and OpenAPI 3.1 contract gate.**
- Move to `core/internal/modules/{identity,organization}/core/{domain,ports,usecase}`, `adapters/{inbound/http,outbound/postgres}`, and `core/internal/platform/{actor,authn,http,config,postgres}`; delete flat `core/internal/{domain,adapter,usecase,config}`.
- No cross-context imports of `core` (`identity` and `organization` never import each other's `core`).
- Total fail-closed `huma.NewError` override mapping every status to `urn:canchalibre:error:<slug>` (never `about:blank`), `r.NotFound` -> `404 urn:canchalibre:error:not-found`, `r.MethodNotAllowed` -> `405 urn:canchalibre:error:method-not-allowed`, explicit `Responses` with per-error `Example` and `415` on every body-accepting operation, and `oas3-missing-example` re-enabled in `vacuum.ruleset.yaml`.
- Check: `vacuum lint -r vacuum.ruleset.yaml --fail-severity error --min-score 98` exits 0 with score >= 98; import-boundary test passes.

**S6 — Unguarded local-startup database reset and seed.**
- On every start of `cmd/api`, run `DROP DATABASE IF EXISTS <target> WITH (FORCE)` and `CREATE DATABASE <target>` against the maintenance database, apply `0001_identity_schema.sql` and `0001_organization_spine.sql` in lexical order, and load `core/dev/seed.topology.json` paired with `.pi/api.json.local` credentials.
- Check: `scripts/verify-contract.sh --check` boots the binary against port `5435`, proves the reset recreates the schema and seeds the 5 actors + 2 businesses + 3 branches, and exits 0.

**S7 — Identity & Organization endpoints (17 operations), per-request guard, atomic refresh rotation, and `provision-admin` CLI.**
- Implement all 17 operations in `docs/roadmap-backend.md` §5.2 (`login`, `refresh`, `activateInvitation`, `requestPasswordRecovery`, `resetPassword`, `logout`, `getOwnAccount`, `listOwnContexts`, `selectContext`, `assignCoOwner`, `revokeCoOwner`, `inviteSupportOperator`, `revokeSupportOperator`, `listMemberships`, `inviteMember`, `revokeMembership`, `revokeInvitation`).
- Per-request guard (`internal/platform/authn`) executes the single indexed query (`membership_guard_idx`) so a revoked membership, closed/suspended account, or disabled business/branch cuts the very next request with `401`/`403`.
- Atomic refresh exchange (`UPDATE ... RETURNING` + `INSERT` in one transaction): concurrent refreshes have exactly one winner, and presenting an already-rotated token revokes the entire `family_id` returning `401 urn:canchalibre:error:refresh-reuse`.
- Private CLI `core/cmd/provision-admin --email <email> [--ttl <duration>] [--supersede] [--dry-run]` with exit codes `0` (success), `1` (refused/failed), `2` (usage error), printing the activation URL once to stdout and writing an `identity_audit` row.
- Check: `go test -race ./...` passes across domain, use cases, Postgres integration (including concurrent refresh rotation and last-co-owner protection), HTTP contract tests, and `provision-admin` CLI tests.

**S8 — 100% empirical & adversarial verification (`api-contract-verifier`).**
- Run `api-contract-verifier` against the live API on port `8080` and PostgreSQL on port `5435`: full §5.2 case matrix + hostile/malformed/boundary/Unicode-normalization/SQL-injection inputs + read-only SQL proof that every refusal left domain tables unchanged.
- Check: `api-contract-verifier` report passes at **100%** with zero findings.

## Tasks

- T1.1 Reconcile S1 design and endpoint inventory (`docs/roadmap-backend.md`, `docs/design-identity-endpoints.md`, `docs/design-identity-ddl.md`, `scripts/tests/test-t3-rule-reconciliation.sh`) with the 17 operations (`inviteSupportOperator`, `revokeSupportOperator`), 12-char NFC password + Argon2id/HS256/SHA-256 crypto, `password_recovery_token` DDL, and mandatory `DATABASE_URL`. — S1–S5 — route: inline — status: in_progress — commit: pending
- T1.2 Build platform foundation (`core/internal/platform/{config,actor,http,postgres}`), SQL schema files (`0001_identity_schema.sql`, `0001_organization_spine.sql`), startup `DROP DATABASE ... WITH (FORCE)` + `CREATE DATABASE` + schema + seed loader, pin `pgx/v5` and `golang.org/x/crypto` in `core/go.mod`, and remove flat `core/internal/{domain,adapter,usecase,config}`. — S3, S4, S5, S6 — route: inline — status: pending — commit: pending
- T1.3 Implement `identity` and `organization` core (`domain`, `ports`, `usecase`), Postgres outbound repositories (`adapters/outbound/postgres`), and `internal/platform/authn` guard middleware, with unit and Postgres concurrency/integration tests (`go test -race ./...`). — S1, S2, S3, S5, S7 — route: inline — status: pending — commit: pending
- T1.4 Implement inbound HTTP adapters for all 17 S1 operations, `core/cmd/api` composition root, `core/cmd/provision-admin` CLI, re-enable `oas3-missing-example` in `vacuum.ruleset.yaml`, and pass `go test -race ./...` + `vacuum lint --min-score 98`. — S2, S4, S5, S6, S7 — route: inline — status: pending — commit: pending
- T1.5 Run `scripts/verify-contract.sh --check`, execute `api-contract-verifier` (empirical §5.2 matrix + adversarial + SQL non-mutation proof at 100%), and run independent `gentle-ai-verify`. — S1–S8 — route: delegated (`api-contract-verifier` + `gentle-ai-verify`) — status: pending — commit: pending

## Log

L1. User's original request and decisions verbatim (2026-10-16):
«1- no va a entrar en este s1, a dime que sirgnifica el s1... no entra por que eso lo vamos a agregar cuando hagamos el login del cliente, ahora nos concetraremos en la base.. no nos interesan por ahora el resto 2- el usuarii soporte solo lo provisiona el admin desde la plataforma 3- minima de 12 caracteres, luego toma las mejores desiciones de seguridad en encriptacion, que sea seguro y rapido 4- si, sacalo y si ko esta que falle»
Answered questionnaire verbatim:
- `Alcance Login S1`: «Solo base WORK/CUSTOMER + seed (Recomendado) — Sin registro de cliente, pero quedan kind (WORK/CUSTOMER) y el PLAYER del seed para testear rechazos 403.»
- `Admin y SOPORTE`: «Invitar + revocar SOPORTE (Recomendado) — POST /admin/support-operators (emite invitación de 1 uso) y DELETE /admin/support-operators/{accountId} para revocarlo.»
- `Arranque de S1`: «Con esto estamos, arrancá S1 — Actualizar estos puntos en el contrato de S1 y empezar la implementación.»
