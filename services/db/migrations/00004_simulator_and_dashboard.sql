-- Simulator, live dashboard and graceful degradation (step 6; E7, E8, E11, I4). Additive only.
-- A rehearsal is a sale moment under an event id of its own (design D-1): the checkout gate picks an
-- event's current sale and stock, caps and queues are keyed by event, so sharing the real event id
-- would mix the rehearsal into the real sale. Organiser data like every sales table: organiser_id
-- NOT NULL, row-level security enabled and forced, composite foreign keys.

-- +goose Up
ALTER TABLE sales.sales ADD COLUMN rehearsal_of_event uuid;
ALTER TABLE sales.sales ADD COLUMN rehearsal_of_sale uuid;
ALTER TABLE sales.sales ADD COLUMN rehearsal_version int CHECK (rehearsal_version IS NULL OR rehearsal_version >= 1);
-- When the worker deleted the rehearsal's Redis data (D-1 clean-up).
ALTER TABLE sales.sales ADD COLUMN rehearsal_cleaned_at timestamptz;
ALTER TABLE sales.sales ADD CONSTRAINT sales_rehearsal_complete CHECK (
    (rehearsal_of_event IS NULL) = (rehearsal_of_sale IS NULL) AND (rehearsal_of_event IS NULL) = (rehearsal_version IS NULL));
ALTER TABLE sales.sales ADD CONSTRAINT sales_rehearsal_source_fkey
    FOREIGN KEY (organiser_id, rehearsal_of_sale) REFERENCES sales.sales (organiser_id, id);
CREATE INDEX sales_rehearsal_idx ON sales.sales (organiser_id, rehearsal_of_event) WHERE rehearsal_of_event IS NOT NULL;
-- Degraded mode (D-9): new holds refused, admissions held, the shop shows the busy message.
ALTER TABLE sales.sales ADD COLUMN degraded boolean NOT NULL DEFAULT false;
ALTER TABLE sales.sales ADD COLUMN degraded_at timestamptz;
ALTER TABLE sales.sales ADD COLUMN degraded_by text;

-- Rehearsal orders and the run that made them (from the simulator's token); kept out of reports.
ALTER TABLE sales.orders ADD COLUMN rehearsal boolean NOT NULL DEFAULT false;
ALTER TABLE sales.orders ADD COLUMN sim_run_id uuid;
-- Organiser-wide figures read real orders only (review D-a; step 10's reports use this view).
CREATE VIEW sales.real_orders WITH (security_invoker = true) AS SELECT * FROM sales.orders WHERE NOT rehearsal;
GRANT SELECT ON sales.real_orders TO eps_core;
REVOKE ALL ON sales.real_orders FROM eps_web;
CREATE INDEX orders_sale_state_idx ON sales.orders (organiser_id, sale_id, state) WHERE sale_id IS NOT NULL;
-- A rehearsal's attempts pay under the rehearsal's own merchant account and breaker (D-1).
ALTER TABLE sales.payment_attempts ADD COLUMN merchant_event uuid;
CREATE INDEX attempts_created_idx ON sales.payment_attempts (organiser_id, created_at) WHERE purpose = 'order';

-- Simulation runs (D-2). The knobs are validated by the core; the reports and summary are the
-- simulator's ground truth (counts only, never personal data).
CREATE TABLE sales.simulation_runs (
    id             uuid        PRIMARY KEY,
    organiser_id   uuid        NOT NULL,
    sale_id        uuid        NOT NULL,
    event_id       uuid        NOT NULL,
    state          text        NOT NULL CHECK (state IN ('pending', 'starting', 'running', 'finishing', 'done', 'failed', 'stopped')),
    knobs          jsonb       NOT NULL,
    created_by     text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    claimed_at     timestamptz,
    opens_at       timestamptz,
    ended_at       timestamptz,
    last_report_at timestamptz,
    stop_requested boolean     NOT NULL DEFAULT false,
    outage_on_at   timestamptz,
    outage_off_at  timestamptz,
    report         jsonb,
    summary        jsonb,
    error          text CHECK (error IS NULL OR length(error) <= 200),
    UNIQUE (organiser_id, id),
    FOREIGN KEY (organiser_id, sale_id) REFERENCES sales.sales (organiser_id, id)
);
-- One active run per organiser (and so per sale); one per environment is checked under a lock.
CREATE UNIQUE INDEX simulation_runs_one_active ON sales.simulation_runs (organiser_id)
    WHERE state IN ('pending', 'starting', 'running', 'finishing');
CREATE INDEX simulation_runs_sale_idx ON sales.simulation_runs (organiser_id, sale_id, created_at DESC);

ALTER TABLE sales.simulation_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE sales.simulation_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY organiser_isolation ON sales.simulation_runs
    USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
    WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid);
GRANT SELECT, INSERT, UPDATE ON sales.simulation_runs TO eps_core;
REVOKE ALL ON sales.simulation_runs FROM eps_web;

-- The index role reads only ids, organisers, states and times (spec-map SECURITY DEFINER rule).
GRANT SELECT (rehearsal_of_event, rehearsal_version, rehearsal_cleaned_at, created_at) ON sales.sales TO eps_sales_index;
GRANT SELECT (id, organiser_id, sale_id, event_id, state, created_at, ended_at, last_report_at) ON sales.simulation_runs TO eps_sales_index;
CREATE POLICY sales_index ON sales.simulation_runs FOR SELECT TO eps_sales_index USING (true);

-- +goose StatementBegin
-- The source of a rehearsal event (the core's snapshot source): its organiser, the source event and
-- the pinned snapshot version. Nothing for an event that is not a live rehearsal.
CREATE FUNCTION sales.rehearsal_source(p_event uuid)
RETURNS TABLE (organiser_id uuid, source_event uuid, version int)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT s.organiser_id, s.rehearsal_of_event, s.rehearsal_version FROM sales.sales s
    WHERE s.event_id = p_event AND s.rehearsal_of_event IS NOT NULL AND s.deleted_at IS NULL
    LIMIT 1
$fn$;
-- +goose StatementEnd
-- +goose StatementBegin
-- Runs in the given states across organisers, oldest first (claim, the one-per-environment rule,
-- the lost-simulator check and the aggregator's watched sales): ids and organisers only.
CREATE FUNCTION sales.simulation_runs_in(p_states text[], p_limit int)
RETURNS TABLE (id uuid, organiser_id uuid, sale_id uuid, event_id uuid, state text, last_report_at timestamptz, created_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT r.id, r.organiser_id, r.sale_id, r.event_id, r.state, r.last_report_at, r.created_at FROM sales.simulation_runs r
    WHERE r.state = ANY (p_states) ORDER BY r.created_at LIMIT least(greatest(p_limit, 1), 100)
$fn$;
-- +goose StatementEnd
-- +goose StatementBegin
-- Rehearsals whose Redis data can go: ended (reset or deleted) or with no run active or ended since
-- p_before, and not cleaned since their last run (D-1 clean-up).
CREATE FUNCTION sales.rehearsals_to_clean(p_before timestamptz, p_limit int)
RETURNS TABLE (organiser_id uuid, event_id uuid, sale_id uuid)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = pg_catalog, pg_temp
AS $fn$
    SELECT s.organiser_id, s.event_id, s.id FROM sales.sales s
    WHERE s.rehearsal_of_event IS NOT NULL
      AND NOT EXISTS (SELECT 1 FROM sales.simulation_runs r WHERE r.organiser_id = s.organiser_id AND r.sale_id = s.id
                      AND r.state IN ('pending', 'starting', 'running', 'finishing'))
      -- A reset rehearsal waits out its holds and payment attempts (at most 15 + 5 minutes).
      AND ((s.deleted_at IS NOT NULL AND s.deleted_at < now() - interval '20 minutes' AND s.rehearsal_cleaned_at IS NULL)
           OR (s.deleted_at IS NULL
               AND NOT EXISTS (SELECT 1 FROM sales.simulation_runs r WHERE r.organiser_id = s.organiser_id AND r.sale_id = s.id AND r.ended_at > p_before)
               AND EXISTS (SELECT 1 FROM sales.simulation_runs r WHERE r.organiser_id = s.organiser_id AND r.sale_id = s.id
                           AND r.ended_at > coalesce(s.rehearsal_cleaned_at, '-infinity'::timestamptz))))
    ORDER BY s.created_at LIMIT least(greatest(p_limit, 1), 100)
$fn$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION sales.rehearsal_source(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.simulation_runs_in(text[], int) FROM PUBLIC;
REVOKE ALL ON FUNCTION sales.rehearsals_to_clean(timestamptz, int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sales.rehearsal_source(uuid) TO eps_core;
GRANT EXECUTE ON FUNCTION sales.simulation_runs_in(text[], int) TO eps_core;
GRANT EXECUTE ON FUNCTION sales.rehearsals_to_clean(timestamptz, int) TO eps_core;
GRANT CREATE ON SCHEMA sales TO eps_sales_index;
ALTER FUNCTION sales.rehearsal_source(uuid) OWNER TO eps_sales_index;
ALTER FUNCTION sales.simulation_runs_in(text[], int) OWNER TO eps_sales_index;
ALTER FUNCTION sales.rehearsals_to_clean(timestamptz, int) OWNER TO eps_sales_index;
REVOKE CREATE ON SCHEMA sales FROM eps_sales_index;

-- The simulator's runs write their own events.
ALTER TABLE sales.outbox DROP CONSTRAINT outbox_topic_check;
ALTER TABLE sales.outbox ADD CONSTRAINT outbox_topic_check CHECK (topic IN ('orders', 'payments', 'wallet', 'inventory', 'sales', 'simulations'));
