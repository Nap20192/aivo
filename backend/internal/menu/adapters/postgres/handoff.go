package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"aivo/internal/domain/menu"
	"aivo/internal/menu/ports"

	"github.com/google/uuid"
)

func (s *PostgresStore) CreateHandoff(ctx context.Context, h domain.Handoff) error {
	lines, err := json.Marshal(h.Lines)
	if err != nil {
		return fmt.Errorf("store: create handoff: encode lines: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create handoff: begin: %w", err)
	}
	defer tx.Rollback()

	// A new handoff replaces the previous active one from this table.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM cart_handoffs WHERE table_id = $1 AND used_at IS NULL`, h.TableID); err != nil {
		return fmt.Errorf("store: create handoff: clear previous: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO cart_handoffs (id, restaurant_id, table_id, customer_id, code, lines, note, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		h.ID, h.RestaurantID, h.TableID, h.CustomerID, h.Code, lines, h.Note, h.ExpiresAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("code collision: %w", ports.ErrConflict)
		}
		return fmt.Errorf("store: create handoff: %w", err)
	}
	return tx.Commit()
}

const handoffCols = `id, restaurant_id, table_id, customer_id, code, lines, note, expires_at, used_at, created_at`

func scanHandoff(row interface{ Scan(...any) error }) (domain.Handoff, error) {
	var h domain.Handoff
	var lines []byte
	err := row.Scan(&h.ID, &h.RestaurantID, &h.TableID, &h.CustomerID, &h.Code, &lines, &h.Note, &h.ExpiresAt, &h.UsedAt, &h.CreatedAt)
	if err != nil {
		return h, err
	}
	if err := json.Unmarshal(lines, &h.Lines); err != nil {
		return h, fmt.Errorf("decode lines: %w", err)
	}
	return h, nil
}

func (s *PostgresStore) HandoffByCode(ctx context.Context, restaurantID uuid.UUID, code string) (domain.Handoff, error) {
	h, err := scanHandoff(s.db.QueryRowContext(ctx,
		`SELECT `+handoffCols+` FROM cart_handoffs
		 WHERE restaurant_id = $1 AND code = $2 AND used_at IS NULL AND expires_at > now()`,
		restaurantID, code))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Handoff{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Handoff{}, fmt.Errorf("store: handoff by code: %w", err)
	}
	return h, nil
}

func (s *PostgresStore) MarkHandoffUsed(ctx context.Context, restaurantID, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE cart_handoffs SET used_at = now()
		 WHERE restaurant_id = $1 AND id = $2 AND used_at IS NULL AND expires_at > now()`,
		restaurantID, id)
	if err != nil {
		return fmt.Errorf("store: mark handoff used: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (s *PostgresStore) UnmarkHandoffUsed(ctx context.Context, id uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE cart_handoffs SET used_at = NULL WHERE id = $1`, id); err != nil {
		return fmt.Errorf("store: unmark handoff: %w", err)
	}
	return nil
}
