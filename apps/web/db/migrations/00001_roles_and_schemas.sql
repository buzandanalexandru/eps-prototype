-- Roles and schemas (P2, U2). Runs before every other migration of either schema.
-- eps_web and eps_core cannot log in and cannot bypass row-level security; every application
-- transaction switches to one of them with SET LOCAL ROLE. The migrating user (the Neon branch
-- owner) owns the tables, so the application roles never own a table.

-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_web') THEN
        CREATE ROLE eps_web NOLOGIN NOBYPASSRLS NOINHERIT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_core') THEN
        CREATE ROLE eps_core NOLOGIN NOBYPASSRLS NOINHERIT;
    END IF;
END
$$;
-- +goose StatementEnd

-- The connection user may switch to either role (PostgreSQL 16+ grant options).
GRANT eps_web TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;
GRANT eps_core TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;

CREATE SCHEMA IF NOT EXISTS control;
CREATE SCHEMA IF NOT EXISTS sales;
REVOKE ALL ON SCHEMA control FROM PUBLIC;
REVOKE ALL ON SCHEMA sales FROM PUBLIC;
GRANT USAGE ON SCHEMA control TO eps_web, eps_core;
GRANT USAGE ON SCHEMA sales TO eps_web, eps_core;

-- Control tables created later by the migrator are writable by the web app only. The core gets
-- explicit read grants on the few control tables it needs.
ALTER DEFAULT PRIVILEGES IN SCHEMA control GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO eps_web;
ALTER DEFAULT PRIVILEGES IN SCHEMA control GRANT USAGE, SELECT ON SEQUENCES TO eps_web;
