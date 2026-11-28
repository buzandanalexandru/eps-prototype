-- Storefront builder (step 2, design D2.1; B1 to B7, U3). One storefront per brand, stored as
-- versioned JSON documents: one draft (edited in place, guarded by a revision number) and any number
-- of published versions, which never change after they are written. Preview links store only the
-- SHA-256 of their token; uploaded images are stored here (PNG, JPEG, WebP, at most 1 MB).
-- Every table carries organiser_id NOT NULL with forced row-level security, and every link is a
-- composite foreign key that includes organiser_id.

-- +goose Up
-- The theatre template joins the brand's template choice (B2).
ALTER TABLE control.brands DROP CONSTRAINT brands_template_check;
ALTER TABLE control.brands ADD CONSTRAINT brands_template_check CHECK (template IN ('festival', 'resort', 'club', 'theatre', 'blank'));

CREATE TABLE control.storefronts (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id    uuid        NOT NULL REFERENCES control.organisers (id) ON DELETE CASCADE,
    brand_id        uuid        NOT NULL UNIQUE,
    live_version_id uuid,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organiser_id, id),
    FOREIGN KEY (organiser_id, brand_id) REFERENCES control.brands (organiser_id, id) ON DELETE CASCADE
);

CREATE TABLE control.storefront_versions (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id   uuid        NOT NULL,
    storefront_id  uuid        NOT NULL,
    -- Published and archived versions are numbered 1, 2, 3, ... per storefront; the draft has none.
    number         int         CHECK (number >= 1),
    state          text        NOT NULL CHECK (state IN ('draft', 'published', 'archived')),
    document       jsonb       NOT NULL,
    -- The draft's revision, bumped by every save; on a published version, the draft revision it was
    -- published from (one version per draft revision: a double click publishes once).
    revision       int         NOT NULL DEFAULT 1 CHECK (revision >= 1),
    source         text        NOT NULL DEFAULT 'studio' CHECK (source IN ('studio', 'template', 'seed', 'ai')),
    restored_from  int,
    created_by     text        REFERENCES control."user" ("id") ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    published_at   timestamptz,
    CHECK ((state = 'draft') = (number IS NULL)),
    CHECK ((state = 'draft') = (published_at IS NULL)),
    CHECK (octet_length(document::text) <= 600000),
    UNIQUE (organiser_id, id),
    UNIQUE (organiser_id, storefront_id, id),
    UNIQUE (storefront_id, number),
    FOREIGN KEY (organiser_id, storefront_id) REFERENCES control.storefronts (organiser_id, id) ON DELETE CASCADE
);
-- At most one draft per storefront, and one published version per draft revision.
CREATE UNIQUE INDEX storefront_versions_one_draft ON control.storefront_versions (storefront_id) WHERE state = 'draft';
CREATE UNIQUE INDEX storefront_versions_from_revision ON control.storefront_versions (storefront_id, revision) WHERE state <> 'draft';
CREATE INDEX storefront_versions_created_by_idx ON control.storefront_versions (created_by);

-- The live version must be a published version of the same storefront.
ALTER TABLE control.storefronts
    ADD FOREIGN KEY (organiser_id, id, live_version_id) REFERENCES control.storefront_versions (organiser_id, storefront_id, id);
CREATE INDEX storefronts_live_idx ON control.storefronts (live_version_id);

-- Published versions are immutable: only the state may move from published to archived (a restore
-- writes a new version), and the author may be cleared when the user is deleted. Published and
-- archived rows are never deleted.
-- +goose StatementBegin
CREATE FUNCTION control.storefront_versions_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, pg_temp
AS $fn$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.state <> 'draft' THEN
            RAISE EXCEPTION 'published storefront versions cannot be deleted' USING ERRCODE = 'P0001';
        END IF;
        RETURN OLD;
    END IF;
    IF OLD.state = 'draft' THEN
        IF NEW.state <> 'draft' OR NEW.number IS NOT NULL THEN
            RAISE EXCEPTION 'a draft is published by writing a new version' USING ERRCODE = 'P0001';
        END IF;
        RETURN NEW;
    END IF;
    IF NEW.document IS DISTINCT FROM OLD.document OR NEW.number IS DISTINCT FROM OLD.number
       OR NEW.revision IS DISTINCT FROM OLD.revision OR NEW.storefront_id IS DISTINCT FROM OLD.storefront_id
       OR NEW.organiser_id IS DISTINCT FROM OLD.organiser_id OR NEW.published_at IS DISTINCT FROM OLD.published_at
       OR NEW.restored_from IS DISTINCT FROM OLD.restored_from
       -- the author may only be cleared (a deleted user, GDPR)
       OR (NEW.created_by IS DISTINCT FROM OLD.created_by AND NEW.created_by IS NOT NULL)
       OR NEW.source IS DISTINCT FROM OLD.source
       OR NOT (NEW.state = OLD.state OR (OLD.state = 'published' AND NEW.state = 'archived')) THEN
        RAISE EXCEPTION 'published storefront versions are read-only' USING ERRCODE = 'P0001';
    END IF;
    RETURN NEW;
END
$fn$;
-- +goose StatementEnd
CREATE TRIGGER storefront_versions_guard BEFORE UPDATE OR DELETE ON control.storefront_versions
    FOR EACH ROW EXECUTE FUNCTION control.storefront_versions_guard();

CREATE TABLE control.preview_links (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id  uuid        NOT NULL,
    storefront_id uuid        NOT NULL,
    token_hash    text        NOT NULL UNIQUE CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    created_by    text        REFERENCES control."user" ("id") ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL DEFAULT now() + interval '7 days',
    revoked_at    timestamptz,
    CHECK (expires_at > created_at AND expires_at <= created_at + interval '30 days'),
    FOREIGN KEY (organiser_id, storefront_id) REFERENCES control.storefronts (organiser_id, id) ON DELETE CASCADE
);
CREATE INDEX preview_links_storefront_idx ON control.preview_links (storefront_id, created_at DESC);
CREATE INDEX preview_links_created_by_idx ON control.preview_links (created_by);

CREATE TABLE control.assets (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL REFERENCES control.organisers (id) ON DELETE CASCADE,
    mime         text        NOT NULL CHECK (mime IN ('image/png', 'image/jpeg', 'image/webp')),
    bytes        bytea       NOT NULL CHECK (octet_length(bytes) BETWEEN 1 AND 1048576),
    sha256       text        NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    width        int         NOT NULL CHECK (width BETWEEN 1 AND 10000),
    height       int         NOT NULL CHECK (height BETWEEN 1 AND 10000),
    created_by   text        REFERENCES control."user" ("id") ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organiser_id, id)
);
CREATE INDEX assets_org_idx ON control.assets (organiser_id, created_at DESC);
CREATE INDEX assets_created_by_idx ON control.assets (created_by);

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['storefronts', 'storefront_versions', 'preview_links', 'assets'] LOOP
        EXECUTE format('ALTER TABLE control.%I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE control.%I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY organiser_isolation ON control.%I
            USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
            WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)$p$, t);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- Narrow the web role's rights: versions are never deleted by the app (drafts go with their brand),
-- uploads are never changed, and a preview link can only be revoked.
REVOKE DELETE ON control.storefront_versions FROM eps_web;
REVOKE UPDATE ON control.storefront_versions FROM eps_web;
GRANT UPDATE (document, revision, state, updated_at) ON control.storefront_versions TO eps_web;
REVOKE UPDATE ON control.storefronts FROM eps_web;
GRANT UPDATE (live_version_id) ON control.storefronts TO eps_web;
REVOKE UPDATE ON control.assets FROM eps_web;
REVOKE UPDATE ON control.preview_links FROM eps_web;
GRANT UPDATE (revoked_at) ON control.preview_links TO eps_web;

-- Two narrow cross-organiser reads for visitors without an organiser scope, each a SECURITY DEFINER
-- function owned by its own NOLOGIN NOBYPASSRLS role (spec-map rule) that may read only what the
-- function returns, through policies of its own:
--   preview_link_target(hash): which storefront a valid (unexpired, unrevoked) preview link shows;
--     the app then reads that draft in a transaction scoped to its organiser.
--   public_asset(id): an uploaded image's bytes for /assets/<id> (fan pages show them).
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_preview_read') THEN
        CREATE ROLE eps_preview_read NOLOGIN NOBYPASSRLS NOINHERIT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_asset_read') THEN
        CREATE ROLE eps_asset_read NOLOGIN NOBYPASSRLS NOINHERIT;
    END IF;
END
$$;
-- +goose StatementEnd
GRANT eps_preview_read TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;
GRANT eps_asset_read TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;
GRANT USAGE ON SCHEMA control TO eps_preview_read, eps_asset_read;
GRANT SELECT (organiser_id, storefront_id, token_hash, expires_at, revoked_at) ON control.preview_links TO eps_preview_read;
CREATE POLICY preview_read ON control.preview_links FOR SELECT TO eps_preview_read USING (true);
GRANT SELECT (id, mime, bytes, sha256) ON control.assets TO eps_asset_read;
CREATE POLICY asset_read ON control.assets FOR SELECT TO eps_asset_read USING (true);

-- +goose StatementBegin
CREATE FUNCTION control.preview_link_target(hash text)
RETURNS TABLE (organiser_id uuid, storefront_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT l.organiser_id, l.storefront_id FROM control.preview_links l
    WHERE l.token_hash = hash AND l.revoked_at IS NULL AND l.expires_at > now()
$fn$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION control.public_asset(asset_id uuid)
RETURNS TABLE (mime text, bytes bytea, sha256 text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT a.mime, a.bytes, a.sha256 FROM control.assets a WHERE a.id = asset_id
$fn$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION control.preview_link_target(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION control.public_asset(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION control.preview_link_target(text) TO eps_web;
GRANT EXECUTE ON FUNCTION control.public_asset(uuid) TO eps_web;
GRANT CREATE ON SCHEMA control TO eps_preview_read, eps_asset_read;
ALTER FUNCTION control.preview_link_target(text) OWNER TO eps_preview_read;
ALTER FUNCTION control.public_asset(uuid) OWNER TO eps_asset_read;
REVOKE CREATE ON SCHEMA control FROM eps_preview_read, eps_asset_read;
