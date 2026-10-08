# Feature: Workflow and Testing Skills

## Context & Objectives
Define project-level skills for CanchaLibre covering testing (Go backend and Next.js frontend without Playwright), semantic commit standards, git branching model with merge tracking and docs exceptions, and automated changelog and tagging workflows.

## Tasks
- [x] Task 1: Create `backend-go-testing` skill (`.pi/skills/backend-go-testing/SKILL.md`)
- [x] Task 2: Create `frontend-nextjs-testing` skill (`.pi/skills/frontend-nextjs-testing/SKILL.md`)
- [x] Task 3: Create `git-commit-conventions` skill (`.pi/skills/git-commit-conventions/SKILL.md`)
- [x] Task 4: Create `git-branching-workflow` skill (`.pi/skills/git-branching-workflow/SKILL.md`)
- [x] Task 5: Create `release-changelog-tagging` skill (`.pi/skills/release-changelog-tagging/SKILL.md`)

## Evidence & Verification
- Skills follow LLM-first contract (concise, imperative, frontmatter description with triggers, concrete decision gates).
- `frontend-nextjs-testing` explicitly excludes Playwright, focusing on Vitest, React Testing Library, Server Actions, and MSW.
- Git workflow details branch isolation (`feat/*`, `fix/*`), explicit merge commits (`--no-ff`), and strict boundary for direct `main` commits (docs/typos only, never API contracts or architecture).
