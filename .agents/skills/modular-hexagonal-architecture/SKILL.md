---
name: modular-hexagonal-architecture
description: "Trigger: architecture, modular monolith, bounded context, hexagonal, ports and adapters, microservices migration, integrations, inter-module communication. Enforce bounded-context modular monolith boundaries with per-context hexagonal architecture and decoupled integrations in Go."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when designing, scaffolding, refactoring, or reviewing backend bounded contexts, module boundaries, ports, adapters, or third-party integrations in Go for CanchaLibre.

## Hard Rules

- **Bounded Context Module Isolation**:
  - Each module in `internal/modules/{context}` represents a **bounded context**, not an individual entity or single database table. A bounded context may contain multiple related aggregates and entities that share cohesive business invariants.
  - Do not assume a pre-fixed context map; design context boundaries around cohesive domain rules and transaction ownership.
  - Cross-context database queries (joins or foreign keys across context schemas) and direct imports of another context's internal domain, use cases, repositories, or SQL adapters are strictly forbidden.
- **Per-Context Hexagonal Structure**:
  - `internal/modules/{context}/core/domain`: Pure entities, aggregates, value objects, domain events, and sentinel errors. Zero framework or infrastructure imports.
  - `internal/modules/{context}/core/ports`:
    - `inbound.go`: Public interfaces and portable contract DTOs exposed to driving actors (HTTP, webhooks, CLI, or other bounded contexts).
    - `outbound.go`: Interfaces for persistence repositories, external services (Evolution API / Baileys, Google Maps), cross-context dependencies, and event dispatchers.
  - `internal/modules/{context}/core/usecase`: Business workflows orchestrating domain models and outbound ports.
  - `internal/modules/{context}/adapters/inbound`: HTTP handlers (Chi router `github.com/go-chi/chi/v5`), webhook receivers, CLI commands.
  - `internal/modules/{context}/adapters/outbound`: PostgreSQL repositories (`pgx/v5`), external API clients (Evolution API, Google Maps), cross-context client adapters, event publishers.
  - The existing flat skeleton in `core/internal/{domain,usecase,adapter}` is legacy bootstrap scaffolding; do not use it as a template for new bounded contexts.
- **Inter-Context Integration Contract**:
  - **Synchronous**: Consumer context A defines its own outbound port in `A/core/ports/outbound.go`. An outbound adapter in `A/adapters/outbound` calls provider context B's public inbound port (`B/core/ports/inbound.go`), Wired at the composition root (`cmd/api`) with zero circular dependencies.
  - **Asynchronous**: Domain events are published via an outbound port. In monolithic runtime, use bounded in-process Go channels and worker pools. In-memory channels are volatile and not durable across restarts; operations requiring guaranteed delivery or timed retries (e.g., WhatsApp notice retries or expiration) must persist state in PostgreSQL rather than relying solely on volatile channels.
  - **Future Service Extraction**: Bounded-context isolation and contract DTOs prepare modules for potential extraction into independent services, but extraction is a design goal, not a zero-effort guarantee. Extracting a context later requires explicit design for network transport, distributed consistency/sagas, and operational isolation.
- **Third-Party Integration Contract (Evolution API, Google Maps)**:
  - Outbound integrations implement explicit outbound ports. Core packages never import third-party SDKs or HTTP client libraries directly.
  - Inbound webhooks act as driving adapters that validate credentials/signatures before delegating to inbound ports.
  - Do not introduce payment gateway or PSP adapters; CanchaLibre operates strictly in ARS with manual staff payment recording.

## Decision Gates

| Need | Required Pattern | Forbidden Pattern |
| --- | --- | --- |
| Defining module granularity | Group cohesive aggregates/entities into a bounded context (`internal/modules/{context}`) | Creating a separate module per table/entity or using a flat global `internal/domain` |
| Context A needs capability from Context B | Define outbound port in A; adapter calls B's public inbound port wired at composition root | Direct cross-schema SQL join, importing B's repository, or importing B's internal domain types |
| Calling external API (Evolution, Google Maps) | Define outbound port in `core/ports` and implement in `adapters/outbound` | Calling external HTTP endpoints inside `core/usecase` or `core/domain` |
| Monolith events & background retries | Bounded in-process channels for immediate dispatch + DB persistence for durable retries | Relying solely on volatile in-memory channels for state that must survive process restarts |
| Future microservice extraction | Preserve strict context contracts and schema isolation; design distributed consistency when extracting | Assuming network extraction requires zero consistency, transaction, or operational changes |

## Execution Steps

1. Identify the bounded context and its cohesive aggregates/invariants before creating packages.
2. Scaffold `internal/modules/{context}/core/{domain,ports,usecase}` and `adapters/{inbound,outbound}`.
3. Define pure domain entities, aggregates, and sentinel errors in `core/domain`.
4. Declare public inbound contracts in `core/ports/inbound.go` and secondary dependencies in `core/ports/outbound.go` using portable contract types.
5. Implement business workflows in `core/usecase` depending only on `core/domain` and `core/ports`.
6. Implement persistence (`pgx/v5`), external integrations, cross-context adapters, and HTTP handlers (`go-chi/chi/v5`) under `adapters/`.
7. Wire dependencies and cross-context adapters in the composition root (`cmd/api`) without circular imports.

## Output Contract

Return:
- Bounded context directory structure verified under `internal/modules/{context}`.
- Inbound and outbound ports separated with portable contract types and zero cross-context internal imports.
- Verification that no cross-schema database joins, foreign keys, or unapproved PSP/Cloud API adapters exist.
- Durability boundary documented when background events or retries are involved.

## References

- Target: Go 1.24+, Modular Monolith by Bounded Context, Chi router (`github.com/go-chi/chi/v5`).
- Backend Go Patterns: `.agents/skills/backend-go-patterns/SKILL.md`.
- Database Isolation: `.agents/skills/database-postgres-patterns/SKILL.md`.
- Server Limits: `.agents/skills/server-infrastructure/SKILL.md`.
