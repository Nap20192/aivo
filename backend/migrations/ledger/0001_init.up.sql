-- Ledger (GL) context: append-only double-entry journal. A journal
-- document has a lifecycle (draft -> posted -> cancelled) that is the
-- posting gate (D4); posted documents are immutable and corrected only by
-- a reversal (D1). Two dates on every document (D7). Fixed dimensions:
-- restaurant_id + cost_center_id (no open dimensions). All amount columns
-- are bigint cents -- single currency (company base); multicurrency
-- deferred (reference §16.4).

-- Chart of accounts (per restaurant). Only a postable leaf account takes
-- lines; type/normal_side freeze after the first posting (enforced in the
-- app/store).
CREATE TABLE accounts (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    code          text NOT NULL,
    name          text NOT NULL,
    type          text NOT NULL, -- asset|liability|revenue|expense|equity|statistical
    normal_side   text NOT NULL, -- debit|credit
    postable      boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX accounts_restaurant_code_idx ON accounts (restaurant_id, code);
CREATE INDEX accounts_restaurant_idx ON accounts (restaurant_id);

-- Flat per-restaurant cost centers (seed: one "main"). No tree, no
-- allocation engine -- shallow until a named requirement (reference §10).
CREATE TABLE cost_centers (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    code          text NOT NULL,
    name          text NOT NULL
);

CREATE UNIQUE INDEX cost_centers_restaurant_code_idx ON cost_centers (restaurant_id, code);

-- Journal documents (aggregate root).
CREATE TABLE journal_documents (
    id              uuid PRIMARY KEY,
    restaurant_id   uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    kind            text NOT NULL,               -- shift_acceptance|manual|reversal
    state           text NOT NULL DEFAULT 'draft', -- draft|posted|cancelled
    accounting_date date NOT NULL,               -- business date of the fact (D7)
    recorded_at     timestamptz NOT NULL DEFAULT now(), -- wall clock of the record (D7)
    posted_at       timestamptz,
    cancelled_at    timestamptz,
    source_kind     text,   -- 'shift' | 'manual' | NULL
    source_id       uuid,   -- e.g. the shift id (no FK -- cross-context)
    reversal_of     uuid REFERENCES journal_documents (id),
    created_by      uuid NOT NULL REFERENCES users (id)
);

CREATE INDEX journal_documents_restaurant_date_idx ON journal_documents (restaurant_id, accounting_date);
CREATE INDEX journal_documents_source_idx ON journal_documents (source_kind, source_id);

-- One live (non-cancelled) document per shift source: shift acceptance is
-- idempotent, a re-accept conflicts (409). refuted §15.2 / §16.5.
CREATE UNIQUE INDEX journal_documents_one_live_per_shift_idx
    ON journal_documents (source_kind, source_id)
    WHERE state <> 'cancelled' AND source_kind = 'shift';

-- One-sided lines (debit XOR credit, amount > 0). Append-only once the
-- document is posted.
CREATE TABLE journal_lines (
    id             uuid PRIMARY KEY,
    document_id    uuid NOT NULL REFERENCES journal_documents (id) ON DELETE CASCADE,
    account_id     uuid NOT NULL REFERENCES accounts (id),
    side           text NOT NULL, -- debit|credit
    amount_cents   bigint NOT NULL, -- > 0, single currency (§16.4)
    cost_center_id uuid NOT NULL REFERENCES cost_centers (id),
    memo           text NOT NULL DEFAULT '',
    seq            integer NOT NULL,
    CONSTRAINT journal_lines_amount_positive CHECK (amount_cents > 0),
    CONSTRAINT journal_lines_side CHECK (side IN ('debit', 'credit'))
);

CREATE INDEX journal_lines_document_seq_idx ON journal_lines (document_id, seq);
CREATE INDEX journal_lines_account_idx ON journal_lines (account_id);

-- Per-restaurant GL-semantics config: maps a fixed purpose to an account.
-- Changing the map changes what posting does -- this is the per-deployment
-- GL-treatment knob (refuted §15.6), not a fixed property of the system.
CREATE TABLE ledger_account_map (
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    purpose       text NOT NULL,
    account_id    uuid NOT NULL REFERENCES accounts (id)
);

CREATE UNIQUE INDEX ledger_account_map_restaurant_purpose_idx ON ledger_account_map (restaurant_id, purpose);
