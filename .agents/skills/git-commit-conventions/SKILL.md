---
name: git-commit-conventions
description: "Trigger: commit, git commit, conventional commits, semantic commit, commit message. Enforce Conventional Commits specification with CanchaLibre module scopes, imperative mood, and atomic work units."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when preparing, composing, amending, or validating Git commit messages in CanchaLibre.

## Hard Rules

- **Conventional Format Structure**:
  ```text
  <type>(<scope>): <subject>

  [optional body]

  [optional footer(s)]
  ```
- **Allowed Types**:
  - `feat`: New user-facing or API feature (bumps MINOR version).
  - `fix`: Bug fix in logic or behavior (bumps PATCH version).
  - `docs`: Documentation updates only (guides, README, specs).
  - `refactor`: Code refactoring without behavior or API changes.
  - `perf`: Performance improvement.
  - `test`: Adding or correcting tests; no production code change.
  - `build`: Dependency bumps, Dockerfile, or build tooling updates.
  - `ci`: CI/CD workflows and automated pipelines.
  - `chore`: Maintenance tasks, meta configs, housekeeping.
- **Strict Project Scopes**:
  - Backend Modules: `booking`, `court`, `auth`, `payment`, `notification`, `core`.
  - Infrastructure & Contracts: `api`, `db`, `infra`, `openapi`.
  - Frontend: `web`, `ui`, `feature-canchas`, `feature-reservas`, `feature-auth`.
  - Global scope omitted only for root configuration files (e.g. root `.gitignore`).
- **Subject Line Discipline**:
  - Imperative, present tense: `add`, `fix`, `change`, `remove` (never `added`, `adds`, `fixing`).
  - Lowercase first letter after colon. No trailing period (`.`).
  - Hard limit of 72 characters for the subject line.
- **Breaking Changes**:
  - Breaking API or schema changes must append `!` before the colon (e.g. `feat(api)!: migrate reservation payload`) or include `BREAKING CHANGE: <explanation>` in the footer (bumps MAJOR version).
- **Atomic Work-Unit Rule**:
  - Every commit must be a coherent work unit: code, tests, and documentation changes for a single behavior must be committed together. Never split a feature and its corresponding unit tests into separate commits.

## Decision Gates

| Change | Commit Message Pattern | Forbidden Pattern |
| --- | --- | --- |
| New booking endpoint | `feat(booking): add slot reservation endpoint` | `feat: added endpoint.` |
| Fix race condition in reservation | `fix(booking): prevent double-booking with row lock` | `fix: bug fixed` |
| Add a column to the court table | `feat(court): add surface type column to court table` | `chore: update db` |
| Update README or guides | `docs(readme): add local postgres setup steps` | `docs: updates` |
| Breaking API contract change | `feat(api)!: rename court_id parameter to courtId` | `fix: changed param name` |

## Execution Steps

1. Inspect staged changes: `git diff --cached --stat`.
2. Confirm the commit is an atomic work unit (code + tests + docs colocated).
3. Determine primary `type` and module `scope`.
4. Compose imperative subject line under 72 characters in English.
5. If breaking, add exclamation mark `!` and describe breaking change in footer.
6. Commit with `git commit -m "..."`.

## Output Contract

Return:
- Structured commit message conforming to Conventional Commits.
- Verified type, scope, and character length boundaries.

## References

- Specification: Conventional Commits v1.0.0.
- Release Automation: `.agents/skills/release-changelog-tagging/SKILL.md`.
