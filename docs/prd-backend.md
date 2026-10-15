# CanchaLibre — Backend MVP PRD

**Status:** Requirements draft derived from the agreed [MVP](mvp.md), including decisions O-01–O-12. This document specifies backend outcomes, not implemented features or permission to implement, deploy, purchase services, or activate billing.

**Scope:** Go backend capabilities, persistence invariants, authorization, external integrations, background processing, and backend verification. The MVP remains the product source of truth; this PRD does not replace it or silently resolve unspecified rules.

## 1. Purpose and reading guide

Provide a trustworthy reservation agenda for establishments: offered inventory is distinct from customer requests, only confirmation occupies a slot, and simultaneous actions must not bypass capacity, customer limits, or access boundaries.

- Read sections 3–5 for the required capabilities and their acceptance conditions.
- Use the `BE-*` identifiers to plan implementation and tests. All listed capabilities belong to the MVP; their order here is not a delivery commitment.
- Section 6 maps every MVP acceptance example to backend requirements.
- Sections 7–8 separate engineering prerequisites and pre-operation checks from agreed product behavior.

A requirement is not complete merely because an endpoint exists. Its authorization, state effects, rejection behavior, and applicable concurrency checks must be demonstrated.

## 2. Boundary and current baseline

### Included

- Public and authenticated backend operations, including administrative commands and restricted support procedures where the MVP calls for them.
- Business rules, scoped data access, persistence, lifecycle processing, and integration with Google/Apple sign-in, WhatsApp through Evolution/Baileys, and Google Maps.
- Data returned for client-visible status, public profiles, history, failure lists, and map results. UI rendering itself is not included.
- Backend contracts and verification evidence sufficient for a separate frontend to consume them.

### Excluded

- Pages, React components, Next.js Server Actions, browser redirects, map rendering, device permission dialogs, and frontend implementation or generated frontend code.
- Payment processing, gateways, refunds, payment movement ledgers, automatic booking acceptance, waiting lists, recurring weekly inventory, promotional notices, and reminders.
- Automatic identity verification, document image processing/storage, general-purpose support dashboards, and unrestricted administrator access to establishment operations.
- Concrete endpoint paths, table DDL, exact bounded-context map, JWT claim layouts, cryptographic key parameters, and concrete Compose files. These require slice-level technical design within the normative baseline below; this PRD describes the outcomes they must preserve.

### Normative technical baseline (project skills)

Project skills under `.agents/skills/` already fix the following technical decisions for CanchaLibre; implementation slices must honor them rather than re-deciding them:

| Area | Normative rule | Source skill |
| --- | --- | --- |
| Modular architecture | Modular monolith organized by **bounded context** (`internal/modules/{context}/core/{domain,ports,usecase}` and `adapters/{inbound,outbound}`), not one module per table/entity. A context may hold multiple cohesive aggregates. Inter-context calls go strictly through public inbound ports wired at the composition root (`cmd/api`) with portable contract DTOs, preparing contexts for potential future service extraction without assuming zero-effort extraction. | `modular-hexagonal-architecture`, `backend-go-patterns` |
| HTTP & error contracts | Go 1.24+, Chi router (`github.com/go-chi/chi/v5`) compatible with `net/http`, explicit JSON DTO structs, and RFC 7807 Problem Details responses mapped from sentinel domain errors (400, 403, 404, 409, 422, 500). | `backend-go-patterns`, `api-contracts-openapi` |
| Persistence & migrations | PostgreSQL 16+, `pgx/v5` (`pgxpool` `MaxConns: 15–20`, `MinConns: 2`, `MaxConnLifetime: 1h`, server `max_connections = 50`), `UUIDv7`/`UUIDv4` keys, schema-per-bounded-context isolation (`schema_{context}`) with zero cross-schema foreign keys or SQL joins, and reversible pure-SQL Goose migrations (`-- +goose Up` / `Down`) inside each context. | `database-postgres-patterns`, `backend-go-patterns` |
| Concurrency & events | Transactional row locks (`SELECT ... FOR UPDATE`) and `btree_gist` / `tstzrange` constraints scoped to court offers, court confirmations, and customer cross-branch confirmed overlaps while keeping pending requests non-exclusive. Monolith events use bounded in-process Go channels and worker pools with context propagation; timers/retries requiring durability across restarts persist state in PostgreSQL. | `database-postgres-patterns`, `modular-hexagonal-architecture`, `golang-concurrency` |
| Security & actor guards | Four actors (`ADMIN`, `OWNER`, `EMPLOYEE`, `PLAYER`); backend verifies signature, expiry, role, active membership in explicitly selected branch context, enabled business/branch scope, and own-record ownership. `ADMIN` is barred from internal reservations, customers, and payments. | `auth-security-patterns`, `golang-security` |
| Infrastructure & dependencies | Single-node Docker Compose / systemd on 2 vCPU, 8 GB RAM, 100 GB NVMe VPS; CI-only builds; soft `GOMEMLIMIT=150MiB` and `GOMAXPROCS=2` with distinct hard container limits; pinned dependencies audited with `govulncheck` and permissive licenses only; tests run with `go test -race ./...`. | `server-infrastructure`, `dependency-selection-audit`, `dependency-version-pinning`, `backend-go-testing` |

### Observed repository baseline

At MVP commit `be7a250`, `core/` is an initial Go/Chi HTTP skeleton, not an implemented reservation platform:

| Area | Existing baseline | Not established by the baseline |
| --- | --- | --- |
| Entrypoint/configuration | `core/cmd/api/main.go`, `core/internal/config/config.go` | Production integrations, database initialization, or secure production provisioning |
| HTTP | `core/internal/adapter/http/router.go`: `/health`, `/api/v1/ping`; JSON/Problem Details helpers in `response.go` | Authentication, business operations, tenant isolation, or public discovery |
| Domain/use cases/storage | Sentinel errors in `core/internal/domain/errors.go`; initial layer directories | Business models, workflows, repositories, migrations, or transactional guards |
| Tests | Initial health-endpoint test in `core/internal/adapter/http/router_test.go` | Proof of any MVP acceptance example |

No requirement below is marked implemented based on this skeleton. No implementation test suite was run to prepare this PRD.

## 3. Actors and backend authorization boundary

| Actor/context | Backend authority | Explicit exclusion |
| --- | --- | --- |
| Platform administrator | Business onboarding, co-owner assignments, branch approvals, and suspension/reactivation | Cannot read internal establishment reservations, customers, or payments merely because of administrator status |
| Business co-owner | Equal access to their business's branches; courts, rules, employees, local customer restrictions, public information, and daily operations | Cannot assign/revoke co-owners or override platform suspension |
| Branch employee | Offered inventory, reservation operations, attendance, and manual payments in explicitly selected assigned branches | Cannot create courts, change branch rules, block customers, or access unassigned branches |
| Customer | Public browsing and their own profile, eligible requests, permitted cancellation/withdrawal, status, and history | Cannot access another customer's records or establishment internal information |
| Restricted support procedure | The narrowly authorized phone/DNI review and correction actions specified in the MVP | Not a fifth business role, a public support signup, or a grant of internal establishment access |

Work and customer identities remain independent even if the same person/email owns both. Backend membership and enabled-context checks are authoritative; frontend state or a submitted branch identifier never grants access.

## 4. Functional requirements

Each row includes the minimum observable result or invariant and its MVP source. References to AC identifiers mean future acceptance criteria, not tests already present.

### 4.1 Accounts, membership, and access

| ID | Requirement and acceptance condition | MVP source |
| --- | --- | --- |
| BE-ACC-01 | Support customer sign-in through Google and Apple and optional local email/password access, including recovery. Social sign-in does not require a password. Request eligibility is evaluated separately from successful sign-in. | §7; AC-15, AC-24 |
| BE-ACC-02 | Permit independent customer and work accounts to share an email. Sign-in identifies the account context; credentials, recovery, permissions, and customer DNI uniqueness do not spill into the work identity. | §§3, 7; AC-42 |
| BE-ACC-03 | Allow an authenticated social customer to add a password at any time with verification of the email used for access/recovery. Linking sign-in methods is explicit and verifies both accesses; email coincidence alone never merges accounts. | §7; AC-42 |
| BE-ACC-04 | Provide invitation-based email/password activation and recovery for work roles, with no public work-role signup. Provision the first administrator through an authorized private server command and a single-use activation invitation, never a default password. | §7; AC-43 |
| BE-ACC-05 | Allow only the platform administrator to assign/revoke business co-owners. Co-owners have equal branch access. Refuse last-co-owner revocation without changing membership until a replacement exists. | §3; AC-43 |
| BE-ACC-06 | Allow co-owners to invite/manage employees and branch memberships. An employee may belong to multiple branches; operations require an explicitly selected authorized context. Revoking one context does not revoke another independent membership. | §3; AC-18, AC-26 |
| BE-ACC-07 | Enforce the four-actor permission matrix on every protected operation. Refuse employee attempts to manage courts/rules/customer blocks, and refuse administrator reads of internal reservations, customers, and payments. Rejection must not mutate domain state. | §3; AC-18, AC-19, AC-26 |
| BE-ACC-08 | Require first name, last name, declared DNI, and verified phone before accepting a customer reservation request. An incomplete/unverified customer may still browse. A complete profile is not verified identity. | §7; AC-15, AC-16, AC-24 |
| BE-ACC-09 | Let co-owners block/unblock new customer requests for one branch with a recorded reason. A local block neither cancels existing reservations nor affects other branches; employees cannot perform it. | §7; AC-17, AC-18 |

### 4.2 Phone verification and protected DNI handling

| ID | Requirement and acceptance condition | MVP source |
| --- | --- | --- |
| BE-ID-01 | Issue a six-digit phone-verification code through WhatsApp, valid for 5 minutes, with at most 5 attempts per code. A replacement invalidates the prior code; successful verification consumes it. Codes are not logged. | §7; AC-46 |
| BE-ID-02 | Enforce at least 60 seconds between code sends and at most 5 sends per phone per hour, plus abuse controls. Denied sends do not dispatch a message or enable booking. Limits must not be bypassed by simultaneous requests. | §7; AC-46 |
| BE-ID-03 | Keep a verification-code request separate from optional reservation-notice consent. Verifying a phone never subscribes a customer to notices. | §§7, 10; AC-46, AC-48 |
| BE-ID-04 | Permit restricted support verification by calling the number and checking a code displayed in the customer's application session when WhatsApp is unavailable/restricted or the person has no WhatsApp. Record the intervention; do not waive proof of control. While waiting, browsing is allowed and requests are refused, without a response-time promise. Already-eligible customers continue operating during channel outages. | §7; AC-47 |
| BE-ID-05 | Normalize declared DNI by removing dots/spaces and ignoring leading zeroes; reject letters and other characters. `12.345.678`, `12345678`, and `012345678` must compare as one DNI. Store only a protected uniqueness fingerprint and final digits, not a recoverable full number. | §7; AC-44 |
| BE-ID-06 | Enforce customer DNI uniqueness without revealing another account's information, including under simultaneous eligibility attempts. A duplicate blocks the claimant's booking enablement, not automatically the existing account. A claim alone neither proves legitimacy nor authorizes restriction/reassignment. | §7; AC-16, AC-44 |
| BE-ID-07 | After booking enablement, correct DNI only through restricted support with legitimacy/duplicate review and a recorded change. If recovery cannot resolve a conflict, correction/reassignment requires private live human review of person/document. A known DNI or verified phone is insufficient; retain decision/rationale, not photos or recordings. This is a support procedure outside the MVP interface, not an automated identity-verification feature. | §7; AC-44, AC-45 |
| BE-ID-08 | After effective account closure, retain the DNI fingerprint/final digits for 30 days, preventing reuse during that period, then delete them without erasing establishment reservation history. Legal review is required before operation. This does not define a broader account-deletion or history-retention policy. | §7; AC-45 |

### 4.3 Businesses, branches, courts, and archival

| ID | Requirement and acceptance condition | MVP source |
| --- | --- | --- |
| BE-ORG-01 | Support platform-enabled businesses with one or more branches, including a branch for a single-location business. Maintain branch-scoped courts, employees, agenda, rules, and local restrictions; shared city or account does not grant membership. | §§2–4; AC-26 |
| BE-ORG-02 | Accept owner requests for new branches and platform approval/rejection. Before approval, refuse public rental operation. Return rejection reasons and allow correction/resubmission of the same request with its review history preserved. | §4; AC-25, AC-41 |
| BE-ORG-03 | Allow co-owners to directly edit public description, photos, contacts, names, and an approved branch's address/location without platform reapproval. Require confirmation/correction of the proposed map point before publication. Approval, memberships, and suspensions are not owner-editable public fields. | §§3–4; AC-41 |
| BE-ORG-04 | Allow co-owners to manage physical courts, supported sports, default slot duration, and base price. An employee may create offered slots but not courts. Multiple sports on a court do not imply separate capacity. | §§3–4; AC-01, AC-03, AC-18 |
| BE-ORG-05 | Keep owner-editable request/cancellation lead times per branch: default 1 hour/4 hours respectively, whole minutes from 0 through 10,080 inclusive. Refuse invalid settings without replacing existing values. | §6; AC-27 |
| BE-ORG-06 | Archive businesses, branches, and courts rather than permanently deleting them; retire offers without deleting history. Refuse archival until pending requests are resolved, future confirmed reservations explicitly cancelled with a reason, and already-started reservations finished with attendance recorded. Archival must not silently cancel reservations. | §5; AC-40 |
| BE-ORG-07 | Allow an actor authorized to manage an entity to explicitly unarchive it, preserving approval requirements, suspensions, restrictions, references, and history. Closed requests and cancelled reservations never revive. | §5; AC-40 |

### 4.4 Offered inventory and availability

| ID | Requirement and acceptance condition | MVP source |
| --- | --- | --- |
| BE-OFF-01 | Let co-owners and authorized branch employees create an offered slot for a court, sport, date, start/end time, and ARS price, overriding court duration/price defaults. Creation makes an offer, not a customer reservation or payment. | §§2, 4–5; AC-01, AC-02 |
| BE-OFF-02 | Refuse overlapping offers on the same physical court regardless of sport, including concurrent creation. A rejected creation leaves existing inventory unchanged. | §4; AC-03, AC-30 |
| BE-OFF-03 | Generate batches from court, dates, time range, duration, and price. Any generated-slot conflict rejects the entire batch with conflict information and creates no slots. Existing offers remain unchanged; no partial success is reported. | §4; AC-02, AC-30 |
| BE-OFF-04 | Generate only complete intervals and expose the excluded remainder before staff confirm creation. For 18:00–22:30 at 60 minutes, offer four slots ending at 22:00 and exclude 22:00–22:30, not a shortened fifth slot. | §4; AC-31 |
| BE-OFF-05 | Limit batch dates to 180 days from the current date in the agreed Argentine timezone: day 180 is within the horizon, day 181 is outside it. Do not interpret this as a slot-count cap or automatic weekly recurrence. | §4; AC-32 |
| BE-OFF-06 | Keep availability separate from request/reservation/payment status. Pending requests do not occupy a slot; an active confirmed reservation does. Cutoff/start passage can make an unoccupied offer unrequestable without making it occupied. | §§2, 5–6; AC-01, AC-04, AC-08, AC-14 |
| BE-OFF-07 | Refuse edits to court, date, times, or price while pending requests or a confirmed reservation exist. Preserve customer-accepted price. Staff must resolve affected requests/reservations with a reason and offer another slot rather than silently reschedule them. | §5; AC-10 |
| BE-OFF-08 | Permit staff to block an offer with only pending requests, closing those requests as unavailable with a visible reason. Refuse blocking while a confirmed reservation exists until explicit cancellation with a reason. | §5; AC-11, AC-12 |
| BE-OFF-09 | Cancellation of a confirmed reservation normally restores availability unless another restriction applies. Reopening a withdrawn/restricted offer is explicit and preserves its court/date/times/price; it requires an enabled scope, future start, no confirmed reservation, and no other restriction. Changing those terms when reoffering requires retiring the old offer and creating another. Nothing closed revives. | §§5, 9; AC-06, AC-21, AC-33 |

### 4.5 Requests, confirmation, cancellation, and attendance

| ID | Requirement and acceptance condition | MVP source |
| --- | --- | --- |
| BE-RES-01 | Accept eligible customer requests only against existing requestable offers. Multiple customers can be pending for one slot without holding capacity. Return a pending outcome that distinguishes a request from a confirmed reservation. | §§5–7; AC-04, AC-15 |
| BE-RES-02 | Enforce at most one pending request per customer per branch, at most five across the platform, and no duplicate pending request for the same customer/slot. Enforce these limits during concurrent submissions; rejection must not create a request or corrupt pending counts. | §7; AC-07 |
| BE-RES-03 | Apply inclusive request cutoffs strictly before slot start. A four-hour requirement for 20:00 permits 16:00 and refuses later submissions. Zero lead time still refuses a request at or after 20:00. | §6; AC-28, AC-29 |
| BE-RES-04 | Let authorized staff confirm/reject pending requests. Confirmation rechecks enabled scope, availability, start time, and customer overlap; the new-request cutoff does not prevent confirming a request received in time, until strictly before start. | §6; AC-05, AC-28 |
| BE-RES-05 | Confirmation must atomically occupy the slot and close all its other pending requests as unavailable with a visible reason. Simultaneous attempts must not yield two confirmed allocations for the same court/time. Confirmation and manual guest allocation must respect the same capacity invariant. | §6; AC-05, AC-13 |
| BE-RES-06 | Refuse overlapping confirmed reservations for one platform customer, even across branches and under simultaneous confirmations. Do not impose an additional count cap on non-overlapping confirmations. Do not reveal the customer's other branch reservation details when reporting a conflict. | §§6–7; AC-37 |
| BE-RES-07 | Expire unanswered pending requests at slot start; they cannot then be confirmed or continue consuming pending eligibility. Expiration creates no confirmed reservation. Closed competing requests never reactivate when a winner is cancelled; there is no waiting list. | §6; AC-06, AC-08, AC-28 |
| BE-RES-08 | Let authorized staff directly allocate an available offered slot to a guest before start with name/phone only. No customer account, DNI, or phone verification is required. Guest contacts remain private to the establishment and automatic notices are not sent. | §6; AC-13 |
| BE-RES-09 | Let customers withdraw their pending requests. Preserve history using the product label `Solicitud retirada`, distinguishing withdrawal from cancellation of a confirmed reservation; release both branch/global pending counts. | §6; AC-36 |
| BE-RES-10 | Permit customer cancellation of their confirmed reservation at the inclusive branch cutoff, never at/after slot start even with zero lead time. After the cutoff refuse in-app customer cancellation and provide the outcome needed to direct them to the branch. Staff cancellation requires a reason. | §6; AC-09, AC-29 |
| BE-RES-11 | Only authorized owners/employees manually record attendance: no-show from the exact start, without mandatory grace time; completed from the exact end. Refuse early outcomes and never infer attendance automatically. | §6; AC-35 |
| BE-RES-12 | Expose a customer's own reservation/request outcomes, visible reasons, and history, including withdrawn, unavailable, expired, cancelled, completed, and no-show results. Preserve history through offer retirement, archival, suspension, and cancellation; enforce own-record access. | §§5–7, 9–10; AC-06, AC-08, AC-15, AC-20, AC-22, AC-36 |

### 4.6 Manual payment records

| ID | Requirement and acceptance condition | MVP source |
| --- | --- | --- |
| BE-PAY-01 | Let authorized branch staff record accumulated ARS amounts for cash, bank transfer, card, and other on a reservation. Sum the method amounts into a total; do not create a ledger of individual payment/refund movements. | §8; AC-14, AC-34 |
| BE-PAY-02 | Calculate payment status from recorded total versus agreed price: zero pending, a positive amount below price partial, equal to price paid. Refuse a total above agreed price without changing amounts/status. Example: ARS 5,000 cash + ARS 15,000 transfer against ARS 20,000 is paid; ARS 22,000 is refused. | §8; AC-14, AC-34 |
| BE-PAY-03 | Preserve separation of agreed price, recorded money, availability, and reservation state. Payment edits do not confirm/cancel reservations, cancellation does not erase payment data, and the platform neither verifies nor moves money. Enforce establishment access isolation, including refusal of administrator reads. | §§3, 6, 8–9; AC-14, AC-19, AC-34 |

### 4.7 Suspension and reactivation

| ID | Requirement and acceptance condition | MVP source |
| --- | --- | --- |
| BE-SUS-01 | Let the platform administrator suspend/enable an entire business or individual branch without gaining access to internal customer/reservation/payment data. A branch operates only when both its own state and its business state allow it; re-enabling one does not clear the other's suspension. | §§3, 9; AC-19, AC-20, AC-39 |
| BE-SUS-02 | Suspension hides affected public inventory, denies scoped staff operational access, cancels future confirmed reservations, and closes pending requests as unavailable with visible suspension reason. Release pending counts, preserve customer history and already-started reservation state (including ended slots without attendance), and do not process refunds. Unaffected authorized contexts remain operational. | §9; AC-20, AC-38 |
| BE-SUS-03 | On reactivation, prior available future offers may return, but suspension-cancelled reservations and closed requests do not revive. Slots whose reservations were cancelled by suspension require explicit manual reopening. Preserve pre-existing staff blocks and other suspensions. | §9; AC-21, AC-33, AC-39 |

### 4.8 Public discovery and location

| ID | Requirement and acceptance condition | MVP source |
| --- | --- | --- |
| BE-DIS-01 | Expose public branch information and offered availability without sign-in, with city, sport, date, and time filtering. Return only publicly enabled scope and its public information; never disclose reservation-holder identity, DNI, internal customer contacts, or payments. | §§4, 10; AC-23, AC-51 |
| BE-DIS-02 | Accept coordinates or a manually resolved reference address for proximity search. Use initial radius 5 km, configurable from 1 to 50 km, without automatically widening an empty search. Order matching results by straight-line distance by default; offer ascending lowest-price-of-matching-slots ordering. Do not substitute travel time or price from nonmatching slots. | §10; AC-50 |
| BE-DIS-03 | Use Google Maps for map/address services and branch-coordinate proposal/confirmation. Support city search without device coordinates. Backend operations must not require device permission to browse; permission prompts and map rendering belong to the frontend. | §§4, 10; AC-41, AC-50, AC-52 |
| BE-DIS-04 | When map/address services fail or quota prevents use, retain filtered city/list results. Compute proximity when usable coordinates already exist; a new unresolved reference address waits for the provider. Do not fabricate coordinates or bypass usage terms. | §10; AC-51 |
| BE-DIS-05 | Interpret slot dates/times, cutoffs, and batch-date horizons in `America/Argentina/Buenos_Aires`, independently of client device timezone. Expose unambiguous temporal information so consumers can display the agreed local time. | §§6, 10; AC-09, AC-28, AC-29, AC-32, AC-51 |

### 4.9 WhatsApp reservation notices and integration operation

| ID | Requirement and acceptance condition | MVP source |
| --- | --- | --- |
| BE-NOT-01 | Record explicit, separate acceptance/refusal/revocation for reservation notices. Refusal/revocation does not prevent booking or in-app history; dispatch and retries respect current consent. No automatic notices go to manual-reservation guests. | §10; AC-13, AC-48 |
| BE-NOT-02 | Generate only the selected transactional notices: confirmation, rejection, unavailability, and cancellation. Include status, branch, court/sport, date/time, a public reason where applicable, and a sign-in-protected link. Exclude DNI, payment data, internal notes, receipt notices, expiry notices, reminders, and promotions. | §10; AC-48 |
| BE-NOT-03 | Use Evolution API with Baileys/WhatsApp Web and a single dedicated CanchaLibre number for verification codes and reservation notices. Only platform operators manage its connection/session. Do not introduce SMS or official Cloud API as an assumed fallback. | §§7, 10; AC-46, AC-52 |
| BE-NOT-04 | Keep reservation outcomes independent of notice delivery. Expose failures, a comprehensible reason, and contact phone in reservation detail and a failure list limited to staff's authorized branches, without granting administrator operational reads. | §10; AC-22, AC-49 |
| BE-NOT-05 | Allow at most two retries within 15 minutes for transient notice failures known not to have been accepted. Do not retry permanent rejection; check acceptance state before resending an uncertain result to avoid duplicates. Respect consent and budget on every attempt. | §10; AC-49 |
| BE-NOT-06 | Plan for 2,000–5,000 notices/month without treating that estimate as guaranteed capacity. Accept Evolution session disconnection/number restriction as an operational risk, with the fallback behavior in BE-ID-04 and BE-NOT-04. Do not claim zero total hosting/maintenance cost. | §§7, 10; AC-47, AC-52 |
| BE-NOT-07 | Keep additional external-service spending within the final agreed USD 50/month budget, excluding the existing server. Validate selected Google APIs/quotas/cost controls; alerts alone are not a cutoff. No automatic budget increase, purchase, billing activation, or provider replacement is authorized by this PRD. | §§10, 12; AC-52 |

## 5. Cross-cutting backend acceptance requirements

These are engineering obligations for delivering the product rules above, not additional customer features or new commercial policies.

| ID | Requirement and evidence needed | Source |
| --- | --- | --- |
| BE-ENG-01 | Enforce enabled scope, actor permissions, membership, and own-record checks server-side. Work/customer separation and revocation must hold regardless of client-supplied identifiers. Do not trust a frontend guard to authorize a request. | MVP §3; AC-18, AC-19, AC-26, AC-42 |
| BE-ENG-02 | Make all state-changing workflows preserve invariants under concurrency: offer exclusion, whole-batch rejection, customer/branch/global pending limits, confirmation/court capacity, cross-branch customer overlap, verification limits, and suspension versus new operations. A sequential read-then-write check alone is not proof. Keep pending requests non-exclusive. | MVP §§4–7, 9; AC-03, AC-05, AC-07, AC-30, AC-37, AC-46 |
| BE-ENG-03 | Refused operations preserve stored business state and relevant counts. Multi-effect operations must not expose a winner without closing competing requests, a partial conflicting batch, or suspended scope accepting new operations. Document and test the consistency boundary before implementation. | MVP §§4–9; AC-05, AC-18, AC-27, AC-30, AC-34, AC-38 |
| BE-ENG-04 | Make start-time expiration, verification-code expiry, bounded notice retries, and post-closure DNI deletion effective at their agreed boundaries. Scheduled processing must not let a stale pending record bypass or incorrectly exhaust eligibility. Define restart/recovery behavior during technical design without adding automatic attendance or revived requests. | MVP §§6–7, 10; AC-08, AC-28, AC-45, AC-46, AC-49 |
| BE-ENG-05 | Keep private data and secrets out of public responses, unscoped staff queries, logs, and notice payloads. Protect DNI fingerprints, phone verification, provider credentials, sessions, and support actions; record only the reviews/interventions required by the MVP. Do not retain full recoverable DNI, document images, or recordings. | MVP §§3, 7, 10, 12; AC-16, AC-19, AC-23, AC-44–AC-49 |
| BE-ENG-06 | Specify typed backend request/response DTO structs and stable business rejection meanings mapped to standard HTTP status codes (400, 403, 404, 409, 422, 500) using RFC 7807 Problem Details without exposing SQL/provider internals. Produce a machine-readable OpenAPI 3.1 contract for implemented operations once generator toolchain compatibility is validated. Concrete endpoint paths and pagination shapes are slice-level design work. | Existing HTTP baseline; `backend-go-patterns`, `api-contracts-openapi`; MVP authorization and failure outcomes |
| BE-ENG-07 | Enforce per-bounded-context hexagonal boundaries (`internal/modules/{context}/core/{domain,ports,usecase}` and `adapters/{inbound,outbound}`) with schema-per-context PostgreSQL isolation (`schema_{context}`), reversible Goose migrations, external integrations behind outbound ports, and inter-context calls strictly through public inbound ports wired at `cmd/api`. The existing flat files in `core/internal/` are bootstrap scaffolding to be replaced as bounded contexts are introduced. | MVP §14; `modular-hexagonal-architecture`, `backend-go-patterns`, `database-postgres-patterns` |
| BE-ENG-08 | Enforce `context.Context` propagation, bounded in-process worker pools (with PostgreSQL persistence for durable timers/retries), `pgxpool` limits (`MaxConns: 15–20`, `MinConns: 2`, `MaxConnLifetime: 1h`, PostgreSQL `max_connections = 50`), and Go runtime flags (`GOMAXPROCS=2`, soft `GOMEMLIMIT=150MiB` paired with hard container limits). Measure and recalculate combined container memory with Next.js standalone and Evolution/Baileys co-hosting on the 2 vCPU / 8 GB VPS before deployment. | MVP §§10, 12; `backend-go-patterns`, `server-infrastructure`, `golang-concurrency` |

## 6. MVP acceptance traceability

Every source acceptance example has at least one backend requirement. This is a coverage index, not a claim that tests have run or an endpoint contract has been approved.

| MVP example | Backend requirements |
| --- | --- |
| AC-01 | BE-OFF-01, BE-OFF-06 |
| AC-02 | BE-OFF-01, BE-OFF-03 |
| AC-03 | BE-ORG-04, BE-OFF-02, BE-ENG-02 |
| AC-04 | BE-OFF-06, BE-RES-01 |
| AC-05 | BE-RES-04, BE-RES-05, BE-ENG-02, BE-ENG-03 |
| AC-06 | BE-OFF-09, BE-RES-07, BE-RES-12 |
| AC-07 | BE-RES-02, BE-ENG-02 |
| AC-08 | BE-OFF-06, BE-RES-07, BE-ENG-04 |
| AC-09 | BE-RES-10, BE-DIS-05 |
| AC-10 | BE-OFF-07 |
| AC-11 | BE-OFF-08 |
| AC-12 | BE-OFF-08 |
| AC-13 | BE-RES-05, BE-RES-08, BE-NOT-01 |
| AC-14 | BE-OFF-06, BE-PAY-01, BE-PAY-02, BE-PAY-03 |
| AC-15 | BE-ACC-01, BE-ACC-08, BE-RES-01, BE-RES-12 |
| AC-16 | BE-ACC-08, BE-ID-06, BE-ENG-05 |
| AC-17 | BE-ACC-09 |
| AC-18 | BE-ACC-06, BE-ACC-07, BE-ACC-09, BE-ORG-04, BE-ENG-01 |
| AC-19 | BE-ACC-07, BE-PAY-03, BE-SUS-01, BE-ENG-05 |
| AC-20 | BE-RES-12, BE-SUS-01, BE-SUS-02 |
| AC-21 | BE-OFF-09, BE-SUS-03 |
| AC-22 | BE-RES-12, BE-NOT-04 |
| AC-23 | BE-DIS-01, BE-ENG-05 |
| AC-24 | BE-ACC-01, BE-ACC-08, BE-ID-01, BE-ID-04 |
| AC-25 | BE-ORG-02 |
| AC-26 | BE-ACC-06, BE-ACC-07, BE-ORG-01, BE-ENG-01 |
| AC-27 | BE-ORG-05 |
| AC-28 | BE-RES-03, BE-RES-04, BE-RES-07, BE-DIS-05 |
| AC-29 | BE-RES-03, BE-RES-10, BE-DIS-05 |
| AC-30 | BE-OFF-02, BE-OFF-03, BE-ENG-03 |
| AC-31 | BE-OFF-04 |
| AC-32 | BE-OFF-05, BE-DIS-05 |
| AC-33 | BE-OFF-09, BE-SUS-03 |
| AC-34 | BE-PAY-01, BE-PAY-02, BE-PAY-03 |
| AC-35 | BE-RES-11 |
| AC-36 | BE-RES-09, BE-RES-12 |
| AC-37 | BE-RES-06, BE-ENG-02 |
| AC-38 | BE-SUS-02, BE-ENG-03 |
| AC-39 | BE-SUS-01, BE-SUS-03 |
| AC-40 | BE-ORG-06, BE-ORG-07 |
| AC-41 | BE-ORG-02, BE-ORG-03, BE-DIS-03 |
| AC-42 | BE-ACC-02, BE-ACC-03, BE-ENG-01 |
| AC-43 | BE-ACC-04, BE-ACC-05 |
| AC-44 | BE-ID-05, BE-ID-06, BE-ID-07 |
| AC-45 | BE-ID-07, BE-ID-08, BE-ENG-04 |
| AC-46 | BE-ID-01, BE-ID-02, BE-ID-03, BE-NOT-03 |
| AC-47 | BE-ID-04, BE-NOT-06 |
| AC-48 | BE-ID-03, BE-NOT-01, BE-NOT-02 |
| AC-49 | BE-NOT-04, BE-NOT-05, BE-ENG-04 |
| AC-50 | BE-DIS-02, BE-DIS-03 |
| AC-51 | BE-DIS-01, BE-DIS-04, BE-DIS-05 |
| AC-52 | BE-DIS-03, BE-NOT-03, BE-NOT-06, BE-NOT-07, BE-ENG-08 |

## 7. Technical prerequisites and unresolved detail

Project skills already settle the bounded-context hexagonal layout, schema-per-context isolation, four-actor authorization rules, RFC 7807 error contracts, connection pool bounds, and single-node VPS baseline (see section 2). The items below are the remaining slice-level technical design decisions. Resolve only what the selected implementation slice needs; if a detail changes observable product behavior, ask for a product decision first.

| Area | Required design clarification |
| --- | --- |
| Bounded-context map and cross-context consistency | Skills fix the per-context hexagonal structure (`internal/modules/{context}`) and forbid cross-schema joins/foreign keys. Before scaffolding, define the concrete bounded-context map (which cohesive aggregates live together) and how multi-effect workflows (e.g., confirming a reservation while closing competing requests, or branch suspension cascades) maintain consistency within or across contexts. |
| Auth token and branch-context transport | `auth-security-patterns` fixes the four actors (`ADMIN`, `OWNER`, `EMPLOYEE`, `PLAYER`), independent work/customer accounts, and required authority checks (role + membership + enabled scope + own record). Define the concrete JWT claim layout, token TTL/refresh lifecycle, and how an authenticated staff request selects its active branch context (e.g., path/header/claim validated against active memberships). |
| Concrete table DDL and exclusion predicates | `database-postgres-patterns` mandates `schema_{context}`, Goose migrations, and `btree_gist` / `tstzrange` guards. Design the concrete tables and state predicates so physical court offers do not overlap, confirmed reservations are mutually exclusive per court and per customer time range across branches, and pending requests remain non-exclusive. |
| OpenAPI 3.1 generator toolchain | `api-contracts-openapi` sets OpenAPI 3.1 as the target and marks `swaggo/swag` (which natively targets Swagger 2.0) as a blocking toolchain prerequisite. Validate the generator version/output or agree on the OpenAPI 3.1 toolchain before enabling automated contract and TypeScript type generation. |
| Sign-in, linking, and invitation delivery | Define OAuth/OIDC callback contracts for Google/Apple, explicit account-linking verification flows, and the email provider/templates for work-account invitations and password recovery. The MVP excludes reservation-notification email, not work activation/recovery email. |
| DNI fingerprint and support procedure | Choose the keyed cryptographic fingerprint mechanism (e.g., HMAC-SHA256 with secret key management), stored suffix length, privileged support command/endpoint contract, audit record schema, and effective account-closure trigger for the 30-day retention clock. |
| Monetary representation and boundary validation | Define exact ARS representation in Go and PostgreSQL (e.g., integer pesos or fixed-scale numeric), permitted price ranges, and adjacent slot interval semantics (`[start, end)`). Do not add a refund ledger, discounts, grace periods, or complex pricing rules. |
| Batch, discovery, and proximity query shape | Define batch request ceilings, pagination/filtering parameters for public discovery, history, and failure lists, and the PostgreSQL/Go distance calculation method for 1–50 km straight-line proximity search. |
| Public photos and Google Maps coordinate terms | Define photo URL/storage handling and validate Google Maps API selection, quota caps, and permitted coordinate caching/confirmation terms within the USD 50/month additional budget. |
| Evolution API protocol and container sizing | Define the Evolution API (Baileys) REST/webhook contract, durable retry scheduling in PostgreSQL (max 2 retries within 15 minutes), and empirically measure combined container memory (`mem_limit`) for Go, PostgreSQL, Next.js standalone, Evolution, and proxy on the 8 GB VPS. |

## 8. Verification and readiness

### Backend acceptance evidence required during implementation

1. Test domain invariants and use-case authorization/lifecycle outcomes, including exact cutoffs, expiry, DNI normalization, accumulated payments, and refused operations leaving data/counts unchanged.
2. Verify backend public contracts through HTTP/command interfaces as applicable, including valid outputs, stable rejection semantics, scoped read access, and no sensitive-data disclosure.
3. Exercise PostgreSQL integration and concurrent operations against isolated test data: court offer collisions, whole-batch rejection, competing confirmations, cross-branch customer overlap, pending limits, and verification send/attempt limits. A race-detector pass alone does not prove database invariants.
4. Exercise provider adapters with controlled success, permanent failure, transient failure, uncertain acceptance, revocation before retry, and outage cases. Use controlled time for expiry and retry tests; do not send real customer messages or create provider charges as an implicit test prerequisite.
5. Follow the project backend testing convention (`go test -race` for Go tests), preserve existing health behavior, and verify any selected migration/contract workflow when it is implemented. This documentation task does not claim those checks have already passed.
6. Review all AC-01–AC-52 backend obligations via section 6 before declaring the backend MVP complete. Device permission prompts and visual rendering remain separate frontend acceptance work.

### Required before production operation

- Legal review of the agreed DNI support/retention policy, including 30-day post-closure deletion of fingerprint/final digits.
- Authorized provisioning of Google credentials/billing controls and the dedicated WhatsApp number/session, with secrets kept out of repository/chat/logs.
- Evidence of Google coverage, applicable API usage terms, quota/cost controls within USD 50/month additional spend, and allowed coordinate retention. A budget alert alone is insufficient.
- Evidence that the existing server can host Evolution safely with the backend/database, plus monitoring and operational recovery for channel disconnection or number restriction.
- A defined authorized support process for phone verification/DNI review and for cases where support is unavailable; do not promise an unagreed response time.

## 9. Next step

Agree a backend implementation slice using the `BE-*` identifiers, resolve its necessary design details, then plan its schema/contracts and acceptance checks. This PRD is a requirements inventory, not a migration plan, delivery schedule, endpoint specification, or instruction to begin implementation.
