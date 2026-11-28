-- Checkout and payments (step 4; O3, O6, P3). Every table is organiser data: organiser_id NOT NULL,
-- row-level security enabled and forced, one organiser policy, composite foreign keys
-- (organiser_id, id) so a row can never point at another organiser's row.
--
-- Scans that must cross organisers (the worker's hold sweeper, payment reconciler, instalment
-- runner and outbox relay; a fan's own orders and wallets across organisers; the organiser of an
-- order a fan opens by id) go through narrow SECURITY DEFINER functions owned by the dedicated
-- NOLOGIN NOBYPASSRLS role eps_sales_index (spec-map rule), each returning only ids and the
-- organiser they belong to; the work itself then runs under eps_core with that organiser's scope.

-- +goose Up
CREATE TABLE sales.orders (
    id                  uuid        PRIMARY KEY,
    organiser_id        uuid        NOT NULL,
    event_id            uuid        NOT NULL,
    snapshot_version    int         NOT NULL CHECK (snapshot_version >= 1),
    buyer_kind          text        NOT NULL CHECK (buyer_kind IN ('account', 'guest')),
    buyer_id            text        NOT NULL CHECK (length(buyer_id) BETWEEN 1 AND 200),
    -- Set when the fan gives contact details; normalised (trimmed, lower case) for claims.
    buyer_email         text        CHECK (buyer_email IS NULL OR buyer_email = lower(btrim(buyer_email))),
    buyer_name          text        CHECK (buyer_name IS NULL OR length(buyer_name) <= 200),
    locale              text        NOT NULL DEFAULT 'ro' CHECK (locale IN ('ro', 'en')),
    currency            text        NOT NULL CHECK (currency IN ('RON', 'EUR')),
    state               text        NOT NULL CHECK (state IN ('held', 'paying', 'payment_failed', 'paid', 'issued', 'expired', 'cancelled', 'refunded')),
    total_minor         bigint      NOT NULL CHECK (total_minor >= 0),
    face_minor          bigint      NOT NULL CHECK (face_minor >= 0),
    fan_fee_minor       bigint      NOT NULL CHECK (fan_fee_minor >= 0),
    organiser_net_minor bigint      NOT NULL,
    -- Units taken per pool instance ({"<instanceId>": qty}), the hold's draw on stock.
    draws               jsonb       NOT NULL,
    hold_expires_at     timestamptz NOT NULL,
    -- The hold's own deadline (now + hold minutes at creation): a payment extends hold_expires_at
    -- only while its attempt is open, and only up to this plus the attempt limit and grace.
    hold_base_expires_at timestamptz NOT NULL,
    -- The inventory hold that carries the order's stock: the order id, or <id>_r<n> after a late
    -- payment re-took the stock.
    hold_ref            text        NOT NULL,
    pay_plan            text        NOT NULL DEFAULT 'full' CHECK (pay_plan IN ('full', 'instalments')),
    answers             jsonb       NOT NULL DEFAULT '{}'::jsonb,
    details_given       boolean     NOT NULL DEFAULT false,
    last_error_code     text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    paid_at             timestamptz,
    UNIQUE (organiser_id, id)
);
-- One live order (held, paying or payment failed) per buyer per sale (design D-7).
CREATE UNIQUE INDEX orders_one_live_per_buyer ON sales.orders (organiser_id, event_id, buyer_id)
    WHERE state IN ('held', 'paying', 'payment_failed');
CREATE INDEX orders_due_idx ON sales.orders (hold_expires_at) WHERE state IN ('held', 'payment_failed');
CREATE INDEX orders_event_state_idx ON sales.orders (organiser_id, event_id, state);
CREATE INDEX orders_buyer_idx ON sales.orders (buyer_id);
CREATE INDEX orders_email_idx ON sales.orders (buyer_email) WHERE buyer_kind = 'guest';

CREATE TABLE sales.order_lines (
    organiser_id             uuid   NOT NULL,
    order_id                 uuid   NOT NULL,
    line_no                  int    NOT NULL CHECK (line_no >= 1),
    product_id               uuid   NOT NULL,
    qty                      int    NOT NULL CHECK (qty BETWEEN 1 AND 100),
    instance_id              text,
    price_step_index         int    NOT NULL CHECK (price_step_index >= 0),
    unit_all_in_minor        bigint NOT NULL CHECK (unit_all_in_minor >= 0),
    unit_face_minor          bigint NOT NULL CHECK (unit_face_minor >= 0),
    unit_fan_fee_minor       bigint NOT NULL CHECK (unit_fan_fee_minor >= 0),
    unit_organiser_net_minor bigint NOT NULL,
    CHECK (unit_all_in_minor = unit_face_minor + unit_fan_fee_minor),
    PRIMARY KEY (organiser_id, order_id, line_no),
    FOREIGN KEY (organiser_id, order_id) REFERENCES sales.orders (organiser_id, id) ON DELETE CASCADE
);
CREATE INDEX order_lines_product_idx ON sales.order_lines (organiser_id, product_id);

CREATE TABLE sales.order_history (
    organiser_id uuid        NOT NULL,
    order_id     uuid        NOT NULL,
    seq          bigserial,
    from_state   text,
    to_state     text        NOT NULL,
    reason       text        NOT NULL DEFAULT '',
    at           timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (organiser_id, order_id, seq),
    FOREIGN KEY (organiser_id, order_id) REFERENCES sales.orders (organiser_id, id) ON DELETE CASCADE
);

-- Provider-saved cards (D13): EPS keeps only the provider, its token and a display label.
CREATE TABLE sales.saved_payment_methods (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL,
    owner_id     text        NOT NULL,
    provider     text        NOT NULL CHECK (provider IN ('a', 'b')),
    token        text        NOT NULL CHECK (token ~ '^pm_[ab]_[A-Za-z0-9]{8,64}$'),
    label        text        NOT NULL CHECK (length(label) <= 80),
    brand        text        NOT NULL DEFAULT 'visa',
    created_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz,
    UNIQUE (organiser_id, id),
    UNIQUE (organiser_id, provider, token)
);
CREATE INDEX saved_methods_owner_idx ON sales.saved_payment_methods (organiser_id, owner_id) WHERE deleted_at IS NULL;

-- Wallet accounts (D-11): fan_balance, fan_season_credit, psp_clearing, organiser_sales, stand_sales.
CREATE TABLE sales.wallet_accounts (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id  uuid        NOT NULL,
    kind          text        NOT NULL CHECK (kind IN ('fan_balance', 'fan_season_credit', 'psp_clearing', 'organiser_sales', 'stand_sales', 'credit_breakage')),
    owner         text        NOT NULL CHECK (length(owner) BETWEEN 1 AND 200),
    currency      text        NOT NULL CHECK (currency IN ('RON', 'EUR')),
    balance_minor bigint      NOT NULL DEFAULT 0,
    -- The fan's wallet code a stand types in (fan_balance only).
    code          text        UNIQUE CHECK (code IS NULL OR code ~ '^[A-Z0-9]{4}-[A-Z0-9]{4}$'),
    expires_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    -- A fan can never spend more than they hold.
    CONSTRAINT fan_balance_not_negative CHECK (kind NOT IN ('fan_balance', 'fan_season_credit') OR balance_minor >= 0),
    UNIQUE (organiser_id, id),
    UNIQUE (organiser_id, kind, owner, currency)
);

CREATE TABLE sales.journal_entries (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL,
    kind         text        NOT NULL CHECK (kind IN ('topup', 'order_payment', 'stand_payment', 'season_close', 'credit_expiry', 'reversal')),
    ref          text        NOT NULL DEFAULT '',
    memo         text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organiser_id, id)
);
CREATE INDEX journal_entries_ref_idx ON sales.journal_entries (organiser_id, ref);

CREATE TABLE sales.journal_lines (
    id            bigserial   PRIMARY KEY,
    organiser_id  uuid        NOT NULL,
    entry_id      uuid        NOT NULL,
    account_id    uuid        NOT NULL,
    amount_minor  bigint      NOT NULL CHECK (amount_minor <> 0),
    currency      text        NOT NULL CHECK (currency IN ('RON', 'EUR')),
    FOREIGN KEY (organiser_id, entry_id) REFERENCES sales.journal_entries (organiser_id, id),
    FOREIGN KEY (organiser_id, account_id) REFERENCES sales.wallet_accounts (organiser_id, id)
);
CREATE INDEX journal_lines_entry_idx ON sales.journal_lines (organiser_id, entry_id);
CREATE INDEX journal_lines_account_idx ON sales.journal_lines (organiser_id, account_id);

-- Payment attempts (D-2), for orders, wallet top-ups and instalment parts.
CREATE TABLE sales.payment_attempts (
    id              uuid        PRIMARY KEY,
    organiser_id    uuid        NOT NULL,
    purpose         text        NOT NULL CHECK (purpose IN ('order', 'topup', 'instalment')),
    order_id        uuid,
    wallet_account_id uuid,
    instalment_part int,
    buyer_id        text        NOT NULL,
    provider        text        NOT NULL CHECK (provider IN ('a', 'b')),
    -- The idempotency key sent to the provider: the attempt id, or inst:<plan>:<part>:<try>.
    provider_key    text        NOT NULL,
    method          text        NOT NULL CHECK (method IN ('hosted_card', 'apple_pay', 'google_pay', 'saved_card')),
    saved_method_id uuid,
    save_card       boolean     NOT NULL DEFAULT false,
    -- The plan this attempt pays for (D-b): an instalment deposit, or the full amount.
    plan            text        NOT NULL DEFAULT 'full' CHECK (plan IN ('full', 'instalments')),
    -- For a deposit: the schedule offered when it was sent (rebuilt if it succeeds late).
    offer           jsonb,
    -- Why a failed attempt failed (provider_unavailable, timeout, ...); provider_unavailable is
    -- checked once more with the provider by idempotency key (rechecked).
    failure_reason  text,
    rechecked       boolean     NOT NULL DEFAULT false,
    amount_minor    bigint      NOT NULL CHECK (amount_minor > 0),
    currency        text        NOT NULL CHECK (currency IN ('RON', 'EUR')),
    state           text        NOT NULL CHECK (state IN ('created', 'submitted', 'challenge_required', 'succeeded', 'declined', 'failed', 'unknown', 'refund_pending', 'refunded')),
    charge_id       text,
    action_url      text,
    error_code      text,
    press_id        text        NOT NULL DEFAULT '',
    -- A failover attempt names the attempt it replaced.
    failover_of     uuid,
    deadline_at     timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    resolved_at     timestamptz,
    UNIQUE (organiser_id, id),
    UNIQUE (provider, provider_key),
    CHECK (purpose = 'topup' OR order_id IS NOT NULL),
    CHECK ((purpose = 'topup') = (wallet_account_id IS NOT NULL)),
    FOREIGN KEY (organiser_id, order_id) REFERENCES sales.orders (organiser_id, id),
    FOREIGN KEY (organiser_id, wallet_account_id) REFERENCES sales.wallet_accounts (organiser_id, id),
    FOREIGN KEY (organiser_id, saved_method_id) REFERENCES sales.saved_payment_methods (organiser_id, id)
);
-- At most one unresolved and at most one succeeded attempt per order (D-2).
CREATE UNIQUE INDEX attempts_one_open_per_order ON sales.payment_attempts (order_id)
    WHERE purpose = 'order' AND state IN ('created', 'submitted', 'challenge_required', 'unknown');
CREATE UNIQUE INDEX attempts_one_success_per_order ON sales.payment_attempts (order_id)
    WHERE purpose = 'order' AND state = 'succeeded';
CREATE INDEX attempts_unresolved_idx ON sales.payment_attempts (updated_at)
    WHERE state IN ('created', 'submitted', 'challenge_required', 'unknown', 'refund_pending');
CREATE INDEX attempts_order_idx ON sales.payment_attempts (organiser_id, order_id);

-- Client idempotency keys (D-3), kept 24 hours.
CREATE TABLE sales.idempotency_keys (
    organiser_id uuid        NOT NULL,
    scope        text        NOT NULL,
    key          text        NOT NULL CHECK (length(key) BETWEEN 8 AND 100),
    request_hash text        NOT NULL,
    status       int,
    response     jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organiser_id, scope, key)
);
CREATE INDEX idempotency_created_idx ON sales.idempotency_keys (created_at);

CREATE TABLE sales.webhook_events (
    organiser_id uuid        NOT NULL,
    provider     text        NOT NULL,
    event_id     text        NOT NULL,
    received_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, event_id)
);
CREATE INDEX webhook_events_org_idx ON sales.webhook_events (organiser_id);

CREATE TABLE sales.instalment_plans (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    organiser_id    uuid        NOT NULL,
    order_id        uuid        NOT NULL,
    state           text        NOT NULL CHECK (state IN ('pending', 'active', 'completed', 'cancelled', 'defaulted')),
    saved_method_id uuid,
    -- Due dates are calendar dates in the event's time zone.
    timezone        text        NOT NULL DEFAULT 'Europe/Bucharest',
    deposit_minor   bigint      NOT NULL CHECK (deposit_minor >= 0),
    total_minor     bigint      NOT NULL CHECK (total_minor > 0),
    -- "Simulate next due date" (demo): days the plan's clock is moved forward.
    clock_offset_days int       NOT NULL DEFAULT 0 CHECK (clock_offset_days BETWEEN 0 AND 1000),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organiser_id, id),
    UNIQUE (organiser_id, order_id),
    FOREIGN KEY (organiser_id, order_id) REFERENCES sales.orders (organiser_id, id),
    FOREIGN KEY (organiser_id, saved_method_id) REFERENCES sales.saved_payment_methods (organiser_id, id)
);

CREATE TABLE sales.instalment_parts (
    organiser_id uuid        NOT NULL,
    plan_id      uuid        NOT NULL,
    n            int         NOT NULL CHECK (n >= 1),
    due_on       date        NOT NULL,
    amount_minor bigint      NOT NULL CHECK (amount_minor > 0),
    state        text        NOT NULL CHECK (state IN ('scheduled', 'charging', 'paid', 'overdue', 'cancelled')),
    tries        int         NOT NULL DEFAULT 0,
    attempt_id   uuid,
    grace_until  date,
    paid_at      timestamptz,
    PRIMARY KEY (organiser_id, plan_id, n),
    FOREIGN KEY (organiser_id, plan_id) REFERENCES sales.instalment_plans (organiser_id, id)
);
CREATE INDEX instalment_parts_due_idx ON sales.instalment_parts (due_on) WHERE state = 'scheduled';

-- The transactional outbox (P3, D-12), written in the same transaction as each change.
CREATE TABLE sales.outbox (
    seq          bigserial   PRIMARY KEY,
    id           uuid        NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    organiser_id uuid        NOT NULL,
    event_id     uuid,
    topic        text        NOT NULL CHECK (topic IN ('orders', 'payments', 'wallet')),
    type         text        NOT NULL CHECK (type ~ '^[a-z_]+\.[a-z_]+$'),
    version      int         NOT NULL DEFAULT 1,
    payload      jsonb       NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT clock_timestamp(),
    relayed_at   timestamptz
);
CREATE INDEX outbox_pending_idx ON sales.outbox (seq) WHERE relayed_at IS NULL;
CREATE INDEX outbox_org_idx ON sales.outbox (organiser_id, seq);

-- +goose StatementBegin
-- Journal lines are append-only; corrections are reversing entries (D-11).
CREATE FUNCTION sales.journal_lines_append_only() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, pg_temp
AS $fn$
BEGIN
    RAISE EXCEPTION 'journal lines are append-only' USING ERRCODE = 'P0001';
END
$fn$;
-- +goose StatementEnd
CREATE TRIGGER journal_lines_append_only BEFORE UPDATE OR DELETE ON sales.journal_lines
    FOR EACH ROW EXECUTE FUNCTION sales.journal_lines_append_only();
CREATE TRIGGER journal_lines_no_truncate BEFORE TRUNCATE ON sales.journal_lines
    FOR EACH STATEMENT EXECUTE FUNCTION sales.journal_lines_append_only();

-- +goose StatementBegin
-- Cached balances move with every line, in the same transaction; the CHECK on fan accounts makes
-- an overspend roll the whole entry back. Runs as the caller (RLS applies).
CREATE FUNCTION sales.journal_lines_apply() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, pg_temp
AS $fn$
BEGIN
    UPDATE sales.wallet_accounts SET balance_minor = balance_minor + NEW.amount_minor
    WHERE organiser_id = NEW.organiser_id AND id = NEW.account_id AND currency = NEW.currency;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'journal line for an unknown account or another currency' USING ERRCODE = 'P0001';
    END IF;
    RETURN NEW;
END
$fn$;
-- +goose StatementEnd
CREATE TRIGGER journal_lines_apply AFTER INSERT ON sales.journal_lines
    FOR EACH ROW EXECUTE FUNCTION sales.journal_lines_apply();

-- +goose StatementBegin
-- Every entry's lines add up to zero per currency, checked at commit (deferred), and an entry has
-- at least two lines.
CREATE FUNCTION sales.journal_entry_balanced() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog, pg_temp
AS $fn$
DECLARE
    bad int;
BEGIN
    SELECT count(*) INTO bad FROM (
        SELECT currency, sum(amount_minor) AS s, count(*) AS n FROM sales.journal_lines
        WHERE organiser_id = NEW.organiser_id AND entry_id = NEW.entry_id GROUP BY currency
    ) x WHERE x.s <> 0;
    IF bad > 0 THEN
        RAISE EXCEPTION 'journal entry % does not add up to zero', NEW.entry_id USING ERRCODE = '23514';
    END IF;
    IF (SELECT count(*) FROM sales.journal_lines WHERE organiser_id = NEW.organiser_id AND entry_id = NEW.entry_id) < 2 THEN
        RAISE EXCEPTION 'journal entry % has fewer than two lines', NEW.entry_id USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END
$fn$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER journal_entry_balanced AFTER INSERT ON sales.journal_lines
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION sales.journal_entry_balanced();

-- Row-level security on every table.
-- +goose StatementBegin
DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['orders', 'order_lines', 'order_history', 'saved_payment_methods', 'wallet_accounts',
        'journal_entries', 'journal_lines', 'payment_attempts', 'idempotency_keys', 'webhook_events',
        'instalment_plans', 'instalment_parts', 'outbox']
    LOOP
        EXECUTE format('ALTER TABLE sales.%I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE sales.%I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format($p$CREATE POLICY organiser_isolation ON sales.%I
            USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
            WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)$p$, t);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON sales.%I TO eps_core', t);
        -- The web app reads none of these (it reaches sales data through the core): no grant.
        EXECUTE format('REVOKE ALL ON sales.%I FROM eps_web', t);
    END LOOP;
END
$$;
-- +goose StatementEnd
-- Journal lines: the core may only add them.
REVOKE UPDATE, DELETE, TRUNCATE ON sales.journal_lines FROM eps_core;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA sales TO eps_core;

-- The cross-organiser index functions, owned by their own NOLOGIN NOBYPASSRLS role.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'eps_sales_index') THEN
        CREATE ROLE eps_sales_index NOLOGIN NOBYPASSRLS NOINHERIT;
    END IF;
END
$$;
-- +goose StatementEnd
GRANT eps_sales_index TO CURRENT_USER WITH INHERIT FALSE, SET TRUE;
GRANT USAGE ON SCHEMA sales TO eps_sales_index;
GRANT SELECT (id, organiser_id, event_id, state, hold_expires_at, buyer_id, buyer_kind, buyer_email, created_at) ON sales.orders TO eps_sales_index;
CREATE POLICY sales_index ON sales.orders FOR SELECT TO eps_sales_index USING (true);
GRANT SELECT (id, organiser_id, order_id, state, updated_at, deadline_at, purpose, failure_reason, rechecked) ON sales.payment_attempts TO eps_sales_index;
CREATE POLICY sales_index ON sales.payment_attempts FOR SELECT TO eps_sales_index USING (true);
GRANT SELECT (organiser_id, plan_id, n, due_on, state, grace_until) ON sales.instalment_parts TO eps_sales_index;
CREATE POLICY sales_index ON sales.instalment_parts FOR SELECT TO eps_sales_index USING (true);
GRANT SELECT (id, organiser_id, state, clock_offset_days, timezone) ON sales.instalment_plans TO eps_sales_index;
CREATE POLICY sales_index ON sales.instalment_plans FOR SELECT TO eps_sales_index USING (true);
GRANT SELECT (id, organiser_id, kind, owner, currency, balance_minor, code) ON sales.wallet_accounts TO eps_sales_index;
CREATE POLICY sales_index ON sales.wallet_accounts FOR SELECT TO eps_sales_index USING (true);
GRANT SELECT (seq, organiser_id, relayed_at, created_at) ON sales.outbox TO eps_sales_index;
GRANT SELECT (created_at), DELETE ON sales.idempotency_keys TO eps_sales_index;
CREATE POLICY sales_index ON sales.idempotency_keys FOR ALL TO eps_sales_index USING (true);
GRANT SELECT (received_at), DELETE ON sales.webhook_events TO eps_sales_index;
CREATE POLICY sales_index ON sales.webhook_events FOR ALL TO eps_sales_index USING (true);
CREATE POLICY sales_index ON sales.outbox FOR SELECT TO eps_sales_index USING (true);

-- +goose StatementBegin
-- The organiser of an order (a fan opens an order by its id; the core then reads it under that
-- organiser's scope and checks the caller may see it).
CREATE FUNCTION sales.order_organiser(p_order uuid) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$ SELECT organiser_id FROM sales.orders WHERE id = p_order $fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Held or payment-failed orders past their deadline (the hold sweeper).
CREATE FUNCTION sales.due_holds(p_limit int)
RETURNS TABLE (organiser_id uuid, order_id uuid, hold_expires_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT o.organiser_id, o.id, o.hold_expires_at FROM sales.orders o
    WHERE o.hold_expires_at <= now()
      AND (o.state IN ('held', 'payment_failed')
           OR (o.state = 'paying' AND NOT EXISTS (SELECT 1 FROM sales.payment_attempts a WHERE a.order_id = o.id
                AND a.state IN ('created', 'submitted', 'challenge_required', 'unknown'))))
    ORDER BY o.hold_expires_at LIMIT least(greatest(p_limit, 1), 5000)
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Attempts whose outcome is not known yet and that are older than p_age (the reconciler).
CREATE FUNCTION sales.unresolved_attempts(p_age interval, p_limit int)
RETURNS TABLE (organiser_id uuid, attempt_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT a.organiser_id, a.id FROM sales.payment_attempts a
    WHERE a.state IN ('created', 'submitted', 'challenge_required', 'unknown', 'refund_pending')
      AND a.updated_at <= now() - p_age
    ORDER BY a.updated_at LIMIT least(greatest(p_limit, 1), 1000)
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Instalment parts due on or before today in the event's time zone (moved by the plan's
-- simulated clock).
CREATE FUNCTION sales.due_instalment_parts(p_now timestamptz, p_limit int)
RETURNS TABLE (organiser_id uuid, plan_id uuid, n int)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT p.organiser_id, p.plan_id, p.n FROM sales.instalment_parts p
    JOIN sales.instalment_plans l ON l.organiser_id = p.organiser_id AND l.id = p.plan_id AND l.state = 'active'
    WHERE p.state = 'scheduled' AND p.due_on <= (p_now AT TIME ZONE l.timezone)::date + l.clock_offset_days
    ORDER BY p.due_on LIMIT least(greatest(p_limit, 1), 1000)
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Active plans with an overdue part whose 7-day grace ended (the plan defaults).
CREATE FUNCTION sales.defaulted_plans(p_now timestamptz, p_limit int)
RETURNS TABLE (organiser_id uuid, plan_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT DISTINCT l.organiser_id, l.id FROM sales.instalment_parts p
    JOIN sales.instalment_plans l ON l.organiser_id = p.organiser_id AND l.id = p.plan_id AND l.state = 'active'
    WHERE p.state = 'overdue' AND p.grace_until < (p_now AT TIME ZONE l.timezone)::date + l.clock_offset_days
    LIMIT least(greatest(p_limit, 1), 1000)
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Attempts failed as provider_unavailable in the last 5 minutes and not yet checked again by key
-- (a reset or a lost answer may still have charged).
CREATE FUNCTION sales.recheck_attempts(p_limit int)
RETURNS TABLE (organiser_id uuid, attempt_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT a.organiser_id, a.id FROM sales.payment_attempts a
    WHERE a.state = 'failed' AND a.failure_reason = 'provider_unavailable' AND NOT a.rechecked
      AND a.updated_at > now() - interval '5 minutes' AND a.updated_at <= now() - interval '10 seconds'
    LIMIT least(greatest(p_limit, 1), 1000)
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Purges client idempotency keys older than 24 hours and webhook ids older than 7 days.
CREATE FUNCTION sales.purge_expired()
RETURNS TABLE (idempotency_keys bigint, webhook_events bigint)
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
DECLARE
    a bigint;
    b bigint;
BEGIN
    DELETE FROM sales.idempotency_keys WHERE created_at < now() - interval '24 hours';
    GET DIAGNOSTICS a = ROW_COUNT;
    DELETE FROM sales.webhook_events WHERE received_at < now() - interval '7 days';
    GET DIAGNOSTICS b = ROW_COUNT;
    RETURN QUERY SELECT a, b;
END
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Organisers with events waiting in the outbox (the relay then claims them per organiser).
CREATE FUNCTION sales.outbox_organisers(p_limit int)
RETURNS TABLE (organiser_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT DISTINCT o.organiser_id FROM (
        SELECT organiser_id FROM sales.outbox WHERE relayed_at IS NULL ORDER BY seq LIMIT 5000
    ) o LIMIT least(greatest(p_limit, 1), 1000)
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- A buyer's own orders across organisers (account order list).
CREATE FUNCTION sales.orders_of_buyer(p_buyer text, p_limit int)
RETURNS TABLE (organiser_id uuid, order_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT o.organiser_id, o.id FROM sales.orders o WHERE o.buyer_id = p_buyer
    ORDER BY o.created_at DESC LIMIT least(greatest(p_limit, 1), 200)
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Guest orders placed with an email (a claim by the verified owner of that email, D-9).
CREATE FUNCTION sales.guest_orders_by_email(p_email text, p_limit int)
RETURNS TABLE (organiser_id uuid, order_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT o.organiser_id, o.id FROM sales.orders o
    WHERE o.buyer_kind = 'guest' AND o.buyer_email = lower(btrim(p_email))
    ORDER BY o.created_at LIMIT least(greatest(p_limit, 1), 200)
$fn$;
-- +goose StatementEnd

-- +goose StatementBegin
-- A fan's wallets across organisers (the wallet screen), and a wallet by its stand code.
CREATE FUNCTION sales.wallets_of_fan(p_owner text)
RETURNS TABLE (organiser_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT DISTINCT w.organiser_id FROM sales.wallet_accounts w
    WHERE w.owner = p_owner AND w.kind IN ('fan_balance', 'fan_season_credit')
$fn$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION sales.order_organiser(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.due_holds(int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.unresolved_attempts(interval, int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.due_instalment_parts(timestamptz, int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.defaulted_plans(timestamptz, int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.recheck_attempts(int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.purge_expired() FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.outbox_organisers(int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.orders_of_buyer(text, int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.guest_orders_by_email(text, int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.wallets_of_fan(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sales.order_organiser(uuid), sales.due_holds(int), sales.unresolved_attempts(interval, int),
    sales.due_instalment_parts(timestamptz, int), sales.defaulted_plans(timestamptz, int), sales.recheck_attempts(int), sales.purge_expired(), sales.outbox_organisers(int), sales.orders_of_buyer(text, int),
    sales.guest_orders_by_email(text, int), sales.wallets_of_fan(text) TO eps_core;
GRANT CREATE ON SCHEMA sales TO eps_sales_index;
ALTER FUNCTION sales.order_organiser(uuid) OWNER TO eps_sales_index;
ALTER FUNCTION sales.due_holds(int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.unresolved_attempts(interval, int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.due_instalment_parts(timestamptz, int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.defaulted_plans(timestamptz, int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.recheck_attempts(int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.purge_expired() OWNER TO eps_sales_index;
ALTER FUNCTION sales.outbox_organisers(int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.orders_of_buyer(text, int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.guest_orders_by_email(text, int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.wallets_of_fan(text) OWNER TO eps_sales_index;
REVOKE CREATE ON SCHEMA sales FROM eps_sales_index;

-- +goose StatementBegin
-- The outbox backlog: events not yet relayed and the age of the oldest (metrics).
CREATE FUNCTION sales.outbox_backlog()
RETURNS TABLE (pending bigint, oldest_seconds double precision)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT count(*), coalesce(extract(epoch FROM now() - min(created_at)), 0) FROM sales.outbox WHERE relayed_at IS NULL
$fn$;
-- +goose StatementEnd
-- +goose StatementBegin
-- Payment attempts with no outcome yet (metrics).
CREATE FUNCTION sales.unresolved_count()
RETURNS bigint
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT count(*) FROM sales.payment_attempts WHERE state IN ('created', 'submitted', 'challenge_required', 'unknown')
$fn$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION sales.outbox_backlog() FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.unresolved_count() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sales.outbox_backlog(), sales.unresolved_count() TO eps_core;
GRANT CREATE ON SCHEMA sales TO eps_sales_index;
ALTER FUNCTION sales.outbox_backlog() OWNER TO eps_sales_index;
ALTER FUNCTION sales.unresolved_count() OWNER TO eps_sales_index;
REVOKE CREATE ON SCHEMA sales FROM eps_sales_index;
