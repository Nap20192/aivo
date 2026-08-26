DROP INDEX journal_documents_one_live_per_source_idx;

CREATE UNIQUE INDEX journal_documents_one_live_per_shift_idx
    ON journal_documents (source_kind, source_id)
    WHERE state <> 'cancelled' AND source_kind = 'shift';
