-- Two tech-card formats on the same recipe data: 'simple' (lean, costing
-- only — default) and 'ttk' (adds the ГОСТ 31987-2012
-- технико-технологическая карта text sections). See
-- domain/inventory/techcard.go for the field meanings.
ALTER TABLE tech_cards
    ADD COLUMN format             text NOT NULL DEFAULT 'simple',
    ADD COLUMN scope_note         text,
    ADD COLUMN presentation_note  text,
    ADD COLUMN storage_note       text,
    ADD COLUMN organoleptic_note  text;

ALTER TABLE tech_cards
    ADD CONSTRAINT tech_cards_format_check CHECK (format IN ('simple', 'ttk'));

-- Yield after cooking loss (ГОСТ выход / Western AP→EP), per mille of the
-- gross qty. Informational — never affects costing (see NetQty doc).
ALTER TABLE tech_card_lines
    ADD COLUMN yield_permille integer NOT NULL DEFAULT 1000;

ALTER TABLE tech_card_lines
    ADD CONSTRAINT tech_card_lines_yield_check CHECK (yield_permille > 0 AND yield_permille <= 1000);
