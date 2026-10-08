---
name: server-infrastructure
description: "Trigger: server, vps, infrastructure, deploy, sizing, hardware constraints, resource limits. Enforce deployment boundaries and runtime budgets for the 2 vCPU, 8 GB RAM, 100 GB NVMe server."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when designing services, setting deployment parameters, configuring databases, or enforcing resource budgets for the target production server.

Target Hardware Baseline:
- Compute: 2 vCPU (KVM2).
- Memory: 8 GB RAM.
- Storage: 100 GB NVMe m.2.

## Hard Rules

- **Zero on-server builds**: Never run compilers, asset bundlers, or heavy multi-stage Docker builds on the VPS. Builds happen in CI/CD; only lean pre-built binaries or runtime images deploy to the server.
- **Strict Memory Budgeting**:
  - PostgreSQL: Max 2.5 GB total RAM allocation (`shared_buffers = 1GB`, `effective_cache_size = 4GB`, `work_mem = 32MB`).
  - Go Backend: Max 150 MB RAM cap (`GOMEMLIMIT=150MiB`, `GOMAXPROCS=2`).
  - Reverse Proxy (Caddy/Nginx): Max 60 MB RAM.
  - Web Frontend: Static SPA served directly via reverse proxy; zero persistent Node.js runtime memory on the VPS.
  - System and Page Cache: Keep at least 3.5 GB free for Linux kernel I/O cache over the NVMe.
- **Connection Ceiling**: Set database connection pools to 15–20 active connections max. Never exceed `max_connections = 50` on PostgreSQL.
- **Orchestration**: Single-node Docker Compose or native systemd services only. No Kubernetes or distributed control planes.

## Decision Gates

| Scenario | Allowed Action | Forbidden Action |
| --- | --- | --- |
| Building application artifacts | Build in GitHub Actions / external CI | Running `go build` or `pnpm build` directly on the VPS |
| High-concurrency spikes | Queue or reject at reverse proxy/app level | Spawning unbounded DB connections or unbuffered worker pools |
| Database caching | Tune PostgreSQL `shared_buffers` and indexes | Adding heavy separate caching servers without memory recalculation |
| Frontend hosting | Pre-render static bundle to Caddy/Nginx | Running heavy SSR Node servers if SPA meets product needs |

## Execution Steps

1. Verify target service configurations against the 8 GB RAM and 2 vCPU caps before provisioning.
2. Ensure Docker Compose definitions set strict `mem_limit` and `cpus` limits for each container.
3. Configure PostgreSQL parameters based on the 100 GB NVMe disk and 2.5 GB memory allowance.
4. Set Go runtime environment variables (`GOMAXPROCS=2`, `GOMEMLIMIT=150MiB`) in the production runner.
5. Setup automated log rotation (`max-size: 50m`, `max-file: 3`) to protect the 100 GB NVMe from disk saturation.

## Output Contract

Return:
- Confirmed resource allocation per container/service.
- Environment variables and connection pool limits applied.
- Verification that deployment relies on pre-built artifacts without in-host compilation.

## References

- Target Host: KVM2 (2 vCPU, 8 GB RAM, 100 GB NVMe m.2).
