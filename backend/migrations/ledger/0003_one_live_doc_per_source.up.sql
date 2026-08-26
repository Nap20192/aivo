-- Broaden the "one live document per source" invariant from shift
-- acceptance only to every source kind: the ledger gRPC server (inbound
-- side of inventory's outbox, split-inventory-microservice) needs this
-- as its DB-level idempotency backstop for Post*/Reverse*Journal, which
-- can otherwise receive the same at-least-once-delivered event twice
-- concurrently.
DROP INDEX journal_documents_one_live_per_shift_idx;

CREATE UNIQUE INDEX journal_documents_one_live_per_source_idx
    ON journal_documents (source_kind, source_id)
    WHERE state <> 'cancelled' AND source_kind IS NOT NULL;
