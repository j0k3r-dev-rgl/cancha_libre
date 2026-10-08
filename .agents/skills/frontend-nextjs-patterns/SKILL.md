---
name: frontend-nextjs-patterns
description: "Trigger: frontend, nextjs, react, server actions, tailwind, app router, components, ui, standalone. Enforce Next.js App Router patterns, Server Actions, feature architecture, and Tailwind styling for CanchaLibre."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when creating, refactoring, or reviewing frontend components, Next.js App Router pages, Server Actions, styles, or client-server state handling in `apps/web`.

## Hard Rules

- **Next.js App Router & Server Components Default**:
  - All components in `app/` and `src/components/` default to React Server Components (RSC).
  - Use `"use client"` strictly at the lowest leaf level of the component tree that requires browser APIs, event listeners, or client-side React hooks (`useState`, `useEffect`).
- **Feature-Driven Structure**:
  - Domain-specific logic is organized by feature: `src/features/{feature-name}/` (e.g., `canchas/`, `reservas/`, `turnos/`, `auth/`).
  - Each feature contains its own `components/`, `actions/` (Server Actions), and optional `hooks/`.
  - Shared generic UI components live in `src/components/ui/` (primitives styled with Tailwind).
- **Server Actions for Go API Communication**:
  - Mutating operations and private queries must run through Server Actions (`"use server"`).
  - Server Actions handle communication with the Go backend API. Internal API endpoints, service tokens, and backend URLs must never be exposed to the client browser.
  - Server Actions must validate inputs with Zod before dispatching requests to the Go backend.
  - Types for backend payloads and responses must be imported from the generated API contract (`api.generated.ts`).
- **Styling with Tailwind CSS**:
  - Use utility classes with mobile-first responsive design (`sm:`, `md:`, `lg:`).
  - Avoid inline CSS or arbitrary one-off values when theme tokens exist.
- **Resource & Deployment Boundary (2 vCPU / 8 GB VPS)**:
  - Configure `output: 'standalone'` in `next.config.ts`.
  - Builds must occur in CI/CD; never run `next build` on the production VPS.

## Decision Gates

| Task | Required Pattern | Forbidden Pattern |
| --- | --- | --- |
| Data mutation (e.g. creating booking) | Next.js Server Action (`"use server"`) calling Go backend | Calling Go API directly from client browser with exposed credentials |
| Fetching public data (e.g. court catalog) | React Server Component with direct server fetch | Empty client component fetching via `useEffect` on mount |
| Interactive UI (e.g. time slot picker) | Isolate leaf component with `"use client"` | Putting `"use client"` at the page level |
| Input validation in actions | Zod schema validation before calling Go API | Sending unvalidated raw FormData to backend |
| Deployment configuration | `output: 'standalone'` with lean Docker container | Full Node.js build with complete `node_modules` on VPS |

## Execution Steps

1. Create or place the page in `app/(routes)/...` as a Server Component.
2. If the view belongs to a specific business area, place implementation inside `src/features/{feature-name}/`.
3. Create Server Actions in `src/features/{feature-name}/actions/` with `"use server"`, validate inputs with Zod, and call the Go backend.
4. Use generated TypeScript contracts (`src/types/api.generated.ts`) for all request/response types.
5. Build interactive inputs with Tailwind and wrap them in focused `"use client"` leaf components.
6. Verify responsive layout and error handling states (using RFC 7807 problem details returned by Go).

## Output Contract

Return:
- Component files located in appropriate feature directory.
- Clear separation between Server Components and Client Components.
- Server Actions with input validation and typed Go API calls.
- Verification of Tailwind responsive styling and standalone configuration.

## References

- Framework: Next.js (App Router).
- Styling: Tailwind CSS.
- API Contracts: `.agents/skills/api-contracts-openapi/SKILL.md`.
- Server Limits: `.agents/skills/server-infrastructure/SKILL.md`.
