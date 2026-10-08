---
name: release-changelog-tagging
description: "Trigger: release, changelog, tag, version, semver, git tag, cut release. Enforce Semantic Versioning, automated Keep a Changelog compilation from Conventional Commits, and annotated Git tag creation."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when cutting a release, updating `CHANGELOG.md`, calculating Semantic Versioning (SemVer) bumps, or generating and verifying Git release tags in CanchaLibre.

## Hard Rules

- **Deterministic SemVer Calculation**:
  - Version format must strictly follow `v<MAJOR>.<MINOR>.<PATCH>`.
  - Bump rules based on Conventional Commits since the previous tag:
    - **MAJOR**: Any commit containing `BREAKING CHANGE:` in the footer or `!` after the type/scope (e.g. `feat(api)!:`).
    - **MINOR**: Any commit of type `feat` without breaking changes.
    - **PATCH**: Commits containing `fix`, `perf`, or operational bug fixes without new features.
    - Commits of type `docs`, `chore`, `style`, `test`, or `ci` do not increment version numbers on their own.
- **Keep a Changelog Standard**:
  - The single source of truth is root `CHANGELOG.md`.
  - Format must adhere to [Keep a Changelog](https://keepachangelog.com/):
    - Top section: `## [Unreleased]`.
    - Version sections: `## [vX.Y.Z] - YYYY-MM-DD`.
    - Standard categories under each version:
      - `### Added`: for new features (`feat`).
      - `### Changed`: for modifications to existing functionality (`refactor`, `perf`).
      - `### Fixed`: for bug fixes (`fix`).
      - `### Removed`: for deprecated/removed capabilities.
- **Strict Annotated Tags**:
  - All release tags must be annotated: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`.
  - Lightweight tags (`git tag vX.Y.Z`) are strictly forbidden because they lack author, date, and verification metadata.
  - Tag name must strictly start with lowercase `v` followed by SemVer numbers.
- **Pre-Release Verification Gate**:
  - Releases must only be cut from the `main` branch.
  - The working directory must be completely clean (`git status --porcelain` returns empty).
  - All test suites (`go test -race ./...` and `pnpm test`) must pass before tag creation.

## Decision Gates

| Commits Since Last Tag | Version Bump | Action |
| --- | --- | --- |
| Contains `BREAKING CHANGE` or `!` | MAJOR (`v1.2.3` -> `v2.0.0`) | Update CHANGELOG, tag `v2.0.0` |
| Contains `feat(...)` | MINOR (`v1.2.3` -> `v1.3.0`) | Update CHANGELOG, tag `v1.3.0` |
| Contains only `fix(...)` | PATCH (`v1.2.3` -> `v1.2.4`) | Update CHANGELOG, tag `v1.2.4` |
| Only `docs` or `chore` | No release bump | Accumulate in `## [Unreleased]` |

## Execution Steps

1. Checkout `main` and ensure clean status: `git checkout main && git pull origin main && git status`.
2. Inspect log of commits since the last tag:
   ```bash
   PREV_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
   if [ -n "$PREV_TAG" ]; then
     git log ${PREV_TAG}..HEAD --oneline
   else
     git log --oneline
   fi
   ```
3. Determine version bump (MAJOR, MINOR, or PATCH).
4. Update `CHANGELOG.md`: transfer relevant entries from `[Unreleased]` to `## [vX.Y.Z] - YYYY-MM-DD`.
5. Commit changelog: `git add CHANGELOG.md && git commit -m "chore(release): cut version vX.Y.Z"`.
6. Create annotated tag: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`.
7. Push to remote: `git push origin main && git push origin vX.Y.Z`.

## Output Contract

Return:
- Updated `CHANGELOG.md` with categorized release entries.
- Verified annotated Git tag matching `vX.Y.Z`.
- SemVer calculation rationale based on committed conventional changes.

## References

- Specification: Semantic Versioning 2.0.0.
- Changelog Format: Keep a Changelog.
- Commit Standard: `.agents/skills/git-commit-conventions/SKILL.md`.
