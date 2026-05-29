package storage

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	defer tx.Rollback()

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
		return err
	}
	defer stmt.Close()

	now := nowRFC3339()
	for _, friend := range friends {
		if _, err := stmt.Exec(owner, friend.PublicKey, friend.Alias, boolInt(friend.Online), friend.Unread, now); err != nil {
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

	return mapSlice(rows, friendRow.friend), nil
}

func (s *Store) SaveMessages(peerKey string, messages []Message) error {
	owner := s.ownerKey()
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

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
		return err
	}
	defer stmt.Close()

	now := nowRFC3339()
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
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ownerKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.owner
}
