package storage

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type Friend struct {
	PublicKey string
	Alias     string
	Online    bool
	Unread    int32
}

type Message struct {
	ID        string
	ClientID  string
	PeerKey   string
	Sender    string
	Text      string
	Timestamp string
	Delivery  string
	Error     string
	ServerSeq int64
}

type OutboxItem struct {
	ID          string
	ReceiverKey string
	Text        string
	Attempts    int
	NextRetryAt string
	Error       string
	Status      int
	CreatedAt   string
}

type Store struct {
	db    *sqlx.DB
	mu    sync.RWMutex
	owner string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SetOwner(publicKey string) error {
	publicKey = strings.TrimSpace(publicKey)
	s.mu.Lock()
	s.owner = publicKey
	s.mu.Unlock()
	return nil
}

func (s *Store) SaveFriends(friends []Friend) error {
	owner := s.ownerKey()
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	stmt, err := tx.Preparex(`
		INSERT INTO friends(owner_pub_key, pub_key, alias, is_online, unread, updated_at)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_pub_key, pub_key) DO UPDATE SET
			alias = excluded.alias,
			is_online = excluded.is_online,
			unread = excluded.unread,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, friend := range friends {
		if _, err := stmt.Exec(owner, friend.PublicKey, friend.Alias, boolInt(friend.Online), friend.Unread, now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Friends() ([]Friend, error) {
	owner := s.ownerKey()
	var rows []friendRow
	if err := s.db.Select(&rows, `
		SELECT pub_key, alias, is_online, unread
		FROM friends
		WHERE owner_pub_key = ?
		ORDER BY alias COLLATE NOCASE, pub_key
	`, owner); err != nil {
		return nil, err
	}

	friends := make([]Friend, 0, len(rows))
	for _, row := range rows {
		friends = append(friends, row.friend())
	}
	return friends, nil
}

func (s *Store) SaveMessages(peerKey string, messages []Message) error {
	owner := s.ownerKey()
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	stmt, err := tx.Preparex(`
		INSERT INTO messages(owner_pub_key, id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_pub_key, id) DO UPDATE SET
			client_msg_id = excluded.client_msg_id,
			peer_pub_key = excluded.peer_pub_key,
			sender_pub_key = excluded.sender_pub_key,
			text = excluded.text,
			timestamp = excluded.timestamp,
			delivery = excluded.delivery,
			error = excluded.error,
			server_seq = excluded.server_seq,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, message := range messages {
		if message.ID == "" {
			continue
		}
		if message.PeerKey == "" {
			message.PeerKey = peerKey
		}
		if message.ClientID == "" {
			message.ClientID = message.ID
		}
		if _, err := stmt.Exec(
			owner,
			message.ID,
			message.ClientID,
			message.PeerKey,
			message.Sender,
			message.Text,
			message.Timestamp,
			message.Delivery,
			message.Error,
			message.ServerSeq,
			now,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Messages(peerKey string, limit int) ([]Message, error) {
	messages, _, _, err := s.History(peerKey, "", limit, "older")
	return messages, err
}

func (s *Store) History(peerKey string, cursor string, limit int, direction string) ([]Message, string, bool, error) {
	owner := s.ownerKey()
	var rows []messageRow
	if limit <= 0 {
		limit = 50
	}
	if strings.EqualFold(strings.TrimSpace(direction), "newer") {
		cursorSeq, hasCursor, err := parseCursor(cursor)
		if err != nil {
			return nil, "", false, err
		}
		query := `
			SELECT id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq
			FROM messages
			WHERE owner_pub_key = ? AND peer_pub_key = ?
			ORDER BY server_seq ASC, timestamp ASC
			LIMIT ?
		`
		args := []any{owner, peerKey, limit + 1}
		if hasCursor {
			query = `
				SELECT id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq
				FROM messages
				WHERE owner_pub_key = ? AND peer_pub_key = ? AND server_seq > ?
				ORDER BY server_seq ASC, timestamp ASC
				LIMIT ?
			`
			args = []any{owner, peerKey, cursorSeq, limit + 1}
		}
		if err := s.db.Select(&rows, query, args...); err != nil {
			return nil, "", false, err
		}
		messages := make([]Message, 0, min(len(rows), limit+1))
		for _, row := range rows {
			messages = append(messages, row.message())
		}
		hasMore := len(rows) > limit
		if hasMore {
			messages = messages[:limit]
		}
		nextCursor := ""
		if hasMore && len(messages) > 0 {
			nextCursor = strconv.FormatInt(messages[len(messages)-1].ServerSeq, 10)
		}
		return messages, nextCursor, hasMore, nil
	}

	cursorSeq, hasCursor, err := parseCursor(cursor)
	if err != nil {
		return nil, "", false, err
	}
	query := `
			SELECT id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq
			FROM (
				SELECT id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq
				FROM messages
				WHERE owner_pub_key = ? AND peer_pub_key = ?
				ORDER BY server_seq DESC, timestamp DESC
				LIMIT ?
			)
			ORDER BY server_seq ASC, timestamp ASC
	`
	args := []any{owner, peerKey, limit + 1}
	if hasCursor {
		query = `
			SELECT id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq
			FROM (
				SELECT id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq
				FROM messages
				WHERE owner_pub_key = ? AND peer_pub_key = ? AND server_seq < ?
				ORDER BY server_seq DESC, timestamp DESC
				LIMIT ?
			)
			ORDER BY server_seq ASC, timestamp ASC
		`
		args = []any{owner, peerKey, cursorSeq, limit + 1}
	}
	if err := s.db.Select(&rows, query, args...); err != nil {
		return nil, "", false, err
	}

	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, row.message())
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[1:]
	}
	nextCursor := ""
	if hasMore && len(messages) > 0 {
		nextCursor = strconv.FormatInt(messages[0].ServerSeq, 10)
	}
	return messages, nextCursor, hasMore, nil
}

func (s *Store) MaxServerSeq(peerKey string) (int64, error) {
	owner := s.ownerKey()
	var seq int64
	err := s.db.QueryRow(`
		SELECT COALESCE(MAX(server_seq), 0)
		FROM messages
		WHERE owner_pub_key = ? AND peer_pub_key = ? AND server_seq > 0
	`, owner, peerKey).Scan(&seq)
	return seq, err
}

func (s *Store) FirstServerSeqGap(peerKey string) (previousSeq int64, missingSeq int64, ok bool, err error) {
	owner := s.ownerKey()
	var rows []struct {
		ServerSeq int64 `db:"server_seq"`
	}
	if err := s.db.Select(&rows, `
		SELECT DISTINCT server_seq
		FROM messages
		WHERE owner_pub_key = ? AND peer_pub_key = ? AND server_seq > 0
		ORDER BY server_seq ASC
	`, owner, peerKey); err != nil {
		return 0, 0, false, err
	}
	var previous int64
	for _, row := range rows {
		if previous > 0 && row.ServerSeq > previous+1 {
			return previous, previous + 1, true, nil
		}
		previous = row.ServerSeq
	}
	return 0, 0, false, nil
}

func (s *Store) MessageByClientID(clientID string) (Message, error) {
	owner := s.ownerKey()
	var row messageRow
	if err := s.db.Get(&row, `
		SELECT id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq
		FROM messages
		WHERE owner_pub_key = ? AND (client_msg_id = ? OR id = ?)
		ORDER BY server_seq DESC, timestamp DESC
		LIMIT 1
	`, owner, clientID, clientID); err != nil {
		return Message{}, err
	}
	return row.message(), nil
}

func (s *Store) MarkConversationRead(peerKey string, seq int64) error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`
		UPDATE friends
		SET unread = 0, last_seen_seq = MAX(last_seen_seq, ?), updated_at = ?
		WHERE owner_pub_key = ? AND pub_key = ?
	`, seq, time.Now().UTC().Format(time.RFC3339), owner, peerKey)
	return err
}

func (s *Store) EnqueueOutbox(item OutboxItem, senderKey string) error {
	owner := s.ownerKey()
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO outbox(owner_pub_key, id, receiver_pub_key, text, attempts, next_retry_at, error, status, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(owner_pub_key, id) DO UPDATE SET
			receiver_pub_key = excluded.receiver_pub_key,
			text = excluded.text,
			attempts = excluded.attempts,
			next_retry_at = excluded.next_retry_at,
			error = excluded.error,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, owner, item.ID, item.ReceiverKey, item.Text, item.Attempts, item.NextRetryAt, item.Error, item.Status, item.CreatedAt, item.CreatedAt); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO messages(owner_pub_key, id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)
		ON CONFLICT(owner_pub_key, id) DO UPDATE SET
			delivery = excluded.delivery,
			error = excluded.error,
			updated_at = excluded.updated_at
	`, owner, item.ID, item.ID, item.ReceiverKey, senderKey, item.Text, item.CreatedAt, "pending", item.Error, item.CreatedAt); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) DueOutbox(now string, limit int) ([]OutboxItem, error) {
	owner := s.ownerKey()
	if limit <= 0 {
		limit = 20
	}
	var rows []outboxRow
	if err := s.db.Select(&rows, `
		SELECT id, receiver_pub_key, text, attempts, next_retry_at, error, status, created_at
		FROM outbox
		WHERE owner_pub_key = ? AND status = 0 AND (next_retry_at = '' OR next_retry_at <= ?)
		ORDER BY created_at
		LIMIT ?
	`, owner, now, limit); err != nil {
		return nil, err
	}

	items := make([]OutboxItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.item())
	}
	return items, nil
}

func (s *Store) MarkOutboxSending(id string) error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`UPDATE outbox SET status = 1, error = '', updated_at = ? WHERE owner_pub_key = ? AND id = ?`, time.Now().UTC().Format(time.RFC3339), owner, id)
	return err
}

func (s *Store) MarkOutboxSent(clientID string, serverID string, timestamp string, serverSeq int64, senderKey string) error {
	owner := s.ownerKey()
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	var row outboxMessageRow
	if err := tx.Get(&row, `SELECT receiver_pub_key, text FROM outbox WHERE owner_pub_key = ? AND id = ?`, owner, clientID); err != nil {
		_ = tx.Rollback()
		return err
	}
	_, _ = tx.Exec(`DELETE FROM messages WHERE owner_pub_key = ? AND id = ?`, owner, clientID)
	if _, err := tx.Exec(`
		INSERT INTO messages(owner_pub_key, id, client_msg_id, peer_pub_key, sender_pub_key, text, timestamp, delivery, error, server_seq, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, 'sent', '', ?, ?)
		ON CONFLICT(owner_pub_key, id) DO UPDATE SET
			delivery = 'sent',
			error = '',
			server_seq = excluded.server_seq,
			updated_at = excluded.updated_at
	`, owner, serverID, clientID, row.ReceiverKey, senderKey, row.Text, timestamp, serverSeq, time.Now().UTC().Format(time.RFC3339)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM outbox WHERE owner_pub_key = ? AND id = ?`, owner, clientID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkOutboxRetry(id string, attempts int, nextRetryAt string, message string) error {
	owner := s.ownerKey()
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`UPDATE outbox SET attempts = ?, next_retry_at = ?, error = ?, status = 0, updated_at = ? WHERE owner_pub_key = ? AND id = ?`, attempts, nextRetryAt, message, now, owner, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE messages SET delivery = 'pending', error = ?, updated_at = ? WHERE owner_pub_key = ? AND (client_msg_id = ? OR id = ?)`, message, now, owner, id, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkOutboxFailed(id string, attempts int, message string) error {
	owner := s.ownerKey()
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`UPDATE outbox SET attempts = ?, next_retry_at = '', error = ?, status = 2, updated_at = ? WHERE owner_pub_key = ? AND id = ?`, attempts, message, now, owner, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`UPDATE messages SET delivery = 'failed', error = ?, updated_at = ? WHERE owner_pub_key = ? AND (client_msg_id = ? OR id = ?)`, message, now, owner, id, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) OutboxStatus() (pending int, failed int, err error) {
	owner := s.ownerKey()
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE owner_pub_key = ? AND status IN (0, 1)`, owner).Scan(&pending); err != nil {
		return 0, 0, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE owner_pub_key = ? AND status = 2`, owner).Scan(&failed); err != nil {
		return 0, 0, err
	}
	return pending, failed, nil
}

func (s *Store) RetryFailedOutbox() error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`UPDATE outbox SET status = 0, next_retry_at = '', error = '', updated_at = ? WHERE owner_pub_key = ? AND status = 2`, time.Now().UTC().Format(time.RFC3339), owner)
	return err
}

func (s *Store) ClearFailedOutbox() error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`
		DELETE FROM messages WHERE owner_pub_key = ? AND id IN (SELECT id FROM outbox WHERE owner_pub_key = ? AND status = 2);
		DELETE FROM outbox WHERE owner_pub_key = ? AND status = 2;
	`, owner, owner, owner)
	return err
}

func (s *Store) RecoverOutbox() error {
	owner := s.ownerKey()
	_, err := s.db.Exec(`UPDATE outbox SET status = 0, next_retry_at = '', updated_at = ? WHERE owner_pub_key = ? AND status = 1`, time.Now().UTC().Format(time.RFC3339), owner)
	return err
}

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
	pk := make([]string, 0, len(columns))
	for i := 1; i <= len(columns); i++ {
		for _, column := range columns {
			if column.PK == i {
				pk = append(pk, column.Name)
			}
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
	for _, column := range columns {
		if column.Name == name {
			return true
		}
	}
	return false
}

func (s *Store) ownerKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.owner
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseCursor(cursor string) (int64, bool, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, false, nil
	}
	seq, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil || seq < 0 {
		return 0, false, strconv.ErrSyntax
	}
	return seq, true, nil
}

type friendRow struct {
	PublicKey string `db:"pub_key"`
	Alias     string `db:"alias"`
	Online    int    `db:"is_online"`
	Unread    int32  `db:"unread"`
}

func (r friendRow) friend() Friend {
	return Friend{
		PublicKey: r.PublicKey,
		Alias:     r.Alias,
		Online:    r.Online != 0,
		Unread:    r.Unread,
	}
}

type messageRow struct {
	ID        string `db:"id"`
	ClientID  string `db:"client_msg_id"`
	PeerKey   string `db:"peer_pub_key"`
	Sender    string `db:"sender_pub_key"`
	Text      string `db:"text"`
	Timestamp string `db:"timestamp"`
	Delivery  string `db:"delivery"`
	Error     string `db:"error"`
	ServerSeq int64  `db:"server_seq"`
}

func (r messageRow) message() Message {
	return Message{
		ID:        r.ID,
		ClientID:  r.ClientID,
		PeerKey:   r.PeerKey,
		Sender:    r.Sender,
		Text:      r.Text,
		Timestamp: r.Timestamp,
		Delivery:  r.Delivery,
		Error:     r.Error,
		ServerSeq: r.ServerSeq,
	}
}

type outboxRow struct {
	ID          string `db:"id"`
	ReceiverKey string `db:"receiver_pub_key"`
	Text        string `db:"text"`
	Attempts    int    `db:"attempts"`
	NextRetryAt string `db:"next_retry_at"`
	Error       string `db:"error"`
	Status      int    `db:"status"`
	CreatedAt   string `db:"created_at"`
}

func (r outboxRow) item() OutboxItem {
	return OutboxItem{
		ID:          r.ID,
		ReceiverKey: r.ReceiverKey,
		Text:        r.Text,
		Attempts:    r.Attempts,
		NextRetryAt: r.NextRetryAt,
		Error:       r.Error,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
	}
}

type outboxMessageRow struct {
	ReceiverKey string `db:"receiver_pub_key"`
	Text        string `db:"text"`
}
