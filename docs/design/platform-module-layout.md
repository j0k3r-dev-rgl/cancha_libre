# Platform module layout

Extracted verbatim from [`docs/design-identity-endpoints.md`](identity-endpoints.md) §2. The section number is preserved, so every existing citation still resolves; only the file that owns the content changed.

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
