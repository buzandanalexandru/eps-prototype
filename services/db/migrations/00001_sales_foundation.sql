-- Sales schema foundation (owned by the Go core, P2).
-- The roles and both schemas are created by control migration 00001, which always runs first.

-- +goose Up
GRANT USAGE ON SCHEMA sales TO eps_core, eps_web;

-- Tables created later by the migrator are writable by the core and readable by the web app.
ALTER DEFAULT PRIVILEGES IN SCHEMA sales GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO eps_core;
ALTER DEFAULT PRIVILEGES IN SCHEMA sales GRANT USAGE, SELECT ON SEQUENCES TO eps_core;
ALTER DEFAULT PRIVILEGES IN SCHEMA sales GRANT SELECT ON TABLES TO eps_web;

-- A sample organiser table, used only by the isolation tests (step 0). Every sales table follows
-- this shape: organiser_id NOT NULL, RLS enabled AND forced, one organiser policy.
CREATE TABLE sales.probe (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL,
    label        text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX probe_organiser_idx ON sales.probe (organiser_id);

ALTER TABLE sales.probe ENABLE ROW LEVEL SECURITY;
ALTER TABLE sales.probe FORCE ROW LEVEL SECURITY;
CREATE POLICY organiser_isolation ON sales.probe
    USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
    WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON sales.probe TO eps_core;
GRANT SELECT ON sales.probe TO eps_web;
