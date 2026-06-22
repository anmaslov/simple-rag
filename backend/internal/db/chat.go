package db

import (
	"context"
	"encoding/json"

	"confluence-rag/backend/internal/models"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) EnsureChatSession(ctx context.Context, sessionID, title string) (string, error) {
	if sessionID != "" {
		return sessionID, nil
	}
	q, args, err := psql.Insert("chat_sessions").
		Columns("title").
		Values(truncate(title, 80)).
		Suffix("RETURNING id::text").
		ToSql()
	if err != nil {
		return "", err
	}
	var id string
	err = r.pool.QueryRow(ctx, q, args...).Scan(&id)
	return id, err
}

func (r *Repository) SaveChatMessage(ctx context.Context, sessionID, role, content string, sources json.RawMessage) error {
	if sources == nil {
		sources = json.RawMessage("[]")
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q, args, err := psql.Insert("chat_messages").
			Columns("session_id", "role", "content", "sources_json").
			Values(sessionID, role, content, sources).
			ToSql()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return err
		}
		q, args, err = psql.Update("chat_sessions").
			Set("updated_at", sq.Expr("now()")).
			Where(sq.Eq{"id": sessionID}).
			ToSql()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, q, args...)
		return err
	})
}

func (r *Repository) ListChatSessions(ctx context.Context) ([]models.ChatSession, error) {
	q, args, err := psql.Select("id::text", "title", "created_at", "updated_at").
		From("chat_sessions").
		OrderBy("updated_at DESC").
		Limit(100).
		ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, q, args...)
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
	q, args, err := psql.Select("id", "session_id::text", "role", "content", "sources_json", "created_at").
		From("chat_messages").
		Where(sq.Eq{"session_id": sessionID}).
		OrderBy("created_at", "id").
		ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, q, args...)
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
	q, args, err := psql.Delete("chat_sessions").Where(sq.Eq{"id": id}).ToSql()
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, q, args...)
	return err
}
