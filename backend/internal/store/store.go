// Package store — доступ к PostgreSQL через pgx (ADR 0001): пул, транзакции и запросы.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/zbekxzz/gulder/backend/migrations"
)

// ErrNotFound — запись не найдена (или принадлежит другому пользователю: §13, чужой ID → 404).
var ErrNotFound = errors.New("not found")

// Store — обёртка над пулом соединений.
type Store struct {
	Pool *pgxpool.Pool
}

// Open создаёт пул и проверяет соединение.
func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{Pool: pool}, nil
}

// Close закрывает пул.
func (s *Store) Close() { s.Pool.Close() }

// Querier — общий интерфейс пула и транзакции.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// InTx выполняет fn в одной транзакции (§18.2): commit при успехе, rollback при ошибке.
func (s *Store) InTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func newGoose(db *sql.DB) (*goose.Provider, error) {
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return nil, fmt.Errorf("goose provider: %w", err)
	}
	return p, nil
}

// Migrate применяет миграции вверх (или down/reset) — отдельный шаг перед запуском (§18.6).
func Migrate(ctx context.Context, url, direction string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()
	p, err := newGoose(db)
	if err != nil {
		return err
	}
	switch direction {
	case "up":
		_, err = p.Up(ctx)
	case "down":
		_, err = p.Down(ctx)
	case "reset":
		_, err = p.DownTo(ctx, 0)
	default:
		return fmt.Errorf("unknown direction %q", direction)
	}
	if err != nil {
		return fmt.Errorf("migrate %s: %w", direction, err)
	}
	return nil
}

// MigrationsCurrent сообщает, применены ли все миграции (для /readyz).
func (s *Store) MigrationsCurrent(ctx context.Context) (bool, error) {
	db := stdlib.OpenDBFromPool(s.Pool)
	defer func() { _ = db.Close() }()
	p, err := newGoose(db)
	if err != nil {
		return false, err
	}
	pending, err := p.HasPending(ctx)
	if err != nil {
		return false, fmt.Errorf("check pending migrations: %w", err)
	}
	return !pending, nil
}
