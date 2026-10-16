---
name: auth-security-patterns
description: "Trigger: auth, authentication, authorization, rbac, roles, jwt, cookie, session, login, logout, redirect, protected routes. Enforce HttpOnly JWT cookies, Server Actions auth flow, scoped actor capability checks (four business roles plus audited internal support), and route redirection in CanchaLibre."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

## Activation Contract

Use this skill when implementing, refactoring, or reviewing authentication, authorization, session lifecycle, JWT handling, actor guards, Server Actions auth flow, or protected routes in CanchaLibre.

## Hard Rules

- **HttpOnly Secure Cookies for JWT**:
  - Store tokens strictly in `HttpOnly`, `Secure`, `SameSite=Lax` cookies.
  - Client storage (`localStorage`, `sessionStorage`, browser variables) is strictly forbidden for authentication tokens.
- **Server Actions for Auth Dispatch**:
  - Auth ops (login, social sign-in, logout) and private mutations run exclusively through Next.js Server Actions (`"use server"`).
  - Server Actions forward bearer token (`Authorization: Bearer <token>`) to Go backend; cookies managed via Next.js `cookies()`.
  - Never expose Go API auth endpoints or credentials directly to browser scripts.
- **Four Business Actors plus One Internal Support Actor with Scoped Authority**:
  - Business actors: `ADMIN`, `OWNER`, `EMPLOYEE`, `PLAYER`. Do not treat `ADMIN` as a universal superuser; a role string check alone is never sufficient.
  - `ADMIN`: Platform onboarding, co-owner assignments, branch approvals, and suspensions. **Never** reads internal branch reservations, customers, or payments; no operational override.
  - `OWNER`: Manages courts, branch rules, employees, local customer blocks, and operations for their business's branches. Cannot assign/revoke co-owners or override platform suspensions.
  - `EMPLOYEE`: Operational access (offered slots, reservations, attendance, manual payments) strictly in explicitly selected assigned branches. Cannot create courts, change branch rules, or block customers.
  - `PLAYER`: Public browsing, own profile, eligible booking requests, permitted cancellations/withdrawals, status, and history. Access to other customers' records or internal business data is forbidden.
  - `SOPORTE`: Internal operator actor, separate from `ADMIN`. Limited to the restricted identity procedures (phone-verification fallback by call, DNI conflict and correction review) and nothing else. Not a business role, not a UI dashboard, no general access bypass, and no read access to reservations, customers, or payments. Every intervention records actor, reason, and outcome, and it never weakens the role + membership + enabled-scope + own-record checks.
- **Authority Verification (Role + Membership + Enabled Scope + Own Record)**:
  - Backend handlers must independently verify: valid cryptographic signature, unexpired token, actor role, active membership in selected context, enabled scope (both business and branch enabled), and own-record ownership.
  - Operational staff actions require an explicitly selected authorized branch context. Context revocation is independent: revoking one branch membership does not revoke other authorized branches.
  - Refused authorization must not mutate domain state.
- **Account Separation and Sign-in Rules**:
  - Customer and work accounts are independent even when sharing an email address. Credentials, recovery, permissions, and customer DNI uniqueness remain isolated.
  - Customer social sign-in (Google/Apple) makes passwords optional; adding passwords or linking credentials requires explicit verification of both accesses.
  - Work roles (`ADMIN`, `OWNER`, `EMPLOYEE`) and the internal `SOPORTE` actor are invitation-only; no public signup exists for any of them. Initial `ADMIN` is provisioned via private server command with a single-use invitation.
  - Do not invent speculative JWT claim schemas, custom context headers, or token TTLs; enforce cryptographic and boundary invariants without inventing unagreed token layouts.
- **Public Browsing vs Route Redirection UX**:
  - Public discovery (branches, sports, offered slots, availability) is not globally auth-gated; unauthenticated visitors may browse enabled inventory.
  - Protected UX redirects: unauthenticated -> `/login`; insufficient role/context for feature -> `/dashboard`; `PLAYER` on `/dashboard` -> `/`. UX redirects must never weaken backend refusals.
- **Bounded Context Architecture**:
  - Bounded contexts follow canonical layout: `internal/modules/{context}/core/{domain,ports,usecase}` and `adapters/{inbound,outbound}` without assuming a fixed context map.

## Decision Gates

| Scenario | Required Pattern | Forbidden Pattern |
| --- | --- | --- |
| Storing session tokens | `HttpOnly`, `Secure`, `SameSite=Lax` cookie via Server Actions | Storing tokens in `localStorage`, `sessionStorage`, or client state |
| Auth & private dispatch | Server Action calling Go API with `Authorization: Bearer <token>` | Browser client `fetch` calling Go backend directly |
| Backend authorization | Role + membership + enabled scope + own-record check | Role-only string check (`role === 'ADMIN'`) or trusting client branch ID |
| `ADMIN` accessing operational data | Refuse access (reservations, customers, payments are internal to branch) | Allowing `ADMIN` global read/override on establishment operational data |
| `SOPORTE` intervention | Privileged, audited identity procedure (phone-verification fallback, DNI conflict/correction) with actor, reason, and outcome recorded | Operating as a support dashboard, reading establishment operational data, acting without an audit record, or treating support as a role that skips membership/scope checks |
| Staff branch context access | Require active membership in explicitly selected branch context | Allowing cross-branch access based on shared business, city, or account |
| Shared email (work & customer) | Treat work and customer accounts as independent isolated contexts | Merging accounts or permissions based on matching email |
| Public catalog discovery | Allow unauthenticated public browsing of enabled branch offers | Forcing authentication to view public offers or branches |
| Insufficient role on route | Next.js UX redirect to `/dashboard` (or `/` for `PLAYER` on `/dashboard`) | Rendering broken UI, generic 500, or relying on redirect for security |

## Execution Steps

1. Define the business actor capabilities (`'ADMIN' | 'OWNER' | 'EMPLOYEE' | 'PLAYER'`) and the internal `'SOPORTE'` actor in shared contracts.
2. In Go backend bounded contexts (`internal/modules/{context}/core/...` and `adapters/inbound/http`), implement authentication and middleware verifying token signature, expiration, actor role, active membership, enabled entity scope, and own-record ownership.
3. Enforce strict boundary checks: reject `ADMIN` reads on internal reservations/customers/payments, isolate branch staff to explicitly selected memberships, and keep work/customer accounts separated.
4. In Next.js, implement Server Actions (`src/features/auth/actions/`) to manage login, social auth, logout, and token forwarding without exposing backend endpoints to browser scripts.
5. In Next.js layouts and page components, apply route guards: redirect unauthenticated requests to `/login`, role/context mismatches to `/dashboard`, and `PLAYER` at `/dashboard` to `/`. Keep public browsing routes accessible without authentication.
6. Verify that backend refusals return RFC 7807 problem details without mutating state, regardless of frontend UI state.

## Output Contract

Return:
- Secure `HttpOnly`, `Secure`, `SameSite=Lax` cookie configuration verified.
- Server Actions auth flow forwarding Bearer tokens server-side to the Go API.
- Four business actor capabilities plus the internal `SOPORTE` actor enforced with role + membership + enabled scope + own-record verification.
- Explicit exclusion of `ADMIN` access to internal reservations, customer records, and payment data.
- Audited `SOPORTE` authority limited to the phone/DNI procedures, with no operational read access.
- Independent account contexts for work and customer profiles even when sharing email.
- Route redirection UX rules enforced without compromising backend security invariant.

## References

- Go Backend Architecture: `.agents/skills/backend-go-patterns/SKILL.md`.
- Next.js Patterns: `.agents/skills/frontend-nextjs-patterns/SKILL.md`.
- API Contracts: `.agents/skills/api-contracts-openapi/SKILL.md`.
- Monolith Bounded Contexts: `.agents/skills/modular-hexagonal-architecture/SKILL.md`.
