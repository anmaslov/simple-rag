package db

import (
	"context"
	"encoding/json"

	"confluence-rag/backend/internal/models"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) EnsureChatSession(ctx context.Context, sessionID, title string) (string, error) {
	if sessionID != "" {
		return sessionID, nil
	}
	var id string
	err := r.pool.QueryRow(ctx, "INSERT INTO chat_sessions(title) VALUES($1) RETURNING id::text", truncate(title, 80)).Scan(&id)
	return id, err
}

func (r *Repository) SaveChatMessage(ctx context.Context, sessionID, role, content string, sources json.RawMessage) error {
	if sources == nil {
		sources = json.RawMessage("[]")
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO chat_messages(session_id,role,content,sources_json) VALUES($1,$2,$3,$4)", sessionID, role, content, sources); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE chat_sessions SET updated_at=now() WHERE id=$1", sessionID)
		return err
	})
}

func (r *Repository) ListChatSessions(ctx context.Context) ([]models.ChatSession, error) {
	rows, err := r.pool.Query(ctx, "SELECT id::text,title,created_at,updated_at FROM chat_sessions ORDER BY updated_at DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ChatSession
	for rows.Next() {
		var v models.ChatSession
		if err := rows.Scan(&v.ID, &v.Title, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) ListChatMessages(ctx context.Context, sessionID string) ([]models.ChatMessage, error) {
	rows, err := r.pool.Query(ctx, "SELECT id,session_id::text,role,content,sources_json,created_at FROM chat_messages WHERE session_id=$1 ORDER BY created_at,id", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ChatMessage
	for rows.Next() {
		var v models.ChatMessage
		var raw json.RawMessage
		if err := rows.Scan(&v.ID, &v.SessionID, &v.Role, &v.Content, &raw, &v.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &v.Sources); err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteChatSession(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM chat_sessions WHERE id=$1", id)
	return err
}
