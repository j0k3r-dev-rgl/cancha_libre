---
name: server-infrastructure
description: "Trigger: server, vps, infrastructure, deploy, sizing, hardware constraints, resource limits. Enforce deployment boundaries and runtime budgets for the 2 vCPU, 8 GB RAM, 100 GB NVMe server."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when designing services, configuring containers, setting deployment parameters, sizing databases, or enforcing runtime budgets for the target production server.

Target Hardware Baseline:
- Compute: 2 vCPU (KVM2).
- Memory: 8 GB RAM.
- Storage: 100 GB NVMe m.2.

## Hard Rules

- **Zero On-Server Builds**:
  - Compilers, asset bundlers, and heavy multi-stage Docker builds are strictly forbidden on the VPS.
  - Builds must execute in CI/CD pipelines; only lean pre-built binaries or standalone runtime images deploy to the server.
- **Runtime Limits & Database Tuning Parameters**:
  - `GOMEMLIMIT=150MiB` is a soft Go GC pacing target, not a hard OS memory cap. Container definitions must set distinct hard container limits (`mem_limit`). Set `GOMAXPROCS=2`.
  - PostgreSQL parameters: `shared_buffers = 1GB`. Note that `effective_cache_size` (e.g. 4GB) is an advisory planner estimate of OS cache, not reserved physical RAM; `work_mem` (e.g. 32MB) multiplies by active query sort/hash operations and concurrency, not a static global reservation.
  - Database connection ceiling: Application connection pools must stay at 15–20 active connections max; never exceed `max_connections = 50` on PostgreSQL.
  - Reverse Proxy (Caddy/Nginx): Lean reverse proxy with ~60 MB RAM budget.
  - Log rotation: Automated log rotation (`max-size: 50m`, `max-file: 3`) to protect the 100 GB NVMe from disk saturation.
- **Next.js Standalone Runtime Integration**:
  - Reconcile with `frontend-nextjs-patterns`: Next.js runs as a lean standalone Node.js server (`output: 'standalone'`) to execute Server Actions and server-side bearer forwarding. It is not a zero-Node static SPA.
- **Provisional Evolution API / Baileys Co-Hosting**:
  - WhatsApp integration via Evolution API (Baileys / WhatsApp Web engine) is provisionally hosted on the existing server, subject to measured capacity and container isolation.
  - Baileys connects via WebSocket; do not make unverified Chromium claims, invent unmeasured memory budgets, or assume zero-cost hosting. Session disconnection and number restrictions are recognized operational risks.
- **Budget Recalculation & Monolith Deployment**:
  - Combined memory allocation across PostgreSQL, Go backend, Next.js standalone, Evolution/Baileys, and proxy must be recalculated and validated through load testing before deployment, rather than declaring all 8 GB pre-allocated. Leave sufficient unallocated memory for Linux kernel page cache over the NVMe.
  - Single-node Docker Compose or native systemd services only. No Kubernetes, service meshes, or distributed infrastructure.
  - Modular bounded contexts (`internal/modules/{context}`) provide structural boundaries for future service extraction, but no microservices are deployed now.

## Decision Gates

| Scenario | Allowed Action | Forbidden Action |
| --- | --- | --- |
| Building application artifacts | Build in external CI/CD pipelines | Running `go build`, `pnpm build`, or compilers directly on the VPS |
| Setting Go memory limits | Configure soft `GOMEMLIMIT` in Go runtime and distinct hard `mem_limit` in Compose | Confusing Go GC soft target with OS hard memory caps |
| PostgreSQL memory planning | Treat `effective_cache_size` as planner estimate; monitor `work_mem` concurrency | Assuming `effective_cache_size` is reserved RAM or ignoring `work_mem` multipliers |
| Frontend runtime hosting | Run lean Next.js standalone Node.js container (`output: 'standalone'`) | Assuming static-only SPA without Node runtime when Server Actions are required |
| WhatsApp integration | Host Evolution/Baileys provisionally subject to measured capacity | Assuming zero-cost operation, inventing unmeasured budgets, or adding extra VPS |
| Deployment architecture | Single-node Docker Compose / systemd with bounded contexts in monolith | Deploying distributed microservices, Kubernetes, or service meshes |
| Database connections | Limit pool to 15–20 active connections (`max_connections = 50`) | Allowing unbounded connection pools or connection leaks |

## Execution Steps

1. Verify target service definitions against the 2 vCPU and 8 GB RAM baseline before provisioning.
2. In Docker Compose, configure explicit hard container limits (`mem_limit` and `cpus`) for Go backend, Next.js standalone server, PostgreSQL, Evolution API, and reverse proxy.
3. Configure Go runtime environment variables (`GOMAXPROCS=2`, `GOMEMLIMIT=150MiB`) recognizing `GOMEMLIMIT` as a soft GC pacing target.
4. Configure PostgreSQL parameters: set `shared_buffers = 1GB`, `max_connections = 50`, connection pool ceiling 15–20, and account for `work_mem` concurrency and planner-only `effective_cache_size`.
5. Provision Next.js standalone container (`output: 'standalone'`) and Evolution/Baileys container with process isolation; measure resource consumption under test workloads.
6. Recalculate combined memory allocation to preserve sufficient unallocated headroom for Linux kernel I/O cache over the 100 GB NVMe.
7. Setup automated Docker/systemd log rotation (`max-size: 50m`, `max-file: 3`).

## Output Contract

Return:
- Confirmed resource allocation per container/service respecting measured 8 GB RAM budget.
- Distinction between Go soft `GOMEMLIMIT` and OS/Compose hard container limits verified.
- PostgreSQL parameters verified (`effective_cache_size` recognized as planner estimate; `work_mem` concurrency accounted for; connection pool <= 20).
- Next.js standalone runtime and provisional Evolution/Baileys co-hosting reconciled without unmeasured budget inventions.
- Verification that deployment uses pre-built artifacts in single-node Compose without microservices.

## References

- Target Host: KVM2 (2 vCPU, 8 GB RAM, 100 GB NVMe m.2).
- Backend Go Patterns: `.agents/skills/backend-go-patterns/SKILL.md`.
- Next.js Patterns: `.agents/skills/frontend-nextjs-patterns/SKILL.md`.
- Monolith Bounded Contexts: `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
