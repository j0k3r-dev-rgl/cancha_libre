---
name: git-branching-workflow
description: "Trigger: branch, git branch, merge, pull request, feature branch, main branch, git workflow. Enforce feature branch isolation, explicit merge commits (--no-ff), and strict direct-to-main exceptions for passive documentation."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when starting new tasks, branching, merging code into `main`, creating pull requests, or deciding whether a change requires an isolated branch in CanchaLibre.

## Hard Rules

- **Feature Branch Isolation**:
  - All new features, bug fixes, refactoring, database schema changes, and contract updates must be implemented in a dedicated branch created from the latest `main`.
  - Direct commits of executable source code, tests, or SQL schema files to `main` are strictly prohibited.
- **Branch Naming Standard**:
  - `feat/<short-kebab-name>` (e.g. `feat/court-reservations`)
  - `fix/<short-kebab-name>` (e.g. `fix/slot-overlap-guard`)
  - `refactor/<short-kebab-name>` (e.g. `refactor/chi-router-middleware`)
  - `chore/<short-kebab-name>` (e.g. `chore/update-go-deps`)
- **Explicit Merge Traceability**:
  - Feature branches merged into `main` must use explicit merge commits (`git merge --no-ff <branch>` or GitHub PR "Create a merge commit").
  - Fast-forward merges (`--ff-only`) that erase the historical boundary of the feature are forbidden. Merge commits provide the audit log of when a feature entered `main`.
- **Direct-to-Main Exception Gate (Passive Docs Only)**:
  - Direct commits to `main` are allowed **exclusively** for passive, non-functional documentation:
    - Typo corrections in markdown files (`README.md`, `CONTRIBUTING.md`).
    - Project onboarding or local setup instructions.
    - Automated changelog bumps (`CHANGELOG.md`) during release procedures.
  - **Strict Architecture Boundary**:
    - OpenAPI specifications (`docs/swagger.*`), API contract definitions, database schemas, and architectural decision records impacting system behavior are **NOT** passive documentation. They must follow standard branch isolation.
- **Branch Hygiene**:
  - Always rebase or merge `main` into the feature branch before final integration.
  - Delete local and remote feature branches immediately after successful merge.

## Decision Gates

| Change Description | Workflow Path | Forbidden Action |
| --- | --- | --- |
| New feature or bug fix | Create `feat/*` or `fix/*` branch -> merge `--no-ff` | Committing directly to `main` |
| Database schema change | Dedicated branch -> tested against Postgres -> merge | Committing schema changes on `main` directly |
| OpenAPI contract change | Dedicated branch with synchronized TS types -> merge | Pushing spec changes directly to `main` |
| Markdown typo in README | Direct commit to `main` with `docs(...)` message | Opening an overhead PR for a single-character typo |
| Release notes / CHANGELOG | Direct commit to `main` as part of release workflow | Creating long-lived branches for release docs |

## Execution Steps

1. Verify base: `git checkout main && git pull origin main`.
2. For code, tests, or architecture:
   - Branch: `git checkout -b feat/<feature-name>`.
   - Implement with semantic work-unit commits.
   - Run tests (`go test -race ./...` and `pnpm test`).
   - Merge to main: `git checkout main && git merge --no-ff feat/<feature-name>`.
3. For passive documentation (typos, README):
   - Verify that changed files are strictly `.md` and non-architectural.
   - Commit directly to `main`: `git commit -m "docs(<scope>): <description>"`.
4. Delete merged branch: `git branch -d feat/<feature-name>`.

## Output Contract

Return:
- Confirmed branch naming adherence.
- Merge commit presence in git history for features and fixes.
- Verification that no code or schema changes bypassed branch protection.

## References

- Commit Standard: `.agents/skills/git-commit-conventions/SKILL.md`.
- Release Workflow: `.agents/skills/release-changelog-tagging/SKILL.md`.
