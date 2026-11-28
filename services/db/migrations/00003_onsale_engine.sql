-- On-sale engine (step 5; E1-E6, E12, O1, O2). Organiser data like every sales table: organiser_id
-- NOT NULL, row-level security enabled and forced, one organiser policy, composite foreign keys.
-- Additive only: the new order columns are nullable, so step 4's code keeps working.

-- +goose Up
-- A sale moment (presale, general sale) of one event. Caps per person count across all of them.
CREATE TABLE sales.sales (
    id              uuid        PRIMARY KEY,
    organiser_id    uuid        NOT NULL,
    event_id        uuid        NOT NULL,
    name            text        NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    settings_version int        NOT NULL DEFAULT 1 CHECK (settings_version >= 1),
    -- The shuffle's audit record (D-2): the seed of the permutation and how many it ordered.
    shuffle_seed    text,
    shuffled_count  int,
    shuffled_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      text        NOT NULL,
    UNIQUE (organiser_id, id)
);
CREATE INDEX sales_event_idx ON sales.sales (organiser_id, event_id);

-- Every save is a new version (who and when), never an update (E1).
CREATE TABLE sales.sale_settings (
    organiser_id     uuid        NOT NULL,
    sale_id          uuid        NOT NULL,
    version          int         NOT NULL CHECK (version >= 1),
    opens_at         timestamptz NOT NULL,
    prequeue_minutes int         NOT NULL CHECK (prequeue_minutes BETWEEN 5 AND 120),
    queue_on         boolean     NOT NULL,
    rate_per_minute  int         NOT NULL CHECK (rate_per_minute BETWEEN 10 AND 60000),
    active_limit     int         NOT NULL CHECK (active_limit BETWEEN 1 AND 100000),
    paused           boolean     NOT NULL DEFAULT false,
    hold_minutes     int         NOT NULL CHECK (hold_minutes BETWEEN 5 AND 15),
    cap_per_order    int         NOT NULL CHECK (cap_per_order BETWEEN 1 AND 20),
    cap_per_person   int         NOT NULL CHECK (cap_per_person BETWEEN 1 AND 50),
    bot_defence      boolean     NOT NULL,
    otp_on           boolean     NOT NULL,
    otp_threshold    int         NOT NULL CHECK (otp_threshold BETWEEN 1 AND 20),
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       text        NOT NULL,
    PRIMARY KEY (organiser_id, sale_id, version),
    FOREIGN KEY (organiser_id, sale_id) REFERENCES sales.sales (organiser_id, id) ON DELETE CASCADE
);

-- Audit of positions and admissions, written in batches by the worker (never per fan on the hot
-- path). The identity is a hash: the waiting room keeps no names or emails.
CREATE TABLE sales.queue_entries (
    organiser_id  uuid        NOT NULL,
    sale_id       uuid        NOT NULL,
    qid           text        NOT NULL CHECK (length(qid) BETWEEN 8 AND 64),
    identity_hash text        NOT NULL,
    joined_at     timestamptz NOT NULL,
    position      bigint,
    admitted_at   timestamptz,
    bot_score     int,
    PRIMARY KEY (organiser_id, sale_id, qid),
    FOREIGN KEY (organiser_id, sale_id) REFERENCES sales.sales (organiser_id, id) ON DELETE CASCADE
);

-- Tickets per person per event at payment (the record of truth behind the Redis cap check).
CREATE TABLE sales.cap_ledger (
    organiser_id  uuid        NOT NULL,
    event_id      uuid        NOT NULL,
    identity_hash text        NOT NULL,
    order_id      uuid        NOT NULL,
    tickets       int         NOT NULL CHECK (tickets >= 0),
    at            timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organiser_id, order_id),
    FOREIGN KEY (organiser_id, order_id) REFERENCES sales.orders (organiser_id, id) ON DELETE CASCADE
);
CREATE INDEX cap_ledger_identity_idx ON sales.cap_ledger (organiser_id, event_id, identity_hash);

-- One-time codes asked before payment (E12, simulated): outcomes only, never the code.
CREATE TABLE sales.otp_challenges (
    organiser_id uuid        NOT NULL,
    order_id     uuid        NOT NULL,
    seq          bigserial,
    outcome      text        NOT NULL CHECK (outcome IN ('sent', 'verified', 'wrong', 'expired', 'locked')),
    at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organiser_id, order_id, seq),
    FOREIGN KEY (organiser_id, order_id) REFERENCES sales.orders (organiser_id, id) ON DELETE CASCADE
);

-- Every reconciler correction (D-10).
CREATE TABLE sales.inventory_adjustments (
    organiser_id uuid        NOT NULL,
    event_id     uuid        NOT NULL,
    seq          bigserial,
    instance_id  text        NOT NULL,
    shard        int,
    kind         text        NOT NULL CHECK (kind IN ('shard_invariant', 'allocation', 'sold_raised', 'oversell_risk', 'unpaid_commit')),
    delta        bigint      NOT NULL,
    detail       text        NOT NULL DEFAULT '',
    at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organiser_id, event_id, seq)
);

-- The sale an order was held under, the queue place whose pass it used, the cap identity (a hash
-- of the account id or normalised guest email) and the sale's caps, and the OTP decision.
ALTER TABLE sales.orders ADD COLUMN sale_id uuid;
ALTER TABLE sales.orders ADD COLUMN pass_qid text;
ALTER TABLE sales.orders ADD COLUMN cap_identity text;
ALTER TABLE sales.orders ADD COLUMN cap_per_person int;
ALTER TABLE sales.orders ADD COLUMN cap_tickets int;
ALTER TABLE sales.orders ADD COLUMN otp_required boolean NOT NULL DEFAULT false;
ALTER TABLE sales.orders ADD COLUMN otp_verified_at timestamptz;
CREATE INDEX orders_cap_identity_idx ON sales.orders (organiser_id, event_id, cap_identity) WHERE cap_identity IS NOT NULL;

-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['sales', 'sale_settings', 'queue_entries', 'cap_ledger', 'otp_challenges', 'inventory_adjustments']
    LOOP
        EXECUTE format('ALTER TABLE sales.%I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE sales.%I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY organiser_isolation ON sales.%I
            USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
            WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)$p$, t);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON sales.%I TO eps_core', t);
        EXECUTE format('REVOKE ALL ON sales.%I FROM eps_web', t);
    END LOOP;
END
$$;
-- +goose StatementEnd
-- Adjustments and OTP outcomes are append-only.
REVOKE UPDATE, DELETE, TRUNCATE ON sales.inventory_adjustments, sales.otp_challenges FROM eps_core;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA sales TO eps_core;

-- +goose StatementBegin
-- The organiser and event of a sale (the worker's queue audit and the gateway's directory reload
-- find a sale by id; the work then runs under that organiser's scope).
CREATE FUNCTION sales.sale_owner(p_sale uuid)
RETURNS TABLE (organiser_id uuid, event_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$ SELECT s.organiser_id, s.event_id FROM sales.sales s WHERE s.id = p_sale $fn$;
-- +goose StatementEnd
GRANT SELECT (id, organiser_id, event_id) ON sales.sales TO eps_sales_index;
CREATE POLICY sales_index ON sales.sales FOR SELECT TO eps_sales_index USING (true);
REVOKE ALL ON FUNCTION sales.sale_owner(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sales.sale_owner(uuid) TO eps_core;
GRANT CREATE ON SCHEMA sales TO eps_sales_index;
ALTER FUNCTION sales.sale_owner(uuid) OWNER TO eps_sales_index;
REVOKE CREATE ON SCHEMA sales FROM eps_sales_index;

-- The reconciler's and the waiting room's events get their own topics.
ALTER TABLE sales.outbox DROP CONSTRAINT outbox_topic_check;
ALTER TABLE sales.outbox ADD CONSTRAINT outbox_topic_check CHECK (topic IN ('orders', 'payments', 'wallet', 'inventory', 'sales'));

-- +goose StatementBegin
-- Events with a sale opening (or opened) around now: the worker keeps their sales' signed copies in
-- Redis, so the gateway (which has no database) always finds them.
CREATE FUNCTION sales.events_with_sales(p_from timestamptz, p_to timestamptz)
RETURNS TABLE (organiser_id uuid, event_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT DISTINCT s.organiser_id, s.event_id FROM sales.sales s
    JOIN sales.sale_settings v ON v.organiser_id = s.organiser_id AND v.sale_id = s.id AND v.version = s.settings_version
    WHERE v.opens_at BETWEEN p_from AND p_to LIMIT 1000
$fn$;
-- +goose StatementEnd
GRANT SELECT (id, organiser_id, event_id, settings_version) ON sales.sales TO eps_sales_index;
GRANT SELECT (organiser_id, sale_id, version, opens_at) ON sales.sale_settings TO eps_sales_index;
CREATE POLICY sales_index ON sales.sale_settings FOR SELECT TO eps_sales_index USING (true);
REVOKE ALL ON FUNCTION sales.events_with_sales(timestamptz, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sales.events_with_sales(timestamptz, timestamptz) TO eps_core;
GRANT CREATE ON SCHEMA sales TO eps_sales_index;
ALTER FUNCTION sales.events_with_sales(timestamptz, timestamptz) OWNER TO eps_sales_index;
REVOKE CREATE ON SCHEMA sales FROM eps_sales_index;
