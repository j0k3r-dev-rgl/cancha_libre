# Endpoint acceptance criteria

Extracted verbatim from [`docs/roadmap-backend.md`](roadmap-backend.md) section 5. Section numbers and headings are preserved, so every existing citation and anchor still resolves; only the file that owns the content changed.

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

Frozen before implementation, as this section requires. Every `type` below comes from the catalog in [`docs/design/platform-error-model.md`](design/platform-error-model.md) §1.1; no endpoint may document a `type` outside it, and `about:blank` is never documented. Each documented error is registered as an explicit huma `Responses` entry with a per-error example, because `Errors []int` alone does not publish the `type` (verified, same document §1.3).

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
| `logout` | empty body | `204` empty body | `401` unauthenticated; `403` forbidden (account not `ACTIVE`, or the actor's role or membership no longer authorized); `500` internal |
| `getOwnAccount` | none | `200` `{account_id, email, kind, status, display_name}` | `401` unauthenticated; `403` forbidden (account not `ACTIVE`, or the actor's role or membership no longer authorized); `500` internal |
| `listOwnContexts` | none | `200` `[{business_id, business_name, branch_id, branch_name, role}]` | `401` unauthenticated; `403` forbidden (account not `ACTIVE`, or the actor's role or membership no longer authorized); `500` internal |
| `selectContext` | `{business_id, branch_id}` | `200` `{access_token, expires_in}` | `400` validation; `401` unauthenticated; `403` context-not-authorized (no active membership, or disabled business/branch — one type, so scope is not probed); `500` internal |
| `assignCoOwner` | `{account_id}` or `{email, intended_role: OWNER}` | `201` `{membership_id, role, business_id}` | `400` validation; `401` unauthenticated; `403` forbidden (not `ADMIN`); `404` not-found (unknown business); `409` conflict (already an active co-owner); `500` internal |
| `revokeCoOwner` | none | `204` empty body | `401` unauthenticated; `403` forbidden (not `ADMIN`); `404` not-found (unknown business, or no such active co-owner); `409` last-co-owner (would leave the business without a co-owner); `500` internal |
| `inviteSupportOperator` | `{email}` | `201` `{invitation_id, expires_at}` | `400` validation; `401` unauthenticated; `403` forbidden (not `ADMIN`); `409` conflict (already an active `SOPORTE` or unexpired invitation); `500` internal |
| `revokeSupportOperator` | none | `204` empty body | `400` validation; `401` unauthenticated; `403` forbidden (not `ADMIN`); `404` not-found (unknown account or not a `SOPORTE` operator); `500` internal |
| `listMemberships` | none | `200` `[{membership_id, account_id, role, branch_id, created_at}]` | `401` unauthenticated; `403` forbidden (no active membership in this business); `404` not-found (unknown business, or a business outside scope — indistinguishable); `500` internal |
| `inviteMember` | `{email, intended_role: OWNER \| EMPLOYEE, branch_id?}` | `201` `{invitation_id, expires_at}` | `400` validation (owning shape: `OWNER` takes no `branch_id`, `EMPLOYEE` requires one); `401` unauthenticated; `403` forbidden (not the business's co-owner, or an `EMPLOYEE` attempting it); `404` not-found (unknown business or branch outside scope); `409` conflict (`OWNER` invites only `ADMIN`, `EMPLOYEE` invites `OWNER`/`EMPLOYEE`); `500` internal |
| `revokeMembership` | none | `204` empty body | `401` unauthenticated; `403` forbidden (not the business's co-owner; `EMPLOYEE` attempting it); `404` not-found (unknown membership, or one outside scope); `409` last-co-owner; `500` internal |
| `revokeInvitation` | none | `204` empty body | `401` unauthenticated; `403` forbidden (not the business's co-owner); `404` not-found (unknown invitation, or one outside scope); `500` internal |

**Not endpoints, deliberately.** The first `ADMIN` is provisioned by the private command `core/cmd/provision-admin` (contract in [`docs/design/provision-admin.md`](design/provision-admin.md) §4), never over HTTP: `BE-ACC-04` requires a private server command, and a network-reachable provisioning route would defeat it. `SOPORTE` is provisioned and revoked by `ADMIN` from the platform (`inviteSupportOperator` and `revokeSupportOperator` above, activated through `activateInvitation`), and its phone/DNI procedures are S4's endpoints. Customer registration (`CUSTOMER` / `PLAYER` signup and social sign-in) is deferred to the customer login work; S1 keeps the `WORK`/`CUSTOMER` separation in `schema_identity.account` and the seeded `PLAYER` account to enforce forbidden refusals across the guard matrix.

**Not in this slice.** Business and branch creation, approval and archival are S2 (`BE-ORG-02…07`); S1 owns only the spine memberships attach to, and the roadmap already forbids it from growing into full branch management. Customer profile completion, phone verification and DNI procedures are S4.

**Slice-close condition for the matrix above:** every documented error must be exercised with its observed `type`, and every refusal confirmed not to have mutated data. The `urn:canchalibre:error:method-not-allowed` and `urn:canchalibre:error:not-found` fallbacks are exercised by S1 as part of this, which also closes T0.4's pending live error response.

#### Inventory — S2 (Platform Administration & Core Organization)

Frozen before implementation, as this section requires. Per architecture decision, Slice S2 is bounded to **Platform Administration (`ADMIN` and `SOPORTE`)**, delivering business onboarding and governance, branch approval inbox and supervision, and global user management. Owner-level branch asset management (operating hours, photos, courts) and customer catalog are deferred to subsequent slices.

Full design, rationale, and review checklist are in [`docs/design/organization-endpoints.md`](design/organization-endpoints.md). Every `type` below comes from the catalog in [`docs/design/platform-error-model.md`](design/platform-error-model.md) §1.1; no endpoint may document a `type` outside it, and `about:blank` is never documented. Each documented error is registered as an explicit huma `Responses` entry with a per-error example.

All routes are served under the `/api/v1` prefix. Every path parameter is a UUID.

| Operation ID | Method and path | Actor | Requirements | AC | Idempotency |
| --- | --- | --- | --- | --- | --- |
| `createBusiness` | `POST /admin/businesses` | ADMIN | BE-ORG-01, BE-ACC-05 | AC-26, AC-43 | non-idempotent: creates a new business entity |
| `listBusinesses` | `GET /admin/businesses` | ADMIN | BE-ORG-01 | AC-26 | idempotent |
| `getBusiness` | `GET /admin/businesses/{businessId}` | ADMIN | BE-ORG-01, BE-ACC-05 | AC-26 | idempotent |
| `updateBusiness` | `PATCH /admin/businesses/{businessId}` | ADMIN | BE-ORG-01 | AC-26 | idempotent |
| `suspendBusiness` | `POST /admin/businesses/{businessId}/suspend` | ADMIN | BE-ORG-01, BE-ACC-07 | AC-19 | idempotent: repeated suspension preserves state |
| `reactivateBusiness` | `POST /admin/businesses/{businessId}/reactivate` | ADMIN | BE-ORG-01, BE-ACC-07 | AC-19 | idempotent: repeated reactivation preserves state |
| `archiveBusiness` | `DELETE /admin/businesses/{businessId}` | ADMIN | BE-ORG-06 | AC-40 | idempotent |
| `unarchiveBusiness` | `POST /admin/businesses/{businessId}/unarchive` | ADMIN | BE-ORG-07 | AC-40 | idempotent |
| `listBranchRequests` | `GET /admin/branch-requests` | ADMIN, SOPORTE | BE-ORG-02 | AC-25, AC-41 | idempotent |
| `getBranchRequest` | `GET /admin/branch-requests/{requestId}` | ADMIN, SOPORTE | BE-ORG-02 | AC-25, AC-41 | idempotent |
| `approveBranchRequest` | `POST /admin/branch-requests/{requestId}/approve` | ADMIN, SOPORTE | BE-ORG-02 | AC-25 | non-idempotent: advances review state |
| `rejectBranchRequest` | `POST /admin/branch-requests/{requestId}/reject` | ADMIN, SOPORTE | BE-ORG-02 | AC-25 | non-idempotent: records rejection with reason |
| `listAdminBranches` | `GET /admin/branches` | ADMIN, SOPORTE | BE-ORG-01, BE-ORG-02 | AC-26 | idempotent |
| `suspendBranch` | `POST /admin/branches/{branchId}/suspend` | ADMIN | BE-ORG-01, BE-ACC-07 | AC-19 | idempotent |
| `reactivateBranch` | `POST /admin/branches/{branchId}/reactivate` | ADMIN | BE-ORG-01, BE-ACC-07 | AC-19 | idempotent |
| `listUsers` | `GET /admin/users` | ADMIN | BE-ACC-07 | AC-42 | idempotent |
| `getUser` | `GET /admin/users/{userId}` | ADMIN | BE-ACC-07 | AC-42 | idempotent |
| `suspendUser` | `POST /admin/users/{userId}/suspend` | ADMIN | BE-ACC-07 | AC-19 | idempotent |
| `reactivateUser` | `POST /admin/users/{userId}/reactivate` | ADMIN | BE-ACC-07 | AC-19 | idempotent |

| Operation ID | Request shape | Success response | Documented errors (`status` → `type`) |
| --- | --- | --- | --- |
| `createBusiness` | `{name, legal_name?, tax_id?, owner_emails?}` | `201` `{id, name, legal_name, tax_id, enabled, created_at, owners: [...]}` | `400` validation; `401` unauthenticated; `403` forbidden; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `listBusinesses` | query: `status?`, `search?`, `page?`, `limit?` | `200` `{items: [...], total, page, limit}` | `400` validation; `401` unauthenticated; `403` forbidden; `422` validation; `500` internal |
| `getBusiness` | none | `200` `{id, name, legal_name, tax_id, enabled, suspended_at, archived_at, created_at, owners: [...], branches: [...]}` | `401` unauthenticated; `403` forbidden; `404` not-found; `500` internal |
| `updateBusiness` | `{name?, legal_name?, tax_id?}` | `200` `{id, name, legal_name, tax_id, enabled, suspended_at, archived_at}` | `400` validation; `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `suspendBusiness` | `{reason}` | `200` `{id, suspended_at, enabled}` | `400` validation; `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `reactivateBusiness` | empty body | `200` `{id, suspended_at: null, enabled}` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |
| `archiveBusiness` | none | `204` empty body | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |
| `unarchiveBusiness` | empty body | `200` `{id, archived_at: null}` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |
| `listBranchRequests` | query: `status?`, `page?`, `limit?` | `200` `{items: [...], total, page, limit}` | `400` validation; `401` unauthenticated; `403` forbidden; `422` validation; `500` internal |
| `getBranchRequest` | none | `200` `{id, branch_id, business_id, business_name, branch: {...}, status, submitted_by, submitted_at, reviewed_by, reviewed_at, rejection_reason, history: [...]}` | `401` unauthenticated; `403` forbidden; `404` not-found; `500` internal |
| `approveBranchRequest` | empty body | `200` `{id, branch_id, status: "APPROVED", reviewed_by, reviewed_at}` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |
| `rejectBranchRequest` | `{rejection_reason}` | `200` `{id, branch_id, status: "REJECTED", rejection_reason, reviewed_by, reviewed_at}` | `400` validation; `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `listAdminBranches` | query: `business_id?`, `approval_status?`, `status?`, `city?`, `page?`, `limit?` | `200` `{items: [...], total, page, limit}` | `400` validation; `401` unauthenticated; `403` forbidden; `422` validation; `500` internal |
| `suspendBranch` | `{reason}` | `200` `{id, suspended_at}` | `400` validation; `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `reactivateBranch` | empty body | `200` `{id, suspended_at: null}` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |
| `listUsers` | query: `kind?`, `role?`, `status?`, `search?`, `page?`, `limit?` | `200` `{items: [...], total, page, limit}` | `400` validation; `401` unauthenticated; `403` forbidden; `422` validation; `500` internal |
| `getUser` | none | `200` `{id, email, display_name, kind, platform_role, status, created_at, memberships: [...]}` | `401` unauthenticated; `403` forbidden; `404` not-found; `500` internal |
| `suspendUser` | `{reason}` | `200` `{id, status: "SUSPENDED"}` | `400` validation; `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `415` unsupported-media-type; `422` validation; `500` internal |
| `reactivateUser` | empty body | `200` `{id, status: "ACTIVE"}` | `401` unauthenticated; `403` forbidden; `404` not-found; `409` conflict; `500` internal |

S2 pagination is one-based: `page` defaults to `1`, `limit` defaults to `20` and accepts `1..100`. Invalid numeric ranges or an overflowing calculated offset return `400 validation`; non-numeric query values return `422 validation`. Body-schema failures (missing required properties or wrong types) also return `422 validation`; present-but-invalid domain values return `400 validation`. Full semantics are in [Organization Endpoints](design/organization-endpoints.md#4-verification-and-contract-obligations).

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
