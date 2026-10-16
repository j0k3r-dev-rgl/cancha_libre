CREATE SCHEMA IF NOT EXISTS schema_organization;

CREATE TABLE schema_organization.business (
    id          uuid        PRIMARY KEY,
    name        text        NOT NULL CHECK (name <> ''),
    enabled     boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz NULL
);

CREATE TABLE schema_organization.branch (
    id          uuid        PRIMARY KEY,
    business_id uuid        NOT NULL REFERENCES schema_organization.business (id),
    name        text        NOT NULL CHECK (name <> ''),
    enabled     boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz NULL
);

CREATE INDEX branch_business_idx ON schema_organization.branch (business_id);

-- Makes (branch_id, business_id) referenceable as one pair, so a membership can
-- never point at a branch that belongs to a different business.
ALTER TABLE schema_organization.branch
    ADD CONSTRAINT branch_id_business_key UNIQUE (id, business_id);

-- OWNER is business-wide (branch_id NULL); EMPLOYEE is branch-scoped.
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

-- The guard's single lookup: account + business + (business-wide or branch).
CREATE INDEX membership_guard_idx
    ON schema_organization.membership (account_id, business_id, branch_id)
    WHERE revoked_at IS NULL;

CREATE UNIQUE INDEX membership_one_active_owner_key
    ON schema_organization.membership (account_id, business_id)
    WHERE role = 'OWNER' AND revoked_at IS NULL;

CREATE UNIQUE INDEX membership_one_active_employee_key
    ON schema_organization.membership (account_id, branch_id)
    WHERE role = 'EMPLOYEE' AND revoked_at IS NULL;
