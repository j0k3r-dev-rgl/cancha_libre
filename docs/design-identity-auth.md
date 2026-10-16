# Identity auth: token, refresh and the active context

This design closes the `docs/prd-backend.md` section 7 item *Auth token and branch-context transport*, which S1 (identity) implements. It decides how a work actor authenticates, how the active business + branch travels, and what makes a refusal immediate. It decides no table DDL (T1.3), no frozen route (the endpoint inventory of section 5.2 owns that), and no profile, DNI or phone detail (S4).

## Decisions

| # | Topic | Decision |
| --- | --- | --- |
| D1 | Token transport | The Go API receives `Authorization: Bearer <jwt>`. The Next.js Server Action holds the token; the browser never does. Cookie is `HttpOnly; Secure; SameSite=Lax` (`auth-security-patterns`). |
| D2 | Active context transport | The active business + branch travels as a **claim in the access token**. Switching context re-issues the access token; the session does not change. |
| D3 | Claim layout | `sub` (account id), `actor`, `ctx { business_id, branch_id }`, `iat`, `exp`. `ctx` is **absent** for an actor with no branch scope (`PLAYER`, `SOPORTE`) — absence is meaningful, not zero-valued. |
| D4 | Access lifetime | **30 minutes** from issue. |
| D5 | Refresh | Rotating refresh token in an `HttpOnly` cookie, persisted **hashed** in PostgreSQL, one row per login session, with a **7-day** TTL renewed on every rotation. |
| D6 | Refresh reuse | Presenting an already-rotated token is treated as a leak: the **whole session family is revoked**, with no grace window. |
| D7 | Revocation effect | Immediate. The guard rebuilds authority from the database on every protected request, so the claim is a *selection*, never authority. |
| D8 | Refusals | RFC 7807 problem details: 401 unauthenticated, 403 authenticated without permission. A refusal mutates nothing. |
| D9 | Account separation | Work and customer accounts stay independent even when they share an email address; credentials, recovery and permissions never merge. |
| D10 | Who may sign in | Work actors (`ADMIN`, `OWNER`, `EMPLOYEE`) and the internal `SOPORTE` actor are invitation-only. The first `ADMIN` is provisioned by a private server command with a single-use invitation; no default password exists. |

## The invariant that matters

The token says **where the actor asked to work**. The guard says **whether they may**.

Every protected request re-verifies, and each check fails closed:

1. signature and `exp`;
2. the actor named by the claim;
3. an **active membership** for that account in the claimed business + branch;
4. the business is **enabled** and the branch is **enabled**;
5. own-record ownership when the record belongs to the actor.

Two consequences. A revoked membership, a disabled business or a disabled branch cuts the **next** request even though the access token is still cryptographically valid — that is what makes D7 immediate. And a branch identifier supplied by a client is never trusted: the claim is *validated against memberships*, not believed. `ctx` is a selector, not a permission. This is one indexed read by `(account, business, branch)`, not four separate queries.

## Flow

| Step | What happens |
| --- | --- |
| Sign in | Server Action posts credentials; the backend verifies them, issues access + refresh, and the action sets the cookie. |
| Protected request | Server Action forwards `Bearer`; the guard runs the invariant above and hands the use case the resolved, authorized actor. |
| Switch context | The actor selects another business or branch; the backend validates the membership and issues a **new access token** with the new `ctx`. The session and its refresh token do not change. |
| Refresh | The access token expired; the rotating refresh token is exchanged for a new pair and the used row is replaced. |
| Sign out | The session row is revoked and the cookies are cleared. |
| Revoked while signed in | The next protected request is refused (403), with no session sweep required. |

## Refresh rotation and its failure mode

One row per login session, keyed by a **hash** of the refresh token — the raw value is never stored. The row rotates on every exchange, within a 7-day session TTL. Presenting a token that was already rotated means the value leaked, so the whole session family is revoked (D6) rather than the single exchange refused. Sign-out revokes the row. Revoking a membership does not have to sweep rows, because the per-request invariant already refuses the request; a sweep is hygiene, never authority.

**Concurrency constraint this creates:** with no grace window, two refreshes racing from the same session would revoke a legitimate session. So the exchange must be atomic — one transaction that reads the hash, marks it rotated and returns the new row, so exactly one exchange wins — and the Server Action must keep one refresh in flight per session instead of firing two.

## What this design deliberately leaves open

| Open item | Why it is not decided here | Closed by |
| --- | --- | --- |
| Identity DDL, predicates and the session table shape | Separate design. | T1.3 |
| Endpoint names and their inventory rows | Routes freeze slice by slice, in the roadmap, before implementation. | S1 |

Decided in the second round and recorded above: access TTL 30 minutes (D4), refresh TTL 7 days (D5), and reuse of a rotated refresh revoking the whole session family (D6).

## Review checklist

- [ ] No token can reach browser storage: the cookie is `HttpOnly; Secure; SameSite=Lax`.
- [ ] Every refusal returns RFC 7807 and leaves stored data and counters unchanged.
- [ ] A revoked membership, disabled business or disabled branch cuts the *next* request.
- [ ] A replayed refresh token revokes the session family, and two concurrent refreshes cannot revoke a legitimate session.
- [ ] `ADMIN` is refused on reservations, customers and payments with a cryptographically valid token.
- [ ] `SOPORTE` carries no `ctx` and reaches only the audited identity procedures.
- [ ] Switching context issues a new access token and never widens authority beyond the memberships read in that same request.

## Next step

T1.3 designs the identity DDL and the minimal business/branch/membership spine this document reads from.
