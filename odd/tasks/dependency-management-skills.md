# Feature: Dependency Security and Pinning Skills

## Context & Objectives
Define project-level skills for CanchaLibre to enforce pre-installation dependency auditing (CVE vulnerability scanning, maintenance health, zero-bloat validation, alternative evaluation) and strict exact version pinning (no floating versions, lockfile immutability) across Go and Next.js/pnpm.

## Tasks
- [x] Task 1: Create `dependency-selection-audit` skill (`.pi/skills/dependency-selection-audit/SKILL.md`)
- [x] Task 2: Create `dependency-version-pinning` skill (`.pi/skills/dependency-version-pinning/SKILL.md`)
- [x] Task 3: Establish project-level tooling guardrail with root `.npmrc` (`save-exact=true`)

## Evidence & Verification
- `dependency-selection-audit` enforces security CVE checks (`govulncheck`, `pnpm audit`, OSV), documentation review, bloat rejection, and multi-library comparison.
- `dependency-version-pinning` bans floating version ranges (`^`, `~`, `>=`), enforces exact tags in `go.mod` and exact strings in `package.json`, and requires immutable lockfile usage in CI/Docker (`--frozen-lockfile`, `go mod verify`).
- `.npmrc` enforces `save-exact=true` as a tooling backstop.
