ALTER TABLE tech_card_lines DROP CONSTRAINT IF EXISTS tech_card_lines_yield_check;
ALTER TABLE tech_card_lines DROP COLUMN IF EXISTS yield_permille;

ALTER TABLE tech_cards DROP CONSTRAINT IF EXISTS tech_cards_format_check;
ALTER TABLE tech_cards
    DROP COLUMN IF EXISTS format,
    DROP COLUMN IF EXISTS scope_note,
    DROP COLUMN IF EXISTS presentation_note,
    DROP COLUMN IF EXISTS storage_note,
    DROP COLUMN IF EXISTS organoleptic_note;
