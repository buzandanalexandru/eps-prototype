-- Organiser registry and memberships (I1, U2). Step 1 extends both.

-- +goose Up
-- The organiser registry is a platform table (allowlisted): every organiser's existence is public
-- to the platform, but nothing else about it is stored here.
CREATE TABLE control.organisers (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       text        NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$'),
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT ON control.organisers TO eps_core;

-- Memberships are organiser data: organiser_id NOT NULL, RLS enabled and forced.
CREATE TABLE control.memberships (
    organiser_id uuid        NOT NULL REFERENCES control.organisers (id) ON DELETE CASCADE,
    user_id      text        NOT NULL REFERENCES control."user" ("id") ON DELETE CASCADE,
    role         text        NOT NULL CHECK (role IN ('owner', 'admin', 'staff')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organiser_id, user_id)
);
CREATE INDEX memberships_user_idx ON control.memberships (user_id);

ALTER TABLE control.memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE control.memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY organiser_isolation ON control.memberships
    USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
    WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid);

-- A signed-in user may read their own memberships before any organiser scope exists (to pick the
-- active organiser for a core token). Permissive policies are OR-ed: this one only ever adds the
-- caller's own rows, and only for reads.
CREATE POLICY own_memberships ON control.memberships FOR SELECT
    USING (user_id = nullif(current_setting('app.user_id', true), ''));

-- Demo mailbox (design D0.4): magic links for demo-domain addresses, shown on screen when no mail
-- provider is configured. Platform table (allowlisted); rows expire with the links.
CREATE TABLE control.demo_mailbox (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email      text        NOT NULL,
    url        text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX demo_mailbox_email_idx ON control.demo_mailbox (email, created_at DESC);
