CREATE SCHEMA IF NOT EXISTS schema_identity;

-- One row per independent account. Work and customer accounts may share an
-- email address; uniqueness is therefore per (email, kind), never per email.
CREATE TABLE schema_identity.account (
    id            uuid        PRIMARY KEY,
    email         text        NOT NULL CHECK (email <> ''),
    kind          text        NOT NULL CHECK (kind IN ('CUSTOMER', 'WORK')),
    platform_role text        NULL CHECK (platform_role IN ('ADMIN', 'SOPORTE')),
    status        text        NOT NULL DEFAULT 'PENDING'
                              CHECK (status IN ('PENDING', 'ACTIVE', 'SUSPENDED', 'CLOSED')),
    display_name  text        NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    closed_at     timestamptz NULL,
    CONSTRAINT account_platform_role_is_work
        CHECK (platform_role IS NULL OR kind = 'WORK')
);

CREATE UNIQUE INDEX account_email_kind_key
    ON schema_identity.account (lower(email), kind);

CREATE INDEX account_status_idx
    ON schema_identity.account (status)
    WHERE status <> 'ACTIVE';

-- Password is optional by construction: a social customer has no row, and
-- adding a password later inserts one. Algorithm parameters live in the hash.
CREATE TABLE schema_identity.credential (
    account_id    uuid        PRIMARY KEY
                              REFERENCES schema_identity.account (id) ON DELETE CASCADE,
    password_hash text        NOT NULL CHECK (password_hash <> ''),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Single-use, expiring invitation. The raw token is never stored, only its hash.
CREATE TABLE schema_identity.invitation (
    id            uuid        PRIMARY KEY,
    token_hash    bytea       NOT NULL UNIQUE,
    email         text        NOT NULL CHECK (email <> ''),
    intended_role text        NOT NULL
                              CHECK (intended_role IN ('ADMIN', 'OWNER', 'EMPLOYEE', 'SOPORTE')),
    business_id   uuid        NULL,
    branch_id     uuid        NULL,
    issued_by     uuid        NULL REFERENCES schema_identity.account (id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    consumed_at   timestamptz NULL,
    consumed_by   uuid        NULL REFERENCES schema_identity.account (id),
    CONSTRAINT invitation_scope_shape CHECK (
        (intended_role IN ('ADMIN', 'SOPORTE') AND business_id IS NULL AND branch_id IS NULL)
        OR (intended_role = 'OWNER' AND business_id IS NOT NULL AND branch_id IS NULL)
        OR (intended_role = 'EMPLOYEE' AND business_id IS NOT NULL AND branch_id IS NOT NULL)
    ),
    CONSTRAINT invitation_consumption_shape CHECK (
        (consumed_at IS NULL AND consumed_by IS NULL)
        OR (consumed_at IS NOT NULL AND consumed_by IS NOT NULL)
    )
);

CREATE INDEX invitation_email_idx ON schema_identity.invitation (lower(email));

-- One row per login session. The row is replaced on every rotation; reuse of a
-- replaced token revokes the whole family. No cross-schema reference.
CREATE TABLE schema_identity.refresh_session (
    id          uuid        PRIMARY KEY,
    account_id  uuid        NOT NULL REFERENCES schema_identity.account (id) ON DELETE CASCADE,
    family_id   uuid        NOT NULL,
    token_hash  bytea       NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    rotated_at  timestamptz NULL,
    revoked_at  timestamptz NULL,
    replaced_by uuid        NULL REFERENCES schema_identity.refresh_session (id)
);

CREATE INDEX refresh_session_family_idx ON schema_identity.refresh_session (family_id);
CREATE INDEX refresh_session_account_idx ON schema_identity.refresh_session (account_id);

-- Single-use password recovery token. The raw token is never stored, only its SHA-256 hash.
CREATE TABLE schema_identity.password_recovery_token (
    id          uuid        PRIMARY KEY,
    account_id  uuid        NOT NULL REFERENCES schema_identity.account (id) ON DELETE CASCADE,
    token_hash  bytea       NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz NULL
);

CREATE INDEX password_recovery_token_account_idx
    ON schema_identity.password_recovery_token (account_id);

-- Append-only record of identity decisions, including refusals. Audit is not
-- domain state: recording a refusal does not violate "rejection mutates nothing".
CREATE TABLE schema_identity.identity_audit (
    id                 bigserial   PRIMARY KEY,
    occurred_at        timestamptz NOT NULL DEFAULT now(),
    event              text        NOT NULL CHECK (event <> ''),
    outcome            text        NOT NULL CHECK (outcome IN ('SUCCESS', 'REFUSED')),
    actor_account_id   uuid        NULL REFERENCES schema_identity.account (id),
    subject_account_id uuid        NULL REFERENCES schema_identity.account (id),
    reason             text        NOT NULL DEFAULT '',
    detail             jsonb       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX identity_audit_subject_idx
    ON schema_identity.identity_audit (subject_account_id, occurred_at DESC);
