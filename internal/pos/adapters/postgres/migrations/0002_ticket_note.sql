-- Diner note carried from a cart handoff onto the ticket (surfaced in
-- pos state / ticket views).
ALTER TABLE tickets ADD COLUMN note text NOT NULL DEFAULT '';
