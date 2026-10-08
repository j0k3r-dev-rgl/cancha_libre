---
name: dependency-selection-audit
description: "Trigger: dependency, install, pnpm add, go get, cve, audit, package selection. Enforce pre-installation security audits, CVE vulnerability checks, documentation review, bloat rejection, and multi-library comparisons."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill BEFORE installing, introducing, or evaluating any new third-party dependency in the Go backend or Next.js frontend of CanchaLibre.

## Hard Rules

- **Pre-Installation Security Verification**:
  - Never install a package without verifying its security history.
  - For Go: check against the Go vulnerability database (`govulncheck` or `pkg.go.dev` vulnerability notices).
  - For Node/pnpm: verify with `pnpm audit` or the GitHub Advisory / OSV database.
  - Reject any dependency with unpatched critical or high-severity CVEs.
- **Anti-Bloat & Single-Responsibility Principle**:
  - Always prefer the language standard library or native APIs:
    - Go: standard library (`net/http`, `context`, `sync`, `crypto`) or minimal idiomatic packages (Chi router `github.com/go-chi/chi/v5`) over bulky third-party abstraction layers (`gin`, `fiber`, `echo`).
    - Frontend: native Web APIs (`fetch`, `Intl`, `crypto`, `structuredClone`) over monolithic utility libraries like `lodash` or `moment`.
  - Reject packages with excessive transitive dependencies or "kitchen-sink" architectures when only a small subset of functionality is needed.
- **Maintenance Health & Deprecation Gate**:
  - Prohibit installing deprecated, archived, or abandoned libraries.
  - Maintenance requirements:
    - Active releases within the last 12 months.
    - Clear issue triage and active security patching.
    - Explicit support for Go 1.24+ (backend) or React 19 / Next.js App Router (frontend).
- **Mandatory Alternative Comparison**:
  - If a package is non-trivial or introduces significant runtime overhead, evaluate 2 to 3 viable alternatives.
  - Evaluate: bundle size (Bundlephobia / tree-shaking), binary impact, API ergonomics, and community adoption.
- **Permissive Licensing Only**:
  - Allowed licenses: MIT, Apache-2.0, BSD-2/3-Clause, ISC.
  - Prohibited: Viral copyleft licenses (GPL, AGPL) that could compromise proprietary domain code.

## Decision Gates

| Scenario | Required Action | Forbidden Action |
| --- | --- | --- |
| Utility needed (date, array, string) | Use native language APIs or micro-packages | Installing heavy multi-purpose libraries (e.g. `lodash`, `moment`) |
| Package has unpatched CVE | Reject package; choose safe alternative | Installing and ignoring security advisories |
| Package is archived / unmaintained | Select actively maintained modern fork or alternative | Installing stale packages with last commit > 2 years ago |
| Unsure between multiple libraries | Formulate comparison table (size, CVEs, maintenance) | Picking the first search result without benchmarking |
| HTTP router / API framework | Standardize on Chi (`go-chi/chi/v5`) | Adopting heavy frameworks (`gin`, `echo`, `fiber`) with proprietary contexts or buffer pooling |
| License is AGPL or unclear | Reject or escalate for human review | Adopting viral copyleft packages silently |

## Execution Steps

1. State the concrete functional requirement.
2. Verify if standard library or existing dependencies already fulfill the need.
3. Check candidate library for CVEs via `govulncheck` (Go) or `pnpm audit` / OSV (Node/pnpm).
4. Review documentation for Next.js App Router / Go idiom compatibility.
5. If non-trivial, compare 2–3 alternatives across size, maintenance, and license.
6. Provide recommendation and rationale before proceeding to installation.
7. Delegate exact installation to `.agents/skills/dependency-version-pinning/SKILL.md`.

## Output Contract

Return:
- Security audit result (CVE clearance status).
- Maintenance health check (last release date, repository status).
- Comparison summary if alternatives were evaluated.
- Explicit confirmation that native alternatives were considered and rejected with rationale.

## References

- Go Vulnerabilities: `govulncheck`.
- Version Pinning: `.agents/skills/dependency-version-pinning/SKILL.md`.
- Infrastructure Constraints: `.agents/skills/server-infrastructure/SKILL.md`.
