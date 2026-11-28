-- Checkout questions (step 4, design D-13; D9 Thin). An organiser's questions and consents for a
-- brand's locked checkout. The storefront document's checkout slots hold their ids (afterContact for
-- short text and single choice, consents for checkboxes) and the sale snapshot copies those ids at
-- publish, so a checkout asks exactly the questions of the version it sells. A question never
-- changes after it is created (an edit is a new question with a new id), so an id in a snapshot
-- always means the same question. Organiser data: forced row-level security.

-- +goose Up
CREATE TABLE control.checkout_questions (
    organiser_id uuid        NOT NULL REFERENCES control.organisers (id) ON DELETE CASCADE,
    brand_id     uuid        NOT NULL,
    id           text        NOT NULL CHECK (id ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
    kind         text        NOT NULL CHECK (kind IN ('text', 'choice', 'consent')),
    label        jsonb       NOT NULL CHECK (jsonb_typeof(label) = 'object' AND length(label ->> 'ro') BETWEEN 1 AND 300 AND length(label ->> 'en') BETWEEN 1 AND 300),
    options      jsonb       NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(options) = 'array' AND jsonb_array_length(options) <= 10),
    required     boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organiser_id, brand_id, id),
    CHECK ((kind = 'choice') = (jsonb_array_length(options) >= 2)),
    FOREIGN KEY (organiser_id, brand_id) REFERENCES control.brands (organiser_id, id) ON DELETE CASCADE
);

ALTER TABLE control.checkout_questions ENABLE ROW LEVEL SECURITY;
ALTER TABLE control.checkout_questions FORCE ROW LEVEL SECURITY;
CREATE POLICY organiser_isolation ON control.checkout_questions
    USING (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid)
    WITH CHECK (organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid);
-- The web app creates and removes questions; the core reads them under the order's organiser scope.
REVOKE UPDATE ON control.checkout_questions FROM eps_web;
GRANT SELECT ON control.checkout_questions TO eps_core;

-- The core checks a provider return address against a brand's configured hostnames and a wallet's
-- currency against the organiser's brands (step 4 review): only these columns, under RLS.
GRANT SELECT (organiser_id, hostname, brand_id) ON control.brand_hostnames TO eps_core;
GRANT SELECT (id, organiser_id, currency, name) ON control.brands TO eps_core;
