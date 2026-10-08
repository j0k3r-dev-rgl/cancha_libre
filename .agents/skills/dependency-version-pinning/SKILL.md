---
name: dependency-version-pinning
description: "Trigger: version pinning, exact version, package.json, go.mod, lockfile, save-exact. Enforce strictly pinned dependency versions, ban floating version ranges (^/~/>=), and ensure lockfile immutability across Go and pnpm."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when installing, updating, reviewing, or auditing dependencies in `go.mod` (Go backend) or `package.json` (Next.js frontend), as well as validating lockfiles and CI build steps in CanchaLibre.

## Hard Rules

- **Sole Package Manager (pnpm Only)**:
  - `pnpm` is the sole authorized package manager for JavaScript/TypeScript across the entire repository.
  - Running `npm`, `npx`, `yarn`, or `bun` is strictly forbidden.
  - Creation of `package-lock.json`, `yarn.lock`, or `bun.lockb` is prohibited and must be rolled back immediately.
- **Zero Floating Versions in Package Manifests**:
  - In `package.json`, every entry in `dependencies` and `devDependencies` must specify an **exact version string** without range operators:
    - **Forbidden**: `^1.2.3`, `~1.2.3`, `>=1.2.3`, `1.x`, `*`, or `latest`.
    - **Required**: `"zod": "3.23.8"`, `"tailwind-merge": "2.5.4"`.
  - In `go.mod`, direct dependencies must target an exact tagged semantic version (e.g. `github.com/jackc/pgx/v5 v5.7.2`). Blind upgrades with `go get -u ./...` are strictly prohibited.
- **Mandatory Exact Installation Flags**:
  - Node/pnpm: Always install with `--save-exact` (e.g. `pnpm add <pkg>@<version> --save-exact` or `pnpm add -D <pkg>@<version> --save-exact`).
  - Go: Always supply the exact tag or commit hash: `go get <pkg>@vX.Y.Z`, followed by `go mod tidy` and `go mod verify`.
- **Lockfile Immutability (Supply Chain Guard)**:
  - `pnpm-lock.yaml` and `go.sum` are immutable sources of truth and must always be committed alongside manifest changes in the same work unit.
  - CI/CD pipelines and Docker container builds must enforce frozen lockfiles:
    - Node/pnpm: `pnpm install --frozen-lockfile`. Never run bare `pnpm install` in automated or deployment environments.
    - Go: `go mod download` and `go mod verify`.
- **Deliberate, Isolated Upgrades**:
  - Dependency upgrades must never occur as ambient side effects of feature development.
  - Upgrades must be isolated into dedicated commits (`build(deps): update <pkg> to <version>`), with tests executed before and after.

## Decision Gates

| Manifest State | Action Required | Prohibited Action |
| --- | --- | --- |
| Package manager command | Run strictly with `pnpm` (`pnpm add`, `pnpm install`, `pnpm exec`) | Invoking `npm`, `npx`, `yarn`, or `bun` |
| Adding new package | `pnpm add <pkg>@<version> --save-exact` | Running bare `pnpm add <pkg>` (which defaults to `^`) |
| Range operator (`^` or `~`) found in `package.json` | Immediately strip operator and pin exact version | Leaving ranges hoping minor versions won't break |
| Adding Go package | `go get <module>@vX.Y.Z && go mod tidy` | Using `go get <module>@latest` without pinning tag |
| CI / Production build | `pnpm install --frozen-lockfile` | Allowing lockfile updates during CI or Docker build |
| Upgrading dependency | Upgrade single package, test, commit atomically | Running broad `pnpm update` or `go get -u` across whole project |

## Execution Steps

1. Confirm the dependency has passed security audit via `.agents/skills/dependency-selection-audit/SKILL.md`.
2. Determine the exact target version (e.g. `1.4.2`).
3. For frontend (Next.js):
   - Run `pnpm add <package>@<version> --save-exact` (or `-D` for dev dependencies).
   - Inspect `package.json` and ensure no `^` or `~` was added.
4. For backend (Go):
   - Run `go get <package>@v<version>`.
   - Run `go mod tidy` and `go mod verify`.
5. Verify both the manifest (`package.json`/`go.mod`) and lockfile (`pnpm-lock.yaml`/`go.sum`) are staged and committed together.

## Output Contract

Return:
- Manifest containing only exact version strings with zero range prefixes.
- Cryptographic lockfile updated and consistent.
- Confirmation of `--save-exact` / explicit version tag usage.

## References

- Pre-Install Audit: `.agents/skills/dependency-selection-audit/SKILL.md`.
- Infrastructure & VPS Constraints: `.agents/skills/server-infrastructure/SKILL.md`.
- Commits Standard: `.agents/skills/git-commit-conventions/SKILL.md`.
