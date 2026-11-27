-- The catalogue (step 1, design D1.4; C1 to C5, C8, C9). Draft side only: step 3 compiles it into
-- sale snapshots and the Go core never reads these tables (P1).
-- Every table carries organiser_id NOT NULL with forced row-level security, and every link between
-- catalogue rows is a composite foreign key that includes organiser_id, so a row can never point
-- at another organiser's row even if application code tried.
-- Fan-facing text is jsonb {"ro": "...", "en": "..."}; money is integer minor units.

-- +goose Up
CREATE TABLE control.venues (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL REFERENCES control.organisers (id) ON DELETE CASCADE,
    name         text        NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    address      text        NOT NULL DEFAULT '',
    lat          double precision CHECK (lat BETWEEN -90 AND 90),
    lng          double precision CHECK (lng BETWEEN -180 AND 180),
    stages       text[]      NOT NULL DEFAULT '{}',
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organiser_id, id)
);

CREATE TABLE control.events (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id     uuid        NOT NULL REFERENCES control.organisers (id) ON DELETE CASCADE,
    brand_id         uuid        NOT NULL,
    venue_id         uuid,
    slug             text        NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,58}[a-z0-9]$'),
    name             jsonb       NOT NULL DEFAULT '{}',
    description      jsonb       NOT NULL DEFAULT '{}',
    timezone         text        NOT NULL DEFAULT 'Europe/Bucharest',
    starts_at        timestamptz NOT NULL,
    ends_at          timestamptz NOT NULL,
    status           text        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    edition_of       uuid,
    fee_policy       text        CHECK (fee_policy IN ('absorb', 'pass_on', 'split')),
    min_age          int         CHECK (min_age BETWEEN 1 AND 99),
    consent_under_age int        CHECK (consent_under_age BETWEEN 1 AND 99),
    max_per_account  int         CHECK (max_per_account >= 1),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organiser_id, id),
    UNIQUE (brand_id, slug),
    CHECK (ends_at > starts_at),
    CHECK (consent_under_age IS NULL OR min_age IS NULL OR consent_under_age >= min_age),
    -- A brand with events cannot be deleted by accident (Studio deletes its drafts first, and
    -- refuses while it has published or archived events).
    FOREIGN KEY (organiser_id, brand_id) REFERENCES control.brands (organiser_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (organiser_id, venue_id) REFERENCES control.venues (organiser_id, id),
    FOREIGN KEY (organiser_id, edition_of) REFERENCES control.events (organiser_id, id) ON DELETE SET NULL (edition_of)
);

-- A day belongs to its start date even when it closes after midnight (closes <= opens).
CREATE TABLE control.event_days (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL,
    event_id     uuid        NOT NULL,
    day          date        NOT NULL,
    opens        time        NOT NULL,
    closes       time        NOT NULL,
    UNIQUE (organiser_id, id),
    UNIQUE (organiser_id, event_id, id),
    UNIQUE (event_id, day),
    FOREIGN KEY (organiser_id, event_id) REFERENCES control.events (organiser_id, id) ON DELETE CASCADE
);

CREATE TABLE control.lineup_slots (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL,
    event_id     uuid        NOT NULL,
    day_id       uuid        NOT NULL,
    artist       text        NOT NULL CHECK (length(artist) BETWEEN 1 AND 120),
    stage        text        NOT NULL DEFAULT '',
    starts       time        NOT NULL,
    sort         int         NOT NULL DEFAULT 0,
    UNIQUE (organiser_id, id),
    FOREIGN KEY (organiser_id, event_id) REFERENCES control.events (organiser_id, id) ON DELETE CASCADE,
    -- The day must be a day of the same event.
    FOREIGN KEY (organiser_id, event_id, day_id) REFERENCES control.event_days (organiser_id, event_id, id) ON DELETE CASCADE
);

CREATE TABLE control.products (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id  uuid        NOT NULL,
    event_id      uuid        NOT NULL,
    type          text        NOT NULL CHECK (type IN ('pass', 'day_ticket', 'flex_day', 'add_on', 'voucher', 'table', 'time_slot')),
    name          jsonb       NOT NULL DEFAULT '{}',
    perks         jsonb       NOT NULL DEFAULT '[]',
    config        jsonb       NOT NULL DEFAULT '{}',
    named         boolean     NOT NULL DEFAULT false,
    sales_from    timestamptz,
    sales_until   timestamptz,
    visible       boolean     NOT NULL DEFAULT true,
    max_per_order int         NOT NULL DEFAULT 10 CHECK (max_per_order >= 1),
    sort          int         NOT NULL DEFAULT 0 CHECK (sort BETWEEN -10000 AND 10000),
    -- Ticket lifecycle settings used by step 7 (added now so step 7 needs no migration redo).
    personalise_deadline   timestamptz,
    checkin_deadline       timestamptz,
    late_checkin_fee_minor bigint CHECK (late_checkin_fee_minor >= 0),
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organiser_id, id),
    UNIQUE (organiser_id, event_id, id),
    CHECK (sales_from IS NULL OR sales_until IS NULL OR sales_until > sales_from),
    FOREIGN KEY (organiser_id, event_id) REFERENCES control.events (organiser_id, id) ON DELETE CASCADE
);

CREATE TABLE control.capacity_pools (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL,
    event_id     uuid        NOT NULL,
    name         text        NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    capacity     int         NOT NULL CHECK (capacity >= 0),
    scope        text        NOT NULL DEFAULT 'event' CHECK (scope IN ('event', 'day', 'slot')),
    UNIQUE (organiser_id, id),
    UNIQUE (organiser_id, event_id, id),
    FOREIGN KEY (organiser_id, event_id) REFERENCES control.events (organiser_id, id) ON DELETE CASCADE
);

-- A product and a pool it draws on must belong to the same event (and organiser).
CREATE TABLE control.product_pools (
    organiser_id uuid NOT NULL,
    event_id     uuid NOT NULL,
    product_id   uuid NOT NULL,
    pool_id      uuid NOT NULL,
    PRIMARY KEY (product_id, pool_id),
    FOREIGN KEY (organiser_id, event_id, product_id) REFERENCES control.products (organiser_id, event_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organiser_id, event_id, pool_id) REFERENCES control.capacity_pools (organiser_id, event_id, id) ON DELETE CASCADE
);

-- One ladder per product: by date (from_at) or by sales (after_sold), never both; the first step
-- (position 0) has no threshold.
CREATE TABLE control.price_steps (
    organiser_id uuid        NOT NULL,
    product_id   uuid        NOT NULL,
    position     int         NOT NULL CHECK (position >= 0),
    mode         text        NOT NULL CHECK (mode IN ('by_date', 'by_sales')),
    from_at      timestamptz,
    after_sold   int         CHECK (after_sold >= 1),
    price_minor  bigint      NOT NULL CHECK (price_minor >= 0),
    PRIMARY KEY (product_id, position),
    CHECK (CASE WHEN position = 0 THEN from_at IS NULL AND after_sold IS NULL
                WHEN mode = 'by_date' THEN from_at IS NOT NULL AND after_sold IS NULL
                ELSE after_sold IS NOT NULL AND from_at IS NULL END),
    FOREIGN KEY (organiser_id, product_id) REFERENCES control.products (organiser_id, id) ON DELETE CASCADE
);

-- The deposit is either a fixed amount (minor units) or a share of the all-in price (basis points).
-- monthly_count is the most monthly payments; each buyer gets as many as still end 7 days before
-- the event, counted from their purchase date.
CREATE TABLE control.instalment_plans (
    organiser_id  uuid NOT NULL,
    product_id    uuid PRIMARY KEY,
    deposit_minor bigint CHECK (deposit_minor >= 0),
    deposit_bps   int    CHECK (deposit_bps BETWEEN 0 AND 9999),
    monthly_count int    NOT NULL CHECK (monthly_count BETWEEN 1 AND 24),
    day_of_month  int    NOT NULL DEFAULT 1 CHECK (day_of_month BETWEEN 1 AND 28),
    CHECK (num_nonnulls(deposit_minor, deposit_bps) = 1),
    FOREIGN KEY (organiser_id, product_id) REFERENCES control.products (organiser_id, id) ON DELETE CASCADE
);

CREATE INDEX events_org_idx ON control.events (organiser_id, starts_at);
CREATE INDEX products_event_idx ON control.products (event_id, sort);
CREATE INDEX pools_event_idx ON control.capacity_pools (event_id);
CREATE INDEX product_pools_pool_idx ON control.product_pools (pool_id);
CREATE INDEX lineup_event_idx ON control.lineup_slots (event_id, day_id, stage, starts);
CREATE INDEX lineup_day_idx ON control.lineup_slots (day_id);
CREATE INDEX events_venue_idx ON control.events (venue_id);
CREATE INDEX events_edition_idx ON control.events (edition_of);
CREATE INDEX product_pools_event_idx ON control.product_pools (event_id);

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['venues', 'events', 'event_days', 'lineup_slots', 'products', 'capacity_pools',
                             'product_pools', 'price_steps', 'instalment_plans'] LOOP
        EXECUTE format('ALTER TABLE control.%I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE control.%I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY organiser_isolation ON control.%I
            USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
            WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)$p$, t);
    END LOOP;
END
$$;
-- +goose StatementEnd
