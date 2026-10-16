# Identity endpoint inventory

The three cross-cutting design items that `docs/roadmap-backend.md` section 5.2 requires before S1 (identity) implements anything are designed in documents of their own: the stable RFC 7807 problem-type catalog and its verification in [`docs/design/platform-error-model.md`](platform-error-model.md) (§1, §5), the bounded-context module layout that replaces the flat bootstrap in [`docs/design/platform-module-layout.md`](platform-module-layout.md) (§2), and the contract of the private first-`ADMIN` command (`BE-ACC-04`) in [`docs/design/provision-admin.md`](provision-admin.md) (§4). This document keeps the decision index, the endpoint inventory and the review checklist; the inventory rows themselves live in [`docs/endpoint-acceptance-criteria.md`](../endpoint-acceptance-criteria.md) section 5.2, where that section requires them.

It pairs with [`docs/design/identity-auth.md`](identity-auth.md) (token and active context) and [`docs/design/identity-ddl.md`](identity-ddl.md) (tables). It decides no table DDL, no new product requirement, and no endpoint beyond the identity slice.

## Decisions

| # | Topic | Decision |
| --- | --- | --- |
| D1 | Problem `type` values | Every documented error carries a **stable** `type` URN of the form `urn:canchalibre:error:<slug>`, never `about:blank`, so a client can branch on `type` instead of prose. |
| D2 | How huma emits them | `huma.NewError` is an overridable `var`; the project replaces it once to map status → URN, and each operation declares its errors as **explicit `Responses` entries with a per-error `Example`** (see §1.3 of [`design/platform-error-model.md`](platform-error-model.md) for why `Errors []int` alone is not enough). |
| D3 | Authenticated vs unauthenticated | `401` means "no valid credential"; `403` means "valid credential, no permission". Both are problem details, and a refusal mutates nothing (the only write a refusal may produce is the append-only audit row of the T1.3 design). |
| D4 | Existence disclosure | A record that does not exist and a record that belongs to another actor both return the **same** `404` with the same `type` and no distinguishing detail. |
| D5 | Module layout | Bounded contexts move to `internal/modules/{context}/core/{domain,ports,usecase}` and `adapters/{inbound,outbound}`, per `BE-ENG-07`. Cross-cutting, domain-free code lives in `internal/platform/`. The flat `internal/{domain,adapter,usecase,config}` is **removed** in S1, as `BE-ENG-07` already states it is bootstrap scaffolding. |
| D6 | Shared actor contract | The resolved actor travels as a portable value type in `internal/platform/actor`, so consuming contexts never import identity's core. It is a transport value, not authority. |
| D7 | Where the guard lives | The middleware that enforces the T1.2 invariant lives in `internal/platform/authn` and receives the account, membership and scope lookups as **ports wired at `cmd/api`**. It calls use cases; it never reads tables itself. |
| D8 | Provisioning command | A separate binary `core/cmd/provision-admin`, not an HTTP endpoint and not a subcommand of the API. A private server command cannot be reached over the network, which is the whole point of `BE-ACC-04`. |
| D9 | No default password | The command issues a **single-use, expiring invitation** and prints the activation URL once. It never generates a password, never stores one, and never logs the token. |
| D10 | Route style | Plural resource nouns under `/api/v1`, kebab-case for multiword segments, no verb segments except the two auth actions the PRD's flow names (`login`, `refresh`, `logout`). Path parameters are UUIDs. |

## 3. The endpoint inventory (T2.3)

Frozen in `docs/endpoint-acceptance-criteria.md` section 5.2, as that section requires: 17 operations across auth, session, invitations, platform support-operator management and memberships, each with its actor, `BE-*`, `AC-*`, request, success response and documented errors. Every error there comes from the catalog of §1.1 of [`design/platform-error-model.md`](platform-error-model.md).

Two inventory facts worth stating here because they are decisions, not descriptions:

- **Business and branch creation is S2, not S1.** `BE-ORG-01` gives S1 the minimum spine so memberships can attach, and the roadmap says S1 "must not grow into full branch management". S1 therefore reads and manages memberships against a spine populated by the seed (T1.5) or by S2's onboarding. S1 ships no business-creation endpoint.
- **`SOPORTE` is provisioned by `ADMIN` from the platform.** `POST /api/v1/admin/support-operators` issues a single-use `SOPORTE` invitation activated via `POST /api/v1/auth/invitations/activate`, and `DELETE /api/v1/admin/support-operators/{accountId}` closes a `SOPORTE` operator account and revokes its sessions.
- **Password policy and cryptography.** Passwords are normalized to Unicode NFC and require at least **12 characters** (Unicode runes, max 128 bytes), hashed with **Argon2id** (`m=19456, t=1, p=2`). Access JWTs use **HMAC-SHA256 (`HS256`)** with a mandatory `JWT_SECRET` (min 32 bytes), and opaque tokens (refresh, invitation, password recovery) are stored in PostgreSQL exclusively as **SHA-256** (`bytea`).
- **`POST /api/v1/auth/refresh` is documented as non-idempotent, deliberately.** A repeated exchange of the same refresh token is not a retry to absorb: it is the reuse signal of T1.2 D6. The inventory records it as non-idempotent, and the atomic single-winner exchange of the T1.3 design is what makes concurrent attempts safe.

## 6. Deliberate boundaries and open items

| Item | Why it is not closed here | Closed by |
| --- | --- | --- |
| Business and branch creation, approval, archival | S2's deliverable; S1 gets only the spine memberships attach to. | S2 |
| Customer profile completion and phone verification | S4. | S4 |
| `SOPORTE` phone/DNI procedures in full | S4 designs the procedure; S1 only provisions the actor. | S4 |
| Adding `pgx/v5` to `go.mod` | Build-time dependency work, not design. | S1 |
| The guard's per-request query shape | Already frozen by the T1.3 design; this document only places the middleware. | — |
| Token signing algorithm and key parameters | Resolved for S1: access tokens use `HS256` (`HMAC-SHA256`) with a mandatory `JWT_SECRET` of at least 32 bytes; passwords use `Argon2id` (`m=19456, t=1, p=2`) with Unicode NFC normalization and a minimum of 12 characters; opaque tokens are persisted as `SHA-256`. | S1 (closed) |
| A `SOPORTE` invitation path in the inventory | Resolved for S1: `ADMIN` provisions and revokes `SOPORTE` from the platform via `POST /api/v1/admin/support-operators` (`inviteSupportOperator`) and `DELETE /api/v1/admin/support-operators/{accountId}` (`revokeSupportOperator`). | S1 (closed) |
| Server-side failure logging and the log level | Resolved for S1: the structured log level comes from `LOG_LEVEL` (`debug`, `info`, `warn`, `error`, default `info`); an unrecognized value fails startup instead of falling back silently, because a typo must not hide the logs the operator asked for. A recovered panic keeps the process alive, answers the declared `500` with `urn:canchalibre:error:internal` instead of a body-less 500, and is recorded at ERROR with its stack and request id, so it correlates with the access-log line of the same request; the unexpected-error lines of both adapters carry that request id too. An expired request deadline is recorded at ERROR with its request id, while the `504` it returns stays a transport-level answer: the catalog maps a status to exactly one type and has no `504` slug, so publishing it as a problem document would mean declaring a new status on every operation, and a transport timeout is not a refusal an operation performs. | S1 (closed) |

## 7. Review checklist

- [ ] Every documented error in the inventory uses a `type` from the §1.1 catalog of [`design/platform-error-model.md`](platform-error-model.md).
- [ ] No `about:blank` is documented for any identity endpoint, and the `huma.NewError` override is total: every status maps to a catalog URN and no status falls through to `about:blank`.
- [ ] Every body-accepting operation declares `415`.
- [ ] Each documented error has an explicit huma `Responses` entry with a per-error example, and a contract test asserts it.
- [ ] An unrouted path and a wrong method both return problem details, not `text/plain`.
- [ ] A `404` for a missing record and a `404` for another actor's record are byte-identical in shape.
- [ ] Refusals leave domain tables unchanged; only `identity_audit` may gain a row.
- [ ] A panic inside an operation answers `500` with a catalog URN and is logged at ERROR with its stack and the request id; an expired request deadline is logged at ERROR, and the `504` it returns is the recorded transport-level boundary.
- [ ] No context imports another context's `core`.
- [ ] `response.go` is gone and no caller regressed.
- [ ] `provision-admin` prints the activation URL once, stores no password, and refuses without `--supersede` when an unexpired invitation exists.
- [ ] `POST /api/v1/auth/refresh` is documented as non-idempotent, and concurrent exchanges still produce exactly one winner.

## Next step

The routes are frozen, the error model, layout and command are contracted, and S1 (identity) is implemented and closed on the unmerged `feat/s1-identity` branch. This documentation branch sits on top of `feat/metrics-endpoint`, which sits on top of it. Push, pull request and merge remain the user's decision, and the next slice does not start until the user authorizes it.
