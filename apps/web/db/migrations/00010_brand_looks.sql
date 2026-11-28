-- The trip wallet's brand looks (step 7, design D9): each event card wears its organiser's brand,
-- so the wallet reads a brand's name, slug and published design tokens by brand id. Published data
-- only (the live storefront version's tokens, as the public shop shows them), through a narrow
-- SECURITY DEFINER function owned by eps_shop_read (spec-map rule), for at most 50 brands at once.
-- No table changes and no locks on existing tables.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION control.public_brand_looks(p_brands uuid[])
RETURNS TABLE (brand_id uuid, brand_slug text, brand_name text, tokens jsonb)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT b.id, b.slug, b.name, v.document -> 'tokens'
    FROM control.brands b
    JOIN control.storefronts s ON s.brand_id = b.id
    JOIN control.storefront_versions v ON v.id = s.live_version_id AND v.state = 'published'
    WHERE b.id = ANY (p_brands[1:50])
$fn$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION control.public_brand_looks(uuid[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION control.public_brand_looks(uuid[]) TO eps_web;
GRANT CREATE ON SCHEMA control TO eps_shop_read;
ALTER FUNCTION control.public_brand_looks(uuid[]) OWNER TO eps_shop_read;
REVOKE CREATE ON SCHEMA control FROM eps_shop_read;
