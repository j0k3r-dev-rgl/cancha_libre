---
name: frontend-nextjs-testing
description: "Trigger: test, testing, vitest, frontend test, react testing library, server actions test, msw, ui test. Enforce Vitest, React Testing Library, Server Action isolation, and MSW API mocking for Next.js (no Playwright)."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when authoring, modifying, or reviewing tests in `apps/web` for React components, custom hooks, Server Actions, and UI state logic in CanchaLibre.

## Hard Rules

- **Execution Engine & Boundaries**:
  - Use Vitest with `happy-dom` or `jsdom` and `@testing-library/react`.
  - Heavy browser automation (Playwright/Cypress) is strictly excluded. All frontend verification runs through deterministic, headless Vitest suites.
- **User-Centric Accessible Queries**:
  - Test UI components from the user's perspective. Use accessible query priority:
    1. `getByRole` (with accessible name, e.g. `{ name: /reservar/i }`)
    2. `getByLabelText`
    3. `getByPlaceholderText`
    4. `getByText`
  - Prohibited: Querying by CSS selectors (`container.querySelector`), Tailwind utility classes, or asserting on internal component state.
- **Server Actions Testing**:
  - Server Actions (`"use server"`) must be tested directly as isolated async functions without rendering full page trees.
  - Verify Zod input validation errors, authorization guards, and mapped RFC 7807 error responses.
  - Prohibited: Invoking live Go backends during unit tests.
- **Contract-Driven API Mocking (MSW)**:
  - Mock backend HTTP calls using Mock Service Worker (MSW) handlers.
  - All mock payloads must type-check against generated TypeScript contracts (`src/types/api.generated.ts`). Never hand-write loose, untyped JSON mocks.
- **User Event Simulation**:
  - Always use `@testing-library/user-event` (v14+) over `fireEvent` to replicate real browser event dispatching (typing, clicking, blurring).
- **Next.js Router Isolation**:
  - Mock `next/navigation` (`useRouter`, `usePathname`, `useSearchParams`, `redirect`) with standard mock stubs.

## Decision Gates

| Target Under Test | Test Strategy | Forbidden Pattern |
| --- | --- | --- |
| Interactive component (e.g. TurnoPicker) | React Testing Library (`render` + `userEvent`) | Asserting on internal component state or CSS classes |
| Server Action (`"use server"`) | Direct async function invocation + MSW | Requiring full page browser automation to test an action |
| API boundary / Data fetching | MSW handlers typed with `api.generated.ts` | Manual untyped `global.fetch = vi.fn()` overrides |
| Custom React Hook | `renderHook` from `@testing-library/react` | Wrapping hooks in dummy visual components manually |
| Full flow verification | Vitest integration test combining component + action mock | Installing or running Playwright/browser drivers |

## Execution Steps

1. Create test file alongside feature: `src/features/{feature}/__tests__/{component}.test.tsx` or `{action}.test.ts`.
2. For components: set up MSW handlers if data is fetched, render with RTL, interact with `userEvent`, assert DOM role changes.
3. For Server Actions: pass valid and invalid FormData/objects, assert Zod validation rejections and formatted problem responses.
4. Execute `pnpm test` (or `vitest run`).
5. Ensure 100% deterministic execution without network leaks.

## Output Contract

Return:
- Colocated test files adhering to Vitest + RTL conventions.
- Tests relying strictly on accessible roles and user interactions.
- Zero Playwright or external browser dependencies.

## References

- Framework: Vitest, React Testing Library, Mock Service Worker (MSW).
- Frontend Patterns: `.agents/skills/frontend-nextjs-patterns/SKILL.md`.
- API Contracts: `.agents/skills/api-contracts-openapi/SKILL.md`.
