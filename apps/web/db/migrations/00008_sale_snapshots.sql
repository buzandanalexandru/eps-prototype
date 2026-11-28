-- Sale snapshots (step 3, design D3.1 to D3.3; P1). Publishing a storefront compiles, per event it
-- sells, an immutable snapshot of the products, pools, prices and rules, which the Go core reads
-- instead of the draft catalogue. Organiser data: organiser_id NOT NULL, forced row-level security,
-- composite foreign keys. The document is stored as the exact text that was hashed (document_text,
-- so Go verifies the SHA-256 over the same bytes) with a generated jsonb copy for queries.

-- +goose Up
CREATE TABLE control.sale_snapshots (
    id                    uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id          uuid        NOT NULL,
    event_id              uuid        NOT NULL,
    version               int         NOT NULL CHECK (version >= 1),
    schema_version        int         NOT NULL CHECK (schema_version = 1),
    document_text         text        NOT NULL CHECK (octet_length(document_text) <= 4194304),
    document              jsonb       GENERATED ALWAYS AS (document_text::jsonb) STORED,
    -- SHA-256 of document_text (checked here, so a stored hash can never disagree with its bytes).
    sha256                text        NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    -- SHA-256 of the sale terms (the document without id, version and time): an unchanged event
    -- is not given a new version by a design-only publish.
    terms_sha256          text        NOT NULL CHECK (terms_sha256 ~ '^[0-9a-f]{64}$'),
    storefront_version_id uuid,
    published_by          text        REFERENCES control."user" ("id") ON DELETE SET NULL,
    published_at          timestamptz NOT NULL DEFAULT now(),
    CHECK (sha256 = encode(sha256(convert_to(document_text, 'UTF8')), 'hex')),
    -- The document names itself: its id, organiser, event and version are the row's.
    CHECK ((document_text::jsonb ->> 'snapshotId') = id::text
           AND (document_text::jsonb ->> 'organiserId') = organiser_id::text
           AND (document_text::jsonb ->> 'eventId') = event_id::text
           AND (document_text::jsonb ->> 'version') = version::text
           AND (document_text::jsonb ->> 'schemaVersion') = schema_version::text),
    UNIQUE (organiser_id, id),
    UNIQUE (event_id, version),
    UNIQUE (organiser_id, event_id, version),
    -- Snapshots outlive nothing they describe: an event with snapshots cannot be deleted.
    FOREIGN KEY (organiser_id, event_id) REFERENCES control.events (organiser_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (organiser_id, storefront_version_id) REFERENCES control.storefront_versions (organiser_id, id)
);
CREATE INDEX sale_snapshots_published_by_idx ON control.sale_snapshots (published_by);
CREATE INDEX sale_snapshots_storefront_version_idx ON control.sale_snapshots (organiser_id, storefront_version_id);

-- The live snapshot version of each event: null before its first publish and while it is off sale
-- (a publish of its brand's shop that no longer sells it takes it off sale).
ALTER TABLE control.events ADD COLUMN live_snapshot_version int;
-- How long a checkout hold lasts (minutes, 5 to 15; step 4), carried in the snapshot's rules.
ALTER TABLE control.events ADD COLUMN hold_minutes int NOT NULL DEFAULT 8 CHECK (hold_minutes BETWEEN 5 AND 15);
-- Sample data: a seeded line-up entry is labelled as a sample artist by the shop, in its language.
ALTER TABLE control.lineup_slots ADD COLUMN sample boolean NOT NULL DEFAULT false;
ALTER TABLE control.events
    ADD CONSTRAINT events_live_snapshot_fkey FOREIGN KEY (organiser_id, id, live_snapshot_version)
    REFERENCES control.sale_snapshots (organiser_id, event_id, version);

-- Immutable: no role may change or delete a snapshot, nor empty the table.
-- +goose StatementBegin
CREATE FUNCTION control.sale_snapshots_immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, pg_temp
AS $fn$
BEGIN
    IF TG_OP = 'UPDATE' AND NEW.published_by IS NULL AND OLD.published_by IS NOT NULL
       AND NEW.id = OLD.id AND NEW.document_text = OLD.document_text AND NEW.sha256 = OLD.sha256
       AND NEW.terms_sha256 = OLD.terms_sha256 AND NEW.version = OLD.version AND NEW.event_id = OLD.event_id
       AND NEW.organiser_id = OLD.organiser_id AND NEW.published_at = OLD.published_at
       AND NEW.storefront_version_id IS NOT DISTINCT FROM OLD.storefront_version_id THEN
        RETURN NEW; -- only the author may be cleared (a deleted user, GDPR)
    END IF;
    RAISE EXCEPTION 'sale snapshots are immutable' USING ERRCODE = 'P0001';
END
$fn$;
-- +goose StatementEnd
CREATE TRIGGER sale_snapshots_immutable BEFORE UPDATE OR DELETE ON control.sale_snapshots
    FOR EACH ROW EXECUTE FUNCTION control.sale_snapshots_immutable();
CREATE TRIGGER sale_snapshots_no_truncate BEFORE TRUNCATE ON control.sale_snapshots
    FOR EACH STATEMENT EXECUTE FUNCTION control.sale_snapshots_immutable();

ALTER TABLE control.sale_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE control.sale_snapshots FORCE ROW LEVEL SECURITY;
CREATE POLICY organiser_isolation ON control.sale_snapshots
    USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
    WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid);
-- The web role writes snapshots and reads them; it never changes or deletes one.
REVOKE UPDATE, DELETE, TRUNCATE ON control.sale_snapshots FROM eps_web;
GRANT UPDATE (published_by) ON control.sale_snapshots TO eps_web;

-- Two narrow published-only reads (D3.3), each a SECURITY DEFINER function owned by its own NOLOGIN
-- NOBYPASSRLS role (spec-map rule), which may read only the columns the function returns:
--   live_snapshot(event, version): the live snapshot of an event, or one earlier version of it;
--     the Go core's only way to sale terms (eps_core) and the shop's (eps_web).
--   public_storefront(slug, hostname): a brand's public fields and live storefront version, by its
--     slug or one of its hostnames; nothing for a brand without a published version.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_snapshot_read') THEN
        CREATE ROLE eps_snapshot_read NOLOGIN NOBYPASSRLS NOINHERIT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_shop_read') THEN
        CREATE ROLE eps_shop_read NOLOGIN NOBYPASSRLS NOINHERIT;
    END IF;
END
$$;
-- +goose StatementEnd
GRANT eps_snapshot_read TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;
GRANT eps_shop_read TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;
GRANT USAGE ON SCHEMA control TO eps_snapshot_read, eps_shop_read;

GRANT SELECT (id, organiser_id, event_id, version, document_text, sha256, published_at) ON control.sale_snapshots TO eps_snapshot_read;
CREATE POLICY snapshot_read ON control.sale_snapshots FOR SELECT TO eps_snapshot_read USING (true);
GRANT SELECT (id, live_snapshot_version) ON control.events TO eps_snapshot_read;
CREATE POLICY snapshot_read ON control.events FOR SELECT TO eps_snapshot_read USING (live_snapshot_version IS NOT NULL);

GRANT SELECT (id, organiser_id, slug, name, languages, currency) ON control.brands TO eps_shop_read;
CREATE POLICY shop_read ON control.brands FOR SELECT TO eps_shop_read USING (true);
GRANT SELECT (hostname, brand_id) ON control.brand_hostnames TO eps_shop_read;
CREATE POLICY shop_read ON control.brand_hostnames FOR SELECT TO eps_shop_read USING (true);
GRANT SELECT (id, name, is_sample) ON control.organisers TO eps_shop_read;
CREATE POLICY shop_read ON control.organisers FOR SELECT TO eps_shop_read USING (true);
GRANT SELECT (brand_id, live_version_id) ON control.storefronts TO eps_shop_read;
CREATE POLICY shop_read ON control.storefronts FOR SELECT TO eps_shop_read USING (live_version_id IS NOT NULL);
GRANT SELECT (id, number, document, published_at, state) ON control.storefront_versions TO eps_shop_read;
CREATE POLICY shop_read ON control.storefront_versions FOR SELECT TO eps_shop_read USING (state <> 'draft');

-- +goose StatementBegin
CREATE FUNCTION control.live_snapshot(p_event uuid, p_version int DEFAULT NULL)
RETURNS TABLE (snapshot_id uuid, organiser_id uuid, event_id uuid, version int, document_text text, sha256 text, published_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    -- The live version, or an earlier one (orders in flight); never a version above the live one,
    -- and nothing at all for an event that is off sale (live_snapshot_version null).
    SELECT s.id, s.organiser_id, s.event_id, s.version, s.document_text, s.sha256, s.published_at
    FROM control.sale_snapshots s
    JOIN control.events e ON e.id = s.event_id
    WHERE s.event_id = p_event
      AND e.live_snapshot_version IS NOT NULL
      AND s.version = coalesce(p_version, e.live_snapshot_version)
      AND s.version <= e.live_snapshot_version
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION control.public_storefront(p_brand_slug text, p_hostname text)
RETURNS TABLE (brand_id uuid, organiser_id uuid, brand_slug text, brand_name text, currency text, languages text[],
               organiser_name text, is_sample boolean, version_id uuid, version_number int, published_at timestamptz,
               document jsonb, hostnames text[])
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT b.id, b.organiser_id, b.slug, b.name, b.currency, b.languages, o.name, o.is_sample,
           v.id, v.number, v.published_at, v.document,
           coalesce((SELECT array_agg(h.hostname ORDER BY h.hostname) FROM control.brand_hostnames h WHERE h.brand_id = b.id), '{}')
    FROM control.brands b
    JOIN control.organisers o ON o.id = b.organiser_id
    JOIN control.storefronts s ON s.brand_id = b.id
    JOIN control.storefront_versions v ON v.id = s.live_version_id AND v.state = 'published'
    WHERE (p_brand_slug IS NOT NULL AND b.slug = p_brand_slug)
       OR (p_hostname IS NOT NULL AND b.id = (SELECT h.brand_id FROM control.brand_hostnames h WHERE h.hostname = p_hostname))
    LIMIT 1
$fn$;
-- +goose StatementEnd

-- What a public shop page may read of a live snapshot: the document without the organiser's side
-- of each price (organiserNetMinor), the fee settings and hidden products.
-- +goose StatementBegin
CREATE FUNCTION control.public_snapshot(p_event uuid)
RETURNS TABLE (event_id uuid, organiser_id uuid, version int, document jsonb)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT s.event_id, s.organiser_id, s.version,
           jsonb_set(s.document - 'fee', '{products}', coalesce((
               SELECT jsonb_agg((p - 'priceSteps') || jsonb_build_object('priceSteps',
                          (SELECT coalesce(jsonb_agg(st - 'organiserNetMinor' ORDER BY (st ->> 'index')::int), '[]'::jsonb)
                           FROM jsonb_array_elements(p -> 'priceSteps') st))
                      ORDER BY ord)
               FROM jsonb_array_elements(s.document -> 'products') WITH ORDINALITY AS x(p, ord)
               WHERE (p ->> 'visible')::boolean), '[]'::jsonb))
    FROM control.sale_snapshots s
    JOIN control.events e ON e.id = s.event_id AND s.version = e.live_snapshot_version
    WHERE s.event_id = p_event
$fn$;
-- +goose StatementEnd
GRANT SELECT (event_id, organiser_id, version, document) ON control.sale_snapshots TO eps_snapshot_read;

REVOKE ALL ON FUNCTION control.public_snapshot(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION control.public_snapshot(uuid) TO eps_web;
REVOKE ALL ON FUNCTION control.live_snapshot(uuid, int) FROM PUBLIC;
REVOKE ALL ON FUNCTION control.public_storefront(text, text) FROM PUBLIC;
-- Only the core reads sale terms through live_snapshot; the web app's public pages use public_snapshot.
GRANT EXECUTE ON FUNCTION control.live_snapshot(uuid, int) TO eps_core;
GRANT EXECUTE ON FUNCTION control.public_storefront(text, text) TO eps_web;
GRANT CREATE ON SCHEMA control TO eps_snapshot_read, eps_shop_read;
ALTER FUNCTION control.live_snapshot(uuid, int) OWNER TO eps_snapshot_read;
ALTER FUNCTION control.public_snapshot(uuid) OWNER TO eps_snapshot_read;
ALTER FUNCTION control.public_storefront(text, text) OWNER TO eps_shop_read;
REVOKE CREATE ON SCHEMA control FROM eps_snapshot_read, eps_shop_read;
