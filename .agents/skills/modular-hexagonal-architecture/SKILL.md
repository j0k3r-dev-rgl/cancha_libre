---
name: modular-hexagonal-architecture
description: "Trigger: architecture, modular monolith, hexagonal, ports and adapters, microservices migration, integrations, inter-module communication. Enforce modular monolith boundaries with per-module hexagonal architecture and decoupled integrations in Go."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when designing, scaffolding, refactoring, or reviewing any backend module, boundary, port, adapter, or third-party integration in Go for CanchaLibre.

## Hard Rules

- **Module Boundary Isolation**:
  - Every business area lives in its own root directory (`internal/modules/{module-name}`).
  - Cross-module database queries (joins across module tables) and direct imports of another module's internal adapters or SQL packages are strictly forbidden.
- **Per-Module Hexagonal Structure**:
  - `core/domain`: Pure entities, value objects, domain events, and sentinel errors. Zero framework/infrastructure imports.
  - `core/ports`:
    - `inbound.go`: Public interfaces defining use cases exposed to driving actors (HTTP, webhooks, other modules).
    - `outbound.go`: Interfaces for data stores, external services (Pix, WhatsApp), and event dispatchers.
  - `core/usecase`: Business logic implementations orchestrating domain and outbound ports.
  - `adapters/inbound`: HTTP controllers (Chi router `github.com/go-chi/chi/v5`), webhook receivers (validating signatures/payloads), CLI handlers.
  - `adapters/outbound`: PostgreSQL repositories (`pgx`), third-party clients (WhatsApp Cloud API, Pix PSP), event publishers.
- **Inter-Module Integration Contract**:
  - **Synchronous**: Module A calls Module B strictly through Module B's public inbound port interface. In monolithic mode, wired via Go interfaces in memory. For future microservices, swapped for gRPC/HTTP client adapters with zero Core changes.
  - **Asynchronous**: Domain events published to an Event Bus interface. In monolithic mode, in-memory channels. For microservices, swapped for RabbitMQ, Kafka, or NATS publishers.
- **Third-Party Integration Contract (Pix, WhatsApp, etc.)**:
  - Outbound calls must implement an explicit outbound port. Core code never imports third-party SDKs or HTTP client libraries directly.
  - Inbound webhooks must be treated as driving adapters that validate security/signatures before delegating to inbound ports.

## Decision Gates

| Need | Required Pattern | Forbidden Pattern |
| --- | --- | --- |
| Module A needs data from Module B | Call Module B's public Inbound Port interface | Direct SQL join or importing Module B's repository |
| Calling external API (Pix, WhatsApp) | Define outbound port in `core/ports` and implement in `adapters/outbound/external` | Calling external HTTP endpoints inside usecase or domain |
| Handling external Webhooks | Adapter in `adapters/inbound/webhook` -> validate -> call inbound usecase port | Business logic inside webhook handler |
| Inter-module notifications | Publish domain event to Event Bus interface | Direct circular dependency between modules |

## Execution Steps

1. Scaffold module tree: `core/{domain,ports,usecase}` and `adapters/{inbound,outbound}`.
2. Define domain entities and business errors in `core/domain`.
3. Declare use case interfaces in `core/ports/inbound.go` and secondary dependencies in `core/ports/outbound.go`.
4. Implement business workflows in `core/usecase` depending only on interfaces.
5. Implement adapters for persistence (`pgx`), integrations (Pix, WhatsApp), and delivery (HTTP Chi, webhooks).
6. Wire dependencies in module bootstrap (`wire.go` or module constructor) and register public inbound ports.

## Output Contract

Return:
- Verified per-module hexagonal directory structure.
- Inbound and outbound ports clearly separated.
- Verification that no inter-module database joins or illegal adapter imports exist.

## References

- Target: Go 1.24+, Modular Monolith ready for microservices extraction, Chi router (`github.com/go-chi/chi/v5`).
- Server Limits: `.agents/skills/server-infrastructure/SKILL.md`.
