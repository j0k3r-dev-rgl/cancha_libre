# CanchaLibre MVP — Product scope and business rules

**Status:** Product scope updated with the twelve agreed decisions (O-01–O-12). This document describes intended behavior, not implemented functionality, and does not authorize implementation or service activation. [Section 12](#12-decision-register-and-pre-operation-checks) summarizes the decisions and the checks still required before operation.

CanchaLibre helps court owners offer rental slots, manage branch operations, and approve customer reservations. Owners are the primary customers: a trustworthy agenda and protection against abusive booking requests take priority over automatic booking acceptance.

## 1. MVP at a glance

| Area | Agreed scope |
| --- | --- |
| Market | Argentina; prices and recorded amounts in Argentine pesos (ARS). |
| Organization | Businesses with one or more branches, multiple co-owners, and branch-specific employee access. |
| Inventory | Physical courts supporting one or more sports; offered slots created individually or in batches. |
| Booking | Customer requests require establishment approval; pending requests never occupy a slot. |
| Payments | Staff record accumulated amounts by method; the total determines payment status. The platform does not move money. |
| Customer access | Google, Apple, and optional email/password access; complete profile and verified phone before requesting reservations. Verification uses WhatsApp or an exceptional support call, not SMS. |
| Discovery | Public search, availability, branch details, Google Maps, and proximity search. |
| Notices | In-application status and optional transactional WhatsApp notices through Evolution API / Baileys. |
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
| Create/manage business onboarding and public commercial information | Yes | Direct editing of public description, photos, contacts, name, address, and location; new branches require approval | No | No |
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

- A business may have multiple co-owners with equal access to its branches. Only the platform administrator assigns or revokes them. Revoking the last co-owner is refused until a replacement is assigned.
- An owner may manage multiple branches of their business, including several in the same city. The data model supports one work account belonging to more than one business, but the MVP restricts each work account to a single business while this restriction is enabled. This is a temporary product restriction, not a permanent rule: enabling multi-business work accounts requires revisiting this rule and its authorization checks, not a data migration.
- Employees may belong to multiple branches. Staff must operate in an explicitly selected authorized context.
- Revoking access removes authority for that context, not another independently authorized branch.
- Customer and work accounts are separate, even when they use the same email address. Sign-in distinguishes the two account contexts; credentials, recovery, and permissions remain independent. Sharing an email does not link them. Customer DNI uniqueness applies to customer profiles, not to work accounts.
- Permissions must be enforced by the backend, not just by hidden interface elements.
- A platform administrator cannot inspect operational data while suspending a business. System-driven cancellation does not grant administrator access to reservation details.

## 4. Businesses, branches, and courts

### Branch onboarding and public information

Owners request new branches. A branch cannot publicly offer rentals before platform approval. If rejected, the owner receives the reason and can correct and resubmit the same request, preserving its review history.

Owners directly edit the public description, photos, contact information, and names of their business and branches, including an approved branch's address and location. These edits do not require prior or renewed platform approval. Ownership, memberships, approval, and suspension remain subject to their separate permissions.

The public branch profile includes address, contact information, photos, courts, sports, prices, offered slots, availability, and cancellation rules. Courts, employees, and schedules are managed independently per branch. Before publication, the owner must confirm or correct the map point proposed from the address; a branch without confirmed coordinates cannot be published.

### Court scheduling

- A physical court may support multiple sports; each offered slot specifies its sport.
- All sports share the same physical court availability. Changing the sport does not create independent capacity.
- Each court has a default slot duration and base price. Staff can override duration and price when creating slots.
- Staff can create one slot or generate a batch from a court, dates, time range, and duration.
- Offered slots must not overlap for the same physical court, including across different sports.
- Automatic ongoing weekly recurrence is not included in the agreed creation flow.
- If any generated slot overlaps an existing offer on the same physical court, the entire batch is refused: no slots are created, existing offers are unchanged, and the conflicts are shown.
- Only complete duration intervals are generated. Before confirmation, staff see any final interval excluded because it is shorter than the chosen duration. For example, 18:00–22:30 with 60-minute slots produces four slots ending at 22:00 and explicitly excludes 22:00–22:30. This does not allow partial creation when a generated slot conflicts.
- Batch generation may offer dates up to 180 days from the current date. This is a future-date horizon, not a count of slots.

## 5. Offered slots and availability

| Availability | Meaning |
| --- | --- |
| Available | The offered service can receive a request, subject to branch lead-time rules and customer eligibility. |
| Reserved / occupied | An active confirmed reservation occupies the slot. Staff do not independently toggle it to free while that reservation remains active. |
| Blocked | Staff have withdrawn availability, for example for maintenance or closure. No customer reservation is required. |

Availability is separate from reservation and payment status. Reaching a request cutoff or slot start can make an otherwise unoccupied offer no longer requestable; it does not make it occupied.

### Editing and withdrawal

- Court, date, start/end time, and price can be edited only without pending requests or a confirmed reservation.
- If there are affected requests or a confirmed reservation, staff first close the requests or cancel the confirmed reservation with a reason, then offer a new slot. Silent reprogramming is not part of the MVP.
- A request preserves the price presented to and accepted by the customer.
- Blocking a slot with pending requests closes those requests for lack of availability.
- Blocking a slot with a confirmed reservation is refused until staff explicitly cancel that reservation with a reason.
- Cancelling a confirmed reservation normally makes the slot available again, unless it is blocked or affected by another restriction. Suspension has the special reopening policy described in section 9.
- Reopening is an explicit action on the same offer, without changing its court, date, time, or price. It requires an enabled branch, a slot that has not started, no confirmed reservation, and no other active restriction. Closed requests and cancelled reservations never revive.
- To change those details when reoffering withdrawn inventory, staff retire the old offer and create another, preserving history.
- Removing an offer must not erase customer reservation history. Offers are retired rather than permanently deleted.

### Archival and recovery

- Businesses, branches, and courts can be archived, not permanently deleted. Their history and references remain available within the existing access boundaries.
- Before archival, pending requests must be resolved and future confirmed reservations explicitly cancelled with a reason. Already-started reservations must finish and have their attendance outcome recorded. Archival is refused until these prerequisites are met; it does not silently cancel reservations.
- A user authorized to manage the entity may explicitly unarchive it. Existing approval requirements, suspensions, and other restrictions still apply; unarchiving does not revive closed requests or cancelled reservations.
- These inventory archival rules do not establish a personal-data retention policy. The agreed DNI retention rule is in section 7.

## 6. Reservation lifecycle

| State / outcome | Meaning |
| --- | --- |
| Pending approval | Submitted by the customer; awaiting branch staff. Does not occupy the slot. |
| Confirmed | Accepted by staff; occupies the slot. May still be unpaid. |
| Rejected | Staff declined the request. |
| Unavailable | Another request was confirmed or the offer was withdrawn; the reason is visible to the customer. |
| Cancelled | A customer, staff member, or suspension action ended a confirmed reservation as applicable. |
| Request withdrawn (`Solicitud retirada`) | The customer withdrew a pending request before confirmation; it remains in history and no longer counts as pending. |
| Expired | An unanswered pending request reached the slot start time. |
| Completed | The confirmed service was used. |
| No-show | The customer did not attend. |

These are product meanings, not prescribed storage enum names. Withdrawing an offer closes its pending requests as unavailable with a visible reason, rather than presenting them as customer-withdrawn requests.

### Customer request and approval

- Customers request an existing offered slot; they do not create the offered inventory.
- Each branch has an owner-editable request lead time, defaulting to **1 hour**. Request and cancellation lead times accept whole minutes from **0 to 7 days inclusive** (0–10,080 minutes).
- The exact cutoff is included: a request requiring four hours of advance time for a 20:00 slot is allowed at 16:00, but not after. Regardless of the configured value, including zero, requesting is never allowed at or after slot start.
- Staff may create manual reservations before slot start. No new reservation is created for an already-started slot.
- A pending request expires at slot start if not resolved earlier.
- Staff confirmation must verify that the slot remains enabled and unoccupied and that the customer has no overlapping confirmed reservation, including at another branch. Concurrent attempts cannot produce two confirmed reservations for the same physical court/time or overlapping confirmed reservations for the same customer.
- Confirmation closes every other pending request for that slot as unavailable, with a visible reason.
- Those requests do not automatically reactivate if the winning reservation is later cancelled. There is no waiting list in the MVP.
- Customers must be told that submitting a request does not reserve the slot; only establishment confirmation does.
- Staff may confirm a pending request received in time until strictly before slot start, even after the cutoff for new customer requests. At slot start, the pending request expires and cannot be confirmed.

### Manual reservations

Owners and employees can directly create a confirmed reservation against an available offered slot for a guest without a platform account.

Guest contact requires name and phone, is private to the establishment context, and does not require a platform customer account, DNI, or phone verification. Creating this reservation is distinct from creating the offered slot. Staff contact guests outside the platform; automatic WhatsApp notices are not sent to guests in the MVP.

### Cancellation and attendance

- Each branch has an owner-editable cancellation lead time, defaulting to **4 hours**, in the same whole-minute range of 0–7 days as request lead time. The exact cutoff is included: for a 20:00 slot with four hours required, 16:00 is allowed and later is refused. Cancellation by the customer is never allowed at or after slot start, including with a zero lead time.
- After that cutoff, the customer must contact the branch; staff handle any cancellation with a reason.
- Customers can withdraw pending requests; they are not committed reservations. The history label is **`Solicitud retirada`**, and the request stops counting toward pending limits.
- Owners and employees manually record **no-show from slot start** with no mandatory grace period, and **completed from slot end**. Neither attendance outcome is set automatically. Employees can alert owners about abuse but cannot block customers themselves.
- Cancellation does not move money or silently remove recorded payment information.

## 7. Customer access and abuse protection

### Authentication and booking eligibility

- Customer access includes Google and Apple, plus optional local email/password credentials and account recovery/linking.
- Social sign-in does not require the customer to create a password. A signed-in customer may add an optional password at any time, verifying the email used for sign-in and recovery.
- Linking sign-in methods requires an explicit action and verification of both accesses; matching email addresses alone never merges accounts.
- Before requesting a reservation, a customer provides first name, last name, phone, and DNI, and verifies control of the phone using the WhatsApp or exceptional support procedure below. SMS is not used.
- A complete profile with a verified phone is **not** verified identity. The DNI is self-declared in this MVP.
- Staff and platform administrator access uses invitation-based email/password activation and recovery; there is no public signup for work roles.
- An authorized operator provisions the first platform administrator through a private server-side command that sends a single-use activation invitation. There is no default password or public administrator signup.

### Phone verification and channel failure

- The normal flow sends a **six-digit code through WhatsApp**, valid for **5 minutes**, with at most **5 attempts per code**, at least **60 seconds between sends**, and at most **5 sends per phone per hour**, plus abuse controls.
- A new code invalidates the previous one. Codes are not logged and cannot be reused after successful verification.
- Requesting a verification code does not subscribe the customer to reservation notices; consent to those notices remains optional and revocable.
- When the WhatsApp channel is unavailable or restricted, or the customer does not have WhatsApp, restricted support may exceptionally verify control of the number through a call to that number and a code displayed in the customer's application session. The intervention is recorded; verification is not waived.
- If support is unavailable, the customer can browse but cannot request a reservation until verification is completed. No response-time commitment is made.
- A WhatsApp outage does not prevent already-eligible customers from requesting reservations or inspecting status/history in the application.

### Request limits and local restrictions

- At most **one pending request per customer per branch**.
- At most **five pending requests per customer across the platform**.
- Duplicate pending requests from the same customer for the same slot are forbidden.
- There is no additional count limit on confirmed reservations, but a customer cannot have confirmed reservations with overlapping times, even across different branches. This is separate from physical-court capacity and pending-request limits.
- Owners can block and unblock new customer requests for a branch, recording a reason.
- A local block does not automatically cancel existing reservations or apply to other branches.
- Manual approval, request limits, and verified contact reduce abuse; none guarantees that a customer will attend or that their identity is genuine.

### Duplicate DNI handling

A DNI already associated with another customer account prevents booking enablement using that document. The customer receives a recovery/support route without disclosure of the other account's information.

- Normalize the DNI before comparison by removing dots and spaces and ignoring leading zeroes. For example, `12.345.678`, `12345678`, and `012345678` identify the same declared document. Letters and other characters are rejected; normalization does not validate authenticity.
- Store only a protected uniqueness fingerprint and the final digits, not a recoverable full DNI. Support compares the number supplied by the person without retrieving the original number.
- Once booking eligibility has been enabled, DNI corrections require restricted support to assess legitimacy and duplicates and record the change.
- A duplicate claim prevents the claimant's booking enablement, but does not automatically restrict the existing account. Support may restrict it if evidence warrants doing so. The claim alone never establishes legitimacy or authorizes reassignment.
- Conflicts are handled through restricted support outside the MVP interface, without exposing the other account's information or branch operational data. If account recovery cannot resolve the conflict, support performs a private live human review comparing the person with their document before correction/reassignment. Knowing the DNI or verifying a phone is insufficient. Record the decision and rationale without retaining document photos, recordings, or automated identity-verification output.
- No interface, endpoint, or storage path exists for identity images. A document is inspected live by restricted support only; the platform never receives, uploads, transmits, stores, or later deletes a document photo, recording, or automated verification output. This is a hard prohibition, not a retention policy, and it must not be implemented "for later review".
- After effective account closure, retain the fingerprint and final digits for **30 days**, preventing reuse during that period, then delete them. This does not delete establishment reservation history. This product retention policy requires legal review before operation; it is not a claim of regulatory compliance.

## 8. Manual payment information

Staff record the accumulated amount received in ARS for each method: **cash**, **bank transfer**, **card**, and **other**. A reservation can combine methods without recording individual payment movements.

The sum determines the total and the payment status; staff do not choose an independent status:

| Total recorded against the agreed price | Calculated status |
| --- | --- |
| Zero | **Pending** |
| Greater than zero and less than the price | **Partial** |
| Equal to the price | **Paid** |
| Greater than the price | Refused; the recorded total cannot exceed the agreed price. |

For an ARS 20,000 reservation, ARS 5,000 cash is partial; ARS 5,000 cash plus ARS 15,000 bank transfer is paid. A total of ARS 22,000 is refused. Any excess money actually received is handled outside this record.

The customer-agreed reservation price remains distinct from the amount paid. Status and amounts describe payments verified by establishment staff, not by CanchaLibre. Updating payment information does not change the reservation state; cancellation does not silently erase it.

There is no checkout, payment gateway, automatic collection, automatic refund, or individual payment/refund movement ledger.

## 9. Suspension and reactivation

The platform administrator can suspend either a whole business or an individual branch.

### Suspension effects

1. The affected business/branch disappears from public discovery and its future offered slots are hidden.
2. Owners and employees lose operational access to the affected scope. Other independently enabled contexts remain accessible.
3. Future confirmed reservations are cancelled. Affected pending requests close as **unavailable**, with the visible reason **suspension**; they stop counting toward pending limits and never reactivate automatically.
4. Customers retain their reservation history, including cancellation results. Hiding public inventory must not hide or delete a customer's history.
5. Suspension does not process payments or refunds and does not let the administrator inspect internal data.
6. Confirmed reservations whose start time has already arrived retain their state, including ended slots without an attendance result. No cancellation or attendance outcome is inferred automatically. Staff still lose operational access while suspended.

### Reactivation effects

- Staff regain access to the enabled scope.
- Future offers that were available before suspension can become visible again.
- Cancelled reservations never revive automatically.
- Slots that held reservations cancelled by suspension require manual reopening.
- Existing staff blocks must not be silently removed.
- A branch can operate only when both the business and the branch are enabled. Their suspensions are independent: re-enabling a business does not clear a branch's own suspension, and re-enabling a branch does not override a suspended business.

## 10. Discovery and notifications

### Public discovery

No sign-in is required to browse enabled branches and availability. The MVP includes city, sport, date/time filtering, a map, and proximity search, plus the public branch profile described in section 4.

- **Google Maps** supplies maps and address lookup. Before publication, the owner confirms or corrects the branch's proposed coordinates.
- Request device location only when the visitor selects **Near me**. If permission is refused or location is unavailable, allow manual city selection or a reference address.
- Proximity search starts at **5 km**, adjustable from **1 to 50 km**. When there are no results, offer to widen the radius; do not widen it automatically.
- Results satisfying the selected sport/date/time filters default to straight-line distance from the reference point, not travel time. The visitor can switch to ascending price, using the lowest price of slots satisfying the filters.
- Display and interpret all slot dates, times, and lead-time cutoffs in **`America/Argentina/Buenos_Aires`**, clearly identified. Do not convert to the visitor's device timezone.
- If maps/address lookup fails or reaches its usable quota, preserve city-based search and filtered results as a list. Proximity still works when coordinates are already available, including device location or an already-resolved reference; a new reference address must wait for the provider.

Public discovery must not expose reservation-holder identity, DNI, or internal payment information. Reviews are outside the agreed scope.

### Customer and staff notices

- Customers can inspect request/reservation status and history inside the application.
- With explicit, separate, recorded customer acceptance, WhatsApp sends transactional confirmation, rejection, unavailability, and cancellation notices. Acceptance is optional and revocable; refusing or revoking it does not prevent reservations or access to status/history.
- Promotional messages, request-receipt messages, expiry messages, and reminders were not selected.
- No SMS or reservation-notification email fallback is included. Verification codes are separate from reservation notices and their optional consent.
- Notices include status, branch, court/sport, date, time, and a public-facing reason when applicable, plus a private link requiring sign-in. Do not include DNI, payment information, or internal notes.
- A WhatsApp delivery failure never undoes a confirmation or cancellation. Authorized owners/employees see a comprehensible failure reason and contact phone in both the reservation detail and a failure list restricted to their branches; they can contact the customer outside the system.
- Guests without accounts do not receive automatic notices; the establishment contacts them manually outside CanchaLibre.

### Providers, budget, and delivery operation

- WhatsApp uses **Evolution API with Baileys / WhatsApp Web**, not the official Cloud API. Codes and notices use one number dedicated to CanchaLibre, with the connection/session managed only by platform operators, not a personal or per-branch account.
- Plan to host Evolution on the existing server, subject to CPU, memory, and isolation checks. Baileys avoids official API per-message charges but does not make hosting/maintenance free or guarantee availability; session disconnection or number restriction remains an accepted operational risk.
- Planning volume is **2,000–5,000 reservation notices per month**. Verification demand is not yet known; do not infer a guaranteed free capacity from this estimate.
- For failed reservation notices, allow at most **two retries within 15 minutes**, only for transient failures known not to have been accepted. Do not retry permanent rejections. If acceptance is uncertain, check the delivery state before resending to avoid duplicates. Every retry respects consent and budget.
- The final agreed additional external-service budget is **up to USD 50 per month**, accepting possible Google Maps charges. This replaces the intermediate zero-additional-cost preference and excludes the existing server cost. It does not authorize a purchase, billing activation, or automatic budget increase.
- Before enabling services, validate the selected Google APIs, applicable free usage, quotas, billing controls, and usage/coordinate-retention terms. Budget alerts alone are not a spending cutoff. Secure credentials and the Evolution session without sharing secrets in chat. Concrete credentials and production configuration are pre-operation work, not supplied by this product document.

## 11. Acceptance examples

These examples are **future acceptance criteria**, not claims that tests or features currently exist. Prices and scenario-specific settings are illustrative; defaults and limits explicitly identified as such reflect the agreed product rules.

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
| AC-09 | A branch requires cancellation at least four hours before a 20:00 turn. | Customer cancellation at 15:00 or exactly 16:00 is permitted; after 16:00 it is refused and directs them to the branch. |
| AC-10 | Staff attempt to change the time or price of a slot with a pending request or confirmed reservation. | The edit is refused; the existing agreement is unchanged. Staff must resolve it and offer another slot. |
| AC-11 | An employee tries to block a slot with a confirmed reservation. | Blocking is refused until explicit cancellation with a reason. |
| AC-12 | Staff block a slot with only pending requests. | The slot is blocked and its requests close as unavailable. |
| AC-13 | Staff reserve an available slot for a walk-in, with name and phone. | A confirmed guest reservation is created; no platform customer account, DNI, or phone verification is required. No automatic WhatsApp notice is sent to the guest. |
| AC-14 | Staff record ARS 5,000 cash against an agreed price of ARS 20,000. | The accumulated cash amount and total are ARS 5,000, and the calculated payment status is partial. No payment is processed and the reservation state is unchanged. |
| AC-15 | A customer has a complete profile but an unverified phone. | They can browse but cannot submit a reservation request. |
| AC-16 | Another customer account already declares the same DNI. | Booking enablement using that DNI is refused with a private recovery/review route; no other account details are disclosed. |
| AC-17 | An owner blocks a customer in Branch A. | New requests to A are refused; existing reservations and access to Branch B are not automatically affected. |
| AC-18 | An employee attempts to create a court, change branch rules, or block a customer. | Authorization is refused without changing data. |
| AC-19 | A platform administrator attempts to read internal branch reservations or payments. | Access is refused, despite the administrator's ability to suspend the branch. |
| AC-20 | The administrator suspends Branch A while Branch B remains enabled. | A is hidden, scoped staff access is denied, and A's future confirmed reservations are cancelled; customer history remains. B is unaffected. |
| AC-21 | Branch A is re-enabled. | Prior available future offers may return; cancelled reservations stay cancelled and their slots require manual reopening. |
| AC-22 | WhatsApp fails after staff confirm a reservation. | The reservation remains confirmed, the customer can see it in the application, and staff see the notice failure. |
| AC-23 | An anonymous visitor searches by city, sport, date, or map. | Only publicly enabled branches/offers are shown; internal customer/payment information is never exposed. |
| AC-24 | A customer uses Google or Apple successfully. | They are not forced to create a password; booking still requires profile completion and phone verification through WhatsApp or the exceptional support-call procedure. |
| AC-25 | An owner requests a new branch. | It cannot publicly offer rentals before platform approval. |
| AC-26 | An employee authorized for A requests internal data from unauthorized B. | Access is refused; sharing an account or city does not grant branch membership. |
| AC-27 | A branch starts with default lead times, then its owner configures request/cancellation lead times. | Defaults are 1 hour to request and 4 hours to cancel. Both accept whole minutes from 0 through 10,080 inclusive; negative, fractional-minute, or above-range settings are refused without changing the current settings. |
| AC-28 | A 20:00 slot requires four hours of request lead time; staff receive a request at 16:00 and try to confirm it at 19:00. | The request at the exact cutoff is accepted, later new requests are refused, and staff may confirm the timely pending request at 19:00 if enabled, available, and not overlapping another confirmed reservation for that customer. At 20:00 a still-pending request expires and cannot be confirmed. |
| AC-29 | Request and cancellation lead times are zero for a 20:00 slot. | An eligible customer may request or cancel before 20:00, but neither action is allowed at 20:00 or later. |
| AC-30 | One generated slot in a batch conflicts with an existing offer on the same physical court. | The entire batch is refused and conflicts are shown. No generated slots are created and existing offers are unchanged, including when the sports differ. |
| AC-31 | Staff generate 60-minute slots from 18:00 to 22:30. | Before confirmation they see that 22:00–22:30 is excluded. With no conflict, four complete offered slots are created from 18:00 through 22:00; no shortened fifth slot is created. |
| AC-32 | A batch would offer slots on dates 180 or 181 days from the current date. | The date 180 days ahead is within the allowed horizon; the date 181 days ahead is refused. The horizon is evaluated using the agreed Argentine timezone. |
| AC-33 | Staff explicitly reopen an eligible future offer previously withdrawn or requiring reopening after suspension. | The same court, date, time, and price are preserved; no closed request or cancelled reservation revives. Reopening is refused if the branch/business is disabled, the slot has started, or a confirmed reservation or another restriction remains. Changing those details when reoffering requires retiring the old offer and creating another. |
| AC-34 | Staff record totals of ARS 0, ARS 5,000 cash plus ARS 15,000 transfer, or ARS 22,000 for an ARS 20,000 reservation. | Zero calculates pending; the two-method total of ARS 20,000 calculates paid without creating individual movements. ARS 22,000 is refused without changing the recorded amounts or status. Reservation state and agreed price remain unchanged. |
| AC-35 | Staff try to record no-show or completion before, at, and after the respective slot boundaries. | No-show is allowed from the exact start without mandatory grace time, but not before; completion is allowed from the exact end, but not before. Neither outcome is recorded automatically. |
| AC-36 | A customer withdraws a pending request. | History shows `Solicitud retirada`, not a cancelled confirmed reservation. The request stops counting toward both branch and global pending limits. |
| AC-37 | Staff try to confirm overlapping reservations for the same customer at different branches, including concurrently. | At most one of the overlapping reservations is confirmed; the other confirmation is refused without creating an overlap. There is no additional count cap on non-overlapping confirmed reservations. |
| AC-38 | A branch is suspended with pending requests, future confirmed reservations, and already-started confirmed reservations, including an ended slot without attendance recorded. | Pending requests become unavailable with a visible suspension reason and stop counting toward limits; future confirmed reservations are cancelled. Already-started reservations retain their state, without automatic attendance. Customer history remains and staff operational access is denied. |
| AC-39 | A business and one of its branches have independent suspensions. | Re-enabling the business does not clear the branch suspension, and re-enabling the branch does not override the business suspension. Closed requests and cancelled reservations never revive. |
| AC-40 | An authorized user tries to archive a court, branch, or business with unresolved pending requests or confirmed reservations. | Archival is refused until pending requests are resolved, future confirmations are explicitly cancelled with a reason, and already-started reservations finish with an attendance result. Archival performs no silent cancellation or permanent deletion. Explicit unarchival preserves approval requirements, suspensions, restrictions, and history, without reviving closed reservations or requests. |
| AC-41 | An owner edits public description, photos, contact, name, address, or location of an approved branch, or corrects a rejected new-branch application. | Public edits do not require platform reapproval; map coordinates must be confirmed for publication. The rejected application can be corrected and resubmitted with its reason and review history retained, but cannot operate publicly until approved. |
| AC-42 | A person uses the same email for a work account and a customer account, or adds a password/sign-in method to a social customer account. | Work and customer credentials, recovery, and permissions stay independent. A password can be added at any time from an authenticated customer session with verified email. Linking requires explicit action and verification of both accesses; email coincidence alone never merges accounts. |
| AC-43 | An authorized operator provisions the first administrator, or an administrator attempts to revoke a business's last co-owner. | Initial provisioning sends a single-use activation invitation through a private command without a default password or public signup. Last-co-owner revocation is refused without changing membership until a replacement is assigned. |
| AC-44 | Customers supply `12.345.678`, `12345678`, and `012345678`, or attempt to correct an enabled profile's DNI. | The three inputs share one uniqueness comparison; letters and other invalid characters are refused. Only a protected fingerprint and final digits are stored, not a recoverable full number. Correction requires restricted support with a recorded legitimacy/duplicate review. A claim alone neither blocks the existing account nor reassigns the DNI or exposes another account's information. |
| AC-45 | Recovery cannot resolve a DNI conflict, or an account is effectively closed. | Reassignment requires private live human support review, not merely a known DNI or verified phone; decision/rationale are recorded without photos or recordings. On closure, fingerprint/final digits prevent reuse for 30 days and are then deleted without erasing establishment reservation history, subject to legal review before operation. |
| AC-46 | A customer requests and enters a WhatsApp verification code, requests a replacement, or exceeds an attempt/send limit. | The six-digit code expires after 5 minutes, allows at most 5 attempts, and is never logged or reused after success. A replacement invalidates the old code. Sends less than 60 seconds apart or beyond 5 per phone per hour are refused without sending; verification never opts the customer into reservation notices. |
| AC-47 | WhatsApp is unavailable/restricted, or a customer has no WhatsApp, and phone verification is needed. | Support may verify by a call to that number and a code displayed in the customer's session, recording the intervention. If support is unavailable, browsing remains possible but requests stay disabled, with no promised response time. Already-eligible customers continue operating. |
| AC-48 | A customer accepts, refuses, or revokes WhatsApp reservation notices. | Acceptance is explicit, separate, and recorded; refusal/revocation does not restrict reservations or in-app history. Sent notices contain the selected summary/public reason and an authenticated link, not DNI, payments, or internal notes. Guests receive no automatic notices. |
| AC-49 | A notice fails transiently, is permanently rejected, or has an uncertain acceptance result. | At most two retries occur within 15 minutes only for eligible transient failures, respecting consent and budget. Permanent rejection is not retried; uncertain acceptance is checked before any resend. Authorized staff see failures, contact phone, and a comprehensible reason in both reservation detail and their scoped failure list; reservation state never changes. |
| AC-50 | A visitor chooses Near me, denies location permission, or finds no results. | Device permission is requested only for Near me; city/manual reference-address search remains available. Initial radius is 5 km, adjustable from 1 to 50 km. No results prompts an optional radius increase, never automatic expansion. Matching results default to straight-line distance with a lowest-matching-price sort option. |
| AC-51 | A visitor in a different device timezone views a turn, or Maps/address lookup fails. | Turn times and cutoffs remain explicitly in America/Argentina/Buenos_Aires. City search and filtered results continue as a list; proximity works with available coordinates, while a new unresolved reference address waits for the provider. No private reservation-holder or payment data is exposed. |
| AC-52 | The Google Maps and Evolution integrations are prepared for operation. | Maps is the selected Google service family; WhatsApp uses Evolution/Baileys and one dedicated CanchaLibre number managed by platform operators, with no SMS. The additional service budget is at most USD 50/month, not zero, excluding the existing server. Billing activation/purchases require authorization; quotas/cost controls, Google usage terms/coverage, credentials, and existing-server capacity/isolation must be validated before operation, not presumed proven by this document. |

## 12. Decision register and pre-operation checks

All twelve original product decisions have been agreed. This register preserves their identifiers; detailed rules appear in the sections referenced below. Agreement does not mean the features, integrations, or safeguards have been implemented or verified.

| ID | Agreed outcome | Rules |
| --- | --- | --- |
| O-01 | Evolution / Baileys without SMS; Google Maps; up to USD 50/month additional external-service spend; bounded retries and channel fallbacks. | Sections 7 and 10 |
| O-02 | Defaults of 1 hour to request and 4 hours to cancel; whole minutes from 0–7 days; inclusive cutoff except slot start; approve timely requests until before start. | Section 6 |
| O-03 | Reject the whole batch on conflict; complete intervals only with prior remainder notice; 180-day horizon; reopen the same unchanged offer. | Sections 4–5 |
| O-04 | Normalized DNI fingerprint and final digits; restricted support correction/live review without images; 30-day post-closure retention, subject to legal review. | Section 7 |
| O-05 | Independent work/customer accounts may share email; optional password at any time and explicit verified linking; private first-admin invitation command; replace the last co-owner before revocation. | Sections 3 and 7 |
| O-06 | Owners directly edit public fields, including name/location; rejected branch applications can be corrected and resubmitted with review history. | Section 4 |
| O-07 | Preserve already-started reservations; pending requests become unavailable due to suspension; business and branch suspensions remain independent. | Section 9 |
| O-08 | Resolve requests/reservations before archival; reversible authorized archival/retirement instead of permanent deletion; retain history. | Section 5 |
| O-09 | Accumulated ARS amounts by method; calculated status; totals above the agreed price refused; no movement ledger. | Section 8 |
| O-10 | Optional revocable consent; no automatic guest notices; minimal reservation summary and authenticated link; scoped failure detail/list. | Section 10 |
| O-11 | Google Maps, optional device location and manual fallback, 5 km initial radius adjustable 1–50 km, distance/price ordering, confirmed branch coordinates, Argentine timezone. | Sections 4 and 10 |
| O-12 | Manual no-show from start and completion from end; `Solicitud retirada`; no overlapping confirmed reservations for one customer across branches. | Sections 6–7 |

### Required before operation

- Obtain legal review of the DNI support and retention policy, including deletion of its fingerprint/final digits after the agreed 30-day period. The policy is not proof of regulatory compliance.
- Validate Google Maps coverage for target Argentine locations, applicable pricing/quotas and controls against exceeding the agreed budget, and licensing/retention conditions for coordinate use. Alerts alone do not cap charges.
- Check that the existing server can safely host Evolution alongside CanchaLibre, and configure restricted credentials, sessions, and failure monitoring. Do not claim that the messaging channel has guaranteed uptime or zero total cost.
- Provision the dedicated number and service credentials through authorized operational work; this document contains no credentials and authorizes no account creation, billing activation, installation, or deployment.

If these checks show that the agreed scope cannot be met, return for a product decision rather than silently changing the rules.

## 13. Explicitly outside this MVP

- AI-assisted identity verification, automated DNI photo processing, and any retention of document photos or review recordings; the platform has no upload, transmission, storage, or deletion path for identity images. The restricted live human support review in section 7 is included, but does not make every customer a verified identity.
- Online payment processing, automated collections/refunds, and individual payment/refund ledgers.
- Automatic reservation acceptance, waitlists, or automatic revival of closed requests.
- Reprogramming that requires a customer's new acceptance.
- Automatically recurring weekly slot generation and complex pricing rules.
- Promotional messaging, automated reservation reminders, public reviews, and multi-country/multi-currency operation.

## 14. Delivery boundary and next step

This product draft does not prescribe a delivery order or authorize implementation. The existing technical baseline remains Go/Chi, Next.js, and PostgreSQL with modular boundaries; its current authentication guidance must be reconciled with this MVP's four roles, co-ownership, branch memberships, and separate accounts before implementing access control.

**Next step:** Agree on the first implementation slice and its acceptance checks, addressing the relevant pre-operation validations in section 12. Google Maps, WhatsApp notices and phone verification (with the exceptional support-call alternative), and multiple branches remain part of the agreed MVP even if delivered in different slices. SMS is excluded.
