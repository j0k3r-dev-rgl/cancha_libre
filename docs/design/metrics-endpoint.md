# Private metrics endpoint: surface, instruments and boundaries

Status: implemented. Owner of the work unit: `odd/tasks/metrics-endpoint.md`.
Related: [`docs/design/platform-error-model.md`](platform-error-model.md) (the RFC 7807 catalog this endpoint reuses), [`docs/roadmap-backend.md`](../roadmap-backend.md) §7 and §8.

## Decisions

- **`/metrics` is a platform route, not an operation.** It is mounted on the raw Chi router, so it never enters the OpenAPI 3.1 document clients read. There is no `operationId`, no `Responses` entry and no schema for it.
- **The endpoint is opt-in and the switch is the token.** With `METRICS_TOKEN` unset the route is never mounted, the recording middleware is never installed and a scrape costs nothing. With a token, the endpoint exists and demands it.
- **`METRICS_TOKEN` shorter than 32 bytes fails startup**, like `JWT_SECRET` and `LOG_LEVEL`: a guessable credential in front of operational detail is worse than no endpoint.
- **The client library is `github.com/VictoriaMetrics/metrics`, pinned at one exact version.** It is chosen for the 2 vCPU / 150 MiB budget; the exposition is Prometheus-compatible, so replacing it later is mechanical.
- **No Prometheus and no Grafana live in this repository.** This document only defines what the API publishes and how it is gated. Where the scraper runs is the operator's decision.

## 1. The surface

| Aspect | Behaviour |
| --- | --- |
| Path | `GET /metrics` |
| Mounting | Raw Chi route, added only when the router receives a handler (that is, only when `METRICS_TOKEN` is set). Unmounted, the path is an unrouted `404 urn:canchalibre:error:not-found`. |
| Authentication | `Authorization: Bearer <METRICS_TOKEN>`. The scheme is matched case-insensitively (RFC 6750) and the token with `crypto/subtle.ConstantTimeCompare`. |
| Refusal | `401 urn:canchalibre:error:unauthenticated` as an `application/problem+json` body from the same catalog every API operation uses. No metrics are rendered. |
| Success | `200` with `Content-Type: text/plain; version=0.0.4; charset=utf-8` and the Prometheus text exposition. |
| Wrong method | The router's `405 urn:canchalibre:error:method-not-allowed`. |
| Contract | Never appears in `openapi.json`; `TestOpenAPISpecNeverPublishesThePrivateMetricsPath` keeps it out. |

An unconfigured token refuses every request, so the handler fails closed even if a future caller mounts it without a credential.

## 2. Instruments and cardinality

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `canchalibre_http_requests_total` | counter | `method`, `route`, `status` | Requests served, by the route the router matched. |
| `canchalibre_http_request_duration_seconds` | histogram (`le` buckets) | `method`, `route` | Request duration in seconds. |
| `canchalibre_db_pool_conns` | gauge | `state` = `total`, `idle`, `acquired` | PostgreSQL pool connections, read at scrape time. |
| `canchalibre_db_pool_max_conns` | gauge | — | Configured pool ceiling. |
| `go_*`, `process_*` | library-provided | — | Go runtime and process numbers: goroutines, GC, CPU seconds, resident memory, file descriptors. |

Three rules keep the series bounded, and each one has a test:

1. **`route` is the router's template**, never the requested path. `DELETE /api/v1/admin/support-operators/9f5d…` is reported as `/api/v1/admin/support-operators/{accountId}`, so a million account ids produce one series.
2. **`method` is a closed set.** Anything outside `GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS` is reported as `other`.
3. **A path the router never attributed is reported as `unmatched`**, so unknown-path scanning cannot create a series per guessed path.

No identifier, path segment, email, actor name or query string can reach a label value through this surface. The scrape path itself is excluded from the request counter: a 15-second scrape would otherwise dominate a panel meant to describe product traffic.

## 3. Configuration

| Variable | Required | Effect |
| --- | --- | --- |
| `METRICS_TOKEN` | no | Unset or empty: the endpoint does not exist. Set: it must be at least 32 bytes and is the only accepted bearer token. |
| `LOG_LEVEL` | no | `debug`, `info`, `warn` or `error`; default `info`. Unrelated to metrics, documented here because it lives in the same template. |

The startup log line carries `metrics_enabled` as a boolean, never the token.

## 4. What is deliberately excluded

- **No separate listener or second port.** A Prometheus on another host must scrape this VPS, so the endpoint shares the API port and the token is the gate. Keeping the path out of the public hostname is a reverse-proxy rule, recorded in [`docs/roadmap-backend.md`](../roadmap-backend.md) §7.
- **No per-user or per-tenant labels.** They would be unbounded and they would leak identity into a monitoring system.
- **No Prometheus or Grafana service, compose file or scrape configuration here.**
- **No auth for the metrics endpoint beyond the bearer token**: no JWT, no role, no session. The token is a machine credential, not an actor.

## 5. Verification performed

- `cd core && mise exec -- go test -race -count=1 ./...` — the recording middleware and the registry under the race detector.
- `cd core && mise exec -- go vet ./...`.
- `bash scripts/verify-contract.sh --check` — the served OpenAPI 3.1 document still lints at 100/100 with the private path absent.
- `TestMetricsEndpointServesTheRealRegistry` (`core/cmd/api`) boots the real composition root over a real database and proves the whole path: `401` without the token, `200` with it, a real `/health` request counted under its template, the pool gauges present from the real pool, and no `route="/metrics"` series.
- The cardinality rule was **falsified once**: with the label taken from `r.URL.Path` and the method passed through raw, both middleware tests fail (`expected the route template, got "/api/v1/things/9f5d…"` and `unrouted request recorded as {Method:GET Route:/api/v1/does-not-exist Status:404}`). The implementation was restored byte-identically.

## 6. Deliberate boundaries and open items

| Item | Why it is not closed here | Closed by |
| --- | --- | --- |
| Reverse-proxy rule that keeps `/metrics` off the public hostname | Deployment, not code. The endpoint authenticates, but the operator must not publish it. | Operator (roadmap §7) |
| Where Prometheus and Grafana run | The user's decision, explicitly deferred. | User |
| Alerting rules and dashboards | They depend on where the scraper runs and what the operator wants to page on. | Future task |
| Retention of the metrics history | A property of the scraper, not of this API. | Operator |
| `agentEscalation`-style delivery/review of this endpoint | Commit, push, PR and merge remain the user's decisions. | User |

## 7. Review checklist

- [ ] `GET /metrics` without the bearer token answers `401 urn:canchalibre:error:unauthenticated` and renders no metrics.
- [ ] With `METRICS_TOKEN` unset, `/metrics` is an unrouted `404`, not a weaker endpoint.
- [ ] A `METRICS_TOKEN` shorter than 32 bytes stops startup before the server listens.
- [ ] `/metrics` is absent from the served OpenAPI document.
- [ ] No metric name can carry a raw path, an identifier or an email.
- [ ] The scrape path is not counted in its own request series.
- [ ] The pool gauges report the pool at scrape time, not the value captured at wiring time.
- [ ] The startup log line reports `metrics_enabled` without the token.

## Next step

The endpoint is implemented and gated. What remains is the operator's: put a `METRICS_TOKEN` in the environment, keep the path off the public hostname, and decide where Prometheus and Grafana run.
