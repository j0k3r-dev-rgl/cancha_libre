---
name: auth-security-patterns
description: "Trigger: auth, authentication, authorization, rbac, roles, jwt, cookie, session, login, logout, redirect, protected routes. Enforce HttpOnly JWT cookies, Server Actions auth flow, direct role-based access control, and route redirection in CanchaLibre."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when implementing, refactoring, or reviewing authentication, authorization, session lifecycle, JWT handling, role guards, Server Actions auth flow, or protected routes in CanchaLibre.

## Hard Rules

- **HttpOnly Secure Cookies for JWT**:
  - JWT tokens must be stored strictly in `HttpOnly`, `Secure`, `SameSite=Lax` cookies.
  - Client-side storage (`localStorage`, `sessionStorage`, or variables accessible from browser scripts) is strictly forbidden for authentication tokens.
- **Server Actions for Auth Dispatch**:
  - Authentication operations (login, register, logout) and private queries must run exclusively through Next.js Server Actions (`"use server"`).
  - Server Actions extract the JWT cookie, forward it to the Go backend API via `Authorization: Bearer <token>`, and set/clear cookies using the Next.js `cookies()` API.
  - Never expose the Go API auth endpoints or credentials directly to the client browser.
- **Direct Role-Based Access Control (No Granular Permissions)**:
  - System uses direct role evaluation (`ADMIN`, `OWNER`, `PLAYER`). Dynamic permission matrices, scopes arrays, or ABAC rules are explicitly excluded.
  - Roles:
    - `ADMIN`: Global platform superuser.
    - `OWNER`: Court / sports club manager.
    - `PLAYER`: Regular customer booking court slots.
- **Route Redirection & Protection Policy**:
  - **Unauthenticated**: Any protected route without a valid session redirects to `/login`.
  - **Insufficient Role (Feature Route)**: If an authenticated user accesses a route requiring a role they do not have, redirect to `/dashboard`.
  - **Insufficient Role (Dashboard Root)**: If a `PLAYER` lands on `/dashboard`, redirect to `/` (Home).
- **Backend Cryptographic Verification Invariance**:
  - Frontend route redirection is strictly UX. The Go backend (`Chi router` middleware) must independently verify the JWT cryptographic signature and role claims on every incoming request.

## Decision Gates

| Scenario | Required Pattern | Forbidden Pattern |
| --- | --- | --- |
| Storing JWT session | `HttpOnly` cookie via Next.js `cookies()` | Storing tokens in `localStorage` or `sessionStorage` |
| Login / Logout dispatch | Server Action calling Go backend | Calling Go API directly from client `fetch` in browser |
| Role verification | Direct role string check (`role === 'OWNER'`) | Over-engineered permissions array or dynamic capability trees |
| User lacks role for specific route | Redirect to `/dashboard` | Rendering a broken page or generic 404 |
| `PLAYER` accesses `/dashboard` | Redirect to `/` (Home) | Allowing unauthorized dashboard access or 500 error |
| Backend authorization | Go middleware parsing JWT claims from `Authorization` | Trusting frontend claims without signature verification |

## Execution Steps

1. Define role types in shared types/contracts: `'ADMIN' | 'OWNER' | 'PLAYER'`.
2. In Go backend (`internal/modules/auth`), implement JWT issuance with secret signing, expiration, and payload claims (`sub`, `role`, `exp`).
3. In Next.js, implement Server Actions (`src/features/auth/actions/`) to handle login/logout and set `HttpOnly` cookies.
4. Implement route protection guards in Next.js Server Components / Layouts:
   - If not authenticated -> redirect to `/login`.
   - If role mismatch on specific route -> redirect to `/dashboard`.
   - If accessing `/dashboard` with `PLAYER` role -> redirect to `/`.
5. In Go HTTP handlers, wire `RequireRole(role string)` middleware verifying the JWT claim before executing use cases.

## Output Contract

Return:
- Secure HttpOnly cookie configuration verified.
- Server Actions handling auth flow and forwarding Bearer token to Go.
- Direct role checks applied without granular permissions.
- Route redirection rules (`/dashboard` and `/`) enforced.

## References

- Go Backend Architecture: `.agents/skills/backend-go-patterns/SKILL.md`.
- Next.js Patterns: `.agents/skills/frontend-nextjs-patterns/SKILL.md`.
- API Contracts: `.agents/skills/api-contracts-openapi/SKILL.md`.
