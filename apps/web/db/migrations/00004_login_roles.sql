-- Login roles for the applications (P2, U2). The web app and the Go services connect as these,
-- never as the branch owner (neondb_owner), which is kept for migrations only: on Neon the owner
-- is a member of neon_superuser and bypasses row-level security.
-- eps_web_login / eps_core_login can log in, cannot bypass RLS, own nothing and inherit only the
-- grants of their application role; every application transaction still runs SET LOCAL ROLE.
-- No password here: the pipeline sets one per branch (cmd/migrate -login), so a role without a
-- password simply cannot log in.
-- Rule for later steps: any SECURITY DEFINER function (step 3's control.live_snapshot()) is owned
-- by a dedicated NOLOGIN NOBYPASSRLS role, never by the migrating owner.

-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_web_login') THEN
        CREATE ROLE eps_web_login LOGIN NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_core_login') THEN
        CREATE ROLE eps_core_login LOGIN NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
    END IF;
END
$$;
-- +goose StatementEnd

GRANT eps_web TO eps_web_login WITH INHERIT TRUE, SET TRUE;
GRANT eps_core TO eps_core_login WITH INHERIT TRUE, SET TRUE;
