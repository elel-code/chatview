package storage

import (
	"cmp"
	"slices"
	"strings"
)

func (s *Store) init() error {
	if err := s.migrateOwnerSchema(); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS friends (
			owner_pub_key TEXT NOT NULL DEFAULT '',
			pub_key TEXT NOT NULL,
			alias TEXT NOT NULL DEFAULT '',
			is_online INTEGER NOT NULL DEFAULT 0,
			unread INTEGER NOT NULL DEFAULT 0,
			last_seen_seq INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_pub_key, pub_key)
		);

		CREATE TABLE IF NOT EXISTS messages (
			owner_pub_key TEXT NOT NULL DEFAULT '',
			id TEXT NOT NULL,
			client_msg_id TEXT NOT NULL DEFAULT '',
			peer_pub_key TEXT NOT NULL,
			sender_pub_key TEXT NOT NULL,
			text TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			delivery TEXT NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			server_seq INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_pub_key, id)
		);

		CREATE INDEX IF NOT EXISTS idx_messages_peer_seq
			ON messages(owner_pub_key, peer_pub_key, server_seq);

		CREATE TABLE IF NOT EXISTS outbox (
			owner_pub_key TEXT NOT NULL DEFAULT '',
			id TEXT NOT NULL,
			receiver_pub_key TEXT NOT NULL,
			text TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			next_retry_at TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			status INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_pub_key, id)
		);

		CREATE INDEX IF NOT EXISTS idx_outbox_status_retry
			ON outbox(owner_pub_key, status, next_retry_at);
	`)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`ALTER TABLE messages ADD COLUMN client_msg_id TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_messages_peer_seq ON messages(owner_pub_key, peer_pub_key, server_seq)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_outbox_status_retry ON outbox(owner_pub_key, status, next_retry_at)`)
	return nil
}

func (s *Store) migrateOwnerSchema() error {
	if err := s.migrateFriendsSchema(); err != nil {
		return err
	}
	if err := s.migrateMessagesSchema(); err != nil {
		return err
	}
	return s.migrateOutboxSchema()
}

func (s *Store) migrateFriendsSchema() error {
	if !s.tableExists("friends") || s.primaryKeyColumns("friends") == "owner_pub_key,pub_key" {
		return nil
	}
	_, err := s.db.Exec(`
		ALTER TABLE friends RENAME TO friends_legacy;
		CREATE TABLE friends (
			owner_pub_key TEXT NOT NULL DEFAULT '',
			pub_key TEXT NOT NULL,
			alias TEXT NOT NULL DEFAULT '',
			is_online INTEGER NOT NULL DEFAULT 0,
			unread INTEGER NOT NULL DEFAULT 0,
			last_seen_seq INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_pub_key, pub_key)
		);
		INSERT OR REPLACE INTO friends(owner_pub_key, pub_key, alias, is_online, unread, last_seen_seq, updated_at)
			SELECT '', pub_key, alias, is_online, unread, last_seen_seq, updated_at
			FROM friends_legacy;
		DROP TABLE friends_legacy;
	`)
	return err
}

func (s *Store) migrateMessagesSchema() error {
	if !s.tableExists("messages") || s.primaryKeyColumns("messages") == "owner_pub_key,id" {
		return nil
	}
	if !s.columnExists("messages", "client_msg_id") {
		_, _ = s.db.Exec(`ALTER TABLE messages ADD COLUMN client_msg_id TEXT NOT NULL DEFAULT ''`)
	}
	_, err := s.db.Exec(`
		DROP INDEX IF EXISTS idx_messages_peer_seq;
		ALTER TABLE messages RENAME TO messages_legacy;
		CREATE TABLE messages (
			owner_pub_key TEXT NOT NULL DEFAULT '',
			id TEXT NOT NULL,
			client_msg_id TEXT NOT NULL DEFAULT '',
			peer_pub_key TEXT NOT NULL,
			sender_pub_key TEXT NOT NULL,
			text TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			delivery TEXT NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			server_seq INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_pub_key, id)
		);
		INSERT OR REPLACE INTO messages(owner_pub_key, id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq, updated_at)
			SELECT '', id, COALESCE(NULLIF(client_msg_id, ''), id), peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq, updated_at
			FROM messages_legacy;
		DROP TABLE messages_legacy;
	`)
	return err
}

func (s *Store) migrateOutboxSchema() error {
	if !s.tableExists("outbox") || s.primaryKeyColumns("outbox") == "owner_pub_key,id" {
		return nil
	}
	_, err := s.db.Exec(`
		DROP INDEX IF EXISTS idx_outbox_status_retry;
		ALTER TABLE outbox RENAME TO outbox_legacy;
		CREATE TABLE outbox (
			owner_pub_key TEXT NOT NULL DEFAULT '',
			id TEXT NOT NULL,
			receiver_pub_key TEXT NOT NULL,
			text TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			next_retry_at TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			status INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(owner_pub_key, id)
		);
		INSERT OR REPLACE INTO outbox(owner_pub_key, id, receiver_pub_key, text, attempts, next_retry_at, error, status, created_at, updated_at)
			SELECT '', id, receiver_pub_key, text, attempts, next_retry_at, error, status, created_at, updated_at
			FROM outbox_legacy;
		DROP TABLE outbox_legacy;
	`)
	return err
}

func (s *Store) tableExists(name string) bool {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

func (s *Store) primaryKeyColumns(table string) string {
	type columnInfo struct {
		Name string `db:"name"`
		PK   int    `db:"pk"`
	}
	var columns []columnInfo
	if err := s.db.Select(&columns, `PRAGMA table_info(`+table+`)`); err != nil {
		return ""
	}
	slices.SortFunc(columns, func(a, b columnInfo) int {
		return cmp.Compare(a.PK, b.PK)
	})
	pk := make([]string, 0, len(columns))
	for _, column := range columns {
		if column.PK > 0 {
			pk = append(pk, column.Name)
		}
	}
	return strings.Join(pk, ",")
}

func (s *Store) columnExists(table string, name string) bool {
	type columnInfo struct {
		Name string `db:"name"`
	}
	var columns []columnInfo
	if err := s.db.Select(&columns, `PRAGMA table_info(`+table+`)`); err != nil {
		return false
	}
	return slices.ContainsFunc(columns, func(column columnInfo) bool {
		return column.Name == name
	})
}
