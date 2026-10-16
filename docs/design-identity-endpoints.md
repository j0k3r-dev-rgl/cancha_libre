# Identity endpoints: error model, module layout and the provisioning command

This closes the three cross-cutting design items that `docs/roadmap-backend.md` section 5.2 requires before S1 (identity) implements anything: the stable RFC 7807 problem-type catalog, the bounded-context module layout that replaces the flat bootstrap, and the contract of the private first-`ADMIN` command (`BE-ACC-04`). The endpoint inventory itself lives in the roadmap, where section 5.2 requires it.

It pairs with [`docs/design-identity-auth.md`](design-identity-auth.md) (token and active context) and [`docs/design-identity-ddl.md`](design-identity-ddl.md) (tables). It decides no table DDL, no new product requirement, and no endpoint beyond the identity slice.

## Decisions

| # | Topic | Decision |
| --- | --- | --- |
| D1 | Problem `type` values | Every documented error carries a **stable** `type` URN of the form `urn:canchalibre:error:<slug>`, never `about:blank`, so a client can branch on `type` instead of prose. |
| D2 | How huma emits them | `huma.NewError` is an overridable `var`; the project replaces it once to map status → URN, and each operation declares its errors as **explicit `Responses` entries with a per-error `Example`** (see §1.3 for why `Errors []int` alone is not enough). |
| D3 | Authenticated vs unauthenticated | `401` means "no valid credential"; `403` means "valid credential, no permission". Both are problem details, and a refusal mutates nothing (the only write a refusal may produce is the append-only audit row of the T1.3 design). |
| D4 | Existence disclosure | A record that does not exist and a record that belongs to another actor both return the **same** `404` with the same `type` and no distinguishing detail. |
| D5 | Module layout | Bounded contexts move to `internal/modules/{context}/core/{domain,ports,usecase}` and `adapters/{inbound,outbound}`, per `BE-ENG-07`. Cross-cutting, domain-free code lives in `internal/platform/`. The flat `internal/{domain,adapter,usecase,config}` is **removed** in S1, as `BE-ENG-07` already states it is bootstrap scaffolding. |
| D6 | Shared actor contract | The resolved actor travels as a portable value type in `internal/platform/actor`, so consuming contexts never import identity's core. It is a transport value, not authority. |
| D7 | Where the guard lives | The middleware that enforces the T1.2 invariant lives in `internal/platform/authn` and receives the account, membership and scope lookups as **ports wired at `cmd/api`**. It calls use cases; it never reads tables itself. |
| D8 | Provisioning command | A separate binary `core/cmd/provision-admin`, not an HTTP endpoint and not a subcommand of the API. A private server command cannot be reached over the network, which is the whole point of `BE-ACC-04`. |
| D9 | No default password | The command issues a **single-use, expiring invitation** and prints the activation URL once. It never generates a password, never stores one, and never logs the token. |
| D10 | Route style | Plural resource nouns under `/api/v1`, kebab-case for multiword segments, no verb segments except the two auth actions the PRD's flow names (`login`, `refresh`, `logout`). Path parameters are UUIDs. |

## 1. The error model (T2.1)

### 1.1 Catalog

One row per `type`. The catalog is closed: an operation may only document types from this table.

| `type` | Status | Meaning | Mutates nothing |
| --- | --- | --- | --- |
| `urn:canchalibre:error:validation` | 400, 422 | Request shape or field constraint failed. The `errors[]` array names each field. | yes |
| `urn:canchalibre:error:unauthenticated` | 401 | No valid credential: missing, malformed, expired or revoked. | yes |
| `urn:canchalibre:error:refresh-reuse` | 401 | A rotated refresh token was presented again. The whole session family is revoked (T1.2 D6). | revokes the family, by design |
| `urn:canchalibre:error:forbidden` | 403 | Authenticated, but not permitted: no active membership, disabled business or branch, or a role without the capability. | yes |
| `urn:canchalibre:error:not-found` | 404 | No such record, **or** a record outside the actor's scope. Indistinguishable on purpose (D4). | yes |
| `urn:canchalibre:error:conflict` | 409 | The request contradicts current state: an email already used for that account kind, a duplicate active membership. | yes |
| `urn:canchalibre:error:last-co-owner` | 409 | Revoking the membership would leave the business without a co-owner (`BE-ACC-05`). | yes |
| `urn:canchalibre:error:invitation-consumed` | 409 | The invitation was already activated. | yes |
| `urn:canchalibre:error:invitation-expired` | 409 | The invitation is past its expiry. | yes |
| `urn:canchalibre:error:context-not-authorized` | 403 | The requested business + branch is not covered by an active membership. The claim is never trusted. | yes |
| `urn:canchalibre:error:method-not-allowed` | 405 | The path exists; the method does not. | yes |
| `urn:canchalibre:error:unsupported-media-type` | 415 | Wrong `Content-Type`. Emitted by huma's own negotiation on any body-accepting operation, so it must be declared per operation (§1.3). | yes |
| `urn:canchalibre:error:internal` | 500 | Unexpected failure. Never carries SQL, driver or provider internals (`BE-ENG-06`). | yes |

### 1.2 Shape

`type`, `title`, `status`, `detail`, and `errors[]` for field-level detail. `instance` stays unset: there is no error-log service to point at, and fabricating a URL would be worse than omitting it. `title` is the HTTP status text and never varies between occurrences of the same `type`; `detail` is the only human-facing variable text.

### 1.3 Mechanism, verified rather than assumed

`huma.NewError` is an overridable `var`, so the status → URN mapping is applied in one place. Verified against huma v2.39.1 in a scratch module:

- The override reaches the wire: a handler returning `huma.Error403Forbidden` answered `403`, `Content-Type: application/problem+json`, `type = urn:canchalibre:error:forbidden`, `title = Forbidden`.
- **`Errors: []int` alone does not publish the `type`.** `defineErrors` expands every code in that list from a single `huma.NewError(0, "")` example, so the contract shows one generic schema and the statement "the contract declares a stable `type`" would be false.
- Declaring the error as an explicit `Responses["403"]` entry with `Content: {"application/problem+json": {Schema: ErrorModel, Example: {...type...}}}` **does** carry the URN into the published document (verified: the generated YAML contains the urn and the media type).

Consequence for S1, and it is a task-level rule: **every documented error of every frozen endpoint gets an explicit `Responses` entry with a per-error example.** `Errors []int` is not a substitute, and a contract test must assert each documented `type` appears in the served document. This is also what lets `oas3-missing-example` be re-enabled, the item T0 left open.

Two further findings, both observed while probing this mechanism, and both turned into rules because each one silently breaks D1:

- **A status the override does not map leaks `about:blank`.** With an override that mapped only `404`, a `huma.NewError(422, …)` produced `type = "about:blank"` — the exact outcome D1 forbids. The override must therefore be **total and fail closed**: an unmapped status maps to `urn:canchalibre:error:internal` rather than falling through to `about:blank`. The `type` is chosen by the override, never by huma's default.
- **`415` is reachable without any endpoint declaring it.** huma's own `Content-Type` negotiation emits it for every operation with a body: `POST` with `Content-Type: text/plain` answered `415` with `type = urn:canchalibre:error:unsupported-media-type`, and the generated contract did **not** declare `415` for that operation. So each body-accepting operation must declare `415` explicitly, or its contract omits a refusal it actually performs. The catalog row stays, and this is how it is used.

### 1.4 The fallback

An unrouted path must not fall back to Chi's plain-text `404`. Verified with the fallback installed:

| Request | Observed before | Observed with the fallback |
| --- | --- | --- |
| `GET /api/v1/ping` (routed) | 200 | 200 `application/json` |
| `POST /api/v1/ping` (wrong method) | — | 405 `application/problem+json`, `urn:canchalibre:error:method-not-allowed` |
| `GET /api/v1/nope` (unrouted) | 404 `text/plain` | 404 `application/problem+json`, `urn:canchalibre:error:not-found` |

So `r.NotFound` and `r.MethodNotAllowed` are both wired to render a problem detail. This closes the open item T1 routed to S1 and T0.4's missing live error path, since `405` and `404` are then exercised at runtime by the harness.

## 2. Module layout (T2.2)

```text
core/
  cmd/
    api/                    # composition root: wires ports, owns /health and /ping
    provision-admin/        # private first-ADMIN command (D8)
  internal/
    modules/
      identity/
        core/{domain,ports,usecase}
        adapters/inbound/http
        adapters/outbound/postgres/schema/
      organization/
        core/{domain,ports,usecase}
        adapters/inbound/http
        adapters/outbound/postgres/schema/
    platform/
      actor/                # portable resolved-actor value type (D6)
      authn/                # guard middleware, ports injected at cmd/api (D7)
      http/                 # health/ping probes and the problem-details encoder
      config/               # environment loading
```

| Current path | Fate in S1 |
| --- | --- |
| `internal/domain/errors.go` | Replaced by per-context sentinel errors in `modules/{context}/core/domain`. |
| `internal/adapter/http/router.go` | Split: `/health` and `/api/v1/ping` move to `platform/http`; the huma config and problem-details wiring become `platform/http`; route registration moves to the composition root. |
| `internal/adapter/http/response.go` | **Deleted.** Its RFC 7807 struct is superseded by the huma error model of §1; it has no caller today, and S1 is where the roadmap already assigned its removal. |
| `internal/adapter/http/router_test.go` | Moves with the probes to `platform/http`, keeping the existing contract assertions. |
| `internal/config/config.go` | Moves to `platform/config`, gaining the pool and JWT settings the identity slice needs. |
| `internal/adapter/postgres/` (empty) | Removed. Each context owns its pool and its apply-on-reset `schema/*.sql`, applied in lexical order when the API binary recreates the local database (T3.2). |
| `internal/usecase/` (empty) | Removed. |

Rule this layout creates and S1 must honor: **no cross-context imports of `core`.** `booking` never imports `identity/core`; it receives what it needs through an inbound port wired at `cmd/api`. A contract test greps for forbidden import edges, because this is the boundary that is cheapest to check and most expensive to repair later.

## 3. The endpoint inventory (T2.3)

Frozen in `docs/roadmap-backend.md` section 5.2, as that section requires: 15 operations across auth, session, invitations and memberships, each with its actor, `BE-*`, `AC-*`, request, success response and documented errors. Every error there comes from the catalog of §1.1.

Two inventory facts worth stating here because they are decisions, not descriptions:

- **Business and branch creation is S2, not S1.** `BE-ORG-01` gives S1 the minimum spine so memberships can attach, and the roadmap says S1 "must not grow into full branch management". S1 therefore reads and manages memberships against a spine populated by the seed (T1.5) or by S2's onboarding. S1 ships no business-creation endpoint.
- **`POST /api/v1/auth/refresh` is documented as non-idempotent, deliberately.** A repeated exchange of the same refresh token is not a retry to absorb: it is the reuse signal of T1.2 D6. The inventory records it as non-idempotent, and the atomic single-winner exchange of the T1.3 design is what makes concurrent attempts safe.

## 4. The provisioning command (T2.4)

```text
core/cmd/provision-admin --email <email> [--ttl <duration>] [--supersede] [--dry-run]
```

| Aspect | Contract |
| --- | --- |
| Input | `--email` (required, validated as an email), `--ttl` (default `24h`), `--supersede` (replace an existing unexpired `ADMIN` invitation), `--dry-run` (validate and report, change nothing). |
| Environment | `DATABASE_URL` and the invitation signing secret. Missing either is a usage failure, never a silent default. |
| Effect | Creates a `WORK` account in `PENDING` if the email has none, and one single-use `ADMIN` invitation via the same `invitation` table and `invitation_scope_shape` the T1.3 plan already defines. |
| Output | The activation URL, printed **once** to stdout. Never to the structured logger, never to a file the command creates. |
| Refusals | An existing unexpired `ADMIN` invitation without `--supersede`; an email already an active `ADMIN`; a malformed email. Exit code 1 with a plain reason on stderr. |
| Exit codes | `0` success, `1` refused or failed, `2` usage error. |
| Password | None, ever. The activation step is the account's only way to receive a credential (`BE-ACC-04`). |
| Audit | One `identity_audit` row with the actor as the OS-command invocation (`actor_account_id` null, detail recording the binary and the operator's `--email` argument). |

The invitation is activated through `POST /api/v1/auth/invitations/activate` from the inventory, so the private command and the co-owner/employee invitation path share exactly one activation mechanism.

## 5. Verification performed

Against huma v2.39.1 in a throwaway module (Go 1.27.2), using the pinned versions from `core/go.mod`:

| # | Claim | Result |
| --- | --- | --- |
| 1 | `huma.NewError` is overridable | Compiles and replaces the mapper. |
| 2 | The override reaches the wire | `403`, `application/problem+json`, `type = urn:canchalibre:error:forbidden`. |
| 3 | `Errors []int` alone publishes the URN | **False.** One shared schema; the contract carries no `type`. Recorded because the opposite is the easy assumption. |
| 4 | An explicit `Responses` entry with a per-error example publishes the URN | **True.** The generated YAML contains the urn and the media type. |
| 5 | The fallback makes an unrouted path problem-shaped | `404`, `application/problem+json`, `urn:canchalibre:error:not-found`. |
| 6 | Wrong method on a real route is a `405`, not a `404` | `405`, `application/problem+json`, `urn:canchalibre:error:method-not-allowed`. |
| 7 | An unmapped status leaks `about:blank` | **True** — `huma.NewError(422, …)` under a partial override returned `type = "about:blank"`. Hence the total, fail-closed override of §1.3. |
| 8 | `415` is reachable via content negotiation | **True** — `POST` with `Content-Type: text/plain` returned `415` and the operation's contract did **not** declare `415`. Hence the per-operation declaration rule. |

The scratch module was removed; nothing was written to `core/`.

## 6. Deliberate boundaries and open items

| Item | Why it is not closed here | Closed by |
| --- | --- | --- |
| Business and branch creation, approval, archival | S2's deliverable; S1 gets only the spine memberships attach to. | S2 |
| Customer profile completion and phone verification | S4. | S4 |
| `SOPORTE` phone/DNI procedures in full | S4 designs the procedure; S1 only provisions the actor. | S4 |
| Adding `pgx/v5` to `go.mod` | Build-time dependency work, not design. | S1 |
| The guard's per-request query shape | Already frozen by the T1.3 design; this document only places the middleware. | — |
| Token signing algorithm and key parameters | Named as unresolved in PRD §7 and not needed to freeze routes; S1 decides it with `golang-security`. | S1 |
| A `SOPORTE` invitation path in the inventory | Included as `intended_role: SOPORTE`, but the actor is provisioned, never self-served; S1 confirms whether the operator or the `ADMIN` issues it. | S1, small |

## 7. Review checklist

- [ ] Every documented error in the inventory uses a `type` from the §1.1 catalog.
- [ ] No `about:blank` is documented for any identity endpoint, and the `huma.NewError` override is total: every status maps to a catalog URN and no status falls through to `about:blank`.
- [ ] Every body-accepting operation declares `415`.
- [ ] Each documented error has an explicit huma `Responses` entry with a per-error example, and a contract test asserts it.
- [ ] An unrouted path and a wrong method both return problem details, not `text/plain`.
- [ ] A `404` for a missing record and a `404` for another actor's record are byte-identical in shape.
- [ ] Refusals leave domain tables unchanged; only `identity_audit` may gain a row.
- [ ] No context imports another context's `core`.
- [ ] `response.go` is gone and no caller regressed.
- [ ] `provision-admin` prints the activation URL once, stores no password, and refuses without `--supersede` when an unexpired invitation exists.
- [ ] `POST /api/v1/auth/refresh` is documented as non-idempotent, and concurrent exchanges still produce exactly one winner.

## Next step

With the routes frozen and the error model, layout and command contracted, S1 (identity) can start without re-deciding any of them. S1 itself remains unauthorized until the user says so.
