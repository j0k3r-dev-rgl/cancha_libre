# CanchaLibre — Backend delivery roadmap

**Status:** Planning artifact. It orders the work, fixes how endpoints are documented and verified, and defines the acceptance criteria that close each slice. It does **not** authorize implementation, deployment, purchase, or service activation.

**Sources of truth:** product behavior in [`docs/mvp.md`](mvp.md); backend requirements in [`docs/prd-backend.md`](prd-backend.md). `BE-*` identifiers come from the PRD, `AC-*` from the MVP. This roadmap adds no new product requirement.

## 1. How to read this document

- The work is split into eight capability slices plus one blocking toolchain block (**T0**). Slices are ordered; each one is reviewable and verifiable on its own.
- **Routes are not frozen anywhere in this roadmap.** They are frozen only after block T0 closes (section 3).
- Acceptance criteria live at three levels — slice, endpoint, task (section 5) — and every endpoint additionally requires empirical verification against a running instance (section 5.4).
- A slice is not "done" because code exists: it is done when its criteria are demonstrated with evidence.

## 2. Slice map

| Slice | Capability | Backend requirements | Depends on | Acceptance examples closed |
| --- | --- | --- | --- | --- |
| **T0** | OpenAPI 3.1 toolchain (blocks route freezing) | BE-ENG-06 | — | — |
| **S1** | Accounts, actors, memberships, explicit branch context, authorization guards, plus the minimal business/branch/membership spine | BE-ACC-01…07, BE-ORG-01 (minimum spine), BE-ENG-01/05 | T0 | AC-18, AC-26, AC-42 (partial), AC-43 |
| **S2** | Business enabling, branches, courts, branch rules, approvals, archival, public photo gallery, geocoding that publication depends on | BE-ORG-02…07, BE-DIS-03/05, BE-ENG-07 | S1 | AC-03 (first half), AC-18, AC-25, AC-27, AC-32 (timezone), AC-40 (partial), AC-41 |
| **S3** | Offered slots, whole-batch generation, exclusion, blocks, reopening | BE-OFF-01…09, BE-ENG-02 | S2 | AC-01, AC-02, AC-03, AC-10, AC-11, AC-12, AC-30, AC-31, AC-33 (partial) |
| **S4** | Customer profile completion, phone verification, DNI fingerprint and support procedure, delivery channel, post-closure retention | BE-ACC-08, BE-ID-01…08, BE-NOT-03, BE-ENG-04 | S1 | AC-16, AC-24, AC-44, AC-45 (partial), AC-46, AC-47 |
| **S5** | Public search, proximity, ordering, map/address failure behaviour | BE-DIS-01/02/04 | S2, S3 | AC-23, AC-50, AC-51 |
| **S6** | Requests, confirmation, cancellation, attendance, pending limits, local customer blocks, notice behaviours booking produces | BE-ACC-09, BE-RES-01…12, BE-NOT-01/02/04/05, BE-ENG-02/03/06 | S3, S4 | AC-04…AC-09, AC-13, AC-15, AC-17, AC-22, AC-28, AC-29, AC-35, AC-36, AC-37, AC-40 (completion), AC-48, AC-49 |
| **S7** | Accumulated amounts by method and calculated status | BE-PAY-01…03 | S6 | AC-14, AC-34 |
| **S8** | Business and branch suspension and reactivation | BE-SUS-01…03, BE-ENG-03 | S2, S3, S6 | AC-19 (completion), AC-20, AC-21, AC-33 (completion), AC-38, AC-39 |
| **Operación** | External-service provisioning, budget controls, and measured server capacity (no code) | BE-NOT-06/07, BE-ENG-08 | S4, S5 | AC-52 |

### Dependency corrections applied

The original capability split contained two cycles; both are resolved here.

1. **`BE-ACC-05/06` need an organization spine.** Memberships attach to a business and a branch, so S1 owns the minimal spine (business, branch, membership). S2 then owns the commercial and public surface of the same context. S1 must not grow into full branch management.
2. **`BE-ORG-03` needs confirmed coordinates before publication**, and geocoding sat in the discovery slice. Geocoding (`BE-DIS-03`) moved into S2; S5 keeps search, proximity and failure behaviour only. `BE-DIS-05` (Argentine timezone) is applied from S2 because lead times, the 180-day horizon and cutoffs all depend on it.

## 3. Blocks before the slices

### Block T0 — OpenAPI 3.1 toolchain

**Decision: `danielgtaylor/huma` v2 + `humachi`, pinned to v2.39.1.**

| Option | License | Stars | Release | Last push | Assessment |
| --- | --- | --- | --- | --- | --- |
| **huma v2 + humachi** | MIT | 4.461 | v2.39.1 | 2026-10-15 | **Chosen.** Code-first, OpenAPI 3.1 native, active, mature major version, chi adapter |
| `oapi-codegen` | Apache-2.0 | 8.620 | v2.8.0 | 2026-10-16 | Viable spec-first alternative; inverts the flow (spec authored before code) |
| `swaggest/openapi-go` | MIT | 358 | v0.2.61 | 2026-04-14 | Pre-1.0 and the least active |
| `oaswrap/spec` + `chiopenapi` | MIT | 120 | v0.5.2 | 2026-07-25 | Pure generator, no handler coupling, but pre-1.0 and low traction |
| `gork-labs/gork` | MIT | 1 | none | 2026-10-11 | Rejected: no releases, 1 star |
| `swaggo/swag` | — | — | — | — | Rejected: emits Swagger 2.0 only (already flagged in PRD §7) |

**Adoption consequence:** huma defines the handler signature, so it sets the style of every `adapters/inbound/http` boundary. Handlers stay thin: validate and map, call the use case, map the result. It does not reach into `core/`.

**Why it must come first:** the OpenAI 3.1 toolchain determines route shape, parameter and body declaration style, and how error responses are described. Freezing routes before choosing it guarantees rewriting them.

#### Tasks

| ID | Task | Evidence that closes it |
| --- | --- | --- |
| T0.1 | Pin `huma` v2.39.1 and its transitive dependencies; run the dependency audit from `dependency-selection-audit` (license, CVE, bloat) | Pinned `go.mod`, `govulncheck` clean, license check recorded — see *Dependency audit record* below |
| T0.2 | Mount `humachi.New` over the existing chi router without changing `/health` and `/api/v1/ping` behaviour | Existing health test still passes; no route regression |
| T0.3 | Expose `/openapi.json` in OpenAPI 3.1 and lint it | Spec validates as 3.1 and passes the lint gate — see *Lint gate* below |
| T0.4 | Prove one endpoint round-trips spec ↔ handler, including one documented error response | Spec entry and observed success response agree, and the error response is declared with the RFC 7807 media type. Exercising a live error response is deferred to S1, where a handler has a real failure path — see *Open items* |
| T0.5 | Give `api-tools` a way to read a Go/huma spec, or a documented fallback, and add the project `.pi/api.json` | Tool returns the spec through the extension and reports failures as failures |

**T0.5 resolution:** the extension framework list gained a generic `openapi` mode whose candidate URL is `/openapi.json`, so a Go/huma spec is discovered by `api_swagger` instead of falling back to `/v3/api-docs`. The extension test suite (67 tests) and `tsc --noEmit` pass, and a live run against the API listed `GET /health` and `GET /api/v1/ping`. The empirical verification in section 5.4 can therefore cover the contract.

**Exit criteria for T0:** the spec is valid 3.1, one endpoint is verified end to end, the extension can read the spec, and the toolchain versions are pinned. Only then does route freezing start, slice by slice.

**Closed by:** commit `477719b` (`feat(openapi): serve the OpenAPI 3.1 contract and gate its linting`), which carries the toolchain pins, the contract, the lint gate, the api-tools support and this roadmap. The specification round this roadmap builds on is committed as `f68f6d8` (`docs(core): apply the first decision round to the MVP and PRD`), so the sections cited from `docs/prd-backend.md` match the committed copy.

#### Dependency audit record (T0.1)

Audit run against `core/` with the Go vulnerability database of 2026-10-16.

- **Binary closure.** `go list -deps ./cmd/api` links exactly three modules: `canchalibre/core`, `github.com/danielgtaylor/huma/v2@v2.39.1` and `github.com/go-chi/chi/v5@v5.3.2`. The 68 modules reported by `go list -m all` (including the main module; gin, fiber v2/v3, echo v4/v5, quic-go, mongo-driver, fasthttp, …) are huma's test dependencies: they exist in `go.sum` and never reach the binary.
- **Licenses.** huma v2.39.1 MIT; chi v5.3.2 MIT. No copyleft inside the binary closure.
- **Bloat.** Beyond the standard library, the only build inputs are the two modules above; nothing else is compiled or shipped.
- **Toolchain patch.** The repository pins Go 1.27.2 in `.mise.toml` and `core/go.mod`, because Go 1.27.1 carried six standard library vulnerabilities that reach the HTTP server (`http.Server.ListenAndServe` and `Shutdown` in `cmd/api/main.go`), all fixed in 1.27.2: GO-2026-6617, GO-2026-6613, GO-2026-6612, GO-2026-6611, GO-2026-6608 and GO-2026-6603. `govulncheck ./...` on 1.27.2 reports none of them.
- **Baseline noise, now closed.** On the 1.27.1 baseline `govulncheck` additionally reported five vulnerabilities in imported packages whose vulnerable symbols are never called (GO-2026-6610, 6609, 6607, 6605 and 6604, the last one Windows only) and two in required modules (`html/template`: GO-2026-6600 and 6599). All seven were also standard library issues fixed in 1.27.2, so the pinned toolchain reports none of them either. They are recorded so a subsequent audit does not report them as new.
- **Dead code.** `internal/adapter/http/response.go` (`WriteJSON`, `WriteProblemDetails`, `ProblemDetails`) has no caller after the huma migration. It is resolved in S1.

#### Response `$schema` link convention (decision)

huma's default `SchemaLinkTransformer` is kept. Every struct response body carries a `$schema` field pointing at `/schemas/{Type}.json` plus a `Link: </schemas/{Type}.json>; rel="describedBy"` header, and the same transformer declares that field in `components.schemas.{Type}` as `readOnly: true`.

This was initially reported as contract drift; it is not. Verified against a running instance: the served body validates against the published contract. The outcome is a rule, recorded in `.agents/skills/api-contracts-openapi/SKILL.md`: a response DTO is not documented until a test asserts the `$schema` field, its target document and its read-only declaration in the contract. The documented `example` URL stays generic (`https://example.com/schemas/...`) because the configuration declares no `Servers`; the served value uses the request host.

#### Lint gate (T0.3)

The contract is linted with `vacuum`, pinned in `.mise.toml` and installed as a cosign-verified release binary through mise's aqua backend, so it never enters the shipped dependency closure. The rules come from `vacuum.ruleset.yaml`, which enables the recommended ruleset minus five rules, each silenced with its reason in that file.

From the repository root, with the API serving the spec:

```bash
mise exec -- vacuum lint -r vacuum.ruleset.yaml --fail-severity error --min-score 98 <spec-file-or-url>
```

**Gate:** exit code 0 and a quality score of at least 98. It applies to every endpoint before its slice can close.

The score floor is not decoration: it is what makes the silences load-bearing. With the recommended ruleset and no silences the current spec scores 96 and fails; with the silences it scores 100 and passes. Errors fail on their own, and the remaining warnings are advisory until they consume the two points of headroom the floor leaves.

**Progress:** the very first run — made before this ruleset existed, so with the recommended set and no silences — reported 1 error, 16 warnings and 6 informs (score 79/100). The error and three warnings were ours and were fixed in the router: the missing `info.description`, the undeclared `system` tag used by both operations, the missing descriptions of `HealthOutputBody` and `PingOutputBody`, and the missing `servers` entry, which also made the documented `$schema` example a truthful URL. The five remaining rules cover huma's built-in error schemas, the `$schema` keyword and huma's repeated response descriptions. Current run: exit code 0, score 100/100.

**Accepted cost:** silencing `oas3-missing-example` also stops enforcing examples on our own success responses. Re-enable it in the same task that gives the project its own RFC 7807 error model. The gate command is manual today; folding it into the runner of section 5.4 belongs to that pending item. Every silence is load-bearing: removing the ruleset drops the score to 96, below the floor.

### Block T1 — preparation before identity

Slice S1 starts from written design, not from decisions taken while coding it. This block prepares exactly what section 4 says S1 needs, and it is the only work authorized before the slices. It delivers design, harness and seed layout — never product behaviour. A task that would change observable product behaviour stops and asks for a product decision first, as section 7 requires.

#### Tasks

| ID | Task | Evidence that closes it |
| --- | --- | --- |
| T1.1 | Record the authorization boundaries of this block | `docs/prd-backend.md` states that this preparation block is authorized while the slices are not; the block is defined here with its tasks and exit criteria |
| T1.2 | Design the auth token and the active-context transport (section 7, *Auth token and branch-context transport*) | A written design: token layout, TTL/refresh lifecycle and revocation, and how a staff request selects its active business + branch validated against active memberships — never trusted from a client-supplied identifier |
| T1.3 | Design the identity DDL and the minimal business/branch/membership spine (section 7, *Concrete table DDL and exclusion predicates*) | An apply-on-reset SQL plan for the identity schema and the spine, with the predicates, indexes and audit-record shape that S1 and S4 consume |
| T1.4 | Build the section 5.4 verification harness | A project subagent definition that is read-only over the repository, plus a runner that boots the API, reads the spec through `api-tools`, exercises the case matrix and returns a verdict per cell — delivered as `.pi/agents/api-contract-verifier.md`, `.pi/extensions.json` and `scripts/verify-contract.sh`, self-tested end to end |
| T1.5 | Fix the seed layout for the five actors and the two branches | A defined seed file layout referenced from the ignored `.pi/api.json` through `auth.accounts_file`, so the credentials stay out of the repository — delivered as the committed `.pi/api.json.example` and `.pi/api.json.local.example`, the gitignored `.pi/api.json.local`, and the committed credential-free `core/dev/seed.topology.json`; proven by resolving `ADMIN` through the extension |
| T1.6 | Close the toolchain reproducibility debt | `govulncheck` pinned in `.mise.toml` so the T0.1 audit reproduces from the repository, and the `openapi.json` question decided — pinned as `go:golang.org/x/vuln/cmd/govulncheck@1.9.0` (verified: `No vulnerabilities found` with no `@latest`), and `openapi.json` is generated only during checks, never committed |

**Exit criteria for T1:** the identity slice can start without re-deciding anything — section 7 is resolved for identity, the harness of section 5.4 runs, the seed layout is fixed, and block T0's evidence reproduces from a clean clone.

**Delivered so far:** T1.1 recorded the block's authorization boundary; T1.2 is [`docs/design-identity-auth.md`](design-identity-auth.md) (token layout, active-context claim, TTL/refresh lifecycle and the guard invariant that makes a revocation immediate); T1.3 is [`docs/design-identity-ddl.md`](design-identity-ddl.md) (the apply-on-reset SQL for `schema_identity` and the `schema_organization` spine, with the guard's single indexed read and the atomic refresh exchange, each verified against a real PostgreSQL instance); T1.4 is the section 5.4 harness (`.pi/agents/api-contract-verifier.md`, `.pi/extensions.json`, `scripts/verify-contract.sh`); T1.5 is the seed layout (section 5.4); T1.6 pinned `govulncheck` and decided against committing `openapi.json`.

**What this block takes over from section 8:** the section 5.4 subagent definition (T1.4), the seed credentials layout (T1.5) and the committed-spec question (T1.6).

**What this block does not touch:** the dead code left by the huma migration (`internal/adapter/http/response.go`) stays with S1, where the roadmap already assigned it, and the identity slice itself stays unauthorized until the user authorizes it.

### Block T2 — identity routes frozen (diseño, sin código)

Section 5.2 requires each slice to add its endpoints to an inventory **before** they are implemented, and T1 did not cover that. This block supplies it, together with the three cross-cutting contracts an endpoint cannot be frozen without: the stable RFC 7807 problem-type catalog, the bounded-context module layout that replaces the flat bootstrap (`BE-ENG-07`), and the private first-`ADMIN` command of `BE-ACC-04`. It delivers design only — no product behaviour, no Go code — and S1 stays unauthorized.

| ID | Task | Evidence that closes it |
| --- | --- | --- |
| T2.1 | Fix the RFC 7807 problem-type catalog and the error convention | [`docs/design-identity-endpoints.md`](design-identity-endpoints.md) §1: the closed `type` catalog, the shape, the verified huma mechanism, and the unrouted-path fallback |
| T2.2 | Fix the bounded-context module layout and the fate of the flat scaffolding | Same document §2: the target tree, and the fate of each current file |
| T2.3 | Freeze the identity endpoint inventory as section 5.2 rows | The inventory in section 5.2 of this document: 15 operations with actor, `BE-*`, `AC-*`, idempotency, request, success response and every documented error |
| T2.4 | Fix the contract of the private first-`ADMIN` provisioning command | Same document §4: the `provision-admin` contract, its refusals and its exit codes |
| T2.5 | Reconcile section 7/8 so nothing frozen is still called pending | This document's section 8 and `docs/prd-backend.md` section 7 |

**Exit criteria for T2:** the identity endpoints are frozen with every error typed, the module layout is decided, the provisioning command is contracted, and S1 can start without re-deciding any route. The identity slice itself remains unauthorized until the user authorizes it.

## 4. What each slice delivers

Every slice follows the same shape: deliverable, out of scope, verification required to close.

### S1 — identity
- **Delivers:** independent customer and work accounts that may share an email; single-use invitation activation; local sign-in, logout and recovery; the first `ADMIN` provisioned by private server command; co-owner assignment and revocation with last-co-owner protection; employee invitations and branch memberships; resolution of the active business + branch context validated against memberships; the actor guard matrix.
- **Out of scope:** social sign-in and explicit account linking (`BE-ACC-01/03` stay partial); customer profile completion and phone verification (S4); local customer blocks (S6); full branch management (S2).
- **Verification:** domain tests over memberships and context; the full permission matrix over HTTP, asserting each allowed and each refused cell; refusals return RFC 7807 and mutate nothing; Postgres integration for memberships; concurrency on assignment and revocation; invitation token single-use and expiry.

### S2 — organization
- **Delivers:** business creation and enabling; branch requests with approval, rejection reason and preserved review history; direct owner editing of public fields; public profile with photo gallery; courts and sports; default duration and base price; branch request/cancellation lead times; geocoding of the branch address and coordinate confirmation; archival and unarchival with their prerequisites.
- **Out of scope:** offered slots (S3); suspension (S8); public search (S5).
- **Verification:** every context's `schema/*.sql` applies from scratch on the local-start reset in lexical order, leaving no residue of the previous state; per-field authorization; publication refused without confirmed coordinates; archival refused with unresolved pending requests or confirmed reservations and never silently cancelling anything; photo type/size validation; lead-time boundary values (0 and 10.080 accepted, negative and fractional refused, invalid settings leave stored values unchanged).

### S3 — inventory
- **Delivers:** single slot creation; batch generation from court, dates, range, duration and price; complete-intervals-only with the excluded remainder shown before confirming; whole-batch rejection on any conflict regardless of sport; 180-day horizon in the Argentine timezone; blocking and reopening; offer retirement that preserves history.
- **Out of scope:** requests and reservations (S6); recurring weekly generation (out of MVP).
- **Verification:** exclusion constraint proven with two real concurrent transactions against Postgres — a race-detector pass alone is not proof; batch conflict creates nothing and leaves existing offers unchanged; 180/181-day boundary with controlled clock; reopening refused when scope is disabled, the slot started, or another restriction remains.

### S4 — identity verification
- **Delivers:** customer profile completion and booking eligibility; six-digit WhatsApp verification code with its expires/attempts/between-sends/per-hour limits; separation between verification and notice consent; DNI normalization, protected fingerprint, uniqueness, restricted support review and correction, and post-closure retention.
- **Out of scope:** reservation notices themselves (S6); publication and branch data (S2).
- **Verification:** controlled time for expiry and the send/attempt limits; concurrency on limits; adapter doubles for success, permanent failure, transient failure, uncertain acceptance and outage; DNI variants collapsing to one fingerprint; no code or DNI in logs; no upload, storage or retention path for identity images.

### S5 — discovery
- **Delivers:** public search without sign-in by city, sport, date and time; proximity from device location or a resolved reference address with a 5 km initial radius adjustable 1–50 km; ordering by straight-line distance or lowest matching price; degradation to a filtered list when the map/address provider fails.
- **Out of scope:** geocoding and coordinate confirmation (S2).
- **Verification:** public responses asserted to contain no holder identity, DNI, contact or payment data; suspended or disabled scope never appears; empty result offers widening without widening automatically; distance edge cases at 1 km and 50 km; Maps double for failure and quota exhaustion.

### S6 — booking
- **Delivers:** customer requests against requestable offers; staff confirmation, rejection and cancellation; the one-per-branch, five-platform-wide and no-duplicate pending limits; inclusive cutoffs, start-time expiry and closure of competing requests; guest allocation without an account; withdrawal with its product label; attendance recording; cross-branch customer overlap; local customer blocks; and the transactional notices booking produces.
- **Out of scope:** payment amounts (S7); suspension (S8).
- **Verification:** competing confirmations against Postgres with real concurrency, asserting exactly one winner and every other pending request closed as unavailable; overlap refusal without creating a second reservation; limits enforced under simultaneous submissions with unchanged counters after refusal; boundary minutes for request and cancellation cutoffs; attendance refused before its exact boundary; notice delivery failure never changes reservation state.

### S7 — payments
- **Delivers:** accumulated ARS amounts per method; total and calculated status (pending, partial, paid); refusal of any total above the agreed price.
- **Out of scope:** any gateway, refund, ledger of movements or automatic collection.
- **Verification:** status derived from the sum, never set independently; over-price refusal leaves stored amounts and status unchanged; a provider-side failure cannot exist because the platform processes no money.

### S8 — suspension
- **Delivers:** business and branch suspension and reactivation; hidden public inventory; scoped staff denial; cancellation of future confirmations and closure of pending requests with a visible suspension reason; preservation of already-started reservations and customer history; independent business and branch states.
- **Out of scope:** refunds or payment handling; administrator access to operational data.
- **Verification:** suspension with pending requests, future confirmations and an already-started reservation asserted end to end; `ADMIN` still refused on reservation, customer and payment reads; reactivation revives nothing and requires manual reopening; independent business/branch suspension combinations.

## 5. Acceptance criteria

### 5.1 Slice level

A slice closes only when all of the following hold:

1. Every `BE-*` assigned to it is implemented and its observable result demonstrated.
2. Every `AC-*` listed for it in section 2 is demonstrated, or explicitly recorded as partial with the slice that completes it.
3. The reset recreates the schema from scratch on every local start and leaves no residue of the previous state.
4. The slice's endpoints appear in the OpenAPI 3.1 spec.
5. Empirical verification from section 5.4 passes with no unexplained case.
6. The relevant checks from `docs/prd-backend.md` §8 ran, and any that could not run is reported as unavailable — never silently skipped.
7. The adversarial verification defined in section 5.4 passes at 100%: the §5.2 case matrix plus the hostile-input, input-normalization and SQL-injection cases, and a single finding blocks the slice.

### 5.2 Endpoint level

Once T0 closes, each slice adds its endpoints to an inventory in this document before they are implemented. Every endpoint row records its method, path, actor, `BE-*` requirements, `AC-*`, request shape, success response, and each documented error with its RFC 7807 `type`. The endpoint is then verified case by case:

| Case | Required outcome |
| --- | --- |
| Valid input, authorized actor | Documented success status and body; state changes exactly as described |
| Invalid input | 400 or 422 with an RFC 7807 `type`; stored data and counters unchanged |
| No authentication | 401, no partial work started |
| Authenticated actor without permission | 403, no state mutation, no information leak |
| Not found or other actor's record | 404 that does not disclose whether the record exists |
| Conflicting state (duplicate, overlap, limit reached) | 409 or 422 with a stable `type`; no reservation, offer, or counter created |
| Repeated identical request | Documented idempotency or documented non-idempotency — never undefined |
| Concurrent duplicates, when the invariant requires it | Exactly one effect; the losers receive a stable refusal |

#### Inventory — S1 (identity)

Frozen before implementation, as this section requires. Every `type` below comes from the catalog in [`docs/design-identity-endpoints.md`](design-identity-endpoints.md) §1.1; no endpoint may document a `type` outside it, and `about:blank` is never documented. Each documented error is registered as an explicit huma `Responses` entry with a per-error example, because `Errors []int` alone does not publish the `type` (verified, same document §1.3).

All routes are served under the `/api/v1` prefix, which the table below omits for readability: `POST /auth/login` is served at `/api/v1/auth/login`. Every path parameter is a UUID. `AMBIENT` means no credential is required.

In the error column of the second table, the value after `status →` is the catalog **slug**; the wire value is `urn:canchalibre:error:<slug>`. So `403 forbidden` means `type = urn:canchalibre:error:forbidden` on the wire. The slug is used only to keep the rows readable, and the mapping is one-to-one.

`—` in the `AC` column means **no acceptance criterion in the MVP covers this operation, deliberately**. The MVP's acceptance table has no criterion for session refresh, token-based recovery or logout, because those are engineering obligations of `BE-ACC-01` rather than customer-visible acceptance examples; filling the cell with an unrelated AC would be a fabricated requirement. Every other row cites the AC it demonstrates.

| Operation ID | Method and path | Actor | Requirements | AC | Idempotency |
| --- | --- | --- | --- | --- | --- |
| `login` | `POST /auth/login` | AMBIENT | BE-ACC-01/02/04, BE-ENG-05 | AC-42 | non-idempotent: each call issues a new session |
| `refresh` | `POST /auth/refresh` | AMBIENT (refresh cookie) | BE-ACC-01 | — | **non-idempotent by design**: a replayed token is the reuse signal that revokes the family |
| `activateInvitation` | `POST /auth/invitations/activate` | AMBIENT (invitation token) | BE-ACC-04, BE-ORG-01 | AC-43 | non-idempotent: activation is single-use |
| `requestPasswordRecovery` | `POST /auth/password/recovery` | AMBIENT | BE-ACC-01 | — | idempotent from the client's view: the same answer whether or not the email exists |
| `resetPassword` | `POST /auth/password/reset` | AMBIENT (single-use token) | BE-ACC-01/04 | — | non-idempotent: the token is consumed |
| `logout` | `POST /auth/logout` | authenticated | BE-ACC-01 | — | idempotent: revoking an already-revoked session succeeds |
| `getOwnAccount` | `GET /me` | authenticated | BE-ACC-02, BE-ENG-05 | AC-42 | idempotent |
| `listOwnContexts` | `GET /me/contexts` | authenticated | BE-ACC-06, BE-ORG-01 | AC-26 | idempotent |
| `selectContext` | `POST /me/context` | authenticated | BE-ACC-06, BE-ENG-01 | AC-18, AC-26 | non-idempotent: issues a new access token |
| `assignCoOwner` | `POST /admin/businesses/{businessId}/co-owners` | ADMIN | BE-ACC-05, BE-ENG-01 | AC-43 | non-idempotent: an existing active membership conflicts |
| `revokeCoOwner` | `DELETE /admin/businesses/{businessId}/co-owners/{accountId}` | ADMIN | BE-ACC-05 | AC-43 | idempotent: revoking an already-revoked membership succeeds |
| `inviteSupportOperator` | `POST /admin/support-operators` | ADMIN | BE-ACC-04/07, BE-ENG-01 | AC-43 | non-idempotent: issues a single-use `SOPORTE` invitation |
| `revokeSupportOperator` | `DELETE /admin/support-operators/{accountId}` | ADMIN | BE-ACC-07 | — | idempotent: revoking an already-closed `SOPORTE` account succeeds |
| `listMemberships` | `GET /businesses/{businessId}/memberships` | OWNER | BE-ACC-06, BE-ENG-05 | AC-26 | idempotent |
| `inviteMember` | `POST /businesses/{businessId}/invitations` | OWNER | BE-ACC-04/06 | AC-43 | non-idempotent: issues a new invitation |
| `revokeMembership` | `DELETE /businesses/{businessId}/memberships/{membershipId}` | OWNER | BE-ACC-06 | AC-26 | idempotent: revoking an already-revoked membership succeeds |
| `revokeInvitation` | `DELETE /businesses/{businessId}/invitations/{invitationId}` | OWNER | BE-ACC-04/06 | AC-43 | idempotent |

| Operation ID | Request shape | Success response | Documented errors (`status` → `type`) |
| --- | --- | --- | --- |
| `login` | `{email, password, accountKind}` | `200` `{access_token, expires_in, context: [{business_id, branch_id, role}] \| null}` | `400` validation; `401` unauthenticated; `403` forbidden (account not `ACTIVE`); `500` internal |
| `refresh` | empty body; refresh token in the `HttpOnly` cookie | `200` `{access_token, expires_in}` and a rotated refresh cookie | `401` unauthenticated (absent/expired); `401` refresh-reuse (**family revoked**); `500` internal |
| `activateInvitation` | `{token, password}` | `200` `{account_id, status}` | `400` validation (password policy); `409` invitation-consumed; `409` invitation-expired; `403` forbidden (email already an active `ADMIN`); `500` internal |
| `requestPasswordRecovery` | `{email, accountKind}` | `202` empty body | `400` validation; `500` internal. Deliberately **no** `404`: the answer is identical whether or not the account exists |
| `resetPassword` | `{token, password}` | `200` empty body | `400` validation; `401` unauthenticated (token invalid or expired — one type, so the reason is not disclosed); `500` internal |
| `logout` | empty body | `204` empty body | `401` unauthenticated; `500` internal |
| `getOwnAccount` | none | `200` `{account_id, email, kind, status, display_name}` | `401` unauthenticated; `500` internal |
| `listOwnContexts` | none | `200` `[{business_id, business_name, branch_id, branch_name, role}]` | `401` unauthenticated; `500` internal |
| `selectContext` | `{business_id, branch_id}` | `200` `{access_token, expires_in}` | `400` validation; `401` unauthenticated; `403` context-not-authorized (no active membership, or disabled business/branch — one type, so scope is not probed); `500` internal |
| `assignCoOwner` | `{account_id}` or `{email, intended_role: OWNER}` | `201` `{membership_id, role, business_id}` | `400` validation; `401` unauthenticated; `403` forbidden (not `ADMIN`); `404` not-found (unknown business); `409` conflict (already an active co-owner); `500` internal |
| `revokeCoOwner` | none | `204` empty body | `401` unauthenticated; `403` forbidden (not `ADMIN`); `404` not-found (unknown business, or no such active co-owner); `409` last-co-owner (would leave the business without a co-owner); `500` internal |
| `inviteSupportOperator` | `{email}` | `201` `{invitation_id, expires_at}` | `400` validation; `401` unauthenticated; `403` forbidden (not `ADMIN`); `409` conflict (already an active `SOPORTE` or unexpired invitation); `500` internal |
| `revokeSupportOperator` | none | `204` empty body | `400` validation; `401` unauthenticated; `403` forbidden (not `ADMIN`); `404` not-found (unknown account or not a `SOPORTE` operator); `500` internal |
| `listMemberships` | none | `200` `[{membership_id, account_id, role, branch_id, created_at}]` | `401` unauthenticated; `403` forbidden (no active membership in this business); `404` not-found (unknown business, or a business outside scope — indistinguishable); `500` internal |
| `inviteMember` | `{email, intended_role: OWNER \| EMPLOYEE, branch_id?}` | `201` `{invitation_id, expires_at}` | `400` validation (owning shape: `OWNER` takes no `branch_id`, `EMPLOYEE` requires one); `401` unauthenticated; `403` forbidden (not the business's co-owner, or an `EMPLOYEE` attempting it); `404` not-found (unknown business or branch outside scope); `409` conflict (`OWNER` invites only `ADMIN`, `EMPLOYEE` invites `OWNER`/`EMPLOYEE`); `500` internal |
| `revokeMembership` | none | `204` empty body | `401` unauthenticated; `403` forbidden (not the business's co-owner; `EMPLOYEE` attempting it); `404` not-found (unknown membership, or one outside scope); `409` last-co-owner; `500` internal |
| `revokeInvitation` | none | `204` empty body | `401` unauthenticated; `403` forbidden (not the business's co-owner); `404` not-found (unknown invitation, or one outside scope); `500` internal |

**Not endpoints, deliberately.** The first `ADMIN` is provisioned by the private command `core/cmd/provision-admin` (contract in [`docs/design-identity-endpoints.md`](design-identity-endpoints.md) §4), never over HTTP: `BE-ACC-04` requires a private server command, and a network-reachable provisioning route would defeat it. `SOPORTE` is provisioned and revoked by `ADMIN` from the platform (`inviteSupportOperator` and `revokeSupportOperator` above, activated through `activateInvitation`), and its phone/DNI procedures are S4's endpoints. Customer registration (`CUSTOMER` / `PLAYER` signup and social sign-in) is deferred to the customer login work; S1 keeps the `WORK`/`CUSTOMER` separation in `schema_identity.account` and the seeded `PLAYER` account to enforce forbidden refusals across the guard matrix.

**Not in this slice.** Business and branch creation, approval and archival are S2 (`BE-ORG-02…07`); S1 owns only the spine memberships attach to, and the roadmap already forbids it from growing into full branch management. Customer profile completion, phone verification and DNI procedures are S4.

**Slice-close condition for the matrix above:** every documented error must be exercised with its observed `type`, and every refusal confirmed not to have mutated data. The `urn:canchalibre:error:method-not-allowed` and `urn:canchalibre:error:not-found` fallbacks are exercised by S1 as part of this, which also closes T0.4's pending live error response.

### 5.3 Task level

Each task starts with one RED test per rule it implements, written before the code, asserting every observable effect of that rule: response status, exact error `type`, and that rejected input left stored data and counters unchanged. Boundaries, error cases and concurrency cases named by the rule are covered. Work that cannot have a meaningful RED — passive documentation, unavailable runner — is stated as such with its reason and gets proportionate functional or structural verification instead.

### 5.4 Empirical contract verification

Documentation and verification are the same loop here: the spec is the claim and the empirical run is the proof.

**Definition of done for an endpoint:** it appears in the generated OpenAPI 3.1 spec, **and** every case in the table above was exercised against a running instance through the API tooling, **and** the observed status codes and problem `type` values match the contract, **and** every refusal was confirmed not to have mutated data. An endpoint in the spec with an uncovered case fails its slice.

**Tooling:** the project-local `api-tools` extension at `.pi/extensions/api-tools` provides `api_status`, `api_auth_status`, `api_login`, `api_rest_request`, `api_swagger` and `api_graphql`. It reads exactly `.pi/api.json` in the project root — no parent lookup and no `.local` override (`src/config.ts:206`) — and returns a disabled configuration when that file is missing, so each checkout copies the committed template once:

```bash
cp .pi/api.json.example .pi/api.json
```

The extension registers **nothing** unless the project opts in through a committed `.pi/extensions.json` containing `{"api-tools": true}` (`index.ts:11-26`). Verified in both directions: with that file present the six tools are available; with it removed every `api_*` tool is absent from the runtime. It is committed because it is an opt-in flag, not machine state.

`.pi/api.json` is gitignored machine state, because `api_login` persists the live `access_token` into the very file it reads (`src/tools.ts:224`). The committed `.pi/api.json.example` carries the shared shape — base URL, framework and response limits — and never a credential or a token. That is also why the extension's own `api_json_tracked` warning must stay silent here: the file it reads may not be tracked, since git applies ignore rules only to untracked files.

**Subagent:** the project subagent `api-contract-verifier` (`.pi/agents/api-contract-verifier.md`) exercises the endpoints — it logs in as each actor, hits every documented case, reconciles the spec against observed behaviour, and returns a case matrix with a verdict per cell plus the list of documented cases it could not cover. It is declared read-only: its tools are `read`, `grep`, `find`, `bash` and the read-side `api_*` tools, and it has no `edit` and no `write`. `bash` is scoped by its instructions to booting the API, building, linting, reading repository state and reading the database.

**Adversarial verification.** Functionality alone is not acceptance. The same subagent must also try to break every endpoint, and the slice closes only at 100%. Beyond the case matrix it submits hostile, malformed, boundary and unexpected data — a number where a string is declared and the reverse, oversized and empty payloads, unicode and control characters, duplicate and conflicting submissions — and it covers input normalization and SQL-injection payloads in identifiers, emails, names and tokens. Two assertions are what make the pass mean something rather than look like it: a rejected input leaves stored data unchanged, and every refusal is confirmed against the database. That is why the subagent has read access to it and why `bash` is scoped to include database reads rather than repository state alone; the §5.2 matrix demands the first assertion and it cannot be proven over HTTP, because a `400` that also wrote a row is indistinguishable from a clean one on the wire. The subagent returns a report listing every case with its observed outcome and a pass or fail per case. **A single finding blocks the slice**: there is no partial pass and no advisory-only result.

**Runner:** `scripts/verify-contract.sh` owns the deterministic half. It builds the API to a temporary binary, boots it, waits for `/health` and tears it down on exit. It builds instead of using `go run` on purpose: `go run` executes the server as a grandchild, so killing the wrapper leaves the real listener holding the port. When the port is already serving, it reuses that instance and never kills a process it did not start.

```bash
scripts/verify-contract.sh --check   # boot, prove readiness, tear down
```

One caveat when booting: the contract declares a fixed server URL (`router.go`, `APIServerURL`), so a run on a non-default port serves `$schema` self-links that still name `localhost:8080`. Verification runs on the default port; a different port makes those links inaccurate.

**Required accounts:** one account per actor — `ADMIN`, `OWNER`, `EMPLOYEE`, `PLAYER` and `SOPORTE` — seeded through a credentials file that is gitignored. Cross-actor and cross-branch cases depend on having more than one business and more than one branch in the seed.

**Seed layout (T1.5).** Three files, one committed shape and two machine-local files, because the extension **requires** the accounts file to be git-ignored: `src/accounts.ts` refuses a `tracked` file, an `unignored_untracked` one, and even one whose status cannot be verified. A seed file that is committed therefore cannot be used by the extension at all.

| File | Committed | Holds |
| --- | --- | --- |
| `.pi/api.json.example` | yes | The shared shape, with `auth.type: login`, `login_path`, `accounts_file: .pi/api.json.local` and the default `account_alias` |
| `.pi/api.json.local.example` | yes | The five actor entries by `role` and `email`, with placeholder passwords |
| `.pi/api.json.local` | no, gitignored | The real development credentials for those five actors |
| `core/dev/seed.topology.json` | yes | The dataset topology: businesses, branches, memberships, platform actors and the refusal matrix. Holds no credential; accounts are matched by email |

`accounts_file` points at `.pi/api.json.local` rather than a new ignored name on purpose: that is the path `.gitignore` already covers, so the block adds no extra ignore rule. Setup per checkout:

```bash
cp .pi/api.json.example .pi/api.json
cp .pi/api.json.local.example .pi/api.json.local   # then set real dev passwords
```

Alias resolution is case-insensitive over `role`, `username` or `email`, so a verifier asks for the actor by name (`api_login` with alias `ADMIN`). Every password parsed from the file is registered for redaction and never appears in tool parameters, results or logs.

The layout was proven end to end before it was documented: with `accounts_file` set to `.pi/api.json.local`, `api_login` resolved the `ADMIN` account and issued the request to `login_path`, failing only because the login endpoint does not exist yet. A rejection at the accounts-file layer would have surfaced a different `configuration` error, so reaching the HTTP layer is the proof that the ignore gate passed and the alias resolved. `core/dev/seed.topology.json` defines two businesses (`B1`, `B2`) and three branches, because section 5.4 asks for more than one business: a cross-branch refusal inside `B1` must be distinguishable from a cross-business refusal.

**Prerequisites for this verification:** block T0 is closed (spec generated and readable by the extension), a runnable local instance exists, and the seed data covers all five actors plus two branches. Until then, features may be implemented but no slice can close.

## 6. Requirement coverage

Every `BE-*` group is assigned. Nothing is left dangling.

| Requirement group | Slice |
| --- | --- |
| `BE-ACC-01…07` | S1 |
| `BE-ACC-08` | S4 |
| `BE-ACC-09` | S6 |
| `BE-ID-01…08` | S4 |
| `BE-ORG-01` | S1 (minimum spine) and S2 (rest) |
| `BE-ORG-02…07` | S2 |
| `BE-OFF-01…09` | S3 |
| `BE-RES-01…12` | S6 |
| `BE-PAY-01…03` | S7 |
| `BE-SUS-01…03` | S8 |
| `BE-DIS-01/02/04` | S5 |
| `BE-DIS-03/05` | S2 |
| `BE-NOT-01/02/04/05` | S6 |
| `BE-NOT-03` | S4 |
| `BE-NOT-06/07` | Operation prerequisites (section 7) |
| `BE-ENG-01` | S1 |
| `BE-ENG-02` | S3 and S6 |
| `BE-ENG-03` | S6, S7 and S8 |
| `BE-ENG-04` | S4 and S6 |
| `BE-ENG-05` | S1 and S4 |
| `BE-ENG-06` | T0, then a standing obligation of every slice |
| `BE-ENG-07` | S1 (layout) and S2 |
| `BE-ENG-08` | Infrastructure block, measured against the real VPS |

## 7. Operation prerequisites

These are not resolved by code and each one blocks real operation, not development.

- Verified email domain and account for work invitations and recovery (`BE-ACC-04` real delivery).
- Legal review of the agreed DNI support and retention policy, including the 30-day post-closure deletion.
- Google Maps credentials plus evidence of coverage, usage terms, quota and cost controls within the agreed additional USD 50/month (`BE-NOT-07`).
- Dedicated WhatsApp number and Evolution/Baileys session, managed only by platform operators.
- Measured container memory for Go, PostgreSQL, Next.js standalone, Evolution and proxy on the 2 vCPU / 8 GB VPS before enabling co-hosting (`BE-ENG-08`).
- A defined authorized support process and the person or group that embodies `SOPORTE`.

## 8. Open items

- **T0.4 (live error response):** the documented 500 is never exercised at runtime because `/health` and `/api/v1/ping` have no failure path. Closed as a design in block T2: the unrouted-path and wrong-method fallbacks give the harness two live error responses to observe (`404` and `405`, both problem-shaped — see [`docs/design-identity-endpoints.md`](design-identity-endpoints.md) §1.4), and S1 implements them.
- **Unregistered-route 404 shape (found by the T1.4 self-test):** a request to a path with no registered operation returned Chi's fallback `404` as `text/plain`, not RFC 7807. The design is frozen in T2 (one `NotFound` and one `MethodNotAllowed` handler rendering a problem detail, verified/observed) and S1 implements it, together with the project's own error model. That same task re-enables the `oas3-missing-example` rule the contract's lint gate currently silences.
- **Subagent definition:** closed in block T1 (T1.4). The definition is `.pi/agents/api-contract-verifier.md`, read-only by declared tools (`read`, `grep`, `find`, `bash`, `api_status`, `api_auth_status`, `api_login`, `api_swagger`, `api_rest_request`), with the deterministic boot half in `scripts/verify-contract.sh`. The model and effort settings still follow the active subagent profile rather than being pinned per agent.
- **Seed credentials:** closed in block T1 (T1.5). The layout is the committed `.pi/api.json.example` plus `.pi/api.json.local.example`, the gitignored `.pi/api.json.local` holding the five actor credentials, and the credential-free `core/dev/seed.topology.json` defining the dataset and refusal matrix. See section 5.4.
- **Committed spec:** closed in block T1 (T1.6). `openapi.json` is **not** committed; it is generated and linted during checks. It is deterministic (the same `sha256` from three consecutive reads) but host-dependent, since it declares `servers: http://localhost:8080` and three localhost `$schema` URLs, so committing it would bake a development URL into the repository. The skill already forbids hand-authoring it, and the runner of section 5.4 now generates and lints the live document on every run, which is the stronger guarantee. If a future slice wants a reviewable diff of the contract, the honest artifact is a spec regenerated from a fixed commit, not a copy carrying `localhost`.
- **Remaining technical design from `docs/prd-backend.md` §7:** concrete DDL and exclusion predicates for offers and reservations; batch ceilings and pagination shapes; Evolution REST/webhook contract and durable retries; HMAC secret rotation and stored suffix length; public photo storage. The identity items (JWT claim layout and branch-context transport, identity DDL and spine) are closed in T1; the identity endpoint paths are closed in T2.
