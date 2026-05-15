package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"chatview/internal/contextx"
)

func (s *Store) AuthenticateToken(ctx context.Context, header string) (contextx.Principal, error) {
	token := strings.TrimSpace(header)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if token == "" {
		return contextx.Principal{}, sql.ErrNoRows
	}

	var principal contextx.Principal
	err := s.DB.GetContext(ctx, &principal, `
		SELECT s.pub_key, u.role, s.token
		FROM sessions s
		JOIN users u ON u.pub_key = s.pub_key
		WHERE s.token = $1 AND s.expires_at > now() AND u.status = 1
	`, token)
	if err != nil {
		return contextx.Principal{}, err
	}
	return principal, nil
}

func (s *Store) MarkSessionClient(ctx context.Context, token, clientID string, online bool) error {
	if token == "" {
		return errors.New("session token is required")
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE sessions
		SET client_id = $2, is_online = $3
		WHERE token = $1 AND expires_at > now()
	`, token, clientID, online)
	return err
}

func (s *Store) IsUserOnline(ctx context.Context, pubKey string) (bool, error) {
	var online bool
	err := s.DB.GetContext(ctx, &online, `
		SELECT EXISTS(
			SELECT 1 FROM sessions
			WHERE pub_key = $1 AND is_online = true AND expires_at > now()
		)
	`, pubKey)
	return online, err
}

func (s *Store) HealPresence(ctx context.Context, onlinePubKeys map[string]bool) error {
	rows, err := s.DB.QueryxContext(ctx, `SELECT DISTINCT pub_key FROM sessions WHERE is_online = true`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var pubKey string
		if err := rows.Scan(&pubKey); err != nil {
			return err
		}
		if !onlinePubKeys[pubKey] {
			if _, err := s.DB.ExecContext(ctx, `UPDATE sessions SET is_online = false WHERE pub_key = $1`, pubKey); err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

func SessionExpires(ttl time.Duration) time.Time {
	return time.Now().UTC().Add(ttl)
}
