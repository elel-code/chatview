package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type Store struct {
	DB *sqlx.DB
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) ApplyMigrations(ctx context.Context, dir string) error {
	if _, err := s.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return err
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return err
	}
	slices.Sort(files)
	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), ".up.sql")
		var exists bool
		err := s.DB.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		tx, err := s.DB.BeginTxx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(sqlBytes)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, version)
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SeedAdmin(ctx context.Context, pubKey string) error {
	if pubKey == "" {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO users (pub_key, role, status)
		VALUES ($1, 1, 1)
		ON CONFLICT (pub_key) DO UPDATE SET role = 1, status = 1, updated_at = now()
	`, pubKey)
	return err
}

func (s *Store) CleanupStaleOnline(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET is_online = false WHERE is_online = true`)
	return err
}

func (s *Store) CleanupExpired(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM challenges WHERE expires_at < now();
		UPDATE sessions SET is_online = false WHERE expires_at < now();
		DELETE FROM sessions WHERE expires_at < now() - interval '24 hours';
	`)
	return err
}

func (s *Store) RunCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = s.CleanupExpired(context.Background())
		case <-ctx.Done():
			return
		}
	}
}

func NormalizeParticipants(a, b string) (string, string, error) {
	if a == "" || b == "" {
		return "", "", errors.New("pub_key is required")
	}
	if a == b {
		return "", "", errors.New("cannot chat with self")
	}
	if a < b {
		return a, b, nil
	}
	return b, a, nil
}

func EnsureConversation(ctx context.Context, tx *sqlx.Tx, userA, userB string) (string, error) {
	a, b, err := NormalizeParticipants(userA, userB)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.GetContext(ctx, &id, `
		INSERT INTO conversations (participant_a, participant_b)
		VALUES ($1, $2)
		ON CONFLICT (participant_a, participant_b)
		DO UPDATE SET participant_a = EXCLUDED.participant_a
		RETURNING id
	`, a, b)
	return id, err
}

func LookupConversation(ctx context.Context, q sqlx.QueryerContext, userA, userB string) (string, error) {
	a, b, err := NormalizeParticipants(userA, userB)
	if err != nil {
		return "", err
	}
	var id string
	err = sqlx.GetContext(ctx, q, &id, `
		SELECT id FROM conversations
		WHERE participant_a = $1 AND participant_b = $2
	`, a, b)
	return id, err
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func IsSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40001"
}

func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
