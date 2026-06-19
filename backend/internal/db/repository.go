package db

import (
	"confluence-rag/backend/internal/domain"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

var (
	_ domain.Repository            = (*Repository)(nil)
	_ domain.ConnectionsRepository = (*Repository)(nil)
	_ domain.ScopesRepository      = (*Repository)(nil)
	_ domain.DocumentsRepository   = (*Repository)(nil)
	_ domain.SpacesRepository      = (*Repository)(nil)
	_ domain.PagesRepository       = (*Repository)(nil)
	_ domain.SyncJobsRepository    = (*Repository)(nil)
	_ domain.ChatRepository        = (*Repository)(nil)
	_ domain.SearchRepository      = (*Repository)(nil)
)
