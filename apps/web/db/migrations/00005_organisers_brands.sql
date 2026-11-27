-- Organisers, brands, team and fees (step 1, design D1.1, D1.2, D1.3, D1.6; A1, A2, A3, A6).
-- Every organiser table carries organiser_id NOT NULL with row-level security enabled and forced;
-- the only new platform table is control.platform_fee (allowlisted).

-- +goose Up
-- Organisers gain the sample flag (sample-data capability) and the onboarding step a new
-- organiser has reached ('brand', then 'done'), so an interrupted onboarding resumes.
ALTER TABLE control.organisers
    ADD COLUMN is_sample        boolean NOT NULL DEFAULT false,
    ADD COLUMN onboarding_state text    NOT NULL DEFAULT 'done' CHECK (onboarding_state IN ('brand', 'done'));

-- The active organiser of a sign-in session (D1.1): a Better Auth session field, read when the core
-- token is issued. A membership check always follows, so a stale value grants nothing.
ALTER TABLE control."session" ADD COLUMN "activeOrganiserId" text;

-- Roles (D1.6): owner, admin, box office, finance; 'staff' stays valid for rows written by step 0.
-- Scope: the whole organiser, one brand or one event.
ALTER TABLE control.memberships DROP CONSTRAINT memberships_role_check;
ALTER TABLE control.memberships
    ADD CONSTRAINT memberships_role_check CHECK (role IN ('owner', 'admin', 'box_office', 'finance', 'staff')),
    ADD COLUMN scope_type text NOT NULL DEFAULT 'organiser' CHECK (scope_type IN ('organiser', 'brand', 'event')),
    ADD COLUMN scope_id   uuid,
    ADD CONSTRAINT memberships_scope_check CHECK ((scope_type = 'organiser') = (scope_id IS NULL));

-- Brands: several per organiser, each with a globally unique slug (its shop address /s/<slug>).
CREATE TABLE control.brands (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL REFERENCES control.organisers (id) ON DELETE CASCADE,
    slug         text        NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$'),
    name         text        NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    languages    text[]      NOT NULL DEFAULT '{ro,en}' CHECK (languages <@ '{ro,en}' AND cardinality(languages) >= 1),
    currency     text        NOT NULL DEFAULT 'RON' CHECK (currency IN ('RON', 'EUR')),
    template     text        NOT NULL DEFAULT 'festival' CHECK (template IN ('festival', 'resort', 'club', 'blank')),
    logo_url     text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organiser_id, id)
);
-- Platform-level uniqueness: enforced by the index regardless of row-level security.
CREATE UNIQUE INDEX brands_slug_key ON control.brands (slug);

-- Hostnames for host-based shop addresses, set only by configuration or the seed (B8 is cut).
CREATE TABLE control.brand_hostnames (
    hostname     text        PRIMARY KEY CHECK (hostname ~ '^[a-z0-9.-]{3,253}$'),
    organiser_id uuid        NOT NULL,
    brand_id     uuid        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (organiser_id, brand_id) REFERENCES control.brands (organiser_id, id) ON DELETE CASCADE
);

-- Team invitations: a pending row and a copyable link; only the token's hash is stored.
CREATE TABLE control.invitations (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL REFERENCES control.organisers (id) ON DELETE CASCADE,
    email        text        NOT NULL CHECK (email = lower(email) AND position('@' in email) > 1),
    role         text        NOT NULL CHECK (role IN ('owner', 'admin', 'box_office', 'finance')),
    scope_type   text        NOT NULL DEFAULT 'organiser' CHECK (scope_type IN ('organiser', 'brand', 'event')),
    scope_id     uuid,
    token_hash   text        NOT NULL UNIQUE,
    invited_by   text        REFERENCES control."user" ("id") ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL DEFAULT now() + interval '14 days',
    accepted_at  timestamptz,
    CHECK ((scope_type = 'organiser') = (scope_id IS NULL))
);
CREATE INDEX invitations_org_idx ON control.invitations (organiser_id, created_at DESC);

-- The organiser's fee policy (one row per organiser; no row means 'pass_on').
CREATE TABLE control.fee_policies (
    organiser_id uuid        PRIMARY KEY REFERENCES control.organisers (id) ON DELETE CASCADE,
    policy       text        NOT NULL CHECK (policy IN ('absorb', 'pass_on', 'split')),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- The published platform fee (platform table, readable by everyone, written only by migrations).
CREATE TABLE control.platform_fee (
    id             int         GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    percent_bps    int         NOT NULL CHECK (percent_bps BETWEEN 0 AND 10000),
    fixed_minor    int         NOT NULL CHECK (fixed_minor >= 0),
    currency       text        NOT NULL DEFAULT 'RON' CHECK (currency IN ('RON', 'EUR')),
    effective_from timestamptz NOT NULL
);
REVOKE INSERT, UPDATE, DELETE ON control.platform_fee FROM eps_web;
GRANT SELECT ON control.platform_fee TO eps_core;
INSERT INTO control.platform_fee (percent_bps, fixed_minor, currency, effective_from)
VALUES (250, 0, 'RON', '2026-01-01T00:00:00Z'), (250, 0, 'EUR', '2026-01-01T00:00:00Z');

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['brands', 'brand_hostnames', 'invitations', 'fee_policies'] LOOP
        EXECUTE format('ALTER TABLE control.%I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE control.%I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY organiser_isolation ON control.%I
            USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
            WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)$p$, t);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- Slug availability (D1.2) across organisers, without revealing the holder. The function is
-- SECURITY DEFINER, owned by its own NOLOGIN NOBYPASSRLS role (spec-map rule), which may read only
-- the slug column of brands through a policy of its own; it returns a boolean and nothing else.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_slug_check') THEN
        CREATE ROLE eps_slug_check NOLOGIN NOBYPASSRLS NOINHERIT;
    END IF;
END
$$;
-- +goose StatementEnd
GRANT eps_slug_check TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;
GRANT USAGE ON SCHEMA control TO eps_slug_check;
GRANT SELECT (slug) ON control.brands TO eps_slug_check;
GRANT SELECT (slug) ON control.organisers TO eps_slug_check;
CREATE POLICY slug_check_read ON control.brands FOR SELECT TO eps_slug_check USING (true);

-- +goose StatementBegin
CREATE FUNCTION control.slug_available(kind text, candidate text) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT CASE kind
        WHEN 'brand' THEN NOT EXISTS (SELECT 1 FROM control.brands b WHERE b.slug = candidate)
        WHEN 'organiser' THEN NOT EXISTS (SELECT 1 FROM control.organisers o WHERE o.slug = candidate)
        ELSE false
    END
$fn$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION control.slug_available(text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION control.slug_available(text, text) TO eps_web;
-- The new owner needs CREATE on the schema only while ownership changes hands.
GRANT CREATE ON SCHEMA control TO eps_slug_check;
ALTER FUNCTION control.slug_available(text, text) OWNER TO eps_slug_check;
REVOKE CREATE ON SCHEMA control FROM eps_slug_check;
