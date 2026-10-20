CREATE SCHEMA IF NOT EXISTS schema_organization;

-- 1. BUSINESS (Global legal entity or commercial brand)
CREATE TABLE schema_organization.business (
    id              uuid        PRIMARY KEY,
    name            text        NOT NULL CHECK (name <> ''),
    legal_name      text        NULL,
    tax_id          text        NULL,
    enabled         boolean     NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    suspended_at    timestamptz NULL,
    archived_at     timestamptz NULL
);

-- 2. BRANCH (Physical facility with commercial, physical, and fiscal data)
CREATE TABLE schema_organization.branch (
    id                  uuid            PRIMARY KEY,
    business_id         uuid            NOT NULL REFERENCES schema_organization.business (id),
    name                text            NOT NULL CHECK (name <> ''),
    slug                text            NOT NULL,
    description         text            NULL,
    phone               text            NULL,
    whatsapp            text            NULL,
    email               text            NULL,

    -- Local fiscal data
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
    coordinates_confirmed boolean       NOT NULL DEFAULT false,

    -- Lead times in whole minutes (BE-ORG-05: 0 to 10,080 min, defaults 1h and 4h)
    min_booking_lead_minutes        int NOT NULL DEFAULT 60 CHECK (min_booking_lead_minutes BETWEEN 0 AND 10080),
    min_cancellation_lead_minutes   int NOT NULL DEFAULT 240 CHECK (min_cancellation_lead_minutes BETWEEN 0 AND 10080),

    -- Operational and review states
    approval_status     text            NOT NULL DEFAULT 'PENDING_APPROVAL'
        CHECK (approval_status IN ('PENDING_APPROVAL', 'APPROVED', 'REJECTED')),
    enabled             boolean         NOT NULL DEFAULT false,
    suspended_at        timestamptz     NULL,
    created_at          timestamptz     NOT NULL DEFAULT now(),
    archived_at         timestamptz     NULL,

    CONSTRAINT branch_slug_business_key UNIQUE (business_id, slug),
    CONSTRAINT branch_id_business_key UNIQUE (id, business_id)
);

CREATE INDEX branch_business_idx ON schema_organization.branch (business_id);
CREATE INDEX branch_approval_idx ON schema_organization.branch (approval_status) WHERE archived_at IS NULL;
CREATE INDEX branch_geo_idx ON schema_organization.branch (latitude, longitude)
    WHERE approval_status = 'APPROVED' AND coordinates_confirmed = true AND enabled = true;

-- 3. MEMBERSHIP (OWNER is business-wide, EMPLOYEE is branch-scoped)
CREATE TABLE schema_organization.membership (
    id          uuid        PRIMARY KEY,
    account_id  uuid        NOT NULL,
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
    submitted_by        uuid        NOT NULL,
    submitted_at        timestamptz NOT NULL DEFAULT now(),
    reviewed_by         uuid        NULL,
    reviewed_at         timestamptz NULL,
    status              text        NOT NULL CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),
    rejection_reason    text        NULL,
    CONSTRAINT branch_rejection_requires_reason CHECK (
        status <> 'REJECTED' OR (rejection_reason IS NOT NULL AND rejection_reason <> '')
    )
);

CREATE INDEX branch_approval_history_idx ON schema_organization.branch_approval_history (branch_id, submitted_at DESC);

-- 5. BRANCH OPERATING HOURS (Open intervals; absence of rows = closed; multiple intervals = split shifts)
CREATE TABLE schema_organization.branch_operating_hours (
    id                  uuid        PRIMARY KEY,
    branch_id           uuid        NOT NULL REFERENCES schema_organization.branch (id) ON DELETE CASCADE,
    day_of_week         smallint    NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    open_time           time        NOT NULL,
    close_time          time        NOT NULL,
    CONSTRAINT valid_operating_interval CHECK (close_time > open_time)
);

CREATE INDEX branch_hours_idx ON schema_organization.branch_operating_hours (branch_id, day_of_week);

-- 6. PUBLIC BRANCH PHOTO GALLERY (MinIO / S3; cover photo derived from sort_order = 0)
CREATE TABLE schema_organization.branch_photo (
    id                  uuid        PRIMARY KEY,
    branch_id           uuid        NOT NULL REFERENCES schema_organization.branch (id) ON DELETE CASCADE,
    storage_key         text        NOT NULL,
    mime_type           text        NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    file_size_bytes     bigint      NOT NULL CHECK (file_size_bytes <= 10485760),
    sort_order          int         NOT NULL DEFAULT 0,
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
