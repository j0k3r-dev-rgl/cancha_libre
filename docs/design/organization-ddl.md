# Organization DDL: Businesses, Branches, Courts, Operating Hours and Approvals

This design specifies the complete database schema for the **Organization bounded context (Slice S2)** in `schema_organization`. It supersedes the minimal spine frozen in [`docs/design/identity-ddl.md`](identity-ddl.md) and delivers the data structures required by `BE-ORG-01` through `BE-ORG-07`, `BE-DIS-03` (geocoding), and `BE-DIS-05` (timezone/operating rules).

It pairs with the upcoming `docs/design/organization-endpoints.md`, which defines the HTTP operations and contract verification.

---

## Decisions

| # | Topic | Decision |
| --- | --- | --- |
| D1 | Context isolation | All organization and physical asset entities live in `schema_organization`. No cross-schema foreign keys: references to `schema_identity.account` (such as `submitted_by`, `reviewed_by`, or `membership.account_id`) are stored as **logical UUIDs** validated through application ports. |
| D2 | Derived vs. stored state (no denormalization) | State that can be computed deterministically is **never stored as a mutable column**: <br>1. `is_published` is not stored: a branch is published if and only if `approval_status = 'APPROVED'`, `coordinates_confirmed = true`, `enabled = true`, neither business nor branch is suspended, and neither is archived.<br>2. `public_url` of photos is not stored: only `storage_key` is persisted. The base URL / CDN domain is injected by configuration.<br>3. `is_cover` flag is not stored: the cover photo is the one with `sort_order = 0` (or lowest order). |
| D3 | Onboarding and administrative authority | `ADMIN` creates the business entity (`business`) and assigns the initial co-owners (`membership`). `ADMIN` possesses platform-wide visibility and can suspend accounts, businesses, or branches. |
| D4 | Branch approval lifecycle (`BE-ORG-02`) | A branch begins in `approval_status = 'PENDING_APPROVAL'`. An `ADMIN` or `SOPORTE` operator can approve or reject it. Rejection strictly requires a non-empty `rejection_reason`. Full review history is appended to `branch_approval_history` without mutating past entries. |
| D5 | Geocoding and map confirmation (`BE-DIS-03`, `BE-ORG-03`) | Google Maps Geocoding API resolves addresses to `latitude`, `longitude`, and `google_place_id` behind an abstract `GeocodingPort`. The branch owner must explicitly confirm or adjust proposed coordinates (`coordinates_confirmed = true`) before the branch can be published. |
| D6 | Media storage (`BE-ORG-03`) | Handled via MinIO in development and S3-compatible cloud storage in production behind a `StoragePort`. Stores only relative `storage_key`, MIME type, file size (max 10 MB), and `sort_order`. |
| D7 | Flexible operating hours | `branch_operating_hours` records open intervals (`day_of_week`, `open_time`, `close_time`). Days without rows represent closed days; multiple rows for the same day support split shifts (e.g. morning and evening). |
| D8 | Lead times (`BE-ORG-05`) | Configured per branch in whole minutes: minimum booking lead time (default 60 min) and minimum cancellation lead time (default 240 min), constrained between 0 and 10,080 minutes (7 days). |
| D9 | Courts and sports (`BE-ORG-04`) | Physical courts (`court`) belong to a branch and store supported sports (`sports text[]`), surface type, indoor/lighting flags, default duration in minutes, and base price in ARS cents. |
| D10 | Archival instead of hard delete (`BE-ORG-06`, `BE-ORG-07`) | Deletions are logical via `archived_at timestamptz`. Archival of businesses, branches, or courts is refused at the application layer if unresolved pending requests or active reservations exist. |

---

## Schema DDL: `schema_organization`

```sql
CREATE SCHEMA IF NOT EXISTS schema_organization;

-- 1. BUSINESS (Global legal entity or commercial brand)
CREATE TABLE schema_organization.business (
    id              uuid        PRIMARY KEY,
    name            text        NOT NULL CHECK (name <> ''),
    legal_name      text        NULL,                        -- Central company legal name
    tax_id          text        NULL,                        -- Central CUIT / tax identifier
    enabled         boolean     NOT NULL DEFAULT true,       -- Master platform switch (ADMIN)
    created_at      timestamptz NOT NULL DEFAULT now(),
    suspended_at    timestamptz NULL,                        -- Administrative suspension (ADMIN)
    archived_at     timestamptz NULL
);

-- 2. BRANCH (Physical facility with commercial, physical, and fiscal data)
CREATE TABLE schema_organization.branch (
    id                  uuid            PRIMARY KEY,
    business_id         uuid            NOT NULL REFERENCES schema_organization.business (id),
    name                text            NOT NULL CHECK (name <> ''),
    slug                text            NOT NULL,                    -- URL-friendly unique identifier
    description         text            NULL,
    phone               text            NULL,
    whatsapp            text            NULL,
    email               text            NULL,

    -- Local fiscal data (optional override if branch operates as an independent franchise or legal entity)
    legal_name          text            NULL,
    tax_id              text            NULL,

    -- Physical location and Google Maps coordinates (BE-DIS-03, BE-ORG-03)
    street_address      text            NOT NULL CHECK (street_address <> ''),
    floor_apt           text            NULL,
    city                text            NOT NULL CHECK (city <> ''),
    state_province      text            NOT NULL CHECK (state_province <> ''),
    postal_code         text            NOT NULL,
    country             text            NOT NULL DEFAULT 'AR',
    latitude            numeric(10, 7)  NULL,
    longitude           numeric(10, 7)  NULL,
    google_place_id     text            NULL,
    coordinates_confirmed boolean       NOT NULL DEFAULT false,      -- Required before publication

    -- Lead times in whole minutes (BE-ORG-05: 0 to 10,080 min, defaults 1h and 4h)
    min_booking_lead_minutes        int NOT NULL DEFAULT 60 CHECK (min_booking_lead_minutes BETWEEN 0 AND 10080),
    min_cancellation_lead_minutes   int NOT NULL DEFAULT 240 CHECK (min_cancellation_lead_minutes BETWEEN 0 AND 10080),

    -- Operational and review states
    approval_status     text            NOT NULL DEFAULT 'PENDING_APPROVAL'
        CHECK (approval_status IN ('PENDING_APPROVAL', 'APPROVED', 'REJECTED')),
    enabled             boolean         NOT NULL DEFAULT false,      -- Owner switch
    suspended_at        timestamptz     NULL,                        -- Administrative suspension (ADMIN)
    created_at          timestamptz     NOT NULL DEFAULT now(),
    archived_at         timestamptz     NULL,

    CONSTRAINT branch_slug_business_key UNIQUE (business_id, slug),
    CONSTRAINT branch_id_business_key UNIQUE (id, business_id)
);

CREATE INDEX branch_business_idx ON schema_organization.branch (business_id);
CREATE INDEX branch_approval_idx ON schema_organization.branch (approval_status) WHERE archived_at IS NULL;
CREATE INDEX branch_geo_idx ON schema_organization.branch (latitude, longitude)
    WHERE approval_status = 'APPROVED' AND coordinates_confirmed = true AND enabled = true;

-- 3. MEMBERSHIP (Retained from spine: OWNER is business-wide, EMPLOYEE is branch-scoped)
CREATE TABLE schema_organization.membership (
    id          uuid        PRIMARY KEY,
    account_id  uuid        NOT NULL,                            -- Logical ref to schema_identity.account
    business_id uuid        NOT NULL REFERENCES schema_organization.business (id),
    branch_id   uuid        NULL,
    role        text        NOT NULL CHECK (role IN ('OWNER', 'EMPLOYEE')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz NULL,
    CONSTRAINT membership_role_scope CHECK (
        (role = 'OWNER' AND branch_id IS NULL)
        OR (role = 'EMPLOYEE' AND branch_id IS NOT NULL)
    ),
    CONSTRAINT membership_branch_in_business
        FOREIGN KEY (branch_id, business_id)
        REFERENCES schema_organization.branch (id, business_id)
);

CREATE INDEX membership_guard_idx
    ON schema_organization.membership (account_id, business_id, branch_id)
    WHERE revoked_at IS NULL;

CREATE UNIQUE INDEX membership_one_active_owner_key
    ON schema_organization.membership (account_id, business_id)
    WHERE role = 'OWNER' AND revoked_at IS NULL;

CREATE UNIQUE INDEX membership_one_active_employee_key
    ON schema_organization.membership (account_id, branch_id)
    WHERE role = 'EMPLOYEE' AND revoked_at IS NULL;

-- 4. BRANCH APPROVAL HISTORY (BE-ORG-02: Preserved review rounds)
CREATE TABLE schema_organization.branch_approval_history (
    id                  uuid        PRIMARY KEY,
    branch_id           uuid        NOT NULL REFERENCES schema_organization.branch (id),
    submitted_by        uuid        NOT NULL,                    -- Logical ref to schema_identity.account (OWNER)
    submitted_at        timestamptz NOT NULL DEFAULT now(),
    reviewed_by         uuid        NULL,                        -- Logical ref to schema_identity.account (ADMIN/SOPORTE)
    reviewed_at         timestamptz NULL,
    status              text        NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    rejection_reason    text        NULL,                        -- Mandatory when status = 'REJECTED'
    CONSTRAINT branch_rejection_requires_reason CHECK (
        status <> 'REJECTED' OR (rejection_reason IS NOT NULL AND rejection_reason <> '')
    )
);

CREATE INDEX branch_approval_history_idx ON schema_organization.branch_approval_history (branch_id, submitted_at DESC);

-- 5. BRANCH OPERATING HOURS (Open intervals; absence of rows = closed; multiple intervals = split shifts)
CREATE TABLE schema_organization.branch_operating_hours (
    id                  uuid        PRIMARY KEY,
    branch_id           uuid        NOT NULL REFERENCES schema_organization.branch (id) ON DELETE CASCADE,
    day_of_week         smallint    NOT NULL CHECK (day_of_week BETWEEN 0 AND 6), -- 0 = Sunday, 1 = Monday, ..., 6 = Saturday
    open_time           time        NOT NULL,
    close_time          time        NOT NULL,
    CONSTRAINT valid_operating_interval CHECK (close_time > open_time)
);

CREATE INDEX branch_hours_idx ON schema_organization.branch_operating_hours (branch_id, day_of_week);

-- 6. PUBLIC BRANCH PHOTO GALLERY (MinIO / S3; cover photo derived from sort_order = 0)
CREATE TABLE schema_organization.branch_photo (
    id                  uuid        PRIMARY KEY,
    branch_id           uuid        NOT NULL REFERENCES schema_organization.branch (id) ON DELETE CASCADE,
    storage_key         text        NOT NULL,                    -- Object key in MinIO/S3 (e.g. branches/{id}/{uuid}.webp)
    mime_type           text        NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    file_size_bytes     bigint      NOT NULL CHECK (file_size_bytes <= 10485760), -- 10 MB limit
    sort_order          int         NOT NULL DEFAULT 0,          -- Lowest sort_order is cover photo
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX branch_photo_order_idx ON schema_organization.branch_photo (branch_id, sort_order ASC);

-- 7. PHYSICAL COURTS (BE-ORG-04: sports, surfaces, duration, and baseline price)
CREATE TABLE schema_organization.court (
    id                          uuid            PRIMARY KEY,
    branch_id                   uuid            NOT NULL REFERENCES schema_organization.branch (id),
    name                        text            NOT NULL CHECK (name <> ''),
    court_number                int             NULL,
    sports                      text[]          NOT NULL CHECK (cardinality(sports) > 0),
    surface                     text            NOT NULL CHECK (surface <> ''),
    is_indoor                   boolean         NOT NULL DEFAULT false,
    has_lighting                boolean         NOT NULL DEFAULT false,
    base_price_cents            bigint          NOT NULL CHECK (base_price_cents >= 0),
    default_duration_minutes    int             NOT NULL DEFAULT 60 CHECK (default_duration_minutes > 0),
    created_at                  timestamptz     NOT NULL DEFAULT now(),
    archived_at                 timestamptz     NULL
);

CREATE INDEX court_branch_idx ON schema_organization.court (branch_id) WHERE archived_at IS NULL;
```

---

## Adjustment to `schema_identity.account`

To support platform-wide administrative account suspension (`ADMIN` suspending any user):

```sql
ALTER TABLE schema_identity.account 
    DROP CONSTRAINT IF EXISTS account_status_check;

ALTER TABLE schema_identity.account 
    ADD CONSTRAINT account_status_check 
    CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED', 'CLOSED'));
```

---

## Derived Publication Predicate

In application queries and domain services, whether a branch is publicly visible and available for rental searches is evaluated via the following derived predicate:

```text
is_published = (
    branch.approval_status = 'APPROVED'
    AND branch.coordinates_confirmed = true
    AND branch.enabled = true
    AND branch.suspended_at IS NULL
    AND branch.archived_at IS NULL
    AND business.enabled = true
    AND business.suspended_at IS NULL
    AND business.archived_at IS NULL
)
```
