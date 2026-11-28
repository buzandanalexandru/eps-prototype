-- Tickets, the door feed, door devices and scans (step 7; F2 to F5, F8, G2, O5). Additive only:
-- new tables, column grants and SECURITY DEFINER functions. Every table is organiser data:
-- organiser_id NOT NULL, row-level security enabled and forced, one organiser policy, composite
-- foreign keys (organiser_id, id).
--
-- Lesson from step 6's production deploy (its migration deadlocked against the live worker): the
-- only existing table this migration needs a lock on (sales.orders: the tickets' foreign key and
-- the partial index) is locked first, in one statement, with a short lock timeout; cmd/migrate
-- retries the whole migration on a lock timeout or a deadlock (SQLSTATE 55P03 / 40P01).

-- +goose Up
SET LOCAL lock_timeout = '5s';
LOCK TABLE sales.orders IN ACCESS EXCLUSIVE MODE;

-- One ticket per paid unit (design D1): the unit key (order, line, unit) makes issuing idempotent.
CREATE TABLE sales.tickets (
    id               uuid        PRIMARY KEY,
    organiser_id     uuid        NOT NULL,
    event_id         uuid        NOT NULL,
    order_id         uuid        NOT NULL,
    line_no          int         NOT NULL CHECK (line_no >= 1),
    unit_no          int         NOT NULL CHECK (unit_no >= 1),
    product_id       uuid        NOT NULL,
    instance_id      text        CHECK (instance_id IS NULL OR length(instance_id) <= 200),
    snapshot_version int         NOT NULL CHECK (snapshot_version >= 1),
    -- Reissuing (claim, day pick, later transfer, resale, upgrade) bumps the generation (D2: uint16).
    generation       int         NOT NULL DEFAULT 1 CHECK (generation BETWEEN 1 AND 65535),
    holder_kind      text        NOT NULL CHECK (holder_kind IN ('account', 'guest', 'erased')),
    holder_id        text        NOT NULL CHECK (length(holder_id) BETWEEN 1 AND 200),
    -- The holder before the last reissue: they are told the ticket has moved instead of "not found".
    prev_holder_id   text        CHECK (prev_holder_id IS NULL OR length(prev_holder_id) <= 200),
    state            text        NOT NULL DEFAULT 'valid' CHECK (state IN ('valid', 'suspended', 'cancelled')),
    cancel_reason    text        CHECK (cancel_reason IS NULL OR length(cancel_reason) <= 40),
    holder_name      text        CHECK (holder_name IS NULL OR length(holder_name) BETWEEN 1 AND 120),
    picked_day       date,
    checked_in_at    timestamptz,
    -- The wallet entry that paid a late check-in (O6).
    checkin_entry_id uuid,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organiser_id, id),
    UNIQUE (organiser_id, order_id, line_no, unit_no),
    FOREIGN KEY (organiser_id, order_id) REFERENCES sales.orders (organiser_id, id)
);
CREATE INDEX tickets_holder_idx ON sales.tickets (holder_id, organiser_id);
CREATE INDEX tickets_event_idx ON sales.tickets (organiser_id, event_id, id);
CREATE INDEX tickets_order_idx ON sales.tickets (organiser_id, order_id);

-- Paid orders not issued yet (the issuer's backstop for a lost event).
CREATE INDEX orders_paid_not_issued_idx ON sales.orders (paid_at) WHERE state = 'paid';

-- The door feed (D6): per event, one ordered sequence of revocations, suspensions and their
-- lifting, entries, conflicts and manifest changes. Its sequence is taken from the event's counter
-- row under a row lock, so feed rows commit in sequence order and a reader at cursor N has seen
-- every row up to N.
CREATE TABLE sales.door_counters (
    organiser_id uuid   NOT NULL,
    event_id     uuid   NOT NULL,
    seq          bigint NOT NULL DEFAULT 0 CHECK (seq >= 0),
    PRIMARY KEY (organiser_id, event_id)
);

CREATE TABLE sales.door_feed (
    organiser_id uuid        NOT NULL,
    event_id     uuid        NOT NULL,
    seq          bigint      NOT NULL CHECK (seq >= 1),
    kind         text        NOT NULL CHECK (kind IN ('revoked', 'suspended', 'lifted', 'entry', 'conflict', 'manifest')),
    ticket_id    uuid        NOT NULL,
    -- revoked: generations below this are invalid; manifest/entry: the generation concerned.
    generation   int         CHECK (generation IS NULL OR generation BETWEEN 1 AND 65535),
    data         jsonb       NOT NULL DEFAULT '{}'::jsonb,
    at           timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (organiser_id, event_id, seq),
    FOREIGN KEY (organiser_id, ticket_id) REFERENCES sales.tickets (organiser_id, id)
);

-- A door phone prepared for an event, with the gate it stands at.
CREATE TABLE sales.door_devices (
    id           uuid        PRIMARY KEY,
    organiser_id uuid        NOT NULL,
    event_id     uuid        NOT NULL,
    gate         text        NOT NULL CHECK (length(gate) BETWEEN 1 AND 40),
    created_by   text        NOT NULL CHECK (length(created_by) BETWEEN 1 AND 200),
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_sync_at timestamptz,
    UNIQUE (organiser_id, id)
);
CREATE INDEX door_devices_event_idx ON sales.door_devices (organiser_id, event_id);

-- Scans uploaded by door devices (D8), idempotent by the device's scan id. One admitted scan per
-- ticket; a later duplicate (by device time) is a conflict.
CREATE TABLE sales.ticket_scans (
    organiser_id uuid        NOT NULL,
    id           uuid        NOT NULL,
    event_id     uuid        NOT NULL,
    ticket_id    uuid        NOT NULL,
    generation   int         NOT NULL CHECK (generation BETWEEN 1 AND 65535),
    device_id    uuid        NOT NULL,
    gate         text        NOT NULL CHECK (length(gate) BETWEEN 1 AND 40),
    scanned_at   timestamptz NOT NULL,
    received_at  timestamptz NOT NULL DEFAULT now(),
    result       text        NOT NULL CHECK (result IN ('admitted', 'conflict')),
    PRIMARY KEY (organiser_id, id),
    FOREIGN KEY (organiser_id, ticket_id) REFERENCES sales.tickets (organiser_id, id),
    FOREIGN KEY (organiser_id, device_id) REFERENCES sales.door_devices (organiser_id, id)
);
CREATE UNIQUE INDEX ticket_scans_one_admitted ON sales.ticket_scans (organiser_id, ticket_id) WHERE result = 'admitted';
CREATE INDEX ticket_scans_event_idx ON sales.ticket_scans (organiser_id, event_id);

-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['tickets', 'door_counters', 'door_feed', 'door_devices', 'ticket_scans']
    LOOP
        EXECUTE format('ALTER TABLE sales.%I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE sales.%I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY organiser_isolation ON sales.%I
            USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
            WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)$p$, t);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE ON sales.%I TO eps_core', t);
        EXECUTE format('REVOKE ALL ON sales.%I FROM eps_web', t);
    END LOOP;
END
$$;
-- +goose StatementEnd
-- The feed and scans are append-only (a conflict is a new row, never an edit of the first entry's
-- feed row); a scan's result may change once, when an earlier scan by device time arrives.
REVOKE UPDATE ON sales.door_feed FROM eps_core;

-- The index role reads ids, organisers, holders and states only (spec-map SECURITY DEFINER rule).
GRANT SELECT (id, organiser_id, holder_id, order_id) ON sales.tickets TO eps_sales_index;
CREATE POLICY sales_index ON sales.tickets FOR SELECT TO eps_sales_index USING (true);
GRANT SELECT (rehearsal, paid_at) ON sales.orders TO eps_sales_index;
GRANT SELECT (owner_id, organiser_id) ON sales.saved_payment_methods TO eps_sales_index;
CREATE POLICY sales_index ON sales.saved_payment_methods FOR SELECT TO eps_sales_index USING (true);

-- +goose StatementBegin
-- The organiser of a ticket (a holder opens a ticket by its id; the core then reads it under that
-- organiser's scope and checks the caller holds it).
CREATE FUNCTION sales.ticket_organiser(p_ticket uuid) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$ SELECT organiser_id FROM sales.tickets WHERE id = p_ticket $fn$;
-- +goose StatementEnd
-- +goose StatementBegin
-- The organisers whose tickets a holder holds (the trip wallet; the tickets are then read under
-- each organiser's scope).
CREATE FUNCTION sales.holder_organisers(p_holder text)
RETURNS TABLE (organiser_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$ SELECT DISTINCT t.organiser_id FROM sales.tickets t WHERE t.holder_id = p_holder LIMIT 200 $fn$;
-- +goose StatementEnd
-- +goose StatementBegin
-- Paid orders still without tickets p_age after payment (the issuer's backstop for a lost event).
CREATE FUNCTION sales.paid_orders_unissued(p_age interval, p_limit int)
RETURNS TABLE (organiser_id uuid, order_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT o.organiser_id, o.id FROM sales.orders o
    WHERE o.state = 'paid' AND NOT o.rehearsal AND o.paid_at <= now() - p_age
    ORDER BY o.paid_at LIMIT least(greatest(p_limit, 1), 500)
$fn$;
-- +goose StatementEnd
-- +goose StatementBegin
-- Orders marked issued by step 4's stub, which made no tickets (the one-off backfill), a page at a
-- time in id order after p_after.
CREATE FUNCTION sales.issued_orders_without_tickets(p_after uuid, p_limit int)
RETURNS TABLE (organiser_id uuid, order_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT o.organiser_id, o.id FROM sales.orders o
    WHERE o.state = 'issued' AND NOT o.rehearsal AND o.id > p_after
      AND NOT EXISTS (SELECT 1 FROM sales.tickets t WHERE t.organiser_id = o.organiser_id AND t.order_id = o.id)
    ORDER BY o.id LIMIT least(greatest(p_limit, 1), 500)
$fn$;
-- +goose StatementEnd
-- +goose StatementBegin
-- The organisers holding a person's sales data (the GDPR erasure, run under each organiser's
-- scope): orders bought by the person, guest orders placed with the person's verified email,
-- tickets they hold, their wallets and saved cards.
CREATE FUNCTION sales.person_organisers(p_buyer text, p_email text)
RETURNS TABLE (organiser_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT o.organiser_id FROM sales.orders o WHERE o.buyer_id = p_buyer
    UNION SELECT o.organiser_id FROM sales.orders o WHERE p_email <> '' AND o.buyer_kind = 'guest' AND o.buyer_email = lower(btrim(p_email))
    UNION SELECT t.organiser_id FROM sales.tickets t WHERE t.holder_id = p_buyer
    UNION SELECT w.organiser_id FROM sales.wallet_accounts w WHERE w.owner = p_buyer
    UNION SELECT m.organiser_id FROM sales.saved_payment_methods m WHERE m.owner_id = p_buyer
$fn$;
-- +goose StatementEnd
-- +goose StatementBegin
-- Sale moments opened within the last 400 days or later (the erasure deletes the person's waiting-
-- room audit rows, stored under a keyed hash of sale and subject): ids and organisers only.
CREATE FUNCTION sales.recent_sales(p_limit int)
RETURNS TABLE (organiser_id uuid, sale_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT s.organiser_id, s.id FROM sales.sales s WHERE s.created_at > now() - interval '400 days'
    ORDER BY s.created_at DESC LIMIT least(greatest(p_limit, 1), 5000)
$fn$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION sales.ticket_organiser(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.holder_organisers(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.paid_orders_unissued(interval, int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.issued_orders_without_tickets(uuid, int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.person_organisers(text, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.recent_sales(int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sales.ticket_organiser(uuid), sales.holder_organisers(text), sales.paid_orders_unissued(interval, int),
    sales.issued_orders_without_tickets(uuid, int), sales.person_organisers(text, text), sales.recent_sales(int) TO eps_core;
GRANT CREATE ON SCHEMA sales TO eps_sales_index;
ALTER FUNCTION sales.ticket_organiser(uuid) OWNER TO eps_sales_index;
ALTER FUNCTION sales.holder_organisers(text) OWNER TO eps_sales_index;
ALTER FUNCTION sales.paid_orders_unissued(interval, int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.issued_orders_without_tickets(uuid, int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.person_organisers(text, text) OWNER TO eps_sales_index;
ALTER FUNCTION sales.recent_sales(int) OWNER TO eps_sales_index;
REVOKE CREATE ON SCHEMA sales FROM eps_sales_index;
