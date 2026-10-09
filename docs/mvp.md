# CanchaLibre MVP — Product scope and business rules

**Status:** First product draft based on the agreed MVP decisions. This document describes intended behavior, not implemented functionality. Open decisions are listed in [section 12](#12-open-decisions).

CanchaLibre helps court owners offer rental slots, manage branch operations, and approve customer reservations. Owners are the primary customers: a trustworthy agenda and protection against abusive booking requests take priority over automatic booking acceptance.

## 1. MVP at a glance

| Area | Agreed scope |
| --- | --- |
| Market | Argentina; prices and recorded amounts in Argentine pesos (ARS). |
| Organization | Businesses with one or more branches, multiple co-owners, and branch-specific employee access. |
| Inventory | Physical courts supporting one or more sports; offered slots created individually or in batches. |
| Booking | Customer requests require establishment approval; pending requests never occupy a slot. |
| Payments | Staff manually record status, amount paid, and method. The platform does not move money. |
| Customer access | Google, Apple, and optional email/password access; complete profile and SMS-verified phone before requesting reservations. |
| Discovery | Public search, availability, branch details, map, and proximity search. |
| Notices | In-application status and transactional WhatsApp notices, subject to customer acceptance. |
| Identity | Unique self-declared customer DNI, with a private support route for conflicts. No verified-identity claim. |

### Primary customer journey

1. The platform administrator enables a business and assigns its co-owners.
2. An owner requests additional branches; the platform approves them before public operation.
3. Owners configure courts and branch rules, and invite employees.
4. An owner or employee creates offered slots. These are available, not reserved.
5. A customer browses publicly, signs in, completes the required profile, verifies their phone, and submits a request.
6. Branch staff confirm or reject the request. Only confirmation occupies the slot.
7. Staff manage attendance and manually record payment information; the customer sees the reservation outcome and history.

## 2. Shared vocabulary

| Concept | Meaning |
| --- | --- |
| Business / establishment | The business grouping managed by its co-owners. It contains one or more branches. |
| Branch | A physical location with its own address, courts, employees, agenda, cancellation rules, and client restrictions. Even a single-location business has a branch. |
| Court | A physical rental resource within a branch, with one or more supported sports. |
| Offered slot (`turno`) | A service offered for a particular court, sport, date, start/end time, and price. Creating it does not assign a customer or occupy the court. |
| Reservation request | A customer's pending request for an offered slot. It does not block availability. |
| Reservation | The request/allocation lifecycle, including approval, rejection, cancellation, and attendance outcomes. |
| Availability block | A staff action that makes a slot unavailable without creating a customer reservation. |
| Payment record | A manual description of money handled outside the platform, not a payment transaction processed by CanchaLibre. |

**Example:** Creating a padel slot on Court 1 for Friday, 18:00–19:00, at ARS 20,000 offers that service. A pending customer request leaves it available. A confirmed reservation occupies it.

## 3. Roles, permissions, and isolation

| Capability | Platform administrator | Business co-owner | Branch employee | Customer |
| --- | --- | --- | --- | --- |
| Create/manage business onboarding and public commercial information | Yes | Within owner-managed business information; exact editable fields pending | No | No |
| Assign/revoke co-owners | Yes | No | No | No |
| Approve new branches | Yes | Requests approval | No | No |
| Suspend/enable a business or branch | Yes | No platform suspension authority | No | No |
| Manage branch courts, supported sports, and base prices | No | Yes | No | No |
| Configure branch request/cancellation lead times | No | Yes | No | No |
| Invite/manage employees and their branch access | No | Yes | No | No |
| Create offered slots and block availability | No | Yes | Yes, assigned branches only | No |
| Confirm/reject/cancel reservations | No | Yes | Yes, assigned branches only | Own cancellation within the allowed cutoff |
| Record payments and attendance | No | Yes | Yes, assigned branches only | No |
| Block/unblock a customer locally, with a reason | No | Yes | No | No |
| Browse public offers and request a reservation | Public browsing only; work and customer accounts remain separate | Via a separate customer account | Via a separate customer account | Yes, subject to eligibility |
| View internal reservations, customers, or payments | **No** | Own business only | Assigned branches only | Own reservations/history only |

### Membership rules

- A business may have multiple co-owners with equal access to its branches. Only the platform administrator assigns or revokes them.
- An owner may manage multiple branches of their business, including several in the same city. Ownership across separate businesses is not specified in this draft.
- Employees may belong to multiple branches. Staff must operate in an explicitly selected authorized context.
- Revoking access removes authority for that context, not another independently authorized branch.
- Customer and work accounts are separate. Customer DNI uniqueness applies to customer profiles, not to work accounts.
- Permissions must be enforced by the backend, not just by hidden interface elements.
- A platform administrator cannot inspect operational data while suspending a business. System-driven cancellation does not grant administrator access to reservation details.

## 4. Businesses, branches, and courts

### Branch onboarding and public information

Owners request new branches. A branch cannot publicly offer rentals before platform approval.

The public branch profile includes address, contact information, photos, courts, sports, prices, offered slots, availability, and cancellation rules. Courts, employees, and schedules are managed independently per branch.

### Court scheduling

- A physical court may support multiple sports; each offered slot specifies its sport.
- All sports share the same physical court availability. Changing the sport does not create independent capacity.
- Each court has a default slot duration and base price. Staff can override duration and price when creating slots.
- Staff can create one slot or generate a batch from a court, dates, time range, and duration.
- Offered slots must not overlap for the same physical court, including across different sports.
- Automatic ongoing weekly recurrence is not included in the agreed creation flow.
- Rules for a batch that contains conflicts or incomplete final intervals remain open; no all-or-nothing behavior is assumed.

## 5. Offered slots and availability

| Availability | Meaning |
| --- | --- |
| Available | The offered service can receive a request, subject to branch lead-time rules and customer eligibility. |
| Reserved / occupied | An active confirmed reservation occupies the slot. Staff do not independently toggle it to free while that reservation remains active. |
| Blocked | Staff have withdrawn availability, for example for maintenance or closure. No customer reservation is required. |

Availability is separate from reservation and payment status. Reaching a request cutoff or slot start can make an otherwise unoccupied offer no longer requestable; it does not make it occupied.

### Editing and withdrawal

- Court, date, start/end time, and price can be edited only without pending requests or a confirmed reservation.
- If there are affected requests or a confirmed reservation, staff first cancel them with a reason and then offer a new slot. Silent reprogramming is not part of the MVP.
- A request preserves the price presented to and accepted by the customer.
- Blocking a slot with pending requests closes those requests for lack of availability.
- Blocking a slot with a confirmed reservation is refused until staff explicitly cancel that reservation with a reason.
- Cancelling a confirmed reservation normally makes the slot available again, unless it is blocked or affected by another restriction. Suspension has the special reopening policy described in section 9.
- Removing an offer must not erase customer reservation history. Exact archival controls for courts and offers remain open.

## 6. Reservation lifecycle

| State / outcome | Meaning |
| --- | --- |
| Pending approval | Submitted by the customer; awaiting branch staff. Does not occupy the slot. |
| Confirmed | Accepted by staff; occupies the slot. May still be unpaid. |
| Rejected | Staff declined the request. |
| Unavailable | Another request was confirmed or the offer was withdrawn; the reason is visible to the customer. |
| Cancelled | A customer, staff member, or suspension action ended the request/reservation as applicable. |
| Expired | An unanswered pending request reached the slot start time. |
| Completed | The confirmed service was used. |
| No-show | The customer did not attend. |

These are product meanings, not prescribed storage enum names. Precise attendance transition timing and the presentation of a withdrawn pending request remain open.

### Customer request and approval

- Customers request an existing offered slot; they do not create the offered inventory.
- Each branch configures the minimum advance time for customer requests.
- Staff may create manual reservations before slot start. No new reservation is created for an already-started slot.
- A pending request expires at slot start if not resolved earlier.
- Staff confirmation must verify that the slot remains enabled and unoccupied. Concurrent attempts cannot produce two confirmed reservations for the same physical court/time.
- Confirmation closes every other pending request for that slot as unavailable, with a visible reason.
- Those requests do not automatically reactivate if the winning reservation is later cancelled. There is no waiting list in the MVP.
- Customers must be told that submitting a request does not reserve the slot; only establishment confirmation does.
- Whether request lead time also constrains approval of an already-pending request is an open decision.

### Manual reservations

Owners and employees can directly create a confirmed reservation against an available offered slot for a guest without a platform account.

Guest contact requires name and phone, is private to the establishment context, and does not require customer-account DNI or SMS verification. Creating this reservation is distinct from creating the offered slot.

### Cancellation and attendance

- Each branch configures how far in advance a customer may cancel a confirmed reservation in the application.
- After that cutoff, the customer must contact the branch; staff handle any cancellation with a reason.
- Customers can withdraw pending requests; they are not committed reservations.
- Owners and employees record completion or no-show. Employees can alert owners about abuse but cannot block customers themselves.
- Cancellation does not move money or silently remove recorded payment information.

## 7. Customer access and abuse protection

### Authentication and booking eligibility

- Customer access includes Google and Apple, plus optional local email/password credentials and account recovery/linking.
- Social sign-in does not require the customer to create a password.
- Before requesting a reservation, a customer provides first name, last name, phone, and DNI, and verifies the phone by SMS.
- A complete profile with a verified phone is **not** verified identity. The DNI is self-declared in this MVP.
- Staff and platform administrator access uses invitation-based email/password activation and recovery; there is no public signup for work roles.
- Exact local-password enrollment and separate-account identifiers remain to be defined.

### Request limits and local restrictions

- At most **one pending request per customer per branch**.
- At most **five pending requests per customer across the platform**.
- Duplicate pending requests from the same customer for the same slot are forbidden.
- Owners can block and unblock new customer requests for a branch, recording a reason.
- A local block does not automatically cancel existing reservations or apply to other branches.
- Manual approval, request limits, and verified contact reduce abuse; none guarantees that a customer will attend or that their identity is genuine.

### Duplicate DNI handling

A DNI already associated with another customer account prevents booking enablement using that document. The customer receives a recovery/support route without disclosure of the other account's information.

Conflicts are handled through restricted support outside the MVP interface. A self-declared DNI can be claimed by the wrong person: the support process must establish legitimacy before reassignment, and must not expose branch operational data. Document normalization, safe retention, correction, and review procedures remain open.

## 8. Manual payment information

For a reservation, staff record:

- Status: **pending**, **partial**, or **paid**.
- Total amount paid, in ARS.
- Method: **cash**, **bank transfer**, **card**, or **other**.

The customer-agreed reservation price remains distinct from the amount paid. Status and amounts describe payments verified by establishment staff, not by CanchaLibre.

There is no checkout, payment gateway, automatic collection, automatic refund, or individual payment/refund movement ledger. Multiple-method representation, overpayments, and validation between status and amount remain open rather than assumed.

## 9. Suspension and reactivation

The platform administrator can suspend either a whole business or an individual branch.

### Suspension effects

1. The affected business/branch disappears from public discovery and its future offered slots are hidden.
2. Owners and employees lose operational access to the affected scope. Other independently enabled contexts remain accessible.
3. Future confirmed reservations are cancelled; affected pending requests can no longer be fulfilled. Their exact closure outcome is listed as open.
4. Customers retain their reservation history, including cancellation results. Hiding public inventory must not hide or delete a customer's history.
5. Suspension does not process payments or refunds and does not let the administrator inspect internal data.

### Reactivation effects

- Staff regain access to the enabled scope.
- Future offers that were available before suspension can become visible again.
- Cancelled reservations never revive automatically.
- Slots that held reservations cancelled by suspension require manual reopening.
- Existing staff blocks must not be silently removed.
- Interaction between business-level suspension and independently suspended branches, and treatment of already-started reservations, require explicit decisions.

## 10. Discovery and notifications

### Public discovery

No sign-in is required to browse enabled branches and availability. The MVP includes city, sport, date/time filtering, a map, and proximity search, plus the public branch profile described in section 4.

Map/location provider, radius/ranking behavior, consent, and a fallback when location is unavailable remain open. Public discovery must not expose reservation-holder identity, DNI, or internal payment information. Reviews are outside the agreed scope.

### Customer and staff notices

- Customers can inspect request/reservation status and history inside the application.
- With customer acceptance, WhatsApp sends transactional confirmation, rejection, unavailability, and cancellation notices.
- Promotional messages, request-receipt messages, expiry messages, and reminders were not selected.
- SMS is for phone verification, not reservation notices. No reservation-notification email fallback was selected.
- A WhatsApp delivery failure never undoes a confirmation or cancellation. Staff see the failure and can contact the customer outside the system.
- Provider selection, delivery/retry policy, acceptance handling, and communication to guests without accounts remain open.

## 11. Acceptance examples

These examples are **future acceptance criteria**, not claims that tests or features currently exist. Configuration values and prices below are illustrative, not product defaults.

| ID | Scenario | Expected result |
| --- | --- | --- |
| AC-01 | An employee creates an offered slot for Court 1, Friday 18:00–19:00, padel, ARS 20,000. | An available offered slot exists; no customer, reservation, or payment is created. |
| AC-02 | Staff generate seven days of hourly slots for one court. | Slots are offered, not occupied. Each respects the chosen court, interval, duration, and price. |
| AC-03 | A basketball offer overlaps a football offer on the same physical court. | The conflicting offer is not created; changing sports does not bypass capacity. |
| AC-04 | Two eligible customers request the same available slot. | Both may await approval; neither holds the slot. |
| AC-05 | Two employees try to confirm different requests for the same slot concurrently. | Exactly one reservation is confirmed. Other pending requests close as unavailable; no double-booking occurs. |
| AC-06 | The confirmed reservation is cancelled. | The slot becomes available unless restricted; previously closed requests do not reactivate. |
| AC-07 | A customer already has a pending request in Branch A. | A second pending request in A is refused. A request in B is possible only if below the five-global-pending limit. |
| AC-08 | A pending request is unanswered when its slot starts. | It expires and no reservation is created. |
| AC-09 | A branch requires cancellation at least four hours before a 20:00 turn. | A 15:00 cancellation is permitted; a 17:00 customer cancellation is refused and directs them to the branch. Exact cutoff equality remains open. |
| AC-10 | Staff attempt to change the time or price of a slot with a pending request or confirmed reservation. | The edit is refused; the existing agreement is unchanged. Staff must resolve it and offer another slot. |
| AC-11 | An employee tries to block a slot with a confirmed reservation. | Blocking is refused until explicit cancellation with a reason. |
| AC-12 | Staff block a slot with only pending requests. | The slot is blocked and its requests close as unavailable. |
| AC-13 | Staff reserve an available slot for a walk-in, with name and phone. | A confirmed guest reservation is created; no platform customer account, DNI, or SMS is required. |
| AC-14 | Staff record partial payment of ARS 5,000 against ARS 20,000, by cash. | Those manual details are stored; no payment is processed and the reservation state is unchanged. |
| AC-15 | A customer has a complete profile but an unverified phone. | They can browse but cannot submit a reservation request. |
| AC-16 | Another customer account already declares the same DNI. | Booking enablement using that DNI is refused with a private recovery/review route; no other account details are disclosed. |
| AC-17 | An owner blocks a customer in Branch A. | New requests to A are refused; existing reservations and access to Branch B are not automatically affected. |
| AC-18 | An employee attempts to create a court, change branch rules, or block a customer. | Authorization is refused without changing data. |
| AC-19 | A platform administrator attempts to read internal branch reservations or payments. | Access is refused, despite the administrator's ability to suspend the branch. |
| AC-20 | The administrator suspends Branch A while Branch B remains enabled. | A is hidden, scoped staff access is denied, and A's future confirmed reservations are cancelled; customer history remains. B is unaffected. |
| AC-21 | Branch A is re-enabled. | Prior available future offers may return; cancelled reservations stay cancelled and their slots require manual reopening. |
| AC-22 | WhatsApp fails after staff confirm a reservation. | The reservation remains confirmed, the customer can see it in the application, and staff see the notice failure. |
| AC-23 | An anonymous visitor searches by city, sport, date, or map. | Only publicly enabled branches/offers are shown; internal customer/payment information is never exposed. |
| AC-24 | A customer uses Google or Apple successfully. | They are not forced to create a password; booking still requires profile completion and SMS verification. |
| AC-25 | An owner requests a new branch. | It cannot publicly offer rentals before platform approval. |
| AC-26 | An employee authorized for A requests internal data from unauthorized B. | Access is refused; sharing an account or city does not grant branch membership. |

## 12. Open decisions

The draft records confirmed scope without choosing these values or procedures. Resolve each before implementing the affected flow; an open decision is not a license to infer a requirement.

| ID | Decision to resolve |
| --- | --- |
| O-01 | SMS, WhatsApp, and map providers; budgets, credentials, delivery/retry limits, and operational failure handling. |
| O-02 | Defaults, permitted ranges, and exact boundary equality for request/cancellation lead times; whether approval is allowed inside the customer request cutoff. |
| O-03 | Batch conflict handling, partial batches, incomplete final intervals, maximum generation horizon, and how staff reopening relates to editing an existing offer. |
| O-04 | DNI normalization, correction, protected retention, and restricted support verification/reassignment procedure. |
| O-05 | Separate customer/work account identifiers, optional password enrollment/linking, first administrator provisioning, and loss of the last co-owner. |
| O-06 | Which public business/branch fields owners can edit versus platform-controlled fields; approval rejection and correction flow. |
| O-07 | Suspension of already-started reservations, pending-request outcome on suspension, and business/branch reactivation precedence. |
| O-08 | Court/business/branch archival and removal controls that preserve history; no destructive history deletion is specified. |
| O-09 | Validation of payment status versus amount, overpayments, and representation of more than one external payment method. |
| O-10 | WhatsApp acceptance/refusal or revocation, guest notices, notification failure visibility, and notice details sufficient for customers without exposing private information. |
| O-11 | Search radius/ranking, location permission and fallback, branch coordinates, and scheduling timezone/date presentation. |
| O-12 | Completion/no-show timing, withdrawal labels, and whether additional restrictions on confirmed bookings are needed beyond the agreed pending-request limits. |

## 13. Explicitly outside this MVP

- AI-assisted identity verification and DNI photo processing. For the future verification flow, document photos must not be retained; proof of identity and duplicate detection require separate design.
- Online payment processing, automated collections/refunds, and individual payment/refund ledgers.
- Automatic reservation acceptance, waitlists, or automatic revival of closed requests.
- Reprogramming that requires a customer's new acceptance.
- Automatically recurring weekly slot generation and complex pricing rules.
- Promotional messaging, automated reservation reminders, public reviews, and multi-country/multi-currency operation.

## 14. Delivery boundary and next step

This product draft does not prescribe a delivery order or authorize implementation. The existing technical baseline remains Go/Chi, Next.js, and PostgreSQL with modular boundaries; its current authentication guidance must be reconciled with this MVP's four roles, co-ownership, branch memberships, and separate accounts before implementing access control.

**Next step:** Review this document, resolve the open decisions for the first implementation slice, then agree on that slice's scope and acceptance checks. Maps, WhatsApp, SMS verification, and multiple branches remain part of the agreed MVP even if delivered in different slices.
