package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"aivo/internal/domain/platform"
	"aivo/internal/platform/ports"

	"github.com/google/uuid"
)

func (s *Store) AssistantThread(ctx context.Context, restaurantID uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	// Lazy get-or-create; the unique index on restaurant_id makes the
	// upsert race-safe.
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO assistant_threads (id, restaurant_id) VALUES ($1, $2)
		 ON CONFLICT (restaurant_id) DO UPDATE SET restaurant_id = EXCLUDED.restaurant_id
		 RETURNING id`, id, restaurantID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("store: assistant thread: %w", err)
	}
	return id, nil
}

func (s *Store) CreateAssistantMessage(ctx context.Context, restaurantID uuid.UUID, m domain.AssistantMessage) error {
	attachments, err := json.Marshal(orEmptyAttachments(m.Attachments))
	if err != nil {
		return fmt.Errorf("store: assistant message: encode attachments: %w", err)
	}
	actions, err := domain.EncodeActions(m.Actions)
	if err != nil {
		return fmt.Errorf("store: assistant message: encode actions: %w", err)
	}
	// thread ownership double-checked via the subquery scope.
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO assistant_messages (id, thread_id, role, text, attachments, actions, action_status)
		 SELECT $1, t.id, $3, $4, $5, $6, $7
		 FROM assistant_threads t WHERE t.id = $2 AND t.restaurant_id = $8`,
		m.ID, m.ThreadID, m.Role, m.Text, attachments, actions, m.ActionStatus, restaurantID)
	if err != nil {
		return fmt.Errorf("store: assistant message: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func orEmptyAttachments(a []domain.Attachment) []domain.Attachment {
	if a == nil {
		return []domain.Attachment{}
	}
	return a
}

const assistantMsgCols = `m.id, m.thread_id, m.role, m.text, m.attachments, m.actions, m.action_status, m.created_at`

func scanAssistantMessage(row interface{ Scan(...any) error }) (domain.AssistantMessage, error) {
	var m domain.AssistantMessage
	var attachments, actions []byte
	if err := row.Scan(&m.ID, &m.ThreadID, &m.Role, &m.Text, &attachments, &actions, &m.ActionStatus, &m.CreatedAt); err != nil {
		return m, err
	}
	if err := json.Unmarshal(attachments, &m.Attachments); err != nil {
		return m, fmt.Errorf("decode attachments: %w", err)
	}
	if err := json.Unmarshal(actions, &m.Actions); err != nil {
		return m, fmt.Errorf("decode actions: %w", err)
	}
	return m, nil
}

func (s *Store) AssistantMessages(ctx context.Context, restaurantID uuid.UUID, limit int) ([]domain.AssistantMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+assistantMsgCols+` FROM (
		   SELECT m.* FROM assistant_messages m
		   JOIN assistant_threads t ON t.id = m.thread_id
		   WHERE t.restaurant_id = $1
		   ORDER BY m.created_at DESC LIMIT $2
		 ) m ORDER BY m.created_at ASC`, restaurantID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: assistant messages: %w", err)
	}
	defer rows.Close()

	msgs := []domain.AssistantMessage{}
	for rows.Next() {
		m, err := scanAssistantMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("store: assistant messages: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (s *Store) AssistantMessageByID(ctx context.Context, restaurantID, id uuid.UUID) (domain.AssistantMessage, error) {
	m, err := scanAssistantMessage(s.db.QueryRowContext(ctx,
		`SELECT `+assistantMsgCols+` FROM assistant_messages m
		 JOIN assistant_threads t ON t.id = m.thread_id
		 WHERE t.restaurant_id = $1 AND m.id = $2`, restaurantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AssistantMessage{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.AssistantMessage{}, fmt.Errorf("store: assistant message by id: %w", err)
	}
	return m, nil
}

func (s *Store) SetAssistantMessageStatus(ctx context.Context, restaurantID, id uuid.UUID, status string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE assistant_messages m SET action_status = $1
		 FROM assistant_threads t
		 WHERE t.id = m.thread_id AND t.restaurant_id = $2 AND m.id = $3 AND m.action_status IS NULL`,
		status, restaurantID, id)
	if err != nil {
		return fmt.Errorf("store: set message status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Either wrong tenant/id (looks identical) or already decided.
		if _, lookErr := s.AssistantMessageByID(ctx, restaurantID, id); lookErr != nil {
			return lookErr
		}
		return fmt.Errorf("actions already decided: %w", ports.ErrConflict)
	}
	return nil
}
