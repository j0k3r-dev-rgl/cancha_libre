# Platform error model

Extracted verbatim from [`docs/design-identity-endpoints.md`](identity-endpoints.md) §1, together with its verification §5. Section numbers are preserved, so every existing citation still resolves; only the file that owns the content changed.

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
